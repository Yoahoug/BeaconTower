package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/assets"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/wg"
	"github.com/gin-gonic/gin"
)

// ---------- WG 组网 API（doc/12 §5） ----------
// 全部位于 /api/v1/admin/wg/*，RequireAuth + RequireCSRF + audit。

type wgNetworkInput struct {
	Subnet    string `json:"subnet"`
	HubIP     string `json:"hub_ip"`
	Iface     string `json:"iface"`
	Keepalive int    `json:"keepalive"`
	MTU       int    `json:"mtu"`
}

type wgSpokeInput struct {
	ServerID int64  `json:"server_id"`
	WgIP     string `json:"wg_ip"`
}

type wgPlanInput struct {
	Network     *wgNetworkInput `json:"network"`
	HubServerID int64           `json:"hub_server_id"`
	HubPort     int             `json:"hub_port"`
	Spokes      []wgSpokeInput  `json:"spokes"`
}

// wgResolveNetwork 取现有网络；无则用输入参数校验并构造（入库发生在 apply）。
func (a *App) wgResolveNetwork(in *wgNetworkInput) (*store.WGNetwork, error) {
	if netRow, err := a.DB.GetWGNetwork(); err != nil || netRow != nil {
		return netRow, err
	}
	if in == nil || in.Subnet == "" || in.HubIP == "" {
		return nil, errors.New("尚未初始化组网，请提供 subnet 与 hub_ip")
	}
	subnet := strings.TrimSpace(in.Subnet)
	if _, _, err := net.ParseCIDR(subnet); err != nil {
		return nil, fmt.Errorf("subnet 格式无效: %q", subnet)
	}
	hubIP := strings.TrimSpace(in.HubIP)
	if ip := net.ParseIP(hubIP); ip == nil {
		return nil, fmt.Errorf("hub_ip 格式无效: %q", hubIP)
	}
	n := &store.WGNetwork{
		Subnet:    subnet,
		HubIP:     hubIP,
		Iface:     strings.TrimSpace(in.Iface),
		Keepalive: in.Keepalive,
		MTU:       in.MTU,
	}
	if n.Iface == "" {
		n.Iface = "wg0"
	}
	if n.Keepalive <= 0 {
		n.Keepalive = 25
	}
	if n.MTU <= 0 {
		n.MTU = 1420
	}
	return n, nil
}

// wgTakenIPs 已占用的 WG IP（成员 + hub 虚拟 IP）。
func (a *App) wgTakenIPs(netRow *store.WGNetwork) ([]string, error) {
	peers, err := a.DB.ListWGPeers()
	if err != nil {
		return nil, err
	}
	taken := []string{netRow.HubIP}
	for _, p := range peers {
		if p.Status != "left" {
			taken = append(taken, p.WgIP)
		}
	}
	return taken, nil
}

func nullI64(n sql.NullInt64) any {
	if n.Valid {
		return n.Int64
	}
	return nil
}

func nullF64(n sql.NullFloat64) any {
	if n.Valid {
		return n.Float64
	}
	return nil
}

// WGOverview 组网总览：网络 + hub 槽位（含月流量）+ 成员 + 服务器清单。
func (a *App) WGOverview(c *gin.Context) {
	netRow, err := a.DB.GetWGNetwork()
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	resp := gin.H{"network": nil, "hubs": []gin.H{}, "peers": []gin.H{},
		"servers": []gin.H{}, "running_task": false}
	if netRow != nil {
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		// hub 槽位
		hubs, _ := a.DB.ListWGHub()
		hubViews := []gin.H{}
		for _, h := range hubs {
			rx, tx, _ := a.DB.WGHubTrafficRange(h.ServerID, monthStart.Unix(), now.Unix())
			name := ""
			if srv, _ := a.DB.GetServer(h.ServerID); srv != nil {
				name = srv.Name
			}
			hubViews = append(hubViews, gin.H{
				"server_id": h.ServerID, "name": name,
				"listen_port": h.ListenPort, "endpoint": h.Endpoint,
				"status": h.Status, "last_error": h.LastError,
				"quota_gb": nullF64(h.QuotaGB),
				"month_rx": rx, "month_tx": tx,
				"is_active": h.ServerID == netRow.ActiveHubServerID,
				"has_keys":  len(h.PrivateKeyEnc) > 0,
			})
		}
		// 成员
		peers, _ := a.DB.ListWGPeers()
		peerViews := []gin.H{}
		inNet := map[int64]bool{}
		for _, p := range peers {
			peerViews = append(peerViews, gin.H{
				"id": p.ID, "kind": p.Kind, "server_id": nullI64(p.ServerID),
				"name": p.Name, "wg_ip": p.WgIP, "public_key": p.PublicKey,
				"managed": p.Managed, "status": p.Status, "last_error": p.LastError,
				"last_handshake": nullI64(p.LastHandshake),
				"rx_bytes":       p.RxBytes, "tx_bytes": p.TxBytes,
				"can_export": len(p.PrivateKeyEnc) > 0,
			})
			if p.ServerID.Valid {
				inNet[p.ServerID.Int64] = p.Status != "left"
			}
		}
		for _, h := range hubs {
			inNet[h.ServerID] = true
		}
		resp["network"] = gin.H{
			"subnet": netRow.Subnet, "hub_ip": netRow.HubIP, "iface": netRow.Iface,
			"keepalive": netRow.Keepalive, "mtu": netRow.MTU,
			"active_hub_server_id": netRow.ActiveHubServerID,
		}
		resp["hubs"] = hubViews
		resp["peers"] = peerViews
		running, _ := a.DB.HasRunningWGTask()
		resp["running_task"] = running
		// 服务器清单（向导选择用）
		servers, _ := a.DB.ListServers()
		srvViews := []gin.H{}
		for _, s := range servers {
			srvViews = append(srvViews, gin.H{
				"id": s.ID, "name": s.Name, "is_self": s.IsSelf, "in_network": inNet[s.ID],
			})
		}
		resp["servers"] = srvViews
	}
	middleware.OK(c, resp)
}

