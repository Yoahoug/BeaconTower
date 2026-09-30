package handler

// ============================================================
// 中心节点面板 + 凭证中心（doc/12 §8）
//
// 用户在组网页点中心卡（如「阿里云-My」）进入本面板：
//   概览（端点/端口/密钥指纹/月流量额度/实测 UDP）
//   使用端凭证（新增/重命名/下载/二维码/移除/同步到所有中心/打包下载）
//   成员（SSH 服务器，复用成员表的验证/移出）
// ============================================================

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/wg"
)

// wgRenderSpokeConf 渲染成员的 spoke 原生 conf（导出唯一出口，内含私钥/PSK，绝不缓存）。
func (a *App) wgRenderSpokeConf(netRow *store.WGNetwork, peer *store.WGPeer, hub *store.WGHub) (conf, endpoint string, err error) {
	if netRow == nil || peer == nil || hub == nil {
		return "", "", fmt.Errorf("缺少网络/成员/中心信息")
	}
	if !wg.ValidKey(hub.PublicKey) {
		return "", "", fmt.Errorf("目标中心节点未就绪（缺公钥）")
	}
	endpoint = hub.Endpoint
	if endpoint == "" {
		endpoint = a.WG.ResolveEndpoint(hub.ServerID, hub.ListenPort)
	}
	if endpoint == "" {
		return "", "", fmt.Errorf("中心节点端点未知（先完成一次配置/巡检）")
	}
	priv, err := a.WG.DecryptBlob(peer.PrivateKeyEnc)
	if err != nil {
		return "", "", fmt.Errorf("私钥解密失败")
	}
	psk, err := a.WG.DecryptBlob(peer.PskEnc)
	if err != nil {
		return "", "", fmt.Errorf("PSK 解密失败")
	}
	bits, err := wg.SubnetBits(netRow.Subnet)
	if err != nil {
		return "", "", err
	}
	conf, err = wg.SpokeConfFile(priv, peer.WgIP+"/"+strconv.Itoa(bits), hub.PublicKey,
		endpoint, netRow.Subnet, psk, netRow.Keepalive, netRow.MTU)
	if err != nil {
		return "", "", fmt.Errorf("配置渲染失败: %w", err)
	}
	return conf, endpoint, nil
}

// wgConfTag 中心在导出文件名里的 A/B 标记（A=现役，B=备援）。
func wgConfTag(netRow *store.WGNetwork, hubID int64) string {
	if netRow != nil && hubID == netRow.ActiveHubServerID {
		return "A"
	}
	return "B"
}

func wgConfFileName(peerName, tag string) string {
	return strings.ReplaceAll(peerName, " ", "_") + "-" + tag + ".conf"
}

// wgZipSafeName zip 条目名只能用 ASCII：中文名在 Windows/Info-ZIP 下会被解成乱码
// （zip 规范的 EFS 标记各家支持不一）。UTF-8 原名保留在 README 里对照。
func wgZipSafeName(peerID int64, peerName, tag string) string {
	var b strings.Builder
	for _, r := range peerName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		default:
			b.WriteByte('_')
		}
	}
	// 连着分隔符一起修剪：纯中文名「验证-临时」会被替换成 "_-_"，
	// 只 Trim "_" 会剩下 "-"，文件名变成没有名字的 "--A.conf"（多个中文名还会互相覆盖）
	name := strings.Trim(b.String(), "_.-")
	if name == "" {
		name = fmt.Sprintf("peer%d", peerID) // 全中文/全符号名：用成员 ID 兜底
	}
	return name + "-" + tag + ".conf"
}

// wgKeyFP 公钥指纹（SHA-256 前 12 位十六进制，便于口头/截图核对）。
func wgKeyFP(pub string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(pub)))
	return hex.EncodeToString(sum[:])[:12]
}

// wgHubPeersSplit 成员分组：使用端（设备，可导出凭证）与 SSH 成员（服务器）。
// 导入成员没有私钥但也必须出现在面板里（can_export=false）。
func wgHubPeersSplit(peers []*store.WGPeer) (creds, members []gin.H) {
	creds, members = []gin.H{}, []gin.H{}
	for _, p := range peers {
		if p.Status == "left" {
			continue
		}
		view := gin.H{
			"id": p.ID, "kind": p.Kind, "name": p.Name, "wg_ip": p.WgIP,
			"public_key": p.PublicKey, "status": p.Status, "last_error": p.LastError,
			"last_handshake": nullI64(p.LastHandshake),
			"rx_bytes":       p.RxBytes, "tx_bytes": p.TxBytes,
			"can_export": len(p.PrivateKeyEnc) > 0, "managed": p.Managed,
			"server_id": nullI64(p.ServerID),
		}
		if p.Kind == "server" || p.ServerID.Valid {
			members = append(members, view)
		} else {
			creds = append(creds, view)
		}
	}
	return creds, members
}

