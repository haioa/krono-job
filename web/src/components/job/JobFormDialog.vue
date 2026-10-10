<script setup lang="ts">
import { reactive, ref, watch, computed } from 'vue'
import type { JobDef } from '@/api/types'
import { createJob, updateJob } from '@/api'
import { ApiError } from '@/api/http'
import CronPicker from './CronPicker.vue'
import { genTaskId } from '@/utils/id'

const props = defineProps<{
  open: boolean
  job: JobDef | null
}>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const isEdit = computed(() => !!props.job)
const saving = ref(false)

// ---------- 表单状态（业务语义字段） ----------
const f = reactive({
  name: '', // 任务名称（给人看的）
  taskId: '', // 任务标识（英文ID，后端 task_type）
  freqType: 'custom', // 频率方式：minute / hour / day / week / custom
  cron: '', // 自定义 cron
  callType: 'http', // 调用方式：http / grpc
  enabled: true, // 是否启用
  timeoutSec: 30, // 超时秒数（人话）
  retry: 3, // 失败重试次数
  multiNode: false, // 是否多台服务器
  endpoint: '', // 单节点地址
  endpoints: [] as string[], // 多节点地址
  lb: 'round_robin', // 负载均衡
  method: 'POST', // HTTP 方法
  grpcService: '',
  grpcMethod: '',
  requestType: '',
  comment: '', // 备注
})

// 键值对行（可视化编辑，避免小白写 JSON）
interface KV {
  key: string
  value: string
}
const headerRows = ref<KV[]>([])
const payloadRows = ref<KV[]>([])
const metadataRows = ref<KV[]>([])
const endpointRows = ref<KV[]>([]) // 多节点地址行（用 key 列存地址）

const errors = reactive<Record<string, string>>({})

// ---------- 执行频率：模板与中文解释 ----------
const freqTemplates = [
  { id: 'minute', label: '每分钟', cron: '* * * * *' },
  { id: 'hour', label: '每小时整点', cron: '0 * * * *' },
  { id: 'day', label: '每天固定时刻', cron: '0 2 * * *' },
  { id: 'week', label: '每周一固定时刻', cron: '0 9 * * 1' },
  { id: 'custom', label: '自定义（高级）', cron: '' },
]

// 把单个 cron 字段解析成中文描述片段
type Field =
  | { kind: 'every' }
  | { kind: 'step'; step: number }
  | { kind: 'list'; nums: number[] }
  | { kind: 'range'; nums: number[] }
  | { kind: 'raw'; raw: string }

function parseField(seg: string, min: number, max: number): Field {
  if (seg === '*') return { kind: 'every' }
  if (seg.startsWith('*/')) {
    const step = Number(seg.slice(2))
    return { kind: 'step', step: step > 0 ? step : 1 }
  }
  // 列表或区间：拆成逗号分隔的每一项，支持 a-b / a-b/c
  const parts = seg.split(',').map((s) => s.trim()).filter(Boolean)
  if (parts.length && parts.every((s) => /^(\d+)(-\d+)?(\/\d+)?$/.test(s))) {
    const nums: number[] = []
    let rangeHit = false
    for (const part of parts) {
      const [baseRaw, stepStr] = part.split('/')
      const base = baseRaw ?? ''
      const step = stepStr ? Number(stepStr) : 1
      if (base.includes('-')) {
        const [aRaw, bRaw] = base.split('-')
        const a = Number(aRaw)
        const b = Number(bRaw)
        for (let n = a; n <= b; n += step) nums.push(n)
        rangeHit = true
      } else {
        nums.push(Number(base))
      }
    }
    nums.sort((a, b) => a - b)
    if (rangeHit) return { kind: 'range', nums }
    return { kind: 'list', nums }
  }
  return { kind: 'raw', raw: seg }
}

// 单位中文
const unitText = {
  minute: '分钟',
  hour: '小时',
  day: '日',
  month: '月',
  week: '星期',
}
const weekNames = ['日', '一', '二', '三', '四', '五', '六']

// 把「小时/分」转成 HH:mm
function pad2(n: number) {
  return n < 10 ? '0' + n : '' + n
}

function timeText(hh: number, mi: number) {
  return `${pad2(hh)}:${pad2(mi)}`
}

