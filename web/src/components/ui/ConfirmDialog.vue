<!-- ============================================================
     通用确认弹窗（a11y：role=dialog + 焦点管理 + ESC 关闭）
     用法：<ConfirmDialog title message @cancel @confirm :danger :busy />
     ============================================================ -->
<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import AppIcon from '../AppIcon.vue'

defineProps({
  title: { type: String, required: true },
  message: { type: String, required: true },
  confirmText: { type: String, default: '确认' },
  cancelText: { type: String, default: '取消' },
  danger: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
})

const emit = defineEmits(['cancel', 'confirm'])
const cancelBtn = ref(null)
let prevFocus = null

function onKey(e) {
  if (e.key === 'Escape') emit('cancel')
  // 简易焦点陷阱：Tab 在弹窗内循环
  if (e.key === 'Tab') {
    const root = e.currentTarget
    const focusables = root.querySelectorAll('button:not(:disabled)')
    if (!focusables.length) return
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
}

onMounted(() => {
  prevFocus = document.activeElement
  cancelBtn.value?.focus()
})

onUnmounted(() => {
  if (prevFocus && typeof prevFocus.focus === 'function') prevFocus.focus()
})
</script>

<template>
  <div class="bt-modal-mask" @click.self="$emit('cancel')" @keydown="onKey">
    <div class="bt-modal" role="dialog" aria-modal="true" :aria-label="title">
      <div class="bt-modal__head">
        <div class="bt-modal__title">{{ title }}</div>
      </div>
      <div class="bt-modal__body">
        <p class="bt-modal__desc">{{ message }}</p>
      </div>
      <div class="bt-modal__foot">
        <button ref="cancelBtn" class="bt-btn bt-btn--ghost" type="button" @click="$emit('cancel')">
          {{ cancelText }}
        </button>
        <button
          class="bt-btn"
          :class="danger ? 'bt-btn--danger' : 'bt-btn--primary'"
          type="button"
          :disabled="busy"
          @click="$emit('confirm')"
        >
          <AppIcon v-if="busy" name="refresh" class="is-spin" aria-hidden="true" />
          {{ busy ? '处理中…' : confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>
