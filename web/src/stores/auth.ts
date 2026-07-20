import { defineStore } from 'pinia'
import { ref } from 'vue'

import { login as apiLogin } from '@/api'
import type { UserInfo } from '@/api/types'

// 鉴权状态：token 持久化到 localStorage，刷新页面后自动恢复登录态。
export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('krono_token') ?? '')
  const user = ref<UserInfo | null>(null)
  const isAuthenticated = ref<boolean>(token.value !== '')

  function setToken(t: string) {
    token.value = t
    localStorage.setItem('krono_token', t)
    isAuthenticated.value = true
  }

  async function login(username: string, password: string) {
    const res = await apiLogin({ username, password })
    setToken(res.token)
    user.value = res.user
    return res
  }

  function logout() {
    token.value = ''
    user.value = null
    isAuthenticated.value = false
    localStorage.removeItem('krono_token')
  }

  return { token, user, isAuthenticated, login, logout }
})
