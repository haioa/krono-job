# krono-job 项目计划 / 设计文档（v0.2）

> 用途：实现前的蓝图，已通过评审，所有【待确认】项已闭合。本文为后续编码的权威依据。
> 状态：设计冻结，待按 M0~M7 顺序实现。

---

## 一、项目定位

一个**零侵入、单二进制交付**的分布式定时任务调度平台：

- 调度内核基于 `Asynq`（Redis 上的延迟/定时任务队列）。
- 管理后台：Gin + Vue3（go:embed 打包进二进制），仅负责登录鉴权、执行日志查询、任务暂停/恢复。
- 下游业务微服务**被动接收触发**（HTTP Webhook / gRPC），无需集成 SDK。
- 运维监控（重试、死信、队列调优）交由独立部署的 `Asynqmon` UI，与本服务解耦。

---

## 二、技术栈（已确定）

| 领域 | 选型 |
|------|------|
| 语言 | Go 1.26+ |
| Web 框架 | Gin |
| 调度内核 | Asynq（Scheduler + Worker，Redis 后端） |
| 前端 | Vue3（构建产物 `web/dist/` 经 `go:embed` 打入二进制） |
| 数据库 | PostgreSQL 16（**仅 2 张表**） |
| 缓存/队列 | Redis 7 |
| 配置 | Viper / YAML（config.yaml + jobs.yaml） |
| 日志 | Zap / ZeroLog（pkg/logger） |
| 鉴权 | JWT（HS256），Secret 经环境变量注入 |
| 密码哈希 | bcrypt |
| 部署 | Docker 多阶段 + docker-compose（含 Asynqmon 独立容器） |

---

## 三、系统架构

```
                               ┌──────────────────────────────────────────────┐
                               │     Asynqmon UI (独立运维监控服务)             │
                               │  (通过 Docker/独立进程运行，直连 Redis 端口)    │
                               └──────────────────────┬───────────────────────┘
                                                      │ (直连监控队列)
                                                      ▼
【Vue3 管理后台】 ───(JWT)───► 【Gin 平台网关】 ───► 【Redis 消息/调度总线】
   (go:embed 打包)               │                      ▲
                                 │                      │ (Cron 定时推送)
                                 ▼                      │
                       【PG 数据库 (仅2张表)】   【Asynq Scheduler 调度器】
                       (管理员认证 + 执行日志)         │ (加载 jobs.yaml)
                                                        │
                                                        ▼
                                           【Asynq Worker 通用分发器】
                                                        │
                                         ┌──────────────┴──────────────┐
                                         ▼                             ▼
                              【HTTP Webhook 调用】            【gRPC 接口调用】
                                         │                             │
                                         └──────────────┬──────────────┘
                                                        ▼
                                               【下游业务微服务集群】
                                            (被动接收触发，无需集成 SDK)
```

---

## 四、数据库设计（PostgreSQL，仅 2 张表）

> **UUID 约定**：所有主键使用 `uuid_generate_v4()`，依赖 `uuid-ossp` 扩展。migration 首行必须 `CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`（PG 13+ 亦可用内置 `gen_random_uuid()`，但本项目统一用 `uuid_generate_v4()`）。

### 4.1 管理员用户表 `sys_user`

```sql
CREATE TABLE sys_user (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    username text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    nickname text,
    status text NOT NULL DEFAULT 'active', -- 'active', 'disabled'
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_sys_user_username ON sys_user(username);
```

### 4.2 调度执行日志表 `job_exec_log`

```sql
CREATE TABLE job_exec_log (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_type text NOT NULL,
    trigger_type text NOT NULL,      -- 'cron' / 'manual'
    protocol text NOT NULL,          -- 'http' / 'grpc'
    target_endpoint text NOT NULL,
    status text NOT NULL,            -- 'success', 'failed', 'skipped'
    response_body text,              -- 超长截断至 8KB
    error_msg text,
    retry_count integer NOT NULL DEFAULT 0,  -- 实际重试次数（决策 2-A）
    start_at timestamptz NOT NULL,
    end_at timestamptz,
    execution_duration_ms bigint,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_job_exec_log_task_type ON job_exec_log(task_type);
CREATE INDEX idx_job_exec_log_created_at ON job_exec_log(created_at DESC);
```

### 4.3 任务定义来源（不落库）