// WGPlan 组网预检（dry-run）：分配 IP + 并发 SSH 探测，返回各节点判定；不落库不改配置。
func (a *App) WGPlan(c *gin.Context) {
	var in wgPlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误")
		return
	}
	netRow, err := a.wgResolveNetwork(in.Network)
	if err != nil {
		middleware.Fail(c, 2010, err.Error())
		return
	}
	hubID, hubPort, blocked, hubView := a.wgPlanHub(netRow, &in)
	if blocked {
		middleware.Fail(c, 2010, hubView["error"].(string))
		return
	}
	alloc, issuesByServer, fatal := a.wgAllocate(netRow, in.Spokes)
	if fatal != "" {
		middleware.Fail(c, 2012, fatal)
		return
	}
	// 并发探测
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	type probeOut struct {
		serverID int64
		issues   []wg.Issue
	}
	roles := map[int64]wg.Role{hubID: wg.RoleHub}
	if netRow.ActiveHubServerID > 0 && hubID != netRow.ActiveHubServerID {
		roles[hubID] = wg.RoleStandby
	}
	targets := []int64{hubID}
	for _, s := range alloc {
		targets = append(targets, s.ServerID)
		roles[s.ServerID] = wg.RoleSpoke
	}
	// 已有 hub 槽位 / 已在网成员 = 重配：预检对同名接口降级为警告
	reprovision := map[int64]bool{}
	if hr, _ := a.DB.GetWGHub(hubID); hr != nil {
		reprovision[hubID] = true
	}
	for _, al := range alloc {
		if al.Reprovision {
			reprovision[al.ServerID] = true
		}
	}
	outCh := make(chan probeOut, len(targets))
	var runWG sync.WaitGroup
	for _, id := range targets {
		runWG.Add(1)
		go func(id int64) {
			defer runWG.Done()
			port := 0
			if id == hubID {
				port = hubPort
			}
			_, issues, err := a.WG.ProbeServer(ctx, id, port, roles[id], reprovision[id])
			if err != nil {
				issues = []wg.Issue{{Level: wg.Err, Msg: err.Error()}}
			}
			outCh <- probeOut{id, issues}
		}(id)
	}
	runWG.Wait()
	close(outCh)
	for o := range outCh {
		issuesByServer[o.serverID] = append(issuesByServer[o.serverID], o.issues...)
	}
	hubIssues := issuesByServer[hubID]
	delete(issuesByServer, hubID)
	middleware.OK(c, gin.H{
		"network": gin.H{"subnet": netRow.Subnet, "hub_ip": netRow.HubIP, "iface": netRow.Iface,
			"keepalive": netRow.Keepalive, "mtu": netRow.MTU},
		"hub":     hubViewWithIssues(hubView, hubIssues),
		"spokes":  a.wgSpokesView(in.Spokes, alloc, issuesByServer),
		"blocked": wg.HasErr(hubIssues),
	})
}

// wgPlanHub 解析/校验 plan 的中心节点，返回 (hubID, port, blocked, 视图)。
func (a *App) wgPlanHub(netRow *store.WGNetwork, in *wgPlanInput) (int64, int, bool, gin.H) {
	hubID := in.HubServerID
	if hubID == 0 {
		hubID = netRow.ActiveHubServerID
	}
	if hubID == 0 {
		return 0, 0, true, gin.H{"error": "缺少中心节点：请指定 hub_server_id"}
	}
	srv, _ := a.DB.GetServer(hubID)
	if srv == nil {
		return 0, 0, true, gin.H{"error": "中心节点不存在"}
	}
	if srv.IsSelf {
		return 0, 0, true, gin.H{"error": "本机节点暂不支持担任中心节点（面板容器无 NET_ADMIN）"}
	}
	hubRow, _ := a.DB.GetWGHub(hubID)
	port := in.HubPort
	if port <= 0 && hubRow != nil {
		port = hubRow.ListenPort
	}
	if port <= 0 {
		port = 51820
	}
	return hubID, port, false, gin.H{
		"server_id": hubID, "name": srv.Name, "listen_port": port,
		"role": wgRoleText(netRow, hubID),
	}
}

func hubViewWithIssues(view gin.H, issues []wg.Issue) gin.H {
	view["issues"] = wgIssuesView(issues)
	return view
}

func wgRoleText(netRow *store.WGNetwork, hubID int64) string {
	if netRow.ActiveHubServerID == 0 || netRow.ActiveHubServerID == hubID {
		return string(wg.RoleHub)
	}
	return string(wg.RoleStandby)
}

