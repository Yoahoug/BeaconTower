package tasks

import (
	"database/sql"
	"log"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/wg"
)

// Start 后台任务：采样清理（10min）+ 小时聚合（整点+5min 时触发检查）+ 会话清理（每小时）
// + WG 组网巡检（5min：握手判活/hub 流量差值/配置漂移）+ 月累计 kWh 月初清零。
// retention 天数从 setting 表读取（doc/03 §3）。
func Start(db *store.DB, wgRunner *wg.Runner, stop <-chan struct{}) {
	go func() {
		cleanTick := time.NewTicker(10 * time.Minute)
		defer cleanTick.Stop()
		aggTick := time.NewTicker(5 * time.Minute)
		defer aggTick.Stop()
		sessTick := time.NewTicker(time.Hour)
		defer sessTick.Stop()
		wgTick := time.NewTicker(5 * time.Minute)
		defer wgTick.Stop()
		curMonth := time.Now().Format("2006-01")
		for {
			select {
			case <-stop:
				return
			case <-cleanTick.C:
				cleanup(db)
			case <-aggTick.C:
				aggregate(db)
			case <-sessTick.C:
				_ = db.CleanExpiredSessions(time.Now().Unix())
			case <-wgTick.C:
				if wgRunner != nil {
					wgRunner.Patrol()
				}
			}
			// 月翻转检查放在所有 case 之后统一做（ticker 周期远小于月份粒度）
			if m := time.Now().Format("2006-01"); m != curMonth {
				curMonth = m
				if err := db.ResetAllMonthKwh(); err != nil {
					log.Printf("[tasks] reset month kwh: %v", err)
				} else {
					log.Printf("[tasks] month rolled over to %s, month_kwh reset", m)
				}
			}
		}
	}()
}

func retentionOf(db *store.DB) (sampleDays int, hourlyDays int) {
	s, _ := db.GetSettings()
	sampleDays = atoi(s["retention_days"], 30)
	if sampleDays < 1 {
		sampleDays = 1
	}
	if sampleDays > 365 {
		sampleDays = 365
	}
	// 原始采样保留 24h（doc/03），retention_days 控制小时聚合保留
	hourlyDays = sampleDays
	return 1, hourlyDays
}

func cleanup(db *store.DB) {
	sampleDays, hourlyDays := retentionOf(db)
	now := time.Now().Unix()
	if n, err := db.DeleteSamplesBefore(now - int64(sampleDays)*86400); err != nil {
		log.Printf("[tasks] clean samples: %v", err)
	} else if n > 0 {
		log.Printf("[tasks] cleaned %d samples", n)
	}
	hourCut := (now - int64(hourlyDays)*86400) / 3600 * 3600
	if n, err := db.DeleteHourlyBefore(hourCut); err != nil {
		log.Printf("[tasks] clean hourly: %v", err)
	} else if n > 0 {
		log.Printf("[tasks] cleaned %d hourly rows", n)
	}
	if n, err := db.DeleteDailyBefore((now - int64(hourlyDays)*86400) / 86400 * 86400); err != nil {
		log.Printf("[tasks] clean daily: %v", err)
	} else if n > 0 {
		log.Printf("[tasks] cleaned %d daily rows", n)
	}
	// 流量日记录与小时聚合同生命周期更新（今日实时 + 昨日定稿）
	aggregateDailyTraffic(db)
}

// dayFloorTs 当日零点（本地时区）。
func dayFloorTs(now int64) int64 {
	t := time.Unix(now, 0)
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return t.Unix()
}

// aggregateDailyTraffic 按日流量聚合：今日（实时，用截至当前的样本首尾差）+
// 昨日（全天样本定稿）。首尾累计计数器差；回绕/重启归零（last < first）时
// 退化为「当日最后一个有效窗口差」累加不可行，直接取 0 并保留已有较大值（幂等 upsert 取 max 语义由调用方保证）。
func aggregateDailyTraffic(db *store.DB) {
	now := time.Now().Unix()
	today := dayFloorTs(now)
	yesterday := today - 86400
	servers, err := db.ListServers()
	if err != nil {
		return
	}
	for _, s := range servers {
		// 今日：零点至今（实时值，随轮次收敛）
		if in, out, ok := trafficDelta(db, s.ID, today, now); ok {
			_ = db.UpsertDailyTraffic(s.ID, today, in, out, now)
		}
		// 昨日：全天定稿（只在有完整覆盖时重算，幂等）
		if in, out, ok := trafficDelta(db, s.ID, yesterday, today-1); ok {
			_ = db.UpsertDailyTraffic(s.ID, yesterday, in, out, now)
		}
	}
}

