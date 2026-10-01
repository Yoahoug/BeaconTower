package frp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// 真机联调（默认跳过）。两个平台都需要真实账号，用于验证字段映射与鉴权链路：
//
//	BT_NATFRP_TOKEN   面板 →「查看访问密钥」取得
//	BT_CHMLFRP_LIVE=1 跑设备码流程（会打印授权链接，需人工在浏览器确认）
//
//	go test ./internal/frp/ -run TestLive -v -timeout 600s
//
// ChmlFrp 的令牌会缓存到 /tmp/bt_chmlfrp_token.json（含 refresh_token），
// 后续再跑直接复用并自动刷新，不必反复授权。

const chmlTokenCache = "/tmp/bt_chmlfrp_token.json"

type cachedToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// chmlToken 取可用令牌：缓存命中且未临近过期直接复用，否则用 refresh_token 续期，
// 都没有才走设备码（需要人工点授权）。
func chmlToken(t *testing.T, flow *DeviceFlow) *Token {
	t.Helper()
	ctx := context.Background()
	if b, err := os.ReadFile(chmlTokenCache); err == nil {
		var c cachedToken
		if json.Unmarshal(b, &c) == nil {
			if c.AccessToken != "" && time.Now().Unix() < c.ExpiresAt-120 {
				t.Logf("复用缓存令牌（%ds 后过期）", c.ExpiresAt-time.Now().Unix())
				return &Token{AccessToken: c.AccessToken, RefreshToken: c.RefreshToken, ExpiresIn: int(c.ExpiresAt - time.Now().Unix())}
			}
			if c.RefreshToken != "" {
				if tok, err := RefreshChmlfrpToken(ctx, "", c.RefreshToken); err == nil {
					saveChmlToken(t, tok)
					t.Logf("用 refresh_token 续期成功，新令牌 %ds 有效", tok.ExpiresIn)
					return tok
				} else {
					t.Logf("续期失败（%v），回退到设备码授权", err)
				}
			}
		}
	}
	if os.Getenv("BT_CHMLFRP_LIVE") == "" {
		t.Skip("无缓存令牌且未设置 BT_CHMLFRP_LIVE=1：跳过 ChmlFrp 真机联调")
	}

	s, err := flow.Start(ctx, 0)
	if err != nil {
		t.Fatalf("发起设备码失败: %v", err)
	}
	fmt.Printf("\n========== 请在浏览器打开并确认授权 ==========\n  %s\n  用户码: %s\n=============================================\n\n",
		s.VerifyURL, s.UserCode)

	deadline := time.Now().Add(time.Duration(s.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		cur, err := flow.Poll(ctx, s.ID)
		if err != nil {
			t.Fatalf("轮询失败: %v", err)
		}
		if cur.Status == "ok" {
			tok := flow.Token(s.ID)
			saveChmlToken(t, tok)
			fmt.Printf(">>> 授权成功，令牌已缓存到 %s\n", chmlTokenCache)
			return tok
		}
		if cur.Status != "pending" {
			t.Fatalf("授权失败: %s %s", cur.Status, cur.Error)
		}
	}
	t.Fatal("授权超时")
	return nil
}

func saveChmlToken(t *testing.T, tok *Token) {
	t.Helper()
	if tok == nil {
		return
	}
	b, _ := json.Marshal(cachedToken{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, ExpiresAt: tok.ExpiresAt()})
	if err := os.WriteFile(chmlTokenCache, b, 0o600); err != nil {
		t.Logf("缓存令牌失败: %v", err)
	}
}

// ---------- 原始响应形状 dump ----------
// 目的：平台各端点的包装外壳不统一（有的 {"msg","code","data"}，有的裸数组），
// 靠文档推断会写出错误的解包逻辑。一次性把每个端点的真实形状打出来。

var secretKeys = regexp.MustCompile(`"(apitoken|nodetoken|usertoken|password|access_token|refresh_token)"\s*:\s*"[^"]*"`)

func redact(s string) string {
	return secretKeys.ReplaceAllString(s, `"$1":"***"`)
}

func shape(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(trimmed, "["):
		var arr []json.RawMessage
		if json.Unmarshal([]byte(trimmed), &arr) == nil {
			return fmt.Sprintf("裸数组[%d]", len(arr))
		}
		return "裸数组(解析失败)"
	case strings.HasPrefix(trimmed, "{"):
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(trimmed), &m) == nil {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			return "对象{" + strings.Join(keys, ",") + "}"
		}
		return "对象(解析失败)"
	default:
		return "文本"
	}
}

