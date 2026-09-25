// ============================================================
// BeaconTower · 认证 API 门面（公开首屏唯一鉴权出口）
// - 轻量：只处理 status / setup / login / logout / changePassword，
//   不引用任何节点/凭据/种子数据，保证公开 bundle 不含管理代码。
// - 当前 MOCK_MODE=true：浏览器本地模拟（localStorage，与 admin/store.js
//   共用同一套 key，行为一致）；M1 后端就绪后置 false，走 http。
// - 后端语义对齐 doc/04 §2：响应包 {code,msg,data} 由 http.js 统一解包；
//   CSRF 头由 http.js 对非 GET 自动附加。
// ============================================================
import { http } from './http'

export const MOCK_MODE = true

const NS = 'beacontower.admin'

const read = (key, fallback) => {
  try {
    const raw = localStorage.getItem(`${NS}.${key}`)
    return raw ? JSON.parse(raw) : fallback
  } catch {
    return fallback
  }
}

const write = (key, value) => {
  localStorage.setItem(`${NS}.${key}`, JSON.stringify(value))
}

const delay = (ms = 260) => new Promise((r) => setTimeout(r, ms))
const uid = () => Date.now() * 1000 + Math.floor(Math.random() * 1000)

// 简单哈希（演示用；后端为 Argon2id）。与 admin/store.js 同算法，保持兼容。
function hashPw(s) {
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0
  return `demo$${h.toString(16)}$${s.length}`
}

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

const session = {
  get token() {
    return sessionStorage.getItem(`${NS}.session`)
  },
  set(v) {
    if (v) sessionStorage.setItem(`${NS}.session`, v)
    else sessionStorage.removeItem(`${NS}.session`)
  },
}

function getAdmin() {
  return read('admin', null)
}

function getSiteTitle() {
  try {
    const s = read('settings', null)
    if (s && typeof s.site_title === 'string' && s.site_title) return s.site_title
  } catch {
    /* 忽略 */
  }
  return 'BeaconTower · 信标塔'
}

async function logAudit(action, target, detail) {
  try {
    const list = read('audit', null) || []
    list.unshift({
      id: uid(),
      ts: Math.floor(Date.now() / 1000),
      actor: getAdmin()?.username || 'system',
      action,
      target,
      detail,
      source_ip_hash: 'sha256:' + Math.random().toString(16).slice(2, 10),
    })
    write('audit', list.slice(0, 500))
  } catch {
    /* 审计写失败不阻塞主流程 */
  }
}

function withTimeout(promise, ms = 15000) {
  let timer = 0
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error('请求超时，请重试')), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

const mockAuth = {
  async status() {
    const a = getAdmin()
    return {
      initialized: !!a,
      loggedIn: !!session.token && !!a,
      site_title: getSiteTitle(),
      username: a?.username || '',
    }
  },

  // 初始化（仅一次）
  async setup({ username, password }) {
    await delay(400)
    if (getAdmin()) {
      const e = new Error('已初始化，禁止重复初始化')
      e.code = 1004
      throw e
    }
    if (!username || username.length < 3) {
      const e = new Error('用户名至少 3 个字符')
      e.code = 1001
      throw e
    }
    if (!passwordStrength(password).ok) {
      const e = new Error('密码不满足强度要求：至少 10 位，含大小写字母、数字与符号')
      e.code = 1001
      throw e
    }
    write('admin', { username, password_hash: hashPw(password), created_at: Math.floor(Date.now() / 1000) })
    session.set('sess-' + Math.random().toString(36).slice(2))
    resetAuthCache()
    await logAudit('setup', 'admin', `初始化管理员 ${username}`)
    return { ok: true }
  },

  async login({ username, password }) {
    await delay(400)
    const a = getAdmin()
    if (!a || a.username !== username || a.password_hash !== hashPw(password)) {
      await logAudit('login_fail', 'admin', `登录失败（${username || '空用户名'}）`)
      const e = new Error('用户名或密码错误')
      e.code = 1005
      throw e
    }
    session.set('sess-' + Math.random().toString(36).slice(2))
    resetAuthCache()
    await logAudit('login', 'admin', '管理员登录成功')
    return { ok: true }
  },

  async logout() {
    await logAudit('logout', 'admin', '管理员登出')
    session.set(null)
    resetAuthCache()
    return { ok: true }
  },

  // 热更新密码：即时生效，不清当前会话（后端将吊销其他会话，见 M1）。
  async changePassword({ old_password, new_password }) {
    await delay(300)
    const a = getAdmin()
    if (!a || a.password_hash !== hashPw(old_password)) {
      const e = new Error('旧密码不正确')
      e.code = 1001
      throw e
    }
    if (!passwordStrength(new_password).ok) {
      const e = new Error('新密码不满足强度要求：至少 10 位，含大小写字母、数字与符号')
      e.code = 1001
      throw e
    }
    write('admin', { ...a, password_hash: hashPw(new_password) })
    await logAudit('password', 'admin', '修改管理员密码（热更新，即时生效）')
    return { ok: true }
  },
}

const realAuth = {
  status: () => http.get('/v1/admin/status'),
  setup: (payload) => http.post('/v1/admin/setup', payload),
  login: (payload) => http.post('/v1/admin/login', payload),
  logout: () => http.post('/v1/admin/logout', {}),
  changePassword: (payload) => http.post('/v1/admin/password', payload),
}

const impl = MOCK_MODE ? mockAuth : realAuth

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

// ---------- 时间格式化（审计用；与 ../admin/store.js 同实现，保持行为一致） ----------
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
