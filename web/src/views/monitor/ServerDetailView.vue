<!-- ============================================================
     节点详情页：公开只读 · 历史曲线（ECharts TrendChart 复用）
     - range 切换：1h/6h/24h/7d（7d 为小时聚合，或需登录）
     - 三图：CPU / 内存+磁盘 / 吞吐（+ 功耗曲线，有 RAPL 才显示）
     - 头部复用 NodeCard 概要；a11y：图表 role=img 由 TrendChart 保障
     ============================================================ -->
<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../../components/AppIcon.vue'
import TrendChart from '../../components/charts/TrendChart.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import NodeCard from '../../components/monitor/NodeCard.vue'
import { useMonitorStore } from '../../stores/monitor'
import { monitorClient, normalizePoints, RANGE_META } from '../../api/monitor'
import { fmtBps, fmtSizeShort, fmtWatts } from '../../utils/format'

const route = useRoute()
const router = useRouter()
const monitor = useMonitorStore()

const nodeId = computed(() => Number(route.params.id))
const server = computed(() => monitor.servers.find((s) => s.id === nodeId.value) || null)

const range = ref(typeof route.query.range === 'string' && ['1h', '6h', '24h', '7d'].includes(route.query.range) ? route.query.range : '1h')
const points = ref([])
const status = ref('loading') // loading | ready | error
const error = ref('')
const needLogin = ref(false)
let ctrl = null

const rangeLabel = computed(() => (RANGE_META.find((r) => r.key === range.value)?.label || '').replace('近 ', '近'))

