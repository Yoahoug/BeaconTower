package collector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
)

// SSHCred 解密后的连接凭据（内存态，不落盘）。见文件底部 SSH 执行段说明。

// collectScriptNet 网络/连接数/进程数采集段，单独成常量以便单测直接执行
// （netdev_script_test.go 用临时目录经 BT_NETBASE/BT_SYSNET/BT_PROCDIR 注入假 proc 树）。
const collectScriptNet = `bt_netbase=${BT_NETBASE:-/proc/net}
bt_sysnet=${BT_SYSNET:-/sys/class/net}
bt_procdir=${BT_PROCDIR:-/proc}
# 流量口径（与面板原生采集一致）：只统计物理网卡（sysfs 有 device 链接＝真实 PCI/USB 网卡）。
# 桥/隧道/veth 与物理网卡是同一份流量（br0 与成员 enp3s0f1 各计一次＝翻倍），故排除；
# 拿不到判据（无 sysfs）时退化为「累计字节最多的单接口」——绝不重复计数。
bt_phys=""
for d in $bt_sysnet/*/device; do
  [ -e "$d" ] || continue
  i=${d%/device}; i=${i##*/}
  [ "$i" = lo ] || bt_phys="$bt_phys $i"
done
echo bt_net=$( [ -r $bt_netbase/dev ] && awk -v phys="$bt_phys" '/:/{n=$1; sub(/:$/,"",n); if(n=="lo") next; if(phys!=""){if(index(" "phys" ", " "n" ")==0) next; rx+=$2; tx+=$10; next} if($2+$10>best){best=$2+$10; rx=$2; tx=$10}} END{print rx+0":"tx+0}' $bt_netbase/dev 2>/dev/null || echo 0:0 )
echo bt_tcp=$( [ -r $bt_netbase/tcp ] && awk 'END{print NR-1+0}' $bt_netbase/tcp 2>/dev/null || echo 0 )
echo bt_tcp6=$( [ -r $bt_netbase/tcp6 ] && awk 'END{print NR-1+0}' $bt_netbase/tcp6 2>/dev/null || echo 0 )
echo bt_udp=$( [ -r $bt_netbase/udp ] && awk 'END{print NR-1+0}' $bt_netbase/udp 2>/dev/null || echo 0 )
echo bt_udp6=$( [ -r $bt_netbase/udp6 ] && awk 'END{print NR-1+0}' $bt_netbase/udp6 2>/dev/null || echo 0 )
echo bt_proc=$(ls $bt_procdir 2>/dev/null | grep -c '^[0-9]')
`