func dumpEndpoint(t *testing.T, ctx context.Context, token, label string, method, rawURL string, body io.Reader) {
	t.Helper()
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+token)
	if body != nil {
		hdr.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	raw, status, err := request(ctx, newHTTPClient(20*time.Second), method, rawURL, hdr, body)
	if err != nil {
		t.Errorf("%-28s 请求失败: %v", label, err)
		return
	}
	s := redact(string(raw))
	t.Logf("%-28s HTTP %d  %s\n    %s", label, status, shape(s), head(s, 420))
}

func TestLiveChmlfrpRawShapes(t *testing.T) {
	if _, err := os.Stat(chmlTokenCache); err != nil && os.Getenv("BT_CHMLFRP_LIVE") == "" {
		t.Skip("无缓存令牌且未设置 BT_CHMLFRP_LIVE=1：跳过")
	}
	flow := NewDeviceFlow("")
	tok := chmlToken(t, flow)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	base := ChmlfrpBase
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /userinfo", http.MethodGet, base+"/userinfo", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /tunnel", http.MethodGet, base+"/tunnel", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /node", http.MethodGet, base+"/node", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /node_stats", http.MethodGet, base+"/node_stats", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /flow_last_7_days", http.MethodGet, base+"/flow_last_7_days", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /get_user_free_subdomains", http.MethodGet, base+"/get_user_free_subdomains", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /list_available_domains", http.MethodGet, base+"/list_available_domains", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /panelinfo", http.MethodGet, base+"/panelinfo", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /qiandao_info", http.MethodGet, base+"/qiandao_info", nil)
	dumpEndpoint(t, ctx, tok.AccessToken, "GET /node_uptime?time=7", http.MethodGet, base+"/node_uptime?time=7", nil)

	// 需要隧道/节点参数的两个接口，先取一条真实数据再打
	c := NewChmlfrp("", tok.AccessToken)
	if tuns, err := c.Tunnels(ctx); err == nil && len(tuns) > 0 {
		tn := tuns[0]
		dumpEndpoint(t, ctx, tok.AccessToken, "GET /tunnel/last7days",
			http.MethodGet, base+"/tunnel/last7days?tunnel_id="+url.QueryEscape(tn.RemoteID), nil)
		dumpEndpoint(t, ctx, tok.AccessToken, "GET /tunnel_config",
			http.MethodGet, base+"/tunnel_config?node="+url.QueryEscape(tn.NodeName)+"&tunnel_names="+url.QueryEscape(tn.Name), nil)
		dumpEndpoint(t, ctx, tok.AccessToken, "GET /refresh_tunnel(form)",
			http.MethodPost, base+"/refresh_tunnel", strings.NewReader(url.Values{"tunnel_name": {tn.Name}}.Encode()))
	} else if err != nil {
		t.Errorf("Tunnels 失败，跳过依赖隧道的接口: %v", err)
	}
	if nodes, err := c.Nodes(ctx); err == nil && len(nodes) > 0 {
		dumpEndpoint(t, ctx, tok.AccessToken, "GET /node_status_info",
			http.MethodGet, base+"/node_status_info?nodename="+url.QueryEscape(nodes[0].Name), nil)
	}
}

// ---------- 归一化映射验证 ----------

