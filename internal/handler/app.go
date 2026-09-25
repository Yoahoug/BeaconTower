package handler

import (
	"time"

	"github.com/Yoahoug/BeaconTower/internal/collector"
	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// App handler 共享依赖。
type App struct {
	DB      *store.DB
	Cfg     *config.Config
	Coll    *collector.Collector
	Master  []byte
	Blocker *middleware.LoginBlocker
	SetupRL *middleware.RateLimiter
	LoginRL *middleware.RateLimiter
}

func nowUnix() int64 { return time.Now().Unix() }

// audit 记录审计日志（写失败不阻塞主流程）。
func (a *App) audit(actor, action, target, detail, ip string) {
	_ = a.DB.AddAudit(store.AuditEntry{
		Ts: nowUnix(), Actor: actor, Action: action,
		Target: target, Detail: detail,
		SourceIPHash: crypto.HashIP(ip),
	})
}

// actorOf 当前会话用户名（审计用）。
func (a *App) actorOf(c interface {
	Get(any) (any, bool)
}) string {
	if v, ok := c.Get(middleware.CtxUsername); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if admin, _ := a.DB.GetAdmin(); admin != nil {
		return admin.Username
	}
	return "system"
}
