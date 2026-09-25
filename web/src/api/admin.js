// ============================================================
// BeaconTower · 管理端 API 门面（唯一出口）
// 当前实现：浏览器本地模拟（委托 ./_adminMock.js，语义对齐 doc/04）。
// 后端就绪后：本文件内把每个方法体换成 http.* 调用即可，
// 上层 stores / views 零改动。禁止视图层绕过本文件直调 mock。
// ============================================================
import { adminApi as mockApi, passwordStrength, fmtTs, agoFromTs } from '../admin/store'

export { passwordStrength, fmtTs, agoFromTs }

// TODO(M1)：后端就绪后将 MOCK_MODE 置 false 并补齐 http 实现。
const MOCK_MODE = true

function withTimeout(promise, ms = 15000) {
  let timer = 0
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error('请求超时，请重试')), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

export const adminClient = {
  status: () => withTimeout(mockApi.status()),
  setup: (payload) => withTimeout(mockApi.setup(payload)),
  login: (payload) => withTimeout(mockApi.login(payload)),
  logout: () => withTimeout(mockApi.logout()),
  changePassword: (payload) => withTimeout(mockApi.changePassword(payload)),

  listServers: (signal) => withTimeout(mockApi.listServers(), 15000),
  testConnection: (ssh) => withTimeout(mockApi.testConnection(ssh), 30000),
  createServer: (payload) => withTimeout(mockApi.createServer(payload)),
  updateServer: (id, payload) => withTimeout(mockApi.updateServer(id, payload)),
  deleteServer: (id) => withTimeout(mockApi.deleteServer(id)),
  reorder: (ids) => withTimeout(mockApi.reorder(ids)),
  recalibratePower: (id, w) => withTimeout(mockApi.recalibratePower(id, w)),

  getSettings: () => withTimeout(mockApi.getSettings()),
  saveSettings: (next) => withTimeout(mockApi.saveSettings(next)),

  listAudit: (page, size) => withTimeout(mockApi.listAudit(page, size)),
  resetAll: () => withTimeout(mockApi.resetAll()),
}

export { MOCK_MODE }