func TestLiveChmlfrp(t *testing.T) {
	if _, err := os.Stat(chmlTokenCache); err != nil && os.Getenv("BT_CHMLFRP_LIVE") == "" {
		t.Skip("无缓存令牌且未设置 BT_CHMLFRP_LIVE=1：跳过")
	}
	flow := NewDeviceFlow("")
	tok := chmlToken(t, flow)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	c := NewChmlfrp("", tok.AccessToken)

	acc, err := c.UserInfo(ctx)
	if err != nil {
		t.Fatalf("UserInfo 失败: %v", err)
	}
	t.Logf("账号: uid=%s name=%s group=%s 限速=%s 隧道已用=%d 上限=%d 连接数=%d 上传=%d 下载=%d 实名=%s",
		acc.UID, acc.Username, acc.GroupName, acc.SpeedLimit,
		acc.TunnelUsed, acc.TunnelQuota, acc.Conns, acc.TrafficUp, acc.TrafficDown, acc.Realname)
	t.Logf("Extra: %+v", acc.Extra)

	tuns, err := c.Tunnels(ctx)
	if err != nil {
		t.Fatalf("Tunnels 失败: %v", err)
	}
	t.Logf("隧道 %d 条", len(tuns))
	for i, tn := range tuns {
		if i >= 6 {
			break
		}
		t.Logf("  #%s %q proto=%s node=%q online=%v conns=%d local=%s:%d remote=%s up=%d down=%d ver=%s extra=%q reason=%q",
			tn.RemoteID, tn.Name, tn.Proto, tn.NodeName, tn.Online, tn.Conns,
			tn.LocalIP, tn.LocalPort, tn.Remote, tn.TodayUp, tn.TodayDown, tn.ClientVer, tn.Extra, tn.StatusReason)
	}
	if len(tuns) > 0 {
		tn := tuns[0]
		if cfg, err := c.TunnelConfig(ctx, tn.NodeName, []string{tn.Name}); err != nil {
			t.Errorf("TunnelConfig 失败: %v", err)
		} else {
			t.Logf("隧道 #%s 的 frpc 配置:\n%s", tn.RemoteID, head(cfg, 400))
		}
		if pts, err := c.TunnelLast7Days(ctx, tn.RemoteID); err != nil {
			t.Errorf("TunnelLast7Days 失败: %v", err)
		} else {
			t.Logf("隧道 #%s 近 7 日流量：%d 点 末点=%+v", tn.RemoteID, len(pts), last(pts))
		}
	}

	nodes, err := c.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes 失败: %v", err)
	}
	t.Logf("节点 %d 个", len(nodes))
	for i, n := range nodes {
		if i >= 6 {
			break
		}
		t.Logf("  #%s %s area=%s group=%s caps=%v online=%v load=%.1f",
			n.RemoteID, n.Name, n.Area, n.GroupName, n.Caps, n.Online, n.Load)
	}

	if pts, err := c.FlowLast7Days(ctx); err != nil {
		t.Errorf("FlowLast7Days 失败: %v", err)
	} else {
		t.Logf("账号近 7 日流量：%d 点 末点=%+v", len(pts), last(pts))
	}
	if subs, err := c.Subdomains(ctx); err != nil {
		t.Errorf("Subdomains 失败: %v", err)
	} else {
		t.Logf("免费二级域名 %d 条", len(subs))
	}
	if doms, err := c.AvailableDomains(ctx); err != nil {
		t.Errorf("AvailableDomains 失败: %v", err)
	} else {
		t.Logf("可用主域名 %d 个", len(doms))
		for i, d := range doms {
			if i >= 3 {
				break
			}
			b, _ := json.Marshal(d)
			t.Logf("  %s", head(string(b), 200))
		}
	}
	// 刷新链路必须能跑通，且**新令牌要立刻持久化**：实测 refresh_token 是一次性的，
	// 刷新后旧 access_token 立即失效、refresh_token 同时轮换，
	// 不保存新值就等于把链路掐断（面板运行中同理）。
	if nt, err := RefreshChmlfrpToken(ctx, "", tok.RefreshToken); err != nil {
		t.Errorf("刷新令牌失败（长期运行会失联）: %v", err)
	} else {
		saveChmlToken(t, nt)
		t.Logf("刷新令牌链路 ✓（新令牌 %ds 有效，已回写缓存）", nt.ExpiresIn)
	}
}

