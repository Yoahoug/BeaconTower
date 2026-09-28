package wg

// ============================================================
// 面板侧 UDP 可达性实测（真握手探测）
//
// 动机：云厂商安全组是否放行 UDP 端口，节点自查（ss/ufw）看不出来，
// 也不能调云 API。WireGuard 只回应「已知成员」的握手，而面板自己就托管着
// 成员的私钥——于是面板可以按协议自造一个标准握手发起包（Noise IK, msg type 1）
// 发到目标 hub 的 ip:port：收到握手回应（type 2）= 端口真的通；超时 = 没放行。
//
// 参考 WireGuard 白皮书 §5.4：
//   CONSTRUCTION = "Noise_IKpsk2_25519_ChaChaPoly_BLAKE2s"
//   KDF_n(key, input) = HMAC-BLAKE2s 链（t1=HMAC(t0,0x1), t2=HMAC(t0,t1||0x2)…）
//   HASH = BLAKE2s-256；MAC = keyed BLAKE2s-128；AEAD = ChaCha20Poly1305（nonce=计数器小端）
//
// 副作用与边界（写进 doc/12 / doc/11）：
//   - 探测会以「某个面板托管成员」的身份握手，hub 会把该成员的端点临时更新为
//     面板出口地址，直到该成员自己下一次握手（keepalive ≤25s）自动纠正。
//     因此优先挑「离线设备」当探针；没有可用私钥（首次组网、尚无成员）时跳过探测。
//   - 时间戳必须严格递增（hub 有重放保护），这里用当前时间，每次探测新取。
// ============================================================

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

const (
	wgConstruction = "Noise_IKpsk2_25519_ChaChaPoly_BLAKE2s"
	wgIdentifier   = "WireGuard v1 zx2c4 Jason@zx2c4.com"
	wgLabelMAC1    = "mac1----"

	wgMsgInitiation = 1
	wgMsgResponse   = 2

	// 布局：type(4) + sender(4) + ephemeral(32) + static(48) + timestamp(28) + mac1(16) + mac2(16)
	wgInitMsgLen  = 148
	wgMac1Offset  = 116
	wgRespRecvOff = 8 // 回应包里的 receiver index（= 我们发出的 sender index）
	wgRespMsgLen  = 92
)

// b2s256 BLAKE2s-256。
func b2s256(parts ...[]byte) []byte {
	h, _ := blake2s.New256(nil)
	for _, p := range parts {
		_, _ = h.Write(p)
	}
	return h.Sum(nil)
}

// b2s128Keyed keyed BLAKE2s-128（WireGuard 的 MAC）。
func b2s128Keyed(key, in []byte) []byte {
	h, _ := blake2s.New128(key)
	_, _ = h.Write(in)
	return h.Sum(nil)
}

// hmacB2s HMAC-BLAKE2s-256。
func hmacB2s(key, in []byte) []byte {
	m := hmac.New(func() hash.Hash {
		h, _ := blake2s.New256(nil)
		return h
	}, key)
	_, _ = m.Write(in)
	return m.Sum(nil)
}

// kdf1/kdf2 WireGuard KDF 链。
func kdf1(key, input []byte) []byte {
	t0 := hmacB2s(key, input)
	return hmacB2s(t0, []byte{0x1})
}

func kdf2(key, input []byte) ([]byte, []byte) {
	t0 := hmacB2s(key, input)
	t1 := hmacB2s(t0, []byte{0x1})
	t2 := hmacB2s(t0, append(append([]byte{}, t1...), 0x2))
	return t1, t2
}

// tai64n 12 字节时间戳（8B 秒 + 4B 纳秒，大端；TAI64 基址沿用 wg 实现的偏移）。
func tai64n(now time.Time) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[:8], uint64(now.Unix())+0x400000000000000A)
	binary.BigEndian.PutUint32(b[8:], uint32(now.Nanosecond()))
	return b
}

// aeadSeal ChaCha20Poly1305 加密（nonce = 32 位计数器小端补零）。
func aeadSeal(key, plain, ad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	return aead.Seal(nil, nonce, plain, ad), nil
}

