package wg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// ImportInput 导入任务载荷。
type ImportInput struct {
	HubServerID     int64   `json:"hub_server_id"`
	StandbyServerID int64   `json:"standby_server_id,omitempty"`
	Candidates      []int64 `json:"candidates"` // 参与匹配的 SSH 服务器（含 hub/standby 时自动跳过）
}

// RegisterStandby 把一台已配好 WG 的节点登记为备援 hub 槽位：
// 只读取它的 conf/dump 取公钥、端口与端点，不推送任何配置（真正「校正备援」在切换任务里做）。
// 用于「已经手工配好备源、只是面板不知道」的场景。
func (r *Runner) RegisterStandby(ctx context.Context, serverID int64) (*store.WGHub, error) {
	network, err := r.DB.GetWGNetwork()
	if err != nil {
		return nil, err
	}
	if network == nil {
		return nil, errors.New("尚未组网：请先用组网向导或导入现有网络")
	}
	if network.ActiveHubServerID == serverID {
		return nil, errors.New("该节点已是现役中心，无需登记为备援")
	}
	if err := r.importStandby(ctx, network, serverID); err != nil {
		return nil, err
	}
	return r.DB.GetWGHub(serverID)
}

// hubPeerState 从现役 hub 读到的 peer 摘要。
type hubPeerState struct {
	PublicKey   string
	WgIP        string
	PSK         string
	Comment     string
	Endpoint    string
	HandshakeA  int64
	RX, TX      uint64
}

// RunImport 执行导入任务（阻塞；go RunImport）。
// ① 读现役 hub 的 dump+conf → 建网/建 hub 槽位，未匹配 peer 先记为设备；
// ② 逐个 SSH 候选服务器读 conf，按公钥匹配升级为 server 成员（并收编其私钥）；
// ③ 备援 hub 读 conf 建槽位（warm standby）。
func (r *Runner) RunImport(taskID int64) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[wg] import task %d panic: %v", taskID, p)
			_ = r.DB.FinishWGTask(taskID, "failed", fmt.Sprintf("内部错误: %v", p), time.Now().Unix())
		}
	}()
	task, err := r.DB.GetWGTask(taskID)
	if err != nil || task == nil {
		return
	}
	var in ImportInput
	_ = json.Unmarshal([]byte(task.Payload), &in)
	network, err := r.DB.GetWGNetwork()
	if err != nil {
		// 早退也必须收尾：任务卡在 running 会让 HasRunningWGTask 永久为真，
		// 之后所有组网操作都被 1004 挡住，只能重启进程解套
		_ = r.DB.FinishWGTask(taskID, "failed", "组网配置读取失败: "+err.Error(), time.Now().Unix())
		return
	}
	if network == nil {
		network = &store.WGNetwork{Iface: "wg0", Keepalive: 25, MTU: 1420}
	}
	steps, err := r.DB.ListWGTaskSteps(taskID)
	if err != nil {
		_ = r.DB.FinishWGTask(taskID, "failed", "步骤读取失败: "+err.Error(), time.Now().Unix())
		return
	}
	// 步骤按 title 前缀分类：中心/备援/匹配
	okN, failN := 0, 0
	var peerStates map[string]*hubPeerState
	for _, st := range steps {
		_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
		var err error
		logLine := ""
		switch {
		case st.ServerID.Int64 == in.HubServerID && in.HubServerID > 0:
			peerStates, err = r.importHub(context.Background(), network, in.HubServerID)
			if peerStates != nil {
				logLine = fmt.Sprintf("发现 %d 个 peer", len(peerStates))
			}
			if n2, e := r.DB.GetWGNetwork(); e == nil && n2 != nil {
				network = n2 // hub 步骤可能建网，后续步骤用最新配置
			}
		case in.StandbyServerID > 0 && st.ServerID.Int64 == in.StandbyServerID:
			err = r.importStandby(context.Background(), network, in.StandbyServerID)
		default:
			logLine, err = r.importCandidate(context.Background(), network, st.ServerID.Int64, peerStates)
		}
		now := time.Now().Unix()
		if err != nil {
			failN++
			_ = r.DB.FinishWGTaskStep(st.ID, "failed", "失败: "+err.Error(), now)
			log.Printf("[wg] import task %d step %d 失败: %v", taskID, st.ID, err)
		} else {
			okN++
			_ = r.DB.FinishWGTaskStep(st.ID, "ok", logLine, now)
		}
	}
	status := "done"
	if failN > 0 && okN == 0 {
		status = "failed"
	} else if failN > 0 {
		status = "partial"
	}
	summary := fmt.Sprintf("成功 %d，失败 %d", okN, failN)
	_ = r.DB.FinishWGTask(taskID, status, summary, time.Now().Unix())
	log.Printf("[wg] import task %d 结束: %s (%s)", taskID, status, summary)
}