function cronExplain(cron: string): string {
  if (!cron) return ''
  const p = cron.trim().split(/\s+/)
  if (p.length !== 5) return '（格式：分 时 日 月 周，用空格分隔）'
  const [miS, hhS, ddS, moS, dwS] = p as [string, string, string, string, string]

  const mi = parseField(miS, 0, 59)
  const hh = parseField(hhS, 0, 23)
  const dd = parseField(ddS, 1, 31)
  const mo = parseField(moS, 1, 12)
  const dw = parseField(dwS, 0, 7)

  // 兜底：任意字段无法解析则给出原始表达
  const rawHit = [mi, hh, dd, mo, dw].some((x) => x.kind === 'raw')
  if (rawHit) return `按 cron「${cron}」执行（分 时 日 月 周）`

  // 时间描述（时 + 分）
  // 取首个被指定的分钟/小时用于「X点Y分」
  const firstMi = (mi.kind === 'list' || mi.kind === 'range') && mi.nums.length ? (mi.nums[0] ?? 0) : 0
  const firstHh = (hh.kind === 'list' || hh.kind === 'range') && hh.nums.length ? (hh.nums[0] ?? 0) : 0

  // 频率副词
  let freq = ''
  if (mi.kind === 'step' || hh.kind === 'step' || dd.kind === 'step' || mo.kind === 'step' || dw.kind === 'step') {
    // 找到哪个维度是 step
    const stepDim = [mi, hh, dd, mo, dw].find((x) => x.kind === 'step')
    const step = stepDim && 'step' in stepDim ? stepDim.step : 1
    // 定位 step 的单位
    if (mi.kind === 'step') freq = `每隔 ${step} 分钟`
    else if (hh.kind === 'step' && mi.kind === 'every') freq = `每隔 ${step} 小时整点`
    else if (hh.kind === 'step') freq = `每隔 ${step} 小时（${timeText(firstHh, firstMi)} 起）`
    else if (dd.kind === 'step') freq = `每隔 ${step} 天`
    else if (mo.kind === 'step') freq = `每隔 ${step} 个月`
    else if (dw.kind === 'step') freq = `每隔 ${step} 周`
  }

  // 日 / 月 / 周 限定
  const when: string[] = []
  if (dd.kind === 'list') when.push(`每月 ${dd.nums.join('、')} 日`)
  else if (dd.kind === 'range') when.push(`每月 ${dd.nums[0]!}~${dd.nums[dd.nums.length - 1]!} 日`)
  if (mo.kind === 'list') when.push(`${mo.nums.map((n) => n + '月').join('、')}`)
  else if (mo.kind === 'range') when.push(`${mo.nums[0]!}~${mo.nums[mo.nums.length - 1]!}月`)
  if (dw.kind === 'list') when.push(`每周${dw.nums.map((n) => weekNames[n % 7]).join('、')}`)
  else if (dw.kind === 'range') when.push(`每周${weekNames[dw.nums[0]! % 7]}~${weekNames[dw.nums[dw.nums.length - 1]! % 7]}`)

  // 组合常见场景
  // 0. 每隔 N（步进）：优先处理，避免具体分支把 step 字段当成「固定值」
  if (freq) {
    const t = timeText(firstHh, firstMi)
    const timePart = mi.kind !== 'every' && mi.kind !== 'step' && hh.kind !== 'every' && hh.kind !== 'step' ? ` ${t}` : ''
    const whenPart = when.length ? `（${when.join('，')}）` : ''
    return `${freq}${timePart} 执行${whenPart}`
  }
  // 1. 每分钟
  if (mi.kind === 'every' && hh.kind === 'every' && dd.kind === 'every' && mo.kind === 'every' && dw.kind === 'every')
    return '每分钟执行一次'
  // 2. 每小时整点（分固定，时 every）
  if (mi.kind !== 'every' && hh.kind === 'every' && dd.kind === 'every' && mo.kind === 'every' && dw.kind === 'every') {
    const mins = mi.kind === 'list' ? mi.nums.join('、') + ' 分' : mi.kind === 'range' ? `${mi.nums[0]!}~${mi.nums[mi.nums.length - 1]!} 分` : ''
    return `每小时的 ${mins}执行`
  }
  // 3. 每天固定时刻（分固定、时固定、日月周 every）
  if (mi.kind !== 'every' && hh.kind !== 'every' && dd.kind === 'every' && mo.kind === 'every' && dw.kind === 'every') {
    const t = timeText(firstHh, firstMi)
    const extra = mi.kind === 'list' ? `（${mi.nums.map((n) => pad2(n)).join('、')} 分）` : mi.kind === 'range' ? `（${mi.nums[0]!}~${mi.nums[mi.nums.length - 1]!} 分）` : ''
    return `每天 ${t}${extra} 执行`
  }
  // 4. 每周固定时刻（周指定）
  if (dw.kind !== 'every' && dd.kind === 'every' && mo.kind === 'every') {
    const t = timeText(firstHh, firstMi)
    return `每周${dw.kind === 'list' ? dw.nums.map((n) => weekNames[n % 7]).join('、') : ''} ${t} 执行`
  }
  // 5. 每月固定日时刻
  if (dd.kind !== 'every' && mo.kind === 'every' && dw.kind === 'every') {
    const t = timeText(firstHh, firstMi)
    const days = dd.kind === 'list' ? dd.nums.join('、') + ' 日' : dd.kind === 'range' ? `${dd.nums[0]!}~${dd.nums[dd.nums.length - 1]!} 日` : ''
    return `每月 ${days} ${t} 执行`
  }
  // 7. 其他：组合 when + 时间
  const t = timeText(firstHh, firstMi)
  const head = when.length ? when.join('，') + ' ' : ''
  return `${head}${t} 执行`
}

