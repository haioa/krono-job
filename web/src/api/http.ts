// 轻量 API 客户端：基于原生 fetch，自动注入 Bearer Token，统一处理 401/错误。
// 基址默认跟随部署 base（vite base，例如 /krono-job/ -> /krono-job/api）；
// 独立根路径部署时 BASE_URL 为 '/'，即 '/api'，与旧行为一致；
// 可用环境变量 VITE_API_BASE 覆盖（如 http://localhost:10010/api）。

const BASE: string = import.meta.env.VITE_API_BASE ?? `${import.meta.env.BASE_URL}api`

// 登录页地址：同样跟随部署 base，避免子路径部署时跳到站点根路径导致 404。
const LOGIN_PATH = `${import.meta.env.BASE_URL}login`

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const token = localStorage.getItem('krono_token')
  const headers = new Headers(init?.headers)
  if (!(init?.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  let res: Response
  try {
    res = await fetch(BASE + path, { ...init, headers })
  } catch (e) {
    throw new ApiError('网络错误，无法连接服务', 0)
  }

  if (res.status === 401) {
    const isLogin = path.includes('/auth/login')
    if (!isLogin) {
      localStorage.removeItem('krono_token')
      if (window.location.pathname !== LOGIN_PATH) {
        window.location.href = LOGIN_PATH
      }
    }
    let msg = '未授权'
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      /* ignore */
    }
    throw new ApiError(msg, res.status)
  }

  if (res.status === 204) {
    return undefined as T
  }

  if (!res.ok) {
    let msg = `请求失败 (${res.status})`
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      /* ignore */
    }
    throw new ApiError(msg, res.status)
  }

  return (await res.json()) as T
}
