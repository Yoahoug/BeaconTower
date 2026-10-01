package frp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 以下 fixture 直接取自线上真实响应（敏感值已替换），用于固化字段映射。
// 这些坑都是真机联调时才暴露出来的，靠文档推断会写错：
//   - ChmlFrp 把布尔语义字段序列化成字符串 "true"/"false"
//   - ChmlFrp 的 uptime 是 ISO 时刻而不是秒数
//   - ChmlFrp 的 tunnel 是上限、tunnelCount 是已用（与官方文档说明相反）
//   - Sakura 节点 flag 低两位是独立能力位，从不同时置位

const chmlTunnelFixture = `{"msg":"获取隧道数据成功","code":200,"state":"success","data":[{
  "id":356713,"name":"new","localip":"192.168.0.10","type":"tcp","nport":3000,
  "dorp":"34280","node":"英国伦敦","state":"true","userid":39983,
  "encryption":"false","compression":"false","ap":"",
  "uptime":"2020-01-01T00:00:00.000+00:00","client_version":"0.61.2",
  "today_traffic_in":123,"today_traffic_out":456,"cur_conns":7,
  "traffic_date":"2026-09-28","nodestate":"online","ip":"uk.frp.one",
  "server_port":7000,"node_token":"SECRET","node_ip":"194.147.16.88"
}]}`

func TestChmlfrpTunnelParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, 期望 Bearer tok", got)
		}
		_, _ = w.Write([]byte(chmlTunnelFixture))
	}))
	defer srv.Close()

	c := NewChmlfrp(srv.URL, "tok")
	tuns, err := c.Tunnels(context.Background())
	if err != nil {
		t.Fatalf("Tunnels 失败: %v", err)
	}
	if len(tuns) != 1 {
		t.Fatalf("隧道数 = %d, 期望 1", len(tuns))
	}
	got := tuns[0]
	if got.RemoteID != "356713" || got.Name != "new" || got.Proto != "tcp" {
		t.Errorf("基础字段映射错误: %+v", got)
	}
	if got.LocalPort != 3000 || got.Remote != "34280" || got.NodeName != "英国伦敦" {
		t.Errorf("端口/节点映射错误: local=%d remote=%q node=%q", got.LocalPort, got.Remote, got.NodeName)
	}
	if !got.Online {
		t.Error("state=\"true\" 应解析为在线（平台用字符串表达布尔）")
	}
	if got.Conns != 7 || got.TodayUp != 123 || got.TodayDown != 456 {
		t.Errorf("用量字段映射错误: conns=%d up=%d down=%d", got.Conns, got.TodayUp, got.TodayDown)
	}
	// uptime 是「上次启动时刻」，应换算成已运行秒数；fixture 用 2020 年，
	// 到现在的秒数必然远大于 1 年，用它区分「换算成功」与「原样透传 0」
	if got.Uptime < 86400*365 {
		t.Errorf("uptime 未按 ISO 时刻换算成时长: %d", got.Uptime)
	}
	// encryption/compression 是字符串 "false"，不能被当成 true
	if got.Extra != "节点 uk.frp.one" {
		t.Errorf("Extra 应只含节点信息（未启用加密/压缩），实际 %q", got.Extra)
	}
}

