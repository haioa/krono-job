# krono-job

定时任务调度平台：Go（Gin + Asynq）后端 + Vue3 前端，内置 PostgreSQL 与 Redis 的**一体化**部署镜像。

> 单次 `docker run` 即可独立运行全部组件（Web/API + 调度内核 + 工作进程 + PostgreSQL + Redis），无需外部依赖。

---

## 1. 镜像构建

构建为多阶段镜像：`node` 构建前端 → `golang` 用 vendored 依赖离线构建二进制 → `ubuntu` 内置 PostgreSQL 16 + Redis 7。

```bash
# 标准构建（需可访问 Docker Hub）
docker build -t krono-job:latest -f build/Dockerfile .

# 国内网络：使用镜像加速源（base 镜像 + apt 源均可覆盖）
docker build -t krono-job:latest \
  --build-arg BASE_REGISTRY=docker.m.daocloud.io \
  --build-arg APT_MIRROR=http://mirrors.aliyun.com/ubuntu \
  -f build/Dockerfile .
```

构建前提：

- `web/dist` 由镜像内 `node` 阶段重新构建，无需本地预构建。
- gRPC 类任务直接在 `configs/jobs.yaml` 中以 `protocol: grpc` 声明即可（见下方「gRPC 任务配置」），无需改动代码或引入额外依赖。

---

## 2. 运行（单镜像独立运行）

```bash
docker run -d --name krono-job \
  -p 10010:10010 \
  -p 5432:5432 \
  -p 6379:6379 \
  -e KRONO_JWT_SECRET=请改成随机长字符串 \
  krono-job:latest
```

- 平台 Web / API：`http://localhost:10010`
- PostgreSQL：`localhost:5432`（用户 `krono` / 密码 `kronopass`，库 `krono_job`）
- Redis：`localhost:6379`（无密码）

---

## 3. 运行（docker-compose，附带 asynqmon 监控）

```bash
docker compose -f build/docker-compose.yml up -d
```

- 平台：`http://localhost:10010`
- asynqmon 队列监控：`http://localhost:8080`（连接 `krono:6379`）

---

## 4. 配置（环境变量）

所有配置均可通过环境变量（或 `.env` 文件）注入，环境变量优先级高于 `configs/config.yaml` 中的默认值。

**使用 `.env` 文件（推荐，避免敏感信息入库）**：

1. 复制模板：`cp .env.example .env`
2. 编辑 `.env` 填入真实值（数据库密码、JWT 密钥、管理员密码等敏感项仅存在于 `.env`，
   `.env` 已被 `.gitignore` 忽略，不会提交到仓库）。
3. 程序已引入 `github.com/joho/godotenv/autoload`，启动时自动加载运行目录下的 `.env`；
   容器部署也可通过 `docker run --env-file .env` 或 docker-compose 的 `env_file` 注入。

变量命名规则：`KRONO_<SECTION>_<KEY>`（与 `configs/config.yaml` 层级对应，`.` 换成 `_`），
完整列表与说明见仓库根目录 `.env.example`。`configs/config.yaml` 现仅保留非敏感默认值，
敏感项（密码 / 密钥 / 初始管理员密码）一律由环境变量提供。

以下为镜像内置默认值（均可被环境变量覆盖）：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `KRONO_SERVER_PORT` | `10010` | HTTP 监听端口 |
| `KRONO_REDIS_ADDR` | `127.0.0.1:6379` | Redis 地址（容器内） |
| `KRONO_REDIS_PASSWORD` | 空 | Redis 密码 |
| `KRONO_POSTGRES_HOST` | `127.0.0.1` | PostgreSQL 主机（容器内） |
| `KRONO_POSTGRES_PORT` | `5432` | PostgreSQL 端口 |
| `KRONO_POSTGRES_USER` | `krono` | 数据库角色 |
| `KRONO_POSTGRES_PASSWORD` | `kronopass` | 数据库密码 |
| `KRONO_POSTGRES_DBNAME` | `krono_job` | 数据库名 |
| `KRONO_POSTGRES_SCHEMA` | `public` | 模式 |
| `KRONO_POSTGRES_SSLMODE` | `disable` | SSL 模式 |
| `KRONO_BOOTSTRAP_ADMIN_USER` | `admin` | 首个管理员用户名（表空时插入） |
| `KRONO_BOOTSTRAP_ADMIN_PASS` | `admin123` | 首个管理员密码（**生产务必修改**） |
| `KRONO_JWT_SECRET` | `change-me-in-prod` | JWT 签名密钥（**生产务必修改**） |

