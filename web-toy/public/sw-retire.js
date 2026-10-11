// BeaconTower 根 scope 退役 Service Worker：注销自身并清空全部缓存。
// 背景：v2.7 及之前 classic 的 Workbox 预缓存 SW 注册在根 scope；主题化后
// 已装 PWA 的老用户导航 / 会永远命中旧缓存。此文件由后端在 GET /sw.js
// 原样返回，浏览器更新检查取到后即完成迁出（与 router.go fallback 一致）。
self.addEventListener('install', function () { self.skipWaiting(); });
self.addEventListener('activate', function (event) {
  event.waitUntil((async function () {
    var keys = await caches.keys();
    for (var i = 0; i < keys.length; i++) { await caches.delete(keys[i]); }
    var list = await self.clients.matchAll({ type: 'window' });
    for (var j = 0; j < list.length; j++) {
      try { await list[j].navigate(list[j].url); } catch (e) { /* 隐藏窗口等场景忽略 */ }
    }
    await self.registration.unregister();
  })());
});
