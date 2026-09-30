package wg

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

// ---------- 测试用响应方（按白皮书独立实现一遍，用于校验发起包能被解出来） ----------

type testResponder struct {
	priv []byte
	pub  []byte
}

func newTestResponder(t *testing.T) testResponder {
	t.Helper()
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成响应方密钥失败: %v", err)
	}
	priv, _ := decodeKey(kp.Private)
	pub, _ := decodeKey(kp.Public)
	return testResponder{priv: priv, pub: pub}
}

// open 解开发起包，返回发起方静态公钥与时间戳；任何一步失败即返回 false。
func (r testResponder) open(msg []byte) (initiatorPub []byte, ts []byte, ok bool) {
	if len(msg) != wgInitMsgLen || binary.LittleEndian.Uint32(msg[0:4]) != wgMsgInitiation {
		return nil, nil, false
	}
	// mac1 校验
	want := b2s128Keyed(b2s256([]byte(wgLabelMAC1), r.pub), msg[:wgMac1Offset])
	if !equalBytes(want, msg[wgMac1Offset:wgMac1Offset+16]) {
		return nil, nil, false
	}
	eph := msg[8:40]
	ck := b2s256([]byte(wgConstruction))
	h := b2s256(ck, []byte(wgIdentifier))
	h = b2s256(h, r.pub)
	ck = kdf1(ck, eph)
	h = b2s256(h, eph)
	es, err := curve25519.X25519(r.priv, eph)
	if err != nil {
		return nil, nil, false
	}
	ck, k := kdf2(ck, es)
	initPub, err := aeadOpen(k, msg[40:88], h)
	if err != nil || len(initPub) != 32 {
		return nil, nil, false
	}
	h = b2s256(h, msg[40:88])
	ss, err := curve25519.X25519(r.priv, initPub)
	if err != nil {
		return nil, nil, false
	}
	_, k = kdf2(ck, ss)
	ts, err = aeadOpen(k, msg[88:116], h)
	if err != nil || len(ts) != 12 {
		return nil, nil, false
	}
	return initPub, ts, true
}

