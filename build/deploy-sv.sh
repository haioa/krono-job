#!/data/data/com.termux/files/usr/bin/sh
# ============================================================
# krono-job 一键部署脚本
#   编译后端(嵌入已有 web/dist) -> 落地配置 -> 生成 sv 服务 -> 启动
#   前端需提前在开发机构建并提交（web/dist 已在仓库中），本脚本只负责后端+部署。
#
# 前置条件：
#   - 仓库已 clone 到 $PROJECT_DIR
#   - web/dist 已存在（前端需提前 build 并 commit，详见脚本末尾说明）
#   - PostgreSQL 与 Redis 已就绪（本脚本不负责拉起它们）
#   - termux 环境：$PREFIX 已定义，sv / proot 可用
#
# 说明：
#   krono-job 的 server 通过 -f 指定配置文件（默认 configs/config.yaml），
#   与本项目其它服务(gen-sv.sh 的 -f ./etc/<config>) 约定一致；
#   它不使用 etcd，因此本脚本生成的 run 脚本不含 etcd 等待逻辑。
# ============================================================

# ============ 配置 ============
PROJECT_DIR="$HOME/backend/krono-job"   # 仓库根目录（在此构建）
DEPLOY_DIR="$HOME/backend"              # 运行根目录（二进制/配置落在此）
SV_DIR="$PREFIX/var/service"
SVC_NAME="krono-job"
APP_NAME="krono_job_linux_arm64"        # 产出二进制名（与 gen-sv.sh 命名一致）
CONFIG_NAME="krono_job.yaml"            # 配置文件名（etc/ 下）
GOPRIVATE_DEP="gitee.com/haioa/*"        # 私有依赖域名（go mod vendor / 下载时需要直连 git）

# 注入环境变量
export PATH="$HOME/go/bin:$PATH"
export GOTOOLCHAIN=local
export GOPRIVATE="$GOPRIVATE_DEP"
export GOOS=linux
export GOARCH=arm64
export CGO_ENABLED=0

echo "🕒 部署开始: $(date)"
echo "📂 仓库目录: $PROJECT_DIR"
echo "📂 运行目录: $DEPLOY_DIR"

# 1. 拉取最新代码
cd "$PROJECT_DIR" || { echo "❌ 找不到 $PROJECT_DIR"; exit 1; }
echo "⬇️ git pull ..."
git pull || { echo "❌ git pull 失败"; exit 1; }

# 2. 校验前端产物（前端需提前在开发机 build 并提交，本脚本不构建前端）
#    go:embed 需要 web/dist 存在，否则编译出的二进制不含页面。
#    不仅要有 index.html，还要 assets/ 下有真实 js/css（防止「占位 dist」蒙混过关）。
if [ ! -f web/dist/index.html ] || [ -z "$(ls -A web/dist/assets 2>/dev/null)" ]; then
  echo "❌ web/dist 缺失或不完整（无 index.html 或 assets 为空），停止部署！"
  echo "   前端需提前构建：在开发机执行 pnpm install && pnpm run build-only，"
  echo "   然后将 web/dist 提交并 git push，服务器 git pull 后即可继续。"
  echo "   若服务器上 dist 被误清空：cd $PROJECT_DIR && git restore web/dist"
  exit 1
fi
echo "✅ 前端产物 web/dist 已就绪（$(ls web/dist/assets | wc -l) 个资源文件）"

# 3. 同步 vendor（vendor/ 被 .gitignore 忽略，不随 git 下发；每次按最新 go.mod 重新生成，避免 inconsistent vendoring）
echo "📦 同步 vendor (go mod vendor)..."
go mod vendor
if [ $? -ne 0 ]; then
  echo "❌ go mod vendor 失败，请确认服务器可访问模块代理，且已配置私有依赖 git 凭证，例如："
  echo "   export GOPRIVATE=gitee.com/haioa/*"
  exit 1
fi

