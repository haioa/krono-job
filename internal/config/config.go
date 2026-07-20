package config

import (
	"fmt"
	"os"
	"strings"

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
}

// JobsConfig 是 jobs.yaml（调度内核输入，PLAN §五）的加载配置。
type JobsConfig struct {
	Path string `mapstructure:"path"` // jobs.yaml 路径，默认 configs/jobs.yaml
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

	return &c, nil
}

// applyEnvOverride 当环境变量 key 存在且非空时，用其值覆盖 *dst；否则保持原值（来自 YAML）。
func applyEnvOverride(dst *string, key string) {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		*dst = val
	}
}
