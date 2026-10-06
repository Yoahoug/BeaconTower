<!-- ============================================================
     内网穿透平台管理（doc/13）
     平台卡只做展示（各自特色指标），不做「点击切换下方视图」；
     下方是跨平台归一化的一份内容：合并隧道表（平台列标注归属）、
     合并在用节点表（在用展开 / 未在用折叠）、双平台同图流量历史。
     平台差异（Sakura 锁定/迁移/认证、ChmlFrp 下线/二级域名）以
     附加列与独立小节出现，不另起版式。
     ============================================================ -->
<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useAdminStore } from '../../stores/admin'
import AppIcon from '../../components/AppIcon.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import StateEmpty from '../../components/ui/StateEmpty.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import TrendChart from '../../components/charts/TrendChart.vue'
import { fmtBytes } from '../../utils/format'
import { fmtTs, agoFromTs } from '../../api/auth'

const admin = useAdminStore()

// 设备码授权轮询间隔：官方建议 5s，取 3s 让用户等待感更短（后端已按平台
// interval 节流，前端多问几次不会真的多打上游）
const DEVICE_POLL_MS = 3000

// 节点折叠区默认收起；展开状态按平台 id 记忆（解绑后重置无害）

const kindLabel = { natfrp: 'Sakura', chmlfrp: 'ChmlFrp', cloudflared: 'Cloudflare' }
const protoLabel = { tcp: 'TCP', udp: 'UDP', http: 'HTTP', https: 'HTTPS' }

const platforms = computed(() => admin.frpOverview?.platforms || [])

// 每个平台一份详情：账号用量（平台卡）+ 节点候选（建隧道/迁移弹窗）。
// 概览接口不带节点，避免首屏多拉几 MB；详情没回来时节点候选为空。
const detailsById = computed(() => admin.frpDetails || {})
const allNodes = computed(() =>
  platforms.value.flatMap((p) => detailsById.value[p.id]?.nodes || []),
)
const inUseNodes = computed(() => allNodes.value.filter((n) => n.in_use))
const idleNodes = computed(() => allNodes.value.filter((n) => !n.in_use))

// 跨平台合并隧道表：概览接口已带 platform_name/kind，直接平铺
const allTunnels = computed(() => admin.frpOverview?.tunnels || [])

// 节点折叠展开集合（跨平台共用一个开关，表本身就是合并的）
const idleExpanded = ref(false)

function nodePlatform(nodeId) {
  return platforms.value.find((p) => detailsById.value[p.id]?.nodes?.some((n) => n.id === nodeId)) || null
}

function tunnelPlatform(t) {
  return platforms.value.find((p) => p.id === t.platform_id) || null
}

// Cloudflare 独立专线只读判定：同步时平台侧锁了编辑/删除的（非面板统一容器
// 下的规则，如运维手工建的 New-api 专线），操作按钮一律禁用。
function isCfReadonly(t) {
  return t.platform_kind === 'cloudflared' && (t.lock_edit || t.lock_delete)
}

// ---------- 详情加载：进入页面拉一次全部平台，写操作后只刷对应平台 ----------
async function loadDetails() {
  await Promise.all(platforms.value.map((p) => admin.loadFrpDetail(p.id)))
}

async function refresh() {
  // 「刷新」= 现在就向平台拉一次最新状态（轻量同步），再读本地快照；
  // 只读本地库会出现「官方平台已恢复、面板还显示离线」的假象（实测踩过）。
  await admin.frpRefreshLive()
  await loadDetails()
  await loadAllSubdomains()
  await loadDeployments()
}

// 页面停留期间每 60s 只重读本地快照（不发平台请求）：后端每 3 分钟同步一次，
// 这里让页面自动跟上那次同步，省得用户手动刷新。
let idleTimer = 0
async function idleReload() {
  if (admin.frpSaving) return
  await admin.loadFrp()
  await loadDeployments()
}

const syncingId = ref(0)
async function syncOne(p) {
  syncingId.value = p.id
  try {
    await admin.frpSync(p.id, true)
    toast(`${p.name} 同步完成`)
  } catch (e) {
    toast(e?.message || '同步失败', true)
  } finally {
    syncingId.value = 0
  }
}

// ---------- 轻提示（页内，不用 alert） ----------
const tip = ref({ text: '', error: false })
let tipTimer = 0
function toast(text, error = false) {
  tip.value = { text, error }
  window.clearTimeout(tipTimer)
  tipTimer = window.setTimeout(() => (tip.value = { text: '', error: false }), 4000)
}

// ---------- 弹窗 ----------
const bindModal = ref(null) // Sakura / Cloudflare 绑定
const cfHelpModal = ref(false) // Cloudflare Token 获取指引
const deviceModal = ref(null) // ChmlFrp 设备码
const tunnelModal = ref(null) // 建/改隧道
// 建/改隧道弹窗针对 ChmlFrp 的额外提示：平台规则比 Sakura 严（真机实测）
const tunnelModalIsChml = computed(() => tunnelModalKind.value === 'chmlfrp')
const tunnelModalIsCf = computed(() => tunnelModalKind.value === 'cloudflared')
const tunnelModalKind = computed(() => {
  const m = tunnelModal.value
  if (!m) return ''
  return platforms.value.find((p) => p.id === m.platformId)?.kind || ''
})
const migrateModal = ref(null) // 迁移节点
const lockModal = ref(null) // 锁定设置
const confirmState = ref(null) // 删除确认

let pollTimer = 0

function closeModal() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = 0
  }
  bindModal.value = null
  deviceModal.value = null
  tunnelModal.value = null
  migrateModal.value = null
  lockModal.value = null
  deployModal.value = null
  deployLogs.value = null
}

// ---------- 绑定 Sakura / Cloudflare ----------
function openBind(kind = 'natfrp') {
  if (kind === 'cloudflared') {
    bindModal.value = {
      kind,
      name: 'Cloudflare',
      token: '',
      busy: false,
      error: '',
    }
    return
  }
  bindModal.value = { kind: 'natfrp', name: 'Sakura', token: '', busy: false, error: '' }
}
// 帮助弹窗：指导用户在 CF 后台创建 API Token（Account/Zone ID 由面板自动发现）
function openCfHelp() {
  cfHelpModal.value = true
}
async function submitBind() {
  const m = bindModal.value
  if (m.kind === 'cloudflared') {
    if (!m.token.trim()) {
      m.error = '请填写 API Token（点击弹窗标题旁的「?」查看获取步骤）'
      return
    }
    m.busy = true
    m.error = ''
    try {
      await admin.frpBindCloudflared({
        name: m.name.trim() || 'Cloudflare',
        token: m.token.trim(),
      })
      closeModal()
      await refresh()
      toast('Cloudflare 绑定成功')
    } catch (e) {
      m.error = e?.message || '绑定失败'
    } finally {
      if (bindModal.value) bindModal.value.busy = false
    }
    return
  }
  if (!m.token.trim()) {
    m.error = '请填写访问密钥'
    return
  }
  m.busy = true
  m.error = ''
  try {
    await admin.frpBindNatfrp({ name: m.name.trim() || 'Sakura', token: m.token.trim() })
    closeModal()
    await refresh()
    toast('Sakura 绑定成功')
  } catch (e) {
    m.error = e?.message || '绑定失败'
  } finally {
    if (bindModal.value) bindModal.value.busy = false
  }
}

// ---------- ChmlFrp 设备码授权 ----------
// reuseId > 0 表示给已有平台重新授权（保留隧道镜像与用量历史）
async function openDevice(reuseId = 0) {
  deviceModal.value = {
    reuseId,
    status: 'starting',
    userCode: '',
    verifyUrl: '',
    error: '',
    secondsLeft: 0,
  }
  try {
    const r = await admin.deviceStart(reuseId)
    deviceModal.value.status = 'pending'
    deviceModal.value.userCode = r.user_code
    deviceModal.value.verifyUrl = r.verify_url
    deviceModal.value.secondsLeft = r.expires_in
    startPolling(r.session_id)
  } catch (e) {
    deviceModal.value.status = 'error'
    deviceModal.value.error = e?.message || '发起授权失败'
  }
}
function startPolling(sessionId) {
  if (pollTimer) window.clearInterval(pollTimer)
  pollTimer = window.setInterval(async () => {
    const m = deviceModal.value
    if (!m) return
    if (m.secondsLeft > 0) m.secondsLeft -= DEVICE_POLL_MS / 1000
    try {
      const r = await admin.devicePoll(sessionId)
      if (r.status === 'ok') {
        window.clearInterval(pollTimer)
        pollTimer = 0
        closeModal()
        await refresh()
        toast(`ChmlFrp 授权成功${r.username ? `（${r.username}）` : ''}`)
        return
      }
      if (r.status !== 'pending') {
        window.clearInterval(pollTimer)
        pollTimer = 0
        m.status = r.status
        m.error = r.error || '授权失败，请重新发起'
      }
    } catch (e) {
      window.clearInterval(pollTimer)
      pollTimer = 0
      m.status = 'error'
      m.error = e?.message || '轮询失败'
    }
  }, DEVICE_POLL_MS)
}

// ---------- 建/改隧道 ----------
function openTunnelCreate() {
  tunnelModal.value = {
    mode: 'create',
    platformId: platforms.value[0]?.id || 0,
    nodes: allNodes.value,
    name: '',
    proto: 'tcp',
    nodeId: '',
    localIp: '127.0.0.1',
    localPort: '',
    remotePort: '',
    domain: '',
    note: '',
    extra: '',
    busy: false,
    error: '',
  }
}

// 建隧道时切换平台 → 节点候选跟随该平台的节点镜像（Cloudflare 无节点概念）
function onCreatePlatformChange() {
  const m = tunnelModal.value
  if (!m || m.mode !== 'create') return
  m.nodeId = ''
  m.nodes = detailsById.value[m.platformId]?.nodes || []
}
function openTunnelEdit(t) {
  const sec = detailsById.value[t.platform_id]
  tunnelModal.value = {
    mode: 'edit',
    id: t.id,
    platformId: t.platform_id,
    platformName: t.platform_name,
    nodes: sec?.nodes || [],
    name: t.name,
    proto: t.proto,
    nodeId: '',
    nodeName: t.node_name,
    localIp: t.local_ip,
    localPort: String(t.local_port || ''),
    remotePort: /^\d+$/.test(t.remote) ? t.remote : '',
    domain: /^\d+$/.test(t.remote) ? '' : t.remote,
    note: t.extra || '',
    extra: '',
    busy: false,
    error: '',
  }
}

// 隧道名输入框随平台切换占位语义：Cloudflare 的「隧道名」是完整域名
// （ingress hostname），其余平台是任意名称。
const tunnelNamePlaceholder = computed(() => (tunnelModalIsCf.value ? 'app.yoahoug.dev' : ''))

async function submitTunnel() {
  const m = tunnelModal.value
  const payload = {
    name: m.name.trim(),
    proto: m.proto,
    node_id: m.nodeId,
    local_ip: m.localIp.trim(),
    local_port: Number(m.localPort) || 0,
    remote_port: Number(m.remotePort) || 0,
    domain: m.domain.trim(),
    note: m.note.trim(),
    extra: m.extra.trim(),
  }
  if (!payload.name) {
    m.error = '请填写隧道名'
    return
  }
  if (!payload.local_port) {
    m.error = '请填写本地端口'
    return
  }
  if ((m.proto === 'http' || m.proto === 'https') && !payload.domain) {
    m.error = 'HTTP(S) 隧道必须填写绑定域名'
    return
  }
  if (tunnelModalIsCf.value && !payload.name.includes('.')) {
    m.error = 'Cloudflare 的隧道名是 ingress 域名，须填完整域名（如 app.yoahoug.dev）'
    return
  }
  m.busy = true
  m.error = ''
  try {
    if (m.mode === 'create') {
      if (!tunnelModalIsCf.value && !payload.node_id) {
        m.error = '请选择节点'
        return
      }
      await admin.frpTunnelCreate(m.platformId, payload)
      toast('隧道已创建')
    } else {
      await admin.frpTunnelUpdate(m.id, payload, m.platformId)
      toast('隧道已更新')
    }
    closeModal()
  } catch (e) {
    m.error = e?.message || '提交失败'
  } finally {
    if (tunnelModal.value) tunnelModal.value.busy = false
  }
}

