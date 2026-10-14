package httputil

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haioa/krono-job/internal/scheduler"
)

func TestSplitPayload(t *testing.T) {
	// 新格式：path/query/body 三层分层
	p := map[string]any{
		"path":  map[string]any{"id": "123"},
		"query": map[string]any{"page": 1},
		"body":  map[string]any{"scene": "expire"},
	}
	pathParams, queryParams, body := splitPayload(p)
	if pathParams["id"] != "123" || queryParams["page"] != 1 || body["scene"] != "expire" {
		t.Fatalf("layered payload split wrong: path=%v query=%v body=%v", pathParams, queryParams, body)
	}

	// 新格式：仅 path + query（GET 场景，无 body）
	p = map[string]any{
		"path":  map[string]any{"id": "1"},
		"query": map[string]any{"full": true},
	}
	pathParams, queryParams, body = splitPayload(p)
	if pathParams["id"] != "1" || queryParams["full"] != true || len(body) != 0 {
		t.Fatalf("path+query split wrong: path=%v query=%v body=%v", pathParams, queryParams, body)
	}

	// 新格式：仅 body
	p = map[string]any{"body": map[string]any{"a": 1}}
	pathParams, queryParams, body = splitPayload(p)
	if len(pathParams) != 0 || len(queryParams) != 0 || body["a"] != 1 {
		t.Fatalf("body-only split wrong: path=%v query=%v body=%v", pathParams, queryParams, body)
	}

	// 旧格式：平铺整体作为 body（path/query/body 是字符串标量时不算分层）
	p = map[string]any{"scene": "expire", "batch_size": 100, "path": "/tmp", "query": "x"}
	pathParams, queryParams, body = splitPayload(p)
	if len(pathParams) != 0 || len(queryParams) != 0 || body["scene"] != "expire" || body["path"] != "/tmp" {
		t.Fatalf("legacy payload split wrong: path=%v query=%v body=%v", pathParams, queryParams, body)
	}
}

