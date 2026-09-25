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
  },
})
