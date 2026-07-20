#!/data/data/com.termux/files/usr/bin/sh
# ============================================================
# krono-job 部署脚本（含前端）
#
# 架构说明：
#   前端 (web/) 经 `pnpm build-only` 产出 web/dist 静态资源，
#   后端通过 web/embed.go 的 go:embed 把 web/dist 打进单个二进制，
#   因此「先构建前端，再编译后端」是必须的先后顺序。
#
# 注意：
#   1. web/dist 当前已提交进 Git（.gitignore 仅忽略根 /dist/），
#      所以即使不现场构建前端，git pull 后也能拿到一份可用产物；
#      但现场构建可保证前端代码与后端同步、是最新状态。
#   2. 本脚本只复制二进制到运行目录，与旧脚本一致。
#      若你的 sv 运行目录没有 configs/，请自行管理配置文件
#      （生产配置不要被仓库覆盖）。
# ============================================================

# ============ 配置变量 ============
PROJECT_DIR="$HOME/backend/krono-job"        # 仓库根目录
WEB_DIR="$PROJECT_DIR/web"                   # 前端目录
TARGET_DIR="$HOME/backend"                   # 二进制落地目录
APP_NAME="krono_job_linux_arm64"             # 产出二进制名
SERVICE_NAME="krono-job"                     # sv 服务名
BUILD_FRONTEND="true"                        # 是否现场构建前端（需要 node + pnpm）

# 注入环境变量
export PATH="$HOME/go/bin:$PATH"
export GOTOOLCHAIN=local

# 仅编译 Go 二进制（会嵌入 web/dist），显式指定目标平台，关闭 CGO 生成静态二进制
export GOOS=linux
export GOARCH=arm64
export CGO_ENABLED=0

echo "🕒 开始部署时间: $(date)"
echo "📂 切换到项目目录: $PROJECT_DIR"
cd "$PROJECT_DIR" || { echo "❌ 找不到目录 $PROJECT_DIR"; exit 1; }

# 1. 拉取代码
echo "⬇️ 正在从 Git 拉取最新代码..."
git pull
if [ $? -ne 0 ]; then
    echo "❌ git pull 失败，停止部署！"
    exit 1
fi

# 2. 构建前端（可选）
if [ "$BUILD_FRONTEND" = "true" ]; then
    echo "🎨 检查前端构建环境 (node / pnpm)..."
    if ! command -v node >/dev/null 2>&1 || ! command -v pnpm >/dev/null 2>&1; then
        echo "⚠️  未找到 node 或 pnpm，跳过前端构建，将使用仓库内已提交的 web/dist"
    else
        echo "🔨 正在构建前端 ($WEB_DIR)..."
        cd "$WEB_DIR" || { echo "❌ 找不到 web 目录"; exit 1; }
        pnpm install
        if [ $? -ne 0 ]; then
            echo "❌ pnpm install 失败，停止部署！"
            exit 1
        fi
        pnpm run build-only
        if [ $? -ne 0 ]; then
            echo "❌ 前端构建失败，停止部署！"
            exit 1
        fi
        cd "$PROJECT_DIR"
    fi
else
    echo "ℹ️  已关闭前端构建，将使用仓库内已提交的 web/dist"
fi

# 校验前端产物存在（go:embed 需要它，否则二进制里没有页面）
if [ ! -f "$WEB_DIR/dist/index.html" ]; then
    echo "❌ $WEB_DIR/dist/index.html 不存在，前端产物缺失，停止部署！"
    echo "   请开启 BUILD_FRONTEND=true 并确保 node/pnpm 可用，或先将前端构建产物提交到仓库。"
    exit 1
fi

# 3. 编译后端（go:embed 会把 web/dist 打进二进制）
echo "🔨 正在编译 $APP_NAME ..."
go build -mod=vendor -ldflags="-s -w" -o "$APP_NAME" ./cmd/server
if [ $? -ne 0 ]; then
    echo "❌ 编译报错，停止部署！"
    exit 1
fi
echo "✅ 编译成功！"

# 4. 停止服务
echo "🛑 正在停止 $SERVICE_NAME 服务..."
sv stop "$SERVICE_NAME"
sleep 2

# 5. 覆盖文件
echo "📋 正在覆盖复制到 $TARGET_DIR ..."
cp -f "$APP_NAME" "$TARGET_DIR/"

# 6. 重启服务
echo "🚀 正在启动 $SERVICE_NAME 服务..."
sv start "$SERVICE_NAME"

echo "🎉 $SERVICE_NAME 部署流程全部完成！"
