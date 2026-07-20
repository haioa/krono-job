import { apiFetch } from './http'
import type { LoginRequest, LoginResponse, ChangePasswordRequest, JobsResponse, PageResult, LogsQuery, TaskTypeList, JobExecLog, StatsOverview, DailyStatList, TaskRankList, StatsQuery, DeleteLogsResponse, UserListResponse, UserView, CreateUserRequest, UpdateUserRequest } from './types'

// 认证
export function login(req: LoginRequest): Promise<LoginResponse> {
  return apiFetch<LoginResponse>('/auth/login', { method: 'POST', body: JSON.stringify(req) })
}

// 修改密码：无需原密码，直接设置新密码
export function changePassword(req: ChangePasswordRequest): Promise<{ message: string }> {
  return apiFetch('/auth/change-password', { method: 'POST', body: JSON.stringify(req) })
}

// 任务管理
export function getJobs(): Promise<JobsResponse> {
  return apiFetch<JobsResponse>('/jobs')
}

export function pauseJob(taskType: string): Promise<{ task_type: string; paused: boolean }> {
  return apiFetch(`/jobs/${encodeURIComponent(taskType)}/pause`, { method: 'POST' })
}

export function resumeJob(taskType: string): Promise<{ task_type: string; paused: boolean }> {
  return apiFetch(`/jobs/${encodeURIComponent(taskType)}/resume`, { method: 'POST' })
}

export function runJob(taskType: string): Promise<{ task_type: string; trigger_type: string; message: string }> {
  return apiFetch(`/jobs/${encodeURIComponent(taskType)}/run`, { method: 'POST' })
}

// 日志查询
export function getLogs(q: LogsQuery): Promise<PageResult> {
  const params = new URLSearchParams()
  Object.entries(q).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') params.set(k, String(v))
  })
  const qs = params.toString()
  return apiFetch<PageResult>(`/logs${qs ? `?${qs}` : ''}`)
}

// 日志过滤可选项：后端返回的去重 task_type 列表
export function getTaskTypes(): Promise<TaskTypeList> {
  return apiFetch<TaskTypeList>('/logs/task-types')
}

// 单条日志详情
export function getLogDetail(id: string): Promise<JobExecLog> {
  return apiFetch<JobExecLog>(`/logs/${encodeURIComponent(id)}`)
}

// 批量删除日志：传入待删除的日志 id 列表。
export function deleteLogs(ids: string[]): Promise<DeleteLogsResponse> {
  return apiFetch<DeleteLogsResponse>('/logs', {
    method: 'DELETE',
    body: JSON.stringify({ ids }),
  })
}

// 统计看板
function buildStatsParams(q: StatsQuery): string {
  const params = new URLSearchParams()
  Object.entries(q).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') params.set(k, String(v))
  })
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

// 执行结果总览：成功/失败/跳过总数（支持时间范围）。
export function getStatsOverview(q: StatsQuery = {}): Promise<StatsOverview> {
  return apiFetch<StatsOverview>(`/stats/overview${buildStatsParams(q)}`)
}

// 按天聚合的成功/失败/跳过趋势（支持时间范围）。
export function getDailyStats(q: StatsQuery = {}): Promise<DailyStatList> {
  return apiFetch<DailyStatList>(`/stats/daily${buildStatsParams(q)}`)
}

// 调用次数排行榜（支持时间范围与条数 limit）。
export function getTaskRanking(q: StatsQuery = {}): Promise<TaskRankList> {
  return apiFetch<TaskRankList>(`/stats/ranking${buildStatsParams(q)}`)
}

// 用户管理：查询用户列表（不含密码哈希）。
export function getUsers(): Promise<UserListResponse> {
  return apiFetch<UserListResponse>('/users')
}

// 新建用户。
export function createUser(req: CreateUserRequest): Promise<UserView> {
  return apiFetch<UserView>('/users', { method: 'POST', body: JSON.stringify(req) })
}

// 更新用户（昵称、状态、可选密码）。
export function updateUser(id: string, req: UpdateUserRequest): Promise<UserView> {
  return apiFetch<UserView>(`/users/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(req),
  })
}

// 删除用户。
export function deleteUser(id: string): Promise<{ deleted: number; message: string }> {
  return apiFetch(`/users/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