const cronPreview = computed(() => {
  if (f.freqType === 'custom') return cronExplain(f.cron)
  const t = freqTemplates.find((x) => x.id === f.freqType)
  return t ? cronExplain(t.cron) : ''
})

function onFreqPick(id: string) {
  f.freqType = id
  const t = freqTemplates.find((x) => x.id === id)
  if (t) f.cron = t.cron
}

const httpMethods = ['GET', 'POST', 'PUT', 'DELETE']

// ---------- 工具：行 <-> 对象 ----------
function rowsToObj(rows: KV[]): Record<string, string> {
  const o: Record<string, string> = {}
  for (const r of rows) {
    const k = r.key.trim()
    if (k) o[k] = r.value
  }
  return o
}
function objToRows(obj?: Record<string, string>): KV[] {
  if (!obj) return []
  const arr = Object.entries(obj).map(([k, v]) => ({ key: k, value: String(v) }))
  return arr.length ? arr : [{ key: '', value: '' }]
}
function addRow(rows: KV[]) {
  rows.push({ key: '', value: '' })
}
function delRow(rows: KV[], i: number) {
  rows.splice(i, 1)
  if (rows.length === 0) rows.push({ key: '', value: '' })
}
function parseEndpoints(): string[] {
  return endpointRows.value.map((r) => r.key.trim()).filter(Boolean)
}

// ---------- 初始化 / 回填 ----------
function resetForm() {
  f.name = ''
  f.taskId = ''
  f.freqType = 'custom'
  f.cron = ''
  f.callType = 'http'
  f.enabled = true
  f.timeoutSec = 30
  f.retry = 3
  f.multiNode = false
  f.endpoint = ''
  f.endpoints = []
  f.lb = 'round_robin'
  f.method = 'POST'
  f.grpcService = ''
  f.grpcMethod = ''
  f.requestType = ''
  f.comment = ''
  headerRows.value = [{ key: '', value: '' }]
  payloadRows.value = [{ key: '', value: '' }]
  metadataRows.value = [{ key: '', value: '' }]
  endpointRows.value = [{ key: '', value: '' }]
  Object.keys(errors).forEach((k) => delete errors[k])
}

