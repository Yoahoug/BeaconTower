package collector

import (
	"context"
	"database/sql"
	"log"
	"math"
	"sync"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/geoip"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// Snapshot 内存快照：一轮采集的最新全量（SSE 广播源）。
type Snapshot struct {
	Servers []map[string]any `json:"servers"`
	Summary map[string]any   `json:"summary"`
	Ts      int64            `json:"ts"`
}

// Collector 采集器：每节点 goroutine + Ticker + jitter（doc/02 §4.3）。
type Collector struct {
	cfg     *config.Config
	db      *store.DB
	master  []byte
	mu      sync.RWMutex
	snap    *Snapshot
	subs    map[chan *Snapshot]struct{}
	subsMu  sync.Mutex
	prev    map[int64]*prevState // 面板侧差分基线（cpu/net/rapl）
	stop    chan struct{}
	stopped chan struct{}
}

type prevState struct {
	cpuTotal, cpuIdle uint64
	ts                int64
	netRx, netTx      uint64
	rapl              map[string]uint64
	raplAt            int64
	smoothW           float64 // EMA 平滑后整机功率
	lastW             float64 // 上轮整机功率（kWh 梯形积分）
	lastWAt           int64
	batCalib          [][2]float64
	calibrated        bool
}

func New(cfg *config.Config, db *store.DB, master []byte) *Collector {
	return &Collector{
		cfg: cfg, db: db, master: master,
		subs: map[chan *Snapshot]struct{}{},
		prev: map[int64]*prevState{},
		stop: make(chan struct{}), stopped: make(chan struct{}),
	}
}

// SnapshotNow 返回当前内存快照（公开 API 直接读内存）。
func (c *Collector) SnapshotNow() *Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.snap == nil {
		return &Snapshot{Servers: []map[string]any{}, Summary: map[string]any{}, Ts: time.Now().Unix()}
	}
	return c.snap
}

// Subscribe 订阅快照广播（SSE 用，带缓冲防慢消费者阻塞采集）。
func (c *Collector) Subscribe() (chan *Snapshot, func()) {
	ch := make(chan *Snapshot, 4)
	c.subsMu.Lock()
	c.subs[ch] = struct{}{}
	c.subsMu.Unlock()
	return ch, func() {
		c.subsMu.Lock()
		delete(c.subs, ch)
		close(ch)
		c.subsMu.Unlock()
	}
}

func (c *Collector) broadcast(s *Snapshot) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for ch := range c.subs {
		select {
		case ch <- s:
		default:
		}
	}
}

// Start 启动采集主循环：每 interval 一轮，全节点并发采集后统一落库+广播。
func (c *Collector) Start() {
	go c.loop()
}

func (c *Collector) Stop() {
	close(c.stop)
	<-c.stopped
}

func (c *Collector) loop() {
	defer close(c.stopped)
	interval := time.Duration(c.cfg.CollectInterval) * time.Second
	// 首轮立即执行一次（冷启动有数据），之后按间隔
	c.tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			c.tick()
		}
	}
}

func (c *Collector) tick() {
	servers, err := c.db.ListServers()
	if err != nil {
		log.Printf("[collector] list servers: %v", err)
		return
	}
	now := time.Now().Unix()
	var wg sync.WaitGroup
	for _, s := range servers {
		if !s.Enabled {
			continue
		}
		wg.Add(1)
		go func(srv *store.Server) {
			defer wg.Done()
			// jitter 由并发调度的自然错峰承担（单轮内各节点并行建连）
			c.collectOne(srv, now)
		}(s)
	}
	wg.Wait()
	c.rebuildSnapshot()
	// private_mode 下 SSE 同样要求登录，由 handler 层控制；此处只管广播
	c.mu.RLock()
	s := c.snap
	c.mu.RUnlock()
	if s != nil {
		c.broadcast(s)
	}
}

