// 内网穿透平台管理（doc/13）：Sakura / ChmlFrp 账号绑定、隧道与节点镜像、
// 用量趋势、隧道写操作与 frpc 配置下发。
//
// 设计要点：
//   - 所有外部调用都带 15~30s 超时，慢操作（同步）走同步阻塞 + 前端轮询任务态；
//   - 平台侧凭据只进不出：接口仅返回 has_token，绝不下发密钥本身；
//   - 平台能力不等价（锁定/迁移只有 Sakura 有，强制下线只有 ChmlFrp 有），
//     不支持的操作用 2020 明确拒绝而不是静默失败。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/frp"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/gin-gonic/gin"
)

// FRP 模块错误码（doc/04 错误码表的 2020 段）
const (
	frpCodeUnsupported = 2020 // 平台不支持该操作
	frpCodeNeedAuth    = 2021 // 凭据失效，需重新授权
	frpCodeUpstream    = 2022 // 上游平台调用失败
)

// frpTimeout 单次外部调用预算。同步要连打 3~4 个接口，给足 45s。
const frpTimeout = 45 * time.Second

// frpWriteTimeout 写操作（建/改/删隧道）的外部调用预算。
const frpWriteTimeout = 30 * time.Second

// frpCtx 构造带超时的外部调用上下文。
func frpCtx(c *gin.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), d)
}

// failFRP 把领域错误翻译成统一错误码：凭据失效要让前端弹重新授权，
// 平台不支持要给出明确原因，其余归为上游故障。
func failFRP(c *gin.Context, err error) {
	switch {
	case errors.Is(err, frp.ErrAuth):
		middleware.Fail(c, frpCodeNeedAuth, "平台凭据已失效，请重新授权："+err.Error())
	case errors.Is(err, frp.ErrUnsupported):
		middleware.Fail(c, frpCodeUnsupported, err.Error())
	case errors.Is(err, frp.ErrNotFound):
		middleware.Fail(c, 2002, "平台不存在或已解绑")
	default:
		middleware.Fail(c, frpCodeUpstream, err.Error())
	}
}

// ---------- 视图构造 ----------

func frpPlatformView(p *store.FRPPlatform, tunnelTotal, tunnelOnline int) gin.H {
	profile := map[string]any{}
	if p.ProfileJSON != "" {
		_ = json.Unmarshal([]byte(p.ProfileJSON), &profile)
	}
	return gin.H{
		"id":               p.ID,
		"kind":             p.Kind,
		"name":             p.Name,
		"username":         p.Username,
		"uid":              p.UID,
		"group_name":       p.GroupName,
		"speed_limit":      p.SpeedLimit,
		"realname":         p.Realname,
		"tunnel_used":      p.TunnelUsed,
		"tunnel_quota":     p.TunnelQuota,
		"tunnel_total":     tunnelTotal,
		"tunnel_online":    tunnelOnline,
		"conns":            p.Conns,
		"conns_src":        p.ConnsSrc,
		"traffic_day_used": p.TrafficDayUsed,
		"traffic_remain":   p.TrafficRemain,
		"traffic_up":       p.TrafficUp,
		"traffic_down":     p.TrafficDown,
		"status":           p.Status,
		"last_error":       p.LastError,
		"last_sync_at":     p.LastSyncAt,
		"has_token":        p.HasToken(),
		"profile":          profile,
		// ChmlFrp 的 access_token 只有 10 分钟寿命，把到期时刻给前端用于显示
		"token_expire_at": p.TokenExpireAt,
	}
}

func frpTunnelView(t *store.FRPTunnel, platformName, platformKind string) gin.H {
	return gin.H{
		"id":            t.ID,
		"platform_id":   t.PlatformID,
		"platform_name": platformName,
		"platform_kind": platformKind,
		"remote_id":     t.RemoteID,
		"name":          t.Name,
		"proto":         t.Proto,
		"node_id":       t.NodeID,
		"node_name":     t.NodeName,
		"local_ip":      t.LocalIP,
		"local_port":    t.LocalPort,
		"remote":        t.Remote,
		"online":        t.Online,
		"status":        t.Status,
		"status_reason": t.StatusReason,
		"conns":         t.Conns,
		"local_conns":   t.LocalConns,
		"today_up":      t.TodayUp,
		"today_down":    t.TodayDown,
		"uptime":        t.Uptime,
		"client_ver":    t.ClientVer,
		"extra":         t.Extra,
		"lock_edit":     t.LockEdit,
		"lock_delete":   t.LockDelete,
		"lock_migrate":  t.LockMigrate,
		"synced_at":     t.SyncedAt,
	}
}

// frpNodeView 单节点视图。inUse 表示该节点是否被本账号某条隧道挂着——
// 平台节点表是全网节点（Sakura 实测 71 条），面板默认只看在用的那几条，
// 由前端按 in_use 过滤（后端不替前端决定筛选口径）。
func frpNodeView(n *store.FRPNode, platformName string, inUse bool) gin.H {
	return gin.H{
		"id":            n.ID,
		"platform_id":   n.PlatformID,
		"platform_name": platformName,
		"remote_id":     n.RemoteID,
		"name":          n.Name,
		"host":          n.Host,
		"area":          n.Area,
		"group_name":    n.GroupName,
		"caps":          n.Caps,
		"online":        n.Online,
		"load":          n.Load,
		"uptime":        n.Uptime,
		"description":   n.Description,
		"in_use":        inUse,
		"synced_at":     n.SyncedAt,
	}
}

