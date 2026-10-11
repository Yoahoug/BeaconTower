/**
 * BeaconTower · 真实后端 API 客户端（toy 端，v2.8 联调落地）
 * 契约 1:1 对齐 internal/router/router.go 的路由与各 handler 响应；
 * 会话/CSRF/错误封套语义与 web/src/api/http.js（classic）一致。
 * mock.ts 仅在显式 VITE_API_MODE=mock 的 dev 构建里启用，生产默认走本文件。
 */

import { http, withTimeout, ApiError, notifyUnauthorized } from './http';
import type {
  PublicSummary,
  ServerItem,
  HistoryResp,
  HistoryPoint,
  TrafficResp,
  WGPublicSnapshot,
  FRPPublicSnapshot,
  AdminStatus,
  WGOverviewData,
  WGTask,
  WGHubProbeResp,
  FRPOverviewData,
  FRPPlatformItem,
  FRPTunnelItem,
  SystemSettingsData,
  AuditResp,
  AdminServerItem,
  TestConnResp,
} from '../types/backend';

// ---------- 派生汇总（对齐 classic monitor store：summary 由 servers 归约） ----------
// 后端虽有 /public/summary，但 SSE 只广播 servers 数组；为让 KPI 与实时推送
// 严格同步，汇总统一在前端由 servers 计算快照字段（classic 同口径）。

export function summarizeServers(servers: ServerItem[]): PublicSummary {
  let online = 0;
  let upBps = 0;
  let downBps = 0;
  let watts = 0;
  let measuredCount = 0;
  let monthKwh = 0;
  let estCost = 0;
  let dayIn = 0;
  let dayOut = 0;
  for (const s of servers) {
    if (s.status === 'online') {
      online++;
      upBps += s.metrics.net_out_bps;
      downBps += s.metrics.net_in_bps;
      if (s.power) {
        measuredCount++;
        watts += s.power.total_w;
        monthKwh += s.power.month_kwh;
        estCost += s.power.est_cost_month || 0;
      }
    }
    if (s.traffic_today) {
      dayIn += s.traffic_today.in_total;
      dayOut += s.traffic_today.out_total;
    }
  }
  const r1 = (v: number) => Math.round(v * 10) / 10;
  return {
    total: servers.length,
    online,
    offline: servers.length - online,
    up_bps: upBps,
    down_bps: downBps,
    watts: r1(watts),
    measured_count: measuredCount,
    month_kwh: r1(monthKwh),
    est_cost_month: r1(estCost),
    day_in_total: dayIn,
    day_out_total: dayOut,
  };
}

// ---------- 历史曲线归一化（对齐 classic monitor.js normalizePoints） ----------
// 1h/6h/24h 原始采样：cpu_pct/mem_used/disk_used/net_in_out_bps/power_w；
// 7d 小时聚合：cpu_avg/cpu_max/mem_avg/net_in_out_avg/power_avg。

export interface HistorySeries {
  ts: number;
  cpu: number | null;
  memUsed: number | null;
  diskUsed: number | null;
  netIn: number | null;
  netOut: number | null;
  power: number | null;
}

export function normalizeHistoryPoints(raw: HistoryResp): HistorySeries[] {
  const list = Array.isArray(raw?.points) ? raw.points : [];
  const num = (v: unknown, d = 0): number => {
    const n = Number(v);
    return Number.isFinite(n) ? n : d;
  };
  return list
    .map((p: HistoryPoint) => {
      const ts = num(p.ts, 0);
      if (!ts) return null;
      const cpu = p.cpu_pct ?? p.cpu_avg ?? null;
      const memUsed = p.mem_used ?? p.mem_avg ?? null;
      const netIn = p.net_in_bps ?? p.net_in_avg ?? null;
      const netOut = p.net_out_bps ?? p.net_out_avg ?? null;
      const power = p.power_w ?? p.power_avg ?? null;
      return {
        ts,
        cpu: cpu == null ? null : num(cpu),
        memUsed: memUsed == null ? null : num(memUsed),
        diskUsed: p.disk_used == null ? null : num(p.disk_used),
        netIn: netIn == null ? null : num(netIn),
        netOut: netOut == null ? null : num(netOut),
        power: power == null ? null : num(power),
      } as HistorySeries;
    })
    .filter((x): x is HistorySeries => x !== null)
    .sort((a, b) => a.ts - b.ts);
}

// ---------- SSE 订阅（/public/stream，对齐 handler/public.go 帧格式） ----------
// event: snapshot | update（data = 预序列化 servers 数组）、ping（data "1"）。
// 断线指数退避 3s→30s，与 classic stores/monitor.js 一致。

export type SSEListener = (servers: ServerItem[]) => void;

export class RealtimeStream {
  private es: EventSource | null = null;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private retryMs = 3000;
  private listeners: SSEListener[] = [];
  private lastServerList: ServerItem[] = [];

  subscribe(cb: SSEListener): void {
    this.listeners.push(cb);
    // 新订阅者立即拿到最近一份快照（避免空窗）
    if (this.lastServerList.length) cb(this.lastServerList);
  }

