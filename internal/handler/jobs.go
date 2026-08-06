package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
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
	Name      string   `json:"name"`
	TaskType  string   `json:"task_type"`
	Cron      string   `json:"cron"`
	Protocol  string   `json:"protocol"`
	Enabled   bool     `json:"enabled"`
	Paused    bool     `json:"paused"`
	Endpoint  string   `json:"endpoint"`            // 单节点地址（endpoints 为空时的回退）
	Endpoints []string `json:"endpoints"`           // 分布式多节点地址列表（优先级高于 endpoint）
	Method    string   `json:"method"`
	Retry     int      `json:"retry"`
	Timeout   string   `json:"timeout"`
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
			Paused:    paused,
			Endpoint:  d.Endpoint,
			Endpoints: d.Endpoints,
			Method:    d.Method,
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

// JobDefView 是可视化编辑用的完整任务定义（含 comment 备注），与 scheduler.JobDef 对应。
type JobDefView struct {
	Name        string            `json:"name"`
	TaskType    string            `json:"task_type"`
	Cron        string            `json:"cron"`
	Protocol    string            `json:"protocol"`
	Enabled     *bool             `json:"enabled"`
	Timeout     string            `json:"timeout"`
	Retry       int               `json:"retry"`
	Endpoint    string            `json:"endpoint"`
	Endpoints   []string          `json:"endpoints"`
	LBStrategy  string            `json:"lb_strategy"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Payload     map[string]any    `json:"payload"`
	GRPCService string            `json:"grpc_service"`
	GRPCMethod  string            `json:"grpc_method"`
	RequestType string            `json:"request_type"`
	Metadata    map[string]string `json:"metadata"`
	Comment     string            `json:"comment"`
}

func toJobDefView(d scheduler.JobDef) JobDefView {
	return JobDefView{
		Name:        d.Name,
		TaskType:    d.TaskType,
		Cron:        d.Cron,
		Protocol:    d.Protocol,
		Enabled:     d.Enabled,
		Timeout:     d.Timeout,
		Retry:       d.Retry,
		Endpoint:    d.Endpoint,
		Endpoints:   d.Endpoints,
		LBStrategy:  d.LBStrategy,
		Method:      d.Method,
		Headers:     d.Headers,
		Payload:     d.Payload,
		GRPCService: d.GRPCService,
		GRPCMethod:  d.GRPCMethod,
		RequestType: d.RequestType,
		Metadata:    d.Metadata,
		Comment:     d.Comment,
	}
}

func fromJobDefView(v JobDefView) scheduler.JobDef {
	return scheduler.JobDef{
		Name:        v.Name,
		TaskType:    v.TaskType,
		Cron:        v.Cron,
		Protocol:    v.Protocol,
		Enabled:     v.Enabled,
		Timeout:     v.Timeout,
		Retry:       v.Retry,
		Endpoint:    v.Endpoint,
		Endpoints:   v.Endpoints,
		LBStrategy:  v.LBStrategy,
		Method:      v.Method,
		Headers:     v.Headers,
		Payload:     v.Payload,
		GRPCService: v.GRPCService,
		GRPCMethod:  v.GRPCMethod,
		RequestType: v.RequestType,
		Metadata:    v.Metadata,
		Comment:     v.Comment,
	}
}

// ListRaw 处理 GET /api/jobs/raw：返回 jobs.yaml 完整定义（含 comment 备注），供可视化表单回显。
func (h *JobsHandler) ListRaw(c *gin.Context) {
	defs, err := scheduler.LoadJobsFile(h.jobsPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 jobs 定义失败: " + err.Error()})
		return
	}
	views := make([]JobDefView, 0, len(defs))
	for _, d := range defs {
		views = append(views, toJobDefView(d))
	}
	c.JSON(http.StatusOK, gin.H{"list": views, "total": len(views)})
}

// Create 处理 POST /api/jobs：向 jobs.yaml 追加一条新任务。
// 采用「读全量 -> 校验去重 -> 追加 -> 整体写回」策略，保证注释与既有任务不被破坏。
func (h *JobsHandler) Create(c *gin.Context) {
	var v JobDefView
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体非法: " + err.Error()})
		return
	}
	if err := scheduler.ValidateJob(fromJobDefView(v)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "校验失败: " + err.Error()})
		return
	}

	defs, err := scheduler.LoadJobsFile(h.jobsPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 jobs 定义失败: " + err.Error()})
		return
	}
	for _, d := range defs {
		if d.TaskType == v.TaskType {
			c.JSON(http.StatusConflict, gin.H{"error": "task_type 已存在: " + v.TaskType})
			return
		}
	}
	defs = append(defs, fromJobDefView(v))
	if err := scheduler.SaveJobsFile(h.jobsPath, defs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入 jobs.yaml 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_type": v.TaskType, "message": "已创建"})
}

// Update 处理 PUT /api/jobs/:task_type：替换 jobs.yaml 中指定任务。
func (h *JobsHandler) Update(c *gin.Context) {
	taskType := c.Param("task_type")
	var v JobDefView
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体非法: " + err.Error()})
		return
	}
	// 路径上的 task_type 优先级最高，防止误改标识。
	if v.TaskType == "" {
		v.TaskType = taskType
	}
	if v.TaskType != taskType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径 task_type 与请求体不一致"})
		return
	}
	if err := scheduler.ValidateJob(fromJobDefView(v)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "校验失败: " + err.Error()})
		return
	}

	defs, err := scheduler.LoadJobsFile(h.jobsPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 jobs 定义失败: " + err.Error()})
		return
	}
	idx := -1
	for i := range defs {
		if defs[i].TaskType == taskType {
			idx = i
			break
		}
	}
	if idx < 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task_type 不存在: " + taskType})
		return
	}
	defs[idx] = fromJobDefView(v)
	if err := scheduler.SaveJobsFile(h.jobsPath, defs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入 jobs.yaml 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_type": taskType, "message": "已更新"})
}

// Delete 处理 DELETE /api/jobs/:task_type：从 jobs.yaml 移除指定任务。
func (h *JobsHandler) Delete(c *gin.Context) {
	taskType := c.Param("task_type")
	if taskType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_type 必填"})
		return
	}
	defs, err := scheduler.LoadJobsFile(h.jobsPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 jobs 定义失败: " + err.Error()})
		return
	}
	idx := -1
	for i := range defs {
		if defs[i].TaskType == taskType {
			idx = i
			break
		}
	}
	if idx < 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task_type 不存在: " + taskType})
		return
	}
	defs = append(defs[:idx], defs[idx+1:]...)
	if err := scheduler.SaveJobsFile(h.jobsPath, defs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入 jobs.yaml 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_type": taskType, "message": "已删除"})
}

// cronPreviewRequest 是 CronPreview 的请求体。
type cronPreviewRequest struct {
	Cron  string `json:"cron"`
	Count int    `json:"count"`
}

// CronPreview 处理 POST /api/jobs/cron-preview：根据 cron 表达式计算最近若干次执行时间。
// 复用与调度内核一致的 robfig/cron/v3 标准 5 字段解析，时区默认 Asia/Shanghai（与调度一致）。
func (h *JobsHandler) CronPreview(c *gin.Context) {
	var req cronPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体非法: " + err.Error()})
		return
	}
	if req.Cron == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cron 表达式不能为空"})
		return
	}
	if req.Count <= 0 || req.Count > 20 {
		req.Count = 5
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.Local
	}
	sched, err := cron.ParseStandard(req.Cron)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cron 表达式不合法: " + err.Error()})
		return
	}

	now := time.Now().In(loc)
	list := make([]string, 0, req.Count)
	t := now
	for i := 0; i < req.Count; i++ {
		next := sched.Next(t)
		if next.IsZero() {
			break
		}
		list = append(list, next.Format("2006-01-02 15:04:05"))
		t = next
	}
	c.JSON(http.StatusOK, gin.H{"cron": req.Cron, "timezone": loc.String(), "list": list})
}
