package wg

// ============================================================
// 新机接管：把现役中心的「身份」搬到一台新机器上（doc/12 §8）
//
// 与「一键切换（A/B 轮换）」的区别：
//   切换 = 目标机已有自己的密钥，成员改指向它 → 使用端要换一套凭证（A/B）；
//   接管 = 目标机继承现役的密钥与端口，成员只改端点 → 使用端凭证只差 Endpoint 一行。
// 典型场景：WG-1 的云流量额度用完，换一台全新 WG-2 顶上。
//
// 步骤约定（handler 建步骤时保持一致）：
//   seq 0      目标机预检 + 安装 WG + 迁移身份（公钥/私钥/监听端口）
//   seq 1      面板侧 UDP 实测（不通即中止，成员零改动）
//   seq 2..n   成员翻转（**首个可翻转的成员就是金丝雀**，失败回滚并中止；
//              载荷显式指定 canary_server_id 时必须落在成员步骤里（会被挪到最前），
//              指定了却找不到就中止——不拿别的成员顶替用户的选择；
//              本机节点排最后，凭据走 WG 地址时跳过并留待人工收尾）
//   seq 1000   提交：置现役 → 停用并退役旧中心
// ============================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// TakeoverInput 新机接管任务载荷。
type TakeoverInput struct {
	TargetServerID int64 `json:"target_server_id"` // 接管方（新机器）
	OldServerID    int64 `json:"old_server_id"`    // 被接管的现役（快照，用于汇总/回滚）
	Port           int   `json:"port"`             // 目标监听端口；0 = 沿用现役端口
	CanaryServerID int64 `json:"canary_server_id"`
	CanaryAuto     bool  `json:"canary_auto"` // 金丝雀是面板自动挑的（挑不中可换人；用户点名的不能换）
}

// TakeoverCommitSeq 提交步骤的 seq（成员步骤 seq 递增，用一个大值占位排在最后）。
const TakeoverCommitSeq = 1000

