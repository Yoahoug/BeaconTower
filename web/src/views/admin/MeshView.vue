<!-- ============================================================
     WG 组网管理（doc/12）：单网 + 主备 hub（A/B 流量额度轮换）。
     - 总览：网络信息条 / hub 额度卡 / 拓扑图 / 成员表
     - 操作：组网向导（预检→执行）、导入现有网络、一键切换 hub、
             设备凭证（conf + 二维码）、手动巡检
     - 任务进度：wg_task/step 2s 轮询至完结
     ============================================================ -->
<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { GraphChart } from 'echarts/charts'
import { CanvasRenderer } from 'echarts/renderers'
import QRCode from 'qrcode'
import AppIcon from '../../components/AppIcon.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import StateError from '../../components/ui/StateError.vue'
import StateSkeleton from '../../components/ui/StateSkeleton.vue'
import { agoFromTs } from '../../api/auth'
import { fmtBytes } from '../../utils/format'
import { useAdminStore } from '../../stores/admin'
import { useUiStore } from '../../stores/ui'

echarts.use([GraphChart, CanvasRenderer])

const admin = useAdminStore()
const ui = useUiStore()

const overview = computed(() => admin.wgOverview)
const network = computed(() => admin.wgOverview?.network || null)
const hubs = computed(() => admin.wgOverview?.hubs || [])
const peers = computed(() => admin.wgOverview?.peers || [])
const servers = computed(() => admin.wgOverview?.servers || [])
const activeHub = computed(() => hubs.value.find((h) => h.is_active) || null)
const standbyHubs = computed(() => hubs.value.filter((h) => !h.is_active))
// 中心区排序：现役永远排第一张卡（备援在后，未纳管垫底）
const orderedHubs = computed(() => [...hubs.value].sort((a, b) => Number(b.is_active) - Number(a.is_active)))
// 网外节点（未纳管，本机除外）：中心区第三段，带「准备为备援」入口
const unmanagedServers = computed(() => servers.value.filter((s) => !s.is_self && !s.in_network))
const runningTask = computed(() => !!admin.wgOverview?.running_task)
const serverPeers = computed(() => peers.value.filter((p) => p.kind === 'server'))
const devicePeers = computed(() => peers.value.filter((p) => p.kind === 'device'))

// ---------- 文案与格式 ----------
function statusTag(s) {
  switch (s) {
    case 'online':
      return { text: '在线', cls: 'bt-tag--success' }
    case 'offline':
      return { text: '离线', cls: 'bt-tag--warning' }
    case 'pending':
      return { text: '待接入', cls: '' }
    case 'joining':
      return { text: '接入中', cls: 'bt-tag--info' }
    case 'error':
      return { text: '异常', cls: 'bt-tag--danger' }
    default:
      return { text: s || '—', cls: '' }
  }
}

// ---------- 拓扑图 ----------
const topoEl = ref(null)
let topoChart = null

function buildTopoOption() {
  if (!network.value) return null
  const nodes = []
  const links = []
  const cat = (s) => (s === 'online' ? '在线' : s === 'offline' ? '离线' : '待接入')
  for (const h of hubs.value) {
    nodes.push({
      id: `hub-${h.server_id}`,
      name: `${h.is_active ? '★ ' : ''}${h.name || `节点${h.server_id}`}`,
      symbolSize: 54,
      category: h.is_active ? '现役中心' : '备援中心',
      itemStyle: { color: h.is_active ? '#0EA5E9' : '#64748B' },
      label: { show: true, formatter: `${h.name}\n:${h.listen_port}` },
      tooltip: { formatter: `${h.endpoint || '端点未知'} · ${cat(h.status)}` },
    })
  }
  for (const p of peers.value) {
    if (p.status === 'left') continue
    const color = p.status === 'online' ? '#10B981' : p.status === 'offline' ? '#F59E0B' : '#64748B'
    nodes.push({
      id: `peer-${p.id}`,
      name: p.name,
      symbolSize: p.kind === 'device' ? 30 : 38,
      category: cat(p.status),
      itemStyle: { color },
      label: { show: true, formatter: `${p.name}\n${p.wg_ip}` },
      tooltip: { formatter: `${p.wg_ip} · ${cat(p.status)} · ↓${fmtBytes(p.rx_bytes)} ↑${fmtBytes(p.tx_bytes)}` },
    })
    if (activeHub.value) {
      links.push({
        source: `hub-${activeHub.value.server_id}`,
        target: `peer-${p.id}`,
        lineStyle: { color, width: p.status === 'online' ? 2.4 : 1.2, type: p.status === 'online' ? 'solid' : 'dashed' },
      })
    }
  }
  return {
    tooltip: {},
    legend: { data: ['现役中心', '备援中心', '在线', '离线', '待接入'], bottom: 0, textStyle: { fontSize: 11 } },
    series: [{
      type: 'graph',
      layout: 'force',
      // 仅拖拽平移：滚轮/触摸滚动留给页面，避免上下滑动时被图表劫持；触屏设备干脆禁用
      roam: window.matchMedia('(pointer: coarse)').matches ? false : 'move',
      force: { repulsion: 320, edgeLength: 110 },
      label: { position: 'bottom', fontSize: 11 },
      edgeSymbol: ['none', 'arrow'],
      edgeSymbolSize: 8,
      data: nodes,
      links,
    }],
  }
}

function renderTopo() {
  if (!topoEl.value) return
  if (!topoChart) {
    topoChart = echarts.init(topoEl.value)
  }
  const opt = buildTopoOption()
  if (opt) topoChart.setOption(opt, true)
}

function resizeTopo() {
  topoChart?.resize()
}

// ---------- 任务进度（2s 轮询） ----------
const taskModal = ref(null) // { id, task, error }
let pollTimer = 0

function stopPoll() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = 0
  }
}

function watchTask(id) {
  stopPoll()
  taskModal.value = { id, task: null, error: '' }
  rememberFocus()
  const tick = async () => {
    if (!taskModal.value) { stopPoll(); return } // 弹窗已关闭：停止轮询
    try {
      const t = await admin.loadWgTask(id)
      if (!taskModal.value) return
      taskModal.value.task = t
      if (t && t.status !== 'running') {
        stopPoll()
        await admin.loadWg()
        ui.notify(t.status === 'done' ? '任务完成' : `任务结束：${t.status} · ${t.result || ''}`)
      }
    } catch (e) {
      if (taskModal.value) taskModal.value.error = e?.message || '任务状态获取失败'
    }
  }
  tick()
  pollTimer = window.setInterval(tick, 2000)
}

function closeTaskModal() {
  taskModal.value = null
  stopPoll()
  restoreFocus()
}

// ---------- 弹窗焦点管理（doc/10 §8：ESC 关闭 / 打开聚焦 / 关闭归还焦点） ----------
let lastFocusEl = null

