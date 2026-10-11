package handler

// ============================================================
// 中心节点一键就绪 + 使用端凭证重新签发（doc/12 §18）
//
// 在此之前，「网外节点 → 备援中心」只有「准备为备援」一个入口，而它是**只读登记**：
// 前提是那台机器上已经有手工配好的 WG conf。新买的机器没有 conf，点下去只会拿到
// 「节点上无 /etc/wireguard/wg0.conf」——按钮名叫「准备」看着像一键，实际是半步。
//
// 这里补上真正的整条链路：面板生成密钥 → 渲染全量 conf（含所有服务器成员与设备）
// → SSH 安装/推送 → ufw 放行与转发 → 拉起接口 → 真握手实测 → 置就绪。
// 执行体就是组网向导用的那套引擎（Runner.RunApply → applyHubNode），
// 没有再写第二份实现：新增中心与首次组网走的是同一段代码。
//
// 唯一无法静默的是**云厂商安全组放行 UDP**：面板没有你的云账号凭据，开不了那条规则，
// 只能实测 UDP 是否可达并把结论写进步骤日志（失败提示里会点名去控制台放行）。
// ============================================================

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/wg"
)

type wgProvisionInput struct {
	ServerID   int64 `json:"server_id"`
	ListenPort int   `json:"listen_port"`
}

// WGHubProvision 把一台服务器配置为中心节点（备援 / 现役）：整链路面板自己跑完，
// 过程落 wg_task 步骤，前端 2s 轮询进度弹窗。
// 槽位已存在（含退役、异常）视为明确的重下发：预检对同名接口降级为警告，引擎备份后重写。
func (a *App) WGHubProvision(c *gin.Context) {
	var in wgProvisionInput
	if err := c.ShouldBindJSON(&in); err != nil || in.ServerID == 0 {
		middleware.Fail(c, 1001, "参数错误：需要 server_id")
		return
	}
	if in.ListenPort < 0 || in.ListenPort > 65535 {
		middleware.Fail(c, 1001, "监听端口越界（1-65535）")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网：先用「组网向导」或「导入现有网络」")
		return
	}
	srv, _ := a.DB.GetServer(in.ServerID)
	if srv == nil || srv.IsSelf {
		middleware.Fail(c, 2010, "节点不存在或不支持（本机不能作为中心节点）")
		return
	}
	cred, _ := a.DB.GetCredential(in.ServerID)
	if !sshReady(cred) {
		middleware.Fail(c, 2010, "该节点没有可用的 SSH 凭据：先到「节点管理」录入（面板代管密钥会自动装机）")
		return
	}
	// 中心与成员身份互斥：同一台机器不能既是 hub 又是 peer（conf 会自我指向）
	if p, _ := a.DB.GetWGPeerByServer(in.ServerID); p != nil && p.Status != "left" {
		middleware.Fail(c, 2010, "该节点当前是网内成员：先在成员表「移出」，再配置为中心节点")
		return
	}
	if !a.wgOpLock(c, "已有组网任务在执行，请等待完成") {
		return
	}
	defer a.wgOpUnlock()
	now := time.Now().Unix()
	hubRow, _ := a.DB.GetWGHub(in.ServerID)
	reprovision := hubRow != nil
	if hubRow == nil {
		kp, err := wg.GenerateKeyPair()
		if err != nil {
			middleware.Fail(c, 5000, "生成中心密钥失败")
			return
		}
		port := in.ListenPort
		if port <= 0 {
			port = 51820
		}
		hubRow = &store.WGHub{
			ServerID: in.ServerID, ListenPort: port,
			PublicKey: kp.Public, PrivateKeyEnc: a.WG.Encrypt(kp.Private),
			Endpoint: a.WG.ResolveEndpoint(in.ServerID, port),
			Status:   "pending",
		}
		if err := a.DB.UpsertWGHub(hubRow); err != nil {
			middleware.Fail(c, 5000, "中心槽位写入失败: "+err.Error())
			return
		}
	} else if in.ListenPort > 0 && in.ListenPort != hubRow.ListenPort {
		// 显式改端口：conf 与防火墙放行都按新端口走（旧规则的清理留给运维）
		hubRow.ListenPort = in.ListenPort
		hubRow.Status = "pending"
		if err := a.DB.UpsertWGHub(hubRow); err != nil {
			middleware.Fail(c, 5000, "中心槽位更新失败: "+err.Error())
			return
		}
	}
	// 还没有现役中心（导入中断等）：这台直接成为现役，与组网向导同语义
	if netRow.ActiveHubServerID == 0 {
		if err := a.DB.SetActiveHub(in.ServerID, now); err != nil {
			middleware.Fail(c, 5000, "设置现役中心失败: "+err.Error())
			return
		}
		netRow.ActiveHubServerID = in.ServerID
	}
	role := wgRoleText(netRow, in.ServerID)
	payload, _ := json.Marshal(store.WGTaskPayload{Allocations: []store.WGAlloc{{
		ServerID: in.ServerID, WgIP: netRow.HubIP, Role: role, Reprovision: reprovision,
	}}})
	taskID, err := a.DB.InsertWGTask(&store.WGTask{Kind: "provision", Status: "running",
		Payload: string(payload), CreatedAt: now})
	if err != nil {
		middleware.Fail(c, 5000, "任务创建失败: "+err.Error())
		return
	}
	title := "配置备援中心 · " + srv.Name
	if role == string(wg.RoleHub) {
		title = "配置中心节点 · " + srv.Name
	}
	if _, err := a.DB.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: 0,
		ServerID: sql.NullInt64{Int64: in.ServerID, Valid: true},
		Title:    title, Status: "pending"}); err != nil {
		middleware.Fail(c, 5000, "步骤创建失败: "+err.Error())
		return
	}
	go a.WG.RunApply(taskID)
	a.audit(a.actorOf(c), "wg_hub_provision", srv.Name,
		fmt.Sprintf("task:%d role=%s port=%d reprovision=%v", taskID, role, hubRow.ListenPort, reprovision), ipOf(c))
	middleware.OK(c, gin.H{"task_id": taskID, "role": role, "listen_port": hubRow.ListenPort})
}

