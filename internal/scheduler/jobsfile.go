package scheduler

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// jobsDoc 是 jobs.yaml 的顶层文档结构，使用 yaml.Node 以保留注释与格式。
type jobsDoc struct {
	Node *yaml.Node
}

// headerComment 是每次重写文件时保留在顶部的全局说明注释（避免手工维护无从下手）。
const headerComment = `# krono-job 调度任务定义（声明式，不落库）。
#
# 本文件可由管理后台「任务管理」可视化编辑（增删改查），改动后调度内核通过 fsnotify 热重载，
# 无需重启服务即生效。若 YAML 解析失败，保留上次有效配置并记错误日志。
#
# 字段说明：
#   name        任务显示名称
#   task_type   全局唯一标识，用于日志与暂停态标记（不可重复）
#   cron        标准 5 字段 cron：分 时 日 月 周（按 config.yaml 的 timezone 解析）
#   protocol    http | grpc
#   enabled     是否纳入调度（缺省视为 true；false=停注但不删配置）
#   timeout     单次调用超时（如 30s），超时按失败处理
#   retry       Asynq 最大重试次数
#   endpoint    单节点下游地址（host:port 或完整 URL）；多节点用 endpoints 数组（优先级更高）
#   endpoints   多节点下游地址数组（分布式负载均衡）；仅一个元素时退化为单节点
#   lb_strategy 负载均衡策略 round_robin | least_conn | random | consistent
#   method      HTTP 方法 GET | POST 等（protocol=http 时有效）
#   headers     HTTP 请求头（支持 ${ENV} 引用环境变量）
#   payload     请求体参数（分层约定，可视化编辑产生）：
#                payload.path   路径参数，替换 endpoint 中的 {id} 占位符（自动 URL 编码）
#                payload.query  查询参数，拼接到 URL 的 ?a=1&b=2（GET 传参方式）
#                payload.body   请求体，整体作为 JSON body 发送
#                旧格式（无 path/query/body 键）：整个 payload 作为 JSON body，向后兼容
#   grpc_service / grpc_method / request_type  gRPC 调用描述（protocol=grpc 时有效）
#   metadata    gRPC metadata（支持 ${ENV}）
#   comment     本条任务的备注（仅用于人工维护，不进入调度逻辑）
#
# 注意：多节点请用 endpoints（YAML 数组），不要用逗号塞进单个 endpoint。
`

// readDoc 读取并解析 jobs.yaml 为 yaml.Node 文档。
// 若文件不存在则返回空的 jobs 文档结构，便于前端从零创建。
func readDoc(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在：构造最小合法文档。
			doc := &yaml.Node{Kind: yaml.DocumentNode}
			root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			jobsKey := &yaml.Node{Kind: yaml.ScalarNode, Value: "jobs", Tag: "!!str"}
			jobsVal := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			root.Content = []*yaml.Node{jobsKey, jobsVal}
			doc.Content = []*yaml.Node{root}
			return doc, nil
		}
		return nil, fmt.Errorf("read jobs file %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse jobs file %s: %w", path, err)
	}
	if doc.Kind == 0 {
		// 空文件：构造最小合法文档。
		doc.Kind = yaml.DocumentNode
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		jobsKey := &yaml.Node{Kind: yaml.ScalarNode, Value: "jobs", Tag: "!!str"}
		jobsVal := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		root.Content = []*yaml.Node{jobsKey, jobsVal}
		doc.Content = []*yaml.Node{root}
	}
	return &doc, nil
}

// findJobsSeq 在文档根 mapping 中找到 jobs 序列节点；不存在则创建。
func findJobsSeq(doc *yaml.Node) (*yaml.Node, error) {
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("invalid yaml document: empty content")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid jobs.yaml: root is not a mapping")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "jobs" {
			return root.Content[i+1], nil
		}
	}
	// 未找到 jobs 键，追加一个空序列。
	key := &yaml.Node{Kind: yaml.ScalarNode, Value: "jobs", Tag: "!!str"}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	root.Content = append(root.Content, key, seq)
	return seq, nil
}

