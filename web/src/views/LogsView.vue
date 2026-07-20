<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { getLogs, getTaskTypes, deleteLogs } from '@/api'
import { ApiError } from '@/api/http'
import type { JobExecLog, PageResult, ExecStatus, TaskTypeOption } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'

const route = useRoute()
const router = useRouter()

const result = ref<PageResult | null>(null)
const loading = ref(false)
const error = ref('')

// Task Type 下拉数据：来自后端，显示任务名 name，提交时仍用 task_type。
const taskTypes = ref<TaskTypeOption[]>([])
const taskTypesLoading = ref(false)

async function loadTaskTypes() {
  taskTypesLoading.value = true
  try {
    const res = await getTaskTypes()
    taskTypes.value = res.list || []
  } catch {
    taskTypes.value = []
  } finally {
    taskTypesLoading.value = false
  }
}

// task_type -> 任务名 映射，用于将列表中的 task_type 还原为可读名称。
const taskTypeName = computed<Record<string, string>>(() => {
  const m: Record<string, string> = {}
  for (const t of taskTypes.value) m[t.task_type] = t.name
  return m
})

const filters = reactive({
  task_type: '',
  status: '' as '' | ExecStatus,
  protocol: '' as '' | 'http' | 'grpc',
  start: '',
  end: '',
  sort: 'created_at',
  order: 'desc' as 'asc' | 'desc',
  page: 1,
  page_size: 20,
})

const statusOptions: { value: ExecStatus; label: string }[] = [
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'skipped', label: '跳过' },
]
const protocolOptions = [
  { value: 'http', label: 'HTTP' },
  { value: 'grpc', label: 'gRPC' },
]

const totalPages = computed(() =>
  result.value ? Math.max(1, Math.ceil(result.value.total / filters.page_size)) : 1,
)
const pageInput = ref(1)

async function load() {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    result.value = await getLogs({
      task_type: filters.task_type,
      status: filters.status,
      protocol: filters.protocol,
      start: filters.start || undefined,
      end: filters.end || undefined,
      sort: filters.sort,
      order: filters.order,
      page: filters.page,
      page_size: filters.page_size,
    })
    pageInput.value = filters.page
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载日志失败'
  } finally {
    loading.value = false
  }
}

function search() {
  filters.page = 1
  load()
}

function resetFilters() {
  filters.task_type = ''
  filters.status = ''
  filters.protocol = ''
  filters.start = ''
  filters.end = ''
  filters.page = 1
  load()
}

function changeSort(field: string) {
  if (filters.sort === field) {
    filters.order = filters.order === 'asc' ? 'desc' : 'asc'
  } else {
    filters.sort = field
    filters.order = 'desc'
  }
  load()
}

function sortIndicator(field: string) {
  if (filters.sort !== field) return '⇅'
  return filters.order === 'asc' ? '↑' : '↓'
}

function gotoPage(p: number) {
  if (p < 1 || p > totalPages.value) return
  filters.page = p
  pageInput.value = p
  load()
}

function onPageInput() {
  gotoPage(pageInput.value)
}

