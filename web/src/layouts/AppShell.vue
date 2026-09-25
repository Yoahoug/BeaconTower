<!-- ============================================================
     应用骨架（v2.0 浮动玻璃）：浮动侧栏 + 悬浮顶栏 + 主内容 + 页脚
     - 路由切换走 page 转场（淡入上滑），数据轮询不重播
     - 顶栏进度光线在数据刷新时点亮（is-busy）
     - 面包屑由路由 meta.breadcrumb 驱动
     ============================================================ -->
<script setup>
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { useMonitorStore } from '../stores/monitor'
import { useUiStore } from '../stores/ui'
import { agoText } from '../utils/format'

const route = useRoute()
const monitor = useMonitorStore()
const ui = useUiStore()

const crumbs = computed(() => route.meta.breadcrumb || [{ label: '总览' }])

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
      </nav>

      <div class="app-side-foot">
        <div class="app-side-status" aria-label="全网实时状态">
          <span>节点在线</span>
          <strong class="ok tnum">{{ monitor.summary.online }} / {{ monitor.summary.total }}</strong>
          <span>实时功耗</span>
          <strong class="tnum">{{ monitor.summary.measuredCount ? `${monitor.summary.watts.toFixed(1)} W` : '—' }}</strong>
        </div>
        <div class="app-side-ver">v2.0 · SKY BEACON</div>
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
        <span class="tnum">v2.0 · Sky Beacon</span>
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