func aeadOpen(key, ciphertext, ad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	return aead.Open(nil, nonce, ciphertext, ad)
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------- 发起包 ----------

func TestCraftHandshakeInitLayout(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := decodeKey(kp.Private)
	resp := newTestResponder(t)

	msg, idx, err := CraftHandshakeInit(priv, resp.pub)
	if err != nil {
		t.Fatalf("构造发起包失败: %v", err)
	}
	if len(msg) != wgInitMsgLen {
		t.Fatalf("报文长度 %d，期望 %d", len(msg), wgInitMsgLen)
	}
	if got := binary.LittleEndian.Uint32(msg[0:4]); got != wgMsgInitiation {
		t.Fatalf("type=%d，期望 %d", got, wgMsgInitiation)
	}
	if got := binary.LittleEndian.Uint32(msg[4:8]); got != idx {
		t.Fatalf("sender index 与返回值不一致: %d vs %d", got, idx)
	}
	// mac2 留零；mac1 非零且可复算
	if !allZero(msg[132:148]) {
		t.Fatal("mac2 应为全零（未使用 cookie）")
	}
	if allZero(msg[wgMac1Offset : wgMac1Offset+16]) {
		t.Fatal("mac1 不应为全零")
	}
	want := b2s128Keyed(b2s256([]byte(wgLabelMAC1), resp.pub), msg[:wgMac1Offset])
	if !equalBytes(want, msg[wgMac1Offset:wgMac1Offset+16]) {
		t.Fatal("mac1 复算不一致")
	}
}

func TestCraftHandshakeInitIsOpenable(t *testing.T) {
	// 关键回归：发起包必须能被独立的响应方实现解出「发起方公钥 + 时间戳」。
	// 这条能挡住 KDF 链没推进（chaining key 未更新）之类的严重错误。
	kp, _ := GenerateKeyPair()
	priv, _ := decodeKey(kp.Private)
	pub, _ := decodeKey(kp.Public)
	resp := newTestResponder(t)

	msg, _, err := CraftHandshakeInit(priv, resp.pub)
	if err != nil {
		t.Fatal(err)
	}
	gotPub, ts, ok := resp.open(msg)
	if !ok {
		t.Fatal("响应方无法解开发起包（KDF/AEAD/布局有误）")
	}
	if !equalBytes(gotPub, pub) {
		t.Fatal("解出的发起方公钥与预期不符")
	}
	if allZero(ts) {
		t.Fatal("时间戳不应为空")
	}
}

func TestCraftHandshakeInitFreshEachTime(t *testing.T) {
	kp, _ := GenerateKeyPair()
	priv, _ := decodeKey(kp.Private)
	resp := newTestResponder(t)

	a, ia, err := CraftHandshakeInit(priv, resp.pub)
	if err != nil {
		t.Fatal(err)
	}
	b, ib, err := CraftHandshakeInit(priv, resp.pub)
	if err != nil {
		t.Fatal(err)
	}
	if ia == ib || equalBytes(a, b) {
		t.Fatal("每次探测都应使用新的 sender index 与 ephemeral")
	}
	if equalBytes(a[8:40], b[8:40]) {
		t.Fatal("ephemeral 公钥重复")
	}
}

func TestCraftHandshakeInitRejectsBadKeys(t *testing.T) {
	resp := newTestResponder(t)
	if _, _, err := CraftHandshakeInit(make([]byte, 31), resp.pub); err == nil {
		t.Fatal("私钥长度错误应报错")
	}
	if _, _, err := CraftHandshakeInit(make([]byte, 32), make([]byte, 16)); err == nil {
		t.Fatal("公钥长度错误应报错")
	}
}

// ---------- 回应判定 ----------

func respPacket(senderIndex uint32) []byte {
	b := make([]byte, wgRespMsgLen)
	binary.LittleEndian.PutUint32(b[0:4], wgMsgResponse)
	binary.LittleEndian.PutUint32(b[4:8], 0x11223344)
	binary.LittleEndian.PutUint32(b[8:12], senderIndex)
	return b
}

func TestIsHandshakeResponse(t *testing.T) {
	idx := uint32(0xdeadbeef)
	if !IsHandshakeResponse(respPacket(idx), idx) {
		t.Fatal("正确的回应包应被接受")
	}
	if IsHandshakeResponse(respPacket(idx+1), idx) {
		t.Fatal("receiver index 不匹配不应接受")
	}
	bad := respPacket(idx)
	binary.LittleEndian.PutUint32(bad[0:4], wgMsgInitiation)
	if IsHandshakeResponse(bad, idx) {
		t.Fatal("type 错误不应接受")
	}
	if IsHandshakeResponse(respPacket(idx)[:40], idx) {
		t.Fatal("长度错误不应接受")
	}
}

// ---------- 探测流程 ----------

func TestProbeUDPWithStubResponder(t *testing.T) {
	// 假响应方：收到发起包后回一个 type 2（receiver index 取自请求），验证探测成功路径。
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 256)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n != wgInitMsgLen {
				continue
			}
			_, _ = pc.WriteTo(respPacket(binary.LittleEndian.Uint32(buf[4:8])), addr)
		}
	}()

	kp, _ := GenerateKeyPair()
	priv, _ := decodeKey(kp.Private)
	resp := newTestResponder(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	budget := 2 * time.Second
	out, err := ProbeUDP(ctx, pc.LocalAddr().String(), resp.pub, priv, budget)
	if err != nil {
		t.Fatalf("探测不应报错: %v", err)
	}
	if !out.OK {
		t.Fatal("收到回应应判定可达")
	}
	if out.RTT <= 0 {
		t.Fatal("RTT 应大于 0")
	}
	// 回包是即时的：应第一发就收到，且 RTT 是真 RTT，不能是「发满重试间隔才读」的假高延迟
	if out.Attempt != 1 {
		t.Fatalf("即时回包应第一发即成功，实际第 %d 发", out.Attempt)
	}
	if out.RTT >= budget/6 {
		t.Fatalf("RTT 应为一个往返的量级（< %v），实际 %v", budget/6, out.RTT)
	}
}