// importHub 读取现役 hub：建网（如缺）、hub 槽位（收编私钥）、peer 记录。
func (r *Runner) importHub(ctx context.Context, network *store.WGNetwork, hubServerID int64) (map[string]*hubPeerState, error) {
	strict := r.strictHostKey()
	conn, closer, err := r.dial(ctx, hubServerID, strict)
	if err != nil {
		return nil, err
	}
	defer closer()
	iface := network.Iface
	if iface == "" {
		iface = "wg0"
	}
	confText, err := FetchConf(ctx, conn, iface)
	if err != nil {
		return nil, fmt.Errorf("读取 conf 失败: %w", err)
	}
	if strings.TrimSpace(confText) == "" {
		return nil, fmt.Errorf("节点上无 /etc/wireguard/%s.conf", iface)
	}
	ifc, err := ParseConf([]byte(confText))
	if err != nil {
		return nil, fmt.Errorf("conf 解析失败: %w", err)
	}
	if len(ifc.Address) == 0 {
		return nil, errors.New("conf 缺少 Address，无法确定 hub 虚拟 IP 与子网")
	}
	hubIP, subnetCIDR, err := splitAddressCIDR(ifc.Address[0])
	if err != nil {
		return nil, err
	}
	devs, err := ShowDump(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("读取 wg dump 失败: %w", err)
	}
	var dumpDev *DevState
	for _, d := range devs {
		if d.Interface == iface {
			dumpDev = &d
		}
	}
	if dumpDev == nil {
		return nil, fmt.Errorf("接口 %s 未运行（wg show 无输出）", iface)
	}
	// 建网/校验
	now := time.Now().Unix()
	cur, _ := r.DB.GetWGNetwork()
	netExists := cur != nil
	if !netExists {
		network.Subnet = subnetCIDR
		network.HubIP = hubIP
		network.Iface = iface
		if ifc.MTU > 0 {
			network.MTU = ifc.MTU
		}
		network.CreatedAt, network.UpdatedAt = now, now
		if err := r.DB.EnsureWGNetwork(network); err != nil {
			return nil, err
		}
		network, err = r.DB.GetWGNetwork()
		if err != nil || network == nil {
			return nil, errors.New("网络读取失败")
		}
	} else if cur.Subnet != subnetCIDR {
		return nil, fmt.Errorf("节点子网 %s 与现有网络 %s 不一致", subnetCIDR, cur.Subnet)
	}
	// hub 槽位（收编私钥）
	priv := ifc.PrivateKey
	hubRow, _ := r.DB.GetWGHub(hubServerID)
	if hubRow == nil {
		hubRow = &store.WGHub{ServerID: hubServerID}
	}
	hubRow.ListenPort = dumpDev.ListenPort
	hubRow.PublicKey = dumpDev.PublicKey
	if ValidKey(priv) {
		hubRow.PrivateKeyEnc = r.encrypt(priv)
	}
	if hubRow.Endpoint == "" {
		hubRow.Endpoint = r.resolveEndpoint(hubServerID, dumpDev.ListenPort)
	}
	hubRow.Status = "ok"
	hubRow.CheckedAt.Scan(now)
	if err := r.DB.UpsertWGHub(hubRow); err != nil {
		return nil, err
	}
	if network.ActiveHubServerID == 0 {
		_ = r.DB.SetActiveHub(hubServerID, now)
		network.ActiveHubServerID = hubServerID
	}
	// peer 摘要（conf 提供 PSK/注释；dump 提供运行态）
	pskByPub := map[string]string{}
	commentByPub := map[string]string{}
	for _, p := range ifc.Peers {
		if p.PresharedKey != "" {
			pskByPub[p.PublicKey] = p.PresharedKey
		}
		if p.Comment != "" {
			commentByPub[p.PublicKey] = p.Comment
		}
	}
	states := map[string]*hubPeerState{}
	for _, dp := range dumpDev.Peers {
		st := &hubPeerState{
			PublicKey:  dp.PublicKey,
			Endpoint:   dp.Endpoint,
			HandshakeA: dp.LastHandshakeA,
			RX:         dp.RX,
			TX:         dp.TX,
			PSK:        pskByPub[dp.PublicKey],
			Comment:    commentByPub[dp.PublicKey],
		}
		if len(dp.AllowedIPs) > 0 {
			st.WgIP = firstIP(dp.AllowedIPs[0])
		}
		if st.WgIP == "" {
			continue
		}
		states[dp.PublicKey] = st
	}
	// 尚未被候选匹配的 peer 先落为设备（候选匹配步骤会升级为 server）
	r.ensureDevicePeers(states, now)
	return states, nil
}

