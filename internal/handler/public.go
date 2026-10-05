package handler

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/gin-gonic/gin"
)

// GET /api/v1/public/summary（private_mode 开启时要求登录）
func (a *App) PublicSummary(c *gin.Context) {
	if a.privateMode() && !a.loggedIn(c) {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	snap := a.Coll.SnapshotNow()
	sum := snap.Summary
	middleware.OK(c, gin.H{
		"total": sum["total"], "online": sum["online"], "offline": sum["offline"],
		"up_bps": sum["up_bps"], "down_bps": sum["down_bps"],
		"watts": sum["watts"], "measured_count": sum["measured_count"],
		"month_kwh": sum["month_kwh"], "est_cost_month": sum["est_cost_month"],
		"day_in_total": sum["day_in_total"], "day_out_total": sum["day_out_total"],
	})
}

// GET /api/v1/public/servers
func (a *App) PublicServers(c *gin.Context) {
	if a.privateMode() && !a.loggedIn(c) {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	snap := a.Coll.SnapshotNow()
	if snap.Servers == nil {
		snap.Servers = []map[string]any{}
	}
	middleware.OK(c, snap.Servers)
}

// GET /api/v1/public/servers/:id/history?range=1h|6h|24h|7d
func (a *App) PublicHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	srv, err := a.DB.GetServer(id)
	if err != nil || srv == nil || srv.Hidden {
		middleware.Fail(c, 2002, "节点不存在")
		return
	}
	rng := c.DefaultQuery("range", "1h")
	if rng != "1h" && rng != "6h" && rng != "24h" && rng != "7d" {
		middleware.Fail(c, 1001, "参数错误：range 须为 1h|6h|24h|7d")
		return
	}
	logged := a.loggedIn(c)
	if a.privateMode() && !logged {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	settings, _ := a.DB.GetSettings()
	if rng == "7d" && settings["open_7d_history"] != "true" && !logged {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "长期历史需登录后查看")
		return
	}
	now := nowUnix()
	key := "h|" + itoa(id) + "|" + rng
	a.respondCached(c, key, func() (any, error) {
		var from int64
		switch rng {
		case "1h":
			from = now - 3600
		case "6h":
			from = now - 6*3600
		case "24h":
			from = now - 24*3600
		default:
			// 7d 走小时聚合
			rows, err := a.DB.HourlyInRange(id, hourFloor(now-7*86400), hourFloor(now))
			if err != nil {
				return nil, err
			}
			return gin.H{"range": rng, "points": rows}, nil
		}
		// 短期走原始采样。窗口聚合（等宽分桶取均值）而非 LIMIT 截断：
		// 6h/24h 在 10s 采样下有 2k/8k 点，截断会让曲线只画到半程。
		// 分桶数按范围定：1h→60 点，6h→120，24h→144（每点 10/3/10 分钟）。
		buckets := 60
		if rng == "6h" {
			buckets = 120
		} else if rng == "24h" {
			buckets = 144
		}
		rows, err := a.DB.SampleWindows(id, from, now, buckets)
		if err != nil {
			return nil, err
		}
		return gin.H{"range": rng, "points": windowPoints(rows, showPowerPublic(settings))}, nil
	})
}

// GET /api/v1/public/servers/:id/traffic?days=30 （按日流量记录：收发字节）
func (a *App) PublicTraffic(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	srv, err := a.DB.GetServer(id)
	if err != nil || srv == nil || srv.Hidden {
		middleware.Fail(c, 2002, "节点不存在")
		return
	}
	if a.privateMode() && !a.loggedIn(c) {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days < 1 || days > 365 {
		days = 30
	}
	now := nowUnix()
	today := dayFloor(now)
	key := "t|" + itoa(id) + "|" + itoa(int64(days))
	a.respondCached(c, key, func() (any, error) {
		rows, err := a.DB.DailyTrafficRange(id, today-int64(days-1)*86400, today)
		if err != nil {
			return nil, err
		}
		return gin.H{"days": days, "points": rows}, nil
	})
}

func dayFloor(ts int64) int64 {
	t := time.Unix(ts, 0)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Unix()
}

// historyCache history/traffic 响应的进程内短 TTL 缓存（doc/14 滥用面收敛）：
// 公网匿名可无限刷这两类 SQL 重接口，缓存把单 IP 限速之外的全局成本钉死。
// 键 = id|range（history）与 id|days（traffic）；值 = 预序列化响应体。
type historyCache struct {
	mu    sync.Mutex
	byKey map[string]cacheEntry
}

type cacheEntry struct {
	body    []byte
	expires int64
}

// historyCacheTTL 缓存存活秒数。曲线粒度最小 10s 一点、轮询 10s 一轮，
// 5s 的缓存对页面观感无影响，但把无限并发刷库收敛成每键每 5s 至多一次 SQL。
const historyCacheTTL = 5

// historyCacheMax 条目上限（id × range 组合有限，正常远到不了；防御性兜底）。
const historyCacheMax = 1024

func newHistoryCache() *historyCache { return &historyCache{byKey: map[string]cacheEntry{}} }

func (h *historyCache) get(key string) ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.byKey[key]
	if !ok || e.expires < nowUnix() {
		return nil, false
	}
	return e.body, true
}

func (h *historyCache) put(key string, body []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.byKey) >= historyCacheMax {
		// 简单防膨胀：整体清空重来（过期条目下次 get 也会被忽略）
		h.byKey = map[string]cacheEntry{}
	}
	h.byKey[key] = cacheEntry{body: body, expires: nowUnix() + historyCacheTTL}
}

