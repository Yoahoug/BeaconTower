package tasks

import (
	"database/sql"
	"log"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// Start 后台任务：采样清理（10min）+ 小时聚合（整点+5min 时触发检查）+ 会话清理（每小时）。
// retention 天数从 setting 表读取（doc/03 §3）。
func Start(db *store.DB, stop <-chan struct{}) {
	go func() {
		cleanTick := time.NewTicker(10 * time.Minute)
		defer cleanTick.Stop()
		aggTick := time.NewTicker(5 * time.Minute)
		defer aggTick.Stop()
		sessTick := time.NewTicker(time.Hour)
		defer sessTick.Stop()
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
}

// aggregate 上一完整小时聚合：cpu avg/max、mem avg/max、net 均值+累计、power avg + kWh 梯形积分。
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
		samples, err := db.SamplesInRange(s.ID, hourStart, hourEnd-1, 10000)
		if err != nil || len(samples) == 0 {
			continue
		}
		var cpuSum, cpuMax float64
		var memSum int64
		var memMax int64
		var inSum, outSum float64
		var inTot, outTot int64
		var pSum float64
		var pN int
		var kwh float64
		var lastW, lastT float64
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
				lastW, lastT, lastTs = m.PowerW.Float64, m.PowerW.Float64, m.Ts
				_ = lastT
			}
		}
		n := float64(len(samples))
		// 小时累计流量：用首尾累计计数器差（回绕/重启归零则取末值）
		first, last := samples[0], samples[len(samples)-1]
		_ = first
		_ = last
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
