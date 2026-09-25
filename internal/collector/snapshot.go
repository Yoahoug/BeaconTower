package collector

import (
	"database/sql"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// rebuildSnapshot 从 DB 联查构建内存快照（公开 API 直接读内存，不查库）。
func (c *Collector) rebuildSnapshot() {
	rows, err := c.db.SnapshotRows()
	if err != nil {
		return
	}
	settings, _ := c.db.GetSettings()
	showPower := settings["show_power_public"] != "false"
	showCost := settings["show_cost_public"] != "false"
	price := parsePrice(settings["electric_price"])
	now := time.Now().Unix()

	servers := []map[string]any{}
	var upBps, downBps, watts float64
	online, measured := 0, 0
	var monthKwh, estCost float64

	for _, r := range rows {
		s := r.Srv
		if s.Hidden {
			continue
		}
		status := "offline"
		var m map[string]any
		if r.M != nil && r.M.Status == "online" && now-r.M.Ts < 60 {
			status = "online"
		}
		prof := r.Prof
		cpuCores, memTotal, diskTotal := 0, int64(0), int64(0)
		osName, osVer, arch, virt := "", "", "", ""
		if prof != nil {
			cpuCores, memTotal, diskTotal = prof.CpuCores, prof.MemTotal, prof.DiskTotal
			osName, osVer, arch, virt = prof.OsName, prof.OsVersion, prof.Arch, prof.Virt
		}
		profile := map[string]any{
			"os": osName, "os_version": osVer, "arch": arch,
			"cpu_cores": cpuCores, "mem_total": memTotal,
			"disk_total": diskTotal, "virt": virt,
		}
		if status == "online" {
			online++
			mm := r.M
			upBps += mm.NetOutBps
			downBps += mm.NetInBps
			m = map[string]any{
				"ts": mm.Ts, "cpu_pct": mm.CpuPct,
				"mem_used": mm.MemUsed, "swap_used": mm.SwapUsed, "disk_used": mm.DiskUsed,
				"net_in_bps": mm.NetInBps, "net_out_bps": mm.NetOutBps,
				"tcp_conns": mm.TcpConns, "udp_conns": mm.UdpConns,
				"load1": mm.Load1, "load5": mm.Load5, "load15": mm.Load15,
				"uptime_s": mm.UptimeS, "processes": mm.Processes,
			}
		} else {
			m = map[string]any{
				"ts": int64(0), "cpu_pct": 0, "mem_used": 0, "swap_used": 0,
				"disk_used": 0, "net_in_bps": 0, "net_out_bps": 0,
				"tcp_conns": 0, "udp_conns": 0,
				"load1": 0, "load5": 0, "load15": 0, "uptime_s": 0, "processes": 0,
			}
		}
		item := map[string]any{
			"id": s.ID, "name": s.Name, "region": s.Region,
			"region_source": s.RegionSource, "tags": s.Tags,
			"note_public": s.NotePublic, "status": status,
			"profile": profile, "metrics": m,
		}
		// 功耗（受站点开关控制；RAPL 不可用输出 null）
		if showPower {
			pw := powerOf(r, prof, price, showCost)
			item["power"] = pw
			if pw != nil && status == "online" {
				measured++
				if w, ok := pw["total_w"].(float64); ok {
					watts += w
				}
				if k, ok := pw["month_kwh"].(float64); ok {
					monthKwh += k
				}
				if e, ok := pw["est_cost_month"].(float64); ok {
					estCost += e
				}
			}
		}
		servers = append(servers, item)
	}
	total := len(servers)
	summary := map[string]any{
		"total": total, "online": online, "offline": total - online,
		"up_bps": upBps, "down_bps": downBps,
		"watts": watts, "measured_count": measured,
		"month_kwh": monthKwh, "est_cost_month": estCost,
	}
	c.mu.Lock()
	c.snap = &Snapshot{Servers: servers, Summary: summary, Ts: now}
	c.mu.Unlock()
}

func powerOf(r *store.SnapshotRow, prof *store.Profile, price float64, showCost bool) map[string]any {
	if prof == nil || !prof.PowerRapL || r.M == nil || !r.M.PowerW.Valid {
		return nil
	}
	pw := map[string]any{
		"total_w": round1(r.M.PowerW.Float64),
		"cpu_w":   round1(nullF64(r.M.CpuW)),
		"dram_w":  round1(nullF64(r.M.DramW)),
		"temp_c":  nullableRound1F64(r.M.TempC),
		"freq_mhz": func() any {
			if r.M.FreqMhz.Valid {
				return r.M.FreqMhz.Int64
			}
			return nil
		}(),
		"power_source": func() string {
			if r.M.PowerSrc.Valid && r.M.PowerSrc.String != "" {
				return r.M.PowerSrc.String
			}
			return "ac"
		}(),
		// 今日 kWh 由 sample 窗口推算（M5 接入）；此处先置 nil，月累计用画像累计
		"today_kwh": nil,
		"month_kwh": round3(prof.MonthKwh),
	}
	if showCost {
		pw["est_cost_month"] = round2(prof.MonthKwh * price)
	}
	return pw
}

func parsePrice(s string) float64 {
	var f float64
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			return 0.6
		}
	}
	_, _ = sscan(s, &f)
	if f < 0 || f > 99 {
		return 0.6
	}
	return f
}

func sscan(s string, f *float64) (int, error) {
	// 轻量解析，避免引入额外依赖
	var v float64
	var dec float64 = 1
	seen := false
	afterDot := false
	neg := false
	for i, ch := range s {
		switch {
		case i == 0 && ch == '-':
			neg = true
		case ch >= '0' && ch <= '9':
			seen = true
			if afterDot {
				dec *= 10
				v += float64(ch-'0') / dec
			} else {
				v = v*10 + float64(ch-'0')
			}
		case ch == '.':
			afterDot = true
		default:
			if seen {
				if neg {
					v = -v
				}
				*f = v
				return 1, nil
			}
			return 0, nil
		}
	}
	if neg {
		v = -v
	}
	*f = v
	return 1, nil
}

func nullF64(n sql.NullFloat64) float64 {
	if n.Valid {
		return n.Float64
	}
	return 0
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
func round3(v float64) float64 { return float64(int(v*1000+0.5)) / 1000 }

func nullableRound1F64(n sql.NullFloat64) any {
	if n.Valid {
		return round1(n.Float64)
	}
	return nil
}
