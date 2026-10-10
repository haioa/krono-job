#!/bin/bash
#
# 本地自动化构建并推送到云端仓库 (krono-job)
# 用法:
#   ./build_push.sh            # 默认版本 v1.0
#   ./build_push.sh v1.1       # 指定版本标签
#   VERSION=v1.2 ./build_push.sh
#
# 前置条件:
#   1. 已开启代理（首次构建需拉取 golang:1.26.5-alpine 与 goproxy.cn 依赖）
#   2. Docker 已配置可访问 192.168.31.10:5000（已在 insecure-registries）
#   3. 宿主机已安装 node/pnpm（脚本会在 docker build 前外部构建 web/dist）
#
# 说明:
#   基础服务地址通过 Dockerfile 内的 ENV 注入（KRONO_REDIS_ADDR / KRONO_POSTGRES_HOST / KRONO_POSTGRES_PORT），
#   容器在 app-net 内用服务名互访；如需覆盖，运行容器时用 -e 传入即可。

set -euo pipefail

# ---------- 配置区（按需修改） ----------
REGISTRY="192.168.31.10:5000"
IMAGE_NAME="krono-job"
DEFAULT_VERSION="v1.0"
PORT="10010"
SVC="krono-job"
BUILD_CONTEXT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# krono-job 仅依赖公开模块，无需 git-credentials；此处保留变量以便将来扩展
CREDENTIALS="${BUILD_CONTEXT}/git-credentials"

echo "=================================================="
echo " 构建并推送镜像"
echo " 镜像: ${REGISTRY}/${IMAGE_NAME}:${VERSION:-$DEFAULT_VERSION}"
echo " 上下文: ${BUILD_CONTEXT}"
echo "=================================================="

# ---------- 参数解析 ----------
VERSION="${1:-${VERSION:-$DEFAULT_VERSION}}"
FULL_TAG="${REGISTRY}/${IMAGE_NAME}:${VERSION}"
LOCAL_TAG="${IMAGE_NAME}:local"

# ---------- 构建前清理 BuildKit 缓存，避免多次构建后磁盘写满（仅清缓存，绝不删除任何镜像） ----------
echo "🧹 清理 BuildKit 构建缓存（仅缓存，不影响镜像）..."
docker builder prune -a -f >/dev/null 2>&1 || true

# ---------- 前端 web/dist（需在 Docker 外构建，镜像仅复制该产物；不进 Docker 构建阶段） ----------
echo "🎨 检查前端 web/dist ..."
if [ ! -f "${BUILD_CONTEXT}/web/dist/index.html" ]; then
  echo "   web/dist 不存在，尝试在宿主机外部构建 ..."
  if [ -d "${BUILD_CONTEXT}/web" ]; then
    if command -v pnpm >/dev/null 2>&1; then
      ( cd "${BUILD_CONTEXT}/web" && pnpm install && pnpm run build-only ) || true
    elif command -v npm >/dev/null 2>&1; then
      ( cd "${BUILD_CONTEXT}/web" && npm install && npm run build-only ) || true
    fi
  fi
fi
if [ ! -f "${BUILD_CONTEXT}/web/dist/index.html" ]; then
  echo "❌ web/dist/index.html 不存在，无法打包前端。请先在 Docker 外构建前端："
  echo "   cd web && pnpm install && pnpm run build-only"
  echo "   （本镜像不再在 Docker 内构建前端，仅复制已构建的 dist）"
  exit 1
fi
echo "   ✅ web/dist 就绪"

# ---------- 构建 ----------
echo "📦 开始构建 (DOCKER_BUILDKIT) ..."
DOCKER_BUILDKIT=1 docker build \
  -t "${LOCAL_TAG}" \
  "${BUILD_CONTEXT}"

echo "✅ 构建完成: ${LOCAL_TAG}"

# ---------- 打标签 ----------
docker tag "${LOCAL_TAG}" "${FULL_TAG}"
echo "🏷️  已打标签: ${FULL_TAG}"

# ---------- 推送 ----------
echo "🚀 推送中 ..."
docker push "${FULL_TAG}"

# ---------- 结果 ----------
DIGEST=$(docker inspect --format='{{index .RepoDigests 0}}' "${FULL_TAG}" 2>/dev/null || true)
echo "=================================================="
echo "✅ 推送成功"
echo "   镜像: ${FULL_TAG}"
[[ -n "${DIGEST}" ]] && echo "   Digest: ${DIGEST}"
echo "=================================================="
echo "🖥️  服务器端更新步骤（在服务器上依次执行）:"
echo "--------------------------------------------------"
echo "# 0. 物理机配置目录（映射目录的物理机路径为 /opt/krono-job）"
echo "#    方式一（零配置）：目录为空时，首次启动 entrypoint 会自动把镜像内置默认配置复制到该目录；"
echo "#    方式二（用自定义配置）：先把仓库 configs/ 的内容传到服务器："
echo "#      scp -r configs/. root@<服务器IP>:/opt/krono-job/   # 之后目录里就有 config.yaml / jobs.yaml"
echo "sudo mkdir -p /opt/krono-job"
echo ""
echo "# 1. 拉取最新镜像（刷新本地旧缓存，否则会跑旧版本）"
echo "sudo docker pull ${FULL_TAG}"
echo ""
echo "# 2. 停止并删除旧容器"
echo "sudo docker rm -f ${IMAGE_NAME}"
echo ""
echo "# 3. 用新镜像重新启动，直接 -v 把物理机 /opt/krono-job 映射到容器 /app/configs"
echo "sudo docker run -d --name ${IMAGE_NAME} --restart=always --network app-net -p ${PORT}:${PORT} -v /opt/krono-job:/app/configs ${FULL_TAG}"
echo ""
echo "# 4. 查看日志确认启动正常（目录为空时会看到 📋 初始化配置 日志；jobs.yaml 内为 LAN IP，docker 下需改服务名/环境变量）"
echo "sudo docker logs -f ${IMAGE_NAME}"
echo ""
echo "# 5. 通过 Web UI 修改的任务会写入物理机 /opt/krono-job/jobs.yaml，容器重建也不会丢失。"
echo "    如需直接改 config.yaml，可执行：sudo docker exec -it ${IMAGE_NAME} sh   或   sudo vim /opt/krono-job/config.yaml"
echo "--------------------------------------------------"
echo "💡 提示: 若改了版本号（如 v1.1），上面所有命令中的 ${REGISTRY}/${IMAGE_NAME}:<版本> 需同步替换。"
echo "=================================================="
