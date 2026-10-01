package handler

// ============================================================
// WG 组网 · 游客摘要（doc/04 §1.6，doc/12 §17）
//
// GET /api/v1/public/wg：与 FRPPublicSnapshot 同一套白名单纪律——
// 只回答访客关心的四件事：组网有没有在跑、中心健不健康、成员在线几个、
// 今天/本月中转了多少流量。口径来自 WG 巡检（5min ticker）回填的 wg_peer
// 与 wg_hub_traffic，无 SSH 实时探测（公开端点不做慢操作）。
//
// 明确不下发（响应体禁止串扫描钉死，见 wg_public_test.go）：
//   - 网段拓扑：subnet / hub_ip / iface / 成员 wg_ip / 中心 endpoint / listen_port；
//   - 密钥材料：public_key / key_fp / has_keys（公钥指纹也是拓扑情报）；
//   - 错误原文：last_error 内含 SSH host:port 或 conf 路径，只给健康布尔；
//   - 成员身份细节：设备成员名（Mac/iPhone 等私人设备画像）不下发，只给数量；
//     隐藏节点沿用公开总览口径，不入公开成员表。
// ============================================================

import (
	"net/http"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/gin-gonic/gin"
)

// WGPublicSnapshot 游客面板的组网摘要（字段硬白名单）。
func (a *App) WGPublicSnapshot(c *gin.Context) {
	if a.privateMode() && !a.loggedIn(c) {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	netRow, err := a.DB.GetWGNetwork()
	if err != nil {
		middleware.Fail(c, 5000, "读取组网信息失败")
		return
	}
	// 未初始化组网：显式告知（前端展示引导态），不伪造空运行态
	if netRow == nil {
		middleware.OK(c, gin.H{"initialized": false})
		return
	}

	hubs, err := a.DB.ListWGHub()
	if err != nil {
		middleware.Fail(c, 5000, "读取中心节点失败")
		return
	}
	peers, err := a.DB.ListWGPeers()
	if err != nil {
		middleware.Fail(c, 5000, "读取成员失败")
		return
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	// 中心视图：只报健康与月中转量。名字走节点名（对访客已可见），
	// 端点/端口/公钥/错误原文一律不出门——巡检失败细节在管理端看。
	hubViews := []gin.H{}
	hubActive, hubHealthy := 0, 0
	var trafficMonth int64
	var latest int64
	for _, h := range hubs {
		if h.Status == "retired" {
			continue // 退役槽位的身份已迁走，对访客无意义
		}
		rx, tx, _ := a.DB.WGHubTrafficRange(h.ServerID, monthStart.Unix(), now.Unix())
		name := ""
		if srv, _ := a.DB.GetServer(h.ServerID); srv != nil {
			name = srv.Name
		}
		isActive := h.ServerID == netRow.ActiveHubServerID
		healthy := h.Status == "ok"
		if isActive {
			hubActive++
		}
		if healthy {
			hubHealthy++
		}
		trafficMonth += rx + tx
		if v := h.CheckedAt.Int64; v > latest {
			latest = v
		}
		hubViews = append(hubViews, gin.H{
			"name":      name,
			"is_active": isActive,
			"healthy":   healthy,
			"month_rx":  rx,
			"month_tx":  tx,
		})
	}

	// 成员视图：只收 kind=server 的 SSH 成员（设备成员名是私人设备画像，只给数量）。
	// 逐成员不下发 last_handshake/rx/tx：累计量绑定成员身份，对访客只给在线布尔。
	peerViews := []gin.H{}
	deviceTotal := 0
	onlineCount := 0
	for _, p := range peers {
		if p.Status == "left" {
			continue
		}
		if v := p.CheckedAt.Int64; v > latest {
			latest = v
		}
		if p.Kind != "server" {
			deviceTotal++
			continue
		}
		hidden := false
		if p.ServerID.Valid {
			if srv, _ := a.DB.GetServer(p.ServerID.Int64); srv != nil {
				hidden = srv.Hidden
			}
		}
		if hidden {
			continue
		}
		online := p.Status == "online"
		if online {
			onlineCount++
		}
		peerViews = append(peerViews, gin.H{"name": p.Name, "online": online})
	}

	// 今日中转流量：wg_hub_traffic 按日差值（巡检口径），全部未退役中心合计
	var trafficToday int64
	for _, h := range hubs {
		if h.Status == "retired" {
			continue
		}
		rx, tx, _ := a.DB.WGHubTrafficRange(h.ServerID, dayStart.Unix(), now.Unix())
		trafficToday += rx + tx
	}

	middleware.OK(c, gin.H{
		"initialized": true,
		"hubs":        hubViews,
		"peers":       peerViews,
		"summary": gin.H{
			"hub_active":    hubActive,
			"hub_healthy":   hubHealthy,
			"member_total":  len(peerViews),
			"member_online": onlineCount,
			"device_total":  deviceTotal,
			"traffic_today": trafficToday,
			"traffic_month": trafficMonth,
			"checked_at":    latest,
		},
	})
}