PostgreSQL 的**角色 / 数据库 / `uuid-ossp` 扩展**由 `build/entrypoint.sh` 在容器启动时幂等创建，
平台二进制启动时自动执行表迁移与管理员 bootstrap，无需手动初始化。

调度任务在 `configs/jobs.yaml` 中声明，支持 fsnotify 热重载（修改文件无需重启）。

### gRPC 任务配置

gRPC 任务纯声明式，无需编写对接代码。在 `configs/jobs.yaml` 的 `jobs` 列表中新增一项，
指定下游 `endpoint`（host:port）、服务方法、请求类型与载荷即可：

```yaml
jobs:
  - name: 示例-gRPC调用
    task_type: example_grpc_call        # 全局唯一，用于日志与暂停态标记
    cron: "0,30 * * * *"                # 标准 5 字段 cron：分 时 日 月 周
    protocol: grpc                      # 声明为 gRPC 协议
    enabled: true
    timeout: 30s                        # 单次调用超时，超时按失败处理
    retry: 2                            # Asynq 最大重试次数
    endpoint: "svc-host:9000"           # 下游 gRPC 服务 host:port
    grpc_service: "pkg.Service"         # 包.服务，如 sdc.Invoke
    grpc_method: "MethodName"           # 方法名，最终调用 /pkg.Service/MethodName
    request_type: "pkg.ReqTypeName"     # 请求消息全类型名，用于反序列化 payload
    payload:                            # 方法入参，按 request_type 序列化后发出
      field_a: "value"
      field_b: 123
    metadata:                           # 可选：gRPC metadata（如鉴权头）
      authorization: "Bearer ${SVC_TOKEN}"
```

字段说明：

- `protocol: grpc`：触发 gRPC 适配分支；`endpoint` 为目标服务地址，`grpc_service`/`grpc_method` 拼出方法全路径。
- `request_type`：调度内核据此将 `payload` 反序列化为对应 proto 请求类型，无需在代码中 import 具体 pb。
- `metadata`：可选 gRPC metadata；值支持 `${ENV}` 引用环境变量（启动时替换）。
- 其余 `cron` / `timeout` / `retry` 与 HTTP 任务语义一致。

---

## 5. 数据持久化

镜像内 PostgreSQL 数据目录为 `/var/lib/postgresql/data`。如需跨容器重建保留数据，挂载卷：

```bash
docker run -d --name krono-job \
  -p 10010:10010 -p 5432:5432 -p 6379:6379 \
  -v krono-pgdata:/var/lib/postgresql/data \
  krono-job:latest
```

---

## 6. 说明与排查

- 健康检查：`GET /healthz` 返回 `{"status":"ok"}`。
- 启动日志可见 Redis / PostgreSQL 初始化、角色库扩展创建、Scheduler/Worker 启动。
- 已修复：`sdc_refresh_user_token` 任务的 cron 表达式原采用 Quartz 风格（`0 0,30 * * * ?`），在 asynq 标准 5 字段
  cron 下被误解析为「小时 30」而报错；已改为标准写法 `0,30 * * * *`（每 30 分钟触发一次）。
- 构建若提示 `exec format error`：确保 `build/entrypoint.sh` 为 Unix 换行（LF，无 BOM）。

---

## 7. 本地运行（不使用 Docker）

适用：希望在本机直接用 `go run` 运行，依赖本机已安装的 PostgreSQL 与 Redis（不打包进镜像）。

### 7.1 前置依赖

| 依赖 | 版本要求 | 说明 |
| --- | --- | --- |
| Go | 1.26.5+ | 与 `go.mod` 一致 |
| Node.js | ≥ 22.18.0 或 ≥ 24.12.0 | 仅用于构建前端 |
| pnpm | 随 Node corepack 启用 | `corepack enable` |
| PostgreSQL | 16+ | 需本机运行，并创建角色 / 库 / 扩展 |
| Redis | 7+ | 需本机运行 |

Windows 可用包管理器安装：`scoop install postgresql redis` 或 `choco install postgresql redis-64`。

### 7.2 准备 PostgreSQL 与 Redis

1. 启动本机 PostgreSQL、Redis 服务。
2. 以 superuser（`postgres`）执行 `build/init-db.sql`，幂等创建 `krono` 角色、`krono_job` 库并预置 `uuid-ossp` 扩展：

   ```bash
   psql -U postgres -f build/init-db.sql
   ```

   > 为何手动跑？`uuid-ossp` 扩展需 superuser 权限创建；平台启动时的自动迁移以 `krono` 角色执行 `CREATE EXTENSION IF NOT EXISTS`，若扩展已存在则为无操作。跳过此步会导致迁移失败。

