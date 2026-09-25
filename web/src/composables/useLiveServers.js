import { computed, reactive } from 'vue'
import { mockServers } from '../mock/data'

// 模拟实时指标：每 10s 静默刷新一次（无数字补间 —— 监控面板要沉稳，
// 数字持续跳动反而分散注意力）。历史窗口同步推进。
// 后端接入后，本文件整体替换为 SSE 订阅即可，组件层不用动。
//
// 功耗模拟对齐 power-monitor 参考实现：RAPL 多域功率 + EMA 平滑 +
// kWh 梯形积分统计。原型阶段由 CPU 负载驱动功率抖动（负载高 → 功率高）。

const REFRESH_MS = 10000

const jitter = (value, min, max, step) => {
  const next = value + (Math.random() * 2 - 1) * step
  return Math.min(max, Math.max(min, next))
}

const servers = reactive(mockServers.map((s) => ({ ...s, metrics: { ...s.metrics }, power: s.power ? { ...s.power, watts: { ...s.power.watts }, energy: s.power.energy ? { ...s.power.energy } : null } : null })))

// 全网吞吐滚动窗口（英雄面板双线走势图用）
const netHistory = reactive({ up: [], down: [] })
// 全网功率滚动窗口（英雄面板功耗走势图用，单线）
const powerHistory = reactive({ watts: [] })
let firstRun = true
let secondsSinceUpdate = 0
const lastUpdated = reactive({ seconds: 0 })

// CPU 负载 → 功率放大系数（空闲 ≈1，满载 ≈2.2），再叠小幅噪声模拟 RAPL 读数
function samplePower(s) {
  const p = s.power
  const base = p.baseLoadW || 5
  const loadFactor = 1 + (s.metrics.cpu / 100) * 1.2
  const cpu = Math.max(0.3, jitter(p.watts.cpu || base * 0.35, 0.3, 28, 0.4) * loadFactor * 0.55 + (p.watts.cpu || 1) * 0.45)
  const dram = Math.max(0.2, (p.watts.dram || 0.5) * jitter(1, 0.85, 1.15, 0.06))
  const total = p.rapl ? cpu + dram + base : 0
  p.watts.cpu = round2(cpu)
  p.watts.total = round2(total)
  return total
}

const round2 = (v) => Math.round(v * 100) / 100

function refreshMetrics() {
  secondsSinceUpdate = 0

  for (const s of servers) {
    if (!s.online) continue
    const m = s.metrics
    const cpu = Math.round(jitter(m.cpu, 2, 97, 7))
    m.cpu = cpu
    m.memUsed = Math.min(m.memTotal, Math.max(0.1, jitter(m.memUsed, 0.1, m.memTotal, 0.08)))
    m.netUp = Math.max(0, jitter(m.netUp, 0, m.netUp * 1.8 + 50_000, m.netUp * 0.25 + 5_000))
    m.netDown = Math.max(0, jitter(m.netDown, 0, m.netDown * 1.8 + 200_000, m.netDown * 0.25 + 20_000))
    m.tcp = Math.max(1, Math.round(jitter(m.tcp, 5, m.tcp * 1.6 + 40, 15)))
    s.cpuHistory = [...s.cpuHistory.slice(1), cpu]

    // 功耗：可用节点采样并推进窗口；kWh 累计按平均功率 × 10s 折算
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

  // 全网吞吐入滚动窗口（首跑用带噪声的波形铺满窗口，避免开局两条直线）
  const online = servers.filter((s) => s.online)
  const upTotal = online.reduce((acc, s) => acc + s.metrics.netUp, 0)
  const downTotal = online.reduce((acc, s) => acc + s.metrics.netDown, 0)
  const wattsTotal = online.reduce((acc, s) => acc + (s.power?.rapl ? s.power.watts.total : 0), 0)
  if (firstRun) {
    const wave = (base, amp) =>
      Array.from({ length: 40 }, (_, i) =>
        Math.max(0, base + Math.sin(i / 3.2) * amp * 0.6 + (Math.random() * 2 - 1) * amp * 0.5)
      )
    netHistory.up = wave(upTotal, upTotal * 0.35 + 20_000)
    netHistory.down = wave(downTotal, downTotal * 0.3 + 60_000)
    powerHistory.watts = wave(Math.max(6, wattsTotal), wattsTotal * 0.2 + 2)
    firstRun = false
  } else {
    netHistory.up = [...netHistory.up.slice(1), upTotal]
    netHistory.down = [...netHistory.down.slice(1), downTotal]
    powerHistory.watts = [...powerHistory.watts.slice(1), wattsTotal]
  }
}

function tickUptime() {
  secondsSinceUpdate += 1
  lastUpdated.seconds = secondsSinceUpdate
}

refreshMetrics()
setInterval(refreshMetrics, REFRESH_MS)
setInterval(tickUptime, 5000) // 5s 一跳即可，不必每秒跳

export function useLiveServers() {
  const summary = computed(() => {
    const online = servers.filter((s) => s.online)
    // 功耗汇总口径与吞吐一致：仅统计能读到功率的节点
    const measured = online.filter((s) => s.power?.rapl)
    const monthKwh = measured.reduce((acc, s) => acc + (s.power.energy?.monthKwh ?? 0), 0)
    const estCostMonth = measured.reduce((acc, s) => acc + (s.power.energy?.estCostMonth ?? 0), 0)
    return {
      total: servers.length,
      online: online.length,
      offline: servers.length - online.length,
      upBps: online.reduce((acc, s) => acc + s.metrics.netUp, 0),
      downBps: online.reduce((acc, s) => acc + s.metrics.netDown, 0),
      watts: measured.reduce((acc, s) => acc + s.power.watts.total, 0),
      measuredCount: measured.length,
      monthKwh,
      estCostMonth,
    }
  })

  return { servers, summary, lastUpdated, netHistory, powerHistory }
}
