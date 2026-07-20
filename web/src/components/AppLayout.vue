<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { changePassword } from '@/api'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const title = computed(() => {
  if (route.name === 'stats') return '统计看板'
  if (route.name === 'jobs') return '任务管理'
  if (route.name === 'logs') return '执行日志'
  if (route.name === 'users') return '用户管理'
  return '控制台'
})
const subtitle = computed(() => {
  if (route.name === 'stats') return '执行结果概览、每日趋势与任务调用排行'
  if (route.name === 'jobs') return '查看调度任务，实时暂停或恢复执行'
  if (route.name === 'logs') return '检索任务执行记录与结果'
  if (route.name === 'users') return '管理系统管理员账号'
  return ''
})

const initials = computed(() => {
  const name = auth.user?.nickname || auth.user?.username || '?'
  return name.slice(0, 1).toUpperCase()
})

// 移动端侧边栏抽屉开关
const sidebarOpen = ref(false)
function toggleSidebar() {
  sidebarOpen.value = !sidebarOpen.value
}
function closeSidebar() {
  sidebarOpen.value = false
}
// 路由切换（如点击导航）时自动收起抽屉
watch(
  () => route.fullPath,
  () => {
    sidebarOpen.value = false
  },
)

function onLogout() {
  auth.logout()
  router.push({ name: 'login' })
}

// ---------- 修改密码 ----------
const pwdOpen = ref(false)
const newPassword = ref('')
const confirmPassword = ref('')
const showNew = ref(false)
const showConfirm = ref(false)
const pwdError = ref('')
const pwdSuccess = ref('')
const pwdSubmitting = ref(false)

function openPwd() {
  newPassword.value = ''
  confirmPassword.value = ''
  showNew.value = false
  showConfirm.value = false
  pwdError.value = ''
  pwdSuccess.value = ''
  pwdSubmitting.value = false
  pwdOpen.value = true
}
function closePwd() {
  pwdOpen.value = false
}

async function submitPwd() {
  pwdError.value = ''
  pwdSuccess.value = ''
  if (newPassword.value.length < 6) {
    pwdError.value = '新密码长度至少 6 位'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    pwdError.value = '两次输入的密码不一致'
    return
  }
  pwdSubmitting.value = true
  try {
    await changePassword({ new_password: newPassword.value })
    pwdSuccess.value = '密码修改成功'
    newPassword.value = ''
    confirmPassword.value = ''
    setTimeout(() => {
      pwdOpen.value = false
    }, 1000)
  } catch (e) {
    pwdError.value = e instanceof ApiError ? e.message : '修改密码失败'
  } finally {
    pwdSubmitting.value = false
  }
}
</script>

