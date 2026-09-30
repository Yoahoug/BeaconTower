package wg

import (
	"strings"
	"testing"
)

// TestParseProbeFirewall 回归：预检要如实回读 iptables/nft 是否存在。
func TestParseProbeFirewall(t *testing.T) {
	p := parseProbe("pf_wg=1\npf_wgq=1\npf_fw=0\npf_end=1\n")
	if p.HasFirewall {
		t.Fatal("pf_fw=0 应解析为缺少 iptables/nft")
	}
	if p2 := parseProbe("pf_fw=1\n"); !p2.HasFirewall {
		t.Fatal("pf_fw=1 应解析为存在")
	}
	// 老脚本没有 pf_fw 行时应取零值（缺工具），提示用户而不是静默
	if p3 := parseProbe("pf_wg=1\n"); p3.HasFirewall {
		t.Fatal("无 pf_fw 行时不应假定存在")
	}
}

func TestFirewallInstallScript(t *testing.T) {
	for _, pkg := range []string{"apt", "dnf", "yum", "apk", "pacman"} {
		s := firewallInstallScript(pkg)
		if !strings.Contains(s, "iptables") {
			t.Fatalf("%s 的补装脚本应安装 iptables: %s", pkg, s)
		}
		if !strings.Contains(s, "|| true") {
			t.Fatalf("%s 的补装应尽力而为（失败不阻断）: %s", pkg, s)
		}
	}
	// 极简镜像常删掉 apt lists，先 update 才装得上（实测：不 update 时补装静默失败）
	if !strings.Contains(firewallInstallScript("apt"), "apt-get update") {
		t.Fatal("apt 补装脚本应先 apt-get update")
	}
	if s := firewallInstallScript(""); s != "true" {
		t.Fatalf("未知包管理器应退化为 no-op: %s", s)
	}
}

// TestJudgeFirewallMissing 只有 hub/standby 的 conf 带 iptables PostUp：
// 缺 iptables/nft 时对中心角色给 Warn，对 spoke 不打扰。
func TestJudgeFirewallMissing(t *testing.T) {
	p := &Probe{UID0: true, PkgManager: "apt", HasWg: true, HasWgQuick: true, HasFirewall: false}
	for _, role := range []Role{RoleHub, RoleStandby} {
		found := false
		for _, i := range Judge(p, role, "wg0", 51820, false) {
			if strings.Contains(i.Msg, "iptables") {
				found = true
				if i.Level != Warn {
					t.Fatalf("缺 iptables 不应阻断（能自动补装）: %+v", i)
				}
			}
		}
		if !found {
			t.Fatalf("中心角色（%s）缺 iptables/nft 应有提示", role)
		}
	}
	for _, i := range Judge(p, RoleSpoke, "wg0", 0, false) {
		if strings.Contains(i.Msg, "iptables") {
			t.Fatalf("spoke 不写 PostUp，不该提示 iptables: %+v", i)
		}
	}
	p.HasFirewall = true
	for _, i := range Judge(p, RoleHub, "wg0", 51820, false) {
		if strings.Contains(i.Msg, "iptables") {
			t.Fatalf("有 iptables/nft 时不该提示: %+v", i)
		}
	}
}

// TestHubConfPostUpTolerant 回归：hub conf 的 PostUp 必须 `|| true` 收尾——
// 没有 iptables 的节点上（极简镜像）wg-quick 会因放行规则失败而拒绝拉起接口，
// 实测导致整步「开启转发失败: context deadline exceeded」以外的更硬失败。
func TestHubConfPostUpTolerant(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	conf, err := HubConfFile(kp.Private, "10.66.66.2/24", 51820, 1420, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, "PostUp = iptables") {
		t.Fatalf("hub conf 应有 PostUp 放行 FORWARD: %s", conf)
	}
	for _, line := range strings.Split(conf, "\n") {
		if strings.HasPrefix(line, "PostUp =") && !strings.HasSuffix(strings.TrimSpace(line), "|| true") {
			t.Fatalf("PostUp 应容忍 iptables 缺失（|| true 收尾）: %s", line)
		}
		if strings.HasPrefix(line, "PostDown =") && !strings.Contains(line, "|| true") {
			t.Fatalf("PostDown 应容忍 iptables 缺失: %s", line)
		}
	}
}

