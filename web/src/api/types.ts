// 后端接口数据模型（与 internal/model、handler 保持一致）。

export interface LoginRequest {
  username: string
  password: string
}

export interface UserInfo {
  id: string
  username: string
  nickname: string
}

export interface LoginResponse {
  token: string
  expires_at: string
  user: UserInfo
}

export interface ChangePasswordRequest {
  new_password: string
}

export interface JobView {
  name: string
  task_type: string
  cron: string
  protocol: string
  enabled: boolean
  paused: boolean
  endpoint: string
  endpoints: string[]
  method: string
  retry: number
  timeout: string
}

export interface JobsResponse {
  list: JobView[]
  total: number
}

// ---------- 可视化编辑：jobs.yaml 完整定义（含 comment 备注） ----------
// 与后端 internal/handler/jobs.go 的 JobDefView 保持一致。
export interface JobDef {
  name: string
  task_type: string
  cron: string
  protocol: string // 'http' | 'grpc'
  enabled: boolean | null // 缺省视为 true；显式 false=停注
  timeout: string
  retry: number
  endpoint: string
  endpoints: string[]
  lb_strategy: string
  method: string
  headers: Record<string, string>
  payload: Record<string, any>
  grpc_service: string
  grpc_method: string
  request_type: string
  metadata: Record<string, string>
  comment: string // 人工维护备注，写回为 YAML 注释
}

export interface JobDefListResponse {
  list: JobDef[]
  total: number
}

export interface SimpleResponse {
  task_type?: string
  message: string
}

// cron 表达式预览（最近几次执行时间）
export interface CronPreviewRequest {
  cron: string
  count?: number
}
export interface CronPreviewResponse {
  cron: string
  timezone: string
  list: string[]
}

export type ExecStatus = 'success' | 'failed' | 'skipped'

export interface JobExecLog {
  id: string
  task_type: string
  trigger_type: string
  protocol: string
  target_endpoint: string
  status: ExecStatus
  response_body: string
  error_msg: string | null
  retry_count: number
  start_at: string
  end_at: string | null
  execution_duration_ms: number | null
  created_at: string
}

export interface PageResult {
  total: number
  page: number
  page_size: number
  list: JobExecLog[]
}

export interface LogsQuery {
  page?: number
  page_size?: number
  task_type?: string
  status?: string
  protocol?: string
  start?: string
  end?: string
  sort?: string
  order?: string
}

// GET /api/logs/task-types 返回的可用于过滤的下拉项（显示名称，提交 task_type）。
export interface TaskTypeOption {
  task_type: string
  name: string
}

export interface TaskTypeList {
  list: TaskTypeOption[]
}

// 批量删除日志返回结果：deleted 为实际删除条数。
export interface DeleteLogsResponse {
  deleted: number
  message: string
}

// ---------- 统计看板 ----------
// 执行结果总览：成功/失败/跳过总数。
export interface StatsOverview {
  total: number
  success: number
  failed: number
  skipped: number
}

// 单日执行统计行。
export interface DailyStat {
  day: string // YYYY-MM-DD
  total: number
  success: number
  failed: number
  skipped: number
}

// 调用排行榜条目。
export interface TaskRank {
  task_type: string
  name: string
  total: number
  success: number
  failed: number
  skipped: number
}

// 带时间范围的统计查询参数（start/end 为 YYYY-MM-DD 或 RFC3339）。
export interface StatsQuery {
  start?: string
  end?: string
  limit?: number
}

export interface DailyStatList {
  list: DailyStat[]
}

export interface TaskRankList {
  list: TaskRank[]
}

// ---------- 用户管理 ----------
export type UserStatus = 'active' | 'disabled'

// 返回给前端的安全用户视图（不含密码哈希）。
export interface UserView {
  id: string
  username: string
  nickname: string
  status: UserStatus
  created_at: string
  updated_at: string
}

export interface UserListResponse {
  list: UserView[]
}

export interface CreateUserRequest {
  username: string
  nickname: string
  password: string
  status?: UserStatus
}

export interface UpdateUserRequest {
  nickname?: string
  status?: UserStatus
  password?: string
}

// ---------- 分布式集群：被调用方节点存活状态 ----------
// 单个下游被调用方节点的健康视图（探测结果来自 Redis 集中共享）。
export interface ClusterNode {
  endpoint: string
  protocol: string
  alive: boolean
  error?: string
  checked: number // 最近探测时间 UnixNano
  by?: string // 探测来源节点 ID
}

// GET /api/cluster/nodes 返回。
export interface ClusterNodesResponse {
  distributed: boolean
  nodes: ClusterNode[]
}

// GET /api/cluster/status 返回。
export interface ClusterStatusResponse {
  distributed: boolean
  node_id: string
  role: 'leader' | 'follower' | 'standalone'
  is_leader: boolean
}