<template>
  <div class="layout">
    <!-- 移动端遮罩 -->
    <div v-if="sidebarOpen" class="backdrop" @click="closeSidebar"></div>

    <aside class="sidebar" :class="{ open: sidebarOpen }">
      <div class="brand">
        <span class="logo">
          <svg viewBox="0 0 32 32" width="22" height="22" aria-hidden="true">
            <circle cx="16" cy="16" r="10" fill="none" stroke="#fff" stroke-width="2.4" />
            <path d="M16 10v6l4 3" stroke="#fff" stroke-width="2.4" stroke-linecap="round" fill="none" />
          </svg>
        </span>
        <span class="brand-name">Krono<b>Job</b></span>
      </div>

      <nav class="nav">
        <router-link to="/stats" class="nav-item">
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <path d="M4 20V10M10 20V4M16 20v-7M22 20H2" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span>统计看板</span>
        </router-link>
        <router-link to="/jobs" class="nav-item">
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <path
              d="M4 6h16M4 12h16M4 18h10"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
            />
          </svg>
          <span>任务管理</span>
        </router-link>
        <router-link to="/logs" class="nav-item">
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <path
              d="M5 4h11l3 3v13H5z"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linejoin="round"
            />
            <path d="M8 11h8M8 15h8M8 19h5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
          </svg>
          <span>执行日志</span>
        </router-link>
        <router-link to="/users" class="nav-item">
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <circle cx="9" cy="8" r="3.2" fill="none" stroke="currentColor" stroke-width="2" />
            <path d="M3.5 19a5.5 5.5 0 0 1 11 0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            <path d="M16 6.5a3 3 0 0 1 0 5.8M17.5 19a5.2 5.2 0 0 0-2.7-4.6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
          <span>用户管理</span>
        </router-link>
      </nav>

      <div class="side-foot">
        <div class="user-card">
          <span class="avatar">{{ initials }}</span>
          <div class="u-meta">
            <div class="u-name">{{ auth.user?.nickname || auth.user?.username }}</div>
            <div class="u-role">管理员</div>
          </div>
        </div>
        <button class="pwd-btn" @click="openPwd">
          <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
            <rect x="5" y="11" width="14" height="9" rx="2" fill="none" stroke="currentColor" stroke-width="2" />
            <path d="M8 11V8a4 4 0 0 1 8 0v3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
          <span>修改密码</span>
        </button>
        <button class="logout" @click="onLogout">
          <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
            <path
              d="M14 8V6a2 0 0 0-2-2H6a2 0 0 0-2 2v12a2 0 0 0 2 2h6a2 0 0 0 2-2v-2"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
            />
            <path d="M9 12h11m0 0-3-3m3 3-3 3" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span>退出登录</span>
        </button>
      </div>
    </aside>

    <!-- 修改密码弹窗 -->
    <transition name="modal-fade">
      <div v-if="pwdOpen" class="modal-mask" @click.self="closePwd">
        <div class="modal" role="dialog" aria-modal="true">
          <div class="modal-head">
            <div class="modal-title">
              <span class="modal-icon">
                <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
                  <rect x="5" y="11" width="14" height="9" rx="2.4" fill="none" stroke="currentColor" stroke-width="2" />
                  <path d="M8 11V8a4 4 0 0 1 8 0v3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
                </svg>
              </span>
              <div>
                <h3>修改密码</h3>
                <p>设置新的登录密码，无需输入原密码</p>
              </div>
            </div>
            <button class="modal-close" @click="closePwd" aria-label="关闭">×</button>
          </div>

          <div class="modal-body">
            <div v-if="pwdError" class="alert error">
              <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
                <circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2" />
                <path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
              </svg>
              <span>{{ pwdError }}</span>
            </div>
            <div v-if="pwdSuccess" class="alert success">
              <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
                <circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2" />
                <path d="M6.5 10l2.5 2.5 4.5-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
              <span>{{ pwdSuccess }}</span>
            </div>

            <div class="form-group">
              <label class="lbl">新密码</label>
              <div class="pwd-field">
                <svg class="lead" viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
                  <rect x="4" y="9" width="12" height="8" rx="2" fill="none" stroke="currentColor" stroke-width="1.6" />
                  <path d="M7 9V6.5a3 3 0 0 1 6 0V9" fill="none" stroke="currentColor" stroke-width="1.6" />
                </svg>
                <input
                  class="field"
                  :type="showNew ? 'text' : 'password'"
                  v-model="newPassword"
                  placeholder="请输入新密码（至少 6 位）"
                  autocomplete="new-password"
                  @keyup.enter="submitPwd"
                />
                <button type="button" class="toggle" @click="showNew = !showNew" :aria-label="showNew ? '隐藏密码' : '显示密码'">
                  <svg v-if="!showNew" viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
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
            </div>

            <div class="form-group">
              <label class="lbl">确认新密码</label>
              <div class="pwd-field">
                <svg class="lead" viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
                  <rect x="4" y="9" width="12" height="8" rx="2" fill="none" stroke="currentColor" stroke-width="1.6" />
                  <path d="M7 9V6.5a3 3 0 0 1 6 0V9" fill="none" stroke="currentColor" stroke-width="1.6" />
                </svg>
                <input
                  class="field"
                  :type="showConfirm ? 'text' : 'password'"
                  v-model="confirmPassword"
                  placeholder="请再次输入新密码"
                  autocomplete="new-password"
                  @keyup.enter="submitPwd"
                />
                <button type="button" class="toggle" @click="showConfirm = !showConfirm" :aria-label="showConfirm ? '隐藏密码' : '显示密码'">
                  <svg v-if="!showConfirm" viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
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
            </div>

            <div class="hint-row">
              <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
                <circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" stroke-width="1.4" />
                <path d="M8 7v4M8 4.6v.4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
              </svg>
              <span>密码长度至少 6 位，建议使用大小写字母、数字与符号的组合</span>
            </div>
          </div>

          <div class="modal-foot">
            <button class="btn" @click="closePwd">取消</button>
            <button class="btn btn-primary" :disabled="pwdSubmitting" @click="submitPwd">
              <span v-if="pwdSubmitting" class="spinner dark"></span>
              <span>{{ pwdSubmitting ? '提交中…' : '确认修改' }}</span>
            </button>
          </div>
        </div>
      </div>
    </transition>

    <div class="main">
      <header class="topbar">
        <div class="topbar-left">
          <button class="menu-btn" @click="toggleSidebar" aria-label="打开菜单">
            <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true">
              <path d="M4 6h16M4 12h16M4 18h16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            </svg>
          </button>
          <div>
            <h1>{{ title }}</h1>
            <p v-if="subtitle" class="sub">{{ subtitle }}</p>
          </div>
        </div>

      </header>
      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100%;
}

