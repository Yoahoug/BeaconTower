/**
 * BeaconTower 后端数据模型 1:1 强类型契约
 * 逐字段对齐 Go internal/collector/snapshot.go、internal/store/models.go
 * 与 internal/handler/* 的 gin.H 响应（v2.8 真实联调校准）。
 */

// ---------- 公开监控（/api/v1/public/*） ----------

/** GET /public/summary —— handler/public.go:15-29 */
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
  os: string;
  os_version: string;
  arch: string;
  cpu_cores: number;
  mem_total: number;
  disk_total: number;
  virt: string;
  net_ifaces?: number;
}

export interface ServerMetrics {
  ts: number;
  cpu_pct: number;
  mem_used: number;
  swap_used: number;
  disk_used: number;
  net_in_bps: number;
  net_out_bps: number;
  tcp_conns: number;
  udp_conns: number;
  load1: number;
  load5: number;
  load15: number;
  uptime_s: number;
  processes: number;
}

export interface ServerPower {
  total_w: number;
  cpu_w: number | null;
  dram_w: number | null;
  temp_c: number | null;
  freq_mhz: number | null;
  power_source: 'ac' | 'battery';
  today_kwh: null;
  month_kwh: number;
  est_cost_month?: number;
}

export interface ServerItem {
  id: number;
  name: string;
  region: string;
  region_source: string;
  tags: string[];
  note_public: string;
  status: 'online' | 'offline';
  is_self: boolean;
  profile: ServerProfile;
  metrics: ServerMetrics;
  power: ServerPower | null;
  traffic_today?: { in_total: number; out_total: number };
}

/** GET /public/servers/:id/history —— 短期 points 字段；7d 为小时聚合（*_avg） */
export interface HistoryPoint {
  ts: number;
  cpu_pct?: number;
  cpu_avg?: number;
  cpu_max?: number;
  mem_used?: number;
  mem_avg?: number;
  disk_used?: number;
  net_in_bps?: number;
  net_out_bps?: number;
  net_in_avg?: number;
  net_out_avg?: number;
  power_w?: number;
  power_avg?: number;
}
export interface HistoryResp {
  range: '1h' | '6h' | '24h' | '7d';
  points: HistoryPoint[];
}

/** GET /public/servers/:id/traffic —— 每日收发（字段 ts/in_total/out_total） */
export interface TrafficPoint {
  ts: number;
  in_total: number;
  out_total: number;
}
export interface TrafficResp {
  days: number;
  points: TrafficPoint[];
}

/** GET /public/wg —— handler/wg_public.go（ hubs/peers + summary 子对象） */
export interface WGPublicSnapshot {
  initialized: boolean;
  hubs: { name: string; is_active: boolean; healthy: boolean; month_rx: number; month_tx: number }[];
  peers: { name: string; online: boolean }[];
  summary: {
    hub_active: number;
    hub_healthy: number;
    member_total: number;
    member_online: number;
    device_total: number;
    traffic_today: number;
    traffic_month: number;
    checked_at: number;
  };
}

/** GET /public/frp —— handler/frp.go FRPPublicSnapshot（platforms + summary 子对象） */
export interface FRPPlatformView {
  name: string;
  kind: 'sakura' | 'chmlfrp' | 'cloudflare' | string;
  online: boolean;
  tunnel_total: number;
  tunnel_online: number;
  conns: number;
  conns_src: string;
  traffic_today: number;
  nodes: { name: string; group: string; online: boolean; load: number; uptime: string }[];
  updated_at: number;
}
export interface FRPPublicSnapshot {
  platforms: FRPPlatformView[];
  summary: {
    platform_total: number;
    tunnel_total: number;
    tunnel_online: number;
    conns: number;
    traffic_today: number;
    node_in_use: number;
    node_online: number;
    updated_at: number;
  };
}

// ---------- 认证（/api/v1/admin/status|setup|login|logout|password） ----------

export interface AdminStatus {
  initialized: boolean;
  loggedIn: boolean;
  site_title: string;
  username: string;
}

// ---------- 管理端：WG 组网（/admin/wg/overview，handler/wg.go:169-275） ----------

export interface WGOverviewData {
  network: {
    subnet: string;
    hub_ip: string;
    iface: string;
    keepalive: number;
    mtu: number;
    active_hub_server_id: number;
  } | null;
  hubs: {
    server_id: number;
    name: string;
    listen_port: number;
    endpoint: string;
    status: string;
    last_error: string;
    quota_gb: number | null;
    month_rx: number;
    month_tx: number;
    month_billed: number;
    is_active: boolean;
    has_keys: boolean;
  }[];
  retired_hubs: { server_id: number; name: string }[];
  peers: {
    id: number;
    kind: 'server' | 'device' | string;
    server_id: number | null;
    name: string;
    wg_ip: string;
    public_key: string;
    managed: boolean;
    status: string;
    last_error: string;
    last_handshake: number | null;
    rx_bytes: number;
    tx_bytes: number;
    can_export: boolean;
  }[];
  servers: {
    id: number;
    name: string;
    is_self: boolean;
    in_network: boolean;
    ssh_ready: boolean;
    wg_role: string;
  }[];
  running_task: boolean;
}

