/**
 * BeaconTower 后端数据模型 1:1 强类型契约
 * 严格对应 Go internal/collector/snapshot.go 与 internal/store/models.go
 */

export interface PublicSummary {
  total: number;
  online: number;
  offline: number;
  up_bps: number;
  down_bps: number;
  watts: number;
  measured_count: number;
  month_kwh: number;
  est_cost_month: number;
  day_in_total: number;
  day_out_total: number;
}

export interface ServerProfile {
  os: string;            // 如 "Debian", "Ubuntu", "Arch Linux", "CentOS", "Alpine"
  os_version: string;    // 如 "12.5 (bookworm)", "22.04 LTS"
  arch: string;          // "x86_64" | "aarch64"
  cpu_cores: number;
  mem_total: number;     // 字节
  disk_total: number;    // 字节
  virt: string;          // "kvm" | "docker" | "physical" | "none"
}

export interface ServerMetrics {
  ts: number;
  cpu_pct: number;       // 0 ~ 100
  mem_used: number;      // 字节
  swap_used: number;     // 字节
  disk_used: number;     // 字节
  net_in_bps: number;    // 当前入网字节率 (bytes/s)
  net_out_bps: number;   // 当前出网字节率 (bytes/s)
  tcp_conns: number;
  udp_conns: number;
  load1: number;
  load5: number;
  load15: number;
  uptime_s: number;
  processes: number;
}

export interface ServerPower {
  total_w: number;       // 整机实时功耗 W
  cpu_w: number | null;
  dram_w: number | null;
  temp_c: number | null; // CPU 温度 °C
  freq_mhz: number | null;
  power_source: 'ac' | 'battery';
  today_kwh: number | null;
  month_kwh: number;
  est_cost_month?: number;
}

export interface ServerTrafficToday {
  in_total: number;      // 字节
  out_total: number;     // 字节
}

export interface ServerItem {
  id: number;
  name: string;
  region: string;        // 如 "JP 东京", "HK 香港", "CN 上海", "SG 新加坡", "DE 法兰克福", "US 硅谷"
  region_source: string;
  tags: string[];        // 如 ["核心", "WG-Hub", "FRP-Host"]
  note_public: string;
  status: 'online' | 'offline';
  is_self: boolean;
  profile: ServerProfile;
  metrics: ServerMetrics;
  power: ServerPower | null;
  traffic_today?: ServerTrafficToday;
}

export interface WGHubView {
  name: string;
  is_active: boolean;
  healthy: boolean;
  month_rx: number;
  month_tx: number;
}

export interface WGPeerView {
  name: string;
  online: boolean;
}

export interface WGPublicSnapshot {
  initialized: boolean;
  hubViews?: WGHubView[];
  peerViews?: WGPeerView[];
  trafficToday?: number;
  trafficMonth?: number;
  deviceTotal?: number;
  onlineCount?: number;
}

export interface FRPPlatformView {
  id: number;
  name: string;
  type: string;          // "sakura" | "chmlfrp" | "cloudflare"
  onlineTunnels: number;
  totalTunnels: number;
  todayTraffic: number;
}

export interface FRPPublicSnapshot {
  platforms: FRPPlatformView[];
  sumTunnel: number;
  sumOnline: number;
  sumToday: number;
}

// ---------- 管理端类型拓展 ----------
export interface WGHubItem {
  id: number;
  server_id: number;
  name: string;
  endpoint: string;
  listen_port: number;
  is_active: boolean;
  status: 'online' | 'offline' | 'pending';
  month_rx: number;
  month_tx: number;
  quota_limit_gb?: number;
}

export interface WGPeerItem {
  id: number;
  name: string;
  kind: 'server' | 'device';
  ip: string;
  endpoint?: string;
  status: 'online' | 'offline' | 'pending';
  handshake_ago?: string;
  month_tx: number;
  month_rx: number;
}

export interface WGOverviewData {
  network: {
    cidr: string;
    keepalive_s: number;
    active_hub_id: number;
  };
  hubs: WGHubItem[];
  peers: WGPeerItem[];
  devices: WGPeerItem[];
}

export interface FRPTunnelItem {
  id: number;
  platform_id: number;
  platform_name: string;
  platform_kind: 'sakura' | 'chmlfrp' | 'cloudflare';
  name: string;
  proto: 'tcp' | 'udp' | 'http' | 'https';
  local_addr: string;
  remote_addr: string;
  running: boolean;
  today_traffic: number;
  status: 'online' | 'offline' | 'stopped';
}

export interface FRPPlatformItem {
  id: number;
  name: string;
  kind: 'sakura' | 'chmlfrp' | 'cloudflare';
  token_bound: boolean;
  online_tunnels: number;
  total_tunnels: number;
  today_traffic: number;
  user_info?: {
    username: string;
    traffic_quota?: string;
  };
}

export interface SystemSettingsData {
  interval_s: number;
  retention_days: number;
  show_power_public: boolean;
  show_cost_public: boolean;
  electric_price: number;
  site_title: string;
  open_7d_history: boolean;
  private_mode: boolean;
  strict_host_key: boolean;
}

export interface AuditLogItem {
  id: number;
  ts: number;
  actor: string;
  action: string;
  target: string;
  detail: string;
  source_ip_hash: string;
}

export interface SessionItem {
  id: string;
  device: string;
  browser: string;
  ip: string;
  login_at: number;
  is_current: boolean;
}