function rememberFocus() {
  lastFocusEl = document.activeElement
  nextTick(() => {
    const modal = document.querySelector('.bt-modal')
    modal?.querySelector('button, input, select, textarea')?.focus()
  })
}

function restoreFocus() {
  if (lastFocusEl && document.contains(lastFocusEl)) lastFocusEl.focus?.()
  lastFocusEl = null
}

// ESC 关闭最上层弹窗（busy 中的操作弹窗不关，防误触）
function onMeshKey(e) {
  if (e.key !== 'Escape') return
  if (taskModal.value) { closeTaskModal(); return }
  if (confModal.value) { confModal.value = null; restoreFocus(); return }
  if (assetModal.value) { if (!assetModal.value.busy) { assetModal.value = null; restoreFocus() } return }
  if (deviceModal.value) { if (!deviceModal.value.busy) { deviceModal.value = null; restoreFocus() } return }
  if (switchModal.value) { if (!switchModal.value.busy) { switchModal.value = null; restoreFocus() } return }
  if (importModal.value) { if (!importModal.value.busy) { importModal.value = null; restoreFocus() } return }
  if (wizard.value && !wizard.value.busy) { wizard.value = null; restoreFocus() }
}

onBeforeUnmount(() => {
  stopPoll()
  topoChart?.dispose()
  topoChart = null
  window.removeEventListener('resize', resizeTopo)
  window.removeEventListener('keydown', onMeshKey)
})

// ---------- 组网向导 ----------
const wizard = ref(null)
// { step: 1|2|3, plan, error, busy, hubId, hubPort, ids:[], subnet, hubIp }
const selectedIds = ref([])
const wizardHubId = ref(0)
const wizardHubPort = ref(51820)
const wizardSubnet = ref('10.66.66.0/24')
const wizardHubIp = ref('10.66.66.2')

const selectableServers = computed(() => (servers.value || []).filter((s) => !s.is_self))

function openWizard() {
  selectedIds.value = []
  wizardHubId.value = activeHub.value?.server_id || 0
  wizard.value = { step: 1, plan: null, error: '', busy: false }
  rememberFocus()
}

function togglePick(id) {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}

function wizardPayload() {
  const spokes = selectedIds.value.filter((id) => id !== wizardHubId.value).map((id) => ({ server_id: id }))
  const p = { hub_server_id: wizardHubId.value || undefined, hub_port: wizardHubPort.value, spokes }
  if (!network.value) {
    p.network = { subnet: wizardSubnet.value, hub_ip: wizardHubIp.value, keepalive: 25, mtu: 1420 }
  }
  return p
}

async function runPlan() {
  if (!wizardHubId.value) {
    wizard.value.error = '请选择中心节点（须为公网可达的节点）'
    return
  }
  if (!selectedIds.value.length) {
    wizard.value.error = '请至少勾选一台要接入的节点'
    return
  }
  wizard.value.busy = true
  wizard.value.error = ''
  try {
    const plan = await admin.planWg(wizardPayload())
    if (!wizard.value) return // 请求期间弹窗已关闭
    wizard.value.plan = plan
    wizard.value.step = 2
  } catch (e) {
    if (wizard.value) wizard.value.error = e?.message || '预检失败'
  } finally {
    if (wizard.value) wizard.value.busy = false
  }
}

async function runApply() {
  wizard.value.busy = true
  wizard.value.error = ''
  try {
    const r = await admin.applyWg(wizardPayload())
    wizard.value = null
    restoreFocus()
    ui.notify('组网任务已启动')
    watchTask(r.task_id)
  } catch (e) {
    if (wizard.value) wizard.value.error = e?.message || '执行失败'
  } finally {
    if (wizard.value) wizard.value.busy = false
  }
}

// ---------- 导入现有网络 ----------
const importModal = ref(null)
// { hubId, standbyId, picked:[], busy, error }
const importHubId = ref(0)
const importStandbyId = ref(0)
const importPicked = ref([])

// 可勾选参与匹配的节点：排除已选 hub 与备援
const importPickPool = computed(() =>
  selectableServers.value.filter((x) => x.id !== importHubId.value && x.id !== importStandbyId.value),
)

function openImport() {
  importHubId.value = 0
  importStandbyId.value = 0
  importPicked.value = []
  importModal.value = { busy: false, error: '' }
  rememberFocus()
}

async function runImport() {
  if (!importHubId.value) {
    importModal.value.error = '请选择现役中心节点'
    return
  }
  importModal.value.busy = true
  importModal.value.error = ''
  try {
    const r = await admin.importWg({
      hub_server_id: importHubId.value,
      standby_server_id: importStandbyId.value || undefined,
      candidates: importPicked.value,
    })
    importModal.value = null
    restoreFocus()
    ui.notify('导入任务已启动')
    watchTask(r.task_id)
  } catch (e) {
    if (importModal.value) importModal.value.error = e?.message || '导入失败'
  } finally {
    if (importModal.value) importModal.value.busy = false
  }
}

// ---------- 未纳管节点登记为备援 ----------
// { [serverId]: { busy, error } }：错误就地展示（常见：节点上没有 /etc/wireguard/wg0.conf）
const standbyReg = ref({})

async function registerStandby(s) {
  standbyReg.value = { ...standbyReg.value, [s.id]: { busy: true, error: '' } }
  try {
    const r = await admin.registerStandby(s.id)
    ui.notify(`${s.name} 已登记为备援中心（${r.endpoint || '端点未知'}）`)
  } catch (e) {
    if (standbyReg.value[s.id]) {
      standbyReg.value = { ...standbyReg.value, [s.id]: { busy: false, error: e?.message || '登记备援失败' } }
    }
    return
  }
  standbyReg.value = { ...standbyReg.value, [s.id]: { busy: false, error: '' } }
}

// ---------- hub 切换 ----------
const switchModal = ref(null)
const switchTargetId = ref(0)

const switchCandidates = computed(() => hubs.value.filter((h) => !h.is_active))

function openSwitch(hub) {
  if (!switchCandidates.value.length) {
    ui.notify('暂无备援中心：先在中心区把网外节点登记为备援')
    return
  }
  switchTargetId.value = hub?.server_id || switchCandidates.value[0].server_id
  switchModal.value = { busy: false, error: '' }
  rememberFocus()
}

async function runSwitch() {
  switchModal.value.busy = true
  switchModal.value.error = ''
  try {
    const r = await admin.switchHub({ target_server_id: switchTargetId.value })
    switchModal.value = null
    restoreFocus()
    ui.notify('切换任务已启动（金丝雀验证通过后才会全网切换）')
    watchTask(r.task_id)
  } catch (e) {
    if (switchModal.value) switchModal.value.error = e?.message || '切换失败'
  } finally {
    if (switchModal.value) switchModal.value.busy = false
  }
}

// ---------- 设备凭证 ----------
const deviceModal = ref(null) // { name, busy, error }
const confModal = ref(null) // { peer, hubId, conf, filename, dataUrl, error }

