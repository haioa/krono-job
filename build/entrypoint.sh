#!/bin/sh
set -e

# ============================================================
# krono-job all-in-one 容器启动脚本
# 在单个容器内拉起 Redis + PostgreSQL，幂等初始化库/角色/扩展，
# 然后启动平台后端（二进制内置自动迁移 + 管理员 bootstrap）。
# ============================================================

APP_BIN=/app/krono-job
INIT_SQL=/app/init-db.sql

PGDATA_DIR=/var/lib/postgresql/data
PG_RUN_DIR=/var/run/postgresql
PG_USER=postgres
DB_PORT="${KRONO_POSTGRES_PORT:-5432}"
DB_HOST="${KRONO_POSTGRES_HOST:-127.0.0.1}"

REDIS_PORT="${KRONO_REDIS_PORT:-6379}"

echo "[entrypoint] starting redis-server on :${REDIS_PORT}"
redis-server --port "${REDIS_PORT}" --bind 0.0.0.0 --protected-mode no --save "" --appendonly no --daemonize yes

echo "[entrypoint] initializing postgresql data dir"
if [ ! -d "${PGDATA_DIR}/base" ]; then
  mkdir -p "${PGDATA_DIR}" "${PG_RUN_DIR}"
  chown -R ${PG_USER}:${PG_USER} "${PGDATA_DIR}" "${PG_RUN_DIR}"
  su - ${PG_USER} -c "/usr/lib/postgresql/*/bin/initdb -D ${PGDATA_DIR} -A trust" >/dev/null 2>&1
fi

echo "[entrypoint] starting postgresql"
su - ${PG_USER} -c "/usr/lib/postgresql/*/bin/pg_ctl -D ${PGDATA_DIR} -o '-p ${DB_PORT} -k ${PG_RUN_DIR}' -l /tmp/pg.log start" || true
# 等待 PG 就绪
for i in $(seq 1 30); do
  if su - ${PG_USER} -c "/usr/lib/postgresql/*/bin/pg_isready -h ${DB_HOST} -p ${DB_PORT}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "[entrypoint] ensuring role/db/extension (idempotent via init-db.sql)"
su - ${PG_USER} -c "psql -p ${DB_PORT} -v ON_ERROR_STOP=0 -f ${INIT_SQL}"

echo "[entrypoint] launching krono-job platform"
exec "${APP_BIN}"
