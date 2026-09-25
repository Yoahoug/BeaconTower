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
			pub.GET("/stream", app.PublicStream)
		}
		adm := api.Group("/admin")
		{
			adm.GET("/status", app.Status)
			adm.POST("/setup", app.SetupRL.Middleware(), app.Setup)
			adm.POST("/login", app.LoginRL.Middleware(), app.Login)
			adm.POST("/logout", app.Logout)
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
