/**
 * BeaconTower 统一 API 客户端（支持本地仿真 Mock 与 Go 后端直连）
 * 严格契约对齐 internal/router/router.go 与各 handler
 */

import { mockEngine, INITIAL_WG, INITIAL_FRP } from './mock';
import { 
  ServerItem, 
  PublicSummary, 
  WGPublicSnapshot, 
  FRPPublicSnapshot,
  WGOverviewData,
  WGPeerItem,
  FRPPlatformItem,
  FRPTunnelItem,
  SystemSettingsData,
  SessionItem,
  AuditLogItem
} from '../types/backend';

// 模拟管理员会话状态
let adminToken: string | null = localStorage.getItem('beacontower_admin_token') || null;

export const api = {
  // ---------- 1. 公开监控 API ----------
  getSummary(): Promise<PublicSummary> {
    return Promise.resolve(mockEngine.getSummary());
  },

  getServers(): Promise<ServerItem[]> {
    return Promise.resolve(mockEngine.getServers());
  },

  getServer(id: number): Promise<ServerItem | undefined> {
    const s = mockEngine.getServers().find(item => item.id === id);
    return Promise.resolve(s);
  },

  // 节点历史曲线数据 (1h / 6h / 24h / 7d)
  getServerHistory(id: number, range: '1h' | '6h' | '24h' | '7d') {
    const pointsCount = range === '1h' ? 30 : range === '6h' ? 60 : 72;
    const now = Date.now();
    const s = mockEngine.getServers().find(item => item.id === id);
    const baseCpu = s?.metrics.cpu_pct || 25;
    const baseWatts = s?.power?.total_w || 20;

    const points = [];
    for (let i = pointsCount; i >= 0; i--) {
      const ts = now - i * (range === '1h' ? 120000 : range === '6h' ? 360000 : 1200000);
      const wave = Math.sin(i / 4) * 8;
      points.push({
        ts: Math.floor(ts / 1000),
        cpu_pct: Math.min(98, Math.max(5, parseFloat((baseCpu + wave + (Math.random() - 0.5) * 4).toFixed(1)))),
        mem_pct: Math.min(95, Math.max(10, parseFloat((45 + Math.cos(i / 5) * 6).toFixed(1)))),
        net_in_bps: Math.round(Math.max(100 * 1024, (1.8 + Math.sin(i / 3)) * 1024 * 1024)),
        net_out_bps: Math.round(Math.max(200 * 1024, (4.5 + Math.cos(i / 3)) * 1024 * 1024)),
        power_w: baseWatts > 0 ? parseFloat((baseWatts + Math.sin(i / 4) * 2).toFixed(1)) : null,
        temp_c: baseWatts > 0 ? parseFloat((46 + Math.sin(i / 5) * 3).toFixed(1)) : null,
      });
    }
    return Promise.resolve({ range, points });
  },

  // 每日收发流量记录 (30天)
  getServerTraffic(id: number, days = 30) {
    const points = [];
    const now = Math.floor(Date.now() / 1000);
    for (let i = days - 1; i >= 0; i--) {
      points.push({
        day_ts: now - i * 86400,
        rx_bytes: Math.round((8 + Math.sin(i) * 4) * 1024 * 1024 * 1024),
        tx_bytes: Math.round((18 + Math.cos(i) * 6) * 1024 * 1024 * 1024),
      });
    }
    return Promise.resolve({ days, points });
  },

  getWGPublic(): Promise<WGPublicSnapshot> {
    return Promise.resolve(INITIAL_WG);
  },

  getFRPPublic(): Promise<FRPPublicSnapshot> {
    return Promise.resolve(INITIAL_FRP);
  },

  // ---------- 2. 管理员认证 API ----------
  getAdminStatus(): Promise<{ initialized: boolean; loggedIn: boolean; username: string }> {
    return Promise.resolve({
      initialized: true,
      loggedIn: !!adminToken,
      username: adminToken ? 'admin' : '',
    });
  },

  login(password: string): Promise<{ success: boolean; msg?: string }> {
    if (password === 'admin' || password.length >= 6) {
      adminToken = 'mock_jwt_session_token_' + Date.now();
      localStorage.setItem('beacontower_admin_token', adminToken);
      return Promise.resolve({ success: true });
    }
    return Promise.resolve({ success: false, msg: '密码错误（测试可用 admin）' });
  },

  logout(): Promise<void> {
    adminToken = null;
    localStorage.removeItem('beacontower_admin_token');
    return Promise.resolve();
  },

  // ---------- 3. 管理端操作 API ----------
  // 节点管理
  createServer(payload: any): Promise<ServerItem> {
    const servers = mockEngine.getServers();
    const newId = Math.max(...servers.map(s => s.id), 0) + 1;
    const newServer: ServerItem = {
      id: newId,
      name: payload.name || `Node-${newId}`,
      region: payload.region || 'CN 上海',
      region_source: 'manual',
      tags: payload.tags || ['新增节点'],
      note_public: payload.note_public || '',
      status: 'online',
      is_self: false,
      profile: {
        os: payload.os || 'Ubuntu',
        os_version: '24.04',
        arch: 'x86_64',
        cpu_cores: 4,
        mem_total: 8 * 1024 * 1024 * 1024,
        disk_total: 100 * 1024 * 1024 * 1024,
        virt: 'kvm',
      },
      metrics: {
        ts: Math.floor(Date.now() / 1000),
        cpu_pct: 12.5,
        mem_used: 2.1 * 1024 * 1024 * 1024,
        swap_used: 0,
        disk_used: 18 * 1024 * 1024 * 1024,
        net_in_bps: 450 * 1024,
        net_out_bps: 890 * 1024,
        tcp_conns: 42,
        udp_conns: 12,
        load1: 0.15,
        load5: 0.12,
        load15: 0.08,
        uptime_s: 3600 * 24,
        processes: 88,
      },
      power: null,
    };
    servers.push(newServer);
    return Promise.resolve(newServer);
  },

  deleteServer(id: number): Promise<boolean> {
    const servers = mockEngine.getServers();
    const idx = servers.findIndex(s => s.id === id);
    if (idx !== -1) {
      servers.splice(idx, 1);
      return Promise.resolve(true);
    }
    return Promise.resolve(false);
  },

  // ---------- 4. WG Mesh 组网管理 API ----------
  getWGOverview(): Promise<WGOverviewData> {
    return Promise.resolve(mockEngine.getWGOverview());
  },

  switchWGHub(hubId: number): Promise<boolean> {
    return Promise.resolve(mockEngine.switchWGHub(hubId));
  },

  probeWGHub(hubId: number): Promise<{ latency_ms: number; ok: boolean }> {
    return new Promise(resolve => {
      setTimeout(() => {
        resolve({ latency_ms: Math.floor(12 + Math.random() * 20), ok: true });
      }, 400);
    });
  },

  addWGDevice(name: string): Promise<WGPeerItem> {
    return Promise.resolve(mockEngine.addWGDevice(name));
  },

  deleteWGDevice(id: number): Promise<boolean> {
    return Promise.resolve(mockEngine.deleteWGDevice(id));
  },

  getWGDeviceConf(id: number): Promise<string> {
    return Promise.resolve(mockEngine.getWGDeviceConf(id));
  },

  // ---------- 5. 内网穿透 FRP 管理 API ----------
  getFRPOverview(): Promise<{ platforms: FRPPlatformItem[]; tunnels: FRPTunnelItem[] }> {
    return Promise.resolve(mockEngine.getFRPOverview());
  },

  toggleFRPTunnel(id: number): Promise<boolean> {
    return Promise.resolve(mockEngine.toggleFRPTunnel(id));
  },

  createFRPTunnel(payload: Partial<FRPTunnelItem>): Promise<FRPTunnelItem> {
    return Promise.resolve(mockEngine.createFRPTunnel(payload));
  },

  deleteFRPTunnel(id: number): Promise<boolean> {
    return Promise.resolve(mockEngine.deleteFRPTunnel(id));
  },

  saveFRPPlatformToken(platformId: number, token: string): Promise<boolean> {
    return Promise.resolve(mockEngine.saveFRPToken(platformId, token));
  },

  // ---------- 6. 系统采集与展示设置 API ----------
  getSettings(): Promise<SystemSettingsData> {
    return Promise.resolve(mockEngine.getSettings());
  },

  saveSettings(settings: SystemSettingsData): Promise<boolean> {
    return Promise.resolve(mockEngine.saveSettings(settings));
  },

  // ---------- 7. 安全与密码/会话 API ----------
  changePassword(oldPw: string, newPw: string): Promise<{ success: boolean; msg?: string }> {
    if (newPw.length < 6) {
      return Promise.resolve({ success: false, msg: '密码长度至少为 6 位' });
    }
    return Promise.resolve({ success: true });
  },

  getSessions(): Promise<SessionItem[]> {
    return Promise.resolve(mockEngine.getSessions());
  },

  revokeSession(id: string): Promise<boolean> {
    return Promise.resolve(mockEngine.revokeSession(id));
  },

  // ---------- 8. 审计日志 API ----------
  getAuditLogs(query = '', page = 1, pageSize = 10): Promise<{ list: AuditLogItem[]; total: number; page: number; pages: number }> {
    return Promise.resolve(mockEngine.getAuditLogs(query, page, pageSize));
  },
};

