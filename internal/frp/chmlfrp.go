package frp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ChmlfrpBase ChmlFrp v2 API 基址。v1（cf-v1.uapis.cn/api）已彻底下线。
const ChmlfrpBase = "https://cf-v2.uapis.cn"

// ChmlfrpClient ChmlFrp API 客户端。
//
// 鉴权走轻爪账户 OAuth2 换来的 access_token（见 oauth.go），Bearer 直传，
// 后端会自行校验，不需要再换 ChmlFrp 自有令牌。
//
// 两个必须注意的坑：
//  1. 业务错误几乎都返回 HTTP 200，真实结果在 JSON 的 code 字段里
//     （200 成功 / 400 参数或登录态缺失 / 401 令牌无效）；
//  2. 官方文档有滞后，/node_status_info 的参数实际是 nodename 而非 node，
//     /delete_tunnel 文档说"开发中"但线上已可用。
type ChmlfrpClient struct {
	Base  string
	Token string
	hc    *http.Client
}

func NewChmlfrp(base, token string) *ChmlfrpClient {
	if strings.TrimSpace(base) == "" {
		base = ChmlfrpBase
	}
	return &ChmlfrpClient{Base: strings.TrimRight(base, "/"), Token: token, hc: newHTTPClient(20 * time.Second)}
}

// envelope v2 统一响应外壳。成功响应可能不带 state 字段（实测 /node、
// /panelinfo 都没有），因此判定只看 code。
type envelope struct {
	Msg   string          `json:"msg"`
	Code  int             `json:"code"`
	State string          `json:"state"`
	Data  json.RawMessage `json:"data"`
}

func (e *envelope) ok() bool { return e.Code == 0 || e.Code == 200 }

// call 统一出口：jsonBody 为 nil 时按 form 传（部分端点只认表单）。
//
// 实测所有端点都返回 {"msg":..,"code":..,"data":..} 外壳，业务码在 code 里
// （HTTP 状态码基本恒为 200，不能作为判定依据）。裸 body 分支只是平台改版
// 时的兜底：顶层没有 code 字段就按原样解。
func (c *ChmlfrpClient) call(ctx context.Context, method, path string, query url.Values, form url.Values, jsonBody any, out any) error {
	u := c.Base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+c.Token)

	var body io.Reader
	switch {
	case jsonBody != nil:
		b, err := json.Marshal(jsonBody)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
		hdr.Set("Content-Type", "application/json")
	case form != nil:
		body = strings.NewReader(form.Encode())
		hdr.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	raw, status, err := request(ctx, c.hc, method, u, hdr, body)
	if err != nil {
		return fmt.Errorf("连接 ChmlFrp 失败: %w", err)
	}
	if status == http.StatusNotFound {
		return &apiError{Platform: "ChmlFrp", Status: status, Msg: "接口不存在（平台可能已改版）"}
	}
	if status == http.StatusUnauthorized {
		return fmt.Errorf("%w（ChmlFrp 令牌无效）", ErrAuth)
	}
	if status == http.StatusForbidden {
		// 403 可能来自 WAF/限流（前置 Cloudflare WAF 拦脚本 UA 有前科），不能
		// 一律当凭据失效——误判会把平台降级 unbound、前端弹重新授权。仅当
		// 响应体明确指向鉴权时才归 ErrAuth，否则按普通接口错误处理。
		body := strings.ToLower(string(raw))
		if strings.Contains(body, "token") || strings.Contains(body, "auth") || strings.Contains(body, "登录") || strings.Contains(body, "令牌") {
			return fmt.Errorf("%w（ChmlFrp 令牌无效）", ErrAuth)
		}
		return &apiError{Platform: "ChmlFrp", Status: status, Msg: trimErr(string(raw))}
	}
	if status < 200 || status >= 300 {
		return &apiError{Platform: "ChmlFrp", Status: status, Msg: trimErr(string(raw))}
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err == nil {
		if _, hasCode := probe["code"]; hasCode {
			var env envelope
			if err := decodeJSON(raw, &env); err != nil {
				return err
			}
			if !env.ok() {
				if env.Code == 401 {
					return fmt.Errorf("%w（ChmlFrp：%s）", ErrAuth, env.Msg)
				}
				return &apiError{Platform: "ChmlFrp", Status: status, Code: env.Code, Msg: env.Msg}
			}
			if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
				return nil
			}
			return decodeJSON(env.Data, out)
		}
	}
	// 裸 body 形态（/tunnel 等）
	if out == nil {
		return nil
	}
	return decodeJSON(raw, out)
}

