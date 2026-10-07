package wg

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
)

// ---------- 预检探测 ----------

// probeScript 预检脚本（只读）：输出白名单 key=value。listenPort>0 时额外探测
// 该 UDP 端口占用；iface 非空且本网接口正监听该端口时不算冲突（重配/warm standby）。
func probeScript(listenPort int, iface string) string {
	portPart := "\necho pf_port_busy=0"
	if listenPort > 0 {
		port := strconv.Itoa(listenPort)
		// ss 列布局：State Recv-Q Send-Q Local-Address:Port Peer-Address:Port，
		// 端口在第 4 列（旧实现取 $5 = Peer-Address，恒为 0.0.0.0:* → 永远探不出占用）
		portPart = `
busy=0
if command -v ss >/dev/null 2>&1; then
  ss -uln 2>/dev/null | awk 'NR>1{print $4}' | grep -q ':` + port + `$' && busy=1
elif command -v netstat >/dev/null 2>&1; then
  netstat -uln 2>/dev/null | awk 'NR>1{print $4}' | grep -q ':` + port + `$' && busy=1
fi
if [ "$busy" = 1 ] && command -v wg >/dev/null 2>&1; then
  for i in ` + ifaceOrLoop(iface) + `; do
    [ "$(wg show "$i" listen-port 2>/dev/null)" = "` + port + `" ] && busy=0
  done
fi
echo pf_port_busy=$busy`
	}
	return `pf_busy=0
echo pf_os_id=$(grep '^ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo pf_os_ver=$(grep '^VERSION_ID=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')
echo pf_arch=$(uname -m 2>/dev/null)
echo pf_kernel=$(uname -r 2>/dev/null)
echo pf_virt=$(systemd-detect-virt 2>/dev/null || echo unknown)
[ "$(id -u 2>/dev/null)" = 0 ] && pf_uid0=1 || pf_uid0=0
echo pf_uid0=$pf_uid0
pkg=""
command -v apt-get >/dev/null 2>&1 && pkg=apt
[ -z "$pkg" ] && command -v dnf >/dev/null 2>&1 && pkg=dnf
[ -z "$pkg" ] && command -v yum >/dev/null 2>&1 && pkg=yum
[ -z "$pkg" ] && command -v apk >/dev/null 2>&1 && pkg=apk
[ -z "$pkg" ] && command -v pacman >/dev/null 2>&1 && pkg=pacman
echo pf_pkg=$pkg
command -v wg >/dev/null 2>&1 && pf_wg=1 || pf_wg=0
echo pf_wg=$pf_wg
command -v wg-quick >/dev/null 2>&1 && pf_wgq=1 || pf_wgq=0
echo pf_wgq=$pf_wgq
if modinfo wireguard >/dev/null 2>&1 || [ -d /sys/module/wireguard ]; then pf_mod=1; else pf_mod=0; fi
echo pf_mod=$pf_mod
echo pf_ifaces=$(wg show interfaces 2>/dev/null | tr '\n' ' ')
wgcidr=""
for _i in $(wg show interfaces 2>/dev/null); do
  for _a in $(ip -4 -o addr show dev "$_i" 2>/dev/null | awk '{print $4}'); do
    wgcidr="$wgcidr$_i:$_a,"
  done
done
echo pf_wg_cidrs=$wgcidr
ufw status 2>/dev/null | head -n1 | grep -qi active && pf_ufw=1 || pf_ufw=0
echo pf_ufw=$pf_ufw
command -v systemctl >/dev/null 2>&1 && pf_sd=1 || pf_sd=0
echo pf_systemd=$pf_sd
if command -v iptables >/dev/null 2>&1 || command -v nft >/dev/null 2>&1; then pf_fw=1; else pf_fw=0; fi
echo pf_fw=$pf_fw` + portPart + `
echo pf_end=1`
}

// ifaceOrLoop 生成待排查的 wg 接口名列表：给了接口名就只查它，否则遍历全部
// （避免「另一条网卡占着同一端口」被误判成本接口占用）。
func ifaceOrLoop(iface string) string {
	if strings.TrimSpace(iface) == "" {
		return "$(wg show interfaces 2>/dev/null)"
	}
	return "'" + sanitizeIface(iface) + "'"
}

