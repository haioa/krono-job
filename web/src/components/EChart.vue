<script setup lang="ts">
import { ref, shallowRef, onMounted, onBeforeUnmount, watch } from 'vue'
import { init, use, type ECharts, type EChartsCoreOption } from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

// 仅按需注册用到的图表与组件，避免打包整个 echarts。
use([LineChart, BarChart, TooltipComponent, LegendComponent, GridComponent, CanvasRenderer])

// 通用 ECharts 封装：根据 option 渲染，option 变化时增量更新，容器尺寸变化时自适应。
const props = withDefaults(
  defineProps<{
    option: EChartsCoreOption
    height?: string
  }>(),
  { height: '360px' },
)

const el = ref<HTMLDivElement | null>(null)
const chart = shallowRef<ECharts | null>(null)

function resize() {
  chart.value?.resize()
}

onMounted(() => {
  if (!el.value) return
  chart.value = init(el.value)
  chart.value.setOption(props.option)
  window.addEventListener('resize', resize)
})

watch(
  () => props.option,
  (opt) => {
    if (chart.value && opt) {
      chart.value.setOption(opt, true)
    }
  },
  { deep: true },
)

onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  chart.value?.dispose()
  chart.value = null
})
</script>

<template>
  <div ref="el" class="echart" :style="{ height }"></div>
</template>

<style scoped>
.echart {
  width: 100%;
}
</style>