/* ---------- 侧边栏 ---------- */
.sidebar {
  width: 244px;
  flex-shrink: 0;
  background: var(--sidebar);
  color: #fff;
  display: flex;
  flex-direction: column;
  padding: 20px 14px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px 22px;
}
.logo {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.12);
  display: flex;
  align-items: center;
  justify-content: center;
}
.brand-name {
  font-size: 18px;
  font-weight: 600;
  letter-spacing: 0.01em;
}
.brand-name b {
  font-weight: 800;
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 10px 12px;
  border-radius: 10px;
  color: #c5c8e0;
  font-size: 14px;
  font-weight: 500;
  position: relative;
  transition: background 0.16s, color 0.16s;
}
.nav-item:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
}
.nav-item.router-link-active {
  background: rgba(255, 255, 255, 0.14);
  color: #fff;
}
.nav-item.router-link-active::before {
  content: '';
  position: absolute;
  left: -14px;
  top: 50%;
  transform: translateY(-50%);
  width: 4px;
  height: 22px;
  border-radius: 0 4px 4px 0;
  background: #fff;
}

.side-foot {
  border-top: 1px solid rgba(255, 255, 255, 0.12);
  padding-top: 14px;
}
.user-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 6px 12px;
}
.avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.16);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 15px;
}
.u-name {
  font-size: 13px;
  font-weight: 600;
}
.u-role {
  font-size: 11px;
  color: #a9add0;
}
.logout {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 36px;
  border: 1px solid rgba(255, 255, 255, 0.22);
  background: transparent;
  color: #e7e9f7;
  border-radius: 9px;
  font-size: 13px;
  transition: background 0.16s;
}
.logout:hover {
  background: rgba(255, 255, 255, 0.12);
}
.pwd-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 36px;
  margin-bottom: 8px;
  border: 1px solid rgba(255, 255, 255, 0.22);
  background: transparent;
  color: #e7e9f7;
  border-radius: 9px;
  font-size: 13px;
  transition: background 0.16s;
}
.pwd-btn:hover {
  background: rgba(255, 255, 255, 0.12);
}