// frpNodesInUse 在用的节点 remote_id 集合（按平台分组）：隧道记录里的 node_id
// 存的就是节点 remote_id（见 runner.fillNodeNames）。
func (a *App) frpNodesInUse() map[int64]map[string]bool {
	tunnels, err := a.DB.ListFRPTunnels()
	if err != nil {
		return nil
	}
	m := make(map[int64]map[string]bool)
	for _, t := range tunnels {
		if t.NodeID == "" {
			continue
		}
		if m[t.PlatformID] == nil {
			m[t.PlatformID] = map[string]bool{}
		}
		m[t.PlatformID][t.NodeID] = true
	}
	return m
}

// platformIndex 平台 ID → 记录，避免逐行查库。
func (a *App) platformIndex() map[int64]*store.FRPPlatform {
	list, _ := a.DB.ListFRPPlatforms()
	m := make(map[int64]*store.FRPPlatform, len(list))
	for _, p := range list {
		m[p.ID] = p
	}
	return m
}

// ---------- 总览 ----------

// FRPOverview 穿透总览：平台卡片 + 跨平台隧道汇总。
func (a *App) FRPOverview(c *gin.Context) {
	platforms, err := a.DB.ListFRPPlatforms()
	if err != nil {
		middleware.Fail(c, 5000, "读取平台列表失败: "+err.Error())
		return
	}
	tunnels, err := a.DB.ListFRPTunnels()
	if err != nil {
		middleware.Fail(c, 5000, "读取隧道列表失败: "+err.Error())
		return
	}
	total := map[int64]int{}
	online := map[int64]int{}
	for _, t := range tunnels {
		total[t.PlatformID]++
		if t.Online {
			online[t.PlatformID]++
		}
	}

	views := make([]gin.H, 0, len(platforms))
	var sumTunnel, sumOnline, sumConns int
	for _, p := range platforms {
		views = append(views, frpPlatformView(p, total[p.ID], online[p.ID]))
		sumTunnel += total[p.ID]
		sumOnline += online[p.ID]
		sumConns += p.Conns
	}

	// 隧道列表一并下发，前端首屏不必再发一次请求
	idx := make(map[int64]*store.FRPPlatform, len(platforms))
	for _, p := range platforms {
		idx[p.ID] = p
	}
	sort.SliceStable(tunnels, func(i, j int) bool { return tunnels[i].PlatformID < tunnels[j].PlatformID })
	tViews := make([]gin.H, 0, len(tunnels))
	for _, t := range tunnels {
		var pn, pk string
		if p := idx[t.PlatformID]; p != nil {
			pn, pk = p.Name, p.Kind
		}
		tViews = append(tViews, frpTunnelView(t, pn, pk))
	}

	middleware.OK(c, gin.H{
		"platforms": views,
		"tunnels":   tViews,
		"summary": gin.H{
			"platform_count": len(platforms),
			"tunnel_total":   sumTunnel,
			"tunnel_online":  sumOnline,
			"conns":          sumConns,
		},
	})
}

