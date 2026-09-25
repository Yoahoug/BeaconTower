package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
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
			middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
			return
		}
		middleware.OK(c, gin.H{"range": rng, "points": rows})
		return
	}
	// 短期走原始采样（降采样到 ≤240 点）
	samples, err := a.DB.SamplesInRange(id, from, now, 2000)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	middleware.OK(c, gin.H{"range": rng, "points": downsampleMetrics(samples, 240, showPowerPublic(settings))})
	return
}

// GET /api/v1/public/stream（SSE：首帧 snapshot + 每轮 update + 15s ping）
func (a *App) PublicStream(c *gin.Context) {
	if a.privateMode() && !a.loggedIn(c) {
		middleware.AbortCode(c, http.StatusUnauthorized, 1002, "未登录或会话已过期")
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	snap := a.Coll.SnapshotNow()
	writeSSE(c, "snapshot", snap.Servers)
	c.Writer.Flush()

	ch, unsub := a.Coll.Subscribe()
	defer unsub()
	ping := newIntervalTicker(15 * time.Second)
	defer ping.Stop()
	notify := c.Writer.CloseNotify()
	for {
		select {
		case <-notify:
			return
		case <-c.Request.Context().Done():
			return
		case s, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(c, "update", s.Servers)
			c.Writer.Flush()
		case <-ping.C:
			_, _ = c.Writer.WriteString(": ping\n\n")
			c.Writer.Flush()
		}
	}
}

func writeSSE(c *gin.Context, event string, v any) {
	b, _ := jsonMarshal(v)
	_, _ = c.Writer.WriteString("event: " + event + "\n")
	for _, line := range strings.Split(string(b), "\n") {
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

// downsampleMetrics 降采样到 ≤n 点（等距抽稀，保证首尾；公开字段白名单）。
func downsampleMetrics(samples []*store.Metric, n int, showPower bool) []map[string]any {
	if len(samples) == 0 {
		return []map[string]any{}
	}
	step := 1
	if len(samples) > n {
		step = (len(samples) + n - 1) / n
	}
	out := []map[string]any{}
	for i := 0; i < len(samples); i += step {
		m := samples[i]
		p := map[string]any{
			"ts": m.Ts, "cpu_pct": m.CpuPct, "mem_used": m.MemUsed,
			"disk_used": m.DiskUsed, "net_in_bps": m.NetInBps, "net_out_bps": m.NetOutBps,
		}
		if showPower && m.PowerW.Valid {
			p["power_w"] = m.PowerW.Float64
		}
		out = append(out, p)
	}
	// 保证尾点
	last := samples[len(samples)-1]
	if len(out) == 0 || out[len(out)-1]["ts"] != last.Ts {
		p := map[string]any{
			"ts": last.Ts, "cpu_pct": last.CpuPct, "mem_used": last.MemUsed,
			"disk_used": last.DiskUsed, "net_in_bps": last.NetInBps, "net_out_bps": last.NetOutBps,
		}
		if showPower && last.PowerW.Valid {
			p["power_w"] = last.PowerW.Float64
		}
		out = append(out, p)
	}
	return out
}

var _ = time.Now
