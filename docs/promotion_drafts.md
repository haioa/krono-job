# krono-job 推广文案草稿（多风格）

> 仓库地址（双源）：
> - GitHub：https://github.com/haioa/krono-job
> - Gitee：https://gitee.com/haioa/krono-job
>
> 截图目录：`docs/images/`（login / dashbord / job / log / user）
> 本文档为临时草稿，发布前请按平台规范微调并替换截图为图床链接。

---

## 风格一：技术教程风（适合 CSDN / 掘金 / 思否）

标题：**krono-job：一个 Docker 镜像跑起来的分布式定时任务调度平台（Go + Vue3 开源）**

做后端同学应该都遇到过：业务里一堆定时任务散落在各个微服务，crontab 难管理、失败无日志、重试靠手写、暂停恢复没有入口……自己造一套调度平台又怕重。

最近我把内部用的调度平台整理开源了 —— **krono-job**，主打「零侵入 + 单镜像交付」。

### 它解决了什么

- **下游零侵入**：业务服务**被动接收触发**（HTTP Webhook 或 gRPC），不需要引入任何 SDK、不需要改代码。
- **一条命令跑全套**：`docker run` 一个镜像，内置 Web/API + 调度内核 + Worker + PostgreSQL + Redis，无需外部依赖。
- **配置即任务**：任务在 `jobs.yaml` 里声明式定义，支持 fsnotify 热重载，改完文件不用重启。
- **数据库只有 2 张表**：管理员表 + 执行日志表，暂停态存 Redis，极简。

### 架构一览

```
【Vue3 管理后台】 ──JWT──► 【Gin 网关】 ──► 【Redis 调度总线】
                                   │
                            【Asynq Scheduler】──cron 推送──►【Asynq Worker】
                                                                 │
                                               HTTP Webhook / gRPC 调用
                                                                 │
                                                      下游业务微服务
```

运维监控（重试、死信、队列调优）交给独立的 **Asynqmon** UI，与平台解耦。

### 30 秒跑起来

```bash
docker run -d --name krono-job \
  -p 10010:10010 -p 5432:5432 -p 6379:6379 \
  -e KRONO_JWT_SECRET=请改成随机长字符串 \
  krono-job:latest
```

打开 `http://localhost:10010` 就能看到管理后台。

### 加一个 HTTP 任务，只改 yaml

```yaml
jobs:
  - name: 订单超时自动取消
    task_type: order_timeout_cancel
    cron: "0 */5 * * *"      # 每 5 分钟
    protocol: http
    endpoint: "http://order-svc:8080/api/timeout/cancel"
    method: POST
    payload:
      scene: "auto_expire"
```

gRPC 任务也只需在 yaml 里声明 `endpoint`、`grpc_service/method`、`request_type`、`payload`，**参数全在配置里，不写死在代码**，新增下游服务最多加一行 `import _`。

### 管理后台长啥样

登录、仪表盘（执行趋势 + 任务排行榜）、任务列表（暂停/恢复/立即执行）、执行日志（分页/过滤/排序/详情）、用户管理，一套齐全。

![仪表盘](docs/images/dashbord.png)

### 技术栈

Go 1.26 · Gin · Asynq · PostgreSQL 16 · Redis 7 · Vue3（go:embed 打包进二进制）· JWT · bcrypt

### 开源地址

- GitHub：https://github.com/haioa/krono-job
- Gitee：https://gitee.com/haioa/krono-job

欢迎 Star / Issue / PR，也欢迎在评论区聊聊你们团队的定时任务是怎么管的。

---

## 风格二：痛点故事风（适合 知乎文章 / 公众号）

标题：**我们团队的定时任务，终于不用再「散养」了**

先说个扎心的场景：

公司里定时任务一开始都写在 crontab 里，或者各自服务里 `@Scheduled`。时间一长就变成了——

- 谁也不知道线上到底跑了多少个定时任务；
- 某个任务挂了，第二天才被发现，因为**根本没有执行日志**；
- 想临时暂停一个任务？得登机器改代码或注释 cron；
- 下游换了地址，得发版；
- 重试、死信、并发控制，全靠硬编码 if-else。

我们调研过市面上的方案：XXL-JOB 要部署调度中心 + 执行器，业务得接入 SDK；一些云调度要绑定云厂商。我们想要的是：**业务服务别动，最好一个镜像就能把调度 + 后台 + 存储全跑了。**

于是有了 **krono-job**。

### 核心思路：下游「被动接收」

