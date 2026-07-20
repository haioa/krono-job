<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'

import { getStatsOverview, getDailyStats, getTaskRanking } from '@/api'
import { ApiError } from '@/api/http'
import type { StatsOverview, DailyStat, TaskRank } from '@/api/types'
import EChart from '@/components/EChart.vue'

// ---------- 状态 ----------
const overview = ref<StatsOverview | null>(null)
const daily = ref<DailyStat[]>([])
const ranking = ref<TaskRank[]>([])
const loading = ref(false)
const error = ref('')

const range = reactive({
  start: '',
  end: '',
})

// 当前选中的快捷区间标识，用于高亮；手动改日期时为 null。
const activePreset = ref<'7' | '30' | 'all' | null>(null)

// 近 N 天快捷区间（含今天）。
function setLastDays(n: number) {
  const end = new Date()
  const start = new Date()
  start.setDate(end.getDate() - (n - 1))
  range.end = fmtDate(end)
  range.start = fmtDate(start)
  activePreset.value = String(n) as '7' | '30'
  load()
}

function clearRange() {
  range.start = ''
  range.end = ''
  activePreset.value = 'all'
  load()
}

// 手动修改日期输入框时，取消快捷区间高亮。
function onManualDate() {
  activePreset.value = null
}

function fmtDate(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

// ---------- 数据加载 ----------
async function load() {
  loading.value = true
  error.value = ''
  try {
    const q = {
      start: range.start || undefined,
      end: range.end || undefined,
    }
    const [ov, dy, rk] = await Promise.all([
      getStatsOverview(q),
      getDailyStats(q),
      getTaskRanking({ ...q, limit: 10 }),
    ])
    overview.value = ov
    daily.value = fillDailyGaps(dy.list, range.start, range.end)
    ranking.value = rk.list
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载统计数据失败'
  } finally {
    loading.value = false
  }
}

// 按所选时间范围补齐缺失日期，保证折线图连续（无数据的日期填 0）。
function fillDailyGaps(list: DailyStat[], start: string, end: string): DailyStat[] {
  if (list.length === 0) return []
  let from = start
  let to = end
  if (!from || !to) {
    from = list[0]?.day ?? ''
    to = list[list.length - 1]?.day ?? ''
  }
  const cur = new Date(from + 'T00:00:00')
  const last = new Date(to + 'T00:00:00')
  if (isNaN(cur.getTime()) || isNaN(last.getTime()) || cur > last) return list

  const byDay = new Map(list.map((d) => [d.day, d]))
  const out: DailyStat[] = []
  while (cur <= last) {
    const key = fmtDate(cur)
    out.push(byDay.get(key) ?? { day: key, total: 0, success: 0, failed: 0, skipped: 0 })
    cur.setDate(cur.getDate() + 1)
  }
  return out
}

// ---------- 卡片数据 ----------
const cards = computed(() => {
  const o = overview.value
  return [
    { key: 'total', label: '总执行数', value: o?.total ?? 0, cls: 'total', icon: 'M4 6h16M4 12h16M4 18h10' },
    { key: 'success', label: '成功', value: o?.success ?? 0, cls: 'success', icon: 'M5 13l4 4L19 7' },
    { key: 'failed', label: '失败', value: o?.failed ?? 0, cls: 'failed', icon: 'M6 6l12 12M18 6L6 18' },
    { key: 'skipped', label: '跳过', value: o?.skipped ?? 0, cls: 'skipped', icon: 'M8 8l8 8M16 8l-8 8' },
  ]
})

// ---------- 每日趋势图 ----------
const dailyOption = computed(() => {
  const days = daily.value.map((d) => d.day)
  const mk = (name: string, key: keyof DailyStat, color: string) => ({
    name,
    type: 'line',
    smooth: true,
    showSymbol: false,
    itemStyle: { color },
    areaStyle: { color, opacity: 0.12 },
    data: daily.value.map((d) => (d[key] as number)),
  })
  return {
    tooltip: { trigger: 'axis' },
    legend: { data: ['成功', '失败', '跳过'], top: 0 },
    grid: { left: 48, right: 20, top: 40, bottom: 32 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: days,
      axisLine: { lineStyle: { color: '#d7dae8' } },
      axisLabel: { color: '#8a91a6' },
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: '#eef0f6' } },
      axisLabel: { color: '#8a91a6' },
    },
    series: [
      mk('成功', 'success', '#16a34a'),
      mk('失败', 'failed', '#e11d48'),
      mk('跳过', 'skipped', '#8a91a6'),
    ],
  }
})

// ---------- 调用排行榜图 ----------
const rankingOption = computed(() => {
  // 倒序使调用最多的任务显示在顶部
  const rows = [...ranking.value].reverse()
  const names = rows.map((r) => r.name || r.task_type)
  const pick = (key: keyof TaskRank) => rows.map((r) => (r[key] as number))
  return {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    legend: { data: ['成功', '失败', '跳过'], top: 0 },
    grid: { left: 8, right: 24, top: 40, bottom: 12, containLabel: true },
    xAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: '#eef0f6' } },
      axisLabel: { color: '#8a91a6' },
    },
    yAxis: {
      type: 'category',
      data: names,
      axisLine: { lineStyle: { color: '#d7dae8' } },
      axisLabel: { color: '#4a5167' },
    },
    series: [
      { name: '成功', type: 'bar', stack: 't', data: pick('success'), itemStyle: { color: '#16a34a' }, barWidth: '58%' },
      { name: '失败', type: 'bar', stack: 't', data: pick('failed'), itemStyle: { color: '#e11d48' } },
      { name: '跳过', type: 'bar', stack: 't', data: pick('skipped'), itemStyle: { color: '#8a91a6' } },
    ],
  }
})

