package cluster

import (
	"context"
	"net/http"
	"time"
)

// httpClient 是探活用的极简 HTTP 客户端（仅 HEAD 探活，避免拖带请求体）。
type httpClient struct {
	timeout time.Duration
}

// head 对 url 发送 HEAD 请求，2xx/3xx/4xx（非 5xx）均视为存活；
// 连接/超时错误返回 error（调用方据此回落到 TCP 判定）。
func (c *httpClient) head(ctx context.Context, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	// 5xx 视为不健康，其余视为可达。
	return resp.StatusCode < 500, nil
}
