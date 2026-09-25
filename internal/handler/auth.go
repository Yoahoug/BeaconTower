package handler

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/gin-gonic/gin"
)

// 密码强度（与前端 passwordStrength 同规则：≥10 位含大小写+数字+符号）。
func passwordOK(pw string) bool {
	if len(pw) < 10 {
		return false
	}
	var lower, upper, digit, symbol bool
	for _, r := range pw {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			symbol = true
		}
	}
	return lower && upper && digit && symbol
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]{3,32}$`)

// GET /api/v1/admin/status
func (a *App) Status(c *gin.Context) {
	has, err := a.DB.HasAdmin()
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	settings, _ := a.DB.GetSettings()
	siteTitle := a.Cfg.SiteTitle
	if settings["site_title"] != "" {
		siteTitle = settings["site_title"]
	}
	out := gin.H{"initialized": has, "loggedIn": false, "site_title": siteTitle, "username": ""}
	if has {
		if token, err := c.Cookie("bt_session"); err == nil && token != "" {
			if ok, _ := a.DB.GetSession(crypto.HashToken(token), nowUnix()); ok {
				admin, _ := a.DB.GetAdmin()
				out["loggedIn"] = true
				if admin != nil {
					out["username"] = admin.Username
				}
			}
		}
	}
	middleware.OK(c, out)
}

// POST /api/v1/admin/setup
func (a *App) Setup(c *gin.Context) {
	has, _ := a.DB.HasAdmin()
	if has {
		middleware.Fail(c, 1004, "已初始化，禁止重复初始化")
		return
	}
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		SetupToken string `json:"setup_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	// 可选初始化令牌（doc/04 §2.2）
	if a.Cfg.SetupToken != "" && req.SetupToken != a.Cfg.SetupToken {
		middleware.Fail(c, 1001, "参数错误：setup_token 不正确")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !usernameRe.MatchString(req.Username) {
		middleware.Fail(c, 1001, "参数错误：用户名须为 3–32 位字母/数字/下划线/点/横线")
		return
	}
	if !passwordOK(req.Password) {
		middleware.Fail(c, 1001, "参数错误：密码至少 10 位，须含大小写字母、数字与符号")
		return
	}
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	if err := a.DB.CreateAdmin(req.Username, hash, nowUnix()); err != nil {
		middleware.Fail(c, 1004, "已初始化，禁止重复初始化")
		return
	}
	a.issueSession(c, req.Username)
	a.audit(req.Username, "setup", "admin", "初始化管理员 "+req.Username, ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// POST /api/v1/admin/login
func (a *App) Login(c *gin.Context) {
	ip := ipOf(c)
	if blocked, _ := a.Blocker.Check(ip); blocked {
		middleware.AbortCode(c, http.StatusTooManyRequests, 1006, "登录失败过多，账号已临时锁定，请稍后再试")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	admin, _ := a.DB.GetAdmin()
	fail := func() {
		a.Blocker.Fail(ip)
		a.audit(req.Username, "login_fail", "admin", "登录失败（"+orEmpty(req.Username)+")", ip)
		middleware.Fail(c, 1005, "用户名或密码错误")
	}
	if admin == nil || admin.Username != req.Username {
		fail()
		return
	}
	if !crypto.VerifyPassword(req.Password, admin.PasswordHash) {
		fail()
		return
	}
	a.Blocker.Success(ip)
	_ = a.DB.TouchAdminLogin(nowUnix())
	a.issueSession(c, admin.Username)
	a.audit(admin.Username, "login", "admin", "管理员登录成功", ip)
	middleware.OK(c, gin.H{"ok": true})
}

// POST /api/v1/admin/logout
func (a *App) Logout(c *gin.Context) {
	username := ""
	if v, ok := c.Get(middleware.CtxUsername); ok {
		username, _ = v.(string)
	}
	if token, err := c.Cookie("bt_session"); err == nil && token != "" {
		_ = a.DB.DeleteSession(crypto.HashToken(token))
	}
	clearSessionCookie(c)
	if username == "" {
		if admin, _ := a.DB.GetAdmin(); admin != nil {
			username = admin.Username
		}
	}
	a.audit(username, "logout", "admin", "管理员登出", ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// POST /api/v1/admin/password（改密：验旧密码 + 热更新 + 吊销其他会话）
func (a *App) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	admin, _ := a.DB.GetAdmin()
	if admin == nil || !crypto.VerifyPassword(req.OldPassword, admin.PasswordHash) {
		middleware.Fail(c, 1001, "参数错误：旧密码不正确")
		return
	}
	if !passwordOK(req.NewPassword) {
		middleware.Fail(c, 1001, "参数错误：新密码至少 10 位，须含大小写字母、数字与符号")
		return
	}
	hash, err := crypto.HashPassword(req.NewPassword)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	if err := a.DB.UpdateAdminPassword(hash); err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	// 热更新即时生效：吊销其他会话，当前会话保持（doc/05 + 前端注释 M1 语义）
	if v, ok := c.Get(middleware.CtxSessionHash); ok {
		if h, ok := v.(string); ok && h != "" {
			_ = a.DB.DeleteOtherSessions(h)
		}
	}
	a.audit(admin.Username, "password", "admin", "修改管理员密码（热更新，即时生效）", ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

func (a *App) issueSession(c *gin.Context, username string) {
	token, err := crypto.NewSessionToken()
	if err != nil {
		return
	}
	_ = a.DB.CreateSession(crypto.HashToken(token), nowUnix()+24*3600, nowUnix())
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("bt_session", token, 24*3600, "/", "", secure, true)
	// CSRF 双提交 token（可读 cookie，前端 http.js 自动附 X-CSRF-Token）
	csrf, _ := crypto.NewSessionToken()
	c.SetCookie("bt_csrf", csrf[:32], 24*3600, "/", "", secure, false)
	c.Set(middleware.CtxUsername, username)
	c.Set(middleware.CtxSessionHash, crypto.HashToken(token))
}

func clearSessionCookie(c *gin.Context) {
	c.SetCookie("bt_session", "", -1, "/", "", false, true)
}

func ipOf(c *gin.Context) string {
	if v, ok := c.Get(middleware.CtxClientIP); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return c.ClientIP()
}

func orEmpty(s string) string {
	if s == "" {
		return "空用户名"
	}
	return s
}

var _ = time.Now
