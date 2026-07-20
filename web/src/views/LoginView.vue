<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const username = ref('')
const password = ref('')
const showPwd = ref(false)
const error = ref('')
const loading = ref(false)

async function onSubmit() {
  error.value = ''
  if (!username.value || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    const redirect = (route.query.redirect as string) || '/stats'
    router.replace(redirect)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '登录失败，请重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login">
    <!-- 左侧品牌区 -->
    <aside class="hero">
      <div class="hero-top">
        <span class="logo">
          <svg viewBox="0 0 32 32" width="26" height="26" aria-hidden="true">
            <circle cx="16" cy="16" r="10" fill="none" stroke="#fff" stroke-width="2.4" />
            <path d="M16 10v6l4 3" stroke="#fff" stroke-width="2.4" stroke-linecap="round" fill="none" />
          </svg>
        </span>
        <span class="hero-brand">Krono<b>Job</b></span>
      </div>

      <div class="hero-copy">
        <h1>分布式定时任务<br />调度平台</h1>
        <p>统一调度 · 实时管控 · 全链路执行日志</p>
      </div>

      <ul class="features">
        <li>
          <svg viewBox="0 0 20 20" width="16" height="16"><path d="M4 10l4 4 8-9" fill="none" stroke="#fff" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/></svg>
          Cron 表达式驱动的多协议任务调度
        </li>
        <li>
          <svg viewBox="0 0 20 20" width="16" height="16"><path d="M4 10l4 4 8-9" fill="none" stroke="#fff" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/></svg>
          Redis 实时暂停 / 恢复，秒级生效
        </li>
        <li>
          <svg viewBox="0 0 20 20" width="16" height="16"><path d="M4 10l4 4 8-9" fill="none" stroke="#fff" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/></svg>
          执行结果、错误与耗时全程可追溯
        </li>
      </ul>
    </aside>

    <!-- 右侧表单区 -->
    <main class="form-side">
      <form class="form-card" @submit.prevent="onSubmit">
        <h2>欢迎回来</h2>
        <p class="form-sub">登录以管理你的调度任务</p>

        <div v-if="error" class="alert error">
          <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
            <circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2" />
            <path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
          <span>{{ error }}</span>
        </div>

        <label class="lbl">用户名</label>
        <div class="input-icon">
          <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
            <circle cx="10" cy="7" r="3.4" fill="none" stroke="currentColor" stroke-width="1.6" />
            <path d="M4 16c0-3 2.7-5 6-5s6 2 6 5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
          </svg>
          <input v-model="username" class="field" type="text" autocomplete="username" placeholder="请输入用户名" />
        </div>

        <label class="lbl" style="margin-top: 14px">密码</label>
        <div class="input-icon">
          <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
            <rect x="4" y="9" width="12" height="8" rx="2" fill="none" stroke="currentColor" stroke-width="1.6" />
            <path d="M7 9V6.5a3 3 0 0 1 6 0V9" fill="none" stroke="currentColor" stroke-width="1.6" />
          </svg>
          <input v-model="password" class="field" :type="showPwd ? 'text' : 'password'" autocomplete="current-password" placeholder="请输入密码" />
          <button type="button" class="toggle" @click="showPwd = !showPwd" :aria-label="showPwd ? '隐藏密码' : '显示密码'">
            <svg v-if="!showPwd" viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
              <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" fill="none" stroke="currentColor" stroke-width="1.8" />
              <circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" stroke-width="1.8" />
            </svg>
            <svg v-else viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
              <path d="M3 3l18 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
              <path d="M10.6 6.2A9.7 9.7 0 0 1 12 6c6.5 0 10 6 10 6a16.7 16.7 0 0 1-3.3 3.9M6.2 7.9A16.6 16.6 0 0 0 2 12s3.5 6 10 6a9.6 9.6 0 0 0 4.2-.9" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
              <path d="M9.5 9.7A3 3 0 0 0 12 15a3 3 0 0 0 2.3-1.1" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
            </svg>
          </button>
        </div>

        <button class="btn btn-primary btn-block" style="margin-top: 22px; height: 42px" type="submit" :disabled="loading">
          <span v-if="loading" class="spinner"></span>
          <span>{{ loading ? '登录中…' : '登录' }}</span>
        </button>
      </form>
    </main>
  </div>
</template>

<style scoped>
.login {
  height: 100%;
  display: grid;
  grid-template-columns: 1.05fr 1fr;
}

/* 左侧品牌 */
.hero {
  position: relative;
  background: var(--sidebar);
  color: #fff;
  padding: 44px 48px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.hero::after {
  content: '';
  position: absolute;
  right: -120px;
  bottom: -120px;
  width: 320px;
  height: 320px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.06);
}
.hero-top {
  display: flex;
  align-items: center;
  gap: 11px;
}
.logo {
  width: 40px;
  height: 40px;
  border-radius: 11px;
  background: rgba(255, 255, 255, 0.14);
  display: flex;
  align-items: center;
  justify-content: center;
}
.hero-brand {
  font-size: 22px;
  font-weight: 600;
}
.hero-brand b {
  font-weight: 800;
}
.hero-copy {
  margin-top: auto;
}
.hero-copy h1 {
  font-size: 36px;
  line-height: 1.2;
  font-weight: 800;
  margin: 0;
  letter-spacing: -0.01em;
}
.hero-copy p {
  margin: 16px 0 0;
  color: #c5c8e0;
  font-size: 15px;
}
.features {
  list-style: none;
  padding: 0;
  margin: 32px 0 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.features li {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
  color: #e7e9f7;
}
.features li svg {
  flex-shrink: 0;
  background: rgba(255, 255, 255, 0.14);
  border-radius: 50%;
  padding: 2px;
}

/* 右侧表单 */
.form-side {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  background: var(--bg);
}
.form-card {
  width: 360px;
  max-width: 100%;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-md);
  padding: 38px 34px;
}
.form-card h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
}
.form-sub {
  margin: 6px 0 22px;
  color: var(--muted);
}
.input-icon {
  position: relative;
}
.input-icon svg {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--muted);
}
.input-icon .field {
  padding-left: 36px;
  padding-right: 42px;
}
.input-icon .toggle {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--muted);
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.16s ease, color 0.16s ease;
}
.input-icon .toggle:hover {
  color: var(--text-2);
  background: var(--bg);
}

@media (max-width: 860px) {
  .login {
    grid-template-columns: 1fr;
  }
  .hero {
    display: none;
  }
}
</style>
