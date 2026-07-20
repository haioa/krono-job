-- krono-job 初始 Schema (000001)
-- 仅 2 张表：sys_user（管理员认证） + job_exec_log（调度执行日志）
-- 显式限定模式为 public（PostgreSQL 为「库-模式-表」三级结构）。

SET search_path TO public;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 4.1 管理员用户表
CREATE TABLE IF NOT EXISTS public.sys_user (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    username text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    nickname text,
    status text NOT NULL DEFAULT 'active',  -- 'active' / 'disabled'
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sys_user_username ON public.sys_user(username);

-- 4.2 调度执行日志表
CREATE TABLE IF NOT EXISTS public.job_exec_log (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_type text NOT NULL,
    trigger_type text NOT NULL,        -- 'cron' / 'manual'
    protocol text NOT NULL,            -- 'http' / 'grpc'
    target_endpoint text NOT NULL,
    status text NOT NULL,              -- 'success' / 'failed' / 'skipped'
    response_body text,                -- 超长截断至 8KB（见 model.MaxResponseBodyBytes）
    error_msg text,
    retry_count integer NOT NULL DEFAULT 0,  -- 实际重试次数（决策 2-A）
    start_at timestamptz NOT NULL,
    end_at timestamptz,
    execution_duration_ms bigint,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_job_exec_log_task_type ON public.job_exec_log(task_type);
CREATE INDEX IF NOT EXISTS idx_job_exec_log_created_at ON public.job_exec_log(created_at DESC);
