<!-- ============================================================
     迷你走势线（轻量 SVG，卡片级）
     - 入场时沿路径“描画”一遍（pathLength 归一化，无需 JS 测量）
     - 面积 = 顶部同色渐变渐隐；末端实时脉冲点
     - 数据轮询更新只换路径、不重播入场动画
     ============================================================ -->
<script setup>
import { computed, useId } from 'vue'

const props = defineProps({
  data: { type: Array, required: true },
  color: { type: String, default: '#0ea5e9' },
  height: { type: Number, default: 56 },
  label: { type: String, default: '走势' },
  format: { type: Function, default: (v) => String(v) },
})

const W = 300
const H = 48
const gid = `spark-${useId()}`

const maxVal = computed(() => Math.max(1, ...props.data) * 1.15)

const points = computed(() => {
  if (props.data.length < 2) return []
  const stepX = W / (props.data.length - 1)
  return props.data.map((v, i) => ({
    x: i * stepX,
    y: H - (v / maxVal.value) * (H - 6) - 3,
  }))
})

const path = computed(() =>
  points.value.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' '),
)

const area = computed(() => (path.value ? `${path.value} L${W},${H} L0,${H} Z` : ''))

const last = computed(() => points.value[points.value.length - 1] || null)

const tip = computed(() => {
  if (!props.data.length) return `${props.label}：暂无数据`
  return `${props.label}：最新 ${props.format(props.data[props.data.length - 1])}，峰值 ${props.format(Math.max(...props.data))}`
})
</script>

<template>
  <div role="img" :aria-label="tip" :title="tip">
    <svg
      :viewBox="`0 0 ${W} ${H}`"
      preserveAspectRatio="none"
      aria-hidden="true"
      :style="{
        width: '100%', height: height + 'px', display: 'block',
        borderRadius: '10px',
        background: 'linear-gradient(180deg, rgba(244,247,253,.6), rgba(255,255,255,.35))',
      }"
    >
      <defs>
        <linearGradient :id="gid" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" :stop-color="color" stop-opacity="0.22" />
          <stop offset="100%" :stop-color="color" stop-opacity="0" />
        </linearGradient>
      </defs>
      <line
        x1="0"
        :y1="H / 2"
        :x2="W"
        :y2="H / 2"
        stroke="rgba(100, 116, 160, 0.18)"
        stroke-width="1"
        stroke-dasharray="3 4"
        vector-effect="non-scaling-stroke"
      />
      <path v-if="area" :d="area" :fill="`url(#${gid})`" class="spark-area" />
      <path
        v-if="path"
        :d="path"
        fill="none"
        :stroke="color"
        stroke-width="1.8"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
        pathLength="1"
        class="spark-line"
      />
      <!-- 末端实时点：外圈脉冲 + 实心核 -->
      <g v-if="last" class="spark-end">
        <circle :cx="last.x" :cy="last.y" r="5" :fill="color" opacity="0.2" class="spark-end__halo" />
        <circle :cx="last.x" :cy="last.y" r="2.4" :fill="color" />
      </g>
    </svg>
  </div>
</template>

<style scoped>
/* 入场描画一次：pathLength 归一化后 dasharray 恒为 1，与路径长度无关 */
.spark-line {
  stroke-dasharray: 1;
  animation: spark-draw 1s var(--bt-ease-out, ease-out) 0.1s both;
}

.spark-area {
  animation: spark-fade 0.9s ease 0.45s both;
}

.spark-end__halo {
  animation: spark-pulse 2s ease-out infinite;
  transform-origin: center;
  transform-box: fill-box;
}

@keyframes spark-draw {
  from {
    stroke-dashoffset: 1;
  }

  to {
    stroke-dashoffset: 0;
  }
}

@keyframes spark-fade {
  from {
    opacity: 0;
  }
}

@keyframes spark-pulse {
  0% {
    transform: scale(0.7);
    opacity: 0.5;
  }

  70%,
  100% {
    transform: scale(1.9);
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .spark-line,
  .spark-area,
  .spark-end__halo {
    animation: none;
  }
}
</style>
