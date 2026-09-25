// ============================================================
// BeaconTower · 管理端 API 门面（唯一出口）
// 当前实现：浏览器本地模拟（委托 ../admin/store.js，语义对齐 doc/04）。
// 后端就绪后：本文件内把每个方法体换成 http.* 调用即可，
// 上层 stores / views 零改动。禁止视图层绕过本文件直调 mock。
//
// 接口隐藏说明：本文件仅被管理端路由（懒加载 chunk）引用；公开页
// 首屏 bundle 只包含 api/auth.js + api/http.js，节点/凭据管理代码
// 不进入公开首屏。MOCK_MODE=false 后此处同样只走 /api/v1 管理路径。
// ============================================================
import { adminApi as mockApi } from '../admin/store'
import { authClient, passwordStrength } from './auth'
import { http } from './http'

export { passwordStrength }
export { fmtTs, agoFromTs } from '../admin/store'

// TODO(M1)：后端就绪后将 MOCK_MODE 置 false 并补齐 http 实现。
const MOCK_MODE = true

function withTimeout(promise, ms = 15000) {
  let timer = 0
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error('请求超时，请重试')), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

// 后端语义（doc/04 §3-§4）：管理节点/设置/审计全部走 /api/v1/admin/*。
const realAdmin = {
  status: () => http.get('/v1/admin/status'),
  setup: (payload) => http.post('/v1/admin/setup', payload),
  login: (payload) => http.post('/v1/admin/login', payload),
  logout: () => http.post('/v1/admin/logout', {}),
  changePassword: (payload) => http.post('/v1/admin/password', payload),

  listServers: () => http.get('/v1/admin/servers'),
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

export const adminClient = MOCK_MODE
  ? {
      // 鉴权走轻量 authClient：公开 bundle 与管理 bundle 共用同一套语义。
      status: authClient.status,
      setup: authClient.setup,
      login: authClient.login,
      logout: authClient.logout,
      changePassword: authClient.changePassword,

      listServers: (signal) => withTimeout(mockApi.listServers(), 15000),
      testConnection: (ssh) => withTimeout(mockApi.testConnection(ssh), 30000),
      createServer: (payload) => withTimeout(mockApi.createServer(payload)),
      updateServer: (id, payload) => withTimeout(mockApi.updateServer(id, payload)),
      deleteServer: (id) => withTimeout(mockApi.deleteServer(id)),
      reorder: (ids) => withTimeout(mockApi.reorder(ids)),
      recalibratePower: (id, w) => withTimeout(mockApi.recalibratePower(id, w)),
      relocate: (id) => withTimeout(mockApi.relocate(id)),

      getSettings: () => withTimeout(mockApi.getSettings()),
      saveSettings: (next) => withTimeout(mockApi.saveSettings(next)),

      listAudit: (page, size) => withTimeout(mockApi.listAudit(page, size)),
      resetAll: () => withTimeout(mockApi.resetAll()),
    }
  : realAdmin

export { MOCK_MODE }
