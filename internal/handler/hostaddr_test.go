package handler

import "testing"

func TestParseProcRouteDefaultGateway(t *testing.T) {
	// 实测数据（ops 宿主机容器的 /proc/net/route）：默认路由网关 01F0A8C0 = 192.168.240.1
	real := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t01F0A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"eth0\t00F0A8C0\t00000000\t0001\t0\t0\t0\t00F0FFFF\t0\t0\t0\n"
	if got := parseProcRoute(real); got != "192.168.240.1" {
		t.Fatalf("parseProcRoute(real) = %q, want 192.168.240.1", got)
	}
	cases := []struct {
		name, in, want string
	}{
		{"docker0 网关", "eth0\t00000000\t0101A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n", "192.168.1.1"},
		{"172.17.0.1", "eth0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n", "172.17.0.1"},
		{"无默认路由", "eth0\t00F0A8C0\t00000000\t0001\t0\t0\t0\t00F0FFFF\t0\t0\t0\n", ""},
		{"默认路由但网关闭", "eth0\t00000000\t00000000\t0003\t0\t0\t0\t00000000\t0\t0\t0\n", ""},
		{"默认路由但 UP 未置位", "eth0\t00000000\t01F0A8C0\t0002\t0\t0\t0\t00000000\t0\t0\t0\n", ""},
		{"空内容", "", ""},
		{"仅表头", "Iface\tDestination\tGateway \tFlags\n", ""},
	}
	for _, c := range cases {
		if got := parseProcRoute(c.in); got != c.want {
			t.Errorf("%s: parseProcRoute = %q, want %q", c.name, got, c.want)
		}
	}
}
