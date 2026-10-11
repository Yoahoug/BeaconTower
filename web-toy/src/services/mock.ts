import { 
  PublicSummary, 
  ServerItem, 
  WGPublicSnapshot, 
  FRPPublicSnapshot,
  WGHubItem,
  WGPeerItem,
  WGOverviewData,
  FRPPlatformItem,
  FRPTunnelItem,
  SystemSettingsData,
  AuditLogItem,
  SessionItem
} from '../types/backend';

// 初始模拟数据：高度拟真多节点分布
export const INITIAL_SERVERS: ServerItem[] = [
  {
    id: 1,
    name: 'Tokyo-Master-01',
    region: 'JP 东京',
    region_source: 'auto',
    tags: ['核心', 'WG-Hub', '生产'],
    note_public: '东京主算力节点 · BGP 高速线路',
    status: 'online',
    is_self: true,
    profile: {
      os: 'Debian',
      os_version: '12.5 (bookworm)',
      arch: 'x86_64',
      cpu_cores: 8,
      mem_total: 16 * 1024 * 1024 * 1024,
      disk_total: 512 * 1024 * 1024 * 1024,
      virt: 'kvm',
    },
    metrics: {
      ts: Math.floor(Date.now() / 1000),
      cpu_pct: 24.5,
      mem_used: 6.2 * 1024 * 1024 * 1024,
      swap_used: 512 * 1024 * 1024,
      disk_used: 184 * 1024 * 1024 * 1024,
      net_in_bps: 1.55 * 1024 * 1024,
      net_out_bps: 6.12 * 1024 * 1024,
      tcp_conns: 168,
      udp_conns: 42,
      load1: 0.85,
      load5: 0.62,
      load15: 0.45,
      uptime_s: 45 * 86400 + 3600 * 14,
      processes: 182,
    },
    power: {
      total_w: 18.5,
      cpu_w: 9.8,
      dram_w: 2.4,
      temp_c: 48.2,
      freq_mhz: 3200,
      power_source: 'ac',
      today_kwh: null,
      month_kwh: 13.32,
      est_cost_month: 9.32,
    },
    traffic_today: {
      in_total: 14.8 * 1024 * 1024 * 1024,
      out_total: 28.6 * 1024 * 1024 * 1024,
    },
  },
  {
    id: 2,
    name: 'HongKong-Edge-02',
    region: 'HK 香港',
    region_source: 'manual',
    tags: ['穿透网关', 'FRP-Host'],
    note_public: 'CN2 GIA 专线 · 低延迟反代',
    status: 'online',
    is_self: false,
    profile: {
      os: 'Ubuntu',
      os_version: '24.04 LTS',
      arch: 'x86_64',
      cpu_cores: 4,
      mem_total: 8 * 1024 * 1024 * 1024,
      disk_total: 120 * 1024 * 1024 * 1024,
      virt: 'kvm',
    },
    metrics: {
      ts: Math.floor(Date.now() / 1000),
      cpu_pct: 14.2,
      mem_used: 2.8 * 1024 * 1024 * 1024,
      swap_used: 0,
      disk_used: 42 * 1024 * 1024 * 1024,
      net_in_bps: 890 * 1024,
      net_out_bps: 2.1 * 1024 * 1024,
      tcp_conns: 94,
      udp_conns: 18,
      load1: 0.35,
      load5: 0.28,
      load15: 0.21,
      uptime_s: 18 * 86400 + 3600 * 6,
      processes: 114,
    },
    power: null, // 无 RAPL 功耗计量
    traffic_today: {
      in_total: 8.2 * 1024 * 1024 * 1024,
      out_total: 19.5 * 1024 * 1024 * 1024,
    },
  },
  {
    id: 3,
    name: 'HomeLab-NAS-Intel',
    region: 'CN 上海',
    region_source: 'auto',
    tags: ['NAS', 'Docker', '功耗监测'],
    note_public: '家庭私有云存储 · 实时功耗计量',
    status: 'online',
    is_self: false,
    profile: {
      os: 'Arch Linux',
      os_version: 'Rolling',
      arch: 'x86_64',
      cpu_cores: 16,
      mem_total: 32 * 1024 * 1024 * 1024,
      disk_total: 4096 * 1024 * 1024 * 1024,
      virt: 'physical',
    },
    metrics: {
      ts: Math.floor(Date.now() / 1000),
      cpu_pct: 38.6,
      mem_used: 19.4 * 1024 * 1024 * 1024,
      swap_used: 1024 * 1024 * 1024,
      disk_used: 2840 * 1024 * 1024 * 1024,
      net_in_bps: 3.4 * 1024 * 1024,
      net_out_bps: 1.2 * 1024 * 1024,
      tcp_conns: 240,
      udp_conns: 86,
      load1: 1.45,
      load5: 1.20,
      load15: 0.98,
      uptime_s: 92 * 86400 + 3600 * 2,
      processes: 260,
    },
    power: {
      total_w: 36.4,
      cpu_w: 22.1,
      dram_w: 5.6,
      temp_c: 54.8,
      freq_mhz: 3800,
      power_source: 'ac',
      today_kwh: null,
      month_kwh: 26.2,
      est_cost_month: 18.34,
    },
    traffic_today: {
      in_total: 32.4 * 1024 * 1024 * 1024,
      out_total: 12.1 * 1024 * 1024 * 1024,
    },
  },
  {
    id: 4,
    name: 'Singapore-Cloud-ARM',
    region: 'SG 新加坡',
    region_source: 'auto',
    tags: ['ARM', 'DNS', '测试'],
    note_public: 'ARM 算力节点 · WireGuard 网关',
    status: 'online',
    is_self: false,
    profile: {
      os: 'Debian',
      os_version: '12.4 (bookworm)',
      arch: 'aarch64',
      cpu_cores: 4,
      mem_total: 24 * 1024 * 1024 * 1024,
      disk_total: 200 * 1024 * 1024 * 1024,
      virt: 'kvm',
    },
    metrics: {
      ts: Math.floor(Date.now() / 1000),
      cpu_pct: 8.5,
      mem_used: 4.1 * 1024 * 1024 * 1024,
      swap_used: 0,
      disk_used: 48 * 1024 * 1024 * 1024,
      net_in_bps: 450 * 1024,
      net_out_bps: 720 * 1024,
      tcp_conns: 56,
      udp_conns: 112,
      load1: 0.18,
      load5: 0.15,
      load15: 0.12,
      uptime_s: 63 * 86400,
      processes: 98,
    },
    power: null,
    traffic_today: {
      in_total: 5.6 * 1024 * 1024 * 1024,
      out_total: 8.2 * 1024 * 1024 * 1024,
    },
  },
  {
    id: 5,
    name: 'Frankfurt-Backup-DE',
    region: 'DE 法兰克福',
    region_source: 'auto',
    tags: ['欧洲链路', '冷备'],
    note_public: '异地容灾异构节点',
    status: 'online',
    is_self: false,
    profile: {
      os: 'CentOS',
      os_version: 'Stream 9',
      arch: 'x86_64',
      cpu_cores: 2,
      mem_total: 4 * 1024 * 1024 * 1024,
      disk_total: 80 * 1024 * 1024 * 1024,
      virt: 'kvm',
    },
    metrics: {
      ts: Math.floor(Date.now() / 1000),
      cpu_pct: 5.2,
      mem_used: 1.5 * 1024 * 1024 * 1024,
      swap_used: 0,
      disk_used: 22 * 1024 * 1024 * 1024,
      net_in_bps: 120 * 1024,
      net_out_bps: 180 * 1024,
      tcp_conns: 32,
      udp_conns: 12,
      load1: 0.08,
      load5: 0.06,
      load15: 0.05,
      uptime_s: 31 * 86400,
      processes: 78,
    },
    power: null,
    traffic_today: {
      in_total: 1.8 * 1024 * 1024 * 1024,
      out_total: 2.4 * 1024 * 1024 * 1024,
    },
  },
  {
    id: 6,
    name: 'SiliconValley-Lab',
    region: 'US 硅谷',
    region_source: 'auto',
    tags: ['开发机', 'Docker'],
    note_public: '微服务测试沙箱 · 电池待机',
    status: 'online',
    is_self: false,
    profile: {
      os: 'Ubuntu',
      os_version: '22.04 LTS',
      arch: 'x86_64',
      cpu_cores: 8,
      mem_total: 16 * 1024 * 1024 * 1024,
      disk_total: 256 * 1024 * 1024 * 1024,
      virt: 'physical',
    },
    metrics: {
      ts: Math.floor(Date.now() / 1000),
      cpu_pct: 18.2,
      mem_used: 7.8 * 1024 * 1024 * 1024,
      swap_used: 256 * 1024 * 1024,
      disk_used: 96 * 1024 * 1024 * 1024,
      net_in_bps: 820 * 1024,
      net_out_bps: 1.4 * 1024 * 1024,
      tcp_conns: 88,
      udp_conns: 24,
      load1: 0.42,
      load5: 0.38,
      load15: 0.31,
      uptime_s: 12 * 86400,
      processes: 146,
    },
    power: {
      total_w: 13.5,
      cpu_w: 7.2,
      dram_w: 1.8,
      temp_c: 44.5,
      freq_mhz: 2800,
      power_source: 'battery',
      today_kwh: null,
      month_kwh: 9.72,
      est_cost_month: 6.8,
    },
    traffic_today: {
      in_total: 6.4 * 1024 * 1024 * 1024,
      out_total: 9.8 * 1024 * 1024 * 1024,
    },
  },
  {
    id: 7,
    name: 'Edge-Router-Guangzhou',
    region: 'CN 广州',
    region_source: 'manual',
    tags: ['路由器', '边缘'],
    note_public: '华南内网穿透接入点',
    status: 'online',
    is_self: false,
    profile: {
      os: 'Alpine',
      os_version: '3.19',
      arch: 'x86_64',
      cpu_cores: 2,
      mem_total: 2 * 1024 * 1024 * 1024,
      disk_total: 32 * 1024 * 1024 * 1024,
      virt: 'docker',
    },
    metrics: {
      ts: Math.floor(Date.now() / 1000),
      cpu_pct: 11.0,
      mem_used: 0.6 * 1024 * 1024 * 1024,
      swap_used: 0,
      disk_used: 8 * 1024 * 1024 * 1024,
      net_in_bps: 650 * 1024,
      net_out_bps: 620 * 1024,
      tcp_conns: 45,
      udp_conns: 16,
      load1: 0.22,
      load5: 0.18,
      load15: 0.15,
      uptime_s: 5 * 86400,
      processes: 42,
    },
    power: null,
    traffic_today: {
      in_total: 4.1 * 1024 * 1024 * 1024,
      out_total: 4.3 * 1024 * 1024 * 1024,
    },
  },
  {
    id: 8,
    name: 'Legacy-Node-Standby',
    region: 'CN 北京',
    region_source: 'manual',
    tags: ['维护中', '离线备用'],
    note_public: '旧系统维护停机中',
    status: 'offline',
    is_self: false,
    profile: {
      os: 'CentOS',
      os_version: '7.9',
      arch: 'x86_64',
      cpu_cores: 4,
      mem_total: 8 * 1024 * 1024 * 1024,
      disk_total: 100 * 1024 * 1024 * 1024,
      virt: 'kvm',
    },
    metrics: {
      ts: 0,
      cpu_pct: 0,
      mem_used: 0,
      swap_used: 0,
      disk_used: 0,
      net_in_bps: 0,
      net_out_bps: 0,
      tcp_conns: 0,
      udp_conns: 0,
      load1: 0,
      load5: 0,
      load15: 0,
      uptime_s: 0,
      processes: 0,
    },
    power: null,
  },
];