func (c *Collector) credFor(serverID int64) (*store.Credential, *SSHCred, error) {
	cred, err := c.db.GetCredential(serverID)
	if err != nil || cred == nil {
		return nil, nil, err
	}
	pw, err := crypto.DecryptString(c.master, cred.PasswordEnc)
	if err != nil {
		return cred, nil, err
	}
	key, err := crypto.DecryptString(c.master, cred.PrivateKeyEnc)
	if err != nil {
		return cred, nil, err
	}
	pp, err := crypto.DecryptString(c.master, cred.PassphraseEnc)
	if err != nil {
		return cred, nil, err
	}
	return cred, &SSHCred{
		Host: cred.Host, Port: cred.Port, Username: cred.Username,
		AuthType: cred.AuthType, Password: pw, PrivateKey: key, Passphrase: pp,
	}, nil
}

func (c *Collector) collectOne(srv *store.Server, now int64) {
	cred, sshCred, err := c.credFor(srv.ID)
	if err != nil || sshCred == nil {
		c.markFail(srv.ID, "凭据解密失败", now)
		return
	}
	settings, _ := c.db.GetSettings()
	strict := settings["strict_host_key"] == "true"
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	res, err := DialAndCollect(ctx, sshCred, cred.HostKeyFP, strict)
	if err != nil {
		c.markFail(srv.ID, err.Error(), now)
		return
	}
	// TOFU：首次记录指纹；非严格模式下指纹变化则更新
	if cred.HostKeyFP == "" && res.HostKeyFP != "" {
		_ = c.db.SetCollectResult(srv.ID, res.HostKeyFP, "", now)
		cred.HostKeyFP = res.HostKeyFP
	} else if res.HostKeyFP != "" && res.HostKeyFP != cred.HostKeyFP && !strict {
		_ = c.db.SetCollectResult(srv.ID, res.HostKeyFP, "", now)
	}
	c.applySample(srv, cred, res, now)
}

func (c *Collector) markFail(serverID int64, reason string, now int64) {
	_ = c.db.SetCollectResult(serverID, "", reason, nil)
	m := &store.Metric{ServerID: serverID, Ts: now, Status: "offline"}
	_ = c.db.UpsertLatest(m)
	// offline 不写 sample（曲线自然断点，前端显示离线）
}

