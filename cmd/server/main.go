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
	"go.uber.org/zap"

	"github.com/haioa/krono-job/internal/cluster"
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

	// ===== 分布式集群装配 =====
	// health 为调用方主动探测的下游存活缓存（下游不注册/不写 Redis，零侵入）。
	// 为 nil 时 Worker 退化为单 endpoint 调用（无存活过滤、无 LB）。
	var health *cluster.NodeHealth
	var leader *cluster.Leader
	var nodeID string

	// 调度内核提前声明，供 Leader 选举回调在成为主节点时按需启动。
	sched := scheduler.New(repo.Redis, cfg.Jobs.Path, log, cfg.TimezoneLocation())

	if cfg.Cluster.Distributed() {
		nodeID = cfg.Cluster.NodeID
		if nodeID == "" {
			nodeID = defaultNodeID()
		}
		// 下游存活完全由调用方主动探测（TCP/HEAD/gRPC-health），结果写入 Redis 集中共享，
		// 供前端看板与各实例统一读取；不依赖下游注册心跳。
		// 探测结果在 Redis 的 TTL 取探测间隔的 1.5 倍，超过未刷新即视为节点失联。
		const probeInterval = 10 * time.Second
		healthTTL := probeInterval * 3 / 2
		health = cluster.NewNodeHealth(cfg.Cluster.ProbeTimeout, repo.Redis, cfg.Cluster.HealthKey, healthTTL, cfg.Cluster.SkipVerifyTLS)

		// Leader 选举：只有抢到选主锁的节点才运行 Asynq Scheduler（触发 krono:dispatch），
		// 其余节点作为纯 Worker 消费任务，避免多实例重复触发同一任务。
		leader = cluster.NewLeader(repo.Redis, cfg.Cluster.LeaderKey, nodeID,
			cluster.WithLeaderTTL(cfg.Cluster.LeaderTTL),
			cluster.WithLeaderRefresh(cfg.Cluster.LeaderTTL/3),
			cluster.WithLeaderChangeHook(func(isLeader bool) {
				if isLeader {
					if sched.IsRunning() {
						return
					}
					log.Infow("本节点成为 Leader，启动调度内核", "node_id", nodeID)
					if err := sched.Start(); err != nil {
						log.Warnw("调度内核启动异常", "err", err)
					}
				} else {
					if !sched.IsRunning() {
						return
					}
					log.Infow("本节点退位为 Follower，停止调度内核（仅消费任务）", "node_id", nodeID)
					sched.Shutdown()
				}
			}),
		)
		go leader.Run(context.Background())
		defer leader.Stop()

		// 后台周期探测：遍历所有任务的下游 endpoints，持续探活并写 Redis，
		// 保证前端看板随时能看到被调用方节点存活状态（即使当前没有任务在分发）。
		// 每个分布式实例都参与探测，结果以 endpoint 为粒度共享到同一 Redis key。
		probeCtx, probeCancel := context.WithCancel(context.Background())
		go runHealthProbe(probeCtx, sched, health, nodeID, probeInterval, log)
		defer probeCancel()

		log.Infow("分布式模式已启用，开始参与选主（调用方探测下游存活，下游无需注册）",
			"node_id", nodeID, "probe_timeout", cfg.Cluster.ProbeTimeout, "probe_interval", probeInterval)
	}

	// 调度内核（M3）：加载 jobs.yaml 并注册 Asynq Scheduler，附带 fsnotify 热重载。
	// 非分布式模式：直接启动；分布式模式：交由 Leader 回调按需启动/停止。
	if !cfg.Cluster.Distributed() {
		if err := sched.Start(); err != nil {
			log.Warnw("调度内核启动异常（热重载监听未建立），已忽略", "err", err)
		}
	}
	defer sched.Shutdown()

	// 执行器（M4）：消费 krono:dispatch 任务，按协议分发并记录日志。
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	wkr := worker.New(redisOpt, repo, log, health, nodeID, cfg.Cluster.SkipVerifyTLS)
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

		logsH := handler.NewLogsHandler(repo, cfg.Jobs.Path, cfg.TimezoneLocation())
		jobsH := handler.NewJobsHandler(repo, cfg.Jobs.Path, asynqClient, log)
		statsH := handler.NewStatsHandler(repo, cfg.Jobs.Path, cfg.TimezoneLocation())
		usersH := handler.NewUsersHandler(repo)
		clusterH := handler.NewClusterHandler(health, leader, nodeID, cfg.Cluster.Distributed())

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
		// 可视化编辑：jobs.yaml 增删改查（注意 /jobs/raw 必须在 /jobs/:task_type 之前注册）。
		protected.GET("/jobs/raw", jobsH.ListRaw)
		protected.POST("/jobs/cron-preview", jobsH.CronPreview)
		protected.POST("/jobs", jobsH.Create)
		protected.PUT("/jobs/:task_type", jobsH.Update)
		protected.DELETE("/jobs/:task_type", jobsH.Delete)
		protected.POST("/jobs/:task_type/pause", jobsH.Pause)
		protected.POST("/jobs/:task_type/resume", jobsH.Resume)
		protected.POST("/jobs/:task_type/run", jobsH.Run)

		// 统计看板：总览、每日趋势、调用排行榜（均支持 start/end 时间范围过滤）。
		protected.GET("/stats/overview", statsH.Overview)
		protected.GET("/stats/daily", statsH.Daily)
		protected.GET("/stats/ranking", statsH.Ranking)

		// 分布式集群：查看下游节点存活与选主状态（非分布式模式下返回 disabled）。
		protected.GET("/cluster/nodes", clusterH.Nodes)
		protected.GET("/cluster/status", clusterH.Status)
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

	// 子路径部署（server.base_path 非空，如 /krono-job）时包一层前缀处理器，
	// 兼容两种访问方式：
	//   1) 经 nginx 反代：网关已剥掉前缀并带上 X-Forwarded-Prefix，原样放行给 gin；
	//   2) 直连 :10010：剥掉前缀后交给 gin，并把裸的页面请求重定向到带前缀的地址。
	// 根路径部署时 base_path 为空，handler 就是 gin engine，行为与之前完全一致。
	var handler http.Handler = r
	if cfg.Server.BasePath != "" {
		handler = basePathHandler(cfg.Server.BasePath, r)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: handler,
	}

	go func() {
		if cfg.Server.BasePath != "" {
			log.Infof("server listening on :%d (base path: /%s)", cfg.Server.Port, cfg.Server.BasePath)
		} else {
			log.Infof("server listening on :%d", cfg.Server.Port)
		}
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

// basePathHandler 让「前后端一体」的服务同时支持子路径反代与直连访问。
//
// 注意：前缀剥离必须做在 gin engine 之外。gin 的路由匹配在 handler chain 执行之前就已完成，
// 写在 gin 中间件里改 c.Request.URL.Path 不会触发重新匹配，因此这里用 net/http 层包装。
//
//   - /krono-job/xxx（直连带前缀）    -> 剥成 /xxx 后交给 gin
//   - /xxx 且带 X-Forwarded-Prefix    -> 网关已剥过前缀，原样交给 gin（避免与浏览器形成 302 死循环）
//   - / 等裸页面请求（直连）          -> 302 到 /krono-job/...
//   - /api、/healthz、静态资源请求     -> 原样交给 gin（不跳转，避免影响接口与非 HTML 资源）
func basePathHandler(base string, next http.Handler) http.Handler {
	base = strings.Trim(base, "/")
	if base == "" {
		return next
	}
	prefix := "/" + base

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		// 1) 带前缀：剥掉后交给 gin（直连 :10010/krono-job/... 走这里）
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			u := *r.URL
			u.Path = strings.TrimPrefix(p, prefix)
			if u.Path == "" {
				u.Path = "/"
			}
			r.URL = &u
			next.ServeHTTP(w, r)
			return
		}

		// 2) 网关（nginx）已剥过前缀：原样放行
		if r.Header.Get("X-Forwarded-Prefix") != "" {
			next.ServeHTTP(w, r)
			return
		}

		// 3) 直连的页面导航请求：补前缀后重定向（仅 HTML 导航，避免影响接口与静态资源）
		isHTMLNav := r.Method == http.MethodGet &&
			strings.Contains(r.Header.Get("Accept"), "text/html")
		if isHTMLNav && !strings.HasPrefix(p, "/api") && p != "/healthz" && !strings.Contains(p, ".") {
			target := prefix + p
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
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

// defaultNodeID 生成节点唯一标识：优先 hostname，拼上进程 PID，确保同机多实例不冲突。
func defaultNodeID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// runHealthProbe 是后台周期探测器：遍历所有任务的下游 endpoints，按协议主动探活，
// 结果写入 Redis（由 cluster.NodeHealth 完成），供前端看板与各实例统一读取。
// 即使当前没有任何任务在分发，也能持续刷新被调用方节点的存活状态。
func runHealthProbe(ctx context.Context, sched *scheduler.Scheduler, health *cluster.NodeHealth, nodeID string, interval time.Duration, log *zap.SugaredLogger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// 启动即先探一轮。
	probeOnce(ctx, sched, health, nodeID, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probeOnce(ctx, sched, health, nodeID, log)
		}
	}
}

// probeOnce 对当前所有任务的全部下游 endpoints 做一次探测（按 endpoint 去重）。
func probeOnce(ctx context.Context, sched *scheduler.Scheduler, health *cluster.NodeHealth, nodeID string, log *zap.SugaredLogger) {
	type key struct{ ep, proto string }
	seen := make(map[key]struct{})
	for _, job := range sched.AllJobs() {
		// 仅对启用的、配置了多/单 endpoint 的任务探测。
		for _, ep := range job.TargetEndpoints() {
			k := key{ep, job.Protocol}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			alive, reason := health.Check(ctx, ep, job.Protocol, nodeID)
			log.Debugw("health probe", "endpoint", ep, "protocol", job.Protocol,
				"alive", alive, "by", nodeID, "reason", reason)
		}
	}
}
