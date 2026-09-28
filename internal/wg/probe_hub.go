package wg

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// HubProbeOutcome 面板侧对某个 hub 端点的 UDP 可达性探测结果。
type HubProbeOutcome struct {
	Skipped bool   // 无法探测（无端点 / 无可用成员私钥）
	Reason  string // 跳过原因（Skipped=true 时有意义）
	Peer    string // 用作探针的成员名（透明起见写进日志）
	Result  ProbeUDPResult
}

// FailMessage 探测到「不通」时的可读说明。
// 两个可能原因都要说：端口没放行，或该中心还不认识探针成员（备援未同步过全量成员）。
func (o HubProbeOutcome) FailMessage(port int) string {
	p := port
	if p == 0 {
		p = endpointPort(o.Result.Target)
	}
	return fmt.Sprintf("UDP %d 从面板侧握手无回应：请确认云安全组/防火墙已放行 UDP %d 入方向；"+
		"若该中心是备援，先用「同步到所有中心」把成员补齐再测（目标 %s）", p, p, o.Result.Target)
}

// pickProbePeer 选一台面板托管成员当探针。
// 优先离线设备（探测会把 hub 眼里该成员的端点临时指向面板出口，离线的没人受影响），
// 其次离线服务器成员，最后取最近握手最旧的那台。私钥不可解密的（导入成员）跳过。
func (r *Runner) pickProbePeer() (*store.WGPeer, error) {
	peers, err := r.DB.ListWGPeers()
	if err != nil {
		return nil, err
	}
	rank := func(p *store.WGPeer) int {
		switch {
		case p.Kind == "device" && p.Status != "online":
			return 0
		case p.Kind == "server" && p.Status != "online":
			return 1
		default:
			return 2
		}
	}
	var best *store.WGPeer
	for _, p := range peers {
		if p.Status == "left" || len(p.PrivateKeyEnc) == 0 {
			continue
		}
		if best == nil || rank(p) < rank(best) ||
			(rank(p) == rank(best) && p.LastHandshake.Int64 < best.LastHandshake.Int64) {
			best = p
		}
	}
	return best, nil
}

// ProbeHubUDP 以面板托管的某个成员身份，向 hub 端点发一次真握手，判断 UDP 是否放行。
// 只读：不发任何配置改动；唯一的副作用是 hub 可能把探针成员的端点临时更新为本次出口地址，
// 该成员下一次握手（keepalive ≤25s）会自动纠正，因此优先挑离线成员。
func (r *Runner) ProbeHubUDP(ctx context.Context, hub *store.WGHub, perTry time.Duration) (HubProbeOutcome, error) {
	var out HubProbeOutcome
	if hub == nil {
		out.Skipped, out.Reason = true, "未指定中心节点"
		return out, nil
	}
	if strings.TrimSpace(hub.Endpoint) == "" {
		out.Skipped, out.Reason = true, "中心端点未知（先完成一次配置/巡检拿到公网端点）"
		return out, nil
	}
	return r.ProbeHubEndpoint(ctx, hub.Endpoint, hub.PublicKey, perTry)
}

// ProbeHubEndpoint 对「指定端点 + 指定中心公钥」发真握手。
// 与 ProbeHubUDP 的区别：端点/公钥由调用方给定，供预检「计划中的端口/待生成密钥」场景使用。
func (r *Runner) ProbeHubEndpoint(ctx context.Context, endpoint, hubPubB64 string, perTry time.Duration) (HubProbeOutcome, error) {
	var out HubProbeOutcome
	if strings.TrimSpace(endpoint) == "" {
		out.Skipped, out.Reason = true, "中心端点未知（先完成一次配置/巡检拿到公网端点）"
		return out, nil
	}
	if !ValidKey(hubPubB64) {
		out.Skipped, out.Reason = true, "中心公钥未知（尚未生成密钥，配置后再探测）"
		return out, nil
	}
	respPub, err := decodeKey(hubPubB64)
	if err != nil {
		return out, fmt.Errorf("中心公钥无效: %w", err)
	}
	peer, err := r.pickProbePeer()
	if err != nil {
		return out, err
	}
	if peer == nil {
		out.Skipped, out.Reason = true, udpProbeUnsupported
		return out, nil
	}
	privB64 := r.decrypt(peer.PrivateKeyEnc)
	if privB64 == "" {
		out.Skipped, out.Reason = true, "探针成员私钥无法解密（可能是导入成员）"
		return out, nil
	}
	priv, err := decodeKey(privB64)
	if err != nil {
		return out, fmt.Errorf("探针成员私钥无效: %w", err)
	}
	if perTry <= 0 {
		perTry = 3 * time.Second
	}
	out.Peer = peer.Name
	out.Result, err = ProbeUDP(ctx, endpoint, respPub, priv, perTry)
	return out, err
}

