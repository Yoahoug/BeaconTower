package frp

import (
	"net"
	"testing"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// procLine 构造一条 /proc/net/tcp 行（local/rem 为真实 IP 字符串 + 端口，
// 由测试侧做小端序编码，反向验证 parseProcAddr 的解析正确性）。
func procLine(t *testing.T, localIP, remoteIP string, localPort, remotePort int, st string) string {
	t.Helper()
	enc := func(ipStr string, port int) string {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("bad ip %q", ipStr)
		}
		v4 := ip.To4()
		var hex string
		if v4 != nil {
			// IPv4：一组 4 字节小端序
			hex = hexByte(v4[3]) + hexByte(v4[2]) + hexByte(v4[1]) + hexByte(v4[0])
		} else {
			// IPv6：4 组 4 字节，每组小端序
			b := ip.To16()
			for g := 0; g < 4; g++ {
				grp := b[g*4 : g*4+4]
				hex += hexByte(grp[3]) + hexByte(grp[2]) + hexByte(grp[1]) + hexByte(grp[0])
			}
		}
		return hex + ":" + hexPort(port)
	}
	return "   1: " + enc(localIP, localPort) + " " + enc(remoteIP, remotePort) + " " + st + " 00000000:00000000 02:00000000 00000000     0        0 0 2 0000000000000000 20 4 30 10 -1"
}

func hexByte(b byte) string {
	const d = "0123456789ABCDEF"
	return string([]byte{d[b>>4], d[b&0xF]})
}

func hexPort(p int) string {
	const d = "0123456789ABCDEF"
	return string([]byte{d[(p>>12)&0xF], d[(p>>8)&0xF], d[(p>>4)&0xF], d[p&0xF]})
}

func TestParseProcAddr(t *testing.T) {
	// 127.0.0.1:3000 → 小端序 0100007F，端口 0BB8
	ip, port, ok := parseProcAddr("0100007F:0BB8")
	if !ok || port != 3000 || !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("v4 解析错误: %v %v %v", ip, port, ok)
	}
	// v4-mapped 127.0.0.1（tcp6 文件里常见）
	ip, port, ok = parseProcAddr("0000000000000000FFFF00000100007F:0BB8")
	if !ok || port != 3000 || ip.To4() == nil || !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("v4-mapped 解析错误: %v %v %v", ip, port, ok)
	}
	// 真实 IPv6
	ip, _, ok = parseProcAddr("24098A62424975710000000000001009:0016")
	if !ok || ip == nil || ip.To4() != nil {
		t.Fatalf("v6 解析错误: %v %v %v", ip, port, ok)
	}
	// 非法输入
	if _, _, ok := parseProcAddr("nonsense"); ok {
		t.Fatal("非法地址不应解析成功")
	}
	if _, _, ok := parseProcAddr("0100007F:0000"); ok {
		t.Fatal("端口 0 不应解析成功（listen 态由状态位过滤，端口 0 无意义）")
	}
}

// countAll 等价旧顶层入口：解析两段原文后按端口集合计数（wildcard 语义）。
func countAll(tcp4, tcp6 string, want map[int]bool) int {
	eps := map[string]tunnelEndpoint{}
	for port := range want {
		e := tunnelEndpoint{port: port, wildcard: true}
		eps[endpointKey(e)] = e
	}
	return sumHits(countEndpoints(parseProcSockets(tcp4, tcp6), eps))
}

func sumHits(hits map[string]int) int {
	n := 0
	for _, v := range hits {
		n += v
	}
	return n
}

func TestCountLocalConnsSameHostDedup(t *testing.T) {
	// 场景复刻服务器实测：frpc(127.0.0.1) → 服务(192.168.0.10:3000)，同机两条 socket
	// frpc 侧：local 51752 → remote 192.168.0.10:3000（命中）
	// 服务侧：local 3000 → remote 192.168.0.10:51752（本地端口=隧道端口，必须排除）
	// 平台控制通道：local 40123 → remote 194.147.16.88:7000（远端端口不命中）
	body := procLine(t, "192.168.0.10", "192.168.0.10", 51752, 3000, "01") + "\n" +
		procLine(t, "192.168.0.10", "192.168.0.10", 3000, 51752, "01") + "\n" +
		procLine(t, "192.168.0.10", "194.147.16.88", 40123, 7000, "01") + "\n" +
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	want := map[int]bool{3000: true}
	if got := countAll(body, "", want); got != 1 {
		t.Fatalf("同机去重失败: got %d want 1", got)
	}
}

func TestCountLocalConnsStatesAndWildcards(t *testing.T) {
	// TIME_WAIT(06) 与 LISTEN(0A) 不计；wildcard 端点（local_ip 空）只按端口
	body := procLine(t, "192.168.0.10", "192.168.0.10", 51752, 3000, "06") + "\n" +
		procLine(t, "0.0.0.0", "0.0.0.0", 3000, 0, "0A") + "\n" +
		procLine(t, "10.0.0.5", "192.168.0.10", 60111, 3000, "01") + "\n"
	if got := countAll(body, "", map[int]bool{3000: true}); got != 1 {
		t.Fatalf("状态过滤失败: got %d want 1", got)
	}
}