// wgAllocate 校验/分配成员 IP（不入库）。
func (a *App) wgAllocate(netRow *store.WGNetwork, spokes []wgSpokeInput) ([]store.WGAlloc, map[int64][]wg.Issue, string) {
	taken, err := a.wgTakenIPs(netRow)
	if err != nil {
		return nil, nil, "读取成员失败: " + err.Error()
	}
	allocator, err := wg.NewAllocator(netRow.Subnet, taken)
	if err != nil {
		return nil, nil, err.Error()
	}
	issues := map[int64][]wg.Issue{}
	var allocs []store.WGAlloc
	for _, s := range spokes {
		srv, _ := a.DB.GetServer(s.ServerID)
		if srv == nil {
			issues[s.ServerID] = append(issues[s.ServerID], wg.Issue{Level: wg.Err, Msg: "节点不存在"})
			continue
		}
		if srv.IsSelf {
			issues[s.ServerID] = append(issues[s.ServerID], wg.Issue{Level: wg.Err, Msg: "本机节点暂不支持经 SSH 管理 WG"})
			continue
		}
		existing, _ := a.DB.GetWGPeerByServer(s.ServerID)
		if existing != nil && existing.Status != "left" {
			allocs = append(allocs, store.WGAlloc{ServerID: s.ServerID, WgIP: existing.WgIP,
				Role: string(wg.RoleSpoke), Reprovision: true})
			continue
		}
		ip := strings.TrimSpace(s.WgIP)
		if ip == "" {
			ip, err = allocator.Next()
			if err != nil {
				issues[s.ServerID] = append(issues[s.ServerID], wg.Issue{Level: wg.Err, Msg: err.Error()})
				continue
			}
		} else if err := allocator.Take(ip); err != nil {
			issues[s.ServerID] = append(issues[s.ServerID], wg.Issue{Level: wg.Err, Msg: err.Error()})
			continue
		}
		allocs = append(allocs, store.WGAlloc{ServerID: s.ServerID, WgIP: ip, Role: string(wg.RoleSpoke)})
	}
	return allocs, issues, ""
}

func wgIssuesView(issues []wg.Issue) []gin.H {
	out := []gin.H{}
	for _, i := range issues {
		lvl := "warn"
		if i.Level == wg.Err {
			lvl = "error"
		}
		out = append(out, gin.H{"level": lvl, "msg": i.Msg})
	}
	return out
}

func (a *App) wgSpokesView(spokes []wgSpokeInput, allocs []store.WGAlloc, issues map[int64][]wg.Issue) []gin.H {
	byServer := map[int64]store.WGAlloc{}
	for _, al := range allocs {
		byServer[al.ServerID] = al
	}
	out := []gin.H{}
	for _, s := range spokes {
		name := ""
		if srv, _ := a.DB.GetServer(s.ServerID); srv != nil {
			name = srv.Name
		}
		al := byServer[s.ServerID]
		out = append(out, gin.H{
			"server_id": s.ServerID, "name": name, "wg_ip": al.WgIP,
			"allocated": al.WgIP != "",
			"issues":    wgIssuesView(issues[s.ServerID]),
		})
	}
	return out
}

