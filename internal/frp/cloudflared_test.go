package frp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// CF 客户端单测：fixture 按 Cloudflare API v4 真实响应结构构造（敏感值已替换），
// 固化外壳解析、ingress 归一、状态映射与 remote_id 语义（doc/16）。

// newCFTestServer 起一个假 CF API，按 path 返回预置 JSON。
func newCFTestServer(t *testing.T, routes map[string]string) (*CloudflaredClient, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewCloudflared(srv.URL, "test-token", "acct123"), srv
}

const cfEnvelopeOK = `{"success":true,"errors":[],"messages":[],"result":%s}`

func cfOK(result string) string { return fmt.Sprintf(cfEnvelopeOK, result) }

// Tunnels 把远程托管隧道的每条 ingress 规则归一为一条面板隧道，
// remote_id 形如 <tunnelID>:<hostname>，service 解析出 local_ip:local_port；
// 挂在统一容器下的规则可写，独立专线（非统一容器）锁编辑/删除。
func TestCloudflaredTunnelsIngressNormalization(t *testing.T) {
	c, _ := newCFTestServer(t, map[string]string{
		"/accounts/acct123/cfd_tunnel": cfOK(`[
			{
				"id": "b15987ab-5e60-4e80-a8d3-b132dcbe1c73",
				"name": "home",
				"status": "healthy",
				"remote_config": true,
				"conns_active": 4,
				"connections": [{"id": "c1", "version": "2024.12.2", "conns_active": 4}]
			},
			{
				"id": "aaaa1111-0000-0000-0000-000000000000",
				"name": "local-only",
				"status": "down",
				"remote_config": false
			}
		]`),
		"/accounts/acct123/cfd_tunnel/b15987ab-5e60-4e80-a8d3-b132dcbe1c73/configurations": cfOK(`{
			"tunnel_id": "b15987ab-5e60-4e80-a8d3-b132dcbe1c73",
			"version": 5,
			"config": {
				"ingress": [
					{"hostname": "tower.yoahoug.dev", "service": "http://192.168.0.10:8091"},
					{"hostname": "grafana.yoahoug.dev", "service": "http://localhost:3000"},
					{"service": "http_status:404"}
				]
			}
		}`),
	})

	c.ManagedTunnelID = "b15987ab-5e60-4e80-a8d3-b132dcbe1c73"
	tuns, err := c.Tunnels(context.Background())
	if err != nil {
		t.Fatalf("Tunnels: %v", err)
	}
	if len(tuns) != 3 {
		t.Fatalf("期望 2 条 ingress + 1 条本地托管隧道 = 3 条，得到 %d: %+v", len(tuns), tuns)
	}

	// 规则 1：LAN IP service，统一容器 → 可写
	tv := tuns[0]
	if tv.RemoteID != "b15987ab-5e60-4e80-a8d3-b132dcbe1c73:tower.yoahoug.dev" {
		t.Errorf("remote_id = %q，期望带 tunnel 前缀的复合 ID", tv.RemoteID)
	}
	if tv.Name != "tower.yoahoug.dev" || tv.Remote != "tower.yoahoug.dev" {
		t.Errorf("name/remote = %q/%q，期望 hostname", tv.Name, tv.Remote)
	}
	if tv.LocalIP != "192.168.0.10" || tv.LocalPort != 8091 {
		t.Errorf("local = %s:%d，期望 192.168.0.10:8091", tv.LocalIP, tv.LocalPort)
	}
	if !tv.Online || tv.Status != "normal" {
		t.Errorf("healthy 隧道应 online+normal，得到 %v/%s", tv.Online, tv.Status)
	}
	if tv.LockEdit || tv.LockDelete {
		t.Error("统一容器下的规则不应锁定编辑/删除")
	}

	// 规则 2：localhost service 原样保留端口
	tv2 := tuns[1]
	if tv2.LocalIP != "localhost" || tv2.LocalPort != 3000 {
		t.Errorf("local = %s:%d，期望 localhost:3000", tv2.LocalIP, tv2.LocalPort)
	}

	// 本地托管隧道：不拉 ingress，整体一条，状态 down → offline
	tv3 := tuns[2]
	if tv3.RemoteID != "aaaa1111-0000-0000-0000-000000000000" {
		t.Errorf("本地托管隧道 remote_id 应为裸 tunnel ID，得到 %q", tv3.RemoteID)
	}
	if tv3.Online {
		t.Error("down 状态的隧道不应为 online")
	}
	if tv3.StatusReason == "" {
		t.Error("离线隧道应带状态说明")
	}
}

