<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { getLogDetail } from '@/api'
import { ApiError } from '@/api/http'
import type { JobExecLog } from '@/api/types'

const route = useRoute()
const router = useRouter()

const log = ref<JobExecLog | null>(null)
const loading = ref(false)
const error = ref('')
const notFound = ref(false)

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
function protocolTag(p: string) {
  return (p || '').toUpperCase()
}
function statusLabel(s: string) {
  if (s === 'success') return '成功'
  if (s === 'failed') return '失败'
  if (s === 'skipped') return '跳过'
  return s
}

// 尝试将响应体美化输出：可解析为 JSON 则格式化，否则原样展示。
const prettyBody = computed(() => {
  const raw = log.value?.response_body
  if (!raw) return ''
  const trimmed = raw.trim()
  if (!trimmed) return ''
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return raw
  }
})

const triggerLabel = computed(() =>
  log.value?.trigger_type === 'manual' ? '手动触发' : '定时触发',
)

function back() {
  router.push({ name: 'logs' })
}

async function load() {
  const id = String(route.params.id || '')
  if (!id) {
    error.value = '缺少日志 ID'
    return
  }
  loading.value = true
  error.value = ''
  notFound.value = false
  try {
    log.value = await getLogDetail(id)
    if (!log.value) notFound.value = true
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      notFound.value = true
    } else {
      error.value = e instanceof ApiError ? e.message : '加载日志详情失败'
    }
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-header">
      <div class="ph-left">
        <button class="btn ghost-back" @click="back">
          <svg viewBox="0 0 24 24" width="15" height="15"><path d="M15 18l-6-6 6-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
          返回
        </button>
        <div class="ph-title-wrap">
          <h1 class="ph-title">执行日志详情</h1>
          <span v-if="log" class="ph-sub mono">{{ log.task_type }}</span>
        </div>
      </div>
    </header>

    <div v-if="error" class="alert error" style="margin-bottom: 16px">
      <svg viewBox="0 0 20 20" width="16" height="16"><circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2"/><path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
      <span>{{ error }}</span>
    </div>

    <div v-if="notFound" class="card empty">
      <svg viewBox="0 0 24 24" width="40" height="40"><path d="M5 4h11l3 3v13H5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M8 11h8M8 15h8M8 19h5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
      <span>未找到该条日志</span>
    </div>

    <div v-else-if="loading && !log" class="card skeleton-card">
      <span class="skeleton" style="width: 40%"></span>
      <span class="skeleton" style="width: 70%"></span>
      <span class="skeleton" style="width: 55%"></span>
    </div>

    <template v-else-if="log">
      <!-- 概览 -->
      <section class="card overview">
        <div class="ov-main">
          <span class="ov-ico" :class="log.status">
            <svg v-if="log.status === 'success'" viewBox="0 0 24 24" width="20" height="20"><path d="M5 13l4 4 10-11" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/></svg>
            <svg v-else viewBox="0 0 24 24" width="20" height="20"><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="2"/><path d="M12 7v6M12 16v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
          </span>
          <div class="ov-text">
            <div class="ov-badges">
              <span class="badge" :class="log.status">{{ statusLabel(log.status) }}</span>
              <span class="badge" :class="log.trigger_type === 'manual' ? 'manual' : 'cron'">{{ triggerLabel }}</span>
            </div>
            <div class="ov-task mono">{{ log.task_type }}</div>
          </div>
        </div>
        <div class="ov-metrics">
          <div class="metric"><span class="m-k">协议</span><span class="m-v">{{ protocolTag(log.protocol) }}</span></div>
          <div class="metric"><span class="m-k">耗时</span><span class="m-v mono">{{ dur(log.execution_duration_ms) }}</span></div>
          <div class="metric"><span class="m-k">重试</span><span class="m-v">{{ log.retry_count }}</span></div>
          <div class="metric"><span class="m-k">执行时间</span><span class="m-v mono">{{ fmt(log.start_at) }}</span></div>
        </div>
      </section>

      <!-- 详情网格 -->
      <section class="card detail-grid">
        <div class="kv">
          <span class="k">目标地址</span>
          <span class="v mono ellipsis" :title="log.target_endpoint">{{ log.target_endpoint || '—' }}</span>
        </div>
        <div class="kv">
          <span class="k">开始时间</span>
          <span class="v mono">{{ fmt(log.start_at) }}</span>
        </div>
        <div class="kv">
          <span class="k">结束时间</span>
          <span class="v mono">{{ fmt(log.end_at) }}</span>
        </div>
        <div class="kv">
          <span class="k">记录时间</span>
          <span class="v mono">{{ fmt(log.created_at) }}</span>
        </div>
      </section>

      <!-- 错误信息 -->
      <section v-if="log.error_msg" class="card err-block">
        <div class="block-title">
          <span class="bt-ico danger">
            <svg viewBox="0 0 20 20" width="13" height="13"><circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="1.8"/><path d="M10 6v5M10 14v.5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>
          </span>
          错误信息
        </div>
        <pre class="err-pre">{{ log.error_msg }}</pre>
      </section>

      <!-- 响应体 -->
      <section class="card resp-block">
        <div class="block-title">
          <span class="bt-ico info">
            <svg viewBox="0 0 20 20" width="13" height="13"><path d="M5 4h11l3 3v13H5z" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="M8 11h8M8 15h8M8 19h5" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>
          </span>
          响应内容
        </div>
        <pre v-if="prettyBody" class="resp-pre">{{ prettyBody }}</pre>
        <div v-else class="muted empty-body">无响应内容</div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.page {
  width: 100%;
}
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}
.ph-left {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}
.ghost-back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 12px;
  border: 1px solid var(--border-d);
  background: var(--panel);
  color: var(--text-2);
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 500;
  transition: all 0.15s ease;
}
.ghost-back:hover {
  border-color: var(--primary);
  color: var(--primary);
}
.ph-title-wrap {
  min-width: 0;
}
.ph-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--text);
}
.ph-sub {
  font-size: 12.5px;
  color: var(--muted);
  margin-top: 2px;
}
.card.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 56px 0;
  color: var(--muted);
}
.skeleton-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px;
}
.skeleton {
  display: block;
  height: 16px;
  border-radius: 6px;
  background: linear-gradient(90deg, #eef0f6 25%, #e3e6f0 37%, #eef0f6 63%);
  background-size: 400% 100%;
  animation: shimmer 1.4s ease infinite;
}
@keyframes shimmer {
  0% { background-position: 100% 0; }
  100% { background-position: -100% 0; }
}
.overview {
  padding: 20px 22px;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
}
.ov-main {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}
.ov-ico {
  width: 46px;
  height: 46px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.ov-ico.success {
  background: var(--success-l);
  color: var(--success);
}
.ov-ico.failed,
.ov-ico.skipped {
  background: var(--danger-l);
  color: var(--danger);
}
.ov-text {
  min-width: 0;
}
.ov-badges {
  display: flex;
  gap: 8px;
  margin-bottom: 6px;
}
.ov-task {
  font-size: 15px;
  font-weight: 600;
  color: var(--text);
}
.ov-metrics {
  display: flex;
  gap: 28px;
  flex-wrap: wrap;
}
.metric {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.m-k {
  font-size: 12px;
  color: var(--muted);
}
.m-v {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
}
.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  background: var(--border);
  overflow: hidden;
  margin-bottom: 16px;
}
@media (max-width: 640px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }
  .overview {
    flex-direction: column;
    align-items: flex-start;
  }
}
.kv {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 18px;
  background: var(--panel);
}
.kv .k {
  font-size: 12px;
  color: var(--muted);
}
.kv .v {
  font-size: 13px;
  color: var(--text);
}
.ellipsis {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.block-title {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-2);
  margin-bottom: 12px;
}
.bt-ico {
  width: 22px;
  height: 22px;
  border-radius: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.bt-ico.danger {
  background: var(--danger-l);
  color: var(--danger);
}
.bt-ico.info {
  background: var(--info-l);
  color: var(--info);
}
.err-block {
  padding: 16px 18px;
  margin-bottom: 16px;
  border-color: #f7c9d5;
}
.err-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--mono);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--danger);
  background: var(--danger-l);
  border-radius: var(--radius-sm);
  padding: 12px 14px;
}
.resp-block {
  padding: 16px 18px;
}
.resp-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--mono);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--text);
  background: var(--panel-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 14px 16px;
  max-height: 520px;
  overflow: auto;
}
.empty-body {
  padding: 14px 0;
}
</style>
