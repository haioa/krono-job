// Package grpcpool 维护到下游 gRPC 服务的连接池，按 endpoint（host:port）复用连接。
// 使用 grpc.NewClient（懒连接）：首次 Invoke 时才真正建连，关闭由 Close 统一回收。
package grpcpool

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Pool 按 endpoint 缓存 *grpc.ClientConn。
type Pool struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

// New 构造一个空连接池。
func New() *Pool {
	return &Pool{conns: make(map[string]*grpc.ClientConn)}
}

// Get 返回 endpoint 对应的连接；不存在则新建并缓存。
// MVP 阶段使用 insecure 明文传输；生产如需 TLS 在此扩展 credentials。
func (p *Pool) Get(endpoint string) (*grpc.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if c, ok := p.conns[endpoint]; ok {
		return c, nil
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", endpoint, err)
	}
	p.conns[endpoint] = conn
	return conn, nil
}

// Close 关闭所有缓存连接（进程退出时调用）。
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ep, c := range p.conns {
		_ = c.Close()
		delete(p.conns, ep)
	}
}