func TestBuildURL(t *testing.T) {
	// 无占位符、无 query：原样返回
	got, err := buildURL("http://svc:8080/api/health", nil, nil)
	if err != nil || got != "http://svc:8080/api/health" {
		t.Fatalf("plain url wrong: %q err=%v", got, err)
	}

	// 占位符替换 + URL 编码
	got, err = buildURL("http://svc:8080/api/order/{id}/item/{name}",
		map[string]any{"id": 123, "name": "a b&c"}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "http://svc:8080/api/order/123/item/a%20b&c" // PathEscape 不编码 &（path 中合法字符）
	if got != want {
		t.Fatalf("placeholder replace wrong:\n got=%q\nwant=%q", got, want)
	}

	// query 参数拼接（GET 传参方式）
	got, err = buildURL("http://svc:8080/api/list", nil,
		map[string]any{"page": 1, "size": 20, "kw": "a b"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "http://svc:8080/api/list?kw=a+b&page=1&size=20" {
		t.Fatalf("query append wrong: %q", got)
	}

	// endpoint 自带 query + payload.query 同名覆盖、异名保留
	got, err = buildURL("http://svc:8080/api/list?page=1&debug=true", nil,
		map[string]any{"page": 99})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "http://svc:8080/api/list?debug=true&page=99" {
		t.Fatalf("query merge wrong: %q", got)
	}

	// 数组值展开为多个同名参数
	got, err = buildURL("http://svc:8080/api/tags", nil,
		map[string]any{"tag": []any{"a", "b c"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "http://svc:8080/api/tags?tag=a&tag=b+c" {
		t.Fatalf("query array wrong: %q", got)
	}

	// path + query 组合
	got, err = buildURL("http://svc:8080/api/user/{uid}/orders?sort=desc",
		map[string]any{"uid": "u1"}, map[string]any{"page": 2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "http://svc:8080/api/user/u1/orders?page=2&sort=desc" {
		t.Fatalf("path+query wrong: %q", got)
	}

	// 无 scheme 的 host:port/path 形式同样安全
	got, err = buildURL("10.0.0.1:8080/api/x/{id}", map[string]any{"id": 7}, map[string]any{"v": 1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "10.0.0.1:8080/api/x/7?v=1" {
		t.Fatalf("no-scheme url wrong: %q", got)
	}

	// 缺少参数：报错并指出缺失项
	_, err = buildURL("http://svc:8080/api/order/{id}", map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected error for missing path param")
	}

	// ${var} 与字面量花括号不受影响（非标识符占位符原样保留）
	got, err = buildURL("http://svc:8080/api/${token}/x{1}", nil, nil)
	if err != nil || got != "http://svc:8080/api/${token}/x{1}" {
		t.Fatalf("non-placeholder braces changed: %q err=%v", got, err)
	}
}

func TestDoEndToEnd(t *testing.T) {
	type got struct {
		path   string
		rawURL string
		method string
		body   string
		ctype  string
		query  map[string][]string
	}
	var received got
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.path = r.URL.EscapedPath() // EscapedPath 保留原始编码（Path 会解码）
		received.rawURL = r.URL.String()
		received.method = r.Method
		received.ctype = r.Header.Get("Content-Type")
		received.query = r.URL.Query()
		b, _ := io.ReadAll(r.Body)
		received.body = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	// 场景 1：新格式 path/query/body 三层（可视化编辑产生的配置）
	def := scheduler.JobDef{
		TaskType: "t1",
		Protocol: "http",
		Method:   "POST",
		Endpoint: srv.URL + "/api/order/{id}/item/{name}",
		Timeout:  "5s",
		Payload: map[string]any{
			"path":  map[string]any{"id": 123, "name": "a b"},
			"query": map[string]any{"page": 1, "size": 20},
			"body":  map[string]any{"scene": "expire", "batch_size": 100, "force": true},
		},
	}
	res, err := Do(context.Background(), def, false)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if res.StatusCode != 200 || res.Body != `{"ok":true}` {
		t.Fatalf("resp wrong: %+v", res)
	}
	if received.path != "/api/order/123/item/a%20b" {
		t.Fatalf("path replace wrong: %q", received.path)
	}
	if received.query["page"][0] != "1" || received.query["size"][0] != "20" {
		t.Fatalf("query wrong: %v", received.query)
	}
	if received.ctype != "application/json" {
		t.Fatalf("content-type wrong: %q", received.ctype)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(received.body), &body); err != nil {
		t.Fatalf("body not json: %v (%s)", err, received.body)
	}
	if body["batch_size"] != float64(100) || body["force"] != true {
		t.Fatalf("body types lost: %v", body)
	}

	// 场景 2：旧格式平铺 payload 整体作为 body（向后兼容）
	received = got{}
	def2 := scheduler.JobDef{
		TaskType: "t2",
		Protocol: "http",
		Method:   "POST",
		Endpoint: srv.URL + "/api/hook",
		Timeout:  "5s",
		Payload:  map[string]any{"scene": "legacy"},
	}
	if _, err := Do(context.Background(), def2, false); err != nil {
		t.Fatalf("Do legacy failed: %v", err)
	}
	if received.path != "/api/hook" || received.body != `{"scene":"legacy"}` {
		t.Fatalf("legacy payload wrong: path=%q body=%q", received.path, received.body)
	}

	// 场景 3：仅 path + query（GET 风格），无 body
	received = got{}
	def3 := scheduler.JobDef{
		TaskType: "t3",
		Protocol: "http",
		Method:   "GET",
		Endpoint: srv.URL + "/api/user/{uid}",
		Timeout:  "5s",
		Payload: map[string]any{
			"path":  map[string]any{"uid": "u-1"},
			"query": map[string]any{"full": true},
		},
	}
	if _, err := Do(context.Background(), def3, false); err != nil {
		t.Fatalf("Do path+query failed: %v", err)
	}
	if received.path != "/api/user/u-1" || received.body != "" {
		t.Fatalf("path+query wrong: path=%q body=%q", received.path, received.body)
	}
	if received.query["full"][0] != "true" {
		t.Fatalf("query full wrong: %v", received.query)
	}

	// 场景 4：占位符缺少参数 -> 报错
	def4 := def3
	def4.Payload = map[string]any{"path": map[string]any{}}
	if _, err := Do(context.Background(), def4, false); err == nil {
		t.Fatal("expected error for missing path param")
	}
}
