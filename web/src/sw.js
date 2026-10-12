/// <reference lib="webworker" />
// BeaconTower · classic 主题 Service Worker（v2.8.3 起改为 injectManifest 自管；doc/15 修订）
//
// 为什么不沿用 generateSW 的默认形态（v2.8.1→v2.8.2 线上「旧壳钉死」实测复盘）：
//   1) 导航（HTML 壳）改为 NetworkFirst：在线永远拿服务器最新壳，离线才回退预缓存。
//      旧形态 createHandlerBoundToURL('index.html') 是「只认预缓存」——只要新 SW
//      因浏览器差异停在 waiting，用户就被旧壳钉死，发布会「看起来没生效」。
//   2) skipWaiting 放进 install 事件：vite-plugin-pwa 的 autoUpdate 会把它放在脚本
//      顶层执行，实测部分 Chromium 内核「promise resolve 但不生效」；install 期调用
//      才是各内核公认的可靠时机。
//   3) /api/** 完全不注册路由：请求不进 SW（SSE 长连也不在 SW 里挂待决 fetch 事件），
//      断网宁缺毋假、直连网络，也不给接管流程添乱。
import { cleanupOutdatedCaches, matchPrecache, precacheAndRoute } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'

// 新版本装完立即接管（install 期调用是规范与各内核公认的写法）
self.addEventListener('install', () => {
  self.skipWaiting()
})

// 页面侧催办：waiting 状态下收到 SKIP_WAITING 再跳一次等待（页面侧兜底用）
self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') self.skipWaiting()
})

// 接管后立刻控制现有页面（配合页面侧 controllerchange 刷新）
self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

// 关键顺序：导航路由必须先于 precacheAndRoute 注册。
// workbox 的 PrecacheRoute 会把 /classic/ 这类目录导航按 directoryIndex 归到
// index.html 上（等于「只认预缓存」），先注册哪一个，路由表就按谁先命中。
// 因此这里先挂 NetworkFirst 导航，再让预缓存兜其余请求（含离线回退）。
// 1.5s：正常网络在这个窗口内必定能拿到在线壳；超出即判慢网/断网，直接吃预缓存，
// 不让启动干等。预缓存的壳在 skipWaiting + clientsClaim 下随 SW 更新，兜底代价
// 只是「可能晚一个版本」，不会再复现 v2.8.1 那种被旧壳钉死。
const SHELL_TIMEOUT_MS = 1500
registerRoute(
  new NavigationRoute(
    async ({ request }) => {
      try {
        const timeout = new Promise((_, reject) =>
          setTimeout(() => reject(new Error('shell-timeout')), SHELL_TIMEOUT_MS),
        )
        return await Promise.race([fetch(request), timeout])
      } catch {
        return (await matchPrecache('index.html')) || Response.error()
      }
    },
    { denylist: [/^\/api\//] },
  ),
)

cleanupOutdatedCaches()
precacheAndRoute(self.__WB_MANIFEST)
