// 3D 倾斜指令：鼠标进入后卡片随光标轻微偏转，离开后弹性复位。
// 同时输出 --mx/--my（光标在卡内的百分比坐标），驱动镜面高光跟随。
// 用法：v-tilt（注册为局部指令 vTilt）

const MAX_DEG = 4.5 // 最大偏转角度

const reduced =
  typeof window !== 'undefined' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

function onMove(e) {
  const el = e.currentTarget
  const rect = el.getBoundingClientRect()
  const px = (e.clientX - rect.left) / rect.width
  const py = (e.clientY - rect.top) / rect.height
  el.style.setProperty('--ry', `${((px - 0.5) * MAX_DEG * 2).toFixed(2)}deg`)
  el.style.setProperty('--rx', `${((0.5 - py) * MAX_DEG * 2).toFixed(2)}deg`)
  el.style.setProperty('--mx', `${(px * 100).toFixed(1)}%`)
  el.style.setProperty('--my', `${(py * 100).toFixed(1)}%`)
  el.classList.add('is-tilting') // 跟随阶段用短过渡，跟手不拖沓
}

function onLeave(e) {
  const el = e.currentTarget
  el.style.setProperty('--rx', '0deg')
  el.style.setProperty('--ry', '0deg')
  el.classList.remove('is-tilting') // 移除后回落到长过渡，弹性恢复
}

export const tilt = {
  mounted(el) {
    if (reduced) return
    el.classList.add('tilt')
    el.addEventListener('mousemove', onMove)
    el.addEventListener('mouseleave', onLeave)
  },
  unmounted(el) {
    el.removeEventListener('mousemove', onMove)
    el.removeEventListener('mouseleave', onLeave)
  },
}
