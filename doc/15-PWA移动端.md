# 15 · PWA 移动端（v2.5，v2.8.3 修订见 §8）

> 同一份 Vue 前端，iPhone「添加到主屏幕」后独立窗口运行，观感接近原生。
> 桌面端布局零回归（所有新增 UI 只在 ≤720px 视口生效）。
> 安全边界见 doc/14；本文只讲 PWA 与移动端体验。

## 1. PWA 基础设施

| 项 | 实现 |
| --- | --- |
| manifest | `web/public/manifest.webmanifest`（手写，字段语义可见）：`display: standalone`、`start_url: /`、`scope: /`；`theme_color` / `background_color` 随设计系统基调取值（v3 深色重构后需同步更新） |
| 图标 | `web/public/icons/`：192 / 512 / 512 maskable（满幅品牌渐变 + 信标缩至 78% 安全区）/ 180 apple-touch-icon，全部由 index.html 内联 SVG logo 生成（cairosvg），满幅渐变底避免系统给透明角填白 |
| Service Worker | `vite-plugin-pwa`（devDependency，**injectManifest** 模式，v2.8.3 起自管源码 `web/src/sw.js`）：Workbox 预缓存构建产物（31 条目，含 `index.html`）+ **导航 NetworkFirst**（1.5s 上限：在线必拿服务器最新壳，慢网/断网才回退预缓存）；`skipWaiting()` 在 install 事件内调用、`activate` 时 `clientsClaim()`；`rollupFormat: 'iife'` 产出单文件经典脚本（不依赖 Safari 16.4+ 的模块化 SW）。详见 §8 |
| API 缓存策略 | `/api/*` **不注册任何 SW 路由**：请求不进 SW（SSE 长连也不在 SW 里挂待决 fetch 事件），断网宁缺毋假，离线数据由 §3 的快照机制负责并明确标注。壳不做「只认预缓存」的 `navigateFallback`——那正是 §8「旧壳钉死」的成因 |
| MIME（Go 侧） | 路由层显式注册 `.webmanifest → application/manifest+json` 等；`/sw.js`、`/workbox-*.js` 以 JS MIME 服务（workbox 运行时在 dist 根，不在 /assets 下，掉进 SPA 回退会让 SW install 静默失败）；HEAD 与 GET 并行注册（gin 不自动派生 HEAD，iOS 安装预检走 HEAD） |
| index.html | manifest link、apple-touch-icon、`apple-mobile-web-app-capable/title/status-bar-style=black-translucent`、`viewport-fit=cover`（正文延伸到刘海/圆角外，由安全区 padding 兜底）；主题 boot 脚本 `<script defer src="/boot-theme.js">`（v2.8.3 起由阻塞改 defer：主题同步仍在模块执行前完成，首屏少一次串行往返） |

## 2. 移动端原生感（≤720px 生效，桌面零回归）

- **底部玻璃 tab bar**（`components/TabBar.vue`）：只有公开路由（概览/穿透/组网），遵守「公开 UI 不设任何管理入口」约束；选中态与侧栏同语言（白色药丸 + 品牌色）；`env(safe-area-inset-bottom)` 适配 home indicator。
- **全局手势卫生**（`@media (pointer: coarse)`）：`-webkit-tap-highlight-color: transparent`（去 iOS 点按灰块）、`touch-action: manipulation`（消除双击缩放延迟）、`overscroll-behavior-y: none`（PWA 独立窗口下拉不露白底）。
- **安全区**：`.app-shell` 顶部 `env(safe-area-inset-top)`（配 black-translucent 状态栏）；内容区/页脚底部为 tab bar + home indicator 让位。
- **玻璃质感**：tab bar 与引导卡沿用当前设计系统的玻璃配方（backdrop-blur + 半透明描边 + 顶部高光），并带 `prefers-reduced-transparency` 降级。

## 3. 离线体验（断网不白屏）

