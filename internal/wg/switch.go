package wg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// SwitchInput hub 切换任务载荷（A/B 流量额度轮换核心流程）。
type SwitchInput struct {
	TargetServerID int64 `json:"target_server_id"`       // 目标 hub（切换后的现役）
	CanaryServerID int64 `json:"canary_server_id"`       // 金丝雀 spoke（先切先验；0=自动选）
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
	var in SwitchInput
	_ = json.Unmarshal([]byte(task.Payload), &in)
	network, err := r.DB.GetWGNetwork()
	if err != nil || network == nil {
		_ = r.DB.FinishWGTask(taskID, "failed", "组网配置缺失", time.Now().Unix())
		return
	}
	steps, err := r.DB.ListWGTaskSteps(taskID)
	if err != nil {
		return
	}
	// 步骤分类：title 前缀约定（handler 创建）：备援/金丝雀/切换
	okN, failN, skipN := 0, 0, 0
	canaryOK := false
	rolledBack := false
	for _, st := range steps {
		_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
		now := time.Now().Unix()
		var err error
		logLine := ""
		switch {
		case st.Seq == 0: // 目标 hub 校正
			err = r.applyHubNode(context.Background(), network, in.TargetServerID, string(RoleStandby))
		case st.Seq == 1: // 金丝雀
			var canary *store.WGPeer
			if st.ServerID.Valid {
				canary, _ = r.DB.GetWGPeerByServer(st.ServerID.Int64)
			}
			if canary == nil {
				skipN++
				_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "无可用金丝雀成员", now)
				continue
			}
			err = r.flipSpoke(context.Background(), network, canary, in.TargetServerID)
			if err != nil {
				// 自动回滚到原现役 hub
				if rbErr := r.rollbackSpoke(context.Background(), network, canary); rbErr != nil {
					log.Printf("[wg] switch task %d 金丝雀回滚失败: %v", taskID, rbErr)
				} else {
					rolledBack = true
				}
			} else {
				canaryOK = true
				logLine = "金丝雀验证通过"
			}
		default: // 其余 SSH 成员
			var peer *store.WGPeer
			if st.ServerID.Valid {
				peer, _ = r.DB.GetWGPeerByServer(st.ServerID.Int64)
			}
			if peer == nil {
				skipN++
				_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "成员不存在", now)
				continue
			}
			if !canaryOK {
				skipN++
				_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "金丝雀未通过，跳过", now)
				continue
			}
			err = r.flipSpoke(context.Background(), network, peer, in.TargetServerID)
		}
		now = time.Now().Unix()
		if err != nil {
			failN++
			_ = r.DB.FinishWGTaskStep(st.ID, "failed", "失败: "+err.Error(), now)
			log.Printf("[wg] switch task %d step %d 失败: %v", taskID, st.ID, err)
		} else {
			okN++
			_ = r.DB.FinishWGTaskStep(st.ID, "ok", logLine, now)
		}
	}
	// 汇总：金丝雀通过才算切换成功（至少目标 hub + 金丝雀成功）
	status := "failed"
	summary := fmt.Sprintf("成功 %d，失败 %d，跳过 %d", okN, failN, skipN)
	if canaryOK {
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
	} else if rolledBack {
		summary += "；已回滚到原中心节点"
	}
	_ = r.DB.FinishWGTask(taskID, status, summary, time.Now().Unix())
	log.Printf("[wg] switch task %d 结束: %s (%s)", taskID, status, summary)
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
//（PublicKey/Endpoint/PSK/keepalive）→ 写回 → 重启接口 → 验证。
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
	if probe, perr := ProbeNode(ctx2, conn, 0); perr == nil {
		hasSystemd = probe.Systemd
	}
	if err := BringUp(ctx2, conn, network.Iface, hasSystemd); err != nil {
		return fmt.Errorf("拉起接口失败: %w", err)
	}
	online, detail, err := VerifySpoke(ctx2, conn, network.HubIP, network.Iface)
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
	ifc.Peers[0].PublicKey = hubPub
	ifc.Peers[0].Endpoint = endpoint
	ifc.Peers[0].PersistentKeepalive = network.Keepalive
	if psk != "" {
		ifc.Peers[0].PresharedKey = psk
	}
	// Address 缺失时以 DB 记录补齐
	if len(ifc.Address) == 0 {
		return "", errors.New("原 conf 缺少 Address")
	}
	return Render(ifc)
}

func bits2(bits int) string { return strconv.Itoa(bits) }