// respondCached 命中缓存直接回预序列化包体；未命中由 produce 生成后回填。
func (a *App) respondCached(c *gin.Context, key string, produce func() (any, error)) {
	cache := a.histCacheOf()
	if body, ok := cache.get(key); ok {
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return
	}
	data, err := produce()
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	body, merr := jsonMarshal(gin.H{"code": 0, "msg": "ok", "data": data})
	if merr != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	cache.put(key, body)
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// windowPoints SQL 分桶聚合行 → 曲线点（字段白名单同 downsampleMetrics）。
func windowPoints(rows []map[string]any, showPower bool) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		p := map[string]any{
			"ts": r["ts"], "cpu_pct": r["cpu"], "mem_used": r["mem"],
			"disk_used": r["disk"], "net_in_bps": r["net_in"], "net_out_bps": r["net_out"],
		}
		if showPower {
			p["power_w"] = r["power"]
		}
		out = append(out, p)
	}
	return out
}

// GET /api/v1/public/stream（SSE：首帧 snapshot + 每轮 update + 15s ping）
func (a *App) PublicStream(c *gin.Context) {
	if a.privateMode() && !a.loggedIn(c) {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	// 释放 SSEGate 的单 IP 并发名额（含正常返回与 panic 两条路径）
	defer middleware.ReleaseSSE(c)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	// 每次写帧限时：客户端收包卡死时写端最多阻塞 30s，然后由写超时触发
	// 连接关闭，flush goroutine 不会无限堆积（连接从 ConnContext 拿）。
	if conn := middleware.ConnOf(c.Request.Context()); conn != nil {
		defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	}
	writeSSE(c, "snapshot", a.Coll.SSEFrame())
	c.Writer.Flush()

	ch, unsub := a.Coll.Subscribe()
	defer unsub()
	ping := newIntervalTicker(15 * time.Second)
	defer ping.Stop()
	notify := c.Writer.CloseNotify()
	for {
		if conn := middleware.ConnOf(c.Request.Context()); conn != nil {
			_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		}
		select {
		case <-notify:
			return
		case <-c.Request.Context().Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(c, "update", a.Coll.SSEFrame())
			c.Writer.Flush()
		case <-ping.C:
			_, _ = c.Writer.WriteString(": ping\n\n")
			c.Writer.Flush()
		}
	}
}

// writeSSE data 载荷传预序列化字节（本轮所有 SSE 连接共用同一份 Marshal 结果）。
func writeSSE(c *gin.Context, event string, data []byte) {
	_, _ = c.Writer.WriteString("event: " + event + "\n")
	for _, line := range strings.Split(string(data), "\n") {
		_, _ = c.Writer.WriteString("data: " + line + "\n")
	}
	_, _ = c.Writer.WriteString("\n")
}

func (a *App) privateMode() bool {
	s, _ := a.DB.GetSettings()
	return s["private_mode"] == "true"
}

func (a *App) loggedIn(c *gin.Context) bool {
	token, err := c.Cookie("bt_session")
	if err != nil || token == "" {
		return false
	}
	ok, _ := a.DB.GetSession(hashToken(token), nowUnix())
	return ok
}

func hourFloor(ts int64) int64 { return ts - ts%3600 }

func showPowerPublic(settings map[string]string) bool {
	return settings["show_power_public"] != "false"
}

var _ = time.Now
