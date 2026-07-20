# ============================================================
# 阶段 1：构建前端（Vue3 -> web/dist）
# 若 Docker Hub 不可达，可加 --build-arg BASE_REGISTRY=<镜像站> 指定加速源，例如：
#   --build-arg BASE_REGISTRY=docker.m.daocloud.io
# 或 docker.m.daocloud.io / registry.cn-hangzhou.aliyuncs.com 等。
# ============================================================
ARG BASE_REGISTRY=docker.io
FROM ${BASE_REGISTRY}/library/node:24-bookworm AS frontend
WORKDIR /web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && corepack prepare pnpm@11.13.1 --activate
RUN pnpm install --frozen-lockfile
COPY web/ ./
# 仅做产物构建（跳过 vue-tsc 类型检查，避免部署构建被类型问题阻塞）
RUN pnpm run build-only

# ============================================================
# 阶段 2：构建后端二进制（go:embed 需要 web/dist 已存在）
# ============================================================
FROM ${BASE_REGISTRY}/library/golang:1.26-bookworm AS backend
WORKDIR /src
# 依赖已通过 `go mod vendor` 落地到 vendor/（含 gitee.com 私有依赖），
# 构建时完全离线，无需访问网络或 Git 凭证。
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .
COPY --from=frontend /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /out/krono-job ./cmd/server

# ============================================================
# 阶段 3：一体化运行镜像
#   Go 二进制 + PostgreSQL 16 + Redis 7，全部打包进单个镜像，
#   可直接 `docker run` 独立运行，无需外部 PG/Redis。
# ============================================================
FROM ${BASE_REGISTRY}/library/ubuntu:24.04 AS runtime
# apt 镜像源（Docker 构建环境常无法访问 archive.ubuntu.com，默认用国内镜像；
# 可经 --build-arg APT_MIRROR=<其它镜像> 覆盖，如 http://mirrors.tuna.tsinghua.edu.cn/ubuntu）
ARG APT_MIRROR=http://mirrors.aliyun.com/ubuntu
ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
RUN sed -i "s|http://archive.ubuntu.com/ubuntu|${APT_MIRROR}|g; s|http://security.ubuntu.com/ubuntu|${APT_MIRROR}|g" /etc/apt/sources.list.d/ubuntu.sources
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      postgresql postgresql-contrib redis-server ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=backend /out/krono-job /app/krono-job
COPY configs /app/configs
COPY build/entrypoint.sh /app/entrypoint.sh
COPY build/init-db.sql /app/init-db.sql
RUN chmod +x /app/entrypoint.sh /app/krono-job

# ---- 平台运行环境变量（均可用 -e 覆盖；PG/Redis 均在本容器内）----
ENV KRONO_SERVER_PORT=10010
# Redis（内置，无密码，监听 0.0.0.0 以便同网络 asynqmon 可连）
ENV KRONO_REDIS_ADDR=127.0.0.1:6379
ENV KRONO_REDIS_PASSWORD=""
ENV KRONO_REDIS_DB=0
# PostgreSQL（内置，库/角色在 entrypoint 中幂等创建）
ENV KRONO_POSTGRES_HOST=127.0.0.1
ENV KRONO_POSTGRES_PORT=5432
ENV KRONO_POSTGRES_USER=krono
ENV KRONO_POSTGRES_PASSWORD=kronopass
ENV KRONO_POSTGRES_DBNAME=krono_job
ENV KRONO_POSTGRES_SCHEMA=public
ENV KRONO_POSTGRES_SSLMODE=disable
# 首个管理员（表空时自动插入）
ENV KRONO_BOOTSTRAP_ADMIN_USER=admin
ENV KRONO_BOOTSTRAP_ADMIN_PASS=admin123
# 生产环境务必通过 -e KRONO_JWT_SECRET=... 覆盖（默认仅为本地测试值）
ENV KRONO_JWT_SECRET=change-me-in-prod

EXPOSE 10010 5432 6379
ENTRYPOINT ["/app/entrypoint.sh"]