// WGApply 执行组网：确保网络/hub/成员记录就绪 → 建任务 → 后台执行。
func (a *App) WGApply(c *gin.Context) {
	var in wgPlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误")
		return
	}
	if running, _ := a.DB.HasRunningWGTask(); running {
		middleware.Fail(c, 1004, "已有组网任务在执行，请等待完成")
		return
	}
	netRow, err := a.wgResolveNetwork(in.Network)
	if err != nil {
		middleware.Fail(c, 2010, err.Error())
		return
	}
	hubID := in.HubServerID
	if hubID == 0 {
		hubID = netRow.ActiveHubServerID
	}
	if hubID == 0 {
		middleware.Fail(c, 2010, "缺少中心节点")
		return
	}
	hubSrv, _ := a.DB.GetServer(hubID)
	if hubSrv == nil || hubSrv.IsSelf {
		middleware.Fail(c, 2010, "中心节点不存在或不支持")
		return
	}
	now := time.Now().Unix()
	// 网络（首次入库）
	if cur, _ := a.DB.GetWGNetwork(); cur == nil {
		netRow.CreatedAt, netRow.UpdatedAt = now, now
		if err := a.DB.EnsureWGNetwork(netRow); err != nil {
			middleware.Fail(c, 5000, "网络初始化失败: "+err.Error())
			return
		}
		netRow, _ = a.DB.GetWGNetwork()
	}
	// hub 槽位（存在则复用；缺钥由引擎自愈）
	hubRow, _ := a.DB.GetWGHub(hubID)
	hubExisted := hubRow != nil
	if hubRow == nil {
		kp, err := wg.GenerateKeyPair()
		if err != nil {
			middleware.Fail(c, 5000, "生成 hub 密钥失败")
			return
		}
		port := in.HubPort
		if port <= 0 {
			port = 51820
		}
		hubRow = &store.WGHub{
			ServerID: hubID, ListenPort: port,
			PublicKey:     kp.Public,
			PrivateKeyEnc: a.WG.Encrypt(kp.Private),
			Endpoint:      a.WG.ResolveEndpoint(hubID, port),
			Status:        "pending",
		}
		if err := a.DB.UpsertWGHub(hubRow); err != nil {
			middleware.Fail(c, 5000, "hub 槽位写入失败: "+err.Error())
			return
		}
	}
	// 首个 hub 成为现役；非现役为备胎（warm standby）
	if netRow.ActiveHubServerID == 0 {
		_ = a.DB.SetActiveHub(hubID, now)
		netRow.ActiveHubServerID = hubID
	}
	alloc, issues, fatal := a.wgAllocate(netRow, in.Spokes)
	if fatal != "" {
		middleware.Fail(c, 2012, fatal)
		return
	}
	for _, al := range alloc {
		for _, i := range issues[al.ServerID] {
			if i.Level == wg.Err {
				middleware.Fail(c, 2010, "成员预检分配失败: "+i.Msg)
				return
			}
		}
	}
	names := map[int64]string{hubID: hubSrv.Name}
	for _, s := range in.Spokes {
		if srv, _ := a.DB.GetServer(s.ServerID); srv != nil {
			names[s.ServerID] = srv.Name
		}
	}
	// 确保成员记录（已存在则复用 IP/密钥并重置状态）
	allocByServer := map[int64]store.WGAlloc{}
	for _, al := range alloc {
		allocByServer[al.ServerID] = al
	}
	for _, s := range in.Spokes {
		al, ok := allocByServer[s.ServerID]
		if !ok {
			continue
		}
		existing, _ := a.DB.GetWGPeerByServer(s.ServerID)
		if existing != nil {
			existing.Status = "pending"
			existing.LastError = ""
			_ = a.DB.UpdateWGPeer(existing)
			continue
		}
		kp, err := wg.GenerateKeyPair()
		if err != nil {
			middleware.Fail(c, 5000, "生成密钥失败")
			return
		}
		psk, err := wg.GeneratePSK()
		if err != nil {
			middleware.Fail(c, 5000, "生成 PSK 失败")
			return
		}
		srvID := sql.NullInt64{Int64: s.ServerID, Valid: true}
		if _, err := a.DB.InsertWGPeer(&store.WGPeer{
			Kind: "server", ServerID: srvID, Name: names[s.ServerID],
			WgIP: al.WgIP, PublicKey: kp.Public,
			PrivateKeyEnc: a.WG.Encrypt(kp.Private), PskEnc: a.WG.Encrypt(psk),
			Managed: true, Status: "pending", CreatedAt: now,
		}); err != nil {
			middleware.Fail(c, 2012, "成员写入失败（IP 冲突？）: "+err.Error())
			return
		}
	}
	// 任务 + 步骤
	allocs := []store.WGAlloc{{ServerID: hubID, WgIP: netRow.HubIP,
		Role: wgRoleText(netRow, hubID), Reprovision: hubExisted}}
	allocs = append(allocs, alloc...)
	payload, _ := json.Marshal(store.WGTaskPayload{Allocations: allocs})
	taskID, err := a.DB.InsertWGTask(&store.WGTask{Kind: "apply", Status: "running",
		Payload: string(payload), CreatedAt: now})
	if err != nil {
		middleware.Fail(c, 5000, "任务创建失败: "+err.Error())
		return
	}
	seq := int64(0)
	if _, err := a.DB.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: seq,
		ServerID: sql.NullInt64{Int64: hubID, Valid: true},
		Title:    "中心节点 · " + names[hubID], Status: "pending"}); err != nil {
		middleware.Fail(c, 5000, "步骤创建失败: "+err.Error())
		return
	}
	for _, al := range alloc {
		seq++
		_, _ = a.DB.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: seq,
			ServerID: sql.NullInt64{Int64: al.ServerID, Valid: true},
			Title:    "接入 · " + names[al.ServerID], Status: "pending"})
	}
	go a.WG.RunApply(taskID)
	a.audit(a.actorOf(c), "wg_apply", fmt.Sprintf("task:%d", taskID),
		fmt.Sprintf("hub=%d spokes=%v", hubID, spokeIDs(alloc)), ipOf(c))
	middleware.OK(c, gin.H{"task_id": taskID})
}

func spokeIDs(allocs []store.WGAlloc) []int64 {
	out := []int64{}
	for _, a := range allocs {
		out = append(out, a.ServerID)
	}
	return out
}

// WGTaskGet 任务详情（含步骤）。
func (a *App) WGTaskGet(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "任务 ID 无效")
		return
	}
	task, _ := a.DB.GetWGTask(id)
	if task == nil {
		middleware.Fail(c, 2002, "任务不存在")
		return
	}
	steps, _ := a.DB.ListWGTaskSteps(id)
	stepViews := []gin.H{}
	for _, st := range steps {
		stepViews = append(stepViews, gin.H{
			"id": st.ID, "seq": st.Seq, "server_id": nullI64(st.ServerID),
			"title": st.Title, "status": st.Status, "log": st.Log,
		})
	}
	middleware.OK(c, gin.H{
		"id": task.ID, "kind": task.Kind, "status": task.Status,
		"result": task.Result, "created_at": task.CreatedAt,
		"finished_at": nullI64(task.FinishedAt),
		"steps":       stepViews,
	})
}