// CraftHandshakeInit 按 WireGuard Noise IK 构造握手发起包（msg type 1）。
// 返回完整 148 字节报文与 sender index（用于匹配回应）。
func CraftHandshakeInit(initiatorPriv, responderPub []byte) ([]byte, uint32, error) {
	if len(initiatorPriv) != 32 {
		return nil, 0, errors.New("发起方私钥长度须为 32 字节")
	}
	if len(responderPub) != 32 {
		return nil, 0, errors.New("响应方公钥长度须为 32 字节")
	}
	initiatorPub, err := curve25519.X25519(initiatorPriv, curve25519.Basepoint)
	if err != nil {
		return nil, 0, fmt.Errorf("推导发起方公钥失败: %w", err)
	}

	// 随机 ephemeral 私钥（同样做 clamping）
	ephPriv := make([]byte, 32)
	if _, err := rand.Read(ephPriv); err != nil {
		return nil, 0, err
	}
	ephPriv[0] &= 248
	ephPriv[31] &= 127
	ephPriv[31] |= 64
	ephPub, err := curve25519.X25519(ephPriv, curve25519.Basepoint)
	if err != nil {
		return nil, 0, err
	}

	var senderIdxBytes [4]byte
	if _, err := rand.Read(senderIdxBytes[:]); err != nil {
		return nil, 0, err
	}
	senderIdx := binary.LittleEndian.Uint32(senderIdxBytes[:])

	// Noise 状态推进
	ck := b2s256([]byte(wgConstruction))
	h := b2s256(ck, []byte(wgIdentifier))
	h = b2s256(h, responderPub)
	ck = kdf1(ck, ephPub)
	h = b2s256(h, ephPub)

	es, err := curve25519.X25519(ephPriv, responderPub)
	if err != nil {
		return nil, 0, err
	}
	ck, k := kdf2(ck, es) // Noise：chaining key 必须随每步推进，否则后续密钥全错
	encStatic, err := aeadSeal(k, initiatorPub, h)
	if err != nil {
		return nil, 0, err
	}
	h = b2s256(h, encStatic)

	ss, err := curve25519.X25519(initiatorPriv, responderPub)
	if err != nil {
		return nil, 0, err
	}
	ck, k = kdf2(ck, ss)
	encTS, err := aeadSeal(k, tai64n(time.Now()), h)
	if err != nil {
		return nil, 0, err
	}

	msg := make([]byte, wgInitMsgLen)
	binary.LittleEndian.PutUint32(msg[0:4], wgMsgInitiation)
	binary.LittleEndian.PutUint32(msg[4:8], senderIdx)
	copy(msg[8:40], ephPub)
	copy(msg[40:88], encStatic)
	copy(msg[88:116], encTS)
	// mac1 = keyed BLAKE2s-128(HASH(LABEL_MAC1 || responderPub), msg[:116])；mac2 留零
	mac1Key := b2s256([]byte(wgLabelMAC1), responderPub)
	copy(msg[wgMac1Offset:wgMac1Offset+16], b2s128Keyed(mac1Key, msg[:wgMac1Offset]))
	return msg, senderIdx, nil
}

// IsHandshakeResponse 判断收到的报文是否为针对 senderIndex 的握手回应（type 2）。
func IsHandshakeResponse(msg []byte, senderIndex uint32) bool {
	if len(msg) != wgRespMsgLen {
		return false
	}
	if binary.LittleEndian.Uint32(msg[0:4]) != wgMsgResponse {
		return false
	}
	return binary.LittleEndian.Uint32(msg[wgRespRecvOff:wgRespRecvOff+4]) == senderIndex
}

// ProbeUDPResult 探测结果。
type ProbeUDPResult struct {
	OK      bool
	RTT     time.Duration
	Target  string
	Attempt int
	// Refused = 收到 ICMP 端口不可达（写/读时报 ECONNREFUSED）：目标主机在线、路径通，
	// 但该端口确实没有监听——与「安全组静默丢包」是完全不同的结论。
	Refused bool
}

