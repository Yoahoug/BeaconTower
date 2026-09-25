// ============================================================
// BeaconTower v2.0 · 数字补间 composable
// 监控数据每 10s 刷新一次：数值不跳变，而是沿 easeOutExpo 曲线
// 平滑滚动到位（KPI / 仪表环 / 功耗读数共用）。
// 性能：单个 rAF 循环，组件卸载即取消；不可见标签页由 store 暂停
// 轮询，补间自然不再触发。
// ============================================================
import { ref, watch, onMounted, onUnmounted } from 'vue'

/** 先极快后极缓的减速曲线：数字“冲出去再稳稳停住” */
export const easeOutExpo = (t) => (t >= 1 ? 1 : 1 - 2 ** (-10 * t))

const REDUCED = () =>
  typeof window !== 'undefined' &&
  window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

/**
 * 把 getter 返回的数值变成“平滑滚动”的响应式数值。
 * @param {() => number} getTarget 目标值 getter（通常读 store）
 * @param {{ duration?: number, ease?: (t:number)=>number }} options
 * @returns {import('vue').Ref<number>} 展示用数值（每帧更新）
 */
export function useTween(getTarget, { duration = 800, ease = easeOutExpo } = {}) {
  const display = ref(0)
  let raf = 0

  function animateTo(target) {
    cancelAnimationFrame(raf)
    const from = display.value
    if (from === target || REDUCED()) {
      display.value = target
      return
    }
    const start = performance.now()
    const step = (now) => {
      const t = Math.min(1, (now - start) / duration)
      display.value = from + (target - from) * ease(t)
      if (t < 1) raf = requestAnimationFrame(step)
    }
    raf = requestAnimationFrame(step)
  }

  onMounted(() => animateTo(Number(getTarget()) || 0))
  watch(() => Number(getTarget()) || 0, animateTo)
  onUnmounted(() => cancelAnimationFrame(raf))

  return display
}