// WGTaskList 最近任务。
func (a *App) WGTaskList(c *gin.Context) {
	tasks, err := a.DB.ListWGTasks(10)
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	out := []gin.H{}
	for _, t := range tasks {
		out = append(out, gin.H{"id": t.ID, "kind": t.Kind, "status": t.Status,
			"result": t.Result, "created_at": t.CreatedAt, "finished_at": nullI64(t.FinishedAt)})
	}
	middleware.OK(c, gin.H{"tasks": out})
}

// ---------- 设备（Mac/iPhone 等非 SSH 成员） ----------

type wgDeviceInput struct {
	Name string `json:"name"`
}

// WGDeviceCreate 新增设备成员：面板生成密钥，前端取 conf 出二维码。
func (a *App) WGDeviceCreate(c *gin.Context) {
	var in wgDeviceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		middleware.Fail(c, 1001, "设备名称不能为空")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil || netRow.ActiveHubServerID == 0 {
		middleware.Fail(c, 2010, "尚未初始化组网或缺少现役中心节点")
		return
	}
	if running, _ := a.DB.HasRunningWGTask(); running {
		middleware.Fail(c, 1004, "组网任务执行中，稍后再试")
		return
	}
	taken, _ := a.wgTakenIPs(netRow)
	allocator, err := wg.NewAllocator(netRow.Subnet, taken)
	if err != nil {
		middleware.Fail(c, 2010, err.Error())
		return
	}
	ip, err := allocator.Next()
	if err != nil {
		middleware.Fail(c, 2010, err.Error())
		return
	}
	kp, err := wg.GenerateKeyPair()
	if err != nil {
		middleware.Fail(c, 5000, "生成密钥失败")
		return
	}
	psk, err := wg.GeneratePSK()
	if err != nil {
		middleware.Fail(c, 5000, "生成 PSK 失败")
		return
	}
	id, err := a.DB.InsertWGPeer(&store.WGPeer{
		Kind: "device", Name: name, WgIP: ip, PublicKey: kp.Public,
		PrivateKeyEnc: a.WG.Encrypt(kp.Private), PskEnc: a.WG.Encrypt(psk),
		Managed: true, Status: "pending", CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		middleware.Fail(c, 2012, "设备写入失败（IP 冲突？）: "+err.Error())
		return
	}
	// 立即热加到现役 hub（失败仅告警，设备状态留 pending，可在 hub 就绪后重试）
	hub, _ := a.DB.GetWGHub(netRow.ActiveHubServerID)
	warn := ""
	if hub != nil {
		if err := a.wgHubAddOne(hub, netRow, kp.Public, psk, ip, name); err != nil {
			warn = "hub 热加失败: " + err.Error()
		}
	}
	a.audit(a.actorOf(c), "wg_device_create", name, "ip="+ip+" "+warn, ipOf(c))
	middleware.OK(c, gin.H{"id": id, "wg_ip": ip, "warn": warn})
}

// wgHubAddOne 向 hub 热加单个 peer（设备新增路径，同步执行）。
func (a *App) wgHubAddOne(hub *store.WGHub, netRow *store.WGNetwork, pub, psk, ip, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, closer, err := a.WG.DialHub(ctx, hub.ServerID)
	if err != nil {
		return err
	}
	defer closer()
	bits, err := wg.SubnetBits(netRow.Subnet)
	if err != nil {
		return err
	}
	peer := wg.Peer{
		Comment:      name,
		PublicKey:    pub,
		PresharedKey: psk,
		AllowedIPs:   []string{ip + "/" + strconv.Itoa(bits)},
	}
	return wg.HubAddPeer(ctx, conn, netRow.Iface, peer, ip+"/"+strconv.Itoa(bits))
}

// WGPeerConf 导出成员配置（设备凭证 / 二维码内容）。
// 仅 managed 且面板持有私钥的成员可导出；hub 参数指定目标中心（默认现役，切换场景用）。
func (a *App) WGPeerConf(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "ID 无效")
		return
	}
	peer, _ := a.DB.GetWGPeer(id)
	if peer == nil {
		middleware.Fail(c, 2002, "成员不存在")
		return
	}
	if !peer.Managed || len(peer.PrivateKeyEnc) == 0 {
		middleware.Fail(c, 2010, "该成员为导入成员（面板未持有私钥），无法导出配置")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	hubID := netRow.ActiveHubServerID
	if q := c.Query("hub"); q != "" {
		if v, e := strconv.ParseInt(q, 10, 64); e == nil {
			hubID = v
		}
	}
	hub, _ := a.DB.GetWGHub(hubID)
	if hub == nil || !wg.ValidKey(hub.PublicKey) {
		middleware.Fail(c, 2010, "目标中心节点未就绪")
		return
	}
	endpoint := hub.Endpoint
	if endpoint == "" {
		endpoint = a.WG.ResolveEndpoint(hub.ServerID, hub.ListenPort)
	}
	if endpoint == "" {
		middleware.Fail(c, 2010, "中心节点端点未知")
		return
	}
	priv, err := a.WG.DecryptBlob(peer.PrivateKeyEnc)
	if err != nil {
		middleware.Fail(c, 5000, "私钥解密失败")
		return
	}
	psk, _ := a.WG.DecryptBlob(peer.PskEnc)
	bits, err := wg.SubnetBits(netRow.Subnet)
	if err != nil {
		middleware.Fail(c, 2010, err.Error())
		return
	}
	conf, err := wg.SpokeConfFile(priv, peer.WgIP+"/"+strconv.Itoa(bits), hub.PublicKey,
		endpoint, netRow.Subnet, psk, netRow.Keepalive, netRow.MTU)
	if err != nil {
		middleware.Fail(c, 5000, "配置渲染失败: "+err.Error())
		return
	}
	tag := "B"
	if hubID == netRow.ActiveHubServerID {
		tag = "A"
	}
	a.audit(a.actorOf(c), "wg_peer_conf_export", peer.Name,
		"hub="+strconv.FormatInt(hubID, 10), ipOf(c))
	middleware.OK(c, gin.H{
		"conf": conf,
		"filename": strings.ReplaceAll(peer.Name, " ", "_") + "-" + tag + ".conf",
		"endpoint": endpoint, "wg_ip": peer.WgIP,
	})
}