function openDevice() {
  deviceModal.value = { name: '', busy: false, error: '' }
  rememberFocus()
}

async function createDevice() {
  const name = (deviceModal.value.name || '').trim()
  if (!name) {
    deviceModal.value.error = '请输入设备名称'
    return
  }
  deviceModal.value.busy = true
  deviceModal.value.error = ''
  try {
    const r = await admin.createDevice(name)
    deviceModal.value = null
    restoreFocus()
    ui.notify(r.warn ? `设备已创建，但 ${r.warn}` : `设备已创建（${r.wg_ip}）`)
  } catch (e) {
    if (deviceModal.value) deviceModal.value.error = e?.message || '创建失败'
  } finally {
    if (deviceModal.value) deviceModal.value.busy = false
  }
}

let confSeq = 0

async function openConf(peer, hubId = 0) {
  const seq = ++confSeq
  confModal.value = { peer, hubId, conf: '', filename: '', dataUrl: '', error: '', loading: true }
  rememberFocus()
  try {
    const r = await admin.peerConf(peer.id, hubId)
    if (seq !== confSeq || !confModal.value) return // 已关闭或已切到其他 hub（A/B 竞态，丢弃旧响应）
    confModal.value.conf = r.conf
    confModal.value.filename = r.filename
    const url = await QRCode.toDataURL(r.conf, { width: 320, margin: 1 })
    if (seq !== confSeq || !confModal.value) return
    confModal.value.dataUrl = url
  } catch (e) {
    if (seq !== confSeq || !confModal.value) return
    confModal.value.error = e?.message || '配置导出失败'
  } finally {
    if (confModal.value && seq === confSeq) confModal.value.loading = false
  }
}