3. 确认 Redis 监听 `127.0.0.1:6379`，无密码（默认）。

### 7.3 配置敏感项

```bash
cp .env.example .env
# 编辑 .env 填入：KRONO_JWT_SECRET、KRONO_BOOTSTRAP_ADMIN_PASS、KRONO_POSTGRES_PASSWORD 等
```

本地默认值说明（`configs/config.yaml`，与镜像默认值不同）：

- PostgreSQL：角色 `krono` / 密码 `password`（注意：镜像内为 `kronopass`），库 `krono_job`；
- Redis：`127.0.0.1:6379`，DB `15`（注意：镜像内为 DB `0`）；
- 若直接使用默认 `config.yaml`，请确保 7.2 中创建的 `krono` 角色密码为 `password`；否则用 `.env` 的 `KRONO_POSTGRES_PASSWORD` 覆盖。

### 7.4 构建前端（须先于后端编译）

前端产物 `web/dist` 通过 `go:embed` 打包进后端二进制，因此**必须先在 `web/` 构建前端**：

```bash
cd web
pnpm install
pnpm build          # 等价于 build-only（跳过 vue-tsc 类型检查）
# 如需严格类型检查：pnpm build
```

产物生成于 `web/dist`；仓库已附带一份 `web/dist`，若仅修改后端可直接跳过本步。

### 7.5 运行（`go run`）

```bash
# 从仓库根目录执行，默认加载 configs/config.yaml
go run ./cmd/server

# 或显式指定配置文件
go run ./cmd/server -f configs/config.yaml
```

启动后平台会：自动迁移建表 → 创建首个管理员（账号 `admin` / 密码取自 `.env` 的 `KRONO_BOOTSTRAP_ADMIN_PASS`）→ 启动 Asynq Scheduler 与 Worker。

- 平台 Web / API：`http://localhost:10010`
- 健康检查：`GET /healthz` → `{"status":"ok"}`

可选：单独执行数据库迁移（建表 / 扩表）：

```bash
go run ./cmd/migrate
```

---

## 8. 本地编译二进制

### 8.1 编译

确保前端已构建（`web/dist` 就绪）后，使用已落地的 `vendor/` 离线依赖编译：

```bash
go build -mod=vendor -ldflags="-s -w" -o bin/krono-job ./cmd/server
```

- `-mod=vendor`：使用 `vendor/`（含 `gitee.com` 私有依赖），无需访问网络或 Git 凭证；
- `-ldflags="-s -w"`：去除调试信息，减小体积；
- 二进制已内置前端（`web/dist`）与自动迁移、管理员 bootstrap，单文件即可运行。

### 8.2 运行二进制

```bash
# configs/（含 config.yaml、jobs.yaml）需与运行目录相对路径一致
./bin/krono-job -f configs/config.yaml
```

> 二进制默认读取**当前工作目录**下的 `configs/config.yaml` 与 `configs/jobs.yaml`。若把二进制放到别处，请一并复制 `configs/` 目录，或用 `-f` 指定绝对路径。

### 8.3 跨平台交叉编译

本项目无 CGO 依赖，可直接交叉编译：

```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -mod=vendor -ldflags="-s -w" -o bin/krono-job-linux-amd64 ./cmd/server

# macOS arm64
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -mod=vendor -ldflags="-s -w" -o bin/krono-job-darwin-arm64 ./cmd/server

# Windows amd64（PowerShell）
$env:CGO_ENABLED=0
go build -mod=vendor -ldflags="-s -w" -o bin/krono-job.exe ./cmd/server
```

### 8.4 常见问题

- 前端页面空白 / 提示「前端未构建」：未先构建 `web/dist`，请回到 7.4 构建前端后再编译后端。
- 迁移报 `permission denied to create extension "uuid-ossp"`：未以 superuser 执行 `build/init-db.sql`，见 7.2。
- 连接 PG / Redis 失败：确认本机服务已启动，且地址 / 密码与 `.env` 或 `config.yaml` 一致（本地默认见 7.3）。
- 找不到 `configs/jobs.yaml`：在运行目录下确认 `configs/` 存在，或用 `-f` 指定配置路径（`KRONO_JOBS_PATH` 也可经环境变量覆盖）。