// WGPeerDelete 将成员移出网络并删除记录。
func (a *App) WGPeerDelete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "ID 无效")
		return
	}
	peer, _ := a.DB.GetWGPeer(id)
	if peer == nil {
		middleware.Fail(c, 2002, "成员不存在")
		return
	}
	if running, _ := a.DB.HasRunningWGTask(); running {
		middleware.Fail(c, 1004, "组网任务执行中，稍后再试")
		return
	}
	detail, err := a.WG.RemovePeer(peer)
	if err != nil {
		middleware.Fail(c, 2011, "移除失败: "+err.Error()+"（"+detail+"）")
		return
	}
	if err := a.DB.DeleteWGPeer(id); err != nil {
		middleware.Fail(c, 5000, "记录删除失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_peer_delete", peer.Name, "ip="+peer.WgIP+" "+detail, ipOf(c))
	middleware.OK(c, gin.H{"detail": detail})
}

// WGPeerVerify 手动复验成员连通性（SSH 查看接口与握手）。
func (a *App) WGPeerVerify(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "ID 无效")
		return
	}
	peer, _ := a.DB.GetWGPeer(id)
	if peer == nil || peer.Kind != "server" || !peer.ServerID.Valid {
		middleware.Fail(c, 2002, "成员不存在或非 SSH 成员")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	online, detail, err := a.WG.VerifyPeer(ctx, peer.ServerID.Int64, netRow)
	if err != nil {
		middleware.Fail(c, 2011, err.Error())
		return
	}
	peer.CheckedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	if online {
		peer.Status = "online"
		peer.LastError = ""
	} else {
		peer.Status = "offline"
		peer.LastError = detail
	}
	_ = a.DB.UpdateWGPeer(peer)
	middleware.OK(c, gin.H{"online": online, "detail": detail})
}

// ---------- 导入现有网络 / hub 切换（doc/12 §6-7） ----------

type wgImportInput struct {
	HubServerID     int64   `json:"hub_server_id"`
	StandbyServerID int64   `json:"standby_server_id"`
	Candidates      []int64 `json:"candidates"`
}

// WGImport 导入现有 WG 网络：读现役 hub 实况建网收编成员，备援建 warm standby。
func (a *App) WGImport(c *gin.Context) {
	var in wgImportInput
	if err := c.ShouldBindJSON(&in); err != nil || in.HubServerID == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 hub_server_id")
		return
	}
	if running, _ := a.DB.HasRunningWGTask(); running {
		middleware.Fail(c, 1004, "已有组网任务在执行，请等待完成")
		return
	}
	hubSrv, _ := a.DB.GetServer(in.HubServerID)
	if hubSrv == nil || hubSrv.IsSelf {
		middleware.Fail(c, 2010, "中心节点不存在或不支持")
		return
	}
	if in.StandbyServerID > 0 {
		st, _ := a.DB.GetServer(in.StandbyServerID)
		if st == nil || st.IsSelf {
			middleware.Fail(c, 2010, "备援节点不存在或不支持")
			return
		}
	}
	// 候选去重并剔除 hub/standby
	excl := map[int64]bool{in.HubServerID: true, in.StandbyServerID: true}
	var candidates []int64
	for _, id := range in.Candidates {
		if !excl[id] {
			excl[id] = true
			candidates = append(candidates, id)
		}
	}
	now := time.Now().Unix()
	payload, _ := json.Marshal(wg.ImportInput{
		HubServerID:     in.HubServerID,
		StandbyServerID: in.StandbyServerID,
		Candidates:      candidates,
	})
	taskID, err := a.DB.InsertWGTask(&store.WGTask{Kind: "import", Status: "running",
		Payload: string(payload), CreatedAt: now})
	if err != nil {
		middleware.Fail(c, 5000, "任务创建失败: "+err.Error())
		return
	}
	seq := int64(0)
	nameOf := func(id int64) string {
		if s, _ := a.DB.GetServer(id); s != nil {
			return s.Name
		}
		return strconv.FormatInt(id, 10)
	}
	steps := []*store.WGTaskStep{{
		TaskID: taskID, Seq: seq, ServerID: sql.NullInt64{Int64: in.HubServerID, Valid: true},
		Title: "读取中心节点 · " + nameOf(in.HubServerID), Status: "pending",
	}}
	if in.StandbyServerID > 0 {
		seq++
		steps = append(steps, &store.WGTaskStep{TaskID: taskID, Seq: seq,
			ServerID: sql.NullInt64{Int64: in.StandbyServerID, Valid: true},
			Title:    "读取备援节点 · " + nameOf(in.StandbyServerID), Status: "pending"})
	}
	for _, id := range candidates {
		seq++
		steps = append(steps, &store.WGTaskStep{TaskID: taskID, Seq: seq,
			ServerID: sql.NullInt64{Int64: id, Valid: true},
			Title:    "匹配成员 · " + nameOf(id), Status: "pending"})
	}
	for _, st := range steps {
		if _, err := a.DB.InsertWGTaskStep(st); err != nil {
			middleware.Fail(c, 5000, "步骤创建失败: "+err.Error())
			return
		}
	}
	go a.WG.RunImport(taskID)
	a.audit(a.actorOf(c), "wg_import", fmt.Sprintf("task:%d", taskID),
		fmt.Sprintf("hub=%d standby=%d candidates=%v", in.HubServerID, in.StandbyServerID, candidates), ipOf(c))
	middleware.OK(c, gin.H{"task_id": taskID})
}

