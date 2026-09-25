package store

import (
	"database/sql"
	"encoding/json"
)

// ---------- 领域模型 ----------

type Admin struct {
	Username     string
	PasswordHash string
	CreatedAt    int64
	LastLoginAt  sql.NullInt64
}

type Server struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Region       string   `json:"region"`
	RegionSource string   `json:"region_source"`
	Tags         []string `json:"tags"`
	NotePublic   string   `json:"note_public"`
	NotePrivate  string   `json:"note_private"`
	SortOrder    int      `json:"sort_order"`
	Hidden       bool     `json:"hidden"`
	Enabled      bool     `json:"enabled"`
	CreatedAt    int64    `json:"created_at"`
}

type Credential struct {
	ServerID      int64
	Host          string
	Port          int
	Username      string
	AuthType      string // password | key
	PasswordEnc   []byte
	PrivateKeyEnc []byte
	PassphraseEnc []byte
	HostKeyFP     string
	LastError     string
	LastSuccessAt sql.NullInt64
}

type Profile struct {
	ServerID       int64
	Hostname       string
	OsName         string
	OsVersion      string
	Kernel         string
	Arch           string
	CpuModel       string
	CpuCores       int
	MemTotal       int64
	SwapTotal      int64
	DiskTotal      int64
	DisksJSON      string
	Virt           string
	PublicIP       string
	GeoCountry     string
	GeoCity        string
	PowerRapL      bool
	PowerBattery   bool
	BaseLoadW      float64
	BaseLoadSource string
	MonthKwh       float64
}

type Metric struct {
	ServerID    int64
	Ts          int64
	Status      string
	CpuPct      float64
	MemUsed     int64
	MemTotal    int64
	SwapUsed    int64
	SwapTotal   int64
	DiskUsed    int64
	DiskTotal   int64
	NetInBps    float64
	NetOutBps   float64
	NetInTotal  int64
	NetOutTotal int64
	TcpConns    int
	UdpConns    int
	Load1       float64
	Load5       float64
	Load15      float64
	UptimeS     int64
	Processes   int
	PowerW      sql.NullFloat64
	CpuW        sql.NullFloat64
	DramW       sql.NullFloat64
	TempC       sql.NullFloat64
	FreqMhz     sql.NullInt64
	PowerSrc    sql.NullString
}

type AuditEntry struct {
	ID           int64  `json:"id"`
	Ts           int64  `json:"ts"`
	Actor        string `json:"actor"`
	Action       string `json:"action"`
	Target       string `json:"target"`
	Detail       string `json:"detail"`
	SourceIPHash string `json:"source_ip_hash"`
}

// ---------- admin ----------

func (db *DB) HasAdmin() (bool, error) {
	var n int
	err := db.SQL.QueryRow(`SELECT COUNT(*) FROM admin WHERE id = 1`).Scan(&n)
	return n > 0, err
}

