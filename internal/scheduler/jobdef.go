package scheduler

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DispatchTaskType 是 Asynq 任务类型，Worker（M4）据此消费调度触发的任务。
// Scheduler 把整条 JobDef（已解析 ${ENV}）作为任务载荷下发，使 Worker 与 jobs.yaml 解耦。
const DispatchTaskType = "krono:dispatch"

// DefaultTimeout 是单次调用默认超时（决策 §5：默认 30s）。
const DefaultTimeout = 30 * time.Second

// JobDef 对应 jobs.yaml 中 jobs 数组的单条声明式任务定义。
// yaml 标签用于解析配置文件；json 标签用于把定义作为 Asynq 任务载荷透传给 Worker（M4）。
type JobDef struct {
	Name        string                 `yaml:"name" json:"name"`
	TaskType    string                 `yaml:"task_type" json:"task_type"`
	Cron        string                 `yaml:"cron" json:"cron"`
	Protocol    string                 `yaml:"protocol" json:"protocol"`
	TriggerType string                 `yaml:"-" json:"trigger_type"` // 运行期注入：manual=手动触发；空=hron（默认）
	Enabled     *bool                  `yaml:"enabled" json:"enabled"`
	Timeout     string                 `yaml:"timeout" json:"timeout"`
	Retry       int                    `yaml:"retry" json:"retry"`
	Endpoint    string                 `yaml:"endpoint" json:"endpoint"`
	Method      string                 `yaml:"method" json:"method"`
	Headers     map[string]string      `yaml:"headers" json:"headers"`
	Payload     map[string]any `yaml:"payload" json:"payload"`
	GRPCService string                 `yaml:"grpc_service" json:"grpc_service"`
	GRPCMethod  string                 `yaml:"grpc_method" json:"grpc_method"`
	RequestType string                 `yaml:"request_type" json:"request_type"`
	Metadata    map[string]string      `yaml:"metadata" json:"metadata"`
}

// IsEnabled 返回该任务是否纳入调度：enabled 缺省视为 true（决策 §5）。
func (j JobDef) IsEnabled() bool {
	return j.Enabled == nil || *j.Enabled
}

// TimeoutDuration 解析 timeout 字符串（如 "30s"），非法或空时回落到默认 30s。
func (j JobDef) TimeoutDuration() time.Duration {
	if j.Timeout == "" {
		return DefaultTimeout
	}
	d, err := time.ParseDuration(j.Timeout)
	if err != nil || d <= 0 {
		return DefaultTimeout
	}
	return d
}

// ResolveEnv 返回一份副本，把所有字符串字段中的 ${VAR} 替换为当前环境变量。
// headers / metadata / endpoint 明确支持 ${ENV}（决策 §5）；payload 内的 ${var}
// 为可选运行期模板，MVP 阶段按静态环境变量一并替换。
func (j JobDef) ResolveEnv() JobDef {
	r := j
	r.Endpoint = expandEnv(r.Endpoint)
	r.Method = expandEnv(r.Method)
	r.GRPCService = expandEnv(r.GRPCService)
	r.GRPCMethod = expandEnv(r.GRPCMethod)
	r.RequestType = expandEnv(r.RequestType)
	if r.Headers != nil {
		h := make(map[string]string, len(r.Headers))
		for k, v := range r.Headers {
			h[k] = expandEnv(v)
		}
		r.Headers = h
	}
	if r.Metadata != nil {
		m := make(map[string]string, len(r.Metadata))
		for k, v := range r.Metadata {
			m[k] = expandEnv(v)
		}
		r.Metadata = m
	}
	if r.Payload != nil {
		r.Payload = expandEnvInMap(r.Payload)
	}
	return r
}

func expandEnv(s string) string {
	return os.Expand(s, func(key string) string { return os.Getenv(key) })
}

func expandEnvInValue(v any) any {
	switch x := v.(type) {
	case string:
		return expandEnv(x)
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = expandEnvInValue(val)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, val := range x {
			s[i] = expandEnvInValue(val)
		}
		return s
	default:
		return v
	}
}

func expandEnvInMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, val := range m {
		out[k] = expandEnvInValue(val)
	}
	return out
}

// LoadJobsFile 读取并解析 jobs.yaml，返回已通过基础校验的任务定义列表。
// 整文件解析/结构错误直接返回 error，由调用方决定保留旧配置（PLAN §6.3）。
func LoadJobsFile(path string) ([]JobDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read jobs file %s: %w", path, err)
	}
	var doc struct {
		Jobs []JobDef `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse jobs file %s: %w", path, err)
	}

	seen := make(map[string]bool, len(doc.Jobs))
	out := make([]JobDef, 0, len(doc.Jobs))
	for i := range doc.Jobs {
		def := doc.Jobs[i]
		if err := validateJob(def); err != nil {
			return nil, fmt.Errorf("validate job[%d] (task_type=%q): %w", i, def.TaskType, err)
		}
		if seen[def.TaskType] {
			return nil, fmt.Errorf("duplicate task_type %q in jobs file", def.TaskType)
		}
		seen[def.TaskType] = true
		out = append(out, def)
	}
	return out, nil
}

// validateJob 执行单条任务的必填/枚举校验；cron 表达式合法性交由 Asynq Register 兜底。
func validateJob(def JobDef) error {
	if def.TaskType == "" {
		return fmt.Errorf("task_type is required")
	}
	if def.Cron == "" {
		return fmt.Errorf("cron is required")
	}
	if def.Protocol != "http" && def.Protocol != "grpc" {
		return fmt.Errorf("protocol must be http or grpc, got %q", def.Protocol)
	}
	if def.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	return nil
}
