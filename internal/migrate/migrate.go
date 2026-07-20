// Package migrate 提供启动时幂等建表能力：按文件名顺序执行嵌入的 *.up.sql。
// 迁移语句应包含 IF NOT EXISTS 等幂等操作，可重复安全执行（自修复 schema）。
package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run 按文件名顺序执行 fsys 中 dir 目录下所有 *.up.sql。
// pool 为 nil 时直接返回错误，由调用方决定告警或退出。
func Run(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, dir string) error {
	if pool == nil {
		return fmt.Errorf("postgres 连接池未初始化，跳过迁移")
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("读取迁移目录 %s: %w", dir, err)
	}
	var ups []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		data, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return fmt.Errorf("读取 %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("执行迁移 %s: %w", name, err)
		}
	}
	return nil
}
