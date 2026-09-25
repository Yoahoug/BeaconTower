// 管理端模拟后端：对齐 doc/04-API接口设计 的管理 API 语义。
// 数据持久化在 localStorage（beacontower.admin.*），后端就绪后整体替换为
// 真 API 调用（fetch + Cookie 会话 + CSRF），视图层不变。
//
// 已实现语义：
// - 初始化向导（未初始化时可创建管理员，完成后永久关闭）
// - 登录/登出（会话 token 存 sessionStorage，刷新不丢）
// - 节点 CRUD + 试连（回读画像/geo/host key 指纹/功耗能力探测）
// - 凭据不回显原则：读取接口永不返回密码/私钥
// - 站点设置、审计日志（登录/节点变更/设置变更全记录）

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

// 简单哈希（演示用；后端为 Argon2id）
const hashPw = (s) => {
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0
  return `demo$${h.toString(16)}$${s.length}`
}

// ---------- 密码强度（对齐 05 文档：≥10 位，大小写+数字+符号） ----------
export function passwordStrength(pw) {
  const checks = {
    length: pw.length >= 10,
    lower: /[a-z]/.test(pw),
    upper: /[A-Z]/.test(pw),
    digit: /\d/.test(pw),
    symbol: /[^A-Za-z0-9]/.test(pw),
  }
  checks.ok = Object.values(checks).every(Boolean)
  return checks
}

// ---------- 画像/试连模拟（后端就绪后走 SSH 真实回读） ----------
function fakeProbe(ssh) {
  const seed = [...`${ssh.host}:${ssh.port}`].reduce((a, c) => a + c.charCodeAt(0), 0)
  const osPool = [
    { os_name: 'Ubuntu', os_version: '24.04' },
    { os_name: 'Debian', os_version: '12' },
    { os_name: 'AlmaLinux', os_version: '9.4' },
    { os_name: 'Rocky Linux', os_version: '9.3' },
  ]
  const cpuPool = [
    'AMD EPYC 9654 96-Core Processor',
    'Intel(R) Xeon(R) Platinum 8375C',
    'Intel(R) Core(TM) i5-6300HQ CPU @ 2.30GHz',
  ]
  const os = osPool[seed % osPool.length]
  const cores = [2, 4, 8, 16][seed % 4]
  const hasRapl = seed % 3 !== 1 // 约三分之一节点不暴露 RAPL（AMD/虚拟机场景）
  const geoPool = [
    { country: 'HK', city: 'Hong Kong', region: '香港' },
    { country: 'JP', city: 'Tokyo', region: '东京' },
    { country: 'SG', city: 'Singapore', region: '新加坡' },
    { country: 'US', city: 'Los Angeles', region: '洛杉矶' },
  ]
  const geo = geoPool[seed % geoPool.length]
  return {
    ok: true,
    latency_ms: 12 + (seed % 80),
    profile: {
      hostname: `node-${(seed % 89 + 10).toString(36)}`,
      os_name: os.os_name,
      os_version: os.os_version,
      kernel: `6.8.0-${40 + (seed % 20)}-generic`,
      arch: seed % 5 === 0 ? 'aarch64' : 'x86_64',
      cpu_model: cpuPool[seed % cpuPool.length],
      cpu_cores: cores,
      mem_total: [2, 4, 8, 16, 32][seed % 5] * 1024 ** 3,
      disk_total: [40, 80, 120, 240][seed % 4] * 1024 ** 3,
      virt: seed % 4 === 0 ? 'KVM' : '物理机',
      // 功耗能力探测：RAPL 暴露 / 电池存在 / 校准基础功耗
      power: {
        rapl: hasRapl,
        battery: seed % 4 === 0,
        base_load_w: hasRapl ? Math.round((4 + (seed % 12)) * 10) / 10 : 0,
        base_load_source: 'default',
      },
    },
    geo: { public_ip: `203.0.113.${seed % 254 + 1}`, country: geo.country, city: geo.city, region_text: geo.region },
    host_key_fp: `SHA256:${[...Array(11)].map(() => 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'[(seed * 7 + Math.floor(Math.random() * 64)) % 64]).join('')}`,
  }
}

// ---------- 初始数据 ----------
function seedAudit() {
  const now = Math.floor(Date.now() / 1000)
  return [
    { id: uid(), ts: now - 86400 * 2, actor: 'system', action: 'install', target: 'panel', detail: 'BeaconTower 管理端就绪（原型模式，数据存于浏览器本地）' },
  ]
}