// ---------- 账号 ----------

type chmlUserInfo struct {
	ID            int    `json:"id"`
	Username      string `json:"username"`
	Usergroup     string `json:"usergroup"`
	Realname      any    `json:"realname"`
	Tunnel        int    `json:"tunnel"`
	TunnelCount   int    `json:"tunnelCount"`
	TotalCurConns int    `json:"totalCurConns"`
	TotalUpload   int64  `json:"total_upload"`
	TotalDownload int64  `json:"total_download"`
	Bandwidth     int    `json:"bandwidth"`
	Integral      int    `json:"integral"`
	Term          string `json:"term"`
	Regtime       string `json:"regtime"`
}

func (c *ChmlfrpClient) UserInfo(ctx context.Context) (*Account, error) {
	var u chmlUserInfo
	if err := c.call(ctx, http.MethodGet, "/userinfo", nil, nil, nil, &u); err != nil {
		return nil, err
	}
	acc := &Account{
		UID:       strconv.Itoa(u.ID),
		Username:  u.Username,
		GroupName: u.Usergroup,
		// 字段名极易读反：实测 tunnel=4 / tunnelCount=1，而账号下确实只有 1 条隧道，
		// 免费用户默认可建 4 条 —— 所以 tunnel 是上限、tunnelCount 是已用，
		// 与官方文档的说明相反
		TunnelUsed:  u.TunnelCount,
		TunnelQuota: u.Tunnel,
		Conns:       u.TotalCurConns,
		TrafficUp:   u.TotalUpload,
		TrafficDown: u.TotalDownload,
		// ChmlFrp 不限流量，剩余恒为 0；带宽国内为 bandwidth，境外翻 4 倍（官方文档原文）
		SpeedLimit: fmt.Sprintf("%d Mbps（境外 %d Mbps）", u.Bandwidth, u.Bandwidth*4),
		Realname:   normalizeRealname(u.Realname),
		Extra: map[string]any{
			"integral":     u.Integral,
			"term":         u.Term,
			"regtime":      u.Regtime,
			"group_expire": u.Term,
		},
	}
	return acc, nil
}

// normalizeRealname 实名状态在不同版本里可能是布尔、字符串或对象，统一成中文短语。
func normalizeRealname(v any) string {
	switch x := v.(type) {
	case nil:
		return "未知"
	case bool:
		if x {
			return "已实名"
		}
		return "未实名"
	case string:
		if x == "" {
			return "未知"
		}
		return x
	default:
		return fmt.Sprint(x)
	}
}

// FlowLast7Days 账号近 7 日流量。
func (c *ChmlfrpClient) FlowLast7Days(ctx context.Context) ([]TrafficPoint, error) {
	var rows []struct {
		Time       string `json:"time"`
		TrafficIn  int64  `json:"traffic_in"`
		TrafficOut int64  `json:"traffic_out"`
	}
	if err := c.call(ctx, http.MethodGet, "/flow_last_7_days", nil, nil, nil, &rows); err != nil {
		return nil, err
	}
	out := make([]TrafficPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, TrafficPoint{Label: r.Time, Used: r.TrafficIn + r.TrafficOut})
	}
	return out, nil
}

// ---------- 隧道 ----------

type chmlTunnel struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Node      string `json:"node"`
	NodeState string `json:"nodestate"`
	Type      string `json:"type"`
	LocalIP   string `json:"localip"`
	NPort     int    `json:"nport"`
	Dorp      string `json:"dorp"`
	IP        string `json:"ip"`
	// 平台把布尔语义的字段序列化成字符串（实测 "encryption":"false"），
	// 直接声明成 bool 会让整个列表解码失败
	Encryption  string `json:"encryption"`
	Compression string `json:"compression"`
	AP          string `json:"ap"`
	State       string `json:"state"`  // 同样是字符串 "true"/"false"
	Uptime      string `json:"uptime"` // ISO 8601「上次启动时间」，不是时长
	ClientVer   string `json:"client_version"`
	TodayUp     int64  `json:"today_traffic_in"`
	TodayDown   int64  `json:"today_traffic_out"`
	CurConns    int    `json:"cur_conns"`
	TrafficDate string `json:"traffic_date"`
	ServerPort  int    `json:"server_port"`
}

