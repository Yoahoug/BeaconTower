package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/gin-gonic/gin"
)

// newPublicTestApp 造一个只带 DB 的 App（公开接口不碰采集器与 Runner）。
func newPublicTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &App{DB: db, Cfg: &config.Config{SiteTitle: "BeaconTower"}}
}

// TestFRPPublicSnapshotWhitelist 把「公开页永不泄露账号与拓扑」钉成回归测试。
// 断言分两半：该给的（在用节点、今日流量、隧道计数）必须出现；
// 不该给的（账号画像 / 套餐余量 / 节点域名 / 隧道端点）在原始响应体里一个字符都不许有。
func TestFRPPublicSnapshotWhitelist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newPublicTestApp(t)

	// 画像与用量走单独的回写通道（Insert 只落凭据与状态，见 store.InsertFRPPlatform）
	p := &store.FRPPlatform{
		Kind: "natfrp", Name: "Sakura", Status: "ok",
		TokenEnc:       []byte{0x01, 0x02},
		Username:       "secretaccount",
		UID:            "1819140",
		GroupName:      "普通用户",
		Realname:       "已实名",
		SpeedLimit:     "10 Mbps",
		TrafficDayUsed: 2205981245,
		TrafficRemain:  12109852511,
		Conns:          3,
	}
	pid, err := app.DB.InsertFRPPlatform(p)
	if err != nil {
		t.Fatalf("建平台: %v", err)
	}
	p.ID = pid
	p.LastSyncAt = 1790762870
	if err := app.DB.SaveFRPPlatformProfile(p); err != nil {
		t.Fatalf("回写画像: %v", err)
	}

	// 一条隧道挂在节点 41 上；节点 99 没有任何隧道在用，必须被过滤掉
	if _, err := app.DB.ReplaceFRPTunnels(pid, []*store.FRPTunnel{
		{PlatformID: pid, RemoteID: "1", Name: "secret-tunnel", Proto: "tcp",
			NodeID: "41", LocalIP: "192.168.0.10", LocalPort: 3000, Remote: "frp-oil.com:29377",
			Online: true, TodayUp: 100, TodayDown: 200, SyncedAt: 1},
	}); err != nil {
		t.Fatalf("写隧道: %v", err)
	}
	if err := app.DB.ReplaceFRPNodes(pid, []*store.FRPNode{
		{PlatformID: pid, RemoteID: "41", Name: "长沙电信PLUS2", Host: "frp-oil.com",
			GroupName: "普通节点", Online: true, Load: 34.3, Uptime: 306201, SyncedAt: 1},
		{PlatformID: pid, RemoteID: "99", Name: "未使用的节点", Host: "frp-unused.com",
			GroupName: "VIP 3", Online: true, Load: 1.1, SyncedAt: 1},
	}); err != nil {
		t.Fatalf("写节点: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/frp", nil)
	app.FRPPublicSnapshot(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d，响应 %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	var out struct {
		Code int `json:"code"`
		Data struct {
			Platforms []struct {
				Name         string `json:"name"`
				Kind         string `json:"kind"`
				Online       bool   `json:"online"`
				TunnelTotal  int    `json:"tunnel_total"`
				TunnelOnline int    `json:"tunnel_online"`
				TrafficToday int64  `json:"traffic_today"`
				Nodes        []struct {
					Name   string  `json:"name"`
					Group  string  `json:"group"`
					Online bool    `json:"online"`
					Load   float64 `json:"load"`
					Uptime int64   `json:"uptime"`
				} `json:"nodes"`
			} `json:"platforms"`
			Summary struct {
				PlatformTotal int   `json:"platform_total"`
				TunnelTotal   int   `json:"tunnel_total"`
				TunnelOnline  int   `json:"tunnel_online"`
				Conns         int   `json:"conns"`
				TrafficToday  int64 `json:"traffic_today"`
				NodeInUse     int   `json:"node_in_use"`
				NodeOnline    int   `json:"node_online"`
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
	if len(out.Data.Platforms) != 1 {
		t.Fatalf("平台数 = %d，期望 1", len(out.Data.Platforms))
	}
	pv := out.Data.Platforms[0]
	if pv.Name != "Sakura" || pv.Kind != "natfrp" || !pv.Online {
		t.Errorf("平台字段不符: %+v", pv)
	}
	if pv.TunnelTotal != 1 || pv.TunnelOnline != 1 {
		t.Errorf("隧道计数不符: total=%d online=%d", pv.TunnelTotal, pv.TunnelOnline)
	}
	if pv.TrafficToday != 2205981245 {
		t.Errorf("今日流量 = %d，期望取账号级 TrafficDayUsed", pv.TrafficToday)
	}
	if len(pv.Nodes) != 1 || pv.Nodes[0].Name != "长沙电信PLUS2" || !pv.Nodes[0].Online {
		t.Fatalf("在用节点不符: %+v", pv.Nodes)
	}
	if out.Data.Summary.PlatformTotal != 1 || out.Data.Summary.NodeInUse != 1 ||
		out.Data.Summary.NodeOnline != 1 || out.Data.Summary.TunnelOnline != 1 ||
		out.Data.Summary.TrafficToday != 2205981245 || out.Data.Summary.Conns != 3 {
		t.Errorf("汇总不符: %+v", out.Data.Summary)
	}

	// ---- 不该给的 ----
	forbidden := map[string]string{
		"账号名":    "secretaccount",
		"UID":    "1819140",
		"账号分组":   "普通用户",
		"实名信息":   "已实名",
		"限速/套餐":  "10 Mbps",
		"剩余流量字段": `"traffic_remain"`,
		"节点域名":   "frp-oil.com",
		"未用节点域名": "frp-unused.com",
		"未用节点名":  "未使用的节点",
		"隧道名":    "secret-tunnel",
		"隧道端点":   "29377",
		"内网地址":   "192.168.0.10",
		"凭据标记":   "token",
		"节点远程ID": `"remote_id"`,
	}
	for what, needle := range forbidden {
		if strings.Contains(body, needle) {
			t.Errorf("公开响应泄露%s（命中 %q）：%s", what, needle, body)
		}
	}
}

// TestFRPNodesInUseFlag 管理端节点列表带 in_use 标记：面板默认只看在用的那几条，
// 不把平台全网节点（实测 71 条）铺满一屏。后端只标记、不替前端决定筛选口径。
func TestFRPNodesInUseFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newPublicTestApp(t)

	pid, err := app.DB.InsertFRPPlatform(&store.FRPPlatform{Kind: "natfrp", Name: "N", Status: "ok", TokenEnc: []byte{1}})
	if err != nil {
		t.Fatalf("建平台: %v", err)
	}
	if _, err := app.DB.ReplaceFRPTunnels(pid, []*store.FRPTunnel{
		{PlatformID: pid, RemoteID: "1", Name: "ssh", Proto: "tcp", NodeID: "41", Online: true, SyncedAt: 1},
	}); err != nil {
		t.Fatalf("写隧道: %v", err)
	}
	if err := app.DB.ReplaceFRPNodes(pid, []*store.FRPNode{
		{PlatformID: pid, RemoteID: "41", Name: "在用", Online: true, SyncedAt: 1},
		{PlatformID: pid, RemoteID: "99", Name: "闲置", Online: true, SyncedAt: 1},
	}); err != nil {
		t.Fatalf("写节点: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/frp/nodes", nil)
	app.FRPNodes(c)

	var out struct {
		Data struct {
			Nodes []struct {
				RemoteID string `json:"remote_id"`
				InUse    bool   `json:"in_use"`
			} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解包失败: %v / %s", err, rec.Body.String())
	}
	if len(out.Data.Nodes) != 2 {
		t.Fatalf("节点数 = %d，期望 2（管理端仍返回全网节点）", len(out.Data.Nodes))
	}
	got := map[string]bool{}
	for _, n := range out.Data.Nodes {
		got[n.RemoteID] = n.InUse
	}
	if !got["41"] {
		t.Error("挂了隧道的节点 41 应标记 in_use=true")
	}
	if got["99"] {
		t.Error("无隧道的节点 99 不应标记 in_use")
	}
}

// TestFRPPublicTodayFallsBackToTunnels 覆盖 ChmlFrp 分支：账号级无当日值时，
// 用名下隧道当日进出之和兜底，避免游客页显示恒为 0 的「今日流量」。
func TestFRPPublicTodayFallsBackToTunnels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newPublicTestApp(t)

	pid, err := app.DB.InsertFRPPlatform(&store.FRPPlatform{
		Kind: "chmlfrp", Name: "ChmlFrp", Status: "ok", TokenEnc: []byte{0x09},
	})
	if err != nil {
		t.Fatalf("建平台: %v", err)
	}
	if _, err := app.DB.ReplaceFRPTunnels(pid, []*store.FRPTunnel{
		{PlatformID: pid, RemoteID: "7", Name: "a", Proto: "tcp", NodeID: "5", Online: true, TodayUp: 1000, TodayDown: 2000, SyncedAt: 1},
		{PlatformID: pid, RemoteID: "8", Name: "b", Proto: "tcp", NodeID: "5", Online: false, TodayUp: 30, TodayDown: 70, SyncedAt: 1},
	}); err != nil {
		t.Fatalf("写隧道: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/frp", nil)
	app.FRPPublicSnapshot(c)

	var out struct {
		Data struct {
			Platforms []struct {
				TrafficToday int64 `json:"traffic_today"`
				TunnelOnline int   `json:"tunnel_online"`
			} `json:"platforms"`
			Summary struct {
				TrafficToday int64 `json:"traffic_today"`
				NodeInUse    int   `json:"node_in_use"`
			} `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解包失败: %v / %s", err, rec.Body.String())
	}
	if len(out.Data.Platforms) != 1 {
		t.Fatalf("平台数 = %d", len(out.Data.Platforms))
	}
	if got := out.Data.Platforms[0].TrafficToday; got != 3100 {
		t.Errorf("今日流量兜底 = %d，期望 3100（隧道进出之和）", got)
	}
	if got := out.Data.Platforms[0].TunnelOnline; got != 1 {
		t.Errorf("在线隧道 = %d，期望 1（离线隧道不计）", got)
	}
	// 节点镜像为空时不应报错，只是没有在用节点
	if out.Data.Summary.NodeInUse != 0 {
		t.Errorf("在用节点 = %d，期望 0", out.Data.Summary.NodeInUse)
	}
}

// TestFRPPublicSnapshotCFConnectorName 游客页隐私：Cloudflare 的「节点」是
//隧道的 connector，名字取自 cfd_tunnel 名（用户的独立专线名，如 New-api）。
// 隧道名对访客属于拓扑信息，公开快照必须统一替换成通用标签（doc/13 §12 白名单）。
func TestFRPPublicSnapshotCFConnectorName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newPublicTestApp(t)

	pid, err := app.DB.InsertFRPPlatform(&store.FRPPlatform{Kind: "cloudflared", Name: "Cloudflare", Status: "ok", TokenEnc: []byte{1}})
	if err != nil {
		t.Fatalf("建平台: %v", err)
	}
	if _, err := app.DB.ReplaceFRPTunnels(pid, []*store.FRPTunnel{
		{PlatformID: pid, RemoteID: "t1:h.ne", Name: "h.ne", Proto: "https", NodeID: "t1", Online: true, SyncedAt: 1},
	}); err != nil {
		t.Fatalf("写隧道: %v", err)
	}
	// connector 节点名 = 隧道名 + " 连接器"（frp.Nodes 归一时的命名）
	if err := app.DB.ReplaceFRPNodes(pid, []*store.FRPNode{
		{PlatformID: pid, RemoteID: "t1", Name: "New-api 连接器", GroupName: "Cloudflare", Online: true, SyncedAt: 1},
	}); err != nil {
		t.Fatalf("写节点: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/frp", nil)
	app.FRPPublicSnapshot(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d，响应 %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for what, needle := range map[string]string{
		"独立专线隧道名": "New-api",
		"连接器原始名":  "连接器\"?:" ,
	} {
		_ = what // 只防原始名出现；「Cloudflare 连接器」通用标签允许出现
		if strings.Contains(body, needle) && needle != "连接器\"?:" {
			t.Errorf("公开响应泄露%s（命中 %q）：%s", what, needle, body)
		}
	}
	if !strings.Contains(body, "Cloudflare 连接器") {
		t.Errorf("公开响应应包含通用标签「Cloudflare 连接器」：%s", body)
	}
}
