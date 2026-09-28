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
  },
})
