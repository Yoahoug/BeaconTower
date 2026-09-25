<script setup>
// 应用骨架：侧栏 + 顶栏 + 主内容 + 页脚 + 移动抽屉 + 全局提示。
// 所有页面共用此布局；面包屑由路由 meta.breadcrumb 驱动。
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { useMonitorStore } from '../stores/monitor'
import { useAdminStore } from '../stores/admin'
import { useUiStore } from '../stores/ui'
import { agoText } from '../utils/format'

const route = useRoute()
const router = useRouter()
const monitor = useMonitorStore()
const admin = useAdminStore()
const ui = useUiStore()

const crumbs = computed(() => route.meta.breadcrumb || [{ label: '总览' }])
const isAdminSection = computed(() => route.path.startsWith('/admin'))

function toggleDrawer() {
  ui.setDrawer(!ui.drawerOpen)
}

function closeDrawer() {
  ui.setDrawer(false)
}

function onKey(e) {
  if (e.key === 'Escape' && ui.drawerOpen) closeDrawer()
}

// 路由切换时自动收起移动抽屉
watch(() => route.fullPath, closeDrawer)

onMounted(() => {
  monitor.start()
  admin.checkAuth().catch(() => {})
  window.addEventListener('keydown', onKey)
})

onUnmounted(() => {
  monitor.stop()
  window.removeEventListener('keydown', onKey)
})

function goLogin() {
  router.push('/admin/login')
}
</script>

<template>
  <a class="skip-link" href="#main-content">跳到主内容</a>
  <div class="app-shell">
    <!-- 移动端遮罩 -->
    <div v-if="ui.drawerOpen" class="app-scrim" @click="closeDrawer" aria-hidden="true" />

    <aside class="app-sidebar" :class="{ 'is-open': ui.drawerOpen }" aria-label="主导航">
      <RouterLink to="/" class="app-brand" @click="closeDrawer">
        <span class="app-brand__logo" aria-hidden="true"><AppIcon name="beacon" /></span>
        <span class="app-brand__text">
          <span class="app-brand__name">BeaconTower</span>
          <span class="app-brand__sub">信标塔监控</span>
        </span>
      </RouterLink>

      <nav class="app-nav">
        <div class="app-nav__group">监控</div>
        <RouterLink to="/" class="app-nav__item" :class="{ 'is-active': route.path === '/' }" @click="closeDrawer">
          <AppIcon name="dashboard" aria-hidden="true" />
          <span>总览</span>
        </RouterLink>

        <div class="app-nav__group">管理</div>
        <RouterLink
          to="/admin/servers"
          class="app-nav__item"
          :class="{ 'is-active': isAdminSection }"
          @click="closeDrawer"
        >
          <AppIcon name="sliders" aria-hidden="true" />
          <span>管理面板</span>
        </RouterLink>
      </nav>

      <div class="app-side-foot">
        <div class="app-side-status" aria-label="全网实时状态">
          <span>在线</span>
          <strong class="ok tnum">{{ monitor.summary.online }} / {{ monitor.summary.total }}</strong>
          <span>实时功耗</span>
          <strong class="tnum">{{ monitor.summary.measuredCount ? `${monitor.summary.watts.toFixed(1)} W` : '—' }}</strong>
        </div>
        <div class="app-side-ver">v1.0 · 企业版重构</div>
      </div>
    </aside>

    <div class="app-main">
      <header class="app-topbar">
        <button
          class="bt-btn bt-btn--ghost bt-btn--sm app-topbar__menu-btn"
          type="button"
          :aria-expanded="ui.drawerOpen"
          aria-controls="main-content"
          aria-label="打开导航菜单"
          @click="toggleDrawer"
        >
          <AppIcon name="menu" />
        </button>

        <nav aria-label="面包屑">
          <ol class="app-breadcrumb">
            <li v-for="(c, i) in crumbs" :key="i">
              <RouterLink v-if="c.to && i < crumbs.length - 1" :to="c.to">{{ c.label }}</RouterLink>
              <span v-else :aria-current="i === crumbs.length - 1 ? 'page' : undefined">{{ c.label }}</span>
            </li>
          </ol>
        </nav>

        <div class="app-topbar__spacer" />

        <div class="app-topbar__meta">
          <span class="hide-sm tnum">更新于 {{ agoText(monitor.secondsSinceUpdate) }}</span>
          <span class="bt-tag" :class="monitor.summary.offline ? 'bt-tag--danger' : 'bt-tag--success'">
            <span class="dot" aria-hidden="true"></span>
            {{ monitor.summary.offline ? `${monitor.summary.offline} 台离线` : '全网正常' }}
          </span>
          <button
            v-if="!admin.loggedIn"
            class="bt-btn bt-btn--default bt-btn--sm"
            type="button"
            @click="goLogin"
          >
            管理员登录
          </button>
          <span v-else class="app-topbar__user" :title="`已登录：${admin.username}`">
            <span class="app-topbar__avatar" aria-hidden="true">{{ (admin.username || 'A').slice(0, 1).toUpperCase() }}</span>
            <span class="ellipsis" style="max-width: 96px">{{ admin.username }}</span>
          </span>
        </div>
      </header>

      <main id="main-content" class="app-content" tabindex="-1">
        <RouterView />
      </main>

      <footer class="app-footer">
        <span>BeaconTower · 数据经加密 SSH 自动采集 · 公开页面不展示 IP 及敏感信息</span>
        <span class="tnum">v1.0 · 企业级前端</span>
      </footer>
    </div>

    <!-- 全局轻提示 -->
    <div
      v-if="ui.toast"
      role="status"
      aria-live="polite"
      style="position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%); z-index: 80; background: #161b26; color: #fff; padding: 9px 16px; border-radius: 8px; font-size: 13px; box-shadow: 0 8px 28px rgba(16,24,40,.25);"
    >
      {{ ui.toast }}
    </div>
  </div>
</template>
