package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/haioa/krono-job/internal/config"
	"github.com/haioa/krono-job/internal/model"
)

// Repository 持有 PG 连接池与 Redis 客户端，供后续各层复用。
type Repository struct {
	PG     *pgxpool.Pool
	Redis  *redis.Client
	Schema string // 数据库模式（schema），默认 public
}

// New 初始化 PG 与 Redis 连接。M0 阶段对连接失败采取"告警但不退出"策略，
// 便于在无基础设施时也能启动并验证 /healthz；生产可改为 fail-fast。
func New(ctx context.Context, cfg *config.Config, log *zap.SugaredLogger) (*Repository, error) {
	r := &Repository{}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Redis（懒连接，此处仅探活）
	r.Redis = redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := r.Redis.Ping(pingCtx).Err(); err != nil {
		log.Warnw("redis 暂不可达，继续启动", "addr", cfg.Redis.Addr, "err", err)
	}

	// PostgreSQL
	pool, err := pgxpool.New(pingCtx, cfg.Postgres.DSN())
	if err != nil {
		log.Warnw("postgres 暂不可达，继续启动", "dsn", cfg.Postgres.DSN(), "err", err)
	} else {
		if cfg.Postgres.MaxConns > 0 {
			pool.Config().MaxConns = cfg.Postgres.MaxConns
		}
		r.PG = pool
	}

	schema := cfg.Postgres.Schema
	if schema == "" {
		schema = model.DefaultSchema
	}
	r.Schema = schema

	return r, nil
}

// SysUserTable 返回完全限定的 sys_user 表名（schema.table）。
func (r *Repository) SysUserTable() string {
	return model.QualifiedTableName(r.Schema, model.TableSysUser)
}

// JobExecLogTable 返回完全限定的 job_exec_log 表名（schema.table）。
func (r *Repository) JobExecLogTable() string {
	return model.QualifiedTableName(r.Schema, model.TableJobExecLog)
}

// Close 释放底层连接。
func (r *Repository) Close() {
	if r.PG != nil {
		r.PG.Close()
	}
	if r.Redis != nil {
		_ = r.Redis.Close()
	}
}