# 4. 编译后端（go:embed 会把 web/dist 打进二进制）
echo "🔨 编译 $APP_NAME ..."
go build -mod=vendor -ldflags="-s -w" -o "$DEPLOY_DIR/$APP_NAME" ./cmd/server
if [ $? -ne 0 ]; then
  echo "❌ 编译失败，停止部署！"
  exit 1
fi
echo "✅ 编译成功 -> $DEPLOY_DIR/$APP_NAME"

# 4.5 校验二进制确实内嵌了前端（grep 内嵌的 index.html 标题，避免部署「无前端」的残缺包）
if grep -aq "Krono-Job 控制台" "$DEPLOY_DIR/$APP_NAME"; then
  echo "✅ 二进制已内嵌前端页面（web/dist 已打入）"
else
  echo "❌ 二进制未包含前端内容！编译时 web/dist 为空/占位，已删除该残缺二进制。"
  echo "   请确认 web/dist/assets 下有真实 js/css 后，重新运行本脚本。"
  rm -f "$DEPLOY_DIR/$APP_NAME"
  exit 1
fi

# 5. 落地配置文件（仅首次，避免覆盖线上已修改的配置）
mkdir -p "$DEPLOY_DIR/etc" "$DEPLOY_DIR/configs"
if [ ! -f "$DEPLOY_DIR/etc/$CONFIG_NAME" ]; then
  echo "📋 首次生成配置: $DEPLOY_DIR/etc/$CONFIG_NAME"
  cp "configs/config.yaml" "$DEPLOY_DIR/etc/$CONFIG_NAME"
else
  echo "ℹ️  已存在 $DEPLOY_DIR/etc/$CONFIG_NAME，保留不覆盖（如需更新请手动修改）"
fi
if [ ! -f "$DEPLOY_DIR/configs/jobs.yaml" ]; then
  echo "📋 首次生成调度配置: $DEPLOY_DIR/configs/jobs.yaml"
  cp "configs/jobs.yaml" "$DEPLOY_DIR/configs/jobs.yaml"
else
  echo "ℹ️  已存在 $DEPLOY_DIR/configs/jobs.yaml，保留不覆盖"
fi

# 6. 生成 sv 服务（无 etcd 依赖，krono-job 用 PG + Redis）
SVC_PATH="$SV_DIR/$SVC_NAME"
echo "⚙️  生成 sv 服务: $SVC_NAME"
mkdir -p "$SVC_PATH/log"
cat <<EOF > "$SVC_PATH/run"
#!/data/data/com.termux/files/usr/bin/sh
exec 2>&1

export GOMAXPROCS=4
export PATH=\$HOME/go/bin:\$PATH
# 如需用环境变量覆盖配置（非空即覆盖 yaml 同名值，KRONO_<SECTION>_<KEY>），可在此追加，例如：
# export KRONO_JWT_SECRET="改成随机长串"
# export KRONO_POSTGRES_PASSWORD="你的库密码"
# export KRONO_REDIS_ADDR="127.0.0.1:6379"

cd "$DEPLOY_DIR" || exit 1

exec proot -b \$PREFIX/etc/resolv.conf:/etc/resolv.conf \\
     ./$APP_NAME -f ./etc/$CONFIG_NAME
EOF

cat <<EOF > "$SVC_PATH/log/run"
#!/data/data/com.termux/files/usr/bin/sh
mkdir -p "$PREFIX/var/log/$SVC_NAME"
exec svlogd -tt "$PREFIX/var/log/$SVC_NAME"
EOF

chmod +x "$SVC_PATH/run" "$SVC_PATH/log/run"

# 7. 重启服务（先停止旧进程以释放端口并加载新二进制，再启动）
echo "🚀 重启 $SVC_NAME ..."
sv stop "$SVC_NAME" 2>/dev/null
sleep 2
sv start "$SVC_NAME" 2>/dev/null || sv up "$SVC_NAME"
sleep 2
sv status "$SVC_NAME"

echo "🎉 krono-job 部署完成！"
echo "💡 查看日志: tail -f $PREFIX/var/log/$SVC_NAME/current"
