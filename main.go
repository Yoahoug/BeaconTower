// BeaconTower · 信标塔：自托管服务器监控面板（SSH 无 Agent 采集，单进程单端口）。
package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/collector"
	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/geoip"
	"github.com/Yoahoug/BeaconTower/internal/handler"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/router"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/tasks"
)

//go:embed web/dist
var webDist embed.FS

// hasDist embed 为空目录时 Sub 会失败，router 内据此降级为 dev 模式。
var hasDist = true

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	master, err := crypto.MasterKey(cfg.DataDir, cfg.MasterKeyHex)
	if err != nil {
		log.Fatalf("主密钥初始化失败: %v", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "beacontower.db"))
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	coll := collector.New(cfg, db, master)
	app := &handler.App{
		DB: db, Cfg: cfg, Coll: coll, Master: master,
		Blocker: middleware.NewLoginBlocker(),
		SetupRL: middleware.NewRateLimiter(5, time.Minute, 1006, "操作过于频繁，请稍后再试"),
		LoginRL: middleware.NewRateLimiter(5, time.Minute, 1006, "操作过于频繁，请稍后再试"),
	}

	// 初始化令牌提示（doc/04 §2.2）
	if has, _ := db.HasAdmin(); !has && cfg.SetupToken != "" {
		log.Printf("[beacontower] 初始化令牌 BEACON_SETUP_TOKEN 已启用（首次初始化需携带）")
	}
	if cfg.MasterKeyHex == "" {
		log.Printf("[beacontower] 未提供 BEACON_MASTER_KEY，已使用 data/master.key（0600）；生产环境建议显式提供并单独备份")
	}

	// 离线 IP 库（可选）：缺失则降级在线回显 + 静态映射
	geoip.Init(cfg.DataDir)

	// 本机节点：面板自身作为第一个节点（免 SSH，本地进程采集）
	if id, err := db.EnsureSelfServer(time.Now().Unix()); err != nil {
		log.Printf("[beacontower] 创建本机节点失败: %v", err)
	} else {
		log.Printf("[beacontower] 本机节点就绪 (id=%d)", id)
	}

	// 后台任务 + 采集器
	taskStop := make(chan struct{})
	tasks.Start(db, taskStop)
	coll.Start()

	engine := router.New(app, webDist, hasDist)
	addr := "0.0.0.0:" + cfg.Port
	fmt.Printf("[beacontower] listening on %s (data=%s)\n", addr, cfg.DataDir)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
	close(taskStop)
}
