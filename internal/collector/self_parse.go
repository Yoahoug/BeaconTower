package collector

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 本机节点（is_self）采集的纯解析函数：从 /proc、/sys 文本内容提取字段，
// 与 collectScript 的 awk/grep 输出语义一致，不依赖平台（可单测）。

// meminfoParse 提取 MemTotal/MemAvailable/MemFree/SwapTotal/SwapFree（kB 原值）。
func meminfoParse(data []byte) (total, avail, free, swapT, swapF int64) {
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n := parseInt(strings.Fields(strings.TrimSpace(v))[0])
		switch strings.TrimSpace(k) {
		case "MemTotal":
			total = n
		case "MemAvailable":
			avail = n
		case "MemFree":
			free = n
		case "SwapTotal":
			swapT = n
		case "SwapFree":
			swapF = n
		}
	}
	return
}

// applyMemWiring 依据 meminfo 字段装配内存指标（原生路径与脚本 bt_mem_* 同语义）：
// MemUsed = MemTotal - MemAvailable（无 MemAvailable 时回落 MemFree）。
func applyMemWiring(s *RawSample, total, avail, free, swapT, swapF int64) {
	s.MemTotal = total * 1024
	if avail > 0 {
		s.setMemAvail(avail * 1024)
	} else {
		s.setMemFree(free * 1024)
	}
	s.SwapTotal = swapT * 1024
	s.SwapUsed = (swapT - swapF) * 1024
	if s.SwapUsed < 0 {
		s.SwapUsed = 0
	}
}

// netDevEntry 单接口收发累计字节（/proc/net/dev 一行）。
type netDevEntry struct {
	Name string
	Rx   uint64
	Tx   uint64
}

// netDevEntries 解析 /proc/net/dev（跳过两行头部；含 lo，由调用方决定是否统计）。
func netDevEntries(data []byte) []netDevEntry {
	var out []netDevEntry
	for _, line := range strings.Split(string(data), "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name == "" {
			continue
		}
		f := strings.Fields(line[idx+1:])
		if len(f) < 10 {
			continue
		}
		out = append(out, netDevEntry{Name: name, Rx: parseU(f[0]), Tx: parseU(f[8])})
	}
	return out
}

// netDevSum 汇总全部非回环接口的收发字节（无接口判据时的最粗口径，桥/隧道会重复计数）。
func netDevSum(data []byte) (rx, tx uint64) {
	for _, e := range netDevEntries(data) {
		if e.Name == "lo" {
			continue
		}
		rx += e.Rx
		tx += e.Tx
	}
	return
}

// selectNetIfaces 挑选参与「本机流量」统计的接口，规避重复计数与虚拟接口污染。
// 判据两级：
//  1. isPhysical 命中（sysfs 有 device 链接＝真实 PCI/USB 网卡）：只统计物理网卡。
//     宿主上桥/隧道/veth 的收发字节与物理网卡是同一份流量（br0 与成员 enp3s0f1
//     各计一次＝翻倍），只算物理网卡既无重复，也覆盖 bond/桥成员分担的场景。
//  2. 判据不可用（容器未挂宿主 sysfs）或一个都不命中：取累计字节最多的单个接口。
//     单网卡主机结果一致；多网卡主机低估，但绝不会重复计数。
//
// 恒不含 lo；无可用接口返回 nil。
func selectNetIfaces(entries []netDevEntry, isPhysical func(string) bool) []string {
	var phys []string
	var top string
	var topBytes uint64
	for _, e := range entries {
		if e.Name == "lo" {
			continue
		}
		if isPhysical != nil && isPhysical(e.Name) {
			phys = append(phys, e.Name)
			continue
		}
		if n := e.Rx + e.Tx; n > topBytes {
			topBytes, top = n, e.Name
		}
	}
	if len(phys) > 0 {
		return phys
	}
	if top != "" {
		return []string{top}
	}
	return nil
}

// sumNetIfaces 按接口名集合求收发字节之和（未出现的名字忽略）。
func sumNetIfaces(entries []netDevEntry, names []string) (rx, tx uint64) {
	if len(names) == 0 {
		return 0, 0
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	for _, e := range entries {
		if want[e.Name] {
			rx += e.Rx
			tx += e.Tx
		}
	}
	return
}

// netCounters 读 net/dev 文件并按「物理网卡优先」口径汇总收发字节。
// sysNetRoot 为可用于物理判据的 sysfs class/net 目录（空则不判物理，退化取单接口）。
func netCounters(devFile, sysNetRoot string) (rx, tx uint64) {
	b, err := os.ReadFile(devFile)
	if err != nil {
		return 0, 0
	}
	entries := netDevEntries(b)
	names := selectNetIfaces(entries, func(name string) bool {
		return isPhysicalNetIface(sysNetRoot, name)
	})
	if len(names) == 0 {
		return netDevSum(b)
	}
	return sumNetIfaces(entries, names)
}

// isPhysicalNetIface 判据：<sysNetRoot>/<name>/device 存在 => 真实 PCI/USB 网卡。
// 桥（br0/br-*/docker0/virbr0）、veth、隧道（wg0/tun 及自定义命名的 tun，如 SakuraiTunnel）、
// bond、macvlan/vlan 均无该链接，从而被排除，避免与物理网卡重复计数。
// sysNetRoot 为空时不做判定（返回 false，由 selectNetIfaces 退化取单接口）。
func isPhysicalNetIface(sysNetRoot, name string) bool {
	if sysNetRoot == "" || !validIfaceName(name) {
		return false
	}
	_, err := os.Stat(filepath.Join(sysNetRoot, name, "device"))
	return err == nil
}

// validIfaceName 接口名白名单字符集（内核允许的字符），同时挡住路径穿越。
func validIfaceName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_' || c == '-' || c == '.' || c == ':' || c == '@':
		default:
			return false
		}
	}
	return name != "." && name != ".."
}

