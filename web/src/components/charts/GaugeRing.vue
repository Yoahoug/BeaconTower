<!-- ============================================================
     渐变仪表环（轻量 SVG）
     - 弧形描边 = 双 stop 渐变；阈值自动换色（绿 → 琥珀 → 玫红）
     - 弧线端点一颗发光小球，数值与弧长同一条补间曲线
     - a11y：role=img + 完整读屏文案
     约束：卡片级高频组件只用 SVG，禁止 mount ECharts 实例。
     ============================================================ -->
<script setup>
import { computed, useId } from 'vue'
import { useTween } from '../../composables/useTween'

const props = defineProps({
  label: { type: String, required: true },
  value: { type: Number, required: true }, // 0-100
  size: { type: Number, default: 96 },
  // auto = 阈值自动（默认）；brand/violet/amber 可强制指定色调
  tone: { type: String, default: 'auto' },
})

const SW = 8
const r = computed(() => (props.size - SW) / 2 - 3)
const c = computed(() => 2 * Math.PI * r.value)

const shown = useTween(() => props.value, { duration: 850 })
const pct = computed(() => Math.min(100, Math.max(0, shown.value)))
const offset = computed(() => c.value * (1 - pct.value / 100))

// 渐变色调表：stopA → stopB 均为“亮 → 稳”的同色系渐变
const TONES = {
  ok: ['#34d399', '#059669'],
  warn: ['#fbbf24', '#d97706'],
  danger: ['#fb7185', '#e11d48'],
  brand: ['#38bdf8', '#6366f1'],
  violet: ['#c4b5fd', '#8b5cf6'],
  amber: ['#fcd34d', '#f59e0b'],
}

const toneKey = computed(() => {
  if (props.tone !== 'auto') return props.tone
  if (pct.value >= 85) return 'danger'
  if (pct.value >= 60) return 'warn'
  return 'ok'
})

const stops = computed(() => TONES[toneKey.value] || TONES.ok)
const gid = `gauge-${useId()}`

// 弧线端点发光小球（-90° 起顺时针）
const cap = computed(() => {
  const angle = ((pct.value / 100) * 360 - 90) * (Math.PI / 180)
  const cx = props.size / 2 + r.value * Math.cos(angle)
  const cy = props.size / 2 + r.value * Math.sin(angle)
  return { x: cx.toFixed(2), y: cy.toFixed(2) }
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
      <defs>
        <linearGradient :id="gid" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" :stop-color="stops[0]" />
          <stop offset="100%" :stop-color="stops[1]" />
        </linearGradient>
      </defs>
      <!-- 轨道 -->
      <circle
        :cx="size / 2"
        :cy="size / 2"
        :r="r"
        fill="none"
        stroke="rgba(100, 116, 160, 0.12)"
        :stroke-width="SW"
      />
      <!-- 渐变值弧 -->
      <circle
        :cx="size / 2"
        :cy="size / 2"
        :r="r"
        fill="none"
        :stroke="`url(#${gid})`"
        :stroke-width="SW"
        stroke-linecap="round"
        :stroke-dasharray="c.toFixed(2)"
        :stroke-dashoffset="offset.toFixed(2)"
        :transform="`rotate(-90 ${size / 2} ${size / 2})`"
      />
      <!-- 端点发光小球 -->
      <circle
        v-if="pct > 0.5"
        :cx="cap.x"
        :cy="cap.y"
        r="3.2"
        :fill="stops[0]"
        :style="{ filter: `drop-shadow(0 0 4px ${stops[0]})` }"
      />
    </svg>
    <div
      aria-hidden="true"
      class="tnum"
      :style="{
        position: 'absolute', inset: 0, display: 'grid', placeItems: 'center',
        fontSize: '15px', fontWeight: 700, letterSpacing: '-0.02em',
      }"
    >
      {{ display }}
    </div>
  </div>
</template>
