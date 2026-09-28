package wg

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// 假中心：收到合法发起包就回 type 2（模拟内核态 WG 只回应已知成员的行为）。
func startFakeHub(t *testing.T) (endpoint string, hubPub string, stop func()) {
	t.Helper()
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成中心密钥: %v", err)
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听 UDP: %v", err)
	}
	go func() {
		buf := make([]byte, 256)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n != wgInitMsgLen || binary.LittleEndian.Uint32(buf[0:4]) != wgMsgInitiation {
				continue
			}
			_, _ = pc.WriteTo(respPacket(binary.LittleEndian.Uint32(buf[4:8])), addr)
		}
	}()
	return pc.LocalAddr().String(), kp.Public, func() { pc.Close() }
}

// 造一个「已有中心 + 一台面板托管成员」的最小库。
func newProbeTestRunner(t *testing.T, endpoint, hubPub string) (*Runner, *store.DB, int64) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatalf("打开库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	now := time.Now().Unix()
	if err := db.EnsureWGNetwork(&store.WGNetwork{Subnet: "10.66.66.0/24", HubIP: "10.66.66.2",
		Iface: "wg0", Keepalive: 25, MTU: 1420, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("建网: %v", err)
	}
	r := NewRunner(db, []byte("0123456789abcdef0123456789abcdef"))
	hubID, err := db.CreateServer(&store.Server{Name: "公网机", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatalf("建节点: %v", err)
	}
	hubKP, _ := GenerateKeyPair()
	if err := db.UpsertWGHub(&store.WGHub{ServerID: hubID, ListenPort: endpointPort(endpoint),
		PublicKey: hubPub, PrivateKeyEnc: r.Encrypt(hubKP.Private), Endpoint: endpoint,
		Status: "ok", CheckedAt: sql.NullInt64{Int64: now, Valid: true}}); err != nil {
		t.Fatalf("写中心: %v", err)
	}
	peerKP, _ := GenerateKeyPair()
	if _, err := db.InsertWGPeer(&store.WGPeer{Kind: "device", Name: "离线手机",
		WgIP: "10.66.66.13", PublicKey: peerKP.Public, PrivateKeyEnc: r.Encrypt(peerKP.Private),
		PskEnc: r.Encrypt("psk"), Managed: true, Status: "offline", CreatedAt: now}); err != nil {
		t.Fatalf("写成员: %v", err)
	}
	return r, db, hubID
}

func TestProbeHubUDPWithDB(t *testing.T) {
	endpoint, hubPub, stop := startFakeHub(t)
	defer stop()
	r, db, hubID := newProbeTestRunner(t, endpoint, hubPub)

	hub, _ := db.GetWGHub(hubID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := r.ProbeHubUDP(ctx, hub, time.Second)
	if err != nil {
		t.Fatalf("探测出错: %v", err)
	}
	if out.Skipped || !out.Result.OK {
		t.Fatalf("应探测成功，得到 skipped=%v ok=%v reason=%s", out.Skipped, out.Result.OK, out.Reason)
	}
	if out.Peer != "离线手机" {
		t.Fatalf("应挑到托管的离线成员当探针，得到 %q", out.Peer)
	}

	// ValidateHubUDP：可达 = 放行
	if blocked, reason := r.ValidateHubUDP(ctx, hub, hub.ListenPort); blocked {
		t.Fatalf("可达时不该阻断: %s", reason)
	}
	// HubUDPNote：写成一行可读结论
	note := r.HubUDPNote(ctx, hub, hub.ListenPort, true)
	if !contains(note, "握手成功") {
		t.Fatalf("实测结论文案不对: %s", note)
	}
}

func TestValidateHubUDPBlocksWhenNoListener(t *testing.T) {
	// 目标端口无人监听：有托管私钥时真握手无回应、没有时靠 ICMP，两条路都要判「确定不通」而阻断
	endpoint, hubPub, stop := startFakeHub(t)
	stop() // 关掉假中心，端口释放
	r, db, hubID := newProbeTestRunner(t, endpoint, hubPub)
	hub, _ := db.GetWGHub(hubID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	blocked, reason := r.ValidateHubUDP(ctx, hub, hub.ListenPort)
	if !blocked {
		t.Fatal("端口无监听应阻断切换")
	}
	if !contains(reason, "无监听") && !contains(reason, "没有监听") && !contains(reason, "握手无回应") {
		t.Fatalf("原因应可执行（放行 UDP 或说明端口无监听）: %s", reason)
	}
}

// 假中心（挑食版）：只回应「认识」的成员——初始包里的发起方静态公钥在 allowed 里才回包。
// 真实内核 WG 正是这个行为（未知成员静默丢弃），用来复现「备援中心不认识某个成员」的场景。
func startPickFakeHub(t *testing.T, allowed []string) (endpoint string, hubPub string, stop func()) {
	t.Helper()
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成中心密钥: %v", err)
	}
	priv, _ := decodeKey(kp.Private)
	pub, _ := decodeKey(kp.Public)
	resp := testResponder{priv: priv, pub: pub}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听 UDP: %v", err)
	}
	go func() {
		buf := make([]byte, 256)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			initPub, _, ok := resp.open(buf[:n])
			if !ok {
				continue
			}
			known := false
			for _, a := range allowed {
				if a == base64.StdEncoding.EncodeToString(initPub) {
					known = true
				}
			}
			if !known {
				continue // 不认识的成员：静默丢弃（与内核 WG 一致）
			}
			_, _ = pc.WriteTo(respPacket(binary.LittleEndian.Uint32(buf[4:8])), addr)
		}
	}()
	return pc.LocalAddr().String(), kp.Public, func() { pc.Close() }
}

func TestProbeHubEndpointFallsBackToKnownPeer(t *testing.T) {
	// 中心只认识第二台成员（备援没同步过新成员的真实场景）：
	// 换身份再试应判定「端口通」，而不是误报「UDP 没放行」→ 误阻断切换。
	kp2, _ := GenerateKeyPair()
	endpoint, hubPub, stop := startPickFakeHub(t, []string{kp2.Public})
	defer stop()
	r, db, hubID := newProbeTestRunner(t, endpoint, hubPub)
	now := time.Now().Unix()
	// 库里第一台（排序在前）中心不认识；第二台才认识
	if _, err := db.InsertWGPeer(&store.WGPeer{Kind: "device", Name: "新的手机",
		WgIP: "10.66.66.14", PublicKey: kp2.Public, PrivateKeyEnc: r.Encrypt(kp2.Private),
		Managed: true, Status: "offline", CreatedAt: now}); err != nil {
		t.Fatalf("写成员: %v", err)
	}
	hub, _ := db.GetWGHub(hubID)
	out, err := r.ProbeHubUDP(context.Background(), hub, 800*time.Millisecond)
	if err != nil {
		t.Fatalf("探测出错: %v", err)
	}
	if out.Skipped || !out.Result.OK {
		t.Fatalf("中心认识第二个身份时应判定可达: skipped=%v ok=%v peers=%v", out.Skipped, out.Result.OK, out.Peers)
	}
	if out.Peer != "新的手机" {
		t.Fatalf("应回退到「新的手机」才握手成功: %s（试过 %v）", out.Peer, out.Peers)
	}
	if len(out.Peers) != 2 {
		t.Fatalf("应记录两个尝试过的身份: %v", out.Peers)
	}
}

func TestProbeHubEndpointAllUnknownPeerReportsBothCauses(t *testing.T) {
	// 所有身份都不被中心认识：文案必须同时给出「放行 UDP」与「同步成员」两条路，
	// 并如实说明换了几个身份
	stranger, _ := GenerateKeyPair() // 中心只认识这台无关成员，库里的探针成员它都不认识
	endpoint, hubPub, stop := startPickFakeHub(t, []string{stranger.Public})
	defer stop()
	r, db, hubID := newProbeTestRunner(t, endpoint, hubPub)
	now := time.Now().Unix()
	kp3, _ := GenerateKeyPair()
	if _, err := db.InsertWGPeer(&store.WGPeer{Kind: "device", Name: "新的手机",
		WgIP: "10.66.66.14", PublicKey: kp3.Public, PrivateKeyEnc: r.Encrypt(kp3.Private),
		Managed: true, Status: "offline", CreatedAt: now}); err != nil {
		t.Fatalf("写成员: %v", err)
	}
	hub, _ := db.GetWGHub(hubID)
	out, err := r.ProbeHubUDP(context.Background(), hub, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("探测出错: %v", err)
	}
	if out.Result.OK || out.Skipped {
		t.Fatalf("都不认识时应判不通: %+v", out)
	}
	msg := out.FailMessage(hub.ListenPort)
	for _, want := range []string{"放行 UDP", "同步到所有中心", "2 个成员身份"} {
		if !contains(msg, want) {
			t.Fatalf("文案缺少 %q: %s", want, msg)
		}
	}
}

func TestProbeHubUDPSkipsWithoutManagedPeer(t *testing.T) {
	endpoint, hubPub, stop := startFakeHub(t)
	defer stop()
	db, err := store.Open(filepath.Join(t.TempDir(), "nop.db"))
	if err != nil {
		t.Fatalf("打开库: %v", err)
	}
	defer db.Close()
	now := time.Now().Unix()
	_ = db.EnsureWGNetwork(&store.WGNetwork{Subnet: "10.66.66.0/24", HubIP: "10.66.66.2",
		Iface: "wg0", Keepalive: 25, MTU: 1420, CreatedAt: now, UpdatedAt: now})
	hubKP, _ := GenerateKeyPair()
	hubID, _ := db.CreateServer(&store.Server{Name: "公网机", Enabled: true, CreatedAt: now})
	_ = db.UpsertWGHub(&store.WGHub{ServerID: hubID, ListenPort: endpointPort(endpoint),
		PublicKey: hubPub, PrivateKeyEnc: []byte{}, Endpoint: endpoint, Status: "ok"})
	// 只有导入成员（无私钥）：应跳过并给出原因，而不是报错或误判不通
	_, _ = db.InsertWGPeer(&store.WGPeer{Kind: "device", Name: "Mac", WgIP: "10.66.66.11",
		PublicKey: hubKP.Public, Managed: false, Status: "online", CreatedAt: now})
	r := NewRunner(db, []byte("0123456789abcdef0123456789abcdef"))
	hub, _ := db.GetWGHub(hubID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := r.ProbeHubUDP(ctx, hub, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if !out.Skipped || out.Result.OK {
		t.Fatalf("无托管私钥时应跳过: skipped=%v ok=%v", out.Skipped, out.Result.OK)
	}
	if !contains(out.Reason, "首次组网") {
		t.Fatalf("跳过原因应可读: %s", out.Reason)
	}
	// 跳过时硬门禁必须放行（不能凭「无法探测」拦住切换）
	if blocked, _ := r.ValidateHubUDP(ctx, hub, hub.ListenPort); blocked {
		t.Fatal("探测手段不可用时应放行")
	}
}
