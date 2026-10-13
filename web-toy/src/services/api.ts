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
  WGTask,
  AdminServerItem,
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
  WGHubDetailData,
  WGSyncHubsResp,
  WGHubProbeResp,
  WGPlanInput,
  WGPlanResp,
  WGImportInput,
  WGTakeoverInput,
  WGAssetItem,
  WGAssetInput,
  WGAssetProbeResult,
} from '../types/backend';
import {
  realApi,
  stream,
  summarizeServers,
  normalizeHistoryPoints,
  type HistorySeries,
} from './real';
import { ApiError, notifyUnauthorized, saveBlob } from './http';

// 显式开关：npm run dev 仅在 VITE_API_MODE=mock 时走仿真引擎（联调用）。
// 直接读 import.meta.env 的成员（不要 optional chaining）：构建期 Vite 会把
// import.meta.env 内联成字面量，Rollup 才能把 mock 分支与仿真引擎 chunk 一起摇掉。
const USE_MOCK = (import.meta as unknown as { env: Record<string, string | undefined> }).env.VITE_API_MODE === 'mock';
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
  /** 最近一次收到 SSE 帧（snapshot/update/ping/open）的时刻（毫秒）：判活与轮询退化用 */
  lastEventAt(): number;
  // 认证
  getAdminStatus(): Promise<AdminStatus>;
  login(username: string, password: string): Promise<{ success: boolean; msg?: string }>;
  /** 首次初始化（未初始化时建管理员账号，成功后即持会话） */
  setup(payload: { username: string; password: string; setup_token?: string }): Promise<void>;
  logout(): Promise<void>;
  // 节点管理
  /** 管理态节点清单（含 ssh/隐藏/暂停/排序/画像/功耗等公开页不下发的字段） */
  getAdminServers(): Promise<AdminServerItem[]>;
  deleteServer(id: number): Promise<void>;
  updateServer(id: number, payload: Record<string, unknown>): Promise<void>;
  rotateServerKey(id: number, password: string): Promise<{ detail?: string }>;
  relocateServer(id: number): Promise<{ detail?: string; region?: string }>;
  recalibratePower(id: number, baseLoadW: number): Promise<void>;
  reorderServers(ids: number[]): Promise<void>;
  testConnection(name: string): Promise<{ ok: boolean; msg: string }>;
  /** 新节点引导试连：SSH 密码一次性引导装钥，返回面板私钥（内存态）与画像 probe */
  testConnectionBootstrap(ssh: { host: string; port: number; username: string; auth_type: string; password: string }): Promise<{ ok: boolean; msg: string; panelPrivateKey: string; probe: Record<string, unknown> }>;
  createServer(payload: { name: string; region?: string; note_public?: string; ssh: Record<string, unknown>; panel_private_key?: string; probe?: unknown }): Promise<void>;
  // WG 组网
  getWGOverview(): Promise<WGOverviewData>;
  switchWGHub(hubServerId: number): Promise<{ task_id?: number }>;
  pollSwitchTask(taskId: number): Promise<{ done: boolean; ok: boolean; status: string; result: string }>;
  /** 任务详情（含 steps，用于进度弹窗） */
  getWGTask(taskId: number): Promise<WGTask>;
  /** 实测中心 UDP：mode=handshake 为面板侧真握手，reach 为无托管私钥时的粗判 */
  probeWGHub(hubServerId: number): Promise<WGHubProbeResp>;
  patrol(): Promise<void>;
  /** 中心节点面板（概览 / 凭证 / 成员 / 主备就绪度） */
  getWGHubDetail(hubServerId: number): Promise<WGHubDetailData>;
  /** 把成员下发到所有未退役中心 */
  syncWGPeerHubs(peerId: number): Promise<WGSyncHubsResp>;
  addWGDevice(name: string): Promise<{ id: number; wg_ip: string; warn?: string }>;
  deleteWGDevice(id: number): Promise<void>;
  /** 通用成员移出（服务器成员与设备共用 DELETE /wg/peers/:id） */
  deleteWGPeer(id: number): Promise<void>;
  /** hubId 为 0 时后端取现役中心 */
  getWGDeviceConf(id: number, hubId?: number): Promise<{ conf: string; filename: string }>;
  verifyWGPeer(id: number): Promise<{ online: boolean; detail: string }>;
  renameWGPeer(id: number, name: string): Promise<void>;
  reissueWGPeer(id: number): Promise<{ wg_ip?: string; warn?: string }>;
  /** 清理已退役中心的记录 */
  cleanupWGHub(serverId: number): Promise<void>;
  /** 把网外节点配置为中心（备援/现役），返回后台任务 id */
  provisionWGHub(serverId: number, listenPort?: number): Promise<{ task_id: number; role: string; listen_port: number }>;
  /** 只读登记机器上已手工配好的 WG 为备援槽位 */
  registerWGStandby(serverId: number): Promise<{
    endpoint: string;
    listen_port: number;
    status: string;
    udp: { checked: boolean; mode: string; ok: boolean; hint: string };
  }>;
  /** 纳管本机（面板宿主）入网 */
  adoptWGServer(serverId: number): Promise<{ server_id: number; detail: string }>;
  /** 组网预检（dry-run，不落库不改配置） */
  wgPlan(payload: WGPlanInput): Promise<WGPlanResp>;
  /** 执行组网向导（落库 + 后台任务，进度走 getWGTask） */
  wgApply(payload: WGPlanInput): Promise<{ task_id: number }>;
  /** 导入现有网络（读现役 hub 实况收编成员） */
  wgImport(payload: WGImportInput): Promise<{ task_id: number }>;
  /** 新机接管现役中心 */
  wgTakeover(payload: WGTakeoverInput): Promise<{ task_id: number }>;
  // 资产中转
  getWGAssets(): Promise<{ assets: WGAssetItem[] }>;
  upsertWGAsset(payload: WGAssetInput): Promise<{ id: number }>;
  deleteWGAsset(id: number): Promise<void>;
  probeWGAsset(id: number): Promise<{ results: WGAssetProbeResult[] }>;
  fetchWGAsset(id: number): Promise<{ path: string; size: number; sha256: string; via: string }>;
  pushWGAsset(id: number, serverId: number): Promise<void>;
  /** 下载使用端凭证 zip（A=现役 / B=备援） */
  downloadWGCreds(): Promise<void>;
  // FRP（内网穿透）
  getFRPOverview(): Promise<{ platforms: FRPPlatformItem[]; tunnels: FRPTunnelItem[] }>;
  toggleFRPTunnel(id: number, running: boolean): Promise<void>;
  createFRPTunnel(payload: Partial<FRPTunnelItem> & { local_addr?: string }): Promise<void>;
  deleteFRPTunnel(id: number): Promise<void>;
  saveFRPPlatformToken(platform: { id?: number; kind: string }, token: string): Promise<void>;
  /** full=false 为轻量同步（只要账号与隧道，不带节点列表） */
  syncFRPPlatform(id: number, full?: boolean): Promise<void>;
  getFRPNodes(): Promise<{ nodes: FRPNodeItem[] }>;
  getFRPPlatformDetail(id: number): Promise<FRPPlatformDetailData>;
  getFRPFlow(id: number, kind?: string): Promise<{ points: FRPFlowPoint[] }>;
  renameFRPPlatform(id: number, name: string): Promise<void>;
  deleteFRPPlatform(id: number): Promise<void>;
  updateFRPTunnel(id: number, payload: FRPTunnelInput): Promise<void>;
  lockFRPTunnel(id: number, locks: { edit: boolean; delete: boolean; migrate: boolean }): Promise<void>;
  migrateFRPTunnel(id: number, nodeId: string): Promise<void>;
  authFRPTunnel(id: number, ip?: string): Promise<{ ip: string }>;
  createFRPTunnelFull(platformId: number, payload: FRPTunnelInput): Promise<void>;
  /** 下载 frpc 配置（浏览器存盘） */
  downloadFRPTunnelConfig(t: FRPTunnelItem): Promise<void>;
  getFRPTunnelTraffic(id: number): Promise<{ points: { label: string; used: number }[] }>;
  // ChmlFrp 二级域名
  getFRPSubdomains(platformId: number): Promise<{ subdomains: FRPSubdomainItem[] }>;
  getFRPAvailableDomains(platformId: number): Promise<{ domains: { domain: string }[] }>;
  createFRPSubdomain(platformId: number, payload: FRPSubdomainInput): Promise<void>;
  updateFRPSubdomain(platformId: number, payload: FRPSubdomainInput): Promise<void>;
  deleteFRPSubdomain(platformId: number, domain: string, record: string): Promise<void>;
  // 客户端托管（面板经 SSH 在节点上起 frpc 容器）
  getFRPDeployments(refresh?: boolean): Promise<{ deployments: FRPDeployItem[] }>;
  createFRPDeployment(payload: {
    platform_id: number;
    server_id: number;
    tunnel_ids: number[];
    install_docker: boolean;
    image?: string;
  }): Promise<{ deployment: FRPDeployItem; log_tail?: string }>;
  syncFRPDeployment(id: number): Promise<{ deployment: FRPDeployItem; log_tail?: string }>;
  actionFRPDeployment(id: number, action: 'restart' | 'stop' | 'start'): Promise<{ deployment: FRPDeployItem }>;
  statusFRPDeployment(id: number): Promise<{ deployment: FRPDeployItem }>;
  getFRPDeployLogs(id: number, tail?: number): Promise<{ container: string; logs: string }>;
  deleteFRPDeployment(id: number, keep?: boolean): Promise<void>;
  serverDocker(serverId: number, install?: boolean): Promise<{ docker?: FRPDockerStatus; error?: string }>;
  // ChmlFrp 设备码授权
  startFRPDevice(reuseId?: number): Promise<FRPDeviceStartData>;
  pollFRPDevice(sessionId: string): Promise<FRPDevicePollData>;
  cancelFRPDevice(sessionId: string): Promise<void>;
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
  lastEventAt: () => stream.lastEventAt,

  getAdminStatus: () => realApi.getAdminStatus(),
  login: (username, password) => realApi.login(username, password),
  setup: (payload) => realApi.setup(payload),
  logout: () => realApi.logout(),

  deleteServer: (id) => realApi.deleteServer(id),
  getAdminServers: () => realApi.getAdminServers(),
  updateServer: (id, payload) => realApi.updateServer(id, payload),
  rotateServerKey: (id, password) => realApi.rotateServerKey(id, password),
  relocateServer: (id) => realApi.relocate(id),
  recalibratePower: (id, w) => realApi.recalibratePower(id, w),
  reorderServers: (ids) => realApi.reorderServers(ids),
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
  probeWGHub: (hubServerId) => realApi.probeWGHub(hubServerId),
  getWGHubDetail: (hubServerId) => realApi.getWGHubDetail(hubServerId),
  syncWGPeerHubs: (peerId) => realApi.syncWGPeerHubs(peerId),
  patrol: async () => {
    await realApi.patrol();
  },
  addWGDevice: (name) => realApi.addWGDevice(name),
  deleteWGDevice: (id) => realApi.deleteWGDevice(id).then(() => undefined),
  deleteWGPeer: (id) => realApi.deleteWGDevice(id).then(() => undefined),
  getWGDeviceConf: (id, hubId = 0) => realApi.getWGDeviceConf(id, hubId).then((d) => ({ conf: d.conf, filename: d.filename })),
  verifyWGPeer: (id) => realApi.verifyWGPeer(id),
  renameWGPeer: (id, name) => realApi.renameWGPeer(id, name),
  reissueWGPeer: (id) => realApi.reissueWGPeer(id),
  cleanupWGHub: (serverId) => realApi.cleanupWGHub(serverId),
  provisionWGHub: (serverId, listenPort = 0) => realApi.provisionWGHub(serverId, listenPort),
  registerWGStandby: (serverId) => realApi.registerWGStandby(serverId),
  adoptWGServer: (serverId) => realApi.adoptWGServer(serverId),
  wgPlan: (payload) => realApi.wgPlan(payload),
  wgApply: (payload) => realApi.wgApply(payload),
  wgImport: (payload) => realApi.wgImport(payload),
  wgTakeover: (payload) => realApi.wgTakeover(payload),
  getWGAssets: () => realApi.wgAssets(),
  upsertWGAsset: (payload) => realApi.wgAssetUpsert(payload),
  deleteWGAsset: (id) => realApi.wgAssetDelete(id),
  probeWGAsset: (id) => realApi.wgAssetProbe(id),
  fetchWGAsset: (id) => realApi.wgAssetFetch(id),
  pushWGAsset: (id, serverId) => realApi.wgAssetPush(id, serverId),
  async downloadWGCreds() {
    const { blob, filename } = await realApi.downloadWGCreds();
    saveBlob(blob, filename || 'beacontower-wg-creds.zip');
  },
  getWGTask: (taskId) => realApi.getWGTask(taskId),

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
    if (platform.kind === 'cloudflared') {
      await realApi.bindCloudflaredPlatform('Cloudflare', token);
    } else if (platform.kind === 'natfrp') {
      await realApi.bindSakuraPlatform('Sakura', token);
    } else {
      // ChmlFrp 走 OAuth2 设备码（交互式授权），不支持静态 token 绑定
      throw new ApiError({ code: 'UNSUPPORTED', message: 'ChmlFrp 走设备码授权（不支持静态令牌），请在穿透页使用 ChmlFrp 授权流程' });
    }
  },
  syncFRPPlatform: (id, full = true) => realApi.syncFRPPlatform(id, full),
  getFRPNodes: () => realApi.getFRPNodes(),
  getFRPPlatformDetail: (id) => realApi.getFRPPlatformDetail(id),
  getFRPFlow: (id, kind = 'day') => realApi.getFRPFlow(id, kind),
  renameFRPPlatform: (id, name) => realApi.renameFRPPlatform(id, name),
  deleteFRPPlatform: (id) => realApi.deleteFRPPlatform(id),
  updateFRPTunnel: (id, payload) => realApi.updateFRPTunnel(id, payload),
  lockFRPTunnel: (id, locks) => realApi.lockFRPTunnel(id, locks),
  migrateFRPTunnel: (id, nodeId) => realApi.migrateFRPTunnel(id, nodeId),
  authFRPTunnel: (id, ip = '') => realApi.authFRPTunnel(id, ip),
  async createFRPTunnelFull(platformId, payload) {
    const r = await realApi.createFRPTunnel(platformId, payload);
    // 建完平台的回读同步失败不算致命：隧道已经建成了，只是列表要等下一次同步
    if (r?.sync_error) throw new ApiError({ code: 'UPSTREAM', message: `隧道已创建，但回读同步失败：${r.sync_error}` });
  },
  async downloadFRPTunnelConfig(t) {
    const { blob, filename } = await realApi.downloadFRPTunnelConfig(t.id);
    saveBlob(blob, filename || `${t.platform_kind}-${t.name}.conf`);
  },
  getFRPTunnelTraffic: (id) => realApi.getFRPTunnelTraffic(id),
  getFRPSubdomains: (platformId) => realApi.getFRPSubdomains(platformId),
  getFRPAvailableDomains: (platformId) => realApi.getFRPAvailableDomains(platformId),
  createFRPSubdomain: (platformId, payload) => realApi.createFRPSubdomain(platformId, payload),
  updateFRPSubdomain: (platformId, payload) => realApi.updateFRPSubdomain(platformId, payload),
  deleteFRPSubdomain: (platformId, domain, record) => realApi.deleteFRPSubdomain(platformId, domain, record),
  getFRPDeployments: (refresh = false) => realApi.getFRPDeployments(refresh),
  createFRPDeployment: (payload) => realApi.createFRPDeployment(payload),
  syncFRPDeployment: (id) => realApi.syncFRPDeployment(id),
  actionFRPDeployment: (id, action) => realApi.actionFRPDeployment(id, action),
  statusFRPDeployment: (id) => realApi.statusFRPDeployment(id),
  getFRPDeployLogs: (id, tail = 200) => realApi.getFRPDeployLogs(id, tail),
  deleteFRPDeployment: (id, keep = false) => realApi.deleteFRPDeployment(id, keep),
  serverDocker: (serverId, install = false) => realApi.serverDocker(serverId, install),
  startFRPDevice: (reuseId = 0) => realApi.startFRPDevice(reuseId),
  pollFRPDevice: (sessionId) => realApi.pollFRPDevice(sessionId),
  cancelFRPDevice: (sessionId) => realApi.cancelFRPDevice(sessionId),

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
    lastEventAt: () => Date.now(),

    getAdminStatus: () =>
      Promise.resolve({ initialized: true, loggedIn: engine.isLoggedIn(), site_title: '信标塔 · BeaconTower', username: engine.isLoggedIn() ? 'admin' : '' }),
    login: (username, password) => Promise.resolve(engine.login(username, password)),
    setup: () => Promise.resolve(undefined),
    logout: () => Promise.resolve(engine.logout()),

    deleteServer: (id) => Promise.resolve(engine.deleteServer(id)).then(() => undefined),
    getAdminServers: () => Promise.resolve(engine.getAdminServers()),
    updateServer: () => Promise.resolve(undefined),
    rotateServerKey: () => Promise.resolve({ detail: '（mock）已重建密钥' }),
    relocateServer: () => Promise.resolve({ detail: '（mock）已重新定位', region: 'JP 东京' }),
    recalibratePower: () => Promise.resolve(undefined),
    reorderServers: () => Promise.resolve(undefined),
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
    getWGTask: () =>
      Promise.resolve({
        id: 0, kind: 'mock', status: 'success', result: '', created_at: 0, finished_at: null,
        steps: [{ id: 1, seq: 0, server_id: null, title: '（mock）模拟步骤', status: 'success', log: '' }],
      } as WGTask),
    probeWGHub: () =>
      Promise.resolve({
        mode: 'handshake', ok: true, skipped: false, reason: '', peer: '（mock）探针成员',
        target: '（mock）中心端点', attempt: 1, rtt_ms: 12, hint: '',
      } as WGHubProbeResp),
    getWGHubDetail: (id) => Promise.resolve(engine.getWGHubDetail(id)),
    syncWGPeerHubs: () => Promise.resolve({ synced: 3, total: 3, results: [] }),
    patrol: () => Promise.resolve(undefined),
    addWGDevice: (name) => {
      const d = engine.addWGDevice(name);
      return Promise.resolve({ id: d.id, wg_ip: d.wg_ip, warn: d.warn });
    },
    deleteWGDevice: (id) => Promise.resolve(engine.deleteWGDevice(id)).then(() => undefined),
    deleteWGPeer: (id) => Promise.resolve(engine.deleteWGDevice(id)).then(() => undefined),
    getWGDeviceConf: (id) => Promise.resolve({ conf: engine.getWGDeviceConf(id), filename: 'wg0.conf' }),
    verifyWGPeer: () => Promise.resolve({ online: true, detail: '（mock）握手正常' }),
    renameWGPeer: () => Promise.resolve(undefined),
    reissueWGPeer: () => Promise.resolve({}),
    cleanupWGHub: () => Promise.resolve(undefined),
    provisionWGHub: () => Promise.resolve({ task_id: 0, role: 'standby', listen_port: 51830 }),
    registerWGStandby: () =>
      Promise.resolve({ endpoint: 'mock:51830', listen_port: 51830, status: 'ready', udp: { checked: true, mode: 'handshake', ok: true, hint: '' } }),
    adoptWGServer: () => Promise.resolve({ server_id: 0, detail: '（mock）已纳管' }),
    wgPlan: (payload) =>
      Promise.resolve({
        network: { subnet: payload.network?.subnet || '10.66.66.0/24', hub_ip: payload.network?.hub_ip || '10.66.66.1', iface: 'wg0', keepalive: 25, mtu: 1420 },
        hub: { server_id: payload.hub_server_id, name: '（mock）中心节点', listen_port: payload.hub_port || 51820, role: 'hub', issues: [] },
        spokes: payload.spokes.map((s) => ({ server_id: s.server_id, name: `（mock）成员 #${s.server_id}`, wg_ip: s.wg_ip || '10.66.66.9', allocated: true, issues: [{ level: 'warn' as const, msg: '（mock）接口已存在，将重配' }] })),
        blocked: false,
      }),
    wgApply: () => Promise.resolve({ task_id: 0 }),
    wgImport: () => Promise.resolve({ task_id: 0 }),
    wgTakeover: () => Promise.resolve({ task_id: 0 }),
    getWGAssets: () => Promise.resolve({ assets: [] }),
    upsertWGAsset: () => Promise.resolve({ id: 0 }),
    deleteWGAsset: () => Promise.resolve(undefined),
    probeWGAsset: () => Promise.resolve({ results: [] }),
    fetchWGAsset: () => Promise.resolve({ path: '/data/assets/mock', size: 0, sha256: '', via: '（mock）' }),
    pushWGAsset: () => Promise.resolve(undefined),
    downloadWGCreds: () =>
      Promise.resolve(undefined).then(() => {
        saveBlob(new Blob(['（mock）凭证包'], { type: 'application/zip' }), 'beacontower-wg-creds.zip');
      }),

    getFRPOverview: () => Promise.resolve(engine.getFRPOverview()),
    toggleFRPTunnel: (id) => Promise.resolve(engine.toggleFRPTunnel(id)).then(() => undefined),
    createFRPTunnel: (payload) => Promise.resolve(engine.createFRPTunnel(payload)).then(() => undefined),
    deleteFRPTunnel: (id) => Promise.resolve(engine.deleteFRPTunnel(id)).then(() => undefined),
    saveFRPPlatformToken: (p, token) =>
      Promise.resolve(engine.saveFRPToken(p.id ?? 0, token)).then(() => undefined),
    syncFRPPlatform: () => Promise.resolve(undefined),
    getFRPNodes: () => Promise.resolve({ nodes: engine.getFRPNodes() }),
    getFRPPlatformDetail: (id) => Promise.resolve(engine.getFRPPlatformDetail(id)),
    getFRPFlow: () => Promise.resolve({ points: engine.getFRPFlow() }),
    renameFRPPlatform: () => Promise.resolve(undefined),
    deleteFRPPlatform: () => Promise.resolve(undefined),
    updateFRPTunnel: () => Promise.resolve(undefined),
    lockFRPTunnel: () => Promise.resolve(undefined),
    migrateFRPTunnel: () => Promise.resolve(undefined),
    authFRPTunnel: () => Promise.resolve({ ip: '（mock）当前来源 IP' }),
    createFRPTunnelFull: () => Promise.resolve(undefined),
    downloadFRPTunnelConfig: () =>
      Promise.resolve(undefined).then(() => {
        saveBlob(new Blob(['# （mock）frpc 配置示例\n'], { type: 'text/plain' }), 'mock-frpc.conf');
      }),
    getFRPTunnelTraffic: () => Promise.resolve({ points: [] }),
    getFRPSubdomains: () => Promise.resolve({ subdomains: engine.getFRPSubdomains() }),
    getFRPAvailableDomains: () => Promise.resolve({ domains: engine.getFRPAvailableDomains() }),
    createFRPSubdomain: () => Promise.resolve(undefined),
    updateFRPSubdomain: () => Promise.resolve(undefined),
    deleteFRPSubdomain: () => Promise.resolve(undefined),
    getFRPDeployments: () => Promise.resolve({ deployments: engine.getFRPDeployments() }),
    createFRPDeployment: () =>
      Promise.reject(new ApiError({ code: 'UNSUPPORTED', message: '（mock）不支持客户端托管' })),
    syncFRPDeployment: () =>
      Promise.reject(new ApiError({ code: 'UNSUPPORTED', message: '（mock）不支持客户端托管' })),
    actionFRPDeployment: () =>
      Promise.reject(new ApiError({ code: 'UNSUPPORTED', message: '（mock）不支持客户端托管' })),
    statusFRPDeployment: () =>
      Promise.reject(new ApiError({ code: 'UNSUPPORTED', message: '（mock）不支持客户端托管' })),
    getFRPDeployLogs: () => Promise.resolve({ container: '', logs: '（mock）无日志' }),
    deleteFRPDeployment: () => Promise.resolve(undefined),
    serverDocker: () => Promise.resolve({ docker: { present: true, version: '27.0.0', daemon_ok: true, err: '' } }),
    startFRPDevice: () =>
      Promise.resolve({ session_id: 'mock', user_code: 'MOCK-CODE', verify_url: 'https://example.com', expires_in: 600, interval: 5 }),
    pollFRPDevice: () => Promise.resolve({ status: 'pending' }),
    cancelFRPDevice: () => Promise.resolve(undefined),

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
