package router

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/Yoahoug/BeaconTower/internal/handler"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/gin-gonic/gin"
)

// ---------- 主题托管拓扑（doc/18） ----------
// v2.9 起根路径 "/" = 默认主题 Sky Glass（classic）；其余主题挂独立前缀
// （toy → /toy/），资源与根文件按前缀作用域隔离，SPA 回退分别落到各自的
// index.html。旧 /classic/* 地址 302 回根路径，已装 PWA 的书签不失效。
// 新增主题三步：前端新目录产出 dist → main.go 加 embed 并在 registerThemes 注册
// → 各前端 THEME 清单加一条，即可经 ?theme=<id> 互跳与偏好持久化。

type themeApp struct {
	id        string // 主题 ID = localStorage beacontower_preferred_theme 的合法值
	mountPath string // "/" 或 "/toy/"（?theme= 302 的目标）
	fs        fs.FS
	index     []byte
	isDefault bool
}

// New 组装 Gin 引擎：API + 多主题静态托管 + SPA 回退（doc/04 §5、doc/18）。
func New(app *handler.App, toyDist embed.FS, toyReady bool, classicDist embed.FS, classicReady bool) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.ClientIP(app.Cfg))
	r.Use(middleware.SecurityHeaders())
	// 请求体上限 1MB：面板所有接口的合法 JSON 都远小于此；不限的话未鉴权的
	// login/setup 能把整个 body 读进内存（多 IP 并发即可打爆内存）
	r.Use(func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		}
		c.Next()
	})

	// /healthz：无鉴权、不进访问日志（doc/04 §5）
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// 公开路由按 IP 限速（doc/14：挂公网后防止脚本爬取与滥用拖垮面板）。
	// healthz 除外（容器健康检查本机自访）。GET-only 白名单在组内逐条注册。
	pubRL := middleware.NewPublicRateLimiter().Middleware()

	api := r.Group("/api/v1")
	{
		pub := api.Group("/public", pubRL)
		{
			pub.GET("/summary", app.PublicSummary)
			pub.GET("/servers", app.PublicServers)
			pub.GET("/servers/:id/history", app.PublicHistory)
			pub.GET("/servers/:id/traffic", app.PublicTraffic)
			// SSE：先限单 IP 并发数再进 handler（doc/14 滥用面收敛），
			// 限速器对长连接只在握手时计一次，不惩罚长连。
			pub.GET("/stream", middleware.SSEGate(4), app.PublicStream)
			// 穿透摘要（doc/13 §12）：硬白名单，访客只看用量与在用节点健康度
			pub.GET("/frp", app.FRPPublicSnapshot)
			// WG 组网摘要（doc/04 §1.6）：硬白名单，访客只看健康度与成员在线数
			pub.GET("/wg", app.WGPublicSnapshot)
		}
		adm := api.Group("/admin")
		{
			adm.GET("/status", app.Status)
			adm.POST("/setup", app.SetupRL.Middleware(), app.Setup)
			adm.POST("/login", app.LoginRL.Middleware(), app.Login)
			adm.POST("/logout", middleware.RequireCSRF(), app.Logout)
			auth := adm.Group("", middleware.RequireAuth(app.DB), middleware.RequireCSRF())
			{
				auth.POST("/password", app.ChangePassword)
				auth.GET("/servers", app.ListServers)
				auth.POST("/servers", app.CreateServer)
				auth.PUT("/servers/:id", app.UpdateServer)
				auth.DELETE("/servers/:id", app.DeleteServer)
				auth.PUT("/servers/order", app.ReorderServers)
				auth.PUT("/servers/:id/power-calibration", app.RecalibratePower)
				auth.POST("/servers/test", app.TestConnection)
				auth.POST("/servers/:id/rotate-key", app.RotateServerKey)
				auth.POST("/servers/:id/locate", app.Relocate)
				auth.GET("/settings", app.GetSettings)
				auth.PUT("/settings", app.SaveSettings)
				auth.GET("/audit", app.ListAudit)
				auth.GET("/wg/overview", app.WGOverview)
				auth.POST("/wg/plan", app.WGPlan)
				auth.POST("/wg/apply", app.WGApply)
				auth.POST("/wg/import", app.WGImport)
				auth.POST("/wg/hubs/standby", app.WGRegisterStandby)
				// 一键把网外节点配置为中心（备援/现役）：整链路静默跑完，进度落 wg_task
				auth.POST("/wg/hubs/provision", app.WGHubProvision)
				auth.POST("/wg/adopt", app.WGAdoptServer)
				// 中心节点面板 + 凭证中心
				auth.GET("/wg/hubs/:id", app.WGHubDetail)
				auth.POST("/wg/hubs/:id/probe", app.WGHubProbe)
				auth.DELETE("/wg/hubs/:id", app.WGHubCleanup)
				auth.POST("/wg/takeover", app.WGTakeover)
				auth.PUT("/wg/peers/:id", app.WGPeerRename)
				auth.POST("/wg/peers/:id/sync-hubs", app.WGPeerSyncHubs)
				// 重新签发使用端凭证：导入成员换面板托管密钥，各中心整体替换后可出二维码
				auth.POST("/wg/peers/:id/reissue", app.WGPeerReissue)
				auth.GET("/wg/creds.zip", app.WGCredsZip)
				auth.POST("/wg/switch-hub", app.WGSwitchHub)
				auth.POST("/wg/patrol", app.WGPatrol)
				auth.GET("/wg/tasks", app.WGTaskList)
				auth.GET("/wg/tasks/:id", app.WGTaskGet)
				auth.POST("/wg/devices", app.WGDeviceCreate)
				auth.GET("/wg/peers/:id/conf", app.WGPeerConf)
				auth.POST("/wg/peers/:id/verify", app.WGPeerVerify)
				auth.DELETE("/wg/peers/:id", app.WGPeerDelete)
				auth.GET("/wg/assets", app.WGAssetList)
				auth.POST("/wg/assets", app.WGAssetUpsert)
				auth.DELETE("/wg/assets/:id", app.WGAssetDelete)
				auth.POST("/wg/assets/:id/probe", app.WGAssetProbe)
				auth.POST("/wg/assets/:id/fetch", app.WGAssetFetch)
				auth.POST("/wg/assets/:id/push", app.WGAssetPush)

				// 内网穿透平台（Sakura / ChmlFrp / Cloudflare，doc/13、doc/16）
				auth.GET("/frp/overview", app.FRPOverview)
				auth.GET("/frp/nodes", app.FRPNodes)
				auth.POST("/frp/platforms", app.FRPBindNatfrp)
				auth.POST("/frp/cloudflared", app.FRPBindCloudflared)
				auth.PUT("/frp/platforms/:id", app.FRPPlatformRename)
				auth.DELETE("/frp/platforms/:id", app.FRPPlatformDelete)
				auth.GET("/frp/platforms/:id", app.FRPPlatformDetail)
				auth.POST("/frp/platforms/:id/sync", app.FRPSync)
				auth.GET("/frp/platforms/:id/flow", app.FRPFlow)
				auth.POST("/frp/platforms/:id/tunnels", app.FRPTunnelCreate)
				auth.GET("/frp/platforms/:id/subdomains", app.FRPSubdomains)
				auth.GET("/frp/platforms/:id/subdomains/available", app.FRPSubdomainsAvailable)
				auth.POST("/frp/platforms/:id/subdomains", app.FRPSubdomainCreate)
				auth.PUT("/frp/platforms/:id/subdomains", app.FRPSubdomainUpdate)
				auth.DELETE("/frp/platforms/:id/subdomains", app.FRPSubdomainDelete)
				auth.PUT("/frp/tunnels/:id", app.FRPTunnelUpdate)
				auth.DELETE("/frp/tunnels/:id", app.FRPTunnelDelete)
				auth.POST("/frp/tunnels/:id/lock", app.FRPTunnelLock)
				auth.POST("/frp/tunnels/:id/migrate", app.FRPTunnelMigrate)
				auth.POST("/frp/tunnels/:id/offline", app.FRPTunnelOffline)
				auth.POST("/frp/tunnels/:id/auth", app.FRPTunnelAuth)
				auth.GET("/frp/tunnels/:id/config", app.FRPTunnelConfig)
				auth.GET("/frp/tunnels/:id/traffic", app.FRPTunnelTraffic)
				// ChmlFrp 授权走 OAuth2 设备码（交互式，无法用静态密钥替代）
				// 客户端托管：面板经 SSH 在节点上部署/管理 frpc 容器（doc/13 §14）
				auth.GET("/frp/deployments", app.FRPDeployList)
				auth.POST("/frp/deployments", app.FRPDeployCreate)
				auth.POST("/frp/deployments/:id/sync", app.FRPDeploySync)
				auth.POST("/frp/deployments/:id/action", app.FRPDeployAction)
				auth.POST("/frp/deployments/:id/status", app.FRPDeployStatus)
				auth.GET("/frp/deployments/:id/logs", app.FRPDeployLogs)
				auth.DELETE("/frp/deployments/:id", app.FRPDeployDelete)
				auth.POST("/frp/servers/:id/docker", app.FRPServerDocker)
				auth.POST("/frp/chmlfrp/device", app.FRPChmlfrpDeviceStart)
				auth.GET("/frp/chmlfrp/device/:sid", app.FRPChmlfrpDevicePoll)
				auth.DELETE("/frp/chmlfrp/device/:sid", app.FRPChmlfrpDeviceCancel)
			}
		}
	}

	themes := registerThemes(toyDist, toyReady, classicDist, classicReady)
	if len(themes) == 0 {
		// dev 模式（无 embed 产物）：/api 未命中仍返回 JSON 404
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"code": 2002, "msg": "接口不存在", "data": nil})
				return
			}
			c.String(http.StatusNotFound, "frontend not embedded (dev mode)")
		})
		return r
	}
	defaultTheme := themes[0]
	byID := make(map[string]*themeApp, len(themes))
	for _, t := range themes {
		byID[t.id] = t
	}

	// PWA 端点（.webmanifest 等）不在 Go 默认 mime 表内，Content-Type
	// 错了 iOS/Safari 会拒绝装为 Web App（doc/14 §6）
	for ext, typ := range map[string]string{
		".webmanifest": "application/manifest+json; charset=utf-8",
		".json":        "application/json; charset=utf-8",
		".js":          "text/javascript; charset=utf-8",
		".css":         "text/css; charset=utf-8",
		".svg":         "image/svg+xml",
		".png":         "image/png",
		".ico":         "image/x-icon",
		".txt":         "text/plain; charset=utf-8",
	} {
		_ = mime.AddExtensionType(ext, typ)
	}

	// 主题入口：/ → 默认主题；?theme=<id> → 302 到对应主题挂载路径。
	// 目标即默认主题或 ID 未知时清参落回默认主题，绝不形成重定向环。
	// 302 带上 ?theme=<id>：目标主题的 boot 脚本据此写偏好（显式覆盖），
	// 否则参数在跳转中丢失，偏好还是旧值时会被 boot 又弹回原主题。
	r.GET("/", func(c *gin.Context) {
		if id := c.Query("theme"); id != "" {
			if target, ok := byID[id]; ok && !target.isDefault {
				// 保留其他查询参数（?theme=toy&from=...），只覆写 theme。
				q := c.Request.URL.Query()
				q.Set("theme", id)
				c.Redirect(http.StatusFound, target.mountPath+"?"+q.Encode())
				return
			}
		}
		serveIndex(c, defaultTheme)
	})

	// 旧主题前缀 /classic/* 永久迁回根路径（v2.8 及之前的书签 / 已装 PWA 的
	// start_url 都指向这里）。302 而非 301：避免浏览器把迁移永久缓存，后续调整
	// 拓扑时还有回旋余地。查询串一并保留（deep link 的 ?id= 等不丢）。
	legacyRedirect := func(c *gin.Context) {
		// 收敛为同源绝对路径：TrimLeft 防止 /classic//evil.com 变成协议相对
		// 跳转（//evil.com 会被浏览器当成外站），这是一个真实的开放重定向面。
		target := "/" + strings.TrimLeft(strings.TrimPrefix(c.Request.URL.Path, "/classic"), "/")
		if c.Request.URL.RawQuery != "" {
			target += "?" + c.Request.URL.RawQuery
		}
		c.Redirect(http.StatusFound, target)
	}
	r.GET("/classic", legacyRedirect)
	r.GET("/classic/*any", legacyRedirect)

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"code": 2002, "msg": "接口不存在", "data": nil})
			return
		}
		// 非默认主题前缀作用域（当前仅 /toy/）：资源/根文件/SPA 回退都限定在
		// 该前缀下，互不串扰。注册表顺序里默认主题永远排第一，这里跳过它。
		for _, t := range themes {
			if t.isDefault {
				continue
			}
			prefix := strings.TrimSuffix(t.mountPath, "/")
			if p == prefix {
				target := t.mountPath
				if c.Request.URL.RawQuery != "" {
					target += "?" + c.Request.URL.RawQuery
				}
				c.Redirect(http.StatusFound, target)
				return
			}
			if strings.HasPrefix(p, t.mountPath) {
				serveThemeFile(c, t, strings.TrimPrefix(p, prefix))
				return
			}
		}
		// 根 scope = 默认主题（Sky Glass）
		serveThemeFile(c, defaultTheme, p)
	})
	return r
}

