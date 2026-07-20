package handler

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/haioa/krono-job/internal/model"
	"github.com/haioa/krono-job/internal/repository"
	"github.com/haioa/krono-job/internal/scheduler"
)

// LogsHandler 处理执行日志查询接口（GET /api/logs）。
type LogsHandler struct {
	repo     *repository.Repository
	jobsPath string // jobs.yaml 路径，用于补齐"已配置但尚未产生日志"的任务类型
}

// NewLogsHandler 构造日志 Handler。
func NewLogsHandler(repo *repository.Repository, jobsPath string) *LogsHandler {
	return &LogsHandler{repo: repo, jobsPath: jobsPath}
}

// List 处理 GET /api/logs，支持分页/过滤/排序。
// 查询参数：page, page_size, task_type, status, protocol, start, end, sort, order。
func (h *LogsHandler) List(c *gin.Context) {
	q := &model.LogQuery{
		Page:      atoiDefault(c.Query("page"), 1),
		PageSize:  atoiDefault(c.Query("page_size"), 20),
		TaskType:  c.Query("task_type"),
		Status:    c.Query("status"),
		Protocol:  c.Query("protocol"),
		SortField: c.Query("sort"),
		SortOrder: c.Query("order"),
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
	if s := c.Query("start"); s != "" {
		if t, err := parseTimeParam(s); err == nil {
			q.Start = &t
		}
	}
	if e := c.Query("end"); e != "" {
		if t, err := parseTimeParam(e); err == nil {
			q.End = &t
		}
	}

	res, err := h.repo.QueryLogs(c.Request.Context(), q)
	if err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询日志失败"})
		return
	}
	c.JSON(http.StatusOK, res)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func parseTimeParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// YYYY-MM-DD 视为「本地时区当天 00:00:00」。
	// 不能按 UTC 解析，否则与数据库以绝对时刻存储的 created_at 相差本地时区偏移（如 UTC+8 差 8 小时），
	// 且起点会被整体前移，导致按天过滤查不到数据。
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), nil
	}
	return time.Time{}, errors.New("invalid time format")
}

// taskTypeOption 是可用于日志过滤的下拉项：value 为 task_type，label 为任务名 name。
type taskTypeOption struct {
	TaskType string `json:"task_type"`
	Name     string `json:"name"`
}

// TaskTypes 处理 GET /api/logs/task-types，返回可用于日志过滤的下拉列表。
// 来源：日志表中已出现过的 task_type，并合并 jobs.yaml 中已配置但尚无日志的任务类型。
// 每个 task_type 尽量显示 jobs.yaml 中配置的任务名 name，便于用户识别（而非直接展示 task_type 字符串）。
func (h *LogsHandler) TaskTypes(c *gin.Context) {
	rawTypes, err := h.repo.DistinctTaskTypes(c.Request.Context())
	if err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务类型失败"})
		return
	}

	// 建立 task_type -> 任务名 映射，优先使用 jobs.yaml 中配置的名称。
	nameByType := make(map[string]string)
	if defs, derr := scheduler.LoadJobsFile(h.jobsPath); derr == nil {
		for _, d := range defs {
			if d.TaskType == "" {
				continue
			}
			if _, ok := nameByType[d.TaskType]; !ok {
				nameByType[d.TaskType] = d.Name
			}
		}
	}

	seen := make(map[string]struct{})
	opts := make([]taskTypeOption, 0, len(rawTypes)+len(nameByType))
	add := func(tt string) {
		if tt == "" {
			return
		}
		if _, ok := seen[tt]; ok {
			return
		}
		seen[tt] = struct{}{}
		name := nameByType[tt]
		if name == "" {
			name = tt // 无对应任务名时回退为 task_type 本身
		}
		opts = append(opts, taskTypeOption{TaskType: tt, Name: name})
	}

	// 日志中已出现过的任务类型
	for _, t := range rawTypes {
		add(t)
	}
	// 补充 jobs.yaml 中已配置但尚无日志的任务类型
	for tt := range nameByType {
		add(tt)
	}

	// 按名称（其次 task_type）排序，便于阅读。
	sort.Slice(opts, func(i, j int) bool {
		if opts[i].Name != opts[j].Name {
			return opts[i].Name < opts[j].Name
		}
		return opts[i].TaskType < opts[j].TaskType
	})

	c.JSON(http.StatusOK, gin.H{"list": opts})
}

// DeleteLogsRequest 是 DELETE /api/logs 的请求体，携带待删除的日志 id 列表。
type DeleteLogsRequest struct {
	IDs []string `json:"ids"`
}

// Delete 处理 DELETE /api/logs，批量删除指定 id 的执行日志。
// 请求体：{"ids": ["id1","id2",...]}，ids 至少包含一个元素。
// 返回实际删除条数 deleted，可能与传入数量不一致（如部分 id 不存在）。
func (h *LogsHandler) Delete(c *gin.Context) {
	var req DeleteLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids 必填且不能为空"})
		return
	}
	// 去重，避免重复 id 影响计数与无意义重复执行。
	ids := dedupeStrings(req.IDs)

	affected, err := h.repo.DeleteExecLogs(c.Request.Context(), ids)
	if err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除日志失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"deleted": affected,
		"message": fmt.Sprintf("已删除 %d 条日志", affected),
	})
}

// dedupeStrings 对字符串切片去重并保持原顺序。
func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Detail 处理 GET /api/logs/:id，返回单条执行日志的完整详情（含完整响应体）。
func (h *LogsHandler) Detail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 必填"})
		return
	}

	log, err := h.repo.GetLogByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询日志详情失败"})
		return
	}
	if log == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "日志不存在"})
		return
	}
	c.JSON(http.StatusOK, log)
}
