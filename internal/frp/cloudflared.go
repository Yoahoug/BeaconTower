package frp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CloudflaredBase Cloudflare API v4 基址。
const CloudflaredBase = "https://api.cloudflare.com/client/v4"

// KindCloudflared 平台标识，与 frp_platform.kind 一致。
const KindCloudflared = "cloudflared"

// CloudflaredClient Cloudflare Tunnel（cfd_tunnel）API 客户端。
//
// 鉴权用 API Token（长期有效），随请求带三要素之一 account_id 定位资源；
// token 权限要求：Account → Cloudflare Tunnel → Edit + Zone → DNS → Edit
// （建隧道和写 CNAME 各一把锁，只读监控给 Read 即可）。
//
// 与前两个平台不同的两个协议特性：
//  1. 外壳 {"success":false,"errors":[{code,message}]}——错误也可能配 HTTP 200，
//     只看状态码不行；403/9109 等鉴权错误统一归 ErrAuth；
//  2. 隧道即服务：一条 cfd_tunnel（= connector 进程）可承载任意条 ingress 规则。
//     面板把「一条 ingress 规则 = 一条隧道」归一给上层（name=hostname、
//     remote=hostname、local_ip:local_port 解析自 service），这样隧道列表、
//     游客看板、托管容器三条既有链路零改动接入。
//
// 凭据序列化成 JSON（account_id/api_token/zone_id）后整体走 TokenEnc 加密落库，
// 复用现有 frp_platform 的凭据字段，零 schema 变更。
type CloudflaredClient struct {
	Base      string
	Token     string
	AccountID string
	ZoneID    string
	// ManagedTunnelID 统一容器（面板专属物理隧道）ID，Runner 从凭据注入。
	// 空值时所有规则按只读展示（写操作会被拒绝）。
	ManagedTunnelID string
	hc              *http.Client
}

// CloudflaredCreds Cloudflare 平台的凭据与托管资源标识（加密前的明文形态）。
// UnifiedTunnelID 是面板专属的「统一容器」物理隧道 ID：面板创建的所有节点
// （ingress 规则）都挂在这条隧道下，连接器容器也只为它启动。首次建节点时
// 懒创建并回写进凭据（与 AccountID 的自动发现同一管线）。
type CloudflaredCreds struct {
	AccountID       string `json:"account_id"`
	Token           string `json:"token"`
	ZoneID          string `json:"zone_id"`
	UnifiedTunnelID string `json:"unified_tunnel_id,omitempty"`
}

// ManagedTunnelName 面板统一容器的物理隧道名（每账号一条，懒创建）。
const ManagedTunnelName = "beacontower-managed"

func NewCloudflared(base, token, accountID string) *CloudflaredClient {
	if strings.TrimSpace(base) == "" {
		base = CloudflaredBase
	}
	return &CloudflaredClient{
		Base:      strings.TrimRight(base, "/"),
		Token:     token,
		AccountID: accountID,
		hc:        newHTTPClient(20 * time.Second),
	}
}

// EncodeCloudflaredCreds 三要素序列化（Runner 落库前调用）。
func EncodeCloudflaredCreds(c CloudflaredCreds) (string, error) {
	if c.Token == "" {
		return "", errors.New("Cloudflare 凭据缺少 API Token")
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ParseCloudflaredCreds 从 TokenEnc 解出三要素。兼容只存裸 token 的历史形态
// （这种形态没有 account_id，会在下一次 API 调用处报错提示重新绑定）。
func ParseCloudflaredCreds(s string) (*CloudflaredCreds, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w（未保存 Cloudflare 凭据）", ErrAuth)
	}
	if !strings.HasPrefix(s, "{") {
		return &CloudflaredCreds{Token: s}, nil
	}
	var c CloudflaredCreds
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, fmt.Errorf("Cloudflare 凭据损坏，请重新绑定: %w", err)
	}
	return &c, nil
}

