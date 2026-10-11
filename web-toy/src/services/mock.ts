/**
 * BeaconTower · toy 端仿真引擎（仅显式 VITE_API_MODE=mock 的 dev 会话启用）
 * 生产构建 tree-shake 掉本文件；真实契约见 services/real.ts。
 * 保留精简版假数据：联调 UI 用，不含任何真实凭据与后门（登录须走 api.login，
 * mock 密码同样要求 ≥10 位强度，杜绝弱口令习惯）。
 */

import type {
  PublicSummary,
  ServerItem,
  WGPublicSnapshot,
  FRPPublicSnapshot,
  WGOverviewData,
  FRPPlatformItem,
  FRPTunnelItem,
  SystemSettingsData,
  AuditLogItem,
} from '../types/backend';

// ---------- 仿真数据 ----------

const INITIAL_SERVERS: ServerItem[] = [
  {
    id: 1, name: 'Tokyo-Master-01', region: 'JP 东京', region_source: 'auto',
    tags: ['核心', 'WG-Hub', '生产'], note_public: '东京主算力节点 · BGP 高速线路',
    status: 'online', is_self: true,
    profile: { os: 'Debian', os_version: '12.5 (bookworm)', arch: 'x86_64', cpu_cores: 8, mem_total: 16 * 1024 ** 3, disk_total: 512 * 1024 ** 3, virt: 'kvm' },
    metrics: { ts: 0, cpu_pct: 24.5, mem_used: 6.2 * 1024 ** 3, swap_used: 512 * 1024 ** 2, disk_used: 184 * 1024 ** 3, net_in_bps: 1.55 * 1024 ** 2 * 10, net_out_bps: 6.12 * 1024 ** 2 * 10, tcp_conns: 168, udp_conns: 42, load1: 0.85, load5: 0.62, load15: 0.45, uptime_s: 45 * 86400 + 3600 * 14, processes: 182 },
    power: { total_w: 18.5, cpu_w: 9.8, dram_w: 2.4, temp_c: 48.2, freq_mhz: 3200, power_source: 'ac', today_kwh: null, month_kwh: 13.32, est_cost_month: 9.32 },
    traffic_today: { in_total: 14.8 * 1024 ** 3, out_total: 28.6 * 1024 ** 3 },
  },
  {
    id: 2, name: 'HongKong-Edge-02', region: 'HK 香港', region_source: 'manual',
    tags: ['穿透网关', 'FRP-Host'], note_public: 'CN2 GIA 专线 · 低延迟反代',
    status: 'online', is_self: false,
    profile: { os: 'Ubuntu', os_version: '24.04 LTS', arch: 'x86_64', cpu_cores: 4, mem_total: 8 * 1024 ** 3, disk_total: 120 * 1024 ** 3, virt: 'kvm' },
    metrics: { ts: 0, cpu_pct: 14.2, mem_used: 2.8 * 1024 ** 3, swap_used: 0, disk_used: 42 * 1024 ** 3, net_in_bps: 890 * 1024, net_out_bps: 2.1 * 1024 ** 2 * 10, tcp_conns: 94, udp_conns: 18, load1: 0.35, load5: 0.28, load15: 0.21, uptime_s: 18 * 86400 + 3600 * 6, processes: 114 },
    power: null,
    traffic_today: { in_total: 8.2 * 1024 ** 3, out_total: 19.5 * 1024 ** 3 },
  },
  {
    id: 3, name: 'HomeLab-NAS-Intel', region: 'CN 上海', region_source: 'auto',
    tags: ['NAS', 'Docker', '功耗监测'], note_public: '家庭私有云存储 · 实时功耗计量',
    status: 'online', is_self: false,
    profile: { os: 'Arch Linux', os_version: 'Rolling', arch: 'x86_64', cpu_cores: 16, mem_total: 32 * 1024 ** 3, disk_total: 4096 * 1024 ** 3, virt: 'physical' },
    metrics: { ts: 0, cpu_pct: 38.6, mem_used: 19.4 * 1024 ** 3, swap_used: 1024 ** 3, disk_used: 2840 * 1024 ** 3, net_in_bps: 3.4 * 1024 ** 2 * 10, net_out_bps: 1.2 * 1024 ** 2 * 10, tcp_conns: 240, udp_conns: 86, load1: 1.45, load5: 1.2, load15: 0.98, uptime_s: 92 * 86400 + 3600 * 2, processes: 260 },
    power: { total_w: 36.4, cpu_w: 22.1, dram_w: 5.6, temp_c: 54.8, freq_mhz: 3800, power_source: 'ac', today_kwh: null, month_kwh: 26.2, est_cost_month: 18.34 },
    traffic_today: { in_total: 32.4 * 1024 ** 3, out_total: 12.1 * 1024 ** 3 },
  },
  {
    id: 8, name: 'Legacy-Node-Standby', region: 'CN 北京', region_source: 'manual',
    tags: ['维护中', '离线备用'], note_public: '旧系统维护停机中',
    status: 'offline', is_self: false,
    profile: { os: 'CentOS', os_version: '7.9', arch: 'x86_64', cpu_cores: 4, mem_total: 8 * 1024 ** 3, disk_total: 100 * 1024 ** 3, virt: 'kvm' },
    metrics: { ts: 0, cpu_pct: 0, mem_used: 0, swap_used: 0, disk_used: 0, net_in_bps: 0, net_out_bps: 0, tcp_conns: 0, udp_conns: 0, load1: 0, load5: 0, load15: 0, uptime_s: 0, processes: 0 },
    power: null,
  },
];

