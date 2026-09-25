<!-- 通用错误块：图标 + 文案 + 重试按钮（a11y: role=alert） -->
<script setup>
import AppIcon from '../AppIcon.vue'

defineProps({
  message: { type: String, default: '加载失败，请重试' },
  retryText: { type: String, default: '重新加载' },
})

defineEmits(['retry'])
</script>

<template>
  <div class="bt-error-block" role="alert">
    <AppIcon name="warn" aria-hidden="true" />
    <div>{{ message }}</div>
    <button class="bt-btn bt-btn--default bt-btn--sm" type="button" @click="$emit('retry')">
      <AppIcon name="refresh" aria-hidden="true" />{{ retryText }}
    </button>
  </div>
</template>
