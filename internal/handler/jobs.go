package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/haioa/krono-job/internal/model"
	"github.com/haioa/krono-job/internal/repository"
	"github.com/haioa/krono-job/internal/scheduler"
)

// JobsHandler 处理任务管理接口：列出任务、暂停/恢复、手动执行。
// paused 状态由 Redis Set 实时读取/写入（决策 1-A），与调度内核/Worker 共用同一数据源。
type JobsHandler struct {
	repo     *repository.Repository
	jobsPath string
	client   *asynq.Client   // 用于手动执行时把任务立即投递到队列
	log      *zap.SugaredLogger
}

// NewJobsHandler 构造任务管理 Handler。
func NewJobsHandler(repo *repository.Repository, jobsPath string, client *asynq.Client, log *zap.SugaredLogger) *JobsHandler {
	return &JobsHandler{repo: repo, jobsPath: jobsPath, client: client, log: log}
}

// JobView 是 GET /api/jobs 返回的精简任务视图。
type JobView struct {
	Name     string `json:"name"`
	TaskType string `json:"task_type"`
	Cron     string `json:"cron"`
	Protocol string `json:"protocol"`
	Enabled  bool   `json:"enabled"`
	Paused   bool   `json:"paused"`
	Endpoint string `json:"endpoint"`
	Method   string `json:"method"`
	Retry    int    `json:"retry"`
	Timeout  string `json:"timeout"`
}

// List 处理 GET /api/jobs，列出 jobs.yaml 中的全部任务定义，并实时标注 paused 状态。
// paused 来自 Redis Set（krono:paused_tasks）；若 Redis 不可用则降级为全部未暂停。
func (h *JobsHandler) List(c *gin.Context) {
	defs, err := scheduler.LoadJobsFile(h.jobsPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 jobs 定义失败: " + err.Error()})
		return
	}

	pausedSet := map[string]struct{}{}
	if members, rerr := h.repo.PausedTaskMembers(c.Request.Context()); rerr == nil {
		for _, t := range members {
			pausedSet[t] = struct{}{}
		}
	}

	views := make([]JobView, 0, len(defs))
	for _, d := range defs {
		_, paused := pausedSet[d.TaskType]
		views = append(views, JobView{
			Name:     d.Name,
			TaskType: d.TaskType,
			Cron:     d.Cron,
			Protocol: d.Protocol,
			Enabled:  d.IsEnabled(),
			Paused:   paused,
			Endpoint: d.Endpoint,
			Method:   d.Method,
			Retry:    d.Retry,
			Timeout:  d.Timeout,
		})
	}
	c.JSON(http.StatusOK, gin.H{"list": views, "total": len(views)})
}

// Pause 处理 POST /api/jobs/:task_type/pause。
func (h *JobsHandler) Pause(c *gin.Context) { h.setPaused(c, true) }

// Resume 处理 POST /api/jobs/:task_type/resume。
func (h *JobsHandler) Resume(c *gin.Context) { h.setPaused(c, false) }

func (h *JobsHandler) setPaused(c *gin.Context, paused bool) {
	taskType := c.Param("task_type")
	if taskType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_type 必填"})
		return
	}

	// 校验 task_type 是否存在于 jobs.yaml，避免误操作未知任务。
	defs, err := scheduler.LoadJobsFile(h.jobsPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 jobs 定义失败: " + err.Error()})
		return
	}
	found := false
	for _, d := range defs {
		if d.TaskType == taskType {
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "task_type 不存在: " + taskType})
		return
	}

	ctx := c.Request.Context()
	var opErr error
	if paused {
		opErr = h.repo.PauseTask(ctx, taskType)
	} else {
		opErr = h.repo.ResumeTask(ctx, taskType)
	}
	if opErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "操作暂停态失败（Redis 不可用）: " + opErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_type": taskType, "paused": paused})
}

// Run 处理 POST /api/jobs/:task_type/run：立即把任务投递到 Asynq 队列（手动触发）。
// 手动触发不受暂停态影响（Worker 侧据此跳过 paused 检查），并写入 trigger_type='manual' 的日志。
// 该接口仅负责投递，执行结果稍后体现在 /api/logs（异步）。
func (h *JobsHandler) Run(c *gin.Context) {
	taskType := c.Param("task_type")
	// 调试日志：确认后端确实收到了手动执行请求（若看不到这条，说明前端请求未到达后端）。
	if h.log != nil {
		h.log.Infow("manual run requested", "task_type", taskType, "client", c.ClientIP())
	}
	if taskType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_type 必填"})
		return
	}
	if h.client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "任务队列客户端未初始化（Redis 不可用）"})
		return
	}

	// 校验 task_type 是否存在于 jobs.yaml。
	defs, err := scheduler.LoadJobsFile(h.jobsPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 jobs 定义失败: " + err.Error()})
		return
	}
	var def *scheduler.JobDef
	for i := range defs {
		if defs[i].TaskType == taskType {
			def = &defs[i]
			break
		}
	}
	if def == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task_type 不存在: " + taskType})
		return
	}

	// 解析 ${ENV} 并标记为手动触发，使 Worker 区分来源并绕过暂停态。
	resolved := def.ResolveEnv()
	resolved.TriggerType = model.TriggerTypeManual

	payload, err := json.Marshal(resolved)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化任务载荷失败: " + err.Error()})
		return
	}
	task := asynq.NewTask(scheduler.DispatchTaskType, payload,
		asynq.MaxRetry(def.Retry),
		asynq.Timeout(def.TimeoutDuration()),
		asynq.Queue(model.QueueDefault),
	)
	if _, err := h.client.Enqueue(task, asynq.ProcessIn(0)); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "投递任务失败（Redis 不可用）: " + err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"task_type":    taskType,
		"trigger_type": model.TriggerTypeManual,
		"message":      "已投递，稍后可在执行日志查看结果",
	})
}