// parseProbe 解析预检输出（白名单键，宽松：缺失字段取零值）。
func parseProbe(out string) *Probe {
	p := &Probe{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "pf_os_id":
			p.OsID = v
		case "pf_os_ver":
			p.OsVer = v
		case "pf_arch":
			p.Arch = v
		case "pf_kernel":
			p.Kernel = v
		case "pf_virt":
			p.Virt = v
		case "pf_uid0":
			p.UID0 = v == "1"
		case "pf_pkg":
			p.PkgManager = v
		case "pf_wg":
			p.HasWg = v == "1"
		case "pf_wgq":
			p.HasWgQuick = v == "1"
		case "pf_mod":
			p.KernelModule = v == "1"
		case "pf_ifaces":
			p.WgIfaces = strings.Fields(v)
		case "pf_wg_cidrs":
			for _, it := range strings.Split(v, ",") {
				if it = strings.TrimSpace(it); it != "" {
					p.WgCIDRs = append(p.WgCIDRs, it)
				}
			}
		case "pf_ufw":
			p.UfwActive = v == "1"
		case "pf_systemd":
			p.Systemd = v == "1"
		case "pf_fw": // iptables 或 nft 至少有一个
			p.HasFirewall = v == "1"
		case "pf_port_busy":
			p.ListenPortBusy = v == "1"
		}
	}
	return p
}

// ProbeNode 对已建连节点执行预检探测。listenPort>0 时探测端口占用，
// iface 为该节点上属于本网的接口名（正监听该端口时不判为冲突）。
func ProbeNode(ctx context.Context, conn *sshx.Conn, listenPort int, iface string) (*Probe, error) {
	out, err := conn.Run(ctx, probeScript(listenPort, iface))
	if err != nil {
		return nil, err
	}
	return parseProbe(out), nil
}

// ---------- 安装 ----------

// installScript 按包管理器生成安装脚本（幂等：已装则零开销）。
func installScript(pkg string) string {
	switch pkg {
	case "apt":
		return `export DEBIAN_FRONTEND=noninteractive
apt-get update -qq >/dev/null 2>&1 || true
apt-get install -y -qq wireguard-tools >/dev/null 2>&1 || apt-get install -y wireguard-tools`
	case "dnf":
		return `dnf install -y wireguard-tools || yum install -y wireguard-tools`
	case "yum":
		return `yum install -y epel-release >/dev/null 2>&1 || true
yum install -y wireguard-tools`
	case "apk":
		return `apk add --no-cache wireguard-tools-wg wireguard-tools-wg-quick 2>/dev/null || apk add --no-cache wireguard-tools`
	case "pacman":
		return `pacman -Sy --noconfirm wireguard-tools`
	default:
		return `echo unsupported-pkg-manager; exit 1`
	}
}

// firewallInstallScript 补装 iptables（hub/standby 的 conf PostUp 要用，极简系统常缺）。
// 尽力而为：失败不阻断（宿主机本身极可能已装；容器/LXC 里装不上时 PostUp 会失败，
// 但那是该环境不允许 iptables 的硬限制，给用户留出「手动装/换节点」的空间）。
// apt 分支必须先 update：极简镜像常删掉 /var/lib/apt/lists（面板用的 ubuntu 基础镜像即如此），
// 没有索引时 apt-get install 会直接失败——实测踩过，补装静默不生效。
func firewallInstallScript(pkg string) string {
	switch pkg {
	case "apt":
		return `export DEBIAN_FRONTEND=noninteractive
apt-get update -qq >/dev/null 2>&1 || true
apt-get install -y -qq iptables >/dev/null 2>&1 || apt-get install -y iptables >/dev/null 2>&1 || true`
	case "dnf":
		return `dnf install -y iptables >/dev/null 2>&1 || true`
	case "yum":
		return `yum install -y iptables >/dev/null 2>&1 || true`
	case "apk":
		return `apk add --no-cache iptables >/dev/null 2>&1 || true`
	case "pacman":
		return `pacman -Sy --noconfirm iptables >/dev/null 2>&1 || true`
	default:
		return `true`
	}
}

