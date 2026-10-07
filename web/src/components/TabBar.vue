<!-- ============================================================
     底部玻璃 tab bar（PWA 移动端，doc/15）
     - 仅窄视口（≤720px）渲染：桌面端布局零回归
     - 只放公开路由（概览/穿透/组网），遵守「公开 UI 不设任何管理入口」约束
     - env(safe-area-inset-bottom) 适配 iPhone 底部横条
     ============================================================ -->
<script setup>
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import AppIcon from './AppIcon.vue'

const route = useRoute()

const tabs = [
  { to: '/', label: '概览', icon: 'dashboard' },
  { to: '/tunnels', label: '穿透', icon: 'tunnel' },
  { to: '/mesh', label: '组网', icon: 'layers' },
]

const activeOf = (to) => (to === '/' ? route.path === '/' : route.path.startsWith(to))
const ariaOf = computed(() => (to) => (activeOf(to) ? 'page' : undefined))
// 注释里的约束（仅公开路由渲染）此前只靠 CSS 媒体查询，≤720px 时 /admin/*
// 页面也会显示公开 tab（与管理端布局重叠）。这里补路由级排除：登录/初始化/
// 管理面板一律隐藏，公开页回到纯 CSS 判定。
const hidden = computed(() => {
  const p = route.path
  return p.startsWith('/admin')
})
</script>

<template>
  <nav v-if="!hidden" class="bt-tabbar" aria-label="快捷导航">
    <RouterLink
      v-for="t in tabs"
      :key="t.to"
      :to="t.to"
      class="bt-tabbar__item"
      :class="{ 'is-active': activeOf(t.to) }"
      :aria-current="ariaOf(t.to)"
    >
      <AppIcon :name="t.icon" aria-hidden="true" />
      <span>{{ t.label }}</span>
    </RouterLink>
  </nav>
</template>
