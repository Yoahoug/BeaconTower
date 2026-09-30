// ============================================================
// BeaconTower · 管理端 API 门面（唯一出口）
// 全部走后端 /api/v1/admin/*（http.js 统一出口：会话 Cookie +
// CSRF 双提交 + {code,msg,data} 解包）。视图层禁止绕过本文件。
//
// 接口隐藏说明：本文件仅被管理端路由（懒加载 chunk）引用；公开页
// 首屏 bundle 只包含 api/auth.js + api/http.js，节点/凭据管理代码
// 不进入公开首屏。
// ============================================================
import { passwordStrength } from './auth'
import { http } from './http'

export { passwordStrength }
export { fmtTs, agoFromTs } from './auth'

// 后端语义（doc/04 §3-§4）：管理节点/设置/审计全部走 /api/v1/admin/*。
const realAdmin = {
  status: () => http.get('/v1/admin/status'),
  setup: (payload) => http.post('/v1/admin/setup', payload),
  login: (payload) => http.post('/v1/admin/login', payload),
  logout: () => http.post('/v1/admin/logout', {}),
  changePassword: (payload) => http.post('/v1/admin/password', payload),

  listServers: (signal) => http.get('/v1/admin/servers', { signal }),
  testConnection: (ssh) => http.post('/v1/admin/servers/test', { ssh }, { timeoutMs: 30000, retries: 0 }),
  createServer: (payload) => http.post('/v1/admin/servers', payload),
  updateServer: (id, payload) => http.put(`/v1/admin/servers/${id}`, payload),
  deleteServer: (id) => http.del(`/v1/admin/servers/${id}`),
  reorder: (ids) => http.put('/v1/admin/servers/order', { ids }),
  recalibratePower: (id, w) => http.put(`/v1/admin/servers/${id}/power-calibration`, { base_load_w: w }),
  relocate: (id) => http.post(`/v1/admin/servers/${id}/locate`, {}),

  getSettings: () => http.get('/v1/admin/settings'),
  saveSettings: (next) => http.put('/v1/admin/settings', next),

  listAudit: (page, size) => http.get(`/v1/admin/audit?page=${page}&size=${size}`),

  // ---------- WG 组网（doc/12 §5） ----------
  wgOverview: () => http.get('/v1/admin/wg/overview'),
  wgPlan: (payload) => http.post('/v1/admin/wg/plan', payload, { timeoutMs: 60000, retries: 0 }),
  wgApply: (payload) => http.post('/v1/admin/wg/apply', payload, { timeoutMs: 30000, retries: 0 }),
  wgImport: (payload) => http.post('/v1/admin/wg/import', payload, { timeoutMs: 30000, retries: 0 }),
  // 登记备援：只读目标节点 conf 建槽位，不改对方配置（MeshView「未纳管 → 准备为备援」）
  wgRegisterStandby: (serverId) => http.post('/v1/admin/wg/hubs/standby', { server_id: serverId }, { timeoutMs: 60000, retries: 0 }),
  // 纳管已手工配好 WG 的节点（含本机）：只读它的 conf，不改动它
  wgAdoptServer: (serverId) => http.post('/v1/admin/wg/adopt', { server_id: serverId }, { timeoutMs: 90000, retries: 0 }),
  // 中心节点面板 + 凭证中心
  wgHubDetail: (id) => http.get(`/v1/admin/wg/hubs/${id}`, { retries: 0 }),
  wgHubProbe: (id) => http.post(`/v1/admin/wg/hubs/${id}/probe`, {}, { timeoutMs: 40000, retries: 0 }),
  wgPeerRename: (id, name) => http.put(`/v1/admin/wg/peers/${id}`, { name }),
  wgPeerSyncHubs: (id) => http.post(`/v1/admin/wg/peers/${id}/sync-hubs`, {}, { timeoutMs: 90000, retries: 0 }),
  wgTakeover: (payload) => http.post('/v1/admin/wg/takeover', payload, { timeoutMs: 30000, retries: 0 }),
  wgHubCleanup: (id) => http.del(`/v1/admin/wg/hubs/${id}`, { retries: 0 }),
  wgSwitchHub: (payload) => http.post('/v1/admin/wg/switch-hub', payload, { timeoutMs: 30000, retries: 0 }),
  wgPatrol: () => http.post('/v1/admin/wg/patrol', {}, { timeoutMs: 45000, retries: 0 }),
  wgTask: (id) => http.get(`/v1/admin/wg/tasks/${id}`, { retries: 0 }),
  wgDeviceCreate: (name) => http.post('/v1/admin/wg/devices', { name }),
  wgPeerConf: (id, hub = 0) => http.get(`/v1/admin/wg/peers/${id}/conf${hub ? `?hub=${hub}` : ''}`, { retries: 0 }),
  wgPeerVerify: (id) => http.post(`/v1/admin/wg/peers/${id}/verify`, {}, { timeoutMs: 60000, retries: 0 }),
  wgPeerDelete: (id) => http.del(`/v1/admin/wg/peers/${id}`, { timeoutMs: 60000, retries: 0 }),
  wgAssets: () => http.get('/v1/admin/wg/assets'),
  wgAssetUpsert: (payload) => http.post('/v1/admin/wg/assets', payload),
  wgAssetDelete: (id) => http.del(`/v1/admin/wg/assets/${id}`),
  wgAssetProbe: (id) => http.post(`/v1/admin/wg/assets/${id}/probe`, {}, { timeoutMs: 30000, retries: 0 }),
  wgAssetFetch: (id) => http.post(`/v1/admin/wg/assets/${id}/fetch`, {}, { timeoutMs: 600000, retries: 0 }),
  wgAssetPush: (id, serverId) => http.post(`/v1/admin/wg/assets/${id}/push`, { server_id: serverId }, { timeoutMs: 90000, retries: 0 }),

  // ---------- 内网穿透平台（doc/13 §5） ----------
  // 全部走外部平台 API，统一 retries:0 —— 重试会重复建隧道/重复删除
  frpOverview: () => http.get('/v1/admin/frp/overview'),
  frpNodes: () => http.get('/v1/admin/frp/nodes'),
  frpPlatform: (id) => http.get(`/v1/admin/frp/platforms/${id}`, { retries: 0 }),
  frpBindNatfrp: (payload) => http.post('/v1/admin/frp/platforms', payload, { timeoutMs: 60000, retries: 0 }),
  frpPlatformRename: (id, name) => http.put(`/v1/admin/frp/platforms/${id}`, { name }),
  frpPlatformDelete: (id) => http.del(`/v1/admin/frp/platforms/${id}`, { retries: 0 }),
  frpSync: (id, full = true) => http.post(`/v1/admin/frp/platforms/${id}/sync?full=${full ? 1 : 0}`, {}, { timeoutMs: 60000, retries: 0 }),
  frpFlow: (id, kind) => http.get(`/v1/admin/frp/platforms/${id}/flow?kind=${kind}`, { retries: 0 }),
  frpTunnelCreate: (id, payload) => http.post(`/v1/admin/frp/platforms/${id}/tunnels`, payload, { timeoutMs: 60000, retries: 0 }),
  frpSubdomains: (id) => http.get(`/v1/admin/frp/platforms/${id}/subdomains`, { retries: 0 }),
  frpAvailableDomains: (id) => http.get(`/v1/admin/frp/platforms/${id}/subdomains/available`, { retries: 0 }),
  frpSubdomainCreate: (id, payload) => http.post(`/v1/admin/frp/platforms/${id}/subdomains`, payload, { timeoutMs: 30000, retries: 0 }),
  frpSubdomainUpdate: (id, payload) => http.put(`/v1/admin/frp/platforms/${id}/subdomains`, payload, { timeoutMs: 30000, retries: 0 }),
  frpSubdomainDelete: (id, domain, record) =>
    http.del(`/v1/admin/frp/platforms/${id}/subdomains?domain=${encodeURIComponent(domain)}&record=${encodeURIComponent(record)}`, { timeoutMs: 30000, retries: 0 }),

  frpTunnelUpdate: (id, payload) => http.put(`/v1/admin/frp/tunnels/${id}`, payload, { timeoutMs: 30000, retries: 0 }),
  frpTunnelDelete: (id) => http.del(`/v1/admin/frp/tunnels/${id}`, { timeoutMs: 30000, retries: 0 }),
  frpTunnelLock: (id, payload) => http.post(`/v1/admin/frp/tunnels/${id}/lock`, payload, { timeoutMs: 30000, retries: 0 }),
  frpTunnelMigrate: (id, nodeId) => http.post(`/v1/admin/frp/tunnels/${id}/migrate`, { node_id: nodeId }, { timeoutMs: 30000, retries: 0 }),
  frpTunnelOffline: (id) => http.post(`/v1/admin/frp/tunnels/${id}/offline`, {}, { timeoutMs: 30000, retries: 0 }),
  frpTunnelAuth: (id, ip) => http.post(`/v1/admin/frp/tunnels/${id}/auth`, { ip }, { timeoutMs: 30000, retries: 0 }),
  frpTunnelTraffic: (id) => http.get(`/v1/admin/frp/tunnels/${id}/traffic`, { retries: 0 }),

  // ChmlFrp 走 OAuth2 设备码；令牌 10 分钟过期由后端自动续期，续不上时前端弹重新授权
  frpDeviceStart: (reuseId = 0) => http.post('/v1/admin/frp/chmlfrp/device', { reuse_id: reuseId }, { timeoutMs: 30000, retries: 0 }),
  frpDevicePoll: (sid) => http.get(`/v1/admin/frp/chmlfrp/device/${sid}`, { retries: 0 }),
  frpDeviceCancel: (sid) => http.del(`/v1/admin/frp/chmlfrp/device/${sid}`, { retries: 0 }),
}

