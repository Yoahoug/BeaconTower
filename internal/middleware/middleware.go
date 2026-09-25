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
		if len(cfg.TrustedProxies) > 0 {
			if real := strings.TrimSpace(c.GetHeader("X-Real-IP")); real != "" {
				if parsed := net.ParseIP(real); parsed != nil {
					for _, n := range cfg.TrustedProxies {
						if n.Contains(parsed) {
							ip = real
							break
						}
					}
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
		ok, err := db.GetSession(hash, nowUnix())
		if err != nil || !ok {
			abortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
			return
		}
		// 滑动过期
		_ = db.TouchSession(hash, nowUnix()+24*3600)
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