func (db *DB) GetAdmin() (*Admin, error) {
	a := &Admin{}
	err := db.SQL.QueryRow(`SELECT username, password_hash, created_at, last_login_at FROM admin WHERE id = 1`).
		Scan(&a.Username, &a.PasswordHash, &a.CreatedAt, &a.LastLoginAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db *DB) CreateAdmin(username, hash string, now int64) error {
	_, err := db.SQL.Exec(`INSERT INTO admin (id, username, password_hash, created_at) VALUES (1, ?, ?, ?)`,
		username, hash, now)
	return err
}

func (db *DB) UpdateAdminPassword(hash string) error {
	_, err := db.SQL.Exec(`UPDATE admin SET password_hash = ? WHERE id = 1`, hash)
	return err
}

func (db *DB) TouchAdminLogin(now int64) error {
	_, err := db.SQL.Exec(`UPDATE admin SET last_login_at = ? WHERE id = 1`, now)
	return err
}

// ---------- session ----------

func (db *DB) CreateSession(tokenHash string, expiresAt, now int64) error {
	_, err := db.SQL.Exec(`INSERT INTO session (token_hash, expires_at, created_at) VALUES (?, ?, ?)`,
		tokenHash, expiresAt, now)
	return err
}

func (db *DB) GetSession(tokenHash string, now int64) (bool, error) {
	var n int
	err := db.SQL.QueryRow(`SELECT COUNT(*) FROM session WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, now).Scan(&n)
	return n > 0, err
}

func (db *DB) TouchSession(tokenHash string, expiresAt int64) error {
	_, err := db.SQL.Exec(`UPDATE session SET expires_at = ? WHERE token_hash = ?`, expiresAt, tokenHash)
	return err
}

func (db *DB) DeleteSession(tokenHash string) error {
	_, err := db.SQL.Exec(`DELETE FROM session WHERE token_hash = ?`, tokenHash)
	return err
}

// 改密时吊销除当前会话外的其他会话（热更新即时生效，见 M1）。
func (db *DB) DeleteOtherSessions(keepHash string) error {
	_, err := db.SQL.Exec(`DELETE FROM session WHERE token_hash != ?`, keepHash)
	return err
}

func (db *DB) CleanExpiredSessions(now int64) error {
	_, err := db.SQL.Exec(`DELETE FROM session WHERE expires_at <= ?`, now)
	return err
}

// ---------- setting ----------

var defaultSettings = map[string]string{
	"site_title":        "BeaconTower",
	"interval_s":        "10",
	"retention_days":    "30",
	"open_7d_history":   "false",
	"show_power_public": "true",
	"show_cost_public":  "true",
	"electric_price":    "0.6",
	"strict_host_key":   "false",
	"private_mode":      "false",
}

// GetSettings 返回合并默认值后的全部设置。
func (db *DB) GetSettings() (map[string]string, error) {
	out := map[string]string{}
	for k, v := range defaultSettings {
		out[k] = v
	}
	rows, err := db.SQL.Query(`SELECT key, value FROM setting`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (db *DB) SaveSettings(m map[string]string) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range m {
		if _, err := tx.Exec(`INSERT INTO setting (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- audit ----------

func (db *DB) AddAudit(e AuditEntry) error {
	_, err := db.SQL.Exec(`INSERT INTO audit_log (ts, actor, action, target, detail, source_ip_hash)
		VALUES (?, ?, ?, ?, ?, ?)`, e.Ts, e.Actor, e.Action, e.Target, e.Detail, e.SourceIPHash)
	return err
}

func (db *DB) ListAudit(page, size int) (int, []AuditEntry, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	var total int
	if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := db.SQL.Query(`SELECT id, ts, actor, action, target, detail, COALESCE(source_ip_hash,'')
		FROM audit_log ORDER BY id DESC LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	var items []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Ts, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.SourceIPHash); err != nil {
			return 0, nil, err
		}
		items = append(items, e)
	}
	if items == nil {
		items = []AuditEntry{}
	}
	return total, items, rows.Err()
}

// ---------- server ----------

func scanServer(s *Server, tags string, hidden, enabled int, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&s.ID, &s.Name, &s.Region, &s.RegionSource, &tags,
		&s.NotePublic, &s.NotePrivate, &s.SortOrder, &hidden, &enabled, &s.CreatedAt)
}

func (db *DB) ListServers() ([]*Server, error) {
	rows, err := db.SQL.Query(`SELECT id, name, region, region_source, tags, note_public, note_private,
		sort_order, hidden, enabled, created_at FROM server ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Server
	for rows.Next() {
		s := &Server{}
		var tags string
		var hidden, enabled int
		if err := rows.Scan(&s.ID, &s.Name, &s.Region, &s.RegionSource, &tags,
			&s.NotePublic, &s.NotePrivate, &s.SortOrder, &hidden, &enabled, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Tags = decodeTags(tags)
		s.Hidden = hidden == 1
		s.Enabled = enabled == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) GetServer(id int64) (*Server, error) {
	s := &Server{}
	var tags string
	var hidden, enabled int
	err := db.SQL.QueryRow(`SELECT id, name, region, region_source, tags, note_public, note_private,
		sort_order, hidden, enabled, created_at FROM server WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.Region, &s.RegionSource, &tags,
			&s.NotePublic, &s.NotePrivate, &s.SortOrder, &hidden, &enabled, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Tags = decodeTags(tags)
	s.Hidden = hidden == 1
	s.Enabled = enabled == 1
	return s, nil
}

func decodeTags(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var t []string
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return []string{}
	}
	return t
}

func encodeTags(t []string) string {
	if len(t) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(t)
	return string(b)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (db *DB) CreateServer(s *Server) (int64, error) {
	res, err := db.SQL.Exec(`INSERT INTO server
		(name, region, region_source, tags, note_public, note_private, sort_order, hidden, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Name, s.Region, s.RegionSource, encodeTags(s.Tags), s.NotePublic, s.NotePrivate,
		s.SortOrder, boolInt(s.Hidden), boolInt(s.Enabled), s.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) UpdateServer(s *Server) error {
	_, err := db.SQL.Exec(`UPDATE server SET name=?, region=?, region_source=?, tags=?,
		note_public=?, note_private=?, hidden=?, enabled=? WHERE id=?`,
		s.Name, s.Region, s.RegionSource, encodeTags(s.Tags), s.NotePublic, s.NotePrivate,
		boolInt(s.Hidden), boolInt(s.Enabled), s.ID)
	return err
}

func (db *DB) DeleteServer(id int64) error {
	_, err := db.SQL.Exec(`DELETE FROM server WHERE id = ?`, id)
	return err
}

func (db *DB) ReorderServers(ids []int64) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE server SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- credential ----------

func (db *DB) GetCredential(serverID int64) (*Credential, error) {
	c := &Credential{}
	err := db.SQL.QueryRow(`SELECT server_id, host, port, username, auth_type,
		password_enc, private_key_enc, passphrase_enc, COALESCE(host_key_fp,''),
		COALESCE(last_error,''), last_success_at
		FROM server_credential WHERE server_id = ?`, serverID).
		Scan(&c.ServerID, &c.Host, &c.Port, &c.Username, &c.AuthType,
			&c.PasswordEnc, &c.PrivateKeyEnc, &c.PassphraseEnc,
			&c.HostKeyFP, &c.LastError, &c.LastSuccessAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (db *DB) UpsertCredential(c *Credential) error {
	_, err := db.SQL.Exec(`INSERT INTO server_credential
		(server_id, host, port, username, auth_type, password_enc, private_key_enc, passphrase_enc, host_key_fp, last_error, last_success_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id) DO UPDATE SET host=excluded.host, port=excluded.port,
			username=excluded.username, auth_type=excluded.auth_type,
			password_enc=excluded.password_enc, private_key_enc=excluded.private_key_enc,
			passphrase_enc=excluded.passphrase_enc, host_key_fp=excluded.host_key_fp,
			last_error=excluded.last_error, last_success_at=excluded.last_success_at`,
		c.ServerID, c.Host, c.Port, c.Username, c.AuthType,
		c.PasswordEnc, c.PrivateKeyEnc, c.PassphraseEnc,
		c.HostKeyFP, c.LastError, nullInt(c.LastSuccessAt))
	return err
}

func nullInt(n sql.NullInt64) any {
	if n.Valid {
		return n.Int64
	}
	return nil
}

func (db *DB) SetCollectResult(serverID int64, fp, lastErr string, successAt any) error {
	_, err := db.SQL.Exec(`UPDATE server_credential SET host_key_fp = COALESCE(NULLIF(?,''), host_key_fp),
		last_error = ?, last_success_at = ? WHERE server_id = ?`, fp, lastErr, successAt, serverID)
	return err
}

// ---------- profile ----------

func (db *DB) GetProfile(serverID int64) (*Profile, error) {
	p := &Profile{}
	err := db.SQL.QueryRow(`SELECT server_id, COALESCE(hostname,''), COALESCE(os_name,''), COALESCE(os_version,''),
		COALESCE(kernel,''), COALESCE(arch,''), COALESCE(cpu_model,''), COALESCE(cpu_cores,0),
		COALESCE(mem_total,0), COALESCE(swap_total,0), COALESCE(disk_total,0), COALESCE(disks_json,''),
		COALESCE(virt,''), COALESCE(public_ip,''), COALESCE(geo_country,''), COALESCE(geo_city,''),
		COALESCE(power_rapl,0), COALESCE(power_battery,0), COALESCE(base_load_w,0),
		COALESCE(base_load_source,'default'), COALESCE(month_kwh,0)
		FROM server_profile WHERE server_id = ?`, serverID).
		Scan(&p.ServerID, &p.Hostname, &p.OsName, &p.OsVersion, &p.Kernel, &p.Arch, &p.CpuModel,
			&p.CpuCores, &p.MemTotal, &p.SwapTotal, &p.DiskTotal, &p.DisksJSON, &p.Virt,
			&p.PublicIP, &p.GeoCountry, &p.GeoCity,
			&p.PowerRapL, &p.PowerBattery, &p.BaseLoadW, &p.BaseLoadSource, &p.MonthKwh)
	// sqlite bool 列以整数存取
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpsertProfile 全字段写入（采集画像/试连画像共用）。
func (db *DB) UpsertProfile(p *Profile, now int64) error {
	_, err := db.SQL.Exec(`INSERT INTO server_profile
		(server_id, hostname, os_name, os_version, kernel, arch, cpu_model, cpu_cores,
		 mem_total, swap_total, disk_total, disks_json, virt, public_ip, geo_country, geo_city,
		 power_rapl, power_battery, base_load_w, base_load_source, month_kwh, collected_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(server_id) DO UPDATE SET hostname=excluded.hostname, os_name=excluded.os_name,
			os_version=excluded.os_version, kernel=excluded.kernel, arch=excluded.arch,
			cpu_model=excluded.cpu_model, cpu_cores=excluded.cpu_cores, mem_total=excluded.mem_total,
			swap_total=excluded.swap_total, disk_total=excluded.disk_total, disks_json=excluded.disks_json,
			virt=excluded.virt, public_ip=excluded.public_ip, geo_country=excluded.geo_country,
			geo_city=excluded.geo_city, power_rapl=excluded.power_rapl, power_battery=excluded.power_battery,
			base_load_w=excluded.base_load_w, base_load_source=excluded.base_load_source,
			collected_at=excluded.collected_at`,
		p.ServerID, p.Hostname, p.OsName, p.OsVersion, p.Kernel, p.Arch, p.CpuModel, p.CpuCores,
		p.MemTotal, p.SwapTotal, p.DiskTotal, p.DisksJSON, p.Virt, p.PublicIP, p.GeoCountry, p.GeoCity,
		boolInt(p.PowerRapL), boolInt(p.PowerBattery), p.BaseLoadW, p.BaseLoadSource, p.MonthKwh, now)
	return err
}

func (db *DB) UpdateRegion(serverID int64, region, source string) error {
	_, err := db.SQL.Exec(`UPDATE server SET region = ?, region_source = ? WHERE id = ?`,
		region, source, serverID)
	return err
}

func (db *DB) UpdatePowerCalibration(serverID int64, baseW float64, source string) error {
	_, err := db.SQL.Exec(`UPDATE server_profile SET base_load_w = ?, base_load_source = ?
		WHERE server_id = ?`, baseW, source, serverID)
	return err
}

func (db *DB) AddMonthKwh(serverID int64, delta float64) error {
	_, err := db.SQL.Exec(`UPDATE server_profile SET month_kwh = COALESCE(month_kwh,0) + ?
		WHERE server_id = ?`, delta, serverID)
	return err
}

// ---------- metrics ----------

func (db *DB) UpsertLatest(m *Metric) error {
	_, err := db.SQL.Exec(`INSERT INTO latest_metric
		(server_id, ts, status, cpu_pct, mem_used, mem_total, swap_used, swap_total,
		 disk_used, disk_total, net_in_bps, net_out_bps, net_in_total, net_out_total,
		 tcp_conns, udp_conns, load1, load5, load15, uptime_s, processes,
		 power_w, cpu_w, dram_w, temp_c, freq_mhz, power_src)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(server_id) DO UPDATE SET ts=excluded.ts, status=excluded.status,
			cpu_pct=excluded.cpu_pct, mem_used=excluded.mem_used, mem_total=excluded.mem_total,
			swap_used=excluded.swap_used, swap_total=excluded.swap_total,
			disk_used=excluded.disk_used, disk_total=excluded.disk_total,
			net_in_bps=excluded.net_in_bps, net_out_bps=excluded.net_out_bps,
			net_in_total=excluded.net_in_total, net_out_total=excluded.net_out_total,
			tcp_conns=excluded.tcp_conns, udp_conns=excluded.udp_conns,
			load1=excluded.load1, load5=excluded.load5, load15=excluded.load15,
			uptime_s=excluded.uptime_s, processes=excluded.processes,
			power_w=excluded.power_w, cpu_w=excluded.cpu_w, dram_w=excluded.dram_w,
			temp_c=excluded.temp_c, freq_mhz=excluded.freq_mhz, power_src=excluded.power_src`,
		m.ServerID, m.Ts, m.Status, m.CpuPct, m.MemUsed, m.MemTotal, m.SwapUsed, m.SwapTotal,
		m.DiskUsed, m.DiskTotal, m.NetInBps, m.NetOutBps, m.NetInTotal, m.NetOutTotal,
		m.TcpConns, m.UdpConns, m.Load1, m.Load5, m.Load15, m.UptimeS, m.Processes,
		nullFloat(m.PowerW), nullFloat(m.CpuW), nullFloat(m.DramW),
		nullFloat(m.TempC), nullInt64(m.FreqMhz), nullStr(m.PowerSrc))
	return err
}

func nullFloat(n sql.NullFloat64) any {
	if n.Valid {
		return n.Float64
	}
	return nil
}

func nullInt64(n sql.NullInt64) any {
	if n.Valid {
		return n.Int64
	}
	return nil
}

func nullStr(n sql.NullString) any {
	if n.Valid {
		return n.String
	}
	return nil
}

// LatestWithServer 快照联查：server + credential(画像外键除外) + profile + latest_metric。
type SnapshotRow struct {
	Srv  *Server
	Prof *Profile
	M    *Metric // 可能为 nil（从未采集）
	Cred *Credential
}

func (db *DB) SnapshotRows() ([]*SnapshotRow, error) {
	servers, err := db.ListServers()
	if err != nil {
		return nil, err
	}
	var out []*SnapshotRow
	for _, s := range servers {
		row := &SnapshotRow{Srv: s}
		if p, err := db.GetProfile(s.ID); err != nil {
			return nil, err
		} else {
			row.Prof = p
		}
		if c, err := db.GetCredential(s.ID); err != nil {
			return nil, err
		} else {
			row.Cred = c
		}
		m := &Metric{}
		err := db.SQL.QueryRow(`SELECT server_id, ts, status, COALESCE(cpu_pct,0),
			COALESCE(mem_used,0), COALESCE(mem_total,0), COALESCE(swap_used,0), COALESCE(swap_total,0),
			COALESCE(disk_used,0), COALESCE(disk_total,0),
			COALESCE(net_in_bps,0), COALESCE(net_out_bps,0),
			COALESCE(net_in_total,0), COALESCE(net_out_total,0),
			COALESCE(tcp_conns,0), COALESCE(udp_conns,0),
			COALESCE(load1,0), COALESCE(load5,0), COALESCE(load15,0),
			COALESCE(uptime_s,0), COALESCE(processes,0),
			power_w, cpu_w, dram_w, temp_c, freq_mhz, power_src
			FROM latest_metric WHERE server_id = ?`, s.ID).
			Scan(&m.ServerID, &m.Ts, &m.Status, &m.CpuPct, &m.MemUsed, &m.MemTotal,
				&m.SwapUsed, &m.SwapTotal, &m.DiskUsed, &m.DiskTotal,
				&m.NetInBps, &m.NetOutBps, &m.NetInTotal, &m.NetOutTotal,
				&m.TcpConns, &m.UdpConns, &m.Load1, &m.Load5, &m.Load15,
				&m.UptimeS, &m.Processes,
				&m.PowerW, &m.CpuW, &m.DramW, &m.TempC, &m.FreqMhz, &m.PowerSrc)
		if err == sql.ErrNoRows {
			row.M = nil
		} else if err != nil {
			return nil, err
		} else {
			row.M = m
		}
		out = append(out, row)
	}
	return out, nil
}

func (db *DB) InsertSample(m *Metric) error {
	_, err := db.SQL.Exec(`INSERT INTO metric_sample
		(server_id, ts, cpu_pct, mem_used, swap_used, disk_used, net_in_bps, net_out_bps,
		 tcp_conns, udp_conns, load1, load5, load15, uptime_s, processes,
		 power_w, cpu_w, dram_w, temp_c, freq_mhz, power_src)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ServerID, m.Ts, m.CpuPct, m.MemUsed, m.SwapUsed, m.DiskUsed,
		m.NetInBps, m.NetOutBps, m.TcpConns, m.UdpConns,
		m.Load1, m.Load5, m.Load15, m.UptimeS, m.Processes,
		nullFloat(m.PowerW), nullFloat(m.CpuW), nullFloat(m.DramW),
		nullFloat(m.TempC), nullInt64(m.FreqMhz), nullStr(m.PowerSrc))
	return err
}

// SamplesInRange 原始采样（访客短期曲线 / 登录全范围）。
func (db *DB) SamplesInRange(serverID int64, from, to int64, limit int) ([]*Metric, error) {
	rows, err := db.SQL.Query(`SELECT ts, COALESCE(cpu_pct,0), COALESCE(mem_used,0), COALESCE(swap_used,0),
		COALESCE(disk_used,0), COALESCE(net_in_bps,0), COALESCE(net_out_bps,0),
		COALESCE(tcp_conns,0), COALESCE(udp_conns,0),
		COALESCE(load1,0), COALESCE(load5,0), COALESCE(load15,0),
		COALESCE(uptime_s,0), COALESCE(processes,0),
		power_w, cpu_w, dram_w, temp_c, freq_mhz, power_src
		FROM metric_sample WHERE server_id = ? AND ts >= ? AND ts <= ? ORDER BY ts LIMIT ?`,
		serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Metric
	for rows.Next() {
		m := &Metric{ServerID: serverID}
		if err := rows.Scan(&m.Ts, &m.CpuPct, &m.MemUsed, &m.SwapUsed, &m.DiskUsed,
			&m.NetInBps, &m.NetOutBps, &m.TcpConns, &m.UdpConns,
			&m.Load1, &m.Load5, &m.Load15, &m.UptimeS, &m.Processes,
			&m.PowerW, &m.CpuW, &m.DramW, &m.TempC, &m.FreqMhz, &m.PowerSrc); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// HourlyInRange 小时聚合（7d 曲线）。
func (db *DB) HourlyInRange(serverID int64, fromHour, toHour int64) ([]map[string]any, error) {
	rows, err := db.SQL.Query(`SELECT hour_ts, COALESCE(cpu_avg,0), COALESCE(cpu_max,0),
		COALESCE(mem_avg,0), COALESCE(mem_max,0),
		COALESCE(net_in_avg,0), COALESCE(net_out_avg,0),
		COALESCE(net_in_total,0), COALESCE(net_out_total,0),
		power_avg, kwh
		FROM metric_hourly WHERE server_id = ? AND hour_ts >= ? AND hour_ts <= ? ORDER BY hour_ts`,
		serverID, fromHour, toHour)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var h, memAvg, memMax, inTot, outTot int64
		var cpuAvg, cpuMax, inAvg, outAvg float64
		var pAvg, kwh sql.NullFloat64
		if err := rows.Scan(&h, &cpuAvg, &cpuMax, &memAvg, &memMax, &inAvg, &outAvg,
			&inTot, &outTot, &pAvg, &kwh); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"ts": h, "cpu_avg": cpuAvg, "cpu_max": cpuMax,
			"mem_avg": memAvg, "mem_max": memMax,
			"net_in_avg": inAvg, "net_out_avg": outAvg,
			"net_in_total": inTot, "net_out_total": outTot,
			"power_avg": nullableFloat(pAvg), "kwh": nullableFloat(kwh),
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, rows.Err()
}

func nullableFloat(n sql.NullFloat64) any {
	if n.Valid {
		return n.Float64
	}
	return nil
}

// UpsertHourly 写入小时聚合。
func (db *DB) UpsertHourly(serverID, hourTs int64, cpuAvg, cpuMax float64, memAvg, memMax int64,
	inAvg, outAvg float64, inTot, outTot int64, pAvg sql.NullFloat64, kwh sql.NullFloat64) error {
	_, err := db.SQL.Exec(`INSERT INTO metric_hourly
		(server_id, hour_ts, cpu_avg, cpu_max, mem_avg, mem_max, net_in_avg, net_out_avg,
		 net_in_total, net_out_total, power_avg, kwh)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(server_id, hour_ts) DO UPDATE SET cpu_avg=excluded.cpu_avg, cpu_max=excluded.cpu_max,
			mem_avg=excluded.mem_avg, mem_max=excluded.mem_max,
			net_in_avg=excluded.net_in_avg, net_out_avg=excluded.net_out_avg,
			net_in_total=excluded.net_in_total, net_out_total=excluded.net_out_total,
			power_avg=excluded.power_avg, kwh=excluded.kwh`,
		serverID, hourTs, cpuAvg, cpuMax, memAvg, memMax, inAvg, outAvg,
		inTot, outTot, nullFloat(pAvg), nullFloat(kwh))
	return err
}

func (db *DB) DeleteSamplesBefore(ts int64) (int64, error) {
	res, err := db.SQL.Exec(`DELETE FROM metric_sample WHERE ts < ?`, ts)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (db *DB) DeleteHourlyBefore(hourTs int64) (int64, error) {
	res, err := db.SQL.Exec(`DELETE FROM metric_hourly WHERE hour_ts < ?`, hourTs)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
