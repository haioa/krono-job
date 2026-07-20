package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	_ "github.com/joho/godotenv/autoload"

	"github.com/haioa/krono-job/internal/config"
	"github.com/haioa/krono-job/internal/handler"
	"github.com/haioa/krono-job/internal/middleware"
	"github.com/haioa/krono-job/internal/migrate"
	"github.com/haioa/krono-job/internal/repository"
	"github.com/haioa/krono-job/internal/scheduler"
	"github.com/haioa/krono-job/internal/service/auth"
	"github.com/haioa/krono-job/internal/worker"
	"github.com/haioa/krono-job/pkg/logger"
	"github.com/haioa/krono-job/scripts"
	"github.com/haioa/krono-job/web"
)

func main() {
	// -f/--config 指定配置文件路径；默认 configs/config.yaml（保持 Docker / 本地开发行为不变）。
	configPath := flag.String("f", "configs/config.yaml", "path to config yaml (e.g. ./etc/krono_job.yaml)")
	flag.Parse()

	log, err := logger.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.JWT.Secret == "" {
		log.Warn("JWT secret 未配置（KRONO_JWT_SECRET），使用随机临时密钥，重启后将失效，生产环境请勿如此")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo, err := repository.New(ctx, cfg, log)
	if err != nil {
		log.Fatalf("init repository: %v", err)
	}
	defer repo.Close()

	// 启动时自动迁移（幂等建表，确保 sys_user 等表存在）
	if err := migrate.Run(ctx, repo.PG, scripts.MigrationsFS, "migrations"); err != nil {
		log.Warnw("数据库迁移未执行（可稍后手动执行 go run ./cmd/migrate 或检查 PG 连接）", "err", err)
	}

	// 鉴权服务 + 首个管理员 bootstrap（表空时按环境变量插入）
	authSvc := auth.New(repo, cfg)
	if err := authSvc.BootstrapAdmin(ctx); err != nil {
		log.Warnw("bootstrap admin 未完成（可稍后手动创建或检查 PG 连接）", "err", err)
	}

	// 调度内核（M3）：加载 jobs.yaml 并注册 Asynq Scheduler，附带 fsnotify 热重载。
	sched := scheduler.New(repo.Redis, cfg.Jobs.Path, log)
	if err := sched.Start(); err != nil {
		log.Warnw("调度内核启动异常（热重载监听未建立），已忽略", "err", err)
	}
	defer sched.Shutdown()

	// 执行器（M4）：消费 krono:dispatch 任务，按协议分发并记录日志。
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	wkr := worker.New(redisOpt, repo, log)
	wkr.Start()
	defer wkr.Shutdown()

	// Asynq 客户端：用于手动执行接口把任务立即投递到队列。
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	r := gin.New()
	r.Use(middleware.Recovery(log), middleware.CORS(), middleware.ContextLog(log))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		authH := handler.NewAuthHandler(authSvc)
		api.POST("/auth/login", authH.Login)

		// M5：受保护路由组，挂载 JWT 中间件。
		// 管理类接口（日志查询、任务列表、暂停/恢复）均需有效 Bearer Token。
		protected := api.Group("")
		protected.Use(middleware.JWT(authSvc))

		protected.POST("/auth/change-password", authH.ChangePassword)

		logsH := handler.NewLogsHandler(repo, cfg.Jobs.Path)
		jobsH := handler.NewJobsHandler(repo, cfg.Jobs.Path, asynqClient, log)
		statsH := handler.NewStatsHandler(repo, cfg.Jobs.Path)
		usersH := handler.NewUsersHandler(repo)

		protected.GET("/logs", logsH.List)
		protected.GET("/logs/task-types", logsH.TaskTypes)
		protected.GET("/logs/:id", logsH.Detail)
		protected.DELETE("/logs", logsH.Delete)

		// 用户管理：增删改查（受 JWT 保护）。
		protected.GET("/users", usersH.List)
		protected.POST("/users", usersH.Create)
		protected.PUT("/users/:id", usersH.Update)
		protected.DELETE("/users/:id", usersH.Delete)
		protected.GET("/jobs", jobsH.List)
		protected.POST("/jobs/:task_type/pause", jobsH.Pause)
		protected.POST("/jobs/:task_type/resume", jobsH.Resume)
		protected.POST("/jobs/:task_type/run", jobsH.Run)

		// 统计看板：总览、每日趋势、调用排行榜（均支持 start/end 时间范围过滤）。
		protected.GET("/stats/overview", statsH.Overview)
		protected.GET("/stats/daily", statsH.Daily)
		protected.GET("/stats/ranking", statsH.Ranking)
	}

	// SPA 静态资源（生产：web/dist 经 go:embed 打包进二进制）。
	// 非 /api 路由一律回退到 index.html，支持前端 history 模式路由。
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		serveSPA(c)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Infof("server listening on :%d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Errorf("graceful shutdown failed: %v", err)
	}
	log.Info("bye")
}

// serveSPA 从内嵌的 web/dist 返回静态资源；未匹配具体文件时回退 index.html
// （支持 Vue history 模式路由，如直接访问 /jobs）。
func serveSPA(c *gin.Context) {
	p := strings.TrimPrefix(c.Request.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if data, err := web.Dist.ReadFile("dist/" + p); err == nil {
		c.Data(http.StatusOK, detectContentType(p), data)
		return
	}
	// SPA 路由回退
	idx, ierr := web.Dist.ReadFile("dist/index.html")
	if ierr != nil {
		c.String(http.StatusNotFound, "前端未构建：请进入 web/ 执行 pnpm build，然后重新构建服务端")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", idx)
}

func detectContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"), strings.HasSuffix(name, ".htm"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