// ---------- Sakura ----------

func TestLiveNatfrp(t *testing.T) {
	token := os.Getenv("BT_NATFRP_TOKEN")
	if token == "" {
		if b, err := os.ReadFile("/tmp/bt_natfrp_token"); err == nil {
			token = strings.TrimSpace(string(b))
		}
	}
	if token == "" {
		t.Skip("未设置 BT_NATFRP_TOKEN 且 /tmp/bt_natfrp_token 不存在：跳过 Sakura 真机联调")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c := NewNatfrp("", token)

	acc, err := c.UserInfo(ctx)
	if err != nil {
		t.Fatalf("UserInfo 失败: %v", err)
	}
	t.Logf("账号: uid=%s name=%s group=%s 限速=%s 隧道上限=%d 今日用量=%d 剩余=%d 实名=%s",
		acc.UID, acc.Username, acc.GroupName, acc.SpeedLimit,
		acc.TunnelQuota, acc.TrafficDayUsed, acc.TrafficRemain, acc.Realname)
	t.Logf("Extra: %+v", acc.Extra)

	tuns, err := c.Tunnels(ctx)
	if err != nil {
		t.Fatalf("Tunnels 失败: %v", err)
	}
	t.Logf("隧道 %d 条", len(tuns))
	for i, tn := range tuns {
		if i >= 6 {
			break
		}
		t.Logf("  #%s %q proto=%s node=%s online=%v status=%s reason=%q local=%s:%d remote=%s locks(edit=%v,del=%v,mig=%v) extra=%q",
			tn.RemoteID, tn.Name, tn.Proto, tn.NodeID, tn.Online, tn.Status, tn.StatusReason,
			tn.LocalIP, tn.LocalPort, tn.Remote, tn.LockEdit, tn.LockDelete, tn.LockMigrate, tn.Extra)
	}
	if len(tuns) > 0 {
		if cfg, err := c.TunnelConfig(ctx, tuns[0].RemoteID, ""); err != nil {
			t.Errorf("TunnelConfig 失败: %v", err)
		} else {
			t.Logf("隧道 #%s 的 frpc 配置前 400 字符:\n%s", tuns[0].RemoteID, head(cfg, 400))
		}
		if tr, err := c.TunnelTraffic(ctx, tuns[0].RemoteID); err != nil {
			t.Errorf("TunnelTraffic 失败: %v", err)
		} else {
			t.Logf("隧道 #%s 流量点数 %d", tuns[0].RemoteID, len(tr))
		}
	}

	nodes, err := c.Nodes(ctx)
	if err != nil {
		t.Fatalf("Nodes 失败: %v", err)
	}
	online := 0
	for _, n := range nodes {
		if n.Online {
			online++
		}
	}
	t.Logf("节点 %d 个（在线 %d）", len(nodes), online)
	for i, n := range nodes {
		if i >= 8 {
			break
		}
		t.Logf("  #%s %s host=%s group=%s caps=%v online=%v load=%.1f uptime=%d",
			n.RemoteID, n.Name, n.Host, n.GroupName, n.Caps, n.Online, n.Load, n.Uptime)
	}

	for _, kind := range []string{"day", "week", "month"} {
		pts, err := c.TrafficHistory(ctx, kind)
		if err != nil {
			t.Errorf("TrafficHistory(%s) 失败: %v", kind, err)
			continue
		}
		t.Logf("流量历史 %s：%d 点，末点=%+v", kind, len(pts), last(pts))
	}
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func last(pts []TrafficPoint) TrafficPoint {
	if len(pts) == 0 {
		return TrafficPoint{}
	}
	return pts[len(pts)-1]
}
