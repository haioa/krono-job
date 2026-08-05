// Package worker 实现 Asynq 任务执行器（M4）。
//
// 消费调度内核（M3）下发的 krono:dispatch 任务：分发前查 Redis 暂停态（命中→skipped），
// 再按 protocol 走 HTTP（httputil）或 gRPC（adapter 通用适配器），调用结束写一条 job_exec_log。
//
// 日志/重试关系遵循决策 2-A：每个调度事件最终落一条日志；success=成功，
// failed=耗尽重试（retry_count 为实际重试次数），skipped=被暂停跳过。
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/haioa/krono-job/internal/cluster"
	"github.com/haioa/krono-job/internal/loadbalance"
	"github.com/haioa/krono-job/internal/model"
	"github.com/haioa/krono-job/internal/repository"
	"github.com/haioa/krono-job/internal/scheduler"
	"github.com/haioa/krono-job/internal/worker/adapter"
	_ "github.com/haioa/krono-job/internal/worker/pbimports" // 副作用导入：注册下游 pb 描述符（决策 13）
	"github.com/haioa/krono-job/pkg/grpcpool"
	"github.com/haioa/krono-job/pkg/httputil"
)

// workerConcurrency 是 Worker 并发消费数。
const workerConcurrency = 10

// Worker 封装 Asynq Server 与 gRPC 连接池。
type Worker struct {
	srv    *asynq.Server
	mux    *asynq.ServeMux
	repo   *repository.Repository
	log    *zap.SugaredLogger
	pool   *grpcpool.Pool
	health         *cluster.NodeHealth // 调用方主动探测的下游存活缓存（心跳由调用方完成，下游无感知）
	nodeID         string              // 本节点标识，写入探测结果供观测
	skipVerifyTLS  bool                // HTTPS 下游跳过证书校验（内网自签/证书 IP 不匹配时直接访问）
}

// New 构造 Worker。redisOpt 通常基于 config.Redis 构造为 asynq.RedisClientOpt，
// 与仓库共用同一 Redis 实例（各自维护连接池）。
// health 可为 nil：为 nil 时 Worker 不感知节点存活，按配置 endpoints 直接调用（无 LB 过滤）。
func New(redisOpt asynq.RedisConnOpt, repo *repository.Repository, log *zap.SugaredLogger, health *cluster.NodeHealth, nodeID string, skipVerifyTLS bool) *Worker {
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: workerConcurrency,
		Queues:      map[string]int{model.QueueDefault: 1},
		// 优雅关停：等待在途任务完成，超时则回压任务到 Redis。
		ShutdownTimeout: 30 * time.Second,
	})
	w := &Worker{
		srv:           srv,
		repo:          repo,
		log:           log,
		pool:          grpcpool.New(),
		health:        health,
		nodeID:        nodeID,
		skipVerifyTLS: skipVerifyTLS,
	}
	w.mux = asynq.NewServeMux()
	w.mux.HandleFunc(scheduler.DispatchTaskType, w.handle)
	return w
}

// selectEndpoint 从任务配置的候选节点中按「存活过滤 + 负载均衡」选出实际调用目标。
// 存活判定完全由调用方主动探测（cluster.NodeHealth）完成：对候选 endpoint 做探活，
// 探活失败者剔除，再从存活节点中按 lb_strategy 选择。下游无需注册/上报任何心跳。
// 非分布式（仅单 endpoint）或 health 为 nil 时直接返回该 endpoint。
func (w *Worker) selectEndpoint(ctx context.Context, def scheduler.JobDef) (string, error) {
	candidates := def.TargetEndpoints()
	if len(candidates) == 0 {
		return "", fmt.Errorf("no endpoint configured for task %q", def.TaskType)
	}
	// 无 health（未启用分布式）或仅单节点：直接返回。
	if w.health == nil || len(candidates) == 1 {
		return candidates[0], nil
	}

	// 调用方主动探测：过滤出存活节点。
	alive := w.health.FilterLive(ctx, candidates, def.Protocol, w.nodeID)
	if len(alive) == 0 {
		w.log.Warnw("probe: no endpoint alive, fallback to all configured (retry downstream)",
			"task_type", def.TaskType)
		alive = candidates
	}
	return loadbalance.New(loadbalance.Strategy(def.LBStrategy)).Select(toCandidates(alive), def.TaskType)
}

// toCandidates 把 endpoint 列表转为负载均衡候选（least_conn 场景可在未来从 Registry 拉取 InFlight）。
func toCandidates(endpoints []string) []loadbalance.Candidate {
	out := make([]loadbalance.Candidate, 0, len(endpoints))
	for _, e := range endpoints {
		out = append(out, loadbalance.Candidate{Endpoint: e})
	}
	return out
}

// Start 非阻塞启动 Worker（在独立 goroutine 中消费队列）。
func (w *Worker) Start() {
	go func() {
		if err := w.srv.Start(w.mux); err != nil {
			w.log.Errorw("worker server start failed", "err", err)
		}
	}()
	w.log.Info("worker started")
}

// Shutdown 优雅关停 Worker 并回收 gRPC 连接。
func (w *Worker) Shutdown() {
	w.srv.Shutdown()
	w.pool.Close()
}