function seedServers() {
  const now = Math.floor(Date.now() / 1000)
  return [
    {
      id: 1,
      name: '本机 · BeaconTower 面板',
      region: '本机部署',
      region_source: 'manual',
      tags: ['面板', '部署机'],
      note_public: '',
      note_private: '面板宿主机，1Panel + Docker 常驻',
      sort_order: 0,
      hidden: false,
      enabled: true,
      created_at: now - 86400 * 210,
      ssh: { host: '127.0.0.1', port: 22, username: 'monitor', auth_type: 'key', host_key_fp: 'SHA256:demoLocalPanelFp00000000000000000000000000' },
      profile: { hostname: 'panel-hub', os_name: 'Ubuntu', os_version: '24.04', kernel: '6.8.0-45-generic', arch: 'x86_64', cpu_model: 'Intel(R) Core(TM) i5-6300HQ CPU @ 2.30GHz', cpu_cores: 4, mem_total: 8 * 1024 ** 3, disk_total: 120 * 1024 ** 3, virt: '物理机', public_ip: '10.66.66.66', geo_country: 'CN', geo_city: '' },
      power: { rapl: true, battery: true, base_load_w: 5.6, base_load_source: 'calibration', last_error: '' },
      last_success_at: now - 5,
    },
    {
      id: 2,
      name: '香港 · 轻量 01',
      region: '香港',
      region_source: 'auto',
      tags: ['Web', '主力'],
      note_public: '',
      note_private: '腾讯云轻量 2C4G，¥288/年，2027-03 到期',
      sort_order: 1,
      hidden: false,
      enabled: true,
      created_at: now - 86400 * 96,
      ssh: { host: '203.0.113.7', port: 22, username: 'root', auth_type: 'password', host_key_fp: 'SHA256:demoHkFp00000000000000000000000000000000000' },
      profile: { hostname: 'web-hk-01', os_name: 'Debian', os_version: '12', kernel: '6.1.0-18-amd64', arch: 'x86_64', cpu_model: 'AMD EPYC Genoa', cpu_cores: 4, mem_total: 4 * 1024 ** 3, disk_total: 60 * 1024 ** 3, virt: 'KVM', public_ip: '203.0.113.7', geo_country: 'HK', geo_city: 'Hong Kong' },
      power: { rapl: false, battery: false, base_load_w: 0, base_load_source: 'default', last_error: '' },
      last_success_at: now - 8,
    },
    {
      id: 3,
      name: '东京 · 中转 02',
      region: '东京',
      region_source: 'auto',
      tags: ['中转'],
      note_public: '',
      note_private: '',
      sort_order: 2,
      hidden: false,
      enabled: true,
      created_at: now - 86400 * 30,
      ssh: { host: '198.51.100.23', port: 2222, username: 'ops', auth_type: 'key', host_key_fp: 'SHA256:demoJpFp00000000000000000000000000000000000' },
      profile: { hostname: 'relay-jp-02', os_name: 'AlmaLinux', os_version: '9.4', kernel: '5.14.0-427.el9', arch: 'x86_64', cpu_model: 'Intel(R) Xeon(R) Silver 4214', cpu_cores: 2, mem_total: 2 * 1024 ** 3, disk_total: 40 * 1024 ** 3, virt: 'KVM', public_ip: '198.51.100.23', geo_country: 'JP', geo_city: 'Tokyo' },
      power: { rapl: true, battery: false, base_load_w: 8.2, base_load_source: 'default', last_error: '认证失败：Permission denied (publickey)' },
      last_success_at: now - 7260,
    },
  ]
}

function seedSettings() {
  return {
    site_title: 'BeaconTower · 信标塔',
    interval_s: 10,
    retention_days: 30,
    open_7d_history: false,
    show_power_public: true,
    show_cost_public: true,
    electric_price: 0.6,
    strict_host_key: false,
    private_mode: false,
  }
}

// ---------- 会话 ----------
const session = {
  get token() {
    return sessionStorage.getItem(`${NS}.session`)
  },
  set(v) {
    if (v) sessionStorage.setItem(`${NS}.session`, v)
    else sessionStorage.removeItem(`${NS}.session`)
  },
}

