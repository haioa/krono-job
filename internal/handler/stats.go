package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/haioa/krono-job/internal/model"
	"github.com/haioa/krono-job/internal/repository"
	"github.com/haioa/krono-job/internal/scheduler"
)

// StatsHandler 处理执行日志统计接口（成功/失败/跳过总数、每日趋势、调用排行榜）。
type StatsHandler struct {
	repo     *repository.Repository
	jobsPath string // jobs.yaml 路径，用于为排行榜的 task_type 补齐可读任务名
}

// NewStatsHandler 构造统计 Handler。
func NewStatsHandler(repo *repository.Repository, jobsPath string) *StatsHandler {
	return &StatsHandler{repo: repo, jobsPath: jobsPath}
}

// parseRange 从查询参数解析可选时间范围（start/end），格式兼容 RFC3339 与 YYYY-MM-DD。
// 若 start/end 均未提供，则默认回落到「近 30 天」，避免无范围查询时拉取全量历史数据。
func (h *StatsHandler) parseRange(c *gin.Context) *model.StatsFilter {
	f := &model.StatsFilter{}
	if s := c.Query("start"); s != "" {
		if t, err := parseTimeParam(s); err == nil {
			f.Start = &t
		}
	}
	if e := c.Query("end"); e != "" {
		if t, err := parseTimeParam(e); err == nil {
			f.End = &t
		}
	}
	// 两者都缺省时强制近 30 天兜底。
	if f.Start == nil && f.End == nil {
		now := time.Now()
		start := now.AddDate(0, 0, -29) // 含今天共 30 天
		f.Start = &start
		f.End = &now
	}
	return f
}

// Overview 处理 GET /api/stats/overview?start=&end=，返回成功/失败/跳过总览。
func (h *StatsHandler) Overview(c *gin.Context) {
	f := h.parseRange(c)
	o, err := h.repo.GetExecStats(c.Request.Context(), f)
	if err != nil {
		h.fail(c, err, "查询统计概览失败")
		return
	}
	c.JSON(http.StatusOK, o)
}

// Daily 处理 GET /api/stats/daily?start=&end=，返回按天聚合的成功/失败/跳过趋势。
func (h *StatsHandler) Daily(c *gin.Context) {
	f := h.parseRange(c)
	list, err := h.repo.GetDailyExecStats(c.Request.Context(), f)
	if err != nil {
		h.fail(c, err, "查询每日统计失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"list": list})
}

// Ranking 处理 GET /api/stats/ranking?start=&end=&limit=，返回调用次数排行榜（含任务名）。
func (h *StatsHandler) Ranking(c *gin.Context) {
	f := h.parseRange(c)
	limit := atoiDefault(c.Query("limit"), 10)
	list, err := h.repo.GetTaskExecRanking(c.Request.Context(), f, limit)
	if err != nil {
		h.fail(c, err, "查询调用排行榜失败")
		return
	}

	// task_type -> 任务名（优先 jobs.yaml 配置名称），便于前端识别。
	nameByType := loadJobNameMap(h.jobsPath)
	for i := range list {
		if n := nameByType[list[i].TaskType]; n != "" {
			list[i].Name = n
		} else {
			list[i].Name = list[i].TaskType
		}
	}

	c.JSON(http.StatusOK, gin.H{"list": list})
}

// fail 统一错误处理：数据库不可用时返回 503，其余返回 500。
func (h *StatsHandler) fail(c *gin.Context, err error, msg string) {
	if errors.Is(err, repository.ErrDBUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}

// loadJobNameMap 读取 jobs.yaml，返回 task_type -> 任务名 映射（解析失败时返回空 map）。
func loadJobNameMap(path string) map[string]string {
	out := make(map[string]string)
	defs, err := scheduler.LoadJobsFile(path)
	if err != nil {
		return out
	}
	for _, d := range defs {
		if d.TaskType == "" {
			continue
		}
		if _, ok := out[d.TaskType]; !ok {
			out[d.TaskType] = d.Name
		}
	}
	return out
}
