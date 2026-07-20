-- 回滚 000001：删除两张表（按依赖反向顺序）
DROP TABLE IF EXISTS public.job_exec_log;
DROP TABLE IF EXISTS public.sys_user;
