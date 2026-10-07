package wg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// SwitchInput hub 切换任务载荷（A/B 流量额度轮换核心流程）。
type SwitchInput struct {
	TargetServerID int64 `json:"target_server_id"` // 目标 hub（切换后的现役）
	CanaryServerID int64 `json:"canary_server_id"` // 金丝雀 spoke（先切先验；0=自动选）
}

// RunSwitchHub 执行 hub 切换任务（阻塞；go RunSwitchHub）。
// ① 目标 hub 补齐全量成员（warm standby 校正）；
// ② 金丝雀 spoke 翻转端点并验证，失败自动回滚原配置；
// ③ 其余 SSH 成员并行翻转；设备成员仅提示换凭证；
// ④ 全部成功后置目标 hub 为现役（旧 hub 保留配置继续做备胎）。
func (r *Runner) RunSwitchHub(taskID int64) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[wg] switch task %d panic: %v", taskID, p)
			_ = r.DB.FinishWGTask(taskID, "failed", fmt.Sprintf("内部错误: %v", p), time.Now().Unix())
		}
	}()
	task, err := r.DB.GetWGTask(taskID)
	if err != nil || task == nil {
		return
	}
	stopHeartbeat := r.startHeartbeat(taskID)
	defer stopHeartbeat()
	var in SwitchInput
	_ = json.Unmarshal([]byte(task.Payload), &in)
	network, err := r.DB.GetWGNetwork()
	if err != nil || network == nil {
		_ = r.DB.FinishWGTask(taskID, "failed", "组网配置缺失", time.Now().Unix())
		return
	}
	steps, err := r.DB.ListWGTaskSteps(taskID)
	if err != nil {
		// 同 import：早退不收尾会把组网操作永久锁死（HasRunningWGTask 恒真）
		_ = r.DB.FinishWGTask(taskID, "failed", "步骤读取失败: "+err.Error(), time.Now().Unix())
		return
	}
	// 步骤分两段：seq==0 为目标 hub 校正；其余是 SSH 成员，金丝雀必须最先执行。
	// 金丝雀按载荷 canary_server_id 认定（旧实现按 seq==1 认定：成员表顺序与
	// PickCanary 的挑选顺序不一致时，真正先切的是另一个节点，失败门控也挂错人）。
	var hubSteps, memberSteps []*store.WGTaskStep
	for _, st := range steps {
		if st.Seq == 0 {
			hubSteps = append(hubSteps, st)
		} else {
			memberSteps = append(memberSteps, st)
		}
	}
	if ci := canaryIndex(memberSteps, in.CanaryServerID); ci > 0 {
		memberSteps[0], memberSteps[ci] = memberSteps[ci], memberSteps[0]
	}
	okN, failN, skipN := 0, 0, 0
	canaryOK := false
	rolledBack := false

	// 0) 面板侧 UDP 硬门禁（切成员之前）：拿到「确定不通」的证据就中止，成员零改动。
	// 旧流程只靠金丝雀试错——金丝雀挂掉后全网已有一台被改坏，这里把失败提前到改动之前。
	targetHub, _ := r.DB.GetWGHub(in.TargetServerID)
	tport := 0
	if targetHub != nil {
		tport = targetHub.ListenPort
	}
	if blocked, reason := r.ValidateHubUDP(context.Background(), targetHub, tport); blocked {
		now := time.Now().Unix()
		for _, st := range steps {
			_ = r.DB.StartWGTaskStep(st.ID, now)
			_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "UDP 预检未通过，未执行", now)
		}
		_ = r.DB.FinishWGTask(taskID, "failed",
			"目标中心 UDP 预检未通过："+reason+"；成员未做任何改动（现役保持不变）", now)
		log.Printf("[wg] switch task %d UDP 预检中止: %s", taskID, reason)
		return
	}

	// 1) 目标 hub 校正（备胎已在网内，重配降级预检）
	hubOK := true
	for _, st := range hubSteps {
		_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
		if err := r.applyHubNode(context.Background(), network, in.TargetServerID, string(RoleStandby), true); err != nil {
			hubOK = false
			failN++
			_ = r.DB.FinishWGTaskStep(st.ID, "failed", "失败: "+err.Error(), time.Now().Unix())
			log.Printf("[wg] switch task %d 备援校正 step %d 失败: %v", taskID, st.ID, err)
		} else {
			okN++
			_ = r.DB.FinishWGTaskStep(st.ID, "ok", "", time.Now().Unix())
		}
	}
	if len(memberSteps) == 0 {
		// 纯设备网（无 SSH 成员）：没有可翻转的 spoke，跳过金丝雀阶段直接置现役
		canaryOK = true
	}

	// 2) 金丝雀先行；3) 其余成员（金丝雀未通过一律跳过）
	for i, st := range memberSteps {
		_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
		now := time.Now().Unix()
		if i == 0 && !hubOK {
			skipN++
			_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "目标 hub 校正失败，已中止切换", now)
			continue
		}
		if i > 0 && !canaryOK {
			skipN++
			_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "金丝雀未通过，跳过", now)
			continue
		}
		var peer *store.WGPeer
		if st.ServerID.Valid {
			peer, _ = r.DB.GetWGPeerByServer(st.ServerID.Int64)
		}
		if peer == nil {
			skipN++
			_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "成员不存在", now)
			continue
		}
		err := r.flipSpoke(context.Background(), network, peer, in.TargetServerID)
		if err != nil {
			if i == 0 {
				// 金丝雀失败：自动回滚到原现役 hub
				if rbErr := r.rollbackSpoke(context.Background(), network, peer); rbErr != nil {
					log.Printf("[wg] switch task %d 金丝雀回滚失败: %v", taskID, rbErr)
				} else {
					rolledBack = true
				}
			}
			failN++
			_ = r.DB.FinishWGTaskStep(st.ID, "failed", "失败: "+err.Error(), time.Now().Unix())
			log.Printf("[wg] switch task %d step %d 失败: %v", taskID, st.ID, err)
			continue
		}
		logLine := ""
		if i == 0 {
			canaryOK = true
			logLine = "金丝雀验证通过"
		}
		okN++
		_ = r.DB.FinishWGTaskStep(st.ID, "ok", logLine, time.Now().Unix())
	}
	// 汇总：金丝雀通过且目标 hub 校正成功才算切换成功
	status := "failed"
	summary := fmt.Sprintf("成功 %d，失败 %d，跳过 %d", okN, failN, skipN)
	if canaryOK && hubOK {
		status = "done"
		if failN > 0 {
			status = "partial"
		}
		now := time.Now().Unix()
		_ = r.DB.SetActiveHub(in.TargetServerID, now)
		// 设备成员提示
		peers, _ := r.DB.ListWGPeers()
		devN := 0
		for _, p := range peers {
			if p.Kind == "device" {
				devN++
			}
		}
		if devN > 0 {
			summary += fmt.Sprintf("；%d 台设备请在组网页面切换凭证（重新扫码/导入）", devN)
		}
	} else if !hubOK {
		// 备胎没配好就切过去会把全网带崩：已中止，现役保持不变
		summary += "；目标 hub 校正失败，已中止切换（现役未变）"
	} else if rolledBack {
		summary += "；已回滚到原中心节点"
	}
	_ = r.DB.FinishWGTask(taskID, status, summary, time.Now().Unix())
	log.Printf("[wg] switch task %d 结束: %s (%s)", taskID, status, summary)
}