// trafficDelta [from,to] 窗口流量：首个与最后一个样本的累计计数器差。
// 样本不足 2 条、计数器未初始化（0）或发生回绕（重启归零）时返回 false（不写库）。
func trafficDelta(db *store.DB, serverID, from, to int64) (inTotal, outTotal int64, ok bool) {
	samples, err := db.SamplesInRange(serverID, from, to, 0)
	if err != nil || len(samples) < 2 {
		return 0, 0, false
	}
	first, last := samples[0], samples[len(samples)-1]
	if first.NetInTotal <= 0 || first.NetOutTotal <= 0 {
		return 0, 0, false // 旧数据（schema v5 前无累计列）
	}
	if last.NetInTotal < first.NetInTotal || last.NetOutTotal < first.NetOutTotal {
		return 0, 0, false // 回绕/重启归零：窗口不可信，跳过
	}
	return last.NetInTotal - first.NetInTotal, last.NetOutTotal - first.NetOutTotal, true
}

// aggregate 上一完整小时聚合：cpu avg/max、mem avg/max、net 均值、
// 小时累计流量（首尾累计计数器差，回绕/重启归零时跳过）、power avg + kWh 梯形积分。
func aggregate(db *store.DB) {
	now := time.Now().Unix()
	hourEnd := now - now%3600
	hourStart := hourEnd - 3600
	// 整点后 5 分钟内才跑，避免数据不全
	if now-hourEnd < 300 {
		// 仍聚合上上小时（幂等 upsert，可重复跑）
		hourEnd, hourStart = hourStart, hourStart-3600
	}
	servers, err := db.ListServers()
	if err != nil {
		return
	}
	for _, s := range servers {
		// 不限条数：首尾累计差需要完整小时样本
		samples, err := db.SamplesInRange(s.ID, hourStart, hourEnd-1, 0)
		if err != nil || len(samples) == 0 {
			continue
		}
		var cpuSum, cpuMax float64
		var memSum int64
		var memMax int64
		var inSum, outSum float64
		var pSum float64
		var pN int
		var kwh float64
		var lastW float64
		var lastTs int64
		for i, m := range samples {
			cpuSum += m.CpuPct
			if m.CpuPct > cpuMax {
				cpuMax = m.CpuPct
			}
			memSum += m.MemUsed
			if m.MemUsed > memMax {
				memMax = m.MemUsed
			}
			inSum += m.NetInBps
			outSum += m.NetOutBps
			if m.PowerW.Valid {
				pSum += m.PowerW.Float64
				pN++
				if i > 0 && lastTs > 0 && m.Ts > lastTs && m.Ts-lastTs < 300 {
					kwh += (lastW + m.PowerW.Float64) / 2 * float64(m.Ts-lastTs) / 3.6e6
				}
				lastW, lastTs = m.PowerW.Float64, m.Ts
			}
		}
		n := float64(len(samples))
		// 小时累计流量：首尾累计计数器差（非负；回绕/重启归零则该小时置 0）
		first, last := samples[0], samples[len(samples)-1]
		var inTot, outTot int64
		if first.NetInTotal > 0 && last.NetInTotal >= first.NetInTotal {
			inTot = last.NetInTotal - first.NetInTotal
		}
		if first.NetOutTotal > 0 && last.NetOutTotal >= first.NetOutTotal {
			outTot = last.NetOutTotal - first.NetOutTotal
		}
		var pAvg, kwhV sql.NullFloat64
		if pN > 0 {
			pAvg = sql.NullFloat64{Float64: pSum / float64(pN), Valid: true}
			kwhV = sql.NullFloat64{Float64: kwh, Valid: true}
		}
		_ = db.UpsertHourly(s.ID, hourStart, cpuSum/n, cpuMax, memSum/int64(n), memMax,
			inSum/n, outSum/n, inTot, outTot, pAvg, kwhV)
	}
}

func atoi(s string, def int) int {
	var n int
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if s == "" {
		return def
	}
	return n
}
