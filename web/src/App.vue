<!-- 应用根：极光背景层 + 路由出口 -->
<script setup>
import { RouterView } from 'vue-router'
import AuroraBackground from './components/aurora/AuroraBackground.vue'
</script>

<template>
  <AuroraBackground />
  <RouterView />
</template>
