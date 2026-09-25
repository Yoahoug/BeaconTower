<script setup>
import { computed, onMounted, ref } from 'vue'

const props = defineProps({
  label: { type: String, required: true },
  sub: { type: String, default: '' },      // 标签后的补充，如 CPU "4核"
  value: { type: Number, required: true }, // 百分比 0-100
  size: { type: Number, default: 58 },     // 直径 px
  delay: { type: Number, default: 0 },     // 入场延迟 ms（三仪表错开画弧）
})

const SW = 5 // 描边宽
const r = computed(() => (props.size - SW) / 2 - 1)
const c = computed(() => 2 * Math.PI * r.value)

// 入场时从 0 画弧到目标值：挂载后下一帧再放开目标值，触发 CSS 过渡
const drawn = ref(false)
onMounted(() => {
  window.setTimeout(() => { drawn.value = true }, 80 + props.delay)
})

const offset = computed(() => {
  const pct = Math.min(100, Math.max(0, props.value))
  const target = c.value * (1 - pct / 100)
  return (drawn.value ? target : c.value).toFixed(2)
})

const level = computed(() => {
  if (props.value >= 85) return 'high'
  if (props.value >= 60) return 'mid'
  return 'low'
})

const display = computed(() => {
  const v = props.value
  return `${v >= 10 || v === 0 ? Math.round(v) : v.toFixed(1)}%`
})
</script>

<template>
  <div class="gauge">
    <div class="gauge-plot" :style="{ width: size + 'px', height: size + 'px' }">
      <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`">
        <circle
          class="gauge-bg"
          :cx="size / 2" :cy="size / 2" :r="r"
          fill="none" :stroke-width="SW"
        />
        <circle
          class="gauge-fg" :class="level"
          :cx="size / 2" :cy="size / 2" :r="r"
          fill="none" :stroke-width="SW" stroke-linecap="round"
          :stroke-dasharray="c.toFixed(2)"
          :stroke-dashoffset="offset"
          :transform="`rotate(-90 ${size / 2} ${size / 2})`"
        />
      </svg>
      <div class="gauge-center">{{ display }}</div>
    </div>
    <div class="gauge-label" :title="`${label} ${sub}`.trim()">
      <span class="gauge-name">{{ label }}</span>
      <span v-if="sub" class="gauge-sub">{{ sub }}</span>
    </div>
  </div>
</template>