function syncFromProp() {
  resetForm()
  const j = props.job
  // 新增场景：自动预填一个不重复的任务标识（UUID + 时间戳）
  if (!j) {
    f.taskId = genTaskId()
    return
  }
  f.name = j.name || ''
  f.taskId = j.task_type || ''
  f.cron = j.cron || ''
  const hit = freqTemplates.find((t) => t.cron && t.cron === j.cron)
  f.freqType = hit ? hit.id : 'custom'
  f.callType = j.protocol === 'grpc' ? 'grpc' : 'http'
  f.enabled = j.enabled === null || j.enabled === undefined ? true : j.enabled
  const m = (j.timeout || '').match(/^(\d+)\s*s/)
  f.timeoutSec = m ? Number(m[1]) : 30
  f.retry = j.retry || 0
  f.multiNode = !!(j.endpoints && j.endpoints.length)
  f.endpoint = j.endpoint || ''
  f.endpoints = j.endpoints || []
  endpointRows.value = (j.endpoints && j.endpoints.length ? j.endpoints : [j.endpoint]).filter(Boolean).map((e) => ({ key: e, value: '' }))
  if (endpointRows.value.length === 0) endpointRows.value = [{ key: '', value: '' }]
  f.lb = j.lb_strategy || 'round_robin'
  f.method = j.method || 'POST'
  f.grpcService = j.grpc_service || ''
  f.grpcMethod = j.grpc_method || ''
  f.requestType = j.request_type || ''
  f.comment = j.comment || ''
  headerRows.value = objToRows(j.headers)
  payloadRows.value = objToRows(j.payload as Record<string, string>)
  metadataRows.value = objToRows(j.metadata)
}

watch(
  () => props.open,
  (v) => {
    if (v) syncFromProp()
  },
  { immediate: true },
)

// ---------- 校验 ----------
function validate(): boolean {
  Object.keys(errors).forEach((k) => delete errors[k])
  let ok = true
  if (!f.name.trim()) {
    errors.name = '请填写一个便于识别的任务名称'
    ok = false
  }
  if (!f.taskId.trim()) {
    errors.taskId = '请填写任务标识（英文、数字、下划线）'
    ok = false
  } else if (!/^[A-Za-z0-9_]+$/.test(f.taskId.trim())) {
    errors.taskId = '只能包含字母、数字和下划线，例如 order_sync'
    ok = false
  }
  const finalCron = f.freqType === 'custom' ? f.cron : freqTemplates.find((t) => t.id === f.freqType)?.cron || ''
  if (!finalCron.trim()) {
    errors.cron = '请选择或填写执行频率'
    ok = false
  }
  if (f.timeoutSec < 1) {
    errors.timeout = '超时时间至少 1 秒'
    ok = false
  }
  if (f.callType === 'http') {
    const ep = f.multiNode ? parseEndpoints() : f.endpoint.trim() ? [f.endpoint.trim()] : []
    if (ep.length === 0) {
      errors.endpoint = '请填写服务地址'
      ok = false
    }
  } else {
    if (!f.grpcService.trim()) {
      errors.grpc = '请填写 gRPC 服务名'
      ok = false
    }
    const ep = f.multiNode ? parseEndpoints() : f.endpoint.trim() ? [f.endpoint.trim()] : []
    if (ep.length === 0) {
      errors.endpoint = '请填写 gRPC 服务地址（host:port）'
      ok = false
    }
  }
  return ok
}

async function submit() {
  if (!validate()) return
  saving.value = true
  try {
    const finalCron = f.freqType === 'custom' ? f.cron.trim() : freqTemplates.find((t) => t.id === f.freqType)?.cron || ''
    const endpoints = f.multiNode ? parseEndpoints() : []
    const singleEndpoint = f.multiNode ? '' : f.endpoint.trim()

    const payload2: JobDef = {
      name: f.name.trim(),
      task_type: f.taskId.trim(),
      cron: finalCron,
      protocol: f.callType,
      enabled: f.enabled,
      timeout: `${f.timeoutSec}s`,
      retry: f.retry,
      endpoint: singleEndpoint,
      endpoints,
      lb_strategy: f.multiNode ? f.lb : '',
      method: f.callType === 'http' ? f.method : '',
      headers: rowsToObj(headerRows.value),
      payload: rowsToObj(payloadRows.value) as Record<string, any>,
      grpc_service: f.callType === 'grpc' ? f.grpcService.trim() : '',
      grpc_method: f.callType === 'grpc' ? f.grpcMethod.trim() : '',
      request_type: f.callType === 'grpc' ? f.requestType.trim() : '',
      metadata: f.callType === 'grpc' ? rowsToObj(metadataRows.value) : {},
      comment: f.comment.trim(),
    }

    if (isEdit.value) {
      // 用表单当前 taskId 作为路径参数：未改则按原标识更新，改了则按新标识更新（后端只校验一致性）。
      await updateJob(f.taskId.trim(), payload2)
    } else {
      await createJob(payload2)
    }
    emit('saved')
    emit('close')
  } catch (e: any) {
    errors._form = e instanceof ApiError ? e.message : e?.message || '保存失败，请检查后重试'
    if (e instanceof SyntaxError) errors._form = '参数格式有误'
  } finally {
    saving.value = false
  }
}

