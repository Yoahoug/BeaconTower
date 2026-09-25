// ============================================================
// BeaconTower · 认证 API 门面（公开首屏唯一鉴权出口）
// - 轻量：只处理 status / setup / login / logout / changePassword，
//   不引用任何节点/凭据/种子数据，保证公开 bundle 不含管理代码。
// - 后端语义对齐 doc/04 §2：响应包 {code,msg,data} 由 http.js 统一解包；
//   CSRF 头由 http.js 对非 GET 自动附加。
// ============================================================
import { http } from './http'

// ---------- 密码强度（对齐 05 文档：≥10 位，大小写+数字+符号） ----------
export function passwordStrength(pw) {
  const p = typeof pw === 'string' ? pw : ''
  const checks = {
    length: p.length >= 10,
    lower: /[a-z]/.test(p),
    upper: /[A-Z]/.test(p),
    digit: /\d/.test(p),
    symbol: /[^A-Za-z0-9]/.test(p),
  }
  checks.ok = Object.values(checks).every(Boolean)
  return checks
}

function withTimeout(promise, ms = 15000) {
  let timer = 0
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error('请求超时，请重试')), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

const realAuth = {
  status: () => http.get('/v1/admin/status'),
  setup: (payload) => http.post('/v1/admin/setup', payload),
  login: (payload) => http.post('/v1/admin/login', payload),
  logout: () => http.post('/v1/admin/logout', {}),
  changePassword: (payload) => http.post('/v1/admin/password', payload),
}

const impl = realAuth

// 路由守卫共用：status 30 秒缓存，避免每次跳转都打一次接口。
// 约束 doc/10 §6.3。退出登录/初始化/登录成功后调用 resetAuthCache()。
let statusCache = null
let statusAt = 0

export async function getCachedStatus() {
  if (statusCache && Date.now() - statusAt < 30_000) return statusCache
  statusCache = await authClient.status()
  statusAt = Date.now()
  return statusCache
}

export function resetAuthCache() {
  statusCache = null
  statusAt = 0
}

export const authClient = {
  status: () => withTimeout(impl.status()),
  setup: (payload) => withTimeout(impl.setup(payload)),
  login: (payload) => withTimeout(impl.login(payload)),
  logout: () => withTimeout(impl.logout()),
  changePassword: (payload) => withTimeout(impl.changePassword(payload)),
}

// ---------- 时间格式化（审计用；后端返回 unix 秒） ----------
export function fmtTs(ts) {
  if (!ts) return '—'
  const d = new Date(ts * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function agoFromTs(ts) {
  if (!ts) return '从未成功'
  const diff = Math.floor(Date.now() / 1000) - ts
  if (diff < 60) return `${diff} 秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  return `${Math.floor(diff / 86400)} 天前`
}