function downloadConf() {
  if (!confModal.value?.conf) return
  const blob = new Blob([confModal.value.conf], { type: 'text/plain' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = confModal.value.filename || 'wg0.conf'
  a.click()
  URL.revokeObjectURL(a.href)
}

// ---------- 成员操作 ----------
const confirmDelete = ref(null)
const deleting = ref(false)
const verifyingId = ref(0)

async function doDelete() {
  if (!confirmDelete.value) return
  deleting.value = true
  try {
    const r = await admin.deletePeer(confirmDelete.value.id)
    ui.notify(`已移出网络 · ${r.detail || ''}`)
    confirmDelete.value = null
  } catch (e) {
    ui.notify(e?.message || '移除失败')
  } finally {
    deleting.value = false
  }
}

async function verifyPeer(p) {
  verifyingId.value = p.id
  try {
    const r = await admin.verifyPeer(p.id)
    ui.notify(r.online ? `${p.name} 在线：${r.detail}` : `${p.name} 未连通：${r.detail}`)
  } catch (e) {
    ui.notify(e?.message || '验证失败')
  } finally {
    verifyingId.value = 0
  }
}

const patrolling = ref(false)

async function patrol() {
  if (patrolling.value) return
  patrolling.value = true
  try {
    await admin.patrolNow()
    await admin.loadWg()
    ui.notify('巡检完成')
  } catch (e) {
    ui.notify(e?.message || '巡检失败')
  } finally {
    patrolling.value = false
  }
}

// ---------- 资产中转 ----------
const assetModal = ref(null)
// { list, form:{name,version,sha256,sources_text,note}, probe:{id, results}, busy, error }
const assetServerId = ref(0)

async function openAssets() {
  assetModal.value = { form: null, probe: null, busy: false, error: '' }
  rememberFocus()
  try {
    await admin.loadAssets()
  } catch (e) {
    // 加载期间用户可能已关闭弹窗（assetModal=null）→ 直接取值会抛 TypeError
    if (assetModal.value) assetModal.value.error = e?.message || '资产列表加载失败'
  }
  if (serverPeers.value.length && !assetServerId.value) {
    assetServerId.value = serverPeers.value[0].server_id
  }
}

function assetRegisterForm() {
  assetModal.value.form = { name: '', version: '', sha256: '', sources_text: '', note: '' }
}

async function saveAsset() {
  const f = assetModal.value.form
  const sources = (f.sources_text || '').split('\n').map((x) => x.trim()).filter(Boolean)
  if (!f.name || !sources.length) {
    assetModal.value.error = '需要资产名与至少一个下载源'
    return
  }
  assetModal.value.busy = true
  assetModal.value.error = ''
  try {
    await admin.upsertAsset({ name: f.name, version: f.version, sha256: f.sha256, sources, note: f.note })
    if (assetModal.value) assetModal.value.form = null
    ui.notify('资产已保存')
  } catch (e) {
    if (assetModal.value) assetModal.value.error = e?.message || '保存失败'
  } finally {
    if (assetModal.value) assetModal.value.busy = false
  }
}

async function probeAsset(id) {
  assetModal.value.busy = true
  assetModal.value.error = ''
  try {
    const r = await admin.probeAsset(id)
    if (assetModal.value) assetModal.value.probe = { id, results: r.results }
  } catch (e) {
    if (assetModal.value) assetModal.value.error = e?.message || '测速失败'
  } finally {
    if (assetModal.value) assetModal.value.busy = false
  }
}

async function fetchAsset(id) {
  assetModal.value.busy = true
  assetModal.value.error = ''
  try {
    const r = await admin.fetchAsset(id)
    ui.notify(`下载完成（${fmtBytes(r.size)}，经 ${r.via}）`)
  } catch (e) {
    if (assetModal.value) assetModal.value.error = e?.message || '下载失败'
  } finally {
    if (assetModal.value) assetModal.value.busy = false
  }
}

async function pushAsset(id) {
  if (!assetServerId.value) {
    assetModal.value.error = '请选择目标节点'
    return
  }
  assetModal.value.busy = true
  assetModal.value.error = ''
  try {
    await admin.pushAsset(id, assetServerId.value)
    ui.notify('已推送到节点 /usr/local/bin/')
  } catch (e) {
    if (assetModal.value) assetModal.value.error = e?.message || '推送失败'
  } finally {
    if (assetModal.value) assetModal.value.busy = false
  }
}

async function removeAsset(id) {
  assetModal.value.busy = true
  try {
    await admin.deleteAsset(id)
    ui.notify('资产已删除')
  } catch (e) {
    ui.notify(e?.message || '删除失败')
  } finally {
    if (assetModal.value) assetModal.value.busy = false
  }
}

function shortUrl(u) {
  try {
    const x = new URL(u)
    return x.host.replace(/^www\./, '') + (x.pathname.length > 24 ? x.pathname.slice(0, 24) + '…' : x.pathname)
  } catch {
    return u.slice(0, 32)
  }
}

function probeTag(r) {
  if (!r.status) return { text: '不可用', cls: 'bt-tag--danger' }
  if (r.status === 200 || r.status === 206) return { text: `${r.latency_ms}ms`, cls: 'bt-tag--success' }
  return { text: `HTTP ${r.status}`, cls: 'bt-tag--warning' }
}

// ---------- 生命周期 ----------
onMounted(async () => {
  window.addEventListener('keydown', onMeshKey)
  await admin.loadWg()
  await nextTick()
  renderTopo()
  window.addEventListener('resize', resizeTopo)
})

watch(() => admin.wgOverview, () => nextTick(renderTopo), { deep: false })
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>WG 组网</h1>
        <p class="page-head__desc">WireGuard 一键组网 · 主备中心（A/B 流量额度轮换）· 设备凭证二维码</p>
      </div>
      <div class="page-head__actions">
        <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="admin.wgLoading || runningTask || patrolling" @click="patrol">
          <AppIcon name="refresh" aria-hidden="true" />{{ patrolling ? '巡检中…' : '手动巡检' }}
        </button>
        <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="openAssets">
          <AppIcon name="download" aria-hidden="true" />资产中转
        </button>
        <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="runningTask" @click="openImport">
          <AppIcon name="download" aria-hidden="true" />导入现有网络
        </button>
        <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="runningTask" @click="openWizard">
          <AppIcon name="plus" aria-hidden="true" />组网向导
        </button>
      </div>
    </div>

    <StateError
      v-if="admin.wgError"
      style="margin-bottom: 12px"
      :message="admin.wgError"
      @retry="admin.loadWg()"
    />

    <StateSkeleton v-if="admin.wgLoading && !network" style="margin-bottom: 12px" :rows="4" />

    <!-- 未初始化引导 -->
    <div v-if="!admin.wgLoading && !network && !admin.wgError" class="bt-card">
      <div class="bt-card__body" style="text-align: center; padding: 40px 16px">
        <AppIcon name="beacon" style="width: 40px; height: 40px" aria-hidden="true" />
        <h2 style="margin: 12px 0 6px">尚未初始化 WG 组网</h2>
        <p style="color: var(--bt-text-2, #667); margin: 0 0 20px">
          已有现网（如 wg1/wg2 星型组网）可直接导入纳管；也可以用向导从零组一张新网。
        </p>
        <div style="display: flex; gap: 12px; justify-content: center; flex-wrap: wrap">
          <button class="bt-btn bt-btn--primary" type="button" @click="openImport">
            <AppIcon name="download" aria-hidden="true" />导入现有网络（推荐）
          </button>
          <button class="bt-btn bt-btn--default" type="button" @click="openWizard">
            <AppIcon name="plus" aria-hidden="true" />全新组网
          </button>
        </div>
      </div>
    </div>

    <!-- 已初始化总览 -->
    <template v-if="network">
      <div class="bt-card" style="margin-bottom: 12px">
        <div class="bt-card__body mesh-bar">
          <span class="bt-tag bt-tag--info"><AppIcon name="shield" aria-hidden="true" />{{ network.subnet }}</span>
          <span>hub 虚拟 IP <b class="mono">{{ network.hub_ip }}</b></span>
          <span>接口 <b class="mono">{{ network.iface }}</b></span>
          <span>keepalive {{ network.keepalive }}s · MTU {{ network.mtu }}</span>
          <span v-if="activeHub">现役：<b>{{ activeHub.name }}</b></span>
          <span v-if="runningTask" class="bt-tag bt-tag--warning">任务执行中…</span>
        </div>
      </div>

      <!-- 中心节点：现役在前 / 备援在后 / 网外未纳管（可登记为备援） -->
      <div class="mesh-hubs">
        <div v-for="h in orderedHubs" :key="h.server_id" class="bt-card bt-card--hover">
          <div class="bt-card__head">
            <div class="bt-card__title">
              {{ h.name }}
              <span class="bt-tag" :class="h.is_active ? 'bt-tag--success' : ''">{{ h.is_active ? '现役' : '备援' }}</span>
              <span class="bt-tag" :class="statusTag(h.status).cls">{{ statusTag(h.status).text }}</span>
            </div>
          </div>
          <div class="bt-card__body">
            <div class="mesh-hub-line mono">{{ h.endpoint || '端点未知' }}</div>
            <div class="mesh-hub-line">
              本月出向（计费）<b class="tnum">{{ fmtBytes(h.month_billed) }}</b>
              <span v-if="h.quota_gb" class="tnum">/ {{ h.quota_gb }} GB</span>
            </div>
            <div v-if="h.quota_gb" class="mesh-quota">
              <div
                class="mesh-quota__bar"
                :class="{ 'is-warn': h.month_billed / (h.quota_gb * 1e9) > 0.8, 'is-full': h.month_billed / (h.quota_gb * 1e9) >= 1 }"
                :style="{ width: Math.min(100, (h.month_billed / (h.quota_gb * 1e9)) * 100) + '%' }"
              />
            </div>
            <div class="mesh-hub-line mesh-hub-sub">
              双向合计 {{ fmtBytes(h.month_rx + h.month_tx) }} · 入向不计费
            </div>
            <div v-if="h.last_error" class="mesh-hub-error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />{{ h.last_error }}
            </div>
            <div class="mesh-hub-actions">
              <template v-if="h.is_active">
                <span v-if="standbyHubs.length" class="bt-tag bt-tag--info">流量额度用完时切到备援</span>
                <span v-else class="mesh-hub-sub">尚无备援：登记网外节点后即可一键切换</span>
              </template>
              <button v-else class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="runningTask" @click="openSwitch(h)">
                <AppIcon name="pulse" aria-hidden="true" />切换为现役
              </button>
            </div>
          </div>
        </div>

        <div v-for="s in unmanagedServers" :key="'idle-' + s.id" class="bt-card mesh-hub-idle">
          <div class="bt-card__head">
            <div class="bt-card__title">
              {{ s.name }}
              <span class="bt-tag">未纳管</span>
            </div>
          </div>
          <div class="bt-card__body">
            <div class="mesh-hub-line mesh-hub-sub">不在本网 · 登记只读取它的 WG 配置，不改动它</div>
            <div class="mesh-hub-actions">
              <button
                class="bt-btn bt-btn--ghost bt-btn--sm"
                type="button"
                :disabled="runningTask || !activeHub || standbyReg[s.id]?.busy"
                @click="registerStandby(s)"
              >
                <AppIcon name="shield" aria-hidden="true" />{{ standbyReg[s.id]?.busy ? '登记中…' : '准备为备援' }}
              </button>
            </div>
            <div v-if="standbyReg[s.id]?.error" class="mesh-hub-error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />{{ standbyReg[s.id].error }}
            </div>
          </div>
        </div>
      </div>

      <!-- 拓扑 -->
      <div class="bt-card" style="margin-bottom: 12px">
        <div class="bt-card__head"><div class="bt-card__title">拓扑</div></div>
        <div ref="topoEl" class="mesh-topo" aria-label="组网拓扑图" />
      </div>

      <!-- 成员表 -->
      <div class="bt-card">
        <div class="bt-card__head">
          <div class="bt-card__title">成员（{{ peers.length }}）</div>
          <div class="mesh-peer-actions">
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="runningTask" @click="openDevice">
              <AppIcon name="apple" aria-hidden="true" />添加设备
            </button>
            <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="runningTask" @click="openWizard">
              <AppIcon name="plus" aria-hidden="true" />接入节点
            </button>
          </div>
        </div>
        <div class="bt-card__body bt-table-wrap" style="padding: 0">
          <table class="bt-table mesh-table">
            <caption>组网成员 · 共 {{ peers.length }} 个</caption>
            <thead>
              <tr>
                <th scope="col">名称</th><th scope="col">类型</th><th scope="col">WG IP</th><th scope="col">状态</th>
                <th scope="col">最近握手</th><th scope="col">收 / 发</th><th scope="col">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in peers" :key="p.id">
                <td>
                  {{ p.name }}
                  <span v-if="p.last_error" class="bt-tag bt-tag--danger" :title="p.last_error">!</span>
                </td>
                <td>{{ p.kind === 'device' ? '设备' : '节点' }}</td>
                <td class="mono">{{ p.wg_ip }}</td>
                <td>
                  <span class="bt-tag pulse-dot" :class="statusTag(p.status).cls">{{ statusTag(p.status).text }}</span>
                </td>
                <td class="tnum">{{ p.last_handshake ? agoFromTs(p.last_handshake) : '从未' }}</td>
                <td class="tnum">{{ fmtBytes(p.rx_bytes) }} / {{ fmtBytes(p.tx_bytes) }}</td>
                <td class="mesh-row-actions">
                  <button v-if="p.kind === 'server'" class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="verifyingId === p.id" @click="verifyPeer(p)">
                    {{ verifyingId === p.id ? '验证中…' : '验证' }}
                  </button>
                  <button v-if="p.can_export" class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="openConf(p, activeHub?.server_id || 0)">
                    凭证 / QR
                  </button>
                  <button class="bt-btn bt-btn--ghost bt-btn--sm bt-text-danger" type="button" @click="confirmDelete = p">
                    <AppIcon name="trash" aria-hidden="true" />移出
                  </button>
                </td>
              </tr>
              <tr v-if="!peers.length">
                <td colspan="7" style="text-align: center; padding: 24px" class="bt-text-muted">暂无成员，用右上角「接入节点」或「导入现有网络」开始</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>

    <!-- 组网向导 -->
    <Transition name="modal">
    <div v-if="wizard" class="bt-modal-mask" @click.self="wizard.busy ? null : (wizard = null, restoreFocus())">
      <div class="bt-modal bt-modal--lg" role="dialog" aria-modal="true" aria-label="组网向导">
        <div class="bt-modal__head">
          <div class="bt-modal__title">组网向导 · 第 {{ wizard.step }} 步 / 3</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="wizard.busy" @click="wizard = null">关闭</button>
        </div>
        <div class="bt-modal__body">
          <!-- 第 1 步：选择 -->
          <template v-if="wizard.step === 1">
            <div v-if="!network" class="mesh-wizard-params">
              <label class="bt-field"><span class="bt-field__label">子网 CIDR</span>
                <input v-model="wizardSubnet" class="bt-input mono" type="text" /></label>
              <label class="bt-field"><span class="bt-field__label">hub 虚拟 IP</span>
                <input v-model="wizardHubIp" class="bt-input mono" type="text" /></label>
              <label class="bt-field"><span class="bt-field__label">hub 监听端口</span>
                <input v-model.number="wizardHubPort" class="bt-input" type="number" min="1" max="65535" /></label>
            </div>
            <p class="mesh-hint">中心节点（★）：须公网 UDP 端口可达（云服务器）；再勾选要接入的节点。</p>
            <div class="mesh-pick-list">
              <div v-for="s in selectableServers" :key="s.id" class="bt-check mesh-pick">
                <input
                  :id="'wg-hub-' + s.id"
                  type="radio"
                  name="wg-hub"
                  :checked="wizardHubId === s.id"
                  @change="wizardHubId = s.id"
                />
                <label :for="'wg-hub-' + s.id" title="设为中心节点">★ 中心</label>
                <input
                  :id="'wg-mem-' + s.id"
                  type="checkbox"
                  :checked="selectedIds.includes(s.id)"
                  :disabled="wizardHubId === s.id"
                  @change="togglePick(s.id)"
                />
                <label :for="'wg-mem-' + s.id">
                  {{ s.name }} · {{ wizardHubId === s.id ? '作为中心' : s.in_network ? '已在网' : '接入' }}
                </label>
              </div>
            </div>
          </template>
          <!-- 第 2 步：预检结果 -->
          <template v-else-if="wizard.step === 2 && wizard.plan">
            <div class="mesh-plan-hub">
              <b>★ {{ wizard.plan.hub.name }}</b>
              <span class="bt-tag bt-tag--info">{{ wizard.plan.hub.role === 'hub' ? '现役中心' : '备援中心' }} :{{ wizard.plan.hub.listen_port }}</span>
              <span v-for="(i, idx) in wizard.plan.hub.issues" :key="idx" class="bt-tag" :class="i.level === 'error' ? 'bt-tag--danger' : 'bt-tag--warning'">{{ i.msg }}</span>
            </div>
            <div v-for="s in wizard.plan.spokes" :key="s.server_id" class="mesh-plan-row">
              <b>{{ s.name }}</b>
              <span class="mono">{{ s.wg_ip || '未分配' }}</span>
              <span v-for="(i, idx) in s.issues" :key="idx" class="bt-tag" :class="i.level === 'error' ? 'bt-tag--danger' : 'bt-tag--warning'">{{ i.msg }}</span>
            </div>
            <p v-if="wizard.plan.blocked" class="bt-text-danger mesh-hint" role="alert">存在阻断级问题（红标），请处理后重试。</p>
          </template>
          <div v-if="wizard.error" class="bt-alert bt-alert--error" role="alert">
            <AppIcon name="warn" aria-hidden="true" />{{ wizard.error }}
          </div>
        </div>
        <div class="bt-modal__foot">
          <button v-if="wizard.step === 1" class="bt-btn bt-btn--primary" type="button" :disabled="wizard.busy" @click="runPlan">
            {{ wizard.busy ? '预检中…（SSH 探测各节点）' : '下一步：预检' }}
          </button>
          <template v-else-if="wizard.step === 2">
            <button class="bt-btn bt-btn--ghost" type="button" @click="wizard.step = 1">上一步</button>
            <button class="bt-btn bt-btn--primary" type="button" :disabled="wizard.busy || wizard.plan?.blocked" @click="runApply">
              {{ wizard.busy ? '提交中…' : '确认执行组网' }}
            </button>
          </template>
        </div>
      </div>
    </div>
    </Transition>

    <!-- 导入现有网络 -->
    <Transition name="modal">
    <div v-if="importModal" class="bt-modal-mask" @click.self="importModal.busy ? null : (importModal = null, restoreFocus())">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="导入现有网络">
        <div class="bt-modal__head">
          <div class="bt-modal__title">导入现有 WG 网络</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="importModal = null; restoreFocus()">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack">
            <p class="bt-modal__desc">
              选择现役中心节点，面板将读取其配置与运行态，自动纳管网内成员（按公钥匹配；无法匹配的记为设备）。
            </p>
            <label class="bt-field"><span class="bt-field__label">现役中心节点 ★</span>
              <select v-model="importHubId" class="bt-select">
                <option :value="0" disabled>请选择</option>
                <option v-for="s in selectableServers" :key="s.id" :value="s.id">{{ s.name }}</option>
              </select>
            </label>
            <label class="bt-field"><span class="bt-field__label">备援节点（可选，warm standby）</span>
              <select v-model="importStandbyId" class="bt-select">
                <option :value="0">无</option>
                <option v-for="s in selectableServers.filter((x) => x.id !== importHubId)" :key="s.id" :value="s.id">{{ s.name }}</option>
              </select>
            </label>
            <div class="bt-field">
              <span class="bt-field__label">参与匹配的节点（面板将 SSH 读取其 WG 配置）</span>
              <div v-if="importPickPool.length" class="mesh-pick-list">
                <label v-for="s in importPickPool" :key="s.id" class="bt-check mesh-pick">
                  <input v-model="importPicked" type="checkbox" :value="s.id" />
                  <span>{{ s.name }}</span>
                </label>
              </div>
              <p v-else class="bt-hint">暂无可参与匹配的其他节点（先去掉 hub/备援选择，或在「节点管理」添加节点）</p>
            </div>
            <div v-if="importModal.error" class="bt-alert bt-alert--error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />{{ importModal.error }}
            </div>
          </div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--ghost" type="button" :disabled="importModal.busy" @click="importModal = null; restoreFocus()">取消</button>
          <button class="bt-btn bt-btn--primary" type="button" :disabled="importModal.busy" @click="runImport">
            {{ importModal.busy ? '导入中…' : '开始导入' }}
          </button>
        </div>
      </div>
    </div>
    </Transition>

    <!-- 切换 hub -->
    <Transition name="modal">
    <div v-if="switchModal" class="bt-modal-mask" @click.self="switchModal.busy ? null : (switchModal = null, restoreFocus())">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="切换现役中心">
        <div class="bt-modal__head">
          <div class="bt-modal__title">切换现役中心节点</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="switchModal = null; restoreFocus()">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack">
            <p class="bt-modal__desc">
              流程：校正备援 hub → 金丝雀节点先切换验证（失败自动回滚）→ 其余节点切换。
              设备（Mac/iPhone）需在切换后重新扫码/导入对应凭证。
            </p>
            <label class="bt-field"><span class="bt-field__label">目标中心（切换后现役）</span>
              <select v-model="switchTargetId" class="bt-select">
                <option v-for="h in switchCandidates" :key="h.server_id" :value="h.server_id">{{ h.name }}（:{{ h.listen_port }}）</option>
              </select>
            </label>
            <div v-if="switchModal.error" class="bt-alert bt-alert--error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />{{ switchModal.error }}
            </div>
          </div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--ghost" type="button" :disabled="switchModal.busy" @click="switchModal = null; restoreFocus()">取消</button>
          <button class="bt-btn bt-btn--danger" type="button" :disabled="switchModal.busy" @click="runSwitch">
            {{ switchModal.busy ? '提交中…' : '开始切换' }}
          </button>
        </div>
      </div>
    </div>
    </Transition>

    <!-- 添加设备 -->
    <Transition name="modal">
    <div v-if="deviceModal" class="bt-modal-mask" @click.self="deviceModal.busy ? null : (deviceModal = null, restoreFocus())">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="添加设备">
        <div class="bt-modal__head">
          <div class="bt-modal__title">添加设备（Mac / iPhone / Win）</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="deviceModal = null; restoreFocus()">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div class="bt-form-stack">
            <label class="bt-field"><span class="bt-field__label">设备名称</span>
              <input v-model="deviceModal.name" class="bt-input" type="text" placeholder="如 iPhone 15" @keyup.enter="createDevice" />
            </label>
            <p class="bt-modal__desc">面板生成密钥并热加入现役 hub；创建后点成员表「凭证 / QR」扫码导入。</p>
            <div v-if="deviceModal.error" class="bt-alert bt-alert--error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />{{ deviceModal.error }}
            </div>
          </div>
        </div>
        <div class="bt-modal__foot">
          <button class="bt-btn bt-btn--ghost" type="button" :disabled="deviceModal.busy" @click="deviceModal = null; restoreFocus()">取消</button>
          <button class="bt-btn bt-btn--primary" type="button" :disabled="deviceModal.busy" @click="createDevice">
            {{ deviceModal.busy ? '创建中…' : '创建' }}
          </button>
        </div>
      </div>
    </div>
    </Transition>

    <!-- 设备凭证 / QR -->
    <Transition name="modal">
    <div v-if="confModal" class="bt-modal-mask" @click.self="confModal = null">
      <div class="bt-modal" role="dialog" aria-modal="true" aria-label="设备凭证">
        <div class="bt-modal__head">
          <div class="bt-modal__title">凭证 · {{ confModal.peer.name }}</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="confModal = null; restoreFocus()">关闭</button>
        </div>
        <div class="bt-modal__body mesh-conf">
          <div v-if="hubs.length > 1" class="mesh-conf-hubs">
            <button
              v-for="h in hubs" :key="h.server_id"
              class="bt-btn bt-btn--sm" :class="confModal.hubId === h.server_id ? 'bt-btn--primary' : 'bt-btn--ghost'"
              type="button" @click="openConf(confModal.peer, h.server_id)"
            >
              {{ h.is_active ? 'A · 现役' : 'B · 备援' }}（{{ h.name }}）
            </button>
          </div>
          <div v-if="confModal.loading" style="padding: 24px; text-align: center">生成中…</div>
          <template v-else-if="confModal.error">
            <div class="bt-alert bt-alert--error" role="alert">
              <AppIcon name="warn" aria-hidden="true" />{{ confModal.error }}
            </div>
          </template>
          <template v-else>
            <img :src="confModal.dataUrl" alt="WireGuard 配置二维码" class="mesh-qr" />
            <textarea class="bt-textarea mono mesh-conf-text" readonly :value="confModal.conf" rows="10" />
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="downloadConf">
              <AppIcon name="download" aria-hidden="true" />下载 .conf
            </button>
          </template>
        </div>
      </div>
    </div>
    </Transition>

    <!-- 任务进度 -->
    <Transition name="modal">
    <div v-if="taskModal" class="bt-modal-mask" @click.self="taskModal.task && taskModal.task.status !== 'running' ? closeTaskModal() : null">
      <div class="bt-modal bt-modal--lg" role="dialog" aria-modal="true" aria-label="任务进度">
        <div class="bt-modal__head">
          <div class="bt-modal__title">
            任务 #{{ taskModal.id }}
            <span v-if="taskModal.task" class="bt-tag" :class="taskModal.task.status === 'running' ? 'bt-tag--info' : taskModal.task.status === 'done' ? 'bt-tag--success' : 'bt-tag--danger'">
              {{ taskModal.task.status === 'running' ? '执行中' : taskModal.task.status === 'done' ? '完成' : taskModal.task.status === 'partial' ? '部分成功' : '失败' }}
            </span>
          </div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="closeTaskModal()">关闭</button>
        </div>
        <div class="bt-modal__body">
          <div v-for="st in taskModal.task?.steps || []" :key="st.id" class="mesh-step">
            <div class="mesh-step__head">
              <span class="bt-tag" :class="st.status === 'ok' ? 'bt-tag--success' : st.status === 'failed' ? 'bt-tag--danger' : st.status === 'running' ? 'bt-tag--info' : ''">
                {{ st.status === 'ok' ? '成功' : st.status === 'failed' ? '失败' : st.status === 'running' ? '执行中' : st.status === 'skipped' ? '跳过' : '等待' }}
              </span>
              {{ st.title }}
            </div>
            <pre v-if="st.log" class="mesh-step__log">{{ st.log }}</pre>
          </div>
          <div v-if="taskModal.error" class="bt-alert bt-alert--error" role="alert">
            <AppIcon name="warn" aria-hidden="true" />{{ taskModal.error }}
          </div>
        </div>
      </div>
    </div>
    </Transition>

    <!-- 资产中转 -->
    <Transition name="modal">
    <div v-if="assetModal" class="bt-modal-mask" @click.self="assetModal.busy ? null : (assetModal = null, restoreFocus())">
      <div class="bt-modal bt-modal--lg" role="dialog" aria-modal="true" aria-label="资产中转">
        <div class="bt-modal__head">
          <div class="bt-modal__title">资产中转（GitHub 加速测速 · 面板缓存 · SSH 推送）</div>
          <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="assetModal = null; restoreFocus()">关闭</button>
        </div>
        <div class="bt-modal__body">
          <p class="bt-modal__desc">
            国内节点难以直连 GitHub 时，在此登记资产（官方直链 / 加速镜像 / 自有仓库 release），
            面板测速择优下载校验后缓存，再经 SSH 推送到节点（节点零外网依赖）。
          </p>
          <div v-if="assetModal.error" class="bt-alert bt-alert--error mesh-asset-alert" role="alert">
            <AppIcon name="warn" aria-hidden="true" />{{ assetModal.error }}
          </div>
          <div class="mesh-asset-toolbar">
            <span class="mesh-asset-toolbar__label">推送目标</span>
            <select v-model="assetServerId" class="bt-select mesh-asset-toolbar__select">
              <option v-for="p in serverPeers" :key="p.server_id" :value="p.server_id">{{ p.name }}</option>
            </select>
            <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="assetRegisterForm">
              <AppIcon name="plus" aria-hidden="true" />登记资产
            </button>
          </div>
          <!-- 登记表单 -->
          <div v-if="assetModal.form" class="mesh-asset-form">
            <label class="bt-field"><span class="bt-field__label">名称（将作为节点上的可执行名）</span>
              <input v-model="assetModal.form.name" class="bt-input" type="text" placeholder="wireguard-go" /></label>
            <label class="bt-field"><span class="bt-field__label">版本</span>
              <input v-model="assetModal.form.version" class="bt-input" type="text" placeholder="0.0.20230223" /></label>
            <label class="bt-field"><span class="bt-field__label">sha256（可选，下载校验）</span>
              <input v-model="assetModal.form.sha256" class="bt-input mono" type="text" /></label>
            <label class="bt-field"><span class="bt-field__label">下载源（每行一个，GitHub 链接自动测镜像）</span>
              <textarea v-model="assetModal.form.sources_text" class="bt-textarea mono" rows="3"
                placeholder="https://github.com/owner/repo/releases/download/v1/xxx" /></label>
            <div class="mesh-form-actions">
              <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="assetModal.form = null">取消</button>
              <button class="bt-btn bt-btn--primary bt-btn--sm" type="button" :disabled="assetModal.busy" @click="saveAsset">保存</button>
            </div>
          </div>
          <!-- 资产列表 -->
          <div v-for="as in admin.assets" :key="as.id" class="mesh-asset-row">
            <div class="mesh-asset-main">
              <b>{{ as.name }}</b>
              <span class="mono">{{ as.version }}</span>
              <span class="bt-tag" :class="as.cached ? 'bt-tag--success' : 'bt-tag--warning'">
                {{ as.cached ? `已缓存 ${fmtBytes(as.size)}` : '未缓存' }}
              </span>
              <span v-if="as.note" class="bt-text-muted">{{ as.note }}</span>
            </div>
            <div v-if="assetModal.probe && assetModal.probe.id === as.id" class="mesh-probe-results">
              <span v-for="(r, idx) in assetModal.probe.results.slice(0, 6)" :key="idx"
                class="bt-tag" :class="probeTag(r).cls" :title="r.url">
                {{ shortUrl(r.url) }} · {{ probeTag(r).text }}{{ r.err ? ' · ' + r.err : '' }}
              </span>
            </div>
            <div class="mesh-asset-actions">
              <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="assetModal.busy" @click="probeAsset(as.id)">测速</button>
              <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="assetModal.busy" @click="fetchAsset(as.id)">
                {{ as.cached ? '重新下载' : '下载缓存' }}
              </button>
              <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" :disabled="assetModal.busy || !as.cached" @click="pushAsset(as.id)">推送节点</button>
              <button class="bt-btn bt-btn--ghost bt-btn--sm bt-text-danger" type="button" @click="removeAsset(as.id)">
                <AppIcon name="trash" aria-hidden="true" />
              </button>
            </div>
          </div>
          <div v-if="admin.assetsLoading && !admin.assets.length && !assetModal.form" class="bt-hint" style="text-align: center; padding: 12px">
            正在加载资产…
          </div>
          <div v-else-if="!admin.assets.length && !assetModal.form" class="mesh-asset-empty">
            <AppIcon name="empty" aria-hidden="true" />
            <p>暂无资产</p>
            <span>点「登记资产」添加，如 wireguard-go 二进制、静态 wg 工具</span>
          </div>
        </div>
      </div>
    </div>
    </Transition>

    <!-- 移出确认 -->
    <ConfirmDialog
      v-if="confirmDelete"
      :title="`移出 ${confirmDelete.name}`"
      :message="`将下线其 WG 接口并从中心节点移除该成员（${confirmDelete.wg_ip}）。确认执行？`"
      confirm-text="移出网络"
      :danger="true"
      :busy="deleting"
      @cancel="confirmDelete = null"
      @confirm="doDelete"
    />
  </div>
</template>

<style scoped>
.mesh-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 18px;
  align-items: center;
  font-size: 13px;
}
.mesh-hubs {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 268px));
  gap: 10px;
  margin-bottom: 12px;
  align-items: start;
}
/* 中心区卡片刻意做小：信息密度高，不需要铺满整行 */
.mesh-hubs > .bt-card .bt-card__title {
  font-size: var(--bt-font-md);
}
.mesh-hubs > .bt-card .bt-card__head {
  padding: var(--bt-space-3) var(--bt-space-4) 0;
}
.mesh-hubs > .bt-card .bt-card__body {
  padding: var(--bt-space-2) var(--bt-space-4) var(--bt-space-4);
}
.mesh-hubs .mesh-hub-line {
  font-size: 12px;
  margin-bottom: 4px;
}
.mesh-hubs .mesh-hub-actions {
  margin-top: 6px;
}
.mesh-hub-line {
  font-size: 13px;
  margin-bottom: 6px;
}
/* 网外未纳管节点：虚框表示「还不属于这张网」 */
.mesh-hub-idle {
  border-style: dashed;
  border-color: var(--bt-border-strong);
  background: transparent;
}
.mesh-hub-idle .mesh-hub-sub {
  line-height: 1.55;
}
.mesh-hub-error {
  color: var(--bt-danger, #d64545);
  font-size: 12px;
  margin: 6px 0;
}
.mesh-hub-actions {
  margin-top: 8px;
}
.mesh-quota {
  height: 6px;
  border-radius: 3px;
  background: rgba(128, 128, 128, 0.18);
  overflow: hidden;
  margin: 4px 0 8px;
}
.mesh-quota__bar {
  height: 100%;
  background: var(--bt-success-500, #10b981);
}
.mesh-quota__bar.is-warn {
  background: var(--bt-warning-500, #f59e0b);
}
.mesh-quota__bar.is-full {
  background: var(--bt-danger-500, #d64545);
}
.mesh-topo {
  height: 340px;
}
.mesh-peer-actions {
  display: flex;
  gap: 8px;
}
.mesh-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.mesh-table th,
.mesh-table td {
  padding: 8px 10px;
  border-bottom: 1px solid rgba(128, 128, 128, 0.15);
  text-align: left;
}
.mesh-table th {
  font-weight: 600;
  color: #778;
  font-size: 12px;
}
.mesh-row-actions {
  white-space: nowrap;
}
.mesh-row-actions .bt-btn {
  margin-right: 4px;
}
.mesh-hint {
  font-size: var(--bt-font-md);
  color: var(--bt-text-3);
  line-height: var(--bt-line-md);
  margin: 0 0 var(--bt-space-3);
}
.mesh-pick-list {
  display: flex;
  flex-direction: column;
  gap: var(--bt-space-2);
  max-height: 260px;
  overflow: auto;
  padding: 2px;
}
.mesh-pick {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 10px;
  border: 1px solid var(--bt-border-strong);
  border-radius: var(--bt-radius-md);
  transition:
    border-color var(--bt-duration-fast) ease,
    background var(--bt-duration-fast) ease;
}
.mesh-pick:hover {
  border-color: rgba(14, 165, 233, 0.45);
  background: rgba(14, 165, 233, 0.05);
}
.mesh-wizard-params {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
  margin-bottom: 8px;
}
.mesh-plan-hub,
.mesh-plan-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  padding: 8px;
  border: 1px solid rgba(128, 128, 128, 0.2);
  border-radius: 8px;
  margin-bottom: 6px;
  font-size: 13px;
}
.mesh-conf {
  text-align: center;
}
.mesh-conf-hubs {
  display: flex;
  gap: 8px;
  justify-content: center;
  margin-bottom: 10px;
  flex-wrap: wrap;
}
.mesh-qr {
  width: 300px;
  max-width: 100%;
  border-radius: 8px;
  background: #fff;
  padding: 8px;
}
.mesh-conf-text {
  width: 100%;
  margin: 10px 0;
  font-size: 12px;
  text-align: left;
}
.mesh-step {
  border: 1px solid rgba(128, 128, 128, 0.2);
  border-radius: 8px;
  padding: 8px 10px;
  margin-bottom: 8px;
}
.mesh-step__head {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 13px;
}
.mesh-step__log {
  margin: 6px 0 0;
  padding: 8px;
  background: rgba(128, 128, 128, 0.1);
  border-radius: 6px;
  font-size: 12px;
  max-height: 140px;
  overflow: auto;
  white-space: pre-wrap;
}
.bt-text-muted {
  opacity: 0.6;
}
.mesh-asset-alert {
  margin-top: var(--bt-space-4);
}
/* 顶部工具栏：推送目标（带标签）+ 登记按钮右对齐 */
.mesh-asset-toolbar {
  display: flex;
  align-items: center;
  gap: var(--bt-space-2);
  margin: var(--bt-space-4) 0;
}
.mesh-asset-toolbar__label {
  flex-shrink: 0;
  font-size: var(--bt-font-sm);
  font-weight: var(--bt-weight-medium);
  color: var(--bt-text-2);
}
.mesh-asset-toolbar__select {
  width: auto;
  min-width: 150px;
  max-width: 240px;
}
.mesh-asset-toolbar .bt-btn {
  margin-left: auto;
  flex-shrink: 0;
}
.mesh-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--bt-space-2);
}
.mesh-asset-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 26px 16px;
  border: 1px dashed var(--bt-border-strong);
  border-radius: var(--bt-radius-lg);
  text-align: center;
  color: var(--bt-text-3);
}
.mesh-asset-empty svg {
  width: 26px;
  height: 26px;
  opacity: 0.55;
  margin-bottom: 4px;
}
.mesh-asset-empty p {
  margin: 0;
  font-size: var(--bt-font-md);
  font-weight: var(--bt-weight-semibold);
  color: var(--bt-text-2);
}
.mesh-asset-empty span {
  font-size: var(--bt-font-sm);
}
.mesh-asset-form {
  border: 1px solid var(--bt-border-strong);
  border-radius: var(--bt-radius-lg);
  padding: var(--bt-space-4);
  margin-bottom: var(--bt-space-3);
  display: grid;
  gap: var(--bt-space-3);
}
.mesh-asset-row {
  border: 1px solid var(--bt-border);
  border-radius: var(--bt-radius-lg);
  padding: var(--bt-space-3) var(--bt-space-4);
  margin-bottom: var(--bt-space-2);
}
.mesh-asset-main {
  display: flex;
  gap: var(--bt-space-2);
  align-items: center;
  flex-wrap: wrap;
  font-size: var(--bt-font-md);
}
.mesh-asset-actions {
  display: flex;
  gap: var(--bt-space-2);
  margin-top: var(--bt-space-2);
}
.mesh-probe-results {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 6px;
}
</style>
