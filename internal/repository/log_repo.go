package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/haioa/krono-job/internal/model"
)

// InsertExecLog 写入一条调度执行日志，成功后回写 id、created_at。
// response_body 超过 8KB 自动截断（决策 11），避免大响应撑爆日志表。
func (r *Repository) InsertExecLog(ctx context.Context, l *model.JobExecLog) error {
	if r.PG == nil {
		return ErrDBUnavailable
	}
	body := l.ResponseBody
	if len(body) > model.MaxResponseBodyBytes {
		body = body[:model.MaxResponseBodyBytes]
	}
	sql := `INSERT INTO ` + r.JobExecLogTable() + `
		(task_type, trigger_type, protocol, target_endpoint, status, response_body, error_msg, retry_count, start_at, end_at, execution_duration_ms)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at`
	err := r.PG.QueryRow(ctx, sql,
		l.TaskType, l.TriggerType, l.Protocol, l.TargetEndpoint, l.Status,
		body, l.ErrorMsg, l.RetryCount, l.StartAt, l.EndAt, l.ExecutionDurationMs,
	).Scan(&l.ID, &l.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert job_exec_log: %w", err)
	}
	return nil
}

// DeleteExecLogs 批量删除指定 id 的执行日志，返回实际删除条数。
// ids 为空时直接返回 0（不执行 SQL）。使用 id = ANY($1) 占位符配合 pgx 数组编码，
// 既避免逐个拼接带来的 SQL 注入风险，也只需一次往返。
func (r *Repository) DeleteExecLogs(ctx context.Context, ids []string) (int64, error) {
	if r.PG == nil {
		return 0, ErrDBUnavailable
	}
	if len(ids) == 0 {
		return 0, nil
	}
	sql := `DELETE FROM ` + r.JobExecLogTable() + ` WHERE id = ANY($1)`
	tag, err := r.PG.Exec(ctx, sql, ids)
	if err != nil {
		return 0, fmt.Errorf("delete job_exec_log: %w", err)
	}
	return tag.RowsAffected(), nil
}

// allowedSortFields 是日志排序字段白名单，防止 SQL 注入。
var allowedSortFields = map[string]bool{
	"created_at": true,
	"start_at":   true,
	"task_type":  true,
	"status":     true,
	"protocol":   true,
	"retry_count": true,
}

// QueryLogs 按条件分页查询日志，返回总数与当前页列表。
// 排序字段/方向均做白名单约束；过滤条件与分页参数使用占位符，避免注入。
func (r *Repository) QueryLogs(ctx context.Context, q *model.LogQuery) (*model.PageResult, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}

	var where []string
	var args []interface{}
	n := 1
	addCond := func(cond string, val interface{}) {
		where = append(where, fmt.Sprintf(cond, n))
		args = append(args, val)
		n++
	}
	if q.TaskType != "" {
		addCond("task_type = $%d", q.TaskType)
	}
	if q.Status != "" {
		addCond("status = $%d", q.Status)
	}
	if q.Protocol != "" {
		addCond("protocol = $%d", q.Protocol)
	}
	if q.Start != nil {
		addCond("created_at >= $%d", *q.Start)
	}
	if q.End != nil {
		// 结束日含当天全天：用次日零点作为开区间上界。
		endExclusive := q.End.AddDate(0, 0, 1)
		addCond("created_at < $%d", endExclusive)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := r.PG.QueryRow(ctx, "SELECT count(*) FROM "+r.JobExecLogTable()+whereClause, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count job_exec_log: %w", err)
	}

	sortField := "created_at"
	if q.SortField != "" && allowedSortFields[q.SortField] {
		sortField = q.SortField
	}
	sortOrder := "DESC"
	if o := strings.ToUpper(q.SortOrder); o == "ASC" || o == "DESC" {
		sortOrder = o
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// 列表不返回 response_body（数据量大），仅详情接口 GetLogByID 返回。
	listSQL := `SELECT id, task_type, trigger_type, protocol, target_endpoint, status,
	                   error_msg, retry_count, start_at, end_at, execution_duration_ms, created_at
	            FROM ` + r.JobExecLogTable() + whereClause +
		fmt.Sprintf(" ORDER BY %s %s", sortField, sortOrder) +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, pageSize, offset)

	rows, err := r.PG.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query job_exec_log: %w", err)
	}
	defer rows.Close()

	list := make([]model.JobExecLog, 0, pageSize)
	for rows.Next() {
		var l model.JobExecLog
		if err := rows.Scan(
			&l.ID, &l.TaskType, &l.TriggerType, &l.Protocol, &l.TargetEndpoint, &l.Status,
			&l.ErrorMsg, &l.RetryCount, &l.StartAt, &l.EndAt, &l.ExecutionDurationMs, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan job_exec_log: %w", err)
		}
		list = append(list, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job_exec_log: %w", err)
	}

	return &model.PageResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		List:     list,
	}, nil
}

// DistinctTaskTypes 返回日志表中出现过的所有 task_type（去重、按字典序排列），
// 用于前端日志过滤下拉框，避免用户手工输入。
func (r *Repository) DistinctTaskTypes(ctx context.Context) ([]string, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	rows, err := r.PG.Query(ctx, "SELECT DISTINCT task_type FROM "+r.JobExecLogTable()+" WHERE task_type <> '' ORDER BY task_type")
	if err != nil {
		return nil, fmt.Errorf("distinct task_type: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, 8)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan task_type: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task_type: %w", err)
	}
	return out, nil
}

