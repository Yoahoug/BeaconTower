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
  // 采集侧实际统计的网卡名（物理口逗号分隔列表；空＝未判定）
  net_ifaces?: string;
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
  // 与 admin 端同口径：natfrp（樱花）/ chmlfrp / cloudflared
  kind: 'natfrp' | 'chmlfrp' | 'cloudflared' | string;
  online: boolean;
  tunnel_total: number;
  tunnel_online: number;
  conns: number;
  conns_src: string;
  traffic_today: number;
  nodes: { name: string; group: string; online: boolean; load: number; uptime: number }[];
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

/** 中心槽位视图：overview.hubs[] 与中心面板 hub 同一形状，差别只在面板多几个只读字段 */
export type WGHubView = WGOverviewData['hubs'][number];
export type WGPeerView = WGOverviewData['peers'][number];

/** GET /admin/wg/hubs/:id —— 中心节点面板（handler/wg_hub.go:134） */
export interface WGHubDetailData {
  hub: WGHubView & {
    public_key: string;
    key_fp: string;
    checked_at: number | null;
    can_render: boolean;
  };
  /** 使用端（设备）成员：手机/笔记本扫码即用 */
  creds: WGPeerView[];
  /** SSH 服务器成员：配置由面板下发 */
  members: WGPeerView[];
  /** 主备就绪度：现役 + 所有未退役备援能否一键换中心 */
  hubs_readiness: {
    server_id: number;
    name: string;
    is_active: boolean;
    status: string;
    ready: boolean;
    missing: string;
    endpoint: string;
    listen_port: number;
  }[];
  network: { subnet: string; hub_ip: string; iface: string; active_hub_server_id: number };
}