// handle 是 krono:dispatch 任务的统一处理器。
func (w *Worker) handle(ctx context.Context, task *asynq.Task) error {
	var def scheduler.JobDef
	if err := json.Unmarshal(task.Payload(), &def); err != nil {
		w.log.Errorw("decode dispatch payload failed", "err", err)
		return err // 载荷异常：交给 Asynq 重试/死信，保留现场
	}

	// 调试日志：确认 Worker 确实消费到了该任务（若看不到这条，说明任务未被 Worker 取出，
	// 通常是 Redis 不可达 / Worker 未启动 / 运行的是旧二进制）。
	w.log.Infow("dispatch received",
		"task_type", def.TaskType,
		"protocol", def.Protocol,
		"endpoint", def.Endpoint,
		"endpoints", def.TargetEndpoints(),
		"distributed", def.IsDistributed(),
		"trigger_type", def.TriggerType,
	)

	// 分布式选节点：存活过滤 + 负载均衡，结果写回 def.Endpoint 供后续 HTTP/gRPC 复用。
	target, err := w.selectEndpoint(ctx, def)
	if err != nil {
		w.log.Errorw("select endpoint failed", "task_type", def.TaskType, "err", err)
		return err
	}
	def.Endpoint = target

	rc, _ := asynq.GetRetryCount(ctx)
	maxR, _ := asynq.GetMaxRetry(ctx)
	startAt := time.Now()

	// 暂停态检查（决策 1-A）：命中则记 skipped 并跳过调用，不重试。
	// 手动触发（trigger_type=manual）不受暂停态影响，始终执行（PLAN 手动执行）。
	bypassPause := def.TriggerType == model.TriggerTypeManual
	if !bypassPause {
		paused, err := w.repo.Redis.SIsMember(ctx, model.RedisKeyPausedTasks, def.TaskType).Result()
		if err != nil {
			w.log.Warnw("check paused state failed, proceed anyway", "task_type", def.TaskType, "err", err)
		}
		if paused {
			w.log.Infow("task paused, skipped", "task_type", def.TaskType)
			w.writeLog(def, model.ExecStatusSkipped, "", "", 0, startAt, nil)
			return nil
		}
	}

	var (
		respBody string
		callErr  error
	)

	switch def.Protocol {
	case model.ProtocolHTTP:
		res, e := httputil.Do(ctx, def, w.skipVerifyTLS)
		if e != nil {
			callErr = e
			w.log.Warnw("http call failed", "task_type", def.TaskType, "endpoint", def.Endpoint, "err", e)
		} else {
			respBody = res.Body
			w.log.Infow("http call done",
				"task_type", def.TaskType, "endpoint", def.Endpoint, "status", res.StatusCode,
				"body_len", len(res.Body))
			if res.StatusCode >= 400 {
				callErr = fmt.Errorf("http status %d", res.StatusCode)
			}
		}
	case model.ProtocolGRPC:
		conn, e := w.pool.Get(def.Endpoint)
		if e != nil {
			callErr = e
			break
		}
		res, e := adapter.Invoke(ctx, def, conn)
		if e != nil {
			callErr = e
		} else {
			respBody = res.ReplyJSON
		}
	default:
		callErr = fmt.Errorf("unsupported protocol %q", def.Protocol)
	}

	endAt := time.Now()

	if callErr == nil {
		w.writeLog(def, model.ExecStatusSuccess, respBody, "", rc, startAt, &endAt)
		return nil
	}

	// 失败：末次重试记 failed 并结束；否则返回 err 让 Asynq 重试（本次不落库）。
	if rc >= maxR {
		w.writeLog(def, model.ExecStatusFailed, respBody, callErr.Error(), rc, startAt, &endAt)
	}
	return callErr
}

// writeLog 写一条 job_exec_log。PG 不可用时降级为日志告警，不影响主流程。
func (w *Worker) writeLog(def scheduler.JobDef, status, respBody, errMsg string, retryCount int, startAt time.Time, endAt *time.Time) {
	if w.repo.PG == nil {
		w.log.Warnw("PG unavailable, skip exec log", "task_type", def.TaskType, "status", status)
		return
	}
	logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// trigger_type 取任务载荷中的值，缺省回落为 cron（手动触发为 manual）。
	trigger := def.TriggerType
	if trigger == "" {
		trigger = model.TriggerTypeCron
	}

	l := &model.JobExecLog{
		TaskType:       def.TaskType,
		TriggerType:    trigger,
		Protocol:       def.Protocol,
		TargetEndpoint: def.Endpoint,
		Status:         status,
		ResponseBody:   respBody,
		RetryCount:     retryCount,
		StartAt:        startAt,
		EndAt:          endAt,
	}
	if errMsg != "" {
		l.ErrorMsg = &errMsg
	}
	if endAt != nil {
		d := endAt.Sub(startAt).Milliseconds()
		l.ExecutionDurationMs = &d
	}

	if err := w.repo.InsertExecLog(logCtx, l); err != nil {
		w.log.Errorw("insert exec log failed", "task_type", def.TaskType, "err", err)
	}
}