// HubEndpointOnPort 用 hub 行里的主机名/IP 拼出指定端口的端点（port<=0 时沿用行里的端口）。
// 预检时目标端口可能尚未落库（首次组网用计划端口），故不能直接用 hub.Endpoint。
func HubEndpointOnPort(endpoint string, port int) string {
	host := strings.TrimSpace(endpoint)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else {
		host = strings.TrimSuffix(host, ":")
	}
	if host == "" {
		return ""
	}
	if port <= 0 {
		if p := endpointPort(endpoint); p > 0 {
			port = p
		} else {
			return ""
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// ReachHintMessage 把粗判结论翻成给用户看的一句话（needsAttention=true 表示需要用户去放行 UDP）。
// expectRunning=true 表示该中心本应已在跑 WG（register/switch/巡检场景）：
// 此时「端口无监听」是真问题；首次安装（false）时它只是说明路径通、还没装而已。
func ReachHintMessage(port int, endpoint string, hint UDPReach, expectRunning bool) (msg string, needsAttention bool) {
	switch hint {
	case ReachClosed:
		if expectRunning {
			return fmt.Sprintf("UDP %d 路径可达但端口无监听（%s）：请确认该节点的 WireGuard 已启动、监听端口一致", port, endpoint), true
		}
		return fmt.Sprintf("UDP %d 路径可达（%s）：主机在线、端口暂无监听，安装后会自动实测握手", port, endpoint), false
	case ReachListener:
		return fmt.Sprintf("UDP %d 已有服务监听（%s）：若同时运行其它服务请确认端口不冲突", port, endpoint), false
	case ReachNoRoute:
		return udpHintFallback(port), true
	default:
		return udpHintFallback(port), true
	}
}

func udpHintFallback(port int) string {
	return fmt.Sprintf("UDP %d 入方向无法确认放行：如为云主机请在安全组放行 UDP %d 入方向（安装完成后面板会自动实测握手）", port, port)
}

// ValidateHubUDP 翻成员之前的硬门禁：只在拿到「确定不通」的证据时才阻断。
//   - 真握手失败 → 阻断（成员零改动）；
//   - 无托管私钥时退化为 ICMP 粗判：目标端口无监听 → 阻断；超时（安全组静默丢包与未监听不可分）→ 放行，交给金丝雀兜底；
//   - 探测手段完全不可用（端点未知）→ 放行并在原因里说明。
func (r *Runner) ValidateHubUDP(ctx context.Context, hub *store.WGHub, port int) (bool, string) {
	if hub == nil {
		return false, "目标中心槽位不存在"
	}
	endpoint := HubEndpointOnPort(hub.Endpoint, port)
	if endpoint == "" {
		endpoint = r.resolveEndpoint(hub.ServerID, port)
	}
	if endpoint == "" {
		return false, "中心端点未知（无法预检 UDP）"
	}
	if ValidKey(hub.PublicKey) {
		out, err := r.ProbeHubEndpoint(ctx, endpoint, hub.PublicKey, 2500*time.Millisecond)
		if err != nil {
			return false, "UDP 探测出错（已放行）: " + err.Error()
		}
		if !out.Skipped {
			if !out.Result.OK {
				return true, out.FailMessage(port)
			}
			return false, ""
		}
	}
	hint, err := ProbeUDPReach(ctx, endpoint, 2*time.Second)
	if err != nil {
		return false, "UDP 粗判出错（已放行）: " + err.Error()
	}
	if hint == ReachClosed {
		return true, fmt.Sprintf("UDP %d 无监听（%s，目标主机在但该端口没有服务）：切过去成员会全部失联，已中止", port, endpoint)
	}
	return false, ""
}

// HubUDPNote 中心配置完成后的实测结论（一行，写进任务步骤日志）。
// includeSkip=true 时把「无法实测」的原因也写出来（首次组网阶段还没有托管成员，只能如实说明）。
func (r *Runner) HubUDPNote(ctx context.Context, hub *store.WGHub, port int, includeSkip bool) string {
	if hub == nil {
		return ""
	}
	out, err := r.ProbeHubUDP(ctx, hub, 2*time.Second)
	if err != nil {
		return "UDP 实测出错（不影响配置）: " + err.Error()
	}
	if out.Skipped {
		if !includeSkip {
			return ""
		}
		return "UDP 未实测：" + out.Reason
	}
	if !out.Result.OK {
		return out.FailMessage(port)
	}
	return fmt.Sprintf("UDP %d 面板侧握手成功（%s，探针成员 %s）：端口已放行",
		port, out.Result.RTT.Round(time.Millisecond), out.Peer)
}
