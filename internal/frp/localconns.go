// 本地连接计数（doc/13 §1 能力边界的补位）：Sakura OpenAPI v4 不提供任何
// 连接数字段（/user/info、/tunnels 均无，官方 spec 已核实），「实时连接数」
// 在 Sakura 侧永远无数据。本文件改为在 frpc 所在节点上直接数 socket：
//
//   - frp 对每条访客连接，frpc 都会向隧道的 local_ip:local_port 拨一条
//     TCP 连接，因此「远端端点 = 隧道本地端点 且 ESTABLISHED」的 socket
//     数就是活跃转发连接数（比平台侧的 yamux 虚拟流更真实）；
//   - frpc ↔ 平台服务器的控制通道不拨隧道本地端口，按端点过滤天然排除；
//   - 本地服务与 frpc 同机时一条连接出现两个 socket（frpc 侧 + 服务侧），
//     按「本地端口 ≠ 隧道本地端口」只保留 frpc 侧，恰好每连接计一次；
//   - 平台级归因只对 Sakura（natfrp）生效：它的平台 API 无值，本地计数
//     是唯一来源；ChmlFrp 有平台值（totalCurConns），不受本管线覆盖。
//
// 口径边界（UI 同步注明）：按隧道本地端点匹配，若同端点存在绕过隧道的
// 直连（如局域网直接访问该端口）会被一并计入；UDP 无连接语义不计；
// frpc 若跑在面板 SSH 不到的机器上则采不到。
package frp

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// connSrcPlatform / connSrcLocal 平台连接数的两种口径（frp_platform.conns_src）。
const (
	connSrcPlatform = "platform"
	connSrcLocal    = "local"
)

// localSock 从 /proc/net/tcp{,6} 解出的一条 ESTABLISHED 连接。
type localSock struct {
	rip   net.IP // 远端 IP（v4-mapped 已归一为 v4）
	rport int
	lport int
}

// ProbeDialer 按节点 ID 建立 SSH 连接（实现挂在 Deployer.dial 上，
// Runner 不直接持有凭据解密逻辑）。
type ProbeDialer func(ctx context.Context, serverID int64) (*sshx.Conn, func(), error)

// ---------- 纯函数：解析与计数（单测覆盖） ----------

// parseProcAddr 解析 /proc/net 地址列。格式为 4 字节组的十六进制，每组按
// 小端序打印（IPv4 一组、IPv6 四组，v4-mapped 形如 "0000000000000000FFFF00000100007F"）。
func parseProcAddr(s string) (net.IP, int, bool) {
	ip, _, ok := strings.Cut(s, ":")
	if !ok || len(ip) == 0 || len(ip)%8 != 0 || len(ip) > 32 {
		return nil, 0, false
	}
	raw := make([]byte, 0, len(ip)/2)
	for i := 0; i < len(ip); i += 8 {
		// 每组 8 个 hex 字符 = 4 字节，组内字节序反转（小端 → 网络序）
		for j := 6; j >= 0; j -= 2 {
			b, err := strconv.ParseUint(ip[i+j:i+j+2], 16, 8)
			if err != nil {
				return nil, 0, false
			}
			raw = append(raw, byte(b))
		}
	}
	parsed := net.IP(raw)
	if v4 := parsed.To4(); v4 != nil {
		parsed = v4
	}
	port, err := strconv.ParseUint(s[len(s)-4:], 16, 16)
	if err != nil || port == 0 {
		return nil, 0, false
	}
	return parsed, int(port), true
}

// parseProcSockets 从 tcp4/tcp6 两段原文提取 ESTABLISHED 连接（状态 01）。
func parseProcSockets(tcp4, tcp6 string) []localSock {
	var out []localSock
	for _, body := range []string{tcp4, tcp6} {
		for _, line := range strings.Split(body, "\n") {
			f := strings.Fields(line)
			// 行格式：sl local_address rem_address st ...（表头行字段不足直接跳过）
			if len(f) < 4 || f[3] != "01" {
				continue
			}
			rip, rport, ok := parseProcAddr(f[2])
			if !ok {
				continue
			}
			_, lport, ok := parseProcAddr(f[1])
			if !ok {
				continue
			}
			out = append(out, localSock{rip: rip, rport: rport, lport: lport})
		}
	}
	return out
}

// tunnelEndpoint 计数用的隧道本地端点（已归一）。wildcard 表示 local_ip
// 非具体 IP（空 / 0.0.0.0 / :: / 域名），此时只按端口匹配。
type tunnelEndpoint struct {
	port     int
	ip       net.IP
	wildcard bool
}