// procConnCount 统计 /proc/net/{tcp,tcp6,udp,udp6} 条目数（首行 header 不计）。
func procConnCount(data []byte) int {
	n := 0
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		n++
	}
	return n
}

// osReleaseGet 提取 NAME/ID/VERSION_ID 并去引号。
func osReleaseGet(data []byte) (name, id, ver string) {
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch strings.TrimSpace(k) {
		case "NAME":
			name = v
		case "ID":
			id = v
		case "VERSION_ID":
			ver = v
		}
	}
	return
}

// cpuinfoCores 统计 processor 行数。
func cpuinfoCores(data []byte) int {
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "processor") {
			n++
		}
	}
	return n
}

// cpuinfoModel 提取 model name（x86），ARM 等无 model name 时兜底 Model。
// x86 cpuinfo 里 "model"（数字）出现在 "model name" 之前，须全量扫描后取优先级；
// "model name" 为 N/A（部分 ARM）时同样回落 Model。
func cpuinfoModel(data []byte) string {
	modelName, model := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			k = strings.TrimSpace(strings.ReplaceAll(k, "\t", " "))
			switch k {
			case "model name":
				if modelName == "" {
					modelName = strings.TrimSpace(v)
				}
			case "Model", "model":
				if model == "" {
					model = strings.TrimSpace(v)
				}
			}
		}
	}
	if modelName != "" && modelName != "N/A" {
		return modelName
	}
	return model
}

// mountsFilter 解析 /proc/self/mounts，剔除伪文件系统（df 默认隐藏 + 脚本 -x 的三类），
// 每挂载点只留首条。size/avail 由调用方 statfs 回填，此处只做过滤。
func mountsFilter(data []byte) []string {
	skip := map[string]bool{
		"tmpfs": true, "devtmpfs": true, "overlay": true, "proc": true, "sysfs": true,
		"devpts": true, "cgroup": true, "cgroup2": true, "mqueue": true, "shm": true,
		"ramfs": true, "hugetlbfs": true, "bpf": true, "tracefs": true, "debugfs": true,
		"configfs": true, "pstore": true, "securityfs": true, "efivarfs": true,
		"fusectl": true, "autofs": true, "binfmt_misc": true, "nsfs": true,
		"rpc_pipefs": true, "selinuxfs": true, "openpromfs": true, "none": true,
	}
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		mp, fstype := f[1], f[2]
		if skip[fstype] || seen[mp] || strings.HasPrefix(mp, "/proc") ||
			strings.HasPrefix(mp, "/sys") || strings.HasPrefix(mp, "/dev") {
			continue
		}
		seen[mp] = true
		out = append(out, mp)
	}
	return out
}

// dmiVirt 由 DMI 厂商串推断虚拟化类型（systemd-detect-virt 的轻量替代，
// 容器内无该二进制；无法判定返回 unknown，与脚本缺省一致）。
func dmiVirt(vendor string) string {
	v := strings.ToLower(vendor)
	switch {
	case strings.Contains(v, "qemu"), strings.Contains(v, "red hat"),
		strings.Contains(v, "amazon"), strings.Contains(v, "bochs"),
		strings.Contains(v, "openstack"):
		return "kvm"
	case strings.Contains(v, "microsoft"):
		return "hyperv"
	case strings.Contains(v, "vmware"):
		return "vmware"
	case strings.Contains(v, "innotek"):
		return "vbox"
	default:
		return "unknown"
	}
}

var ip4Re = regexp.MustCompile(`(?:\d{1,3}\.){3}\d{1,3}`)

// ip4Extract 从响应体提取首个 IPv4（ip-api JSON 等价解析）。
func ip4Extract(body string) string {
	return ip4Re.FindString(body)
}

// cleanTrim 去空白并截断（cleanStr 的轻量封装别名）。
func cleanTrim(v string, n int) string { return cleanStr(strings.TrimSpace(v), n) }
