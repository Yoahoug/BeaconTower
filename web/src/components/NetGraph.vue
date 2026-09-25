<script setup>
import { computed } from 'vue'
import { fmtBps } from '../utils/format'

const props = defineProps({
  up: { type: Array, required: true },   // 上行速率序列
  down: { type: Array, required: true }, // 下行速率序列
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
  </div>
</template>
