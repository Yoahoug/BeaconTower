// ============================================================
// BeaconTower · 管理端 Store（Pinia）
// 收敛旧 AdminPanelView.vue 内散落的 loading/authed/tab 状态：
// - auth：初始化与否 / 是否登录 / 用户名
// - servers/settings/audit：统一 loading + error + 重试
// 视图层只读 state、只调 actions，禁止直调 adminClient/authClient。
// 鉴权（status/logout）走轻量 authClient，节点/设置/审计走 adminClient，
// 保证公开首屏不含管理代码（管理 chunk 懒加载后才引入本 store）。
// ============================================================
import { defineStore } from 'pinia'
import { adminClient } from '../api/admin'
import { authClient, resetAuthCache } from '../api/auth'

export const useAdminStore = defineStore('admin', {
  state: () => ({
    initialized: false,
    loggedIn: false,
    username: '',
    siteTitle: '',
    authChecked: false,

    servers: [],
    serversLoading: false,
    serversError: '',

    settings: null,
    settingsLoading: false,
    settingsSaving: false,
    settingsError: '',
    settingsSaved: false,

    auditItems: [],
    auditTotal: 0,
    auditPage: 1,
    auditSize: 20,
    auditLoading: false,
    auditError: '',
    auditFilter: '',

    // WG 组网
    wgOverview: null,
    wgLoading: false,
    wgError: '',

    // 资产中转
    assets: [],
    assetsLoading: false,
    assetsError: '',

    // 内网穿透平台（doc/13）
    frpOverview: null,
    frpLoading: false,
    frpError: '',
    // 每平台一份详情（账号 + 隧道 + 节点 + 用量趋势），键为平台 id：
    // 双平台同屏展开，不再是「选中一个」的单详情模型
    frpDetails: {},
    frpDetailLoading: false,
    frpSaving: false, // 写操作进行中（建/改/删隧道、同步等）
  }),

  getters: {
    auditPages: (s) => Math.max(1, Math.ceil(s.auditTotal / s.auditSize)),
    filteredAudit: (s) => {
      const q = s.auditFilter.trim().toLowerCase()
      if (!q) return s.auditItems
      return s.auditItems.filter((x) =>
        `${x.action} ${x.detail} ${x.actor} ${x.target}`.toLowerCase().includes(q),
      )
    },
  },

  actions: {
    async checkAuth() {
      try {
        const st = await authClient.status()
        this.initialized = st.initialized
        this.loggedIn = st.loggedIn
        this.username = st.username || ''
        this.siteTitle = st.site_title || ''
      } finally {
        this.authChecked = true
      }
    },

    markAuthed(username = '') {
      resetAuthCache()
      this.loggedIn = true
      this.initialized = true
      if (username) this.username = username
    },

    async logout() {
      await authClient.logout()
      this.loggedIn = false
      this.username = ''
    },

    async loadServers() {
      this.serversLoading = true
      this.serversError = ''
      try {
        this.servers = await adminClient.listServers()
      } catch (e) {
        this.serversError = e?.message || '节点列表加载失败'
      } finally {
        this.serversLoading = false
      }
    },

    async loadSettings() {
      this.settingsLoading = true
      this.settingsError = ''
      try {
        this.settings = await adminClient.getSettings()
      } catch (e) {
        this.settingsError = e?.message || '设置加载失败'
      } finally {
        this.settingsLoading = false
      }
    },

    async saveSettings() {
      if (!this.settings) return
      this.settingsSaving = true
      this.settingsSaved = false
      this.settingsError = ''
      try {
        await adminClient.saveSettings({ ...this.settings })
        this.settingsSaved = true
        window.setTimeout(() => {
          this.settingsSaved = false
        }, 2000)
      } catch (e) {
        this.settingsError = e?.message || '保存失败'
      } finally {
        this.settingsSaving = false
      }
    },

    async loadAudit(page = 1) {
      this.auditLoading = true
      this.auditError = ''
      try {
        this.auditPage = page
        const r = await adminClient.listAudit(page, this.auditSize)
        this.auditItems = r.items
        this.auditTotal = r.total
      } catch (e) {
        this.auditError = e?.message || '审计日志加载失败'
      } finally {
        this.auditLoading = false
      }
    },

    setAuditFilter(q) {
      this.auditFilter = q
    },

    // ---------- WG 组网（doc/12 §6） ----------
    async loadWg() {
      this.wgLoading = true
      this.wgError = ''
      try {
        this.wgOverview = await adminClient.wgOverview()
      } catch (e) {
        this.wgError = e?.message || '组网信息加载失败'
      } finally {
        this.wgLoading = false
      }
    },

    async planWg(payload) {
      return adminClient.wgPlan(payload)
    },

    async applyWg(payload) {
      const r = await adminClient.wgApply(payload)
      await this.loadWg()
      return r
    },

    async importWg(payload) {
      const r = await adminClient.wgImport(payload)
      await this.loadWg()
      return r
    },

    async switchHub(payload) {
      const r = await adminClient.wgSwitchHub(payload)
      await this.loadWg()
      return r
    },

    // 未纳管节点 → 备援槽位（只读对方 conf；成功后该节点即进网）
    async registerStandby(serverId) {
      const r = await adminClient.wgRegisterStandby(serverId)
      await this.loadWg()
      return r
    },

    // 已手工配好的节点（含本机）→ 纳管为受管成员（保留 WG IP 与流量历史）
    async adoptServer(serverId) {
      const r = await adminClient.wgAdoptServer(serverId)
      await this.loadWg()
      return r
    },

    // 中心节点面板
    async loadHub(id) {
      return adminClient.wgHubDetail(id)
    },

    async hubProbe(id) {
      const r = await adminClient.wgHubProbe(id)
      await this.loadWg()
      return r
    },

    async renamePeer(id, name) {
      const r = await adminClient.wgPeerRename(id, name)
      await this.loadWg()
      return r
    },

    async syncPeerHubs(id) {
      const r = await adminClient.wgPeerSyncHubs(id)
      await this.loadWg()
      return r
    },

    // 新机接管（身份迁移）
    async takeover(payload) {
      const r = await adminClient.wgTakeover(payload)
      await this.loadWg()
      return r
    },

    async cleanupHub(id) {
      await adminClient.wgHubCleanup(id)
      await this.loadWg()
    },

    async patrolNow() {
      await adminClient.wgPatrol()
      await this.loadWg()
    },

    async loadWgTask(id) {
      return adminClient.wgTask(id)
    },

    async createDevice(name) {
      const r = await adminClient.wgDeviceCreate(name)
      await this.loadWg()
      return r
    },

    async peerConf(id, hub = 0) {
      return adminClient.wgPeerConf(id, hub)
    },

    async verifyPeer(id) {
      const r = await adminClient.wgPeerVerify(id)
      await this.loadWg()
      return r
    },

    async deletePeer(id) {
      const r = await adminClient.wgPeerDelete(id)
      await this.loadWg()
      return r
    },

    async loadAssets() {
      this.assetsLoading = true
      this.assetsError = ''
      try {
        this.assets = (await adminClient.wgAssets()).assets
      } catch (e) {
        this.assetsError = e?.message || '资产加载失败'
        throw e // 调用方（弹窗内）需要感知失败
      } finally {
        this.assetsLoading = false
      }
    },

    async upsertAsset(payload) {
      const r = await adminClient.wgAssetUpsert(payload)
      await this.loadAssets()
      return r
    },

    async deleteAsset(id) {
      const r = await adminClient.wgAssetDelete(id)
      await this.loadAssets()
      return r
    },

    async probeAsset(id) {
      return adminClient.wgAssetProbe(id)
    },

    async fetchAsset(id) {
      const r = await adminClient.wgAssetFetch(id)
      await this.loadAssets()
      return r
    },

    async pushAsset(id, serverId) {
      return adminClient.wgAssetPush(id, serverId)
    },

    // ---------- 内网穿透平台（doc/13 §6） ----------
    async loadFrp() {
      this.frpLoading = true
      this.frpError = ''
      try {
        this.frpOverview = await adminClient.frpOverview()
      } catch (e) {
        this.frpError = e?.message || '穿透平台信息加载失败'
      } finally {
        this.frpLoading = false
      }
    },

    async loadFrpDetail(id) {
      this.frpDetailLoading = true
      try {
        const d = await adminClient.frpPlatform(id)
        this.frpDetails = { ...this.frpDetails, [id]: d }
      } catch (e) {
        // 单个平台详情失败不要清掉已有数据（同步抖动不该打飞整个页面），
        // 错误进入 frpError 由页面顶部统一展示
        this.frpError = e?.message || '平台详情加载失败'
      } finally {
        this.frpDetailLoading = false
      }
    },

    // 写操作统一包一层 frpSaving：外部平台调用慢，期间按钮要禁用防重复提交。
    // detailId 指明写操作发生在哪个平台，只刷新那一份详情（同屏多平台，
    // 全量重拉既慢又会在别的平台卡片上闪骨架屏）
    async frpRun(fn, { reload = true, detailId = 0 } = {}) {
      this.frpSaving = true
      try {
        const r = await fn()
        if (reload) {
          await this.loadFrp()
          if (detailId) await this.loadFrpDetail(detailId)
        }
        return r
      } finally {
        this.frpSaving = false
      }
    },

    frpSync(id, full = true) {
      return this.frpRun(() => adminClient.frpSync(id, full), { detailId: id })
    },

    frpBindNatfrp(payload) {
      return this.frpRun(() => adminClient.frpBindNatfrp(payload))
    },

    frpDeletePlatform(id) {
      return this.frpRun(() => adminClient.frpPlatformDelete(id), { detailId: 0 })
    },

    frpRenamePlatform(id, name) {
      return this.frpRun(() => adminClient.frpPlatformRename(id, name), { detailId: id })
    },

    frpTunnelCreate(platformId, payload) {
      return this.frpRun(() => adminClient.frpTunnelCreate(platformId, payload), { detailId: platformId })
    },

    frpTunnelUpdate(tunnelId, payload, platformId) {
      return this.frpRun(() => adminClient.frpTunnelUpdate(tunnelId, payload), { detailId: platformId })
    },

    frpTunnelDelete(tunnelId, platformId) {
      return this.frpRun(() => adminClient.frpTunnelDelete(tunnelId), { detailId: platformId })
    },

    frpTunnelLock(tunnelId, payload, platformId) {
      return this.frpRun(() => adminClient.frpTunnelLock(tunnelId, payload), { detailId: platformId })
    },

    frpTunnelMigrate(tunnelId, nodeId, platformId) {
      return this.frpRun(() => adminClient.frpTunnelMigrate(tunnelId, nodeId), { detailId: platformId })
    },

    frpTunnelOffline(tunnelId, platformId) {
      return this.frpRun(() => adminClient.frpTunnelOffline(tunnelId), { detailId: platformId })
    },

    frpTunnelAuth(tunnelId, ip, platformId) {
      return this.frpRun(() => adminClient.frpTunnelAuth(tunnelId, ip), { detailId: platformId })
    },

    frpFlow(id, kind) {
      return adminClient.frpFlow(id, kind)
    },

    // ---------- 客户端托管（doc/13 §14）：面板经 SSH 在节点上跑 frpc 容器 ----------
    // 装载/卸载/同步都要落库，统一走 frpRun 防重复提交；纯读接口不包。
    frpDeployments(refreshLive = false) {
      return adminClient.frpDeployments(refreshLive)
    },

    frpDeployCreate(payload) {
      return this.frpRun(() => adminClient.frpDeployCreate(payload))
    },

    frpDeploySync(id) {
      return this.frpRun(() => adminClient.frpDeploySync(id))
    },

    frpDeployAction(id, action) {
      return this.frpRun(() => adminClient.frpDeployAction(id, action))
    },

    frpDeployStatus(id) {
      return adminClient.frpDeployStatus(id)
    },

    frpDeployLogs(id, tail = 200) {
      return adminClient.frpDeployLogs(id, tail)
    },

    frpDeployDelete(id, keep = false) {
      return this.frpRun(() => adminClient.frpDeployDelete(id, keep))
    },

    frpServerDocker(serverId, install = false) {
      return adminClient.frpServerDocker(serverId, install)
    },

    frpTunnelTraffic(id) {
      return adminClient.frpTunnelTraffic(id)
    },

    // 设备码授权：发起/轮询/取消。轮询由视图按固定间隔驱动，
    // 这里只做一次转发，不缓存会话（会话状态在后端内存里）
    deviceStart(reuseId = 0) {
      return adminClient.frpDeviceStart(reuseId)
    },

    devicePoll(sessionId) {
      return adminClient.frpDevicePoll(sessionId)
    },

    deviceCancel(sessionId) {
      return adminClient.frpDeviceCancel(sessionId)
    },

    // 免费二级域名（ChmlFrp）
    subdomains(platformId) {
      return adminClient.frpSubdomains(platformId)
    },

    availableDomains(platformId) {
      return adminClient.frpAvailableDomains(platformId)
    },

    async createSubdomain(platformId, payload) {
      const r = await adminClient.frpSubdomainCreate(platformId, payload)
      return r
    },

    async updateSubdomain(platformId, payload) {
      const r = await adminClient.frpSubdomainUpdate(platformId, payload)
      return r
    },

    async deleteSubdomain(platformId, domain, record) {
      return adminClient.frpSubdomainDelete(platformId, domain, record)
    },
  },
})