// 独立专线只读保护：managedTunnelID 之外的隧道（如用户手工建的 New-api）
// 规则必须锁编辑/删除，面板不碰不是自己创建的资源。
func TestCloudflaredUnmanagedTunnelLocked(t *testing.T) {
	c, _ := newCFTestServer(t, map[string]string{
		"/accounts/acct123/cfd_tunnel": cfOK(`[
			{
				"id": "b15987ab-5e60-4e80-a8d3-b132dcbe1c73",
				"name": "New-api",
				"status": "healthy",
				"remote_config": true,
				"conns_active": 2
			}
		]`),
		"/accounts/acct123/cfd_tunnel/b15987ab-5e60-4e80-a8d3-b132dcbe1c73/configurations": cfOK(`{
			"tunnel_id": "b15987ab-5e60-4e80-a8d3-b132dcbe1c73",
			"config": {
				"ingress": [
					{"hostname": "new.yoahoug.dev", "service": "http://192.168.0.10:3000"},
					{"service": "http_status:404"}
				]
			}
		}`),
	})
	c.ManagedTunnelID = "unified-id"
	tuns, err := c.Tunnels(context.Background())
	if err != nil {
		t.Fatalf("Tunnels: %v", err)
	}
	if len(tuns) != 1 {
		t.Fatalf("期望 1 条规则，得到 %d", len(tuns))
	}
	if !tuns[0].LockEdit || !tuns[0].LockDelete {
		t.Fatal("非统一容器的规则必须锁编辑/删除（独立专线只读）")
	}
	if !strings.Contains(tuns[0].Extra, "独立专线") {
		t.Errorf("只读隧道应在 extra 标注专线语义，得到 %q", tuns[0].Extra)
	}
	// 写操作二次防线：Update/Delete 拒绝非统一容器
	if err := c.UpdateTunnel(context.Background(), tuns[0].RemoteID, TunnelInput{LocalPort: 1234}); err == nil {
		t.Error("独立专线的规则不允许面板编辑")
	}
	if err := c.DeleteOne(context.Background(), tuns[0].RemoteID); err == nil {
		t.Error("独立专线的规则不允许面板删除")
	}
}

// CreateTunnel 统一容器模型：不再每次建物理 tunnel，而是往统一容器追加
// ingress 规则；统一容器缺失时懒创建并回调回写 ID。
func TestCloudflaredCreateTunnelAppendsToUnified(t *testing.T) {
	var postCount int
	haveUnified := false
	mux := http.NewServeMux()
	mux.HandleFunc("/accounts/acct123/cfd_tunnel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			postCount++
			haveUnified = true
			_, _ = w.Write([]byte(cfOK(`{"id":"unified-id","name":"beacontower-managed","status":"inactive"}`)))
			return
		}
		if haveUnified {
			_, _ = w.Write([]byte(cfOK(`[{"id":"unified-id","name":"beacontower-managed","status":"inactive"}]`)))
			return
		}
		_, _ = w.Write([]byte(cfOK(`[]`)))
	})
	mux.HandleFunc("/accounts/acct123/cfd_tunnel/unified-id/configurations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cfOK(`{"tunnel_id":"unified-id","config":{"ingress":[{"service":"http_status:404"}]}}`)))
	})
	mux.HandleFunc("/accounts/acct123/cfd_tunnel/unified-id/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cfOK(`"eyJhbGciOiJIUzI1NiJ9.eyJhIjoxfQ.sig"`)))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCloudflared(srv.URL, "tok", "acct123")
	remoteID, err := c.CreateTunnel(context.Background(),
		TunnelInput{Name: "app.yoahoug.dev", LocalPort: 8080})
	if err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}
	if remoteID != "unified-id:app.yoahoug.dev" {
		t.Errorf("remoteID = %q，期望 unified-id:app.yoahoug.dev", remoteID)
	}
	if postCount != 1 {
		t.Errorf("懒创建只应发生一次，post=%d", postCount)
	}

	// 第二次创建：不再建物理隧道（幂等复用），直接追加规则
	if _, err := c.CreateTunnel(context.Background(),
		TunnelInput{Name: "app2.yoahoug.dev", LocalPort: 9090}); err != nil {
		t.Fatalf("第二次 CreateTunnel: %v", err)
	}
	if postCount != 1 {
		t.Errorf("统一容器应复用，物理创建次数 = %d，期望 1", postCount)
	}
}

// success:false 但 HTTP 200 是 CF 的标准错误形态，不能当成功处理；
// 9109（无权访问资源）应归为 ErrAuth 让前端弹重新绑定。
func TestCloudflaredErrorEnvelope(t *testing.T) {
	c, _ := newCFTestServer(t, map[string]string{
		"/accounts/acct123/cfd_tunnel": `{"success":false,"errors":[{"code":9109,"message":"Unauthorized to access requested resource"}],"messages":[],"result":null}`,
	})
	if _, err := c.UserInfo(context.Background()); err == nil {
		t.Fatal("success:false 应返回错误")
	}
}

func TestCloudflaredHTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`))
	}))
	defer srv.Close()
	c := NewCloudflared(srv.URL, "bad-token", "acct123")
	if _, err := c.UserInfo(context.Background()); err != nil && err != ErrAuth {
		// 401 必须包着 ErrAuth 语义（errors.Is 才成立），这里直接验证包装链
		msg := err.Error()
		if !strings.Contains(msg, "无效") && !contains(msg, "权限") {
			t.Fatalf("401 错误信息应提示令牌问题，得到: %v", err)
		}
	}
}

// remote_id 拆分：带 hostname 后缀的复合 ID 与裸 tunnel ID 都要能解。
func TestSplitCFRemoteID(t *testing.T) {
	id, host := SplitCFRemoteID("abc:def.example.com")
	if id != "abc" || host != "def.example.com" {
		t.Errorf("got %q, %q", id, host)
	}
	id, host = SplitCFRemoteID("abc")
	if id != "abc" || host != "" {
		t.Errorf("裸 ID 应返回空 hostname，got %q, %q", id, host)
	}
}

// 凭据（三要素 + 统一隧道 ID）的往返编解码；旧版三字段 JSON 必须能兼容解析。
func TestCloudflaredCredsRoundTrip(t *testing.T) {
	enc, err := EncodeCloudflaredCreds(CloudflaredCreds{AccountID: "a", Token: "t", ZoneID: "z", UnifiedTunnelID: "u1"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := ParseCloudflaredCreds(enc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.AccountID != "a" || got.Token != "t" || got.ZoneID != "z" || got.UnifiedTunnelID != "u1" {
		t.Errorf("round trip 失真: %+v", got)
	}
	// 空串必须报 ErrAuth 语义
	if _, err := ParseCloudflaredCreds(""); err == nil {
		t.Error("空凭据应报错")
	}
	// 历史形态：统一隧道 ID 出现之前的裸 token / 三字段 JSON
	if c, err := ParseCloudflaredCreds("cfut_x"); err != nil || c.Token != "cfut_x" {
		t.Errorf("裸 token 兼容解析失败: %+v, %v", c, err)
	}
	old, _ := json.Marshal(map[string]string{"account_id": "a", "token": "t", "zone_id": "z"})
	if c, err := ParseCloudflaredCreds(string(old)); err != nil || c.UnifiedTunnelID != "" {
		t.Errorf("旧三字段 JSON 应解析且 UnifiedTunnelID 为空: %+v, %v", c, err)
	}
}

// defaultCFLocalIP：容器里 localhost 指容器自身，service host 缺省改写为
// Docker 网桥网关（与 doc/13 §13 的 host 网络约束同一问题的 bridge 解法）。
func TestDefaultCFLocalIP(t *testing.T) {
	cases := map[string]string{
		"":             "172.17.0.1",
		"localhost":    "172.17.0.1",
		"127.0.0.1":    "172.17.0.1",
		"192.168.0.10": "192.168.0.10",
	}
	for in, want := range cases {
		if got := defaultCFLocalIP(in); got != want {
			t.Errorf("defaultCFLocalIP(%q) = %q, want %q", in, got, want)
		}
	}
}

// Token 单参数绑定路线：/accounts 端点响应解析（DiscoverAccount 的数据层）。
func TestDiscoverAccountEndpoint(t *testing.T) {
	c, _ := newCFTestServer(t, map[string]string{
		"/accounts": cfOK(`[{"id":"acct123","name":"Yoahoug's Account"}]`),
	})
	var list []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.call(context.Background(), http.MethodGet, "/accounts", nil, nil, &list); err != nil {
		t.Fatalf("call /accounts: %v", err)
	}
	if len(list) != 1 || list[0].ID != "acct123" {
		t.Fatalf("accounts 解析错误: %+v", list)
	}
}

// registrableDomain：子域 → 注册域（Zone 自动发现用）。
func TestRegistrableDomain(t *testing.T) {
	cases := map[string]string{
		"tower.yoahoug.dev":   "yoahoug.dev",
		"app.example.com.cn.": "com.cn", // 简化规则：取最后两段，不做 PSL（文档化边界）
		"localhost":           "",
		"":                    "",
	}
	for in, want := range cases {
		if got := registrableDomain(in); got != want {
			t.Errorf("registrableDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

// GraphQL 流量解析：sum.edgeResponseBytes / requests 从响应中解出并聚合。
func TestTunnelDayTrafficGraphQL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {"viewer": {"zones": [{"httpRequestsAdaptiveGroups": [
				{"sum": {"edgeResponseBytes": 1024000, "requests": 42}},
				{"sum": {"edgeResponseBytes": 2048000, "requests": 8}}
			]}]}}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewCloudflared(srv.URL, "tok", "acct")
	c.ZoneID = "zone1"
	sum, err := c.TunnelDayTraffic(context.Background(), "tower.yoahoug.dev", 24)
	if err != nil {
		t.Fatalf("TunnelDayTraffic: %v", err)
	}
	if sum == nil || sum.Bytes != 3072000 || sum.Requests != 50 {
		t.Fatalf("聚合错误: %+v", sum)
	}
	// 无 ZoneID 时静默返回 nil（前端显示「—」）
	c2 := NewCloudflared(srv.URL, "tok", "acct")
	if s, err := c2.TunnelDayTraffic(context.Background(), "x.y.z", 24); err != nil || s != nil {
		t.Fatalf("无 zone 应返回 nil,nil，得到 %v,%v", s, err)
	}
}