// ---------- 审计 ----------
export async function audit(action, target, detail) {
  const list = read('audit', seedAudit())
  list.unshift({
    id: uid(),
    ts: Math.floor(Date.now() / 1000),
    actor: admin()?.username || 'system',
    action,
    target,
    detail,
    source_ip_hash: 'sha256:' + Math.random().toString(16).slice(2, 10),
  })
  write('audit', list.slice(0, 500))
}

// ---------- 导出 API（async，形态对齐真后端调用） ----------
export const adminApi = {
  // 状态：是否已初始化 / 当前会话
  async status() {
    const a = admin()
    return { initialized: !!a, loggedIn: !!session.token && !!a, site_title: settings().site_title, username: a?.username || '' }
  },

  // 初始化（仅一次）
  async setup({ username, password }) {
    await delay(400)
    if (admin()) throw new Error('已初始化，禁止重复初始化')
    if (!username || username.length < 3) throw new Error('用户名至少 3 个字符')
    const st = passwordStrength(password)
    if (!st.ok) throw new Error('密码不满足强度要求：至少 10 位，含大小写字母、数字与符号')
    write('admin', { username, password_hash: hashPw(password), created_at: Math.floor(Date.now() / 1000) })
    session.set('sess-' + Math.random().toString(36).slice(2))
    await audit('setup', 'admin', `初始化管理员 ${username}`)
    return { ok: true }
  },

  async login({ username, password }) {
    await delay(400)
    const a = admin()
    if (!a || a.username !== username || a.password_hash !== hashPw(password)) {
      await audit('login_fail', 'admin', `登录失败（${username || '空用户名'}）`)
      throw new Error('用户名或密码错误')
    }
    session.set('sess-' + Math.random().toString(36).slice(2))
    await audit('login', 'admin', '管理员登录成功')
    return { ok: true }
  },

  async logout() {
    session.set(null)
    return { ok: true }
  },

  async changePassword({ old_password, new_password }) {
    await delay(300)
    const a = admin()
    if (!a || a.password_hash !== hashPw(old_password)) throw new Error('旧密码不正确')
    const st = passwordStrength(new_password)
    if (!st.ok) throw new Error('新密码不满足强度要求：至少 10 位，含大小写字母、数字与符号')
    write('admin', { ...a, password_hash: hashPw(new_password) })
    await audit('password', 'admin', '修改管理员密码')
    return { ok: true }
  },

  // ---- 节点 ----
  async listServers() {
    await delay(120)
    // 凭据不回显：password/private_key/passphrase 永不返回
    return read('servers', seedServers()).map(({ ssh, ...rest }) => ({
      ...rest,
      ssh: ssh ? { host: ssh.host, port: ssh.port, username: ssh.username, auth_type: ssh.auth_type, has_password: !!ssh.password, has_key: !!ssh.private_key, host_key_fp: ssh.host_key_fp } : null,
    }))
  },

  async testConnection(ssh) {
    await delay(900)
    if (!ssh.host || !ssh.username) throw new Error('请填写 SSH 地址与用户名')
    const port = Number(ssh.port) || 22
    // 模拟少数失败场景，便于演示错误呈现
    if (ssh.host.endsWith('.254')) throw new Error('连接超时：主机不可达（8s 超时）')
    return fakeProbe({ ...ssh, port })
  },

  async createServer(payload) {
    await delay(300)
    const list = read('servers', seedServers())
    const id = Math.max(0, ...list.map((s) => s.id)) + 1
    const now = Math.floor(Date.now() / 1000)
    const server = {
      id,
      name: payload.name || `节点 ${id}`,
      region: payload.region || payload.probe?.geo?.region_text || '',
      region_source: payload.region ? 'manual' : 'auto',
      tags: payload.tags || [],
      note_public: payload.note_public || '',
      note_private: payload.note_private || '',
      sort_order: list.length,
      hidden: !!payload.hidden,
      enabled: true,
      created_at: now,
      ssh: {
        host: payload.ssh.host,
        port: Number(payload.ssh.port) || 22,
        username: payload.ssh.username,
        auth_type: payload.ssh.auth_type,
        password: payload.ssh.password || undefined,
        private_key: payload.ssh.private_key || undefined,
        passphrase: payload.ssh.passphrase || undefined,
        host_key_fp: payload.probe?.host_key_fp || '',
      },
      profile: payload.probe?.profile ? { ...payload.probe.profile, public_ip: payload.probe.geo?.public_ip, geo_country: payload.probe.geo?.country, geo_city: payload.probe.geo?.city } : null,
      power: payload.probe?.profile?.power || { rapl: false, battery: false, base_load_w: 0, base_load_source: 'default', last_error: '' },
      last_success_at: payload.probe ? now : null,
    }
    list.push(server)
    write('servers', list)
    await audit('server_create', `server:${id}`, `添加节点 ${server.name}（${server.ssh.host}:${server.ssh.port}）`)
    return { id }
  },

  async updateServer(id, payload) {
    await delay(300)
    const list = read('servers', seedServers())
    const idx = list.findIndex((s) => s.id === id)
    if (idx < 0) throw new Error('节点不存在')
    const old = list[idx]
    const next = {
      ...old,
      ...payload,
      id: old.id,
      region: payload.region !== undefined ? payload.region : old.region,
      region_source: payload.region !== undefined && payload.region ? 'manual' : payload.region === '' ? 'auto' : old.region_source,
      ssh: { ...old.ssh, ...payload.ssh },
    }
    // 凭据留空 = 保留原值（05 文档不回显原则）
    if (!payload.ssh?.password) next.ssh.password = old.ssh.password
    if (!payload.ssh?.private_key) next.ssh.private_key = old.ssh.private_key
    if (!payload.ssh?.passphrase) next.ssh.passphrase = old.ssh.passphrase
    list[idx] = next
    write('servers', list)
    await audit('server_update', `server:${id}`, `编辑节点 ${next.name}`)
    return { ok: true }
  },

  async deleteServer(id) {
    await delay(250)
    const list = read('servers', seedServers())
    const s = list.find((x) => x.id === id)
    write('servers', list.filter((x) => x.id !== id))
    await audit('server_delete', `server:${id}`, `删除节点 ${s?.name || id}`)
    return { ok: true }
  },

  async reorder(ids) {
    const list = read('servers', seedServers())
    for (const [i, id] of ids.entries()) {
      const s = list.find((x) => x.id === id)
      if (s) s.sort_order = i
    }
    list.sort((a, b) => a.sort_order - b.sort_order)
    write('servers', list)
    await audit('server_order', 'servers', '调整节点排序')
    return { ok: true }
  },

  async recalibratePower(id, baseLoadW) {
    await delay(400)
    const list = read('servers', seedServers())
    const s = list.find((x) => x.id === id)
    if (!s) throw new Error('节点不存在')
    s.power = { ...s.power, base_load_w: baseLoadW, base_load_source: 'manual' }
    write('servers', list)
    await audit('power_calibrate', `server:${id}`, `设置节点 ${s.name} 基础功耗 ${baseLoadW}W`)
    return { ok: true }
  },

  // 重新定位（对齐 doc/04 §3：POST /admin/servers/:id/locate）。
  // localStorage 模拟：按 ssh.host 做简单地区映射后覆盖 region。
  async relocate(id) {
    await delay(300)
    const list = read('servers', seedServers())
    const s = list.find((x) => x.id === id)
    if (!s) throw new Error('节点不存在')
    const host = s.ssh?.host || ''
    let region = s.region
    let source = 'auto'
    if (host.includes('203.0.113')) region = '香港'
    else if (host.includes('198.51.100')) region = '东京'
    else if (host.includes('192.0.2')) region = '新加坡'
    else if (host.startsWith('127.') || host.startsWith('10.') || host.startsWith('192.168.') || host === 'localhost') {
      region = '内网'
      source = 'manual'
    }
    s.region = region
    s.region_source = source
    write('servers', list)
    await audit('server_locate', `server:${id}`, `重新定位节点 ${s.name} → ${region}`)
    return { ok: true, region, region_source: source }
  },

  // ---- 设置 ----
  async getSettings() {
    await delay(100)
    return settings()
  },

  async saveSettings(next) {
    await delay(250)
    write('settings', next)
    await audit('settings', 'panel', '更新站点设置')
    return { ok: true }
  },

  // ---- 审计 ----
  async listAudit(page = 1, size = 20) {
    await delay(150)
    const list = read('audit', seedAudit())
    return { total: list.length, items: list.slice((page - 1) * size, page * size) }
  },

  // 演示辅助：重置本地数据
  async resetAll() {
    for (const k of ['admin', 'servers', 'settings', 'audit']) localStorage.removeItem(`${NS}.${k}`)
    session.set(null)
    return { ok: true }
  },
}

function admin() {
  return read('admin', null)
}

function settings() {
  return { ...seedSettings(), ...read('settings', {}) }
}

// 时间格式化（审计用）
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
