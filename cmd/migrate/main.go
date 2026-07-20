// 独立的迁移命令（可选）：go run ./cmd/migrate
// 复用与进程启动相同的嵌入迁移脚本，便于在 CI / 维护时手动执行建表。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/haioa/krono-job/internal/config"
	"github.com/haioa/krono-job/internal/migrate"
	"github.com/haioa/krono-job/scripts"
)

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.Postgres.DSN())
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接 PostgreSQL 失败: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Run(ctx, pool, scripts.MigrationsFS, "migrations"); err != nil {
		fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("全部迁移执行完毕 ✓")
}