func TestEndpointIPMatching(t *testing.T) {
	// 指定 local_ip 的隧道：远端 IP 必须精确匹配（v4-mapped 归一为 v4 后相等）
	ep, ok := newTunnelEndpoint(&store.FRPTunnel{Proto: "tcp", LocalIP: "192.168.0.10", LocalPort: 3000})
	if !ok || ep.wildcard {
		t.Fatalf("端点构造失败: %+v ok=%v", ep, ok)
	}
	v4mapped := localSock{rip: net.IPv4(192, 168, 0, 10), rport: 3000, lport: 51752}
	if !ep.match(v4mapped) {
		t.Fatal("v4-mapped 同值 IP 应命中")
	}
	wrongIP := localSock{rip: net.IPv4(10, 0, 0, 5), rport: 3000, lport: 51752}
	if ep.match(wrongIP) {
		t.Fatal("不同 IP 不应命中")
	}
}

func TestNewTunnelEndpointSkipUDPAndInvalid(t *testing.T) {
	if _, ok := newTunnelEndpoint(&store.FRPTunnel{Proto: "udp", LocalIP: "127.0.0.1", LocalPort: 53}); ok {
		t.Fatal("udp 隧道不参与计数")
	}
	if _, ok := newTunnelEndpoint(&store.FRPTunnel{Proto: "tcp", LocalIP: "127.0.0.1", LocalPort: 0}); ok {
		t.Fatal("端口 0 不参与计数")
	}
	// 域名/空 IP → wildcard
	e, ok := newTunnelEndpoint(&store.FRPTunnel{Proto: "tcp", LocalIP: "my.host.name", LocalPort: 80})
	if !ok || !e.wildcard {
		t.Fatalf("域名应退化为 wildcard: %+v ok=%v", e, ok)
	}
}

func TestEndpointLocalhostNotWildcard(t *testing.T) {
	// local_ip=localhost 必须按回环精确匹配，而不是 wildcard——否则面板 SSH
	// 到任意节点（rport 恰好等于隧道端口的 established socket）都会被误计入。
	ep, ok := newTunnelEndpoint(&store.FRPTunnel{Proto: "tcp", LocalIP: "localhost", LocalPort: 22})
	if !ok || ep.wildcard {
		t.Fatalf("localhost 不应退化为 wildcard: %+v ok=%v", ep, ok)
	}
	loop := localSock{rip: net.IPv4(127, 0, 0, 1), rport: 22, lport: 51000}
	if !ep.match(loop) {
		t.Fatal("回环远端应命中")
	}
	// 线上误计数场景：面板 SSH 采集连接（远端=阿里云节点:22）不得命中
	ssh := localSock{rip: net.IPv4(47, 98, 10, 20), rport: 22, lport: 51001}
	if ep.match(ssh) {
		t.Fatal("非回环远端不应命中（SSH 采集连接误计数回归）")
	}
}

func TestSplitProcDump(t *testing.T) {
	raw := "__BT_TCP4__\n  sl  local rem st\n  0: 0100007F:0BB8 0100007F:1F90 01 ...\n__BT_TCP6__\n  sl  local rem st\n  0: 0000000000000000FFFF00000100007F:0BB8 0100007F:1F90 01 ...\n"
	tcp4, tcp6 := splitProcDump(raw)
	if !contains(tcp4, "0100007F:0BB8") || contains(tcp4, "FFFF0000") {
		t.Fatalf("tcp4 段切分错误: %q", tcp4)
	}
	if !contains(tcp6, "FFFF0000") {
		t.Fatalf("tcp6 段切分错误: %q", tcp6)
	}
	// 缺 tcp6 标记（远端无 tcp6 文件）
	tcp4, tcp6 = splitProcDump("__BT_TCP4__\n  0: 0100007F:0BB8 0100007F:1F90 01\n")
	if tcp6 != "" || !contains(tcp4, "0BB8") {
		t.Fatalf("无 tcp6 段切分错误: %q / %q", tcp4, tcp6)
	}
	// 完全无标记
	if a, b := splitProcDump("garbage"); a != "" || b != "" {
		t.Fatalf("无标记应返回空: %q %q", a, b)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestCountEndpointsSharedEndpoint(t *testing.T) {
	// 两个平台指向同一端点（用户真实场景）：每条连接两个隧道各计一次（按端点命中回填）
	ep, _ := newTunnelEndpoint(&store.FRPTunnel{Proto: "tcp", LocalIP: "192.168.0.10", LocalPort: 3000})
	eps := map[string]tunnelEndpoint{endpointKey(ep): ep}
	socks := parseProcSockets(
		procLine(t, "192.168.0.10", "192.168.0.10", 1111, 3000, "01")+"\n"+
			procLine(t, "192.168.0.10", "192.168.0.10", 2222, 3000, "01")+"\n", "")
	hits := countEndpoints(socks, eps)
	if hits[endpointKey(ep)] != 2 {
		t.Fatalf("共享端点计数错误: %v", hits)
	}
}
