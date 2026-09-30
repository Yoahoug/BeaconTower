// Package frp 内网穿透平台管理（doc/13）：把 NATFRP（樱花）与 ChmlFrp 两个
// 国内穿透平台的账号、隧道、节点、用量收敛成统一的只读镜像 + 一组写操作。
//
// 两平台的协议差异全部封在本包内：
//   - NATFRP 用长期有效的「访问密钥」直连 OpenAPI v4，失败也返回 HTTP 500
//     （真实错误码在 body 的 code 字段），且 Cloudflare WAF 会拦截空/脚本 UA；
//   - ChmlFrp 走轻爪账户 OAuth2 设备码换 access_token（10 分钟过期，必须用
//     refresh_token 续期），v2 API 的业务错误放在 HTTP 200 的 JSON 里。
//
// 上层（handler / tasks）只看到归一化后的类型，不感知这些差别。
package frp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// UA 必须显式设置：NATFRP 前置的 Cloudflare WAF 会把 Python-urllib 这类
// 默认脚本 UA 判成攻击（实测直接 403 error code: 1010），空 UA 同理。
const userAgent = "BeaconTower/2.1 (+https://github.com/Yoahoug/BeaconTower)"

// ErrAuth 凭据失效。handler 据此把平台标记为需重新绑定，而不是当成普通网络错误重试。
var ErrAuth = errors.New("凭据无效或已过期，请重新绑定")

// 平台标识，与 frp_platform.kind 一致。
const (
	KindNatfrp  = "natfrp"
	KindChmlfrp = "chmlfrp"
)

// Account 账号维度的统一视图。两平台字段语义对不齐的地方统一在此归一：
//   - TrafficDayUsed：NATFRP 是「本日消耗」，ChmlFrp 无当日值（用近 7 日接口另算）
//   - TrafficRemain：NATFRP 是套餐剩余流量，ChmlFrp 不限流量（恒 0）
type Account struct {
	UID            string         `json:"uid"`
	Username       string         `json:"username"`
	GroupName      string         `json:"group_name"`
	SpeedLimit     string         `json:"speed_limit"`
	Realname       string         `json:"realname"`
	TunnelUsed     int            `json:"tunnel_used"`
	TunnelQuota    int            `json:"tunnel_quota"`
	Conns          int            `json:"conns"`
	TrafficDayUsed int64          `json:"traffic_day_used"`
	TrafficRemain  int64          `json:"traffic_remain"`
	TrafficUp      int64          `json:"traffic_up"`
	TrafficDown    int64          `json:"traffic_down"`
	Extra          map[string]any `json:"extra,omitempty"`
}

// Tunnel 隧道统一视图。Remote 语义随协议而变：
// tcp/udp 是公网端口，http/https 是绑定的域名。
type Tunnel struct {
	RemoteID     string `json:"remote_id"`
	Name         string `json:"name"`
	Proto        string `json:"proto"`
	NodeID       string `json:"node_id"`
	NodeName     string `json:"node_name"`
	LocalIP      string `json:"local_ip"`
	LocalPort    int    `json:"local_port"`
	Remote       string `json:"remote"`
	Online       bool   `json:"online"`
	Status       string `json:"status"` // normal | banned | unknown
	StatusReason string `json:"status_reason"`
	Conns        int    `json:"conns"`
	TodayUp      int64  `json:"today_up"`
	TodayDown    int64  `json:"today_down"`
	Uptime       int64  `json:"uptime"`
	ClientVer    string `json:"client_ver"`
	Extra        string `json:"extra"`
	LockEdit     bool   `json:"lock_edit"`
	LockDelete   bool   `json:"lock_delete"`
	LockMigrate  bool   `json:"lock_migrate"`
}

// Node 节点统一视图。Caps 是两平台能力位的并集字符串（http/udp/web/…）。
type Node struct {
	RemoteID    string   `json:"remote_id"`
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Area        string   `json:"area"`
	GroupName   string   `json:"group_name"`
	Caps        []string `json:"caps"`
	Online      bool     `json:"online"`
	Load        float64  `json:"load"`
	Uptime      int64    `json:"uptime"`
	Description string   `json:"description"`
}

// TrafficPoint 平台侧流量历史的一个点（时间标签由平台给出，仅作展示）。
type TrafficPoint struct {
	Label  string `json:"label"`
	Used   int64  `json:"used"`
	Remain int64  `json:"remain"`
}

// TunnelInput 建/改隧道的统一入参。字段多余是常态（两平台各用各的子集），
// 由各平台客户端自行取用，handler 只做基本校验。
type TunnelInput struct {
	Name        string `json:"name"`
	Proto       string `json:"proto"` // tcp/udp/http/https
	NodeID      string `json:"node_id"`
	LocalIP     string `json:"local_ip"`
	LocalPort   int    `json:"local_port"`
	RemotePort  int    `json:"remote_port"`
	Domain      string `json:"domain"` // http/https 绑定域名
	Note        string `json:"note"`
	Extra       string `json:"extra"`
	Encryption  bool   `json:"encryption"`
	Compression bool   `json:"compression"`
}

// apiError 平台返回的业务错误。HTTP 状态码在两平台都不可靠，一律解析 body 后构造。
type apiError struct {
	Platform string
	Status   int
	Code     int
	Msg      string
}

func (e *apiError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("%s: HTTP %d", e.Platform, e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Platform, e.Msg)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
}

// request 是所有外部调用的唯一出口：统一 UA / 超时 / 响应体上限。
// body 已被消费，返回的 raw 供调用方自行按平台约定解析。
func request(ctx context.Context, hc *http.Client, method, url string, header http.Header, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	// 上限 4MB：隧道/节点列表最大的 ChmlFrp node_stats 也只有几百 KB，
	// 防止平台侧异常返回把内存打满
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

// decodeJSON 宽松解码：平台偶发返回非 JSON（网关 HTML 错误页），
// 此时给出可诊断的截断片段而不是裸 unmarshal 错误。
func decodeJSON(raw []byte, out any) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("响应不是合法 JSON: %.120s", strings.TrimSpace(string(raw)))
	}
	return nil
}

func trimErr(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