// WGHubDetail 中心节点面板数据：概览 + 使用端凭证 + 成员 + 双中心就绪度。
func (a *App) WGHubDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	hub, _ := a.DB.GetWGHub(id)
	if hub == nil {
		middleware.Fail(c, 2002, "该节点不是中心/备援槽位")
		return
	}
	name := ""
	if srv, _ := a.DB.GetServer(id); srv != nil {
		name = srv.Name
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	rx, tx, _ := a.DB.WGHubTrafficRange(id, monthStart.Unix(), now.Unix())
	peers, _ := a.DB.ListWGPeers()
	creds, members := wgHubPeersSplit(peers)

	// 双中心就绪度：现役 + 所有未退役备援是否都具备公钥/端点（决定「换中心」能否一键完成）
	hubs, _ := a.DB.ListWGHub()
	type hubReady struct {
		ID         int64  `json:"server_id"`
		Name       string `json:"name"`
		Active     bool   `json:"is_active"`
		Status     string `json:"status"`
		Ready      bool   `json:"ready"`
		Missing    string `json:"missing"`
		Endpoint   string `json:"endpoint"`
		ListenPort int    `json:"listen_port"`
	}
	readiness := []hubReady{}
	for _, h := range hubs {
		if h.Status == "retired" {
			continue
		}
		hn := ""
		if srv, _ := a.DB.GetServer(h.ServerID); srv != nil {
			hn = srv.Name
		}
		missing := ""
		switch {
		case !wg.ValidKey(h.PublicKey):
			missing = "缺少密钥（未完成过组网/导入）"
		case strings.TrimSpace(h.Endpoint) == "":
			missing = "端点未知（先完成一次配置或巡检）"
		case len(h.PrivateKeyEnc) == 0:
			missing = "面板未持有私钥（导入的备援无法在此重渲染）"
		}
		readiness = append(readiness, hubReady{
			ID: h.ServerID, Name: hn, Active: h.ServerID == netRow.ActiveHubServerID,
			Status: h.Status, Ready: missing == "", Missing: missing,
			Endpoint: h.Endpoint, ListenPort: h.ListenPort,
		})
	}

	hubView := gin.H{
		"server_id": hub.ServerID, "name": name, "listen_port": hub.ListenPort,
		"endpoint": hub.Endpoint, "status": hub.Status, "last_error": hub.LastError,
		"public_key": hub.PublicKey, "key_fp": wgKeyFP(hub.PublicKey),
		"quota_gb": nullF64(hub.QuotaGB), "checked_at": nullI64(hub.CheckedAt),
		"month_rx": rx, "month_tx": tx, "month_billed": tx,
		"is_active":  hub.ServerID == netRow.ActiveHubServerID,
		"has_keys":   len(hub.PrivateKeyEnc) > 0,
		"can_render": len(hub.PrivateKeyEnc) > 0 && wg.ValidKey(hub.PublicKey),
	}
	middleware.OK(c, gin.H{
		"hub": hubView, "creds": creds, "members": members,
		"hubs_readiness": readiness,
		"network": gin.H{"subnet": netRow.Subnet, "hub_ip": netRow.HubIP, "iface": netRow.Iface,
			"active_hub_server_id": netRow.ActiveHubServerID},
	})
}

