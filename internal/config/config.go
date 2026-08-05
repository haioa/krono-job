package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 是服务整体配置，由 config.yaml 加载，并允许 KRONO_ 前缀环境变量覆盖。
type Config struct {
	Server    ServerConfig
	Redis     RedisConfig
	Postgres  PostgresConfig
	JWT       JWTConfig
	Bootstrap BootstrapConfig
	Jobs      JobsConfig
	Cluster   ClusterConfig // 分布式运行配置（Leader 选举 + 节点心跳注册）
	Timezone  string        // cron 表达式所用时区，默认 Asia/Shanghai；支持 IANA 名（如 America/New_York）或 "Local"
}

// JobsConfig 是 jobs.yaml（调度内核输入，PLAN §五）的加载配置。
type JobsConfig struct {
	Path string `mapstructure:"path"` // jobs.yaml 路径，默认 configs/jobs.yaml
}

// ClusterConfig 控制调度器以分布式方式运行：
//   - 通过 Redis 选主（Leader Election）保证多实例下只有一个节点触发调度，避免重复 dispatch；
//   - 通过调用方主动探测感知下游被调用方（执行节点）存活，配合负载均衡实现多节点分发。
//     下游节点【不】注册心跳（避免臃肿服务适配注册协议），存活完全由 krono-job 主动探测判定。
type ClusterConfig struct {
	Enabled      bool          `mapstructure:"enabled"`       // 是否启用分布式模式（false 时单节点退化运行）
	NodeID       string        `mapstructure:"node_id"`       // 本调度节点唯一标识（缺省自动取 hostname+pid）
	LeaderKey    string        `mapstructure:"leader_key"`    // 选主锁 Redis key，默认 krono:leader
	LeaderTTL    time.Duration `mapstructure:"leader_ttl"`    // 选主锁 TTL，默认 10s
	HealthKey    string        `mapstructure:"health_key"`    // 下游存活探测结果 Redis hash key，默认 krono:node_health
	ProbeTimeout   time.Duration `mapstructure:"probe_timeout"`     // 下游存活探测超时，默认 2s
	DefaultLB      string        `mapstructure:"default_lb"`        // 任务未显式指定 lb_strategy 时使用的默认策略，默认 round_robin
	SkipVerifyTLS  bool          `mapstructure:"skip_verify_tls"`   // 下游为内网自签/证书 IP 不匹配的 HTTPS 时，跳过证书校验直接访问（默认 false，保持安全）
}

// Distributed 便捷判断：是否启用分布式模式。
func (c ClusterConfig) Distributed() bool {
	return c.Enabled
}

type ServerConfig struct {
	Port int
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type PostgresConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	Schema   string `mapstructure:"schema"` // 数据库模式（schema），默认 public；为空时回落到 model.DefaultSchema
	SSLMode  string `mapstructure:"sslmode"`
	MaxConns int32  `mapstructure:"max_conns"`
}

type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"`
}

type BootstrapConfig struct {
	AdminUser string `mapstructure:"admin_user"`
	AdminPass string `mapstructure:"admin_pass"`
}

// DSN 返回 PostgreSQL 连接串。
// 通过 search_path 参数显式设定默认模式，确保后续 SQL 即使省略 schema 也落在目标模式。
func (p PostgresConfig) DSN() string {
	schema := p.Schema
	if schema == "" {
		schema = "public"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s&search_path=%s",
		p.User, p.Password, p.Host, p.Port, p.DBName, p.SSLMode, schema)
}

// Load 读取 yaml 配置并叠加环境变量覆盖（KRONO_<SECTION>_<KEY>）。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("KRONO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// 环境变量仅在「非空」时才覆盖 YAML 中的值。
	// viper 的 AutomaticEnv 会在环境变量存在（即便是空字符串）时用空值覆盖配置文件，
	// 导致 YAML 里已配置的内容被清空，这里手动纠偏。
	applyEnvOverride(&c.Bootstrap.AdminUser, "KRONO_BOOTSTRAP_ADMIN_USER")
	applyEnvOverride(&c.Bootstrap.AdminPass, "KRONO_BOOTSTRAP_ADMIN_PASS")
	applyEnvOverride(&c.JWT.Secret, "KRONO_JWT_SECRET")
	applyEnvOverride(&c.Jobs.Path, "KRONO_JOBS_PATH")

	// jobs.yaml 路径默认值（环境变量优先于默认值）。
	if c.Jobs.Path == "" {
		c.Jobs.Path = "configs/jobs.yaml"
	}

	// 集群配置默认值。
	if c.Cluster.LeaderKey == "" {
		c.Cluster.LeaderKey = "krono:leader"
	}
	if c.Cluster.HealthKey == "" {
		c.Cluster.HealthKey = "krono:node_health"
	}
	if c.Cluster.LeaderTTL <= 0 {
		c.Cluster.LeaderTTL = 10 * time.Second
	}
	if c.Cluster.ProbeTimeout <= 0 {
		c.Cluster.ProbeTimeout = 2 * time.Second
	}
	if c.Cluster.DefaultLB == "" {
		c.Cluster.DefaultLB = "round_robin"
	}

	// cron 时区：默认 Asia/Shanghai，避免依赖运行环境本地时区导致「日志看着像没执行」。
	if c.Timezone == "" {
		c.Timezone = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		// 非法时区名：回落到 Local 并在日志前打告警（调用方通过 TimezoneLocation 拿实际值）。
		c.Timezone = "Local"
	}

	return &c, nil
}

// TimezoneLocation 返回 cron 调度使用的 *time.Location（供 scheduler 使用）。
// 非法时区名已在 Load 时回落为 Local，这里必然成功。
func (c *Config) TimezoneLocation() *time.Location {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}

// applyEnvOverride 当环境变量 key 存在且非空时，用其值覆盖 *dst；否则保持原值（来自 YAML）。
func applyEnvOverride(dst *string, key string) {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		*dst = val
	}
}
