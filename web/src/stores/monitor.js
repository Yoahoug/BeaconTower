// ============================================================
// BeaconTower · 监控数据 Store（Pinia）
// 数据源：后端公开 API（GET /api/v1/public/servers 首屏+保底轮询，
// GET /api/v1/public/stream SSE 增量，SSE 断线自动回退轮询）。
// 后端字段（doc/04 §1.2）→ 卡片字段的映射见 adaptServer()。
// ============================================================
import { defineStore } from 'pinia'
import { http } from '../api/http'

const REFRESH_MS = 10000
const HISTORY_LEN = 40
const SSE_RETRY_BASE_MS = 3000
const SSE_RETRY_MAX_MS = 30000

// 后端公开 metrics（bps/字节/秒）→ 卡片字段（GB/天/单数）的换算
const GB = 1024 ** 3
const DAY_S = 86400

function num(v, d = 0) {
  const n = Number(v)
  return Number.isFinite(n) ? n : d
}

function str(v, d = '') {
  return typeof v === 'string' ? v : d
}

function arr(v) {
  return Array.isArray(v) ? v.filter((x) => typeof x === 'string') : []
}

// 后端单节点 → 卡片形态（含 40 点走势窗口的本地维护）
function adaptServer(raw, prev) {
  const m = raw.metrics || {}
  const p = raw.profile || {}
  const online = raw.status === 'online'
  const memTotalGB = num(p.mem_total, 0) / GB
  const diskTotalGB = num(p.disk_total, 0) / GB
  const cpu = online ? Math.round(num(m.cpu_pct, 0)) : 0
  const pw = raw.power || null
  const rapl = !!pw
  const cpuHistory = prev?.cpuHistory?.slice() || []
  const powerHistory = prev?.powerHistory?.slice() || []
  if (online) {
    cpuHistory.push(cpu)
    while (cpuHistory.length > HISTORY_LEN) cpuHistory.shift()
    if (rapl) {
      powerHistory.push(num(pw.total_w, 0))
      while (powerHistory.length > HISTORY_LEN) powerHistory.shift()
    }
  }
  return {
    id: raw.id,
    name: str(raw.name, `节点 ${raw.id}`),
    region: str(raw.region, '未定位'),
    regionSource: str(raw.region_source, 'auto'),
    tags: arr(raw.tags),
    online,
    profile: {
      os: [str(p.os), str(p.os_version)].filter(Boolean).join(' ') || '—',
      arch: str(p.arch, '—'),
      cores: num(p.cpu_cores, 0),
      virt: str(p.virt, '—'),
    },
    metrics: {
      cpu,
      memUsed: online ? num(m.mem_used, 0) / GB : 0,
      memTotal: memTotalGB,
      diskUsed: online ? num(m.disk_used, 0) / GB : 0,
      diskTotal: diskTotalGB,
      netUp: online ? num(m.net_out_bps, 0) : 0,
      netDown: online ? num(m.net_in_bps, 0) : 0,
      tcp: online ? Math.round(num(m.tcp_conns, 0)) : 0,
      udp: online ? Math.round(num(m.udp_conns, 0)) : 0,
      load: [num(m.load1, 0), num(m.load5, 0), num(m.load15, 0)],
      uptimeDays: online ? num(m.uptime_s, 0) / DAY_S : 0,
      processes: online ? Math.round(num(m.processes, 0)) : 0,
    },
    power: {
      enabled: true,
      rapl,
      powerSource: str(pw?.power_source, 'ac'),
      watts: { total: num(pw?.total_w, 0), cpu: num(pw?.cpu_w, 0) },
      baseLoadW: 0,
      baseLoadSource: 'default',
      tempC: pw?.temp_c ?? null,
      freqMhz: num(pw?.freq_mhz, 0),
      energy: rapl
        ? {
            todayKwh: pw?.today_kwh ?? 0,
            weekKwh: 0,
            monthKwh: num(pw?.month_kwh, 0),
            avgWatts: num(pw?.total_w, 0),
            estCostToday: 0,
            estCostMonth: num(pw?.est_cost_month, 0),
            pricePerKwh: 0,
          }
        : null,
    },
    powerHistory,
    cpuHistory,
    offlineSince: online ? '' : '采集失联',
  }
}

