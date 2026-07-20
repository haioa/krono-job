package main

import (
	"fmt"
	"os"

	"github.com/haioa/krono-job/internal/config"
	"github.com/haioa/krono-job/internal/scheduler"
)

// cfgcheck 是一个轻量配置校验工具：解析 config.yaml 与 jobs.yaml，
// 在真正启动服务前发现配置错误（如缺字段、cron 非法、task_type 重复）。
//
// 用法：
//
//	go run ./cmd/cfgcheck [config.yaml 路径，默认 configs/config.yaml]
func main() {
	cfgPath := "configs/config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config invalid: %v\n", err)
		os.Exit(1)
	}

	jobs, err := scheduler.LoadJobsFile(cfg.Jobs.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jobs invalid: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: config %q valid, %d jobs defined (path=%s)\n", cfgPath, len(jobs), cfg.Jobs.Path)
}