// FRPPublicSnapshot 游客面板的穿透摘要（doc/04 §1.5）。
//
// 下发字段是硬白名单，只回答访客关心的三件事：接了哪几个平台、隧道通不通、
// 今天走了多少流量、在用的节点健不健康。明确不下发的东西分三类——
// 账号画像（用户名 / UID / 实名 / 账号分组）、套餐信息（剩余流量 / 限速）、
// 内网拓扑（节点域名 host、隧道名与本地/公网端点、节点 remote_id）。
// 后两类泄露的不是「平台公开信息」而是「这台机器在用什么」，加字段前先过这一条。
//
// 节点只下发「在用」的那些（被隧道挂载的），平台全网节点表（Sakura 实测 71 条）
// 对访客既无意义也在泄露未使用的拓扑，见 store.ListFRPNodesInUse。
func (a *App) FRPPublicSnapshot(c *gin.Context) {
	if a.privateMode() && !a.loggedIn(c) {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	platforms, err := a.DB.ListFRPPlatforms()
	if err != nil {
		// 公开端点不回显错误原文（可能含 SQL/路径细节，doc/14 §2）
		log.Printf("[frp-public] 读取平台列表失败: %v", err)
		middleware.Fail(c, 5000, "读取平台列表失败")
		return
	}
	tunnels, err := a.DB.ListFRPTunnels()
	if err != nil {
		log.Printf("[frp-public] 读取隧道列表失败: %v", err)
		middleware.Fail(c, 5000, "读取隧道列表失败")
		return
	}

	type agg struct{ total, online int }
	byPlatform := map[int64]*agg{}
	dailySum := map[int64]int64{}
	for _, t := range tunnels {
		st := byPlatform[t.PlatformID]
		if st == nil {
			st = &agg{}
			byPlatform[t.PlatformID] = st
		}
		st.total++
		if t.Online {
			st.online++
		}
		dailySum[t.PlatformID] += t.TodayUp + t.TodayDown
	}

	views := make([]gin.H, 0, len(platforms))
	var sumTunnel, sumOnline, sumConns, sumNodes, sumNodesOnline int
	var sumToday int64
	var latest int64
	for _, p := range platforms {
		st := byPlatform[p.ID]
		if st == nil {
			st = &agg{}
		}
		// Sakura 的 /user/info 直接给当日消耗；ChmlFrp 账号级无当日值（恒 0），
		// 用它名下隧道的当日进出之和兜底，两平台都能出一致的「今日流量」。
		today := p.TrafficDayUsed
		if today == 0 {
			today = dailySum[p.ID]
		}
		nodes, err := a.DB.ListFRPNodesInUse(p.ID)
		if err != nil {
			log.Printf("[frp-public] 读取节点列表失败: %v", err)
			middleware.Fail(c, 5000, "读取节点列表失败")
			return
		}
		nodeViews := make([]gin.H, 0, len(nodes))
		nodeOnline := 0
		for _, n := range nodes {
			if n.Online {
				nodeOnline++
			}
			name := n.Name
			if p.Kind == frp.KindCloudflared {
				// CF 的「节点」是隧道的 connector，名字取自 cfd_tunnel 名
				// （如用户的独立专线名）。隧道名对访客属于拓扑信息（本页
				// 白名单明确不下发），统一换成通用标签，只保留健康与负载。
				name = "Cloudflare 连接器"
			}
			nodeViews = append(nodeViews, gin.H{
				"name":   name,
				"group":  n.GroupName,
				"online": n.Online,
				"load":   n.Load,
				"uptime": n.Uptime,
			})
		}
		views = append(views, gin.H{
			"name":          p.Name,
			"kind":          p.Kind,
			"online":        p.Status == "ok",
			"tunnel_total":  st.total,
			"tunnel_online": st.online,
			"conns":         p.Conns,
			"conns_src":     p.ConnsSrc,
			"traffic_today": today,
			"nodes":         nodeViews,
			"updated_at":    p.LastSyncAt,
		})
		sumTunnel += st.total
		sumOnline += st.online
		sumConns += p.Conns
		sumToday += today
		sumNodes += len(nodes)
		sumNodesOnline += nodeOnline
		if p.LastSyncAt > latest {
			latest = p.LastSyncAt
		}
	}

	middleware.OK(c, gin.H{
		"platforms": views,
		"summary": gin.H{
			"platform_total": len(platforms),
			"tunnel_total":   sumTunnel,
			"tunnel_online":  sumOnline,
			"conns":          sumConns,
			"traffic_today":  sumToday,
			"node_in_use":    sumNodes,
			"node_online":    sumNodesOnline,
			"updated_at":     latest,
		},
	})
}

// FRPPlatformDetail 平台详情：账号 + 用量趋势 + 该平台隧道 + 节点。
func (a *App) FRPPlatformDetail(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	p, _ := a.DB.GetFRPPlatform(id)
	if p == nil {
		middleware.Fail(c, 2002, "平台不存在")
		return
	}
	total, online := 0, 0
	tViews := []gin.H{}
	all, _ := a.DB.ListFRPTunnels()
	for _, t := range all {
		if t.PlatformID != id {
			continue
		}
		total++
		if t.Online {
			online++
		}
		tViews = append(tViews, frpTunnelView(t, p.Name, p.Kind))
	}
	nViews := []gin.H{}
	nodes, _ := a.DB.ListFRPNodes()
	inUse := map[string]bool{}
	for _, t := range all {
		if t.PlatformID == id && t.NodeID != "" {
			inUse[t.NodeID] = true
		}
	}
	for _, n := range nodes {
		if n.PlatformID == id {
			nViews = append(nViews, frpNodeView(n, p.Name, inUse[n.RemoteID]))
		}
	}

	// 用量趋势：面板自留的快照（平台侧历史接口粒度粗，且 ChmlFrp 只有 7 天）
	since := time.Now().Add(-7 * 24 * time.Hour).Unix()
	usage, _ := a.DB.ListFRPUsage(id, since, 500)
	uViews := make([]gin.H, 0, len(usage))
	for _, u := range usage {
		uViews = append(uViews, gin.H{
			"ts":               u.TS,
			"traffic_day_used": u.TrafficDayUsed,
			"traffic_remain":   u.TrafficRemain,
			"traffic_up":       u.TrafficUp,
			"traffic_down":     u.TrafficDown,
			"conns":            u.Conns,
			"tunnel_online":    u.TunnelOnline,
			"tunnel_total":     u.TunnelTotal,
		})
	}

	middleware.OK(c, gin.H{
		"platform": frpPlatformView(p, total, online),
		"tunnels":  tViews,
		"nodes":    nViews,
		"usage":    uViews,
	})
}

// FRPNodes 跨平台节点列表。
func (a *App) FRPNodes(c *gin.Context) {
	nodes, err := a.DB.ListFRPNodes()
	if err != nil {
		middleware.Fail(c, 5000, "读取节点失败: "+err.Error())
		return
	}
	idx := a.platformIndex()
	inUse := a.frpNodesInUse()
	out := make([]gin.H, 0, len(nodes))
	for _, n := range nodes {
		var pn string
		if p := idx[n.PlatformID]; p != nil {
			pn = p.Name
		}
		out = append(out, frpNodeView(n, pn, inUse[n.PlatformID][n.RemoteID]))
	}
	middleware.OK(c, gin.H{"nodes": out})
}

// FRPFlow 账号级流量历史。kind: day|week|month（ChmlFrp 忽略该参数，固定近 7 日）。
func (a *App) FRPFlow(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	ctx, cancel := frpCtx(c, frpTimeout)
	defer cancel()
	pts, err := a.FRP.FlowHistory(ctx, id, c.DefaultQuery("kind", "day"))
	if err != nil {
		failFRP(c, err)
		return
	}
	out := make([]gin.H, 0, len(pts))
	for _, p := range pts {
		out = append(out, gin.H{"label": p.Label, "used": p.Used, "remain": p.Remain})
	}
	middleware.OK(c, gin.H{"points": out})
}

// ---------- 平台绑定 ----------

type frpNatfrpBindInput struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

// FRPBindNatfrp 用访问密钥绑定 Sakura 账号（先验活再落库）。
func (a *App) FRPBindNatfrp(c *gin.Context) {
	var in frpNatfrpBindInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" {
		middleware.Fail(c, 1001, "请填写 Sakura 访问密钥（面板「用户信息」页可查看）")
		return
	}
	if in.Name == "" {
		in.Name = "Sakura"
	}
	ctx, cancel := frpCtx(c, frpTimeout)
	defer cancel()
	p, err := a.FRP.BindNatfrp(ctx, in.Name, in.Token)
	if err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_platform_bind", fmt.Sprintf("frp_platform:%d", p.ID),
		fmt.Sprintf("绑定 Sakura 账号 %s", p.Username), ipOf(c))
	// 绑定后立刻做一次全量同步，让面板马上有数据
	if err := a.FRP.Sync(ctx, p.ID, true); err != nil {
		middleware.OK(c, gin.H{"id": p.ID, "sync_error": err.Error()})
		return
	}
	middleware.OK(c, gin.H{"id": p.ID})
}

