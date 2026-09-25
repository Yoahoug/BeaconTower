<!-- ============================================================
     节点卡片（v2.0 玻璃卡）
     - 指针辉光 + 悬停上浮；OS 徽标微转
     - 三枚渐变仪表环（阈值自动换色 + 端点发光）
     - 吞吐双色格 · 功耗琥珀条 · 双走势（入场描画 + 末端脉冲）
     - 离线 = 虚线描边 + 降饱和 + 文字徽标（状态双通道）
     ============================================================ -->
<script setup>
import { computed } from 'vue'
import AppIcon from '../AppIcon.vue'
import GaugeRing from '../charts/GaugeRing.vue'
import SparkLine from '../charts/SparkLine.vue'
import {
  fmtBps,
  fmtSizeShort,
  fmtUptime,
  agoText,
  fmtWatts,
  fmtKwh,
  fmtCost,
  fmtGhz,
} from '../../utils/format'

const props = defineProps({
  server: { type: Object, required: true },
  lastUpdated: { type: Number, default: 0 },
})

const memPct = computed(() =>
  props.server.metrics.memTotal ? (props.server.metrics.memUsed / props.server.metrics.memTotal) * 100 : 0,
)

const diskPct = computed(() =>
  props.server.metrics.diskTotal ? (props.server.metrics.diskUsed / props.server.metrics.diskTotal) * 100 : 0,
)

const memDetail = computed(() =>
  props.server.online
    ? `${fmtSizeShort(props.server.metrics.memUsed)}/${fmtSizeShort(props.server.metrics.memTotal)}`
    : '—',
)

const hasPower = computed(() => {
  const p = props.server.power
  return !!p?.enabled && !!props.server.online && !!p.rapl
})

const powerNoReason = computed(() => {
  if (!props.server.online) return '离线节点无功耗读数'
  if (!props.server.power?.enabled) return '未启用功耗采集'
  return '该机型未暴露 RAPL 功率计接口（部分 AMD / 虚拟机），无法测量功耗'
})

const cpuHistory = computed(() =>
  props.server.online ? props.server.cpuHistory || [] : [],
)
const powerHistory = computed(() =>
  props.server.online ? props.server.powerHistory || [] : [],
)
</script>

