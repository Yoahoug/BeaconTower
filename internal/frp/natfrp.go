package frp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NatfrpBase Sakura OpenAPI v4 基址（官方 spec: https://api.natfrp.com/docs/）。
const NatfrpBase = "https://api.natfrp.com/v4"

// NatfrpFrpcVersion 是取 frpc 配置时声明的客户端版本，决定返回格式：
// 樱花分支版本（0.51.0-sakura-N）返回 INI（sakura_mode = true），
// 上游 frp 版本（如 0.59.0）返回 TOML。这里跟随 deploy/frpc-natfrp
// 镜像 pin 的版本，保证面板下载的配置与容器里的客户端一致；升级镜像
// 时同步改这里并重建面板镜像。
const NatfrpFrpcVersion = "0.51.0-sakura-14"

// NatfrpClient 樱花内网穿透 API 客户端。
//
// 鉴权用面板里生成的「访问密钥」（长期有效，与登录密码不同），直接
// `Authorization: Bearer <密钥>`，没有换票/签名步骤——官方 spec 里的
// PanelSessionCookie 方案已废弃，且 /user/sign 等少量端点只认 Session，
// 本客户端不实现它们（签到需要过 Geetest 验证码，纯 API 做不到）。
type NatfrpClient struct {
	Base  string
	Token string
	hc    *http.Client
}

func NewNatfrp(base, token string) *NatfrpClient {
	if strings.TrimSpace(base) == "" {
		base = NatfrpBase
	}
	return &NatfrpClient{Base: strings.TrimRight(base, "/"), Token: token, hc: newHTTPClient(20 * time.Second)}
}

// natfrpResp 平台错误统一在 body：实测 HTTP 500 + {"code":401,"msg":"访问密钥无效"}，
// 所以不能只看 HTTP 状态码。
type natfrpResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// call 发起请求并把 body 解到 out（out 为 nil 时只返回原始文本）。
func (n *NatfrpClient) call(ctx context.Context, method, path string, form url.Values, out any) ([]byte, error) {
	var body io.Reader
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+n.Token)
	if form != nil {
		body = strings.NewReader(form.Encode())
		hdr.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	raw, status, err := request(ctx, n.hc, method, n.Base+path, hdr, body)
	if err != nil {
		return nil, fmt.Errorf("连接 Sakura 失败: %w", err)
	}
	if status < 200 || status >= 300 {
		var e natfrpResp
		_ = decodeJSON(raw, &e)
		if e.Code == 401 {
			return nil, fmt.Errorf("%w（Sakura 访问密钥无效）", ErrAuth)
		}
		msg := e.Msg
		if msg == "" {
			msg = trimErr(string(raw))
		}
		return raw, &apiError{Platform: "Sakura", Status: status, Code: e.Code, Msg: msg}
	}
	if out != nil {
		if err := decodeJSON(raw, out); err != nil {
			return raw, err
		}
	}
	return raw, nil
}

// ---------- 账号 ----------

type natfrpUserInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Token    string `json:"token"`
	Speed    string `json:"speed"`
	Tunnels  int    `json:"tunnels"`
	Realname int    `json:"realname"`
	Group    struct {
		Name    string `json:"name"`
		Level   int    `json:"level"`
		Expires int64  `json:"expires"`
	} `json:"group"`
	Traffic []int64 `json:"traffic"` // [本日消耗, 总剩余]
	Sign    struct {
		Signed  bool    `json:"signed"`
		Last    string  `json:"last"`
		Days    int     `json:"days"`
		Traffic float64 `json:"traffic"`
	} `json:"sign"`
	// 冻结账户形态（spec 的 oneOf 第二支）
	Ban *struct {
		Title  string `json:"title"`
		Reason string `json:"reason"`
	} `json:"ban"`
}

