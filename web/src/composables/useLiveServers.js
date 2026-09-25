import { computed, reactive } from 'vue'
import { mockServers } from '../mock/data'

// 模拟实时指标：每 10s 静默刷新一次（无数字补间 —— 监控面板要沉稳，
// 数字持续跳动反而分散注意力）。历史窗口同步推进。
// 后端接入后，本文件整体替换为 SSE 订阅即可，组件层不用动。

const REFRESH_MS = 10000

const jitter = (value, min, max, step) => {
  const next = value + (Math.random() * 2 - 1) * step
  return Math.min(max, Math.max(min, next))
}

const servers = reactive(mockServers.map((s) => ({ ...s, metrics: { ...s.metrics } })))

// 全网吞吐滚动窗口（英雄面板双线走势图用）
const netHistory = reactive({ up: [], down: [] })
let firstRun = true
let secondsSinceUpdate = 0
const lastUpdated = reactive({ seconds: 0 })

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
  }

  // 全网吞吐入滚动窗口（首跑用带噪声的波形铺满窗口，避免开局两条直线）
  const online = servers.filter((s) => s.online)
  const upTotal = online.reduce((acc, s) => acc + s.metrics.netUp, 0)
  const downTotal = online.reduce((acc, s) => acc + s.metrics.netDown, 0)
  if (firstRun) {
    const wave = (base, amp) =>
      Array.from({ length: 40 }, (_, i) =>
        Math.max(0, base + Math.sin(i / 3.2) * amp * 0.6 + (Math.random() * 2 - 1) * amp * 0.5)
      )
    netHistory.up = wave(upTotal, upTotal * 0.35 + 20_000)
    netHistory.down = wave(downTotal, downTotal * 0.3 + 60_000)
    firstRun = false
  } else {
    netHistory.up = [...netHistory.up.slice(1), upTotal]
    netHistory.down = [...netHistory.down.slice(1), downTotal]
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
    return {
      total: servers.length,
      online: online.length,
      offline: servers.length - online.length,
      upBps: online.reduce((acc, s) => acc + s.metrics.netUp, 0),
      downBps: online.reduce((acc, s) => acc + s.metrics.netDown, 0),
    }
  })

  return { servers, summary, lastUpdated, netHistory }
}
