package wg

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/sshx"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// Runner 组网任务执行器：消费 pending 状态的 wg_task，按「中心节点 → spokes 并行」
// 执行步骤并回写状态。实例由 handler 持有（持有主密钥解凭据）。
type Runner struct {
	DB     *store.DB
	Master []byte
}

func NewRunner(db *store.DB, master []byte) *Runner { return &Runner{DB: db, Master: master} }

// 步骤级超时
const (
	probeTimeout = 25 * time.Second
	spokeTimeout = 90 * time.Second
)

// ---------- 凭据与建连 ----------

func (r *Runner) credFor(serverID int64) (*sshx.Cred, string, error) {
	cred, err := r.DB.GetCredential(serverID)
	if err != nil {
		return nil, "", err
	}
	if cred == nil {
		return nil, "", errors.New("节点无 SSH 凭据，请先在节点管理中录入")
	}
	if cred.Host == "local" || cred.Port <= 0 {
		return nil, "", errors.New("本机节点暂不支持 SSH 组网管理（后续版本支持）")
	}
	c := &sshx.Cred{Host: cred.Host, Port: cred.Port, Username: cred.Username, AuthType: cred.AuthType}
	switch cred.AuthType {
	case "password":
		pw, err := crypto.DecryptString(r.Master, cred.PasswordEnc)
		if err != nil {
			return nil, "", errors.New("凭据解密失败")
		}
		c.Password = pw
	case "key":
		key, err := crypto.DecryptString(r.Master, cred.PrivateKeyEnc)
		if err != nil {
			return nil, "", errors.New("凭据解密失败")
		}
		c.PrivateKey = key
		if len(cred.PassphraseEnc) > 0 {
			if pp, err := crypto.DecryptString(r.Master, cred.PassphraseEnc); err == nil {
				c.Passphrase = pp
			}
		}
	default:
		return nil, "", fmt.Errorf("未知认证方式 %q", cred.AuthType)
	}
	return c, cred.HostKeyFP, nil
}

// dial 建连并按 TOFU/严格模式处理指纹（与采集链路同语义）。
func (r *Runner) dial(ctx context.Context, serverID int64, strict bool) (*sshx.Conn, func(), error) {
	cred, fp, err := r.credFor(serverID)
	if err != nil {
		return nil, nil, err
	}
	conn, err := sshx.Dial(ctx, cred, fp, strict)
	if err != nil {
		return nil, nil, err
	}
	if conn.HostKey() != "" && conn.HostKey() != fp && !strict {
		_ = r.DB.SetCredentialFP(serverID, conn.HostKey())
	}
	return conn, func() { _ = conn.Close() }, nil
}

func (r *Runner) strictHostKey() bool {
	s, err := r.DB.GetSettings()
	if err != nil {
		return false
	}
	return s["strict_host_key"] == "true"
}

// decrypt 私钥/PSK 解密（空 BLOB 返回空串）。
func (r *Runner) decrypt(blob []byte) string {
	if len(blob) == 0 {
		return ""
	}
	s, err := crypto.DecryptString(r.Master, blob)
	if err != nil {
		return ""
	}
	return s
}

// encrypt 面板代管密钥加密。
func (r *Runner) encrypt(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := crypto.EncryptString(r.Master, s)
	if err != nil {
		return nil
	}
	return b
}

// ---------- 任务执行 ----------