// ProbeUDP 向 endpoint（host:port）发握手发起包并等回应。
// 收到合法回应即视为 UDP 通路可达；连发 attempts 次仍未果则判定不通。
// timeout 是**整轮总预算**（不是每次尝试的等待上限）：三个握手包在预算内间隔发出、
// 统一等到预算耗尽才判失败——失败判定更快（调用方只等一个预算），抗丢包能力不变。
func ProbeUDP(ctx context.Context, endpoint string, responderPub, initiatorPriv []byte, timeout time.Duration) (ProbeUDPResult, error) {
	res := ProbeUDPResult{Target: endpoint}
	raddr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return res, fmt.Errorf("端点无法解析: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return res, fmt.Errorf("UDP 建连失败: %w", err)
	}
	defer conn.Close()

	const attempts = 3
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	// 三个包均匀铺在预算前半段（首包立即发，成功路径仍是几十毫秒返回）
	gap := timeout / time.Duration(2*attempts)
	idxes := make(map[uint32]bool, attempts)
	start := time.Now()
	for i := 1; i <= attempts; i++ {
		if w := time.Until(start.Add(time.Duration(i-1) * gap)); w > 0 {
			select {
			case <-time.After(w):
			case <-ctx.Done():
				return res, nil
			}
		}
		msg, idx, err := CraftHandshakeInit(initiatorPriv, responderPub)
		if err != nil {
			return res, err
		}
		if _, err := conn.Write(msg); err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				res.Refused = true // ICMP 端口不可达：目标端口没监听
				return res, nil
			}
			return res, fmt.Errorf("UDP 发送失败: %w", err)
		}
		idxes[idx] = true
		res.Attempt = i
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return res, err
	}
	buf := make([]byte, 256)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				res.Refused = true
			}
			return res, nil // 预算耗尽（或 ICMP 不可达），判定不通
		}
		for idx := range idxes {
			if IsHandshakeResponse(buf[:n], idx) {
				res.OK, res.RTT = true, time.Since(start)
				return res, nil
			}
		}
	}
}

// ---------- 无握手私钥时的粗判 ----------

// UDPReach 面板侧「拿不到成员私钥、发不了握手」时的粗判结论。
type UDPReach int

const (
	ReachUnknown  UDPReach = iota // 无结论：超时（云安全组静默丢包与主机未监听无法区分）
	ReachClosed                   // 收到 ICMP 端口不可达：入方向路径通、主机在线，仅该端口暂无监听
	ReachListener                 // 该端口有服务回包（有监听者，但是否为本网 WG 需握手才能确认）
	ReachNoRoute                  // 域名解析失败 / 网络不可达
)

func (r UDPReach) String() string {
	switch r {
	case ReachClosed:
		return "closed"
	case ReachListener:
		return "listener"
	case ReachNoRoute:
		return "noroute"
	default:
		return "unknown"
	}
}

// ProbeUDPReach 发送一个 UDP 空包并按回应粗判可达性：
//   - 目标主机回 ICMP 端口不可达（Go 表现为 ECONNREFUSED）= 路径通、端口未监听；
//   - 收到任何数据 = 有监听者；
//   - 超时 = 无结论（安全组丢弃与未监听不区分），调用方据此只给「提示」不判失败。
//
// 用途：首次组网时尚无托管成员、无法发真握手，先用它区分「网络根本没通」与「只是还没装 WG」。
func ProbeUDPReach(ctx context.Context, endpoint string, wait time.Duration) (UDPReach, error) {
	raddr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return ReachNoRoute, fmt.Errorf("端点无法解析: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return ReachNoRoute, fmt.Errorf("UDP 建连失败: %w", err)
	}
	defer conn.Close()
	if wait <= 0 {
		wait = 2 * time.Second
	}
	deadline := time.Now().Add(wait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	buf := make([]byte, 64)
	// 连发两次：ICMP 不可达可能被限速，第一次读会超时、第二次才拿到错误
	for i := 0; i < 2; i++ {
		if _, err := conn.Write([]byte{0}); err != nil {
			return ReachNoRoute, fmt.Errorf("UDP 发送失败: %w", err)
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return ReachUnknown, err
		}
		if _, err := conn.Read(buf); err == nil {
			return ReachListener, nil
		} else if errors.Is(err, syscall.ECONNREFUSED) {
			return ReachClosed, nil
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			break
		}
	}
	return ReachUnknown, nil
}

// udpProbeUnsupported 探测被跳过的统一说明（首次组网、尚无托管成员）。
const udpProbeUnsupported = "暂无可用于探测的成员私钥（首次组网）：配置完成后以首次握手为准"

// endpointPort 从 host:port 取端口号（取不到返回 0）。
func endpointPort(endpoint string) int {
	i := strings.LastIndex(endpoint, ":")
	if i < 0 {
		return 0
	}
	p, err := strconv.Atoi(endpoint[i+1:])
	if err != nil {
		return 0
	}
	return p
}
