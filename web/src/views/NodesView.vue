<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { getClusterNodes, getClusterStatus } from '../api'
import type { ClusterNode, ClusterStatusResponse } from '../api/types'

const nodes = ref<ClusterNode[]>([])
const status = ref<ClusterStatusResponse | null>(null)
const loading = ref(false)
const error = ref('')
let timer: number | undefined

const roleText: Record<string, string> = {
  leader: '主节点 (Leader)',
  follower: '从节点 (Follower)',
  standalone: '单节点 (Standalone)',
}

function fmtTime(nano: number): string {
  if (!nano) return '-'
  // checked 为 UnixNano
  const d = new Date(Math.floor(nano / 1e6))
  return d.toLocaleTimeString('zh-CN', { hour12: false })
}

async function fetchData() {
  loading.value = true
  error.value = ''
  try {
    const [n, s] = await Promise.all([getClusterNodes(), getClusterStatus()])
    nodes.value = n.nodes
    status.value = s
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

function start() {
  fetchData()
  timer = window.setInterval(fetchData, 8000)
}
function stop() {
  if (timer) window.clearInterval(timer)
}

onMounted(start)
onUnmounted(stop)
</script>

<template>
  <div class="page">
    <h2 class="page-title">被调用方节点状态</h2>
    <p class="page-sub">
      下游服务存活由调用方主动探测（TCP / HTTP HEAD / gRPC Health），结果写入 Redis 共享，无需下游改造。
    </p>

    <div v-if="status" class="status-cards">
      <div class="card">
        <span class="card-label">运行模式</span>
        <span class="card-value">{{ status.distributed ? '分布式' : '单节点' }}</span>
      </div>
      <div class="card">
        <span class="card-label">本节点角色</span>
        <span class="card-value">{{ roleText[status.role] || status.role }}</span>
      </div>
      <div class="card">
        <span class="card-label">节点 ID</span>
        <span class="card-value mono">{{ status.node_id }}</span>
      </div>
      <div class="card">
        <span class="card-label">探测节点数</span>
        <span class="card-value">{{ nodes.length }}</span>
      </div>
    </div>

    <p v-if="!status?.distributed" class="hint">
      当前为单节点模式（cluster.enabled=false）。开启分布式模式后，这里会展示所有被调用方节点的存活状态。
    </p>

    <div v-if="error" class="error">{{ error }}</div>

    <table class="grid" v-if="nodes.length">
      <thead>
        <tr>
          <th>状态</th>
          <th>Endpoint</th>
          <th>协议</th>
          <th>最近探测</th>
          <th>探测来源</th>
          <th>错误信息</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="n in nodes" :key="n.endpoint">
          <td>
            <span :class="['badge', n.alive ? 'ok' : 'down']">
              {{ n.alive ? '存活' : '失联' }}
            </span>
          </td>
          <td class="mono">{{ n.endpoint }}</td>
          <td>{{ n.protocol }}</td>
          <td>{{ fmtTime(n.checked) }}</td>
          <td class="mono">{{ n.by || '-' }}</td>
          <td class="err">{{ n.error || '-' }}</td>
        </tr>
      </tbody>
    </table>

    <p v-else-if="!loading && status?.distributed" class="hint">
      暂无可探测的下游节点。请在 jobs.yaml 为任务配置 endpoints（多节点）后，探测结果会在此展示。
    </p>
  </div>
</template>

<style scoped>
.status-cards {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 10px 16px;
  min-width: 160px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.card-label { font-size: 12px; color: #6b7280; }
.card-value { font-size: 16px; font-weight: 600; color: #111827; }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; }
.grid { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; }
.grid th, .grid td { text-align: left; padding: 10px 12px; border-bottom: 1px solid #f0f0f0; font-size: 14px; }
.grid th { background: #fafafa; color: #6b7280; font-weight: 600; }
.badge { padding: 2px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; }
.badge.ok { background: #dcfce7; color: #166534; }
.badge.down { background: #fee2e2; color: #991b1b; }
.err { color: #b91c1c; font-size: 13px; }
.hint { color: #6b7280; font-size: 14px; }
.error { color: #b91c1c; }
.page-sub { color: #6b7280; font-size: 14px; margin-top: -4px; }
</style>