// newTunnelEndpoint 归一隧道的 local_ip/local_port；无本地端点的隧道
// （udp、端口非法）返回 false，不参与计数。
func newTunnelEndpoint(t *store.FRPTunnel) (tunnelEndpoint, bool) {
	if t == nil || t.LocalPort <= 0 || t.LocalPort > 65535 || strings.EqualFold(t.Proto, "udp") {
		return tunnelEndpoint{}, false
	}
	e := tunnelEndpoint{port: t.LocalPort}
	raw := strings.TrimSpace(t.LocalIP)
	ip := net.ParseIP(raw)
	if ip == nil {
		// 「localhost」是回环的别名，按 127.0.0.1 匹配（frpc 拨的就是它），
		// 不能落进 wildcard——否则面板 SSH 到远程节点（rport=22 的任何
		// established socket）都会被误计成该隧道的访客连接。
		if strings.EqualFold(raw, "localhost") {
			e.ip = net.IPv4(127, 0, 0, 1)
			return e, true
		}
		e.wildcard = true // 空/其他域名等，退化为只按端口
		return e, true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsUnspecified() {
		e.wildcard = true
		return e, true
	}
	e.ip = ip
	return e, true
}

// match 判断一条 socket 是否命中该端点。「本地端口 ≠ 隧道端口」排除同机
// 服务侧 socket（服务 accept 侧的本地端口就是隧道端口）。
func (e tunnelEndpoint) match(s localSock) bool {
	if s.rport != e.port || s.lport == e.port {
		return false
	}
	if e.wildcard {
		return true
	}
	return s.rip != nil && s.rip.Equal(e.ip)
}

// countEndpoints 对一组 socket 按端点集合计数，返回每个端点的命中数
// （key=「ip|port」，与 endpointKey 对应；多个隧道共享同一端点时共用一个 key）。
func countEndpoints(socks []localSock, eps map[string]tunnelEndpoint) map[string]int {
	out := make(map[string]int, len(eps))
	for _, s := range socks {
		for key, e := range eps {
			if e.match(s) {
				out[key]++
			}
		}
	}
	return out
}

func endpointKey(e tunnelEndpoint) string {
	if e.wildcard {
		return "|ep|" + strconv.Itoa(e.port)
	}
	return e.ip.String() + "|ep|" + strconv.Itoa(e.port)
}

// ---------- 采集编排 ----------

// localConnsScript 远端节点一次 exec：带标记输出两个文件原文，面板侧解析。
const localConnsScript = `echo __BT_TCP4__
cat /proc/net/tcp 2>/dev/null
echo __BT_TCP6__
cat /proc/net/tcp6 2>/dev/null
`

// splitProcDump 切分 localConnsScript 的输出（标记缺失时对应段为空）。
func splitProcDump(raw string) (tcp4, tcp6 string) {
	const (
		m4 = "__BT_TCP4__"
		m6 = "__BT_TCP6__"
	)
	i4 := strings.Index(raw, m4)
	if i4 < 0 {
		return "", ""
	}
	rest := raw[i4+len(m4):]
	i6 := strings.Index(rest, m6)
	if i6 < 0 {
		return rest, ""
	}
	return rest[:i6], rest[i6+len(m6):]
}

// readLocalProcs 本机节点（is_self）直读宿主 /proc。容器部署时 /proc/net 是
// 面板容器自己的 netns（frpc 是 host 网络，socket 在宿主 netns），须经宿主
// PID 1 读取（与采集器 selfNetDir 同一口径）；原生部署直接 /proc/net 即宿主。
func readLocalProcs() (string, string, error) {
	base := "/proc/net"
	if _, err := os.Stat("/host/proc/1/net/tcp"); err == nil {
		base = "/host/proc/1/net"
	}
	b4, err4 := os.ReadFile(base + "/tcp")
	b6, err6 := os.ReadFile(base + "/tcp6")
	if err4 != nil && err6 != nil {
		return "", "", err4
	}
	return string(b4), string(b6), nil
}

// CollectLocalConns 全局采集一轮：对每条有本地端点的隧道，在「本机 + 有托管
// 记录的节点」上数 socket 并写回 local_conns；Sakura 平台级 conns 用其隧道
// 端点计数之和（去重端点）+ conns_src=local。任何失败只记日志，不影响主同步。
//
// 探测范围 deliberately 收敛：is_self（直读）∪ frp_deploy 引用的节点（SSH）。
// frpc 跑在其他裸节点上时采不到（该隧道 local_conns 保持 0），这是文档化的边界。
func (r *Runner) CollectLocalConns(ctx context.Context) {
	platforms, err := r.DB.ListFRPPlatforms()
	if err != nil {
		return
	}
	tunnels, err := r.DB.ListFRPTunnels()
	if err != nil {
		return
	}
	// 隧道 → 端点；端点集合去重（跨平台共享端点只数一次，各隧道分别命中）。
	// Cloudflare 的 socket 口径不适用（连接终结在 CF 边缘，cloudflared 与
	// 源站之间另有连接池），其隧道不进端点集合，避免误计数。
	eps := map[string]tunnelEndpoint{}
	byEndpoint := map[string][]*store.FRPTunnel{} // 端点 key → 命中该端点的隧道
	kindByPlatform := map[int64]string{}
	for _, p := range platforms {
		kindByPlatform[p.ID] = p.Kind
	}
	for _, t := range tunnels {
		if kindByPlatform[t.PlatformID] == KindCloudflared {
			continue
		}
		e, ok := newTunnelEndpoint(t)
		if !ok {
			continue
		}
		key := endpointKey(e)
		if _, seen := eps[key]; !seen {
			eps[key] = e
		}
		byEndpoint[key] = append(byEndpoint[key], t)
	}
	if len(eps) == 0 {
		return
	}

	// 组装探测点：本机 + 托管节点
	type probe struct {
		serverID int64 // 0 = 本机直读
		self     bool
	}
	var probes []probe
	selfSrv, err := r.DB.GetSelfServer()
	if err == nil && selfSrv != nil {
		probes = append(probes, probe{serverID: selfSrv.ID, self: true})
	}
	if deps, err := r.DB.ListFRPDeploys(); err == nil {
		seen := map[int64]bool{}
		for _, d := range deps {
			if seen[d.ServerID] {
				continue
			}
			seen[d.ServerID] = true
			probes = append(probes, probe{serverID: d.ServerID})
		}
	}
	if len(probes) == 0 {
		return
	}

	// 逐探测点计数，端点命中数跨点累加（同一端点的 frpc 只会跑在其中一台，
	// 其余点自然为 0；万一多台都跑，累加与真实连接数一致）
	hits := map[string]int{}
	for _, pb := range probes {
		var tcp4, tcp6 string
		if pb.self {
			var err error
			tcp4, tcp6, err = readLocalProcs()
			if err != nil {
				log.Printf("[frp] 本机连接计数读取失败: %v", err)
				continue
			}
		} else {
			if r.DialForProbe == nil {
				continue
			}
			conn, closer, err := r.DialForProbe(ctx, pb.serverID)
			if err != nil {
				log.Printf("[frp] 节点 %d 连接计数拨号失败: %v", pb.serverID, err)
				continue
			}
			out, err := conn.Run(ctx, localConnsScript)
			closer()
			if err != nil {
				log.Printf("[frp] 节点 %d 连接计数采集失败: %v", pb.serverID, err)
				continue
			}
			tcp4, tcp6 = splitProcDump(out)
		}
		for key, n := range countEndpoints(parseProcSockets(tcp4, tcp6), eps) {
			hits[key] += n
		}
	}
	if len(hits) == 0 {
		return
	}

	// 回写隧道级 local_conns
	writes := map[int64]map[string]int64{} // platformID → remoteID → count
	for key, n := range hits {
		for _, t := range byEndpoint[key] {
			if writes[t.PlatformID] == nil {
				writes[t.PlatformID] = map[string]int64{}
			}
			writes[t.PlatformID][t.RemoteID] = int64(n)
		}
	}
	for pid, m := range writes {
		if err := r.DB.SetFRPTunnelLocalConns(pid, m); err != nil {
			log.Printf("[frp] 平台 %d 本地连接数回写失败: %v", pid, err)
		}
	}

	// Sakura 平台级：conns = 其隧道端点去重之和，口径标记 local。
	// ChmlFrp 有平台值，不覆盖（其 Sync 已标 platform）。
	for _, p := range platforms {
		if p.Kind != KindNatfrp {
			continue
		}
		sum := int64(0)
		seen := map[string]bool{}
		for _, t := range tunnels {
			if t.PlatformID != p.ID {
				continue
			}
			e, ok := newTunnelEndpoint(t)
			if !ok {
				continue
			}
			key := endpointKey(e)
			if seen[key] {
				continue
			}
			seen[key] = true
			sum += int64(hits[key])
		}
		if err := r.DB.SetFRPPlatformConns(p.ID, int(sum), connSrcLocal); err != nil {
			log.Printf("[frp] 平台 %d 本地连接总数回写失败: %v", p.ID, err)
		}
	}
}