// UserInfo 拉取账号信息并归一化。
func (n *NatfrpClient) UserInfo(ctx context.Context) (*Account, error) {
	var u natfrpUserInfo
	if _, err := n.call(ctx, http.MethodGet, "/user/info", nil, &u); err != nil {
		return nil, err
	}
	acc := &Account{
		UID:         strconv.Itoa(u.ID),
		Username:    u.Name,
		GroupName:   u.Group.Name,
		SpeedLimit:  u.Speed,
		TunnelQuota: u.Tunnels,
		Extra: map[string]any{
			"sign_signed":  u.Sign.Signed,
			"sign_days":    u.Sign.Days,
			"sign_traffic": u.Sign.Traffic,
			"group_expire": u.Group.Expires,
			"group_level":  u.Group.Level,
		},
	}
	if u.Realname > 0 {
		acc.Realname = "已实名"
	} else {
		acc.Realname = "未实名"
	}
	if len(u.Traffic) >= 2 {
		acc.TrafficDayUsed = u.Traffic[0]
		acc.TrafficRemain = u.Traffic[1]
	}
	if u.Ban != nil {
		acc.Realname = "账户已冻结"
		acc.Extra["ban_reason"] = u.Ban.Reason
	}
	return acc, nil
}

// TrafficHistory 流量历史。kind 取 day/week/month。
func (n *NatfrpClient) TrafficHistory(ctx context.Context, kind string) ([]TrafficPoint, error) {
	switch kind {
	case "day", "week", "month":
	default:
		kind = "day"
	}
	var rows [][]any
	if _, err := n.call(ctx, http.MethodGet, "/user/traffic_history?type="+kind, nil, &rows); err != nil {
		return nil, err
	}
	out := make([]TrafficPoint, 0, len(rows))
	for _, r := range rows {
		// spec 把三个元素都声明为 string，但实际后两项是数字，统一按 any 收再做类型归一
		if len(r) < 3 {
			continue
		}
		p := TrafficPoint{Label: toStr(r[0])}
		p.Used = toInt64(r[1])
		p.Remain = toInt64(r[2])
		out = append(out, p)
	}
	return out, nil
}

// ---------- 隧道 ----------

type natfrpTunnel struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Node         int    `json:"node"`
	Online       bool   `json:"online"`
	Status       int    `json:"status"`
	StatusReason string `json:"status_reason"`
	Note         string `json:"note"`
	Extra        string `json:"extra"`
	Remote       string `json:"remote"`
	LocalIP      string `json:"local_ip"`
	LocalPort    int    `json:"local_port"`
	Locks        struct {
		Edit    bool `json:"edit"`
		Delete  bool `json:"delete"`
		Migrate bool `json:"migrate"`
	} `json:"locks"`
}

func (n *NatfrpClient) Tunnels(ctx context.Context) ([]*Tunnel, error) {
	var list []natfrpTunnel
	if _, err := n.call(ctx, http.MethodGet, "/tunnels", nil, &list); err != nil {
		return nil, err
	}
	out := make([]*Tunnel, 0, len(list))
	for _, t := range list {
		item := &Tunnel{
			RemoteID:     strconv.Itoa(t.ID),
			Name:         t.Name,
			Proto:        t.Type,
			NodeID:       strconv.Itoa(t.Node),
			LocalIP:      t.LocalIP,
			LocalPort:    t.LocalPort,
			Remote:       t.Remote,
			Online:       t.Online,
			Status:       "normal",
			StatusReason: t.StatusReason,
			Extra:        t.Note,
			LockEdit:     t.Locks.Edit,
			LockDelete:   t.Locks.Delete,
			LockMigrate:  t.Locks.Migrate,
		}
		if t.Status == 2 {
			item.Status = "banned"
		}
		out = append(out, item)
	}
	return out, nil
}

// ---------- 节点 ----------

type natfrpNode struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Description string `json:"description"`
	VIP         int    `json:"vip"`
	Flag        int    `json:"flag"`
}

type natfrpNodeStat struct {
	ID     int     `json:"id"`
	Online int     `json:"online"` // -1 离线，>=0 在线
	Uptime int64   `json:"uptime"`
	Load   float64 `json:"load"`
}

// Sakura 节点 flag 位域（官方 spec /nodes 描述）。
//
// 低两位是**独立的两个能力位**，不是"两位都置位才允许 HTTP"的掩码：
// 实测 71 个节点里 flag&0b11 只有 0/1/2 三种取值，从不出现 3，
// 若按掩码判定会导致"全网没有一个节点支持建站"的荒谬结果。
//
//	0 = 不支持建站；1 = 支持 HTTP 建站；2 = 支持 HTTPS 建站
const (
	natfrpFlagHTTP      = 1 << 0 // 允许 HTTP 隧道（建站）
	natfrpFlagHTTPS     = 1 << 1 // 允许 HTTPS 隧道（建站）
	natfrpFlagCreate    = 1 << 2 // 允许创建隧道（满载时为 0）
	natfrpFlagMainland  = 1 << 3 // 内地节点
	natfrpFlagNoDefense = 1 << 4
	natfrpFlagUDP       = 1 << 5
	natfrpFlagPrivate   = 1 << 6
	natfrpFlagEnforceCA = 1 << 8 // 强制访问认证
	natfrpFlagOffline   = 1 << 9
	natfrpFlagBeta      = 1 << 10
)

