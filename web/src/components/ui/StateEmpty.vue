<!-- 通用空态：图标 + 标题 + 描述 + 操作位（slot） -->
<script setup>
import AppIcon from '../AppIcon.vue'

defineProps({
  title: { type: String, default: '暂无数据' },
  desc: { type: String, default: '' },
  icon: { type: String, default: 'empty' },
})
</script>

<template>
  <div class="bt-empty" role="status">
    <AppIcon :name="icon" aria-hidden="true" />
    <div class="bt-empty__title">{{ title }}</div>
    <p v-if="desc" class="bt-empty__desc">{{ desc }}</p>
    <slot />
  </div>
</template>