/** POST /admin/wg/peers/:id/sync-hubs —— 把成员公钥整体下发给每台中心 */
export interface WGSyncHubsResp {
  synced: number;
  total: number;
  results: { ok: boolean; name?: string; server_id?: number; error?: string }[];
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
// kind 取值来自 internal/frp：natfrp（樱花）/ chmlfrp / cloudflared。
// 注意不是 'sakura'/'cloudflare'——平台卡与页签都按后端口径分流。

export interface FRPPlatformItem {
  id: number;
  kind: 'natfrp' | 'chmlfrp' | 'cloudflared' | string;
  name: string;
  username: string;
  uid: string;
  group_name: string;
  speed_limit: string;
  realname: string;
  tunnel_used: number;
  /** >0 配额；<0 不限；0 平台未报 */
  tunnel_quota: number;
  tunnel_total: number;
  tunnel_online: number;
  conns: number;
  /** platform=平台侧连接数；local=面板节点侧实测 */
  conns_src: string;
  traffic_day_used: number;
  traffic_remain: number;
  traffic_up: number;
  traffic_down: number;
  status: string;
  last_error: string;
  last_sync_at: number;
  has_token: boolean;
  /** frpPlatformView 解 JSON 后下发：任意键值（natfrp 有 sign_signed/sign_days） */
  profile: Record<string, unknown>;
  /** ChmlFrp access_token 到期时刻（10 分钟寿命） */
  token_expire_at: number;
}

export interface FRPTunnelItem {
  id: number;
  platform_id: number;
  platform_name: string;
  platform_kind: 'natfrp' | 'chmlfrp' | 'cloudflared' | string;
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
  /** 秒（store.FRPTunnel.Uptime int64） */
  uptime: number;
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
  /** store.FRPNode.Caps []string → JSON 数组 */
  caps: string[];
  online: boolean;
  load: number;
  /** 秒 */
  uptime: number;
  description: string;
  in_use: boolean;
  synced_at: number;
}

/** GET /admin/frp/platforms/:id → FRPPlatformDetail（账号 + 趋势 + 该平台隧道与节点） */
export interface FRPUsagePoint {
  ts: number;
  traffic_day_used: number;
  traffic_remain: number;
  traffic_up: number;
  traffic_down: number;
  conns: number;
  tunnel_online: number;
  tunnel_total: number;
}

export interface FRPPlatformDetailData {
  platform: FRPPlatformItem;
  tunnels: FRPTunnelItem[];
  nodes: FRPNodeItem[];
  usage: FRPUsagePoint[];
}

/** GET /admin/frp/platforms/:id/flow → {points:[{label,used,remain}]} */
export interface FRPFlowPoint {
  label: string;
  used: number;
  remain?: number;
}

export type FRPTunnelInput = {
  name: string;
  proto: string;
  node_id?: string;
  local_ip?: string;
  local_port?: number;
  remote_port?: number;
  domain?: string;
  note?: string;
  extra?: string;
  encryption?: boolean;
  compression?: boolean;
};

/** /admin/frp/deployments 视图（frpDeployView） */
export interface FRPDeployTunnelRef {
  id: number;
  name: string;
  proto?: string;
  local_ip?: string;
  local_port?: number;
  remote?: string;
  missing?: boolean;
}

export interface FRPDeployItem {
  id: number;
  platform_id: number;
  platform_kind: string;
  platform_name: string;
  server_id: number;
  server_name: string;
  tunnels: FRPDeployTunnelRef[];
  tunnel_ids: number[];
  image: string;
  container: string;
  config_path: string;
  /** running | stopped | missing | error | ... */
  status: string;
  last_error: string;
  docker_ver: string;
  /** 隧道增删改后置位：需「同步」重建容器 */
  dirty: boolean;
  last_sync_at: number;
  created_at: number;
  updated_at: number;
}

/** POST /admin/frp/servers/:id/docker → {docker:{present,version,daemon_ok,err}, error?} */
export interface FRPDockerStatus {
  present: boolean;
  version: string;
  daemon_ok: boolean;
  err: string;
}

/** ChmlFrp 免费二级域名（仅 chmlfrp 平台支持） */
export interface FRPSubdomainItem {
  id: number;
  domain: string;
  record: string;
  type: string;
  target: string;
  ttl: string;
  remarks: string;
}

export interface FRPSubdomainInput {
  domain: string;
  record: string;
  type: string;
  target: string;
  ttl: string;
  remarks?: string;
}

/** POST /admin/frp/chmlfrp/device → 设备码授权会话 */
export interface FRPDeviceStartData {
  session_id: string;
  user_code: string;
  verify_url: string;
  expires_in: number;
  interval: number;
}

export interface FRPDevicePollData {
  status: string;
  user_code?: string;
  verify_url?: string;
  error?: string;
  platform_id?: number;
  username?: string;
  sync_error?: string;
}

// ---------- 管理端：WG 组网向导 / 导入 / 接管 / 资产中转（handler/wg.go） ----------

/** 预检/执行共用的入参（wgPlanInput） */
export interface WGPlanInput {
  network?: {
    subnet?: string;
    hub_ip?: string;
    iface?: string;
    keepalive?: number;
    mtu?: number;
  };
  hub_server_id: number;
  hub_port?: number;
  spokes: { server_id: number; wg_ip?: string }[];
}

export interface WGIssue {
  level: 'warn' | 'error';
  msg: string;
}

export interface WGPlanResp {
  network: { subnet: string; hub_ip: string; iface: string; keepalive: number; mtu: number };
  hub: { server_id: number; name: string; listen_port: number; role: string; issues: WGIssue[] };
  spokes: { server_id: number; name: string; wg_ip: string; allocated: boolean; issues: WGIssue[] }[];
  /** true = 存在 error 级问题，向导不应放行执行 */
  blocked: boolean;
}

/** 导入现有网络（wgImportInput） */
export interface WGImportInput {
  hub_server_id: number;
  standby_server_id?: number;
  candidates?: number[];
}

/** 新机接管现役中心（wgTakeoverInput） */
export interface WGTakeoverInput {
  target_server_id: number;
  port?: number;
  canary_server_id?: number;
}

export interface WGAssetItem {
  id: number;
  name: string;
  version: string;
  arch: string;
  sha256: string;
  sources: string[];
  /** 已缓存到 data/assets/（可直接推送） */
  cached: boolean;
  size: number;
  note: string;
  updated_at: number;
}

export interface WGAssetInput {
  name: string;
  version: string;
  arch: string;
  sha256?: string;
  sources: string[];
  note?: string;
}

export interface WGAssetProbeResult {
  url: string;
  status: number;
  latency_ms: number;
  err?: string;
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
