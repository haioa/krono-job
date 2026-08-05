package cluster

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"
)

// httpClient 是探活用的极简 HTTP 客户端（仅 HEAD 探活，避免拖带请求体）。
type httpClient struct {
	timeout       time.Duration
	skipVerifyTLS bool // 与分发调用保持一致：跳过 HTTPS 证书校验时探活也跳过，避免探测与分发语义冲突
}

// head 对 url 发送 HEAD 请求，2xx/3xx/4xx（非 5xx）均视为存活；
// 连接/超时错误返回 error（调用方据此判为不健康）。
func (c *httpClient) head(ctx context.Context, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{
		Timeout: c.timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig(c.skipVerifyTLS),
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	// 5xx 视为不健康，其余视为可达。
	return resp.StatusCode < 500, nil
}

// tlsConfig 构造探活客户端的 TLS 配置（与 httputil 同语义）。
func tlsConfig(skipVerify bool) *tls.Config {
	if !skipVerify {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true}
}
