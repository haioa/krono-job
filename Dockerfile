FROM golang:1.26.5-alpine AS builder

LABEL stage=gobuilder

ENV CGO_ENABLED=0 \
    GOPROXY=https://goproxy.cn,direct \
    GO111MODULE=on

RUN apk update --no-cache && apk add --no-cache tzdata

WORKDIR /build

ADD go.mod .
ADD go.sum .
RUN go mod download

# 复制全部源码（含宿主机外部构建好的 web/dist，供 web/embed.go 的 //go:embed dist 使用）
COPY . .

RUN go build -ldflags="-s -w" -o /app/krono-job ./cmd/server


# 运行阶段改用 alpine（而非 scratch），以便用 entrypoint 在首次启动时把默认配置
# 复制到挂载的命名卷，实现配置持久化（详见 docker-entrypoint.sh）。
FROM alpine:3.20

# 证书与时区
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai

# 默认配置烤到 /app/configs.default（不被卷覆盖）；运行时由 entrypoint 复制到 /app/configs
COPY ./configs /app/configs.default
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

ENV TZ=Asia/Shanghai

# 连接 app-net 内的基础服务（均可通过 docker run -e 覆盖）
# 服务名与凭据对齐 admin-grpc 的 admin-tx.yaml（redis / postgres 为共享实例）。
# KRONO_* 命名见 configs/config.yaml 顶部注释；未设置的项回退到配置文件默认值。
# 注意：redis 有密码、postgres 用户为 admin，必须与共享实例一致，否则连不上。
# JWT 密钥与启动管理员密码为随机生成；生产建议用 docker run -e 覆盖，避免固化进镜像。
ENV KRONO_REDIS_ADDR=redis:6379 \
    KRONO_REDIS_PASSWORD=123456 \
    KRONO_REDIS_DB=15 \
    KRONO_POSTGRES_HOST=postgres \
    KRONO_POSTGRES_PORT=5432 \
    KRONO_POSTGRES_USER=admin \
    KRONO_POSTGRES_PASSWORD=passwd \
    KRONO_POSTGRES_DBNAME=krono_job \
    KRONO_POSTGRES_SCHEMA=public \
    KRONO_POSTGRES_SSLMODE=disable \
    KRONO_POSTGRES_MAX_CONNS=10 \
    KRONO_JWT_SECRET=xxxxxxxxxxxxxxxxxxxxxxxx \
    KRONO_BOOTSTRAP_ADMIN_USER=admin \
    KRONO_BOOTSTRAP_ADMIN_PASS=123456

WORKDIR /app
COPY --from=builder /app/krono-job /app/krono-job

EXPOSE 10010

# 通过 entrypoint 初始化配置后再启动；命令参数保持原来的 -f configs/config.yaml
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["-f", "configs/config.yaml"]