// ensureDevicePeers 将 hub 上发现、且尚无 wg_peer 记录的 peer 落为设备成员。
func (r *Runner) ensureDevicePeers(states map[string]*hubPeerState, now int64) {
	existing, _ := r.DB.ListWGPeers()
	byPub := map[string]*store.WGPeer{}
	for _, p := range existing {
		byPub[p.PublicKey] = p
	}
	for pub, st := range states {
		if byPub[pub] != nil {
			continue
		}
		name := st.Comment
		if name == "" {
			name = "设备-" + pub[:6]
		}
		row := &store.WGPeer{
			Kind: "device", Name: name, WgIP: st.WgIP, PublicKey: pub,
			PskEnc:     r.encrypt(st.PSK),
			Managed:    false,
			Status:     "offline",
			CreatedAt:  now,
		}
		if st.HandshakeA > 0 && time.Since(time.Unix(st.HandshakeA, 0)) <= 3*time.Minute {
			row.Status = "online"
		}
		row.LastHandshake.Scan(st.HandshakeA)
		row.RxBytes, row.TxBytes = int64(st.RX), int64(st.TX)
		row.CheckedAt.Scan(now)
		if _, err := r.DB.InsertWGPeer(row); err != nil {
			log.Printf("[wg] import 设备 peer 写入失败 %s: %v", name, err)
		}
	}
}

// importCandidate 读取候选服务器 conf，按公钥匹配 hub peer：命中则升级为 server 成员。
func (r *Runner) importCandidate(ctx context.Context, network *store.WGNetwork, serverID int64, states map[string]*hubPeerState) (string, error) {
	if states == nil {
		return "", errors.New("中心节点未读取成功，无法匹配")
	}
	strict := r.strictHostKey()
	conn, closer, err := r.dial(ctx, serverID, strict)
	if err != nil {
		return "", err
	}
	defer closer()
	iface := network.Iface
	confText, err := FetchConf(ctx, conn, iface)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(confText) == "" {
		return "节点无 " + iface + " 配置，未加入网络", nil
	}
	ifc, err := ParseConf([]byte(confText))
	if err != nil {
		return "", fmt.Errorf("conf 解析失败: %w", err)
	}
	// 本机公钥
	pub, err := PublicKeyFromPrivate(ifc.PrivateKey)
	if err != nil {
		return "", errors.New("conf 私钥无法推导公钥")
	}
	st := states[pub]
	if st == nil {
		return "公钥未在中心节点 peer 列表中（可能属于其他网络）", nil
	}
	srv, _ := r.DB.GetServer(serverID)
	name := ""
	if srv != nil {
		name = srv.Name
	}
	now := time.Now().Unix()
	existing, _ := r.DB.ListWGPeers()
	var row *store.WGPeer
	for _, p := range existing {
		if p.PublicKey == pub {
			row = p
		}
	}
	if row == nil {
		row = &store.WGPeer{Kind: "device", Name: name, WgIP: st.WgIP, PublicKey: pub,
			Managed: false, CreatedAt: now}
		if _, err := r.DB.InsertWGPeer(row); err != nil {
			return "", err
		}
	}
	// 升级为 server 成员并收编密钥
	row.Kind = "server"
	row.ServerID.Scan(serverID)
	row.Name = name
	row.WgIP = st.WgIP
	row.PrivateKeyEnc = r.encrypt(ifc.PrivateKey)
	if row.PskEnc == nil || len(row.PskEnc) == 0 {
		row.PskEnc = r.encrypt(st.PSK)
	}
	row.Managed = true
	row.LastHandshake.Scan(st.HandshakeA)
	row.RxBytes, row.TxBytes = int64(st.RX), int64(st.TX)
	row.CheckedAt.Scan(now)
	if st.HandshakeA > 0 && time.Since(time.Unix(st.HandshakeA, 0)) <= 3*time.Minute {
		row.Status = "online"
		row.LastError = ""
	} else {
		row.Status = "offline"
	}
	if err := r.DB.UpdateWGPeer(row); err != nil {
		return "", err
	}
	return "已纳管 · WG IP " + st.WgIP, nil
}

