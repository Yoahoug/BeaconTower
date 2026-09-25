<script setup>
import { RouterLink, useRoute } from 'vue-router'
import AppIcon from './AppIcon.vue'
import { useLiveServers } from '../composables/useLiveServers'
import { fmtBps } from '../utils/format'

const route = useRoute()
const { summary } = useLiveServers()

// 规划中的入口先以禁用态占位，落地后替换为 RouterLink
const soonItems = [
  { icon: 'server', label: '服务器' },
  { icon: 'pulse', label: '历史' },
]
</script>

<template>
  <aside class="sidebar">
    <RouterLink to="/" class="side-brand">
      <span class="side-logo"><AppIcon name="beacon" /></span>
      <span class="side-brand-text">
        <span class="side-name">BeaconTower</span>
        <span class="side-sub">信标塔监控</span>
      </span>
    </RouterLink>

    <nav class="side-nav">
      <div class="side-group">监控</div>
      <RouterLink to="/" class="side-item" :class="{ active: route.path === '/' }">
        <AppIcon name="dashboard" class="side-ico" />
        <span>总览</span>
      </RouterLink>

      <div v-for="item in soonItems" :key="item.label" class="side-item soon" :title="'规划中，后续版本提供'">
        <AppIcon :name="item.icon" class="side-ico" />
        <span>{{ item.label }}</span>
        <span class="soon-tag">规划中</span>
      </div>
    </nav>

    <div class="side-bottom">
      <div class="side-status">
        <div class="status-row">
          <span>在线</span>
          <span class="status-val ok">{{ summary.online }} / {{ summary.total }}</span>
        </div>
        <div class="status-row">
          <span>总吞吐</span>
          <span class="status-val">↑{{ fmtBps(summary.upBps) }}</span>
        </div>
        <div class="status-row">
          <span></span>
          <span class="status-val">↓{{ fmtBps(summary.downBps) }}</span>
        </div>
      </div>
      <div class="side-version">v0.1 · 原型演示</div>
    </div>
  </aside>
</template>