krono-job 不要求业务服务集成任何 SDK。它到点了，就通过 **HTTP Webhook** 或 **gRPC** 去调你的服务，就像有人定时帮你 curl 一下。你的服务只管暴露一个接口。

加一个定时任务？在 `jobs.yaml` 里写几行：

```yaml
jobs:
  - name: 库存每日同步
    cron: "0 2 * * *"
    protocol: grpc
    endpoint: "product-svc:9000"
    grpc_service: "mall.Product"
    grpc_method: "SyncStock"
```

改完文件，平台通过 fsnotify **热重载**，不用重启。

### 一个镜像，全家桶

最省心的是部署。一条 `docker run`，镜像里自带：

- Web 管理后台 + API（Gin + Vue3）
- 调度内核 + 工作进程（Asynq）
- PostgreSQL 16 + Redis 7

也就是说，你不需要先准备数据库、先部署 Redis、再部署调度中心……一个容器拉起来，访问 `http://localhost:10010` 就能用。

### 后台该有的都有

不是只有调度，配套的运营能力也齐了：

- **仪表盘**：执行总数 / 成功 / 失败 / 跳过概览，每日趋势折线图，任务调用排行榜；
- **任务管理**：暂停 / 恢复开关、一键「立即执行」、跳转到该任务日志；
- **执行日志**：按任务 / 状态 / 协议 / 时间过滤，可排序，点开看完整响应体和错误；
- **用户管理**：管理员增删改查，密码 bcrypt 存储。

### 为什么敢说「极简」

整个平台落库**只有 2 张表**——管理员表和执行日志表。任务的暂停态存在 Redis 的 Set 里，不引入额外表；任务定义本身只在 yaml 里，不落库。

### 适合谁

- 想统一管理散落各处的定时任务；
- 不想让业务服务耦合调度 SDK；
- 想要一个轻量、能私有化一键部署的调度后台；
- 技术栈偏 Go / 云原生。

项目已开源，欢迎来逛逛、提建议：

- GitHub：https://github.com/haioa/krono-job
- Gitee：https://gitee.com/haioa/krono-job

如果你的团队也在被定时任务折磨，希望能帮上忙。也欢迎在评论区说说你们的解法，一起交流。

---

## 风格三：极简「一行命令」风（适合 Hacker News / V2EX / GitHub README 引流）

标题：**krono-job: self-hosted cron scheduler with a web UI, one Docker image, zero SDK intrusion**

Tired of crontab sprawl and building your own scheduler? krono-job is a distributed job scheduler that:

- Runs **everything in one Docker image**: web UI + API (Gin) + scheduler/worker (Asynq) + PostgreSQL + Redis.
- Triggers downstream services via **HTTP webhook or gRPC** — your services need zero SDK, zero code changes.
- Defines jobs declaratively in `jobs.yaml` with **fsnotify hot-reload**, no restart needed.
- Ships a full admin UI: dashboard, job pause/resume + manual run, execution logs with filtering, user management.
- Uses only **2 PostgreSQL tables**; pause state lives in Redis.

```bash
docker run -d --name krono-job -p 10010:10010 \
  -e KRONO_JWT_SECRET=change-me \
  krono-job:latest
# open http://localhost:10010
```

Stack: Go 1.26 · Gin · Asynq · PostgreSQL 16 · Redis 7 · Vue3 (go:embed).

- GitHub: https://github.com/haioa/krono-job
- Gitee: https://gitee.com/haioa/krono-job

Feedback and PRs welcome.

---

## 风格四：横评对比风（适合「XX 工具推荐」「技术选型」类文章）

标题：**定时任务调度平台怎么选？XXL-JOB、Quartz、云调度 与 krono-job 对比**

很多团队在选型定时任务调度时会在几个方向里纠结，这里做个速览：

| 维度 | crontab / @Scheduled | XXL-JOB | 云厂商调度 | **krono-job** |
| --- | --- | --- | --- | --- |
| 业务侵入 | 低（但无统一管控） | 高（需接 SDK/执行器） | 中（绑定云） | **零侵入（被动 HTTP/gRPC）** |
| 管理后台 | 无 | 有 | 有 | **有（Vue3）** |
| 部署复杂度 | 低 | 中（中心+执行器） | 低（但绑定云） | **极低（单镜像）** |
| 任务定义 | 改机器 | 后台录入 | 控制台 | **yaml 声明 + 热重载** |
| 日志/重试/暂停 | 手写 | 内置 | 内置 | **内置** |
| 存储依赖 | 无 | DB + Redis | 云托管 | **PG + Redis（内置镜像）** |
| 私有化 | 完全 | 完全 | 否 | **完全** |