// ---------- 迁移 / 锁定 ----------
function openMigrate(t) {
  const sec = detailsById.value[t.platform_id]
  migrateModal.value = {
    id: t.id,
    platformId: t.platform_id,
    name: t.name,
    nodes: sec?.nodes || [],
    nodeId: '',
    busy: false,
    error: '',
  }
}
async function submitMigrate() {
  const m = migrateModal.value
  if (!m.nodeId) {
    m.error = '请选择目标节点'
    return
  }
  m.busy = true
  m.error = ''
  try {
    await admin.frpTunnelMigrate(m.id, m.nodeId, m.platformId)
    closeModal()
    toast('迁移已提交')
  } catch (e) {
    m.error = e?.message || '迁移失败'
  } finally {
    if (migrateModal.value) migrateModal.value.busy = false
  }
}

function openLock(t) {
  lockModal.value = {
    id: t.id,
    platformId: t.platform_id,
    name: t.name,
    edit: t.lock_edit,
    del: t.lock_delete,
    migrate: t.lock_migrate,
    busy: false,
    error: '',
  }
}
async function submitLock() {
  const m = lockModal.value
  m.busy = true
  m.error = ''
  try {
    await admin.frpTunnelLock(m.id, { edit: m.edit, delete: m.del, migrate: m.migrate }, m.platformId)
    closeModal()
    toast('锁定设置已更新')
  } catch (e) {
    m.error = e?.message || '操作失败'
  } finally {
    if (lockModal.value) lockModal.value.busy = false
  }
}

// ---------- 其它隧道动作 ----------
async function offlineTunnel(t) {
  try {
    await admin.frpTunnelOffline(t.id, t.platform_id)
    toast('已强制下线')
  } catch (e) {
    toast(e?.message || '操作失败', true)
  }
}
async function authTunnel(t) {
  try {
    const r = await admin.frpTunnelAuth(t.id, '', t.platform_id)
    toast(`已授权访问：${r.ip || '当前来源 IP'}`)
  } catch (e) {
    toast(e?.message || '操作失败', true)
  }
}
function askDeleteTunnel(t) {
  confirmState.value = {
    title: '删除隧道',
    message: `确认在 ${kindLabel[t.platform_kind] || t.platform_kind} 平台删除隧道「${t.name}」？该操作会直接调用平台接口，不可撤销。`,
    confirmText: '删除',
    danger: true,
    busy: false,
    run: async () => {
      await admin.frpTunnelDelete(t.id, t.platform_id)
      toast('隧道已删除')
    },
  }
}
function askDeletePlatform(p) {
  confirmState.value = {
    title: '解绑平台',
    message: `确认解绑「${p.name}」？将移除面板中该账号的隧道与节点镜像（不影响平台侧的实际隧道）。`,
    confirmText: '解绑',
    danger: true,
    busy: false,
    run: async () => {
      await admin.frpDeletePlatform(p.id)
      toast('已解绑')
    },
  }
}
function askDeleteSubdomain(sd) {
  const p = platforms.value.find((x) => x.kind === 'chmlfrp')
  confirmState.value = {
    title: '删除解析记录',
    message: `确认删除 ${sd.record}.${sd.domain}？会直接调用 ChmlFrp 接口。`,
    confirmText: '删除',
    danger: true,
    busy: false,
    run: async () => {
      await admin.deleteSubdomain(p.id, sd.domain, sd.record)
      await loadAllSubdomains()
      toast('解析已删除')
    },
  }
}
async function runConfirm() {
  const c = confirmState.value
  if (!c) return
  c.busy = true
  try {
    await c.run()
    confirmState.value = null
  } catch (e) {
    toast(e?.message || '操作失败', true)
    c.busy = false
  }
}