任务定义**只存在于 `jobs.yaml`**（声明式），任务的运行态（暂停与否）存 Redis（见 4.4），因此 PG 维持仅 2 张表。

### 4.4 暂停状态（Redis，决策 1-A）

- Key：`krono:paused_tasks`（Redis Set），成员为 `task_type` 字符串。
- 暂停：`SADD krono:paused_tasks {task_type}`
- 恢复：`SREM krono:paused_tasks {task_type}`
- 判断：Worker 分发前 `SISMEMBER`；命中则本次调度落库 `status='skipped'` 并跳过调用。
- 持久化：依赖 Redis AOF/RDB，重启不丢；与 Asynq 共用同一 Redis，零新增依赖。

Migration 脚本落于：`scripts/migrations/000001_init_schema.up.sql`

---

## 五、jobs.yaml Schema（决策 4，调度内核输入）

`jobs` 为任务数组，每条任务声明式定义一个定时触发。按 `protocol` 分为 HTTP / gRPC 两类，下方各给一份完整示例。

### 5.1 HTTP 任务示例

```yaml
jobs:
  - name: 订单超时自动取消            # 展示名（前端任务列表展示用）
    task_type: order_timeout_cancel  # 【必填】全局唯一标识，写入 job_exec_log.task_type
    cron: "0 */5 * * *"              # 【必填】标准 5 段 cron，每 5 分钟触发一次
    protocol: http                   # 协议类型：http
    enabled: true                    # 是否纳入调度（默认 true；false 则不注册）
    timeout: 30s                     # 单次调用超时（默认 30s），超时按失败处理
    retry: 3                         # Asynq 最大重试次数（默认 0，即不重试）
    endpoint: "http://order-svc:8080/api/timeout/cancel"  # 【必填】目标 URL
    method: POST                     # http 专用，请求方法（默认 POST）
    headers:                         # http 专用，可选自定义请求头
      Authorization: "Bearer ${TOKEN}"   # 值支持 ${ENV} 引用启动环境变量
      X-Source: "krono-job"
    payload:                         # http 专用，请求体（序列化为 JSON 发送）
      scene: "auto_expire"
      batch_size: 100
    # grpc_* 字段在 http 任务中可省略
```

> HTTP 任务为**纯配置**：填好上述字段即可生效，无需在 krono-job 内写任何对接代码。

### 5.2 gRPC 任务示例

```yaml
jobs:
  - name: 商品库存同步              # 展示名
    task_type: product_stock_sync  # 【必填】全局唯一标识，写入日志
    cron: "0 2 * * *"              # 每天凌晨 2 点触发
    protocol: grpc                 # 协议类型：grpc
    enabled: true
    timeout: 60s
    retry: 2
    endpoint: "product-svc:9000"   # 【必填】下游 gRPC 服务地址 host:port
    grpc_service: "mall.Product"   # grpc 专用：服务全名（包.服务），用于构造完整方法名
    grpc_method: "SyncStock"       # grpc 专用：方法名
    request_type: "mall.ProductSyncStockRequest"  # grpc 专用：请求消息全名，通用适配器据此反序列化 payload
    payload:                       # grpc 专用：该方法的请求参数（JSON），全部在 yaml 定义，代码不写死
      warehouse_id: 1001
      full_sync: true
    metadata:                      # grpc 专用：随 RPC 发送的 metadata（等价 HTTP Header，会经网络传输）
      authorization: "Bearer ${TOKEN}"   # 下游用 metadata.FromIncomingContext(ctx) 读取；支持 ${ENV}
      x-krono-task: "product_stock_sync"
    # method / headers 在 grpc 任务中无效，可省略
```

