<!-- 应用根：只做路由出口，骨架下沉到 layouts/AppShell.vue -->
<script setup>
import { RouterView } from 'vue-router'
</script>

<template>
  <RouterView />
</template>