// RunApply 执行组网任务（阻塞；调用方须 go RunApply(taskID)）。
// 步骤顺序：中心节点（串行）→ spokes（并行）→ 汇总。
func (r *Runner) RunApply(taskID int64) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[wg] task %d panic: %v", taskID, p)
			_ = r.DB.FinishWGTask(taskID, "failed", fmt.Sprintf("内部错误: %v", p), time.Now().Unix())
		}
	}()

	task, err := r.DB.GetWGTask(taskID)
	if err != nil || task == nil {
		return
	}
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
	payload := store.DecodeWGPayload(task.Payload)
	roleByServer := map[int64]string{}
	for _, a := range payload.Allocations {
		roleByServer[a.ServerID] = a.Role
	}
	okN, failN, skipN := 0, 0, 0
	// 1) hub/standby 先行（串行；spoke 依赖其公钥与端点）
	for _, st := range steps {
		role := roleByServer[st.ServerID.Int64]
		if role != string(RoleHub) && role != string(RoleStandby) {
			continue
		}
		_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
		err := r.applyHubNode(context.Background(), network, st.ServerID.Int64, role)
		if err != nil {
			failN++
			_ = r.DB.FinishWGTaskStep(st.ID, "failed", "失败: "+err.Error(), time.Now().Unix())
			log.Printf("[wg] task %d hub step %d 失败: %v", taskID, st.ID, err)
		} else {
			okN++
			_ = r.DB.FinishWGTaskStep(st.ID, "ok", "", time.Now().Unix())
		}
	}
	// 2) spokes 并行
	var runWG sync.WaitGroup
	for _, st := range steps {
		role := roleByServer[st.ServerID.Int64]
		if role != string(RoleSpoke) {
			continue
		}
		runWG.Add(1)
		go func(st *store.WGTaskStep) {
			defer runWG.Done()
			_ = r.DB.StartWGTaskStep(st.ID, time.Now().Unix())
			skipped, err := r.applySpokeNode(context.Background(), network, st.ServerID.Int64)
			now := time.Now().Unix()
			switch {
			case err != nil && skipped:
				skipN++
				_ = r.DB.FinishWGTaskStep(st.ID, "skipped", "跳过: "+err.Error(), now)
			case err != nil:
				failN++
				_ = r.DB.FinishWGTaskStep(st.ID, "failed", "失败: "+err.Error(), now)
				log.Printf("[wg] task %d spoke step %d 失败: %v", taskID, st.ID, err)
			default:
				okN++
				_ = r.DB.FinishWGTaskStep(st.ID, "ok", "", now)
			}
		}(st)
	}
	runWG.Wait()

	// 3) 汇总
	status := "done"
	summary := fmt.Sprintf("成功 %d，失败 %d，跳过 %d", okN, failN, skipN)
	switch {
	case failN > 0 && okN == 0:
		status = "failed"
	case failN > 0:
		status = "partial"
	}
	_ = r.DB.FinishWGTask(taskID, status, summary, time.Now().Unix())
	log.Printf("[wg] task %d 结束: %s (%s)", taskID, status, summary)
}

// applyHubNode 配置 hub/standby 节点：预检 → 安装 → 转发/防火墙 →
// 全量重写 conf（含 DB 全部成员）→ 拉起 → 自检。
func (r *Runner) applyHubNode(ctx context.Context, network *store.WGNetwork, serverID int64, role string) error {
	hub, err := r.DB.GetWGHub(serverID)
	if err != nil || hub == nil {
		return errors.New("中心节点槽位不存在")
	}
	// 密钥自愈：槽位缺钥则补生成（正常由 apply 创建时生成）
	if !ValidKey(hub.PublicKey) {
		kp, err := GenerateKeyPair()
		if err != nil {
			return err
		}
		hub.PublicKey = kp.Public
		hub.PrivateKeyEnc = r.encrypt(kp.Private)
		if err := r.DB.UpsertWGHub(hub); err != nil {
			return err
		}
	}
	bits, err := subnetBits(network.Subnet)
	if err != nil {
		return err
	}
	strict := r.strictHostKey()
	dctx, cancel := context.WithTimeout(ctx, probeTimeout)
	conn, closer, err := r.dial(dctx, serverID, strict)
	if err != nil {
		cancel()
		return err
	}
	defer closer()
	defer cancel()

	probe, err := ProbeNode(dctx, conn, hub.ListenPort)
	if err != nil {
		return fmt.Errorf("预检失败: %w", err)
	}
	wgRole := RoleStandby
	if role == string(RoleHub) {
		wgRole = RoleHub
	}
	if issues := Judge(probe, wgRole, network.Iface, hub.ListenPort); HasErr(issues) {
		return fmt.Errorf("预检不通过: %s", joinIssues(issues))
	}
	if _, err := InstallWireGuard(dctx, conn, probe); err != nil {
		return err
	}
	if err := EnsureForward(dctx, conn); err != nil {
		return fmt.Errorf("开启转发失败: %w", err)
	}
	if err := EnsureUFWAllow(dctx, conn, hub.ListenPort); err != nil {
		return err // 非阻断：安全组需人工放行，conf 仍会写入
	}
	if err := BackupConf(dctx, conn, network.Iface); err != nil {
		return err
	}
	// 全量成员渲染（hub 侧无 Endpoint/keepalive；PSK 解密注入）
	peers, err := r.DB.ListWGPeers()
	if err != nil {
		return err
	}
	var blocks []Peer
	for _, p := range peers {
		if p.Status == "left" {
			continue
		}
		if !ValidKey(p.PublicKey) || p.WgIP == "" {
			continue
		}
		blocks = append(blocks, Peer{
			Comment:             p.Name,
			PublicKey:           p.PublicKey,
			PresharedKey:        r.decrypt(p.PskEnc),
			AllowedIPs:          []string{p.WgIP + "/" + strconv.Itoa(bits)},
			PersistentKeepalive: 0,
		})
	}
	priv := r.decrypt(hub.PrivateKeyEnc)
	conf, err := HubConfFile(priv, network.HubIP+"/"+strconv.Itoa(bits), hub.ListenPort, network.MTU, blocks)
	if err != nil {
		return err
	}
	if err := WriteConf(dctx, conn, network.Iface, conf); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := BringUp(dctx, conn, network.Iface, probe.Systemd); err != nil {
		return fmt.Errorf("拉起接口失败: %w", err)
	}
	devs, err := ShowDump(dctx, conn)
	if err != nil {
		return err
	}
	up := false
	for _, d := range devs {
		if d.Interface == network.Iface {
			up = true
		}
	}
	if !up {
		return errors.New("接口已写入但未处于运行状态")
	}
	// 端点缺失时自动解析（spoke 拨出目标）
	if hub.Endpoint == "" {
		if ep := r.resolveEndpoint(serverID, hub.ListenPort); ep != "" {
			hub.Endpoint = ep
		}
	}
	hub.Status = "ok"
	hub.LastError = ""
	now := time.Now().Unix()
	hub.CheckedAt.Scan(now)
	return r.DB.UpsertWGHub(hub)
}