// Nodes 拉节点列表并与状态位合并（/nodes 只有静态信息，/node/stats 才有在线与负载）。
// 状态接口失败时不阻断开列表——节点仍然可展示，只是不带实时状态。
func (n *NatfrpClient) Nodes(ctx context.Context) ([]*Node, error) {
	var raw map[string]natfrpNode
	if _, err := n.call(ctx, http.MethodGet, "/nodes", nil, &raw); err != nil {
		return nil, err
	}
	stats := map[int]natfrpNodeStat{}
	var st struct {
		Nodes []natfrpNodeStat `json:"nodes"`
	}
	if _, err := n.call(ctx, http.MethodGet, "/node/stats", nil, &st); err == nil {
		for _, s := range st.Nodes {
			stats[s.ID] = s
		}
	}

	out := make([]*Node, 0, len(raw))
	for id, nd := range raw {
		node := &Node{
			RemoteID:    id,
			Name:        nd.Name,
			Host:        nd.Host,
			Description: nd.Description,
			GroupName:   "普通节点",
			Online:      nd.Flag&natfrpFlagOffline == 0,
		}
		if nd.VIP > 0 {
			node.GroupName = fmt.Sprintf("VIP %d", nd.VIP)
		}
		var caps []string
		if nd.Flag&natfrpFlagHTTP != 0 {
			caps = append(caps, "http")
		}
		if nd.Flag&natfrpFlagHTTPS != 0 {
			caps = append(caps, "https")
		}
		if nd.Flag&natfrpFlagCreate != 0 {
			caps = append(caps, "create")
		}
		if nd.Flag&natfrpFlagMainland != 0 {
			caps = append(caps, "mainland")
		}
		if nd.Flag&natfrpFlagUDP != 0 {
			caps = append(caps, "udp")
		}
		if nd.Flag&natfrpFlagEnforceCA != 0 {
			caps = append(caps, "auth")
		}
		if nd.Flag&natfrpFlagPrivate != 0 {
			caps = append(caps, "private")
		}
		if nd.Flag&natfrpFlagNoDefense != 0 {
			caps = append(caps, "nodefense")
		}
		if nd.Flag&natfrpFlagBeta != 0 {
			caps = append(caps, "beta")
		}
		node.Caps = caps
		if s, ok := stats[nodeID(id)]; ok {
			node.Online = s.Online >= 0
			node.Load = s.Load
			node.Uptime = s.Uptime
		}
		out = append(out, node)
	}
	return out, nil
}

func nodeID(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// ---------- 写操作 ----------

// CreateTunnel 建隧道，返回平台侧隧道 ID。
func (n *NatfrpClient) CreateTunnel(ctx context.Context, in TunnelInput) (string, error) {
	form := url.Values{
		"name": {in.Name},
		"type": {in.Proto},
		"node": {in.NodeID},
	}
	if in.LocalIP != "" {
		form.Set("local_ip", in.LocalIP)
	}
	if in.LocalPort > 0 {
		form.Set("local_port", strconv.Itoa(in.LocalPort))
	}
	if in.Note != "" {
		form.Set("note", in.Note)
	}
	if in.Extra != "" {
		form.Set("extra", in.Extra)
	}
	// http/https 的 remote 是绑定域名；tcp/udp 是端口号（0 = 让平台分配）
	if in.Proto == "http" || in.Proto == "https" {
		if in.Domain == "" {
			return "", errors.New("Sakura 的 HTTP(S) 隧道必须填写绑定域名")
		}
		form.Set("remote", in.Domain)
	} else if in.RemotePort > 0 {
		form.Set("remote", strconv.Itoa(in.RemotePort))
	} else {
		form.Set("remote", "0")
	}
	var res struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Remote string `json:"remote"`
	}
	if _, err := n.call(ctx, http.MethodPost, "/tunnels", form, &res); err != nil {
		return "", err
	}
	if res.ID == 0 {
		return "", errors.New("Sakura 未返回新建隧道 ID")
	}
	return strconv.Itoa(res.ID), nil
}

