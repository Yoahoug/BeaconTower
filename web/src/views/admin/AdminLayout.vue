<!-- ============================================================
     管理端布局（v2.1）：内层浮动玻璃侧栏菜单 + 子页出口。
     - 左侧菜单承载管理子页（节点/WG 组网/采集/安全/审计），替代旧药丸 tab；
     - 顶部条保留面包屑 / 全网状态 / 管理员与退出登录；
     - 子页切换走 page-sub 轻转场；鉴权由路由守卫保证；
     - 移动端菜单折叠为顶部横滚条（复用 admin-tabs--inline）。
     ============================================================ -->
<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { resetAuthCache } from '../../api/auth'
import AppIcon from '../../components/AppIcon.vue'
import { useAdminStore } from '../../stores/admin'
import { useUiStore } from '../../stores/ui'

const router = useRouter()
const route = useRoute()
const admin = useAdminStore()
const ui = useUiStore()

const items = [
  { to: '/admin/servers', label: '节点管理', icon: 'server', desc: 'SSH 凭据与采集节点' },
  { to: '/admin/mesh', label: 'WG 组网', icon: 'layers', desc: '一键组网 · 主备 hub' },
  { to: '/admin/settings', label: '采集与展示', icon: 'sliders', desc: '间隔 / 阈值 / 公开页' },
  { to: '/admin/security', label: '安全与账号', icon: 'key', desc: '密码 / 会话 / 严格模式' },
  { to: '/admin/audit', label: '审计日志', icon: 'log', desc: '管理操作留痕' },
]

const current = computed(
  () => items.find((i) => route.path === i.to || route.path.startsWith(`${i.to}/`)) || items[0],
)

const title = computed(() => current.value.label)

function isActive(to) {
  return route.path === to || route.path.startsWith(`${to}/`)
}

async function logout() {
  try {
    await admin.logout()
  } finally {
    resetAuthCache()
    ui.notify('已退出登录')
    router.push('/admin/login')
  }
}

// 窄屏判定：≤960px 时侧栏转为顶部横滚菜单（与 AppShell 移动端断点一致）
const narrow = ref(false)
let mq
function onMq(e) {
  narrow.value = !e.matches
}
onMounted(() => {
  mq = window.matchMedia('(min-width: 961px)')
  narrow.value = !mq.matches
  mq.addEventListener('change', onMq)
})
onUnmounted(() => mq?.removeEventListener('change', onMq))
</script>

<template>
  <div class="admin-shell">
    <!-- 左侧管理菜单（桌面） -->
    <aside class="admin-menu" aria-label="管理端菜单">
      <div class="admin-menu__head">
        <span class="admin-menu__badge" aria-hidden="true"><AppIcon name="key" /></span>
        <span class="admin-menu__title">管理面板</span>
      </div>

      <nav class="admin-menu__nav">
        <RouterLink
          v-for="i in items"
          :key="i.to"
          :to="i.to"
          class="admin-menu__item"
          :class="{ 'is-active': isActive(i.to) }"
          :aria-current="isActive(i.to) ? 'page' : undefined"
        >
          <AppIcon :name="i.icon" aria-hidden="true" />
          <span class="admin-menu__text">
            <span>{{ i.label }}</span>
            <small>{{ i.desc }}</small>
          </span>
        </RouterLink>
      </nav>

      <div class="admin-menu__foot">
        <span class="bt-tag bt-tag--info">
          <AppIcon name="user" aria-hidden="true" />{{ admin.username || '管理员' }}
        </span>
        <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="logout">
          <AppIcon name="logout" aria-hidden="true" />退出登录
        </button>
      </div>
    </aside>

    <!-- 右侧内容区 -->
    <div class="admin-body">
      <header class="admin-topline">
        <div>
          <h1 class="admin-title">{{ title }}</h1>
          <p class="admin-desc">{{ current.desc }}</p>
        </div>
      </header>

      <!-- 窄屏：横滚菜单替代侧栏 -->
      <nav v-if="narrow" class="admin-tabs admin-tabs--inline" aria-label="管理端子页">
        <RouterLink
          v-for="i in items"
          :key="i.to"
          :to="i.to"
          class="admin-tab"
          :class="{ 'is-active': isActive(i.to) }"
          :aria-current="isActive(i.to) ? 'page' : undefined"
        >
          <AppIcon :name="i.icon" aria-hidden="true" />
          {{ i.label }}
        </RouterLink>
      </nav>

      <RouterView v-slot="{ Component, route: r }">
        <Transition name="page-sub" mode="out-in">
          <component :is="Component" :key="r.path" />
        </Transition>
      </RouterView>
    </div>
  </div>
</template>