function close() {
  emit('close')
}
</script>

<template>
  <div v-if="open" class="modal-mask" @click.self="close">
    <div class="modal">
      <header class="modal-head">
        <div>
          <h3>{{ isEdit ? '编辑定时任务' : '新建定时任务' }}</h3>
          <p class="sub">填写以下信息即可，保存后系统会自动按设定频率执行，无需重启服务。</p>
        </div>
        <button class="icon-btn" @click="close" aria-label="关闭">×</button>
      </header>

      <div class="modal-body">
        <div v-if="errors._form" class="alert error">
          <svg viewBox="0 0 20 20" width="15" height="15"><circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" stroke-width="2"/><path d="M10 6v5M10 14.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
          <span>{{ errors._form }}</span>
        </div>

        <!-- 第 1 步 -->
        <section class="block">
          <div class="block-title"><span class="num">1</span> 任务基本信息</div>

          <div class="fld">
            <label class="lbl">任务名称 <em>*</em></label>
            <input class="field" v-model="f.name" placeholder="例如：每日订单对账" @input="errors.name = ''" />
            <p v-if="errors.name" class="err">{{ errors.name }}</p>
            <p v-else class="tip">给任务起个好记的名字，方便在列表里一眼认出。</p>
          </div>

          <div class="fld">
            <label class="lbl">任务标识（英文ID）<em>*</em></label>
            <input class="field" v-model="f.taskId" placeholder="例如：order_reconcile" @input="errors.taskId = ''" />
            <p v-if="errors.taskId" class="err">{{ errors.taskId }}</p>
            <p v-else class="tip">系统内部使用的唯一编号，只能含字母、数字、下划线；新增时会自动生成，可手动改写。</p>
          </div>

          <div class="fld inline">
            <label class="lbl" style="margin:0">是否启用</label>
            <div class="switch-row">
              <label class="switch">
                <input type="checkbox" v-model="f.enabled" />
                <span class="track"></span>
              </label>
              <span class="switch-text">{{ f.enabled ? '启用（到时间会执行）' : '停用（暂不执行，但保留配置）' }}</span>
            </div>
          </div>
        </section>

        <!-- 第 2 步 -->
        <section class="block">
          <div class="block-title"><span class="num">2</span> 执行频率</div>

          <div class="freq-grid">
            <button
              v-for="t in freqTemplates"
              :key="t.id"
              type="button"
              class="freq-card"
              :class="{ active: f.freqType === t.id }"
              @click="onFreqPick(t.id)"
            >
              <span class="freq-label">{{ t.label }}</span>
              <span v-if="t.cron" class="freq-cron">{{ t.cron }}</span>
            </button>
          </div>

          <div v-if="f.freqType === 'custom'" class="fld" style="margin-top: 12px">
            <label class="lbl">自定义 cron 表达式 <em>*</em></label>
            <CronPicker v-model="f.cron" />
            <p v-if="errors.cron" class="err" style="margin-top:6px">{{ errors.cron }}</p>
          </div>

          <p class="preview" :class="{ bad: !cronPreview }">
            <svg viewBox="0 0 24 24" width="15" height="15"><path d="M12 6v6l4 2" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="2"/></svg>
            {{ cronPreview || '请选择上面的执行频率' }}
          </p>
          <p class="tip">提示：cron 五个字段依次是「分 时 日 月 周」。不确定就先用上面的常用模板。</p>
        </section>

        <!-- 第 3 步 -->
        <section class="block">
          <div class="block-title"><span class="num">3</span> 要调用什么</div>

          <div class="seg">
            <button type="button" class="seg-btn" :class="{ active: f.callType === 'http' }" @click="f.callType = 'http'">HTTP 接口</button>
            <button type="button" class="seg-btn" :class="{ active: f.callType === 'grpc' }" @click="f.callType = 'grpc'">gRPC 服务</button>
          </div>
          <p class="tip">大多数情况选「HTTP 接口」即可；只有对方明确是 gRPC 服务时才选后者。</p>

          <!-- HTTP -->
          <template v-if="f.callType === 'http'">
            <div class="fld inline">
              <label class="lbl" style="margin:0">请求方法</label>
              <select class="field" v-model="f.method" style="max-width:160px">
                <option v-for="m in httpMethods" :key="m" :value="m">{{ m }}</option>
              </select>
            </div>

            <div class="fld">
              <label class="lbl">服务地址</label>
              <label class="chk">
                <input type="checkbox" v-model="f.multiNode" style="width:auto" />
                <span>有多台服务器（负载均衡）</span>
              </label>

              <template v-if="!f.multiNode">
                <input class="field" v-model="f.endpoint" placeholder="例如 10.0.0.1:8080 或 https://api.xxx.com/hook" @input="errors.endpoint = ''" />
                <p v-if="errors.endpoint" class="err">{{ errors.endpoint }}</p>
                <p v-else class="tip">填一台服务器的地址即可。</p>
              </template>

              <template v-else>
                <div class="rows">
                  <div v-for="(r, i) in endpointRows" :key="i" class="row">
                    <input class="field" v-model="r.key" placeholder="例如 10.0.0.1:8080" @input="errors.endpoint = ''" />
                    <button type="button" class="btn btn-sm btn-ghost danger" @click="delRow(endpointRows, i)">移除</button>
                  </div>
                </div>
                <button type="button" class="btn btn-sm btn-ghost" @click="addRow(endpointRows)">+ 再添加一台服务器</button>
                <p v-if="errors.endpoint" class="err" style="margin-top:6px">{{ errors.endpoint }}</p>
                <div class="fld inline" style="margin-top: 10px" v-if="f.multiNode">
                  <label class="lbl" style="margin:0">多台之间如何分配</label>
                  <select class="field" v-model="f.lb" style="max-width:200px">
                    <option value="round_robin">轮流（每台轮流来）</option>
                    <option value="least_conn">挑最闲的</option>
                    <option value="random">随机</option>
                    <option value="consistent">按内容固定</option>
                  </select>
                </div>
              </template>
            </div>

            <div class="fld">
              <label class="lbl">请求头（可选）</label>
              <div class="rows">
                <div v-for="(r, i) in headerRows" :key="i" class="row kv">
                  <input class="field" v-model="r.key" placeholder="名称，如 Authorization" />
                  <input class="field" v-model="r.value" placeholder="值，如 Bearer xxxx" />
                  <button type="button" class="btn btn-sm btn-ghost danger" @click="delRow(headerRows, i)">移除</button>
                </div>
              </div>
              <button type="button" class="btn btn-sm btn-ghost" @click="addRow(headerRows)">+ 添加请求头</button>
            </div>

            <div class="fld">
              <label class="lbl">请求参数（可选）</label>
              <div class="rows">
                <div v-for="(r, i) in payloadRows" :key="i" class="row kv">
                  <input class="field" v-model="r.key" placeholder="参数名，如 order_id" />
                  <input class="field" v-model="r.value" placeholder="参数值，如 123" />
                  <button type="button" class="btn btn-sm btn-ghost danger" @click="delRow(payloadRows, i)">移除</button>
                </div>
              </div>
              <button type="button" class="btn btn-sm btn-ghost" @click="addRow(payloadRows)">+ 添加参数</button>
              <p class="tip">需要传什么数据就填这里，例如 ID、日期等。留空表示不传参数。</p>
            </div>
          </template>

          <!-- gRPC -->
          <template v-else>
            <div class="fld">
              <label class="lbl">服务名 <em>*</em></label>
              <input class="field" v-model="f.grpcService" placeholder="例如 order.OrderService" @input="errors.grpc = ''" />
              <p v-if="errors.grpc" class="err">{{ errors.grpc }}</p>
              <p v-else class="tip">对方提供的 gRPC 服务全称。</p>
            </div>
            <div class="fld">
              <label class="lbl">方法名</label>
              <input class="field" v-model="f.grpcMethod" placeholder="例如 Create" />
            </div>
            <div class="fld">
              <label class="lbl">参数类型</label>
              <input class="field" v-model="f.requestType" placeholder="例如 order.CreateRequest" />
            </div>
            <div class="fld">
              <label class="lbl">服务地址 <em>*</em></label>
              <label class="chk">
                <input type="checkbox" v-model="f.multiNode" style="width:auto" />
                <span>有多台服务器（负载均衡）</span>
              </label>

              <template v-if="!f.multiNode">
                <input class="field" v-model="f.endpoint" placeholder="例如 sdc-grpc:10000 或 192.168.31.45:20001" @input="errors.endpoint = ''" />
                <p v-if="errors.endpoint" class="err">{{ errors.endpoint }}</p>
                <p v-else class="tip">填一台 gRPC 节点的 host:port 即可。</p>
              </template>

              <template v-else>
                <div class="rows">
                  <div v-for="(r, i) in endpointRows" :key="i" class="row">
                    <input class="field" v-model="r.key" placeholder="例如 192.168.31.45:20001" @input="errors.endpoint = ''" />
                    <button type="button" class="btn btn-sm btn-ghost danger" @click="delRow(endpointRows, i)">移除</button>
                  </div>
                </div>
                <button type="button" class="btn btn-sm btn-ghost" @click="addRow(endpointRows)">+ 再添加一台服务器</button>
                <p v-if="errors.endpoint" class="err" style="margin-top:6px">{{ errors.endpoint }}</p>
                <div class="fld inline" style="margin-top: 10px" v-if="f.multiNode">
                  <label class="lbl" style="margin:0">多台之间如何分配</label>
                  <select class="field" v-model="f.lb" style="max-width:200px">
                    <option value="round_robin">轮流（每台轮流来）</option>
                    <option value="least_conn">挑最闲的</option>
                    <option value="random">随机</option>
                    <option value="consistent">按内容固定</option>
                  </select>
                </div>
              </template>
            </div>
            <div class="fld">
              <label class="lbl">附加信息 metadata（可选）</label>
              <div class="rows">
                <div v-for="(r, i) in metadataRows" :key="i" class="row kv">
                  <input class="field" v-model="r.key" placeholder="名称" />
                  <input class="field" v-model="r.value" placeholder="值" />
                  <button type="button" class="btn btn-sm btn-ghost danger" @click="delRow(metadataRows, i)">移除</button>
                </div>
              </div>
              <button type="button" class="btn btn-sm btn-ghost" @click="addRow(metadataRows)">+ 添加 metadata</button>
            </div>
          </template>
        </section>

        <!-- 第 4 步 -->
        <section class="block">
          <div class="block-title"><span class="num">4</span> 高级设置（一般无需改）</div>
          <div class="grid2">
            <div class="fld inline">
              <label class="lbl" style="margin:0">超时（秒）</label>
              <input class="field" type="number" min="1" v-model.number="f.timeoutSec" style="max-width:110px" @input="errors.timeout = ''" />
              <p v-if="errors.timeout" class="err">{{ errors.timeout }}</p>
            </div>
            <div class="fld inline">
              <label class="lbl" style="margin:0">失败重试次数</label>
              <input class="field" type="number" min="0" v-model.number="f.retry" style="max-width:110px" />
              <p class="tip">0 表示不重试。</p>
            </div>
          </div>
          <div class="fld">
            <label class="lbl">备注（给自己看的）</label>
            <textarea class="field comment-area" v-model="f.comment" rows="5" placeholder="例如：该任务依赖前一天的对账数据，失败请手动重试。"></textarea>
            <p class="tip">这段说明会写进配置文件，方便以后维护时一目了然。</p>
          </div>
        </section>
      </div>

      <footer class="modal-foot">
        <button type="button" class="btn btn-ghost" @click="close">取消</button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="submit">
          {{ saving ? '保存中…' : '保存任务' }}
        </button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}