// UpdateTunnel 改隧道。Sakura 只允许改备注 / 本地地址端口 / extra，
// 类型与节点分别要删建和 migrate。
func (n *NatfrpClient) UpdateTunnel(ctx context.Context, id string, in TunnelInput) error {
	form := url.Values{"id": {id}}
	if in.Note != "" {
		form.Set("note", in.Note)
	}
	if in.LocalIP != "" {
		form.Set("local_ip", in.LocalIP)
	}
	if in.LocalPort > 0 {
		form.Set("local_port", strconv.Itoa(in.LocalPort))
	}
	if in.Extra != "" {
		form.Set("extra", in.Extra)
	}
	_, err := n.call(ctx, http.MethodPost, "/tunnel/edit", form, nil)
	return err
}

// DeleteTunnel 批量删除（平台侧单次上限 10 条）。
func (n *NatfrpClient) DeleteTunnel(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 10 {
		return errors.New("Sakura 单次最多删除 10 条隧道")
	}
	form := url.Values{"ids": {strings.Join(ids, ",")}}
	_, err := n.call(ctx, http.MethodPost, "/tunnel/delete", form, nil)
	return err
}

// LockTunnel 锁定编辑/删除/迁移。
func (n *NatfrpClient) LockTunnel(ctx context.Context, id string, edit, del, migrate bool) error {
	form := url.Values{
		"id":      {id},
		"edit":    {boolStr(edit)},
		"delete":  {boolStr(del)},
		"migrate": {boolStr(migrate)},
	}
	_, err := n.call(ctx, http.MethodPost, "/tunnel/lock", form, nil)
	return err
}

// MigrateTunnel 迁移到目标节点。
func (n *NatfrpClient) MigrateTunnel(ctx context.Context, id, nodeID string) error {
	form := url.Values{"id": {id}, "node": {nodeID}}
	_, err := n.call(ctx, http.MethodPost, "/tunnel/migrate", form, nil)
	return err
}

// TunnelConfig 取 frpc 配置文本。query 是逗号分隔的启动目标
// （隧道 ID 或 n+节点 ID），可直接被 frpc -c 加载。frpcVer 留空表示
// 不声明版本（平台按默认的樱花分支格式返回 INI），空串不能直接发给
// 平台——实测会 400。
func (n *NatfrpClient) TunnelConfig(ctx context.Context, query, frpcVer string) (string, error) {
	form := url.Values{"query": {query}}
	if v := strings.TrimSpace(frpcVer); v != "" {
		form.Set("frpc", v)
	}
	raw, err := n.call(ctx, http.MethodPost, "/tunnel/config", form, nil)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// DeleteOne 实现 platform 接口（Sakura 的删除接口本身支持批量）。
func (n *NatfrpClient) DeleteOne(ctx context.Context, id string) error {
	return n.DeleteTunnel(ctx, []string{id})
}

// ConfigFor 实现 platform 接口：Sakura 按隧道 ID 取配置，不需要节点名。
func (n *NatfrpClient) ConfigFor(ctx context.Context, tgt ConfigTarget) (string, error) {
	return n.TunnelConfig(ctx, tgt.RemoteID, NatfrpFrpcVersion)
}

// TunnelTraffic 单隧道流量：{时间戳: 字节}。
func (n *NatfrpClient) TunnelTraffic(ctx context.Context, id string) (map[string]int64, error) {
	var m map[string]int64
	if _, err := n.call(ctx, http.MethodGet, "/tunnel/traffic?id="+url.QueryEscape(id), nil, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// TunnelAuth 通过访问认证（ip 留空则授权请求来源 IP）。
func (n *NatfrpClient) TunnelAuth(ctx context.Context, id, ip string) (string, error) {
	form := url.Values{"id": {id}}
	if ip != "" {
		form.Set("ip", ip)
	}
	raw, err := n.call(ctx, http.MethodPost, "/tunnel/auth", form, nil)
	if err != nil {
		return "", err
	}
	return strings.Trim(string(raw), `"`), nil
}

// ---------- 小工具 ----------

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return int64(f)
	default:
		return 0
	}
}

func toInt(v any) int { return int(toInt64(v)) }
