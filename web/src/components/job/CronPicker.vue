<script setup lang="ts">
import { reactive, ref, watch, onMounted } from 'vue'
import { previewCron } from '@/api'
import { ApiError } from '@/api/http'

const props = defineProps<{ modelValue?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

interface FieldCfg {
  key: 'minute' | 'hour' | 'day' | 'month' | 'week'
  label: string
  min: number
  max: number
  stepPresets: number[] // 常用步长
}
const fields: FieldCfg[] = [
  { key: 'minute', label: '分钟', min: 0, max: 59, stepPresets: [1, 5, 10, 15, 30] },
  { key: 'hour', label: '小时', min: 0, max: 23, stepPresets: [1, 2, 3, 6, 12] },
  { key: 'day', label: '日', min: 1, max: 31, stepPresets: [1, 2, 5, 10, 15] },
  { key: 'month', label: '月', min: 1, max: 12, stepPresets: [1, 2, 3, 6] },
  { key: 'week', label: '周几', min: 0, max: 6, stepPresets: [1, 2] },
]
const weekNames = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

// 每个字段状态：mode=every(*)/step(*/N)/specific(列表)
interface FieldState {
  mode: 'every' | 'step' | 'specific'
  step: number
  values: number[]
}
const state = reactive<Record<FieldCfg['key'], FieldState>>({
  minute: { mode: 'every', step: 1, values: [] },
  hour: { mode: 'every', step: 1, values: [] },
  day: { mode: 'every', step: 1, values: [] },
  month: { mode: 'every', step: 1, values: [] },
  week: { mode: 'every', step: 1, values: [] },
})

const manualMode = ref(false) // 表达式含可视化无法表达的写法（区间等），以手动框为准
const manualValue = ref('') // 手动输入框内容
const preview = ref<string[]>([])
const tz = ref('')
const previewErr = ref('')
const previewing = ref(false)

function fieldToStr(s: FieldState, min: number, max: number): string {
  if (s.mode === 'every') return '*'
  if (s.mode === 'step') return `*/${s.step}`
  if (s.values.length === 0) return '*'
  const sorted = [...s.values].sort((a, b) => a - b)
  return sorted.join(',')
}

function build(): string {
  return fields.map((f) => fieldToStr(state[f.key], f.min, f.max)).join(' ')
}

// 当前生效的表达式：手动模式以手动框为准，否则以可视化状态为准
function currentExpr(): string {
  return manualMode.value ? manualValue.value.trim() : build()
}

function emitCron() {
  emit('update:modelValue', currentExpr())
  fetchPreview()
}

// 把表达式写回可视化点选状态；返回是否完全可视化
function parse(expr: string): boolean {
  const p = expr.trim().split(/\s+/)
  if (p.length !== 5) return false
  let allVisual = true
  fields.forEach((f, idx) => {
    const seg = p[idx]
    if (!seg) {
      allVisual = false
      return
    }
    const st = state[f.key]
    if (seg === '*') {
      st.mode = 'every'
    } else if (seg.startsWith('*/')) {
      st.mode = 'step'
      st.step = Number(seg.slice(2)) || 1
    } else if (seg.includes(',') || /^\d+$/.test(seg)) {
      st.mode = 'specific'
      st.values = seg
        .split(',')
        .map((x) => Number(x))
        .filter((n) => !Number.isNaN(n))
    } else {
      // 区间/复杂写法：可视化无法表达，标记手动模式
      allVisual = false
    }
  })
  return allVisual
}

// 手动输入框输入时调用
function onManualInput() {
  const expr = manualValue.value.trim()
  if (!expr) return
  manualMode.value = !parse(expr) // 能完全可视化则退出手动模式，否则保持
  emitCron()
}

// 可视化点选时调用：回写手动框并退出手动模式
function onVisualChange() {
  manualMode.value = false
  manualValue.value = build()
  emitCron()
}

function toggleValue(field: FieldCfg, n: number) {
  const st = state[field.key]
  if (st.values.includes(n)) {
    st.values = st.values.filter((x) => x !== n)
  } else {
    st.values.push(n)
  }
  st.mode = 'specific'
  onVisualChange()
}

function setMode(field: FieldCfg['key'], mode: 'every' | 'step' | 'specific') {
  state[field].mode = mode
  if (mode === 'specific' && state[field].values.length === 0) {
    state[field].values = []
  }
  onVisualChange()
}

async function fetchPreview() {
  const cron = currentExpr()
  if (!cron) return
  previewing.value = true
  previewErr.value = ''
  try {
    const res = await previewCron({ cron, count: 5 })
    preview.value = res.list
    tz.value = res.timezone
  } catch (e) {
    previewErr.value = e instanceof ApiError ? e.message : '预览失败'
    preview.value = []
  } finally {
    previewing.value = false
  }
}

watch(
  () => props.modelValue,
  (v) => {
    if (v != null && v !== currentExpr()) {
      manualMode.value = !parse(v) // 能可视化则同步点选并退出手动模式
      manualValue.value = v
      fetchPreview()
    }
  },
)

onMounted(() => {
  if (props.modelValue) {
    manualMode.value = !parse(props.modelValue)
    manualValue.value = props.modelValue
  }
  fetchPreview()
})

function range(f: FieldCfg): number[] {
  const arr: number[] = []
  for (let i = f.min; i <= f.max; i++) arr.push(i)
  return arr
}
</script>

<template>
  <div class="cron">
    <div class="expr">
      <span class="expr-label">当前表达式</span>
      <code class="expr-code">{{ currentExpr() }}</code>
    </div>

    <!-- 手动输入框：始终可编辑，与可视化点选双向同步 -->
    <div class="raw">
      <input
        class="field"
        v-model="manualValue"
        placeholder="可直接输入，例如 30 3 * * 1-5 或 */5 * * * *"
        @input="onManualInput"
      />
      <p class="tip" v-if="manualMode">该表达式含区间等复杂写法，已按你输入的内容执行；可视化点选会覆盖此处内容。</p>
      <p class="tip" v-else>可直接手动输入，或下方点选生成；两者会实时同步。</p>
    </div>

    <div class="fields">
      <div v-for="f in fields" :key="f.key" class="cfield">
        <div class="cfield-head">
          <span class="clabel">{{ f.label }}</span>
          <div class="cmodes">
            <button type="button" class="cmode" :class="{ active: state[f.key].mode === 'every' }" @click="setMode(f.key, 'every')">每</button>
            <button type="button" class="cmode" :class="{ active: state[f.key].mode === 'step' }" @click="setMode(f.key, 'step')">每隔</button>
            <button type="button" class="cmode" :class="{ active: state[f.key].mode === 'specific' }" @click="setMode(f.key, 'specific')">指定</button>
          </div>
        </div>

        <!-- 每 -->
        <p v-if="state[f.key].mode === 'every'" class="cmeta">每个{{ f.label }}都执行</p>

        <!-- 每隔 N -->
        <div v-else-if="state[f.key].mode === 'step'" class="cstep">
          <span>每</span>
          <select class="field mini" v-model.number="state[f.key].step" @change="emitCron">
            <option v-for="n in f.stepPresets" :key="n" :value="n">{{ n }}</option>
          </select>
          <span>个{{ f.label }}执行一次</span>
        </div>

        <!-- 指定 -->
        <div v-else class="cgrid">
          <button
            v-for="n in range(f)"
            :key="n"
            type="button"
            class="cval"
            :class="{ on: state[f.key].values.includes(n) }"
            @click="toggleValue(f, n)"
          >
            {{ f.key === 'week' ? weekNames[n] : n }}
          </button>
        </div>
      </div>
    </div>

    <!-- 最近执行时间预览 -->
    <div class="preview-box">
      <div class="pb-head">
        <span>最近执行时间预览</span>
        <span v-if="tz" class="pb-tz">时区：{{ tz }}</span>
      </div>
      <div v-if="previewing" class="pb-loading">计算中…</div>
      <div v-else-if="previewErr" class="pb-err">{{ previewErr }}</div>
      <ul v-else-if="preview.length" class="pb-list">
        <li v-for="(t, i) in preview" :key="i">
          <span class="pb-idx">{{ i + 1 }}</span>{{ t }}
        </li>
      </ul>
      <div v-else class="pb-empty">暂无预览</div>
    </div>
  </div>
</template>

<style scoped>
.cron {
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 14px;
  background: var(--panel-2);
}
.expr {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.expr-label {
  font-size: 12px;
  color: var(--muted);
}
.expr-code {
  font-family: var(--mono);
  font-size: 13px;
  background: var(--panel);
  border: 1px solid var(--border-d);
  border-radius: 6px;
  padding: 5px 10px;
  color: var(--primary-d);
}
.raw .field {
  width: 100%;
}
.tip {
  font-size: 12px;
  color: var(--muted);
  margin: 6px 0 0;
  line-height: 1.5;
}

.fields {
  display: grid;
  gap: 14px;
}
.cfield {
  border-top: 1px dashed var(--border);
  padding-top: 12px;
}
.cfield:first-child {
  border-top: none;
  padding-top: 0;
}
.cfield-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
}
.clabel {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
.cmodes {
  display: inline-flex;
  gap: 4px;
}
.cmode {
  border: 1px solid var(--border-d);
  background: var(--panel);
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 12px;
  cursor: pointer;
  color: var(--text-2);
}
.cmode.active {
  border-color: var(--primary);
  background: var(--primary-l);
  color: var(--primary);
}
.cmeta {
  font-size: 12.5px;
  color: var(--muted);
  margin: 0;
}
.cstep {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-2);
}
.cstep .field.mini {
  width: 80px;
}
.cgrid {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.cval {
  border: 1px solid var(--border-d);
  background: var(--panel);
  border-radius: 6px;
  padding: 5px 9px;
  font-size: 12px;
  cursor: pointer;
  color: var(--text-2);
  min-width: 34px;
  text-align: center;
}
.cval.on {
  border-color: var(--primary);
  background: var(--primary);
  color: #fff;
}

.preview-box {
  margin-top: 16px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 12px 14px;
  background: var(--panel);
}
.pb-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 8px;
}
.pb-tz {
  font-size: 11.5px;
  font-weight: 400;
  color: var(--muted);
}
.pb-loading,
.pb-empty {
  font-size: 12.5px;
  color: var(--muted);
}
.pb-err {
  font-size: 12.5px;
  color: var(--danger);
}
.pb-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.pb-list li {
  display: flex;
  align-items: center;
  gap: 9px;
  font-family: var(--mono);
  font-size: 12.5px;
  color: var(--text-2);
}
.pb-idx {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--primary-l);
  color: var(--primary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-family: inherit;
}
</style>
