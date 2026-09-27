//go:build linux

package collector

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// RunCollectScriptLinux 本机节点（is_self，Linux）原生采集：进程内直接读
// /proc、/sys 与 statfs，零 fork。采集脚本需 fork 30+ 个外部进程（每 10s 一轮），
// 在 docker stats 上表现为周期性 CPU 突发；原生路径完全消除该开销。
// 字段语义与 collectScript 对齐，解析/差分/落库链路共用 applySample。
// 任一关键步骤失败即回退脚本路径（RunCollectScript），行为不变只多开销。
func collectSelfNative(ctx context.Context) (*RawSample, error) {
	raw, err := collectSelfLinux(ctx)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// collectSelfLinux 原生采集主体；返回 error 时由调用方回退脚本。
func collectSelfLinux(ctx context.Context) (*RawSample, error) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		return nil, fmt.Errorf("非 Linux 环境：%w", err)
	}
	s := &RawSample{Rapl: map[string]uint64{}, RaplMax: map[string]uint64{}, Thermal: map[string]int64{}}

	// uptime
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			if v, e := strconv.ParseFloat(f[0], 64); e == nil {
				s.UptimeS = int64(v)
			}
		}
	}
	// load
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) >= 3 {
			s.Load1 = clampF(parseF(f[0]), 0, 1e6)
			s.Load5 = clampF(parseF(f[1]), 0, 1e6)
			s.Load15 = clampF(parseF(f[2]), 0, 1e6)
		}
	}
	// CPU 累计 tick（差分在 applySample）
	if b, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				s.CpuTotal, s.CpuIdle = parseCPU(strings.TrimSpace(line))
				break
			}
		}
	}
	// 核数 / 型号
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		s.CpuCores = cpuinfoCores(b)
		s.CpuModel = cleanTrim(cpuinfoModel(b), 128)
	}
	if s.CpuCores == 0 {
		s.CpuCores = numCPU()
	}
	// mem / swap
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		total, avail, free, swapT, swapF := meminfoParse(b)
		applyMemWiring(s, total, avail, free, swapT, swapF)
	}
	// 磁盘：/ 挂载点 statfs（df -kP / 等价）
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err == nil {
		s.DiskTotal = int64(st.Blocks) * int64(st.Bsize)
		s.DiskUsed = s.DiskTotal - int64(st.Bavail)*int64(st.Bsize)
		if s.DiskUsed < 0 {
			s.DiskUsed = 0
		}
	}
	// 全盘列表：/proc/self/mounts 过滤 + statfs（df -x tmpfs/devtmpfs/overlay 等价）
	if b, err := os.ReadFile("/proc/self/mounts"); err == nil {
		var parts []string
		for _, mp := range mountsFilter(b) {
			var mst syscall.Statfs_t
			if err := syscall.Statfs(mp, &mst); err != nil {
				continue
			}
			total := int64(mst.Blocks) * int64(mst.Bsize) / 1024
			avail := int64(mst.Bavail) * int64(mst.Bsize) / 1024
			if total <= 0 {
				continue
			}
			parts = append(parts, mp+"|"+strconv.FormatInt(total, 10)+"|"+strconv.FormatInt(avail, 10))
		}
		s.DisksRaw = strings.Join(parts, ";")
		if len(s.DisksRaw) > 2048 {
			s.DisksRaw = s.DisksRaw[:2048]
		}
	}
	// 网络
	if b, err := os.ReadFile("/proc/net/dev"); err == nil {
		s.NetRx, s.NetTx = netDevSum(b)
	}
	// tcp/udp 连接数
	s.TcpConns = connFileCount("/proc/net/tcp") + connFileCount("/proc/net/tcp6")
	s.UdpConns = connFileCount("/proc/net/udp") + connFileCount("/proc/net/udp6")
	// 进程数
	if ents, err := os.ReadDir("/proc"); err == nil {
		n := 0
		for _, e := range ents {
			if isPidName(e.Name()) {
				n++
			}
		}
		s.Processes = n
	}
	// 主机名 / os-release / 内核 / 架构
	if h, err := os.Hostname(); err == nil {
		s.Hostname = cleanTrim(h, 64)
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		s.OsName, s.OsID, s.OsVer = osReleaseGet(b)
		s.OsName = cleanTrim(s.OsName, 64)
		s.OsID = cleanTrim(s.OsID, 32)
		s.OsVer = cleanTrim(s.OsVer, 32)
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		s.Kernel = cleanTrim(string(b), 64)
	}
	if u, err := exec.LookPath("uname"); err == nil && ctxAlive(ctx) {
		out, err := exec.CommandContext(ctx, u, "-m").Output()
		if err == nil {
			s.Arch = cleanTrim(string(out), 16)
		}
	} else if h, err := os.ReadFile("/proc/sys/kernel/arch"); err == nil {
		s.Arch = cleanTrim(string(h), 16)
	}
	// 虚拟化：/sys/class/dmi/id（宿主命名空间可见；容器内读不到则 unknown）
	if b, err := os.ReadFile("/sys/class/dmi/id/sys_vendor"); err == nil {
		s.Virt = dmiVirt(string(b))
	}
	// RAPL：两层布局探测，与脚本一致
	raplBase := ""
	for _, c := range []string{"/sys/class/powercap", "/powercap-ro", "/powercap-ro/intel-rapl"} {
		if fileExists(c + "/intel-rapl:0/energy_uj") {
			raplBase = c
			break
		}
	}
	if raplBase != "" {
		dirs, _ := filepath.Glob(raplBase + "/intel-rapl:*")
		sort.Strings(dirs)
		for _, d := range dirs {
			name := filepath.Base(d)
			if strings.Contains(name, "mmio") {
				continue
			}
			if b, err := os.ReadFile(d + "/energy_uj"); err == nil {
				if u, e := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64); e == nil {
					s.Rapl[name] = u
					s.HasRapl = true
				}
			}
			if b, err := os.ReadFile(d + "/max_energy_range_uj"); err == nil {
				if u, e := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64); e == nil && u > 0 {
					s.RaplMax[name] = u
				}
			}
		}
	}
	// 温度
	if zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*"); len(zones) > 0 {
		sort.Strings(zones)
		for _, z := range zones {
			typ := strings.TrimSpace(readFileStr(z + "/type"))
			temp := strings.TrimSpace(readFileStr(z + "/temp"))
			if typ == "" || temp == "" {
				continue
			}
			if t, e := strconv.ParseInt(temp, 10, 64); e == nil && t >= -100000 && t <= 300000 {
				s.Thermal[cleanTrim(typ, 64)] = t
			}
		}
	}
	// 频率
	if freqs, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq"); len(freqs) > 0 {
		var sum int64
		n := 0
		for _, f := range freqs {
			if v, e := strconv.ParseInt(strings.TrimSpace(readFileStr(f)), 10, 64); e == nil && v > 0 {
				sum += v
				n++
			}
		}
		if n > 0 {
			s.FreqMhz = sum / int64(n) / 1000
		}
	}
	// 电池
	pss, _ := filepath.Glob("/sys/class/power_supply/*")
	sort.Strings(pss)
	for _, p := range pss {
		if strings.TrimSpace(readFileStr(p+"/type")) != "Battery" {
			continue
		}
		s.BatStatus = cleanTrim(readFileStr(p+"/status"), 16)
		s.BatPowerU = clampInt(parseInt(strings.TrimSpace(readFileStr(p+"/power_now"))), 0, 1<<40)
		s.HasBat = true
	}
	// 公网 IP：读本机 6h 缓存（由脚本/原生探测回写），不每轮外呼
	s.PubIP = readPubIPCache()
	// 与 parseOutput 一致的完整性校验
	if s.MemTotal == 0 || s.CpuTotal == 0 {
		return nil, fmt.Errorf("本机原生采集不完整（mem/cpu 缺失）")
	}
	s.fixMem()
	return s, nil
}

func connFileCount(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return procConnCount(b)
}

func readFileStr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(b))
}

func isPidName(name string) bool {
	if name == "" {
		return false
	}
	for _, ch := range name {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func ctxAlive(ctx context.Context) bool {
	return ctx != nil && ctx.Err() == nil
}

func numCPU() int {
	n := 0
	ents, err := os.ReadDir("/sys/devices/system/cpu")
	if err != nil {
		return 0
	}
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, "cpu") && len(name) > 3 && isPidName(name[3:]) {
			n++
		}
	}
	return n
}