// GetLogByID 按主键查询单条执行日志（含完整 response_body，未经截断），用于日志详情页。
// 未找到时返回 (nil, nil)。
func (r *Repository) GetLogByID(ctx context.Context, id string) (*model.JobExecLog, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	sql := `SELECT id, task_type, trigger_type, protocol, target_endpoint, status,
	               response_body, error_msg, retry_count, start_at, end_at, execution_duration_ms, created_at
	        FROM ` + r.JobExecLogTable() + ` WHERE id = $1`
	var l model.JobExecLog
	if err := r.PG.QueryRow(ctx, sql, id).Scan(
		&l.ID, &l.TaskType, &l.TriggerType, &l.Protocol, &l.TargetEndpoint, &l.Status,
		&l.ResponseBody, &l.ErrorMsg, &l.RetryCount, &l.StartAt, &l.EndAt, &l.ExecutionDurationMs, &l.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get job_exec_log by id: %w", err)
	}
	return &l, nil
}

// buildStatsWhere 根据可选起止时间构造 created_at 区间过滤的 WHERE 片段与占位符参数。
// 状态过滤使用常量字符串字面量（非占位符），避免与区间占位符编号冲突；状态值来自本代码常量，安全。
func buildStatsWhere(f *model.StatsFilter) (string, []interface{}) {
	if f == nil {
		return "", nil
	}
	var conds []string
	var args []interface{}
	n := 1
	add := func(cond string, v interface{}) {
		conds = append(conds, fmt.Sprintf(cond, n))
		args = append(args, v)
		n++
	}
	if f.Start != nil {
		add("created_at >= $%d", *f.Start)
	}
	if f.End != nil {
		// 结束日视为「当天全天」，故用次日零点作为开区间上界（created_at < end+1 天），
		// 否则 end=2026-07-20 只命中当天 00:00:00 之前，整日数据都会漏掉。
		endExclusive := f.End.AddDate(0, 0, 1)
		add("created_at < $%d", endExclusive)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// GetExecStats 返回执行结果总览：成功/失败/跳过总数（可按时间范围过滤）。
func (r *Repository) GetExecStats(ctx context.Context, f *model.StatsFilter) (*model.StatsOverview, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	where, args := buildStatsWhere(f)
	sql := `SELECT
		COUNT(*) FILTER (WHERE status = 'success') AS success,
		COUNT(*) FILTER (WHERE status = 'failed')  AS failed,
		COUNT(*) FILTER (WHERE status = 'skipped') AS skipped,
		COUNT(*) AS total
	FROM ` + r.JobExecLogTable() + where
	var o model.StatsOverview
	if err := r.PG.QueryRow(ctx, sql, args...).Scan(&o.Success, &o.Failed, &o.Skipped, &o.Total); err != nil {
		return nil, fmt.Errorf("stats overview: %w", err)
	}
	return &o, nil
}

// GetDailyExecStats 按天聚合执行结果（成功/失败/跳过），可按时间范围过滤，按天升序返回。
func (r *Repository) GetDailyExecStats(ctx context.Context, f *model.StatsFilter) ([]model.DailyStat, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	where, args := buildStatsWhere(f)
	sql := `SELECT
		to_char(created_at, 'YYYY-MM-DD') AS day,
		COUNT(*) FILTER (WHERE status = 'success') AS success,
		COUNT(*) FILTER (WHERE status = 'failed')  AS failed,
		COUNT(*) FILTER (WHERE status = 'skipped') AS skipped,
		COUNT(*) AS total
	FROM ` + r.JobExecLogTable() + where + `
	GROUP BY day
	ORDER BY day ASC`
	rows, err := r.PG.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("daily stats: %w", err)
	}
	defer rows.Close()

	list := make([]model.DailyStat, 0, 16)
	for rows.Next() {
		var d model.DailyStat
		if err := rows.Scan(&d.Day, &d.Success, &d.Failed, &d.Skipped, &d.Total); err != nil {
			return nil, fmt.Errorf("scan daily stats: %w", err)
		}
		list = append(list, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily stats: %w", err)
	}
	return list, nil
}

// GetTaskExecRanking 按 task_type 聚合调用次数与结果分布，按总调用数降序返回排行榜（可按时间范围过滤）。
// limit 用于控制返回条数，越界时被收敛到 [1,100]。
func (r *Repository) GetTaskExecRanking(ctx context.Context, f *model.StatsFilter, limit int) ([]model.TaskRank, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	where, args := buildStatsWhere(f)
	args = append(args, limit)
	limitIdx := len(args)
	sql := `SELECT
		task_type,
		COUNT(*) FILTER (WHERE status = 'success') AS success,
		COUNT(*) FILTER (WHERE status = 'failed')  AS failed,
		COUNT(*) FILTER (WHERE status = 'skipped') AS skipped,
		COUNT(*) AS total
	FROM ` + r.JobExecLogTable() + where + `
	GROUP BY task_type
	ORDER BY total DESC
	LIMIT $` + fmt.Sprint(limitIdx)
	rows, err := r.PG.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("task ranking: %w", err)
	}
	defer rows.Close()

	list := make([]model.TaskRank, 0, limit)
	for rows.Next() {
		var t model.TaskRank
		if err := rows.Scan(&t.TaskType, &t.Success, &t.Failed, &t.Skipped, &t.Total); err != nil {
			return nil, fmt.Errorf("scan task ranking: %w", err)
		}
		list = append(list, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task ranking: %w", err)
	}
	return list, nil
}
