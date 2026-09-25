<script setup>
import AppIcon from './AppIcon.vue'
import GaugeRing from './GaugeRing.vue'
import { fmtBps, fmtSizeShort, fmtUptime, agoText } from '../utils/format'

const props = defineProps({
  server: { type: Object, required: true },
  lastUpdated: { type: Number, default: 0 },
  index: { type: Number, default: 0 }, // 用于入场动画的交错延迟
})

const memPct = () =>
  props.server.metrics.memTotal
    ? (props.server.metrics.memUsed / props.server.metrics.memTotal) * 100
    : 0

const diskPct = () =>
  props.server.metrics.diskTotal
    ? (props.server.metrics.diskUsed / props.server.metrics.diskTotal) * 100
    : 0

const memDetail = () =>
  props.server.online
    ? `${fmtSizeShort(props.server.metrics.memUsed)}/${fmtSizeShort(props.server.metrics.memTotal)}`
    : '—'

// CPU 走势图路径（与网络图同款归一化）
const W = 300
const H = 40
const sparkPath = () => {
  const pts = props.server.cpuHistory
  if (!pts.length) return ''
  const stepX = W / (pts.length - 1)
  return pts
    .map((v, i) => {
      const x = (i * stepX).toFixed(2)
      const y = (H - (Math.min(100, v) / 100) * (H - 4) - 2).toFixed(2)
      return `${i === 0 ? 'M' : 'L'}${x},${y}`
    })
    .join(' ')
}
</script>

<template>
  <article
    v-tilt
    class="server-card"
    :class="{ offline: !server.online }"
    :style="{ '--i': index }"
  >
    <div class="card-head">
      <div class="os-icon" :class="{ off: !server.online }">
        <AppIcon name="linux" />
      </div>
      <div class="titles">
        <div class="name">{{ server.name }}</div>
        <div
          class="region"
          :title="server.regionSource === 'auto' ? '地区由公网 IP 自动定位（IP 不公开）' : '地区由管理员设置'"
        >
          <AppIcon v-if="server.regionSource === 'auto'" name="pin" class="pin" />
          <span class="region-text">{{ server.region }} · {{ server.profile.os }} · {{ server.profile.arch }}</span>
        </div>
      </div>
      <span class="status-badge" :class="server.online ? 'ok' : 'offline'">
        <span class="dot"></span>{{ server.online ? '在线' : '离线' }}
      </span>
    </div>

    <div class="gauges">
      <GaugeRing label="CPU" :sub="`${server.profile.cores}核`" :value="server.metrics.cpu" :size="56" :delay="index * 50" />
      <GaugeRing label="内存" :sub="memDetail()" :value="memPct()" :size="56" :delay="index * 50 + 90" />
      <GaugeRing label="磁盘" :sub="`${fmtSizeShort(server.metrics.diskUsed)}/${fmtSizeShort(server.metrics.diskTotal)}`" :value="diskPct()" :size="56" :delay="index * 50 + 180" />
    </div>

    <div class="card-spark">
      <div class="spark-head">
        <span>CPU 走势</span>
        <span>负载 {{ server.metrics.load.map((l) => l.toFixed(2)).join(' / ') }}</span>
      </div>
      <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" class="spark-svg">
        <path :d="sparkPath()" fill="none" stroke="rgba(0,122,255,0.85)" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
      </svg>
    </div>

    <div class="card-net">
      <div class="net-item up"><span class="dir">↑</span><span class="val">{{ fmtBps(server.metrics.netUp) }}</span></div>
      <div class="net-item down"><span class="dir">↓</span><span class="val">{{ fmtBps(server.metrics.netDown) }}</span></div>
    </div>

    <div class="card-meta">
      <span>TCP {{ server.metrics.tcp }}</span>
      <span>UDP {{ server.metrics.udp }}</span>
      <span>进程 {{ server.metrics.processes }}</span>
      <span>{{ server.profile.virt }} · {{ server.profile.cores }}核</span>
    </div>

    <div class="card-foot">
      <span>在线 {{ fmtUptime(server.metrics.uptimeDays) }}</span>
      <span>{{ server.online ? `更新于 ${agoText(lastUpdated)}` : `断开 ${server.offlineSince}` }}</span>
    </div>
  </article>
</template>