// applySample 差分计算（CPU/网速/RAPL 功率）+ 落库 + kWh 积分。
func (c *Collector) applySample(srv *store.Server, cred *store.Credential, res *DialResult, now int64) {
	raw := res.Raw
	p := c.prev[srv.ID]
	if p == nil {
		p = &prevState{rapl: map[string]uint64{}}
		c.prev[srv.ID] = p
	}
	m := &store.Metric{ServerID: srv.ID, Ts: now, Status: "online"}

	// CPU：两次 /proc/stat 差值
	if p.ts > 0 && raw.CpuTotal > p.cpuTotal {
		dTotal := float64(raw.CpuTotal - p.cpuTotal)
		dIdle := float64(raw.CpuIdle - p.cpuIdle)
		if dTotal > 0 {
			m.CpuPct = clampPct((1 - dIdle/dTotal) * 100)
		}
	}
	// 网速：字节差 ÷ 实际间隔
	dt := float64(now - p.ts)
	if p.ts > 0 && dt > 0 && raw.NetRx >= p.netRx && raw.NetTx >= p.netTx {
		m.NetInBps = float64(raw.NetRx-p.netRx) * 8 / dt
		m.NetOutBps = float64(raw.NetTx-p.netTx) * 8 / dt
	}
	p.cpuTotal, p.cpuIdle, p.ts = raw.CpuTotal, raw.CpuIdle, now
	p.netRx, p.netTx = raw.NetRx, raw.NetTx

	m.MemUsed, m.MemTotal = raw.MemUsed, raw.MemTotal
	m.SwapUsed, m.SwapTotal = raw.SwapUsed, raw.SwapTotal
	m.DiskUsed, m.DiskTotal = raw.DiskUsed, raw.DiskTotal
	m.NetInTotal, m.NetOutTotal = int64(raw.NetRx), int64(raw.NetTx)
	m.TcpConns, m.UdpConns = raw.TcpConns, raw.UdpConns
	m.Load1, m.Load5, m.Load15 = raw.Load1, raw.Load5, raw.Load15
	m.UptimeS = raw.UptimeS
	m.Processes = raw.Processes

	// 画像（低频字段）：每次采集都写回画像（开销小，保证画像新鲜；region 自动定位仅 auto 时覆盖）
	prof, _ := c.db.GetProfile(srv.ID)
	if prof == nil {
		prof = &store.Profile{ServerID: srv.ID, BaseLoadSource: "default"}
	}
	prof.Hostname, prof.OsName, prof.OsVersion = raw.Hostname, raw.OsName, raw.OsVer
	prof.Kernel, prof.Arch, prof.CpuModel = raw.Kernel, raw.Arch, raw.CpuModel
	prof.CpuCores = raw.CpuCores
	prof.MemTotal, prof.SwapTotal, prof.DiskTotal = raw.MemTotal, raw.SwapTotal, raw.DiskTotal
	prof.DisksJSON = disksJSON(raw.DisksRaw)
	prof.Virt = normalizeVirt(raw.Virt)
	if raw.PubIP != "" && raw.PubIP != prof.PublicIP {
		prof.PublicIP = raw.PubIP
		if r := geoip.Lookup(raw.PubIP); r != nil && srv.RegionSource == "auto" {
			_ = c.db.UpdateRegion(srv.ID, geoip.RegionText(r), "auto")
		}
		prof.GeoCountry, prof.GeoCity = geoCountryCity(raw.PubIP)
	}
	prof.PowerRapL, prof.PowerBattery = raw.HasRapl, raw.HasBat
	if prof.BaseLoadSource == "" {
		prof.BaseLoadSource = "default"
	}

	// 功耗（doc/09 §3）：RAPL 差分 + 回绕 + 异常过滤 + EMA + kWh 梯形积分
	powerW, cpuW, dramW := c.powerFor(srv.ID, prof, raw, now, p)
	if raw.HasRapl {
		m.PowerW = sqlFloat(powerW)
		m.CpuW = sqlFloat(cpuW)
		m.DramW = sqlFloat(dramW)
		if t, ok := pickTemp(raw.Thermal); ok {
			m.TempC = sqlFloat(t)
		}
		if raw.FreqMhz > 0 {
			m.FreqMhz = sqlInt(raw.FreqMhz)
		}
		m.PowerSrc = sqlStr(powerSrcOf(raw))
		// kWh 梯形积分进月累计
		if p.lastW > 0 && p.lastWAt > 0 && now > p.lastWAt && now-p.lastWAt < 300 {
			kwh := (p.lastW + powerW) / 2 * float64(now-p.lastWAt) / 3.6e6
			if kwh > 0 && kwh < 1 {
				_ = c.db.AddMonthKwh(srv.ID, kwh)
			}
		}
		p.lastW, p.lastWAt = powerW, now
	}
	_ = c.db.UpsertProfile(prof, now)
	_ = c.db.UpsertLatest(m)
	_ = c.db.InsertSample(m)
	_ = c.db.SetCollectResult(srv.ID, "", "", now)
}

// powerFor RAPL 功率计算：package+core+uncore 取 package 域；dram 独立；整机 = package + base。
func (c *Collector) powerFor(serverID int64, prof *store.Profile, raw *RawSample, now int64, p *prevState) (total, cpu, dram float64) {
	if !raw.HasRapl || len(p.rapl) == 0 {
		// 首轮无基线：记录基线，不产出功率
		p.rapl = copyRapl(raw.Rapl)
		p.raplAt = now
		if p.smoothW == 0 {
			p.smoothW = prof.BaseLoadW
		}
		return p.smoothW, 0, 0
	}
	dt := float64(now - p.raplAt)
	if dt <= 0 || dt > 300 {
		p.rapl = copyRapl(raw.Rapl)
		p.raplAt = now
		return p.smoothW, 0, 0
	}
	pkgW := domainWatts(raw.Rapl, p.rapl, raw.RaplMax, dt, []string{"intel-rapl:0", "intel-rapl:1"})
	dramW := domainWatts(raw.Rapl, p.rapl, raw.RaplMax, dt, []string{"intel-rapl:0:2", "intel-rapl:1:2"})
	p.rapl = copyRapl(raw.Rapl)
	p.raplAt = now
	// 异常值过滤：单域 > 100W 丢弃本轮
	if pkgW > 100 || dramW > 100 || pkgW < 0 || dramW < 0 {
		return p.smoothW, 0, 0
	}
	base := prof.BaseLoadW
	if base <= 0 {
		base = 5 // 默认估算（doc/09）
	}
	var inst float64
	if raw.BatStatus == "Discharging" && raw.BatPowerU > 0 {
		// 放电时 power_now 即整机真实功耗
		inst = float64(raw.BatPowerU) / 1e6
		p.batCalib = append(p.batCalib, [2]float64{inst, pkgW + dramW})
		if len(p.batCalib) >= 30 && !p.calibrated && prof.BaseLoadSource != "manual" {
			c.settleCalibration(serverID, prof, p)
		}
	} else {
		inst = pkgW + base
	}
	// EMA 平滑
	if p.smoothW == 0 {
		p.smoothW = inst
	} else {
		p.smoothW += 0.35 * (inst - p.smoothW)
	}
	return p.smoothW, pkgW, dramW
}