// ---------- 配置下载 ----------
// 二进制/文本下载走裸 fetch（doc/10 §8 明示例外），必须 revokeObjectURL
const downloadingId = ref(0)
async function downloadConfig(t) {
  downloadingId.value = t.id
  try {
    const resp = await fetch(`/api/v1/admin/frp/tunnels/${t.id}/config`, { credentials: 'same-origin' })
    if (!resp.ok) {
      let msg = `下载失败（HTTP ${resp.status}）`
      try {
        const j = await resp.json()
        if (j?.msg) msg = j.msg
      } catch {
        /* 非 JSON 错误体，保留默认提示 */
      }
      throw new Error(msg)
    }
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${t.platform_kind}-${t.name}.conf`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast(e?.message || '下载失败', true)
  } finally {
    downloadingId.value = 0
  }
}

// ---------- 客户端托管（面板经 SSH 在节点上部署/管理 frpc 容器，doc/13 §14） ----------
// 与「配置下载」互补：下载 = 手动拿去节点上跑；托管 = 面板拉镜像、起容器、随隧道变更同步。
const deployments = ref([])
const deployLoading = ref(false)
const deployOpsId = ref(0) // 行内动作忙碌标记（同步/重启/停止/启动）
const deployModal = ref(null) // 新建托管弹窗
const deployLogs = ref(null) // 日志弹窗
const deployServers = ref([])

function deployStatusTag(d) {
  if (d.dirty) return { cls: 'bt-tag--warning', text: '待同步' }
  if (d.status === 'running') return { cls: 'bt-tag--success', text: '运行中' }
  if (d.status === 'stopped') return { cls: 'bt-tag--info', text: '已停止' }
  if (d.status === 'missing') return { cls: 'bt-tag--warning', text: '容器丢失' }
  if (d.status === 'error') return { cls: 'bt-tag--danger', text: '异常' }
  return { cls: '', text: '待部署' }
}

async function loadDeployments(refreshLive = false) {
  deployLoading.value = true
  try {
    deployments.value = (await admin.frpDeployments(refreshLive)).deployments || []
  } catch (e) {
    toast(e?.message || '托管列表加载失败', true)
  } finally {
    deployLoading.value = false
  }
}

// 一个节点同一平台只托管一份客户端（唯一键 platform_id+server_id），
// 节点下拉里指出已被占用的，免得用户填完才被后端拒。
function deployTakenServerIds(platformId, exceptId = 0) {
  return deployments.value
    .filter((d) => d.platform_id === platformId && d.id !== exceptId)
    .map((d) => d.server_id)
}

async function openDeploy(t = null) {
  const platformId = t?.platform_id || platforms.value[0]?.id || 0
  deployModal.value = {
    editId: 0,
    platformId,
    serverId: '',
    tunnelIds: t ? [t.id] : [],
    installDocker: false,
    docker: null, // {present, version, daemon_ok, err}
    probing: false,
    busy: false,
    elapsed: 0,
    error: '',
  }
  if (!deployServers.value.length) {
    await admin.loadServers()
    deployServers.value = admin.servers || []
  }
}

// 调整已有托管的隧道集合：平台/节点锁死（换机器走「移除」再部署，避免留下孤儿容器），
// 提交走同一个 upsert 接口（唯一键是 平台+节点），因此直接重同步即可。
function openDeployEdit(d) {
  deployModal.value = {
    editId: d.id,
    platformId: d.platform_id,
    serverId: String(d.server_id),
    tunnelIds: [...d.tunnel_ids],
    installDocker: false,
    docker: { present: true, version: d.docker_ver, daemon_ok: true },
    probing: false,
    busy: false,
    elapsed: 0,
    error: '',
  }
}

// 弹窗里换平台 → 清掉已勾隧道（隧道 id 跨平台不通用）与 Docker 探测结果
function onDeployPlatformChange() {
  const m = deployModal.value
  if (!m) return
  m.tunnelIds = []
  m.docker = null
}

function deployTunnelPool(platformId) {
  return allTunnels.value.filter((t) => t.platform_id === platformId)
}

// 该隧道是否已被「另一条托管」占用（编辑当前这条托管时不算）。
// 同一隧道被两台机器同时跑去抢，平台侧会一直 proxy conflict，严重的会被判为异常——
// 所以这里直接禁选，而不是等用户提交后才报错。
function deployTunnelTaken(t) {
  const d = deployByTunnel.value[t.id]
  return !!d && d.id !== (deployModal.value?.editId || 0)
}

// 节点能否被 SSH 管理：列表接口把凭据摘要放在 ssh 子对象里（凭据本身不回显）
function deploySshReady(s) {
  return !!(s?.ssh && s.ssh.ssh_ready)
}

// 当前弹窗里该平台已被其他托管占用的节点（用于下拉置灰）
const deployTakenIds = computed(() => {
  const m = deployModal.value
  return m ? deployTakenServerIds(m.platformId, m.editId) : []
})

async function probeDocker() {
  const m = deployModal.value
  if (!m?.serverId) {
    m.error = '请先选择节点'
    return
  }
  m.probing = true
  m.error = ''
  try {
    m.docker = (await admin.frpServerDocker(Number(m.serverId), false)).docker
  } catch (e) {
    m.docker = { present: false, err: e?.message || '检测失败' }
  } finally {
    m.probing = false
  }
}

async function submitDeploy() {
  const m = deployModal.value
  if (!m) return
  if (!m.serverId) {
    m.error = '请选择节点'
    return
  }
  if (!m.tunnelIds.length) {
    m.error = '请至少勾选一条隧道'
    return
  }
  m.busy = true
  m.error = ''
  // 装 Docker + 拉镜像可能好几分钟，走动的秒数让用户知道没卡死
  const t0 = Date.now()
  m.elapsed = 0
  const tick = window.setInterval(() => {
    if (deployModal.value) deployModal.value.elapsed = Math.round((Date.now() - t0) / 1000)
  }, 1000)
  try {
    await admin.frpDeployCreate({
      platform_id: m.platformId,
      server_id: Number(m.serverId),
      tunnel_ids: m.tunnelIds,
      install_docker: m.installDocker,
    })
    toast(m.editId ? '已重新同步托管客户端' : '客户端已在节点上部署')
    closeModal()
    await loadDeployments(true)
  } catch (e) {
    m.error = e?.message || '部署失败'
  } finally {
    window.clearInterval(tick)
    if (deployModal.value) deployModal.value.busy = false
  }
}

async function runDeployOp(d, fn, okText) {
  deployOpsId.value = d.id
  try {
    await fn()
    if (okText) toast(okText)
  } catch (e) {
    toast(e?.message || '操作失败', true)
  } finally {
    deployOpsId.value = 0
  }
}

function deploySync(d) {
  return runDeployOp(d, async () => {
    const r = await admin.frpDeploySync(d.id)
    await loadDeployments()
    toast(r?.log_tail ? '已同步（容器有输出，建议看日志）' : '配置已同步')
  })
}

function deployAction(d, action) {
  const text = { restart: '容器已重启', stop: '容器已停止', start: '容器已启动' }[action] || '已执行'
  return runDeployOp(d, async () => {
    await admin.frpDeployAction(d.id, action)
    await loadDeployments()
    toast(text)
  })
}

async function openDeployLogs(d) {
  deployLogs.value = { id: d.id, title: `${d.container} · ${d.server_name}`, loading: true, text: '', error: '' }
  try {
    const r = await admin.frpDeployLogs(d.id, 200)
    if (deployLogs.value) deployLogs.value.text = r?.logs || ''
  } catch (e) {
    if (deployLogs.value) deployLogs.value.error = e?.message || '日志读取失败'
  } finally {
    if (deployLogs.value) deployLogs.value.loading = false
  }
}

function askDeleteDeploy(d) {
  confirmState.value = {
    title: '移除托管',
    message: `确认移除「${d.server_name}」上的 ${kindLabel[d.platform_kind] || d.platform_kind} 客户端？`
      + '将停止并删除节点上的 frpc 容器、清除节点上的配置文件；平台侧隧道与其他机器上手动跑的客户端不受影响。',
    confirmText: '移除',
    danger: true,
    busy: false,
    run: async () => {
      await admin.frpDeployDelete(d.id)
      await loadDeployments()
      toast('已移除托管')
    },
  }
}

// ---------- ChmlFrp 子域名（跨平台唯一的平台特有区块） ----------
const subs = ref({ loading: false, list: [], domains: [], error: '' })
const subModal = ref(null)

async function loadAllSubdomains() {
  const p = platforms.value.find((x) => x.kind === 'chmlfrp')
  if (!p) {
    subs.value = { loading: false, list: [], domains: [], error: '' }
    return
  }
  subs.value.loading = true
  subs.value.error = ''
  try {
    const [a, b] = await Promise.all([admin.subdomains(p.id), admin.availableDomains(p.id)])
    subs.value = { loading: false, list: a.subdomains || [], domains: b.domains || [], error: '' }
  } catch (e) {
    subs.value = { ...(subs.value), loading: false, error: e?.message || '域名信息加载失败' }
  }
}

const subTTLs = ['1分钟', '2分钟', '5分钟', '10分钟', '15分钟', '30分钟', '1小时', '2小时', '5小时', '12小时', '1天']

function openSubCreate() {
  subModal.value = {
    platformId: platforms.value.find((x) => x.kind === 'chmlfrp')?.id || 0,
    mode: 'create',
    domain: (subs.value.domains.map((d) => d.domain) || [])[0] || '',
    record: '',
    type: 'A',
    target: '',
    ttl: '10分钟',
    remarks: '',
    busy: false,
    error: '',
  }
}
function openSubEdit(sd) {
  subModal.value = {
    platformId: platforms.value.find((x) => x.kind === 'chmlfrp')?.id || 0,
    mode: 'edit',
    ...sd,
    busy: false,
    error: '',
  }
}
async function submitSub() {
  const m = subModal.value
  if (!m.domain || !m.record.trim() || !m.target.trim()) {
    m.error = '主域名、主机记录、目标地址均为必填'
    return
  }
  m.busy = true
  m.error = ''
  const payload = {
    domain: m.domain,
    record: m.record.trim(),
    type: m.type,
    target: m.target.trim(),
    ttl: m.ttl,
    remarks: m.remarks || '',
  }
  try {
    if (m.mode === 'create') await admin.createSubdomain(m.platformId, payload)
    else await admin.updateSubdomain(m.platformId, payload)
    closeModal()
    await loadAllSubdomains()
    toast(m.mode === 'create' ? '解析已创建' : '解析已更新')
  } catch (e) {
    m.error = e?.message || '提交失败'
  } finally {
    if (subModal.value) subModal.value.busy = false
  }
}

// ---------- 归一化 ----------
const chmlPlatform = computed(() => platforms.value.find((p) => p.kind === 'chmlfrp') || null)

// 双平台同图流量历史：每个平台一条序列（ChmlFrp 只有近 7 日，
// Sakura 的 day 口径同为 7 点，天然对齐）
const TREND_COLORS = { natfrp: '#0ea5e9', chmlfrp: '#8b5cf6' }
const trend = ref({ loading: false })
const trendSeries = ref([])
const trendLabels = ref([])

async function loadTrend() {
  if (!platforms.value.length) return
  trend.value.loading = true
  try {
    const results = await Promise.all(
      platforms.value.map(async (p) => {
        try {
          const r = await admin.frpFlow(p.id, 'day')
          return { p, points: r.points || [] }
        } catch {
          return { p, points: [] }
        }
      }),
    )
    // 任一平台有点才算有数据；标签以点最多的平台为基准
    const withData = results.filter((r) => r.points.length)
    const base = withData.reduce((best, r) => (r.points.length > best.length ? r.points : best), [])
    trendLabels.value = base.map((x) => x.label)
    trendSeries.value = withData.map((r) => ({
      name: r.p.name,
      color: TREND_COLORS[r.p.kind] || '#0ea5e9',
      data: trendLabels.value.map(
        (l) => Number(r.points.find((x) => x.label === l)?.used) || 0,
      ),
      fill: false,
    }))
  } finally {
    trend.value.loading = false
  }
}

function statusTag(p) {
  if (p.status === 'ok') return { cls: 'bt-tag--success', text: '正常' }
  if (p.status === 'unbound') return { cls: 'bt-tag--danger', text: '需重新授权' }
  if (p.status === 'error') return { cls: 'bt-tag--warning', text: '同步异常' }
  return { cls: 'bt-tag--info', text: '未同步' }
}

function tunnelStatus(t) {
  if (t.online) return { cls: 'bt-tag--success', text: '在线' }
  if (t.status === 'banned') return { cls: 'bt-tag--danger', text: '封禁' }
  if (t.status && t.status !== 'normal' && t.status !== 'unknown') return { cls: 'bt-tag--warning', text: t.status }
  // 离线本身不是错误（可能只是没人跑客户端），但要看得见：给一档比平台标签更实的灰
  return { cls: 'frp-tag-offline', text: '离线' }
}

// 隧道 → 承载它的面板托管（用于「面板托管」标记与「释放占用」）
const deployByTunnel = computed(() => {
  const m = {}
  for (const d of deployments.value) {
    for (const id of d.tunnel_ids || []) m[id] = d
  }
  return m
})

// 释放占用：让面板托管的客户端停下，把隧道让给别的机器/官方客户端。
// 单独成动作是因为「面板占着隧道但用户不知道」会让平台侧后续登录一直 proxy conflict。
function askReleaseTunnel(t) {
  const d = deployByTunnel.value[t.id]
  if (!d) return
  confirmState.value = {
    title: '释放隧道占用',
    message: `确认让「${d.server_name}」上的面板客户端停止占用隧道「${t.name}」？`
      + '将停止该节点上的 frpc 容器（配置与记录保留，随时可再启动）；'
      + '之后这条隧道可由其它机器或官方客户端接管。',
    confirmText: '停止占用',
    danger: false,
    busy: false,
    run: async () => {
      await admin.frpDeployAction(d.id, 'stop')
      await loadDeployments()
      toast('已释放：该隧道的客户端已停止')
    },
  }
}

function loadTone(load) {
  if (load >= 80) return 'is-full'
  if (load >= 50) return 'is-warn'
  return ''
}

function kindIcon(kind) {
  if (kind === 'chmlfrp') return 'globe'
  if (kind === 'cloudflared') return 'shield'
  return 'tunnel'
}

// 平台标签配色：Sakura 蓝、ChmlFrp 紫、Cloudflare 橙（复用 frp-tag-* 样式族）
function kindTagClass(kind) {
  if (kind === 'chmlfrp') return 'frp-tag-chml'
  if (kind === 'cloudflared') return 'frp-tag-cf'
  return 'frp-tag-nat'
}

function capsText(caps) {
  const map = {
    http: 'HTTP 建站', https: 'HTTPS 建站', web: '建站', udp: 'UDP',
    create: '可创建', mainland: '内地', nodefense: '无防', defense: '有防御',
    private: '私有', auth: '强制认证', beta: 'BETA', vip: 'VIP', ipv6: 'IPv6',
  }
  return (caps || []).map((c) => map[c] || c)
}

onMounted(async () => {
  await refresh()
  await loadTrend()
  idleTimer = window.setInterval(idleReload, 60000)
})

onBeforeUnmount(() => {
  window.clearTimeout(tipTimer)
  if (pollTimer) window.clearInterval(pollTimer)
  if (idleTimer) window.clearInterval(idleTimer)
})
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>内网穿透</h1>
        <p class="page-head__desc">
          Sakura / ChmlFrp 隧道统一管理 · 账号用量 · 节点负载 · 后台每 3 分钟同步（「刷新」= 立即向平台拉取）
        </p>
      </div>
      <div class="page-head__actions">
        <button class="bt-btn bt-btn--default bt-btn--sm" type="button" :disabled="admin.frpSaving" @click="refresh"
          title="立即向各平台拉一次最新状态（轻量同步，不含节点列表）">
          <AppIcon name="refresh" aria-hidden="true" />{{ admin.frpSaving ? '同步中…' : '刷新' }}
        </button>
        <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="openDevice(0)">
          <AppIcon name="plus" aria-hidden="true" />授权 ChmlFrp
        </button>
        <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="openBind('cloudflared')">
          <AppIcon name="plus" aria-hidden="true" />绑定 Cloudflare
        </button>
        <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="openBind('natfrp')">
          <AppIcon name="plus" aria-hidden="true" />绑定 Sakura
        </button>
      </div>
    </div>

    <div v-if="tip.text" class="bt-alert" :class="tip.error ? 'bt-alert--error' : 'bt-alert--success'"
      :role="tip.error ? 'alert' : 'status'" style="margin-bottom: 12px">
      {{ tip.text }}
    </div>

    <StateError v-if="admin.frpError" style="margin-bottom: 12px" :message="admin.frpError" @retry="refresh" />
    <StateSkeleton v-if="admin.frpLoading && !platforms.length" style="margin-bottom: 12px" :rows="4" />

    <StateEmpty v-if="!admin.frpLoading && !platforms.length && !admin.frpError"
      title="尚未接入穿透平台"
      desc="绑定 Sakura 访问密钥、授权 ChmlFrp 账号或绑定 Cloudflare API Token 后即可在此管理隧道">
      <div style="display: flex; gap: 8px; justify-content: center; margin-top: 12px; flex-wrap: wrap">
        <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="openBind('natfrp')">绑定 Sakura</button>
        <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="openDevice(0)">授权 ChmlFrp</button>
        <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="openBind('cloudflared')">绑定 Cloudflare</button>
      </div>
    </StateEmpty>

    <!-- ===== 平台卡（纯展示，不切换下方内容；各自特色指标在卡内） ===== -->
    <template v-if="platforms.length">
      <p class="frp-sec-title">穿透平台</p>
      <div class="frp-platforms">
        <div v-for="p in platforms" :key="p.id" class="frp-platform" :class="{ 'is-degraded': p.status !== 'ok' }">
          <span class="frp-platform__avatar" :class="`frp-platform__avatar--${p.kind}`">
            <AppIcon :name="kindIcon(p.kind)" aria-hidden="true" />
          </span>
          <span class="frp-platform__main">
            <span class="frp-platform__head">
              <span class="frp-platform__name">{{ p.name }}</span>
              <span class="bt-tag" :class="statusTag(p).cls">{{ statusTag(p).text }}</span>
            </span>
            <span class="frp-platform__stats">
              <span class="frp-stat">
                <span class="frp-stat__num tnum">{{ p.tunnel_online }}<i>/</i>{{ p.tunnel_total }}</span>
                <span class="frp-stat__label">隧道在线</span>
              </span>
              <span class="frp-stat">
                <span class="frp-stat__num tnum">
                  {{ p.tunnel_quota && p.tunnel_quota > 0 ? `${p.tunnel_used}/${p.tunnel_quota}` : p.tunnel_used }}
                </span>
                <span class="frp-stat__label">配额</span>
              </span>
              <span class="frp-stat">
                <span class="frp-stat__num tnum">{{
                  p.kind === 'natfrp'
                    ? (p.traffic_remain ? fmtBytes(p.traffic_remain) : '—')
                    : p.kind === 'cloudflared' ? '—' : fmtBytes(p.traffic_up)
                }}</span>
                <span class="frp-stat__label">{{ p.kind === 'natfrp' ? '剩余流量' : p.kind === 'cloudflared' ? '流量统计' : '累计上传' }}</span>
              </span>
              <span v-if="p.kind === 'natfrp'" class="frp-stat">
                <span class="frp-stat__num">{{ p.profile?.sign_signed ? `签 ${p.profile?.sign_days || 0} 天` : '未签' }}</span>
                <span class="frp-stat__label">签到</span>
              </span>
              <span v-else-if="p.kind === 'chmlfrp'" class="frp-stat">
                <span class="frp-stat__num tnum">{{ fmtBytes(p.traffic_down) }}</span>
                <span class="frp-stat__label">累计下载</span>
              </span>
              <span v-else class="frp-stat">
                <span class="frp-stat__num tnum">{{ p.conns || '—' }}</span>
                <span class="frp-stat__label">连接器</span>
              </span>
            </span>
            <span v-if="p.tunnel_quota && p.tunnel_quota > 0" class="frp-quota">
              <i class="frp-quota__bar" :class="loadTone((p.tunnel_used / p.tunnel_quota) * 100)"
                :style="{ width: Math.min(100, (p.tunnel_used / p.tunnel_quota) * 100) + '%' }" />
            </span>
            <span v-if="p.last_error" class="frp-platform__err">{{ p.last_error }}</span>
            <span class="frp-platform__foot">同步于 {{ p.last_sync_at ? agoFromTs(p.last_sync_at) : '尚未同步' }}</span>
          </span>
          <span class="frp-platform__side">
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
              :disabled="syncingId === p.id" @click="syncOne(p)">
              <AppIcon name="refresh" aria-hidden="true" />
              {{ syncingId === p.id ? '同步中…' : '同步' }}
            </button>
            <button v-if="p.kind === 'chmlfrp'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
              @click="openDevice(p.id)">重新授权</button>
            <button v-if="p.kind === 'natfrp'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
              @click="openBind('natfrp')">换密钥</button>
            <button v-if="p.kind === 'cloudflared'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
              @click="openBind('cloudflared')">换 Token</button>
            <button class="bt-btn bt-btn--danger-ghost bt-btn--sm" type="button" @click="askDeletePlatform(p)">
              解绑
            </button>
          </span>
        </div>
      </div>

      <!-- ===== 归一化：合并用量条（跨平台一行铺开，每块标平台名） ===== -->
      <div class="bt-card frp-block">
        <div class="bt-card__head">
          <h2 class="bt-card__title">账号用量</h2>
          <span class="bt-text-muted frp-head-hint">同列指标按平台口径换算：剩余流量 / 累计上传</span>
        </div>
        <div class="bt-card__body">
          <div class="frp-tiles">
            <div v-for="p in platforms" :key="`u-${p.id}`" class="frp-tile frp-tile--plat">
              <span class="frp-tile__icon" :class="p.kind === 'chmlfrp' ? 'frp-tile__icon--violet' : p.kind === 'cloudflared' ? 'frp-tile__icon--cf' : 'frp-tile__icon--brand'">
                <AppIcon :name="kindIcon(p.kind)" aria-hidden="true" />
              </span>
              <div class="frp-tile__grid">
                <div class="frp-tile__row">
                  <span class="frp-tile__label">今日流量</span>
                  <span class="frp-tile__value tnum">{{ p.kind === 'cloudflared' ? '—' : (p.traffic_day_used ? fmtBytes(p.traffic_day_used) : '—') }}</span>
                </div>
                <div class="frp-tile__row">
                  <span class="frp-tile__label">{{ p.kind === 'natfrp' ? '剩余流量' : p.kind === 'cloudflared' ? '流量统计' : '累计上传/下载' }}</span>
                  <span class="frp-tile__value tnum">
                    {{ p.kind === 'natfrp'
                      ? (p.traffic_remain ? fmtBytes(p.traffic_remain) : '—')
                      : p.kind === 'cloudflared' ? '平台不提供' : `${fmtBytes(p.traffic_up)} / ${fmtBytes(p.traffic_down)}` }}
                  </span>
                </div>
                <div class="frp-tile__row">
                  <span class="frp-tile__label">限速</span>
                  <span class="frp-tile__value">{{ p.speed_limit || (p.kind === 'cloudflared' ? '不限' : '—') }}</span>
                </div>
                <div class="frp-tile__row">
                  <span class="frp-tile__label">上次同步</span>
                  <span class="frp-tile__value">{{ p.last_sync_at ? fmtTs(p.last_sync_at) : '—' }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- ===== 合并流量历史：每个平台一条序列，同图对比 ===== -->
      <div class="bt-card frp-block">
        <div class="bt-card__head">
          <h2 class="bt-card__title">流量历史</h2>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="loadTrend">
            <AppIcon name="refresh" aria-hidden="true" />重新加载
          </button>
        </div>
        <div class="bt-card__body">
          <StateSkeleton v-if="trend.loading" :rows="3" />
          <StateEmpty v-else-if="!trendSeries.length" title="暂无历史数据"
            desc="平台侧的流量历史接口粒度较粗，点「重新加载」拉取；趋势图也依赖每轮同步的本地快照累积" />
          <TrendChart v-else :series="trendSeries" :x-labels="trendLabels" :height="220"
            :y-formatter="fmtBytes" label="平台流量历史" />
        </div>
      </div>

      <!-- ===== 合并隧道表：平台列标注归属，操作列按平台能力渲染 ===== -->
      <div class="bt-card frp-block">
        <div class="bt-card__head">
          <h2 class="bt-card__title">隧道 <span class="frp-count">{{ allTunnels.length }}</span></h2>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="openTunnelCreate">
            <AppIcon name="plus" aria-hidden="true" />新建隧道
          </button>
        </div>
        <div class="bt-card__body">
          <StateEmpty v-if="!allTunnels.length" title="暂无隧道" desc="点上方「新建隧道」或到平台控制台创建后同步" />
          <div v-else class="bt-table-wrap">
            <table class="bt-table">
              <caption class="bt-text-muted">隧道列表：状态、端点、用量与可执行操作</caption>
              <thead>
                <tr>
                  <th scope="col">状态</th>
                  <th scope="col">名称</th>
                  <th scope="col">平台</th>
                  <th scope="col">节点</th>
                  <th scope="col">本地</th>
                  <th scope="col">公网</th>
                  <th scope="col" class="num">连接数</th>
                  <th scope="col" class="num">今日流量</th>
                  <th scope="col">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="t in allTunnels" :key="t.id">
                  <td>
                    <span class="bt-tag" :class="tunnelStatus(t).cls">{{ tunnelStatus(t).text }}</span>
                    <div v-if="t.status_reason" class="bt-text-muted frp-cell-note">{{ t.status_reason }}</div>
                  </td>
                  <td>
                    <div class="frp-tunnel-name">
                      <span class="bt-tag bt-tag--outline">{{ protoLabel[t.proto] || t.proto }}</span>
                      <b>{{ t.name }}</b>
                      <span v-if="deployByTunnel[t.id]" class="bt-tag frp-tag-hosted"
                        :title="`由面板托管的客户端承载：${deployByTunnel[t.id].server_name} · ${deployByTunnel[t.id].container}`">
                        面板托管
                      </span>
                    </div>
                    <div v-if="t.extra" class="bt-text-muted frp-cell-note">{{ t.extra }}</div>
                    <div v-if="deployByTunnel[t.id]" class="bt-text-muted frp-cell-note">
                      {{ deployByTunnel[t.id].server_name }} · {{ deployByTunnel[t.id].status === 'running' ? '客户端运行中' : '客户端已停止' }}
                    </div>
                  </td>
                  <td>
                    <span class="bt-tag" :class="kindTagClass(t.platform_kind)">
                      {{ kindLabel[t.platform_kind] || t.platform_kind }}
                    </span>
                  </td>
                  <td>{{ t.node_name || t.node_id || '—' }}</td>
                  <td><span class="mono frp-endpoint">{{ t.local_ip }}:{{ t.local_port }}</span></td>
                  <td>
                    <span v-if="t.remote" class="mono frp-endpoint">{{ t.remote }}</span>
                    <span v-else class="bt-text-muted">—</span>
                  </td>
                  <td class="num tnum">
                    <!-- Sakura 平台 API 无连接数（doc/13 §1），显示面板节点侧实测值；都无则「—」 -->
                    <span v-if="(t.conns || 0) > 0" :title="`平台侧连接数 ${t.conns}`">{{ t.conns }}</span>
                    <span v-else-if="(t.local_conns || 0) > 0" title="面板在 frpc 所在节点实测的活跃连接数（Sakura 平台不提供连接数）">{{ t.local_conns }}</span>
                    <span v-else class="bt-text-muted">—</span>
                  </td>
                  <td class="num tnum">{{ fmtBytes((t.today_up || 0) + (t.today_down || 0)) }}</td>
                  <td>
                    <div class="frp-row-actions">
                      <!-- Cloudflare 独立专线（非面板统一容器）只读：编辑/删除禁用 -->
                      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        :disabled="isCfReadonly(t)" :title="isCfReadonly(t) ? '该节点属于独立专线（非面板创建），仅同步展示' : ''"
                        @click="openTunnelEdit(t)">编辑</button>
                      <button v-if="t.platform_kind !== 'cloudflared'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        :disabled="downloadingId === t.id" @click="downloadConfig(t)"
                        :title="t.platform_kind === 'natfrp'
                          ? '下载 frpc 配置（樱花分支 INI，配套 deploy/frpc-natfrp 镜像）'
                          : '下载 frpc 配置（INI，配套 deploy/frpc-chmlfrp 镜像）'">
                        {{ downloadingId === t.id ? '…' : '配置' }}
                      </button>
                      <button v-if="t.platform_kind === 'cloudflared'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        :disabled="isCfReadonly(t)" :title="isCfReadonly(t) ? '该节点属于独立专线（非面板创建），仅同步展示' : '把该平台的 cloudflared 客户端部署到节点上，由面板经 SSH 起容器'"
                        @click="openDeploy(t)">
                        托管
                      </button>
                      <button v-if="t.platform_kind !== 'cloudflared'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="openDeploy(t)"
                        title="把该平台的 frpc 客户端部署到节点上，由面板经 SSH 起容器并同步配置">
                        托管
                      </button>
                      <button v-if="deployByTunnel[t.id]" class="bt-btn bt-btn--ghost bt-btn--sm"
                        type="button" :disabled="deployByTunnel[t.id].status !== 'running'"
                        @click="askReleaseTunnel(t)"
                        title="让面板托管的客户端停止占用这条隧道，交给其它机器/官方客户端接管">
                        释放占用
                      </button>
                      <button v-if="t.platform_kind === 'natfrp'" class="bt-btn bt-btn--ghost bt-btn--sm"
                        type="button" @click="openLock(t)">锁定</button>
                      <button v-if="t.platform_kind === 'natfrp'" class="bt-btn bt-btn--ghost bt-btn--sm"
                        type="button" @click="openMigrate(t)">迁移</button>
                      <button v-if="t.platform_kind === 'natfrp'" class="bt-btn bt-btn--ghost bt-btn--sm"
                        type="button" @click="authTunnel(t)">认证</button>
                      <button v-if="t.platform_kind === 'chmlfrp'" class="bt-btn bt-btn--ghost bt-btn--sm"
                        type="button" @click="offlineTunnel(t)">下线</button>
                      <button class="bt-btn bt-btn--danger-ghost bt-btn--sm" type="button"
                        :disabled="isCfReadonly(t)" :title="isCfReadonly(t) ? '该节点属于独立专线（非面板创建），仅同步展示' : ''"
                        @click="askDeleteTunnel(t)">删除</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- ===== 客户端托管：面板经 SSH 在节点上拉镜像、起容器（doc/13 §14） ===== -->
      <div class="bt-card frp-block">
        <div class="bt-card__head">
          <h2 class="bt-card__title">
            客户端托管 <span class="frp-count">{{ deployments.length }}</span>
          </h2>
          <div class="frp-deploy-head">
            <span class="bt-text-muted frp-head-hint">面板经 SSH 在节点上拉起 frpc 容器 · 隧道变更后需重新同步</span>
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
              :disabled="deployLoading" @click="loadDeployments(true)">
              <AppIcon name="refresh" aria-hidden="true" />刷新状态
            </button>
            <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="openDeploy(null)">
              <AppIcon name="plus" aria-hidden="true" />部署客户端
            </button>
          </div>
        </div>
        <div class="bt-card__body">
          <StateSkeleton v-if="deployLoading && !deployments.length" :rows="2" />
          <StateEmpty v-else-if="!deployments.length" title="尚未托管客户端"
            desc="隧道建好后不必再手动登录节点跑 frpc：选一个平台与节点，面板会拉取镜像、生成配置并在节点上起容器，托管后可在隧道表「托管」列直接同步" />
          <div v-else class="bt-table-wrap">
            <table class="bt-table">
              <caption class="bt-text-muted">节点上的 frpc 容器：运行状态、承载隧道与操作</caption>
              <thead>
                <tr>
                  <th scope="col">状态</th>
                  <th scope="col">节点</th>
                  <th scope="col">平台</th>
                  <th scope="col">承载隧道</th>
                  <th scope="col">容器 / 镜像</th>
                  <th scope="col">上次同步</th>
                  <th scope="col">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="d in deployments" :key="d.id">
                  <td>
                    <span class="bt-tag" :class="deployStatusTag(d).cls">{{ deployStatusTag(d).text }}</span>
                    <div v-if="d.last_error" class="bt-text-muted frp-cell-note" :title="d.last_error">{{ d.last_error }}</div>
                  </td>
                  <td>
                    <b>{{ d.server_name || `#${d.server_id}` }}</b>
                    <div v-if="d.docker_ver" class="bt-text-muted frp-cell-note">Docker {{ d.docker_ver }}</div>
                  </td>
                  <td>
                    <span class="bt-tag" :class="kindTagClass(d.platform_kind)">
                      {{ kindLabel[d.platform_kind] || d.platform_kind }}
                    </span>
                  </td>
                  <td>
                    <div class="frp-deploy-tunnels">
                      <span v-for="t in d.tunnels" :key="t.id" class="bt-tag bt-tag--outline"
                        :class="{ 'is-missing': t.missing }">
                        {{ protoLabel[t.proto] || 'TCP' }} · {{ t.name }}
                      </span>
                    </div>
                  </td>
                  <td>
                    <span class="mono frp-endpoint">{{ d.container }}</span>
                    <div class="bt-text-muted frp-cell-note" :title="`${d.image} · ${d.config_path}`">
                      {{ d.image }} · {{ d.config_path }}
                    </div>
                  </td>
                  <td>{{ d.last_sync_at ? agoFromTs(d.last_sync_at) : '—' }}</td>
                  <td>
                    <div class="frp-row-actions">
                      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        :disabled="deployOpsId === d.id" @click="deploySync(d)"
                        title="重新拉取隧道配置、重建容器（隧道增删改后用这个）">
                        {{ deployOpsId === d.id ? '…' : '同步配置' }}
                      </button>
                      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        :disabled="deployOpsId === d.id" @click="deployAction(d, 'restart')">重启</button>
                      <button v-if="d.status === 'running'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        :disabled="deployOpsId === d.id" @click="deployAction(d, 'stop')"
                        title="停止容器：同时把该客户端承载的隧道让出来，可供其它机器/官方客户端接管">停止</button>
                      <button v-else class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        :disabled="deployOpsId === d.id" @click="deployAction(d, 'start')">启动</button>
                      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="openDeployLogs(d)">日志</button>
                      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        @click="openDeployEdit(d)" title="增删该客户端承载的隧道（节点与平台不可改）">改隧道</button>
                      <button class="bt-btn bt-btn--danger-ghost bt-btn--sm" type="button"
                        @click="askDeleteDeploy(d)">移除</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- ===== 合并节点表：在用展开（标平台），未在用折叠 ===== -->
      <div class="bt-card frp-block">
        <div class="bt-card__head">
          <h2 class="bt-card__title">
            在用节点 <span class="frp-count">{{ inUseNodes.length }}</span>
          </h2>
          <span class="bt-text-muted frp-head-hint">隧道所落节点 · 随「同步」更新</span>
        </div>
        <div class="bt-card__body">
          <StateSkeleton v-if="admin.frpDetailLoading && !allNodes.length" :rows="3" />
          <StateEmpty v-else-if="!allNodes.length" title="暂无节点数据" desc="点平台卡上的「同步」拉取节点列表" />
          <template v-else>
            <StateEmpty v-if="!inUseNodes.length" title="暂无在用节点"
              desc="隧道还没有落到任何节点上；新建隧道或完成一次「同步」后自动出现" />
            <div v-else class="bt-table-wrap">
              <table class="bt-table">
                <caption class="bt-text-muted">在用节点：归属平台、在线状态、负载与可用能力</caption>
                <thead>
                  <tr>
                    <th scope="col">状态</th>
                    <th scope="col">节点</th>
                    <th scope="col">平台</th>
                    <th scope="col">分组</th>
                    <th scope="col">能力</th>
                    <th scope="col" class="num">负载</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="n in inUseNodes" :key="`${nodePlatform(n.id)?.id}-${n.id}`">
                    <td>
                      <span class="bt-tag" :class="n.online ? 'bt-tag--success' : ''">{{ n.online ? '在线' : '离线' }}</span>
                    </td>
                    <td>
                      <div>{{ n.name }}</div>
                      <div v-if="n.host" class="bt-text-muted frp-cell-note mono">{{ n.host }}</div>
                    </td>
                    <td>
                      <span class="bt-tag" :class="kindTagClass(nodePlatform(n.id)?.kind)">
                        {{ kindLabel[nodePlatform(n.id)?.kind] || '—' }}
                      </span>
                    </td>
                    <td>{{ n.group_name || '—' }}</td>
                    <td>
                      <span v-for="c in capsText(n.caps)" :key="c" class="bt-tag bt-tag--outline frp-cap">{{ c }}</span>
                      <span v-if="!n.caps?.length" class="bt-text-muted">—</span>
                    </td>
                    <td class="num">
                      <div v-if="n.load" class="frp-load">
                        <span class="frp-load__num tnum">{{ n.load.toFixed(1) }}%</span>
                        <span class="frp-load__bar">
                          <i :class="loadTone(n.load)" :style="{ width: Math.min(100, n.load) + '%' }" />
                        </span>
                      </div>
                      <span v-else class="bt-text-muted">—</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <!-- 未在用节点折叠区：默认收起，只在迁移/建隧道选节点时才需要 -->
            <div v-if="idleNodes.length" class="frp-fold">
              <button class="frp-fold__toggle" type="button"
                :aria-expanded="idleExpanded" @click="idleExpanded = !idleExpanded">
                <AppIcon :name="idleExpanded ? 'arrow-down' : 'arrow-left'" aria-hidden="true" />
                未在用节点（{{ idleNodes.length }}）
                <span class="bt-text-muted">默认折叠，建隧道 / 迁移时在弹窗下拉中选择</span>
              </button>
              <div v-if="idleExpanded" class="bt-table-wrap frp-fold__body">
                <table class="bt-table">
                  <caption class="bt-text-muted">未在用节点：平台全网节点镜像（供建隧道与迁移参考）</caption>
                  <thead>
                    <tr>
                      <th scope="col">状态</th>
                      <th scope="col">节点</th>
                      <th scope="col">平台</th>
                      <th scope="col">分组</th>
                      <th scope="col">能力</th>
                      <th scope="col" class="num">负载</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="n in idleNodes" :key="`i-${nodePlatform(n.id)?.id}-${n.id}`">
                      <td>
                        <span class="bt-tag" :class="n.online ? 'bt-tag--success' : ''">{{ n.online ? '在线' : '离线' }}</span>
                      </td>
                      <td>
                        <div>{{ n.name }}</div>
                        <div v-if="n.host" class="bt-text-muted frp-cell-note mono">{{ n.host }}</div>
                      </td>
                      <td>
                        <span class="bt-tag" :class="kindTagClass(nodePlatform(n.id)?.kind)">
                          {{ kindLabel[nodePlatform(n.id)?.kind] || '—' }}
                        </span>
                      </td>
                      <td>{{ n.group_name || '—' }}</td>
                      <td>
                        <span v-for="c in capsText(n.caps)" :key="c" class="bt-tag bt-tag--outline frp-cap">{{ c }}</span>
                        <span v-if="!n.caps?.length" class="bt-text-muted">—</span>
                      </td>
                      <td class="num">
                        <div v-if="n.load" class="frp-load">
                          <span class="frp-load__num tnum">{{ n.load.toFixed(1) }}%</span>
                          <span class="frp-load__bar">
                            <i :class="loadTone(n.load)" :style="{ width: Math.min(100, n.load) + '%' }" />
                          </span>
                        </div>
                        <span v-else class="bt-text-muted">—</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- ===== 平台特有：ChmlFrp 免费二级域名 ===== -->
      <div v-if="chmlPlatform" class="bt-card frp-block">
        <div class="bt-card__head">
          <h2 class="bt-card__title">
            免费二级域名 <span class="frp-count">{{ subs.list.length }}</span>
            <span class="bt-tag frp-tag-chml">ChmlFrp</span>
          </h2>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="openSubCreate">
            <AppIcon name="plus" aria-hidden="true" />新建解析
          </button>
        </div>
        <div class="bt-card__body">
          <StateError v-if="subs.error" :message="subs.error" @retry="loadAllSubdomains" />
          <StateSkeleton v-else-if="subs.loading && !subs.list.length" :rows="2" />
          <StateEmpty v-else-if="!subs.list.length" title="暂无解析记录"
            desc="ChmlFrp 提供免费二级域名，可用于 HTTP(S) 隧道的绑定域名" />
          <div v-else class="bt-table-wrap">
            <table class="bt-table">
              <caption class="bt-text-muted">免费二级域名解析记录</caption>
              <thead>
                <tr>
                  <th scope="col">主机记录</th>
                  <th scope="col">主域名</th>
                  <th scope="col">类型</th>
                  <th scope="col">目标</th>
                  <th scope="col">TTL</th>
                  <th scope="col">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="sd in subs.list" :key="`${sd.record}.${sd.domain}`">
                  <td><span class="mono frp-endpoint">{{ sd.record }}</span></td>
                  <td>{{ sd.domain }}</td>
                  <td><span class="bt-tag bt-tag--outline">{{ sd.type }}</span></td>
                  <td><span class="mono frp-endpoint">{{ sd.target }}</span></td>
                  <td>{{ sd.ttl }}</td>
                  <td>
                    <div class="frp-row-actions">
                      <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button"
                        @click="openSubEdit(sd)">编辑</button>
                      <button class="bt-btn bt-btn--danger-ghost bt-btn--sm" type="button"
                        @click="askDeleteSubdomain(sd)">删除</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </template>

    <!-- ===== 弹窗：绑定 Sakura / Cloudflare ===== -->
    <div v-if="bindModal" class="bt-modal-mask" @click.self="bindModal.busy ? null : closeModal()">
      <div class="bt-modal" role="dialog" aria-modal="true"
        :aria-label="bindModal.kind === 'cloudflared' ? '绑定 Cloudflare 账号' : '绑定 Sakura 账号'">
        <div class="bt-modal__head">
          <div class="bt-modal__title frp-cf-title">
            <span>{{ bindModal.kind === 'cloudflared' ? '绑定 Cloudflare 账号' : '绑定 Sakura 账号' }}</span>
            <button v-if="bindModal.kind === 'cloudflared'" class="frp-help-dot" type="button"
              aria-label="如何获取 Cloudflare API Token" title="如何获取 API Token"
              @click="openCfHelp">?</button>
          </div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack">
            <template v-if="bindModal.kind === 'cloudflared'">
              <p class="bt-modal__desc">
                只需粘贴一个 <b>API Token</b>：Account ID 与 Zone ID 由面板自动发现。
                Token 将加密存储，面板只用于读取隧道状态、流量统计并代为管理 ingress。
                还没有 Token？点标题旁的 <b>?</b> 查看两分钟创建指引。
              </p>
              <label class="bt-field">
                <span class="bt-field__label">备注名称</span>
                <input v-model="bindModal.name" class="bt-input" type="text" placeholder="Cloudflare" autocomplete="off">
              </label>
              <label class="bt-field">
                <span class="bt-field__label">API Token <span class="bt-text-danger">*</span></span>
                <input v-model="bindModal.token" class="bt-input" type="password"
                  placeholder="粘贴 API Token（点「?」查看创建步骤）" autocomplete="off">
              </label>
            </template>
            <template v-else>
              <p class="bt-modal__desc">
                访问密钥在 <b>Sakura 面板 → 用户信息</b> 页查看（它不是登录密码）。密钥将加密存储，
                面板只用于读取隧道与节点、并代为调用管理接口。
              </p>
              <label class="bt-field">
                <span class="bt-field__label">备注名称</span>
                <input v-model="bindModal.name" class="bt-input" type="text" placeholder="Sakura" autocomplete="off">
              </label>
              <label class="bt-field">
                <span class="bt-field__label">访问密钥 <span class="bt-text-danger">*</span></span>
                <input v-model="bindModal.token" class="bt-input" type="password" placeholder="粘贴访问密钥"
                  autocomplete="off">
              </label>
            </template>
            <div v-if="bindModal.error" class="bt-alert bt-alert--error" role="alert">{{ bindModal.error }}</div>
          </div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="closeModal">取消</button>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="bindModal.busy" @click="submitBind">
            {{ bindModal.busy ? '验证中…' : '绑定并同步' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：Cloudflare API Token 获取指引 ===== -->
    <div v-if="cfHelpModal" class="bt-modal-mask" @click.self="cfHelpModal = false">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="Cloudflare API Token 获取指引">
        <div class="bt-modal__head">
          <div class="bt-modal__title">获取 Cloudflare API Token</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="cfHelpModal = false">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack frp-help-steps">
            <p class="bt-modal__desc">
              全程约两分钟，只需要做一次。Token 相当于面板访问你 Cloudflare 账号的钥匙，
              随时可以在同一页面撤销。Cloudflare 后台有中文/英文两种界面，下面按
              <b>中文界面（英文原名）</b>对照标注。
            </p>
            <ol class="frp-help-ol">
              <li>
                <b>打开 Token 创建页</b>：
                <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener noreferrer">dash.cloudflare.com/profile/api-tokens</a>
                （登录 Cloudflare 后右上角头像 →
                <b>我的个人资料 / My Profile</b> → <b>API 令牌 / API Tokens</b>）
              </li>
              <li>
                在 <b>创建自定义令牌 / Create Custom Token</b> 卡片点 <b>开始使用 / Get started</b>，
                令牌名称随便起（如 <code>beacontower</code>）。
                提示：也可以直接用 <b>编辑区域 DNS / Edit zone DNS</b> 模板，再补一条 Tunnel 权限，效果相同
              </li>
              <li>
                <b>权限（Permissions）加两条</b>：<br>
                · <code>帐户 / Account</code> → <code>Cloudflare Tunnel</code> → <code>编辑 / Edit</code><br>
                · <code>区域 / Zone</code> → <code>DNS</code> → <code>编辑 / Edit</code>
                （只看不建隧道的话，第二条可省）
              </li>
              <li>
                <b>帐户资源 / Account Resources</b> 选你的账号，
                <b>区域资源 / Zone Resources</b> 选 <b>所有区域 / All zones</b>
                （或指定你的域名所在 zone）→ <b>继续以显示摘要 / Continue to summary</b> →
                <b>创建令牌 / Create Token</b>
              </li>
              <li>
                页面会显示一长串 Token（<b>只显示这一次</b>），复制粘贴到绑定弹窗即可。
                Account ID / Zone ID 不用管——面板会自动发现。
              </li>
            </ol>
            <p class="bt-hint">
              找不到「Cloudflare Tunnel」权限项？在权限下拉的搜索框里直接输入
              <code>Cloudflare Tunnel</code> 或 <code>Cloudflare One</code>
              （该权限项在中文界面下可能仍显示英文名，属正常现象）。
            </p>
          </div>
        </div>
        <div class="bt-modal__foot">
          <a class="bt-btn bt-btn--default bt-btn--sm"
            href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener noreferrer">
            打开 Cloudflare Token 页
          </a>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" @click="cfHelpModal = false">我知道了</button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：ChmlFrp 设备码授权 ===== -->
    <div v-if="deviceModal" class="bt-modal-mask" @click.self="closeModal()">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="授权 ChmlFrp 账号">
        <div class="bt-modal__head">
          <div class="bt-modal__title">授权 ChmlFrp 账号</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack">
            <p class="bt-modal__desc">
              ChmlFrp 使用轻爪账户 OAuth2 登录，需你在浏览器确认一次。授权后令牌由面板自动续期
              （access_token 仅 10 分钟有效）；若续期失败，此页会提示重新授权。
            </p>
            <template v-if="deviceModal.status === 'starting'">
              <StateSkeleton :rows="2" />
            </template>
            <template v-else-if="deviceModal.status === 'pending'">
              <div class="bt-alert bt-alert--info" role="status">
                请在浏览器中打开下面的链接并确认授权（{{ Math.max(0, Math.ceil(deviceModal.secondsLeft)) }} 秒内有效）
              </div>
              <div class="bt-field">
                <span class="bt-field__label">用户码</span>
                <div class="frp-usercode">{{ deviceModal.userCode }}</div>
              </div>
              <a class="bt-btn bt-btn--primary bt-btn--block" :href="deviceModal.verifyUrl" target="_blank" rel="noopener noreferrer">
                打开授权页面
              </a>
              <p class="bt-text-muted">授权完成后本弹窗会自动关闭。</p>
            </template>
            <template v-else>
              <div class="bt-alert bt-alert--error" role="alert">{{ deviceModal.error || '授权失败' }}</div>
            </template>
          </div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：建/改隧道 ===== -->
    <div v-if="tunnelModal" class="bt-modal-mask" @click.self="tunnelModal.busy ? null : closeModal()">
      <div class="bt-modal bt-modal--lg" role="dialog" aria-modal="true"
        :aria-label="tunnelModal.mode === 'create' ? '新建隧道' : '编辑隧道'">
        <div class="bt-modal__head">
          <div class="bt-modal__title">
            {{ tunnelModal.mode === 'create' ? '新建隧道' : `编辑隧道 · ${tunnelModal.name}` }}
          </div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-grid">
            <label v-if="tunnelModal.mode === 'create'" class="bt-field">
              <span class="bt-field__label">平台 <span class="bt-text-danger">*</span></span>
              <select v-model="tunnelModal.platformId" class="bt-select" @change="onCreatePlatformChange">
                <option v-for="p in platforms" :key="p.id" :value="p.id">{{ p.name }}</option>
              </select>
            </label>
            <label class="bt-field">
              <span class="bt-field__label">
                {{ tunnelModalIsCf ? 'Ingress 域名' : '隧道名' }} <span class="bt-text-danger">*</span>
              </span>
              <input v-model="tunnelModal.name" class="bt-input" type="text" :placeholder="tunnelNamePlaceholder"
                autocomplete="off">
            </label>
            <label v-if="!tunnelModalIsCf" class="bt-field">
              <span class="bt-field__label">类型</span>
              <select v-model="tunnelModal.proto" class="bt-select" :disabled="tunnelModal.mode === 'edit'">
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="http">HTTP</option>
                <option value="https">HTTPS</option>
              </select>
            </label>
            <label v-if="!tunnelModalIsCf" class="bt-field">
              <span class="bt-field__label">节点 {{ tunnelModal.mode === 'create' ? '*' : '' }}</span>
              <select v-model="tunnelModal.nodeId" class="bt-select">
                <option value="">{{ tunnelModal.mode === 'edit' ? '（不修改）' : '请选择节点' }}</option>
                <option v-for="n in tunnelModal.nodes" :key="n.remote_id" :value="n.remote_id">
                  {{ n.name }}{{ n.online ? '' : '（离线）' }}
                </option>
              </select>
            </label>
            <label class="bt-field">
              <span class="bt-field__label">本地 IP</span>
              <input v-model="tunnelModal.localIp" class="bt-input" type="text" placeholder="127.0.0.1" autocomplete="off">
            </label>
            <label class="bt-field">
              <span class="bt-field__label">本地端口 <span class="bt-text-danger">*</span></span>
              <input v-model="tunnelModal.localPort" class="bt-input" type="number" min="1" max="65535" autocomplete="off">
            </label>
            <label v-if="tunnelModal.proto === 'tcp' || tunnelModal.proto === 'udp'" class="bt-field">
              <span class="bt-field__label">公网端口</span>
              <input v-model="tunnelModal.remotePort" class="bt-input" type="number" min="0" max="65535"
                :placeholder="tunnelModalIsChml ? '需在节点允许范围内' : '0 = 由平台分配'" autocomplete="off">
            </label>
            <label v-else-if="!tunnelModalIsCf" class="bt-field">
              <span class="bt-field__label">绑定域名 <span class="bt-text-danger">*</span></span>
              <input v-model="tunnelModal.domain" class="bt-input" type="text" placeholder="example.com" autocomplete="off">
            </label>
            <label v-if="tunnelModal.mode === 'create' && !tunnelModalIsCf" class="bt-field">
              <span class="bt-field__label">额外参数（frpc 原样透传）</span>
              <input v-model="tunnelModal.extra" class="bt-input" type="text" placeholder="如 auto_https = auto" autocomplete="off">
            </label>
            <label v-if="!tunnelModalIsCf" class="bt-field">
              <span class="bt-field__label">备注</span>
              <input v-model="tunnelModal.note" class="bt-input" type="text" autocomplete="off">
            </label>
          </div>
          <p class="bt-hint frp-modal-hint">
            说明：Sakura 的编辑只支持本地地址/端口与备注，改类型或节点需删除重建或使用「迁移」；
            ChmlFrp 官方标注 HTTP(S) 隧道暂不支持修改。
          </p>
          <p v-if="tunnelModalIsChml" class="bt-hint frp-modal-hint">
            ChmlFrp 规则：隧道名只能用字母、数字与下划线（不能带连字符或中文）；
            TCP/UDP 的公网端口必须填节点允许范围内的端口，填 0 平台不会代选
            （允许范围以 ChmlFrp 官网节点页为准）。
          </p>
          <p v-if="tunnelModalIsCf" class="bt-hint frp-modal-hint">
            Cloudflare 规则：一个 Tunnel = 一个统一容器（连接器），这里新建的是容器里的一条节点
            （ingress 路由），名称即对外域名（须已在 CF 托管）。所有面板创建的节点共用同一条
            统一容器隧道，托管时也只需要一个 cloudflared 容器；独立专线（如 New-api）不受影响。
            创建时自动写入 ingress + 追加 DNS CNAME（需 Zone ID）。
            本地 IP 留空/127.0.0.1 时按 172.17.0.1（Docker 网桥网关）处理，云上主机也可填内网 IP。
          </p>
          <div v-if="tunnelModal.error" class="bt-alert bt-alert--error" role="alert">{{ tunnelModal.error }}</div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="closeModal">取消</button>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="tunnelModal.busy" @click="submitTunnel">
            {{ tunnelModal.busy ? '提交中…' : '确定' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：迁移节点 ===== -->
    <div v-if="migrateModal" class="bt-modal-mask" @click.self="migrateModal.busy ? null : closeModal()">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="迁移隧道节点">
        <div class="bt-modal__head">
          <div class="bt-modal__title">迁移隧道 · {{ migrateModal.name }}</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack">
            <label class="bt-field">
              <span class="bt-field__label">目标节点</span>
              <select v-model="migrateModal.nodeId" class="bt-select">
                <option value="">请选择节点</option>
                <option v-for="n in migrateModal.nodes" :key="n.remote_id" :value="n.remote_id">
                  {{ n.name }}{{ n.online ? '' : '（离线）' }}{{ n.load ? ` · ${n.load.toFixed(0)}%` : '' }}
                </option>
              </select>
            </label>
            <p class="bt-hint">迁移会让隧道在目标节点重新建立，客户端需要重新获取配置。</p>
            <div v-if="migrateModal.error" class="bt-alert bt-alert--error" role="alert">{{ migrateModal.error }}</div>
          </div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="closeModal">取消</button>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="migrateModal.busy" @click="submitMigrate">
            {{ migrateModal.busy ? '迁移中…' : '迁移' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：锁定设置 ===== -->
    <div v-if="lockModal" class="bt-modal-mask" @click.self="lockModal.busy ? null : closeModal()">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="隧道锁定设置">
        <div class="bt-modal__head">
          <div class="bt-modal__title">锁定设置 · {{ lockModal.name }}</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack">
            <label class="bt-check">
              <input v-model="lockModal.edit" type="checkbox">
              <span>锁定编辑（禁止改本地地址/端口/备注）</span>
            </label>
            <label class="bt-check">
              <input v-model="lockModal.del" type="checkbox">
              <span>锁定删除</span>
            </label>
            <label class="bt-check">
              <input v-model="lockModal.migrate" type="checkbox">
              <span>锁定迁移</span>
            </label>
            <p class="bt-hint">锁定由平台侧生效：锁定后面板上的对应操作会被平台拒绝。</p>
            <div v-if="lockModal.error" class="bt-alert bt-alert--error" role="alert">{{ lockModal.error }}</div>
          </div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="closeModal">取消</button>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="lockModal.busy" @click="submitLock">
            {{ lockModal.busy ? '提交中…' : '保存' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：二级域名 ===== -->
    <div v-if="subModal" class="bt-modal-mask" @click.self="subModal.busy ? null : closeModal()">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="二级域名解析">
        <div class="bt-modal__head">
          <div class="bt-modal__title">{{ subModal.mode === 'create' ? '新建解析' : '编辑解析' }}</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-grid">
            <label class="bt-field">
              <span class="bt-field__label">主域名 <span class="bt-text-danger">*</span></span>
              <select v-model="subModal.domain" class="bt-select" :disabled="subModal.mode === 'edit'">
                <option v-for="d in subs.domains.map((x) => x.domain)" :key="d" :value="d">{{ d }}</option>
              </select>
            </label>
            <label class="bt-field">
              <span class="bt-field__label">主机记录 <span class="bt-text-danger">*</span></span>
              <input v-model="subModal.record" class="bt-input" type="text" :disabled="subModal.mode === 'edit'"
                placeholder="如 home" autocomplete="off">
            </label>
            <label class="bt-field">
              <span class="bt-field__label">类型</span>
              <select v-model="subModal.type" class="bt-select">
                <option value="A">A</option>
                <option value="AAAA">AAAA</option>
                <option value="CNAME">CNAME</option>
                <option value="SRV">SRV</option>
              </select>
            </label>
            <label class="bt-field">
              <span class="bt-field__label">目标地址 <span class="bt-text-danger">*</span></span>
              <input v-model="subModal.target" class="bt-input" type="text" autocomplete="off">
            </label>
            <label class="bt-field">
              <span class="bt-field__label">TTL</span>
              <select v-model="subModal.ttl" class="bt-select">
                <option v-for="t in subTTLs" :key="t" :value="t">{{ t }}</option>
              </select>
            </label>
            <label class="bt-field">
              <span class="bt-field__label">备注</span>
              <input v-model="subModal.remarks" class="bt-input" type="text" autocomplete="off">
            </label>
          </div>
          <div v-if="subModal.error" class="bt-alert bt-alert--error" role="alert">{{ subModal.error }}</div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="closeModal">取消</button>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="subModal.busy" @click="submitSub">
            {{ subModal.busy ? '提交中…' : '确定' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：部署/调整客户端托管 ===== -->
    <div v-if="deployModal" class="bt-modal-mask" @click.self="deployModal.busy ? null : closeModal()">
      <div class="bt-modal bt-modal--lg" role="dialog" aria-modal="true"
        :aria-label="deployModal.editId ? '调整托管隧道' : '部署穿透客户端'">
        <div class="bt-modal__head">
          <div class="bt-modal__title">{{ deployModal.editId ? '调整托管隧道' : '部署穿透客户端' }}</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-grid">
            <label class="bt-field">
              <span class="bt-field__label">平台 <span class="bt-text-danger">*</span></span>
              <select v-model="deployModal.platformId" class="bt-select"
                :disabled="deployModal.busy || deployModal.editId" @change="onDeployPlatformChange">
                <option v-for="p in platforms" :key="p.id" :value="p.id">{{ p.name }}</option>
              </select>
            </label>
            <label class="bt-field">
              <span class="bt-field__label">节点（跑 frpc 的服务器）<span class="bt-text-danger">*</span></span>
              <select v-model="deployModal.serverId" class="bt-select"
                :disabled="deployModal.busy || deployModal.editId" @change="deployModal.docker = null">
                <option value="">请选择节点</option>
                <option v-for="s in deployServers" :key="s.id" :value="String(s.id)"
                  :disabled="!deploySshReady(s) || deployTakenIds.includes(s.id)">
                  {{ s.name }}{{ deploySshReady(s) ? ` · ${s.ssh?.username}@${s.ssh?.host}` : '（未录 SSH，先到节点管理补凭据）' }}{{ deployTakenIds.includes(s.id) ? '（该平台已托管）' : '' }}
                </option>
              </select>
            </label>
          </div>

          <div class="frp-deploy-docker">
            <span class="bt-field__label">节点 Docker</span>
            <button class="bt-btn bt-btn--default bt-btn--sm" type="button"
              :disabled="deployModal.probing || deployModal.busy || !deployModal.serverId" @click="probeDocker">
              {{ deployModal.probing ? '检测中…' : '检测' }}
            </button>
            <span v-if="deployModal.docker" class="frp-deploy-docker__state">
              <span v-if="deployModal.docker.present" class="bt-tag bt-tag--success">
                已安装 {{ deployModal.docker.version }}
              </span>
              <span v-else class="bt-tag bt-tag--warning">未安装</span>
              <span v-if="deployModal.docker.daemon_ok === false" class="bt-text-danger">守护进程未运行，可能需要先启动 Docker</span>
              <span v-else-if="deployModal.docker.err" class="bt-text-muted">{{ deployModal.docker.err }}</span>
            </span>
            <span v-else class="bt-text-muted">部署前先检测：面板会 SSH 到节点确认 docker 可用</span>
          </div>
          <label v-if="deployModal.editId || !deployModal.docker || !deployModal.docker.present" class="frp-deploy-check">
            <input v-model="deployModal.installDocker" type="checkbox" :disabled="deployModal.busy">
            <span>节点未安装 Docker 时自动安装（按系统选 apt/dnf/apk/pacman，可能耗时数分钟）</span>
          </label>

          <div class="bt-field">
            <span class="bt-field__label">承载隧道 <span class="bt-text-danger">*</span>（勾选的隧道都由这台客户端跑）</span>
            <p v-if="!deployTunnelPool(deployModal.platformId).length" class="bt-hint frp-modal-hint">
              该平台下暂无隧道：先关掉本弹窗，在隧道表里「新建隧道」后再回来托管。
            </p>
            <div v-else class="frp-deploy-pool">
              <label v-for="t in deployTunnelPool(deployModal.platformId)" :key="t.id" class="frp-deploy-pool__item"
                :class="{ 'is-taken': deployTunnelTaken(t) }">
                <input v-model="deployModal.tunnelIds" type="checkbox" :value="t.id"
                  :disabled="deployModal.busy || deployTunnelTaken(t)">
                <span class="bt-tag bt-tag--outline">{{ protoLabel[t.proto] || t.proto }}</span>
                <b>{{ t.name }}</b>
                <span class="bt-text-muted mono">{{ t.local_ip }}:{{ t.local_port }} → {{ t.remote || '未分配' }}</span>
                <span v-if="deployTunnelTaken(t)" class="bt-text-muted frp-deploy-pool__note">
                  已被 {{ deployByTunnel[t.id].server_name }} 托管（一条隧道同时只能有一个客户端）
                </span>
              </label>
            </div>
          </div>

          <p class="bt-hint frp-modal-hint">
            说明：同一节点同一平台只能托管一份客户端（一份配置可跑多条隧道）；
            隧道增删改后到列表点「同步配置」即可下发，无需登录节点。
            {{ deployModal.editId ? '当前为调整已有托管：平台与节点不可改，换机器请先「移除」。' : '' }}
          </p>
          <p v-if="deployModal.busy" class="bt-hint frp-modal-hint frp-deploy-progress">
            正在节点上执行（拉镜像/写配置/起容器）… 已等待 {{ deployModal.elapsed }} 秒，首次拉镜像视网络而定
          </p>
          <div v-if="deployModal.error" class="bt-alert bt-alert--error" role="alert">{{ deployModal.error }}</div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" :disabled="deployModal.busy" @click="closeModal">取消</button>
          <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="deployModal.busy" @click="submitDeploy">
            {{ deployModal.busy ? '部署中…' : (deployModal.editId ? '保存并同步' : '部署') }}
          </button>
        </div>
      </div>
    </div>

    <!-- ===== 弹窗：托管客户端日志 ===== -->
    <div v-if="deployLogs" class="bt-modal-mask" @click.self="closeModal()">
      <div class="bt-modal bt-modal--lg" role="dialog" aria-modal="true" aria-label="客户端日志">
        <div class="bt-modal__head">
          <div class="bt-modal__title">客户端日志 · {{ deployLogs.title }}</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
        <div class="bt-modal__body">
          <StateSkeleton v-if="deployLogs.loading" :rows="4" />
          <div v-else-if="deployLogs.error" class="bt-alert bt-alert--error" role="alert">{{ deployLogs.error }}</div>
          <pre v-else class="frp-logs">{{ deployLogs.text || '（暂无输出）' }}</pre>
          <p class="bt-hint frp-modal-hint">
            最近 200 行 stdout/stderr；frpc 连上后通常会打印 start proxy success。
            配置里含平台 token，粘贴分享前请先脱敏。
          </p>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="closeModal">关闭</button>
        </div>
      </div>
    </div>

    <!-- ===== 危险操作确认 ===== -->
    <ConfirmDialog v-if="confirmState" :title="confirmState.title" :message="confirmState.message"
      :confirm-text="confirmState.confirmText" :danger="confirmState.danger" :busy="confirmState.busy"
      @cancel="confirmState.busy ? null : (confirmState = null)" @confirm="runConfirm" />
  </div>
</template>

<style scoped>
/* ================= 小节标题 ================= */

.frp-sec-title {
  margin: 0 0 10px 2px;
  font-size: var(--bt-font-xs);
  font-weight: var(--bt-weight-semibold);
  letter-spacing: 0.08em;
  color: var(--bt-text-4);
}

/* ================= 平台卡（纯展示 + 轻操作） ================= */

.frp-platforms {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(420px, 1fr));
  gap: 12px;
  margin-bottom: 20px;
  padding: 0 4px; /* 让卡片投影不被网格裁切 */
}

.frp-platform {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 16px;
  border-radius: var(--bt-radius-lg);
  background: var(--bt-bg-card);
  backdrop-filter: blur(18px) saturate(160%);
  -webkit-backdrop-filter: blur(18px) saturate(160%);
  border: 1px solid var(--bt-glass-border);
  box-shadow:
    var(--bt-glass-hi),
    var(--bt-shadow-card);
  min-width: 0;
  transition:
    box-shadow var(--bt-duration-base) var(--bt-ease-out),
    border-color var(--bt-duration-base) ease;
}

.frp-platform.is-degraded {
  border-color: rgba(245, 158, 11, 0.4);
}

.frp-platform__avatar {
  width: 42px;
  height: 42px;
  border-radius: 13px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
  color: #fff;
  background: var(--bt-brand-gradient);
  box-shadow: 0 4px 12px -4px rgba(14, 165, 233, 0.5);
}

.frp-platform__avatar svg {
  width: 20px;
  height: 20px;
}

.frp-platform__avatar--chmlfrp {
  background: linear-gradient(135deg, #a78bfa, #8b5cf6);
  box-shadow: 0 4px 12px -4px rgba(139, 92, 246, 0.5);
}

.frp-platform__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.frp-platform__head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.frp-platform__name {
  font-weight: var(--bt-weight-semibold);
  font-size: var(--bt-font-lg);
  letter-spacing: -0.01em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.frp-platform__stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-top: 2px;
}

.frp-stat__num {
  display: block;
  font-size: var(--bt-font-md);
  font-weight: var(--bt-weight-semibold);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.frp-stat__num i {
  font-style: normal;
  color: var(--bt-text-4);
  margin: 0 1px;
}

.frp-stat__label {
  display: block;
  font-size: 11px;
  color: var(--bt-text-4);
  margin-top: 1px;
}

.frp-quota {
  display: block;
  height: 5px;
  border-radius: 3px;
  background: rgba(128, 128, 128, 0.18);
  overflow: hidden;
}

.frp-quota__bar {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--bt-success-500);
}

.frp-quota__bar.is-warn {
  background: var(--bt-warning-500);
}

.frp-quota__bar.is-full {
  background: var(--bt-danger-500);
}

.frp-platform__err {
  font-size: 12px;
  color: var(--bt-danger-600);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.frp-platform__foot {
  font-size: 11px;
  color: var(--bt-text-4);
}

.frp-platform__side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  flex-shrink: 0;
}

/* ================= 内容卡片节奏 ================= */

.frp-block {
  margin-bottom: 16px;
}

.frp-count {
  display: inline-grid;
  place-items: center;
  min-width: 22px;
  height: 20px;
  padding: 0 7px;
  margin-left: 4px;
  border-radius: var(--bt-radius-pill);
  font-size: var(--bt-font-xs);
  font-weight: var(--bt-weight-medium);
  vertical-align: 2px;
  color: var(--bt-brand-600);
  background: var(--bt-brand-50);
  border: 1px solid rgba(14, 165, 233, 0.18);
}

.frp-head-hint {
  font-size: var(--bt-font-xs);
}

/* 平台归属标签：与 outline 系并列的双色区分（颜色只做辅助，文字才是主通道） */
.frp-tag-nat {
  background: var(--bt-brand-100);
  color: var(--bt-info-600);
}

/* 离线：比裸 bt-tag 再实一点，别让「离线」看着像没有状态 */
.frp-tag-offline {
  background: rgba(120, 140, 190, 0.18);
  color: var(--bt-text-2);
}

/* 面板托管标记：隧道走的是面板部署的客户端（与平台侧客户端区分） */
.frp-tag-hosted {
  background: rgba(16, 185, 129, 0.14);
  color: var(--bt-success-600);
}

.frp-tag-chml {
  background: rgba(167, 139, 250, 0.16);
  color: var(--bt-accent-600);
}

.frp-tag-cf {
  background: rgba(248, 115, 22, 0.16);
  color: #ea6a1a;
}

/* 绑定弹窗标题旁的「?」帮助按钮与指引弹窗 */
.frp-cf-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.frp-help-dot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: 1.5px solid var(--bt-border-strong);
  background: var(--bt-bg-subtle);
  color: var(--bt-text-3);
  font-size: 11px;
  font-weight: var(--bt-weight-semibold);
  line-height: 1;
  cursor: pointer;
  transition: color 0.15s ease, border-color 0.15s ease;
}

.frp-help-dot:hover {
  color: var(--bt-brand-600);
  border-color: var(--bt-brand-500);
}

.frp-help-ol {
  margin: 0;
  padding-left: 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--bt-text-2);
}

.frp-help-ol a {
  color: var(--bt-brand-600);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.frp-help-ol code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--bt-bg-subtle);
  border: 1px solid var(--bt-border);
  font-size: 12px;
}

/* ================= 归一化用量瓦片 ================= */

.frp-tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 10px;
}

.frp-tile {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  border-radius: var(--bt-radius-md);
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid var(--bt-border);
  min-width: 0;
  transition:
    border-color var(--bt-duration-fast) ease,
    background var(--bt-duration-fast) ease;
}

.frp-tile:hover {
  border-color: rgba(56, 189, 248, 0.35);
  background: rgba(255, 255, 255, 0.68);
}

.frp-tile__icon {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
  color: var(--bt-brand-600);
  background: var(--bt-brand-50);
  border: 1px solid rgba(14, 165, 233, 0.18);
}

.frp-tile__icon svg {
  width: 16px;
  height: 16px;
}

.frp-tile__icon--violet {
  color: #7c3aed;
  background: rgba(167, 139, 250, 0.14);
  border-color: rgba(139, 92, 246, 0.2);
}

.frp-tile__icon--cf {
  color: #ea6a1a;
  background: rgba(248, 115, 22, 0.12);
  border-color: rgba(248, 115, 22, 0.2);
}

.frp-tile__icon--brand {
  color: var(--bt-brand-600);
  background: var(--bt-brand-50);
  border-color: rgba(14, 165, 233, 0.18);
}

/* 平台块内四行指标的键值对齐 */
.frp-tile__grid {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.frp-tile__row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.frp-tile__label {
  font-size: var(--bt-font-xs);
  color: var(--bt-text-4);
  white-space: nowrap;
}

.frp-tile__value {
  font-size: var(--bt-font-md);
  font-weight: var(--bt-weight-semibold);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ================= 表格微调 ================= */

.frp-tunnel-name {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.frp-tunnel-name .bt-tag {
  font-size: 10px;
  padding: 1px 7px;
}

.frp-endpoint {
  font-size: var(--bt-font-xs);
  letter-spacing: 0.01em;
  white-space: nowrap;
}

/* 数字/地址列不许折行：端口与 IP 从中间断开（3687 / 8）比窄一点更难读 */
.frp-block .bt-table td {
  white-space: nowrap;
}

.frp-block .bt-table td .frp-deploy-tunnels,
.frp-block .bt-table td .frp-tunnel-name {
  white-space: normal;
}

.frp-cell-note {
  font-size: 11px;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.frp-row-actions {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

/* ================= 客户端托管 ================= */

.frp-deploy-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.frp-deploy-tunnels {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-width: 240px;
}

.frp-deploy-tunnels .bt-tag.is-missing {
  color: var(--bt-danger-600);
  text-decoration: line-through;
}

.frp-deploy-docker {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin: 10px 0 2px;
}

.frp-deploy-docker__state {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: var(--bt-font-xs);
}

.frp-deploy-check {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
  font-size: var(--bt-font-sm);
  color: var(--bt-text-2);
  cursor: pointer;
}

.frp-deploy-pool {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 240px;
  overflow: auto;
  padding: 6px 10px;
  border: var(--bt-hairline);
  border-radius: var(--bt-radius-md);
  background: var(--bt-bg-subtle);
}

.frp-deploy-pool__item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 0;
  font-size: var(--bt-font-sm);
  cursor: pointer;
  border-bottom: var(--bt-hairline);
}

.frp-deploy-pool__item:last-child {
  border-bottom: 0;
}

.frp-deploy-pool__item .mono {
  font-size: var(--bt-font-xs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 已被其它托管占用的隧道：置灰并说明原因（同一条隧道只能有一个客户端） */
.frp-deploy-pool__item.is-taken {
  opacity: 0.72;
  cursor: not-allowed;
}

.frp-deploy-pool__note {
  font-size: var(--bt-font-xs);
}

.frp-deploy-progress {
  color: var(--bt-info-600);
}

.frp-logs {
  margin: 0;
  max-height: 46vh;
  overflow: auto;
  padding: 10px 12px;
  background: rgba(128, 128, 128, 0.1);
  border-radius: var(--bt-radius-xs);
  font-family: var(--bt-font-mono);
  font-size: var(--bt-font-sm);
  line-height: var(--bt-line-md);
  white-space: pre-wrap;
  word-break: break-all;
}

.frp-cap {
  margin-right: 4px;
}

.frp-load {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.frp-load__num {
  font-size: var(--bt-font-xs);
  font-weight: var(--bt-weight-medium);
  min-width: 44px;
  text-align: right;
}

.frp-load__bar {
  width: 64px;
  height: 5px;
  border-radius: 3px;
  background: rgba(128, 128, 128, 0.18);
  overflow: hidden;
}

.frp-load__bar i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--bt-success-500);
}

.frp-load__bar i.is-warn {
  background: var(--bt-warning-500);
}

.frp-load__bar i.is-full {
  background: var(--bt-danger-500);
}

/* ================= 未在用节点折叠区 ================= */

.frp-fold {
  margin-top: 12px;
}

/* 手风琴按钮：文字 + 箭头方向双通道，虚线描边弱化视觉权重 */
.frp-fold__toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1.5px dashed var(--bt-border-strong);
  border-radius: var(--bt-radius-md);
  background: transparent;
  font: inherit;
  font-size: var(--bt-font-sm);
  font-weight: var(--bt-weight-medium);
  color: var(--bt-text-2);
  cursor: pointer;
  transition:
    border-color var(--bt-duration-fast) ease,
    background var(--bt-duration-fast) ease,
    color var(--bt-duration-fast) ease;
}

.frp-fold__toggle:hover {
  border-color: rgba(56, 189, 248, 0.45);
  background: rgba(255, 255, 255, 0.6);
  color: var(--bt-text-1);
}

.frp-fold__toggle svg {
  width: 14px;
  height: 14px;
  color: var(--bt-brand-500);
}

.frp-fold__body {
  margin-top: 10px;
}

/* ================= 弹窗微调 ================= */

.frp-usercode {
  font-family: var(--bt-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 20px;
  font-weight: var(--bt-weight-semibold);
  letter-spacing: 0.1em;
}

/* 表单说明与字段拉开距离：bt-modal__body 无 gap，网格表单贴着 hint 太挤 */
.frp-modal-hint {
  margin-top: var(--bt-space-4);
  padding-top: var(--bt-space-3);
  border-top: var(--bt-hairline);
}

/* ================= 窄屏 ================= */

@media (max-width: 720px) {
  .frp-platforms {
    grid-template-columns: 1fr;
  }

  .frp-platform__side {
    flex-direction: row;
    align-items: center;
    width: 100%;
    justify-content: flex-end;
  }

  .frp-platform__stats {
    grid-template-columns: repeat(2, 1fr);
  }

  .frp-load__bar {
    width: 44px;
  }
}
</style>
