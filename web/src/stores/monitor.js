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
// 穿透摘要由面板后台每 3 分钟同步一次平台侧，访客页跟着 10s 轮询毫无意义，
// 单独给一条低频时间线；只在访客页挂载时跑（见 startFrp/stopFrp）。
const FRP_REFRESH_MS = 60000
// WG 组网摘要是巡检（5min ticker）回填的静态库表，同样走 60s 低频轮询
//（见 startWg/stopWg），公开端点不做任何 SSH 实时探测。
const WG_REFRESH_MS = 60000

// 离线快照（doc/15 §离线体验）：最后一次成功的公开全量数据落 localStorage。
// 选 localStorage 而非 IndexedDB：快照是单键 JSON（几十 KB 级），同步读写
// 无 await 开销，启动路径零改造；IndexedDB 的事务/版本管理对这个量级是过度设计。
// 断网打开 → 先渲染旧快照再刷 SSE，显示「数据时间」，不白屏。
const OFFLINE_CACHE_KEY = 'beacontower.snapshot.v1'
const OFFLINE_CACHE_MAX_AGE_S = 7 * 86400 // 超过一周的旧快照不再展示（避免误导）

// 后端公开 metrics 速率字段（net_*_bps）实际是「字节/秒」（/proc/net/dev 字节
// 计数器差分），前端按 B/s 展示 → 卡片字段（GB/天/单数）的换算见下
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
    isSelf: !!raw.is_self,
    region: str(raw.region, '未定位'),
    regionSource: str(raw.region_source, 'auto'),
    tags: arr(raw.tags),
    online,
    profile: {
      os: [str(p.os), str(p.os_version)].filter(Boolean).join(' ') || '—',
      arch: str(p.arch, '—'),
      cores: num(p.cpu_cores, 0),
      virt: str(p.virt, '—'),
      // 流量口径：采集侧实际统计的网卡名（物理口列表或退化时的单接口；空＝未知）
      netIfaces: str(p.net_ifaces, ''),
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
            // 后端今日 kWh 尚未接入（恒为 null）→ 保持 null，卡片显示「—」，
            // 不要兜成 0 让 RAPL 机器都写着「今日 0.000 kWh」
            todayKwh: pw?.today_kwh ?? null,
            weekKwh: 0,
            monthKwh: num(pw?.month_kwh, 0),
            avgWatts: num(pw?.total_w, 0),
            estCostToday: 0,
            // 关闭「公开页电费」时后端不下发该字段 → null（而非 ¥0.00）
            estCostMonth: pw?.est_cost_month ?? null,
            pricePerKwh: 0,
          }
        : null,
    },
    powerHistory,
    cpuHistory,
    // 今日流量（按日记录，字节；无记录为 null。离线节点保留当日已累计值）
    trafficToday: raw.traffic_today
      ? { in: num(raw.traffic_today.in_total, 0), out: num(raw.traffic_today.out_total, 0) }
      : null,
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

    // 离线快照标记：stale=true 表示当前数据来自 localStorage 快照而非实时接口，
    // 顶栏据此显示「离线 · 数据时间」提示（恢复在线后自动消失）
    stale: false,
    staleAt: 0, // 快照抓取时刻（秒级时间戳）

    // 会话失效标记：private_mode/会话过期时公开页错误态据此显示「前往登录」
    needLogin: false,

    // 内网穿透摘要（公开接口 /v1/public/frp，字段白名单见 doc/13 §12）
    frp: null, // { platforms: [], summary: {} }，未拉到时为 null
    frpStatus: 'idle', // idle | loading | ready | error
    frpError: '',
    _frpTimer: 0,
    _frpBusy: false,

    // WG 组网摘要（公开接口 /v1/public/wg，字段白名单见 doc/04 §1.6）
    wg: null, // { initialized, hubs: [], peers: [], summary: {} }，未拉到时为 null
    wgStatus: 'idle', // idle | loading | ready | error
    wgError: '',
    _wgTimer: 0,
    _wgBusy: false,

    _timer: 0,
    _clock: 0,
    _seeded: false,
    _boundVisibility: false,
    _sse: null,
    _sseRetryMs: SSE_RETRY_BASE_MS,
    _sseTimer: 0,
    _lastEventAt: 0,
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
        // 是否至少有一个节点下发了电费（关闭「公开页电费」时全部为 null → 不展示 ¥0.00）
        costMeasured: measured.some((s) => s.power.energy?.estCostMonth != null),
        // 今日全网流量（字节；任一节点有按日记录即计）
        dayIn: state.servers.reduce((a, s) => a + (s.trafficToday?.in ?? 0), 0),
        dayOut: state.servers.reduce((a, s) => a + (s.trafficToday?.out ?? 0), 0),
        dayMeasured: state.servers.some((s) => s.trafficToday),
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
      this._lastEventAt = Date.now() // 首轮不因 0 值误判 stale（首屏本就 fetchAll）
      this.loadOfflineSnapshot() // 断网也能先渲染旧数据，再被实时数据覆盖
      this.fetchAll(true)
      this._timer = window.setInterval(() => {
        if (document.hidden) return // 不可见标签页暂停，省电省请求
        // SSE 活着时轮询退居二线；但 SSE 经反代/中间设备被静默断流时
        // EventSource 的 readyState 仍是 OPEN、onerror 也不触发，页面会停在
        // 首屏数据不动，故用「多久没收到事件」判活，超时就补一次全量
        const stale = Date.now() - this._lastEventAt > REFRESH_MS * 2.5
        if (!this._sse || stale) this.fetchAll()
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
      this.stopFrp()
      this.stopWg()
      this.closeSSE()
      document.removeEventListener('visibilitychange', this._onVisibility)
      this._boundVisibility = false
    },

    _onVisibility() {
      if (document.hidden) {
        // PWA 独立窗口（doc/15）：iOS 切后台不保证触发 SSE 的 onerror，
        // 主动断开防止连接堆积（回到前台重连）；浏览器标签页语义相同。
        this.closeSSE()
        return
      }
      // 回到可见：先看 SSE 是否还活着（_sse 为空 = 切后台时被主动断开），
      // 立即重连 + 刷新一次，避免展示过期数据
      if (!this._sse) this.connectSSE()
      this.fetchAll()
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
      this.needLogin = false
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
        this.needLogin = false
        this.stale = false
        this.saveOfflineSnapshot(list)
      } catch (e) {
        // 会话失效（private_mode/过期）：标记需登录，公开页错误态据此给
        // 「前往登录」入口（此前只显示裸错误块，整站不可用）
        this.needLogin = e?.code === 'UNAUTHORIZED' && e?.status !== 1005 ? true : this.needLogin
        if (!this._seeded) {
          // 有离线快照垫底就不打错误页：旧数据好过白屏（doc/15）
          if (this.stale) {
            this.status = 'ready'
            this.error = ''
          } else {
            this.status = 'error'
            this.error = e?.message || '数据加载失败'
          }
        }
      } finally {
        this._polling = false
      }
    },

    // ---------- 离线快照（localStorage，见文件头说明） ----------

    saveOfflineSnapshot(rawList) {
      if (!Array.isArray(rawList) || !rawList.length) return
      try {
        localStorage.setItem(
          OFFLINE_CACHE_KEY,
          JSON.stringify({ ts: Math.floor(Date.now() / 1000), servers: rawList }),
        )
      } catch {
        /* 隐私模式/配额满：快照是增强功能，失败静默 */
      }
    },

    loadOfflineSnapshot() {
      let snap
      try {
        snap = JSON.parse(localStorage.getItem(OFFLINE_CACHE_KEY) || 'null')
      } catch {
        snap = null
      }
      if (!snap || !Array.isArray(snap.servers) || !snap.servers.length) return
      const ageS = Math.floor(Date.now() / 1000) - (snap.ts || 0)
      if (ageS < 0 || ageS > OFFLINE_CACHE_MAX_AGE_S) {
        try {
          localStorage.removeItem(OFFLINE_CACHE_KEY)
        } catch {
          /* 忽略 */
        }
        return
      }
      this.applyList(snap.servers, true)
      // 快照垫底：标记 stale（顶栏显示「离线 · 数据时间」），实时数据到达后清除
      this.stale = true
      this.staleAt = snap.ts
      // 快照里的走势窗口只有历史窗口尾帧，补间直接用快照值
      this.secondsSinceUpdate = Math.min(ageS, 3600)
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

    // ---------- 内网穿透摘要（公开只读） ----------

    startFrp() {
      if (this._frpTimer) return
      this.fetchFrp()
      this._frpTimer = window.setInterval(() => {
        if (document.hidden) return
        this.fetchFrp()
      }, FRP_REFRESH_MS)
    },

    stopFrp() {
      window.clearInterval(this._frpTimer)
      this._frpTimer = 0
    },

    retryFrp() {
      this.frpStatus = 'loading'
      this.frpError = ''
      this.needLogin = false
      this.fetchFrp()
    },

    async fetchFrp() {
      if (this._frpBusy) return
      this._frpBusy = true
      if (!this.frp) this.frpStatus = 'loading'
      try {
        const d = await http.get('/v1/public/frp')
        this.frp = {
          platforms: Array.isArray(d?.platforms) ? d.platforms : [],
          summary: d?.summary || {},
        }
        this.frpStatus = 'ready'
        this.frpError = ''
      } catch (e) {
        // 会话失效标记（private_mode 下公开页给登录入口）
        this.needLogin = e?.code === 'UNAUTHORIZED' && e?.status !== 1005 ? true : this.needLogin
        // 已有数据时静默失败：穿透摘要抖一下不该把整页打成错误态
        if (!this.frp) {
          this.frpStatus = 'error'
          this.frpError = e?.message || '穿透数据加载失败'
        }
      } finally {
        this._frpBusy = false
      }
    },

    // ---------- WG 组网摘要（公开只读） ----------

    startWg() {
      if (this._wgTimer) return
      this.fetchWg()
      this._wgTimer = window.setInterval(() => {
        if (document.hidden) return
        this.fetchWg()
      }, WG_REFRESH_MS)
    },

    stopWg() {
      window.clearInterval(this._wgTimer)
      this._wgTimer = 0
    },

    retryWg() {
      this.wgStatus = 'loading'
      this.wgError = ''
      this.needLogin = false
      this.fetchWg()
    },

    async fetchWg() {
      if (this._wgBusy) return
      this._wgBusy = true
      if (!this.wg) this.wgStatus = 'loading'
      try {
        const d = await http.get('/v1/public/wg')
        this.wg = {
          initialized: !!d?.initialized,
          hubs: Array.isArray(d?.hubs) ? d.hubs : [],
          peers: Array.isArray(d?.peers) ? d.peers : [],
          summary: d?.summary || {},
        }
        this.wgStatus = 'ready'
        this.wgError = ''
      } catch (e) {
        // 会话失效标记（private_mode 下公开页给登录入口）
        this.needLogin = e?.code === 'UNAUTHORIZED' && e?.status !== 1005 ? true : this.needLogin
        // 已有数据时静默失败：巡检口径的数据抖一下不该把整页打成错误态
        if (!this.wg) {
          this.wgStatus = 'error'
          this.wgError = e?.message || '组网数据加载失败'
        }
      } finally {
        this._wgBusy = false
      }
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
        this._lastEventAt = Date.now()
        try {
          const list = JSON.parse(ev.data)
          if (Array.isArray(list)) this.applyList(list)
        } catch {
          /* 坏帧忽略，等下次 update/轮询 */
        }
        this._sseRetryMs = SSE_RETRY_BASE_MS
      })
      es.addEventListener('update', (ev) => {
        this._lastEventAt = Date.now()
        try {
          const list = JSON.parse(ev.data)
          if (Array.isArray(list)) this.applyList(list)
        } catch {
          /* 坏帧忽略 */
        }
      })
      es.onopen = () => {
        this._lastEventAt = Date.now()
        this._sseRetryMs = SSE_RETRY_BASE_MS
      }
      // 后端 15s 一条 ping 帧，也证明链路活着：采集间隔调大到 5 分钟时，
      // 仅靠 update 判活会让每个 10s tick 都误判 stale 退化为全量轮询
      //（后端已把 ": ping" 注释帧改为 "event: ping" 命名帧，见 public.go）
      es.addEventListener('ping', () => {
        this._lastEventAt = Date.now()
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
