package wg

import (
	"strings"
	"testing"
)

func TestHubEndpointOnPort(t *testing.T) {
	cases := []struct {
		in   string
		port int
		want string
	}{
		{"47.109.156.165:51820", 51830, "47.109.156.165:51830"}, // 端口改用计划端口
		{"47.109.156.165:51820", 0, "47.109.156.165:51820"},     // 端口缺省沿用行里的
		{"[2001:db8::1]:51820", 51830, "[2001:db8::1]:51830"},   // IPv6
		{"", 51820, ""},           // 无主机
		{"47.109.156.165", 0, ""}, // 无端口可推
		{"47.109.156.165", 51820, "47.109.156.165:51820"},  // 主机无端口但给了端口
		{"47.109.156.165:", 51820, "47.109.156.165:51820"}, // 尾冒号容错
	}
	for _, c := range cases {
		if got := HubEndpointOnPort(c.in, c.port); got != c.want {
			t.Fatalf("HubEndpointOnPort(%q,%d)=%q，期望 %q", c.in, c.port, got, c.want)
		}
	}
}

func TestReachHintMessage(t *testing.T) {
	// 首次安装：端口暂无监听 = 路径通，属好消息，不打扰
	if msg, need := ReachHintMessage(51820, "1.2.3.4:51820", ReachClosed, false); need || !strings.Contains(msg, "路径可达") {
		t.Fatalf("首次安装的 ReachClosed 不该提示: %q need=%v", msg, need)
	}
	// 已有中心：端口无监听 = 真问题，要提示
	if _, need := ReachHintMessage(51820, "1.2.3.4:51820", ReachClosed, true); !need {
		t.Fatal("已有中心的 ReachClosed 必须提示")
	}
	// 超时 = 无法确认，要提示用户去放行；且必须点明「去哪儿放行」——
	// 实测中阿里云安全组只放行 TCP 时 UDP 被静默丢弃，用户看到「请放行 UDP」并不知道是云控制台还是服务器防火墙。
	msg, need := ReachHintMessage(51820, "1.2.3.4:51820", ReachUnknown, true)
	if !need || !strings.Contains(msg, "安全组") {
		t.Fatalf("超时应提示放行安全组: %q need=%v", msg, need)
	}
	for _, want := range []string{"入方向", "控制台", "UDP", "0.0.0.0/0", "TCP"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("UDP 未放行提示缺少「%s」，用户无法定位问题：%q", want, msg)
		}
	}
	// 有监听者：不打扰
	if _, need := ReachHintMessage(51820, "1.2.3.4:51820", ReachListener, true); need {
		t.Fatal("ReachListener 不该提示")
	}
}
