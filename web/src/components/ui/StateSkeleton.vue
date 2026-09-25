<!-- 通用骨架屏：行数可配，加载时占位避免布局跳动 -->
<script setup>
defineProps({
  rows: { type: Number, default: 3 },
})
</script>

<template>
  <div aria-busy="true" aria-label="内容加载中" style="display: flex; flex-direction: column; gap: 10px">
    <div v-for="i in rows" :key="i" class="bt-skeleton" :style="{ height: i === 1 ? '28px' : '16px', opacity: 1 - i * 0.12 }" />
  </div>
</template>