// InstallWireGuard 安装 wireguard-tools（依据预检的包管理器），安装后复核 wg 可用。
// 返回是否实际执行了安装。needFirewall=true 且节点缺 iptables/nft 时补装
// （只有 hub/standby 的 conf 带 iptables PostUp；spoke 不需要，避免无谓装包）。
func InstallWireGuard(ctx context.Context, conn *sshx.Conn, p *Probe, needFirewall bool) (bool, error) {
	installed := false
	if !(p.HasWg && p.HasWgQuick) {
		if p.PkgManager == "" {
			return false, fmt.Errorf("无法识别包管理器，请手动安装 wireguard-tools")
		}
		if _, err := conn.Run(ctx, installScript(p.PkgManager)); err != nil {
			return false, fmt.Errorf("安装 wireguard-tools 失败: %w", err)
		}
		out, err := conn.Run(ctx, `command -v wg >/dev/null 2>&1 && command -v wg-quick >/dev/null 2>&1 && echo ok || echo missing`)
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(out) != "ok" {
			return true, fmt.Errorf("安装后 wg 仍不可用（内核过旧或源缺失）")
		}
		installed = true
	}
	if needFirewall && !p.HasFirewall && p.PkgManager != "" {
		if _, err := conn.Run(ctx, firewallInstallScript(p.PkgManager)); err != nil {
			log.Printf("[wg] 补装 iptables 失败（%s）：%v；conf 的 PostUp 已容错，节点仍可入网", p.PkgManager, err)
		}
		installed = true
	}
	return installed, nil
}

// EnsureForward 开启并持久化 IPv4 转发（hub/standby 需要）。
func EnsureForward(ctx context.Context, conn *sshx.Conn) error {
	if err := conn.PushFile(ctx, "/etc/sysctl.d/99-bt-wg-mesh.conf", []byte("net.ipv4.ip_forward = 1\n")); err != nil {
		return err
	}
	_, err := conn.Run(ctx, `sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || echo 1 > /proc/sys/net/ipv4/ip_forward`)
	return err
}

// EnsureUFWAllow ufw 启用时放行 UDP 端口（未启用则跳过）。
func EnsureUFWAllow(ctx context.Context, conn *sshx.Conn, port int) error {
	script := `if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | head -n1 | grep -qi active; then
  ufw allow ` + strconv.Itoa(port) + `/udp >/dev/null 2>&1 && echo fw=allowed || echo fw=failed
else
  echo fw=skip
fi`
	out, err := conn.Run(ctx, script)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(out) {
	case "fw=failed":
		return fmt.Errorf("ufw 放行 UDP %d 失败，请手动检查", port)
	default:
		return nil
	}
}

// BackupConf 修改前备份现有 conf（沿用运维习惯：时间戳后缀，已存在才备份）。
func BackupConf(ctx context.Context, conn *sshx.Conn, iface string) error {
	iface = sanitizeIface(iface)
	_, err := conn.Run(ctx, `[ -f /etc/wireguard/`+iface+`.conf ] && cp /etc/wireguard/`+iface+
		`.conf /etc/wireguard/`+iface+`.conf.bak.$(date +%Y%m%d%H%M%S) || true`)
	return err
}

// sanitizeComment 清洗 conf 注释/成员名：剥离控制字符（换行可注入 [Peer] 段）。
func sanitizeComment(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
}

func sanitizeIface(name string) string { // 接口名仅允许字母数字与下划线/短横线（拼进 shell 用，须白名单）
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "wg0"
	}
	return out
}

// WriteConf 渲染内容写入 /etc/wireguard/<iface>.conf（600）。
func WriteConf(ctx context.Context, conn *sshx.Conn, iface, content string) error {
	iface = sanitizeIface(iface)
	return conn.PushFile(ctx, "/etc/wireguard/"+iface+".conf", []byte(content))
}