// collectScript 单次 exec 采集脚本（doc/02 §4.2 + doc/09 §2.2）。
// 全部只读：/proc、df、uname、os-release、sysfs 功率/温度接口、出口 IP 回显。
// 输出多行 key=value，面板侧按白名单键解析。
// 网络段前缀的 bt_netbase/bt_sysnet/bt_procdir 可由环境变量覆盖：本机节点在容器内
// 执行本脚本时注入宿主路径（见 RunCollectScript + localScriptEnv），远端节点用默认值。
const collectScript = `echo bt_begin=1
echo bt_up_s=$(cut -d. -f1 /proc/uptime 2>/dev/null)
echo bt_load=$(cat /proc/loadavg 2>/dev/null)
echo bt_cpu=$(grep '^cpu ' /proc/stat 2>/dev/null)
echo bt_cpu_n=$(grep -c '^processor' /proc/cpuinfo 2>/dev/null || nproc 2>/dev/null || echo 0)
echo bt_mem_t=$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null)
echo bt_mem_a=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo 2>/dev/null)
echo bt_mem_f=$(awk '/^MemFree:/{print $2}' /proc/meminfo 2>/dev/null)
echo bt_swap_t=$(awk '/^SwapTotal:/{print $2}' /proc/meminfo 2>/dev/null)
echo bt_swap_f=$(awk '/^SwapFree:/{print $2}' /proc/meminfo 2>/dev/null)
echo bt_disk=$(df -kP / 2>/dev/null | awk 'END{print $2":"$4}')
echo bt_disks=$(df -kP -x tmpfs -x devtmpfs -x overlay 2>/dev/null | awk 'NR>1{print $6"|"$2"|"$4}' | tr '\n' ';')
` + collectScriptNet + `echo bt_os_name=$(grep '^NAME=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo bt_hostname=$(hostname 2>/dev/null || uname -n 2>/dev/null || true)
echo bt_os_id=$(grep '^ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo bt_os_ver=$(grep '^VERSION_ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo bt_kernel=$(uname -r 2>/dev/null)
echo bt_arch=$(uname -m 2>/dev/null)
echo bt_cpu_model=$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2-)
echo bt_virt=$(systemd-detect-virt 2>/dev/null || echo unknown)
# 公网 IP：6h 文件缓存（探测外呼慢且被墙环境常空转；缓存命中则零开销）
pubip=""
[ -r /tmp/.bt_pub_ip ] && read -r pubip ts < /tmp/.bt_pub_ip 2>/dev/null
now_s=$(cut -d. -f1 /proc/uptime 2>/dev/null)
case "$pubip$ts$now_s" in
  *[!0-9\ ]*) pubip="" ;;
esac
if [ -z "$pubip" ] || [ -z "$ts" ] || [ -z "$now_s" ] || [ $((now_s - ts)) -gt 21600 ] 2>/dev/null; then
  pubip=$(curl -sS -m 5 -4 https://ip.sb 2>/dev/null || curl -sS -m 5 -4 'http://ip-api.com/json/?fields=query' 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
  case "$pubip" in
    [0-9]*.[0-9]*.[0-9]*.[0-9]*) echo "$pubip $now_s" > /tmp/.bt_pub_ip 2>/dev/null ;;
    *) pubip="" ;;
  esac
fi
echo bt_pub_ip=$pubip
# RAPL：优先标准 sysfs 路径；容器内 Docker 默认 mask 了
# /sys/devices/virtual/powercap（class 符号链接失效），故兜底挂载点 /powercap-ro
# （compose 挂宿主 /sys/devices/virtual/powercap → 容器 /powercap-ro，
#   域目录可能直接在其下、也可能深一层 intel-rapl/，两层都探测）
rapl_base=""
[ -e /sys/class/powercap/intel-rapl:0/energy_uj ] && rapl_base=/sys/class/powercap
if [ -z "$rapl_base" ]; then
  for c in /powercap-ro /powercap-ro/intel-rapl; do
    [ -e "$c/intel-rapl:0/energy_uj" ] && rapl_base=$c && break
  done
fi
if [ -n "$rapl_base" ]; then
  for d in $rapl_base/intel-rapl:*; do
    case $d in *mmio*) continue ;; esac
    echo rapl:${d##*/}=$(cat $d/energy_uj 2>/dev/null)
    echo raplmax:${d##*/}=$(cat $d/max_energy_range_uj 2>/dev/null)
  done
fi
for z in /sys/class/thermal/thermal_zone*; do
  echo tz:$(cat $z/type 2>/dev/null)=$(cat $z/temp 2>/dev/null)
done
echo bt_freq=$(awk '{s+=$1; n++} END{if(n)print int(s/n/1000)}' /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq 2>/dev/null)
for p in /sys/class/power_supply/*; do
  [ "$(cat $p/type 2>/dev/null)" = "Battery" ] || continue
  echo bat:status=$(cat $p/status 2>/dev/null)
  echo bat:power_now=$(cat $p/power_now 2>/dev/null)
done
echo bt_end=1`

// RawSample 采集输出解析后的原始值（面板侧二次计算的输入）。
type RawSample struct {
	UptimeS   int64
	Load1     float64
	Load5     float64
	Load15    float64
	CpuTotal  uint64
	CpuIdle   uint64
	CpuCores  int
	MemTotal  int64 // bytes
	MemUsed   int64
	SwapTotal int64
	SwapUsed  int64
	DiskTotal int64
	DiskUsed  int64
	DisksRaw  string
	NetRx     uint64
	NetTx     uint64
	TcpConns  int
	UdpConns  int
	Processes int
	Hostname  string
	OsName    string
	OsID      string
	OsVer     string
	Kernel    string
	Arch      string
	CpuModel  string
	Virt      string
	PubIP     string
	Rapl      map[string]uint64 // name -> energy_uj
	RaplMax   map[string]uint64
	Thermal   map[string]int64 // type -> millidegree
	FreqMhz   int64
	BatStatus string
	BatPowerU int64 // µW
	HasRapl   bool
	HasBat    bool

	memAvailSet bool
	memFree     int64
}

