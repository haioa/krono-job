package cluster

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// grpcHealthCheck 对 gRPC 下游做「真正的 RPC 级」健康探测，而非仅 TCP 连通。
//
// 探测顺序（兼容优先、逐步升级）：
//  1. 标准 gRPC Health Protocol：调用 grpc.health.v1.Health/Check，
//     返回 ServingStatus_SERVING 即视为存活。这是业界标准，下游只需挂上官方
//     health.Server 即可（无需业务改动）。
//  2. 若下游未实现 health 服务（返回 Unimplemented），退回 TCP dial 判定端口连通性，
//     保证未接入 health 的旧服务仍可被探活发现。
func grpcHealthCheck(ctx context.Context, endpoint string) (bool, error) {
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return false, fmt.Errorf("grpc dial %s: %w", endpoint, err)
	}
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ""})
	if err != nil {
		// 下游未实现 health 服务：退回 TCP 连通性判定。
		if status.Code(err) == codes.Unimplemented {
			return tcpAlive(ctx, endpoint)
		}
		return false, err
	}
	return resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING, nil
}

// tcpAlive 退回 TCP 连通性判定（端口可达即存活）。
func tcpAlive(ctx context.Context, endpoint string) (bool, error) {
	host, port, err := splitHostPort(endpoint)
	if err != nil {
		return false, err
	}
	d := &net.Dialer{Timeout: probeTimeout(ctx)}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return false, err
	}
	_ = conn.Close()
	return true, nil
}

// probeTimeout 从 ctx 取探测超时（存于 value），缺省 2s。
func probeTimeout(ctx context.Context) time.Duration {
	if v, ok := ctx.Value(probeTimeoutKey{}).(time.Duration); ok && v > 0 {
		return v
	}
	return 2 * time.Second
}

type probeTimeoutKey struct{}

// withProbeTimeout 将探测超时写入 ctx，供 tcpAlive 等回退路径取用。
func withProbeTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, probeTimeoutKey{}, d)
}