// BringUp 启用并保持接口（systemd 可用则设开机自启，否则直接 wg-quick up）。
// 已在运行的接口会重启以加载新配置（组网初始化场景可接受）。
func BringUp(ctx context.Context, conn *sshx.Conn, iface string, hasSystemd bool) error {
	iface = sanitizeIface(iface)
	var script string
	if hasSystemd {
		script = `systemctl enable wg-quick@` + iface + ` >/dev/null 2>&1 || true
systemctl restart wg-quick@` + iface + ` 2>/dev/null || { wg-quick down ` + iface + ` >/dev/null 2>&1; wg-quick up ` + iface + `; }`
	} else {
		script = `wg-quick down ` + iface + ` >/dev/null 2>&1; wg-quick up ` + iface
	}
	_, err := conn.Run(ctx, script)
	return err
}

// BringDown 停用接口并取消自启（下线节点用；conf 保留为 .conf.removed 由调用方处理）。
func BringDown(ctx context.Context, conn *sshx.Conn, iface string) error {
	iface = sanitizeIface(iface)
	_, err := conn.Run(ctx, `wg-quick down `+iface+` >/dev/null 2>&1; systemctl disable wg-quick@`+
		iface+` >/dev/null 2>&1 || true; [ -f /etc/wireguard/`+iface+`.conf ] && mv /etc/wireguard/`+
		iface+`.conf /etc/wireguard/`+iface+`.conf.removed.$(date +%Y%m%d%H%M%S) || true`)
	return err
}

// ShowDump 执行 wg show all dump 并解析（接口不存在时返回 nil 状态而非错误）。
func ShowDump(ctx context.Context, conn *sshx.Conn) ([]DevState, error) {
	out, err := conn.Run(ctx, `wg show all dump 2>/dev/null || true`)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return ParseDumpAll([]byte(out))
}

// HubAddPeer hub 热加 peer：wg set 立即生效（不断开存量隧道）+ conf 追加 [Peer] 段
// （SaveConfig=false，重启后仍生效）。psk 为空则不设预共享密钥。
func HubAddPeer(ctx context.Context, conn *sshx.Conn, iface string, peer Peer, ip string) error {
	iface = sanitizeIface(iface)
	if !ValidKey(peer.PublicKey) {
		return fmt.Errorf("peer 公钥无效")
	}
	if !ValidCIDR(ip) {
		return fmt.Errorf("peer AllowedIPs 无效: %q", ip)
	}
	confPath := "/etc/wireguard/" + iface + ".conf"
	// 1) 运行态：wg set（PSK 经远端 mktemp 临时文件传递，不出现在 ps/命令历史里；
	// 固定路径（旧实现 /tmp/.bt_wg_psk）可被多用户节点上的本地攻击者预放 symlink，
	// 让 root 把 PSK 写进任意目标文件；mktemp + 0600 + trap 清理三重兜底）
	var setCmd strings.Builder
	if peer.PresharedKey != "" {
		// 脚本内联 PSK 会进 ps；改走 stdin：第一段 cat 把 PSK 写进 mktemp 文件
		//（sshx.Run 每次独立会话，PSK 数据经 stdin 送入第一段 cat）。
		script := "BT_PSK=$(mktemp /tmp/bt-psk.XXXXXX) || exit 1\n" +
			"trap 'rm -f \"$BT_PSK\"' EXIT INT TERM\n" +
			"cat > \"$BT_PSK\"\n" +
			"umask 077\n" +
			"wg set " + iface + " peer " + peer.PublicKey + " preshared-key \"$BT_PSK\"" +
			" allowed-ips " + ip
		if peer.PersistentKeepalive > 0 {
			script += " persistent-keepalive " + strconv.Itoa(peer.PersistentKeepalive)
		}
		script += "\nrc=$?\nrm -f \"$BT_PSK\"\nexit $rc\n"
		if err := conn.PushStdin(ctx, script, []byte(peer.PresharedKey+"\n")); err != nil {
			return err
		}
	} else {
		setCmd.WriteString("wg set " + iface + " peer " + peer.PublicKey)
		setCmd.WriteString(" allowed-ips " + ip)
		if peer.PersistentKeepalive > 0 {
			setCmd.WriteString(" persistent-keepalive " + strconv.Itoa(peer.PersistentKeepalive))
		}
		if _, err := conn.Run(ctx, setCmd.String()+"\nexit $?"); err != nil {
			return err
		}
	}
	// 2) conf 持久化：先查重，再以独立 exec 通道追加（cat >> 数据走 stdin）
	dup, err := conn.Run(ctx, `grep -qF '`+peer.PublicKey+`' `+confPath+` 2>/dev/null && echo dup=1 || echo dup=0`)
	if err != nil {
		return err
	}
	if strings.TrimSpace(dup) == "dup=1" {
		return nil
	}
	var block strings.Builder
	if comment := sanitizeComment(peer.Comment); comment != "" {
		block.WriteString("# " + comment + "\n")
	}
	block.WriteString("[Peer]\nPublicKey = " + peer.PublicKey + "\n")
	if peer.PresharedKey != "" {
		block.WriteString("PresharedKey = " + peer.PresharedKey + "\n")
	}
	block.WriteString("AllowedIPs = " + ip + "\n")
	if peer.PersistentKeepalive > 0 {
		block.WriteString("PersistentKeepalive = " + strconv.Itoa(peer.PersistentKeepalive) + "\n")
	}
	return conn.AppendFile(ctx, confPath, []byte(block.String()))
}

