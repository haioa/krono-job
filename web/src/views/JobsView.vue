<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'

import { getJobs, getJobsRaw, pauseJob, resumeJob, runJob, deleteJob } from '@/api'
import { ApiError } from '@/api/http'
import type { JobView, JobDef } from '@/api/types'

import JobFormDialog from '@/components/job/JobFormDialog.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'

const router = useRouter()

const jobs = ref<JobView[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
const notice = ref('')
const busy = ref<string | null>(null)
const running = ref<string | null>(null)
const expanded = ref<string | null>(null) // 展开显示目标地址列表的任务 task_type

function toggleExpand(taskType: string) {
  expanded.value = expanded.value === taskType ? null : taskType
}

const pausedCount = computed(() => jobs.value.filter((j) => j.paused).length)
const runningCount = computed(() => jobs.value.filter((j) => !j.paused).length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await getJobs()
    jobs.value = res.list
    total.value = res.total
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载任务失败'
  } finally {
    loading.value = false
  }
}

async function toggle(job: JobView) {
  busy.value = job.task_type
  error.value = ''
  try {
    if (job.paused) {
      await resumeJob(job.task_type)
      job.paused = false
    } else {
      await pauseJob(job.task_type)
      job.paused = true
    }
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '操作失败'
  } finally {
    busy.value = null
  }
}

function protocolTag(p: string) {
  return p === 'grpc' ? 'gRPC' : 'HTTP'
}

async function runNow(job: JobView) {
  running.value = job.task_type
  error.value = ''
  notice.value = ''
  try {
    const res = await runJob(job.task_type)
    notice.value = `${job.name || job.task_type}：${res.message}`
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '手动执行投递失败'
  } finally {
    running.value = null
  }
}

// 跳转到执行日志页，并预选该任务的 task_type 进行过滤。
function viewLogs(job: JobView) {
  router.push({ name: 'logs', query: { task_type: job.task_type } })
}

// ---------- 可视化编辑：新建 / 编辑 / 删除 ----------
const formOpen = ref(false)
const editingJob = ref<JobDef | null>(null)
const confirmOpen = ref(false)
const deletingJob = ref<JobView | null>(null)

function openCreate() {
  editingJob.value = null
  formOpen.value = true
}

// 编辑：从完整定义（含 comment）中加载该任务，回填到表单。
async function openEdit(job: JobView) {
  error.value = ''
  try {
    const res = await getJobsRaw()
    const def = res.list.find((d) => d.task_type === job.task_type) || null
    editingJob.value = def
    formOpen.value = true
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载任务定义失败'
  }
}

function onSaved() {
  notice.value = '已保存，调度内核将通过热重载生效（无需重启）'
  load()
}

function askDelete(job: JobView) {
  deletingJob.value = job
  confirmOpen.value = true
}

