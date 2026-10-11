/**
 * BeaconTower · toy 端 API 门面
 * 生产构建默认 real（真实 Go 后端，契约对齐 internal/router/router.go）；
 * mock 仅在显式 VITE_API_MODE=mock 的 dev 会话里启用——生产面板绝不展示假数据。
 * 视图层一律 import { api } from 'services/api'，禁止绕过。
 */

import type {
  PublicSummary,
  ServerItem,
  WGPublicSnapshot,
  FRPPublicSnapshot,
  AdminStatus,
  WGOverviewData,
  FRPPlatformItem,
  FRPTunnelItem,
  SystemSettingsData,
  AuditLogItem,
  TrafficResp,
} from '../types/backend';
import {
  realApi,
  stream,
  summarizeServers,
  normalizeHistoryPoints,
  type HistorySeries,
} from './real';
import { ApiError, notifyUnauthorized } from './http';

// 显式开关：npm run dev 仅在 VITE_API_MODE=mock 时走仿真引擎（联调用）
const USE_MOCK = ((import.meta as unknown as { env?: Record<string, string> }).env?.VITE_API_MODE ?? '') === 'mock';
export const IS_MOCK = USE_MOCK;

// ---------- 统一接口形状（real 与 mock 双实现共用） ----------

export interface AuditPage {
  list: AuditLogItem[];
  total: number;
  page: number;
  pages: number;
}

export interface ToyApi {
  // 公开监控
  getSummary(): Promise<PublicSummary>;
  getServers(): Promise<ServerItem[]>;
  getServerHistory(id: number, range: '1h' | '6h' | '24h' | '7d'): Promise<HistorySeries[]>;
  getServerTraffic(id: number, days?: number): Promise<TrafficResp>;
  getWGPublic(): Promise<WGPublicSnapshot>;
  getFRPPublic(): Promise<FRPPublicSnapshot>;
  // 实时流
  startStream(): void;
  stopStream(): void;
  onServers(cb: (servers: ServerItem[]) => void): void;
  isStreamRunning(): boolean;
  // 认证
  getAdminStatus(): Promise<AdminStatus>;
  login(username: string, password: string): Promise<{ success: boolean; msg?: string }>;
  logout(): Promise<void>;
  // 节点管理
  deleteServer(id: number): Promise<void>;
  testConnection(name: string): Promise<{ ok: boolean; msg: string }>;
  /** 新节点引导试连：SSH 密码一次性引导装钥，返回面板私钥（内存态）与画像 probe */
  testConnectionBootstrap(ssh: { host: string; port: number; username: string; auth_type: string; password: string }): Promise<{ ok: boolean; msg: string; panelPrivateKey: string; probe: Record<string, unknown> }>;
  createServer(payload: { name: string; region?: string; note_public?: string; ssh: Record<string, unknown>; panel_private_key?: string; probe?: unknown }): Promise<void>;
  // WG 组网
  getWGOverview(): Promise<WGOverviewData>;
  switchWGHub(hubServerId: number): Promise<{ task_id?: number }>;
  pollSwitchTask(taskId: number): Promise<{ done: boolean; ok: boolean; status: string; result: string }>;
  probeWGHub(hubServerId: number): Promise<{ ok: boolean; rttMs?: number; hint: string }>;
  patrol(): Promise<void>;
  addWGDevice(name: string): Promise<{ id: number; wg_ip: string; warn?: string }>;
  deleteWGDevice(id: number): Promise<void>;
  getWGDeviceConf(id: number): Promise<{ conf: string; filename: string }>;
  // FRP
  getFRPOverview(): Promise<{ platforms: FRPPlatformItem[]; tunnels: FRPTunnelItem[] }>;
  toggleFRPTunnel(id: number, running: boolean): Promise<void>;
  createFRPTunnel(payload: Partial<FRPTunnelItem> & { local_addr?: string }): Promise<void>;
  deleteFRPTunnel(id: number): Promise<void>;
  saveFRPPlatformToken(platform: { id?: number; kind: string }, token: string): Promise<void>;
  syncFRPPlatform(id: number): Promise<void>;
  // 设置
  getSettings(): Promise<SystemSettingsData>;
  saveSettings(settings: SystemSettingsData): Promise<void>;
  // 安全
  changePassword(oldPw: string, newPw: string): Promise<{ success: boolean; msg?: string }>;
  // 审计
  getAuditLogs(query?: string, page?: number, pageSize?: number): Promise<AuditPage>;
}