// applySpokeNode 接入 spoke 节点：预检 → 安装 → 写 conf → 拉起 → ping hub 验证。
// 返回 skipped=true 表示无需执行（已在线）。
func (r *Runner) applySpokeNode(ctx context.Context, network *store.WGNetwork, serverID int64) (skipped bool, err error) {
	peer, err := r.DB.GetWGPeerByServer(serverID)
	if err != nil || peer == nil {
		return false, errors.New("成员记录不存在")
	}
	if peer.Status == "online" {
		return true, errors.New("节点已在线")
	}
	hub, err := r.DB.GetWGHub(network.ActiveHubServerID)
	if err != nil || hub == nil || !ValidKey(hub.PublicKey) {
		return false, errors.New("中心节点未就绪（缺少公钥），请先配置中心节点")
	}
	bits, err := subnetBits(network.Subnet)
	if err != nil {
		return false, err
	}
	strict := r.strictHostKey()
	dctx, cancel := context.WithTimeout(ctx, spokeTimeout)
	defer cancel()
	conn, closer, err := r.dial(dctx, serverID, strict)
	if err != nil {
		return false, err
	}
	defer closer()

	probe, err := ProbeNode(dctx, conn, 0)
	if err != nil {
		return false, fmt.Errorf("预检失败: %w", err)
	}
	if issues := Judge(probe, RoleSpoke, network.Iface, 0); HasErr(issues) {
		return false, fmt.Errorf("预检不通过: %s", joinIssues(issues))
	}
	if _, err := InstallWireGuard(dctx, conn, probe); err != nil {
		return false, err
	}
	if err := BackupConf(dctx, conn, network.Iface); err != nil {
		return false, err
	}
	priv := r.decrypt(peer.PrivateKeyEnc)
	psk := r.decrypt(peer.PskEnc)
	endpoint := hub.Endpoint
	if endpoint == "" {
		return false, errors.New("中心节点端点未知，请先配置中心节点")
	}
	conf, err := SpokeConfFile(priv, peer.WgIP+"/"+strconv.Itoa(bits), hub.PublicKey, endpoint,
		network.Subnet, psk, network.Keepalive, network.MTU)
	if err != nil {
		return false, err
	}
	if err := WriteConf(dctx, conn, network.Iface, conf); err != nil {
		return false, fmt.Errorf("写入配置失败: %w", err)
	}
	if err := BringUp(dctx, conn, network.Iface, probe.Systemd); err != nil {
		return false, fmt.Errorf("拉起接口失败: %w", err)
	}
	online, detail, err := VerifySpoke(dctx, conn, network.HubIP, network.Iface)
	if err != nil {
		return false, err
	}
	now := time.Now().Unix()
	peer.CheckedAt.Scan(now)
	if !online {
		peer.Status = "offline"
		peer.LastError = detail
		_ = r.DB.UpdateWGPeer(peer)
		return false, fmt.Errorf("入网验证失败: %s", detail)
	}
	peer.Status = "online"
	peer.LastError = ""
	// 回填 handshake
	if devs, err := ShowDump(dctx, conn); err == nil {
		for _, d := range devs {
			if d.Interface != network.Iface {
				continue
			}
			for _, dp := range d.Peers {
				if dp.LastHandshakeA > 0 {
					peer.LastHandshake.Scan(dp.LastHandshakeA)
					peer.RxBytes = int64(dp.RX)
					peer.TxBytes = int64(dp.TX)
				}
			}
		}
	}
	return false, r.DB.UpdateWGPeer(peer)
}

