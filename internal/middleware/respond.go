package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- 响应包 ----------

// OK 成功包 {code:0,msg,data}（doc/04）。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": data})
}

func abortCode(c *gin.Context, status, code int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"code": code, "msg": msg, "data": nil})
}

// AbortCode 供 handler 包使用。
func AbortCode(c *gin.Context, status, code int, msg string) { abortCode(c, status, code, msg) }

// Fail 参数错误等业务失败（HTTP 200 + code≠0，前端按 code 解包）。
func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg, "data": nil})
}

func nowUnix() int64 { return time.Now().Unix() }

// ---------- 限速 ----------

// RateLimiter 简易内存滑动窗口限速（key=IP+路径）。
type RateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]int64
	Limit  int
	Window time.Duration
	Code   int
	Msg    string
}

func NewRateLimiter(limit int, window time.Duration, code int, msg string) *RateLimiter {
	return &RateLimiter{hits: map[string][]int64{}, Limit: limit, Window: window, Code: code, Msg: msg}
}

func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := clientIPOf(c) + "|" + c.FullPath()
		now := time.Now().Unix()
		r.mu.Lock()
		window := r.Window.Nanoseconds() / 1e9
		kept := r.hits[key][:0]
		for _, t := range r.hits[key] {
			if now-t < window {
				kept = append(kept, t)
			}
		}
		if len(kept) >= r.Limit {
			r.hits[key] = kept
			r.mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests,
				gin.H{"code": r.Code, "msg": r.Msg, "data": nil})
			return
		}
		r.hits[key] = append(kept, now)
		r.mu.Unlock()
		c.Next()
	}
}

// ---------- 登录递增封禁（doc/05 §3） ----------

// LoginBlocker 连续失败递增封禁：5m→15m→1h→24h。
type LoginBlocker struct {
	mu       sync.Mutex
	failures map[string]*failState
}

type failState struct {
	count       int
	blockedTill int64
}

func NewLoginBlocker() *LoginBlocker {
	return &LoginBlocker{failures: map[string]*failState{}}
}

func blockDuration(count int) time.Duration {
	switch {
	case count <= 5:
		return 5 * time.Minute
	case count <= 8:
		return 15 * time.Minute
	case count <= 11:
		return time.Hour
	default:
		return 24 * time.Hour
	}
}

// Check 失败次数达到阈值（5 次）后封禁。
func (b *LoginBlocker) Check(ip string) (blocked bool, retryAfter int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.failures[ip]
	if st == nil {
		return false, 0
	}
	now := time.Now().Unix()
	if now < st.blockedTill {
		return true, st.blockedTill - now
	}
	return false, 0
}

func (b *LoginBlocker) Fail(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.failures[ip]
	if st == nil {
		st = &failState{}
		b.failures[ip] = st
	}
	st.count++
	if st.count >= 5 {
		st.blockedTill = time.Now().Add(blockDuration(st.count)).Unix()
	}
}

func (b *LoginBlocker) Success(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.failures, ip)
}
