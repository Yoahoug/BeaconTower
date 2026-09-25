<script setup>
import { RouterLink, RouterView } from 'vue-router'
import SideBar from './components/SideBar.vue'
</script>

<template>
  <div class="app-shell">
    <SideBar />
    <div class="main-area">
      <RouterView />
      <footer class="site-footer">
        <span>BeaconTower · 数据经加密 SSH 自动采集 · 公开页面不展示 IP 及敏感信息</span>
        <span>v0.1</span>
      </footer>
    </div>
  </div>
</template>