export const INITIAL_WG: WGPublicSnapshot = {
  initialized: true,
  hubs: [{ name: 'Tokyo-Master-01', is_active: true, healthy: true, month_rx: 210 * 1024 ** 3, month_tx: 202 * 1024 ** 3 }],
  peers: INITIAL_SERVERS.slice(0, 3).map((s, i) => ({ name: s.name, online: s.status === 'online' && i < 2 })),
  summary: {
    hub_active: 1, hub_healthy: 1, member_total: 3, member_online: 2,
    device_total: 3, traffic_today: 18.4 * 1024 ** 3, traffic_month: 412.6 * 1024 ** 3, checked_at: Math.floor(Date.now() / 1000),
  },
};

export const INITIAL_FRP: FRPPublicSnapshot = {
  platforms: [
    { name: 'Sakura FRP', kind: 'sakura', online: true, tunnel_total: 2, tunnel_online: 2, conns: 7, conns_src: 'platform', traffic_today: 14.2 * 1024 ** 3, nodes: [{ name: 'jp-tyo-01', group: '', online: true, load: 0.2, uptime: '12d' }], updated_at: Math.floor(Date.now() / 1000) },
    { name: 'Cloudflare', kind: 'cloudflare', online: true, tunnel_total: 1, tunnel_online: 1, conns: 2, conns_src: 'local', traffic_today: 3.8 * 1024 ** 3, nodes: [{ name: 'Cloudflare 连接器', group: '', online: true, load: 0.1, uptime: '30d' }], updated_at: Math.floor(Date.now() / 1000) },
  ],
  summary: { platform_total: 2, tunnel_total: 3, tunnel_online: 3, conns: 9, traffic_today: 18.0 * 1024 ** 3, node_in_use: 2, node_online: 2, updated_at: Math.floor(Date.now() / 1000) },
};

// ---------- 仿真引擎 ----------

export class MockDataEngine {
  private servers: ServerItem[] = JSON.parse(JSON.stringify(INITIAL_SERVERS));
  private listeners: ((servers: ServerItem[], summary: PublicSummary) => void)[] = [];
  private timer: ReturnType<typeof setInterval> | null = null;
  private loggedInFlag = false;

  getServers(): ServerItem[] {
    return this.servers;
  }

  getSummary(): PublicSummary {
    return summarize(this.servers);
  }

  subscribe(cb: (servers: ServerItem[], summary: PublicSummary) => void): void {
    this.listeners.push(cb);
    cb(this.servers, this.getSummary());
  }

  startSimulation(): void {
    if (this.timer) return;
    this.timer = setInterval(() => {
      for (const s of this.servers) {
        if (s.status === 'online') {
          s.metrics.cpu_pct = Math.min(99, Math.max(3, parseFloat((s.metrics.cpu_pct + (Math.random() - 0.48) * 3).toFixed(1))));
          s.metrics.net_in_bps = Math.round(s.metrics.net_in_bps * (0.9 + Math.random() * 0.2));
          s.metrics.net_out_bps = Math.round(s.metrics.net_out_bps * (0.9 + Math.random() * 0.2));
          if (s.power) s.power.total_w = Math.max(5, parseFloat((s.power.total_w + (Math.random() - 0.5) * 0.6).toFixed(1)));
        }
      }
      for (const cb of this.listeners) cb(this.servers, this.getSummary());
    }, 2000);
  }