<template>
  <article
    class="bt-card bt-card--hover spot node-card"
    :class="{ 'is-offline': !server.online }"
    v-spotlight
    :aria-label="`节点 ${server.name}`"
  >
    <div class="node-card__head">
      <div class="node-card__os" :class="{ 'is-off': !server.online }" aria-hidden="true">
        <AppIcon name="linux" />
      </div>
      <div class="node-card__titles">
        <div class="node-card__name" :title="server.name">{{ server.name }}</div>
        <div class="node-card__meta">
          <AppIcon v-if="server.regionSource === 'auto'" name="pin" aria-hidden="true" />
          <span class="ellipsis">{{ server.region }} · {{ server.profile.os }} · {{ server.profile.arch }}</span>
        </div>
      </div>
      <span class="bt-tag" :class="server.online ? 'bt-tag--success' : 'bt-tag--danger'">
        <span
          class="pulse-dot"
          :class="{ 'pulse-dot--still': !server.online }"
          aria-hidden="true"
        />
        {{ server.online ? '在线' : '离线' }}
      </span>
    </div>

    <div class="node-gauges" role="group" aria-label="资源使用率">
      <div class="gauge-cell">
        <GaugeRing label="CPU" :value="server.metrics.cpu" :size="92" />
        <div class="gauge-cell__label"><span>CPU</span><span class="gauge-cell__sub">{{ server.profile.cores }}核</span></div>
      </div>
      <div class="gauge-cell">
        <GaugeRing label="内存" :value="memPct" :size="92" tone="violet" />
        <div class="gauge-cell__label"><span>内存</span><span class="gauge-cell__sub">{{ memDetail }}</span></div>
      </div>
      <div class="gauge-cell">
        <GaugeRing label="磁盘" :value="diskPct" :size="92" tone="brand" />
        <div class="gauge-cell__label">
          <span>磁盘</span>
          <span class="gauge-cell__sub">{{ fmtSizeShort(server.metrics.diskUsed) }}/{{ fmtSizeShort(server.metrics.diskTotal) }}</span>
        </div>
      </div>
    </div>

    <div class="node-net tnum" aria-label="实时吞吐">
      <div class="node-net__cell node-net__cell--up">
        <span class="node-net__dir" aria-hidden="true"><AppIcon name="arrow-up" /></span>
        <span>{{ fmtBps(server.metrics.netUp) }}</span>
      </div>
      <div class="node-net__cell node-net__cell--down">
        <span class="node-net__dir" aria-hidden="true"><AppIcon name="arrow-down" /></span>
        <span>{{ fmtBps(server.metrics.netDown) }}</span>
      </div>
    </div>

    <div
      v-if="hasPower"
      class="node-power"
      :title="`整机 ${fmtWatts(server.power.watts.total)}（CPU ${fmtWatts(server.power.watts.cpu)} + 基础 ${fmtWatts(server.power.baseLoadW)}）`"
    >
      <div class="node-power__main">
        <AppIcon name="bolt" aria-hidden="true" />
        <span class="node-power__val tnum">{{ fmtWatts(server.power.watts.total) }}</span>
        <span class="node-power__split">CPU {{ fmtWatts(server.power.watts.cpu) }}</span>
        <span v-if="server.power.tempC != null" class="node-power__temp">
          <AppIcon name="temp" aria-hidden="true" />{{ server.power.tempC.toFixed(1) }}°C
        </span>
      </div>
      <div class="node-power__sub">
        <span>{{ fmtGhz(server.power.freqMhz) }}</span>
        <span v-if="server.power.energy">今日 {{ fmtKwh(server.power.energy.todayKwh) }} · 月 {{ fmtKwh(server.power.energy.monthKwh) }} ≈ {{ fmtCost(server.power.energy.estCostMonth) }}</span>
      </div>
    </div>
    <div v-else-if="server.power?.enabled" class="node-power--na" :title="powerNoReason">
      <AppIcon name="plug" aria-hidden="true" />
      <span>功耗不可用 · {{ server.online ? '未暴露 RAPL 功率计接口' : '节点离线' }}</span>
    </div>

    <div class="node-spark">
      <div class="node-spark__head">
        <span>CPU 走势 · 近 2 分钟</span>
        <span>负载 {{ server.metrics.load.map((l) => l.toFixed(2)).join(' / ') }}</span>
      </div>
      <SparkLine
        :data="cpuHistory"
        color="#0ea5e9"
        :height="56"
        label="CPU 走势"
        :format="(v) => `${Math.round(v)}%`"
      />
      <template v-if="hasPower && powerHistory.length > 1">
        <div class="node-spark__head" style="margin-top: 8px">
          <span>功耗走势 · 与 CPU 同窗</span>
          <span>{{ fmtWatts(server.power.watts.total) }}</span>
        </div>
        <SparkLine
          :data="powerHistory"
          color="#f59e0b"
          :height="44"
          label="功耗走势"
          :format="(v) => `${Number(v).toFixed(1)} W`"
        />
      </template>
    </div>

    <div class="node-card__foot">
      <span class="tnum">TCP {{ server.metrics.tcp }} · UDP {{ server.metrics.udp }} · 进程 {{ server.metrics.processes }}</span>
      <span>{{ server.profile.virt }} · {{ server.profile.cores }}核</span>
    </div>
    <div class="node-card__foot" style="border-top: none; padding-top: 0">
      <span>在线 {{ fmtUptime(server.metrics.uptimeDays) }}</span>
      <span>{{ server.online ? `更新于 ${agoText(lastUpdated)}` : `断开 ${server.offlineSince}` }}</span>
    </div>
  </article>
</template>