.modal {
  width: min(720px, 95vw);
  max-height: 94vh;
  background: var(--panel);
  border-radius: var(--radius);
  box-shadow: var(--shadow-md);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--border);
}
.modal-head h3 {
  margin: 0;
  font-size: 17px;
  color: var(--text);
}
.modal-head .sub {
  margin: 6px 0 0;
  font-size: 12.5px;
  color: var(--muted);
}
.icon-btn {
  border: none;
  background: transparent;
  font-size: 24px;
  line-height: 1;
  cursor: pointer;
  color: var(--muted);
}
.modal-body {
  padding: 18px 22px;
  overflow-y: auto;
}

.block {
  margin-bottom: 22px;
  padding-bottom: 18px;
  border-bottom: 1px dashed var(--border);
}
.block:last-child {
  border-bottom: none;
  margin-bottom: 0;
}
.block-title {
  display: flex;
  align-items: center;
  gap: 9px;
  font-size: 14px;
  font-weight: 700;
  color: var(--text);
  margin-bottom: 14px;
}
.block-title .num {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--primary);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
}

/* 字段容器（与全局 .field 输入框区分） */
.fld {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 14px;
}
.fld.inline {
  flex-direction: row;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

/* 标签/提示：复用全局 .lbl，这里仅补充内联变体 */
.fld.inline .lbl {
  min-width: 92px;
}
.tip {
  font-size: 12px;
  color: var(--muted);
  margin: 0;
  line-height: 1.5;
}
.err {
  font-size: 12px;
  color: var(--danger);
  margin: 0;
}

.freq-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 10px;
}
.freq-card {
  border: 1px solid var(--border-d);
  background: var(--panel);
  border-radius: var(--radius-sm);
  padding: 12px 10px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
  text-align: left;
  transition: all 0.15s ease;
}
.freq-card:hover {
  border-color: var(--primary);
}
.freq-card.active {
  border-color: var(--primary);
  background: var(--primary-l);
  box-shadow: 0 0 0 2px var(--primary-ring);
}
.freq-label {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text);
}
.freq-cron {
  font-family: var(--mono);
  font-size: 11.5px;
  color: var(--muted);
}
.freq-card.active .freq-cron {
  color: var(--primary-d);
}