// WGPeerReissue 重新签发使用端凭证。
// 导入成员（kind=device、面板无私钥）在这里生成面板托管密钥，并在所有未退役中心
// 整体替换：同 IP、同 PSK，改的是公钥——rebuild + syncconf 会把旧公钥一并撤掉，
// 随后即可用二维码在手机上重新导入。已是面板托管的成员走同一接口 = 「重新下发」（不换钥）。
func (a *App) WGPeerReissue(c *gin.Context) {
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
	// 服务器成员的 conf 由面板经 SSH 下发，换钥会把它踢下线直到重推，不开放这条路
	if peer.Kind != "device" {
		middleware.Fail(c, 2010, "只有使用端（设备）需要重新签发：服务器成员的配置由面板经 SSH 下发")
		return
	}
	netRow, _ := a.DB.GetWGNetwork()
	if netRow == nil {
		middleware.Fail(c, 2010, "尚未初始化组网")
		return
	}
	if !a.wgOpLock(c, "组网任务执行中，稍后再试") {
		return
	}
	defer a.wgOpUnlock()
	rotated := false
	if !peer.Managed || len(peer.PrivateKeyEnc) == 0 {
		kp, err := wg.GenerateKeyPair()
		if err != nil {
			middleware.Fail(c, 5000, "生成密钥失败")
			return
		}
		peer.PublicKey = kp.Public
		peer.PrivateKeyEnc = a.WG.Encrypt(kp.Private)
		peer.Managed = true
		peer.Status = "pending"
		peer.LastError = ""
		if err := a.DB.UpdateWGPeer(peer); err != nil {
			middleware.Fail(c, 5000, "写入成员失败: "+err.Error())
			return
		}
		rotated = true
	}
	// 逐台中心重建 conf（syncconf 撤旧公钥 + 上新公钥），失败只记账不中断
	hubs, _ := a.DB.ListWGHub()
	results := []gin.H{}
	okN, total := 0, 0
	for _, h := range hubs {
		if h.Status == "retired" {
			continue // 退役槽位的身份已迁走
		}
		total++
		item := gin.H{"server_id": h.ServerID, "active": h.ServerID == netRow.ActiveHubServerID,
			"ok": false, "error": ""}
		if srv, _ := a.DB.GetServer(h.ServerID); srv != nil {
			item["name"] = srv.Name
		}
		if err := a.wgHubRebuild(h, netRow); err != nil {
			item["error"] = err.Error()
		} else {
			item["ok"] = true
			okN++
		}
		results = append(results, item)
	}
	// 顺带把新凭证渲染出来（前端直接出二维码，不再多跑一次请求）
	conf, filename, endpoint, renderErr := "", "", "", ""
	if hub, _ := a.DB.GetWGHub(netRow.ActiveHubServerID); hub != nil {
		var rerr error
		if conf, endpoint, rerr = a.wgRenderSpokeConf(netRow, peer, hub); rerr != nil {
			renderErr = rerr.Error()
		} else {
			filename = wgConfFileName(peer.Name, wgConfTag(netRow, hub.ServerID))
		}
	}
	a.audit(a.actorOf(c), "wg_peer_reissue", peer.Name,
		fmt.Sprintf("rotated=%v ok=%d/%d err=%s", rotated, okN, total, renderErr), ipOf(c))
	middleware.OK(c, gin.H{
		"rotated": rotated, "synced": okN, "total": total, "results": results,
		"conf": conf, "filename": filename, "endpoint": endpoint,
		"wg_ip": peer.WgIP, "render_error": renderErr,
	})
}

// wgHubRebuild 按 DB 现状全量重写某中心的 conf 并 syncconf（同步执行，失败即返回）。
func (a *App) wgHubRebuild(hub *store.WGHub, netRow *store.WGNetwork) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	conn, closer, err := a.WG.DialHub(ctx, hub.ServerID)
	if err != nil {
		return err
	}
	defer closer()
	return a.WG.RebuildHubConf(ctx, netRow, hub, conn, 0)
}