// canaryIndex 在成员步骤里定位金丝雀：优先按载荷指定，缺省取第一个成员。
func canaryIndex(steps []*store.WGTaskStep, canaryID int64) int {
	if len(steps) == 0 {
		return -1
	}
	if canaryID != 0 {
		for i, st := range steps {
			if st.ServerID.Valid && st.ServerID.Int64 == canaryID {
				return i
			}
		}
	}
	return 0
}

// PickCanary 自动选择金丝雀：当前在线的 server 成员（非任何 hub）。
func (r *Runner) PickCanary() *store.WGPeer {
	peers, _ := r.DB.ListWGPeers()
	network, _ := r.DB.GetWGNetwork()
	for _, p := range peers {
		if p.Kind != "server" || !p.ServerID.Valid {
			continue
		}
		if network != nil && p.ServerID.Int64 == network.ActiveHubServerID {
			continue
		}
		if p.Status == "online" {
			return p
		}
	}
	// 无在线记录则取任意非 hub server 成员
	for _, p := range peers {
		if p.Kind != "server" || !p.ServerID.Valid {
			continue
		}
		if network != nil && p.ServerID.Int64 == network.ActiveHubServerID {
			continue
		}
		return p
	}
	return nil
}

// flipSpoke 将成员的拨出目标翻转到指定 hub：读原 conf → 改 [Peer] 段
// （PublicKey/Endpoint/PSK/keepalive）→ 写回 → 重启接口 → 验证。
func (r *Runner) flipSpoke(ctx context.Context, network *store.WGNetwork, peer *store.WGPeer, targetHubID int64) error {
	target, _ := r.DB.GetWGHub(targetHubID)
	if target == nil || !ValidKey(target.PublicKey) {
		return errors.New("目标中心节点未就绪（缺少公钥）")
	}
	endpoint := target.Endpoint
	if endpoint == "" {
		endpoint = r.resolveEndpoint(target.ServerID, target.ListenPort)
		if endpoint == "" {
			return errors.New("目标中心节点端点未知")
		}
	}
	psk, _ := r.DecryptBlob(peer.PskEnc)
	bits, err := SubnetBits(network.Subnet)
	if err != nil {
		return err
	}
	strict := r.strictHostKey()
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	conn, closer, err := r.dial(ctx2, peer.ServerID.Int64, strict)
	if err != nil {
		return err
	}
	defer closer()
	// 优先就地改写原 conf 的 [Peer] 段（保留节点 PostUp/MTU/DNS 等个性化配置）；
	// 节点无 conf 且面板持有私钥时才全量重渲染。
	var conf string
	if orig, ferr := FetchConf(ctx2, conn, network.Iface); ferr == nil && strings.TrimSpace(orig) != "" {
		conf, err = r.rewritePeerSection(ctx2, conn, network, target.PublicKey, endpoint, psk)
	} else if peer.Managed && len(peer.PrivateKeyEnc) > 0 {
		priv, _ := r.DecryptBlob(peer.PrivateKeyEnc)
		conf, err = SpokeConfFile(priv, peer.WgIP+"/"+bits2(bits), target.PublicKey,
			endpoint, network.Subnet, psk, network.Keepalive, network.MTU)
	} else {
		return errors.New("节点无现有 conf 且面板未持有其私钥，无法切换")
	}
	if err != nil {
		return err
	}
	if err := BackupConf(ctx2, conn, network.Iface); err != nil {
		return err
	}
	if err := WriteConf(ctx2, conn, network.Iface, conf); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	hasSystemd := true
	if probe, perr := ProbeNode(ctx2, conn, 0, network.Iface); perr == nil {
		hasSystemd = probe.Systemd
	}
	if err := BringUp(ctx2, conn, network.Iface, hasSystemd); err != nil {
		return fmt.Errorf("拉起接口失败: %w", err)
	}
	online, detail, err := VerifySpoke(ctx2, conn, network.HubIP, network.Iface, target.PublicKey)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	peer.CheckedAt.Scan(now)
	if !online {
		peer.Status = "offline"
		peer.LastError = detail
		_ = r.DB.UpdateWGPeer(peer)
		return fmt.Errorf("切换后验证失败: %s", detail)
	}
	peer.Status = "online"
	peer.LastError = ""
	return r.DB.UpdateWGPeer(peer)
}

