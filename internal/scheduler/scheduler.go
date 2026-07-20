package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/haioa/krono-job/internal/model"
)

// Scheduler 封装 Asynq Scheduler，从 jobs.yaml 加载任务定义，并通过 fsnotify
// 监听文件变更实现热重载（增/删/改：先删后注册，PLAN §6.3）。
//
// 并发模型：Asynq Scheduler 本身并发安全，可在运行期动态 Register/Unregister。
// entries 映射由本结构自有锁保护，避免热重载与初始加载竞态。
type Scheduler struct {
	redis *redis.Client
	log   *zap.SugaredLogger
	path  string

	asynq *asynq.Scheduler

	mu      sync.Mutex
	entries map[string]string // task_type -> Asynq entryID

	watcher   *fsnotify.Watcher
	shutdownCh chan struct{}
	doneCh     chan struct{}
}

// New 构造一个调度器。redisClient 与仓库共用同一连接池（asynq Shutdown 对共享连接为 no-op）。
func New(redisClient *redis.Client, jobsPath string, log *zap.SugaredLogger) *Scheduler {
	return &Scheduler{
		redis:      redisClient,
		log:        log,
		path:       jobsPath,
		entries:    make(map[string]string),
		shutdownCh: make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// Start 加载 jobs.yaml、注册任务、启动 Asynq Scheduler 与文件热重载监听。
// 非阻塞：Asynq 在后台 goroutine 调度，热重载在独立 goroutine 中循环。
func (s *Scheduler) Start() error {
	s.asynq = asynq.NewSchedulerFromRedisClient(s.redis, &asynq.SchedulerOpts{
		// 使用本地时区，使 cron 表达更直观；默认 UTC。
		Location: time.Local,
	})

	// 初次加载：即使失败也仅告警，保留空调度（PLAN §6.3 保护）。
	if err := s.reload(); err != nil {
		s.log.Errorw("initial jobs load failed, scheduler started with no jobs", "path", s.path, "err", err)
	}

	s.asynq.Start()

	// fsnotify 监听文件所在目录（覆盖编辑器整文件替换产生的 Create 事件）。
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}
	s.watcher = w
	dir := filepath.Dir(s.path)
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		s.watcher = nil
		return fmt.Errorf("watch dir %s: %w", dir, err)
	}

	go s.watchLoop()
	s.log.Infow("scheduler started", "watch", s.path)
	return nil
}

// reload 重新解析 jobs.yaml，并按差异对 Asynq 注册表做增/删/改。
// 解析失败直接返回 error（调用方保留旧配置）；解析成功则全量对账。
func (s *Scheduler) reload() error {
	defs, err := LoadJobsFile(s.path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newSet := make(map[string]JobDef, len(defs))
	for _, d := range defs {
		newSet[d.TaskType] = d
	}

	// 1) 删除已不存在的任务。
	for taskType, entryID := range s.entries {
		if _, ok := newSet[taskType]; !ok {
			if err := s.asynq.Unregister(entryID); err != nil {
				s.log.Warnw("unregister job failed", "task_type", taskType, "err", err)
			} else {
				s.log.Infow("job removed", "task_type", taskType)
			}
			delete(s.entries, taskType)
		}
	}

	// 2) 新增或变更（先删后注册，避免重复）；enabled=false 则确保未注册。
	for taskType, def := range newSet {
		if oldID, ok := s.entries[taskType]; ok {
			_ = s.asynq.Unregister(oldID)
			delete(s.entries, taskType)
		}
		if !def.IsEnabled() {
			s.log.Infow("job disabled, not registered", "task_type", taskType)
			continue
		}
		entryID, err := s.register(def)
		if err != nil {
			s.log.Errorw("register job failed", "task_type", taskType, "err", err)
			continue
		}
		s.entries[taskType] = entryID
	}
	return nil
}

// register 把单条任务定义注册进 Asynq Scheduler（解析 ${ENV} 后作为任务载荷）。
func (s *Scheduler) register(def JobDef) (string, error) {
	resolved := def.ResolveEnv()
	payload, err := json.Marshal(resolved)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	task := asynq.NewTask(DispatchTaskType, payload,
		asynq.MaxRetry(def.Retry),
		asynq.Timeout(def.TimeoutDuration()),
		asynq.Queue(model.QueueDefault),
	)
	entryID, err := s.asynq.Register(def.Cron, task)
	if err != nil {
		return "", fmt.Errorf("asynq register: %w", err)
	}
	s.log.Infow("job registered",
		"task_type", def.TaskType, "cron", def.Cron,
		"protocol", def.Protocol, "retry", def.Retry, "timeout", def.TimeoutDuration())
	return entryID, nil
}

// watchLoop 监听文件写事件，做 300ms 防抖后触发 reload。
func (s *Scheduler) watchLoop() {
	defer close(s.doneCh)

	var timer *time.Timer
	trigger := make(chan struct{}, 1)
	fire := func() {
		select {
		case trigger <- struct{}{}:
		default:
		}
	}

	for {
		select {
		case <-s.shutdownCh:
			return
		case ev, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(ev.Name) != filepath.Clean(s.path) {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(300*time.Millisecond, fire)
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			s.log.Warnw("fsnotify error", "err", err)
		case <-trigger:
			if err := s.reload(); err != nil {
				s.log.Errorw("hot reload failed, keep previous config", "err", err)
			} else {
				s.log.Info("jobs.yaml hot reloaded")
			}
		}
	}
}

// Shutdown 停止热重载监听并优雅关停 Asynq Scheduler。
func (s *Scheduler) Shutdown() {
	if s.watcher != nil {
		_ = s.watcher.Close()
		s.watcher = nil
	}
	close(s.shutdownCh)
	if s.asynq != nil {
		s.asynq.Shutdown()
	}
	<-s.doneCh
}
