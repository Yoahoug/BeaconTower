package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/gin-gonic/gin"
)

// newWGPublicTestApp 造一个只带 DB 的 App（公开接口不碰采集器与 Runner）。
func newWGPublicTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &App{DB: db, Cfg: &config.Config{SiteTitle: "BeaconTower"}}
}

func nI64(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

// seedWGPublic 造一套「现役 hub + 备援 + 服务器成员 + 设备成员」的巡检后数据。
func seedWGPublic(t *testing.T, app *App) (hubSrv, peerSrv int64) {
	t.Helper()
	now := time.Now().Unix()
	var err error
	hubSrv, err = app.DB.CreateServer(&store.Server{Name: "hub-aliyun", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatalf("建 hub 节点: %v", err)
	}
	peerSrv, err = app.DB.CreateServer(&store.Server{Name: "home-nas", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatalf("建成员节点: %v", err)
	}
	hiddenSrv, err := app.DB.CreateServer(&store.Server{Name: "secret-box", Enabled: true, Hidden: true, CreatedAt: now})
	if err != nil {
		t.Fatalf("建隐藏节点: %v", err)
	}
	if err := app.DB.EnsureWGNetwork(&store.WGNetwork{
		Subnet: "10.66.66.0/24", HubIP: "10.66.66.1", Iface: "wg0",
		Keepalive: 25, MTU: 1420, ActiveHubServerID: hubSrv,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("建网络: %v", err)
	}
	// 现役 hub（ok）+ 备援（error：错误原文含 SSH 地址，绝不能出门）
	if err := app.DB.UpsertWGHub(&store.WGHub{
		ServerID: hubSrv, ListenPort: 51820,
		PublicKey: "pubkeyAAAA", Endpoint: "hub.example.com:51820",
		Status: "ok", CheckedAt: nI64(now),
	}); err != nil {
		t.Fatalf("建现役 hub: %v", err)
	}
	if err := app.DB.UpsertWGHub(&store.WGHub{
		ServerID: peerSrv, ListenPort: 51821,
		PublicKey: "pubkeyBBBB", Endpoint: "nas.internal:51821",
		Status: "error", LastError: "巡检连接失败: dial tcp 203.0.113.9:22 i/o timeout",
		CheckedAt: nI64(now),
	}); err != nil {
		t.Fatalf("建备援 hub: %v", err)
	}
	day := dayFloor(now)
	if err := app.DB.AddWGHubTraffic(hubSrv, day, 1000, 2000, now); err != nil {
		t.Fatalf("写 hub 当日流量: %v", err)
	}
	// 成员：在线 SSH 成员 / 隐藏节点成员 / 在线设备成员 / 已移出成员
	for _, p := range []*store.WGPeer{
		{Kind: "server", ServerID: nI64(peerSrv), Name: "home-nas", WgIP: "10.66.66.2",
			PublicKey: "peerKey1", Status: "online", LastHandshake: nI64(now - 30),
			RxBytes: 111, TxBytes: 222, CheckedAt: nI64(now), CreatedAt: now},
		{Kind: "server", ServerID: nI64(hiddenSrv), Name: "secret-box", WgIP: "10.66.66.3",
			PublicKey: "peerKey2", Status: "online", CheckedAt: nI64(now), CreatedAt: now},
		{Kind: "device", Name: "MacBook-Pro", WgIP: "10.66.66.4",
			PublicKey: "peerKey3", Status: "online", CheckedAt: nI64(now), CreatedAt: now},
		{Kind: "server", ServerID: nI64(peerSrv), Name: "gone", WgIP: "10.66.66.9",
			PublicKey: "peerKey9", Status: "left", CreatedAt: now},
	} {
		if _, err := app.DB.InsertWGPeer(p); err != nil {
			t.Fatalf("写成员 %s: %v", p.Name, err)
		}
	}
	return hubSrv, peerSrv
}

// TestWGPublicSnapshotWhitelist 把「公开页永不泄露组网拓扑与密钥材料」钉成回归测试。
// 断言分两半：该给的（成员名 + 在线、中心健康、今日/月中转流量）必须出现；
// 不该给的（网段 / WG IP / 端点 / 端口 / 公钥 / 错误原文 / 设备名）在原始响应体里
// 一个字符都不许有——白名单是纪律，不是自觉（doc/13 §12.4 同款）。
func TestWGPublicSnapshotWhitelist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newWGPublicTestApp(t)
	seedWGPublic(t, app)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/wg", nil)
	app.WGPublicSnapshot(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d，响应 %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	var out struct {
		Code int `json:"code"`
		Data struct {
			Initialized bool `json:"initialized"`
			Hubs        []struct {
				Name     string `json:"name"`
				IsActive bool   `json:"is_active"`
				Healthy  bool   `json:"healthy"`
				MonthRx  int64  `json:"month_rx"`
				MonthTx  int64  `json:"month_tx"`
			} `json:"hubs"`
			Peers []struct {
				Name   string `json:"name"`
				Online bool   `json:"online"`
			} `json:"peers"`
			Summary struct {
				HubActive    int   `json:"hub_active"`
				HubHealthy   int   `json:"hub_healthy"`
				MemberTotal  int   `json:"member_total"`
				MemberOnline int   `json:"member_online"`
				DeviceTotal  int   `json:"device_total"`
				TrafficToday int64 `json:"traffic_today"`
				TrafficMonth int64 `json:"traffic_month"`
				CheckedAt    int64 `json:"checked_at"`
			} `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("解包失败: %v / %s", err, body)
	}
	if out.Code != 0 {
		t.Fatalf("code=%d，响应 %s", out.Code, body)
	}

	// ---- 该给的 ----
	if !out.Data.Initialized {
		t.Error("initialized 应为 true")
	}
	if len(out.Data.Hubs) != 2 {
		t.Fatalf("中心数 = %d，期望 2（现役 + 未退役备援）", len(out.Data.Hubs))
	}
	hubViews := out.Data.Hubs
	var activeHub, standbyHub *struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
		Healthy  bool   `json:"healthy"`
		MonthRx  int64  `json:"month_rx"`
		MonthTx  int64  `json:"month_tx"`
	}
	for i := range hubViews {
		if hubViews[i].IsActive {
			activeHub = &hubViews[i]
		} else {
			standbyHub = &hubViews[i]
		}
	}
	if activeHub == nil || activeHub.Name != "hub-aliyun" || !activeHub.Healthy {
		t.Errorf("现役中心视图不符: %+v", activeHub)
	}
	if activeHub != nil && (activeHub.MonthRx != 1000 || activeHub.MonthTx != 2000) {
		t.Errorf("现役中心月流量 = rx %d / tx %d，期望 1000 / 2000", activeHub.MonthRx, activeHub.MonthTx)
	}
	if standbyHub == nil || standbyHub.Name != "home-nas" || standbyHub.Healthy {
		t.Errorf("备援中心视图不符（巡检 error 应判不健康）: %+v", standbyHub)
	}
	if len(out.Data.Peers) != 1 || out.Data.Peers[0].Name != "home-nas" || !out.Data.Peers[0].Online {
		t.Fatalf("公开成员不符（隐藏节点/设备/left 均不应出现）: %+v", out.Data.Peers)
	}
	s := out.Data.Summary
	if s.HubActive != 1 || s.HubHealthy != 1 {
		t.Errorf("中心汇总不符: %+v", s)
	}
	if s.MemberTotal != 1 || s.MemberOnline != 1 {
		t.Errorf("成员汇总不符: %+v", s)
	}
	if s.DeviceTotal != 1 {
		t.Errorf("设备数 = %d，期望 1（只给数量不给名字）", s.DeviceTotal)
	}
	if s.TrafficToday != 3000 {
		t.Errorf("今日中转流量 = %d，期望 3000（rx+tx 差值）", s.TrafficToday)
	}
	if s.TrafficMonth < 3000 {
		t.Errorf("月中转流量 = %d，应至少含当日 3000", s.TrafficMonth)
	}
	if s.CheckedAt == 0 {
		t.Error("checked_at 不应缺省")
	}

	// ---- 不该给的（禁止串扫描）----
	forbidden := map[string]string{
		"组网网段":        "10.66.66.0",
		"hub 虚拟IP":    "10.66.66.1",
		"成员 WG IP":    "10.66.66.2",
		"中心端点":        "hub.example.com",
		"备援端点":        "nas.internal",
		"监听端口":        "51820",
		"错误原文":        "203.0.113.9",
		"错误前缀":        "last_error",
		"hub 公钥":      "pubkeyAAAA",
		"成员公钥":        "peerKey1",
		"公钥字段":        "public_key",
		"指纹字段":        "key_fp",
		"私钥持有标记":      "has_keys",
		"隐藏节点成员":      "secret-box",
		"设备成员名":       "MacBook-Pro",
		"已移出成员":       "gone",
		"subnet 字段":   `"subnet"`,
		"wg_ip 字段":    `"wg_ip"`,
		"endpoint 字段": `"endpoint"`,
		"握手时间戳":       "last_handshake",
		"逐成员流量":       "rx_bytes",
	}
	for what, needle := range forbidden {
		if strings.Contains(body, needle) {
			t.Errorf("公开响应泄露%s（命中 %q）：%s", what, needle, body)
		}
	}
}

// TestWGPublicSnapshotUninitialized 未初始化组网时返回显式引导态，不报错。
func TestWGPublicSnapshotUninitialized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newWGPublicTestApp(t)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/wg", nil)
	app.WGPublicSnapshot(c)

	var out struct {
		Code int `json:"code"`
		Data struct {
			Initialized bool `json:"initialized"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解包失败: %v / %s", err, rec.Body.String())
	}
	if out.Code != 0 {
		t.Fatalf("code=%d，响应 %s", out.Code, rec.Body.String())
	}
	if out.Data.Initialized {
		t.Error("未初始化时 initialized 应为 false")
	}
}

// TestWGPublicSnapshotHiddenNodeNotInPeers 隐藏节点成员不进公开成员表。
// 本测试钉住「隐藏节点成员完全不出现在公开面」这一口径。
func TestWGPublicSnapshotHiddenNodeNotInPeers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newWGPublicTestApp(t)
	seedWGPublic(t, app)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/wg", nil)
	app.WGPublicSnapshot(c)

	var out struct {
		Data struct {
			Peers []struct {
				Name string `json:"name"`
			} `json:"peers"`
			Summary struct {
				MemberTotal int `json:"member_total"`
			} `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解包失败: %v", err)
	}
	for _, p := range out.Data.Peers {
		if p.Name == "secret-box" {
			t.Error("隐藏节点成员不应出现在公开成员表")
		}
	}
}
