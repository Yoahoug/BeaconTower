// ============================================================
// BeaconTower · 界面偏好 Store（Pinia）
// 侧栏折叠 / 移动端抽屉 / 全局轻提示。持久化到 localStorage。
// ============================================================
import { defineStore } from 'pinia'

const KEY = 'beacontower.ui'

function read() {
  try {
    return JSON.parse(localStorage.getItem(KEY) || '{}')
  } catch {
    return {}
  }
}

export const useUiStore = defineStore('ui', {
  state: () => ({
    sidebarCollapsed: read().sidebarCollapsed ?? false,
    drawerOpen: false,
    toast: '', // 轻提示文案，空 = 不显示
    _toastTimer: 0,
  }),

  actions: {
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      this.persist()
    },

    setDrawer(open) {
      this.drawerOpen = open
    },

    notify(msg, ms = 2400) {
      this.toast = msg
      window.clearTimeout(this._toastTimer)
      this._toastTimer = window.setTimeout(() => {
        this.toast = ''
      }, ms)
    },

    persist() {
      try {
        localStorage.setItem(KEY, JSON.stringify({ sidebarCollapsed: this.sidebarCollapsed }))
      } catch {
        /* 隐私模式写失败不阻塞 */
      }
    },
  },
})
