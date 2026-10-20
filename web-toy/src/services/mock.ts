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
  WGHubDetailData,
  FRPPlatformItem,
  FRPTunnelItem,
  FRPNodeItem,
  FRPPlatformDetailData,
  FRPDeployItem,
  FRPSubdomainItem,
  SystemSettingsData,
  AuditLogItem,
  AdminServerItem,
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
    { name: 'Sakura FRP', kind: 'natfrp', online: true, tunnel_total: 2, tunnel_online: 2, conns: 7, conns_src: 'platform', traffic_today: 14.2 * 1024 ** 3, nodes: [{ name: 'jp-tyo-01', group: '', online: true, load: 0.2, uptime: 12 * 86400 }], updated_at: Math.floor(Date.now() / 1000) },
    { name: 'Cloudflare', kind: 'cloudflared', online: true, tunnel_total: 1, tunnel_online: 1, conns: 2, conns_src: 'local', traffic_today: 3.8 * 1024 ** 3, nodes: [{ name: 'Cloudflare 连接器', group: '', online: true, load: 0.1, uptime: 30 * 86400 }], updated_at: Math.floor(Date.now() / 1000) },
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
    if (!this.listeners.includes(cb)) this.listeners.push(cb);
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

  // 管理态清单（含 ssh/隐藏/暂停/排序/画像），供节点管理页联调
  getAdminServers(): AdminServerItem[] {
    return this.servers.map((s) => ({
      id: s.id,
      name: s.name,
      region: s.region,
      region_source: s.region_source,
      tags: s.tags,
      note_public: s.note_public,
      note_private: '（mock）私有备注',
      sort_order: s.id,
      hidden: false,
      enabled: true,
      is_self: s.is_self,
      created_at: Math.floor(Date.now() / 1000) - 86400 * 30,
      ssh: {
        local: s.is_self,
        host: s.is_self ? 'host.docker.internal' : `10.0.0.${s.id}`,
        port: 22,
        username: 'root',
        auth_type: 'key',
        has_password: false,
        has_key: !s.is_self,
        host_key_fp: 'SHA256:（mock）',
        ssh_ready: true,
      },
      profile: {
        hostname: s.name,
        os_name: s.profile.os,
        os_version: s.profile.os_version,
        kernel: '（mock）6.x',
        arch: s.profile.arch,
        cpu_model: '（mock）Virtual CPU',
        cpu_cores: s.profile.cpu_cores,
        mem_total: s.profile.mem_total,
        disk_total: s.profile.disk_total,
        virt: s.profile.virt,
        public_ip: '',
        geo_country: s.region,
        geo_city: s.region,
        power: { rapl: !!s.power, battery: false, base_load_w: 0, base_load_source: 'default' },
      },
      power: { rapl: !!s.power, battery: false, base_load_w: 0, base_load_source: 'default', last_error: '' },
      last_success_at: Math.floor(Date.now() / 1000),
      ssh_suggest_host: 'host.docker.internal',
    } as AdminServerItem));
  }

  getWGOverview(): WGOverviewData {
    return {
      network: { subnet: '10.66.66.0/24', hub_ip: '10.66.66.1', iface: 'wg0', keepalive: 25, mtu: 1420, active_hub_server_id: 1 },
      hubs: [
        { server_id: 1, name: 'Tokyo-Master-01', listen_port: 51820, endpoint: '141.147.18.92:51820', status: 'ok', last_error: '', quota_gb: null, month_rx: 142 * 1024 ** 3, month_tx: 284 * 1024 ** 3, month_billed: 284 * 1024 ** 3, is_active: true, has_keys: true },
        { server_id: 2, name: 'HongKong-Edge-02', listen_port: 51820, endpoint: '43.154.88.102:51820', status: 'ok', last_error: '', quota_gb: null, month_rx: 12 * 1024 ** 3, month_tx: 24 * 1024 ** 3, month_billed: 24 * 1024 ** 3, is_active: false, has_keys: true },
        { server_id: 3, name: 'Rack-Standby-03', listen_port: 51830, endpoint: '107.173.35.9:51830', status: 'ok', last_error: '', quota_gb: null, month_rx: 96, month_tx: 184, month_billed: 184, is_active: false, has_keys: false },
      ],
      retired_hubs: [],
      peers: [
        { id: 1, kind: 'server', server_id: 1, name: 'Tokyo-Master-01', wg_ip: '10.66.66.1', public_key: 'MOCK', managed: true, status: 'online', last_error: '', last_handshake: Math.floor(Date.now() / 1000) - 12, rx_bytes: 142 * 1024 ** 3, tx_bytes: 284 * 1024 ** 3, can_export: true },
        { id: 2, kind: 'server', server_id: 2, name: 'HongKong-Edge-02', wg_ip: '10.66.66.2', public_key: 'MOCK', managed: true, status: 'online', last_error: '', last_handshake: Math.floor(Date.now() / 1000) - 8, rx_bytes: 48 * 1024 ** 3, tx_bytes: 92 * 1024 ** 3, can_export: true },
        { id: 3, kind: 'server', server_id: 3, name: 'Rack-Standby-03', wg_ip: '10.66.66.3', public_key: 'MOCK', managed: false, status: 'offline', last_error: '巡检未回包（备援待命）', last_handshake: null, rx_bytes: 96, tx_bytes: 184, can_export: false },
        { id: 101, kind: 'device', server_id: null, name: 'Admin-iPhone', wg_ip: '10.66.66.11', public_key: 'MOCK', managed: true, status: 'online', last_error: '', last_handshake: Math.floor(Date.now() / 1000) - 45, rx_bytes: 3.2 * 1024 ** 3, tx_bytes: 1.1 * 1024 ** 3, can_export: true },
        { id: 102, kind: 'device', server_id: null, name: 'Imported-Laptop', wg_ip: '10.66.66.12', public_key: 'MOCK', managed: false, status: 'online', last_error: '', last_handshake: Math.floor(Date.now() / 1000) - 300, rx_bytes: 1.4 * 1024 ** 3, tx_bytes: 0.6 * 1024 ** 3, can_export: false },
      ],
      servers: [
        { id: 1, name: 'Tokyo-Master-01', is_self: true, in_network: true, ssh_ready: true, wg_role: 'hub' },
        { id: 2, name: 'HongKong-Edge-02', is_self: false, in_network: true, ssh_ready: true, wg_role: 'standby' },
        { id: 3, name: 'Rack-Standby-03', is_self: false, in_network: true, ssh_ready: true, wg_role: 'standby' },
      ],
      running_task: false,
    };
  }

  // 中心节点面板：与后端口径一致——hub 是被点的这台，creds=设备、members=SSH 节点
  getWGHubDetail(hubServerId: number): WGHubDetailData {
    const ov = this.getWGOverview();
    const hub = ov.hubs.find((h) => h.server_id === hubServerId) ?? ov.hubs[0];
    const now = Math.floor(Date.now() / 1000);
    return {
      hub: {
        ...hub,
        public_key: 'MOCKPUBKEYMOCKPUBKEYMOCKPUBKEYMOCKPUBKEYMOCK1234=',
        key_fp: 'a1b2 c3d4 e5f6 0708',
        checked_at: now - 120,
        can_render: hub.has_keys,
      },
      creds: ov.peers.filter((p) => p.kind === 'device'),
      members: ov.peers.filter((p) => p.kind === 'server'),
      hubs_readiness: ov.hubs.map((h) => ({
        server_id: h.server_id, name: h.name, is_active: h.is_active, status: h.status,
        ready: h.has_keys && !!h.endpoint,
        missing: h.has_keys ? (h.endpoint ? '' : '端点未知（先完成一次配置或巡检）') : '面板未持有私钥（导入的备援无法在此重渲染）',
        endpoint: h.endpoint, listen_port: h.listen_port,
      })),
      network: {
        subnet: ov.network!.subnet, hub_ip: ov.network!.hub_ip,
        iface: ov.network!.iface, active_hub_server_id: ov.network!.active_hub_server_id,
      },
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
    const now = Math.floor(Date.now() / 1000);
    return {
      platforms: [
        {
          id: 1, name: 'Sakura', kind: 'natfrp', username: 'mock', uid: '10001', group_name: '免费用户',
          speed_limit: '10 Mbps', realname: '已实名', tunnel_used: 2, tunnel_quota: 2, tunnel_total: 2, tunnel_online: 2,
          conns: 0, conns_src: '', traffic_day_used: 561781691, traffic_remain: 24443484504, traffic_up: 0, traffic_down: 0,
          status: 'ok', last_error: '', last_sync_at: now, has_token: true,
          profile: { sign_signed: true, sign_days: 6, sign_traffic: 253.1 }, token_expire_at: 0,
        },
        {
          id: 2, name: 'ChmlFrp', kind: 'chmlfrp', username: 'mock', uid: '20002', group_name: 'VIP',
          speed_limit: '8 Mbps（境外 32 Mbps）', realname: '已实名', tunnel_used: 1, tunnel_quota: 4, tunnel_total: 1, tunnel_online: 1,
          conns: 3, conns_src: 'platform', traffic_day_used: 0, traffic_remain: 0, traffic_up: 0, traffic_down: 4.2 * 1024 ** 3,
          status: 'ok', last_error: '', last_sync_at: now, has_token: true,
          profile: { group_expire: '9999-09-09', integral: 17188, regtime: '2025-05-15' }, token_expire_at: now + 540,
        },
        {
          id: 3, name: 'Cloudflare', kind: 'cloudflared', username: '', uid: '', group_name: '',
          speed_limit: '', realname: '', tunnel_used: 2, tunnel_quota: -1, tunnel_total: 2, tunnel_online: 2,
          conns: 2, conns_src: 'platform', traffic_day_used: 6994, traffic_remain: 0, traffic_up: 0, traffic_down: 0,
          status: 'ok', last_error: '', last_sync_at: now, has_token: true,
          profile: { plan: 'cloudflare-tunnel' }, token_expire_at: 0,
        },
      ],
      tunnels: [
        { id: 1, platform_id: 1, platform_name: 'Sakura', platform_kind: 'natfrp', remote_id: '1', name: 'New', proto: 'tcp', node_id: 'n1', node_name: '重庆三线PLUS4', local_ip: '192.168.0.10', local_port: 3000, remote: '36878', online: true, status: 'normal', status_reason: '', conns: 0, local_conns: 6, today_up: 0, today_down: 0, uptime: 12 * 86400, client_ver: '', extra: '', lock_edit: false, lock_delete: false, lock_migrate: false, synced_at: now },
        { id: 2, platform_id: 1, platform_name: 'Sakura', platform_kind: 'natfrp', remote_id: '2', name: 'ssh', proto: 'tcp', node_id: 'n2', node_name: '长沙电信PLUS2', local_ip: 'localhost', local_port: 22, remote: '29377', online: true, status: 'normal', status_reason: '', conns: 0, local_conns: 2, today_up: 0, today_down: 0, uptime: 30 * 86400, client_ver: '', extra: '', lock_edit: false, lock_delete: false, lock_migrate: false, synced_at: now },
        { id: 3, platform_id: 2, platform_name: 'ChmlFrp', platform_kind: 'chmlfrp', remote_id: '3', name: 'new', proto: 'tcp', node_id: 'n3', node_name: '英国伦敦', local_ip: '192.168.0.10', local_port: 3000, remote: '34280', online: true, status: 'normal', status_reason: '', conns: 0, local_conns: 6, today_up: 517, today_down: 103, uptime: 3600, client_ver: '', extra: '节点 uk.frp.one', lock_edit: false, lock_delete: false, lock_migrate: false, synced_at: now },
        { id: 4, platform_id: 3, platform_name: 'Cloudflare', platform_kind: 'cloudflared', remote_id: '4', name: 'new.example.dev', proto: 'https', node_id: 'new.example.dev', node_name: 'new.example.dev', local_ip: '192.168.0.10', local_port: 3000, remote: 'new.example.dev', online: true, status: 'normal', status_reason: '', conns: 0, local_conns: 0, today_up: 0, today_down: 0, uptime: 0, client_ver: '', extra: 'cloudflare-tunnel · 独立专线（只读）', lock_edit: true, lock_delete: true, lock_migrate: false, synced_at: now },
        { id: 6, platform_id: 3, platform_name: 'Cloudflare', platform_kind: 'cloudflared', remote_id: '6', name: 'tower.example.dev', proto: 'https', node_id: 'tower.example.dev', node_name: 'tower.example.dev', local_ip: '192.168.0.10', local_port: 8091, remote: 'tower.example.dev', online: true, status: 'normal', status_reason: '', conns: 0, local_conns: 0, today_up: 6994, today_down: 0, uptime: 0, client_ver: '', extra: 'cloudflare-tunnel', lock_edit: false, lock_delete: false, lock_migrate: false, synced_at: now },
      ],
    };
  }

  getFRPNodes(): FRPNodeItem[] {
    const now = Math.floor(Date.now() / 1000);
    return [
      { id: 1, platform_id: 1, platform_name: 'Sakura', remote_id: 'n1', name: '重庆三线PLUS4', host: 'frp-cq.example', area: '重庆', group_name: '普通节点', caps: ['http', 'create', 'udp'], online: true, load: 21.5, uptime: 30 * 86400, description: '', in_use: true, synced_at: now },
      { id: 2, platform_id: 1, platform_name: 'Sakura', remote_id: 'n2', name: '长沙电信PLUS2', host: 'frp-cs.example', area: '长沙', group_name: 'VIP 3', caps: ['https', 'mainland'], online: true, load: 63.2, uptime: 12 * 86400, description: '', in_use: true, synced_at: now },
      { id: 3, platform_id: 2, platform_name: 'ChmlFrp', remote_id: 'n3', name: '英国伦敦', host: 'uk.frp.one', area: '伦敦', group_name: '境外', caps: ['tcp', 'udp', 'nodefense'], online: true, load: 12.4, uptime: 6 * 86400, description: '', in_use: true, synced_at: now },
      { id: 9, platform_id: 1, platform_name: 'Sakura', remote_id: 'n9', name: '[BETA]中国台湾CHT1', host: 'frp-few.example', area: '台湾', group_name: '普通节点', caps: ['http', 'create', 'beta'], online: true, load: 87.9, uptime: 86400, description: '', in_use: false, synced_at: now },
      { id: 10, platform_id: 1, platform_name: 'Sakura', remote_id: 'n10', name: '东莞三线PLUS1', host: '', area: '东莞', group_name: 'VIP 3', caps: ['https', 'mainland', 'udp'], online: false, load: 0, uptime: 0, description: '', in_use: false, synced_at: now },
    ];
  }

  getFRPPlatformDetail(id: number): FRPPlatformDetailData {
    const ov = this.getFRPOverview();
    const p = ov.platforms.find((x) => x.id === id) ?? ov.platforms[0];
    return {
      platform: p,
      tunnels: ov.tunnels.filter((t) => t.platform_id === p.id),
      nodes: this.getFRPNodes().filter((n) => n.platform_id === p.id),
      usage: [],
    };
  }

  getFRPFlow(): { label: string; used: number; remain: number }[] {
    const now = Date.now();
    return Array.from({ length: 7 }, (_, i) => {
      const d = new Date(now - (6 - i) * 86400000);
      return {
        label: `${d.getMonth() + 1}/${d.getDate()}`,
        used: (8 + Math.random() * 12) * 1024 ** 3,
        remain: (40 + Math.random() * 20) * 1024 ** 3,
      };
    });
  }

  getFRPDeployments(): FRPDeployItem[] {
    const now = Math.floor(Date.now() / 1000);
    return [
      {
        id: 1, platform_id: 1, platform_kind: 'natfrp', platform_name: 'Sakura', server_id: 1, server_name: 'Tokyo-Master-01',
        tunnels: [
          { id: 1, name: 'New', proto: 'tcp', local_ip: '192.168.0.10', local_port: 3000, remote: '36878' },
          { id: 2, name: 'ssh', proto: 'tcp', local_ip: 'localhost', local_port: 22, remote: '29377' },
        ],
        tunnel_ids: [1, 2], image: 'ghcr.io/example/beacontower-frpc-natfrp:latest', container: 'beacontower-frpc-natfrp-1',
        config_path: '/etc/beacontower-frpc/natfrp/frpc.ini', status: 'running', last_error: '', docker_ver: '29.2.0',
        dirty: false, last_sync_at: now - 120, created_at: now - 86400, updated_at: now - 120,
      },
      {
        id: 2, platform_id: 3, platform_kind: 'cloudflared', platform_name: 'Cloudflare', server_id: 1, server_name: 'Tokyo-Master-01',
        tunnels: [{ id: 5, name: '（已删除）', missing: true }],
        tunnel_ids: [5], image: 'docker.io/cloudflare/cloudflared:latest', container: 'beacontower-frpc-cloudflared-1',
        config_path: '/etc/beacontower-frpc/cloudflared/README.txt', status: 'stopped', last_error: '', docker_ver: 'Docker version 29.2.0, build 0b9d198',
        dirty: true, last_sync_at: now - 5400, created_at: now - 172800, updated_at: now - 5400,
      },
    ];
  }

  getFRPSubdomains(): FRPSubdomainItem[] {
    return [
      { id: 1, domain: 'frp.example.cn', record: 'nas', type: 'A', target: '1.2.3.4', ttl: '10分钟', remarks: '（mock）家庭 NAS' },
      { id: 2, domain: 'frp.example.cn', record: 'web', type: 'CNAME', target: 'uk.frp.one', ttl: '1小时', remarks: '' },
    ];
  }

  getFRPAvailableDomains(): { domain: string }[] {
    return [{ domain: 'frp.example.cn' }, { domain: 'example.top' }];
  }

  toggleFRPTunnel(id: number): boolean {
    void id;
    return true;
  }

  createFRPTunnel(payload: Partial<FRPTunnelItem>): FRPTunnelItem {
    return { id: 99, platform_id: payload.platform_id ?? 1, platform_name: 'Sakura FRP', platform_kind: 'natfrp', remote_id: '99', name: payload.name || 'mock', proto: 'tcp', node_id: '', node_name: '', local_ip: '127.0.0.1', local_port: 8080, remote: '', online: true, status: 'ok', status_reason: '', conns: 0, local_conns: 0, today_up: 0, today_down: 0, uptime: 0, client_ver: '', extra: '', lock_edit: false, lock_delete: false, lock_migrate: false, synced_at: 0 };
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