func TestChmlfrpTunnelNoNodeState(t *testing.T) {
	// nodestate 为空 = 节点永久下线（官方文档原文），要给用户可读原因
	fixture := `{"code":200,"data":[{"id":1,"name":"t","type":"tcp","state":"false","nodestate":"","node":"已下线节点"}],"state":"success"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()
	tuns, err := NewChmlfrp(srv.URL, "tok").Tunnels(context.Background())
	if err != nil {
		t.Fatalf("Tunnels 失败: %v", err)
	}
	if tuns[0].Online {
		t.Error("state=\"false\" 应为离线")
	}
	if tuns[0].StatusReason == "" {
		t.Error("nodestate 为空时应给出「节点已永久下线」提示")
	}
}

func TestChmlfrpUserInfoMapping(t *testing.T) {
	// tunnel=4 是上限、tunnelCount=1 是已用（账号下确实只有 1 条隧道，
	// 免费用户默认 4 条）——字段名极易读反，这里固化正确方向
	fixture := `{"code":200,"state":"success","data":{
	  "id":39983,"username":"ahhhahh","usergroup":"免费用户","bandwidth":8,
	  "tunnel":4,"tunnelCount":1,"realname":"已实名","integral":16868,
	  "term":"9999-09-09","regtime":"2025-05-15",
	  "total_upload":1653968429,"total_download":239857203,"totalCurConns":3}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()
	acc, err := NewChmlfrp(srv.URL, "tok").UserInfo(context.Background())
	if err != nil {
		t.Fatalf("UserInfo 失败: %v", err)
	}
	if acc.TunnelQuota != 4 {
		t.Errorf("tunnel 字段应映射为上限 4，实际 %d", acc.TunnelQuota)
	}
	if acc.TunnelUsed != 1 {
		t.Errorf("tunnelCount 字段应映射为已用 1，实际 %d", acc.TunnelUsed)
	}
	if acc.Conns != 3 || acc.TrafficUp != 1653968429 || acc.Realname != "已实名" {
		t.Errorf("账号字段映射错误: %+v", acc)
	}
	if acc.SpeedLimit == "" {
		t.Error("限速描述不应为空")
	}
}

func TestChmlfrpErrorSurfacesCode(t *testing.T) {
	// 业务错误藏在 HTTP 200 的 code 里：只看 HTTP 状态码会把失败当成功
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"无效的登录状态","code":401,"state":"fail"}`))
	}))
	defer srv.Close()
	_, err := NewChmlfrp(srv.URL, "tok").UserInfo(context.Background())
	if err == nil {
		t.Fatal("code=401 应返回错误而不是静默成功")
	}
	if !errors.Is(err, ErrAuth) {
		t.Errorf("401 应被识别为凭据失效（触发重新授权），实际 %v", err)
	}
}

func TestNatfrpErrorIsHTTP500WithBodyCode(t *testing.T) {
	// Sakura 的错误统一返回 HTTP 500，真实错误码在 body 的 code 字段
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":401,"msg":"访问密钥无效"}`))
	}))
	defer srv.Close()
	_, err := NewNatfrp(srv.URL, "bad").UserInfo(context.Background())
	if err == nil {
		t.Fatal("应返回错误")
	}
	if !errors.Is(err, ErrAuth) {
		t.Errorf("body code=401 应被识别为凭据失效，实际 %v", err)
	}
}

func TestNatfrpUserInfoAndTunnels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/user/info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":1819140,"name":"u","speed":"10 Mbps","tunnels":2,"realname":2,
		  "group":{"name":"普通用户","level":0,"expires":0},
		  "traffic":[2071158584,12243133112],
		  "sign":{"config":[1,4],"signed":true,"last":"2026-09-30","days":90,"traffic":216.6}}`))
	})
	mux.HandleFunc("/tunnels", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":19859613,"name":"ssh","type":"tcp","node":12,"online":true,
		  "status":0,"status_reason":null,"note":"","extra":"auto_https = auto",
		  "remote":"29377","local_ip":"localhost","local_port":22,
		  "locks":{"edit":false,"delete":true,"migrate":false}}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewNatfrp(srv.URL, "tok")
	ctx := context.Background()

	acc, err := c.UserInfo(ctx)
	if err != nil {
		t.Fatalf("UserInfo 失败: %v", err)
	}
	if acc.TunnelQuota != 2 || acc.TrafficDayUsed != 2071158584 || acc.TrafficRemain != 12243133112 {
		t.Errorf("账号字段映射错误: %+v", acc)
	}
	if acc.Realname != "已实名" {
		t.Errorf("realname=2 应映射为已实名，实际 %q", acc.Realname)
	}

	tuns, err := c.Tunnels(ctx)
	if err != nil {
		t.Fatalf("Tunnels 失败: %v", err)
	}
	if tuns[0].Remote != "29377" || tuns[0].LocalPort != 22 || !tuns[0].Online {
		t.Errorf("隧道字段映射错误: %+v", tuns[0])
	}
	if !tuns[0].LockDelete || tuns[0].LockEdit {
		t.Errorf("locks 映射错误: %+v", tuns[0])
	}
}

func TestNatfrpNodeFlagDecoding(t *testing.T) {
	// 真实分布：flag&0b11 只有 0/1/2，从不出现 3。
	// 若按「两位都置位才允许 HTTP」的掩码语义解读，会把全网节点判成不支持建站。
	mux := http.NewServeMux()
	mux.HandleFunc("/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
		  "10":{"name":"国内可建站","host":"a.example","description":"","vip":3,"flag":46},
		  "73":{"name":"海外仅HTTP","host":"b.example","description":"","vip":0,"flag":37},
		  "2":{"name":"不可建站","host":"c.example","description":"","vip":0,"flag":44},
		  "99":{"name":"离线节点","host":"d.example","description":"","vip":0,"flag":558}
		}`))
	})
	mux.HandleFunc("/node/stats", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"time":1,"nodes":[{"id":10,"online":0,"uptime":100,"load":19.5}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	nodes, err := NewNatfrp(srv.URL, "tok").Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes 失败: %v", err)
	}
	byName := map[string]*Node{}
	for _, n := range nodes {
		byName[n.Name] = n
	}
	has := func(n *Node, cap string) bool {
		for _, c := range n.Caps {
			if c == cap {
				return true
			}
		}
		return false
	}

	if n := byName["国内可建站"]; !has(n, "https") || has(n, "http") {
		t.Errorf("flag=46 (bit1) 应解出 https 建站能力: %v", n.Caps)
	}
	if n := byName["海外仅HTTP"]; !has(n, "http") || has(n, "https") {
		t.Errorf("flag=37 (bit0) 应解出 http 建站能力: %v", n.Caps)
	}
	if n := byName["不可建站"]; has(n, "http") || has(n, "https") {
		t.Errorf("flag=44 (低位为 0) 不应有任何建站能力: %v", n.Caps)
	}
	if n := byName["国内可建站"]; !n.Online || n.Load != 19.5 {
		t.Errorf("节点状态未与 /node/stats 合并: online=%v load=%v", n.Online, n.Load)
	}
	if n := byName["离线节点"]; n.Online {
		t.Error("flag 含 1<<9 应判为离线")
	}
}

