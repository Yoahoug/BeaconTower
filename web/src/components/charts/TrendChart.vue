<!-- ============================================================
     ECharts 趋势图（按需引入的最小封装 · v2.0 亮色玻璃主题）
     - 面积填色改为纵向渐变（顶部 22% 同色 → 底部透明）
     - tooltip 白玻璃浮层；更新动画 500ms 平滑跟随 10s 轮询
     - a11y：role=img + aria-label（含各序列最新值）
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
  // [{ name, color: '#rrggbb', data: number[], fill?: boolean }]
  series: { type: Array, required: true },
  height: { type: Number, default: 240 },
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

// #rrggbb → rgba()，用于渐变面积与柔光（不依赖 echarts 内部模块）
function hexA(hex, alpha) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || '')
  if (!m) return hex
  const n = parseInt(m[1], 16)
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`
}

function render() {
  if (!chart) return
  chart.setOption(
    {
      animationDuration: 600,
      animationEasing: 'cubicOut',
      animationDurationUpdate: 520,
      animationEasingUpdate: 'cubicOut',
      grid: { left: 8, right: 14, top: 12, bottom: 4, containLabel: true },
      // 调用方自带图例（含实时数值），ECharts 内置图例默认隐藏避免重复
      legend: { show: false },
      tooltip: {
        trigger: 'axis',
        confine: true,
        backgroundColor: 'rgba(255, 255, 255, 0.92)',
        borderColor: 'rgba(23, 32, 64, 0.08)',
        textStyle: { color: '#101a2e', fontSize: 12 },
        extraCssText:
          'backdrop-filter: blur(14px); border-radius: 12px; box-shadow: 0 8px 28px -6px rgba(59,73,150,.28); padding: 8px 12px;',
        valueFormatter: (v) => (props.yFormatter ? props.yFormatter(v) : v),
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: labels.value,
        axisLine: { lineStyle: { color: 'rgba(23, 32, 64, 0.12)' } },
        axisTick: { show: false },
        axisLabel: { color: '#9aa3bc', fontSize: 11, hideOverlap: true },
      },
      yAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: 'rgba(23, 32, 64, 0.06)' } },
        axisLabel: {
          color: '#9aa3bc',
          fontSize: 11,
          formatter: (v) => (props.yFormatter ? props.yFormatter(v) : v),
        },
      },
      series: props.series.map((s) => ({
        name: s.name,
        type: 'line',
        data: s.data,
        showSymbol: false,
        smooth: 0.35,
        lineStyle: {
          width: 2.5,
          color: s.color,
          shadowColor: hexA(s.color, 0.35),
          shadowBlur: 8,
          shadowOffsetY: 4,
        },
        itemStyle: { color: s.color },
        areaStyle: s.fill
          ? {
              color: {
                type: 'linear',
                x: 0,
                y: 0,
                x2: 0,
                y2: 1,
                colorStops: [
                  { offset: 0, color: hexA(s.color, 0.22) },
                  { offset: 1, color: hexA(s.color, 0.015) },
                ],
              },
            }
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
