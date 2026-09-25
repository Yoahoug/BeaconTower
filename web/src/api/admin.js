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
}