type wgSwitchInput struct {
	TargetServerID int64 `json:"target_server_id"`
	CanaryServerID int64 `json:"canary_server_id"`
}

// WGSwitchHub 一键切换现役 hub（金丝雀两阶段，失败自动回滚）。
func (a *App) WGSwitchHub(c *gin.Context) {
	var in wgSwitchInput
	if err := c.ShouldBindJSON(&in); err != nil || in.TargetServerID == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 target_server_id")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	if in.TargetServerID == netRow.ActiveHubServerID {
		middleware.Fail(c, 2010, "该节点已是现役中心节点")
		return
	}
	target, _ := a.DB.GetWGHub(in.TargetServerID)
	if target == nil {
		middleware.Fail(c, 2010, "目标节点尚未纳管为 hub/备胎（请先导入或组网）")
		return
	}
	if running, _ := a.DB.HasRunningWGTask(); running {
		middleware.Fail(c, 1004, "已有组网任务在执行，请等待完成")
		return
	}
	// 金丝雀：指定或自动挑选在线 SSH 成员
	canaryID := in.CanaryServerID
	if canaryID == 0 {
		if p := a.WG.PickCanary(); p != nil && p.ServerID.Valid {
			canaryID = p.ServerID.Int64
		}
	}
	now := time.Now().Unix()
	payload, _ := json.Marshal(wg.SwitchInput{TargetServerID: in.TargetServerID, CanaryServerID: canaryID})
	taskID, err := a.DB.InsertWGTask(&store.WGTask{Kind: "switch_hub", Status: "running",
		Payload: string(payload), CreatedAt: now})
	if err != nil {
		middleware.Fail(c, 5000, "任务创建失败: "+err.Error())
		return
	}
	nameOf := func(id int64) string {
		if s, _ := a.DB.GetServer(id); s != nil {
			return s.Name
		}
		return strconv.FormatInt(id, 10)
	}
	seq := int64(0)
	if _, err := a.DB.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: seq,
		ServerID: sql.NullInt64{Int64: in.TargetServerID, Valid: true},
		Title:    "校正备援 hub · " + nameOf(in.TargetServerID), Status: "pending"}); err != nil {
		middleware.Fail(c, 5000, "步骤创建失败: "+err.Error())
		return
	}
	// 其余 SSH 成员步骤（金丝雀 seq=1，其余 seq≥2）
	peers, _ := a.DB.ListWGPeers()
	canaryChosen := false
	for _, p := range peers {
		if p.Kind != "server" || !p.ServerID.Valid {
			continue
		}
		if p.ServerID.Int64 == in.TargetServerID {
			continue
		}
		seq++
		title := "切换 · " + p.Name
		if !canaryChosen && p.ServerID.Int64 == canaryID {
			title = "金丝雀 · " + p.Name
			canaryChosen = true
		}
		if _, err := a.DB.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: seq,
			ServerID: sql.NullInt64{Int64: p.ServerID.Int64, Valid: true},
			Title:    title, Status: "pending"}); err != nil {
			middleware.Fail(c, 5000, "步骤创建失败: "+err.Error())
			return
		}
	}
	go a.WG.RunSwitchHub(taskID)
	a.audit(a.actorOf(c), "wg_switch_hub", fmt.Sprintf("task:%d", taskID),
		fmt.Sprintf("target=%d canary=%d", in.TargetServerID, canaryID), ipOf(c))
	middleware.OK(c, gin.H{"task_id": taskID})
}

// WGPatrol 手动触发一次巡检。
func (a *App) WGPatrol(c *gin.Context) {
	if err := a.WG.PatrolOnce(); err != nil {
		middleware.Fail(c, 2010, err.Error())
		return
	}
	middleware.OK(c, gin.H{"ok": true})
}

// ---------- 资产中转（doc/12 §8） ----------

type wgAssetInput struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Arch    string   `json:"arch"`
	SHA256  string   `json:"sha256"`
	Sources []string `json:"sources"`
	Note    string   `json:"note"`
}

