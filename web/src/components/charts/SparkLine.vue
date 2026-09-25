<!-- ============================================================
     迷你走势线（轻量 SVG，用于卡片内趋势；大面板用 ECharts 趋势图）
     自带基线网格 + 首尾取值 tooltip（title）+ a11y 文案。
     ============================================================ -->
<script setup>
import { computed } from 'vue'

const props = defineProps({
  data: { type: Array, required: true },
  color: { type: String, default: 'var(--bt-brand-500)' },
  height: { type: Number, default: 56 },
  label: { type: String, default: '走势' },
  format: { type: Function, default: (v) => String(v) },
})

const W = 300
const H = 48

const maxVal = computed(() => Math.max(1, ...props.data) * 1.15)

const path = computed(() => {
  if (props.data.length < 2) return ''
  const stepX = W / (props.data.length - 1)
  return props.data
    .map((v, i) => {
      const x = (i * stepX).toFixed(2)
      const y = (H - (v / maxVal.value) * (H - 4) - 2).toFixed(2)
      return `${i === 0 ? 'M' : 'L'}${x},${y}`
    })
    .join(' ')
})

const area = computed(() => (path.value ? `${path.value} L${W},${H} L0,${H} Z` : ''))

const tip = computed(() => {
  if (!props.data.length) return `${props.label}：暂无数据`
  return `${props.label}：最新 ${props.format(props.data[props.data.length - 1])}，峰值 ${props.format(Math.max(...props.data))}`;
})
</script>

<template>
  <div role="img" :aria-label="tip" :title="tip">
    <svg
      :viewBox="`0 0 ${W} ${H}`"
      preserveAspectRatio="none"
      aria-hidden="true"
      :style="{ width: '100%', height: height + 'px', display: 'block', borderRadius: '6px', background: 'var(--bt-bg-subtle)' }"
    >
      <line x1="0" :y1="H / 2" :x2="W" :y2="H / 2" stroke="var(--bt-border)" stroke-width="1" stroke-dasharray="3 3" vector-effect="non-scaling-stroke" />
      <path v-if="area" :d="area" :fill="color" opacity="0.1" />
      <path
        v-if="path"
        :d="path"
        fill="none"
        :stroke="color"
        stroke-width="1.6"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
      />
    </svg>
  </div>
</template>