// rollbackSpoke 金丝雀回滚：恢复指向原现役 hub 的配置。
func (r *Runner) rollbackSpoke(ctx context.Context, network *store.WGNetwork, peer *store.WGPeer) error {
	if network.ActiveHubServerID == 0 {
		return errors.New("原现役 hub 未知，无法回滚")
	}
	return r.flipSpoke(ctx, network, peer, network.ActiveHubServerID)
}

// rewritePeerSection 就地改写 conf 的 [Peer] 段（导入成员：面板无私钥，
// 保留其 [Interface] 原文，仅替换拨出目标）。
func (r *Runner) rewritePeerSection(ctx context.Context, conn *sshx.Conn, network *store.WGNetwork, hubPub, endpoint, psk string) (string, error) {
	orig, err := FetchConf(ctx, conn, network.Iface)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(orig) == "" {
		return "", errors.New("节点无现有 conf 且面板未持有其私钥，无法切换")
	}
	ifc, err := ParseConf([]byte(orig))
	if err != nil {
		return "", fmt.Errorf("原 conf 解析失败: %w", err)
	}
	if len(ifc.Peers) == 0 {
		return "", errors.New("原 conf 无 [Peer] 段")
	}
	// 定位「当前指向现役 hub」的那一段再改写：节点 conf 里有多段 [Peer] 时
	// （该机还连着别的网络），写死 Peers[0] 会把另一段换成新 hub，成员随即失联
	curPub := ""
	if cur, _ := r.DB.GetWGHub(network.ActiveHubServerID); cur != nil {
		curPub = cur.PublicKey
	}
	idx := pickPeerIndex(ifc.Peers, curPub, network.HubIP)
	if idx < 0 {
		return "", fmt.Errorf("原 conf 有 %d 段 [Peer] 但无法定位指向现役 hub 的那一段，请手动核对", len(ifc.Peers))
	}
	ifc.Peers[idx].PublicKey = hubPub
	ifc.Peers[idx].Endpoint = endpoint
	ifc.Peers[idx].PersistentKeepalive = network.Keepalive
	if psk != "" {
		ifc.Peers[idx].PresharedKey = psk
	}
	// Address 缺失时以 DB 记录补齐
	if len(ifc.Address) == 0 {
		return "", errors.New("原 conf 缺少 Address")
	}
	return Render(ifc)
}

// pickPeerIndex 选出要改写的 [Peer]：先认当前现役 hub 的公钥，再退化为
// 「AllowedIPs 覆盖本网 hub IP」（现役未知/密钥被外部改动时仍可定位），
// 最后只剩单段 [Peer] 时直接取它；都无法确定返回 -1（宁可报错也不猜）。
func pickPeerIndex(peers []Peer, curHubPub, hubIP string) int {
	if curHubPub != "" {
		for i := range peers {
			if peers[i].PublicKey == curHubPub {
				return i
			}
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(hubIP)); ip != nil {
		for i := range peers {
			for _, a := range peers[i].AllowedIPs {
				if _, n, err := net.ParseCIDR(strings.TrimSpace(a)); err == nil && n.Contains(ip) {
					return i
				}
			}
		}
	}
	if len(peers) == 1 {
		return 0
	}
	return -1
}

func bits2(bits int) string { return strconv.Itoa(bits) }
