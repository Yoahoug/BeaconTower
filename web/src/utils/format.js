// 数值格式化工具：后端接入后继续复用

export function fmtBps(bps) {
  if (!bps || bps < 0) return '0 B/s'
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  let i = 0
  let v = bps
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

export function fmtGB(gb, digits = 1) {
  if (gb >= 1024) return `${(gb / 1024).toFixed(2)} TB`
  return `${gb.toFixed(digits)} GB`
}

// 紧凑容量写法（仪表盘标签用）：2G / 340G / 1.2T
export function fmtSizeShort(gb) {
  if (gb >= 1024) return `${(gb / 1024).toFixed(1)}T`
  return `${Math.round(gb)}G`
}

export function fmtUptime(days) {
  if (days <= 0) return '—'
  if (days < 1) return '不足 1 天'
  if (days < 365) return `${Math.floor(days)} 天`
  const years = Math.floor(days / 365)
  const rest = Math.floor(days % 365)
  return `${years} 年 ${rest} 天`
}

export function agoText(seconds) {
  if (seconds < 15) return '刚刚'
  if (seconds < 60) return `${Math.round(seconds / 5) * 5} 秒前`
  return `${Math.floor(seconds / 60)} 分钟前`
}