const (
	maxLineLen   = 4096
	maxOutputLen = 256 * 1024
)

// parseOutput 按白名单键解析 key=value 输出（doc/05 §4 恶意输出注入防护：
// 白名单键、长度上限、数值类型校验）。
func parseOutput(out []byte) (*RawSample, error) {
	if len(out) > maxOutputLen {
		return nil, fmt.Errorf("采集输出过大（%d bytes），疑似异常", len(out))
	}
	s := &RawSample{Rapl: map[string]uint64{}, RaplMax: map[string]uint64{}, Thermal: map[string]int64{}}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if len(line) > maxLineLen {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || seen[k+":first"] && isSingle(k) {
			continue
		}
		if isSingle(k) {
			seen[k+":first"] = true
		}
		switch {
		case k == "bt_up_s":
			s.UptimeS = clampInt(parseInt(v), 0, 1<<40)
		case k == "bt_load":
			f := strings.Fields(v)
			if len(f) >= 3 {
				s.Load1, s.Load5, s.Load15 = clampF(parseF(f[0]), 0, 1e6), clampF(parseF(f[1]), 0, 1e6), clampF(parseF(f[2]), 0, 1e6)
			}
		case k == "bt_cpu":
			total, idle := parseCPU(v)
			s.CpuTotal, s.CpuIdle = total, idle
		case k == "bt_cpu_n":
			s.CpuCores = int(clampInt(parseInt(v), 0, 4096))
		case k == "bt_mem_t":
			s.MemTotal = clampInt(parseInt(v), 0, 1<<50) * 1024
		case k == "bt_mem_a":
			s.setMemAvail(clampInt(parseInt(v), 0, 1<<50) * 1024)
		case k == "bt_mem_f":
			s.setMemFree(clampInt(parseInt(v), 0, 1<<50) * 1024)
		case k == "bt_swap_t":
			s.SwapTotal = clampInt(parseInt(v), 0, 1<<50) * 1024
		case k == "bt_swap_f":
			s.SwapUsed = s.SwapTotal - clampInt(parseInt(v), 0, 1<<50)*1024
			if s.SwapUsed < 0 {
				s.SwapUsed = 0
			}
		case k == "bt_disk":
			if a, b, ok := strings.Cut(v, ":"); ok {
				total := clampInt(parseInt(a), 0, 1<<60) * 1024
				avail := clampInt(parseInt(b), 0, 1<<60) * 1024
				s.DiskTotal = total
				s.DiskUsed = total - avail
				if s.DiskUsed < 0 {
					s.DiskUsed = 0
				}
			}
		case k == "bt_disks":
			if len(v) <= 2048 {
				s.DisksRaw = v
			}
		case k == "bt_net":
			if a, b, ok := strings.Cut(v, ":"); ok {
				s.NetRx = clampU(parseU(a))
				s.NetTx = clampU(parseU(b))
			}
		case k == "bt_tcp":
			s.TcpConns = int(clampInt(parseInt(v), 0, 1<<30))
		case k == "bt_tcp6":
			s.TcpConns += int(clampInt(parseInt(v), 0, 1<<30))
		case k == "bt_udp":
			s.UdpConns = int(clampInt(parseInt(v), 0, 1<<30))
		case k == "bt_udp6":
			s.UdpConns += int(clampInt(parseInt(v), 0, 1<<30))
		case k == "bt_proc":
			s.Processes = int(clampInt(parseInt(v), 0, 1<<30))
		case k == "bt_hostname":
			s.Hostname = cleanStr(v, 64)
		case k == "bt_os_name":
			s.OsName = cleanStr(v, 64)
		case k == "bt_os_id":
			s.OsID = cleanStr(v, 32)
		case k == "bt_os_ver":
			s.OsVer = cleanStr(v, 32)
		case k == "bt_kernel":
			s.Kernel = cleanStr(v, 64)
		case k == "bt_arch":
			s.Arch = cleanStr(v, 16)
		case k == "bt_cpu_model":
			s.CpuModel = cleanStr(v, 128)
		case k == "bt_virt":
			s.Virt = cleanStr(v, 32)
		case k == "bt_pub_ip":
			if ip := net.ParseIP(strings.TrimSpace(v)); ip != nil {
				s.PubIP = ip.String()
			}
		case strings.HasPrefix(k, "rapl:"):
			name := cleanStr(strings.TrimPrefix(k, "rapl:"), 64)
			if name == "" || v == "" {
				continue
			}
			u, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				continue
			}
			s.Rapl[name] = u
			s.HasRapl = true
		case strings.HasPrefix(k, "raplmax:"):
			name := cleanStr(strings.TrimPrefix(k, "raplmax:"), 64)
			if name == "" || v == "" {
				continue
			}
			if u, err := strconv.ParseUint(v, 10, 64); err == nil && u > 0 {
				s.RaplMax[name] = u
			}
		case strings.HasPrefix(k, "tz:"):
			typ := cleanStr(strings.TrimPrefix(k, "tz:"), 64)
			if typ == "" || v == "" {
				continue
			}
			if t, err := strconv.ParseInt(v, 10, 64); err == nil && t >= -100000 && t <= 300000 {
				s.Thermal[typ] = t
			}
		case k == "bt_freq":
			s.FreqMhz = clampInt(parseInt(v), 0, 100000)
		case k == "bat:status":
			s.BatStatus = cleanStr(v, 16)
			s.HasBat = true
		case k == "bat:power_now":
			s.BatPowerU = clampInt(parseInt(v), 0, 1<<40)
			s.HasBat = true
		}
	}
	if s.MemTotal == 0 || s.CpuTotal == 0 {
		return nil, fmt.Errorf("采集输出不完整（缺少 mem/cpu），目标机可能不支持")
	}
	// MemAvailable 缺失时用 MemFree 兜底
	s.fixMem()
	return s, nil
}