func TestParseProbeWgCIDRs(t *testing.T) {
	p := parseProbe("pf_wg_cidrs=wg1:10.66.66.11/24,wg2:10.9.9.1/24,wg1:fd00::1/64,\n")
	if len(p.WgCIDRs) != 3 {
		t.Fatalf("应解析 3 条地址记录: %+v", p.WgCIDRs)
	}
	if p.WgCIDRs[0] != "wg1:10.66.66.11/24" {
		t.Fatalf("条目格式应为 iface:cidr: %q", p.WgCIDRs[0])
	}
	if q := parseProbe("pf_ifaces=wg0 \n"); len(q.WgCIDRs) != 0 {
		t.Fatalf("无 pf_wg_cidrs 时应为空: %+v", q.WgCIDRs)
	}
}

func TestCidrOverlap(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"10.66.66.11/24", "10.66.66.0/24", true},
		{"10.66.66.11/32", "10.66.66.0/24", true},
		{"10.66.67.0/24", "10.66.66.0/24", false},
		{"10.66.66.0/23", "10.66.66.0/24", true},
		{"", "10.66.66.0/24", false},
		{"bad", "10.66.66.0/24", false},
		{"10.66.66.1/24", "", false},
	}
	for _, c := range cases {
		if got := cidrOverlap(c.a, c.b); got != c.want {
			t.Fatalf("cidrOverlap(%q,%q)=%v 期望 %v", c.a, c.b, got, c.want)
		}
	}
}

// TestJudgeSubnetOverlap 回归（真机实测）：Mac 上已有 10.66.66.0/24 的既有 WG 网，
// 新建同网段测试网后内核把网段路由留给旧接口——新隧道能握手，但主机访问 10.66.66.x
// 全走旧网（"通了但访问的是别的机器"）。预检必须阻断这种网段重叠。
func TestJudgeSubnetOverlap(t *testing.T) {
	p := &Probe{UID0: true, PkgManager: "apt", HasWg: true, HasWgQuick: true, HasFirewall: true,
		WgIfaces: []string{"wg1"}, WgCIDRs: []string{"wg1:10.66.66.11/24"}}

	p.TargetSubnet = "10.66.66.0/24"
	issues := Judge(p, RoleSpoke, "wg0", 0, false)
	found := false
	for _, i := range issues {
		if strings.Contains(i.Msg, "重叠") {
			found = true
			if i.Level != Err {
				t.Fatalf("网段重叠应阻断（否则用户拿到的是「通了但连错机器」）: %+v", i)
			}
		}
	}
	if !found {
		t.Fatalf("同网段的既有接口应触发重叠阻断: %+v", issues)
	}

	// 换不冲突的网段：只保留「存在其他 WG 接口」的提示，不阻断
	p.TargetSubnet = "10.77.77.0/24"
	for _, i := range Judge(p, RoleSpoke, "wg0", 0, false) {
		if i.Level == Err {
			t.Fatalf("不重叠的既有接口不应阻断: %+v", i)
		}
	}

	// 自身接口（重配场景）参与重叠检测时跳过
	p2 := &Probe{UID0: true, PkgManager: "apt", HasWg: true, HasWgQuick: true, HasFirewall: true,
		WgIfaces: []string{"wg0"}, WgCIDRs: []string{"wg0:10.66.66.2/24"}, TargetSubnet: "10.66.66.0/24"}
	for _, i := range Judge(p2, RoleHub, "wg0", 51820, true) {
		if i.Level == Err && strings.Contains(i.Msg, "重叠") {
			t.Fatalf("自身接口的重叠属重配，不该阻断: %+v", i)
		}
	}
}
