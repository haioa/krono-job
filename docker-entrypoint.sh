#!/bin/sh
set -e

# 持久化配置：将镜像内置的默认配置（/app/configs.default）按需复制到 /app/configs。
# - 挂载命名卷且为空（首次部署）→ 完整复制默认配置；
# - 已存在文件（用户通过 Web UI 或 docker exec 改过）→ 仅补齐缺失文件，保留已有配置。
# 这样配置既能在宿主机无配置文件的情况下自动初始化，又能跨容器重建持久化。
mkdir -p /app/configs
for f in /app/configs.default/*; do
  [ -e "$f" ] || continue
  bf=$(basename "$f")
  if [ ! -e "/app/configs/$bf" ]; then
    cp -r "$f" "/app/configs/$bf"
    echo "📋 初始化配置: /app/configs/$bf"
  fi
done

exec ./krono-job "$@"