// ---------- real 实现（生产默认） ----------

const real: ToyApi = {
  getSummary: () => realApi.getSummary(),
  getServers: () => realApi.getServers(),
  getServerHistory: (id, range) => realApi.getServerHistory(id, range),
  getServerTraffic: (id, days = 30) => realApi.getServerTraffic(id, days),
  getWGPublic: () => realApi.getWGPublic(),
  getFRPPublic: () => realApi.getFRPPublic(),

  startStream: () => stream.start(),
  stopStream: () => stream.stop(),
  onServers: (cb) => stream.subscribe(cb),
  isStreamRunning: () => stream.running,

  getAdminStatus: () => realApi.getAdminStatus(),
  login: (username, password) => realApi.login(username, password),
  logout: () => realApi.logout(),

  deleteServer: (id) => realApi.deleteServer(id),
  async testConnection(name: string) {
    // toy 表格内试连：本机节点走本地采集无需 SSH；其余按记录凭据直接拨测由后端
    // 完成（admin servers 列表带 ssh_ready）。这里查管理列表拿 is_self/ssh_ready。
    try {
      const servers = await realApi.getAdminServers();
      const s = servers.find((x) => x.name === name);
      if (!s) return { ok: false, msg: `节点「${name}」不存在` };
      if (s.is_self) return { ok: true, msg: '本机节点：采集走本地进程，无需 SSH 拨测，指标实时上报中' };
      if (!s.ssh?.host) return { ok: false, msg: '该节点尚未录入 SSH 凭据' };
      const r = await realApi.testConnection({ host: s.ssh.host, port: s.ssh.port, username: s.ssh.username, server_id: s.id });
      const p = r.profile;
      return {
        ok: true,
        msg: `SSH 试连成功：${p.os_name} ${p.os_version} · ${p.cpu_cores}C · ${r.latency_ms}ms${p.power?.rapl ? ' · RAPL 功耗可用' : ''}`,
      };
    } catch (e) {
      return { ok: false, msg: e instanceof Error ? e.message : '试连失败' };
    }
  },
  async testConnectionBootstrap(ssh) {
    // 对齐后端 v2.7 引导语义：bootstrap_key=true 恒走密码通道，试连成功即装钥，
    // 响应含 panel_private_key（内存态直传保存请求，绝不落库/回显）
    const r = await realApi.testConnection({ ...ssh, bootstrap_key: true });
    const p = r.profile;
    return {
      ok: true,
      msg: `连通！${p.os_name} ${p.os_version} · ${p.cpu_cores}C / ${Math.round(p.mem_total / 1024 ** 3)}GB · ${r.latency_ms}ms · 地区 ${r.geo.region_text || r.geo.country || '未知'}`,
      panelPrivateKey: r.panel_private_key ?? '',
      probe: { profile: p, geo: { public_ip: r.geo.public_ip, country: r.geo.country, city: r.geo.city }, host_key_fp: r.host_key_fp },
    };
  },
  async createServer(payload) {
    // 后端 serverInput 契约：panel_private_key 放 ssh 对象内；probe 结构 {profile, geo, host_key_fp}
    const ssh: Record<string, unknown> = { ...(payload.ssh), auth_type: 'key' };
    if (payload.panel_private_key) ssh.panel_private_key = payload.panel_private_key;
    await realApi.createServer({
      name: payload.name,
      region: payload.region,
      note_public: payload.note_public,
      ssh: ssh as never,
      probe: payload.probe,
    });
  },

  getWGOverview: () => realApi.getWGOverview(),
  switchWGHub: (hubServerId) => realApi.switchWGHub(hubServerId),
  async pollSwitchTask(taskId) {
    const t = await realApi.getWGTask(taskId);
    const done = t.status === 'success' || t.status === 'failed';
    return { done, ok: t.status === 'success', status: t.status, result: t.result };
  },
  async probeWGHub(hubServerId) {
    const r = await realApi.probeWGHub(hubServerId);
    return { ok: r.ok, rttMs: r.ok ? r.rtt_ms : undefined, hint: r.hint || r.reason || '' };
  },
  patrol: async () => {
    await realApi.patrol();
  },
  addWGDevice: (name) => realApi.addWGDevice(name),
  deleteWGDevice: (id) => realApi.deleteWGDevice(id).then(() => undefined),
  getWGDeviceConf: (id) => realApi.getWGDeviceConf(id).then((d) => ({ conf: d.conf, filename: d.filename })),

  getFRPOverview: () => realApi.getFRPOverview(),
  // 后端隧道无「启停开关」：running=true 想停 → offline（Sakura 强制下线），
  // stopped 想恢复 → 只能到平台上重开，面板如实提示而不是假装成功
  async toggleFRPTunnel(id, running) {
    if (running) {
      await realApi.offlineFRPTunnel(id);
      return;
    }
    throw new ApiError({ code: 'UNSUPPORTED', message: '面板暂不支持一键重启隧道（各平台 API 未开放），请到平台控制台操作或删除后重建' });
  },
  async createFRPTunnel(payload) {
    const platformId = payload.platform_id ?? 0;
    if (!platformId) throw new ApiError({ code: 'PARAM', message: '请选择穿透平台' });
    // toy 表单是 local_addr 合并字段；后端要 local_ip + local_port
    const addr = (payload.local_addr || '').trim();
    const sep = addr.lastIndexOf(':');
    const localIP = sep > 0 ? addr.slice(0, sep) : addr;
    const localPort = sep > 0 ? Number(addr.slice(sep + 1)) : 0;
    if (!localIP || !Number.isInteger(localPort) || localPort <= 0 || localPort > 65535) {
      throw new ApiError({ code: 'PARAM', message: '内网目标地址格式应为 IP:端口（如 127.0.0.1:8080）' });
    }
    await realApi.createFRPTunnel(platformId, {
      name: payload.name || '',
      proto: String(payload.proto || 'tcp'),
      local_ip: localIP,
      local_port: localPort,
    });
  },
  deleteFRPTunnel: (id) => realApi.deleteFRPTunnel(id),
  async saveFRPPlatformToken(platform, token) {
    if (!token.trim()) throw new ApiError({ code: 'PARAM', message: '令牌不能为空' });
    if (platform.kind === 'cloudflare') {
      await realApi.bindCloudflaredPlatform('', token);
    } else if (platform.kind === 'sakura') {
      await realApi.bindSakuraPlatform('', token);
    } else {
      // ChmlFrp 走 OAuth2 设备码（交互式授权），不支持静态 token 绑定
      throw new ApiError({ code: 'UNSUPPORTED', message: 'ChmlFrp 走设备码授权（不支持静态令牌），请在穿透页使用 ChmlFrp 授权流程' });
    }
  },
  syncFRPPlatform: (id) => realApi.syncFRPPlatform(id),

  getSettings: () => realApi.getSettings(),
  saveSettings: (settings) => realApi.saveSettings(settings),

  changePassword: (oldPw, newPw) => realApi.changePassword(oldPw, newPw),

  async getAuditLogs(query = '', page = 1, pageSize = 10): Promise<AuditPage> {
    const r = await realApi.getAuditLogs(page, pageSize);
    const items = r.items ?? [];
    // 后端无模糊搜索参数：与 classic 一致在前端过滤当前页
    const q = query.trim().toLowerCase();
    const list = q
      ? items.filter(
          (x) =>
            x.action.toLowerCase().includes(q) ||
            x.target.toLowerCase().includes(q) ||
            x.detail.toLowerCase().includes(q) ||
            x.actor.toLowerCase().includes(q),
        )
      : items;
    const pages = Math.max(1, Math.ceil((q ? list.length : r.total) / pageSize));
    return { list, total: q ? list.length : r.total, page, pages };
  },
};