.preview {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 12px 0 2px;
  padding: 10px 12px;
  background: var(--success-l);
  color: var(--success);
  border-radius: var(--radius-sm);
  font-size: 13px;
}
.preview.bad {
  background: var(--warn-l);
  color: var(--warn);
}

.seg {
  display: inline-flex;
  background: var(--panel-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 3px;
  gap: 3px;
  margin-bottom: 10px;
}
.seg-btn {
  border: none;
  background: transparent;
  padding: 7px 16px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  color: var(--text-2);
}
.seg-btn.active {
  background: var(--primary);
  color: #fff;
}

.chk {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  color: var(--text-2);
  cursor: pointer;
}

.rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 4px;
}
.row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.row.kv .field {
  flex: 1;
  min-width: 0;
}

.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

/* 开关文案 */
.switch-row {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.switch-text {
  font-size: 12.5px;
  color: var(--text-2);
}

/* 按钮补充：ghost / 危险态（基础 .btn 来自全局） */
.btn-ghost {
  background: var(--panel);
  color: var(--text);
  border: 1px solid var(--border-d);
}
.btn-ghost:hover {
  border-color: var(--primary);
  color: var(--primary);
}
.btn-sm.danger,
.btn-ghost.danger {
  color: var(--danger);
  border-color: #fca5a5;
}
.btn-sm.danger:hover,
.btn-ghost.danger:hover {
  background: var(--danger-l);
  border-color: var(--danger);
  color: var(--danger);
}

.modal-foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 22px;
  border-top: 1px solid var(--border);
}

/* 备注框：更高、可随内容自动撑开 */
.comment-area {
  resize: vertical;
  min-height: 120px;
  line-height: 1.6;
  font-family: inherit;
  padding: 10px 12px;
}

@media (max-width: 640px) {
  .grid2 {
    grid-template-columns: 1fr;
  }
}
</style>