// isSingle 单值键（重复出现只取第一行）。
func isSingle(k string) bool {
	switch k {
	case "bt_up_s", "bt_load", "bt_cpu", "bt_cpu_n", "bt_mem_t", "bt_mem_a", "bt_mem_f",
		"bt_swap_t", "bt_swap_f", "bt_disk", "bt_disks", "bt_net", "bt_tcp", "bt_tcp6",
		"bt_udp", "bt_udp6", "bt_proc", "bt_hostname", "bt_os_name", "bt_os_id", "bt_os_ver",
		"bt_kernel", "bt_arch", "bt_cpu_model", "bt_virt", "bt_pub_ip", "bt_freq",
		"bat:status", "bat:power_now":
		return true
	}
	return false
}

func (s *RawSample) setMemAvail(v int64) {
	s.MemUsed = s.MemTotal - v
	if s.MemUsed < 0 {
		s.MemUsed = 0
	}
	s.memAvailSet = true
}

func (s *RawSample) setMemFree(v int64) { s.memFree = v }

func (s *RawSample) fixMem() {
	if !s.memAvailSet && s.memFree > 0 {
		s.MemUsed = s.MemTotal - s.memFree
		if s.MemUsed < 0 {
			s.MemUsed = 0
		}
	}
	if s.MemUsed > s.MemTotal {
		s.MemUsed = s.MemTotal
	}
}

func parseCPU(v string) (total, idle uint64) {
	f := strings.Fields(v)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0
	}
	var nums []uint64
	for _, x := range f[1:] {
		n, err := strconv.ParseUint(x, 10, 64)
		if err != nil {
			return 0, 0
		}
		nums = append(nums, n)
	}
	for _, n := range nums {
		total += n
	}
	// idle + iowait
	idle = nums[3]
	if len(nums) > 4 {
		idle += nums[4]
	}
	return total, idle
}