async function confirmDelete() {
  if (!deletingJob.value) return
  const tt = deletingJob.value.task_type
  error.value = ''
  try {
    await deleteJob(tt)
    notice.value = `已删除任务 ${tt}`
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '删除失败'
  } finally {
    confirmOpen.value = false
    deletingJob.value = null
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-header">
      <div class="ph-actions">
        <button class="btn" :disabled="loading" @click="load">
          <svg :class="{ spin: loading }" viewBox="0 0 24 24" width="15" height="15"><path d="M20 11a8 8 0 1 0-2 5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M20 5v6h-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
          刷新
        </button>
        <button class="btn primary" @click="openCreate">
          <svg viewBox="0 0 24 24" width="15" height="15"><path d="M12 5v14M5 12h14" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"/></svg>
          新建任务
        </button>
      </div>
    </header>

    <section class="stat-grid">
      <article class="stat-card">
        <span class="stat-ico blue">
          <svg viewBox="0 0 24 24" width="20" height="20"><path d="M4 6h16M4 12h16M4 18h10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
        </span>
        <div class="stat-body">
          <span class="stat-value">{{ total }}</span>
          <span class="stat-label">任务总数</span>
        </div>
      </article>
      <article class="stat-card">
        <span class="stat-ico green">
          <svg viewBox="0 0 24 24" width="20" height="20"><path d="M5 12l4 4 10-10" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/></svg>
        </span>
        <div class="stat-body">
          <span class="stat-value">{{ runningCount }}</span>
          <span class="stat-label">运行中</span>
        </div>
      </article>
      <article class="stat-card">
        <span class="stat-ico amber">
          <svg viewBox="0 0 24 24" width="20" height="20"><path d="M9 5h6v3l4 4v5a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2v-5l4-4z" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/><path d="M10 19v2M14 19v2" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
        </span>
        <div class="stat-body">
          <span class="stat-value">{{ pausedCount }}</span>
          <span class="stat-label">已暂停</span>
        </div>
      </article>
    </section>

    <div v-if="error" class="alert error" style="margin-bottom: 16px">
      <svg viewBox="0 0 20 20" width="16" height="16"><circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2"/><path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
      <span>{{ error }}</span>
    </div>

    <div v-if="notice" class="alert info" style="margin-bottom: 16px">
      <svg viewBox="0 0 20 20" width="16" height="16"><circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2"/><path d="M10 9v5M10 5.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
      <span>{{ notice }}</span>
    </div>

    <section class="table-card card">
      <header class="tc-head">
        <h2 class="tc-title">调度任务</h2>
        <span class="tc-count">{{ total }} 个任务</span>
      </header>
      <div class="tc-scroll">
        <table class="data-table">
          <thead>
            <tr>
              <th>任务名称</th>
              <th>Task Type</th>
              <th>协议</th>
              <th>Cron</th>
              <th>目标地址</th>
              <th>超时</th>
              <th>状态</th>
              <th class="op">操作</th>
            </tr>
          </thead>
          <tbody v-if="loading && jobs.length === 0">
            <tr v-for="n in 5" :key="n">
              <td colspan="8" style="padding: 16px">
                <span class="skeleton" :style="{ width: 70 + (n % 3) * 10 + '%' }"></span>
              </td>
            </tr>
          </tbody>
          <tbody v-else>
            <tr v-for="job in jobs" :key="job.task_type">
              <td>
                <div class="j-name">{{ job.name || '—' }}</div>
                <div class="j-meta" v-if="job.method">{{ job.method.toUpperCase() }} · 重试 {{ job.retry }}</div>
              </td>
              <td class="mono">{{ job.task_type }}</td>
              <td><span class="tag" :class="job.protocol">{{ protocolTag(job.protocol) }}</span></td>
              <td class="mono">{{ job.cron }}</td>
              <td class="endpoint-cell">
                <template v-if="(job.endpoints && job.endpoints.length) || job.endpoint">
                  <button
                    class="ep-toggle"
                    :class="{ open: expanded === job.task_type }"
                    @click="toggleExpand(job.task_type)"
                  >
                    <span class="ep-count">{{ (job.endpoints && job.endpoints.length) ? job.endpoints.length : 1 }}</span>
                    <span class="ep-unit">个节点</span>
                    <svg class="ep-caret" viewBox="0 0 24 24" width="13" height="13"><path d="M6 9l6 6 6-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  </button>
                  <div v-if="expanded === job.task_type" class="ep-list">
                    <div v-for="(ep, i) in (job.endpoints && job.endpoints.length ? job.endpoints : [job.endpoint])" :key="i" class="ep-item">
                      <span class="ep-dot"></span>{{ ep }}
                    </div>
                  </div>
                </template>
                <span v-else class="muted">—</span>
              </td>
              <td class="mono">{{ job.timeout || '—' }}</td>
              <td>
                <span v-if="job.paused" class="badge paused">已暂停</span>
                <span v-else class="badge running">运行中</span>
              </td>
              <td class="op">
                <div class="op-group">
                  <label class="switch">
                    <input
                      type="checkbox"
                      :checked="!job.paused"
                      :disabled="busy === job.task_type"
                      @change="toggle(job)"
                    />
                    <span class="track"></span>
                    <span class="switch-label">{{ job.paused ? '暂停' : '运行' }}</span>
                  </label>
                  <button
                    class="icon-btn primary"
                    :disabled="running === job.task_type"
                    :title="running === job.task_type ? '执行中…' : '立即执行一次'"
                    @click="runNow(job)"
                  >
                    <svg v-if="running === job.task_type" class="spin" viewBox="0 0 24 24" width="15" height="15"><path d="M20 11a8 8 0 1 0-2 5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M20 5v6h-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
                    <svg v-else viewBox="0 0 24 24" width="15" height="15"><path d="M8 5v14l11-7z" fill="currentColor"/></svg>
                  </button>
                  <button
                    class="icon-btn"
                    :title="`编辑 ${job.task_type}`"
                    @click="openEdit(job)"
                  >
                    <svg viewBox="0 0 24 24" width="15" height="15"><path d="M4 20h4L19 9l-4-4L4 16z" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/><path d="M14 6l4 4" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
                  </button>
                  <button
                    class="icon-btn danger"
                    :title="`删除 ${job.task_type}`"
                    @click="askDelete(job)"
                  >
                    <svg viewBox="0 0 24 24" width="15" height="15"><path d="M5 7h14M9 7V5h6v2M7 7l1 13h8l1-13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  </button>
                  <button
                    class="icon-btn"
                    :title="`查看 ${job.task_type} 的执行日志`"
                    @click="viewLogs(job)"
                  >
                    <svg viewBox="0 0 24 24" width="15" height="15"><path d="M5 4h11l3 3v13H5z" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/><path d="M8 11h8M8 15h8M8 19h5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="jobs.length === 0 && !loading">
              <td colspan="8">
                <div class="empty">
                  <svg viewBox="0 0 24 24" width="40" height="40"><path d="M4 6h16M4 12h16M4 18h10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
                  <span>暂无调度任务</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <JobFormDialog
      :open="formOpen"
      :job="editingJob"
      @close="formOpen = false"
      @saved="onSaved"
    />

    <ConfirmDialog
      :open="confirmOpen"
      title="删除任务"
      :message="deletingJob ? `确认删除任务「${deletingJob.name || deletingJob.task_type}」？该操作会改写 jobs.yaml 并热重载，不可撤销。` : ''"
      confirm-text="删除"
      :danger="true"
      @confirm="confirmDelete"
      @update:open="confirmOpen = $event"
    />
  </div>
</template>

<style scoped>
.page {
  width: 100%;
}
.page-header {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 16px;
  margin-bottom: 20px;
}
.ph-actions {
  flex-shrink: 0;
  display: flex;
  gap: 10px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 20px;
}
.stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.18s ease, transform 0.18s ease, border-color 0.18s ease;
}
.stat-card:hover {
  box-shadow: var(--shadow);
  border-color: var(--border-d);
  transform: translateY(-2px);
}
.stat-ico {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.stat-ico.blue {
  background: var(--primary-l);
  color: var(--primary);
}
.stat-ico.green {
  background: var(--success-l);
  color: var(--success);
}
.stat-ico.amber {
  background: var(--warn-l);
  color: var(--warn);
}
.stat-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.stat-value {
  font-size: 26px;
  font-weight: 700;
  line-height: 1.1;
  color: var(--text);
}
.stat-label {
  font-size: 12.5px;
  color: var(--muted);
}

.table-card {
  overflow: hidden;
}
.tc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
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
  min-width: 760px;
}
.data-table thead th {
  background: var(--panel-2);
  position: sticky;
  top: 0;
  z-index: 1;
}
.j-name {
  font-weight: 600;
  color: var(--text);
}
.j-meta {
  font-size: 11px;
  color: var(--muted);
  margin-top: 2px;
}
.op {
  text-align: right;
  white-space: nowrap;
}
.op-group {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.icon-btn {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--border-d);
  background: var(--panel);
  color: var(--text-2);
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease;
}
.icon-btn:hover:not(:disabled) {
  border-color: var(--primary);
  color: var(--primary);
  background: var(--primary-l);
}
.icon-btn.primary {
  color: var(--primary);
  border-color: var(--primary);
  background: var(--primary-l);
}
.icon-btn.primary:hover:not(:disabled) {
  background: var(--primary);
  color: #fff;
  border-color: var(--primary);
}
.icon-btn.danger {
  color: var(--danger, #ef4444);
  border-color: #fca5a5;
}
.icon-btn.danger:hover:not(:disabled) {
  background: #fef2f2;
  border-color: #ef4444;
  color: #ef4444;
}
.icon-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.spin {
  animation: krono-spin 0.8s linear infinite;
}
@keyframes krono-spin {
  to {
    transform: rotate(360deg);
  }
}

/* 目标地址：节点数量 + 可展开列表 */
.endpoint-cell {
  min-width: 130px;
}
.ep-toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border: 1px solid var(--border-d);
  background: var(--panel);
  border-radius: 999px;
  cursor: pointer;
  color: var(--text-2);
  font-size: 12px;
  transition: border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
}
.ep-toggle:hover {
  border-color: var(--primary);
  color: var(--primary);
  background: var(--primary-l);
}
.ep-toggle.open {
  border-color: var(--primary);
  color: var(--primary);
  background: var(--primary-l);
}
.ep-count {
  font-weight: 700;
  font-size: 13px;
}
.ep-unit {
  color: var(--muted);
}
.ep-toggle.open .ep-unit {
  color: var(--primary);
}
.ep-caret {
  transition: transform 0.18s ease;
}
.ep-toggle.open .ep-caret {
  transform: rotate(180deg);
}
.ep-list {
  margin-top: 8px;
  padding: 8px 10px;
  background: var(--panel-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  max-width: 320px;
}
.ep-item {
  display: flex;
  align-items: center;
  gap: 7px;
  font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  color: var(--text-2);
  padding: 3px 0;
  word-break: break-all;
}
.ep-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--success);
  flex-shrink: 0;
}
.muted {
  color: var(--muted);
}
.alert.info {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-radius: var(--radius);
  background: var(--primary-l);
  border: 1px solid var(--primary);
  color: var(--primary-d);
  font-size: 13px;
}

@media (max-width: 768px) {
  .stat-grid {
    grid-template-columns: 1fr;
  }
}
</style>
