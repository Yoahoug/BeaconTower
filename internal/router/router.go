package router

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/handler"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/gin-gonic/gin"
)

// New 组装 Gin 引擎：API + 静态托管 + SPA 回退（doc/04 §5）。
func New(app *handler.App, webDist embed.FS, hasDist bool) *gin.Engine {
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

	api := r.Group("/api/v1")
	{
		pub := api.Group("/public")
		{
			pub.GET("/summary", app.PublicSummary)
			pub.GET("/servers", app.PublicServers)
			pub.GET("/servers/:id/history", app.PublicHistory)
			pub.GET("/servers/:id/traffic", app.PublicTraffic)
			pub.GET("/stream", app.PublicStream)
			// 穿透摘要（doc/13 §12）：硬白名单，访客只看用量与在用节点健康度
			pub.GET("/frp", app.FRPPublicSnapshot)
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
				auth.POST("/servers/:id/locate", app.Relocate)
				auth.GET("/settings", app.GetSettings)
				auth.PUT("/settings", app.SaveSettings)
				auth.GET("/audit", app.ListAudit)
				auth.GET("/wg/overview", app.WGOverview)
				auth.POST("/wg/plan", app.WGPlan)
				auth.POST("/wg/apply", app.WGApply)
				auth.POST("/wg/import", app.WGImport)
				auth.POST("/wg/hubs/standby", app.WGRegisterStandby)
				auth.POST("/wg/adopt", app.WGAdoptServer)
				// 中心节点面板 + 凭证中心
				auth.GET("/wg/hubs/:id", app.WGHubDetail)
				auth.POST("/wg/hubs/:id/probe", app.WGHubProbe)
				auth.DELETE("/wg/hubs/:id", app.WGHubCleanup)
				auth.POST("/wg/takeover", app.WGTakeover)
				auth.PUT("/wg/peers/:id", app.WGPeerRename)
				auth.POST("/wg/peers/:id/sync-hubs", app.WGPeerSyncHubs)
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

				// 内网穿透平台（Sakura / ChmlFrp，doc/13）
				auth.GET("/frp/overview", app.FRPOverview)
				auth.GET("/frp/nodes", app.FRPNodes)
				auth.POST("/frp/platforms", app.FRPBindNatfrp)
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

	// 静态托管 + SPA 回退
	if hasDist {
		distFS, err := fs.Sub(webDist, "web/dist")
		if err == nil {
			// 带 hash 资源长缓存；index.html no-cache
			r.GET("/assets/*filepath", func(c *gin.Context) {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
				c.FileFromFS(strings.TrimPrefix(c.Request.URL.Path, "/"), http.FS(distFS))
			})
			indexHTML := mustReadIndex(distFS)
			r.NoRoute(func(c *gin.Context) {
				p := c.Request.URL.Path
				if strings.HasPrefix(p, "/api") {
					c.JSON(http.StatusNotFound, gin.H{"code": 2002, "msg": "接口不存在", "data": nil})
					return
				}
				if strings.HasPrefix(p, "/assets/") {
					c.Status(http.StatusNotFound)
					return
				}
				c.Header("Cache-Control", "no-cache")
				c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
			})
		}
	} else {
		// dev 模式（无 embed 产物）：/api 未命中仍返回 JSON 404
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"code": 2002, "msg": "接口不存在", "data": nil})
				return
			}
			c.String(http.StatusNotFound, "frontend not embedded (dev mode)")
		})
	}
	return r
}

func mustReadIndex(distFS fs.FS) []byte {
	b, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		return []byte("<h1>BeaconTower</h1>")
	}
	return b
}

var _ = time.Now