func (c *ChmlfrpClient) Tunnels(ctx context.Context) ([]*Tunnel, error) {
	var list []chmlTunnel
	if err := c.call(ctx, http.MethodGet, "/tunnel", nil, nil, nil, &list); err != nil {
		return nil, err
	}
	out := make([]*Tunnel, 0, len(list))
	for _, t := range list {
		item := &Tunnel{
			RemoteID: strconv.Itoa(t.ID),
			Name:     t.Name,
			Proto:    strings.ToLower(t.Type),
			// ChmlFrp 的 /tunnel 只给节点名不给节点 ID（NodeID 留空），
			// Sync 时由 runner.fillNodeNames 反查补上 —— in_use/迁移/配置
			// 下载都依赖这个 ID，漏了会导致「在用节点」永远匹配不到
			NodeName:  t.Node,
			LocalIP:   t.LocalIP,
			LocalPort: t.NPort,
			Remote:    t.Dorp,
			Conns:     t.CurConns,
			TodayUp:   t.TodayUp,
			TodayDown: t.TodayDown,
			ClientVer: t.ClientVer,
			Status:    "normal",
			Online:    yes(t.State),
		}
		// Sakura 的 uptime 是「已运行秒数」，ChmlFrp 给的是「上次启动时刻」，
		// 统一换算成时长才好在前端同列展示；离线时该值无意义，留 0
		if item.Online {
			if started, err := time.Parse(time.RFC3339, strings.TrimSpace(t.Uptime)); err == nil {
				if d := time.Since(started); d > 0 {
					item.Uptime = int64(d.Seconds())
				}
			}
		} else if strings.TrimSpace(t.NodeState) == "" {
			// nodestate 为空表示节点已永久下线（官方文档原文）
			item.StatusReason = "节点已永久下线"
		}
		var extra []string
		if t.AP != "" {
			extra = append(extra, t.AP)
		}
		if yes(t.Encryption) {
			extra = append(extra, "TLS 加密")
		}
		if yes(t.Compression) {
			extra = append(extra, "压缩")
		}
		if t.IP != "" {
			extra = append(extra, "节点 "+t.IP)
		}
		item.Extra = strings.Join(extra, " · ")
		out = append(out, item)
	}
	return out, nil
}

// ---------- 节点 ----------

type chmlNode struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Area      string `json:"area"`
	NodeGroup string `json:"nodegroup"`
	China     string `json:"china"`
	Web       string `json:"web"`
	UDP       string `json:"udp"`
	Fangyu    string `json:"fangyu"`
	Notes     string `json:"notes"`
	IPv6      bool   `json:"ipv6"`
}

type chmlNodeStat struct {
	NodeName        string  `json:"node_name"`
	ID              int     `json:"id"`
	State           string  `json:"state"`
	BandwidthUsage  float64 `json:"bandwidth_usage_percent"`
	CPUUsage        float64 `json:"cpu_usage"`
	NodeGroup       string  `json:"nodegroup"`
	ClientCounts    int     `json:"client_counts"`
	TunnelCounts    int     `json:"tunnel_counts"`
	CurCounts       int     `json:"cur_counts"`
	TotalTrafficIn  int64   `json:"total_traffic_in"`
	TotalTrafficOut int64   `json:"total_traffic_out"`
}

func yes(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "yes" || v == "true" || v == "1"
}

