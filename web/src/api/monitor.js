// ============================================================
// BeaconTower · 监控 API（公开只读门面）
// 唯一出口 http.js。节点详情页经此取历史曲线；总览快照走 Pinia store。
// ============================================================
import { http } from './http'

export const RANGE_META = [
  { key: '1h', label: '近 1 小时', hint: '原始采样' },
  { key: '6h', label: '近 6 小时', hint: '原始采样' },
  { key: '24h', label: '近 24 小时', hint: '原始采样' },
  { key: '7d', label: '近 7 天', hint: '小时聚合·或需登录' },
]

function withTimeout(promise, ms = 15000) {
  let timer = 0
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error('请求超时，请重试')), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

function num(v, d = 0) {
  const n = Number(v)
  return Number.isFinite(n) ? n : d
}

// points 归一化：短期（ts/cpu_pct/mem_used/disk_used/net_in_out_bps/power_w）
// 与 7d 聚合（ts/cpu_avg/cpu_max/mem_avg/net_in_out_avg/power_avg/kwh）统一为
// { ts, cpu, memUsed, diskUsed, netIn, netOut, power }，缺失为 null。
export function normalizePoints(raw) {
  const list = Array.isArray(raw?.points) ? raw.points : []
  return list
    .map((p) => {
      const ts = num(p.ts, 0)
      if (!ts) return null
      const cpu = p.cpu_pct ?? p.cpu_avg ?? null
      const memUsed = p.mem_used ?? p.mem_avg ?? null
      const diskUsed = p.disk_used ?? null
      const netIn = p.net_in_bps ?? p.net_in_avg ?? null
      const netOut = p.net_out_bps ?? p.net_out_avg ?? null
      const power = p.power_w ?? p.power_avg ?? null
      return {
        ts,
        cpu: cpu == null ? null : num(cpu, 0),
        memUsed: memUsed == null ? null : num(memUsed, 0),
        diskUsed: diskUsed == null ? null : num(diskUsed, 0),
        netIn: netIn == null ? null : num(netIn, 0),
        netOut: netOut == null ? null : num(netOut, 0),
        power: power == null ? null : num(power, 0),
      }
    })
    .filter(Boolean)
    .sort((a, b) => a.ts - b.ts)
}

export const monitorClient = {
  history: (id, range = '1h', signal) =>
    withTimeout(http.get(`/v1/public/servers/${id}/history?range=${range}`, { signal })),
  servers: (signal) => withTimeout(http.get('/v1/public/servers', { signal })),
}