// resolveEndpoint 从画像/凭据推导 hub 对外端点 host:port。
func (r *Runner) resolveEndpoint(serverID int64, port int) string {
	host := ""
	if p, _ := r.DB.GetProfile(serverID); p != nil {
		host = strings.TrimSpace(p.PublicIP)
	}
	if host == "" {
		if cred, _ := r.DB.GetCredential(serverID); cred != nil {
			host = strings.TrimSpace(cred.Host)
		}
	}
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

// ProbeServer 对节点执行 WG 预检探测并给出判定（plan 接口 dry-run 用）。
func (r *Runner) ProbeServer(ctx context.Context, serverID int64, listenPort int, role Role) (*Probe, []Issue, error) {
	strict := r.strictHostKey()
	conn, closer, err := r.dial(ctx, serverID, strict)
	if err != nil {
		return nil, nil, err
	}
	defer closer()
	probe, err := ProbeNode(ctx, conn, listenPort)
	if err != nil {
		return nil, nil, err
	}
	issues := Judge(probe, role, "wg0", listenPort)
	return probe, issues, nil
}

// ResolveEndpoint 推导节点的对外 WG 端点（画像公网 IP → 凭据 host，附端口）。
func (r *Runner) ResolveEndpoint(serverID int64, port int) string {
	return r.resolveEndpoint(serverID, port)
}

// Encrypt 面板代管密钥加密入库。
func (r *Runner) Encrypt(plain string) []byte { return r.encrypt(plain) }

// DecryptBlob 解密面板代管密钥（空 BLOB 返回空串）。
func (r *Runner) DecryptBlob(blob []byte) (string, error) {
	if len(blob) == 0 {
		return "", nil
	}
	return crypto.DecryptString(r.Master, blob)
}

// DialHub 与指定 hub 节点建连（设备热加等同步操作用）。
func (r *Runner) DialHub(ctx context.Context, serverID int64) (*sshx.Conn, func(), error) {
	return r.dial(ctx, serverID, r.strictHostKey())
}

// VerifyPeer SSH 到成员节点查看 WG 运行态：接口存在且与 hub 握手新鲜判在线。
func (r *Runner) VerifyPeer(ctx context.Context, serverID int64, network *store.WGNetwork) (bool, string, error) {
	strict := r.strictHostKey()
	conn, closer, err := r.dial(ctx, serverID, strict)
	if err != nil {
		return false, "", err
	}
	defer closer()
	devs, err := ShowDump(ctx, conn)
	if err != nil {
		return false, "", err
	}
	up := false
	hsA := int64(0)
	for _, d := range devs {
		if d.Interface != network.Iface {
			continue
		}
		up = true
		for _, p := range d.Peers {
			if p.LastHandshakeA > hsA {
				hsA = p.LastHandshakeA
			}
		}
	}
	if !up {
		return false, "接口 " + network.Iface + " 未运行", nil
	}
	if hsA > 0 && time.Since(time.Unix(hsA, 0)) <= 3*time.Minute {
		return true, fmt.Sprintf("握手 %d 秒前", time.Now().Unix()-hsA), nil
	}
	return false, fmt.Sprintf("接口运行中，但最近握手 %s（hub 可能失联）",
		hsFuzzy(hsA)), nil
}

func hsFuzzy(hsA int64) string {
	if hsA <= 0 {
		return "从未"
	}
	return fmt.Sprintf("%d 秒前", time.Now().Unix()-hsA)
}

// RemovePeer 将成员移出网络：server 类节点尽力下线本侧（不可达不阻断），
// hub 侧移除运行态 peer 并全量重写 conf 后 syncconf（不断开其他隧道）。
// 调用方在成功后删除 wg_peer 行。
func (r *Runner) RemovePeer(peer *store.WGPeer) (detail string, err error) {
	network, e := r.DB.GetWGNetwork()
	if e != nil || network == nil {
		return "", errors.New("组网配置缺失")
	}
	strict := r.strictHostKey()
	// 1) 本侧下线（尽力而为）
	if peer.Kind == "server" && peer.ServerID.Valid {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		conn, closer, e := r.dial(ctx, peer.ServerID.Int64, strict)
		if e == nil {
			if e := BringDown(ctx, conn, network.Iface); e != nil {
				detail = "本侧下线失败: " + e.Error()
			} else {
				detail = "本侧已下线"
			}
			closer()
		} else {
			detail = "本侧不可达（可能已重装），跳过本侧清理"
		}
		cancel()
	}
	// 2) hub 侧移除
	hub, e := r.DB.GetWGHub(network.ActiveHubServerID)
	if e != nil || hub == nil {
		return detail, errors.New("中心节点槽位缺失，无法清理 hub 侧")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	conn, closer, e := r.dial(ctx, hub.ServerID, strict)
	if e != nil {
		return detail, fmt.Errorf("hub 不可达: %w", e)
	}
	defer closer()
	if e := HubRemovePeer(ctx, conn, network.Iface, peer.PublicKey); e != nil {
		return detail, fmt.Errorf("hub 移除 peer 失败: %w", e)
	}
	if e := r.rebuildHubConf(ctx, network, hub, conn, peer.ID); e != nil {
		return detail, fmt.Errorf("hub 配置重写失败: %w", e)
	}
	return detail, nil
}

// rebuildHubConf 按 DB 现状（排除 excludePeerID）重写 hub conf 并 syncconf。
func (r *Runner) rebuildHubConf(ctx context.Context, network *store.WGNetwork, hub *store.WGHub, conn *sshx.Conn, excludePeerID int64) error {
	bits, err := subnetBits(network.Subnet)
	if err != nil {
		return err
	}
	peers, err := r.DB.ListWGPeers()
	if err != nil {
		return err
	}
	var blocks []Peer
	for _, p := range peers {
		if p.ID == excludePeerID || p.Status == "left" || !ValidKey(p.PublicKey) || p.WgIP == "" {
			continue
		}
		blocks = append(blocks, Peer{
			Comment:      p.Name,
			PublicKey:    p.PublicKey,
			PresharedKey: r.decrypt(p.PskEnc),
			AllowedIPs:   []string{p.WgIP + "/" + strconv.Itoa(bits)},
		})
	}
	priv := r.decrypt(hub.PrivateKeyEnc)
	if !ValidKey(priv) {
		return errors.New("hub 私钥缺失，无法重写配置")
	}
	conf, err := HubConfFile(priv, network.HubIP+"/"+strconv.Itoa(bits), hub.ListenPort, network.MTU, blocks)
	if err != nil {
		return err
	}
	if err := BackupConf(ctx, conn, network.Iface); err != nil {
		return err
	}
	if err := WriteConf(ctx, conn, network.Iface, conf); err != nil {
		return err
	}
	return SyncConf(ctx, conn, network.Iface)
}

func joinIssues(issues []Issue) string {
	var msgs []string
	for _, i := range issues {
		if i.Level == Err {
			msgs = append(msgs, i.Msg)
		}
	}
	return strings.Join(msgs, "；")
}

func subnetBits(subnetCIDR string) (int, error) {
	_, ipnet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return 0, fmt.Errorf("子网格式无效: %q", subnetCIDR)
	}
	ones, _ := ipnet.Mask.Size()
	return ones, nil
}