func (c *ChmlfrpClient) Nodes(ctx context.Context) ([]*Node, error) {
	var list []chmlNode
	if err := c.call(ctx, http.MethodGet, "/node", nil, nil, nil, &list); err != nil {
		return nil, err
	}
	stats := map[string]chmlNodeStat{}
	var st []chmlNodeStat
	if err := c.call(ctx, http.MethodGet, "/node_stats", nil, nil, nil, &st); err == nil {
		for _, s := range st {
			stats[s.NodeName] = s
		}
	}

	out := make([]*Node, 0, len(list))
	for _, n := range list {
		node := &Node{
			RemoteID:    strconv.Itoa(n.ID),
			Name:        n.Name,
			Area:        n.Area,
			GroupName:   n.NodeGroup,
			Description: n.Notes,
			Online:      true,
		}
		var caps []string
		if yes(n.Web) {
			caps = append(caps, "web")
		}
		if yes(n.UDP) {
			caps = append(caps, "udp")
		}
		if yes(n.China) {
			caps = append(caps, "mainland")
		}
		if yes(n.Fangyu) {
			caps = append(caps, "defense")
		}
		if n.IPv6 {
			caps = append(caps, "ipv6")
		}
		if strings.EqualFold(n.NodeGroup, "vip") {
			caps = append(caps, "vip")
		}
		node.Caps = caps
		if s, ok := stats[n.Name]; ok {
			node.Online = !strings.EqualFold(s.State, "offline")
			node.Load = s.BandwidthUsage
			node.Uptime = 0
		}
		out = append(out, node)
	}
	return out, nil
}

// ---------- 写操作 ----------