> **gRPC 接入注意（对应决策 13）**：因下游在公网、反射关闭，gRPC 走「import pb 包 + 通用适配器」方案，**参数全部由 yaml 定义，不在代码写死**。
> - **参数不写死**：`payload` 即该方法的请求体（JSON）。通用适配器用 `protojson.Unmarshal(payload, req)` 把 yaml 参数字典填充进由 `request_type` 指定的强类型消息，字段映射由 protobuf 描述符自动完成，无需代码逐字段硬编码。
> - **通用调用（无需每方法写代码）**：通过 `conn.Invoke(ctx, "/{包}.{Service}/{Method}", req, reply)` 低层通用入口调用（grpcurl 同源原理），配合全局 proto 类型注册表。整个 krono-job 只需**一个通用适配器函数**即可覆盖所有下游方法。
> - **新增下游服务仅一行 import**：在 krono-job 中 `import _ "downstream/pb"`（一行，触发其 `init` 把消息描述符注册进全局类型表），无需逐方法对接代码。与 HTTP「纯 yaml」的差距仅此一行。
> - **metadata 传递（关键）**：gRPC 网络传输中 `ctx.Value` 不会发送，必须走 `metadata`。适配器调用前用 `metadata.NewOutgoingContext(ctx, md)` 把 `metadata` 字段（及系统注入 `x-trace-id`/`x-task-type`）挂上，下游才能 `metadata.FromIncomingContext(ctx)` 取到。
> - 即：**加 HTTP 任务 = 只改 yaml；加 gRPC 任务 = 改 yaml + 为目标下游 pb 包加一行 `import _`**（参数仍全在 yaml）。若未来下游开反射，这一行 import 也可省。

### 5.3 字段说明（两类共用）

| 字段 | 必填 | 适用 | 说明 |
|------|------|------|------|
| `task_type` | ✅ | 通用 | 全局唯一，日志关联键、暂停态 Redis 成员 |
| `name` | | 通用 | 展示用名称 |
| `cron` | ✅ | 通用 | 标准 5 段 cron 表达式 |
| `protocol` | ✅ | 通用 | `http` / `grpc` |
| `enabled` | | 通用 | 是否纳入调度，默认 `true` |
| `timeout` | | 通用 | 单次调用超时，默认 `30s` |
| `retry` | | 通用 | Asynq 最大重试次数，默认 `0` |
| `endpoint` | ✅ | 通用 | http=目标 URL；grpc=host:port |
| `method` | | http | 请求方法，默认 `POST` |
| `headers` | | http | 自定义请求头，值支持 `${ENV}` 引用环境变量 |
| `payload` | | 通用 | http=请求体(JSON)；grpc=方法请求参数(JSON)，由通用适配器反序列化，参数不写死在代码 |
| `grpc_service` | | grpc | 服务全名（包.服务），用于构造完整方法名 `/包.服务/方法` |
| `grpc_method` | | grpc | 方法名 |
| `request_type` | | grpc | 请求消息全名（如 `mall.ProductSyncStockRequest`），通用适配器据此反序列化 `payload` |
| `metadata` | | grpc | 随 RPC 发送的 gRPC metadata（经网络传输，下游用 `metadata.FromIncomingContext` 读取）；支持 `${ENV}` 引用环境变量 |

> `${VAR}` 占位说明：`headers` 中的值支持 `${环境变量名}` 在启动时替换（如 `${TOKEN}`），便于密钥不落地；`payload` 内的 `${var}` 模板（运行期动态替换）为**可选增强**，MVP 阶段可先作为静态值透传。

---

## 六、核心流程

### 6.1 调度触发流程
1. `scheduler/` 启动读取 `jobs.yaml`，向 Asynq Scheduler 注册 `enabled=true` 的任务。
2. Cron 到点 → Scheduler 推任务入 Redis 队列。
3. `worker/` 取出任务，先查 Redis `krono:paused_tasks`：命中 → 落库 `skipped` 跳过。
4. 未暂停 → 按 `protocol` 分发 HTTP / gRPC，采集响应/错误。
5. **任务最终结束**时落库一条 `job_exec_log`（决策 2-A）：成功→`success`；耗尽重试→`failed`；并写入实际 `retry_count`。

### 6.2 暂停 / 恢复
- API → `SADD`/`SREM` Redis Set；Worker 在分发前拦截。

### 6.4 手动执行（前端点击触发）
- 接口 `POST /api/jobs/:task_type/run`：后端从 `jobs.yaml` 取出任务定义，解析 `${ENV}` 后标记 `trigger_type='manual'`，以 `asynq.ProcessIn(0)` 立即入队（复用 `krono:dispatch` 任务类型与默认队列）。
- Worker（M4）消费该任务时：因 `trigger_type=manual`，**跳过 Redis 暂停态检查**，确保用户显式触发必定执行；并在落库日志时写入 `trigger_type='manual'`（决策 2-A 的单事件单条日志不变）。
- 该接口仅负责"投递"，为异步执行；前端点击后给出"已投递"提示，执行结果在执行日志页（§七 日志查询）查看。