// RunTakeover 执行接管任务（阻塞；调用方 go RunTakeover(taskID)）。
func (r *Runner) RunTakeover(taskID int64) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[wg] takeover task %d panic: %v", taskID, p)
			_ = r.DB.FinishWGTask(taskID, "failed", fmt.Sprintf("内部错误: %v", p), time.Now().Unix())
		}
	}()
	task, err := r.DB.GetWGTask(taskID)
	if err != nil || task == nil {
		return
	}
	var in TakeoverInput
	_ = json.Unmarshal([]byte(task.Payload), &in)
	network, err := r.DB.GetWGNetwork()
	if err != nil || network == nil {
		_ = r.DB.FinishWGTask(taskID, "failed", "组网配置缺失", time.Now().Unix())
		return
	}
	steps, err := r.DB.ListWGTaskSteps(taskID)
	if err != nil {
		_ = r.DB.FinishWGTask(taskID, "failed", "步骤读取失败: "+err.Error(), time.Now().Unix())
		return
	}
	prepSteps, memberSteps, commitStep, canaryFound := splitTakeoverSteps(steps, in.CanaryServerID)

	oldHub, _ := r.DB.GetWGHub(network.ActiveHubServerID)
	targetHub, _ := r.DB.GetWGHub(in.TargetServerID)
	if oldHub == nil || targetHub == nil {
		r.abortTakeover(taskID, "现役中心或目标槽位不存在")
		return
	}
	oldID := oldHub.ServerID
	oldPriv := append([]byte(nil), oldHub.PrivateKeyEnc...)
	oldPub, oldPort := oldHub.PublicKey, oldHub.ListenPort

	ctx := context.Background()
	nameOf := func(id int64) string {
		if s, _ := r.DB.GetServer(id); s != nil {
			return s.Name
		}
		return fmt.Sprint(id)
	}
	var notes []string // 跳过事项：塞进任务摘要，「少翻了一台」不能无声无息
	if !canaryFound {
		// 自动挑的金丝雀挑歪了（例如挑中接管目标）：退回「首台成员当金丝雀」，不拦流程；
		// 用户点名的金丝雀挑不了才是硬错误——不能拿别的成员顶替他的显式选择
		if in.CanaryAuto {
			notes = append(notes, fmt.Sprintf(
				"自动挑选的金丝雀（id=%d）不在可翻转的成员中，已改由首台成员充当金丝雀",
				in.CanaryServerID))
		} else {
			r.abortTakeover(taskID, fmt.Sprintf(
				"指定的金丝雀成员（id=%d）不在可翻转的成员步骤中（金丝雀需是 SSH 纳管的服务器成员）；"+
					"成员未做任何改动（现役仍是 %s）", in.CanaryServerID, nameOf(oldID)))
			return
		}
	}

	// ---------- 阶段 0/1：迁移身份 + 安装 + 实测 ----------
	prepFailed := ""
	for _, st := range prepSteps {
		_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
		switch st.Seq {
		case 0: // 迁移身份并安装
			port := in.Port
			if port <= 0 {
				port = oldPort
			}
			if port <= 0 {
				port = 51820
			}
			targetHub.PrivateKeyEnc = append([]byte(nil), oldPriv...)
			targetHub.PublicKey = oldPub
			targetHub.ListenPort = port
			targetHub.Status = "pending"
			targetHub.LastError = ""
			if err := r.DB.UpsertWGHub(targetHub); err != nil {
				prepFailed = "写入目标槽位失败: " + err.Error()
				_ = r.DB.FinishWGTaskStep(st.ID, "failed", prepFailed, time.Now().Unix())
				break
			}
			if err := r.applyHubNode(ctx, network, in.TargetServerID, string(RoleStandby), true); err != nil {
				prepFailed = err.Error()
				_ = r.DB.FinishWGTaskStep(st.ID, "failed", "失败: "+prepFailed, time.Now().Unix())
				break
			}
			_ = r.DB.FinishWGTaskStep(st.ID, "ok",
				fmt.Sprintf("已迁移中心身份（公钥 %s…，端口 %d）并拉起接口", fp8(oldPub), port),
				time.Now().Unix())
		case 1: // 面板侧实测
			hubNow, _ := r.DB.GetWGHub(in.TargetServerID)
			if hubNow == nil {
				prepFailed = "目标槽位丢失"
				_ = r.DB.FinishWGTaskStep(st.ID, "failed", prepFailed, time.Now().Unix())
				break
			}
			blocked, reason := r.ValidateHubUDP(ctx, hubNow, hubNow.ListenPort)
			if blocked {
				prepFailed = reason
				_ = r.DB.FinishWGTaskStep(st.ID, "failed", reason, time.Now().Unix())
				break
			}
			note := r.HubUDPNote(ctx, hubNow, hubNow.ListenPort, true)
			if note == "" {
				note = "面板侧未取得结论（无托管成员私钥），已按「无确定不通证据」继续"
			}
			_ = r.DB.FinishWGTaskStep(st.ID, "ok", note, time.Now().Unix())
		}
		if prepFailed != "" {
			break
		}
	}
	if prepFailed != "" {
		// 准备阶段失败：目标机保持现状（可重跑），成员零改动，现役不变
		r.abortTakeover(taskID,
			"目标机准备失败："+prepFailed+"；成员未做任何改动（现役仍是 "+nameOf(oldID)+"）")
		return
	}

	// ---------- 阶段 2：成员翻转（首个可翻转的成员就是金丝雀，失败即整体回滚） ----------
	flipped := []*store.WGPeer{}
	canaryDone := false // 金丝雀验证是否已通过
	failMsg := ""
	skipMsg := ""
	for _, st := range memberSteps {
		_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
		now := time.Now().Unix()
		next := func(status, msg string) { _ = r.DB.FinishWGTaskStep(st.ID, status, msg, now) }
		if failMsg != "" || skipMsg != "" {
			next("skipped", "前置步骤未通过，未执行")
			continue
		}
		var peer *store.WGPeer
		if st.ServerID.Valid {
			peer, _ = r.DB.GetWGPeerByServer(st.ServerID.Int64)
		}
		risk := ""
		if peer != nil {
			// 本机（面板宿主）必须走非 WG 地址，否则翻转会把自己掐断
			risk = r.selfFlipRisk(peer, network)
		}
		isCanary := in.CanaryServerID != 0 && st.ServerID.Valid && st.ServerID.Int64 == in.CanaryServerID
		flip, skipReason, failReason := memberGate(peer != nil, risk, isCanary)
		if failReason != "" {
			failMsg = failReason
			next("failed", "失败: "+failReason)
			continue
		}
		if !flip {
			if skipReason == "成员不存在" {
				notes = append(notes, "步骤「"+st.Title+"」对应的成员已不存在，未翻转")
			} else {
				notes = append(notes, skipReason)
				if canaryDone {
					// 非金丝雀位的成员翻不了（本机地址在 WG 网段）：停在这里，
					// 后面的成员也不再动，交人工收尾
					skipMsg = skipReason
				}
			}
			next("skipped", skipReason)
			continue
		}
		if err := r.flipSpoke(ctx, network, peer, in.TargetServerID); err != nil {
			// 失败的成员也可能已被写盘/重启（flipSpoke 先改配置后验握手），
			// 先把它单独翻回去，再走整体回滚——否则它会孤零零留在目标端点上
			rbNote := "（已回滚该成员）"
			if rbErr := r.rollbackSpoke(ctx, network, peer); rbErr != nil {
				log.Printf("[wg] takeover %d 回滚 %s 失败: %v", taskID, peer.Name, rbErr)
				rbNote = "（回滚失败：" + rbErr.Error() + "）"
			}
			if !canaryDone {
				failMsg = "金丝雀（" + peer.Name + "）翻转失败：" + err.Error()
			} else {
				failMsg = fmt.Sprintf("%s 翻转失败：%s", peer.Name, err.Error())
			}
			next("failed", "失败: "+err.Error()+rbNote)
			continue
		}
		flipped = append(flipped, peer)
		if !canaryDone {
			canaryDone = true
			next("ok", "金丝雀验证通过（"+peer.Name+" 端点已指向 "+nameOf(in.TargetServerID)+"）")
			continue
		}
		next("ok", "")
	}
	// 任一成员失败：把已翻的翻回去，整体当作没发生过（旧中心仍在跑）
	if failMsg != "" {
		rbN, rbFail := 0, 0
		for _, p := range flipped {
			if err := r.rollbackSpoke(ctx, network, p); err != nil {
				rbFail++
				log.Printf("[wg] takeover %d 回滚 %s 失败: %v", taskID, p.Name, err)
			} else {
				rbN++
			}
		}
		msg := fmt.Sprintf("接管中止：%s；已回滚 %d 台成员", failMsg, rbN)
		if rbFail > 0 {
			msg += fmt.Sprintf("（%d 台回滚失败，请用「切换为现役」手工翻回 %s）", rbFail, nameOf(oldID))
		}
		msg += "；现役保持 " + nameOf(oldID) + " 不变"
		r.abortTakeover(taskID, msg)
		return
	}

	// ---------- 阶段 3：提交 ----------
	if commitStep != nil {
		_ = r.DB.StartWGTaskStep(commitStep.ID, time.Now().Unix())
	}
	now := time.Now().Unix()
	if err := r.DB.SetActiveHub(in.TargetServerID, now); err != nil {
		r.abortTakeover(taskID, "置现役失败: "+err.Error())
		return
	}
	// 旧中心停用：关接口 + 取消开机自启 + 移走 conf（两台同身份同时在线会互相抢端点）
	oldMsg := "旧中心未停用（目标机即为原现役）"
	if oldID != in.TargetServerID {
		oldMsg = "旧中心 " + nameOf(oldID) + " 已停用（配置已备份为 .removed.*，接口已关闭、开机自启已取消）"
		if err := r.decommissionHub(ctx, oldID, network.Iface); err != nil {
			oldMsg = "旧中心停用失败（请手工 wg-quick down " + network.Iface + "）：" + err.Error()
			log.Printf("[wg] takeover %d 停用旧中心失败: %v", taskID, err)
		}
		h, _ := r.DB.GetWGHub(oldID)
		if h != nil {
			h.Status = "retired"
			h.LastError = ""
			h.PublicKey = "" // 身份已随目标机走，别让退役槽位再被当成中心渲染
			h.PrivateKeyEnc = nil
			h.Endpoint = ""
			_ = r.DB.UpsertWGHub(h)
		}
	}
	if commitStep != nil {
		_ = r.DB.FinishWGTaskStep(commitStep.ID, "ok", oldMsg, now)
	}
	// 使用端凭证：身份不变，只有 Endpoint 一行不同
	devN := 0
	peers, _ := r.DB.ListWGPeers()
	for _, p := range peers {
		if p.Kind == "device" && p.Status != "left" {
			devN++
		}
	}
	summary := fmt.Sprintf("接管完成：%s 已成为现役；%s", nameOf(in.TargetServerID), oldMsg)
	if devN > 0 {
		summary += fmt.Sprintf("；%d 台使用端请到中心面板重新下载凭证（仅 Endpoint 变更）", devN)
	}
	if len(flipped) > 0 {
		summary += fmt.Sprintf("；已翻转 %d 台 SSH 成员", len(flipped))
	} else if len(memberSteps) > 0 {
		summary += "；未翻转任何 SSH 成员"
	}
	if !canaryDone && len(memberSteps) > 0 {
		summary += "；金丝雀验证未执行（首台成员不可安全翻转），请按下列提示人工收尾"
	}
	if skipMsg != "" {
		summary += "；注意：" + skipMsg
	}
	for _, n := range notes {
		summary += "；跳过：" + n
	}
	_ = r.DB.FinishWGTask(taskID, "done", summary, time.Now().Unix())
	log.Printf("[wg] takeover task %d 完成: %s", taskID, summary)
}

