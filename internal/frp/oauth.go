package frp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// 轻爪账户（ChmlFrp 的统一身份提供方）OAuth2 端点。
const (
	QzhuaIssuer       = "https://account-api.qzhua.net"
	QzhuaDeviceAuthEP = QzhuaIssuer + "/oauth2/device_authorization"
	QzhuaTokenEP      = QzhuaIssuer + "/oauth2/token"

	// DefaultChmlfrpClientID 是 ChmlFrp 官方启动器（TechCat-Team/ChmlFrpLauncher）
	// 使用的公共客户端，无 client_secret、已开启设备码授权，实测可直接用于
	// headless 场景。它不属于本项目，若平台方回收或调整，用
	// BEACON_CHMLFRP_CLIENT_ID 换成自建客户端即可（account.qzhua.net/console
	// 申请，需勾选 chmlfrp_api scope）。
	DefaultChmlfrpClientID = "019d4334b34972ca9fd41513e5703dfd"

	// chmlfrpScope 中 chmlfrp_api 是平台自定义 scope，缺了后端不认；
	// offline_access 用于换取 refresh_token。
	chmlfrpScope = "profile email offline_access chmlfrp_api"
)

// Token OAuth2 令牌响应。
type Token struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ExpiresAt 令牌过期时刻（ExpiresIn 缺省时按 10 分钟保守估计）。
func (t *Token) ExpiresAt() int64 {
	sec := t.ExpiresIn
	if sec <= 0 {
		sec = 600
	}
	return time.Now().Unix() + int64(sec)
}

// DeviceSession 一次设备码授权会话（内存态：授权是分钟级交互，
// 重启丢失可接受，无需落库）。
type DeviceSession struct {
	ID         string `json:"id"`
	UserCode   string `json:"user_code"`
	VerifyURL  string `json:"verify_url"`
	Interval   int    `json:"interval"`
	ExpiresIn  int    `json:"expires_in"`
	Status     string `json:"status"` // pending | ok | error | expired
	Error      string `json:"error"`
	PlatformID int64  `json:"platform_id"` // Status=ok 时给出新建的平台 ID
	Username   string `json:"username"`

	deviceCode string
	expiresAt  time.Time
	lastPoll   time.Time
	token      *Token
}

// DeviceFlow 设备码会话池。
type DeviceFlow struct {
	mu       sync.Mutex
	sessions map[string]*DeviceSession
	clientID string
	hc       *http.Client
}