export const INITIAL_WG: WGPublicSnapshot = {
  initialized: true,
  onlineCount: 6,
  deviceTotal: 4,
  trafficToday: 18.4 * 1024 * 1024 * 1024,
  trafficMonth: 412.6 * 1024 * 1024 * 1024,
  hubViews: [
    {
      name: 'Tokyo-Master-01',
      is_active: true,
      healthy: true,
      month_rx: 210 * 1024 * 1024 * 1024,
      month_tx: 202 * 1024 * 1024 * 1024,
    },
  ],
};

export const INITIAL_FRP: FRPPublicSnapshot = {
  sumTunnel: 5,
  sumOnline: 4,
  sumToday: 24.8 * 1024 * 1024 * 1024,
  platforms: [
    { id: 1, name: 'Sakura FRP', type: 'sakura', onlineTunnels: 2, totalTunnels: 2, todayTraffic: 14.2 * 1024 * 1024 * 1024 },
    { id: 2, name: 'ChmlFrp', type: 'chmlfrp', onlineTunnels: 1, totalTunnels: 2, todayTraffic: 6.8 * 1024 * 1024 * 1024 },
    { id: 3, name: 'Cloudflare', type: 'cloudflare', onlineTunnels: 1, totalTunnels: 1, todayTraffic: 3.8 * 1024 * 1024 * 1024 },
  ],
};