export const useMonitorStore = defineStore('monitor', {
  state: () => ({
    servers: [],
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
    _sse: null,
    _sseRetryMs: SSE_RETRY_BASE_MS,
    _sseTimer: 0,
    _pollTimer: 0,
    _polling: false,
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
      this.fetchAll(true)
      this._timer = window.setInterval(() => {
        if (document.hidden) return // 不可见标签页暂停，省电省请求
        // SSE 存活时轮询退居二线（保底对齐）；SSE 断线才高频轮询
        if (!this._sse) this.fetchAll()
      }, REFRESH_MS)
      this._clock = window.setInterval(() => {
        this.secondsSinceUpdate += 1
      }, 5000)
      if (!this._boundVisibility) {
        this._boundVisibility = true
        document.addEventListener('visibilitychange', this._onVisibility)
      }
      this.connectSSE()
    },

    stop() {
      window.clearInterval(this._timer)
      window.clearInterval(this._clock)
      window.clearTimeout(this._sseTimer)
      this._timer = 0
      this._clock = 0
      this._sseTimer = 0
      this.closeSSE()
      document.removeEventListener('visibilitychange', this._onVisibility)
      this._boundVisibility = false
    },

    _onVisibility() {
      // 回到可见时立即刷新一次，避免展示过期数据
      if (!document.hidden) this.fetchAll()
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
      this.fetchAll(true)
      this.connectSSE()
    },

    // 全量拉取（首屏/可见恢复/SSE 断线保底）
    async fetchAll(first = false) {
      if (this._polling) return
      this._polling = true
      try {
        const list = await http.get('/v1/public/servers')
        this.applyList(Array.isArray(list) ? list : [], first)
        if (this.status !== 'ready') this.status = 'ready'
        this.error = ''
      } catch (e) {
        if (!this._seeded) {
          this.status = 'error'
          this.error = e?.message || '数据加载失败'
        }
      } finally {
        this._polling = false
      }
    },

    applyList(list, first = false) {
      this.secondsSinceUpdate = 0
      const prevById = new Map(this.servers.map((s) => [s.id, s]))
      this.servers = list.map((raw) => adaptServer(raw, prevById.get(raw.id)))
      const upTotal = this.servers.filter((s) => s.online).reduce((a, s) => a + s.metrics.netUp, 0)
      const downTotal = this.servers.filter((s) => s.online).reduce((a, s) => a + s.metrics.netDown, 0)
      const wattsTotal = this.servers
        .filter((s) => s.online && s.power?.rapl)
        .reduce((a, s) => a + s.power.watts.total, 0)
      if (!this._seeded || first) {
        this.netUp = Array(HISTORY_LEN).fill(upTotal)
        this.netDown = Array(HISTORY_LEN).fill(downTotal)
        this.watts = Array(HISTORY_LEN).fill(wattsTotal)
        this._seeded = true
      } else {
        this.netUp = [...this.netUp.slice(1), upTotal]
        this.netDown = [...this.netDown.slice(1), downTotal]
        this.watts = [...this.watts.slice(1), wattsTotal]
      }
      this.status = 'ready'
    },

    // SSE 订阅（增量 update；失败指数退避重连，退避期间靠 10s 轮询保底）
    connectSSE() {
      if (this._sse || typeof EventSource === 'undefined') return
      let es
      try {
        es = new EventSource('/api/v1/public/stream')
      } catch {
        return
      }
      this._sse = es
      es.addEventListener('snapshot', (ev) => {
        try {
          const list = JSON.parse(ev.data)
          if (Array.isArray(list)) this.applyList(list)
        } catch {
          /* 坏帧忽略，等下次 update/轮询 */
        }
        this._sseRetryMs = SSE_RETRY_BASE_MS
      })
      es.addEventListener('update', (ev) => {
        try {
          const list = JSON.parse(ev.data)
          if (Array.isArray(list)) this.applyList(list)
        } catch {
          /* 坏帧忽略 */
        }
      })
      es.onerror = () => {
        this.closeSSE()
        window.clearTimeout(this._sseTimer)
        this._sseTimer = window.setTimeout(() => {
          this._sseTimer = 0
          this.connectSSE()
          this.fetchAll()
        }, this._sseRetryMs)
        this._sseRetryMs = Math.min(this._sseRetryMs * 2, SSE_RETRY_MAX_MS)
      }
    },

    closeSSE() {
      try {
        this._sse?.close()
      } catch {
        /* 忽略 */
      }
      this._sse = null
    },
  },
})