// toJobNode 把 JobDef 转为一个 YAML mapping 节点（保留 comment 字段作为行尾/块注释）。
func toJobNode(j JobDef) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}

	add := func(key string, value any) {
		if isZero(value) {
			return
		}
		k := &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}
		var v *yaml.Node
		_ = marshalInto(value, &v)
		m.Content = append(m.Content, k, v)
	}

	add("name", j.Name)
	add("task_type", j.TaskType)
	add("cron", j.Cron)
	add("protocol", j.Protocol)
	if j.Enabled != nil {
		add("enabled", *j.Enabled)
	}
	add("timeout", j.Timeout)
	if j.Retry != 0 {
		add("retry", j.Retry)
	}
	if len(j.Endpoints) > 0 {
		add("endpoints", j.Endpoints)
	} else if j.Endpoint != "" {
		add("endpoint", j.Endpoint)
	}
	if j.LBStrategy != "" {
		add("lb_strategy", j.LBStrategy)
	}
	add("method", j.Method)
	if len(j.Headers) > 0 {
		add("headers", j.Headers)
	}
	if len(j.Payload) > 0 {
		add("payload", j.Payload)
	}
	add("grpc_service", j.GRPCService)
	add("grpc_method", j.GRPCMethod)
	add("request_type", j.RequestType)
	if len(j.Metadata) > 0 {
		add("metadata", j.Metadata)
	}

	// 备注以块注释形式写入本条任务之前，便于人工阅读与维护。
	if strings.TrimSpace(j.Comment) != "" {
		lines := strings.Split(strings.TrimSpace(j.Comment), "\n")
		var sb strings.Builder
		for _, l := range lines {
			sb.WriteString("# " + strings.TrimLeft(l, "# ") + "\n")
		}
		m.HeadComment = sb.String()
	}
	return m
}

// marshalInto 把任意值序列化为 yaml.Node（借助中间 marshal）。
func marshalInto(v any, out **yaml.Node) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return err
	}
	*out = n.Content[0]
	return nil
}

// isZero 判断值是否为零值（用于决定是否写出该字段）。
func isZero(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case int:
		return x == 0
	case []string:
		return len(x) == 0
	case map[string]string:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case bool:
		return false // bool 始终写出（由调用方决定是否传入）
	case *bool:
		return x == nil
	default:
		return false
	}
}

// findJobIndex 在 jobs 序列中按 task_type 查找索引。
func findJobIndex(seq *yaml.Node, taskType string) int {
	for i, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(item.Content); j += 2 {
			if item.Content[j].Value == "task_type" && item.Content[j+1].Value == taskType {
				return i
			}
		}
	}
	return -1
}

// SaveJobsFile 把给定的任务定义列表写回 jobs.yaml，保留全局头部注释。
// 写入流程：解析现有文件 -> 替换为新 jobs 序列 -> 序列化。失败则原样返回 error，不破坏旧文件。
func SaveJobsFile(path string, jobs []JobDef) error {
	doc, err := readDoc(path)
	if err != nil {
		return err
	}
	seq, err := findJobsSeq(doc)
	if err != nil {
		return err
	}
	nodes := make([]*yaml.Node, 0, len(jobs))
	for _, j := range jobs {
		nodes = append(nodes, toJobNode(j))
	}
	seq.Content = nodes

	// 设置文档头部注释（覆盖旧头部，保持说明一致性）。
	if len(doc.Content) > 0 {
		doc.Content[0].HeadComment = headerComment
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal jobs doc: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write jobs file %s: %w", path, err)
	}
	return nil
}

// LoadJobsWithComments 读取并返回带 comment 字段的全部任务定义（供可视化编辑回显）。
func LoadJobsWithComments(path string) ([]JobDef, error) {
	defs, err := LoadJobsFile(path)
	if err != nil {
		return nil, err
	}
	// 回填每条任务在 yaml 中的备注（如有）。
	seq, err := jobsSeqFromDoc(path)
	if err == nil {
		for i := range defs {
			if idx := findJobIndex(seq, defs[i].TaskType); idx >= 0 {
				defs[i].Comment = extractComment(seq.Content[idx])
			}
		}
	}
	return defs, nil
}

// jobsSeqFromDoc 读取并返回文档中的 jobs 序列节点。
func jobsSeqFromDoc(path string) (*yaml.Node, error) {
	doc, err := readDoc(path)
	if err != nil {
		return nil, err
	}
	return findJobsSeq(doc)
}

// extractComment 从 mapping 节点提取备注文本：优先取「注释块」（HeadComment/FootComment），
// 兼容旧格式「comment 字段值」。两种来源都能正确回显到可视化表单。
func extractComment(node *yaml.Node) string {
	var sb strings.Builder
	if node.HeadComment != "" {
		sb.WriteString(node.HeadComment)
	}
	if node.FootComment != "" {
		sb.WriteString(node.FootComment)
	}
	raw := strings.TrimSpace(sb.String())
	if raw == "" {
		// 旧格式：comment 作为普通字段，取其值。
		if v := fieldValue(node, "comment"); v != "" {
			return v
		}
		return ""
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		t := strings.TrimSpace(l)
		t = strings.TrimPrefix(t, "#")
		t = strings.TrimPrefix(t, " ")
		if t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}

// fieldValue 在 mapping 节点中按 key 取标量值；不存在返回空串。
func fieldValue(node *yaml.Node, key string) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1].Value
		}
	}
	return ""
}
