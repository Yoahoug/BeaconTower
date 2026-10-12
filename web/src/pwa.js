// PWA 注册与更新治理（v2.8.3；doc/15 修订）
//
// 背景：classic 主题的预缓存壳曾出现「新版本已发布、手机仍是旧界面」——
// 根因是新 SW 装在 waiting 里没接管，而壳又只认预缓存。除 SW 侧改为
// 导航 NetworkFirst（web/src/sw.js）外，页面侧补三件事：
//   1) 启动与回前台主动查更新：iOS 独立窗口的自动更新检查很懒，可能几天不查；
//   2) 发现「装了但没接管」：按规范用 SKIP_WAITING 催办（waiting 期调用合法）；
//   3) 催办 4s 仍不接管：一次性自愈（注销 + 重载 → 重新注册即首装，必然激活）。
//      自愈 60s 内只做一次，避免任何循环。
import { registerSW } from 'virtual:pwa-register'

const HEAL_KEY = 'bt_sw_heal_at'
const HEAL_MIN_GAP_MS = 60 * 1000
const HEAL_GRACE_MS = 4000
const UPDATE_MIN_GAP_MS = 10 * 60 * 1000

function nudgeOrHeal(reg) {
  if (!reg || !reg.waiting) return
  try {
    reg.waiting.postMessage({ type: 'SKIP_WAITING' })
  } catch {
    /* 极旧内核不支持也无妨：下面自愈兜底 */
  }
  setTimeout(async () => {
    if (!reg.waiting) return // 已接管（控制台/日志可见 controllerchange）
    let last = 0
    try {
      last = Number(sessionStorage.getItem(HEAL_KEY) || 0)
    } catch {
      /* 隐私模式无 sessionStorage：仍允许自愈，只是没有节流记录 */
    }
    if (Date.now() - last < HEAL_MIN_GAP_MS) return
    try {
      sessionStorage.setItem(HEAL_KEY, String(Date.now()))
    } catch {
      /* 同上 */
    }
    try {
      await reg.unregister()
    } catch {
      /* 注销失败也照样重载：下一轮注册仍会走一遍更新检查 */
    }
    location.reload()
  }, HEAL_GRACE_MS)
}

export function setupPwa() {
  if (!('serviceWorker' in navigator)) return
  registerSW({
    immediate: true,
    onRegisteredSW(_swUrl, reg) {
      if (!reg) return
      nudgeOrHeal(reg)
      reg.addEventListener('updatefound', () => {
        const installing = reg.installing
        if (!installing) return
        installing.addEventListener('statechange', () => {
          if (installing.state === 'installed' && reg.waiting) nudgeOrHeal(reg)
        })
      })
      // 启动 + 回前台主动查更新（节流 10 分钟）
      let lastAt = 0
      const kick = () => {
        if (document.visibilityState !== 'visible') return
        if (Date.now() - lastAt < UPDATE_MIN_GAP_MS) return
        lastAt = Date.now()
        reg.update().catch(() => {})
      }
      kick()
      document.addEventListener('visibilitychange', kick)
    },
  })
}
