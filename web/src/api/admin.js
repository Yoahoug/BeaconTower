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
}
