// ============================================================
// BeaconTower · 监控数据 Store（Pinia）
// 替代旧 composables/useLiveServers.js：
// - 单一数据源：servers / netHistory / powerHistory / lastUpdated
// - 页面可见时才轮询（visibilitychange），卸载时清理定时器
// - 失败可重试：status 字段驱动空态 / 错误态 / 骨架屏
// 后端就绪后：把 tick() 内的数据源换成 SSE/HTTP，结果写入同一 state。
// ============================================================
import { defineStore } from 'pinia'
import { mockServers } from '../mock/data'

const REFRESH_MS = 10000
const HISTORY_LEN = 40

const jitter = (value, min, max, step) => {
  const next = value + (Math.random() * 2 - 1) * step
  return Math.min(max, Math.max(min, next))
}

const round2 = (v) => Math.round(v * 100) / 100

function cloneServers() {
  return mockServers.map((s) => ({
    ...s,
    metrics: { ...s.metrics },
    power: s.power
      ? { ...s.power, watts: { ...s.power.watts }, energy: s.power.energy ? { ...s.power.energy } : null }
      : null,
  }))
}

function samplePower(s) {
  const p = s.power
  const base = p.baseLoadW || 5
  const loadFactor = 1 + (s.metrics.cpu / 100) * 1.2
  const cpu = Math.max(
    0.3,
    jitter(p.watts.cpu || base * 0.35, 0.3, 28, 0.4) * loadFactor * 0.55 + (p.watts.cpu || 1) * 0.45,
  )
  const dram = Math.max(0.2, (p.watts.dram || 0.5) * jitter(1, 0.85, 1.15, 0.06))
  const total = p.rapl ? cpu + dram + base : 0
  p.watts.cpu = round2(cpu)
  p.watts.total = round2(total)
  return total
}

export const useMonitorStore = defineStore('monitor', {
  state: () => ({
    servers: cloneServers(),
    netUp: [],
    netDown: [],
    watts: [],
    secondsSinceUpdate: 0,
    status: 'loading', // loading | ready | error
    error: '',
    query: '',
    statusFilter: 'all', // all | online | offline
    _timer: 0,
    _clock: 0,
    _seeded: false,
    _boundVisibility: false,
  }),

  getters: {
    summary(state) {
      const online = state.servers.filter((s) => s.online)
      const measured = online.filter((s) => s.power?.rapl)
      return {
        total: state.servers.length,
        online: online.length,
        offline: state.servers.length - online.length,
        upBps: online.reduce((a, s) => a + s.metrics.netUp, 0),
        downBps: online.reduce((a, s) => a + s.metrics.netDown, 0),
        watts: measured.reduce((a, s) => a + s.power.watts.total, 0),
        measuredCount: measured.length,
        monthKwh: measured.reduce((a, s) => a + (s.power.energy?.monthKwh ?? 0), 0),
        estCostMonth: measured.reduce((a, s) => a + (s.power.energy?.estCostMonth ?? 0), 0),
      }
    },

    filteredServers(state) {
      const q = state.query.trim().toLowerCase()
      return state.servers.filter((s) => {
        if (state.statusFilter === 'online' && !s.online) return false
        if (state.statusFilter === 'offline' && s.online) return false
        if (!q) return true
        return (
          s.name.toLowerCase().includes(q) ||
          (s.region || '').toLowerCase().includes(q) ||
          (s.tags || []).join(' ').toLowerCase().includes(q)
        )
      })
    },
  },

  actions: {
    start() {
      if (this._timer) return
      this.tick(true)
      this._timer = window.setInterval(() => {
        if (document.hidden) return // 不可见标签页暂停，省电省请求
        this.tick()
      }, REFRESH_MS)
      this._clock = window.setInterval(() => {
        this.secondsSinceUpdate += 1
      }, 5000)
      if (!this._boundVisibility) {
        this._boundVisibility = true
        document.addEventListener('visibilitychange', this._onVisibility)
      }
    },

    stop() {
      window.clearInterval(this._timer)
      window.clearInterval(this._clock)
      this._timer = 0
      this._clock = 0
      document.removeEventListener('visibilitychange', this._onVisibility)
      this._boundVisibility = false
    },

    _onVisibility() {
      // 回到可见时立即刷新一次，避免展示过期数据
      if (!document.hidden) this.tick()
    },

    setQuery(q) {
      this.query = q
    },

    setStatusFilter(f) {
      this.statusFilter = f
    },

    retry() {
      this.status = 'loading'
      this.error = ''
      this.tick(true)
    },

    tick(first = false) {
      try {
        this.secondsSinceUpdate = 0
        for (const s of this.servers) {
          if (!s.online) continue
          const m = s.metrics
          const cpu = Math.round(jitter(m.cpu, 2, 97, 7))
          m.cpu = cpu
          m.memUsed = Math.min(m.memTotal, Math.max(0.1, jitter(m.memUsed, 0.1, m.memTotal, 0.08)))
          m.netUp = Math.max(0, jitter(m.netUp, 0, m.netUp * 1.8 + 50_000, m.netUp * 0.25 + 5_000))
          m.netDown = Math.max(0, jitter(m.netDown, 0, m.netDown * 1.8 + 200_000, m.netDown * 0.25 + 20_000))
          m.tcp = Math.max(1, Math.round(jitter(m.tcp, 5, m.tcp * 1.6 + 40, 15)))
          s.cpuHistory = [...s.cpuHistory.slice(1), cpu]

          if (s.power?.enabled && s.power.rapl) {
            const total = samplePower(s)
            s.power.tempC = Math.round(jitter(s.power.tempC ?? 45, 38, 72, 1.2) * 10) / 10
            s.powerHistory = [...(s.powerHistory || []).slice(1), total]
            const e = s.power.energy
            if (e) {
              e.todayKwh = Math.round((e.todayKwh + (total * (REFRESH_MS / 1000)) / 3_600_000) * 10000) / 10000
              e.estCostToday = Math.round(e.todayKwh * e.pricePerKwh * 100) / 100
            }
          }
        }

        const online = this.servers.filter((s) => s.online)
        const upTotal = online.reduce((a, s) => a + s.metrics.netUp, 0)
        const downTotal = online.reduce((a, s) => a + s.metrics.netDown, 0)
        const wattsTotal = online.reduce((a, s) => a + (s.power?.rapl ? s.power.watts.total : 0), 0)

        if (!this._seeded || first) {
          const wave = (base, amp) =>
            Array.from({ length: HISTORY_LEN }, (_, i) =>
              Math.max(0, base + Math.sin(i / 3.2) * amp * 0.6 + (Math.random() * 2 - 1) * amp * 0.5),
            )
          this.netUp = wave(upTotal, upTotal * 0.35 + 20_000)
          this.netDown = wave(downTotal, downTotal * 0.3 + 60_000)
          this.watts = wave(Math.max(6, wattsTotal), wattsTotal * 0.2 + 2)
          this._seeded = true
        } else {
          this.netUp = [...this.netUp.slice(1), upTotal]
          this.netDown = [...this.netDown.slice(1), downTotal]
          this.watts = [...this.watts.slice(1), wattsTotal]
        }
        this.status = 'ready'
      } catch (e) {
        this.status = 'error'
        this.error = e?.message || '数据刷新失败'
      }
    },
  },
})
