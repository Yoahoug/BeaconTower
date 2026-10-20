/**
 * BeaconTower · toy 端格式化工具（纯函数，无数据依赖）
 * 从 mock.ts 拆出：组件只 import 这里，保证生产 bundle 不含仿真引擎。
 */

export function formatBytes(bytes: number, decimals = 1): string {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`;
}

export function formatBps(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec === 0) return '0 B/s';
  const k = 1024;
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
  const i = Math.floor(Math.log(bytesPerSec) / Math.log(k));
  return `${parseFloat((bytesPerSec / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

export function formatUptime(seconds: number): string {
  if (!seconds || seconds <= 0) return '已离线';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return `${days}天${hours}时`;
  const mins = Math.floor((seconds % 3600) / 60);
  return `${hours}时${mins}分`;
}

// 能耗口径与 classic utils/format 对齐：今日 kWh 后端恒为 null（尚未接入），
// 显示「—」而不是 0.000，避免 RAPL 机器被误读成零耗电
export function formatKwh(kwh: number | null | undefined): string {
  if (kwh === null || kwh === undefined || !Number.isFinite(Number(kwh))) return '—';
  const n = Number(kwh);
  // 自适应小数位（与 classic fmtKwh 同口径）：13.320 → 13.3，0.014 保留 3 位，
  // 避免卡片里出现无意义的三位小数噪声。
  if (n >= 100) return `${Math.round(n)} kWh`;
  if (n >= 10) return `${n.toFixed(1)} kWh`;
  return `${n.toFixed(n >= 1 ? 2 : 3)} kWh`;
}

export function formatCost(cny: number | null | undefined): string {
  if (cny === null || cny === undefined || !Number.isFinite(Number(cny))) return '—';
  return `¥${Number(cny).toFixed(2)}`;
}

export function formatGhz(mhz: number | null | undefined): string {
  const n = Number(mhz);
  if (!n) return '—';
  return n >= 1000 ? `${(n / 1000).toFixed(2)} GHz` : `${Math.round(n)} MHz`;
}

// 「更新于 X」/「断开 X」文案（classic agoText 的秒级口径）
export function formatAgo(seconds: number | null | undefined): string {
  const s = Math.max(0, Math.floor(Number(seconds) || 0));
  if (s < 5) return '刚刚';
  if (s < 60) return `${s} 秒前`;
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`;
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`;
  return `${Math.floor(s / 86400)} 天前`;
}