// 工具函数：单位换算与格式化
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

// 模拟动态数据发生器（类似 SSE 实时更新）
export class MockDataEngine {
  private servers: ServerItem[];
  private listeners: ((servers: ServerItem[], summary: PublicSummary) => void)[] = [];
  private timer: any = null;

  constructor() {
    this.servers = JSON.parse(JSON.stringify(INITIAL_SERVERS));
  }

  public getServers(): ServerItem[] {
    return this.servers;
  }

  public getSummary(): PublicSummary {
    let online = 0;
    let upBps = 0;
    let downBps = 0;
    let watts = 0;
    let measuredCount = 0;
    let monthKwh = 0;
    let estCost = 0;
    let dayIn = 0;
    let dayOut = 0;

    for (const s of this.servers) {
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

    return {
      total: this.servers.length,
      online,
      offline: this.servers.length - online,
      up_bps: upBps,
      down_bps: downBps,
      watts: Math.round(watts * 10) / 10,
      measured_count: measuredCount,
      month_kwh: Math.round(monthKwh * 10) / 10,
      est_cost_month: Math.round(estCost * 10) / 10,
      day_in_total: dayIn,
      day_out_total: dayOut,
    };
  }

  public subscribe(cb: (servers: ServerItem[], summary: PublicSummary) => void) {
    this.listeners.push(cb);
    cb(this.servers, this.getSummary());
  }

  public startSimulation() {
    if (this.timer) return;
    this.timer = setInterval(() => {
      // 产生自然的随机微小波动
      for (const s of this.servers) {
        if (s.status === 'online') {
          // CPU 波动 ±2%
          const deltaCpu = (Math.random() - 0.48) * 3;
          s.metrics.cpu_pct = Math.min(99, Math.max(3, parseFloat((s.metrics.cpu_pct + deltaCpu).toFixed(1))));

          // 网速微波动 ±10%
          const factorIn = 0.9 + Math.random() * 0.2;
          const factorOut = 0.9 + Math.random() * 0.2;
          s.metrics.net_in_bps = Math.round(s.metrics.net_in_bps * factorIn);
          s.metrics.net_out_bps = Math.round(s.metrics.net_out_bps * factorOut);

          // 功耗微波动 ±0.3W
          if (s.power) {
            const deltaW = (Math.random() - 0.5) * 0.6;
            s.power.total_w = Math.max(5, parseFloat((s.power.total_w + deltaW).toFixed(1)));
          }
        }
      }
      const summary = this.getSummary();
      for (const cb of this.listeners) {
        cb(this.servers, summary);
      }
    }, 2000);
  }

  public stopSimulation() {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  // 辅助调试：一键切换某个服务器在线/离线
  public toggleServerStatus(id: number) {
    const s = this.servers.find(item => item.id === id);
    if (s) {
      s.status = s.status === 'online' ? 'offline' : 'online';
      const summary = this.getSummary();
      for (const cb of this.listeners) {
        cb(this.servers, summary);
      }
    }
  }

  // ================= WG Mesh 组网数据与操作 =================
  private wgNetwork = {
    cidr: '10.88.0.0/24',
    keepalive_s: 25,
    active_hub_id: 1,
  };

  private wgHubs: WGHubItem[] = [
    {
      id: 1,
      server_id: 1,
      name: 'Tokyo-Master-01',
      endpoint: '141.147.18.92:51820',
      listen_port: 51820,
      is_active: true,
      status: 'online',
      month_rx: 142.5 * 1024 * 1024 * 1024,
      month_tx: 284.1 * 1024 * 1024 * 1024,
      quota_limit_gb: 1000,
    },
    {
      id: 2,
      server_id: 2,
      name: 'HongKong-Edge-02',
      endpoint: '43.154.88.102:51820',
      listen_port: 51820,
      is_active: false,
      status: 'online',
      month_rx: 12.8 * 1024 * 1024 * 1024,
      month_tx: 24.3 * 1024 * 1024 * 1024,
      quota_limit_gb: 500,
    },
  ];

  private wgPeers: WGPeerItem[] = [
    { id: 1, name: 'Tokyo-Master-01', kind: 'server', ip: '10.88.0.1', endpoint: '141.147.18.92:51820', status: 'online', handshake_ago: '12 秒前', month_rx: 142 * 1024 * 1024 * 1024, month_tx: 284 * 1024 * 1024 * 1024 },
    { id: 2, name: 'HongKong-Edge-02', kind: 'server', ip: '10.88.0.2', endpoint: '43.154.88.102:51820', status: 'online', handshake_ago: '8 秒前', month_rx: 48 * 1024 * 1024 * 1024, month_tx: 92 * 1024 * 1024 * 1024 },
    { id: 3, name: 'HomeLab-NAS-Intel', kind: 'server', ip: '10.88.0.3', endpoint: '114.88.21.50:41200', status: 'online', handshake_ago: '25 秒前', month_rx: 210 * 1024 * 1024 * 1024, month_tx: 180 * 1024 * 1024 * 1024 },
    { id: 4, name: 'Singapore-Arm-04', kind: 'server', ip: '10.88.0.4', endpoint: '129.226.70.18:51820', status: 'online', handshake_ago: '18 秒前', month_rx: 35 * 1024 * 1024 * 1024, month_tx: 76 * 1024 * 1024 * 1024 },
    { id: 5, name: 'Frankfurt-AMD-05', kind: 'server', ip: '10.88.0.5', endpoint: '168.119.50.21:51820', status: 'online', handshake_ago: '40 秒前', month_rx: 18 * 1024 * 1024 * 1024, month_tx: 32 * 1024 * 1024 * 1024 },
    { id: 6, name: 'Oregon-Storage-06', kind: 'server', ip: '10.88.0.6', endpoint: '—', status: 'offline', handshake_ago: '3 天前', month_rx: 4 * 1024 * 1024 * 1024, month_tx: 2 * 1024 * 1024 * 1024 },
  ];

  private wgDevices: WGPeerItem[] = [
    { id: 101, name: 'Admin-iPhone-16-Pro', kind: 'device', ip: '10.88.0.11', status: 'online', handshake_ago: '45 秒前', month_rx: 3.2 * 1024 * 1024 * 1024, month_tx: 1.1 * 1024 * 1024 * 1024 },
    { id: 102, name: 'MacBook-M3-Max', kind: 'device', ip: '10.88.0.12', status: 'online', handshake_ago: '2 分钟前', month_rx: 14.5 * 1024 * 1024 * 1024, month_tx: 6.8 * 1024 * 1024 * 1024 },
    { id: 103, name: 'iPad-Pro-M4', kind: 'device', ip: '10.88.0.13', status: 'offline', handshake_ago: '1 天前', month_rx: 0.8 * 1024 * 1024 * 1024, month_tx: 0.2 * 1024 * 1024 * 1024 },
  ];

  public getWGOverview(): WGOverviewData {
    return {
      network: { ...this.wgNetwork },
      hubs: [...this.wgHubs],
      peers: [...this.wgPeers],
      devices: [...this.wgDevices],
    };
  }

  public switchWGHub(hubId: number): boolean {
    this.wgHubs.forEach(h => {
      h.is_active = (h.id === hubId);
    });
    this.wgNetwork.active_hub_id = hubId;
    return true;
  }

  public addWGDevice(name: string): WGPeerItem {
    const newId = 100 + this.wgDevices.length + 1;
    const newIp = `10.88.0.${10 + this.wgDevices.length + 1}`;
    const device: WGPeerItem = {
      id: newId,
      name,
      kind: 'device',
      ip: newIp,
      status: 'pending',
      handshake_ago: '刚刚生成',
      month_rx: 0,
      month_tx: 0,
    };
    this.wgDevices.push(device);
    return device;
  }

  public deleteWGDevice(id: number): boolean {
    const idx = this.wgDevices.findIndex(d => d.id === id);
    if (idx !== -1) {
      this.wgDevices.splice(idx, 1);
      return true;
    }
    return false;
  }

  public getWGDeviceConf(id: number): string {
    const d = this.wgDevices.find(item => item.id === id);
    const ip = d?.ip || '10.88.0.15';
    return `[Interface]
PrivateKey = yAn7e8+MockGeneratedPrivateKeyExample9817=
Address = ${ip}/32
DNS = 1.1.1.1, 8.8.8.8

[Peer]
PublicKey = fG4h7JkL8mNpQrStUvWxYz0123456789=
Endpoint = 141.147.18.92:51820
AllowedIPs = 10.88.0.0/24
PersistentKeepalive = 25`;
  }

  // ================= FRP 内网穿透数据与操作 =================
  private frpPlatforms: FRPPlatformItem[] = [
    { id: 1, name: 'Sakura FRP (樱花穿透)', kind: 'sakura', token_bound: true, online_tunnels: 2, total_tunnels: 2, today_traffic: 14.2 * 1024 * 1024 * 1024, user_info: { username: 'BeaconOps', traffic_quota: '48.5 GB 剩余' } },
    { id: 2, name: 'ChmlFrp (极速穿透)', kind: 'chmlfrp', token_bound: true, online_tunnels: 2, total_tunnels: 2, today_traffic: 6.8 * 1024 * 1024 * 1024, user_info: { username: 'TowerCluster', traffic_quota: '不限量' } },
    { id: 3, name: 'Cloudflare Tunnel', kind: 'cloudflare', token_bound: true, online_tunnels: 1, total_tunnels: 1, today_traffic: 3.8 * 1024 * 1024 * 1024, user_info: { username: 'ZeroTrust Organization', traffic_quota: '全球 Anycast' } },
  ];

  private frpTunnels: FRPTunnelItem[] = [
    { id: 1, platform_id: 1, platform_name: 'Sakura FRP', platform_kind: 'sakura', name: 'sakura-ssh-proxy', proto: 'tcp', local_addr: '127.0.0.1:22', remote_addr: 'jp.natfrp.net:38291', running: true, today_traffic: 8.4 * 1024 * 1024 * 1024, status: 'online' },
    { id: 2, platform_id: 1, platform_name: 'Sakura FRP', platform_kind: 'sakura', name: 'sakura-web-ssl', proto: 'https', local_addr: '127.0.0.1:443', remote_addr: 'status.sakura.net:443', running: true, today_traffic: 5.8 * 1024 * 1024 * 1024, status: 'online' },
    { id: 3, platform_id: 2, platform_name: 'ChmlFrp', platform_kind: 'chmlfrp', name: 'chmlfrp-web-ingress', proto: 'http', local_addr: '127.0.0.1:8080', remote_addr: 'demo.chmlfrp.cn:80', running: true, today_traffic: 4.1 * 1024 * 1024 * 1024, status: 'online' },
    { id: 4, platform_id: 2, platform_name: 'ChmlFrp', platform_kind: 'chmlfrp', name: 'chmlfrp-minecraft', proto: 'tcp', local_addr: '127.0.0.1:25565', remote_addr: 'hk.chmlfrp.cn:25565', running: true, today_traffic: 2.7 * 1024 * 1024 * 1024, status: 'online' },
    { id: 5, platform_id: 3, platform_name: 'Cloudflare Tunnel', platform_kind: 'cloudflare', name: 'cf-beacontower-tunnel', proto: 'https', local_addr: '127.0.0.1:5174', remote_addr: 'tower.example.com', running: true, today_traffic: 3.8 * 1024 * 1024 * 1024, status: 'online' },
  ];

  public getFRPOverview() {
    return {
      platforms: [...this.frpPlatforms],
      tunnels: [...this.frpTunnels],
    };
  }

  public toggleFRPTunnel(id: number): boolean {
    const t = this.frpTunnels.find(item => item.id === id);
    if (t) {
      t.running = !t.running;
      t.status = t.running ? 'online' : 'stopped';
      return true;
    }
    return false;
  }

  public createFRPTunnel(payload: Partial<FRPTunnelItem>): FRPTunnelItem {
    const newId = Math.max(...this.frpTunnels.map(t => t.id), 0) + 1;
    const tunnel: FRPTunnelItem = {
      id: newId,
      platform_id: payload.platform_id || 1,
      platform_name: payload.platform_name || 'Sakura FRP',
      platform_kind: payload.platform_kind || 'sakura',
      name: payload.name || `tunnel-${newId}`,
      proto: payload.proto || 'tcp',
      local_addr: payload.local_addr || '127.0.0.1:8080',
      remote_addr: payload.remote_addr || 'proxy.example.com:8080',
      running: true,
      today_traffic: 0,
      status: 'online',
    };
    this.frpTunnels.push(tunnel);
    return tunnel;
  }

  public deleteFRPTunnel(id: number): boolean {
    const idx = this.frpTunnels.findIndex(t => t.id === id);
    if (idx !== -1) {
      this.frpTunnels.splice(idx, 1);
      return true;
    }
    return false;
  }

  public saveFRPToken(platformId: number, token: string): boolean {
    const p = this.frpPlatforms.find(item => item.id === platformId);
    if (p) {
      p.token_bound = true;
      return true;
    }
    return false;
  }

  // ================= 采集与展示设置 =================
  private settings: SystemSettingsData = {
    interval_s: 10,
    retention_days: 30,
    show_power_public: true,
    show_cost_public: true,
    electric_price: 0.65,
    site_title: '信标塔 · BeaconTower',
    open_7d_history: true,
    private_mode: false,
    strict_host_key: true,
  };

  public getSettings(): SystemSettingsData {
    return { ...this.settings };
  }

  public saveSettings(newSettings: SystemSettingsData): boolean {
    this.settings = { ...newSettings };
    return true;
  }

  // ================= 活动会话管理 =================
  private sessions: SessionItem[] = [
    { id: 'sess-current-01', device: 'Apple MacBook Pro (macOS 15.0)', browser: 'Chrome 130.0', ip: '127.0.0.1', login_at: Math.floor(Date.now() / 1000) - 3600, is_current: true },
    { id: 'sess-mobile-02', device: 'iPhone 16 Pro (iOS 18.1)', browser: 'Mobile Safari', ip: '114.88.21.50', login_at: Math.floor(Date.now() / 1000) - 86400 * 2, is_current: false },
  ];

  public getSessions(): SessionItem[] {
    return [...this.sessions];
  }

  public revokeSession(id: string): boolean {
    const idx = this.sessions.findIndex(s => s.id === id);
    if (idx !== -1) {
      this.sessions.splice(idx, 1);
      return true;
    }
    return false;
  }

  // ================= 审计日志 =================
  private auditLogs: AuditLogItem[] = [
    { id: 106, ts: Math.floor(Date.now() / 1000) - 60, actor: 'admin', action: 'server_test', target: 'Tokyo-Master-01', detail: 'SSH 连通性探测成功 (延迟 12ms)', source_ip_hash: 'a9f2...8e11' },
    { id: 105, ts: Math.floor(Date.now() / 1000) - 1800, actor: 'admin', action: 'login', target: 'Web Console', detail: '会话已验证并颁发 CSRF Token', source_ip_hash: 'a9f2...8e11' },
    { id: 104, ts: Math.floor(Date.now() / 1000) - 7200, actor: 'admin', action: 'wg_patrol', target: 'WireGuard Mesh', detail: '全网 6 节点互通巡检完成，状态良好', source_ip_hash: '系统计划任务' },
    { id: 103, ts: Math.floor(Date.now() / 1000) - 28000, actor: 'admin', action: 'settings', target: '采集与展示', detail: '更新基准电价为 0.65 元/kWh，开启严格 HostKey 校验', source_ip_hash: 'a9f2...8e11' },
    { id: 102, ts: Math.floor(Date.now() / 1000) - 86400, actor: 'admin', action: 'server_create', target: 'HongKong-Edge-02', detail: '纳管新节点成功，自动回读 Ubuntu 24.04 画像', source_ip_hash: 'a9f2...8e11' },
    { id: 101, ts: Math.floor(Date.now() / 1000) - 172800, actor: 'admin', action: 'power_calibrate', target: 'HomeLab-NAS-Intel', detail: '校准底噪整机功耗为 8.5W', source_ip_hash: 'a9f2...8e11' },
    { id: 100, ts: Math.floor(Date.now() / 1000) - 259200, actor: 'admin', action: 'setup', target: 'BeaconTower 集群', detail: '系统完成初始化设置，生成主密钥', source_ip_hash: '127.0.0.1' },
  ];

  public getAuditLogs(query = '', page = 1, pageSize = 10) {
    let list = [...this.auditLogs];
    if (query.trim()) {
      const q = query.toLowerCase().trim();
      list = list.filter(item => 
        item.action.toLowerCase().includes(q) ||
        item.target.toLowerCase().includes(q) ||
        item.detail.toLowerCase().includes(q) ||
        item.actor.toLowerCase().includes(q)
      );
    }
    const total = list.length;
    const pages = Math.ceil(total / pageSize) || 1;
    const offset = (page - 1) * pageSize;
    const sliced = list.slice(offset, offset + pageSize);
    return { list: sliced, total, page, pages };
  }
}

export const mockEngine = new MockDataEngine();

