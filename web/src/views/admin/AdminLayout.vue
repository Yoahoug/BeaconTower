<!-- ============================================================
     管理端布局：页头 + 下划线标签导航（嵌套路由） + 子页出口。
     鉴权由路由守卫保证，进入此时必已登录。
     ============================================================ -->
<script setup>
import { useRouter, RouterLink, RouterView, useRoute } from 'vue-router'
import AppIcon from '../../components/AppIcon.vue'
import { useAdminStore } from '../../stores/admin'
import { useUiStore } from '../../stores/ui'

const router = useRouter()
const route = useRoute()
const admin = useAdminStore()
const ui = useUiStore()

const tabs = [
  { to: '/admin/servers', label: '节点管理', icon: 'server' },
  { to: '/admin/settings', label: '采集与展示', icon: 'sliders' },
  { to: '/admin/security', label: '安全与账号', icon: 'key' },
  { to: '/admin/audit', label: '审计日志', icon: 'log' },
]

function isActive(to) {
  return route.path === to || route.path.startsWith(`${to}/`)
}

async function logout() {
  await admin.logout()
  ui.notify('已退出登录')
  router.push('/')
}
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>管理面板</h1>
        <p class="page-head__desc">SSH 凭据加密存储 · 公开页永不返回敏感字段 · 本原型数据保存在浏览器本地</p>
      </div>
      <div class="page-head__actions">
        <span class="bt-tag bt-tag--outline">管理员 · {{ admin.username || '—' }}</span>
        <button class="bt-btn bt-btn--ghost bt-btn--sm" type="button" @click="logout">
          <AppIcon name="key" aria-hidden="true" />退出登录
        </button>
      </div>
    </div>

    <nav class="admin-tabs" aria-label="管理端子页">
      <RouterLink
        v-for="t in tabs"
        :key="t.to"
        :to="t.to"
        class="admin-tab"
        :class="{ 'is-active': isActive(t.to) }"
        :aria-current="isActive(t.to) ? 'page' : undefined"
      >
        <AppIcon :name="t.icon" aria-hidden="true" />
        {{ t.label }}
      </RouterLink>
    </nav>

    <RouterView />
  </div>
</template>