// ---------- mock 实现（仅显式 dev 开关） ----------

async function mockApi(): Promise<ToyApi> {
  const m = await import('./mock');
  const engine = m.mockEngine;
  return {
    getSummary: () => Promise.resolve(engine.getSummary()),
    getServers: () => Promise.resolve(engine.getServers()),
    getServerHistory: () => Promise.resolve([]),
    getServerTraffic: () => Promise.resolve({ days: 30, points: [] }),
    getWGPublic: () => Promise.resolve(m.INITIAL_WG as WGPublicSnapshot),
    getFRPPublic: () => Promise.resolve(m.INITIAL_FRP as FRPPublicSnapshot),

    startStream: () => engine.startSimulation(),
    stopStream: () => engine.stopSimulation(),
    onServers: (cb) =>
      engine.subscribe((servers) => cb(servers)),
    isStreamRunning: () => engine.isRunning(),

    getAdminStatus: () =>
      Promise.resolve({ initialized: true, loggedIn: engine.isLoggedIn(), site_title: '信标塔 · BeaconTower', username: engine.isLoggedIn() ? 'admin' : '' }),
    login: (username, password) => Promise.resolve(engine.login(username, password)),
    logout: () => Promise.resolve(engine.logout()),

    deleteServer: (id) => Promise.resolve(engine.deleteServer(id)).then(() => undefined),
    testConnection: () => Promise.resolve({ ok: true, msg: '（mock）SSH 试连成功' }),
    testConnectionBootstrap: (ssh) =>
      Promise.resolve({
        ok: true,
        msg: `（mock）连通！Debian 12 · 4C / 8GB · 14ms · 地区 ${ssh.host}`,
        panelPrivateKey: 'mock-only-key',
        probe: {},
      }),
    createServer: () => Promise.resolve(undefined),

    getWGOverview: () => Promise.resolve(engine.getWGOverview()),
    switchWGHub: (hubId) => Promise.resolve(engine.switchWGHub(hubId)).then(() => ({})),
    pollSwitchTask: () => Promise.resolve({ done: true, ok: true, status: 'success', result: '' }),
    probeWGHub: () => Promise.resolve({ ok: true, rttMs: 12, hint: '' }),
    patrol: () => Promise.resolve(undefined),
    addWGDevice: (name) => {
      const d = engine.addWGDevice(name);
      return Promise.resolve({ id: d.id, wg_ip: d.wg_ip, warn: d.warn });
    },
    deleteWGDevice: (id) => Promise.resolve(engine.deleteWGDevice(id)).then(() => undefined),
    getWGDeviceConf: (id) => Promise.resolve({ conf: engine.getWGDeviceConf(id), filename: 'wg0.conf' }),

    getFRPOverview: () => Promise.resolve(engine.getFRPOverview()),
    toggleFRPTunnel: (id) => Promise.resolve(engine.toggleFRPTunnel(id)).then(() => undefined),
    createFRPTunnel: (payload) => Promise.resolve(engine.createFRPTunnel(payload)).then(() => undefined),
    deleteFRPTunnel: (id) => Promise.resolve(engine.deleteFRPTunnel(id)).then(() => undefined),
    saveFRPPlatformToken: (p, token) =>
      Promise.resolve(engine.saveFRPToken(p.id ?? 0, token)).then(() => undefined),
    syncFRPPlatform: () => Promise.resolve(undefined),

    getSettings: () => Promise.resolve(engine.getSettings()),
    saveSettings: (s) => Promise.resolve(engine.saveSettings(s)).then(() => undefined),

    changePassword: (oldPw, newPw) => Promise.resolve(engine.changePassword(oldPw, newPw)),

    getAuditLogs: (query = '', page = 1, pageSize = 10) => Promise.resolve(engine.getAuditLogs(query, page, pageSize)),
  };
}

// ---------- 出口 ----------

// 惰性初始化：mock 路径动态引入（real 构建不含仿真引擎）；real 直接同步可用。
// 首个调用触发初始化，后续调用复用同一实例。
let apiInstance: ToyApi | null = null;

function getApi(): ToyApi {
  if (!apiInstance) {
    apiInstance = real;
  }
  return apiInstance;
}

async function ensureApi(): Promise<ToyApi> {
  if (!apiInstance) {
    apiInstance = USE_MOCK ? await mockApi() : real;
  }
  return apiInstance;
}

export const api: ToyApi = new Proxy({} as ToyApi, {
  get(_t, prop: string) {
    const target = getApi() as unknown as Record<string, unknown>;
    const v = target[prop];
    return typeof v === 'function' ? v.bind(target) : v;
  },
});

export async function initApi(): Promise<void> {
  await ensureApi();
}

// 兼容旧引用路径
export { summarizeServers, normalizeHistoryPoints };
export type { HistorySeries };
export { ApiError, notifyUnauthorized };
