package model

import "time"

// LogQuery 日志分页查询参数，对应 GET /api/logs 的过滤条件。
type LogQuery struct {
	Page      int        // 页码，从 1 开始
	PageSize  int        // 每页条数
	TaskType  string     // 按任务类型过滤（精确匹配）
	Status    string     // 按执行状态过滤：success/failed/skipped
	Protocol  string     // 按协议过滤：http/grpc
	Start     *time.Time // 按 created_at 区间下界过滤（含）
	End       *time.Time // 按 created_at 区间上界过滤（含）
	SortField string     // 排序字段（白名单约束在 repository 层）
	SortOrder string     // 排序方向：asc/desc
}

// PageResult 通用分页返回结构。
type PageResult struct {
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	List     interface{} `json:"list"`
}

// StatsFilter 统计接口通用的可选时间范围过滤（基于 created_at）。
type StatsFilter struct {
	Start *time.Time // 区间下界（含）
	End   *time.Time // 区间上界（含）
}

// StatsOverview 执行结果总览：成功/失败/跳过总数（可按时间范围过滤）。
type StatsOverview struct {
	Total   int64 `json:"total"`
	Success int64 `json:"success"`
	Failed  int64 `json:"failed"`
	Skipped int64 `json:"skipped"`
}

// DailyStat 单日执行统计行：按天聚合的成功/失败/跳过数。
type DailyStat struct {
	Day     string `json:"day"`     // YYYY-MM-DD
	Total   int64  `json:"total"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
	Skipped int64  `json:"skipped"`
}

// TaskRank 调用排行榜条目：按 task_type 聚合的调用次数与结果分布。
type TaskRank struct {
	TaskType string `json:"task_type" db:"task_type"`
	Name     string `json:"name"`
	Total    int64  `json:"total" db:"total"`
	Success  int64  `json:"success" db:"success"`
	Failed   int64  `json:"failed" db:"failed"`
	Skipped  int64  `json:"skipped" db:"skipped"`
}