function fmtClock(ts) {
  const d = new Date(ts * 1000)
  const p = (n) => String(n).padStart(2, '0')
  if (range.value === '7d') return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:00`
  return `${p(d.getHours())}:${p(d.getMinutes())}`
}

const xLabels = computed(() => points.value.map((p) => fmtClock(p.ts)))
const hasPower = computed(() => points.value.some((p) => p.power != null))

const cpuSeries = computed(() => [
  { name: 'CPU', color: '#0ea5e9', data: points.value.map((p) => p.cpu ?? 0), fill: true },
])
const memSeries = computed(() => [
  { name: '内存已用', color: '#8b5cf6', data: points.value.map((p) => (p.memUsed ?? 0) / 1024 ** 3), fill: true },
  { name: '磁盘已用', color: '#38bdf8', data: points.value.map((p) => (p.diskUsed ?? 0) / 1024 ** 3), fill: false },
])
const netSeries = computed(() => [
  { name: '上行', color: '#0ea5e9', data: points.value.map((p) => p.netOut ?? 0), fill: true },
  { name: '下行', color: '#8b5cf6', data: points.value.map((p) => p.netIn ?? 0), fill: true },
])
const powerSeries = computed(() => [
  { name: '功率', color: '#f59e0b', data: points.value.map((p) => p.power ?? 0), fill: true },
])

async function load() {
  ctrl?.abort()
  ctrl = new AbortController()
  status.value = 'loading'
  error.value = ''
  needLogin.value = false
  try {
    const raw = await monitorClient.history(nodeId.value, range.value, ctrl.signal)
    points.value = normalizePoints(raw)
    status.value = 'ready'
  } catch (e) {
    if (e?.name === 'AbortError' || e?.code === 'ABORTED') return
    status.value = 'error'
    // 7d 未开放/私有模式：后端 401，前端引导登录而非只显示错误码
    if (e?.code === 'UNAUTHORIZED') {
      needLogin.value = true
      error.value = e?.message || '该范围历史需登录后查看'
    } else if (e?.code === 'HTTP_ERROR' && /不存在/.test(e?.message || '')) {
      error.value = '节点不存在或已隐藏'
    } else {
      error.value = e?.message || '历史曲线加载失败'
    }
  }
}

function setRange(k) {
  if (range.value === k) return
  range.value = k
  router.replace({ query: { ...route.query, range: k } })
}

function goBack() {
  router.push('/')
}

function goLogin() {
  router.push({ path: '/admin/login', query: { redirect: route.fullPath } })
}

watch(range, load)
watch(nodeId, () => {
  points.value = []
  load()
})

onMounted(() => {
  if (!monitor._seeded) monitor.fetchAll()
  load()
})

onUnmounted(() => {
  ctrl?.abort()
})
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>{{ server ? server.name : `节点 ${nodeId}` }} · 历史曲线</h1>
        <p class="page-head__desc">
          {{ server ? `${server.region} · ${server.profile.os} · ${server.online ? '在线' : '离线'}` : '公开只读，不展示 IP 等敏感信息' }}
        </p>
      </div>
      <div class="page-head__actions">
        <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="goBack">
          <AppIcon name="arrow-left" aria-hidden="true" />返回总览
        </button>
      </div>
    </div>

    <div v-if="server" class="detail-card bt-enter" style="--i: 0">
      <NodeCard :server="server" :linkable="false" :last-updated="monitor.secondsSinceUpdate" />
    </div>

    <!-- range 切换：分段控件 + aria-pressed -->
    <div class="bt-seg detail-range" role="group" aria-label="历史范围">
      <button
        v-for="r in RANGE_META"
        :key="r.key"
        type="button"
        class="bt-seg__item"
        :aria-pressed="range === r.key"
        :title="r.hint"
        @click="setRange(r.key)"
      >
        {{ r.label }}
      </button>
    </div>

    <StateSkeleton v-if="status === 'loading'" :rows="4" />
    <div v-else-if="status === 'error' && needLogin" class="bt-card">
      <StateEmpty title="需登录查看" :desc="error || '长期历史默认仅管理员可见，登录后返回本页继续浏览。'">
        <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="goLogin">
          前往登录
        </button>
      </StateEmpty>
    </div>
    <StateError v-else-if="status === 'error'" :message="error" @retry="load" />
    <div v-else-if="!points.length" class="bt-card">
      <StateEmpty title="暂无历史数据" desc="该节点在此范围内还没有采样（新纳管或离线较久）。" />
    </div>

    <template v-else-if="status === 'ready'">
      <section class="bt-card trend-panel bt-enter" style="--i: 1" aria-label="CPU 历史">
        <div class="bt-card__head">
          <div class="bt-card__title">CPU 使用率 · {{ rangeLabel }}</div>
        </div>
        <div class="bt-card__body">
          <TrendChart :series="cpuSeries" :x-labels="xLabels" :height="220" label="CPU 历史曲线" :y-formatter="(v) => `${Math.round(v)}%`" />
        </div>
      </section>

      <section class="bt-card trend-panel bt-enter" style="--i: 2" aria-label="内存与磁盘历史">
        <div class="bt-card__head">
          <div class="bt-card__title">内存 / 磁盘已用 · {{ rangeLabel }}</div>
        </div>
        <div class="bt-card__body">
          <TrendChart :series="memSeries" :x-labels="xLabels" :height="200" label="内存磁盘历史曲线" :y-formatter="(v) => fmtSizeShort(v)" />
        </div>
      </section>

      <section class="bt-card trend-panel bt-enter" style="--i: 3" aria-label="吞吐历史">
        <div class="bt-card__head">
          <div class="bt-card__title">网络吞吐 · {{ rangeLabel }}</div>
        </div>
        <div class="bt-card__body">
          <TrendChart :series="netSeries" :x-labels="xLabels" :height="200" label="吞吐历史曲线" :y-formatter="fmtBps" />
        </div>
      </section>

      <section v-if="hasPower" class="bt-card trend-panel bt-enter" style="--i: 4" aria-label="功耗历史">
        <div class="bt-card__head">
          <div class="bt-card__title">整机功耗 · {{ rangeLabel }}</div>
        </div>
        <div class="bt-card__body">
          <TrendChart :series="powerSeries" :x-labels="xLabels" :height="180" label="功耗历史曲线" :y-formatter="fmtWatts" />
        </div>
      </section>
    </template>
  </div>
</template>