  stopSimulation(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  isRunning(): boolean {
    return this.timer !== null;
  }

  // 会话模拟：只在内存标记，不写任何 token 到 localStorage（生产无此路径）
  isLoggedIn(): boolean {
    return this.loggedInFlag;
  }

  login(_username: string, password: string): { success: boolean; msg?: string } {
    // 与真实策略同强度：≥10 位，杜绝弱口令习惯带进生产
    if (password.length >= 10) {
      this.loggedInFlag = true;
      return { success: true };
    }
    return { success: false, msg: '（mock）密码长度至少 10 位' };
  }

  logout(): void {
    this.loggedInFlag = false;
  }

  deleteServer(id: number): boolean {
    const idx = this.servers.findIndex((s) => s.id === id);
    if (idx !== -1) {
      this.servers.splice(idx, 1);
      return true;
    }
    return false;
  }

  getWGOverview(): WGOverviewData {
    return {
      network: { subnet: '10.66.66.0/24', hub_ip: '10.66.66.1', iface: 'wg0', keepalive: 25, mtu: 1420, active_hub_server_id: 1 },
      hubs: [
        { server_id: 1, name: 'Tokyo-Master-01', listen_port: 51820, endpoint: '141.147.18.92:51820', status: 'ok', last_error: '', quota_gb: null, month_rx: 142 * 1024 ** 3, month_tx: 284 * 1024 ** 3, month_billed: 284 * 1024 ** 3, is_active: true, has_keys: true },
        { server_id: 2, name: 'HongKong-Edge-02', listen_port: 51820, endpoint: '43.154.88.102:51820', status: 'ok', last_error: '', quota_gb: null, month_rx: 12 * 1024 ** 3, month_tx: 24 * 1024 ** 3, month_billed: 24 * 1024 ** 3, is_active: false, has_keys: true },
      ],
      retired_hubs: [],
      peers: [
        { id: 1, kind: 'server', server_id: 1, name: 'Tokyo-Master-01', wg_ip: '10.66.66.1', public_key: 'MOCK', managed: true, status: 'online', last_error: '', last_handshake: Math.floor(Date.now() / 1000) - 12, rx_bytes: 142 * 1024 ** 3, tx_bytes: 284 * 1024 ** 3, can_export: true },
        { id: 2, kind: 'server', server_id: 2, name: 'HongKong-Edge-02', wg_ip: '10.66.66.2', public_key: 'MOCK', managed: true, status: 'online', last_error: '', last_handshake: Math.floor(Date.now() / 1000) - 8, rx_bytes: 48 * 1024 ** 3, tx_bytes: 92 * 1024 ** 3, can_export: true },
        { id: 101, kind: 'device', server_id: null, name: 'Admin-iPhone', wg_ip: '10.66.66.11', public_key: 'MOCK', managed: true, status: 'online', last_error: '', last_handshake: Math.floor(Date.now() / 1000) - 45, rx_bytes: 3.2 * 1024 ** 3, tx_bytes: 1.1 * 1024 ** 3, can_export: true },
      ],
      servers: [
        { id: 1, name: 'Tokyo-Master-01', is_self: true, in_network: true, ssh_ready: true, wg_role: 'hub' },
        { id: 2, name: 'HongKong-Edge-02', is_self: false, in_network: true, ssh_ready: true, wg_role: 'standby' },
      ],
      running_task: false,
    };
  }

  switchWGHub(hubId: number): boolean {
    void hubId;
    return true;
  }

  addWGDevice(name: string): { id: number; wg_ip: string; warn: string } {
    void name;
    return { id: 900, wg_ip: '10.66.66.90', warn: '（mock）设备已入库（仿真数据）' };
  }

  deleteWGDevice(id: number): boolean {
    void id;
    return true;
  }

  getWGDeviceConf(id: number): string {
    void id;
    return '[Interface]\nPrivateKey = MOCK-ONLY-FOR-UI-DEBUG\nAddress = 10.66.66.90/32\n\n[Peer]\nPublicKey = MOCK\nAllowedIPs = 10.66.66.0/24';
  }

  getFRPOverview(): { platforms: FRPPlatformItem[]; tunnels: FRPTunnelItem[] } {
    return {
      platforms: [
        { id: 1, name: 'Sakura FRP', kind: 'sakura', username: 'mock', uid: '', group_name: '', speed_limit: '', realname: '', tunnel_used: 2, tunnel_quota: 10, tunnel_total: 2, tunnel_online: 2, conns: 7, conns_src: 'platform', traffic_day_used: 14.2 * 1024 ** 3, traffic_remain: 48 * 1024 ** 3, traffic_up: 0, traffic_down: 0, status: 'ok', last_error: '', last_sync_at: Math.floor(Date.now() / 1000), has_token: true, profile: '', token_expire_at: 0 },
      ],
      tunnels: [
        { id: 1, platform_id: 1, platform_name: 'Sakura FRP', platform_kind: 'sakura', remote_id: '1', name: 'mock-ssh', proto: 'tcp', node_id: '1', node_name: 'jp-tyo-01', local_ip: '127.0.0.1', local_port: 22, remote: 'jp.natfrp.net:38291', online: true, status: 'ok', status_reason: '', conns: 3, local_conns: 3, today_up: 2 * 1024 ** 3, today_down: 6 * 1024 ** 3, uptime: '12d', client_ver: '', extra: '', lock_edit: false, lock_delete: false, lock_migrate: false, synced_at: Math.floor(Date.now() / 1000) },
      ],
    };
  }

  toggleFRPTunnel(id: number): boolean {
    void id;
    return true;
  }

  createFRPTunnel(payload: Partial<FRPTunnelItem>): FRPTunnelItem {
    return { id: 99, platform_id: payload.platform_id ?? 1, platform_name: 'Sakura FRP', platform_kind: 'sakura', remote_id: '99', name: payload.name || 'mock', proto: 'tcp', node_id: '', node_name: '', local_ip: '127.0.0.1', local_port: 8080, remote: '', online: true, status: 'ok', status_reason: '', conns: 0, local_conns: 0, today_up: 0, today_down: 0, uptime: '', client_ver: '', extra: '', lock_edit: false, lock_delete: false, lock_migrate: false, synced_at: 0 };
  }

  deleteFRPTunnel(id: number): boolean {
    void id;
    return true;
  }

  saveFRPToken(platformId: number, token: string): boolean {
    void platformId;
    void token;
    return true;
  }

  getSettings(): SystemSettingsData {
    return { site_title: '信标塔 · BeaconTower', interval_s: 10, retention_days: 30, open_7d_history: true, show_power_public: true, show_cost_public: true, electric_price: 0.65, strict_host_key: true, private_mode: false };
  }

  saveSettings(s: SystemSettingsData): boolean {
    void s;
    return true;
  }

  changePassword(_o: string, n: string): { success: boolean; msg?: string } {
    if (n.length >= 10) return { success: true };
    return { success: false, msg: '（mock）新密码至少 10 位' };
  }

  getAuditLogs(_query = '', page = 1, pageSize = 10): { list: AuditLogItem[]; total: number; page: number; pages: number } {
    const logs: AuditLogItem[] = [
      { id: 106, ts: Math.floor(Date.now() / 1000) - 60, actor: 'admin', action: 'login', target: 'admin', detail: '（mock）管理员登录成功', source_ip_hash: 'mock' },
      { id: 105, ts: Math.floor(Date.now() / 1000) - 3600, actor: 'admin', action: 'server_test', target: 'Tokyo-Master-01', detail: '（mock）SSH 连通性探测成功', source_ip_hash: 'mock' },
    ];
    const pages = Math.ceil(logs.length / pageSize) || 1;
    return { list: logs.slice((page - 1) * pageSize, page * pageSize), total: logs.length, page, pages };
  }
}

function summarize(servers: ServerItem[]): PublicSummary {
  let online = 0;
  let upBps = 0;
  let downBps = 0;
  let watts = 0;
  let measured = 0;
  let kwh = 0;
  let cost = 0;
  let dayIn = 0;
  let dayOut = 0;
  for (const s of servers) {
    if (s.status === 'online') {
      online++;
      upBps += s.metrics.net_out_bps;
      downBps += s.metrics.net_in_bps;
      if (s.power) {
        measured++;
        watts += s.power.total_w;
        kwh += s.power.month_kwh;
        cost += s.power.est_cost_month || 0;
      }
    }
    if (s.traffic_today) {
      dayIn += s.traffic_today.in_total;
      dayOut += s.traffic_today.out_total;
    }
  }
  const r1 = (v: number) => Math.round(v * 10) / 10;
  return { total: servers.length, online, offline: servers.length - online, up_bps: upBps, down_bps: downBps, watts: r1(watts), measured_count: measured, month_kwh: r1(kwh), est_cost_month: r1(cost), day_in_total: dayIn, day_out_total: dayOut };
}

export const mockEngine = new MockDataEngine();
