<!-- 404：未知路由的明确终点（渐变大字 + 仅返回总览；公开页不设管理入口） -->
<script setup>
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <div class="error-wrap">
    <div class="error-code tnum bt-enter" style="--i: 0">404</div>
    <h1 class="error-title bt-enter" style="--i: 1">页面不存在</h1>
    <p class="error-desc bt-enter" style="--i: 2">链接可能已失效或地址输入有误。请返回总览继续浏览。</p>
    <div class="error-actions bt-enter" style="--i: 3">
      <button class="bt-btn bt-btn--primary" type="button" @click="router.push('/')">返回总览</button>
    </div>
  </div>
</template>