### 6.3 热重载（fsnotify 监听 jobs.yaml）
- 新增任务 → 注册 Cron；删除任务 → `Scheduler.Remove` 清理旧注册；变更 → 先删后注册，避免重复。
- YAML 解析失败 → 保留旧配置，记错误日志并告警，不中断运行。

---

## 七、API 契约（决策 5，前后端并行依据）

**认证**
- `POST /api/auth/login` ← `{username, password}` → `{token, expires_at, user:{id,username,nickname}}`
- 后续请求头：`Authorization: Bearer <token>`

**日志查询（分页 + 过滤）**
- `GET /api/logs?page=1&page_size=20&task_type=&status=&protocol=&start=&end=&sort=created_at:desc`
- → `{total, page, page_size, list:[ job_exec_log 行 ]}`

**任务控制**
- `GET /api/jobs` → `[{task_type,name,cron,protocol,endpoint,enabled,paused}]`（paused 由 Redis 实时读）
- `POST /api/jobs/:task_type/pause` → `{task_type, paused:true}`
- `POST /api/jobs/:task_type/resume` → `{task_type, paused:false}`
- `POST /api/jobs/:task_type/run` → `{task_type, trigger_type:"manual", message}`（202 Accepted）。立即把任务投递到 Asynq 队列执行（手动触发）。手动触发**不受暂停态影响**（Worker 侧据此跳过 paused 检查），执行结束写入 `trigger_type='manual'` 的 `job_exec_log`，结果可在 `/api/logs` 查询。

**运维**
- `GET /healthz` → 200

**前端托管**：Gin 以 `go:embed` 托管 `web/dist`，`/` 返回 Vue 应用，`/api/*` 走接口。

---

## 八、决策闭合汇总（原待确认项已全部解决）

| 编号 | 原问题 | 决策 |
|------|--------|------|
| 1 | jobs.yaml Schema | §五 模板，字段表已定 |
| 2 | 暂停态存储 | **Redis Set `krono:paused_tasks`**，保留 2 张表（决策 1-A） |
| 3 | 日志与重试关系 | **每事件一条日志** + `retry_count` 字段；`failed`=耗尽重试（决策 2-A） |
| 4 | 首个管理员 | **环境变量 bootstrap**（决策 3-A）：`BOOTSTRAP_ADMIN_USER`/`BOOTSTRAP_ADMIN_PASS`，表空则插入 |
| 5 | 前后端 API | §七 已规范 REST 路径与 JSON |
| 6 | UUID 生成 | 全表 `uuid_generate_v4()` + `uuid-ossp` 扩展 |
| 7 | 密码哈希 | bcrypt |
| 8 | JWT | HS256，Secret 环境变量注入，有效期 24h |
| 9 | 部署地址 | compose 内 PG/Redis 用服务名 `postgres`/`redis` |
| 10 | 健康检查 | 暴露 `/healthz` + 优雅关停 |
| 11 | 日志截断 | `response_body` 超 8KB 截断 |
| 12 | 构建顺序 | Dockerfile 多阶段：先 `npm run build` 再 `go build`（go:embed 需 dist 存在） |
| 13 | gRPC 调用方案 | **反射关闭（公网环境）**，采用 **import 下游 pb 包 + 通用适配器**。**参数全部由 yaml 定义、不写死在代码**：通用适配器用 `protojson.Unmarshal(payload, req)` + `conn.Invoke("/包.服务/方法", ...)` 调用，配合全局 proto 类型注册表。新增下游服务仅需 `import _ "downstream/pb"`（一行，注册描述符），无需逐方法对接代码。 |

---

## 九、实施里程碑（M0~M7，可执行任务）

### M0 脚手架
- [x] `go.mod` 添加依赖：gin, asynq, viper, pgx/sqlx, go-redis, jwt, bcrypt, zap, yaml, fsnotify。
- [x] 建立目录结构（§五布局）。
- [x] `internal/config`：Viper 加载 `config.yaml`（Redis/PG/JWT/端口/环境变量覆盖）。
- [x] `pkg/logger`：Zap 封装，输出 stdout。
- [x] `internal/repository`：PG（pgx 连接池）+ Redis 客户端初始化。
- [x] `cmd/server/main.go`：装配各组件骨架，提供 `/healthz`。

### M1 数据层
- [x] `scripts/migrations/000001_init_schema.up.sql`：`uuid-ossp` 扩展 + 2 张表（含 `retry_count`）。
- [x] `internal/model`：Entity（SysUser, JobExecLog）、DTO、Redis Key 常量。
- [x] `repository`：sys_user CRUD、job_exec_log 写入与分页查询（按 task_type/status/protocol/时间区间过滤）。