  start(): void {
    if (this.es || typeof window === 'undefined') return;
    const es = new EventSource('/api/v1/public/stream');
    this.es = es;
    const apply = (ev: MessageEvent) => {
      try {
        const list = JSON.parse(ev.data);
        if (Array.isArray(list)) {
          this.lastServerList = list;
          for (const cb of this.listeners) cb(list);
        }
        this.retryMs = 3000;
      } catch {
        /* 坏帧静默忽略（classic 同口径） */
      }
    };
    es.addEventListener('snapshot', apply);
    es.addEventListener('update', apply);
    es.addEventListener('ping', () => {
      this.retryMs = 3000;
    });
    es.onerror = () => {
      this.stop();
      this.retryTimer = setTimeout(() => this.start(), this.retryMs);
      this.retryMs = Math.min(this.retryMs * 2, 30000);
    };
  }

  stop(): void {
    if (this.es) {
      this.es.close();
      this.es = null;
    }
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
  }

  get running(): boolean {
    return this.es !== null;
  }
}

export const stream = new RealtimeStream();

// 会话失效时停流：401 广播后 EventSource 也会持续报错，直接停掉等重新登录
if (typeof window !== 'undefined') {
  window.addEventListener('bt:unauthorized', () => stream.stop());
}

// ---------- 具体 API（端点逐一映射 router.go） ----------

