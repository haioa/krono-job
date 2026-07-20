// 轻量 API 客户端：基于原生 fetch，自动注入 Bearer Token，统一处理 401/错误。
// 基址默认使用相对路径 /api（dev 经 Vite 代理、生产随二进制同源）；
// 可用环境变量 VITE_API_BASE 覆盖（如 http://localhost:10010/api）。

const BASE: string = import.meta.env.VITE_API_BASE ?? '/api'

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
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
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
