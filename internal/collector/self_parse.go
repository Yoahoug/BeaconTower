package collector

import (
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

// netDevSum 聚合 /proc/net/dev 的 rx/tx 字节（排除 lo，跳过头部两行）。
func netDevSum(data []byte) (rx, tx uint64) {
	for _, line := range strings.Split(string(data), "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name == "lo" || name == "" {
			continue
		}
		f := strings.Fields(line[idx+1:])
		if len(f) < 10 {
			continue
		}
		rx += parseU(f[0])
		tx += parseU(f[8])
	}
	return
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
