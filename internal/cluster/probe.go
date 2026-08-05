package cluster

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"
)

// Probe 是对下游节点的主动健康探测（存活判定的核心）。
// 下游被调用方不感知 krono、也不注册心跳；存活完全由调用方主动探测决定：
// 调度侧在分发前对候选 endpoint 做轻量探活，探活通过者视为「存活」，再交给负载均衡选择。
// 探测结果缓存在 NodeHealth（带 TTL），供选择目标与 /api/cluster/nodes 观测展示。
//
// 探测策略按协议分两类：
//   - grpc：优先调用标准 gRPC Health Protocol（grpc.health.v1.Health/Check），
//     返回 SERVING 即存活；下游未实现 health 服务时退回 TCP 连通判定。
//   - http：优先对 <endpoint>/healthz 发 HEAD 请求，非 5xx 即视为存活；
//     若无法解析出 healthz 路径，则退回 TCP dial。
type Probe struct {
	timeout       time.Duration
	client        *net.Dialer
	skipVerifyTLS bool
}

// NewProbe 构造探活器，timeout 为单次探测上限（建议 1~3s，避免拖慢分发）。
// skipVerifyTLS 为 true 时，对 HTTPS 下游跳过证书校验（与分发调用保持一致）。
func NewProbe(timeout time.Duration, skipVerifyTLS bool) *Probe {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Probe{
		timeout:       timeout,
		client:        &net.Dialer{Timeout: timeout},
		skipVerifyTLS: skipVerifyTLS,
	}
}

// Check 对单个 endpoint 做健康探测，返回是否存活与错误（仅用于日志，不影响存活判定）。
func (p *Probe) Check(ctx context.Context, endpoint, protocol string) (bool, error) {
	ctx = withProbeTimeout(ctx, p.timeout)
	switch protocol {
	case "grpc":
		// 真正 RPC 级探测：gRPC Health Protocol；未实现时退回 TCP 连通。
		return grpcHealthCheck(ctx, endpoint)
	case "http":
		// 尝试 healthz 探活；解析失败则退回 TCP。
		u, err := url.Parse(endpoint)
		if err == nil && u.Host != "" {
			hc := &httpClient{timeout: p.timeout, skipVerifyTLS: p.skipVerifyTLS}
			live, err := hc.head(ctx, endpoint+"/healthz")
			if err == nil {
				return live, nil
			}
			// /healthz 探测失败：退回 TCP 连通性判定（端点本身可达即可）。
		}
		host, port, err := splitHostPort(endpoint)
		if err != nil {
			return false, err
		}
		conn, err := p.client.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
		if err != nil {
			return false, err
		}
		_ = conn.Close()
		return true, nil
	default:
		return false, fmt.Errorf("unknown protocol %q", protocol)
	}
}

// FilterLive 对候选 endpoints 批量探活，返回存活列表。
// protocol 取自任务配置（所有候选同协议）。探活并发执行，整体受 ctx 超时约束。
func (p *Probe) FilterLive(ctx context.Context, endpoints []string, protocol string) []string {
	type res struct {
		ep string
		ok bool
	}
	ch := make(chan res, len(endpoints))
	for _, ep := range endpoints {
		go func(e string) {
			ok, _ := p.Check(ctx, e, protocol)
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

// splitHostPort 兼容 host:port 与带 scheme 的 URL，返回 host 与 port。
func splitHostPort(endpoint string) (string, string, error) {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		return host, port, nil
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", "", fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
	}
	return host, port, nil
}
