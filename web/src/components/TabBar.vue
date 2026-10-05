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
</script>

<template>
  <nav class="bt-tabbar" aria-label="快捷导航">
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
