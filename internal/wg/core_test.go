package wg

import (
	"strings"
	"testing"
	"time"
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
	if err := a.Take("10.66.66.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Next(); err != nil {
		t.Fatal(err)
	}
	// 重复占用应报错
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
	_ = a.Take(ip1)
	ip2, err := a.Next()
	if err != nil || ip2 != "10.66.66.2" {
		t.Fatalf("应得 .2: %s %v", ip2, err)
	}
	_ = a.Take(ip2)
	if _, err := a.Next(); err == nil {
		t.Fatal("耗尽后应报错")
	}
}

func TestJudgeHubErrors(t *testing.T) {
	// 非 root + 端口占用 → 两条 Err
	p := &Probe{UID0: false, PkgManager: "apt", ListenPortBusy: true, UfwActive: true}
	issues := Judge(p, RoleHub, "wg0", 51820)
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
	issues := Judge(p, RoleSpoke, "wg0", 51820)
	if HasErr(issues) {
		t.Fatalf("正常 spoke 不应阻断: %+v", issues)
	}
	// 已有 wg0 → Err
	p.WgIfaces = []string{"wg0"}
	issues = Judge(p, RoleSpoke, "wg0", 51820)
	if !HasErr(issues) {
		t.Fatal("已存在同接口应阻断")
	}
}

func TestJudgeUnsupportedOS(t *testing.T) {
	p := &Probe{UID0: true, OsID: "freebsd", PkgManager: ""}
	issues := Judge(p, RoleSpoke, "wg0", 51820)
	if !HasErr(issues) {
		t.Fatal("不支持系统应阻断")
	}
}

func TestJudgeProbeError(t *testing.T) {
	p := &Probe{Err: "连接超时"}
	issues := Judge(p, RoleSpoke, "wg0", 51820)
	if len(issues) != 1 || !HasErr(issues) {
		t.Fatalf("探测失败应单条 Err: %+v", issues)
	}
}