- 最后一次成功的 `/api/v1/public/servers` 全量响应落 **localStorage**（`beacontower.snapshot.v1`，原始 JSON 单键，几十 KB 级）。选 localStorage 而非 IndexedDB：单键 JSON、同步读写零 await、启动路径零改造；IndexedDB 的事务/版本管理对这个量级是过度设计。
- 打开流程：先渲染快照（顶栏出现「离线 · 数据时间 HH:MM」警告标签）→ 再刷 SSE/轮询，实时数据到达后标签自动消失。
- 快照超过 7 天视为过期清除（避免把一周前的数据当现状误导）；写失败（隐私模式/配额）静默跳过。
- app shell 本身由 SW precache 保证离线可加载（实测：停掉后端进程后 reload，总览页正常渲染快照数据）。

## 4. SSE 与前后台切换

`stores/monitor.js` 的 visibilitychange 处理：

- **切后台**：主动 `closeSSE()`。iOS 切后台不保证触发 EventSource 的 onerror，被动等待会堆积死连接（对齐 doc/14 的 SSE 并发上限，客户端自己先收敛）。
- **回前台**：立即 `connectSSE()` 重连 + `fetchAll()` 刷新一次，先渲染再等推送。

## 5. iOS 安装引导（`components/InstallHint.vue`）

Web 端没有 beforeinstallprompt（那是 Chromium 的），iOS 只能引导用户手动操作：

- 触发条件（三者同时满足）：iOS UA（含 iPadOS 桌面态 UA 的 maxTouchPoints 判定）、Safari（排除 CriOS/FxiOS/EdgiOS）、非 standalone 模式。
- 文案引导「分享 → 添加到主屏幕」，右上角可关闭，`localStorage['beacontower.pwa.hint.dismissed']` 记住不再弹。
- 已安装（standalone）用户永不看见。

## 6. 验证记录（2026-10-05 本地实例）

| 项 | 结果 |
| --- | --- |
| `npm run build` | 通过；dist 无 sourcemap；precache 30 entries |
| `go build ./...`（embed dist） | 通过 |
| manifest MIME | `application/manifest+json; charset=utf-8`（GET/HEAD 一致） |
| sw.js / workbox MIME | `text/javascript` |
| SW 注册 | `serviceWorker.controller = true`，scope `/`，caches 30 条 |
| 移动视口 390×844 走查 | 总览/穿透/组网/节点详情无横向滚动；tab bar 选中态正确 |
| 桌面视口 1440×900 回归 | tab bar 不渲染、侧栏正常、菜单按钮隐藏 |
| 离线（停后端进程）reload | app shell 来自 precache，总览页渲染快照 + 「离线 · 数据时间」标签，不白屏 |
| 恢复在线 | 标签消失，SSE 重连，数据回到实时 |

## 7. iPhone 真机验收清单（部署 https://tower.yoahoug.dev/ 后逐项过）

- [ ] Safari 打开站点，底部出现「分享 → 添加到主屏幕」引导卡（首次），可关闭且刷新后不再出现
- [ ] 添加到主屏幕：图标为信标 logo（无白边/黑边），名称「信标塔」
- [ ] 从主屏幕打开：独立窗口（无 Safari 工具栏），状态栏黑字半透明，正文延伸到状态栏/圆角外但内容不被裁
- [ ] 底部 tab bar 悬浮，与 home indicator 不重叠；三个 tab 切换正常、选中态正确
- [ ] 总览页上下滚动流畅，无橡皮筋露底，卡片无横向滚动
- [ ] SSE 实时刷新：数字自动更新（对比两台设备或与服务器日志）；切后台再回来 2s 内恢复更新
- [ ] 断网（开飞行模式）后重开 App：显示最后快照 + 「离线 · 数据时间」提示，不白屏
- [ ] 关飞行模式：提示消失，数据恢复实时
- [ ] 节点详情/穿透/组网页返回导航正常；深链（如 /server/1）从主屏打开可直达
- [ ] 锁屏半小时后打开：自动刷新到最新数据，无卡死
- [ ] 侧栏抽屉：右下角菜单按钮打开后不被顶栏/底部 tab bar 遮挡，点遮罩可关
- [ ] 发版后（服务端已更新）打开 App 即为新界面；版本串见侧栏底部

## 8. v2.8.3 修订（2026-10-12）：SW 自管，收口「旧壳钉死」

**症状**：classic 主题连发两版（v2.8.1 白屏修复、v2.8.2 抽屉层级）后，手机从主屏 PWA
启动始终是旧界面；杀进程重开、Safari 强刷都无效——同一域名在桌面浏览器打开却已是新版。

