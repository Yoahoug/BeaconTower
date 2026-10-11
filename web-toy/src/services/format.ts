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
