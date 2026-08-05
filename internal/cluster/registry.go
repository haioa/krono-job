package cluster

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// NodeHealth 是调用方侧的下游节点健康缓存（探测驱动，零下游侵入，结果集中共享到 Redis）。
//
// 设计原则（按你的要求）：
//   - 下游被调用方【不】主动注册/续租心跳（避免臃肿服务适配注册协议）；
//   - 存活判定完全由调用方（krono-job）主动探测完成：对候选 endpoint 做轻量探活（见 Probe）；
//   - 探测结果【写入 Redis 集中共享】（带 TTL），多个 krono 实例、前端看板都能读到统一的
//     被调用方节点存活视图，无需落 PG 表持久化；
//   - 本地内存缓存仅作为分发决策的快速命中（避免每次分发都打网络），Redis 作为跨实例展示源。
type NodeHealth struct {
	probe   *Probe
	rdb     *redis.Client // 可选：非 nil 时探测结果写入 Redis 集中共享
	key     string        // Redis hash key（endpoint -> 探测结果 JSON）
	ttl     time.Duration // 单条探测结果在 Redis 的存活窗口；超过未更新即视为过期（节点失联）
	timeout time.Duration // 单次探测超时（透传给 Probe）

	mu    sync.RWMutex
	cache map[string]entry // 本地内存缓存，用于分发决策快速命中
}
type entry struct {
	alive   bool
	checked time.Time
	err     string
}

// 探测结果在 Redis 中的存储结构（与本地 entry 对齐，附带协议字段便于展示）。
type probeRecord struct {
	Endpoint string `json:"endpoint"`
	Protocol string `json:"protocol"`
	Alive    bool   `json:"alive"`
	Error    string `json:"error,omitempty"`
	Checked  int64  `json:"checked"` // 最近探测时间 UnixNano
	By       string `json:"by"`      // 探测来源节点 ID（便于观测是哪个实例探的）
}

// NewNodeHealth 构造节点健康缓存。
//   - probeTimeout：单次探测超时（建议 1~3s）。
//   - rdb / key / ttl：可选，非 nil 时探测结果写入 Redis 共享；为 nil 时仅本地缓存（单实例可用）。
func NewNodeHealth(probeTimeout time.Duration, rdb *redis.Client, key string, ttl time.Duration) *NodeHealth {
	if probeTimeout <= 0 {
		probeTimeout = 2 * time.Second
	}
	if ttl <= 0 {
		ttl = 30 * time.Second // 默认 Redis 结果窗口 30s，需由周期探测器在窗口内刷新
	}
	return &NodeHealth{
		probe:   NewProbe(probeTimeout),
		rdb:     rdb,
		key:     key,
		ttl:     ttl,
		timeout: probeTimeout,
		cache:   make(map[string]entry),
	}
}

// Check 探测单个 endpoint 存活（命中本地缓存且未过期则直接用缓存，避免每次打网络）。
// 探测完成后把结果写回本地缓存，并（若配置了 rdb）异步写入 Redis 共享。
// 返回是否存活、错误信息（仅用于日志/展示）。
func (h *NodeHealth) Check(ctx context.Context, endpoint, protocol, by string) (bool, string) {
	h.mu.RLock()
	if e, ok := h.cache[endpoint]; ok && time.Since(e.checked) < h.ttl/3 {
		h.mu.RUnlock()
		return e.alive, e.err
	}
	h.mu.RUnlock()

	alive, err := h.probe.Check(ctx, endpoint, protocol)
	reason := ""
	if err != nil {
		reason = err.Error()
	}

	h.mu.Lock()
	h.cache[endpoint] = entry{alive: alive, checked: time.Now(), err: reason}
	h.mu.Unlock()

	if h.rdb != nil {
		rec := probeRecord{
			Endpoint: endpoint,
			Protocol: protocol,
			Alive:    alive,
			Error:    reason,
			Checked:  time.Now().UnixNano(),
			By:       by,
		}
		b, _ := json.Marshal(rec)
		// 单条 HSET；整体 key 用 PEXPIRE 续租，保证所有条目共享同一过期窗口。
		if err := h.rdb.HSet(ctx, h.key, endpoint, b).Err(); err == nil {
			_ = h.rdb.PExpire(ctx, h.key, h.ttl).Err()
		}
	}
	return alive, reason
}

// FilterLive 批量探测候选 endpoints，返回存活列表（各 endpoint 并发探测）。
// 仅用于分发决策：结果仍会通过 Check 写入 Redis。
func (h *NodeHealth) FilterLive(ctx context.Context, endpoints []string, protocol, by string) []string {
	type res struct {
		ep string
		ok bool
	}
	ch := make(chan res, len(endpoints))
	for _, ep := range endpoints {
		go func(e string) {
			ok, _ := h.Check(ctx, e, protocol, by)
			ch <- res{ep: e, ok: ok}
		}(ep)
	}
	out := make([]string, 0, len(endpoints))
	for range endpoints {
		r := <-ch
		if r.ok {
			out = append(out, r.ep)
		}
	}
	return out
}

// Snapshot 返回下游节点健康视图。优先从 Redis 读取（集群级统一视图，包含其它实例探到的节点），
// 当 Redis 不可用或未配置时回退到本地缓存。
func (h *NodeHealth) Snapshot(ctx context.Context) []NodeView {
	if h.rdb != nil {
		if data, err := h.rdb.HGetAll(ctx, h.key).Result(); err == nil && len(data) > 0 {
			out := make([]NodeView, 0, len(data))
			for _, v := range data {
				var rec probeRecord
				if err := json.Unmarshal([]byte(v), &rec); err != nil {
					continue
				}
				out = append(out, NodeView{
					Endpoint: rec.Endpoint,
					Protocol: rec.Protocol,
					Alive:    rec.Alive,
					Error:    rec.Error,
					Checked:  rec.Checked,
					By:       rec.By,
				})
			}
			return out
		}
	}
	// 回退：本地缓存。
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]NodeView, 0, len(h.cache))
	for ep, e := range h.cache {
		out = append(out, NodeView{
			Endpoint: ep,
			Alive:    e.alive,
			Error:    e.err,
			Checked:  e.checked.UnixNano(),
		})
	}
	return out
}

// NodeView 是供运维展示的节点健康视图。
type NodeView struct {
	Endpoint string `json:"endpoint"`
	Protocol string `json:"protocol"`
	Alive    bool   `json:"alive"`
	Error    string `json:"error,omitempty"`
	Checked  int64  `json:"checked"` // 最近探测时间 UnixNano
	By       string `json:"by,omitempty"`
}
