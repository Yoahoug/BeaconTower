<!-- ============================================================
     iOS 安装引导（doc/15）：Web 端无 beforeinstallprompt，轻量引导
     「分享 → 添加到主屏幕」。仅 iOS Safari 且非独立窗口时出现；
     可关闭并记住选择（localStorage）。
     ============================================================ -->
<script setup>
import { onMounted, ref } from 'vue'
import AppIcon from './AppIcon.vue'

const KEY = 'beacontower.pwa.hint.dismissed'
const visible = ref(false)

onMounted(() => {
  try {
    if (localStorage.getItem(KEY)) return
    const ua = navigator.userAgent || ''
    const isIOS = /iPad|iPhone|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
    const standalone =
      window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true
    const isSafari = /Safari/.test(ua) && !/CriOS|FxiOS|EdgiOS/.test(ua)
    visible.value = isIOS && isSafari && !standalone
  } catch {
    /* 隐私模式：不引导 */
  }
})

function dismiss() {
  visible.value = false
  try {
    localStorage.setItem(KEY, '1')
  } catch {
    /* 忽略 */
  }
}
</script>

<template>
  <Transition name="toast">
    <div v-if="visible" class="bt-install-hint" role="dialog" aria-label="安装到主屏幕引导">
      <button class="bt-install-hint__close" type="button" aria-label="不再提示" @click="dismiss">
        <AppIcon name="close" />
      </button>
      <p><strong>把信标塔装到主屏幕</strong></p>
      <p>点底部工具栏的「分享」，再选「添加到主屏幕」，即可全屏打开、离线看最后快照。</p>
    </div>
  </Transition>
</template>
