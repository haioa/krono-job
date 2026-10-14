// Package httputil 实现 HTTP 类型任务的 Webhook 调用。
// HTTP 任务为「纯配置」：请求方法/头/体全部来自 jobs.yaml，无需为具体下游写对接代码。
//
// payload 分层约定（新版，可视化编辑产生的格式）：
//
//	payload:
//	  path:            # 路径参数：替换 endpoint 中的 {id} 占位符（值做 URL 编码）
//	    id: "123"
//	  query:           # 查询参数：拼接到 URL 的 ?a=1&b=2（GET 请求的传参方式）
//	    page: 1
//	    size: 20
//	  body:            # 请求体：整体作为 JSON body 发送
//	    scene: "auto_expire"
//	    batch_size: 100
//
// 兼容旧格式：payload 中不含 path/query/body 对象键时，整个 payload 作为 JSON body，
// 既有 jobs.yaml 任务零改动。
package httputil

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/haioa/krono-job/internal/scheduler"
)

// tlsConfig 构造 HTTP 客户端的 TLS 配置。
// skipVerify 为 true 时跳过证书校验（InsecureSkipVerify），用于内网自签/证书 IP 不匹配的下游；
// 为 false 时返回 nil，沿用 Go 默认的安全校验。
func tlsConfig(skipVerify bool) *tls.Config {
	if !skipVerify {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true}
}

// DefaultMethod 是 HTTP 任务的默认请求方法。
const DefaultMethod = "POST"

// payload 分层约定的键名。
const (
	payloadPathKey  = "path"  // 路径参数映射
	payloadQueryKey = "query" // 查询参数映射（URL ?a=1&b=2）
	payloadBodyKey  = "body"  // 请求体
)

// placeholderRe 匹配 endpoint 中的 {param} 占位符（标识符风格）。
// ${var} 形式优先匹配并原样保留，避免误把 Shell/环境变量占位当成路径占位符。
var placeholderRe = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}|\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Result 是 Webhook 调用的结果，供 Worker 落库日志与下游判断。
type Result struct {
	StatusCode int
	Body       string // 响应体（可能已被截断）
}

// splitPayload 把 payload 拆分为路径参数、查询参数与请求体。
// 分层判定要求 path/query/body 的值为对象（map）；否则按旧格式处理：
// 整个 payload 作为请求体，保证既有任务零改动。
func splitPayload(payload map[string]any) (pathParams, queryParams, body map[string]any) {
	p, pOK := payload[payloadPathKey].(map[string]any)
	q, qOK := payload[payloadQueryKey].(map[string]any)
	b, bOK := payload[payloadBodyKey].(map[string]any)
	if !pOK && !qOK && !bOK {
		return nil, nil, payload
	}
	return p, q, b
}

// replacePlaceholders 把 URL 中的 {key} 占位符替换为路径参数值（URL 编码）。
// 占位符缺少对应参数时返回错误，避免静默发出错误请求。
func replacePlaceholders(rawURL string, pathParams map[string]any) (string, error) {
	if !strings.Contains(rawURL, "{") {
		return rawURL, nil
	}
	var missing []string
	out := placeholderRe.ReplaceAllStringFunc(rawURL, func(m string) string {
		// ${var} 形式原样保留（环境变量占位，非路径参数）
		if strings.HasPrefix(m, "${") {
			return m
		}
		key := m[1 : len(m)-1]
		v, ok := pathParams[key]
		if !ok {
			missing = append(missing, key)
			return m // 保留原占位符，最终由错误分支拦截
		}
		return url.PathEscape(fmt.Sprint(v))
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("地址中的占位符缺少路径参数: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// buildURL 拼接最终请求 URL：替换 {key} 路径占位符 + 合并 query 参数。
// endpoint 自带的查询参数会保留，payload.query 中的同名参数覆盖它；
// 值为数组时展开为多个同名参数（?tag=a&tag=b）。
func buildURL(endpoint string, pathParams, queryParams map[string]any) (string, error) {
	// 1) 拆分 endpoint 自带的 query（对无 scheme 的 host:port/path 形式同样安全）
	base, rawQuery := endpoint, ""
	if i := strings.Index(endpoint, "?"); i >= 0 {
		base, rawQuery = endpoint[:i], endpoint[i+1:]
	}

	// 2) 替换路径占位符
	base, err := replacePlaceholders(base, pathParams)
	if err != nil {
		return "", err
	}

	// 3) 合并查询参数：endpoint 原有参数 + payload.query（同名覆盖）
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("解析地址中的查询参数失败: %w", err)
	}
	for k, v := range queryParams {
		values.Del(k) // 先删后加，保证 payload 中的值覆盖 endpoint 同名字段
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				values.Add(k, fmt.Sprint(item))
			}
			continue
		}
		values.Set(k, fmt.Sprint(v))
	}
	if encoded := values.Encode(); encoded != "" {
		return base + "?" + encoded, nil
	}
	return base, nil
}

// Do 按 JobDef 发送一次 HTTP 请求，返回响应体、状态码与错误。
// timeout 由 def.TimeoutDuration() 控制；ctx 取消（含 Asynq 任务超时）也会中止请求。
// skipVerifyTLS 为 true 时，对 HTTPS 下游跳过证书校验（用于内网自签/证书 IP 不匹配的端点），
// 普通 HTTP 不受影响。
func Do(ctx context.Context, def scheduler.JobDef, skipVerifyTLS bool) (*Result, error) {
	method := def.Method
	if method == "" {
		method = DefaultMethod
	}

	// 拆分 payload：路径参数（替换 endpoint 占位符）+ 查询参数（URL ?a=1）+ 请求体（JSON body）
	pathParams, queryParams, body := splitPayload(def.Payload)
	target, err := buildURL(def.Endpoint, pathParams, queryParams)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range def.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: def.TimeoutDuration(),
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig(skipVerifyTLS),
		},
	}
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