// memberGate 决定一个成员步骤怎么走（纯函数，门控单独测）：
//
//	flip=true        正常翻转；金丝雀尚未通过时它同时充当金丝雀
//	skipReason!=""   跳过该成员（原因进步骤日志与任务摘要，不阻断其余成员）
//	failReason!=""   任务级失败：中止接管，已翻成员整体回滚
//
// 关键回归点：成员可用时必须 flip。早期实现写的是「canaryOK := len(memberSteps) == 0」，
// 于是只要有成员，首台就恒被判「无可用的金丝雀成员」跳过——只有本机一个 SSH 成员的
// 接管会「成功」，却一台都没翻，本机仍指着已退役的旧中心。
func memberGate(peerExists bool, selfRisk string, isCanary bool) (flip bool, skipReason, failReason string) {
	if isCanary && !peerExists {
		return false, "", "指定的金丝雀成员已不存在（可能刚被移除），成员未做任何改动"
	}
	if !peerExists {
		return false, "成员不存在", ""
	}
	if selfRisk != "" {
		if isCanary {
			// 用户点名的金丝雀翻不了：不许拿别的成员顶替，直接中止
			return false, "", "指定的金丝雀成员不能安全翻转：" + selfRisk
		}
		return false, selfRisk, ""
	}
	return true, "", ""
}

