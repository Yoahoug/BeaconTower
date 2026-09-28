package wg

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestProbeUDPLive 真机联调：对一台真实运行的 WireGuard hub 发握手，验证自造发起包被内核态 WG 接受。
// 默认跳过，需要三个环境变量（用面板临时建一台设备成员、从 conf 里取私钥即可）：
//
//	BT_PROBE_ENDPOINT  hub 公网端点，如 47.109.156.165:51820
//	BT_PROBE_HUB_PUB   hub 公钥（base64）
//	BT_PROBE_PEER_PRIV 该 hub 已知的某成员私钥（base64）
func TestProbeUDPLive(t *testing.T) {
	ep := os.Getenv("BT_PROBE_ENDPOINT")
	hubPub := os.Getenv("BT_PROBE_HUB_PUB")
	peerPriv := os.Getenv("BT_PROBE_PEER_PRIV")
	if ep == "" || hubPub == "" || peerPriv == "" {
		t.Skip("未设置 BT_PROBE_* 环境变量：跳过真机联调")
	}
	respPub, err := decodeKey(hubPub)
	if err != nil {
		t.Fatalf("hub 公钥无效: %v", err)
	}
	priv, err := decodeKey(peerPriv)
	if err != nil {
		t.Fatalf("成员私钥无效: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ProbeUDP(ctx, ep, respPub, priv, 4*time.Second)
	if err != nil {
		t.Fatalf("探测出错: %v", err)
	}
	if !out.OK {
		t.Fatalf("hub 未回应握手（端点 %s）：UDP 可能未放行，或该私钥不属于本网成员", ep)
	}
	t.Logf("UDP 可达 ✓ endpoint=%s attempt=%d rtt=%s", ep, out.Attempt, out.RTT.Round(time.Millisecond))
}