type frpCloudflaredBindInput struct {
	Name      string `json:"name"`
	AccountID string `json:"account_id"` // 可空：由 token 自动发现
	Token     string `json:"token"`
	ZoneID    string `json:"zone_id"` // 可空：首次同步时按隧道 hostname 自动发现
}

// FRPBindCloudflared 用 API Token 绑定 Cloudflare（先验活再落库）。
// account_id/zone_id 均可空：Account ID 由 token 自动发现，Zone ID 延迟到
// 首次同步按隧道 hostname 发现——用户只需创建并粘贴一个 API Token。
func (a *App) FRPBindCloudflared(c *gin.Context) {
	var in frpCloudflaredBindInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" {
		middleware.Fail(c, 1001, "请填写 Cloudflare API Token（创建方式见弹窗内的「?」帮助）")
		return
	}
	if in.Name == "" {
		in.Name = "Cloudflare"
	}
	ctx, cancel := frpCtx(c, frpTimeout)
	defer cancel()
	p, err := a.FRP.BindCloudflared(ctx, in.Name, frp.CloudflaredCreds{
		AccountID: strings.TrimSpace(in.AccountID),
		Token:     in.Token,
		ZoneID:    strings.TrimSpace(in.ZoneID),
	})
	if err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_platform_bind", fmt.Sprintf("frp_platform:%d", p.ID),
		"绑定 Cloudflare 账号", ipOf(c))
	// 绑定后立刻做一次全量同步，让面板马上有数据
	if err := a.FRP.Sync(ctx, p.ID, true); err != nil {
		middleware.OK(c, gin.H{"id": p.ID, "sync_error": err.Error()})
		return
	}
	middleware.OK(c, gin.H{"id": p.ID})
}

type frpPlatformUpdateInput struct {
	Name string `json:"name"`
}

