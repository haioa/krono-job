package model

import "time"

// SysUser 管理员用户实体，对应 sys_user 表。
// id 使用 string 承载 uuid（pgx 原生支持 text 格式 uuid 扫描，便于直接 JSON 序列化）。
type SysUser struct {
	ID           string    `db:"id" json:"id"`
	Username     string    `db:"username" json:"username"`
	PasswordHash string    `db:"password_hash" json:"-"`
	Nickname     string    `db:"nickname" json:"nickname"`
	Status       string    `db:"status" json:"status"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// JobExecLog 调度执行日志实体，对应 job_exec_log 表。
// 一次调度事件写一条（决策 2-A）；ErrorMsg/EndAt/ExecutionDurationMs 可为空。
type JobExecLog struct {
	ID                string    `db:"id" json:"id"`
	TaskType          string    `db:"task_type" json:"task_type"`
	TriggerType       string    `db:"trigger_type" json:"trigger_type"`
	Protocol          string    `db:"protocol" json:"protocol"`
	TargetEndpoint    string    `db:"target_endpoint" json:"target_endpoint"`
	Status            string    `db:"status" json:"status"`
	ResponseBody      string    `db:"response_body" json:"response_body"`
	ErrorMsg          *string   `db:"error_msg" json:"error_msg"`
	RetryCount        int       `db:"retry_count" json:"retry_count"`
	StartAt           time.Time `db:"start_at" json:"start_at"`
	EndAt             *time.Time `db:"end_at" json:"end_at"`
	ExecutionDurationMs *int64  `db:"execution_duration_ms" json:"execution_duration_ms"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
}
