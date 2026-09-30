package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/gin-gonic/gin"
)

// Ctx keys
const (
	CtxUsername    = "bt_username"
	CtxSessionHash = "bt_session_hash"
	CtxClientIP    = "bt_client_ip"
)

const sessionCookie = "bt_session"

// ClientIP 反代可信时取 X-Real-IP，否则直连 IP（doc/05 §2.3）。
func ClientIP(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.Request.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		// 只有「直连对端本身」落在可信代理网段时才采信转发头。
		// 旧实现是拿 X-Real-IP 的**取值**去匹配可信网段，等于任何来源
		// 自报一个内网地址就能改写面板记录的客户端 IP（审计与限流都受影响）。
		if len(cfg.TrustedProxies) > 0 {
			if peer := net.ParseIP(ip); peer != nil {
				for _, n := range cfg.TrustedProxies {
					if !n.Contains(peer) {
						continue
					}
					if real := net.ParseIP(strings.TrimSpace(c.GetHeader("X-Real-IP"))); real != nil {
						ip = real.String()
					}
					break
				}
			}
		}
		c.Set(CtxClientIP, ip)
		c.Next()
	}
}

func clientIPOf(c *gin.Context) string {
	if v, ok := c.Get(CtxClientIP); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return c.ClientIP()
}

// SecurityHeaders 安全响应头（doc/05 §3）。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

// RequireAuth 管理 API 会话校验（滑动过期 24h）。
func RequireAuth(db *store.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookie)
		if err != nil || token == "" {
			abortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
			return
		}
		hash := crypto.HashToken(token)
		now := nowUnix()
		expiresAt, ok, err := db.SessionExpiresAt(hash, now)
		if err != nil || !ok {
			abortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
			return
		}
		// 滑动过期：仅当剩余不足 1h 时才续满 24h。监控页每 10s 轮询一次，
		// 每请求都 UPDATE 会让会话行一天被重写八千多次；降频后有效语义
		// 不变（持续使用则不过期），最长续期间隙 1h。
		if now+3600 >= expiresAt {
			_ = db.TouchSession(hash, now+24*3600)
		}
		c.Set(CtxSessionHash, hash)
		c.Next()
	}
}

// RequireCSRF 非 GET 管理 API 双提交校验（doc/04 §2.4）。
func RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		cookie, err := c.Cookie("bt_csrf")
		header := c.GetHeader("X-CSRF-Token")
		if err != nil || cookie == "" || header == "" || !crypto.SubtleEqualFold(cookie, header) {
			abortCode(c, http.StatusForbidden, 1003, "CSRF 校验失败")
			return
		}
		c.Next()
	}
}
