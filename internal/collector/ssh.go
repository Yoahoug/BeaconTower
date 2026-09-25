package collector

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// FingerprintSHA256 计算 host key 指纹（SHA256:...，TOFU 展示用）。
func FingerprintSHA256(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// SSHCred 解密后的连接凭据（内存态，不落盘）。
type SSHCred struct {
	Host       string
	Port       int
	Username   string
	AuthType   string // password | key
	Password   string
	PrivateKey string
	Passphrase string
}

// collectScript 单次 exec 采集脚本（doc/02 §4.2 + doc/09 §2.2）。
// 全部只读：/proc、df、uname、os-release、sysfs 功率/温度接口、出口 IP 回显。
// 输出多行 key=value，面板侧按白名单键解析。
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
echo bt_net=$(awk '/:/{gsub(/:/," "); if($1!="lo"){rx+=$2;tx+=$10}} END{print rx+0":"tx+0}' /proc/net/dev 2>/dev/null)
echo bt_tcp=$(awk 'END{print NR-1+0}' /proc/net/tcp 2>/dev/null)
echo bt_tcp6=$(awk 'END{print NR-1+0}' /proc/net/tcp6 2>/dev/null)
echo bt_udp=$(awk 'END{print NR-1+0}' /proc/net/udp 2>/dev/null)
echo bt_udp6=$(awk 'END{print NR-1+0}' /proc/net/udp6 2>/dev/null)
echo bt_proc=$(ls /proc 2>/dev/null | grep -c '^[0-9]')
echo bt_os_name=$(grep '^NAME=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo bt_hostname=$(hostname 2>/dev/null || uname -n 2>/dev/null || true)
echo bt_os_id=$(grep '^ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo bt_os_ver=$(grep '^VERSION_ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo bt_kernel=$(uname -r 2>/dev/null)
echo bt_arch=$(uname -m 2>/dev/null)
echo bt_cpu_model=$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2-)
echo bt_virt=$(systemd-detect-virt 2>/dev/null || echo unknown)
echo bt_pub_ip=$(curl -sS -m 5 -4 https://ip.sb 2>/dev/null || curl -sS -m 5 -4 'http://ip-api.com/json/?fields=query' 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
for d in /sys/class/powercap/intel-rapl:*; do
  echo rapl:${d##*/}=$(cat $d/energy_uj 2>/dev/null)
  echo raplmax:${d##*/}=$(cat $d/max_energy_range_uj 2>/dev/null)
done
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

// ---------- SSH 执行 ----------

// authMethods 组装认证方式。
func authMethods(c *SSHCred) ([]ssh.AuthMethod, error) {
	switch c.AuthType {
	case "password":
		if c.Password == "" {
			return nil, fmt.Errorf("认证失败：密码为空")
		}
		return []ssh.AuthMethod{ssh.Password(c.Password)}, nil
	case "key":
		if c.PrivateKey == "" {
			return nil, fmt.Errorf("认证失败：私钥为空")
		}
		var signer ssh.Signer
		var err error
		if c.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(c.PrivateKey), []byte(c.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(c.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("认证失败：私钥无法解析")
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("参数错误：未知认证方式 %q", c.AuthType)
	}
}

// DialResult 一次连接+执行的结果。
type DialResult struct {
	Raw       *RawSample
	HostKeyFP string
	LatencyMs int64
}

// DialAndCollect 建连、校验 host key、执行采集脚本并解析。
// storedFP 为空 = TOFU 记录；strict=true 时指纹不匹配直接拒绝。
func DialAndCollect(ctx context.Context, cred *SSHCred, storedFP string, strict bool) (*DialResult, error) {
	methods, err := authMethods(cred)
	if err != nil {
		return nil, classifyErr(err)
	}
	var presented ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User:    cred.Username,
		Auth:    methods,
		Timeout: 8 * time.Second,
		HostKeyCallback: func(host string, remote net.Addr, key ssh.PublicKey) error {
			presented = key
			fp := FingerprintSHA256(key)
			if storedFP == "" {
				return nil // TOFU：由上层持久化
			}
			if fp != storedFP {
				if strict {
					return fmt.Errorf("host key 指纹不匹配（严格模式，拒绝连接）")
				}
				return nil // 非严格：记录但放行（上层更新指纹并可告警）
			}
			return nil
		},
	}
	addr := net.JoinHostPort(cred.Host, strconv.Itoa(cred.Port))
	start := time.Now()
	// ssh.Dial 不接受 ctx：用 goroutine + ctx 取消兜底
	type dialOut struct {
		client *ssh.Client
		err    error
	}
	dch := make(chan dialOut, 1)
	go func() {
		cl, err := ssh.Dial("tcp", addr, cfg)
		dch <- dialOut{cl, err}
	}()
	var client *ssh.Client
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("连接超时：主机不可达（8s 超时）")
	case o := <-dch:
		if o.err != nil {
			return nil, classifyErr(o.err)
		}
		client = o.client
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return nil, classifyErr(err)
	}
	defer sess.Close()

	type execOut struct {
		out []byte
		err error
	}
	ech := make(chan execOut, 1)
	go func() {
		out, err := sess.Output("sh -c '" + collectScriptSingleQuoteSafe() + "'")
		if len(out) > maxOutputLen {
			out = out[:maxOutputLen]
		}
		ech <- execOut{out, err}
	}()
	var out []byte
	select {
	case <-ctx.Done():
		sess.Close()
		return nil, fmt.Errorf("连接超时：主机不可达（8s 超时）")
	case o := <-ech:
		if o.err != nil {
			return nil, classifyErr(o.err)
		}
		out = o.out
	}
	raw, err := parseOutput(out)
	if err != nil {
		return nil, err
	}
	fp := ""
	if presented != nil {
		fp = FingerprintSHA256(presented)
	}
	return &DialResult{Raw: raw, HostKeyFP: fp, LatencyMs: time.Since(start).Milliseconds()}, nil
}

// collectScriptSingleQuoteSafe 脚本内无单引号（awk 程序均用双引号），可直接包入 sh -c '...'。
func collectScriptSingleQuoteSafe() string {
	if strings.Contains(collectScript, "'") {
		panic("collectScript must not contain single quotes")
	}
	return collectScript
}

// classifyErr SSH 错误分类（doc/04 错误码 2001 的 msg 分类）。
func classifyErr(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unable to authenticate"),
		strings.Contains(msg, "Authentication failed"),
		strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "no supported methods"),
		strings.Contains(msg, "认证失败"):
		return fmt.Errorf("认证失败：%s", trimErr(msg))
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("连接被拒绝：目标端口未监听 SSH")
	case strings.Contains(msg, "no route"),
		strings.Contains(msg, "unreachable"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "不可达"):
		return fmt.Errorf("连接超时：主机不可达（8s 超时）")
	case strings.Contains(msg, "指纹不匹配"):
		return err
	default:
		if len(msg) > 160 {
			msg = msg[:160]
		}
		return fmt.Errorf("SSH 连接失败：%s", msg)
	}
}

func trimErr(msg string) string {
	// 去掉 Go ssh 库的前缀噪音，保留关键原因
	if i := strings.Index(msg, "unable to authenticate"); i >= 0 {
		return "用户名或密码/密钥错误"
	}
	if strings.Contains(msg, "permission denied") {
		return "Permission denied（用户名或密码/密钥错误）"
	}
	if len(msg) > 160 {
		return msg[:160]
	}
	return msg
}
