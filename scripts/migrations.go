package scripts

import "embed"

// MigrationsFS 内嵌数据库迁移脚本（仅 .up.sql），供 internal/migrate 在启动时自动建表。
// 配合 go:embed 打进二进制，实现单二进制自包含交付；迁移语句均需幂等（含 IF NOT EXISTS）。
//
//go:embed migrations/*.up.sql
var MigrationsFS embed.FS
