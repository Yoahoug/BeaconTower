<!-- ============================================================
     应用骨架（v2.0 浮动玻璃）：浮动侧栏 + 悬浮顶栏 + 主内容 + 页脚
     - 路由切换走 page 转场（淡入上滑），数据轮询不重播
     - 顶栏进度光线在数据刷新时点亮（is-busy）
     - 面包屑由路由 meta.breadcrumb 驱动
     - 管理菜单并入主侧栏（v2.2）：仅登录后的 /admin 子页出现
       「管理面板」分组与退出登录，公开页不暴露任何管理入口
     ============================================================ -->
<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { getCachedStatus, resetAuthCache } from '../api/auth'
import { useMonitorStore } from '../stores/monitor'
import { useUiStore } from '../stores/ui'
import { agoText } from '../utils/format'

const route = useRoute()
const router = useRouter()
const monitor = useMonitorStore()
const ui = useUiStore()

const crumbs = computed(() => route.meta.breadcrumb || [{ label: '总览' }])

// ---------- 管理端导航（并入主侧栏；仅在登录后的 /admin 子页出现，公开页不暴露入口） ----------
const adminNav = [
  { to: '/admin/servers', label: '节点管理', icon: 'server' },
  { to: '/admin/mesh', label: 'WG 组网', icon: 'layers' },
  { to: '/admin/frp', label: '内网穿透', icon: 'tunnel' },
  { to: '/admin/settings', label: '采集与展示', icon: 'sliders' },
  { to: '/admin/security', label: '安全与账号', icon: 'key' },
  { to: '/admin/audit', label: '审计日志', icon: 'log' },
]

const inAdmin = computed(() => route.meta.requiresAuth === true)
const adminName = ref('')

function isActiveAdmin(to) {
  return route.path === to || route.path.startsWith(`${to}/`)
}

// 用户名复用守卫的 status 缓存（30s），进入管理端不额外打接口
watch(
  inAdmin,
  async (on) => {
    if (!on) {
      adminName.value = ''
      return
    }
    try {
      adminName.value = (await getCachedStatus())?.username || ''
    } catch {
      adminName.value = ''
    }
  },
  { immediate: true },
)

async function logout() {
  try {
    // 动态引入管理 store：公开首屏 bundle 不含管理代码
    const { useAdminStore } = await import('../stores/admin')
    await useAdminStore().logout()
  } catch {
    // 请求没到服务端就报错：会话其实还有效，跳登录页会被守卫弹回来；
    // 如实提示，留在当前页让用户重试
    resetAuthCache()
    ui.notify('退出失败，请重试')
    return
  }
  resetAuthCache()
  ui.notify('已退出登录')
  router.push('/admin/login')
}

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
  window.addEventListener('keydown', onKey)
})

onUnmounted(() => {
  monitor.stop()
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <a class="skip-link" href="#main-content">跳到主内容</a>
  <div class="app-shell">
    <!-- 移动端遮罩 -->
    <Transition name="modal">
      <div v-if="ui.drawerOpen" class="app-scrim" @click="closeDrawer" aria-hidden="true" />
    </Transition>

    <aside class="app-sidebar" :class="{ 'is-open': ui.drawerOpen }" aria-label="主导航">
      <RouterLink to="/" class="app-brand" @click="closeDrawer">
        <span class="app-brand__logo bt-beacon-glow" aria-hidden="true"><AppIcon name="beacon" /></span>
        <span class="app-brand__text">
          <span class="app-brand__name">BeaconTower</span>
          <span class="app-brand__sub">信标塔 · 全网监控</span>
        </span>
      </RouterLink>

      <nav class="app-nav">
        <div class="app-nav__group">监控</div>
        <RouterLink
          to="/"
          class="app-nav__item"
          :class="{ 'is-active': route.path === '/' }"
          :aria-current="route.path === '/' ? 'page' : undefined"
          @click="closeDrawer"
        >
          <AppIcon name="dashboard" aria-hidden="true" />
          <span>状态总览</span>
        </RouterLink>
        <RouterLink
          to="/tunnels"
          class="app-nav__item"
          :class="{ 'is-active': route.path === '/tunnels' }"
          :aria-current="route.path === '/tunnels' ? 'page' : undefined"
          title="穿透状态"
          @click="closeDrawer"
        >
          <AppIcon name="tunnel" aria-hidden="true" />
          <span>穿透状态</span>
        </RouterLink>

        <template v-if="inAdmin">
          <div class="app-nav__group">管理面板</div>
          <RouterLink
            v-for="i in adminNav"
            :key="i.to"
            :to="i.to"
            class="app-nav__item"
            :class="{ 'is-active': isActiveAdmin(i.to) }"
            :aria-current="isActiveAdmin(i.to) ? 'page' : undefined"
            :title="i.label"
            @click="closeDrawer"
          >
            <AppIcon :name="i.icon" aria-hidden="true" />
            <span>{{ i.label }}</span>
          </RouterLink>
          <button class="app-nav__item app-nav__item--action" type="button" title="退出登录" @click="logout">
            <AppIcon name="logout" aria-hidden="true" />
            <span>退出登录</span>
          </button>
        </template>
      </nav>

      <div class="app-side-foot">
        <div v-if="inAdmin" class="app-side-user">
          <AppIcon name="user" aria-hidden="true" />
          <span>{{ adminName || '管理员' }}</span>
        </div>
        <div class="app-side-status" aria-label="全网实时状态">
          <span>节点在线</span>
          <strong class="ok tnum">{{ monitor.summary.online }} / {{ monitor.summary.total }}</strong>
          <span>实时功耗</span>
          <strong class="tnum">{{ monitor.summary.measuredCount ? `${monitor.summary.watts.toFixed(1)} W` : '—' }}</strong>
        </div>
        <div class="app-side-ver">v2.3 · SKY BEACON</div>
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
            <span
              class="pulse-dot"
              :class="{ 'pulse-dot--still': !!monitor.summary.offline }"
              aria-hidden="true"
            />
            {{ monitor.summary.offline ? `${monitor.summary.offline} 台离线` : '全网正常' }}
          </span>
        </div>
      </header>
      <!-- 数据刷新进度光线（静默刷新反馈，平时透明） -->
      <div class="bt-progress" :class="{ 'is-busy': monitor.status === 'loading' }" aria-hidden="true" />

      <main id="main-content" class="app-content" tabindex="-1">
        <RouterView v-slot="{ Component, route: r }">
          <Transition name="page" mode="out-in">
            <component :is="Component" :key="r.path" />
          </Transition>
        </RouterView>
      </main>

      <footer class="app-footer">
        <span>BeaconTower · 数据经加密 SSH 自动采集 · 公开页面不展示 IP 及敏感信息</span>
        <span class="tnum">v2.3 · Sky Beacon</span>
      </footer>
    </div>

    <!-- 全局轻提示（上滑淡入） -->
    <Transition name="toast">
      <div v-if="ui.toast" class="app-toast" role="status" aria-live="polite">
        {{ ui.toast }}
      </div>
    </Transition>
  </div>
</template>
