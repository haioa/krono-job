// Package httputil 实现 HTTP 类型任务的 Webhook 调用。
// HTTP 任务为「纯配置」：请求方法/头/体全部来自 jobs.yaml，无需为具体下游写对接代码。
package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/haioa/krono-job/internal/scheduler"
)

// DefaultMethod 是 HTTP 任务的默认请求方法。
const DefaultMethod = "POST"

// Result 是 Webhook 调用的结果，供 Worker 落库日志与下游判断。
type Result struct {
	StatusCode int
	Body       string // 响应体（可能已被截断）
}

// Do 按 JobDef 发送一次 HTTP 请求，返回响应体、状态码与错误。
// timeout 由 def.TimeoutDuration() 控制；ctx 取消（含 Asynq 任务超时）也会中止请求。
func Do(ctx context.Context, def scheduler.JobDef) (*Result, error) {
	method := def.Method
	if method == "" {
		method = DefaultMethod
	}

	var bodyReader io.Reader
	if len(def.Payload) > 0 {
		raw, err := json.Marshal(def.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, def.Endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if len(def.Payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range def.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: def.TimeoutDuration()}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	// 限制读取量，避免超大响应撑爆内存（落库前 repository 还会再截断至 8KB）。
	limited := io.LimitReader(resp.Body, 64*1024)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return &Result{
		StatusCode: resp.StatusCode,
		Body:       string(data),
	}, nil
}
