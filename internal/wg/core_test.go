package wg

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidKey(kp.Private) || !ValidKey(kp.Public) {
		t.Fatalf("密钥格式无效: %q / %q", kp.Private, kp.Public)
	}
	// 公钥必须能由私钥重推导
	pub, err := PublicKeyFromPrivate(kp.Private)
	if err != nil {
		t.Fatal(err)
	}
	if pub != kp.Public {
		t.Fatalf("公私钥不配对: %q vs %q", pub, kp.Public)
	}
}

func TestGenerateKeyPairUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		kp, err := GenerateKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		if seen[kp.Private] {
			t.Fatal("私钥重复")
		}
		seen[kp.Private] = true
	}
}

func TestGeneratePSK(t *testing.T) {
	psk, err := GeneratePSK()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidKey(psk) {
		t.Fatalf("PSK 格式无效: %q", psk)
	}
}

func TestPublicKeyFromPrivateInvalid(t *testing.T) {
	for _, bad := range []string{"", "not-base64!!", "dG9vIHNob3J0"} {
		if _, err := PublicKeyFromPrivate(bad); err == nil {
			t.Fatalf("应拒绝无效私钥 %q", bad)
		}
	}
}

// renderHub 返回一个典型 hub 配置（对齐现网 wg1 的形态）。
func renderHub(t *testing.T) string {
	t.Helper()
	conf, err := Render(&Interface{
		Name:       "wg0",
		Address:    []string{"10.66.66.2/24"},
		PrivateKey: "vDqnUsQjqFLmtFQg5OjrkkzctTD4Xuip4cV2qGflzk8=",
		ListenPort: 51820,
		PostUp:     []string{"iptables -A FORWARD -i wg0 -j ACCEPT; iptables -A FORWARD -o wg0 -j ACCEPT"},
		PostDown:   []string{"iptables -D FORWARD -i wg0 -j ACCEPT; iptables -D FORWARD -o wg0 -j ACCEPT"},
		Peers: []Peer{
			{Comment: "Ubuntu server", PublicKey: "yu19HVXwMqIUk0fcYzXNbSOYPeZWbmavdLOfCgKneOQ=", AllowedIPs: []string{"10.66.66.66/32"}},
			{Comment: "Win PC", PublicKey: "2gXU/NAkG1ldZF/xV7hzqmG6hUPw730GaQGUWgN9wr4=", AllowedIPs: []string{"10.66.66.10/32"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return conf
}

func TestRenderHub(t *testing.T) {
	conf := renderHub(t)
	for _, want := range []string{
		"[Interface]", "Address = 10.66.66.2/24", "ListenPort = 51820", "MTU", // MTU 不应出现
		"[Peer]", "# Win PC", "AllowedIPs = 10.66.66.10/32",
	} {
		if want == "MTU" {
			if strings.Contains(conf, "MTU") {
				t.Fatal("MTU=0 时不应输出 MTU 行")
			}
			continue
		}
		if !strings.Contains(conf, want) {
			t.Fatalf("渲染结果缺少 %q:\n%s", want, conf)
		}
	}
}

func TestRenderSpokeWithEndpoint(t *testing.T) {
	conf, err := Render(&Interface{
		Name:       "wg0",
		Address:    []string{"10.66.66.66/24"},
		PrivateKey: "vDqnUsQjqFLmtFQg5OjrkkzctTD4Xuip4cV2qGflzk8=",
		MTU:        1420,
		Peers: []Peer{{
			PublicKey:           "ncMXL50/nTJ9piJ8Y18Q15jcYorIlTFWG496Nsiuf0Y=",
			AllowedIPs:          []string{"10.66.66.0/24"},
			Endpoint:            "47.109.156.165:51820",
			PersistentKeepalive: 25,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MTU = 1420", "Endpoint = 47.109.156.165:51820", "PersistentKeepalive = 25", "AllowedIPs = 10.66.66.0/24"} {
		if !strings.Contains(conf, want) {
			t.Fatalf("缺少 %q:\n%s", want, conf)
		}
	}
}

func TestRenderRejectsInvalid(t *testing.T) {
	base := &Interface{
		Name: "wg0", Address: []string{"10.66.66.2/24"},
		PrivateKey: "vDqnUsQjqFLmtFQg5OjrkkzctTD4Xuip4cV2qGflzk8=",
		Peers:      []Peer{{PublicKey: "yu19HVXwMqIUk0fcYzXNbSOYPeZWbmavdLOfCgKneOQ=", AllowedIPs: []string{"10.66.66.66/32"}}},
	}
	bad := []*Interface{
		{Name: "wg0", Address: []string{"999.1.1.1/24"}, PrivateKey: base.PrivateKey},
		{Name: "wg0", Address: base.Address, PrivateKey: "short"},
		{Name: "wg0", Address: base.Address, PrivateKey: base.PrivateKey, ListenPort: 70000},
		{Name: "wg0", Address: base.Address, PrivateKey: base.PrivateKey, Peers: []Peer{{PublicKey: "bad", AllowedIPs: []string{"10.0.0.1/32"}}}},
		{Name: "wg0", Address: base.Address, PrivateKey: base.PrivateKey, Peers: []Peer{{PublicKey: base.Peers[0].PublicKey}}},
	}
	for i, ifc := range bad {
		if _, err := Render(ifc); err == nil {
			t.Fatalf("case %d 应渲染失败", i)
		}
	}
}

func TestParseConfRoundtrip(t *testing.T) {
	conf := renderHub(t)
	ifc, err := ParseConf([]byte(conf))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if ifc.Name != "" { // conf 文件本身不含接口名（文件名即接口名）
		t.Logf("接口名: %q（conf 内不含）", ifc.Name)
	}
	if ifc.ListenPort != 51820 || len(ifc.Address) != 1 || ifc.Address[0] != "10.66.66.2/24" {
		t.Fatalf("Interface 解析不符: %+v", ifc)
	}
	if len(ifc.Peers) != 2 {
		t.Fatalf("应有 2 个 peer，得到 %d", len(ifc.Peers))
	}
	if ifc.Peers[0].Comment != "Ubuntu server" || ifc.Peers[0].AllowedIPs[0] != "10.66.66.66/32" {
		t.Fatalf("Peer#0 解析不符: %+v", ifc.Peers[0])
	}
	// 再渲染一次应语义等价（忽略注释顺序差异，关键字段比对）
	if _, err := Render(ifc); err != nil {
		t.Fatalf("解析结果无法再渲染: %v", err)
	}
}

// TestParseConfRealHub 用现网 wg1 conf 的结构（脱敏）做解析回归。
func TestParseConfRealHub(t *testing.T) {
	sample := `[Interface]
Address = 10.66.66.2/24
ListenPort = 51820
PrivateKey = REimAAAAfakefakefakefakefakefakefakefakefake=

[Peer]
# Ubuntu server
PublicKey = ncMXL50/nTJ9piJ8Y18Q15jcYorIlTFWG496Nsiuf0Y=
AllowedIPs = 10.66.66.66/32

[Peer]
# Mac
PublicKey = 3uStSoLMCkuS6sisWzjgpTIrBXFyQUJvkUaqwxQSF/o=
AllowedIPs = 10.66.66.11/32

[Peer]
# iPhone
PublicKey = yu19HVXwMqIUk0fcYzXNbSOYPeZWbmavdLOfCgKneOQ=
AllowedIPs = 10.66.66.13/32
`
	ifc, err := ParseConf([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(ifc.Peers) != 3 || ifc.Peers[2].Comment != "iPhone" {
		t.Fatalf("解析不符: %+v", ifc)
	}
}

func TestParseConfSpoke(t *testing.T) {
	sample := `[Interface]
Address = 10.66.66.66/24
PrivateKey = REimAAAAfakefakefakefakefakefakefakefakefake=
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT

[Peer]
PublicKey = ncMXL50/nTJ9piJ8Y18Q15jcYorIlTFWG496Nsiuf0Y=
Endpoint = 47.109.156.165:51820
AllowedIPs = 10.66.66.0/24
PersistentKeepalive = 25
`
	ifc, err := ParseConf([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(ifc.PostUp) != 1 || ifc.Peers[0].PersistentKeepalive != 25 || ifc.Peers[0].Endpoint != "47.109.156.165:51820" {
		t.Fatalf("解析不符: %+v", ifc)
	}
}

// TestParseDumpAll 用 wg show all dump 真实列布局（脱敏）验证解析。
func TestParseDumpAll(t *testing.T) {
	sample := `wg0	REimAAAAfakefakefakefakefakefakefakefakefake=	ncMXL50/nTJ9piJ8Y18Q15jcYorIlTFWG496Nsiuf0Y=	51820	off
wg0	yu19HVXwMqIUk0fcYzXNbSOYPeZWbmavdLOfCgKneOQ=	(fake-psk)	(none)	10.66.66.66/32	1769550000	671088640	1342177280	25
wg0	3uStSoLMCkuS6sisWzjgpTIrBXFyQUJvkUaqwxQSF/o=	(none)	192.168.0.11:51820	10.66.66.11/32	0	0	0	off
`
	devs, err := ParseDumpAll([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].Interface != "wg0" || devs[0].ListenPort != 51820 {
		t.Fatalf("接口行解析不符: %+v", devs)
	}
	if len(devs[0].Peers) != 2 {
		t.Fatalf("应有 2 个 peer，得到 %d", len(devs[0].Peers))
	}
	p0 := devs[0].Peers[0]
	if p0.Endpoint != "" || !p0.HasPSK || p0.RX != 671088640 || p0.TX != 1342177280 || p0.Keepalive != 25 {
		t.Fatalf("peer0 解析不符: %+v", p0)
	}
	if p0.LastHandshake.IsZero() {
		t.Fatal("peer0 应有握手时间")
	}
	p1 := devs[0].Peers[1]
	if p1.Endpoint != "192.168.0.11:51820" || p1.HasPSK || !p1.LastHandshake.IsZero() {
		t.Fatalf("peer1 解析不符: %+v", p1)
	}
}

func TestParseDumpAllNoPrefix(t *testing.T) {
	// wg show wg0 dump（无接口名前缀，4/8 列）
	sample := `REimAAAAfakefakefakefakefakefakefakefakefake=	ncMXL50/nTJ9piJ8Y18Q15jcYorIlTFWG496Nsiuf0Y=	51820	off
yu19HVXwMqIUk0fcYzXNbSOYPeZWbmavdLOfCgKneOQ=	(none)	(none)	10.66.66.66/32	1769550000	123	456	25
`
	devs, err := ParseDumpAll([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].Interface != "wg0" || len(devs[0].Peers) != 1 {
		t.Fatalf("解析不符: %+v", devs)
	}
	if devs[0].Peers[0].RX != 123 || devs[0].Peers[0].TX != 456 {
		t.Fatalf("流量解析不符: %+v", devs[0].Peers[0])
	}
}

func TestOnlineByHandshake(t *testing.T) {
	now := time.Unix(1800000000, 0)
	if !OnlineByHandshake(now.Add(-2*time.Minute), now) {
		t.Fatal("2 分钟前握手应判在线")
	}
	if OnlineByHandshake(now.Add(-4*time.Minute), now) {
		t.Fatal("4 分钟前握手应判离线")
	}
	if OnlineByHandshake(time.Time{}, now) {
		t.Fatal("从未握手应判离线")
	}
}

func TestAllocatorNextAndTake(t *testing.T) {
	a, err := NewAllocator("10.66.66.0/24", []string{"10.66.66.2", "10.66.66.66/32", "10.66.66.10", "10.66.66.11", "10.66.66.13"})
	if err != nil {
		t.Fatal(err)
	}
	// .2/.10/.11/.13/.66 已占用；第一个空闲应为 .1
	got, err := a.Next()
	if err != nil {
		t.Fatal(err)
	}
	if got != "10.66.66.1" {
		t.Fatalf("应分配 10.66.66.1，得到 %s", got)
	}
	// Next 取到即占用：连续取址必须给出不同地址（回归：曾漏 Take 导致多成员同 IP）
	next, err := a.Next()
	if err != nil {
		t.Fatal(err)
	}
	if next != "10.66.66.3" {
		t.Fatalf("第二个空闲应为 10.66.66.3，得到 %s", next)
	}
	// 重复占用应报错（Next 已占 .1）
	if err := a.Take("10.66.66.1"); err == nil {
		t.Fatal("重复占用应报错")
	}
	// 出网段应报错
	if err := a.Take("192.168.1.1"); err == nil {
		t.Fatal("网段外 IP 应报错")
	}
}

func TestAllocatorSkipsNetworkBroadcast(t *testing.T) {
	a, err := NewAllocator("10.66.66.0/30", nil)
	if err != nil {
		t.Fatal(err)
	}
	// /30 只有 .1/.2 可用（.0 网络、.3 广播）
	ip1, err := a.Next()
	if err != nil || ip1 != "10.66.66.1" {
		t.Fatalf("应得 .1: %s %v", ip1, err)
	}
	ip2, err := a.Next()
	if err != nil || ip2 != "10.66.66.2" {
		t.Fatalf("应得 .2: %s %v", ip2, err)
	}
	if _, err := a.Next(); err == nil {
		t.Fatal("耗尽后应报错")
	}
}

func TestJudgeHubErrors(t *testing.T) {
	// 非 root + 端口占用 → 两条 Err
	p := &Probe{UID0: false, PkgManager: "apt", ListenPortBusy: true, UfwActive: true}
	issues := Judge(p, RoleHub, "wg0", 51820, false)
	if !HasErr(issues) {
		t.Fatal("非 root 应阻断")
	}
	foundPort := false
	for _, i := range issues {
		if i.Level == Err && strings.Contains(i.Msg, "51820 已被占用") {
			foundPort = true
		}
	}
	if !foundPort {
		t.Fatalf("应提示端口占用: %+v", issues)
	}
}

func TestJudgeSpokeWarnings(t *testing.T) {
	// 正常 Ubuntu spoke：无 Err，仅有「未安装将自动安装」类 Warn
	p := &Probe{UID0: true, PkgManager: "apt", Arch: "x86_64", Virt: "kvm", Systemd: true}
	issues := Judge(p, RoleSpoke, "wg0", 51820, false)
	if HasErr(issues) {
		t.Fatalf("正常 spoke 不应阻断: %+v", issues)
	}
	// 已有 wg0 → Err
	p.WgIfaces = []string{"wg0"}
	issues = Judge(p, RoleSpoke, "wg0", 51820, false)
	if !HasErr(issues) {
		t.Fatal("已存在同接口应阻断")
	}
	// 重配（成员校正/重下发）→ 降级为 Warn，不阻断
	issues = Judge(p, RoleSpoke, "wg0", 51820, true)
	if HasErr(issues) {
		t.Fatalf("重配路径不应阻断: %+v", issues)
	}
}

func TestJudgeUnsupportedOS(t *testing.T) {
	p := &Probe{UID0: true, OsID: "freebsd", PkgManager: ""}
	issues := Judge(p, RoleSpoke, "wg0", 51820, false)
	if !HasErr(issues) {
		t.Fatal("不支持系统应阻断")
	}
}

func TestJudgeProbeError(t *testing.T) {
	p := &Probe{Err: "连接超时"}
	issues := Judge(p, RoleSpoke, "wg0", 51820, false)
	if len(issues) != 1 || !HasErr(issues) {
		t.Fatalf("探测失败应单条 Err: %+v", issues)
	}
}

// TestAllocatorWideSubnet 回归：旧实现只改最后一个字节（ip[3] |= byte(i)），
// /23 及以上子网里高位永不进位，可用地址被截断在 x.x.x.1-254 且会重复分配。
func TestAllocatorWideSubnet(t *testing.T) {
	a, err := NewAllocator("10.66.66.0/23", nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	first, err := a.Next()
	if err != nil || first != "10.66.66.1" {
		t.Fatalf("首个地址应为 10.66.66.1（跳过网络地址），得到 %s %v", first, err)
	}
	seen[first] = true
	for i := 1; i < 300; i++ {
		ip, err := a.Next()
		if err != nil {
			t.Fatalf("第 %d 次分配失败（/23 应有 510 个可用地址）: %v", i, err)
		}
		if seen[ip] {
			t.Fatalf("地址重复分配: %s", ip)
		}
		seen[ip] = true
	}
	// 越过 .255 之后必须进入第二个 /24
	if !seen["10.66.67.1"] {
		t.Fatal("未跨入 10.66.67.0/24 网段（IP 分配没有进位）")
	}
}

// TestPickPeerIndex 回归：节点 conf 有多段 [Peer] 时必须定位到指向现役 hub 的那段
// （旧实现硬编码 Peers[0]，会把别的网络的 peer 换成新 hub → 成员失联）。
func TestPickPeerIndex(t *testing.T) {
	peers := []Peer{
		{PublicKey: "OTHER-NET-KEY", AllowedIPs: []string{"10.9.9.0/24"}},
		{PublicKey: "HUB-A-KEY", AllowedIPs: []string{"10.66.66.0/24"}},
		{PublicKey: "HUB-B-KEY", AllowedIPs: []string{"172.20.0.0/24"}},
	}
	// 1) 现役 hub 公钥可精确命中
	if i := pickPeerIndex(peers, "HUB-A-KEY", "10.66.66.2"); i != 1 {
		t.Fatalf("应按公钥命中第 1 段，得到 %d", i)
	}
	// 2) 公钥未知（被外部改动）时按 AllowedIPs 覆盖 hub IP 兜底
	if i := pickPeerIndex(peers, "", "10.66.66.2"); i != 1 {
		t.Fatalf("应按 AllowedIPs 命中第 1 段，得到 %d", i)
	}
	// 3) 单段 [Peer] 直接取它
	single := []Peer{{PublicKey: "HUB-A-KEY", AllowedIPs: []string{"0.0.0.0/0"}}}
	if i := pickPeerIndex(single, "", ""); i != 0 {
		t.Fatalf("单段应取 0，得到 %d", i)
	}
	// 4) 多段且无法判定 → -1（宁可报错也不猜）
	if i := pickPeerIndex(peers, "", ""); i != -1 {
		t.Fatalf("无法判定应返回 -1，得到 %d", i)
	}
}

// TestCanaryIndex 金丝雀定位：按载荷指定优先，缺省回落第一个成员。
func TestCanaryIndex(t *testing.T) {
	steps := []*store.WGTaskStep{
		{ServerID: sqlStepID(2)},
		{ServerID: sqlStepID(3)},
		{ServerID: sqlStepID(4)},
	}
	if i := canaryIndex(steps, 4); i != 2 {
		t.Fatalf("应定位到指定成员（下标 2），得到 %d", i)
	}
	if i := canaryIndex(steps, 0); i != 0 {
		t.Fatalf("未指定时应回落第一个成员，得到 %d", i)
	}
	if i := canaryIndex(steps, 999); i != 0 {
		t.Fatalf("指定的成员不在步骤里时应回落第一个成员，得到 %d", i)
	}
	if i := canaryIndex(nil, 1); i != -1 {
		t.Fatalf("空步骤应返回 -1，得到 %d", i)
	}
}

// TestProbeScriptPortColumn 回归：ss 的输出列是
// State Recv-Q Send-Q Local-Address:Port Peer-Address:Port，端口在第 4 列；
// 旧实现取 $5（Peer-Address，UDP 恒为 0.0.0.0:*）导致占用永远探不出来。
func TestProbeScriptPortColumn(t *testing.T) {
	s := probeScript(51820, "wg0")
	if !strings.Contains(s, "awk 'NR>1{print $4}'") {
		t.Fatal("端口探测应取 ss/netstat 的第 4 列")
	}
	if strings.Contains(s, "print $5") {
		t.Fatal("不得再取第 5 列（Peer-Address）")
	}
	// 本网接口正监听该端口时不算冲突（重配/warm standby 场景）
	if !strings.Contains(s, `wg show "$i" listen-port`) {
		t.Fatal("缺少「占用者即本网接口」豁免逻辑")
	}
	if !strings.Contains(s, "'wg0'") {
		t.Fatal("应带上本网的接口名")
	}
	// 不探测端口时不应带上占用检测
	if s0 := probeScript(0, "wg0"); strings.Contains(s0, "ss -uln") {
		t.Fatal("listenPort=0 不应探测端口占用")
	}
}

// sqlStepID 构造测试用的步骤 server_id。
func sqlStepID(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }
