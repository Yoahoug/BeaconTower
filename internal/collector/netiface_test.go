package collector

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 与真实宿主同形的样本：物理网卡 enp3s0f1 被桥 br0 承接，另有自定义命名的 TUN
// （SakuraiTunnel，名字前缀黑名单抓不住）、wg0、veth。
const netDevSample = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 5000000 4000 0 0 0 0 0 0 5000000 4000 0 0 0 0 0 0
enp3s0f1: 157188193941 100 0 0 0 0 0 0 147561994586 90 0 0 0 0 0 0
    br0: 152619137029 99 0 0 0 0 0 0 148000000000 88 0 0 0 0 0 0
SakuraiTunnel: 900000000 10 0 0 0 0 0 0 800000000 9 0 0 0 0 0 0
   wg0: 70000000 5 0 0 0 0 0 0 60000000 4 0 0 0 0 0 0
 wlp2s0: 1000 1 0 0 0 0 0 0 2000 1 0 0 0 0 0 0
veth123: 1 1 0 0 0 0 0 0 2 1 0 0 0 0 0 0
`

// 降级口径（无物理判据）取「累计字节最多」的样本：桥的累计多于物理网卡。
const netDevFallbackSample = `    lo: 1 1 0 0 0 0 0 0 1 1 0 0 0 0 0 0
enp3s0f1: 1000 1 0 0 0 0 0 0 2000 1 0 0 0 0 0 0
    br0: 9000 1 0 0 0 0 0 0 9000 1 0 0 0 0 0 0
SakuraiTunnel: 500 1 0 0 0 0 0 0 500 1 0 0 0 0 0 0
`

func TestNetDevEntriesParse(t *testing.T) {
	es := netDevEntries([]byte(netDevSample))
	if len(es) != 7 {
		t.Fatalf("entries=%d want 7: %+v", len(es), es)
	}
	if es[0].Name != "lo" || es[0].Rx != 5000000 || es[0].Tx != 5000000 {
		t.Fatalf("lo 解析错误: %+v", es[0])
	}
	var phy *netDevEntry
	for i := range es {
		if es[i].Name == "enp3s0f1" {
			phy = &es[i]
		}
	}
	if phy == nil || phy.Rx != 157188193941 || phy.Tx != 147561994586 {
		t.Fatalf("enp3s0f1 解析错误: %+v", phy)
	}
	// 兜底口径（全接口求和）必须排除 lo，否则本机流量会混入本地回环
	rx, tx := netDevSum([]byte(netDevSample))
	if rx != 157188193941+152619137029+900000000+70000000+1000+1 || tx != 147561994586+148000000000+800000000+60000000+2000+2 {
		t.Fatalf("netDevSum 兜底口径错误 rx=%d tx=%d", rx, tx)
	}
}

// 物理判据命中时只统计物理网卡：桥 br0 与成员 enp3s0f1 是同一份流量，相加即翻倍。
func TestSelectNetIfacesPhysicalWins(t *testing.T) {
	es := netDevEntries([]byte(netDevSample))
	names := selectNetIfaces(es, func(n string) bool { return n == "enp3s0f1" || n == "wlp2s0" })
	if strings.Join(names, ",") != "enp3s0f1,wlp2s0" {
		t.Fatalf("names=%v", names)
	}
	rx, tx := sumNetIfaces(es, names)
	if rx != 157188193941+1000 || tx != 147561994586+2000 {
		t.Fatalf("物理口径求和错误 rx=%d tx=%d", rx, tx)
	}
}

// 判据不可用（容器未挂宿主 sysfs）时退化为累计字节最多的单接口，绝不重复计数。
func TestSelectNetIfacesFallbackMaxTraffic(t *testing.T) {
	es := netDevEntries([]byte(netDevFallbackSample))
	for _, pred := range []func(string) bool{nil, func(string) bool { return false }} {
		names := selectNetIfaces(es, pred)
		if strings.Join(names, ",") != "br0" {
			t.Fatalf("降级口径应取累计最多的 br0，得到 %v", names)
		}
	}
	if names := selectNetIfaces(nil, nil); names != nil {
		t.Fatalf("空输入应返回 nil，得到 %v", names)
	}
	onlyLo := netDevEntries([]byte("    lo: 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16\n"))
	if names := selectNetIfaces(onlyLo, nil); names != nil {
		t.Fatalf("仅回环应返回 nil，得到 %v", names)
	}
}

// fakeSysNet 构造假 sysfs class/net 树：列出的接口带 device 链接（＝物理网卡判据命中）。
func fakeSysNet(t *testing.T, phys ...string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "class", "net")
	for _, n := range phys {
		p := filepath.Join(root, n, "device")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeDevFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dev")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNetCountersPhysicalWithSysfs(t *testing.T) {
	dev := writeDevFile(t, netDevSample)
	rx, tx := netCounters(dev, fakeSysNet(t, "enp3s0f1"))
	if rx != 157188193941 || tx != 147561994586 {
		t.Fatalf("物理口径应只算 enp3s0f1：rx=%d tx=%d", rx, tx)
	}
	// 判据不可用（sysNetRoot 为空）→ 退化取累计最多的单接口（该样本里 br0 最多）
	devFB := writeDevFile(t, netDevFallbackSample)
	rx, tx = netCounters(devFB, "")
	if rx != 9000 || tx != 9000 {
		t.Fatalf("降级口径应取 br0：rx=%d tx=%d", rx, tx)
	}
	// 文件缺失不得 panic/报错，返回 0
	if rx, tx := netCounters(filepath.Join(t.TempDir(), "nope"), ""); rx != 0 || tx != 0 {
		t.Fatalf("缺失文件应返回 0：rx=%d tx=%d", rx, tx)
	}
}

func TestValidIfaceName(t *testing.T) {
	ok := []string{"eth0", "enp3s0f1", "br-61a34153e36f", "eth0.100", "wg0", "veth01aa750", "SakuraiTunnel", "eth0@if5", "en0"}
	for _, n := range ok {
		if !validIfaceName(n) {
			t.Fatalf("%q 应合法", n)
		}
	}
	bad := []string{"", ".", "..", "../etc", "eth0/../..", "eth 0", "eth0\n", strings.Repeat("a", 33)}
	for _, n := range bad {
		if validIfaceName(n) {
			t.Fatalf("%q 应非法", n)
		}
	}
	if isPhysicalNetIface("", "eth0") {
		t.Fatal("sysNetRoot 为空时不应判定为物理网卡")
	}
	if isPhysicalNetIface("/sys/class/net", "../../etc") {
		t.Fatal("非法接口名不应通过路径穿越访问到 device")
	}
}

// TestCollectScriptNetSegment 直接执行采集脚本的网络段（经 BT_NETBASE/BT_SYSNET/BT_PROCDIR
// 注入假 proc/sysfs 树），覆盖 shell+awk 口径：物理网卡优先、无判据取单接口、连接数/进程数。
func TestCollectScriptNetSegment(t *testing.T) {
	base := t.TempDir()
	netDir := filepath.Join(base, "net")
	procDir := filepath.Join(base, "proc")
	if err := os.MkdirAll(netDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(netDir, "dev"), netDevSample)
	write(filepath.Join(netDir, "tcp"), "  sl  local_address rem_address st\n 0: a b c\n 1: a b c\n")
	write(filepath.Join(netDir, "tcp6"), "  sl  local_address\n")
	write(filepath.Join(netDir, "udp"), "  sl  local_address\n 0: a b c\n 1: a b c\n 2: a b c\n")
	write(filepath.Join(netDir, "udp6"), "  sl  local_address\n 0: a b c\n")
	fbNetDir := filepath.Join(base, "net-fb")
	if err := os.MkdirAll(fbNetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(fbNetDir, "dev"), netDevFallbackSample)
	for _, pid := range []string{"1", "42", "314", "notpid"} {
		if err := os.MkdirAll(filepath.Join(procDir, pid), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := func(netBase, sysNet string) map[string]string {
		t.Helper()
		cmd := exec.Command("sh", "-s")
		cmd.Env = append(os.Environ(), "BT_NETBASE="+netBase, "BT_SYSNET="+sysNet, "BT_PROCDIR="+procDir)
		cmd.Stdin = strings.NewReader(collectScriptNet)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("执行网络采集段失败: %v", err)
		}
		m := map[string]string{}
		for _, line := range strings.Split(string(out), "\n") {
			if k, v, ok := strings.Cut(line, "="); ok {
				m[k] = v
			}
		}
		return m
	}

	// 有物理判据（两张网卡）→ 只算物理网卡，桥/隧道/自定义 tun 全部排除
	m := run(netDir, fakeSysNet(t, "enp3s0f1", "wlp2s0"))
	if got, want := m["bt_net"], "157188194941:147561996586"; got != want {
		t.Fatalf("bt_net=%s want %s", got, want)
	}
	if m["bt_tcp"] != "2" || m["bt_tcp6"] != "0" || m["bt_udp"] != "3" || m["bt_udp6"] != "1" {
		t.Fatalf("连接数错误: %v", m)
	}
	if m["bt_proc"] != "3" {
		t.Fatalf("bt_proc=%s want 3", m["bt_proc"])
	}

	// 无判据（sysfs 不存在，如容器未挂宿主 /sys）→ 退化取累计最多的单接口（br0）
	m = run(fbNetDir, filepath.Join(base, "no-such-sysfs"))
	if got, want := m["bt_net"], "9000:9000"; got != want {
		t.Fatalf("降级 bt_net=%s want %s", got, want)
	}

	// 缺 dev 文件（老内核/权限）→ 0:0 且不报错
	m = run(filepath.Join(base, "nope"), filepath.Join(base, "no-such-sysfs"))
	if m["bt_net"] != "0:0" {
		t.Fatalf("dev 缺失应输出 bt_net=0:0，实际 %q", m["bt_net"])
	}
}
