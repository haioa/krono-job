package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/haioa/krono-job/internal/model"
)

// ErrRedisUnavailable 表示 Redis 客户端未初始化（如基础设施未启动）。
var ErrRedisUnavailable = errors.New("redis 连接未初始化")

// IsTaskPaused 判断 task_type 是否处于暂停态（Redis Set 成员）。
func (r *Repository) IsTaskPaused(ctx context.Context, taskType string) (bool, error) {
	if r.Redis == nil {
		return false, ErrRedisUnavailable
	}
	ok, err := r.Redis.SIsMember(ctx, model.RedisKeyPausedTasks, taskType).Result()
	if err != nil {
		return false, fmt.Errorf("sismember paused: %w", err)
	}
	return ok, nil
}

// PauseTask 将 task_type 加入暂停态集合（Worker 分发前会命中该集合 → skipped）。
func (r *Repository) PauseTask(ctx context.Context, taskType string) error {
	if r.Redis == nil {
		return ErrRedisUnavailable
	}
	if err := r.Redis.SAdd(ctx, model.RedisKeyPausedTasks, taskType).Err(); err != nil {
		return fmt.Errorf("sadd paused: %w", err)
	}
	return nil
}

// ResumeTask 将 task_type 从暂停态集合移除。
func (r *Repository) ResumeTask(ctx context.Context, taskType string) error {
	if r.Redis == nil {
		return ErrRedisUnavailable
	}
	if err := r.Redis.SRem(ctx, model.RedisKeyPausedTasks, taskType).Err(); err != nil {
		return fmt.Errorf("srem paused: %w", err)
	}
	return nil
}

// PausedTaskMembers 返回当前暂停态集合的全部成员 task_type。
func (r *Repository) PausedTaskMembers(ctx context.Context) ([]string, error) {
	if r.Redis == nil {
		return nil, ErrRedisUnavailable
	}
	members, err := r.Redis.SMembers(ctx, model.RedisKeyPausedTasks).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers paused: %w", err)
	}
	return members, nil
}
