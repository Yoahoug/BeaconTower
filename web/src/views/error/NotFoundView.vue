<!-- 404：未知路由的明确终点（替代旧静默回首页，便于排障与体验） -->
<script setup>
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <div class="error-wrap">
    <div class="error-code tnum">404</div>
    <h1 class="error-title">页面不存在</h1>
    <p class="error-desc">链接可能已失效或地址输入有误。你可以返回总览，或检查管理端导航后重试。</p>
    <div class="error-actions">
      <button class="bt-btn bt-btn--primary" type="button" @click="router.push('/')">返回总览</button>
      <button class="bt-btn bt-btn--default" type="button" @click="router.push('/admin/servers')">前往管理面板</button>
    </div>
  </div>
</template>