// HubRemovePeer hub 热删 peer（运行态立即移除；conf 由全量重写路径清理）。
func HubRemovePeer(ctx context.Context, conn *sshx.Conn, iface, publicKey string) error {
	iface = sanitizeIface(iface)
	if !ValidKey(publicKey) {
		return fmt.Errorf("公钥无效")
	}
	_, err := conn.Run(ctx, `wg set `+iface+` peer `+publicKey+` remove 2>/dev/null || true`)
	return err
}

// SyncConf 重载接口配置（不重启不断连）：strip → mktemp 临时文件 → wg syncconf。
// 收尾的 rm 不能吞掉退出码（旧实现末尾 rm 恒成功，strip/syncconf 失败被当成成功）。
// 旧固定路径 /tmp/.bt_sync.conf 继承 root 默认 umask（约 0644）且内容含全部 PSK，
// 改用 mktemp（0600）+ trap 清理。
func SyncConf(ctx context.Context, conn *sshx.Conn, iface string) error {
	iface = sanitizeIface(iface)
	_, err := conn.Run(ctx, `umask 077
BT_SYNC=$(mktemp /tmp/bt-sync.XXXXXX) || exit 1
trap 'rm -f "$BT_SYNC"' EXIT INT TERM
wg-quick strip `+iface+` > "$BT_SYNC" 2>/dev/null
rc=1
if [ -s "$BT_SYNC" ]; then
  wg syncconf `+iface+` "$BT_SYNC"
  rc=$?
fi
rm -f "$BT_SYNC"
exit $rc`)
	return err
}

// VerifySpoke 从 spoke 侧验证入网：ping hub 虚拟 IP（重试若干轮），随后以
// handshake 时间兜底判定（ICMP 被禁但 WG 隧道可用时 handshake 仍应新鲜）。
// hubPub 非空时只认该 hub 的握手——接口上还有别的 peer（旧 hub 残留）时，
// 取「全接口最大握手」会把一次失败的切换误判为成功。
func VerifySpoke(ctx context.Context, conn *sshx.Conn, hubIP string, iface string, hubPub string) (online bool, detail string, err error) {
	iface = sanitizeIface(iface)
	pingScript := `ok=0
for i in 1 2 3 4 5; do
  ping -n -c1 -W2 ` + hubIP + ` >/dev/null 2>&1 && ok=1 && break
  sleep 1
done
echo vf_ping=$ok`
	out, err := conn.Run(ctx, pingScript)
	if err != nil {
		return false, "", err
	}
	pingOK := strings.TrimSpace(out) == "vf_ping=1"
	devs, err := ShowDump(ctx, conn)
	if err != nil {
		return false, "", err
	}
	hsA := int64(0)
	for _, d := range devs {
		if d.Interface != iface {
			continue
		}
		for _, p := range d.Peers {
			if hubPub != "" && p.PublicKey != hubPub {
				continue
			}
			if p.LastHandshakeA > hsA {
				hsA = p.LastHandshakeA
			}
		}
	}
	switch {
	case pingOK:
		return true, fmt.Sprintf("ping %s 通，handshake=%d", hubIP, hsA), nil
	case hsA > 0:
		return true, fmt.Sprintf("ping 不通但 handshake=%d（ICMP 可能被禁）", hsA), nil
	default:
		if hubPub != "" {
			return false, fmt.Sprintf("ping %s 不通，且与目标 hub 无握手记录", hubIP), nil
		}
		return false, fmt.Sprintf("ping %s 不通且无握手记录", hubIP), nil
	}
}

