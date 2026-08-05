// Package cluster 提供 krono-job 分布式运行所需的两大基础能力：
//
//  1. Leader 选举（leader.go）：基于 Redis 的原子锁（SET NX）竞选唯一 Leader。
//     只有 Leader 节点运行 Asynq Scheduler（负责触发 krono:dispatch 任务），
//     其余节点作为纯 Worker 消费任务。从而避免多节点重复触发同一任务，
//     同时在 Leader 宕机时自动failover 到其它节点，实现调度层高可用。
//
//  2. 节点注册中心（registry.go）：被调用方节点（下游执行节点）自注册到 Redis，
//     通过心跳续租维持 TTL，过期自动剔除。Worker 在分发任务前据此过滤存活节点，
//     配合 loadbalance 包实现多节点负载均衡。
package cluster

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Leader 基于 Redis 的选主实现。非阻塞式：通过后台 goroutine 持续尝试抢占锁，
// 抢到即为 Leader，锁过期（未续租成功）则退位；其它节点随即竞争。
type Leader struct {
	rdb      *redis.Client
	key      string // Redis key，所有节点竞争同一把锁
	identity string // 本节点唯一标识（例如 hostname+pid 或随机 uuid）
	ttl      time.Duration
	refresh  time.Duration
	onChange func(isLeader bool)
	quit     chan struct{}
	current  bool
}

// LeaderOption 可选配置。
type LeaderOption func(*Leader)

// WithLeaderTTL 设置选主锁的持有 TTL（应大于 refresh 间隔，留足续租余量）。
func WithLeaderTTL(ttl time.Duration) LeaderOption {
	return func(l *Leader) { l.ttl = ttl }
}

// WithLeaderRefresh 设置续租/重试间隔。
func WithLeaderRefresh(interval time.Duration) LeaderOption {
	return func(l *Leader) { l.refresh = interval }
}

// WithLeaderChangeHook 注册主节点状态变化回调（isLeader=true 表示本节点刚成为 Leader）。
func WithLeaderChangeHook(fn func(isLeader bool)) LeaderOption {
	return func(l *Leader) { l.onChange = fn }
}

// NewLeader 构造一个选主器。key 为选主锁键（建议含业务前缀，如 "krono:leader"），
// identity 为本节点唯一标识。
func NewLeader(rdb *redis.Client, key, identity string, opts ...LeaderOption) *Leader {
	l := &Leader{
		rdb:      rdb,
		key:      key,
		identity: identity,
		ttl:      10 * time.Second,
		refresh:  3 * time.Second,
		quit:     make(chan struct{}),
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Run 启动选主循环（阻塞直到 ctx 取消或 Stop 调用）。
// 通过 onChange 回调异步通知主节点状态，调用方据此启动/停止 Scheduler。
func (l *Leader) Run(ctx context.Context) {
	ticker := time.NewTicker(l.refresh)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.stepDown(ctx)
			return
		case <-l.quit:
			l.stepDown(ctx)
			return
		case <-ticker.C:
			l.tryAcquireOrRenew(ctx)
		}
	}
}

// tryAcquireOrRenew 尝试抢占锁或续租。
func (l *Leader) tryAcquireOrRenew(ctx context.Context) {
	// 使用 Lua 保证「仅当持有者为本节点时才允许续租」，避免锁过期后被他人抢占后本节点误续租。
	script := redis.NewScript(`
		local cur = redis.call('GET', KEYS[1])
		if cur == false then
			-- 未持有：尝试抢占
			if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'PX', ARGV[2]) then
				return 1
			end
			return 0
		end
		if cur == ARGV[1] then
			-- 已是本节点持有：续租
			redis.call('PEXPIRE', KEYS[1], ARGV[2])
			return 1
		end
		-- 被其它节点持有
		return 0
	`)
	res, err := script.Run(ctx, l.rdb, []string{l.key}, l.identity, int(l.ttl.Milliseconds())).Int()
	if err != nil && err != redis.Nil {
		// 网络错误：保守地认为失去 Leader 资格，触发退位回调。
		l.setLeader(false)
		return
	}
	l.setLeader(res == 1)
}

// setLeader 更新本节点主状态并在变化时触发回调。
func (l *Leader) setLeader(isLeader bool) {
	if isLeader == l.current {
		return
	}
	l.current = isLeader
	if l.onChange != nil {
		l.onChange(isLeader)
	}
}

// stepDown 主动释放锁（若为本节点持有）。
func (l *Leader) stepDown(ctx context.Context) {
	script := redis.NewScript(`
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('DEL', KEYS[1])
		end
		return 0
	`)
	_, _ = script.Run(ctx, l.rdb, []string{l.key}, l.identity).Int()
	l.setLeader(false)
}

// IsLeader 返回本节点当前是否为主节点。
func (l *Leader) IsLeader() bool {
	return l.current
}

// Stop 停止选主循环。
func (l *Leader) Stop() {
	close(l.quit)
}

// Identity 返回本节点标识。
func (l *Leader) Identity() string {
	return l.identity
}