/* ---------- 修改密码弹窗 ---------- */
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 18, 40, 0.5);
  backdrop-filter: blur(2px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 80;
  padding: 16px;
}
.modal {
  width: 100%;
  max-width: 440px;
  background: var(--panel);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-md);
  overflow: hidden;
  animation: modalPop 0.2s ease;
}
@keyframes modalPop {
  from {
    opacity: 0;
    transform: translateY(10px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
.modal-fade-enter-active,
.modal-fade-leave-active {
  transition: opacity 0.2s ease;
}
.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}
.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 22px 24px;
  border-bottom: 1px solid var(--border);
}
.modal-title {
  display: flex;
  align-items: center;
  gap: 14px;
}
.modal-icon {
  width: 42px;
  height: 42px;
  border-radius: 12px;
  background: var(--primary-l);
  color: var(--primary);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.modal-title h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--text);
}
.modal-title p {
  margin: 3px 0 0;
  font-size: 12.5px;
  color: var(--muted);
}
.modal-close {
  border: none;
  background: transparent;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  font-size: 22px;
  line-height: 1;
  color: var(--muted);
  cursor: pointer;
  transition: background 0.16s, color 0.16s;
}
.modal-close:hover {
  background: var(--bg);
  color: var(--text);
}
.modal-body {
  padding: 22px 24px 8px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.form-group {
  display: flex;
  flex-direction: column;
}
.form-group .lbl {
  margin-bottom: 7px;
}
.pwd-field {
  position: relative;
  display: flex;
  align-items: center;
}
.pwd-field .lead {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--muted);
  pointer-events: none;
}
.pwd-field .field {
  height: 44px;
  font-size: 14px;
  padding-left: 38px;
  padding-right: 44px;
}
.pwd-field .toggle {
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
  transition: background 0.16s, color 0.16s;
}
.pwd-field .toggle:hover {
  background: var(--bg);
  color: var(--text-2);
}
.alert {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-radius: var(--radius-sm);
  font-size: 13px;
}
.alert.error {
  background: var(--danger-l);
  color: var(--danger);
  border: 1px solid #f7c9d5;
}
.alert.success {
  background: var(--success-l);
  color: var(--success);
  border: 1px solid #b9e6c8;
}
.hint-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12.5px;
  color: var(--muted);
  line-height: 1.5;
}
.hint-row svg {
  flex-shrink: 0;
  margin-top: 1px;
}
.modal-foot {
  display: flex;
  gap: 12px;
  padding: 16px 24px 22px;
}
.modal-foot .btn {
  flex: 1;
  height: 44px;
  font-size: 14px;
  font-weight: 600;
}

/* ---------- 主区域 ---------- */
.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.topbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 18px 28px;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.topbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.menu-btn {
  display: none;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 1px solid var(--border-d);
  background: var(--panel);
  color: var(--text);
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}
.menu-btn:hover {
  border-color: var(--primary);
  color: var(--primary);
}
.topbar h1 {
  margin: 0;
  font-size: 19px;
  font-weight: 700;
}
.topbar .sub {
  margin: 3px 0 0;
  font-size: 13px;
  color: var(--muted);
}

.content {
  flex: 1;
  overflow: auto;
  padding: 26px 28px;
}

.backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 18, 40, 0.45);
  z-index: 55;
}

/* ---------- 响应式：≤1024px 侧边栏变为抽屉 ---------- */
@media (max-width: 1024px) {
  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: 248px;
    z-index: 60;
    transform: translateX(-100%);
    transition: transform 0.25s ease;
    box-shadow: var(--shadow-md);
  }
  .sidebar.open {
    transform: translateX(0);
  }
  .menu-btn {
    display: inline-flex;
  }
}

/* ---------- 响应式：窄屏间距收紧 ---------- */
@media (max-width: 768px) {
  .topbar {
    padding: 14px 16px;
  }
  .content {
    padding: 16px;
  }
}
</style>
