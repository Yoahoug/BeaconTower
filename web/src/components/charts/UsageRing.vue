<!-- ============================================================
     仪表环（轻量 SVG 版）
     约束：卡片级高频渲染组件禁止使用 ECharts（包体积 + 实例开销），
     只允许在页面级大趋势图中使用 ECharts。a11y：role=img + 文案。
     ============================================================ -->
<script setup>
import { computed } from 'vue'

const props = defineProps({
  label: { type: String, required: true },
  value: { type: Number, required: true }, // 0-100
  size: { type: Number, default: 96 },
})

const SW = 8
const r = computed(() => (props.size - SW) / 2 - 2)
const c = computed(() => 2 * Math.PI * r.value)

const pct = computed(() => Math.min(100, Math.max(0, props.value)))

const offset = computed(() => (c.value * (1 - pct.value / 100)).toFixed(2))

const color = computed(() => {
  if (pct.value >= 85) return 'var(--bt-danger-500)'
  if (pct.value >= 60) return 'var(--bt-warning-500)'
  return 'var(--bt-success-500)'
})

const display = computed(() => {
  const v = pct.value
  return `${v >= 10 || v === 0 ? Math.round(v) : v.toFixed(1)}%`
})
</script>

<template>
  <div
    role="img"
    :aria-label="`${label} ${display}`"
    :style="{ width: size + 'px', height: size + 'px', position: 'relative' }"
  >
    <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`" aria-hidden="true" style="display: block">
      <circle
        :cx="size / 2"
        :cy="size / 2"
        :r="r"
        fill="none"
        stroke="var(--bt-bg-active)"
        :stroke-width="SW"
      />
      <circle
        :cx="size / 2"
        :cy="size / 2"
        :r="r"
        fill="none"
        :stroke="color"
        :stroke-width="SW"
        stroke-linecap="round"
        :stroke-dasharray="c.toFixed(2)"
        :stroke-dashoffset="offset"
        :transform="`rotate(135 ${size / 2} ${size / 2})`"
        stroke-dasharray-rest="none"
        :style="{ transition: 'stroke-dashoffset 0.4s var(--bt-ease), stroke 0.3s ease' }"
      />
    </svg>
    <div
      aria-hidden="true"
      class="tnum"
      :style="{
        position: 'absolute', inset: 0, display: 'grid', placeItems: 'center',
        fontSize: '15px', fontWeight: 600, letterSpacing: '-0.01em',
      }"
    >
      {{ display }}
    </div>
  </div>
</template>
