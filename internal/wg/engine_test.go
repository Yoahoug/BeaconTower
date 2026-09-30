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