function fmt(d: string | null) {
  if (!d) return '—'
  const dt = new Date(d)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())} ${pad(dt.getHours())}:${pad(dt.getMinutes())}:${pad(dt.getSeconds())}`
}
function dur(ms: number | null) {
  if (ms === null) return '—'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

// 跳转到单条日志详情页。
function openDetail(log: JobExecLog) {
  router.push({ name: 'log-detail', params: { id: log.id } })
}

// 批量删除：选中态与操作。
const selectedIds = ref<string[]>([])
const deleting = ref(false)
const notice = ref('')
const confirmOpen = ref(false)

const listIds = computed(() => (result.value?.list || []).map((l) => l.id))
const selectedCount = computed(() => selectedIds.value.length)
const allSelected = computed(
  () => listIds.value.length > 0 && listIds.value.every((id) => selectedIds.value.includes(id)),
)
const someSelected = computed(
  () => selectedCount.value > 0 && !allSelected.value,
)

function isSelected(id: string) {
  return selectedIds.value.includes(id)
}

function toggleOne(id: string) {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) {
    selectedIds.value.splice(i, 1)
  } else {
    selectedIds.value.push(id)
  }
}

function toggleAll(e: Event) {
  const checked = (e.target as HTMLInputElement).checked
  selectedIds.value = checked ? [...listIds.value] : []
}

async function deleteSelected() {
  if (selectedIds.value.length === 0) return
  confirmOpen.value = true
}

async function confirmDelete() {
  if (selectedIds.value.length === 0) return
  deleting.value = true
  error.value = ''
  notice.value = ''
  try {
    const ids = [...selectedIds.value]
    const res = await deleteLogs(ids)
    selectedIds.value = []
    confirmOpen.value = false
    await load()
    notice.value = res.message || `已删除 ${res.deleted} 条日志`
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '删除日志失败'
  } finally {
    deleting.value = false
  }
}

onMounted(async () => {
  await loadTaskTypes()
  // 支持从任务管理页带 task_type 跳转过来，自动预选并查询。
  const q = route.query.task_type
  if (typeof q === 'string' && q) {
    filters.task_type = q
  }
  await load()
})
</script>

<template>
  <div class="page">
    <!-- 过滤栏 -->
    <div class="card toolbar">
      <div class="toolbar-inner">
        <div class="filters">
          <div class="fgroup">
            <label class="lbl">任务类型</label>
            <div class="field-wrap">
              <svg class="fw-icon" viewBox="0 0 20 20" width="15" height="15"><circle cx="9" cy="9" r="6" fill="none" stroke="currentColor" stroke-width="2"/><path d="M14 14l3 3" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
              <select v-model="filters.task_type" class="field has-icon" :disabled="taskTypesLoading">
                <option value="">全部任务</option>
                <option v-for="t in taskTypes" :key="t.task_type" :value="t.task_type">{{ t.name }}</option>
              </select>
            </div>
          </div>
          <div class="fgroup">
            <label class="lbl">执行状态</label>
            <div class="field-wrap">
              <svg class="fw-icon" viewBox="0 0 20 20" width="15" height="15"><path d="M10 2.5l2.2 4.5 5 .7-3.6 3.5.9 4.9L10 13.9 5.5 16.1l.9-4.9L2.8 7.7l5-.7z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/></svg>
              <select v-model="filters.status" class="field has-icon">
                <option value="">全部状态</option>
                <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
              </select>
            </div>
          </div>
          <div class="fgroup">
            <label class="lbl">调用协议</label>
            <div class="field-wrap">
              <svg class="fw-icon" viewBox="0 0 20 20" width="15" height="15"><path d="M3 10h14M3 5h14M3 15h14" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>
              <select v-model="filters.protocol" class="field has-icon">
                <option value="">全部协议</option>
                <option v-for="o in protocolOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
              </select>
            </div>
          </div>
          <div class="fgroup fgroup-range">
            <label class="lbl">时间范围</label>
            <div class="range">
              <div class="field-wrap">
                <svg class="fw-icon" viewBox="0 0 20 20" width="15" height="15"><rect x="3" y="4" width="14" height="13" rx="2" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="M3 8h14M7 2.5v3M13 2.5v3" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
                <input v-model="filters.start" type="date" class="field has-icon" />
              </div>
              <span class="range-sep">至</span>
              <div class="field-wrap">
                <svg class="fw-icon" viewBox="0 0 20 20" width="15" height="15"><rect x="3" y="4" width="14" height="13" rx="2" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="M3 8h14M7 2.5v3M13 2.5v3" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
                <input v-model="filters.end" type="date" class="field has-icon" />
              </div>
            </div>
          </div>
        </div>
        <div class="toolbar-actions">
          <button class="btn" @click="resetFilters" title="重置筛选条件">
            <svg viewBox="0 0 20 20" width="15" height="15"><path d="M5 5l10 10M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><circle cx="10" cy="10" r="7.5" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>
            重置
          </button>
          <button class="btn" @click="load" title="刷新数据">
            <svg viewBox="0 0 24 24" width="15" height="15"><path d="M20 11a8 8 0 1 0-2 5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M20 5v6h-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
          </button>
          <button class="btn btn-primary" @click="search">
            <svg viewBox="0 0 20 20" width="15" height="15"><circle cx="9" cy="9" r="6" fill="none" stroke="currentColor" stroke-width="2"/><path d="M14 14l3 3" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
            查询
          </button>
        </div>
      </div>
    </div>

    <div v-if="error" class="alert error" style="margin: 0 0 16px">
      <svg viewBox="0 0 20 20" width="16" height="16"><circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2"/><path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
      <span>{{ error }}</span>
    </div>

    <div v-if="notice" class="alert success" style="margin: 0 0 16px">
      <svg viewBox="0 0 20 20" width="16" height="16"><path d="M5 10l3.5 3.5L15 6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
      <span>{{ notice }}</span>
    </div>

    <section class="table-card card">
      <header class="tc-head">
        <div class="tc-head-left">
          <h2 class="tc-title">执行记录</h2>
          <span class="tc-count" v-if="result">共 {{ result.total }} 条</span>
          <span class="tc-selected" v-if="selectedCount">已选 {{ selectedCount }} 条</span>
        </div>
        <button
          v-if="selectedCount"
          class="btn btn-danger"
          :disabled="deleting"
          @click="deleteSelected"
          title="删除选中的日志"
        >
          <svg viewBox="0 0 20 20" width="15" height="15"><path d="M4 6h12M8 6V4h4v2M6 6l1 10h6l1-10" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>
          {{ deleting ? '删除中…' : `删除选中 (${selectedCount})` }}
        </button>
      </header>
      <div class="tc-scroll">
        <table class="data-table">
          <thead>
            <tr>
              <th class="col-check">
                <input
                  type="checkbox"
                  class="cb"
                  :checked="allSelected"
                  :indeterminate="someSelected"
                  :disabled="!(result && result.list.length)"
                  @change="toggleAll"
                  title="全选当前页"
                />
              </th>
              <th class="col-sortable" @click="changeSort('created_at')">执行时间 <span class="sort-ind">{{ sortIndicator('created_at') }}</span></th>
              <th class="col-sortable" @click="changeSort('task_type')">任务 <span class="sort-ind">{{ sortIndicator('task_type') }}</span></th>
              <th class="col-sortable" @click="changeSort('status')">状态 <span class="sort-ind">{{ sortIndicator('status') }}</span></th>
              <th class="col-sortable" @click="changeSort('protocol')">协议 <span class="sort-ind">{{ sortIndicator('protocol') }}</span></th>
              <th>触发</th>
              <th>目标地址</th>
              <th class="col-sortable" @click="changeSort('retry_count')">重试 <span class="sort-ind">{{ sortIndicator('retry_count') }}</span></th>
              <th>耗时</th>
              <th class="col-err">错误信息</th>
              <th class="col-op">操作</th>
            </tr>
          </thead>
          <tbody v-if="loading && !result">
            <tr v-for="n in 8" :key="n">
              <td colspan="11" style="padding: 14px"><span class="skeleton" :style="{ width: 60 + (n % 4) * 9 + '%' }"></span></td>
            </tr>
          </tbody>
          <tbody v-else>
            <tr v-for="log in result?.list" :key="log.id">
              <td class="col-check">
                <input
                  type="checkbox"
                  class="cb"
                  :checked="isSelected(log.id)"
                  @change="toggleOne(log.id)"
                  title="选择此条"
                />
              </td>
              <td class="mono nowrap">{{ fmt(log.created_at) }}</td>
              <td class="mono">{{ taskTypeName[log.task_type] || log.task_type }}</td>
              <td><span class="badge" :class="log.status">{{ log.status }}</span></td>
              <td>{{ (log.protocol || '').toUpperCase() }}</td>
              <td><span class="badge" :class="log.trigger_type === 'manual' ? 'manual' : 'cron'">{{ log.trigger_type }}</span></td>
              <td class="mono ellipsis" :title="log.target_endpoint">{{ log.target_endpoint }}</td>
              <td>{{ log.retry_count }}</td>
              <td class="mono">{{ dur(log.execution_duration_ms) }}</td>
              <td class="err-cell" :title="log.error_msg || ''">{{ log.error_msg || '—' }}</td>
              <td class="col-op">
                <button class="btn sm link" @click="openDetail(log)">详情</button>
              </td>
            </tr>
            <tr v-if="result && result.list.length === 0">
              <td colspan="11">
                <div class="empty">
                  <svg viewBox="0 0 24 24" width="40" height="40"><path d="M5 4h11l3 3v13H5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M8 11h8M8 15h8M8 19h5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
                  <span>未找到匹配的执行日志</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- 分页 -->
    <footer class="pager" v-if="result">
      <span class="muted">第 {{ result.page }} / {{ totalPages }} 页</span>
      <div class="pager-ctrl">
        <button class="btn btn-sm" :disabled="filters.page <= 1" @click="gotoPage(filters.page - 1)">上一页</button>
        <div class="page-box">
          <input
            class="field page-input"
            type="number"
            min="1"
            :max="totalPages"
            v-model.number="pageInput"
            @change="onPageInput"
          />
          <span class="muted">/ {{ totalPages }}</span>
        </div>
        <button class="btn btn-sm" :disabled="filters.page >= totalPages" @click="gotoPage(filters.page + 1)">下一页</button>
      </div>
    </footer>

    <ConfirmDialog
      :open="confirmOpen"
      danger
      title="删除日志"
      :confirm-text="'删除'"
      :loading="deleting"
      :message="`确定要删除选中的 ${selectedCount} 条日志吗？\n此操作不可恢复。`"
      @update:open="confirmOpen = $event"
      @confirm="confirmDelete"
    />
  </div>
</template>

<style scoped>
.page {
  width: 100%;
}

.toolbar {
  padding: 18px 20px;
  margin-bottom: 16px;
}
.toolbar-inner {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 18px 20px;
}
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 16px 18px;
  align-items: flex-end;
  flex: 1 1 auto;
  min-width: 0;
}
.fgroup {
  display: flex;
  flex-direction: column;
  flex: 1 1 150px;
  min-width: 0;
}
.fgroup-range {
  flex: 1 1 300px;
}
.fgroup .field,
.fgroup .field-wrap {
  width: 100%;
}
.lbl {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
  margin-bottom: 7px;
  letter-spacing: 0.01em;
}
.field-wrap {
  position: relative;
  display: flex;
  align-items: center;
}
.fw-icon {
  position: absolute;
  left: 11px;
  color: var(--muted);
  pointer-events: none;
  transition: color 0.15s ease;
}
.field.has-icon {
  padding-left: 34px;
}
.field-wrap:focus-within .fw-icon {
  color: var(--primary);
}
.range {
  display: flex;
  align-items: center;
  gap: 10px;
}
.range .field-wrap {
  flex: 1;
  min-width: 0;
}
.range-sep {
  flex: 0 0 auto;
  color: var(--muted);
  font-size: 12px;
}
.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 0 0 auto;
}
.toolbar-actions .btn {
  height: 38px;
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
  min-width: 0;
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
.tc-selected {
  font-size: 12.5px;
  color: var(--primary);
  font-weight: 600;
  padding: 2px 9px;
  border-radius: 999px;
  background: var(--primary-l);
}
.btn-danger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 36px;
  padding: 0 14px;
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  background: var(--danger);
  border: 1px solid var(--danger);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 0.15s ease, opacity 0.15s ease;
}
.btn-danger:hover:not(:disabled) {
  opacity: 0.88;
}
.btn-danger:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.col-check {
  width: 40px;
  text-align: center;
  white-space: nowrap;
}
.cb {
  width: 16px;
  height: 16px;
  accent-color: var(--primary);
  cursor: pointer;
}
.tc-scroll {
  overflow-x: auto;
}
.data-table {
  min-width: 940px;
}
.data-table thead th {
  background: var(--panel-2);
  position: sticky;
  top: 0;
  z-index: 1;
}
.sort-ind {
  color: var(--muted);
  font-size: 11px;
  margin-left: 2px;
}
.nowrap {
  white-space: nowrap;
}
.err-cell {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--danger);
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
.pager {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 16px;
}
.pager-ctrl {
  display: flex;
  align-items: center;
  gap: 10px;
}
.page-box {
  display: flex;
  align-items: center;
  gap: 6px;
}
.page-input {
  width: 64px;
  height: 30px;
  text-align: center;
  padding: 0 4px;
}

/* ---------- 响应式：分档自适应筛选栏 ---------- */
@media (max-width: 1100px) {
  .toolbar-inner {
    gap: 16px;
  }
}
@media (max-width: 860px) {
  .filters {
    flex: 1 1 100%;
  }
  .fgroup {
    flex: 1 1 calc(50% - 9px);
  }
  .fgroup-range {
    flex: 1 1 100%;
  }
}
@media (max-width: 560px) {
  .fgroup {
    flex: 1 1 100%;
  }
  .toolbar-actions {
    flex: 1 1 100%;
  }
  .toolbar-actions .btn {
    flex: 1;
  }
  .range {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }
  .range-sep {
    display: none;
  }
  .pager {
    flex-direction: column;
    gap: 12px;
    align-items: stretch;
  }
  .pager-ctrl {
    justify-content: space-between;
  }
}
</style>