// DiscoverAccount 用 token 自动发现 Account ID。实测 cfut_ 新版用户 Token
// 可能没有 GET /accounts 的可见性（返回空列表但 success=true），而 zone 的
// account 字段始终带 account id——因此先走 /zones 反查，空了才退回 /accounts
// （兼容纯账户级 Token）。两者都拿不到时给出可操作的指引。
func DiscoverAccount(ctx context.Context, token string) (string, []string, error) {
	c := NewCloudflared("", token, "")
	// 路线 1：zone → account.id（cfut_ Token 实测可用）
	var zones []struct {
		Name    string `json:"name"`
		Account struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"account"`
	}
	if err := c.call(ctx, http.MethodGet, "/zones", url.Values{"per_page": {"50"}}, nil, &zones); err == nil {
		for _, z := range zones {
			if z.Account.ID != "" {
				return z.Account.ID, []string{z.Account.Name}, nil
			}
		}
	}
	// 路线 2：纯账户级 Token 走 /accounts
	var accounts []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.call(ctx, http.MethodGet, "/accounts", url.Values{"per_page": {"5"}}, nil, &accounts); err != nil {
		return "", nil, err
	}
	if len(accounts) == 0 {
		return "", nil, errors.New("该 API Token 看不到任何账户或域名：请检查 Token 是否已授权（Account → Cloudflare Tunnel / Zone → DNS），并在「区域资源」中包含你的域名")
	}
	names := make([]string, 0, len(accounts))
	for _, a := range accounts {
		names = append(names, a.Name)
	}
	return accounts[0].ID, names, nil
}

// DiscoverZone 自动发现 Zone ID：按 hostname 的注册域精确查询（app.example.com →
// example.com）。**不做**"退回账户下第一个 zone"的兜底——token 可见多个域名时
// 那会把 CNAME 写进错误的 zone（E2E 实测踩坑：bt-e2e.yoahoug.dev 被写进
// ahhhahh.cc.cd）。查不到 / 无 DNS 权限时返回空，写 CNAME 的能力静默降级
// （面板提示手动补记录），绝不建错。
func (c *CloudflaredClient) DiscoverZone(ctx context.Context, hostname string) (string, error) {
	host := registrableDomain(hostname)
	if host == "" {
		return "", nil
	}
	q := url.Values{"name": {host}, "per_page": {"50"}}
	var list []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.call(ctx, http.MethodGet, "/zones", q, nil, &list); err != nil {
		return "", err // 无 Zone Read 权限时静默降级
	}
	for _, z := range list {
		if z.Name == host {
			return z.ID, nil
		}
	}
	return "", nil
}

// registrableDomain 取注册域（最后两段；不做 PSL，个人域名场景足够）。
func registrableDomain(hostname string) string {
	parts := strings.Split(strings.TrimSuffix(strings.ToLower(hostname), "."), ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "." + parts[len(parts)-1]
}

// cfEnvelope Cloudflare v4 统一响应外壳。errors 非空即业务失败。
type cfEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Messages []string        `json:"messages"`
	Result   json.RawMessage `json:"result"`
}

// cfError 把第一条错误转成 apiError；鉴权类错误码归 ErrAuth。
// 已知鉴权相关码：10000 认证失败、9109 无权限访问该资源、403 HTTP 状态。
func (c *CloudflaredClient) call(ctx context.Context, method, path string, query url.Values, body []byte, out any) error {
	u := c.Base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+c.Token)
	var reader *strings.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
		hdr.Set("Content-Type", "application/json")
	} else {
		reader = strings.NewReader("")
	}
	raw, status, err := request(ctx, c.hc, method, u, hdr, reader)
	if err != nil {
		return fmt.Errorf("连接 Cloudflare 失败: %w", err)
	}
	var env cfEnvelope
	if err := decodeJSON(raw, &env); err != nil {
		// 非 JSON（网关错误页等）按状态码造错
		return &apiError{Platform: "Cloudflare", Status: status, Msg: trimErr(string(raw))}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("%w（Cloudflare API Token 无效或权限不足）", ErrAuth)
	}
	if !env.Success || len(env.Errors) > 0 {
		first := env.Errors[0]
		msg := first.Message
		if msg == "" {
			msg = trimErr(string(raw))
		}
		if first.Code == 10000 || first.Code == 9109 {
			return fmt.Errorf("%w（Cloudflare API Token 无效或权限不足）", ErrAuth)
		}
		return &apiError{Platform: "Cloudflare", Status: status, Code: first.Code, Msg: msg}
	}
	if out != nil && len(env.Result) > 0 {
		return decodeJSON(env.Result, out)
	}
	return nil
}

// ---------- 流量（GraphQL Analytics） ----------

// gqlPOST 调 GraphQL Analytics 端点（错误在 body 的 errors 数组里，HTTP 恒 200）。
func (c *CloudflaredClient) gqlPOST(ctx context.Context, query string, vars map[string]any, out any) error {
	payload, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/graphql", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("连接 Cloudflare Analytics 失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var gql struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := decodeJSON(raw, &gql); err != nil {
		return &apiError{Platform: "Cloudflare", Status: resp.StatusCode, Msg: trimErr(string(raw))}
	}
	if len(gql.Errors) > 0 {
		return &apiError{Platform: "Cloudflare", Status: resp.StatusCode, Msg: gql.Errors[0].Message}
	}
	if out != nil && len(gql.Data) > 0 {
		return decodeJSON(gql.Data, out)
	}
	return nil
}

// TunnelDayTraffic 按 hostname 查近 N 小时（默认 24h）经 CF 边缘的进出流量。
// 数据源 httpRequestsAdaptiveGroups（官方确认全套餐开放的基础数据集）：
// sum{edgeResponseBytes} 是边缘回源字节数（≈隧道下行到源站的量），requests 计数
// 仅作展示。CF 不区分隧道维度，这里以 ingress hostname = clientRequestHTTPHost
// 过滤——与面板「一条 ingress 规则 = 一条隧道」的归一严格对齐。
// 无 Zone ID 或查询失败时返回 nil（上层显示「—」），不视为错误。
func (c *CloudflaredClient) TunnelDayTraffic(ctx context.Context, hostname string, hours int) (*TunnelTrafficSummary, error) {
	if c.ZoneID == "" || hostname == "" {
		return nil, nil
	}
	if hours <= 0 || hours > 168 {
		hours = 24
	}
	now := time.Now().UTC().Truncate(time.Hour)
	filter := map[string]any{
		"datetime_geq":         now.Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339),
		"datetime_lt":          now.Add(time.Hour).Format(time.RFC3339),
		"clientRequestHTTPHost": hostname,
	}
	const q = `query TunnelTraffic($zoneTag: string, $filter: filter) {
		viewer { zones(filter: {zoneTag: $zoneTag}) {
			httpRequestsAdaptiveGroups(limit: 10000, filter: $filter) {
				sum { edgeResponseBytes requests }
			}
		} }
	}`
	var out struct {
		Viewer struct {
			Zones []struct {
				Groups []struct {
					Sum struct {
						EdgeResponseBytes int64 `json:"edgeResponseBytes"`
						Requests          int64 `json:"requests"`
					} `json:"sum"`
				} `json:"httpRequestsAdaptiveGroups"`
			} `json:"zones"`
		} `json:"viewer"`
	}
	vars := map[string]any{"zoneTag": c.ZoneID, "filter": filter}
	if err := c.gqlPOST(ctx, q, vars, &out); err != nil {
		return nil, err
	}
	s := &TunnelTrafficSummary{}
	for _, z := range out.Viewer.Zones {
		for _, g := range z.Groups {
			s.Bytes += g.Sum.EdgeResponseBytes
			s.Requests += g.Sum.Requests
		}
	}
	return s, nil
}

// TunnelTrafficSummary 单条 ingress 域名的流量聚合。
type TunnelTrafficSummary struct {
	Bytes    int64 `json:"bytes"`
	Requests int64 `json:"requests"`
}

// Verify 验活 + 取账号画像：GET /user 需要 Token 的 User Details 权限，
// 实践中 Tunnel Token 未必有；因此验活直接调 cfd_tunnel 列表（账户级），
// 再尝试补用户邮箱（失败不算错）。
func (c *CloudflaredClient) Verify(ctx context.Context) (string, error) {
	if c.AccountID == "" {
		return "", errors.New("缺少 Cloudflare account_id")
	}
	var tunnels []cfTunnel
	if err := c.call(ctx, http.MethodGet, "/accounts/"+c.AccountID+"/cfd_tunnel", url.Values{"is_deleted": {"false"}}, nil, &tunnels); err != nil {
		return "", err
	}
	var u struct {
		Email string `json:"email"`
		ID    string `json:"id"`
	}
	_ = c.call(ctx, http.MethodGet, "/user", nil, nil, &u)
	return u.Email, nil
}

// UserInfo 实现 platform 接口。CF 没有配额/流量概念，画像字段保持空，
// TunnelQuota 置 -1 让前端区分「无上限」与「0 条」（-1 在视图层渲染为不限）。
func (c *CloudflaredClient) UserInfo(ctx context.Context) (*Account, error) {
	email, err := c.Verify(ctx)
	if err != nil {
		return nil, err
	}
	return &Account{
		// UID 回填 Account ID：Sync 会把 acc.UID 写进平台画像，
		// 不补上绑定时的 account_id 每轮同步都会被清空
		UID:         c.AccountID,
		Username:    email,
		TunnelQuota: -1,
		Extra: map[string]any{
			"plan": "cloudflare-tunnel",
		},
	}, nil
}

// ---------- 隧道 ----------

// cfTunnel /cfd_tunnel 列表项。connections 在列表接口返回的是连接摘要
// （client_id/client_version/origin_ip 等），数量即 connector 边缘连接数。
type cfTunnel struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"` // inactive | degraded | healthy | down
	CreatedAt     time.Time  `json:"created_at"`
	DeletedAt     *time.Time `json:"deleted_at"`
	Connections   []cfConn   `json:"connections"`
	RemoteConfig  bool       `json:"remote_config"`
	ConnsActive   int        `json:"conns_active"`
	ConnsInactive int        `json:"conns_inactive"`
}

type cfConn struct {
	ID            string   `json:"id"`
	Features      []string `json:"features"`
	Version       string   `json:"version"`
	Arch          string   `json:"arch"`
	ConnsActive   int      `json:"conns_active"`
	ConnsInactive int      `json:"conns_inactive"`
	OriginIP      string   `json:"origin_ip"`
}

// Tunnel 的一条 cfd_tunnel。统一容器模型（doc/16 §2）：面板只把 ManagedTunnelID
// 指向的自建物理隧道当作可写容器，其余隧道一律只读展示。ingress 规则按远程
// 托管分流：
//   - 远程托管（dashboard/API 管）：GET .../configurations 拉规则，每条归一为
//     一条面板隧道；挂在统一容器下的规则可编辑/删除，其余（如运维手工建的
//     独立专线）锁编辑与删除——面板不碰不是自己创建的资源；
//   - 本地托管（config.yml 管）：API 看不到规则，整条 tunnel 归一为一条隧道，
//     name 用 tunnel 名，local 端点显示「由本地 config.yml 管理」。
func (c *CloudflaredClient) Tunnels(ctx context.Context) ([]*Tunnel, error) {
	if c.AccountID == "" {
		return nil, errors.New("缺少 Cloudflare account_id")
	}
	var list []cfTunnel
	q := url.Values{"is_deleted": {"false"}, "per_page": {"100"}}
	if err := c.call(ctx, http.MethodGet, "/accounts/"+c.AccountID+"/cfd_tunnel", q, nil, &list); err != nil {
		return nil, err
	}
	out := make([]*Tunnel, 0, len(list))
	for _, t := range list {
		if t.DeletedAt != nil {
			continue
		}
		managed := t.ID != "" && t.ID == c.ManagedTunnelID
		if t.RemoteConfig {
			rules, err := c.ingressRules(ctx, t.ID)
			if err != nil {
				return nil, err
			}
			if len(rules) == 0 {
				out = append(out, c.tunnelView(&t, "", "", managed))
			}
			for _, r := range rules {
				out = append(out, c.tunnelView(&t, r.Hostname, r.Service, managed))
			}
		} else {
			out = append(out, c.tunnelView(&t, "", "", managed))
		}
	}
	return out, nil
}

// cfConfigurations GET/PUT /configurations 的完整 result：
// ingress 数组嵌套在 result.config 下（实测响应），不是顶层。
type cfConfigurations struct {
	TunnelID string          `json:"tunnel_id"`
	Version  int             `json:"version"`
	Config   cfIngressConfig `json:"config"`
}

// cfIngressConfig ingress 配置体（config 字段的形态）。
type cfIngressConfig struct {
	Ingress []cfIngressRule `json:"ingress"`
}

type cfIngressRule struct {
	Hostname string `json:"hostname"`
	Service  string `json:"service"`
}

// ingressRules 拉远程托管隧道的 ingress 规则（catch-all 的空 hostname 规则跳过）。
func (c *CloudflaredClient) ingressRules(ctx context.Context, tunnelID string) ([]cfIngressRule, error) {
	var cfg cfConfigurations
	if err := c.call(ctx, http.MethodGet, "/accounts/"+c.AccountID+"/cfd_tunnel/"+tunnelID+"/configurations", nil, nil, &cfg); err != nil {
		return nil, err
	}
	out := make([]cfIngressRule, 0, len(cfg.Config.Ingress))
	for _, r := range cfg.Config.Ingress {
		if strings.TrimSpace(r.Hostname) == "" {
			continue // catch-all / 404 兜底规则
		}
		out = append(out, r)
	}
	return out, nil
}

// tunnelView 把一条 cfd_tunnel（或其一条 ingress 规则）归一为面板隧道视图。
//
// 状态映射：healthy→normal+online、degraded→normal+online（能服务但不健康，
// 状态原因里说明）、down/inactive→offline。RemoteID 带 tunnel ID 前缀
// （<tunnelID>:<hostname>），保证同 tunnel 多规则时 remote_id 唯一——
// frp_tunnel 的唯一索引建在 (platform_id, remote_id) 上。
//
// managed=false（非统一容器的规则，如运维手工建的独立专线）锁编辑/删除：
// 面板同步展示但不写这类资源，delete 语义只删「自己往统一容器里加的规则」。
func (c *CloudflaredClient) tunnelView(t *cfTunnel, hostname, service string, managed bool) *Tunnel {
	tv := &Tunnel{
		RemoteID:  t.ID,
		Name:      t.Name,
		Proto:     "https",
		NodeID:    t.ID,
		NodeName:  t.Name,
		LocalIP:   "",
		LocalPort: 0,
		Remote:    hostname,
		Status:    "normal",
		Conns:     t.ConnsActive,
		Extra:     "cloudflare-tunnel",
		LockEdit:  !managed,
		LockDelete: !managed,
	}
	if hostname != "" {
		tv.RemoteID = t.ID + ":" + hostname
		tv.Name = hostname
		tv.NodeName = hostname
	}
	if !managed {
		tv.Extra = "cloudflare-tunnel · 独立专线（只读）"
	} else if hostname == "" {
		// 统一容器本体（无规则的远程托管行）：注明它的角色，避免用户
		// 把这行当成可访问的服务
		tv.Extra = "cloudflare-tunnel · 统一容器（面板节点挂在这里）"
	}
	switch t.Status {
	case "healthy":
		tv.Online = true
	case "degraded":
		tv.Online = true
		tv.StatusReason = "连接器不健康，服务可能受影响"
	default:
		tv.Online = false
		tv.StatusReason = "无在线连接器（cloudflared 未运行）"
	}
	if !t.RemoteConfig {
		tv.StatusReason = "本地 config.yml 托管，规则在 cloudflared 侧"
	}
	if service != "" {
		ip, port, ok := parseCFService(service)
		if ok {
			tv.LocalIP = ip
			tv.LocalPort = port
		} else {
			tv.LocalIP = service
			tv.Extra = "service:" + service
		}
	}
	return tv
}

// parseCFService 解析 ingress service（http://localhost:8091 → localhost, 8091）。
// 非 HTTP 型 service（unix:、ssh://、status: 等）解析失败由调用方原样展示。
func parseCFService(service string) (string, int, bool) {
	u, err := url.Parse(service)
	if err != nil || u.Host == "" {
		return "", 0, false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 {
		return "", 0, false
	}
	host := u.Hostname()
	return host, port, true
}

// ---------- 节点 ----------

// Nodes 实现 platform 接口：CF 没有「接入节点」概念，归一为「隧道连接器」——
// 每条远程托管隧道的连接器作为一条节点镜像（remote_id=tunnel ID），
// 供在用节点展示。本地托管隧道没有 connections 数据，跳过。
func (c *CloudflaredClient) Nodes(ctx context.Context) ([]*Node, error) {
	var list []cfTunnel
	q := url.Values{"is_deleted": {"false"}, "per_page": {"100"}}
	if err := c.call(ctx, http.MethodGet, "/accounts/"+c.AccountID+"/cfd_tunnel", q, nil, &list); err != nil {
		return nil, err
	}
	out := make([]*Node, 0, len(list))
	for _, t := range list {
		if t.DeletedAt != nil || len(t.Connections) == 0 {
			continue
		}
		n := &Node{
			RemoteID:  t.ID,
			Name:      t.Name + " 连接器",
			Area:      "",
			GroupName: "Cloudflare",
			Caps:      []string{"connector"},
			Online:    t.Status == "healthy" || t.Status == "degraded",
			Host:      "",
		}
		for _, cn := range t.Connections {
			if cn.Version != "" {
				n.Description = "cloudflared " + cn.Version
				break
			}
		}
		n.Uptime = 0
		out = append(out, n)
	}
	return out, nil
}

// ---------- 写操作 ----------

// TunnelInput 的映射：Name=对外域名（如 app.yoahoug.dev）、LocalIP/LocalPort=service。

// EnsureUnifiedTunnel 返回统一容器的物理隧道 ID，不存在则创建并经 onCreated
// 回写凭据（与 AccountID 自动发现同一懒创建管线）。幂等：已存在直接返回。
// 统一容器模型（doc/16 §2）：一个 tunnel 对应一个 connector 容器，面板的所有
// 节点（ingress 规则）都挂它下面，实现单容器统一管理。
func (c *CloudflaredClient) EnsureUnifiedTunnel(ctx context.Context, onCreated func(id string) error) (string, error) {
	// 先扫现有隧道：面板以前建过（或凭据回写丢失）直接复用
	if id, err := c.FindUnifiedTunnel(ctx); err == nil && id != "" {
		return id, nil
	}
	created, err := c.createRawTunnel(ctx, ManagedTunnelName)
	if err != nil {
		return "", err
	}
	if onCreated != nil {
		if err := onCreated(created.ID); err != nil {
			// 回写失败只影响下次重复查找（按名扫得到），不回滚物理隧道
			return created.ID, nil
		}
	}
	return created.ID, nil
}

// FindUnifiedTunnel 按名查找统一容器隧道，找不到返回空串（不创建）。
func (c *CloudflaredClient) FindUnifiedTunnel(ctx context.Context) (string, error) {
	if c.AccountID == "" {
		return "", errors.New("缺少 Cloudflare account_id")
	}
	var list []cfTunnel
	q := url.Values{"is_deleted": {"false"}, "per_page": {"100"}, "name": {ManagedTunnelName}}
	if err := c.call(ctx, http.MethodGet, "/accounts/"+c.AccountID+"/cfd_tunnel", q, nil, &list); err != nil {
		return "", err
	}
	for _, t := range list {
		if t.DeletedAt == nil && t.Name == ManagedTunnelName {
			return t.ID, nil
		}
	}
	return "", nil
}

// CreateTunnel 在统一容器下新增一条 ingress 规则（= 面板视角的「新建隧道」）+
// 建 DNS CNAME。不再每次建物理 tunnel——统一容器由 EnsureUnifiedTunnel 懒创建。
// onUnifiedCreated 回写统一隧道 ID 进凭据（Runner 侧持久化），失败不阻断。
func (c *CloudflaredClient) CreateTunnel(ctx context.Context, in TunnelInput) (string, error) {
	hostname := strings.TrimSpace(in.Name)
	if hostname == "" || !strings.Contains(hostname, ".") {
		return "", errors.New("Cloudflare 隧道名须为完整域名（如 app.yoahoug.dev）")
	}
	if in.LocalPort <= 0 {
		return "", errors.New("本地端口必填")
	}

	tunnelID, err := c.EnsureUnifiedTunnel(ctx, nil)
	if err != nil {
		return "", err
	}

	// 读现有 ingress + 追加新规则；catch-all 规则保持最后：http_status:404
	existing, err := c.rawIngress(ctx, tunnelID)
	if err != nil {
		return "", err
	}
	for _, r := range existing {
		if r["hostname"] == hostname {
			return "", fmt.Errorf("统一容器里已有域名 %s 的节点，请直接编辑它", hostname)
		}
	}
	rule := map[string]any{"hostname": hostname, "service": fmt.Sprintf("http://%s:%d", defaultCFLocalIP(in.LocalIP), in.LocalPort)}
	ingress := append(existing, rule, map[string]any{"service": "http_status:404"})
	if err := c.putIngress(ctx, tunnelID, ingress); err != nil {
		return "", fmt.Errorf("写入 ingress 规则失败: %w", err)
	}

	// DNS CNAME（失败不回滚 ingress——节点本身已可用，手动补 CNAME 即可）
	if c.ZoneID != "" {
		if err := c.upsertCNAME(ctx, hostname, tunnelID); err != nil {
			return tunnelID + ":" + hostname, fmt.Errorf("节点已创建，但 DNS 记录写入失败（可稍后在 Cloudflare 后台手动补）： %w", err)
		}
	}
	return tunnelID + ":" + hostname, nil
}

// createRawTunnel 创建物理 tunnel（2026-05 起 CF 要求 config_src=cloudflare 即远程托管）。
func (c *CloudflaredClient) createRawTunnel(ctx context.Context, name string) (*cfTunnel, error) {
	payload := map[string]any{"name": name, "config_src": "cloudflare"}
	b, _ := json.Marshal(payload)
	var t cfTunnel
	if err := c.call(ctx, http.MethodPost, "/accounts/"+c.AccountID+"/cfd_tunnel", nil, b, &t); err != nil {
		return nil, err
	}
	if t.ID == "" {
		return nil, errors.New("Cloudflare 未返回新建隧道 ID")
	}
	return &t, nil
}

// rawIngress 取现有具名规则（不含 catch-all）。
// 注意：覆写 ingress 配置时不能直接用它做 kept 列表——CF 要求 catch-all 规则
// 必须始终在数组末位，丢掉它 PUT 会报 "last ingress rule must match all URLs"
// （E2E 实测踩坑）。覆写场景改用 rawFullIngress。
func (c *CloudflaredClient) rawIngress(ctx context.Context, tunnelID string) ([]map[string]any, error) {
	full, err := c.rawFullIngress(ctx, tunnelID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(full))
	for _, r := range full {
		if r["hostname"] == nil || strings.TrimSpace(r["hostname"].(string)) == "" {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// rawFullIngress 取完整 ingress 数组（含末位 catch-all），用于覆写前重建列表。
func (c *CloudflaredClient) rawFullIngress(ctx context.Context, tunnelID string) ([]map[string]any, error) {
	var cfg cfConfigurations
	if err := c.call(ctx, http.MethodGet, "/accounts/"+c.AccountID+"/cfd_tunnel/"+tunnelID+"/configurations", nil, nil, &cfg); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cfg.Config.Ingress))
	for _, r := range cfg.Config.Ingress {
		out = append(out, map[string]any{"hostname": r.Hostname, "service": r.Service})
	}
	return out, nil
}

// putIngress 覆写整份 ingress 配置。
func (c *CloudflaredClient) putIngress(ctx context.Context, tunnelID string, ingress []map[string]any) error {
	b, _ := json.Marshal(map[string]any{"config": map[string]any{"ingress": ingress}})
	return c.call(ctx, http.MethodPut, "/accounts/"+c.AccountID+"/cfd_tunnel/"+tunnelID+"/configurations", nil, b, nil)
}

// upsertCNAME 建/更新指向隧道的代理记录（proxied=true，橙云）。
func (c *CloudflaredClient) upsertCNAME(ctx context.Context, hostname, tunnelID string) error {
	// 先查是否已有记录（同名 A/AAAA/CNAME 会导致新 CNAME 冲突）
	var list []struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	q := url.Values{"name": {hostname}, "per_page": {"50"}}
	if err := c.call(ctx, http.MethodGet, "/zones/"+c.ZoneID+"/dns_records", q, nil, &list); err == nil {
		for _, r := range list {
			if r.Type != "CNAME" || r.Content != tunnelID+".cfargotunnel.com" {
				_ = c.deleteDNSRecord(ctx, r.ID)
			}
		}
	}
	payload := map[string]any{
		"type":    "CNAME",
		"name":    hostname,
		"content": tunnelID + ".cfargotunnel.com",
		"proxied": true,
		"ttl":     1,
	}
	b, _ := json.Marshal(payload)
	return c.call(ctx, http.MethodPost, "/zones/"+c.ZoneID+"/dns_records", nil, b, nil)
}

func (c *CloudflaredClient) deleteDNSRecord(ctx context.Context, recordID string) error {
	return c.call(ctx, http.MethodDelete, "/zones/"+c.ZoneID+"/dns_records/"+recordID, nil, nil, nil)
}

// UpdateTunnel 改统一容器下一条 ingress 规则的 service（其余规则原样保留）。
// 只允许操作统一容器：非面板自建的隧道（独立专线）一律拒绝写。
func (c *CloudflaredClient) UpdateTunnel(ctx context.Context, id string, in TunnelInput) error {
	tunnelID, hostname := SplitCFRemoteID(id)
	if tunnelID == "" {
		return errors.New("Cloudflare 隧道标识无效")
	}
	if tunnelID != c.ManagedTunnelID {
		return errors.New("该节点属于独立专线的隧道（非面板创建），请在 Cloudflare 后台修改")
	}
	if in.LocalPort <= 0 {
		return errors.New("本地端口必填")
	}
	existing, err := c.rawFullIngress(ctx, tunnelID)
	if err != nil {
		return err
	}
	service := fmt.Sprintf("http://%s:%d", defaultCFLocalIP(in.LocalIP), in.LocalPort)
	found := false
	// 只改目标规则；末位 catch-all 原样保留
	end := len(existing)
	if end > 0 && existing[end-1]["hostname"] == nil {
		end-- // catch-all（空 hostname）先摘出来，改完再放回去
	}
	body := existing[:end]
	for i, r := range body {
		if r["hostname"] == hostname {
			body[i]["service"] = service
			found = true
			break
		}
	}
	if !found {
		body = append(body, map[string]any{"hostname": hostname, "service": service})
	}
	return c.putIngress(ctx, tunnelID, append(body, existing[end:]...))
}

// DeleteOne 删统一容器下的一条 ingress 规则 + 对应 DNS 记录。隧道体保留
// （统一容器是所有面板节点的载体，删掉会影响其余节点）。
// 非统一容器的规则（独立专线）一律拒绝——面板不碰不是自己创建的资源。
func (c *CloudflaredClient) DeleteOne(ctx context.Context, id string) error {
	tunnelID, hostname := SplitCFRemoteID(id)
	if tunnelID == "" {
		return errors.New("Cloudflare 隧道标识无效")
	}
	if tunnelID != c.ManagedTunnelID {
		return errors.New("该节点属于独立专线的隧道（非面板创建），请在 Cloudflare 后台删除")
	}
	existing, err := c.rawFullIngress(ctx, tunnelID)
	if err != nil {
		return err
	}
	kept := make([]map[string]any, 0, len(existing))
	// 保留除目标规则外的所有规则（catch-all 在末位原样保留，CF 硬性要求）
	for _, r := range existing {
		if r["hostname"] == hostname {
			continue
		}
		kept = append(kept, r)
	}
	if err := c.putIngress(ctx, tunnelID, kept); err != nil {
		return err
	}
	// DNS 记录一并清掉（失败不阻断——下一轮同步会把隧道镜像删掉）
	if c.ZoneID != "" && hostname != "" {
		_ = c.deleteDNSByHostname(ctx, hostname)
	}
	return nil
}

func (c *CloudflaredClient) deleteDNSByHostname(ctx context.Context, hostname string) error {
	var list []struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	q := url.Values{"name": {hostname}, "type": {"CNAME"}, "per_page": {"50"}}
	if err := c.call(ctx, http.MethodGet, "/zones/"+c.ZoneID+"/dns_records", q, nil, &list); err != nil {
		return err
	}
	for _, r := range list {
		if strings.HasSuffix(r.Content, ".cfargotunnel.com") {
			_ = c.deleteDNSRecord(ctx, r.ID)
		}
	}
	return nil
}

// TunnelToken 取运行令牌（base64 JSON），托管 cloudflared 容器用。
func (c *CloudflaredClient) TunnelToken(ctx context.Context, tunnelID string) (string, error) {
	var tok string
	if err := c.call(ctx, http.MethodGet, "/accounts/"+c.AccountID+"/cfd_tunnel/"+tunnelID+"/token", nil, nil, &tok); err != nil {
		return "", err
	}
	if strings.TrimSpace(tok) == "" {
		return "", errors.New("Cloudflare 未返回隧道运行令牌")
	}
	return strings.TrimSpace(tok), nil
}

// ConfigFor 实现 platform 接口：CF 没有 frpc 式配置文件，返回 cloudflared
// 运行命令说明文本（docker run 一行 + 说明），下载后可直接执行。
func (c *CloudflaredClient) ConfigFor(ctx context.Context, tgt ConfigTarget) (string, error) {
	tunnelID, _ := SplitCFRemoteID(tgt.RemoteID)
	if tunnelID == "" {
		return "", errors.New("Cloudflare 隧道标识无效")
	}
	tok, err := c.TunnelToken(ctx, tunnelID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# cloudflared 运行命令（Cloudflare Tunnel 不使用 frpc 配置文件）\n")
	b.WriteString("# 注意：token 等同于凭据，不要提交到仓库或公开分享\n\n")
	b.WriteString("docker run -d --name beacontower-frpc-cloudflared --restart unless-stopped \\\n")
	b.WriteString("  --network host \\\n")
	b.WriteString("  cloudflare/cloudflared:latest tunnel --no-autoupdate run --token " + tok + "\n")
	return b.String(), nil
}

// DeleteRawTunnel 删除物理 tunnel（仅 E2E 清理用；统一容器与独立专线都不走这里）。
func (c *CloudflaredClient) DeleteRawTunnel(ctx context.Context, tunnelID string) error {
	return c.call(ctx, http.MethodDelete, "/accounts/"+c.AccountID+"/cfd_tunnel/"+tunnelID, nil, nil, nil)
}

// ---------- 小工具 ----------

// tokenRe cloudflared 运行令牌的形态约束：单段标准 base64 字符串（A-Z a-z 0-9
// + / =，实测含 padding 的 `=`，无点号分隔——解码后是 {"a":<account_id>,
// "t":<tunnel_id>} 的 JSON，**不是** JWT 三段式，E2E 实测修正），防止平台返回
// 异常内容被拼进 docker run 命令。
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9+/=]{40,}$`)

// SplitCFRemoteID 拆 "<tunnelID>:<hostname>"（无冒号时整个串就是 tunnelID）。
func SplitCFRemoteID(id string) (string, string) {
	if i := strings.Index(id, ":"); i > 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}

// defaultCFLocalIP ingress service 的 host 缺省值：cloudflared 容器里
// localhost 指容器自身，指向宿主服务时必须用宿主 LAN IP 或 172.17.0.1。
func defaultCFLocalIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || ip == "localhost" || ip == "127.0.0.1" {
		return "172.17.0.1"
	}
	return ip
}