**根因**（两层，缺一不可；本地以「部署后重启容器 + 重开页面」稳定复现）：

1. **壳只认预缓存**。generateSW 默认产出 `createHandlerBoundToURL('index.html')`，导航
   命中 Workbox 的 PrecacheRoute 后完全不看网络——只要「接管」没发生，界面就被预缓存里
   的旧壳钉死，与服务端已发布的新版无关。
2. **新 SW 不接管**。`registerType: 'autoUpdate'` 把 `skipWaiting()` 放在脚本顶层执行，
   实测部分 Chromium 内核（含 iOS 侧 WebView）「promise resolve 但不生效」：install 完成
   即停在 waiting；postMessage 催办、关闭全部客户端、页面侧 `reg.update()` 都推不动它。

**修复**（细节见提交 `73eb3ef`）：

- `web/vite.config.js`：`strategies: 'injectManifest'` + `rollupFormat: 'iife'`，SW 源码
  `web/src/sw.js`：
  - 导航改 **NetworkFirst（1.5s 上限）**：在线必拿服务器最新壳，慢网/断网才回退预缓存。
    1.5s 是「启动不干等」与「壳够新鲜」的折中；兜底的预缓存壳在 skipWaiting 生效后
    已随 SW 更新，代价只是可能晚一个版本；
  - **路由注册顺序：先导航路由、后 `precacheAndRoute`**。反过来的话 PrecacheRoute 会按
    `directoryIndex` 先认领 `/classic/` 这类目录导航，NetworkFirst 永远轮不到；
  - `skipWaiting()` 移入 install 事件（规范与各内核公认时机）+ `message` 催办兜底；
  - `activate` 时 `clientsClaim()`：新 SW 立即接管已开页面；
  - `/api/**` 不注册任何路由。
- `web/src/pwa.js`（新）：页面侧更新治理——启动与回前台 `reg.update()`（10 分钟节流）、
  发现 waiting 就 `SKIP_WAITING` 催办、催办 4s 仍未接管则一次性自愈（注销 + 重载，
  sessionStorage 记时 60s 内不重复）。
- `web/index.html` / `web-toy/index.html`：boot 脚本阻塞改 `defer`。
- 显式声明 `workbox-precaching` / `workbox-routing`（此前靠 vite-plugin-pwa 传递依赖）。
- `web/vite.config.js` 加一个构建后补回 `dist/.placeholder` 的小插件（vite 清空 outDir
  会删掉这个 go:embed 兜底文件，此前每次构建都在 git 里显示「已删除」）。

**验证**（本机浏览器 + 线上域名；均先清空注册与缓存模拟首装）：

| 项 | 结果 |
| --- | --- |
| 首装 | `active` 立即接管（clientsClaim），预缓存 31 条目，含 `index.html`（键名带 `?__WB_REVISION__=` 后缀） |
| 在线导航 | 受控且 `transferSize>0`（来自网络），版本串 v2.8.3；域名侧 TTFB 434ms |
| 断网导航 | `transferSize=0`（预缓存合成壳），应用正常挂载，用时 489ms（不干等超时） |
| 卡死自愈 | 注销 + 重载后必得新壳（原死局路径） |
| 移动端 390×844 | 抽屉 z=31 / 遮罩 30 高于顶栏与 tab bar 20；展开后命中面在抽屉内；安全区上下各 8px |

**旧版自愈（已装 PWA 的手机，一次性）**：v2.8.1 / v2.8.2 的旧壳没有自愈能力，需人工触发
一次——从主屏 App 上滑彻底关闭后重开；若仍是旧界面，则删除主屏图标后重新「添加到主屏幕」
（或在 Safari 的「网站数据」里删除本站数据）。此后新架构（NetworkFirst 壳 + 自愈）不再需要
人工干预。

**教训**：① PWA 的「发布生效」取决于 **SW 生命周期**，不是 HTTP 缓存——壳一旦只认预缓存，
就等于把发布权交给浏览器；② `skipWaiting` 放在哪是兼容性问题，不是代码风格问题；
③ 判断「这份壳来自网络还是缓存」看 `performance.getEntriesByType('navigation')[0].transferSize`
（>0 走网络），比盯着界面猜省事。