func TestProbeUDPUnreachable(t *testing.T) {
	// 无人应答的端口：本机会回 ICMP 端口不可达 → 判不通，且不再白等重试
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	pc.Close() // 立刻释放，端口无人监听

	kp, _ := GenerateKeyPair()
	priv, _ := decodeKey(kp.Private)
	resp := newTestResponder(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := ProbeUDP(ctx, addr, resp.pub, priv, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("探测超时不应作为错误返回: %v", err)
	}
	if out.OK {
		t.Fatal("无回应不应判定可达")
	}
	if !out.Refused {
		t.Fatal("回 ICMP 端口不可达时应标记 Refused（端口没监听，与静默丢包区分开）")
	}
}

func TestProbeUDPSilentDropSpendsWholeBudget(t *testing.T) {
	// 有监听者但从不回包（模拟安全组放行、但服务不认这个成员）：既非可达也非 ICMP 拒绝，
	// 应把三个握手包都发出去，且总共只等一个预算（而不是每个包各等一个预算）。
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() { // 收下不答
		buf := make([]byte, 256)
		for {
			if _, _, err := pc.ReadFrom(buf); err != nil {
				return
			}
		}
	}()

	kp, _ := GenerateKeyPair()
	priv, _ := decodeKey(kp.Private)
	resp := newTestResponder(t)
	budget := 600 * time.Millisecond
	start := time.Now()
	out, err := ProbeUDP(context.Background(), pc.LocalAddr().String(), resp.pub, priv, budget)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("静默丢包不应报错: %v", err)
	}
	if out.OK || out.Refused {
		t.Fatalf("静默丢包应判不通且不标 Refused: %+v", out)
	}
	if out.Attempt != 3 {
		t.Fatalf("应发满 3 个握手包，实际 %d", out.Attempt)
	}
	if elapsed > 3*budget {
		t.Fatalf("等待应受一个总预算约束（%v），实际 %v", budget, elapsed)
	}
}

func TestProbeUDPBadEndpoint(t *testing.T) {
	kp, _ := GenerateKeyPair()
	priv, _ := decodeKey(kp.Private)
	resp := newTestResponder(t)
	out, err := ProbeUDP(context.Background(), "127.0.0.1", resp.pub, priv, 200*time.Millisecond)
	if err == nil || out.OK {
		t.Fatal("缺少端口的端点应报错")
	}
}

// ---------- UDPReach 粗判 ----------

func TestProbeUDPReachClosed(t *testing.T) {
	// 取一个本机空闲 UDP 端口后立刻释放：向它发包会收到 ICMP 端口不可达
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("取空闲端口失败: %v", err)
	}
	ep := c.LocalAddr().String()
	c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := ProbeUDPReach(ctx, ep, 2*time.Second)
	if err != nil {
		t.Fatalf("粗判出错: %v", err)
	}
	if got != ReachClosed {
		t.Fatalf("期望 ReachClosed，得到 %s", got)
	}
}

func TestProbeUDPReachListener(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 64)
		if n, addr, err := pc.ReadFrom(buf); err == nil {
			_, _ = pc.WriteTo([]byte("pong"), addr)
			_ = n
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := ProbeUDPReach(ctx, pc.LocalAddr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("粗判出错: %v", err)
	}
	if got != ReachListener {
		t.Fatalf("期望 ReachListener，得到 %s", got)
	}
}

func TestProbeUDPReachBadEndpoint(t *testing.T) {
	if _, err := ProbeUDPReach(context.Background(), "1.2.3.4:99999", time.Second); err == nil {
		t.Fatal("非法端口应返回错误")
	}
}

func TestHubProbeOutcomeFailMessage(t *testing.T) {
	o := HubProbeOutcome{Result: ProbeUDPResult{Target: "1.2.3.4:51820"}}
	msg := o.FailMessage(51820)
	if msg == "" || !contains(msg, "51820") || !contains(msg, "1.2.3.4:51820") {
		t.Fatalf("提示文案缺信息: %s", msg)
	}
	// 握手超时的文案要给出可照做的定位路径（云控制台 → 安全组 → 入方向 → UDP），
	// 并提醒「放行 TCP 不等于放行 UDP」——真实翻车场景就是安全组只开了 TCP。
	for _, want := range []string{"安全组", "入方向", "控制台", "UDP", "0.0.0.0/0", "TCP"} {
		if !contains(msg, want) {
			t.Fatalf("握手失败文案缺少「%s」：%s", want, msg)
		}
	}
	o2 := HubProbeOutcome{Result: ProbeUDPResult{Target: "1.2.3.4:51830"}}
	if !contains(o2.FailMessage(0), "51830") {
		t.Fatalf("端口缺省时应从端点推断: %s", o2.FailMessage(0))
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}