// FRPPlatformRename 改名（凭据不在此接口更新）。
func (a *App) FRPPlatformRename(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	var in frpPlatformUpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		middleware.Fail(c, 1001, "名称不能为空")
		return
	}
	p, _ := a.DB.GetFRPPlatform(id)
	if p == nil {
		middleware.Fail(c, 2002, "平台不存在")
		return
	}
	// 唯一索引在 (kind, name) 上：重名会直接报 SQLite 约束错，先查一次给出友好提示
	if exist, _ := a.DB.GetFRPPlatformByName(p.Kind, in.Name); exist != nil && exist.ID != id {
		middleware.Fail(c, 1004, "已存在同名平台")
		return
	}
	if err := a.DB.RenameFRPPlatform(id, in.Name); err != nil {
		middleware.Fail(c, 5000, "改名失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "frp_platform_rename", fmt.Sprintf("frp_platform:%d", id), "改名为 "+in.Name, ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// FRPPlatformDelete 解绑平台（连带隧道/节点/用量快照）。
func (a *App) FRPPlatformDelete(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	p, _ := a.DB.GetFRPPlatform(id)
	if p == nil {
		middleware.Fail(c, 2002, "平台不存在")
		return
	}
	// 先清节点侧托管容器（外键级联不会去删服务器上的容器，留着会成孤儿）
	if deps, err := a.DB.ListFRPDeploys(); err == nil {
		dctx, dcancel := context.WithTimeout(c.Request.Context(), frpNodeTimeout)
		for _, d := range deps {
			if d.PlatformID != id {
				continue
			}
			if err := a.Deploy.Remove(dctx, d, true); err != nil {
				a.audit(a.actorOf(c), "frp_deploy_delete", fmt.Sprintf("deploy:%d", d.ID),
					"解绑平台时清理托管容器失败（继续解绑）: "+err.Error(), ipOf(c))
			}
			_ = a.DB.DeleteFRPDeploy(d.ID)
		}
		dcancel()
	}
	// 外键级联已覆盖，但显式清理不依赖 foreign_keys pragma，避免换库时残留
	_ = a.DB.DeleteFRPTunnelsOfPlatform(id)
	_ = a.DB.DeleteFRPNodesOfPlatform(id)
	if err := a.DB.DeleteFRPPlatform(id); err != nil {
		middleware.Fail(c, 5000, "解绑失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "frp_platform_delete", fmt.Sprintf("frp_platform:%d", id),
		"解绑平台 "+p.Name, ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// FRPSync 手动同步。full=true 时连节点列表一起拉。
func (a *App) FRPSync(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	ctx, cancel := frpCtx(c, frpTimeout)
	defer cancel()
	full := c.DefaultQuery("full", "1") != "0"
	if err := a.FRP.Sync(ctx, id, full); err != nil {
		failFRP(c, err)
		return
	}
	p, _ := a.DB.GetFRPPlatform(id)
	a.audit(a.actorOf(c), "frp_sync", fmt.Sprintf("frp_platform:%d", id), "手动同步", ipOf(c))
	var syncedAt int64
	if p != nil {
		syncedAt = p.LastSyncAt
	}
	middleware.OK(c, gin.H{"ok": true, "synced_at": syncedAt})
}

// ---------- ChmlFrp 设备码授权 ----------

type frpDeviceStartInput struct {
	ReuseID int64 `json:"reuse_id"` // 重新授权已有平台时带上，保留历史数据
}

// FRPChmlfrpDeviceStart 发起设备码授权，返回用户码与授权链接。
func (a *App) FRPChmlfrpDeviceStart(c *gin.Context) {
	var in frpDeviceStartInput
	_ = c.ShouldBindJSON(&in) // 允许空 body
	ctx, cancel := frpCtx(c, 20*time.Second)
	defer cancel()
	s, err := a.FRP.Flow.Start(ctx, in.ReuseID)
	if err != nil {
		middleware.Fail(c, frpCodeUpstream, err.Error())
		return
	}
	middleware.OK(c, gin.H{
		"session_id": s.ID,
		"user_code":  s.UserCode,
		"verify_url": s.VerifyURL,
		"expires_in": s.ExpiresIn,
		"interval":   s.Interval,
	})
}

// FRPChmlfrpDevicePoll 轮询授权状态；status=ok 时完成绑定并返回平台 ID。
func (a *App) FRPChmlfrpDevicePoll(c *gin.Context) {
	sid := c.Param("sid")
	ctx, cancel := frpCtx(c, 20*time.Second)
	defer cancel()
	s, err := a.FRP.Flow.Poll(ctx, sid)
	if err != nil {
		middleware.Fail(c, 2002, err.Error())
		return
	}
	out := gin.H{
		"status":     s.Status,
		"user_code":  s.UserCode,
		"verify_url": s.VerifyURL,
		"error":      s.Error,
	}
	if s.Status == "ok" {
		p, err := a.FRP.BindChmlfrp(ctx, sid, s.PlatformID)
		if err != nil {
			out["status"] = "error"
			out["error"] = err.Error()
			middleware.OK(c, out)
			return
		}
		out["platform_id"] = p.ID
		out["username"] = p.Username
		a.audit(a.actorOf(c), "frp_platform_bind", fmt.Sprintf("frp_platform:%d", p.ID),
			"绑定 ChmlFrp 账号 "+p.Username, ipOf(c))
		// 绑定成功即做一次全量同步
		if err := a.FRP.Sync(ctx, p.ID, true); err != nil {
			out["sync_error"] = err.Error()
		}
	}
	middleware.OK(c, out)
}

// FRPChmlfrpDeviceCancel 取消授权会话。
func (a *App) FRPChmlfrpDeviceCancel(c *gin.Context) {
	a.FRP.Flow.Cancel(c.Param("sid"))
	middleware.OK(c, gin.H{"ok": true})
}

// ---------- 隧道写操作 ----------

type frpTunnelInput struct {
	Name        string `json:"name"`
	Proto       string `json:"proto"`
	NodeID      string `json:"node_id"`
	LocalIP     string `json:"local_ip"`
	LocalPort   int    `json:"local_port"`
	RemotePort  int    `json:"remote_port"`
	Domain      string `json:"domain"`
	Note        string `json:"note"`
	Extra       string `json:"extra"`
	Encryption  bool   `json:"encryption"`
	Compression bool   `json:"compression"`
}

var frpProtos = map[string]bool{"tcp": true, "udp": true, "http": true, "https": true}

// chmlTunnelNameRe ChmlFrp 隧道名字符集：真机实测带连字符会回
// 「隧道名不符合规范，仅允许字母、数字和下划线」，在面板侧先拦一道，
// 免得用户对着平台原文猜。
var chmlTunnelNameRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// checkTunnelName 平台侧隧道名规则不等价（Sakura 宽松，ChmlFrp 只吃
// 字母/数字/下划线）。平台读不到时不拦，交给下游报错。
func (a *App) checkTunnelName(platformID int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	p, err := a.DB.GetFRPPlatform(platformID)
	if err != nil || p == nil || p.Kind != frp.KindChmlfrp {
		return nil
	}
	if !chmlTunnelNameRe.MatchString(name) {
		return errors.New("ChmlFrp 隧道名只允许字母、数字与下划线（不能带连字符、中文或空格）")
	}
	return nil
}

func (in *frpTunnelInput) toDomain() (frp.TunnelInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Proto = strings.ToLower(strings.TrimSpace(in.Proto))
	if !frpProtos[in.Proto] {
		return frp.TunnelInput{}, errors.New("隧道类型必须是 tcp/udp/http/https 之一")
	}
	if in.Name == "" {
		return frp.TunnelInput{}, errors.New("隧道名不能为空")
	}
	if in.LocalPort <= 0 || in.LocalPort > 65535 {
		return frp.TunnelInput{}, errors.New("本地端口必须在 1-65535 之间")
	}
	if in.RemotePort < 0 || in.RemotePort > 65535 {
		return frp.TunnelInput{}, errors.New("公网端口必须在 0-65535 之间（0 表示由平台分配）")
	}
	if (in.Proto == "http" || in.Proto == "https") && strings.TrimSpace(in.Domain) == "" {
		return frp.TunnelInput{}, errors.New("HTTP(S) 隧道必须填写绑定域名")
	}
	if in.LocalIP == "" {
		in.LocalIP = "127.0.0.1"
	}
	return frp.TunnelInput{
		Name: in.Name, Proto: in.Proto, NodeID: in.NodeID,
		LocalIP: in.LocalIP, LocalPort: in.LocalPort,
		RemotePort: in.RemotePort, Domain: strings.TrimSpace(in.Domain),
		Note: in.Note, Extra: in.Extra,
		Encryption: in.Encryption, Compression: in.Compression,
	}, nil
}

// FRPTunnelCreate 新建隧道（平台侧 + 立刻回读镜像）。
func (a *App) FRPTunnelCreate(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	var in frpTunnelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	dom, err := in.toDomain()
	if err != nil {
		middleware.Fail(c, 1001, err.Error())
		return
	}
	if err := a.checkTunnelName(id, dom.Name); err != nil {
		middleware.Fail(c, 1001, err.Error())
		return
	}
	// Cloudflare 的节点挂在统一容器下（doc/16 §2），没有「接入节点」可选：
	// NodeID 留空即可，校验只针对 frp 系平台。
	if p, _ := a.DB.GetFRPPlatform(id); p == nil || p.Kind != frp.KindCloudflared {
		if strings.TrimSpace(dom.NodeID) == "" {
			middleware.Fail(c, 1001, "请选择节点")
			return
		}
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	remoteID, err := a.FRP.CreateTunnel(ctx, id, dom)
	if err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_tunnel_create", fmt.Sprintf("frp_platform:%d", id),
		fmt.Sprintf("新建隧道 %s(%s) -> %s:%d", dom.Name, dom.Proto, dom.LocalIP, dom.LocalPort), ipOf(c))
	// 建完立即同步一次，让新隧道马上出现在列表里（不同步节点，省一次外部请求）
	syncErr := a.FRP.Sync(ctx, id, false)
	out := gin.H{"remote_id": remoteID}
	if syncErr != nil {
		out["sync_error"] = syncErr.Error()
	}
	middleware.OK(c, out)
}

// FRPTunnelUpdate 修改隧道。Sakura 只支持备注/本地地址端口；
// ChmlFrp 额外支持节点与端口，但 http/https 类型官方标注暂不支持修改。
func (a *App) FRPTunnelUpdate(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	var in frpTunnelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	// 类型不可改，但 ChmlFrp 的 /update_tunnel 把 porttype 当必填：
	// 入参没带就用库里当前的类型兜底（真机实测漏发回「端口类型缺失」）。
	proto := strings.ToLower(strings.TrimSpace(in.Proto))
	if proto == "" {
		proto = t.Proto
	}
	dom := frp.TunnelInput{
		Name: strings.TrimSpace(in.Name), Proto: proto, NodeID: in.NodeID,
		LocalIP: strings.TrimSpace(in.LocalIP), LocalPort: in.LocalPort,
		RemotePort: in.RemotePort, Domain: strings.TrimSpace(in.Domain),
		Note: in.Note, Extra: in.Extra,
	}
	if dom.LocalPort < 0 || dom.LocalPort > 65535 {
		middleware.Fail(c, 1001, "本地端口必须在 0-65535 之间（0 表示不修改）")
		return
	}
	if err := a.checkTunnelName(t.PlatformID, dom.Name); err != nil {
		middleware.Fail(c, 1001, err.Error())
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.UpdateTunnel(ctx, t.PlatformID, t.RemoteID, dom); err != nil {
		failFRP(c, err)
		return
	}
	dirty, _ := a.DB.MarkFRPDeploysDirtyByTunnel(t.ID)
	a.audit(a.actorOf(c), "frp_tunnel_update", fmt.Sprintf("frp_tunnel:%d", t.ID),
		fmt.Sprintf("修改隧道 %s（%d 个客户端托管待同步）", t.Name, dirty), ipOf(c))
	syncErr := a.FRP.Sync(ctx, t.PlatformID, false)
	out := gin.H{"ok": true}
	if syncErr != nil {
		out["sync_error"] = syncErr.Error()
	}
	middleware.OK(c, out)
}

// FRPTunnelDelete 删除隧道。
func (a *App) FRPTunnelDelete(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	if t.LockDelete {
		middleware.Fail(c, frpCodeUnsupported, "该隧道在平台侧已被锁定删除，请先解锁")
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.DeleteTunnel(ctx, t.PlatformID, t.RemoteID); err != nil {
		failFRP(c, err)
		return
	}
	_ = a.DB.DeleteFRPTunnel(t.ID)
	// 节点侧托管里若包含这条隧道，容器内的配置已过期：标记待同步（前端出「同步配置」角标）
	dirty, _ := a.DB.MarkFRPDeploysDirtyByTunnel(t.ID)
	a.audit(a.actorOf(c), "frp_tunnel_delete", fmt.Sprintf("frp_tunnel:%d", t.ID),
		fmt.Sprintf("删除隧道 %s（%d 个客户端托管待同步）", t.Name, dirty), ipOf(c))
	_ = a.FRP.Sync(ctx, t.PlatformID, false)
	middleware.OK(c, gin.H{"ok": true})
}

type frpLockInput struct {
	Edit    bool `json:"edit"`
	Delete  bool `json:"delete"`
	Migrate bool `json:"migrate"`
}

// FRPTunnelLock Sakura 专属：锁定编辑/删除/迁移。
func (a *App) FRPTunnelLock(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	var in frpLockInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.LockTunnel(ctx, t.PlatformID, t.RemoteID, in.Edit, in.Delete, in.Migrate); err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_tunnel_lock", fmt.Sprintf("frp_tunnel:%d", t.ID),
		fmt.Sprintf("锁定设置 edit=%v delete=%v migrate=%v", in.Edit, in.Delete, in.Migrate), ipOf(c))
	_ = a.FRP.Sync(ctx, t.PlatformID, false)
	middleware.OK(c, gin.H{"ok": true})
}

type frpMigrateInput struct {
	NodeID string `json:"node_id"`
}

// FRPTunnelMigrate Sakura 专属：迁移隧道到另一节点。
func (a *App) FRPTunnelMigrate(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	var in frpMigrateInput
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.NodeID) == "" {
		middleware.Fail(c, 1001, "请选择目标节点")
		return
	}
	if t.LockMigrate {
		middleware.Fail(c, frpCodeUnsupported, "该隧道在平台侧已被锁定迁移，请先解锁")
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.MigrateTunnel(ctx, t.PlatformID, t.RemoteID, in.NodeID); err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_tunnel_migrate", fmt.Sprintf("frp_tunnel:%d", t.ID),
		"迁移到节点 "+in.NodeID, ipOf(c))
	_ = a.FRP.Sync(ctx, t.PlatformID, false)
	middleware.OK(c, gin.H{"ok": true})
}

// FRPTunnelOffline ChmlFrp 专属：强制断开当前连接。
func (a *App) FRPTunnelOffline(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.OfflineTunnel(ctx, t.PlatformID, t.Name); err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_tunnel_offline", fmt.Sprintf("frp_tunnel:%d", t.ID),
		"强制下线隧道 "+t.Name, ipOf(c))
	_ = a.FRP.Sync(ctx, t.PlatformID, false)
	middleware.OK(c, gin.H{"ok": true})
}

type frpAuthInput struct {
	IP string `json:"ip"`
}

// FRPTunnelAuth Sakura 专属：通过访问认证（ip 留空则授权请求来源 IP）。
func (a *App) FRPTunnelAuth(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	var in frpAuthInput
	_ = c.ShouldBindJSON(&in)
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	authorized, err := a.FRP.TunnelAuth(ctx, t.PlatformID, t.RemoteID, strings.TrimSpace(in.IP))
	if err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_tunnel_auth", fmt.Sprintf("frp_tunnel:%d", t.ID),
		"通过访问认证 ip="+authorized, ipOf(c))
	middleware.OK(c, gin.H{"ip": authorized})
}

// FRPTunnelConfig 下载 frpc 配置。返回裸文本（前端用 fetch+blob 存盘），
// 与 WG 凭证 zip 的下载方式一致。
func (a *App) FRPTunnelConfig(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	cfg, err := a.FRP.TunnelConfig(ctx, t.PlatformID, frp.ConfigTarget{
		RemoteID: t.RemoteID, NodeName: t.NodeName, Name: t.Name,
	})
	if err != nil {
		failFRP(c, err)
		return
	}
	if strings.TrimSpace(cfg) == "" {
		middleware.Fail(c, frpCodeUpstream, "平台返回了空配置")
		return
	}
	p, _ := a.DB.GetFRPPlatform(t.PlatformID)
	kind := "frpc"
	if p != nil {
		kind = p.Kind
	}
	name := fmt.Sprintf("%s-%s.conf", kind, sanitizeFileName(t.Name))
	c.Header("Content-Disposition", "attachment; filename=\""+name+"\"")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(cfg))
}

// FRPTunnelTraffic 单隧道流量曲线。
func (a *App) FRPTunnelTraffic(c *gin.Context) {
	t, ok := a.frpTunnelByParam(c)
	if !ok {
		return
	}
	ctx, cancel := frpCtx(c, frpTimeout)
	defer cancel()
	pts, err := a.FRP.TunnelTraffic(ctx, t.PlatformID, *t)
	if err != nil {
		failFRP(c, err)
		return
	}
	out := make([]gin.H, 0, len(pts))
	for _, p := range pts {
		out = append(out, gin.H{"label": p.Label, "used": p.Used})
	}
	middleware.OK(c, gin.H{"points": out})
}

// ---------- ChmlFrp 二级域名 ----------

type frpSubdomainInput struct {
	Domain  string `json:"domain"`
	Record  string `json:"record"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	TTL     string `json:"ttl"`
	Remarks string `json:"remarks"`
}

// FRPSubdomains 列出平台侧二级域名（目前仅 ChmlFrp 支持）。
func (a *App) FRPSubdomains(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	ctx, cancel := frpCtx(c, frpTimeout)
	defer cancel()
	subs, err := a.FRP.Subdomains(ctx, id)
	if err != nil {
		failFRP(c, err)
		return
	}
	out := make([]gin.H, 0, len(subs))
	for _, s := range subs {
		out = append(out, gin.H{
			"id": s.ID, "domain": s.Domain, "record": s.Record, "type": s.Type,
			"target": s.Target, "ttl": s.TTL, "remarks": s.Remarks,
		})
	}
	middleware.OK(c, gin.H{"subdomains": out})
}

// FRPSubdomainsAvailable 可用的主域名（建解析时供选择）。
func (a *App) FRPSubdomainsAvailable(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	ctx, cancel := frpCtx(c, frpTimeout)
	defer cancel()
	doms, err := a.FRP.AvailableDomains(ctx, id)
	if err != nil {
		failFRP(c, err)
		return
	}
	middleware.OK(c, gin.H{"domains": doms})
}

// frpSubdomainValidate ChmlFrp 的取值白名单（平台侧也会校验，这里先挡掉明显错误）。
func frpSubdomainValidate(in *frpSubdomainInput) error {
	in.Domain = strings.TrimSpace(in.Domain)
	in.Record = strings.TrimSpace(in.Record)
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Target = strings.TrimSpace(in.Target)
	in.TTL = strings.TrimSpace(in.TTL)
	if in.Domain == "" || in.Record == "" || in.Target == "" {
		return errors.New("主域名、主机记录、目标地址均为必填")
	}
	switch in.Type {
	case "A", "AAAA", "CNAME", "SRV":
	default:
		return errors.New("记录类型只支持 A / AAAA / CNAME / SRV")
	}
	switch in.TTL {
	case "1分钟", "2分钟", "5分钟", "10分钟", "15分钟", "30分钟",
		"1小时", "2小时", "5小时", "12小时", "1天":
	default:
		return errors.New("TTL 取值不合法")
	}
	return nil
}

// FRPSubdomainCreate 新建免费二级域名（ChmlFrp）。
func (a *App) FRPSubdomainCreate(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	var in frpSubdomainInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	if err := frpSubdomainValidate(&in); err != nil {
		middleware.Fail(c, 1001, err.Error())
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.CreateSubdomain(ctx, id, in.Domain, in.Record, in.Type, in.Target, in.TTL, in.Remarks); err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_subdomain_create", fmt.Sprintf("frp_platform:%d", id),
		fmt.Sprintf("新建解析 %s.%s", in.Record, in.Domain), ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// FRPSubdomainUpdate 修改二级域名（平台仅允许改 TTL 与目标）。
func (a *App) FRPSubdomainUpdate(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	var in frpSubdomainInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	if err := frpSubdomainValidate(&in); err != nil {
		middleware.Fail(c, 1001, err.Error())
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.UpdateSubdomain(ctx, id, in.Domain, in.Record, in.Target, in.TTL, in.Remarks); err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_subdomain_update", fmt.Sprintf("frp_platform:%d", id),
		fmt.Sprintf("修改解析 %s.%s", in.Record, in.Domain), ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// FRPSubdomainDelete 删除二级域名。DELETE 带 body 在部分代理/客户端上不可靠，
// 这里用查询参数传定位信息。
func (a *App) FRPSubdomainDelete(c *gin.Context) {
	id, ok := a.frpIDParam(c)
	if !ok {
		return
	}
	domain := strings.TrimSpace(c.Query("domain"))
	record := strings.TrimSpace(c.Query("record"))
	if domain == "" || record == "" {
		middleware.Fail(c, 1001, "主域名与主机记录为必填")
		return
	}
	ctx, cancel := frpCtx(c, frpWriteTimeout)
	defer cancel()
	if err := a.FRP.DeleteSubdomain(ctx, id, domain, record); err != nil {
		failFRP(c, err)
		return
	}
	a.audit(a.actorOf(c), "frp_subdomain_delete", fmt.Sprintf("frp_platform:%d", id),
		fmt.Sprintf("删除解析 %s.%s", record, domain), ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// ---------- 小工具 ----------

// frpIDParam 解析路径里的平台 ID。
func (a *App) frpIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return 0, false
	}
	return id, true
}

// frpTunnelByParam 按本地隧道 ID 取记录。
func (a *App) frpTunnelByParam(c *gin.Context) (*store.FRPTunnel, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return nil, false
	}
	t, _ := a.DB.GetFRPTunnel(id)
	if t == nil {
		middleware.Fail(c, 2002, "隧道不存在（可能已在平台侧删除，请先同步）")
		return nil, false
	}
	return t, true
}

// sanitizeFileName 过滤进入下载文件名的字符（隧道名可能含任意字符）。
func sanitizeFileName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "tunnel"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}