// WGHubCleanup 删除退役中心槽位（只清记录：机器上的配置在退役时已被移走备份）。
func (a *App) WGHubCleanup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	hub, _ := a.DB.GetWGHub(id)
	if hub == nil {
		middleware.Fail(c, 2002, "该节点不是中心/备援槽位")
		return
	}
	if hub.Status != "retired" {
		middleware.Fail(c, 2010, "只有在退役状态的中心才能清理（在用的请先切换或接管）")
		return
	}
	if err := a.DB.DeleteWGHub(id); err != nil {
		middleware.Fail(c, 5000, "清理失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_hub_cleanup", fmt.Sprintf("hub:%d", id), "清理退役中心", ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// WGHubProbe 手动实测中心 UDP：真握手优先，无托管私钥时退化为 ICMP 粗判。
func (a *App) WGHubProbe(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	hub, _ := a.DB.GetWGHub(id)
	if hub == nil {
		middleware.Fail(c, 2002, "该节点不是中心/备援槽位")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	out, err := a.WG.ProbeHubUDP(ctx, hub, 2500*time.Millisecond)
	if err != nil {
		middleware.Fail(c, 2011, "探测出错: "+err.Error())
		return
	}
	resp := gin.H{"mode": "handshake", "ok": out.Result.OK, "skipped": out.Skipped,
		"reason": out.Reason, "peer": out.Peer, "target": out.Result.Target,
		"attempt": out.Result.Attempt, "rtt_ms": out.Result.RTT.Milliseconds(), "hint": ""}
	if out.Skipped {
		// 无托管私钥：ICMP 粗判（结论弱，只给提示）
		endpoint := wg.HubEndpointOnPort(hub.Endpoint, hub.ListenPort)
		if endpoint == "" {
			endpoint = a.WG.ResolveEndpoint(hub.ServerID, hub.ListenPort)
		}
		if endpoint == "" {
			middleware.Fail(c, 2010, "中心端点未知（先完成一次配置/巡检）")
			return
		}
		hint, rerr := wg.ProbeUDPReach(ctx, endpoint, 2*time.Second)
		if rerr != nil {
			middleware.Fail(c, 2011, "探测出错: "+rerr.Error())
			return
		}
		msg, need := wg.ReachHintMessage(hub.ListenPort, endpoint, hint, true)
		resp["mode"], resp["target"], resp["reach"] = "reach", endpoint, hint.String()
		resp["ok"], resp["hint"] = !need, msg
		a.audit(a.actorOf(c), "wg_hub_probe", fmt.Sprintf("hub:%d", id), "reach="+hint.String(), ipOf(c))
		middleware.OK(c, resp)
		return
	}
	if !out.Result.OK {
		resp["hint"] = out.FailMessage(hub.ListenPort)
	}
	a.audit(a.actorOf(c), "wg_hub_probe", fmt.Sprintf("hub:%d", id),
		fmt.Sprintf("ok=%v rtt=%s peer=%s", resp["ok"], out.Result.RTT, out.Peer), ipOf(c))
	middleware.OK(c, resp)
}

type wgPeerRenameInput struct {
	Name string `json:"name"`
}

// WGPeerRename 重命名使用端（只影响面板显示与导出文件名，WG 公钥/私钥不变）。
func (a *App) WGPeerRename(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	var in wgPeerRenameInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || !validMemberName(name) {
		middleware.Fail(c, 1001, "名称不合法（≤64 字符，不含换行等控制字符）")
		return
	}
	peer, _ := a.DB.GetWGPeer(id)
	if peer == nil {
		middleware.Fail(c, 2002, "成员不存在")
		return
	}
	old := peer.Name
	peer.Name = name
	if err := a.DB.UpdateWGPeer(peer); err != nil {
		middleware.Fail(c, 5000, "重命名失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_peer_rename", name, old+" → "+name, ipOf(c))
	middleware.OK(c, gin.H{"id": id, "name": name})
}

// WGPeerSyncHubs 把某个使用端一键配置到所有中心：
// 现役 + 未退役备援逐台热加（`wg set peer`），失败只记账不中断（返回逐台结果）。
func (a *App) WGPeerSyncHubs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	peer, _ := a.DB.GetWGPeer(id)
	if peer == nil || peer.Status == "left" {
		middleware.Fail(c, 2002, "成员不存在")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	if !wg.ValidKey(peer.PublicKey) || peer.WgIP == "" {
		middleware.Fail(c, 2010, "成员缺少公钥或 WG IP，无法同步")
		return
	}
	if !a.wgOpLock(c, "组网任务执行中，稍后再试") {
		return
	}
	defer a.wgOpUnlock()
	psk, err := a.WG.DecryptBlob(peer.PskEnc)
	if err != nil {
		middleware.Fail(c, 5000, "PSK 解密失败")
		return
	}
	hubs, _ := a.DB.ListWGHub()
	results := []gin.H{}
	// 退役槽位不再下发（它的身份已经迁到别的机器上）
	okN := 0
	total := 0
	for _, h := range hubs {
		if h.Status == "retired" {
			continue
		}
		total++
		item := gin.H{"server_id": h.ServerID, "active": h.ServerID == netRow.ActiveHubServerID, "ok": false, "error": ""}
		if srv, _ := a.DB.GetServer(h.ServerID); srv != nil {
			item["name"] = srv.Name
		}
		if err := a.wgHubAddOne(h, netRow, peer.PublicKey, psk, peer.WgIP, peer.Name); err != nil {
			item["error"] = err.Error()
		} else {
			item["ok"] = true
			okN++
		}
		results = append(results, item)
	}
	a.audit(a.actorOf(c), "wg_peer_sync_hubs", peer.Name,
		fmt.Sprintf("ok=%d/%d", okN, total), ipOf(c))
	middleware.OK(c, gin.H{"synced": okN, "total": total, "results": results})
}

// WGCredsZip 打包下载全部使用端原生 conf（每个中心一份：A=现役 / B=备援）。
func (a *App) WGCredsZip(c *gin.Context) {
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	hubs, _ := a.DB.ListWGHub()
	peers, _ := a.DB.ListWGPeers()
	type item struct {
		peer *store.WGPeer
		hub  *store.WGHub
	}
	var items []item
	for _, p := range peers {
		// 只打包「使用端」凭证：服务器成员（kind=server，如面板宿主）的配置由面板经
		// SSH 下发，混进这个 zip 会和面板「使用端凭证」清单对不上号
		if p.Status == "left" || p.Kind != "device" || len(p.PrivateKeyEnc) == 0 {
			continue
		}
		for _, h := range hubs {
			if h.Status == "retired" {
				continue // 退役槽位的身份已迁走，再出凭证会把人指到已停用的机器
			}
			if len(h.PrivateKeyEnc) == 0 && !wg.ValidKey(h.PublicKey) {
				continue // 连公钥都没有的槽位渲染不出东西
			}
			items = append(items, item{p, h})
		}
	}
	if len(items) == 0 {
		middleware.Fail(c, 2010, "没有可导出的使用端凭证（设备需由面板生成密钥；导入成员无私钥）")
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	used := map[string]int{}
	var readme strings.Builder
	readme.WriteString("BeaconTower WG 使用端凭证\n")
	readme.WriteString("文件名里的 A = 现役中心，B = 备援中心；导入 WireGuard 客户端后按需二选一。\n")
	readme.WriteString("切换中心后旧凭证失效，请回到面板「中心节点」面板重新下载。\n")
	readme.WriteString("（zip 里的文件名已转为 ASCII：中文替换为 _，纯中文名用 peer<ID>；原名见下一列）\n\n")
	failN := 0
	for _, it := range items {
		conf, endpoint, err := a.wgRenderSpokeConf(netRow, it.peer, it.hub)
		if err != nil {
			failN++
			readme.WriteString(fmt.Sprintf("! %s（%s）：%v\n", it.peer.Name, it.hub.Endpoint, err))
			continue
		}
		tag := wgConfTag(netRow, it.hub.ServerID)
		fn := wgZipSafeName(it.peer.ID, it.peer.Name, tag)
		if n := used[fn]; n > 0 {
			fn = strings.TrimSuffix(fn, ".conf") + fmt.Sprintf("(%d).conf", n+1)
		}
		used[wgZipSafeName(it.peer.ID, it.peer.Name, tag)]++
		w, err := zw.Create(fn)
		if err != nil {
			middleware.Fail(c, 5000, "打包失败: "+err.Error())
			return
		}
		_, _ = w.Write([]byte(conf))
		readme.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n", fn, it.peer.Name, it.peer.WgIP, endpoint))
	}
	w, err := zw.Create("README.txt")
	if err != nil {
		middleware.Fail(c, 5000, "打包失败: "+err.Error())
		return
	}
	_, _ = w.Write([]byte(readme.String()))
	if err := zw.Close(); err != nil {
		middleware.Fail(c, 5000, "打包失败: "+err.Error())
		return
	}
	a.audit(a.actorOf(c), "wg_creds_zip", "creds.zip", fmt.Sprintf("%d 个文件", len(items)), ipOf(c))
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="beacontower-wg-creds.zip"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