// splitTakeoverSteps 按 seq 把步骤分成三段，并把金丝雀挪到成员步骤最前。
//
//	seq 0/1           准备（迁移身份 + 实测）
//	seq 2..commit-1   成员翻转
//	seq >= commit     提交（唯一一段）
//
// canaryFound：载荷指定 canaryID 时它是否真的在成员步骤里。指定了却找不到
// （例如选中的是使用端而不是 SSH 纳管的服务器成员）必须由调用方中止——
// 否则会静默地把「首台成员」当金丝雀，跟用户的显式选择不是一回事。
func splitTakeoverSteps(steps []*store.WGTaskStep, canaryID int64) (prep, members []*store.WGTaskStep, commit *store.WGTaskStep, canaryFound bool) {
	for _, st := range steps {
		switch {
		case st.Seq <= 1:
			prep = append(prep, st)
		case st.Seq >= TakeoverCommitSeq:
			commit = st
		default:
			members = append(members, st)
		}
	}
	canaryFound = canaryID == 0
	if canaryID != 0 {
		for _, st := range members {
			if st.ServerID.Valid && st.ServerID.Int64 == canaryID {
				canaryFound = true
				break
			}
		}
	}
	if ci := canaryIndex(members, canaryID); ci > 0 {
		members[0], members[ci] = members[ci], members[0]
	}
	return prep, members, commit, canaryFound
}

// decommissionHub 停用一台中心：关接口 + 取消自启 + 移走 conf。
func (r *Runner) decommissionHub(ctx context.Context, serverID int64, iface string) error {
	strict := r.strictHostKey()
	dctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	conn, closer, err := r.dial(dctx, serverID, strict)
	if err != nil {
		return err
	}
	defer closer()
	return BringDown(dctx, conn, iface)
}

// selfFlipRisk 翻转风险：SSH 地址落在 WG 网段内时，改完配置一重启接口，
// 面板就再也连不上它（配置写下去了、验证回不来，回滚也回不去）。本机最容易踩：
// 面板宿主应填容器可达地址（172.17.0.1/内网 IP），不要填 10.66.66.x。
func (r *Runner) selfFlipRisk(peer *store.WGPeer, network *store.WGNetwork) string {
	if network == nil || peer == nil || !peer.ServerID.Valid {
		return ""
	}
	_, subnet, err := net.ParseCIDR(strings.TrimSpace(network.Subnet))
	if err != nil {
		return ""
	}
	srv, _ := r.DB.GetServer(peer.ServerID.Int64)
	isSelf := srv != nil && srv.IsSelf
	cred, _ := r.DB.GetCredential(peer.ServerID.Int64)
	if cred == nil {
		return ""
	}
	host := strings.TrimSpace(cred.Host)
	ip := net.ParseIP(host)
	if ip == nil || !subnet.Contains(ip) {
		return ""
	}
	who := "该成员"
	if isSelf {
		who = "本机（面板宿主）"
	}
	return who + "的 SSH 地址 " + host + " 属于 WG 网段（" + network.Subnet +
		"），翻转时重启接口会让面板与它失联：请改填容器可达地址（172.17.0.1/内网 IP），或人工收尾"
}

// abortTakeover 中止：把**库里仍是待执行**的步骤标记 skipped（已跑过的步骤保留各自
// 的状态与日志——中止不等于「什么都没发生」，目标机准备、金丝雀翻转都可能已经落过地），任务置失败。
func (r *Runner) abortTakeover(taskID int64, msg string) {
	now := time.Now().Unix()
	_ = r.DB.SkipPendingWGTaskSteps(taskID, "未执行（任务中止）", now)
	_ = r.DB.FinishWGTask(taskID, "failed", msg, now)
	log.Printf("[wg] takeover task %d 中止: %s", taskID, msg)
}

// fp8 公钥短指纹（日志用）。
func fp8(pub string) string {
	if len(pub) <= 8 {
		return pub
	}
	return pub[:8]
}