func parseInt(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

// parseOutputLoose 与 parseOutput 同一白名单，但不做 mem/cpu 完整性校验
//（darwin 本机采集的 mem/cpu 由 Go 侧原生补齐）。
func parseOutputLoose(out []byte) (*RawSample, error) {
	if len(out) > maxOutputLen {
		return nil, fmt.Errorf("采集输出过大（%d bytes），疑似异常", len(out))
	}
	s := &RawSample{Rapl: map[string]uint64{}, RaplMax: map[string]uint64{}, Thermal: map[string]int64{}}
	scan := func(line string) {
		line = strings.TrimRight(line, "\r")
		if line == "" || len(line) > maxLineLen {
			return
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" {
			return
		}
		switch {
		case k == "bt_up_s":
			// boottime 秒 → uptime
			if boot := parseInt(v); boot > 0 {
				s.UptimeS = clampInt(time.Now().Unix()-boot, 0, 1<<40)
			}
		case k == "bt_load":
			f := strings.Fields(v)
			if len(f) >= 3 {
				s.Load1, s.Load5, s.Load15 = clampF(parseF(f[0]), 0, 1e6), clampF(parseF(f[1]), 0, 1e6), clampF(parseF(f[2]), 0, 1e6)
			}
		case k == "bt_cpu_n":
			s.CpuCores = int(clampInt(parseInt(v), 0, 4096))
		case k == "bt_proc":
			s.Processes = int(clampInt(parseInt(v), 0, 1<<30))
		case k == "bt_hostname":
			s.Hostname = cleanStr(v, 64)
		case k == "bt_os_name":
			s.OsName = cleanStr(v, 64)
		case k == "bt_os_id":
			s.OsID = cleanStr(v, 32)
		case k == "bt_os_ver":
			s.OsVer = cleanStr(v, 32)
		case k == "bt_kernel":
			s.Kernel = cleanStr(v, 64)
		case k == "bt_arch":
			s.Arch = cleanStr(v, 16)
		case k == "bt_virt":
			s.Virt = cleanStr(v, 32)
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		scan(line)
	}
	return s, nil
}

// ---------- darwin 本机原生采集（仅本地执行，非 SSH 路径） ----------

func darwinSysctlInt(name string) int64 {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0
	}
	return parseInt(strings.TrimSpace(string(out)))
}

func darwinMem() (total, used int64, ok bool) {
	pageSize := darwinSysctlInt("hw.pagesize")
	if pageSize <= 0 {
		return 0, 0, false
	}
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, 0, false
	}
	var active, wired, compressed int64
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok2 := strings.Cut(line, ":")
		if !ok2 {
			continue
		}
		// vm_stat 数值带尾点（"307651."），单位可能是 " pages." 或 " page."
		v = strings.TrimSpace(v)
		v = strings.TrimSuffix(v, " pages.")
		v = strings.TrimSuffix(v, " page.")
		v = strings.TrimSuffix(v, ".")
		v = strings.TrimSpace(v)
		n := parseInt(v)
		switch {
		case strings.HasPrefix(k, "Pages active"):
			active = n
		case strings.HasPrefix(k, "Pages wired"):
			wired = n
		case strings.HasPrefix(k, "Pages compressed"):
			compressed = n
		}
	}
	used = (active + wired + compressed) * pageSize // purgeable 计入 used 偏保守，此处忽略
	total = darwinSysctlInt("hw.memsize")
	if total <= 0 || used <= 0 {
		return 0, 0, false
	}
	if used > total {
		used = total
	}
	return total, used, true
}