// 进入页面默认展示近 30 天，避免无时间范围时拉取全量数据。
onMounted(() => setLastDays(30))
</script>

<template>
  <div class="page">
    <!-- 概览卡片 -->
    <div class="cards">
      <div v-for="c in cards" :key="c.key" class="card stat-card" :class="c.cls">
        <div class="stat-ico">
          <svg viewBox="0 0 24 24" width="20" height="20"><path :d="c.icon" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg>
        </div>
        <div class="stat-meta">
          <div class="stat-label">{{ c.label }}</div>
          <div class="stat-value">{{ c.value.toLocaleString() }}</div>
        </div>
      </div>
    </div>

    <!-- 时间范围筛选 -->
    <div class="card toolbar">
      <div class="range">
        <div class="field-wrap">
          <svg class="fw-icon" viewBox="0 0 20 20" width="15" height="15"><rect x="3" y="4" width="14" height="13" rx="2" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="M3 8h14M7 2.5v3M13 2.5v3" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
          <input v-model="range.start" type="date" class="field has-icon" @change="onManualDate" />
        </div>
        <span class="range-sep">至</span>
        <div class="field-wrap">
          <svg class="fw-icon" viewBox="0 0 20 20" width="15" height="15"><rect x="3" y="4" width="14" height="13" rx="2" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="M3 8h14M7 2.5v3M13 2.5v3" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
          <input v-model="range.end" type="date" class="field has-icon" @change="onManualDate" />
        </div>
      </div>
      <div class="presets">
        <button class="btn btn-sm" :class="{ active: activePreset === '7' }" @click="setLastDays(7)">近 7 天</button>
        <button class="btn btn-sm" :class="{ active: activePreset === '30' }" @click="setLastDays(30)">近 30 天</button>
        <button class="btn btn-sm" :class="{ active: activePreset === 'all' }" @click="clearRange">全部</button>
      </div>
      <div class="toolbar-actions">
        <button class="btn" @click="load" title="刷新数据">
          <svg viewBox="0 0 24 24" width="15" height="15"><path d="M20 11a8 8 0 1 0-2 5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M20 5v6h-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
          刷新
        </button>
      </div>
    </div>

    <div v-if="error" class="alert error" style="margin-bottom: 16px">
      <svg viewBox="0 0 20 20" width="16" height="16"><circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2"/><path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
      <span>{{ error }}</span>
    </div>

    <!-- 每日趋势 -->
    <section class="card chart-card">
      <header class="cc-head">
        <h2 class="cc-title">每日成功 / 失败 / 跳过趋势</h2>
        <span class="cc-sub" v-if="range.start || range.end">
          {{ range.start || '最早' }} ~ {{ range.end || '今天' }}
        </span>
      </header>
      <div v-if="loading && daily.length === 0" class="chart-skeleton"></div>
      <EChart v-else :option="dailyOption" height="360px" />
    </section>

    <!-- 调用排行榜 -->
    <section class="card chart-card">
      <header class="cc-head">
        <h2 class="cc-title">定时任务调用排行榜</h2>
        <span class="cc-sub">按调用次数降序（Top 10）</span>
      </header>
      <div v-if="loading && ranking.length === 0" class="chart-skeleton"></div>
      <EChart v-else-if="ranking.length" :option="rankingOption" height="420px" />
      <div v-else class="empty">
        <svg viewBox="0 0 24 24" width="40" height="40"><path d="M5 4h11l3 3v13H5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M8 11h8M8 15h8M8 19h5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
        <span>暂无调用记录</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.page {
  width: 100%;
}

/* 概览卡片 */
.cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}
.stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px;
}
.stat-ico {
  width: 46px;
  height: 46px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.stat-label {
  font-size: 13px;
  color: var(--muted);
  margin-bottom: 4px;
}
.stat-value {
  font-size: 26px;
  font-weight: 800;
  line-height: 1;
  color: var(--text);
}
.stat-card.total .stat-ico {
  background: var(--primary-l);
  color: var(--primary);
}
.stat-card.success .stat-ico {
  background: var(--success-l);
  color: var(--success);
}
.stat-card.failed .stat-ico {
  background: var(--danger-l);
  color: var(--danger);
}
.stat-card.skipped .stat-ico {
  background: #eef0f6;
  color: var(--muted);
}

/* 工具栏 */
.toolbar {
  padding: 16px 20px;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 14px 18px;
  flex-wrap: wrap;
}
.range {
  display: flex;
  align-items: center;
  gap: 10px;
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
}
.field.has-icon {
  padding-left: 34px;
  width: 160px;
}
.range-sep {
  color: var(--muted);
  font-size: 12px;
}
.presets {
  display: flex;
  gap: 8px;
}
.btn-sm.active {
  background: var(--primary);
  border-color: var(--primary);
  color: #fff;
}
.toolbar-actions {
  margin-left: auto;
}

/* 图表卡片 */
.chart-card {
  padding: 18px 20px 22px;
  margin-bottom: 16px;
}
.cc-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.cc-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text);
}
.cc-sub {
  font-size: 12.5px;
  color: var(--muted);
}
.chart-skeleton {
  height: 360px;
  border-radius: var(--radius-sm);
  background: linear-gradient(90deg, #eef0f6 25%, #e3e6f0 37%, #eef0f6 63%);
  background-size: 400% 100%;
  animation: shimmer 1.4s ease infinite;
}

@media (max-width: 860px) {
  .cards {
    grid-template-columns: repeat(2, 1fr);
  }
}
@media (max-width: 520px) {
  .cards {
    grid-template-columns: 1fr;
  }
  .toolbar-actions {
    margin-left: 0;
    width: 100%;
  }
  .toolbar-actions .btn {
    flex: 1;
  }
}
</style>