**krono-job 的定位**：当你不想让业务服务耦合调度 SDK，又想要一个能私有化、一键部署、带完整后台的轻量调度平台时，它是一个很省心的选择。下游只需暴露 HTTP 或 gRPC 接口，到点被平台主动调用即可。

开源地址：https://github.com/haioa/krono-job

---

## 风格五：问答体 / 面试科普风（适合知乎回答 / 技术社区）

问题：**如何设计一个低侵入的分布式定时任务调度系统？**

回答（以 krono-job 为例，讲设计取舍）：

1. **调度与执行分离**：用 Asynq（基于 Redis）做调度总线，Scheduler 按 cron 把任务推入队列，Worker 消费执行。这样调度内核和你的业务进程解耦。
2. **下游被动接收，零侵入**：Worker 不直接执行业务逻辑，而是按 `protocol` 发起 HTTP / gRPC 调用。业务服务只需暴露接口，不引入任何 SDK。加任务 = 改 yaml（gRPC 最多加一行 `import _`）。
3. **配置即任务**：任务定义在 `jobs.yaml`，用 fsnotify 监听热重载；YAML 解析失败保留旧配置，不影响运行。
4. **极简存储**：只 2 张表（管理员 + 执行日志）；暂停态放 Redis Set，Worker 分发前 `SISMEMBER` 拦截，命中则落库 `skipped`。
5. **管理后台打包进二进制**：Vue3 构建产物经 `go:embed` 打进 Go 二进制，单文件即可交付，Gin 同时托管前端和 `/api/*`。
6. **运维解耦**：重试、死信、队列监控交给独立的 Asynqmon UI，平台自身保持轻。

项目开源：https://github.com/haioa/krono-job，欢迎对照代码看实现。

---

## 风格六：爆款标题党风（适合头条号 / 百家号 / 引流）

标题备选：

1. **同事用 3 天造了个调度平台，我一条命令就跑起来了**
2. **别再手写 crontab 了！这个开源项目让定时任务管理爽到飞起**
3. **后端必看：一个 Docker 镜像，干掉你所有的定时任务痛点**
4. **零侵入、不接 SDK，这个 Go 调度平台凭什么让我放弃 XXL-JOB？**
5. **我把公司的定时任务平台开源了，star 已破千（附部署教程）**

正文钩子（示例，对应标题 2）：

> 你有没有算过，公司线上到底跑着多少个定时任务？
> 我敢打赌，没人能答得上来——它们藏在 crontab、藏在各个服务的 `@Scheduled`、藏在某个早已离职同事的脚本里。
> 今天分享一个我开源的轻量调度平台 krono-job：一个镜像拉起全套，下游零侵入，yaml 改改就能加任务，后台该有的日志、暂停、重试全都有。
> 下面直接上部署和效果……（接风格一的实操 + 截图）

---

## 风格七：短文案合集（朋友圈 / 微博 / 动态 / 评论区）

**A（微博/朋友圈）：**
折腾了阵子，把内部定时任务调度平台开源了 krono-job：一个 Docker 镜像跑起 Web 后台 + 调度 + Worker + PG + Redis，下游 HTTP/gRPC 被动触发、零侵入、只改 yaml 就能加任务。求 Star 求拍砖 👉 github.com/haioa/krono-job

**B（V2EX / 掘金动态）：**
分享个自己写的轻量定时任务平台 krono-job（Go+Vue3，单镜像交付，下游零 SDK 侵入，仅 2 张表）。适合不想被调度 SDK 绑定的团队。GitHub: github.com/haioa/krono-job

**C（技术群/评论区引流）：**
最近在用一个自托管的定时任务平台 krono-job，部署是真的省心（一条 docker run），业务侧零改造，有需要的可以看看：github.com/haioa/krono-job

**D（英文 tweet / HN 标题）：**
Show HN: krono-job – a self-hosted distributed cron scheduler in a single Docker image, triggers your services over HTTP/gRPC with zero SDK. github.com/haioa/krono-job

---

## 发布清单（Checklist）

- [ ] CSDN：风格一 + 配图（改用图床链接，勿用相对路径）
- [ ] 知乎：风格二（文章）或风格五（回答）
- [ ] 掘金/思否：风格一 或 风格四
- [ ] V2EX / HN：风格三
- [ ] 朋友圈/微博/动态：风格七
- [ ] 头条/百家号（如需）：风格六
- [ ] 发布前把 `docs/images/xxx.png` 换成图床绝对 URL
- [ ] 在 README 与文档里互相引流