### M2 鉴权
- [x] `service/auth`：bcrypt 校验 + JWT 签发（HS256, 24h）/ 解析。
- [x] `middleware/jwt`：Bearer 校验；`middleware/cors`、`recovery`、`context log`。
- [x] `handler`：`POST /api/auth/login`。
- [x] 启动 bootstrap：表空时按 `BOOTSTRAP_ADMIN_USER`/`BOOTSTRAP_ADMIN_PASS` 插入管理员。

### M3 调度内核
- [x] `internal/scheduler`：解析 `jobs.yaml` → 向 Asynq Scheduler 注册 Cron（`enabled`、`retry`、`timeout` 映射为 Asynq 选项）。
- [x] fsnotify 热重载：增/删/改逻辑（先删后注册）。
- [x] YAML 解析失败保护：保留旧配置 + 错误日志。

### M4 执行器
- [x] `internal/worker`：Asynq Worker 注册；分发前查 Redis 暂停态（命中→`skipped`）。
- [x] `pkg/httputil`：Webhook 发送（method/headers/payload/timeout）。【HTTP 任务：纯配置，无需代码】
- [x] `pkg/grpcpool`：gRPC 连接池（按 `endpoint` 复用下游连接）。
- [x] `internal/worker/adapter`：gRPC **通用**适配器——基于 `conn.Invoke` + `protojson` + 全局 proto 类型注册表，用 `request_type` 反序列化 yaml `payload` 并通用调用；下游 pb 包仅作 `import _` 注册描述符，**参数不写死在代码**。【每新增下游服务：yaml 配置 + 一行 `import _`】
- [x] 调用后采集响应/错误，任务结束写一条 `job_exec_log`（含 `retry_count`）。

### M5 管理 API
- [x] `handler`：`GET /api/logs`（分页/过滤/排序）、`GET /api/jobs`（含 paused）、`POST /api/jobs/:task_type/pause|resume`（操作 Redis Set）、`POST /api/jobs/:task_type/run`（手动执行，立即入队，绕过暂停态，日志 `trigger_type='manual'`）。
- [x] `middleware.JWT` 挂载：上述接口全部纳入受保护路由组，需有效 Bearer Token。
- [x] 暂停态数据源统一：paused 读/写均经 Redis Set `krono:paused_tasks`（`repository.PauseTask/ResumeTask/PausedTaskMembers`），与 M4 Worker 共用，零额外表。

### M6 前端
- [x] `web/src`：Vue3 后台（登录、任务列表+暂停/恢复、日志分页/过滤/排序），含路由守卫与 Pinia 鉴权 store。
- [x] 对接 §七 API：`@/api`（login/getJobs/pauseJob/resumeJob/runJob/getLogs）基于原生 fetch，自动注入 Bearer、统一 401 处理。任务列表支持「立即执行」按钮（调用 runJob，投递后提示，结果见日志页）。
- [x] `web/embed.go`：`//go:embed dist`；`cmd/server/main.go` 注册 NoRoute 静态托管 + history 回退；`npm run build` 产出 dist 已验证。
- [x] 开发体验：`vite.config.ts` 增加 `/api` 代理到后端（默认 10010），`pnpm dev` 即可联调。
- [x] 商业化视觉重构：`web/src/style.css` 统一设计系统（靛蓝主色、柔和阴影、圆角、状态药丸、开关控件、骨架屏、加载动画）；登录页改为品牌双栏布局；侧边栏深色渐变 + 顶栏 + 内联 SVG 图标；任务页含概览卡片 + 暂停/恢复开关 + 立即执行；日志页含过滤栏 + 可排序列 + 分页输入。

### M7 部署
- [ ] `build/Dockerfile`：多阶段（node 构建前端 → golang 构建二进制 → 轻量运行镜像）。
- [ ] `build/docker-compose.yml`：cron-platform + asynqmon + redis + postgres（服务名互联）。
- [ ] `README.md`：启动、配置、bootstrap 说明。

---

## 十、评审结论

- [x] 信息是否齐全：**是（v0.2 全部闭合）**
- [x] 是否突破"仅 2 张表"约束：**否（暂停态存 Redis，决策 1-A）**
- [x] 其余待确认项是否全部闭合：**是**
