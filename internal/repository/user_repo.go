package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/haioa/krono-job/internal/model"
)

// ErrDBUnavailable 表示 PostgreSQL 连接池未初始化（如基础设施未启动）。
var ErrDBUnavailable = errors.New("postgres 连接池未初始化")

// ErrNotFound 表示按主键查询未找到记录（如用户已不存在）。
var ErrNotFound = errors.New("记录不存在")

// CreateUser 插入管理员账户，成功后回写由 DB 生成的 id。
func (r *Repository) CreateUser(ctx context.Context, u *model.SysUser) error {
	if r.PG == nil {
		return ErrDBUnavailable
	}
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	if u.Status == "" {
		u.Status = model.UserStatusActive
	}
	sql := `INSERT INTO ` + r.SysUserTable() + ` (username, password_hash, nickname, status, created_at, updated_at)
	             VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`
	if err := r.PG.QueryRow(ctx, sql,
		u.Username, u.PasswordHash, u.Nickname, u.Status, u.CreatedAt, u.UpdatedAt,
	).Scan(&u.ID); err != nil {
		return fmt.Errorf("insert sys_user: %w", err)
	}
	return nil
}

// GetUserByUsername 按用户名查询，未找到返回 (nil, nil)。
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*model.SysUser, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	u := &model.SysUser{}
	sql := `SELECT id, username, password_hash, nickname, status, created_at, updated_at
	             FROM ` + r.SysUserTable() + ` WHERE username = $1`
	err := r.PG.QueryRow(ctx, sql, username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query sys_user by username: %w", err)
	}
	return u, nil
}

// CountUsers 返回 sys_user 表行数（bootstrap 判空用）。
func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	if r.PG == nil {
		return 0, ErrDBUnavailable
	}
	var c int64
	if err := r.PG.QueryRow(ctx, `SELECT count(*) FROM `+r.SysUserTable()).Scan(&c); err != nil {
		return 0, fmt.Errorf("count sys_user: %w", err)
	}
	return c, nil
}

// ListUsers 返回全部管理员（MVP 管理用，按创建时间升序）。
func (r *Repository) ListUsers(ctx context.Context) ([]model.SysUser, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	rows, err := r.PG.Query(ctx, `SELECT id, username, password_hash, nickname, status, created_at, updated_at
	                             FROM `+r.SysUserTable()+` ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list sys_user: %w", err)
	}
	defer rows.Close()

	var users []model.SysUser
	for rows.Next() {
		var u model.SysUser
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan sys_user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sys_user: %w", err)
	}
	return users, nil
}

// UpdateUserPassword 更新密码哈希并刷新 updated_at。
func (r *Repository) UpdateUserPassword(ctx context.Context, id, hash string) error {
	if r.PG == nil {
		return ErrDBUnavailable
	}
	if _, err := r.PG.Exec(ctx, `UPDATE `+r.SysUserTable()+` SET password_hash=$1, updated_at=now() WHERE id=$2`, hash, id); err != nil {
		return fmt.Errorf("update sys_user password: %w", err)
	}
	return nil
}

// UpdateUserStatus 更新账号状态并刷新 updated_at。
func (r *Repository) UpdateUserStatus(ctx context.Context, id, status string) error {
	if r.PG == nil {
		return ErrDBUnavailable
	}
	if _, err := r.PG.Exec(ctx, `UPDATE `+r.SysUserTable()+` SET status=$1, updated_at=now() WHERE id=$2`, status, id); err != nil {
		return fmt.Errorf("update sys_user status: %w", err)
	}
	return nil
}

// GetUserByID 按主键查询管理员，未找到返回 (nil, nil)。
func (r *Repository) GetUserByID(ctx context.Context, id string) (*model.SysUser, error) {
	if r.PG == nil {
		return nil, ErrDBUnavailable
	}
	u := &model.SysUser{}
	sql := `SELECT id, username, password_hash, nickname, status, created_at, updated_at
	             FROM ` + r.SysUserTable() + ` WHERE id = $1`
	err := r.PG.QueryRow(ctx, sql, id).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query sys_user by id: %w", err)
	}
	return u, nil
}

// UpdateUser 更新管理员资料：nickname、status，以及（若提供非空）password_hash。
// 始终刷新 updated_at；未找到（影响 0 行）时返回 repository.ErrNotFound。
func (r *Repository) UpdateUser(ctx context.Context, u *model.SysUser) error {
	if r.PG == nil {
		return ErrDBUnavailable
	}
	var (
		sql  string
		args []interface{}
	)
	if u.PasswordHash != "" {
		sql = `UPDATE ` + r.SysUserTable() + ` SET nickname=$1, status=$2, password_hash=$3, updated_at=now() WHERE id=$4`
		args = []interface{}{u.Nickname, u.Status, u.PasswordHash, u.ID}
	} else {
		sql = `UPDATE ` + r.SysUserTable() + ` SET nickname=$1, status=$2, updated_at=now() WHERE id=$3`
		args = []interface{}{u.Nickname, u.Status, u.ID}
	}
	tag, err := r.PG.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("update sys_user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser 按主键删除管理员，返回实际删除条数。
func (r *Repository) DeleteUser(ctx context.Context, id string) (int64, error) {
	if r.PG == nil {
		return 0, ErrDBUnavailable
	}
	tag, err := r.PG.Exec(ctx, `DELETE FROM `+r.SysUserTable()+` WHERE id=$1`, id)
	if err != nil {
		return 0, fmt.Errorf("delete sys_user: %w", err)
	}
	return tag.RowsAffected(), nil
}
