/**
 * BeaconTower · 真实后端 API 客户端（toy 端，v2.8 联调落地）
 * 契约 1:1 对齐 internal/router/router.go 的路由与各 handler 响应；
 * 会话/CSRF/错误封套语义与 web/src/api/http.js（classic）一致。
 * mock.ts 仅在显式 VITE_API_MODE=mock 的 dev 构建里启用，生产默认走本文件。
 */

import { http, withTimeout, downloadRaw, ApiError, notifyUnauthorized } from './http';
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
  WGHubDetailData,
  WGSyncHubsResp,
  FRPOverviewData,
  FRPPlatformItem,
  FRPTunnelItem,
  FRPNodeItem,
  FRPPlatformDetailData,
  FRPFlowPoint,
  FRPTunnelInput,
  FRPDeployItem,
  FRPDockerStatus,
  FRPSubdomainItem,
  FRPSubdomainInput,
  FRPDeviceStartData,
  FRPDevicePollData,
  WGPlanInput,
  WGPlanResp,
  WGImportInput,
  WGTakeoverInput,
  WGAssetItem,
  WGAssetInput,
  WGAssetProbeResult,
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
  // 最近一次收到任何帧（snapshot/update/ping/open）的时刻：SSE 被反代静默断流时
  // readyState 仍是 OPEN、onerror 也不触发，调用方据此判活并退化为全量轮询
  lastEventAt = 0;

  subscribe(cb: SSEListener): void {
    // 幂等订阅：页面在暂停/恢复、回前台时可能重复注册同一个回调，
    // 不去重会让每帧 apply 多次（走势窗口被重复采样）。
    if (!this.listeners.includes(cb)) this.listeners.push(cb);
    // 新订阅者立即拿到最近一份快照（避免空窗）
    if (this.lastServerList.length) cb(this.lastServerList);
  }

  start(): void {
    if (this.es || typeof window === 'undefined') return;
    const es = new EventSource('/api/v1/public/stream');
    this.es = es;
    this.lastEventAt = Date.now();
    const apply = (ev: MessageEvent) => {
      try {
        const list = JSON.parse(ev.data);
        if (Array.isArray(list)) {
          this.lastServerList = list;
          for (const cb of this.listeners) cb(list);
        }
        this.lastEventAt = Date.now();
        this.retryMs = 3000;
      } catch {
        /* 坏帧静默忽略（classic 同口径） */
      }
    };
    es.addEventListener('snapshot', apply);
    es.addEventListener('update', apply);
    es.addEventListener('ping', () => {
      this.lastEventAt = Date.now();
      this.retryMs = 3000;
    });
    es.onopen = () => {
      this.lastEventAt = Date.now();
      this.retryMs = 3000;
    };
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

  // 首次初始化（仅在未初始化时可用）：建管理员账号，成功后即持会话
  async setup(payload: { username: string; password: string; setup_token?: string }): Promise<void> {
    await withTimeout(http.post('/v1/admin/setup', payload, { timeoutMs: 20000, retries: 0 }), 25000);
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

  // 编辑节点（名称/区域/标签/备注/排序/隐藏/暂停/SSH 参数）
  async updateServer(id: number, payload: Record<string, unknown>): Promise<void> {
    await withTimeout(http.put(`/v1/admin/servers/${id}`, payload, { timeoutMs: 20000 }), 25000);
  },

  // 密钥失联自愈：重输一次性密码重建面板代管密钥（不落库密码）
  async rotateServerKey(id: number, password: string): Promise<{ detail?: string }> {
    return withTimeout(
      http.post<{ detail?: string }>(`/v1/admin/servers/${id}/rotate-key`, { password }, { timeoutMs: 45000, retries: 0 }),
      50000,
    );
  },

  // 重新定位（公网 IP 归属多源兜底，修「未定位」）
  relocate(id: number): Promise<{ detail?: string; region?: string }> {
    return withTimeout(http.post<{ detail?: string; region?: string }>(`/v1/admin/servers/${id}/locate`, {}, { timeoutMs: 40000, retries: 0 }), 45000);
  },

  // 功耗校准：写入基础功耗（电池放电实测口径）
  async recalibratePower(id: number, baseLoadW: number): Promise<void> {
    await withTimeout(http.put(`/v1/admin/servers/${id}/power-calibration`, { base_load_w: baseLoadW }, { timeoutMs: 20000 }), 25000);
  },

  // 排序（拖拽后的顺序写回）
  async reorderServers(ids: number[]): Promise<void> {
    await withTimeout(http.put('/v1/admin/servers/order', { ids }, { timeoutMs: 20000 }), 25000);
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

  // 中心节点面板：概览 + 使用端凭证 + SSH 成员 + 主备就绪度（只读聚合，一次渲染一屏）
  getWGHubDetail(hubServerId: number): Promise<WGHubDetailData> {
    return withTimeout(
      http.get<WGHubDetailData>(`/v1/admin/wg/hubs/${hubServerId}`, { retries: 0 }),
      20000,
    );
  },

  // 把某个成员整体下发到所有未退役中心（重建各中心 conf 并 syncconf）
  syncWGPeerHubs(peerId: number): Promise<WGSyncHubsResp> {
    return withTimeout(
      http.post<WGSyncHubsResp>(`/v1/admin/wg/peers/${peerId}/sync-hubs`, {}, { timeoutMs: 90000, retries: 0 }),
      95000,
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

  // hub 为 0 时后端取现役中心；中心面板里导出的是「你正打开的这台中心」的视图
  getWGDeviceConf(id: number, hubId = 0): Promise<{ conf: string; filename: string; endpoint: string; wg_ip: string }> {
    const q = hubId ? `?hub=${hubId}` : '';
    return withTimeout(
      http.get<{ conf: string; filename: string; endpoint: string; wg_ip: string }>(`/v1/admin/wg/peers/${id}/conf${q}`, { retries: 0 }),
      20000,
    );
  },

  // 成员复验（SSH 实查接口与握手，返回判定与原文）：面板侧要真握手，给足 60s
  verifyWGPeer(id: number): Promise<{ online: boolean; detail: string }> {
    return withTimeout(
      http.post<{ online: boolean; detail: string }>(`/v1/admin/wg/peers/${id}/verify`, {}, { timeoutMs: 60000, retries: 0 }),
      70000,
    );
  },

  // 重命名使用端（只影响面板显示与导出文件名）
  async renameWGPeer(id: number, name: string): Promise<void> {
    await withTimeout(http.put(`/v1/admin/wg/peers/${id}`, { name }, { timeoutMs: 20000, retries: 0 }), 25000);
  },

  // 重新签发使用端凭证（设备换钥重导入；面板托管成员走同一接口＝重新下发）
  reissueWGPeer(id: number): Promise<{ wg_ip?: string; warn?: string }> {
    return withTimeout(
      http.post<{ wg_ip?: string; warn?: string }>(`/v1/admin/wg/peers/${id}/reissue`, {}, { timeoutMs: 60000, retries: 0 }),
      65000,
    );
  },

  // 清理已退役中心的记录（机器上的配置在退役时已备份，不动目标机）
  async cleanupWGHub(serverId: number): Promise<void> {
    await withTimeout(http.del(`/v1/admin/wg/hubs/${serverId}`, { timeoutMs: 20000, retries: 0 }), 25000);
  },

  // 把网外节点配置为中心（备援/现役）：整链路后台任务，进度走 /wg/tasks/:id
  provisionWGHub(serverId: number, listenPort = 0): Promise<{ task_id: number; role: string; listen_port: number }> {
    return withTimeout(
      http.post<{ task_id: number; role: string; listen_port: number }>(
        '/v1/admin/wg/hubs/provision',
        { server_id: serverId, listen_port: listenPort },
        { timeoutMs: 30000, retries: 0 },
      ),
      35000,
    );
  },

  // 只读登记：机器上已有手工配好的 WG，仅读取登记为备援槽位，不写目标机
  registerWGStandby(serverId: number): Promise<{
    endpoint: string;
    listen_port: number;
    status: string;
    udp: { checked: boolean; mode: string; ok: boolean; hint: string };
  }> {
    return withTimeout(
      http.post<{
        endpoint: string;
        listen_port: number;
        status: string;
        udp: { checked: boolean; mode: string; ok: boolean; hint: string };
      }>('/v1/admin/wg/hubs/standby', { server_id: serverId }, { timeoutMs: 60000, retries: 0 }),
      65000,
    );
  },

  // 纳管本机（面板宿主）入网：经宿主 SSH 拉起成员接口并回填握手
  adoptWGServer(serverId: number): Promise<{ server_id: number; detail: string }> {
    return withTimeout(
      http.post<{ server_id: number; detail: string }>('/v1/admin/wg/adopt', { server_id: serverId }, { timeoutMs: 90000, retries: 0 }),
      95000,
    );
  },

  /** 组网预检（dry-run）：分配 IP + 并发 SSH 探测，不落库不改配置 */
  wgPlan(payload: WGPlanInput): Promise<WGPlanResp> {
    return withTimeout(http.post<WGPlanResp>('/v1/admin/wg/plan', payload, { timeoutMs: 60000, retries: 0 }), 65000);
  },

  /** 执行组网：落库 → 建任务 → 后台执行（进度走 /wg/tasks/:id） */
  async wgApply(payload: WGPlanInput): Promise<{ task_id: number }> {
    return withTimeout(
      http.post<{ task_id: number }>('/v1/admin/wg/apply', payload, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  /** 导入现有网络：读现役 hub 实况建网收编成员 */
  async wgImport(payload: WGImportInput): Promise<{ task_id: number }> {
    return withTimeout(
      http.post<{ task_id: number }>('/v1/admin/wg/import', payload, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  /** 新机接管现役中心：继承密钥与端口，成员只改端点 */
  async wgTakeover(payload: WGTakeoverInput): Promise<{ task_id: number }> {
    return withTimeout(
      http.post<{ task_id: number }>('/v1/admin/wg/takeover', payload, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  // 资产中转：面板缓存安装包并推送到节点
  wgAssets(): Promise<{ assets: WGAssetItem[] }> {
    return withTimeout(http.get<{ assets: WGAssetItem[] }>('/v1/admin/wg/assets'), 20000);
  },

  async wgAssetUpsert(payload: WGAssetInput): Promise<{ id: number }> {
    return withTimeout(http.post<{ id: number }>('/v1/admin/wg/assets', payload, { retries: 0 }), 20000);
  },

  async wgAssetDelete(id: number): Promise<void> {
    await withTimeout(http.del(`/v1/admin/wg/assets/${id}`, { retries: 0 }), 20000);
  },

  wgAssetProbe(id: number): Promise<{ results: WGAssetProbeResult[] }> {
    return withTimeout(
      http.post<{ results: WGAssetProbeResult[] }>(`/v1/admin/wg/assets/${id}/probe`, {}, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  wgAssetFetch(id: number): Promise<{ path: string; size: number; sha256: string; via: string }> {
    return withTimeout(
      http.post<{ path: string; size: number; sha256: string; via: string }>(
        `/v1/admin/wg/assets/${id}/fetch`,
        {},
        { timeoutMs: 600000, retries: 0 },
      ),
      610000,
    );
  },

  async wgAssetPush(id: number, serverId: number): Promise<void> {
    await withTimeout(
      http.post(`/v1/admin/wg/assets/${id}/push`, { server_id: serverId }, { timeoutMs: 90000, retries: 0 }),
      95000,
    );
  },

  /** 使用端凭证 zip（A=现役 / B=备援） */
  downloadWGCreds(): Promise<{ blob: Blob; filename: string }> {
    return downloadRaw('/v1/admin/wg/creds.zip', { timeoutMs: 120000 });
  },

  // 内网穿透 FRP 管理（全部走外部平台 API：统一 retries:0，重试会重复建隧道/重复删除）
  getFRPOverview(): Promise<FRPOverviewData> {
    return withTimeout(http.get<FRPOverviewData>('/v1/admin/frp/overview'), 20000);
  },

  getFRPNodes(): Promise<{ nodes: FRPNodeItem[] }> {
    return withTimeout(http.get<{ nodes: FRPNodeItem[] }>('/v1/admin/frp/nodes'), 20000);
  },

  getFRPPlatformDetail(id: number): Promise<FRPPlatformDetailData> {
    return withTimeout(http.get<FRPPlatformDetailData>(`/v1/admin/frp/platforms/${id}`, { retries: 0 }), 20000);
  },

  getFRPFlow(id: number, kind = 'day'): Promise<{ points: FRPFlowPoint[] }> {
    return withTimeout(
      http.get<{ points: FRPFlowPoint[] }>(`/v1/admin/frp/platforms/${id}/flow?kind=${encodeURIComponent(kind)}`, { retries: 0 }),
      20000,
    );
  },

  async bindSakuraPlatform(name: string, token: string): Promise<void> {
    await withTimeout(http.post('/v1/admin/frp/platforms', { name, token }, { timeoutMs: 60000, retries: 0 }), 65000);
  },

  async bindCloudflaredPlatform(name: string, token: string): Promise<void> {
    await withTimeout(http.post('/v1/admin/frp/cloudflared', { name, token }, { timeoutMs: 60000, retries: 0 }), 65000);
  },

  async renameFRPPlatform(id: number, name: string): Promise<void> {
    await withTimeout(http.put(`/v1/admin/frp/platforms/${id}`, { name }), 20000);
  },

  async deleteFRPPlatform(id: number): Promise<void> {
    await withTimeout(http.del(`/v1/admin/frp/platforms/${id}`, { retries: 0 }), 35000);
  },

  async syncFRPPlatform(platformId: number, full = true): Promise<void> {
    await withTimeout(
      http.post(`/v1/admin/frp/platforms/${platformId}/sync?full=${full ? 1 : 0}`, {}, { timeoutMs: 60000, retries: 0 }),
      65000,
    );
  },

  async createFRPTunnel(platformId: number, payload: FRPTunnelInput): Promise<{ remote_id?: string; sync_error?: string }> {
    return withTimeout(
      http.post<{ remote_id?: string; sync_error?: string }>(
        `/v1/admin/frp/platforms/${platformId}/tunnels`,
        payload,
        { timeoutMs: 60000, retries: 0 },
      ),
      65000,
    );
  },

  async updateFRPTunnel(id: number, payload: FRPTunnelInput): Promise<void> {
    await withTimeout(http.put(`/v1/admin/frp/tunnels/${id}`, payload, { timeoutMs: 30000, retries: 0 }), 35000);
  },

  async deleteFRPTunnel(id: number): Promise<void> {
    await withTimeout(http.del(`/v1/admin/frp/tunnels/${id}`, { timeoutMs: 30000, retries: 0 }), 35000);
  },

  async lockFRPTunnel(id: number, locks: { edit: boolean; delete: boolean; migrate: boolean }): Promise<void> {
    await withTimeout(http.post(`/v1/admin/frp/tunnels/${id}/lock`, locks, { timeoutMs: 30000, retries: 0 }), 35000);
  },

  async migrateFRPTunnel(id: number, nodeId: string): Promise<void> {
    await withTimeout(
      http.post(`/v1/admin/frp/tunnels/${id}/migrate`, { node_id: nodeId }, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  async offlineFRPTunnel(id: number): Promise<void> {
    await withTimeout(http.post(`/v1/admin/frp/tunnels/${id}/offline`, {}, { timeoutMs: 30000, retries: 0 }), 35000);
  },

  async authFRPTunnel(id: number, ip = ''): Promise<{ ip: string }> {
    return withTimeout(
      http.post<{ ip: string }>(`/v1/admin/frp/tunnels/${id}/auth`, { ip }, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  /** frpc 配置下载：裸文本（text/plain），必须绕开 JSON 解包 */
  downloadFRPTunnelConfig(id: number): Promise<{ blob: Blob; filename: string }> {
    return downloadRaw(`/v1/admin/frp/tunnels/${id}/config`, { timeoutMs: 60000 });
  },

  getFRPTunnelTraffic(id: number): Promise<{ points: { label: string; used: number }[] }> {
    return withTimeout(http.get<{ points: { label: string; used: number }[] }>(`/v1/admin/frp/tunnels/${id}/traffic`, { retries: 0 }), 30000);
  },

  // ---------- ChmlFrp 免费二级域名 ----------
  getFRPSubdomains(platformId: number): Promise<{ subdomains: FRPSubdomainItem[] }> {
    return withTimeout(http.get<{ subdomains: FRPSubdomainItem[] }>(`/v1/admin/frp/platforms/${platformId}/subdomains`, { retries: 0 }), 30000);
  },

  getFRPAvailableDomains(platformId: number): Promise<{ domains: { domain: string; [k: string]: unknown }[] }> {
    return withTimeout(
      http.get<{ domains: { domain: string; [k: string]: unknown }[] }>(
        `/v1/admin/frp/platforms/${platformId}/subdomains/available`,
        { retries: 0 },
      ),
      30000,
    );
  },

  async createFRPSubdomain(platformId: number, payload: FRPSubdomainInput): Promise<void> {
    await withTimeout(
      http.post(`/v1/admin/frp/platforms/${platformId}/subdomains`, payload, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  async updateFRPSubdomain(platformId: number, payload: FRPSubdomainInput): Promise<void> {
    await withTimeout(
      http.put(`/v1/admin/frp/platforms/${platformId}/subdomains`, payload, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  async deleteFRPSubdomain(platformId: number, domain: string, record: string): Promise<void> {
    await withTimeout(
      http.del(
        `/v1/admin/frp/platforms/${platformId}/subdomains?domain=${encodeURIComponent(domain)}&record=${encodeURIComponent(record)}`,
        { timeoutMs: 30000, retries: 0 },
      ),
      35000,
    );
  },

  // ---------- 客户端托管：面板经 SSH 在节点上部署/管理 frpc 容器 ----------
  getFRPDeployments(refresh = false): Promise<{ deployments: FRPDeployItem[] }> {
    return withTimeout(
      http.get<{ deployments: FRPDeployItem[] }>(`/v1/admin/frp/deployments${refresh ? '?refresh=1' : ''}`, { retries: 0 }),
      120000,
    );
  },

  createFRPDeployment(payload: {
    platform_id: number;
    server_id: number;
    tunnel_ids: number[];
    install_docker: boolean;
    image?: string;
  }): Promise<{ deployment: FRPDeployItem; log_tail?: string }> {
    return withTimeout(
      http.post<{ deployment: FRPDeployItem; log_tail?: string }>('/v1/admin/frp/deployments', payload, {
        timeoutMs: 720000,
        retries: 0,
      }),
      730000,
    );
  },

  async syncFRPDeployment(id: number): Promise<{ deployment: FRPDeployItem; log_tail?: string }> {
    return withTimeout(
      http.post<{ deployment: FRPDeployItem; log_tail?: string }>(`/v1/admin/frp/deployments/${id}/sync`, {}, { timeoutMs: 360000, retries: 0 }),
      370000,
    );
  },

  async actionFRPDeployment(id: number, action: 'restart' | 'stop' | 'start'): Promise<{ deployment: FRPDeployItem }> {
    return withTimeout(
      http.post<{ deployment: FRPDeployItem }>(`/v1/admin/frp/deployments/${id}/action`, { action }, { timeoutMs: 60000, retries: 0 }),
      65000,
    );
  },

  async statusFRPDeployment(id: number): Promise<{ deployment: FRPDeployItem }> {
    return withTimeout(
      http.post<{ deployment: FRPDeployItem }>(`/v1/admin/frp/deployments/${id}/status`, {}, { timeoutMs: 60000, retries: 0 }),
      65000,
    );
  },

  getFRPDeployLogs(id: number, tail = 200): Promise<{ container: string; logs: string }> {
    return withTimeout(
      http.get<{ container: string; logs: string }>(`/v1/admin/frp/deployments/${id}/logs?tail=${tail}`, { retries: 0 }),
      65000,
    );
  },

  async deleteFRPDeployment(id: number, keep = false): Promise<void> {
    await withTimeout(http.del(`/v1/admin/frp/deployments/${id}${keep ? '?keep=1' : ''}`, { retries: 0 }), 65000);
  },

  /** 节点 Docker 探测/一键安装（install=true 会跑几分钟，前端需给出等待提示） */
  async serverDocker(serverId: number, install = false): Promise<{ docker?: FRPDockerStatus; error?: string }> {
    return withTimeout(
      http.post<{ docker?: FRPDockerStatus; error?: string }>(
        `/v1/admin/frp/servers/${serverId}/docker`,
        { install },
        { timeoutMs: install ? 720000 : 60000, retries: 0 },
      ),
      install ? 730000 : 65000,
    );
  },

  // ChmlFrp 走 OAuth2 设备码（令牌 10 分钟寿命，续不上时前端弹重新授权）
  startFRPDevice(reuseId = 0): Promise<FRPDeviceStartData> {
    return withTimeout(
      http.post<FRPDeviceStartData>('/v1/admin/frp/chmlfrp/device', { reuse_id: reuseId }, { timeoutMs: 30000, retries: 0 }),
      35000,
    );
  },

  pollFRPDevice(sessionId: string): Promise<FRPDevicePollData> {
    return withTimeout(http.get<FRPDevicePollData>(`/v1/admin/frp/chmlfrp/device/${sessionId}`, { retries: 0 }), 30000);
  },

  async cancelFRPDevice(sessionId: string): Promise<void> {
    await withTimeout(http.del(`/v1/admin/frp/chmlfrp/device/${sessionId}`, { retries: 0 }), 30000);
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
