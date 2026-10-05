# 15 · PWA 移动端（v2.5）

> 同一份 Vue 前端，iPhone「添加到主屏幕」后独立窗口运行，观感接近原生。
> 桌面端布局零回归（所有新增 UI 只在 ≤720px 视口生效）。
> 安全边界见 doc/14；本文只讲 PWA 与移动端体验。

## 1. PWA 基础设施

| 项 | 实现 |
| --- | --- |
| manifest | `web/public/manifest.webmanifest`（手写，字段语义可见）：`display: standalone`、`start_url: /`、`scope: /`、`theme_color #e8f0fd`、`background_color #edf1f9` |
| 图标 | `web/public/icons/`：192 / 512 / 512 maskable（满幅品牌渐变 + 信标缩至 78% 安全区）/ 180 apple-touch-icon，全部由 index.html 内联 SVG logo 生成（cairosvg），满幅渐变底避免系统给透明角填白 |
| Service Worker | `vite-plugin-pwa`（devDependency，generateSW 模式）：Workbox 预缓存 app shell（构建产物 ~30 个条目），`registerType: 'autoUpdate'`——新版本下载后下次导航生效，不打扰使用中用户 |
| API 缓存策略 | `/api/*` 一律 **NetworkOnly**（含 SSE）；`navigateFallback` 指向 index.html 并 denylist `/api/`。不用 NetworkFirst：断网时把旧缓存当新鲜数据展示，监控数据宁缺毋假——离线数据由 §3 的快照机制负责并明确标注 |
| MIME（Go 侧） | 路由层显式注册 `.webmanifest → application/manifest+json` 等；`/sw.js`、`/workbox-*.js` 以 JS MIME 服务（workbox 运行时在 dist 根，不在 /assets 下，掉进 SPA 回退会让 SW install 静默失败）；HEAD 与 GET 并行注册（gin 不自动派生 HEAD，iOS 安装预检走 HEAD） |
| index.html | manifest link、apple-touch-icon、`apple-mobile-web-app-capable/title/status-bar-style=black-translucent`、`viewport-fit=cover`（正文延伸到刘海/圆角外，由安全区 padding 兜底） |

## 2. 移动端原生感（≤720px 生效，桌面零回归）

- **底部玻璃 tab bar**（`components/TabBar.vue`）：只有公开路由（概览/穿透/组网），遵守「公开 UI 不设任何管理入口」约束；选中态与侧栏同语言（白色药丸 + 品牌色）；`env(safe-area-inset-bottom)` 适配 home indicator。
- **全局手势卫生**（`@media (pointer: coarse)`）：`-webkit-tap-highlight-color: transparent`（去 iOS 点按灰块）、`touch-action: manipulation`（消除双击缩放延迟）、`overscroll-behavior-y: none`（PWA 独立窗口下拉不露白底）。
- **安全区**：`.app-shell` 顶部 `env(safe-area-inset-top)`（配 black-translucent 状态栏）；内容区/页脚底部为 tab bar + home indicator 让位。
- **玻璃规范**：tab bar 与引导卡沿用 tokens 的玻璃变量（`--bt-glass-hi`/`--bt-glass-border` + backdrop-blur），并带 `prefers-reduced-transparency` 降级（doc/10 §3 要求）。

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