// CreateTunnel 建隧道，返回平台侧隧道 ID。
func (c *ChmlfrpClient) CreateTunnel(ctx context.Context, in TunnelInput) (string, error) {
	body := map[string]any{
		"tunnelname":  in.Name,
		"node":        in.NodeID,
		"porttype":    in.Proto,
		"localip":     orDefault(in.LocalIP, "127.0.0.1"),
		"localport":   in.LocalPort,
		"encryption":  in.Encryption,
		"compression": in.Compression,
		"extraparams": in.Extra,
	}
	if in.Proto == "tcp" || in.Proto == "udp" {
		// 0 表示让平台随机分配可用端口
		body["remoteport"] = in.RemotePort
	} else {
		if in.Domain == "" {
			return "", errors.New("ChmlFrp 的 HTTP(S) 隧道必须填写绑定域名")
		}
		body["banddomain"] = in.Domain
	}
	var res struct {
		ID int `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/create_tunnel", nil, nil, body, &res); err != nil {
		return "", err
	}
	if res.ID == 0 {
		// 平台偶发不回 data，回查列表按名字定位（实测创建后立即可见）
		if list, err := c.Tunnels(ctx); err == nil {
			for _, t := range list {
				if t.Name == in.Name {
					return t.RemoteID, nil
				}
			}
		}
		return "", errors.New("ChmlFrp 未返回新建隧道 ID")
	}
	return strconv.Itoa(res.ID), nil
}

// UpdateTunnel 改隧道。注意官方文档标注 http/https 类型暂不支持修改。
func (c *ChmlfrpClient) UpdateTunnel(ctx context.Context, id string, in TunnelInput) error {
	body := map[string]any{"tunnelid": id}
	// porttype 是这个接口的必填项（真机实测漏发会回「端口类型缺失」）；
	// 编辑表单里类型不可改，Proto 由调用方带当前类型兜底。
	if in.Proto != "" {
		body["porttype"] = in.Proto
	}
	if in.Name != "" {
		body["tunnelname"] = in.Name
	}
	if in.NodeID != "" {
		body["node"] = in.NodeID
	}
	if in.LocalIP != "" {
		body["localip"] = in.LocalIP
	}
	if in.LocalPort > 0 {
		body["localport"] = in.LocalPort
	}
	if in.RemotePort > 0 {
		body["remoteport"] = in.RemotePort
	}
	if in.Domain != "" {
		body["banddomain"] = in.Domain
	}
	if in.Extra != "" {
		body["extraparams"] = in.Extra
	}
	return c.call(ctx, http.MethodPost, "/update_tunnel", nil, nil, body, nil)
}

// DeleteTunnel 删隧道（文档说"开发中"但线上面板就是走这个 GET 接口）。
func (c *ChmlfrpClient) DeleteTunnel(ctx context.Context, id string) error {
	q := url.Values{"tunnelid": {id}}
	return c.call(ctx, http.MethodGet, "/delete_tunnel", q, nil, nil, nil)
}

// DeleteOne 实现 platform 接口。
func (c *ChmlfrpClient) DeleteOne(ctx context.Context, id string) error {
	return c.DeleteTunnel(ctx, id)
}

// ConfigFor 实现 platform 接口：ChmlFrp 要「节点名 + 隧道名」定位。
func (c *ChmlfrpClient) ConfigFor(ctx context.Context, tgt ConfigTarget) (string, error) {
	if tgt.NodeName == "" {
		return "", errors.New("ChmlFrp 取配置需要节点名，请先同步一次隧道列表")
	}
	return c.TunnelConfig(ctx, tgt.NodeName, []string{tgt.Name})
}

// OfflineTunnel 强制断开某条隧道的当前连接（ChmlFrp 没有"启动"接口，
// 启动只能靠本地 frpc 进程）。
func (c *ChmlfrpClient) OfflineTunnel(ctx context.Context, name string) error {
	form := url.Values{"tunnel_name": {name}}
	return c.call(ctx, http.MethodPost, "/offline_tunnel", nil, form, nil, nil)
}

// TunnelConfig 取 frpc 配置（ini 文本）。tunnelNames 为空表示该节点下全部隧道。
func (c *ChmlfrpClient) TunnelConfig(ctx context.Context, node string, tunnelNames []string) (string, error) {
	q := url.Values{"node": {node}}
	if len(tunnelNames) > 0 {
		q.Set("tunnel_names", strings.Join(tunnelNames, ","))
	}
	var ini string
	if err := c.call(ctx, http.MethodGet, "/tunnel_config", q, nil, nil, &ini); err != nil {
		return "", err
	}
	return ini, nil
}

// TunnelLast7Days 单隧道近 7 日流量。
func (c *ChmlfrpClient) TunnelLast7Days(ctx context.Context, id string) ([]TrafficPoint, error) {
	var res struct {
		TrafficIn  []int64 `json:"traffic_in"`
		TrafficOut []int64 `json:"traffic_out"`
	}
	q := url.Values{"tunnel_id": {id}}
	if err := c.call(ctx, http.MethodGet, "/tunnel/last7days", q, nil, nil, &res); err != nil {
		return nil, err
	}
	n := len(res.TrafficIn)
	if len(res.TrafficOut) > n {
		n = len(res.TrafficOut)
	}
	out := make([]TrafficPoint, 0, n)
	for i := 0; i < n; i++ {
		var in, outB int64
		if i < len(res.TrafficIn) {
			in = res.TrafficIn[i]
		}
		if i < len(res.TrafficOut) {
			outB = res.TrafficOut[i]
		}
		out = append(out, TrafficPoint{Label: fmt.Sprintf("D-%d", n-i), Used: in + outB})
	}
	return out, nil
}

// ---------- 免费二级域名 ----------

// Subdomain 用户已申请的免费二级域名记录。
type Subdomain struct {
	ID      int    `json:"id"`
	Domain  string `json:"domain"`
	Record  string `json:"record"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	TTL     string `json:"ttl"`
	Remarks string `json:"remarks"`
}

func (c *ChmlfrpClient) Subdomains(ctx context.Context) ([]*Subdomain, error) {
	var list []*Subdomain
	if err := c.call(ctx, http.MethodGet, "/get_user_free_subdomains", nil, nil, nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// AvailableDomains 可用的主域名（用户建解析时选择）。
func (c *ChmlfrpClient) AvailableDomains(ctx context.Context) ([]map[string]any, error) {
	var list []map[string]any
	if err := c.call(ctx, http.MethodGet, "/list_available_domains", nil, nil, nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// CreateSubdomain 新建免费二级域名记录。
func (c *ChmlfrpClient) CreateSubdomain(ctx context.Context, domain, record, typ, target, ttl, remarks string) error {
	body := map[string]any{"domain": domain, "record": record, "type": typ, "target": target, "ttl": ttl, "remarks": remarks}
	return c.call(ctx, http.MethodPost, "/create_free_subdomain", nil, nil, body, nil)
}

// UpdateSubdomain 修改（平台仅允许改 TTL 与目标）。
func (c *ChmlfrpClient) UpdateSubdomain(ctx context.Context, domain, record, target, ttl, remarks string) error {
	body := map[string]any{"domain": domain, "record": record, "target": target, "ttl": ttl, "remarks": remarks}
	return c.call(ctx, http.MethodPost, "/update_free_subdomain", nil, nil, body, nil)
}

// DeleteSubdomain 删除免费二级域名记录。
func (c *ChmlfrpClient) DeleteSubdomain(ctx context.Context, domain, record string) error {
	body := map[string]any{"domain": domain, "record": record}
	return c.call(ctx, http.MethodPost, "/delete_free_subdomain", nil, nil, body, nil)
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