// importStandby 读取备援 hub conf 建/更新槽位（warm standby）。
func (r *Runner) importStandby(ctx context.Context, network *store.WGNetwork, serverID int64) error {
	strict := r.strictHostKey()
	conn, closer, err := r.dial(ctx, serverID, strict)
	if err != nil {
		return err
	}
	defer closer()
	confText, err := FetchConf(ctx, conn, network.Iface)
	if err != nil {
		return err
	}
	if strings.TrimSpace(confText) == "" {
		return fmt.Errorf("节点上无 /etc/wireguard/%s.conf", network.Iface)
	}
	ifc, err := ParseConf([]byte(confText))
	if err != nil {
		return fmt.Errorf("conf 解析失败: %w", err)
	}
	devs, err := ShowDump(ctx, conn)
	if err != nil {
		return err
	}
	var dumpDev *DevState
	for _, d := range devs {
		if d.Interface == network.Iface {
			dumpDev = &d
		}
	}
	pub := ""
	port := 0
	if dumpDev != nil {
		pub, port = dumpDev.PublicKey, dumpDev.ListenPort
	} else {
		pub, err = PublicKeyFromPrivate(ifc.PrivateKey)
		if err != nil {
			return errors.New("私钥无法推导公钥")
		}
		port = ifc.ListenPort
	}
	// 与现役 hub 同钥会导致 peer 无法区分身份
	if network.ActiveHubServerID > 0 {
		if act, _ := r.DB.GetWGHub(network.ActiveHubServerID); act != nil && act.PublicKey == pub {
			return errors.New("备援节点与现役 hub 公钥相同，无法区分身份（请为备援生成独立密钥对）")
		}
	}
	now := time.Now().Unix()
	row, _ := r.DB.GetWGHub(serverID)
	if row == nil {
		row = &store.WGHub{ServerID: serverID}
	}
	row.ListenPort = port
	row.PublicKey = pub
	row.PrivateKeyEnc = r.encrypt(ifc.PrivateKey)
	if row.Endpoint == "" {
		row.Endpoint = r.resolveEndpoint(serverID, port)
	}
	row.Status = "ok"
	row.CheckedAt.Scan(now)
	return r.DB.UpsertWGHub(row)
}

// FetchConf 读取远端 wg-quick conf 文本（不存在返回空串）。
func FetchConf(ctx context.Context, conn *sshx.Conn, iface string) (string, error) {
	return conn.Run(ctx, `cat /etc/wireguard/`+sanitizeIface(iface)+`.conf 2>/dev/null || true`)
}

// splitAddressCIDR 从 Address（如 10.66.66.2/24）拆出裸 IP 与子网 CIDR。
func splitAddressCIDR(addrCIDR string) (string, string, error) {
	ip, ipnet, err := net.ParseCIDR(strings.TrimSpace(addrCIDR))
	if err != nil {
		return "", "", fmt.Errorf("Address 无效: %q", addrCIDR)
	}
	return ip.String(), ipnet.String(), nil
}

func firstIP(cidr string) string {
	if i := strings.Index(cidr, "/"); i > 0 {
		return cidr[:i]
	}
	return cidr
}
