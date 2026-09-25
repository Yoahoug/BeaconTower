<!-- ============================================================
     ECharts 趋势图（按需引入的最小封装）
     约束：
     - 只用 line + tooltip + grid + legend 所需模块，禁止全量 import 'echarts'
     - 空数据 / 加载中由父组件处理，本组件只负责渲染有效序列
     - 图表必须带 role="img" + aria-label（a11y），见下方 chartDom
     ============================================================ -->
<script setup>
import { ref, watch, onMounted, onUnmounted, computed } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps({
  // [{ name, color, data: number[], unit?: string }]
  series: { type: Array, required: true },
  height: { type: Number, default: 240 },
  // x 轴标签：默认按序列长度生成“近 N 分钟”刻度
  xLabels: { type: Array, default: null },
  yFormatter: { type: Function, default: null },
  label: { type: String, default: '趋势图' },
})

const chartDom = ref(null)
let chart = null

const fmtVal = (v) => (props.yFormatter ? props.yFormatter(v) : String(Math.round(Number(v) * 10) / 10))

const a11yLabel = computed(() =>
  `${props.label}：${props.series
    .map((s) => (s.data.length ? `${s.name}最新 ${fmtVal(s.data[s.data.length - 1])}` : `${s.name}暂无数据`))
    .join('；')}`,
)

const labels = computed(() => {
  const n = props.series[0]?.data.length || 0
  if (props.xLabels?.length === n) return props.xLabels
  return Array.from({ length: n }, (_, i) => `-${n - 1 - i}m`)
})

function render() {
  if (!chart) return
  chart.setOption(
    {
      animationDuration: 300,
      grid: { left: 8, right: 12, top: 34, bottom: 4, containLabel: true },
      legend: {
        top: 0,
        right: 0,
        icon: 'roundRect',
        itemWidth: 14,
        itemHeight: 3,
        textStyle: { fontSize: 12, color: '#41495c' },
      },
      tooltip: {
        trigger: 'axis',
        confine: true,
        valueFormatter: (v) => (props.yFormatter ? props.yFormatter(v) : v),
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: labels.value,
        axisLine: { lineStyle: { color: '#e2e6ee' } },
        axisTick: { show: false },
        axisLabel: { color: '#9aa1b2', fontSize: 11, hideOverlap: true },
      },
      yAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#eef1f6' } },
        axisLabel: {
          color: '#9aa1b2',
          fontSize: 11,
          formatter: (v) => (props.yFormatter ? props.yFormatter(v) : v),
        },
      },
      series: props.series.map((s) => ({
        name: s.name,
        type: 'line',
        data: s.data,
        showSymbol: false,
        smooth: 0.25,
        lineStyle: { width: 2, color: s.color },
        itemStyle: { color: s.color },
        areaStyle: s.fill
          ? { color: s.color, opacity: 0.08 }
          : undefined,
        emphasis: { focus: 'series' },
      })),
    },
    { notMerge: true },
  )
}

let ro = null

onMounted(() => {
  chart = echarts.init(chartDom.value)
  render()
  ro = new ResizeObserver(() => chart?.resize())
  if (chartDom.value?.parentElement) ro.observe(chartDom.value.parentElement)
  window.addEventListener('resize', onResize)
})

function onResize() {
  chart?.resize()
}

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  ro?.disconnect()
  chart?.dispose()
  chart = null
})

watch(() => props.series, render, { deep: true })
watch(() => props.yFormatter, render)
</script>

<template>
  <div
    ref="chartDom"
    class="chart-box"
    :style="{ height: height + 'px', minHeight: height + 'px' }"
    role="img"
    :aria-label="a11yLabel"
  />
</template>
