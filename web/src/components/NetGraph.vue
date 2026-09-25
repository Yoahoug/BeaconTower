<script setup>
import { computed } from 'vue'
import { fmtBps } from '../utils/format'

const props = defineProps({
  up: { type: Array, required: true },   // 上行速率序列
  down: { type: Array, required: true }, // 下行速率序列
  watts: { type: Array, default: null }, // 功率序列（可选，独立 Y 轴）
})

const W = 300
const H = 64

// 双线共用同一个 Y 轴（取两者最大值归一化），保持上下行相对关系真实
const maxVal = computed(() => {
  const all = [...props.up, ...props.down]
  return Math.max(1, ...all) * 1.15
})

const toPath = (series) => {
  if (!series.length) return ''
  const stepX = W / (series.length - 1)
  return series
    .map((v, i) => {
      const x = (i * stepX).toFixed(2)
      const y = (H - (v / maxVal.value) * (H - 4) - 2).toFixed(2)
      return `${i === 0 ? 'M' : 'L'}${x},${y}`
    })
    .join(' ')
}

const upPath = computed(() => toPath(props.up))
const downPath = computed(() => toPath(props.down))

// 功率单线：独立归一化（功率与网速量纲不同，不共轴）
const powerPath = computed(() => {
  const s = props.watts ?? []
  if (s.length < 2) return ''
  const max = Math.max(1, ...s) * 1.15
  const stepX = W / (s.length - 1)
  return s
    .map((v, i) => {
      const x = (i * stepX).toFixed(2)
      const y = (H - (v / max) * (H - 4) - 2).toFixed(2)
      return `${i === 0 ? 'M' : 'L'}${x},${y}`
    })
    .join(' ')
})

const lastWatts = computed(() => (props.watts?.length ? props.watts[props.watts.length - 1] : 0))
</script>

<template>
  <div class="net-graph">
    <div class="net-graph-head">
      <span class="legend"><i class="lg up"></i>上行 {{ fmtBps(up[up.length - 1] ?? 0) }}</span>
      <span class="legend"><i class="lg down"></i>下行 {{ fmtBps(down[down.length - 1] ?? 0) }}</span>
      <span class="hint">全网吞吐 · 近 2 分钟</span>
    </div>
    <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" class="net-graph-svg">
      <path :d="downPath" fill="none" stroke="rgba(0,122,255,0.85)" stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
      <path :d="upPath" fill="none" stroke="rgba(52,199,89,0.9)" stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
    </svg>

    <!-- 全网功耗走势：有可测功率节点才显示（RAPL 不可用的机型拿不到读数） -->
    <template v-if="powerPath">
      <div class="net-graph-head power-head">
        <span class="legend"><i class="lg power"></i>功耗 {{ lastWatts.toFixed(1) }} W</span>
        <span class="hint">全网功率 · 与吞吐同窗</span>
      </div>
      <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" class="net-graph-svg power-svg">
        <path :d="powerPath" fill="none" stroke="rgba(255,149,0,0.9)" stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
      </svg>
    </template>
  </div>
</template>