export const realApi = {
  // 公开监控
  async getSummary(): Promise<PublicSummary> {
    return summarizeServers(await this.getServers());
  },

  getServers(): Promise<ServerItem[]> {
    return withTimeout(http.get<ServerItem[]>('/v1/public/servers'));
  },

  getServerHistory(id: number, range: '1h' | '6h' | '24h' | '7d'): Promise<HistorySeries[]> {
    return withTimeout(
      http.get<HistoryResp>(`/v1/public/servers/${id}/history?range=${range}`).then(normalizeHistoryPoints),
    );
  },

  getServerTraffic(id: number, days = 30): Promise<TrafficResp> {
    return withTimeout(http.get<TrafficResp>(`/v1/public/servers/${id}/traffic?days=${days}`));
  },

  getWGPublic(): Promise<WGPublicSnapshot> {
    return withTimeout(
      http.get<WGPublicSnapshot>('/v1/public/wg').then((d) =>
        d.initialized
          ? { ...d, hubs: d.hubs ?? [], peers: d.peers ?? [] }
          : { initialized: false, hubs: [], peers: [], summary: d.summary },
      ),
    );
  },

  getFRPPublic(): Promise<FRPPublicSnapshot> {
    return withTimeout(
      http.get<FRPPublicSnapshot>('/v1/public/frp').then((d) => ({ ...d, platforms: d.platforms ?? [] })),
    );
  },

  // 认证（会话 cookie + CSRF 全由 http.ts 处理；无 mock token）
  getAdminStatus(): Promise<AdminStatus> {
    return withTimeout(http.get<AdminStatus>('/v1/admin/status'));
  },

  async login(username: string, password: string): Promise<{ success: boolean; msg?: string }> {
    try {
      await withTimeout(http.post('/v1/admin/login', { username, password }), 15000);
      return { success: true };
    } catch (e) {
      return { success: false, msg: e instanceof Error ? e.message : '登录失败' };
    }
  },

  async logout(): Promise<void> {
    try {
      await http.post('/v1/admin/logout', {});
    } finally {
      stream.stop();
    }
  },

  // 节点管理
  getAdminServers(): Promise<AdminServerItem[]> {
    return withTimeout(http.get<AdminServerItem[]>('/v1/admin/servers'));
  },

  async createServer(payload: {
    name: string;
    region?: string;
    note_public?: string;
    tags?: string[];
    ssh: Record<string, unknown>;
    probe?: unknown;
  }): Promise<{ id: number }> {
    return withTimeout(http.post<{ id: number }>('/v1/admin/servers', payload), 20000);
  },

  async deleteServer(id: number): Promise<void> {
    await withTimeout(http.del(`/v1/admin/servers/${id}`, { timeoutMs: 20000 }));
  },

  testConnection(ssh: Record<string, unknown>): Promise<TestConnResp> {
    return withTimeout(http.post<TestConnResp>('/v1/admin/servers/test', { ssh }, { timeoutMs: 45000, retries: 0 }), 50000);
  },

  // WG 组网管理
  getWGOverview(): Promise<WGOverviewData> {
    return withTimeout(http.get<WGOverviewData>('/v1/admin/wg/overview'));
  },

  async switchWGHub(hubServerId: number): Promise<{ task_id: number }> {
    // 语义对齐后端：switch-hub 需要的是中心所在「服务器 ID」（hubs[].server_id）
    const r = await withTimeout(
      http.post<{ task_id: number }>('/v1/admin/wg/switch-hub', { target_server_id: hubServerId }, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
    return { task_id: r.task_id };
  },

  async getWGTask(id: number): Promise<WGTask> {
    return http.get<WGTask>(`/v1/admin/wg/tasks/${id}`, { retries: 0 });
  },

  probeWGHub(hubServerId: number): Promise<WGHubProbeResp> {
    return withTimeout(
      http.post<WGHubProbeResp>(`/v1/admin/wg/hubs/${hubServerId}/probe`, {}, { timeoutMs: 40000, retries: 0 }),
      45000,
    );
  },

  patrol(): Promise<{ ok: boolean }> {
    return withTimeout(http.post<{ ok: boolean }>('/v1/admin/wg/patrol', {}, { timeoutMs: 45000, retries: 0 }), 50000);
  },

  addWGDevice(name: string): Promise<{ id: number; wg_ip: string; warn: string }> {
    return withTimeout(http.post<{ id: number; wg_ip: string; warn: string }>('/v1/admin/wg/devices', { name }, { timeoutMs: 60000, retries: 0 }), 65000);
  },

  deleteWGDevice(id: number): Promise<{ detail: string }> {
    return withTimeout(http.del<{ detail: string }>(`/v1/admin/wg/peers/${id}`, { timeoutMs: 60000, retries: 0 }), 65000);
  },

  getWGDeviceConf(id: number): Promise<{ conf: string; filename: string; endpoint: string; wg_ip: string }> {
    return withTimeout(http.get<{ conf: string; filename: string; endpoint: string; wg_ip: string }>(`/v1/admin/wg/peers/${id}/conf`, { retries: 0 }), 20000);
  },

  // 内网穿透 FRP 管理
  getFRPOverview(): Promise<FRPOverviewData> {
    return withTimeout(http.get<FRPOverviewData>('/v1/admin/frp/overview'), 20000);
  },

  getFRPNodes(): Promise<{ nodes: import('../types/backend').FRPNodeItem[] }> {
    return withTimeout(http.get<{ nodes: import('../types/backend').FRPNodeItem[] }>('/v1/admin/frp/nodes'), 20000);
  },

  async bindSakuraPlatform(name: string, token: string): Promise<void> {
    await withTimeout(http.post('/v1/admin/frp/platforms', { name, token }, { timeoutMs: 60000, retries: 0 }), 65000);
  },

  async bindCloudflaredPlatform(name: string, token: string): Promise<void> {
    await withTimeout(http.post('/v1/admin/frp/cloudflared', { name, token }, { timeoutMs: 60000, retries: 0 }), 65000);
  },

  async syncFRPPlatform(platformId: number, full = true): Promise<void> {
    await withTimeout(
      http.post(`/v1/admin/frp/platforms/${platformId}/sync?full=${full ? 1 : 0}`, {}, { timeoutMs: 60000, retries: 0 }),
      65000,
    );
  },

  async createFRPTunnel(
    platformId: number,
    payload: { name: string; proto: string; node_id?: string; local_ip: string; local_port: number; remote_port?: number; domain?: string },
  ): Promise<void> {
    await withTimeout(
      http.post(`/v1/admin/frp/platforms/${platformId}/tunnels`, payload, { timeoutMs: 60000, retries: 0 }),
      65000,
    );
  },

  async deleteFRPTunnel(id: number): Promise<void> {
    await withTimeout(http.del(`/v1/admin/frp/tunnels/${id}`, { timeoutMs: 30000, retries: 0 }), 35000);
  },

  async offlineFRPTunnel(id: number): Promise<void> {
    await withTimeout(http.post(`/v1/admin/frp/tunnels/${id}/offline`, {}, { timeoutMs: 30000, retries: 0 }), 35000);
  },

  // 系统设置
  getSettings(): Promise<SystemSettingsData> {
    return withTimeout(http.get<SystemSettingsData>('/v1/admin/settings'));
  },

  async saveSettings(settings: SystemSettingsData): Promise<void> {
    await withTimeout(http.put('/v1/admin/settings', settings));
  },

  // 安全与密码（后端无「会话列表」端点：toy 安全页只保留改密）
  changePassword(oldPw: string, newPw: string): Promise<{ success: boolean; msg?: string }> {
    return withTimeout(
      http
        .post('/v1/admin/password', { old_password: oldPw, new_password: newPw })
        .then(() => ({ success: true as const }))
        .catch((e: unknown) => {
          const msg = e instanceof ApiError || e instanceof Error ? e.message : '密码更新失败';
          return { success: false as const, msg };
        }),
      20000,
    );
  },

  // 审计日志（page/size；后端无模糊搜索——搜索在客户端过滤当前页附近数据，
  // 与 classic 的 filteredAudit getter 同口径）
  getAuditLogs(page = 1, pageSize = 10): Promise<AuditResp> {
    return withTimeout(http.get<AuditResp>(`/v1/admin/audit?page=${page}&size=${pageSize}`));
  },
};

// re-export：视图统一从 services/api 导入
export { ApiError, notifyUnauthorized };
export type { FRPPlatformItem, FRPTunnelItem };