func NewDeviceFlow(clientID string) *DeviceFlow {
	if strings.TrimSpace(clientID) == "" {
		clientID = DefaultChmlfrpClientID
	}
	return &DeviceFlow{
		sessions: map[string]*DeviceSession{},
		clientID: clientID,
		hc:       newHTTPClient(15 * time.Second),
	}
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("s%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// oauthPost 表单 POST，返回令牌响应。OAuth 端点连错误也用 JSON body 表达
// （error / error_description），因此不看 HTTP 状态码而看 body。
func (d *DeviceFlow) oauthPost(ctx context.Context, endpoint string, form url.Values) (*Token, error) {
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/x-www-form-urlencoded")
	hdr.Set("Accept", "application/json")
	raw, status, err := request(ctx, d.hc, http.MethodPost, endpoint, hdr, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("连接轻爪账户失败: %w", err)
	}
	var t Token
	if err := decodeJSON(raw, &t); err != nil {
		return nil, fmt.Errorf("账户中心返回异常响应（HTTP %d）: %w", status, err)
	}
	if t.Error != "" {
		return nil, &oauthError{Code: t.Error, Desc: t.ErrorDescription}
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("账户中心 HTTP %d", status)
	}
	return &t, nil
}

// oauthError 设备码轮询时的标准 OAuth 错误（authorization_pending 等属正常控制流）。
type oauthError struct {
	Code string
	Desc string
}

func (e *oauthError) Error() string {
	if e.Desc != "" {
		return e.Code + ": " + e.Desc
	}
	return e.Code
}

type deviceAuthResp struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	Error                   string `json:"error"`
	ErrorDescription        string `json:"error_description"`
}

// Start 申请设备码。调用方拿到返回的 VerifyURL 展示给用户（含用户码的直达链接）。
func (d *DeviceFlow) Start(ctx context.Context, reusePlatformID int64) (*DeviceSession, error) {
	form := url.Values{
		"client_id": {d.clientID},
		"scope":     {chmlfrpScope},
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/x-www-form-urlencoded")
	hdr.Set("Accept", "application/json")
	raw, status, err := request(ctx, d.hc, http.MethodPost, QzhuaDeviceAuthEP, hdr, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("连接轻爪账户失败: %w", err)
	}
	var da deviceAuthResp
	if err := decodeJSON(raw, &da); err != nil {
		return nil, fmt.Errorf("账户中心返回异常响应（HTTP %d）: %w", status, err)
	}
	if da.Error != "" || da.DeviceCode == "" {
		msg := da.ErrorDescription
		if msg == "" {
			msg = da.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return nil, fmt.Errorf("申请设备授权失败: %s", msg)
	}
	expiresIn := da.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 300
	}
	interval := da.Interval
	if interval <= 0 {
		interval = 5
	}
	verify := da.VerificationURIComplete
	if verify == "" {
		verify = da.VerificationURI
	}
	s := &DeviceSession{
		ID:         randomID(),
		UserCode:   da.UserCode,
		VerifyURL:  verify,
		Interval:   interval,
		ExpiresIn:  expiresIn,
		Status:     "pending",
		PlatformID: reusePlatformID,
		deviceCode: da.DeviceCode,
		expiresAt:  time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	d.mu.Lock()
	d.gcLocked()
	d.sessions[s.ID] = s
	d.mu.Unlock()
	return s.snapshot(), nil
}

// Poll 推进一次会话。返回的 Status 为 pending 时表示用户尚未完成授权。
// 会话已 OK/错误时直接回放结果，不再打上游（前端轮询可能比 interval 密）。
func (d *DeviceFlow) Poll(ctx context.Context, id string) (*DeviceSession, error) {
	d.mu.Lock()
	s := d.sessions[id]
	if s == nil {
		d.mu.Unlock()
		return nil, errors.New("授权会话不存在或已过期，请重新发起")
	}
	if s.Status != "pending" {
		out := s.snapshot()
		d.mu.Unlock()
		return out, nil
	}
	if time.Now().After(s.expiresAt) {
		s.Status = "expired"
		s.Error = "授权超时，请重新发起"
		out := s.snapshot()
		d.mu.Unlock()
		return out, nil
	}
	// 上游 interval 未到时只回放本地状态，避免把平台的限速触发成 invalid_grant
	if time.Since(s.lastPoll) < time.Duration(s.Interval)*time.Second {
		out := s.snapshot()
		d.mu.Unlock()
		return out, nil
	}
	s.lastPoll = time.Now()
	deviceCode := s.deviceCode
	d.mu.Unlock()

	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {d.clientID},
		"device_code": {deviceCode},
	}
	tok, err := d.oauthPost(ctx, QzhuaTokenEP, form)

	d.mu.Lock()
	defer d.mu.Unlock()
	s = d.sessions[id]
	if s == nil {
		return nil, errors.New("授权会话已失效")
	}
	if err != nil {
		var oe *oauthError
		if errors.As(err, &oe) {
			switch oe.Code {
			case "authorization_pending":
				return s.snapshot(), nil
			case "slow_down":
				s.Interval += 5
				return s.snapshot(), nil
			case "expired_token":
				s.Status = "expired"
				s.Error = "授权码已过期，请重新发起"
				return s.snapshot(), nil
			case "access_denied":
				s.Status = "error"
				s.Error = "用户拒绝了授权"
				return s.snapshot(), nil
			default:
				s.Status = "error"
				s.Error = oe.Error()
				return s.snapshot(), nil
			}
		}
		return nil, err
	}
	s.Status = "ok"
	s.token = tok
	return s.snapshot(), nil
}

// Token 取出会话换到的令牌（仅 Status=ok 时有效）。
func (d *DeviceFlow) Token(id string) *Token {
	d.mu.Lock()
	defer d.mu.Unlock()
	if s := d.sessions[id]; s != nil {
		return s.token
	}
	return nil
}

// SetUsername 授权成功后补记用户名，供前端提示。
func (d *DeviceFlow) SetUsername(id, name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if s := d.sessions[id]; s != nil {
		s.Username = name
	}
}

// Cancel 取消会话（用户关弹窗）。
func (d *DeviceFlow) Cancel(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sessions, id)
}

func (s *DeviceSession) snapshot() *DeviceSession {
	cp := *s
	cp.deviceCode = ""
	cp.token = nil
	return &cp
}

// gcLocked 清理过期会话；调用方须持锁。
func (d *DeviceFlow) gcLocked() {
	if len(d.sessions) < 32 {
		return
	}
	now := time.Now()
	for id, s := range d.sessions {
		if now.After(s.expiresAt.Add(10*time.Minute)) || (s.Status != "pending" && now.After(s.expiresAt)) {
			delete(d.sessions, id)
		}
	}
}

// RefreshChmlfrpToken 用 refresh_token 换新的 access_token。
func RefreshChmlfrpToken(ctx context.Context, clientID, refreshToken string) (*Token, error) {
	if strings.TrimSpace(clientID) == "" {
		clientID = DefaultChmlfrpClientID
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/x-www-form-urlencoded")
	hdr.Set("Accept", "application/json")
	hc := newHTTPClient(15 * time.Second)
	raw, status, err := request(ctx, hc, http.MethodPost, QzhuaTokenEP, hdr, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("连接轻爪账户失败: %w", err)
	}
	var t Token
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("账户中心返回非法响应（HTTP %d）", status)
	}
	if t.Error != "" {
		if t.Error == "invalid_grant" {
			return nil, fmt.Errorf("%w（ChmlFrp 刷新令牌已失效）", ErrAuth)
		}
		return nil, fmt.Errorf("刷新令牌失败: %s %s", t.Error, t.ErrorDescription)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("账户中心 HTTP %d", status)
	}
	return &t, nil
}