// ---------- 配置渲染 ----------

// HubConfFile 生成 hub/standby 全量 conf（PostUp 幂等放行 FORWARD）。
// PostUp/PostDown 一律以 `|| true` 收尾：节点没有 iptables 时（极简镜像/容器）
// wg-quick 不能因为放行规则失败而拒绝拉起接口——成员间互通会因此降级（需 FORWARD 放行），
// 但 hub 本身仍可用，且预检已提示会自动补装。
func HubConfFile(priv string, hubIPCIDR string, listenPort, mtu int, peers []Peer) (string, error) {
	ifc := &Interface{
		Address:    []string{hubIPCIDR},
		PrivateKey: priv,
		ListenPort: listenPort,
		MTU:        mtu,
		PostUp: []string{
			"iptables -C FORWARD -i %i -j ACCEPT 2>/dev/null || iptables -A FORWARD -i %i -j ACCEPT || true",
			"iptables -C FORWARD -o %i -j ACCEPT 2>/dev/null || iptables -A FORWARD -o %i -j ACCEPT || true",
		},
		PostDown: []string{
			"iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true",
			"iptables -D FORWARD -o %i -j ACCEPT 2>/dev/null || true",
		},
		Peers: peers,
	}
	return Render(ifc)
}

// SpokeConfFile 生成 spoke/设备 conf：拨出 hub、仅路由组网子网、keepalive、可选 PSK。
func SpokeConfFile(priv, spokeIPCIDR, hubPub, hubEndpoint, subnetCIDR, psk string, keepalive, mtu int) (string, error) {
	ifc := &Interface{
		Address:    []string{spokeIPCIDR},
		PrivateKey: priv,
		MTU:        mtu,
		Peers: []Peer{{
			PublicKey:           hubPub,
			PresharedKey:        psk,
			AllowedIPs:          []string{subnetCIDR},
			Endpoint:            hubEndpoint,
			PersistentKeepalive: keepalive,
		}},
	}
	return Render(ifc)
}

// SubnetBits 导出子网前缀长度（handler 组装 CIDR 用）。
func SubnetBits(subnetCIDR string) (int, error) {
	return subnetBits(subnetCIDR)
}

// InstallAsset 将面板缓存的资产文件推送到节点 /usr/local/bin/<name>（0755）。
// 用于国内节点装不了包时的兜底分发（wireguard-go / 静态 wg 工具等）。
// assetNameRe 资产名白名单：拼进节点 root 的 shell 命令，必须严格限制。
var assetNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func InstallAsset(ctx context.Context, conn *sshx.Conn, name, localPath string) error {
	// name 会以 root 身份拼进 chmod/command -v：严格白名单（与 handler 入库校验一致，双保险）
	if !assetNameRe.MatchString(name) {
		return fmt.Errorf("资产名含非法字符: %q", name)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	if err := conn.PushFile(ctx, "/usr/local/bin/"+name, data); err != nil {
		return err
	}
	_, err = conn.Run(ctx, "chmod 0755 /usr/local/bin/"+name+" && command -v "+name+" >/dev/null 2>&1 && echo installed || echo placed")
	return err
}