func darwinCpuTicks() (total, idle uint64) {
	// iostat -c 2 首行是开机以来均值，第二行才是上次调用以来的窗口均值；
	// 归一为千分比伪 tick（CpuTotal=1000 恒定，CpuIdle 随窗口浮动），差分即得占用率。
	// 比 top -l 1 便宜一个量级（iostat ~1ms，top 每次 ~300ms 全核遍历，10s 一轮纯属浪费）。
	out, err := exec.Command("iostat", "-c", "2").Output()
	if err != nil {
		return 0, 0
	}
	var idN float64
	windows := 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		// 数据行固定 9 列：KB/t tps MB/s us sy id 1m 5m 15m（表头含 "cpu" 等字母行被 Fields 长度排除）
		if len(f) != 9 {
			continue
		}
		id, e := strconv.ParseFloat(f[5], 64)
		if e != nil {
			continue
		}
		idN = id
		windows++
	}
	// 只有一行数据（iostat 首启无历史窗口）时 id 是开机均值，可用但偏旧；正常取最后一行
	if windows == 0 {
		return 0, 0
	}
	idle = uint64(clampF(idN, 0, 100) * 10) // 百分比 → 千分比
	return 1000, idle
}

func darwinDisk(mount string) (total, used int64) {
	out, err := exec.Command("df", "-k", mount).Output()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	f := strings.Fields(lines[len(lines)-1])
	if len(f) < 3 {
		return 0, 0
	}
	total = clampInt(parseInt(f[1]), 0, 1<<60) * 1024
	avail := clampInt(parseInt(f[3]), 0, 1<<60) * 1024
	used = total - avail
	if used < 0 {
		used = 0
	}
	return total, used
}

func darwinNetTotals() (rx, tx uint64) {
	out, err := exec.Command("netstat", "-ibn").Output()
	if err != nil {
		return 0, 0
	}
	// 口径与 Linux 侧一致：优先物理接口（en*＝以太网/雷电/USB/无线统一命名），
	// utun*/awdl*/llw*/bridge*/gif*/stf*/vmenet*/ap*/p2p* 与 en* 是同一份流量（重复计数）。
	// 一个物理接口都没有时退化为「累计字节最多的单接口」。
	var physRx, physTx uint64
	var foundPhys bool
	var bestRx, bestTx, bestSum uint64
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		// Link 行固定 11 列：<name> <mtu> <Link#n> <mac> Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll
		// 只聚合 Link 行（每接口仅一条，避免同接口多地址行重复累计），排除回环
		if len(f) != 11 || f[0] == "lo0" || !strings.HasPrefix(f[2], "<Link") {
			continue
		}
		r := uint64(clampInt(parseInt(f[6]), 0, 1<<60))
		t := uint64(clampInt(parseInt(f[9]), 0, 1<<60))
		if strings.HasPrefix(f[0], "en") {
			foundPhys = true
			physRx += r
			physTx += t
			continue
		}
		if s := r + t; s > bestSum {
			bestSum, bestRx, bestTx = s, r, t
		}
	}
	if foundPhys {
		return physRx, physTx
	}
	return bestRx, bestTx
}

func parseU(v string) uint64 {
	n, _ := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
	return n
}

func parseF(v string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f
}

