package handler

import (
	"sync"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/collector"
	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/frp"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/wg"
)

// App handler 共享依赖。
type App struct {
	DB      *store.DB
	Cfg     *config.Config
	Coll    *collector.Collector
	Master  []byte
	WG      *wg.Runner
	FRP     *frp.Runner
	Deploy  *frp.Deployer
	Blocker *middleware.LoginBlocker
	SetupRL *middleware.RateLimiter
	LoginRL *middleware.RateLimiter

	// wgOpMu 串行化 WG 任务型操作：HasRunningWGTask 是 check-then-act，
	// 进程内互斥防止双击/并发请求同时通过检查插入两个 running 任务
	wgOpMu sync.Mutex

	// deployMu 串行化穿透客户端托管操作：同一时刻只跑一个「写节点」的动作，
	// 避免两个请求同时 docker rm/run 同一个容器。
	deployMu sync.Mutex
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
