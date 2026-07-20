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
