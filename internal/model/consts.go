package model

// Redis Key 常量（决策 1-A：暂停态存 Redis Set，保留 PG 仅 2 张表约束）
const (
	// RedisKeyPausedTasks 是 Redis Set 的 key，成员为被暂停的 task_type 字符串。
	RedisKeyPausedTasks = "krono:paused_tasks"
)

// 默认数据库模式（schema）。PostgreSQL 为「库-模式-表」三级结构，
// 所有 SQL 均显式限定 schema（默认 public），避免依赖连接的 search_path。
const DefaultSchema = "public"

// 表名（不含 schema），与迁移脚本 scripts/migrations/000001_init_schema.up.sql 保持一致。
const (
	TableSysUser    = "sys_user"
	TableJobExecLog = "job_exec_log"
)

// QualifiedTableName 返回 "schema.table" 形式的完全限定表名。
func QualifiedTableName(schema, table string) string {
	if schema == "" {
		schema = DefaultSchema
	}
	return schema + "." + table
}

// sys_user.status 取值
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// job_exec_log.trigger_type 取值
const (
	TriggerTypeCron   = "cron"
	TriggerTypeManual = "manual"
)

// job_exec_log.protocol 取值
const (
	ProtocolHTTP = "http"
	ProtocolGRPC = "grpc"
)

// job_exec_log.status 取值
const (
	ExecStatusSuccess = "success"
	ExecStatusFailed  = "failed"
	ExecStatusSkipped = "skipped"
)

// MaxResponseBodyBytes 响应体写入前的截断上限（决策 11：超过 8KB 截断）。
const MaxResponseBodyBytes = 8 * 1024

// QueueDefault 是调度任务入队与 Worker（M4）消费的默认队列名。
// Asynq Scheduler 与 Worker 必须约定同一队列才能对接。
const QueueDefault = "default"
