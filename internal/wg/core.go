// Package wg WG 组网模块核心：密钥生成、wg-quick 配置渲染/解析、
// `wg show dump` 状态解析、子网 IP 分配、预检判定（doc/12）。
// 纯函数集合，不触碰网络与磁盘，全部可单测。
package wg

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/curve25519"
)

// ---------- 密钥 ----------

// KeyPair 一对 WireGuard 密钥（base64 标准编码）。
type KeyPair struct {
	Private string
	Public  string
}

// GenerateKeyPair 生成 curve25519 密钥对（私钥已按 RFC 7748 clamping）。
func GenerateKeyPair() (KeyPair, error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return KeyPair{}, fmt.Errorf("生成随机数失败: %w", err)
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return KeyPair{}, fmt.Errorf("推导公钥失败: %w", err)
	}
	return KeyPair{
		Private: base64.StdEncoding.EncodeToString(priv),
		Public:  base64.StdEncoding.EncodeToString(pub),
	}, nil
}

// GeneratePSK 生成 256-bit 预共享密钥（防量子预计算的额外加密层）。
func GeneratePSK() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// PublicKeyFromPrivate 由 base64 私钥推导公钥（导入存量 conf 时校验用）。
func PublicKeyFromPrivate(privB64 string) (string, error) {
	priv, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privB64))
	if err != nil || len(priv) != 32 {
		return "", errors.New("私钥格式无效（应为 44 字符 base64）")
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("推导公钥失败: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

// ValidKey 校验 base64 编码的 256-bit 密钥格式。
func ValidKey(s string) bool {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	return err == nil && len(b) == 32
}

// ---------- wg-quick 配置 ----------

// Peer 单个 [Peer] 段。
type Peer struct {
	Comment             string   // 段前注释行（如 "# Win PC"）
	PublicKey           string   // base64，必填
	PresharedKey        string   // base64，可空
	AllowedIPs          []string // CIDR 列表，必填
	Endpoint            string   // host:port，可空
	PersistentKeepalive int      // 秒；0 = 不写
}

// Interface 单个接口配置（对应一个 wgX.conf）。
type Interface struct {
	Name       string   // wg0
	Address    []string // CIDR 列表
	PrivateKey string
	ListenPort int // 0 = 不写
	MTU        int // 0 = 不写（用内核默认）
	DNS        []string
	Table      string // 空 = auto
	SaveConfig bool
	PreUp      []string
	PostUp     []string
	PreDown    []string
	PostDown   []string
	Peers      []Peer
}

// Render 渲染为 wg-quick conf 文本（[Interface] 在前，[Peer] 依序跟随）。
func Render(ifc *Interface) (string, error) {
	if ifc == nil {
		return "", errors.New("接口配置为空")
	}
	if !ValidKey(ifc.PrivateKey) {
		return "", errors.New("私钥无效")
	}
	if len(ifc.Address) == 0 {
		return "", errors.New("Address 为空")
	}
	for _, cidr := range ifc.Address {
		if !ValidCIDR(cidr) {
			return "", fmt.Errorf("Address 无效: %q", cidr)
		}
	}
	if ifc.ListenPort < 0 || ifc.ListenPort > 65535 {
		return "", fmt.Errorf("ListenPort 无效: %d", ifc.ListenPort)
	}
	var b strings.Builder
	b.WriteString("[Interface]\n")
	b.WriteString("Address = " + strings.Join(ifc.Address, ", ") + "\n")
	if ifc.ListenPort > 0 {
		b.WriteString("ListenPort = " + strconv.Itoa(ifc.ListenPort) + "\n")
	}
	b.WriteString("PrivateKey = " + strings.TrimSpace(ifc.PrivateKey) + "\n")
	if ifc.MTU > 0 {
		b.WriteString("MTU = " + strconv.Itoa(ifc.MTU) + "\n")
	}
	if len(ifc.DNS) > 0 {
		b.WriteString("DNS = " + strings.Join(ifc.DNS, ", ") + "\n")
	}
	if ifc.Table != "" {
		b.WriteString("Table = " + ifc.Table + "\n")
	}
	if ifc.SaveConfig {
		b.WriteString("SaveConfig = true\n")
	}
	for _, v := range ifc.PreUp {
		b.WriteString("PreUp = " + v + "\n")
	}
	for _, v := range ifc.PostUp {
		b.WriteString("PostUp = " + v + "\n")
	}
	for _, v := range ifc.PreDown {
		b.WriteString("PreDown = " + v + "\n")
	}
	for _, v := range ifc.PostDown {
		b.WriteString("PostDown = " + v + "\n")
	}
	for i := range ifc.Peers {
		p := &ifc.Peers[i]
		if !ValidKey(p.PublicKey) {
			return "", fmt.Errorf("Peer#%d 公钥无效", i)
		}
		if len(p.AllowedIPs) == 0 {
			return "", fmt.Errorf("Peer#%d AllowedIPs 为空", i)
		}
		for _, cidr := range p.AllowedIPs {
			if !ValidCIDR(cidr) {
				return "", fmt.Errorf("Peer#%d AllowedIPs 无效: %q", i, cidr)
			}
		}
		if p.PresharedKey != "" && !ValidKey(p.PresharedKey) {
			return "", fmt.Errorf("Peer#%d PresharedKey 无效", i)
		}
		b.WriteString("\n")
		if p.Comment != "" {
			b.WriteString("# " + strings.ReplaceAll(p.Comment, "\n", " ") + "\n")
		}
		b.WriteString("[Peer]\n")
		b.WriteString("PublicKey = " + strings.TrimSpace(p.PublicKey) + "\n")
		if p.PresharedKey != "" {
			b.WriteString("PresharedKey = " + strings.TrimSpace(p.PresharedKey) + "\n")
		}
		b.WriteString("AllowedIPs = " + strings.Join(p.AllowedIPs, ", ") + "\n")
		if p.Endpoint != "" {
			b.WriteString("Endpoint = " + p.Endpoint + "\n")
		}
		if p.PersistentKeepalive > 0 {
			b.WriteString("PersistentKeepalive = " + strconv.Itoa(p.PersistentKeepalive) + "\n")
		}
	}
	return b.String(), nil
}

// ParseConf 解析 wg-quick conf（宽松：未知键忽略，兼容多行/逗号列表）。
// [Peer] 段的前导注释行（如 "# Win PC"）归属其后的 peer。
// 私钥/PSK 照原样返回，调用方按需加密存储。
func ParseConf(data []byte) (*Interface, error) {
	ifc := &Interface{}
	section := ""
	pendingComment := ""
	var cur *Peer
	sec := func(name string) {
		section = strings.ToLower(name)
		if section == "peer" {
			ifc.Peers = append(ifc.Peers, Peer{Comment: pendingComment})
			cur = &ifc.Peers[len(ifc.Peers)-1]
		}
		pendingComment = ""
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if c := strings.TrimSpace(strings.TrimPrefix(line, "#")); c != "" {
				switch {
				case section == "peer" && cur != nil && cur.Comment == "" && cur.PublicKey == "":
					cur.Comment = c // [Peer] 行之后的注释
				case pendingComment == "":
					pendingComment = c // [Peer] 行之前的注释，归属下一个 peer
				}
			}
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec(strings.Trim(line, "[]"))
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if v == "" || v == "(none)" {
			continue
		}
		splitList := func(s string) []string {
			var out []string
			for _, x := range strings.Split(s, ",") {
				if x = strings.TrimSpace(x); x != "" {
					out = append(out, x)
				}
			}
			return out
		}
		switch section {
		case "interface":
			switch strings.ToLower(k) {
			case "address":
				ifc.Address = append(ifc.Address, splitList(v)...)
			case "privatekey":
				ifc.PrivateKey = v
			case "listenport":
				ifc.ListenPort, _ = strconv.Atoi(v)
			case "mtu":
				ifc.MTU, _ = strconv.Atoi(v)
			case "dns":
				ifc.DNS = append(ifc.DNS, splitList(v)...)
			case "table":
				ifc.Table = v
			case "saveconfig":
				ifc.SaveConfig = strings.EqualFold(v, "true")
			case "preup":
				ifc.PreUp = append(ifc.PreUp, v)
			case "postup":
				ifc.PostUp = append(ifc.PostUp, v)
			case "predown":
				ifc.PreDown = append(ifc.PreDown, v)
			case "postdown":
				ifc.PostDown = append(ifc.PostDown, v)
			}
		case "peer":
			switch strings.ToLower(k) {
			case "publickey":
				cur.PublicKey = v
			case "presharedkey":
				cur.PresharedKey = v
			case "allowedips":
				cur.AllowedIPs = append(cur.AllowedIPs, splitList(v)...)
			case "endpoint":
				cur.Endpoint = v
			case "persistentkeepalive":
				cur.PersistentKeepalive, _ = strconv.Atoi(v)
			}
		}
	}
	if ifc.PrivateKey == "" {
		return nil, errors.New("conf 缺少 PrivateKey")
	}
	return ifc, nil
}

// ---------- `wg show all dump` 状态 ----------

// PeerState 单 peer 运行态。
type PeerState struct {
	Interface      string
	PublicKey      string
	HasPSK         bool
	Endpoint       string
	AllowedIPs     []string
	LastHandshake  time.Time // 零值 = 从未握手
	LastHandshakeA int64     // unix 秒（原始值，0 = 从未）
	RX, TX         uint64
	Keepalive      int // 0 = off
}

// DevState 单接口运行态。
type DevState struct {
	Interface  string
	PublicKey  string
	ListenPort int
	Peers      []PeerState
}

// ParseDumpAll 解析 `wg show all dump` 输出（TSV；接口行 5 列、peer 行 9 列，
// 均带接口名前缀；`wg show <if> dump` 则为 4/8 列）。
func ParseDumpAll(data []byte) ([]DevState, error) {
	var devs []DevState
	idx := map[string]int{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		switch len(f) {
		case 5, 4: // 接口行
			off := 0
			name := "wg0"
			if len(f) == 5 {
				off, name = 1, f[0]
			}
			port, _ := strconv.Atoi(f[off+2])
			idx[name] = len(devs)
			devs = append(devs, DevState{Interface: name, PublicKey: non(f[off+1]), ListenPort: port})
		case 9, 8: // peer 行
			off := 0
			name := "wg0"
			if len(f) == 9 {
				off, name = 1, f[0]
			}
			if len(devs) == 0 {
				return nil, errors.New("dump 格式错误：peer 行先于接口行")
			}
			d := &devs[idx[name]]
			hs, _ := strconv.ParseInt(f[off+4], 10, 64)
			rx, _ := strconv.ParseUint(f[off+5], 10, 64)
			tx, _ := strconv.ParseUint(f[off+6], 10, 64)
			ka, _ := strconv.Atoi(f[off+7])
			p := PeerState{
				Interface:      name,
				PublicKey:      non(f[off]),
				HasPSK:         f[off+1] != "(none)" && f[off+1] != "",
				Endpoint:       non(f[off+2]),
				AllowedIPs:     strings.Split(f[off+3], ","),
				LastHandshakeA: hs,
				RX:             rx,
				TX:             tx,
				Keepalive:      ka,
			}
			if hs > 0 {
				p.LastHandshake = time.Unix(hs, 0)
			}
			d.Peers = append(d.Peers, p)
		default:
			// 未知行：跳过（版本差异容忍）
		}
	}
	if len(devs) == 0 {
		return nil, errors.New("dump 输出为空或格式不识别")
	}
	return devs, nil
}

// OnlineByHandshake 握手判活：keepalive 25s 下活体 peer 最迟约 3 分钟内必有新握手。
func OnlineByHandshake(last time.Time, now time.Time) bool {
	if last.IsZero() {
		return false
	}
	return now.Sub(last) <= 3*time.Minute
}

func non(s string) string {
	if s == "(none)" {
		return ""
	}
	return s
}

// ---------- IP 分配 ----------

// Allocator 子网内 IP 分配器（单网内唯一，配合 DB 唯一约束防并发冲突）。
type Allocator struct {
	ipnet *net.IPNet
	taken map[string]bool
}

// NewAllocator 创建分配器；taken 为已占用 IP 列表（裸 IP 或 /32 CIDR 均可）。
func NewAllocator(subnetCIDR string, taken []string) (*Allocator, error) {
	_, ipnet, err := net.ParseCIDR(strings.TrimSpace(subnetCIDR))
	if err != nil {
		return nil, fmt.Errorf("子网格式无效: %q", subnetCIDR)
	}
	a := &Allocator{ipnet: ipnet, taken: map[string]bool{}}
	for _, t := range taken {
		t = strings.TrimSpace(t)
		if strings.Contains(t, "/") {
			if _, n, err := net.ParseCIDR(t); err == nil {
				// /32 CIDR 按单 IP 处理；更宽 CIDR 标记网络地址位（保守近似）
				a.taken[n.IP.String()] = true
				continue
			}
		}
		if ip := net.ParseIP(t); ip != nil {
			a.taken[ip.String()] = true
		}
	}
	return a, nil
}

// InSubnet 判断 IP 是否在子网内。
func (a *Allocator) InSubnet(ip string) bool {
	p := net.ParseIP(strings.TrimSpace(ip))
	return p != nil && a.ipnet.Contains(p)
}

// Take 占用指定 IP（须在子网内且未被占用）。
func (a *Allocator) Take(ip string) error {
	ip = strings.TrimSpace(ip)
	p := net.ParseIP(ip)
	if p == nil {
		return fmt.Errorf("IP 格式无效: %q", ip)
	}
	if !a.ipnet.Contains(p) {
		return fmt.Errorf("IP %s 不在子网 %s 内", ip, a.ipnet.String())
	}
	key := p.String()
	if a.taken[key] {
		return fmt.Errorf("IP %s 已被占用", ip)
	}
	a.taken[key] = true
	return nil
}

// IsFree 查询 IP 是否可用。
func (a *Allocator) IsFree(ip string) bool {
	p := net.ParseIP(strings.TrimSpace(ip))
	return p != nil && a.ipnet.Contains(p) && !a.taken[p.String()]
}

// Next 返回下一个空闲主机地址（跳过网络地址与广播地址）。
func (a *Allocator) Next() (string, error) {
	base := a.ipnet.IP.To4()
	if base == nil {
		return "", errors.New("仅支持 IPv4 子网")
	}
	ones, bits := a.ipnet.Mask.Size()
	if bits != 32 {
		return "", errors.New("仅支持 IPv4 子网")
	}
	// 按整型递增（旧实现只改最后一个字节 ip[3] |= byte(i)，/23 及更宽的
	// 子网里高位永不进位，可分配地址被截断在 x.x.x.1-254 且会重复）
	baseV := binary.BigEndian.Uint32(base)
	total := uint32(1) << uint(bits-ones)
	if ones >= 31 {
		for i := uint32(0); i < total; i++ {
			if ip := uint32ToIP(baseV + i); a.IsFree(ip) {
				return ip, nil
			}
		}
		return "", errors.New("子网已无空闲 IP")
	}
	for i := uint32(1); i < total-1; i++ { // 跳过网络地址与广播地址
		if ip := uint32ToIP(baseV + i); a.IsFree(ip) {
			return ip, nil
		}
	}
	return "", errors.New("子网已无空闲 IP")
}

func uint32ToIP(v uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return net.IP(b[:]).String()
}

// ValidCIDR 校验 CIDR 格式。
func ValidCIDR(s string) bool {
	_, _, err := net.ParseCIDR(strings.TrimSpace(s))
	return err == nil
}
