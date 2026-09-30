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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Yoahoug/BeaconTower/internal/assets"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/wg"
	"github.com/gin-gonic/gin"
)

// ---------- WG 组网 API（doc/12 §5） ----------
// 全部位于 /api/v1/admin/wg/*，RequireAuth + RequireCSRF + audit。

var (
	// ifaceRe 接口名白名单（字母数字开头，含短横线，≤15 字符 IFNAMSIZ）
	ifaceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,14}$`)
	// assetNameRe 资产名白名单：资产名会拼进节点 root 的 shell 命令，必须严格白名单
	assetNameRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	assetVersionRe = regexp.MustCompile(`^[A-Za-z0-9._-]{0,64}$`)
)

// validMemberName 成员名入 conf 注释与文件名：拒绝控制字符（含换行，防 conf 注入）。
func validMemberName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

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

// wgOpLock 串行化任务型操作：拿住进程内互斥锁并检查无 running 任务。
// 返回 false 表示已拒绝（锁已释放或响应已写）。成功后须配对 wgOpUnlock。
func (a *App) wgOpLock(c *gin.Context, hint string) bool {
	a.wgOpMu.Lock()
	if running, _ := a.DB.HasRunningWGTask(); running {
		a.wgOpMu.Unlock()
		middleware.Fail(c, 1004, hint)
		return false
	}
	return true
}

func (a *App) wgOpUnlock() { a.wgOpMu.Unlock() }

// wgResolveNetwork 取现有网络；无则用输入参数校验并构造（入库发生在 apply）。
func (a *App) wgResolveNetwork(in *wgNetworkInput) (*store.WGNetwork, error) {
	if netRow, err := a.DB.GetWGNetwork(); err != nil || netRow != nil {
		return netRow, err
	}
	if in == nil || in.Subnet == "" || in.HubIP == "" {
		return nil, errors.New("尚未初始化组网，请提供 subnet 与 hub_ip")
	}
	subnet := strings.TrimSpace(in.Subnet)
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("subnet 格式无效: %q", subnet)
	}
	hubIP := strings.TrimSpace(in.HubIP)
	ip := net.ParseIP(hubIP)
	if ip == nil {
		return nil, fmt.Errorf("hub_ip 格式无效: %q", hubIP)
	}
	if !ipNet.Contains(ip) {
		return nil, fmt.Errorf("hub_ip %s 不在子网 %s 内", hubIP, subnet)
	}
	iface := strings.TrimSpace(in.Iface)
	if iface == "" {
		iface = "wg0"
	}
	if !ifaceRe.MatchString(iface) {
		return nil, fmt.Errorf("接口名仅允许字母数字与短横线（≤15 字符）: %q", iface)
	}
	if in.MTU > 9000 || (in.MTU > 0 && in.MTU < 1280) {
		return nil, fmt.Errorf("MTU 越界（1280-9000 或 0=默认）: %d", in.MTU)
	}
	if in.Keepalive < 0 || in.Keepalive > 3600 {
		return nil, fmt.Errorf("keepalive 越界（0-3600 秒）: %d", in.Keepalive)
	}
	n := &store.WGNetwork{
		Subnet:    subnet,
		HubIP:     hubIP,
		Iface:     iface,
		Keepalive: in.Keepalive,
		MTU:       in.MTU,
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
// servers 无条件返回：导入/向导正是「未初始化→初始化」的入口，初始化前也要能选节点。
func (a *App) WGOverview(c *gin.Context) {
	netRow, err := a.DB.GetWGNetwork()
	if err != nil {
		middleware.Fail(c, 5000, err.Error())
		return
	}
	resp := gin.H{"network": nil, "hubs": []gin.H{}, "peers": []gin.H{},
		"servers": []gin.H{}, "running_task": false}
	// 服务器清单（向导/导入选择用；未初始化时角色全为空）
	servers, _ := a.DB.ListServers()
	hubs, _ := a.DB.ListWGHub()
	peers, _ := a.DB.ListWGPeers()
	// 组网角色：hub=现役中心 / standby=备援 / spoke=普通成员（前端据此决定能否当接管目标等）
	roleOf := map[int64]string{}
	inNet := map[int64]bool{}
	activeID := int64(0)
	if netRow != nil {
		activeID = netRow.ActiveHubServerID
	}
	for _, h := range hubs {
		if h.Status == "retired" {
			continue
		}
		role := "standby"
		if h.ServerID == activeID {
			role = "hub"
		}
		roleOf[h.ServerID] = role
		inNet[h.ServerID] = true
	}
	for _, p := range peers {
		if !p.ServerID.Valid || p.Status == "left" {
			continue
		}
		inNet[p.ServerID.Int64] = true
		if roleOf[p.ServerID.Int64] == "" {
			roleOf[p.ServerID.Int64] = "spoke"
		}
	}
	srvViews := []gin.H{}
	for _, s := range servers {
		cred, _ := a.DB.GetCredential(s.ID)
		srvViews = append(srvViews, gin.H{
			"id": s.ID, "name": s.Name, "is_self": s.IsSelf, "in_network": inNet[s.ID],
			// 本机节点的 ssh_ready 决定它能否当成员（前端据此提示「先录宿主 SSH」）
			"ssh_ready": sshReady(cred),
			"wg_role":   roleOf[s.ID],
		})
	}
	resp["servers"] = srvViews
	if netRow != nil {
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		// hub 槽位
		hubViews, retiredViews := []gin.H{}, []gin.H{}
		for _, h := range hubs {
			if h.Status == "retired" {
				sname := ""
				if srv, _ := a.DB.GetServer(h.ServerID); srv != nil {
					sname = srv.Name
				}
				retiredViews = append(retiredViews, gin.H{"server_id": h.ServerID, "name": sname})
				continue
			}
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
				// 额度估算口径：云厂商（阿里云/腾讯云轻量等）只对出方向流量计费，
				// 中转场景扣额度 ≈ 本 hub 的出方向累计（tx），rx 仅作参考展示
				"month_billed": tx,
				"is_active":    h.ServerID == netRow.ActiveHubServerID,
				"has_keys":     len(h.PrivateKeyEnc) > 0,
			})
		}
		// 成员
		peerViews := []gin.H{}
		for _, p := range peers {
			peerViews = append(peerViews, gin.H{
				"id": p.ID, "kind": p.Kind, "server_id": nullI64(p.ServerID),
				"name": p.Name, "wg_ip": p.WgIP, "public_key": p.PublicKey,
				"managed": p.Managed, "status": p.Status, "last_error": p.LastError,
				"last_handshake": nullI64(p.LastHandshake),
				"rx_bytes":       p.RxBytes, "tx_bytes": p.TxBytes,
				"can_export": len(p.PrivateKeyEnc) > 0,
			})
		}
		resp["network"] = gin.H{
			"subnet": netRow.Subnet, "hub_ip": netRow.HubIP, "iface": netRow.Iface,
			"keepalive": netRow.Keepalive, "mtu": netRow.MTU,
			"active_hub_server_id": netRow.ActiveHubServerID,
		}
		resp["hubs"] = hubViews
		resp["retired_hubs"] = retiredViews
		resp["peers"] = peerViews
		running, _ := a.DB.HasRunningWGTask()
		resp["running_task"] = running
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
	spokes := dedupSpokes(in.Spokes, hubID)
	alloc, issuesByServer, fatal := a.wgAllocate(netRow, spokes)
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
			_, issues, err := a.WG.ProbeServer(ctx, id, port, roles[id], reprovision[id], netRow.Iface, netRow.Subnet)
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
	if udpIss, _ := a.wgUDPProbeIssues(ctx, hubID, hubPort); len(udpIss) > 0 {
		hubIssues = append(hubIssues, udpIss...)
	}
	middleware.OK(c, gin.H{
		"network": gin.H{"subnet": netRow.Subnet, "hub_ip": netRow.HubIP, "iface": netRow.Iface,
			"keepalive": netRow.Keepalive, "mtu": netRow.MTU},
		"hub":     hubViewWithIssues(hubView, hubIssues),
		"spokes":  a.wgSpokesView(spokes, alloc, issuesByServer),
		"blocked": wg.HasErr(hubIssues),
	})
	a.audit(a.actorOf(c), "wg_plan", fmt.Sprintf("hub:%d", hubID), "预检探测", ipOf(c))
}

// wgUDPProbeIssues 面板侧 UDP 可达性预检，只产出 Warn 级提示（不阻断向导：探测受面板出口环境影响，
// 一律阻断会误伤；硬门禁放在切换/接管这类会动成员的动作里）。
// 手段：中心已有公钥且存在面板托管成员时发真握手（最准，通了不提示、不通才提示）；
// 否则退化为 ICMP 粗判（区分「路径通但没监听」与「无法确认放行」）。
// 第二个返回值是探测方式："handshake"（真握手，结论可靠）/ "reach"（粗判，仅提示）/ "none"（未探测）。
func (a *App) wgUDPProbeIssues(ctx context.Context, hubID int64, port int) ([]wg.Issue, string) {
	if port <= 0 {
		return nil, "none"
	}
	hubRow, _ := a.DB.GetWGHub(hubID)
	endpoint := ""
	if hubRow != nil {
		endpoint = wg.HubEndpointOnPort(hubRow.Endpoint, port)
	}
	if endpoint == "" {
		endpoint = a.WG.ResolveEndpoint(hubID, port)
	}
	if endpoint == "" {
		return nil, "none"
	}
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer cancel()
	// 已知公钥的中心：真握手
	if hubRow != nil && wg.ValidKey(hubRow.PublicKey) {
		out, err := a.WG.ProbeHubUDP(pctx, hubRow, 2500*time.Millisecond)
		if err == nil && !out.Skipped {
			if !out.Result.OK {
				return []wg.Issue{{Level: wg.Warn, Msg: out.FailMessage(port)}}, "handshake"
			}
			return nil, "handshake"
		}
	}
	// 首次组网 / 无托管私钥：粗判
	hint, err := wg.ProbeUDPReach(pctx, endpoint, 2*time.Second)
	if err != nil {
		return nil, "none"
	}
	expectRunning := hubRow != nil && strings.TrimSpace(hubRow.Endpoint) != ""
	if msg, need := wg.ReachHintMessage(port, endpoint, hint, expectRunning); need {
		return []wg.Issue{{Level: wg.Warn, Msg: msg}}, "reach"
	}
	return nil, "reach"
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

// dedupSpokes 去重并剔除 hub：hub 由「中心节点」步骤配置，若同时出现在 spokes 里，
// 任务载荷中它的 role 会被后写的 spoke 覆盖，导致 hub 侧配置被整段跳过。
func dedupSpokes(spokes []wgSpokeInput, hubID int64) []wgSpokeInput {
	seen := map[int64]bool{hubID: true}
	out := make([]wgSpokeInput, 0, len(spokes))
	for _, s := range spokes {
		if s.ServerID == 0 || seen[s.ServerID] {
			continue
		}
		seen[s.ServerID] = true
		out = append(out, s)
	}
	return out
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
			// 本机（宿主机）可以作为成员入网：容器本身没有 NET_ADMIN，配置经由宿主 SSH 施加。
			// 需要先在节点管理里录入宿主 SSH（容器可达地址），占位凭据（host=local）不算。
			cred, _ := a.DB.GetCredential(s.ServerID)
			if !sshReady(cred) {
				issues[s.ServerID] = append(issues[s.ServerID], wg.Issue{Level: wg.Err,
					Msg: "本机节点要先录宿主 SSH（节点管理 → 本机 → 录宿主 SSH，填容器可达地址）才能接入组网"})
				continue
			}
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
	if in.HubPort < 0 || in.HubPort > 65535 {
		middleware.Fail(c, 1001, "hub_port 越界（1-65535）")
		return
	}
	if !a.wgOpLock(c, "已有组网任务在执行，请等待完成") {
		return
	}
	defer a.wgOpUnlock()
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
		if netRow, err = a.DB.GetWGNetwork(); err != nil || netRow == nil {
			middleware.Fail(c, 5000, "网络读取失败")
			return
		}
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
	// 入参去重；hub 不可同时作为成员（避免给 hub 再分配 spoke IP）——
	// 必须在 wgAllocate 之前，否则 hub 会被写成 spoke，任务里 hub 一步都不执行
	names := map[int64]string{hubID: hubSrv.Name}
	spokes := dedupSpokes(in.Spokes, hubID)
	for _, s := range spokes {
		if srv, _ := a.DB.GetServer(s.ServerID); srv != nil {
			names[s.ServerID] = srv.Name
		}
	}
	alloc, issues, fatal := a.wgAllocate(netRow, spokes)
	if fatal != "" {
		middleware.Fail(c, 2012, fatal)
		return
	}
	// 有 error 级 issue 的成员拿不到分配（wgAllocate 全部 continue），不会出现在
	// alloc 里——必须直接遍历 issues，否则这些错误会被静默吞掉、成员无声跳过
	for _, list := range issues {
		for _, i := range list {
			if i.Level == wg.Err {
				middleware.Fail(c, 2010, "成员预检分配失败: "+i.Msg)
				return
			}
		}
	}
	// 确保成员记录（已存在则复用 IP/密钥并重置状态）
	allocByServer := map[int64]store.WGAlloc{}
	for _, al := range alloc {
		allocByServer[al.ServerID] = al
	}
	for _, s := range spokes {
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
		if _, err := a.DB.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: seq,
			ServerID: sql.NullInt64{Int64: al.ServerID, Valid: true},
			Title:    "接入 · " + names[al.ServerID], Status: "pending"}); err != nil {
			middleware.Fail(c, 5000, "步骤创建失败: "+err.Error())
			return
		}
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
	if !validMemberName(name) {
		middleware.Fail(c, 1001, "设备名称含非法字符（不得包含换行等控制字符，≤64 字符）")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil || netRow.ActiveHubServerID == 0 {
		middleware.Fail(c, 2010, "尚未初始化组网或缺少现役中心节点")
		return
	}
	if !a.wgOpLock(c, "组网任务执行中，稍后再试") {
		return
	}
	defer a.wgOpUnlock()
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
	// 立即热加到现役 hub + 各未退役备援（失败仅告警，设备状态留 pending，可稍后「同步到所有中心」）
	warn := ""
	added, failed := 0, 0
	hubs, _ := a.DB.ListWGHub()
	for _, hub := range hubs {
		if err := a.wgHubAddOne(hub, netRow, kp.Public, psk, ip, name); err != nil {
			failed++
			if hub.ServerID == netRow.ActiveHubServerID {
				warn = "hub 热加失败: " + err.Error()
			} else if warn == "" {
				warn = "备援热加失败: " + err.Error()
			}
		} else {
			added++
		}
	}
	if added == 0 && warn == "" {
		warn = "当前没有可用的中心节点，设备已入库但尚未下发到任何中心"
	}
	if failed > 0 && added > 0 {
		warn = fmt.Sprintf("已下发到 %d 台中心，%d 台失败：%s", added, failed, warn)
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
	peer := wg.Peer{
		Comment:      name,
		PublicKey:    pub,
		PresharedKey: psk,
		AllowedIPs:   []string{ip + "/32"}, // hub 侧主机路由，同 applyHubNode
	}
	return wg.HubAddPeer(ctx, conn, netRow.Iface, peer, ip+"/32")
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
	if hub == nil {
		middleware.Fail(c, 2010, "目标中心节点未就绪")
		return
	}
	conf, endpoint, err := a.wgRenderSpokeConf(netRow, peer, hub)
	if err != nil {
		middleware.Fail(c, 2010, err.Error())
		return
	}
	tag := wgConfTag(netRow, hubID)
	a.audit(a.actorOf(c), "wg_peer_conf_export", peer.Name,
		"hub="+strconv.FormatInt(hubID, 10), ipOf(c))
	// conf 含设备私钥与 PSK：禁止任何形式的缓存
	c.Header("Cache-Control", "no-store")
	middleware.OK(c, gin.H{
		"conf":     conf,
		"filename": wgConfFileName(peer.Name, tag),
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
	if !a.wgOpLock(c, "组网任务执行中，稍后再试") {
		return
	}
	defer a.wgOpUnlock()
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
	a.audit(a.actorOf(c), "wg_peer_verify", peer.Name, detail, ipOf(c))
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
	if !a.wgOpLock(c, "已有组网任务在执行，请等待完成") {
		return
	}
	defer a.wgOpUnlock()
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

type wgStandbyInput struct {
	ServerID int64 `json:"server_id"`
}

// WGRegisterStandby 把一台已配好 WG 的节点登记为备援 hub 槽位。
// 场景：网外还有一台手工配好的备源，面板此前不知道它（导入时没选备援）。
// 只读该节点的 conf/dump 取公钥、端口、端点后落库；不推送配置、不改对方任何文件。
func (a *App) WGRegisterStandby(c *gin.Context) {
	var in wgStandbyInput
	if err := c.ShouldBindJSON(&in); err != nil || in.ServerID == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 server_id")
		return
	}
	if !a.wgOpLock(c, "已有组网任务在执行，请等待完成") {
		return
	}
	defer a.wgOpUnlock()
	srv, _ := a.DB.GetServer(in.ServerID)
	if srv == nil || srv.IsSelf {
		middleware.Fail(c, 2010, "节点不存在或不支持（本机不参与组网）")
		return
	}
	if netRow, _ := a.DB.GetWGNetwork(); netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	hub, err := a.WG.RegisterStandby(ctx, in.ServerID)
	if err != nil {
		middleware.Fail(c, 2010, "登记备援失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_standby_register", srv.Name,
		fmt.Sprintf("endpoint=%s port=%d", hub.Endpoint, hub.ListenPort), ipOf(c))
	// 面板侧 UDP 实测：登记成功后顺手验证「这台备援真的能被握手到吗」
	iss, mode := a.wgUDPProbeIssues(ctx, hub.ServerID, hub.ListenPort)
	udp := gin.H{"checked": mode != "none", "mode": mode, "ok": len(iss) == 0, "hint": ""}
	if len(iss) > 0 {
		udp["hint"] = iss[0].Msg
	}
	middleware.OK(c, gin.H{"server_id": hub.ServerID, "endpoint": hub.Endpoint,
		"listen_port": hub.ListenPort, "status": hub.Status, "udp": udp})
}

// WGAdoptServer 纳管一台已手工配好 WG 的节点为受管成员（只读它的 conf，不改动它）。
// 本机节点（宿主机手工配过、面板还没管的那台）也走这条路：录了宿主 SSH 之后即可纳管。
func (a *App) WGAdoptServer(c *gin.Context) {
	var in wgStandbyInput
	if err := c.ShouldBindJSON(&in); err != nil || in.ServerID == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 server_id")
		return
	}
	if !a.wgOpLock(c, "已有组网任务在执行，请等待完成") {
		return
	}
	defer a.wgOpUnlock()
	srv, _ := a.DB.GetServer(in.ServerID)
	if srv == nil {
		middleware.Fail(c, 2010, "节点不存在")
		return
	}
	cred, _ := a.DB.GetCredential(in.ServerID)
	if !sshReady(cred) {
		middleware.Fail(c, 2010, "该节点无可用 SSH 凭据：请先在节点管理里录入"+map[bool]string{true: "宿主 SSH（容器可达地址）", false: " SSH 凭据"}[srv.IsSelf])
		return
	}
	if netRow, _ := a.DB.GetWGNetwork(); netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	detail, err := a.WG.AdoptServer(ctx, in.ServerID)
	if err != nil {
		middleware.Fail(c, 2010, "纳管失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_adopt", srv.Name, detail, ipOf(c))
	middleware.OK(c, gin.H{"server_id": in.ServerID, "detail": detail})
}

type wgTakeoverInput struct {
	TargetServerID int64 `json:"target_server_id"`
	Port           int   `json:"port"`
	CanaryServerID int64 `json:"canary_server_id"`
}

// WGTakeover 新机接管现役中心：目标机继承现役的密钥与端口，成员只改端点，
// 使用端凭证只差 Endpoint 一行（vs「切换」要换一整套 A/B 凭证）。
func (a *App) WGTakeover(c *gin.Context) {
	var in wgTakeoverInput
	if err := c.ShouldBindJSON(&in); err != nil || in.TargetServerID == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 target_server_id")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil || netRow.ActiveHubServerID == 0 {
		middleware.Fail(c, 2010, "尚未初始化组网或缺少现役中心节点")
		return
	}
	if in.TargetServerID == netRow.ActiveHubServerID {
		middleware.Fail(c, 2010, "目标机已是现役中心")
		return
	}
	target, _ := a.DB.GetServer(in.TargetServerID)
	if target == nil {
		middleware.Fail(c, 2002, "目标机不存在")
		return
	}
	cred, _ := a.DB.GetCredential(in.TargetServerID)
	if !sshReady(cred) {
		msg := "目标机无可用 SSH 凭据：请先在节点管理里录入"
		if target.IsSelf {
			msg += "宿主 SSH（容器可达地址）"
		}
		middleware.Fail(c, 2010, msg)
		return
	}
	// 目标机若已是本网成员（spoke），接管会把它变成中心并毁掉它的成员配置 → 先移出
	if peer, _ := a.DB.GetWGPeerByServer(in.TargetServerID); peer != nil && peer.Status != "left" {
		middleware.Fail(c, 2010, "目标机已是本网成员：请先在成员表把它移出，再执行接管")
		return
	}
	if !a.wgOpLock(c, "已有组网任务在执行，请等待完成") {
		return
	}
	defer a.wgOpUnlock()

	// 目标槽位：不存在则创建（新机器就是这种情况）；已存在的备援直接升级
	if hub, _ := a.DB.GetWGHub(in.TargetServerID); hub == nil {
		port := in.Port
		if port <= 0 {
			if cur, _ := a.DB.GetWGHub(netRow.ActiveHubServerID); cur != nil {
				port = cur.ListenPort
			}
		}
		if port <= 0 {
			port = 51820
		}
		if err := a.DB.UpsertWGHub(&store.WGHub{ServerID: in.TargetServerID, ListenPort: port,
			Status: "pending"}); err != nil {
			middleware.Fail(c, 5000, "目标槽位创建失败: "+err.Error())
			return
		}
	}

	// 金丝雀：优先指定；否则自动挑一台在线、且 SSH 不走 WG 网段的 SSH 成员
	canaryID := in.CanaryServerID
	canaryAuto := false
	if canaryID == 0 {
		if p := a.WG.PickCanary(); p != nil && p.ServerID.Valid && p.ServerID.Int64 != in.TargetServerID {
			srv, _ := a.DB.GetServer(p.ServerID.Int64)
			if srv == nil || !srv.IsSelf {
				canaryID = p.ServerID.Int64
				canaryAuto = canaryID != 0
			}
		}
	}
	oldID := netRow.ActiveHubServerID
	now := time.Now().Unix()
	payload, _ := json.Marshal(wg.TakeoverInput{TargetServerID: in.TargetServerID,
		OldServerID: oldID, Port: in.Port, CanaryServerID: canaryID, CanaryAuto: canaryAuto})
	taskID, err := a.DB.InsertWGTask(&store.WGTask{Kind: "takeover", Status: "running",
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
	addStep := func(seq int64, serverID int64, title string) bool {
		_, err := a.DB.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: seq,
			ServerID: sql.NullInt64{Int64: serverID, Valid: serverID > 0},
			Title:    title, Status: "pending"})
		if err != nil {
			middleware.Fail(c, 5000, "步骤创建失败: "+err.Error())
			return false
		}
		return true
	}
	if !addStep(0, in.TargetServerID, "准备目标机（迁移中心身份 + 安装）· "+nameOf(in.TargetServerID)) ||
		!addStep(1, in.TargetServerID, "面板侧 UDP 实测 · "+nameOf(in.TargetServerID)) {
		return
	}
	// 成员步骤：金丝雀最先；本机（面板宿主）排最后（翻转会重启它自己的接口）
	peers, _ := a.DB.ListWGPeers()
	seq := int64(2)
	for _, p := range peers {
		if p.Kind != "server" || !p.ServerID.Valid || p.Status == "left" {
			continue
		}
		if p.ServerID.Int64 == in.TargetServerID {
			continue
		}
		if p.ServerID.Int64 == canaryID {
			continue
		}
		srv, _ := a.DB.GetServer(p.ServerID.Int64)
		if srv != nil && srv.IsSelf {
			continue
		}
		if !addStep(seq, p.ServerID.Int64, "翻转 · "+p.Name) {
			return
		}
		seq++
	}
	if canaryID > 0 {
		if !addStep(seq, canaryID, "金丝雀 · "+nameOf(canaryID)) {
			return
		}
		seq++
	}
	// 本机最后（把上面跳过的本机成员补在这里）
	for _, p := range peers {
		if p.Kind != "server" || !p.ServerID.Valid || p.Status == "left" || p.ServerID.Int64 == in.TargetServerID {
			continue
		}
		srv, _ := a.DB.GetServer(p.ServerID.Int64)
		if srv == nil || !srv.IsSelf {
			continue
		}
		if !addStep(seq, p.ServerID.Int64, "翻转 · "+p.Name+"（本机最后）") {
			return
		}
		seq++
	}
	if !addStep(wg.TakeoverCommitSeq, in.TargetServerID, "提交并停用旧中心 · "+nameOf(oldID)) {
		return
	}
	go a.WG.RunTakeover(taskID)
	a.audit(a.actorOf(c), "wg_takeover", fmt.Sprintf("task:%d", taskID),
		fmt.Sprintf("target=%d old=%d port=%d", in.TargetServerID, oldID, in.Port), ipOf(c))
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
	if target.Status == "retired" {
		middleware.Fail(c, 2010, "该中心已退役（身份已迁走）：请重新组网后再切换")
		return
	}
	if !a.wgOpLock(c, "已有组网任务在执行，请等待完成") {
		return
	}
	defer a.wgOpUnlock()
	// 金丝雀：指定或自动挑选在线 SSH 成员
	canaryID := in.CanaryServerID
	if canaryID == 0 {
		if p := a.WG.PickCanary(); p != nil && p.ServerID.Valid && p.ServerID.Int64 != in.TargetServerID {
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
	a.audit(a.actorOf(c), "wg_patrol", "manual", "", ipOf(c))
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
	if !assetNameRe.MatchString(name) {
		middleware.Fail(c, 1001, "资产名仅允许字母数字与下划线/短横线（1-64 字符）")
		return
	}
	version := strings.TrimSpace(in.Version)
	arch := strings.TrimSpace(in.Arch)
	// version/arch 会拼进缓存文件路径：白名单防路径穿越
	if !assetVersionRe.MatchString(version) || !assetVersionRe.MatchString(arch) {
		middleware.Fail(c, 1001, "version/arch 仅允许字母数字与 . _ -（≤64 字符）")
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
		Name: name, Version: version, Arch: arch,
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
	a.audit(a.actorOf(c), "wg_asset_probe", as.Name, fmt.Sprintf("%d 源", len(results)), ipOf(c))
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
	st, err := os.Stat(dest)
	if err != nil {
		middleware.Fail(c, 5000, "缓存文件状态读取失败: "+err.Error())
		return
	}
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
	// GetWGPeerByServer 无记录时返回 (nil, nil)：必须判 nil，否则任意节点都能被推送
	peer, err := a.DB.GetWGPeerByServer(in.ServerID)
	if err != nil || peer == nil {
		middleware.Fail(c, 2002, "目标节点不是网内成员")
		return
	}
	if peer.Status == "left" {
		middleware.Fail(c, 2002, "目标节点已移出组网")
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
