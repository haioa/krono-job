<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'

import { getUsers, createUser, updateUser, deleteUser } from '@/api'
import { ApiError } from '@/api/http'
import type { UserView, UserStatus, UpdateUserRequest } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import ConfirmDialog from '@/components/ConfirmDialog.vue'

const auth = useAuthStore()

const users = ref<UserView[]>([])
const loading = ref(false)
const error = ref('')
const notice = ref('')

async function load() {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    const res = await getUsers()
    users.value = res.list || []
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载用户失败'
  } finally {
    loading.value = false
  }
}

// 当前登录用户 id，用于禁止其删除/禁用自己。
const myId = computed(() => auth.user?.id)

function isSelf(u: UserView) {
  return !!myId.value && myId.value === u.id
}

function statusLabel(s: UserStatus) {
  return s === 'active' ? '启用' : '禁用'
}

function fmt(d: string | null) {
  if (!d) return '—'
  const dt = new Date(d)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())} ${pad(dt.getHours())}:${pad(dt.getMinutes())}`
}

// ---------- 新增 / 编辑弹窗 ----------
const dialogOpen = ref(false)
const editing = ref<UserView | null>(null)
const form = reactive({
  username: '',
  nickname: '',
  password: '',
  status: 'active' as UserStatus,
})
const showPwd = ref(false)
const formError = ref('')
const saving = ref(false)

function openCreate() {
  editing.value = null
  form.username = ''
  form.nickname = ''
  form.password = ''
  form.status = 'active'
  showPwd.value = false
  formError.value = ''
  saving.value = false
  dialogOpen.value = true
}

function openEdit(u: UserView) {
  editing.value = u
  form.username = u.username
  form.nickname = u.nickname
  form.password = ''
  form.status = u.status
  showPwd.value = false
  formError.value = ''
  saving.value = false
  dialogOpen.value = true
}

function closeDialog() {
  dialogOpen.value = false
}

async function save() {
  formError.value = ''
  const username = form.username.trim()
  if (!username) {
    formError.value = '用户名必填'
    return
  }
  // 编辑态禁止禁用/删除自己，避免锁死控制台。
  if (editing.value && isSelf(editing.value) && form.status === 'disabled') {
    formError.value = '不能禁用当前登录的账号'
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      const req: UpdateUserRequest = { nickname: form.nickname.trim(), status: form.status }
      if (form.password) req.password = form.password
      const updated = await updateUser(editing.value.id, req)
      const i = users.value.findIndex((x) => x.id === updated.id)
      if (i >= 0) users.value[i] = updated
      notice.value = '用户已更新'
    } else {
      if (form.password.length < 6) {
        formError.value = '密码长度至少 6 位'
        saving.value = false
        return
      }
      const created = await createUser({
        username,
        nickname: form.nickname.trim() || username,
        password: form.password,
        status: form.status,
      })
      users.value = [...users.value, created]
      notice.value = '用户已创建'
    }
    dialogOpen.value = false
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

// ---------- 删除确认 ----------
const confirmOpen = ref(false)
const deleteTarget = ref<UserView | null>(null)
const deleting = ref(false)

function askDelete(u: UserView) {
  if (isSelf(u)) return
  deleteTarget.value = u
  confirmOpen.value = true
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  error.value = ''
  notice.value = ''
  try {
    const res = await deleteUser(deleteTarget.value.id)
    users.value = users.value.filter((x) => x.id !== deleteTarget.value!.id)
    confirmOpen.value = false
    notice.value = res.message || '已删除用户'
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '删除失败'
  } finally {
    deleting.value = false
  }
}

const statusOptions: { value: UserStatus; label: string }[] = [
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '禁用' },
]

onMounted(load)
</script>

<template>
  <div class="page">
    <div v-if="error" class="alert error" style="margin-bottom: 16px">
      <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
        <circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2" />
        <path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
      </svg>
      <span>{{ error }}</span>
    </div>
    <div v-if="notice" class="alert success" style="margin-bottom: 16px">
      <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
        <path d="M5 10l3.5 3.5L15 6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      <span>{{ notice }}</span>
    </div>

    <section class="card table-card">
      <header class="tc-head">
        <div class="tc-head-left">
          <h2 class="tc-title">管理员账号</h2>
          <span class="tc-count" v-if="users.length">共 {{ users.length }} 个</span>
        </div>
        <button class="btn btn-primary" @click="openCreate">
          <svg viewBox="0 0 20 20" width="15" height="15" aria-hidden="true">
            <path d="M10 4v12M4 10h12" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
          新增用户
        </button>
      </header>

      <div class="tc-scroll">
        <table class="data-table">
          <thead>
            <tr>
              <th>用户名</th>
              <th>昵称</th>
              <th>状态</th>
              <th>创建时间</th>
              <th class="col-op">操作</th>
            </tr>
          </thead>
          <tbody v-if="loading && users.length === 0">
            <tr v-for="n in 5" :key="n">
              <td colspan="5" style="padding: 14px"><span class="skeleton" :style="{ width: 50 + (n % 3) * 12 + '%' }"></span></td>
            </tr>
          </tbody>
          <tbody v-else>
            <tr v-for="u in users" :key="u.id">
              <td class="mono">{{ u.username }}</td>
              <td>{{ u.nickname || '—' }}</td>
              <td>
                <span class="badge" :class="u.status === 'active' ? 'active' : 'disabled'">{{ statusLabel(u.status) }}</span>
              </td>
              <td class="mono nowrap">{{ fmt(u.created_at) }}</td>
              <td class="col-op">
                <button class="btn sm link" @click="openEdit(u)">编辑</button>
                <button class="btn sm danger-link" :disabled="isSelf(u)" @click="askDelete(u)">删除</button>
              </td>
            </tr>
            <tr v-if="!loading && users.length === 0">
              <td colspan="5">
                <div class="empty">
                  <svg viewBox="0 0 24 24" width="40" height="40" aria-hidden="true">
                    <circle cx="9" cy="8" r="3.2" fill="none" stroke="currentColor" stroke-width="1.5" />
                    <path d="M3.5 19a5.5 5.5 0 0 1 11 0" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
                  </svg>
                  <span>暂无用户</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- 新增 / 编辑弹窗 -->
    <Transition name="modal-fade">
      <div v-if="dialogOpen" class="modal-mask" @click.self="closeDialog">
        <div class="modal" role="dialog" aria-modal="true">
          <div class="modal-head">
            <div class="modal-title">
              <span class="modal-icon">
                <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
                  <circle cx="12" cy="8" r="4" fill="none" stroke="currentColor" stroke-width="2" />
                  <path d="M5 20a7 7 0 0 1 14 0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
                </svg>
              </span>
              <div>
                <h3>{{ editing ? '编辑用户' : '新增用户' }}</h3>
                <p>{{ editing ? '修改资料，密码留空表示不修改' : '创建一名管理员，密码至少 6 位' }}</p>
              </div>
            </div>
            <button class="modal-close" @click="closeDialog" aria-label="关闭">×</button>
          </div>

          <div class="modal-body">
            <div v-if="formError" class="alert error">
              <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
                <circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2" />
                <path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
              </svg>
              <span>{{ formError }}</span>
            </div>

            <div class="form-group">
              <label class="lbl">用户名</label>
              <input
                class="field"
                v-model="form.username"
                :disabled="!!editing"
                placeholder="登录用户名（不可修改）"
                autocomplete="off"
              />
            </div>

            <div class="form-group">
              <label class="lbl">昵称</label>
              <input class="field" v-model="form.nickname" placeholder="展示名称，留空则用用户名" autocomplete="off" />
            </div>

            <div class="form-group">
              <label class="lbl">{{ editing ? '新密码（可选）' : '密码' }}</label>
              <div class="pwd-field">
                <svg class="lead" viewBox="0 0 20 20" width="16" height="16" aria-hidden="true">
                  <rect x="4" y="9" width="12" height="8" rx="2" fill="none" stroke="currentColor" stroke-width="1.6" />
                  <path d="M7 9V6.5a3 3 0 0 1 6 0V9" fill="none" stroke="currentColor" stroke-width="1.6" />
                </svg>
                <input
                  class="field"
                  :type="showPwd ? 'text' : 'password'"
                  v-model="form.password"
                  :placeholder="editing ? '留空则不修改密码' : '至少 6 位'"
                  autocomplete="new-password"
                  @keyup.enter="save"
                />
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
            </div>

            <div class="form-group">
              <label class="lbl">状态</label>
              <div class="field-wrap">
                <select class="field has-icon" v-model="form.status">
                  <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
                </select>
              </div>
            </div>
          </div>

          <div class="modal-foot">
            <button class="btn" @click="closeDialog">取消</button>
            <button class="btn btn-primary" :disabled="saving" @click="save">
              <span v-if="saving" class="spinner"></span>
              <span>{{ saving ? '保存中…' : '保存' }}</span>
            </button>
          </div>
        </div>
      </div>
    </Transition>

    <ConfirmDialog
      :open="confirmOpen"
      danger
      title="删除用户"
      confirm-text="删除"
      :loading="deleting"
      :message="`确定要删除用户「${deleteTarget?.username || ''}」吗？\n此操作不可恢复。`"
      @update:open="confirmOpen = $event"
      @confirm="confirmDelete"
    />
  </div>
</template>

<style scoped>
.page {
  width: 100%;
}
.table-card {
  overflow: hidden;
}
.tc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
}
.tc-head-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.tc-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text);
}
.tc-count {
  font-size: 12.5px;
  color: var(--muted);
}
.tc-scroll {
  overflow-x: auto;
}
.data-table {
  min-width: 640px;
}
.nowrap {
  white-space: nowrap;
}
.col-op {
  text-align: right;
  white-space: nowrap;
  width: 1%;
}
.btn.sm.link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 30px;
  padding: 0 12px;
  font-size: 12px;
  border: 1px solid var(--primary);
  color: var(--primary);
  background: var(--primary-l);
  border-radius: var(--radius-sm);
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}
.btn.sm.link:hover:not(:disabled) {
  background: var(--primary);
  color: #fff;
}
.btn.sm.danger-link {
  display: inline-flex;
  align-items: center;
  height: 30px;
  padding: 0 12px;
  margin-left: 8px;
  font-size: 12px;
  border: 1px solid var(--danger);
  color: var(--danger);
  background: var(--danger-l);
  border-radius: var(--radius-sm);
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}
.btn.sm.danger-link:hover:not(:disabled) {
  background: var(--danger);
  color: #fff;
}
.btn.sm.danger-link:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* ---------- 弹窗（复用设计系统） ---------- */
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
  max-width: 460px;
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
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}
.form-group .field {
  height: 44px;
  font-size: 14px;
}
.form-group .field:disabled {
  background: var(--bg);
  color: var(--muted);
  cursor: not-allowed;
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
.field-wrap {
  position: relative;
  display: flex;
  align-items: center;
}
.field-wrap .field.has-icon {
  width: 100%;
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
.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.5);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
