-- krono-job 内置 PostgreSQL 初始化（幂等）
-- 以 postgres 超级用户执行：创建专用角色、数据库，并预置 uuid-ossp 扩展，
-- 使平台启动时自动迁移（CREATE EXTENSION IF NOT EXISTS）成为无操作。

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'krono') THEN
    CREATE ROLE krono LOGIN PASSWORD 'kronopass';
  END IF;
END
$$;

SELECT 'CREATE DATABASE krono_job OWNER krono'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'krono_job')\gexec

\c krono_job
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