function withTimeout(promise, ms = 15000) {
  let timer = 0
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error('请求超时，请重试')), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

export const adminClient = {
  status: realAdmin.status,
  setup: realAdmin.setup,
  login: realAdmin.login,
  logout: realAdmin.logout,
  changePassword: realAdmin.changePassword,

  listServers: (signal) => withTimeout(realAdmin.listServers(signal), 15000),
  testConnection: (ssh) => withTimeout(realAdmin.testConnection(ssh), 30000),
  createServer: (payload) => withTimeout(realAdmin.createServer(payload)),
  updateServer: (id, payload) => withTimeout(realAdmin.updateServer(id, payload)),
  deleteServer: (id) => withTimeout(realAdmin.deleteServer(id)),
  reorder: (ids) => withTimeout(realAdmin.reorder(ids)),
  recalibratePower: (id, w) => withTimeout(realAdmin.recalibratePower(id, w)),
  relocate: (id) => withTimeout(realAdmin.relocate(id)),

  getSettings: () => withTimeout(realAdmin.getSettings()),
  saveSettings: (next) => withTimeout(realAdmin.saveSettings(next)),

  listAudit: (page, size) => withTimeout(realAdmin.listAudit(page, size)),

  // WG 组网：plan/verify/delete 涉及 SSH 串行探测，给长超时；任务轮询不重试
  wgOverview: () => withTimeout(realAdmin.wgOverview(), 15000),
  wgPlan: (payload) => withTimeout(realAdmin.wgPlan(payload), 70000),
  wgApply: (payload) => withTimeout(realAdmin.wgApply(payload), 35000),
  wgImport: (payload) => withTimeout(realAdmin.wgImport(payload), 35000),
  wgRegisterStandby: (serverId) => withTimeout(realAdmin.wgRegisterStandby(serverId), 45000),
  wgAdoptServer: (serverId) => withTimeout(realAdmin.wgAdoptServer(serverId), 95000),
  wgHubDetail: (id) => withTimeout(realAdmin.wgHubDetail(id), 20000),
  wgHubProbe: (id) => withTimeout(realAdmin.wgHubProbe(id), 45000),
  wgPeerRename: (id, name) => realAdmin.wgPeerRename(id, name),
  wgPeerSyncHubs: (id) => withTimeout(realAdmin.wgPeerSyncHubs(id), 95000),
  wgTakeover: (payload) => withTimeout(realAdmin.wgTakeover(payload), 35000),
  wgHubCleanup: (id) => withTimeout(realAdmin.wgHubCleanup(id), 20000),
  wgSwitchHub: (payload) => withTimeout(realAdmin.wgSwitchHub(payload), 35000),
  wgPatrol: () => withTimeout(realAdmin.wgPatrol(), 50000),
  wgTask: (id) => realAdmin.wgTask(id),
  wgDeviceCreate: (name) => withTimeout(realAdmin.wgDeviceCreate(name), 20000),
  wgPeerConf: (id, hub) => withTimeout(realAdmin.wgPeerConf(id, hub), 20000),
  wgPeerVerify: (id) => withTimeout(realAdmin.wgPeerVerify(id), 70000),
  wgPeerDelete: (id) => withTimeout(realAdmin.wgPeerDelete(id), 70000),
  wgAssets: () => withTimeout(realAdmin.wgAssets(), 15000),
  wgAssetUpsert: (payload) => withTimeout(realAdmin.wgAssetUpsert(payload)),
  wgAssetDelete: (id) => withTimeout(realAdmin.wgAssetDelete(id)),
  wgAssetProbe: (id) => withTimeout(realAdmin.wgAssetProbe(id), 35000),
  wgAssetFetch: (id) => realAdmin.wgAssetFetch(id),
  wgAssetPush: (id, serverId) => withTimeout(realAdmin.wgAssetPush(id, serverId), 95000),

  // 穿透平台：外部 API 往返 + 后端可能的令牌续期，统一给足 60s
  frpOverview: () => withTimeout(realAdmin.frpOverview(), 20000),
  frpNodes: () => withTimeout(realAdmin.frpNodes(), 20000),
  frpPlatform: (id) => withTimeout(realAdmin.frpPlatform(id), 20000),
  frpBindNatfrp: (payload) => withTimeout(realAdmin.frpBindNatfrp(payload), 65000),
  frpPlatformRename: (id, name) => withTimeout(realAdmin.frpPlatformRename(id, name)),
  frpPlatformDelete: (id) => withTimeout(realAdmin.frpPlatformDelete(id), 20000),
  frpSync: (id, full) => withTimeout(realAdmin.frpSync(id, full), 65000),
  frpFlow: (id, kind) => withTimeout(realAdmin.frpFlow(id, kind), 45000),
  frpTunnelCreate: (id, payload) => withTimeout(realAdmin.frpTunnelCreate(id, payload), 65000),
  frpSubdomains: (id) => withTimeout(realAdmin.frpSubdomains(id), 30000),
  frpAvailableDomains: (id) => withTimeout(realAdmin.frpAvailableDomains(id), 30000),
  frpSubdomainCreate: (id, payload) => withTimeout(realAdmin.frpSubdomainCreate(id, payload), 35000),
  frpSubdomainUpdate: (id, payload) => withTimeout(realAdmin.frpSubdomainUpdate(id, payload), 35000),
  frpSubdomainDelete: (id, domain, record) => withTimeout(realAdmin.frpSubdomainDelete(id, domain, record), 35000),
  frpTunnelUpdate: (id, payload) => withTimeout(realAdmin.frpTunnelUpdate(id, payload), 35000),
  frpTunnelDelete: (id) => withTimeout(realAdmin.frpTunnelDelete(id), 35000),
  frpTunnelLock: (id, payload) => withTimeout(realAdmin.frpTunnelLock(id, payload), 35000),
  frpTunnelMigrate: (id, nodeId) => withTimeout(realAdmin.frpTunnelMigrate(id, nodeId), 35000),
  frpTunnelOffline: (id) => withTimeout(realAdmin.frpTunnelOffline(id), 35000),
  frpTunnelAuth: (id, ip) => withTimeout(realAdmin.frpTunnelAuth(id, ip), 35000),
  frpTunnelTraffic: (id) => withTimeout(realAdmin.frpTunnelTraffic(id), 45000),
  frpDeviceStart: (reuseId) => withTimeout(realAdmin.frpDeviceStart(reuseId), 35000),
  frpDevicePoll: (sid) => withTimeout(realAdmin.frpDevicePoll(sid), 20000),
  frpDeviceCancel: (sid) => withTimeout(realAdmin.frpDeviceCancel(sid), 20000),
}