// settleCalibration 电池自动校准结算（doc/09 §2.3）：base = mean(power_now) − mean(pkg+dram)，2–40W 才采纳。
func (c *Collector) settleCalibration(serverID int64, prof *store.Profile, p *prevState) {
	var sTotal, sParts float64
	for _, s := range p.batCalib {
		sTotal += s[0]
		sParts += s[1]
	}
	n := float64(len(p.batCalib))
	base := sTotal/n - sParts/n
	p.calibrated = true
	if base >= 2 && base <= 40 && !math.IsNaN(base) {
		prof.BaseLoadW = math.Round(base*10) / 10
		prof.BaseLoadSource = "calibration"
		_ = c.db.UpdatePowerCalibration(serverID, prof.BaseLoadW, "calibration")
	}
}

func domainWatts(cur, last map[string]uint64, maxm map[string]uint64, dt float64, names []string) float64 {
	var w float64
	for _, n := range names {
		c, ok1 := cur[n]
		l, ok2 := last[n]
		if !ok1 || !ok2 {
			continue
		}
		var delta float64
		if c >= l {
			delta = float64(c - l)
		} else {
			// 回绕
			mx, ok := maxm[n]
			if !ok || mx == 0 {
				continue
			}
			delta = float64(mx-l) + float64(c)
		}
		w += delta / 1e6 / dt
		if w > 0 {
			break // 多 package 取第一个有效域即可（psys 另计，此处保守取 package）
		}
	}
	return w
}

func copyRapl(m map[string]uint64) map[string]uint64 {
	out := map[string]uint64{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func clampPct(v float64) float64 {
	if v != v {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return math.Round(v*10) / 10
}

func powerSrcOf(raw *RawSample) string {
	if raw.BatStatus == "Discharging" {
		return "battery"
	}
	return "ac"
}

func pickTemp(zones map[string]int64) (float64, bool) {
	pref := []string{"x86_pkg_temp", "k10temp", "cpu-thermal", "acpitz"}
	for _, p := range pref {
		for typ, t := range zones {
			if containsFold(typ, p) {
				return float64(t) / 1000, true
			}
		}
	}
	for _, t := range zones {
		return float64(t) / 1000, true
	}
	return 0, false
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if equalFold(s[i:i+len(sub)], sub) {
					return true
				}
			}
			return false
		}())
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func disksJSON(raw string) string {
	if raw == "" {
		return "[]"
	}
	return raw // "mnt|total|avail;..." 透传，管理端按需展示
}

func normalizeVirt(v string) string {
	switch v {
	case "kvm":
		return "KVM"
	case "none", "", "unknown":
		return "物理机"
	default:
		return v
	}
}

func geoCountryCity(pubIP string) (string, string) {
	if r := geoip.Lookup(pubIP); r != nil {
		return r.Country, r.City
	}
	return "", ""
}

func sqlFloat(v float64) sqlNullF { return sqlNullF{Float64: v, Valid: true} }
func sqlInt(v int64) sqlNullI     { return sqlNullI{Int64: v, Valid: true} }
func sqlStr(v string) sqlNullS    { return sqlNullS{String: v, Valid: true} }

// sql Nullable 别名（避免 collector  import database/sql 处处写全名）
type (
	sqlNullF = sql.NullFloat64
	sqlNullI = sql.NullInt64
	sqlNullS = sql.NullString
)