func clampInt(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampU(v uint64) uint64 {
	if v > 1<<60 {
		return 1 << 60
	}
	return v
}

func clampF(v, lo, hi float64) float64 {
	if v != v { // NaN
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func cleanStr(v string, max int) string {
	v = strings.TrimSpace(v)
	v = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, v)
	if len(v) > max {
		v = v[:max]
	}
	return v
}

// ---------- SSH 执行（通用通道已抽至 internal/sshx，此处仅采集封装） ----------

// SSHCred 解密后的连接凭据（内存态，不落盘）。sshx.Cred 的别名，
// 凭据解密/TOFU/错误分类等公共逻辑统一在 sshx 维护。
type SSHCred = sshx.Cred

// DialResult 一次连接+执行的结果。
type DialResult struct {
	Raw       *RawSample
	HostKeyFP string
	LatencyMs int64
}

// DialAndCollect 建连、校验 host key、执行采集脚本并解析。
// storedFP 为空 = TOFU 记录；strict=true 时指纹不匹配直接拒绝。
func DialAndCollect(ctx context.Context, cred *SSHCred, storedFP string, strict bool) (*DialResult, error) {
	res, err := sshx.RunScript(ctx, cred, storedFP, strict, collectScript)
	if err != nil {
		return nil, err
	}
	raw, err := parseOutput([]byte(res.Out))
	if err != nil {
		return nil, err
	}
	return &DialResult{Raw: raw, HostKeyFP: res.HostKeyFP, LatencyMs: res.LatencyMs}, nil
}

// collectScript 经 stdin 喂给远端 sh -s 执行，不再拼装 sh -c 引号，
// 故脚本内允许单引号（grep/awk 的 '^cpu ' 等模式无需改写）。

// RunCollectScript 本地执行采集脚本（本机节点免 SSH：进程内直接 sh -s，
// 与远端节点完全同一脚本/解析/差分链路）。
func RunCollectScript(ctx context.Context) (*RawSample, error) {
	type execOut struct {
		out []byte
		err error
	}
	ech := make(chan execOut, 1)
	cmd := exec.CommandContext(ctx, "sh", "-s")
	// 本机节点：注入宿主路径（容器内 /proc/net、/sys/class/net 只见面板容器自身），
	// 远端节点不经此路径，脚本用默认 /proc 视图。
	cmd.Env = append(os.Environ(), localScriptEnv()...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		defer stdin.Close()
		script := collectScript
		if runtime.GOOS == "darwin" {
			script = collectScriptLocalDarwin
		}
		_, _ = io.WriteString(stdin, script)
	}()
	go func() {
		err := cmd.Wait()
		out := cmd.Stdout.(*bytes.Buffer).Bytes()
		if len(out) > maxOutputLen {
			out = out[:maxOutputLen]
		}
		ech <- execOut{out, err}
	}()
	var out []byte
	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("本机采集超时")
	case o := <-ech:
		if o.err != nil && len(o.out) == 0 {
			return nil, fmt.Errorf("本机采集执行失败：%v", o.err)
		}
		out = o.out
	}
	if runtime.GOOS == "darwin" {
		return parseOutputDarwin(out)
	}
	return parseOutput(out)
}

// parseOutputDarwin 解析 darwin 本机采集输出：mem/cpu/net 等由 Go 侧原生读取
//（脚本只负责 uptime/load/画像），因此不走 Linux 的 mem/cpu 完整性校验。
func parseOutputDarwin(out []byte) (*RawSample, error) {
	s, err := parseOutputLoose(out)
	if err != nil {
		return nil, err
	}
	// CPU/内存/网络：darwin 原生 sysctl/hoststat 补齐
	if total, used, ok := darwinMem(); ok {
		s.MemTotal, s.MemUsed = total, used
		s.setMemAvail(total - used)
		s.memAvailSet = true
	}
	s.CpuTotal, s.CpuIdle = darwinCpuTicks()
	s.CpuCores = int(darwinSysctlInt("hw.ncpu"))
	s.DiskTotal, s.DiskUsed = darwinDisk("/")
	s.NetRx, s.NetTx = darwinNetTotals()
	if s.CpuTotal == 0 || s.MemTotal == 0 {
		return nil, fmt.Errorf("本机采集不完整（darwin sysctl 缺失）")
	}
	return s, nil
}

// collectScriptLocalDarwin darwin 本机采集脚本：macOS 无 /proc，
// 采集 system_profiler/vm_stat/sysctl 等本机等价接口。
const collectScriptLocalDarwin = `echo bt_begin=1
echo bt_up_s=$(sysctl -n kern.boottime 2>/dev/null | grep -oE 'sec = [0-9]+' | grep -oE '[0-9]+' | head -1)
echo bt_load=$(uptime 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | tail -3 | tr '\n' ' ')
echo bt_cpu_n=$(sysctl -n hw.ncpu 2>/dev/null)
echo bt_hostname=$(hostname 2>/dev/null)
echo bt_os_name="macOS"
echo bt_os_ver=$(sw_vers -productVersion 2>/dev/null)
echo bt_kernel=$(uname -r 2>/dev/null)
echo bt_arch=$(uname -m 2>/dev/null)
echo bt_virt=物理机
echo bt_proc=$(ps -axo pid= 2>/dev/null | wc -l | tr -d ' ')
echo bt_end=1`