func TestChmlfrpNodeCaps(t *testing.T) {
	// 平台用字符串 "yes"/"true" 表达布尔能力，yes() 必须都能认
	mux := http.NewServeMux()
	mux.HandleFunc("/node", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"state":"success","data":[
		  {"id":9,"name":"中国香港","area":"中国香港","nodegroup":"vip","china":"yes",
		   "web":"yes","udp":"true","fangyu":"false","notes":"","ipv6":true},
		  {"id":17,"name":"云南电信","area":"中国云南昆明","nodegroup":"vip","china":"yes",
		   "web":"no","udp":"true","fangyu":"true","notes":"","ipv6":false}
		]}`))
	})
	mux.HandleFunc("/node_stats", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"state":"success","data":[
		  {"id":9,"node_name":"中国香港","state":"online","bandwidth_usage_percent":4}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	nodes, err := NewChmlfrp(srv.URL, "tok").Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes 失败: %v", err)
	}
	byName := map[string]*Node{}
	for _, n := range nodes {
		byName[n.Name] = n
	}
	has := func(n *Node, c string) bool {
		for _, x := range n.Caps {
			if x == c {
				return true
			}
		}
		return false
	}
	if n := byName["中国香港"]; !has(n, "web") || !has(n, "udp") || !has(n, "mainland") || !has(n, "ipv6") || !has(n, "vip") {
		t.Errorf("中国香港 能力解析不全: %v", n.Caps)
	}
	if n := byName["中国香港"]; has(n, "defense") {
		t.Errorf("fangyu=false 不应标记防御: %v", n.Caps)
	}
	if n := byName["云南电信"]; has(n, "web") {
		t.Errorf("web=no 不应标记建站: %v", n.Caps)
	}
	if n := byName["云南电信"]; !has(n, "defense") {
		t.Errorf("fangyu=true 应标记防御: %v", n.Caps)
	}
	if n := byName["中国香港"]; !n.Online || n.Load != 4 {
		t.Errorf("节点状态未合并: online=%v load=%v", n.Online, n.Load)
	}
}

func TestNormalizeRealname(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "未知"},
		{true, "已实名"},
		{false, "未实名"},
		{"已实名", "已实名"},
		{"", "未知"},
	}
	for _, c := range cases {
		if got := normalizeRealname(c.in); got != c.want {
			t.Errorf("normalizeRealname(%v) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestTrafficPointSorting(t *testing.T) {
	// Sakura 的 {时间戳:字节} 映射是无序的，必须按数值升序（字符串序会把
	// "999999" 排在 "1000000" 之后）
	pts := []TrafficPoint{{Label: "999999"}, {Label: "1000000"}, {Label: "2"}}
	sortTrafficPoints(pts)
	want := []string{"2", "999999", "1000000"}
	for i, w := range want {
		if pts[i].Label != w {
			t.Fatalf("排序错误: %v", pts)
		}
	}
}
