// ============================================================
// BeaconTower v2.0 · 指针辉光指令（v-spotlight）
// 在元素上写入 --mx/--my CSS 变量，配合 .spot::before 的径向渐变
// 呈现“光随手动”的玻璃反光。rAF 合帧，卸载自动解绑。
// 用法：<div class="spot" v-spotlight>
// ============================================================
function attach(el) {
  let raf = 0
  let px = 0
  let py = 0

  const flush = () => {
    raf = 0
    el.style.setProperty('--mx', `${px}px`)
    el.style.setProperty('--my', `${py}px`)
  }

  const onMove = (e) => {
    const rect = el.getBoundingClientRect()
    px = e.clientX - rect.left
    py = e.clientY - rect.top
    if (!raf) raf = requestAnimationFrame(flush)
  }

  el.addEventListener('pointermove', onMove, { passive: true })
  el.__spotlightCleanup = () => {
    el.removeEventListener('pointermove', onMove)
    cancelAnimationFrame(raf)
  }
}

export const vSpotlight = {
  mounted(el) {
    attach(el)
  },
  unmounted(el) {
    el.__spotlightCleanup?.()
    delete el.__spotlightCleanup
  },
}