/** POST /admin/wg/switch-hub → {task_id}（后台任务，进度走 /wg/tasks/:id） */
export interface WGTask {
  id: number;
  kind: string;
  status: string;
  result: string;
  created_at: number;
  finished_at: number | null;
  steps: { id: number; seq: number; server_id: number | null; title: string; status: string; log: string }[];
}

/** GET /admin/wg/hubs/:id/probe 响应字段（handler/wg_hub.go:240） */
export interface WGHubProbeResp {
  mode: 'handshake' | 'reach';
  ok: boolean;
  skipped: boolean;
  reason: string;
  peer: string;
  target: string;
  attempt: number;
  rtt_ms: number;
  hint: string;
  reach?: string;
}

// ---------- 管理端：内网穿透（/admin/frp/overview，handler/frp.go:65-128） ----------

export interface FRPPlatformItem {
  id: number;
  kind: 'sakura' | 'chmlfrp' | 'cloudflare' | string;
  name: string;
  username: string;
  uid: string;
  group_name: string;
  speed_limit: string;
  realname: string;
  tunnel_used: number;
  tunnel_quota: number;
  tunnel_total: number;
  tunnel_online: number;
  conns: number;
  conns_src: string;
  traffic_day_used: number;
  traffic_remain: number;
  traffic_up: number;
  traffic_down: number;
  status: string;
  last_error: string;
  last_sync_at: number;
  has_token: boolean;
  profile: string;
  token_expire_at: number;
}

export interface FRPTunnelItem {
  id: number;
  platform_id: number;
  platform_name: string;
  platform_kind: 'sakura' | 'chmlfrp' | 'cloudflare' | string;
  remote_id: string;
  name: string;
  proto: 'tcp' | 'udp' | 'http' | 'https' | string;
  node_id: string;
  node_name: string;
  local_ip: string;
  local_port: number;
  remote: string;
  online: boolean;
  status: string;
  status_reason: string;
  conns: number;
  local_conns: number;
  today_up: number;
  today_down: number;
  uptime: string;
  client_ver: string;
  extra: string;
  lock_edit: boolean;
  lock_delete: boolean;
  lock_migrate: boolean;
  synced_at: number;
}

export interface FRPOverviewData {
  platforms: FRPPlatformItem[];
  tunnels: FRPTunnelItem[];
  summary: {
    platform_count: number;
    tunnel_total: number;
    tunnel_online: number;
    conns: number;
  };
}

/** GET /admin/frp/nodes → {nodes:[...]}（frpNodeView） */
export interface FRPNodeItem {
  id: number;
  platform_id: number;
  platform_name: string;
  remote_id: string;
  name: string;
  host: string;
  area: string;
  group_name: string;
  caps: string;
  online: boolean;
  load: number;
  uptime: string;
  description: string;
  in_use: boolean;
  synced_at: number;
}

// ---------- 管理端：设置 / 审计 ----------

export interface SystemSettingsData {
  site_title: string;
  interval_s: number;
  retention_days: number;
  open_7d_history: boolean;
  show_power_public: boolean;
  show_cost_public: boolean;
  electric_price: number;
  strict_host_key: boolean;
  private_mode: boolean;
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
export interface AuditResp {
  total: number;
  items: AuditLogItem[];
}

// ---------- 管理端：节点管理（/admin/servers，handler/servers.go:23-113） ----------

export interface AdminServerSSH {
  local?: boolean;
  host: string;
  port: number;
  username: string;
  auth_type: string;
  has_password: boolean;
  has_key: boolean;
  host_key_fp: string;
  ssh_ready: boolean;
}

export interface AdminServerItem {
  id: number;
  name: string;
  region: string;
  region_source: string;
  tags: string[];
  note_public: string;
  note_private: string;
  sort_order: number;
  hidden: boolean;
  enabled: boolean;
  is_self: boolean;
  created_at: number;
  ssh: AdminServerSSH;
  profile: {
    hostname: string;
    os_name: string;
    os_version: string;
    kernel: string;
    arch: string;
    cpu_model: string;
    cpu_cores: number;
    mem_total: number;
    disk_total: number;
    virt: string;
    public_ip: string;
    geo_country: string;
    geo_city: string;
    power: { rapl: boolean; battery: boolean; base_load_w: number; base_load_source: string };
  };
  power: { rapl: boolean; battery: boolean; base_load_w: number; base_load_source: string; last_error: string };
  last_success_at?: number;
  ssh_suggest_host?: string;
}

/** POST /admin/servers/test（普通试连）响应 */
export interface TestConnResp {
  ok: boolean;
  latency_ms: number;
  bootstrap?: boolean;
  panel_private_key?: string;
  profile: {
    hostname: string;
    os_name: string;
    os_version: string;
    kernel: string;
    arch: string;
    cpu_model: string;
    cpu_cores: number;
    mem_total: number;
    disk_total: number;
    virt: string;
    power: { rapl: boolean; battery: boolean; base_load_w: number; base_load_source: string };
  };
  geo: { public_ip: string; country: string; city: string; region_text: string };
  host_key_fp: string;
}