// WGAssetList 资产列表。
func (a *App) WGAssetList(c *gin.Context) {
	list, err := a.DB.ListWGAssets()
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	out := []gin.H{}
	for _, as := range list {
		out = append(out, gin.H{
			"id": as.ID, "name": as.Name, "version": as.Version, "arch": as.Arch,
			"sha256": as.SHA256, "sources": as.Sources, "cached": as.Path != "",
			"size": as.Size, "note": as.Note, "updated_at": as.UpdatedAt,
		})
	}
	middleware.OK(c, gin.H{"assets": out})
}

// WGAssetUpsert 注册/更新资产（sources 支持官方直链、镜像、自有仓库 release）。
func (a *App) WGAssetUpsert(c *gin.Context) {
	var in wgAssetInput
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" || len(in.Sources) == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 name 与至少一个 source")
		return
	}
	name := strings.TrimSpace(in.Name)
	if len(name) > 64 || strings.ContainsAny(name, "/\\. ") {
		middleware.Fail(c, 1001, "资产名仅允许字母数字与下划线/短横线")
		return
	}
	for _, s := range in.Sources {
		if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			middleware.Fail(c, 1001, "source 须为 http(s) URL: "+s)
			return
		}
	}
	now := time.Now().Unix()
	id, err := a.DB.UpsertWGAsset(&store.WGAsset{
		Name: name, Version: strings.TrimSpace(in.Version), Arch: strings.TrimSpace(in.Arch),
		SHA256: strings.TrimSpace(in.SHA256), Sources: in.Sources,
		Note: strings.TrimSpace(in.Note), UpdatedAt: now,
	})
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_asset_upsert", name, "", ipOf(c))
	middleware.OK(c, gin.H{"id": id})
}

// WGAssetDelete 删除资产（缓存文件一并清理）。
func (a *App) WGAssetDelete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "ID 无效")
		return
	}
	as, _ := a.DB.GetWGAsset(id)
	if as == nil {
		middleware.Fail(c, 2002, "资产不存在")
		return
	}
	if as.Path != "" {
		_ = os.Remove(as.Path)
	}
	if err := a.DB.DeleteWGAsset(id); err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_asset_delete", as.Name, "", ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// WGAssetProbe 并发测速全部源的镜像变体（GitHub 链接自动生成加速候选）。
func (a *App) WGAssetProbe(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "ID 无效")
		return
	}
	as, _ := a.DB.GetWGAsset(id)
	if as == nil {
		middleware.Fail(c, 2002, "资产不存在")
		return
	}
	var urls []string
	for _, s := range as.Sources {
		urls = append(urls, assets.BuildVariants(s)...)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	results := assets.Probe(ctx, urls, 10*time.Second)
	middleware.OK(c, gin.H{"results": results})
}

// WGAssetFetch 按测速序下载并校验，缓存到 data/assets/。
func (a *App) WGAssetFetch(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "ID 无效")
		return
	}
	as, _ := a.DB.GetWGAsset(id)
	if as == nil {
		middleware.Fail(c, 2002, "资产不存在")
		return
	}
	var urls []string
	for _, s := range as.Sources {
		urls = append(urls, assets.BuildVariants(s)...)
	}
	pctx, pcancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	results := assets.Probe(pctx, urls, 10*time.Second)
	pcancel()
	dest := filepath.Join(a.Cfg.DataDir, "assets", as.Name+"-"+as.Version)
	fctx, fcancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer fcancel()
	used, err := assets.Fetch(fctx, results, dest, as.SHA256, 512<<20)
	if err != nil {
		middleware.Fail(c, 2011, "下载失败: "+err.Error())
		return
	}
	st, _ := os.Stat(dest)
	sum := as.SHA256
	if sum == "" {
		if h, err := fileSHA256(dest); err == nil {
			sum = h
		}
	}
	as.Path, as.Size, as.SHA256, as.UpdatedAt = dest, st.Size(), sum, time.Now().Unix()
	if _, err := a.DB.UpsertWGAsset(as); err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_asset_fetch", as.Name, "via="+used, ipOf(c))
	middleware.OK(c, gin.H{"path": dest, "size": as.Size, "sha256": sum, "via": used})
}

// WGAssetPush 将缓存的资产推送到指定节点 /usr/local/bin/（兜底安装路径）。
func (a *App) WGAssetPush(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		middleware.Fail(c, 1001, "ID 无效")
		return
	}
	as, _ := a.DB.GetWGAsset(id)
	if as == nil {
		middleware.Fail(c, 2002, "资产不存在")
		return
	}
	if as.Path == "" {
		middleware.Fail(c, 2010, "资产尚未下载缓存，请先执行下载")
		return
	}
	var in struct {
		ServerID int64 `json:"server_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.ServerID == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 server_id")
		return
	}
	if _, err := a.DB.GetWGPeerByServer(in.ServerID); err != nil {
		middleware.Fail(c, 2002, "目标节点不是网内成员")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, closer, err := a.WG.DialHub(ctx, in.ServerID)
	if err != nil {
		middleware.Fail(c, 2011, err.Error())
		return
	}
	defer closer()
	if err := wg.InstallAsset(ctx, conn, as.Name, as.Path); err != nil {
		middleware.Fail(c, 2011, "推送失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_asset_push", as.Name, fmt.Sprintf("server=%d", in.ServerID), ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// fileSHA256 文件哈希（fetch 路径校验用）。
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