// registerThemes 组装主题注册表；顺序即优先级，第一个是默认主题。
// v2.9 起默认 = Sky Glass（classic，根路径），toy 作为第二主题挂 /toy/。
func registerThemes(toyDist embed.FS, toyReady bool, classicDist embed.FS, classicReady bool) []*themeApp {
	var themes []*themeApp
	if classicReady {
		if sub, err := fs.Sub(classicDist, "web/dist"); err == nil {
			themes = append(themes, &themeApp{
				id: "classic", mountPath: "/", fs: sub,
				index: mustReadIndex(sub), isDefault: true,
			})
		}
	}
	if toyReady {
		if sub, err := fs.Sub(toyDist, "web-toy/dist"); err == nil {
			themes = append(themes, &themeApp{
				id: "toy", mountPath: "/toy/", fs: sub,
				index: mustReadIndex(sub),
			})
		}
	}
	return themes
}

// serveThemeFile 按主题作用域提供静态资源（prefix 为剥掉挂载前缀后的路径，
// 根 scope 时即原始路径）。embed FS 天然免疫目录穿越；白名单外的根文件名
// 绝不回吐 HTML（manifest/图标请求拿 text/html 会被浏览器拒绝）。
func serveThemeFile(c *gin.Context, t *themeApp, p string) {
	// 带 hash 资源长缓存
	if p == "/assets" || strings.HasPrefix(p, "/assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.FileFromFS(strings.TrimPrefix(p, "/"), http.FS(t.fs))
		return
	}
	if p == "/icons" || strings.HasPrefix(p, "/icons/") {
		c.Header("Cache-Control", "public, max-age=86400")
		c.FileFromFS(strings.TrimPrefix(p, "/"), http.FS(t.fs))
		return
	}
	// 主题 SW：默认主题在根（/sw.js = Sky Glass 的 injectManifest SW），非默认
	// 主题在自己的前缀下（/toy/sw.js）。toy 不装 PWA，缺文件时按 404 处理，
	// 绝不回吐 HTML（SW 脚本拿到 HTML 会静默安装失败）。
	if p == "/sw.js" {
		c.Header("Cache-Control", "no-cache")
		if _, err := fs.Stat(t.fs, "sw.js"); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.FileFromFS("sw.js", http.FS(t.fs))
		return
	}
	// Workbox 运行时（vite-plugin-pwa 产出 dist 根的 workbox-*.js）：
	// gin catch-all 的字面段必须以 / 结尾，"/workbox-*filepath" 注册不上，
	// 故按文件名识别。掉进 SPA 回退会让 SW importScripts 拿到 HTML，install 静默失败。
	if n := path.Base(p); strings.HasPrefix(n, "workbox-") && strings.HasSuffix(n, ".js") {
		c.Header("Cache-Control", "public, max-age=86400")
		c.FileFromFS(n, http.FS(t.fs))
		return
	}
	// 主题 boot 脚本（doc/18）：各主题作用域一份，index.html 里以阻塞式同源脚本
	// 引它（内联会被自身 CSP 的 default-src 'self' 拦掉，静默失效）。内容必须与
	// 当前主题拓扑一致，故 no-cache；缺失按 404 处理，绝不回吐 HTML（nosniff 下
	// MIME 不符的脚本会被浏览器拒绝，等于悄悄不执行）。
	if p == "/boot-theme.js" {
		c.Header("Cache-Control", "no-cache")
		b, err := fs.ReadFile(t.fs, "boot-theme.js")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", b)
		return
	}
	// 根级静态文件白名单：短缓存语义与旧版一致（不设 Cache-Control），
	// 文件不存在时按存在性优雅处理——favicon 404，manifest/robots 等文档类
	// 才回退 index（避免安装引导期 404，同时不把 HTML 吐给会校验 MIME 的请求方）。
	switch p {
	case "/favicon.ico", "/robots.txt", "/manifest.webmanifest", "/manifest.json":
		name := strings.TrimPrefix(p, "/")
		if p == "/manifest.json" {
			name = "manifest.webmanifest"
		}
		b, err := fs.ReadFile(t.fs, name)
		if err != nil {
			if p == "/favicon.ico" {
				c.Status(http.StatusNotFound)
				return
			}
			serveIndex(c, t)
			return
		}
		c.Data(http.StatusOK, mime.TypeByExtension(filepath.Ext(name)), b)
		return
	}
	// 其余根级静态文件（如 toy 的 beacon-toy.svg、apple-touch-icon.png）：
	// 存在即按真实 MIME 返回；不存在才落 SPA 回退。不能一律回 index.html——
	// nosniff 下图标/脚本拿到 text/html 会被浏览器拒绝，属于静默失效。
	if name := strings.TrimPrefix(p, "/"); name != "" && !strings.Contains(name, "/") {
		if st, err := fs.Stat(t.fs, name); err == nil && !st.IsDir() {
			c.Header("Cache-Control", "public, max-age=3600")
			c.FileFromFS(name, http.FS(t.fs))
			return
		}
	}
	serveIndex(c, t)
}

func mustReadIndex(distFS fs.FS) []byte {
	b, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		return []byte("<h1>BeaconTower</h1>")
	}
	return b
}

func serveIndex(c *gin.Context, t *themeApp) {
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", t.index)
}
