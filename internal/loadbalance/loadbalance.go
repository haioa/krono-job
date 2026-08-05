// Package loadbalance 从一组存活的下游节点中，按选定策略挑选一个目标节点。
//
// 支持的策略：
//   - round_robin   轮询：请求在节点间均匀分布（无权重场景下的默认选择）
//   - least_conn    最少连接：优先选当前承载连接/请求数最少的节点（需上报负载）
//   - random        随机：简单打散，适合节点性能相近且无需状态的场景
//   - consistent    一致性哈希：以 taskType 为 key 做哈希，相同任务稳定命中同一节点，
//     适合有状态/有缓存亲和性的下游
//
// 选择器对并发安全，状态（如轮询游标、连接计数）以原子方式维护。
package loadbalance

import (
	"fmt"
	"hash/crc32"
	"sync"
	"sync/atomic"
)

// Strategy 是负载均衡策略名。
type Strategy string

const (
	RoundRobin Strategy = "round_robin"
	LeastConn  Strategy = "least_conn"
	Random     Strategy = "random"
	Consistent Strategy = "consistent"
)

// Candidate 是可选的下游节点候选（含负载信息以便 least_conn 使用）。
type Candidate struct {
	Endpoint string
	// InFlight 是当前在途请求/连接数；least_conn 据此择优。非 least_conn 策略可忽略。
	InFlight int64
}

// Balancer 从候选节点中选择目标。
type Balancer interface {
	// Select 从 candidates 中选出一个 endpoint。candidates 必须非空，否则返回错误。
	// key 仅在 consistent 策略下用于哈希（建议传 taskType），其余策略忽略。
	Select(candidates []Candidate, key string) (string, error)
}

// New 按策略名构造负载均衡器；未知策略回落到 round_robin。
func New(strategy Strategy) Balancer {
	switch strategy {
	case LeastConn:
		return &leastConnBalancer{}
	case Random:
		return &randomBalancer{}
	case Consistent:
		return &consistentBalancer{}
	case RoundRobin, "":
		return &roundRobinBalancer{}
	default:
		return &roundRobinBalancer{}
	}
}

// roundRobinBalancer 轮询，使用原子游标避免锁竞争。
type roundRobinBalancer struct {
	cursor uint64
}

func (b *roundRobinBalancer) Select(candidates []Candidate, _ string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("loadbalance: no candidate nodes")
	}
	idx := atomic.AddUint64(&b.cursor, 1) % uint64(len(candidates))
	return candidates[idx].Endpoint, nil
}

// leastConnBalancer 最少连接：扫描当前在途数最小的节点（并列时取第一个）。
type leastConnBalancer struct{}

func (b *leastConnBalancer) Select(candidates []Candidate, _ string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("loadbalance: no candidate nodes")
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.InFlight < best.InFlight {
			best = c
		}
	}
	return best.Endpoint, nil
}

// randomBalancer 随机选取。
type randomBalancer struct {
	mu sync.Mutex
}

func (b *randomBalancer) Select(candidates []Candidate, _ string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("loadbalance: no candidate nodes")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	idx := randIntn(len(candidates))
	return candidates[idx].Endpoint, nil
}

// consistentBalancer 一致性哈希：对 key 做 CRC32，再对候选节点哈希环取模。
// 为简化实现，这里以「key 哈希值 % 节点数」定位，节点集合变化时仍尽量保持亲和性（节点数不变时完全稳定）。
type consistentBalancer struct{}

func (b *consistentBalancer) Select(candidates []Candidate, key string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("loadbalance: no candidate nodes")
	}
	if key == "" {
		key = "default"
	}
	h := crc32.ChecksumIEEE([]byte(key))
	idx := h % uint32(len(candidates))
	return candidates[idx].Endpoint, nil
}

// randIntn 线程安全随机，避免引入包级全局随机源竞争。
func randIntn(n int) int {
	return int(randUint64() % uint64(n))
}

var rngSeed uint64

func randUint64() uint64 {
	// 简单的 xorshift，无需引入 math/rand 全局锁。
	for {
		old := atomic.LoadUint64(&rngSeed)
		if old == 0 {
			old = 0x9E3779B97F4A7C15
		}
		next := old ^ (old << 13)
		next ^= old >> 7
		next ^= old << 17
		if atomic.CompareAndSwapUint64(&rngSeed, old, next) {
			return next>>32 ^ next
		}
	}
}
