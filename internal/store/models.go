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
	IsSelf       bool     `json:"is_self"`
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
	NetIfaces      string // 采集侧实际统计的网卡名（逗号分隔），供前端标注流量口径
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

// SessionExpiresAt 查会话到期时间（不存在/已过期返回 ok=false）。
// RequireAuth 用它决定是否需要滑动续期，避免每请求都 UPDATE 一次会话行。
func (db *DB) SessionExpiresAt(tokenHash string, now int64) (expiresAt int64, ok bool, err error) {
	err = db.SQL.QueryRow(`SELECT expires_at FROM session WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, now).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return expiresAt, true, nil
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

const serverCols = `id, name, region, region_source, tags, note_public, note_private,
		sort_order, hidden, enabled, is_self, created_at`

func scanServerFull(s *Server, row interface {
	Scan(dest ...any) error
}) error {
	var tags string
	var hidden, enabled, isSelf int
	if err := row.Scan(&s.ID, &s.Name, &s.Region, &s.RegionSource, &tags,
		&s.NotePublic, &s.NotePrivate, &s.SortOrder, &hidden, &enabled, &isSelf, &s.CreatedAt); err != nil {
		return err
	}
	s.Tags = decodeTags(tags)
	s.Hidden = hidden == 1
	s.Enabled = enabled == 1
	s.IsSelf = isSelf == 1
	return nil
}

func (db *DB) ListServers() ([]*Server, error) {
	rows, err := db.SQL.Query(`SELECT ` + serverCols + ` FROM server ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Server
	for rows.Next() {
		s := &Server{}
		if err := scanServerFull(s, rows); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) GetServer(id int64) (*Server, error) {
	s := &Server{}
	row := db.SQL.QueryRow(`SELECT `+serverCols+` FROM server WHERE id = ?`, id)
	if err := scanServerFull(s, row); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// GetSelfServer 返回本机节点（is_self=1），无则返回 nil。
func (db *DB) GetSelfServer() (*Server, error) {
	s := &Server{}
	row := db.SQL.QueryRow(`SELECT ` + serverCols + ` FROM server WHERE is_self = 1`)
	if err := scanServerFull(s, row); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// EnsureSelfServer 启动时保证存在本机节点：缺失则创建（sort_order=-1 恒排首位）。
// 同时保证占位凭据行存在：本机采集不走凭据，但错误/最近采集时间复用该行落盘。
func (db *DB) EnsureSelfServer(now int64) (int64, error) {
	if s, err := db.GetSelfServer(); err != nil || s != nil {
		if s != nil {
			db.ensureSelfCredential(s.ID)
			return s.ID, nil
		}
		return 0, err
	}
	res, err := db.SQL.Exec(`INSERT INTO server
		(name, region, region_source, tags, note_public, note_private, sort_order, hidden, enabled, is_self, created_at)
		VALUES (?, ?, 'auto', '[]', '', '', -1, 0, 1, 1, ?)`,
		"本机", "本机", now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err == nil {
		db.ensureSelfCredential(id)
	}
	return id, err
}

func (db *DB) ensureSelfCredential(id int64) {
	_, _ = db.SQL.Exec(`INSERT OR IGNORE INTO server_credential
		(server_id, host, port, username, auth_type) VALUES (?, 'local', 0, 'local', 'password')`, id)
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
		(name, region, region_source, tags, note_public, note_private, sort_order, hidden, enabled, is_self, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Name, s.Region, s.RegionSource, encodeTags(s.Tags), s.NotePublic, s.NotePrivate,
		s.SortOrder, boolInt(s.Hidden), boolInt(s.Enabled), boolInt(s.IsSelf), s.CreatedAt)
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

// SetCollectResult 采集结果回写（试连等单发路径立即执行）。
func (db *DB) SetCollectResult(serverID int64, fp, lastErr string, successAt any) error {
	return setCollectResultExec(db.SQL, serverID, fp, lastErr, successAt)
}

// QueueCollectResult 采集结果回写入本轮缓冲（CommitRound 统一提交）。
func (db *DB) QueueCollectResult(serverID int64, fp, lastErr string, successAt any) error {
	db.queue(func(tx *sql.Tx) error {
		return setCollectResultExec(tx, serverID, fp, lastErr, successAt)
	})
	return nil
}

func setCollectResultExec(e execer, serverID int64, fp, lastErr string, successAt any) error {
	_, err := e.Exec(`UPDATE server_credential SET host_key_fp = COALESCE(NULLIF(?,''), host_key_fp),
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
		COALESCE(base_load_source,'default'), COALESCE(month_kwh,0), COALESCE(net_ifaces,'')
		FROM server_profile WHERE server_id = ?`, serverID).
		Scan(&p.ServerID, &p.Hostname, &p.OsName, &p.OsVersion, &p.Kernel, &p.Arch, &p.CpuModel,
			&p.CpuCores, &p.MemTotal, &p.SwapTotal, &p.DiskTotal, &p.DisksJSON, &p.Virt,
			&p.PublicIP, &p.GeoCountry, &p.GeoCity,
			&p.PowerRapL, &p.PowerBattery, &p.BaseLoadW, &p.BaseLoadSource, &p.MonthKwh, &p.NetIfaces)
	// sqlite bool 列以整数存取
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpsertProfile 全字段写入（试连画像等单发路径立即执行）。
func (db *DB) UpsertProfile(p *Profile, now int64) error {
	return upsertProfileExec(db.SQL, p, now)
}

// QueueProfile 采集画像写入入本轮缓冲（CommitRound 统一提交）。
func (db *DB) QueueProfile(p *Profile, now int64) error {
	db.queue(func(tx *sql.Tx) error { return upsertProfileExec(tx, p, now) })
	return nil
}

const profileUpsertSQL = `INSERT INTO server_profile
	(server_id, hostname, os_name, os_version, kernel, arch, cpu_model, cpu_cores,
	 mem_total, swap_total, disk_total, disks_json, virt, public_ip, geo_country, geo_city,
	 power_rapl, power_battery, base_load_w, base_load_source, month_kwh, net_ifaces, collected_at)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(server_id) DO UPDATE SET hostname=excluded.hostname, os_name=excluded.os_name,
		os_version=excluded.os_version, kernel=excluded.kernel, arch=excluded.arch,
		cpu_model=excluded.cpu_model, cpu_cores=excluded.cpu_cores, mem_total=excluded.mem_total,
		swap_total=excluded.swap_total, disk_total=excluded.disk_total, disks_json=excluded.disks_json,
		virt=excluded.virt, public_ip=excluded.public_ip, geo_country=excluded.geo_country,
		geo_city=excluded.geo_city, power_rapl=excluded.power_rapl, power_battery=excluded.power_battery,
		base_load_w=excluded.base_load_w, base_load_source=excluded.base_load_source,
		net_ifaces=excluded.net_ifaces,
		month_kwh=CASE WHEN excluded.collected_at > 0 THEN server_profile.month_kwh
			ELSE excluded.month_kwh END,
		collected_at=excluded.collected_at`

// upsertProfileExec 画像全字段 upsert。collected_at>0 表示采集链路写入：
// month_kwh 保留库内现值（同轮 QueueMonthKwh 增量与画像 upsert 混排，
// 若用内存画像的旧值覆盖会把本帧累计清掉）；试连画像（collected_at=0）显式覆盖。
func upsertProfileExec(e execer, p *Profile, now int64) error {
	_, err := e.Exec(profileUpsertSQL,
		p.ServerID, p.Hostname, p.OsName, p.OsVersion, p.Kernel, p.Arch, p.CpuModel, p.CpuCores,
		p.MemTotal, p.SwapTotal, p.DiskTotal, p.DisksJSON, p.Virt, p.PublicIP, p.GeoCountry, p.GeoCity,
		boolInt(p.PowerRapL), boolInt(p.PowerBattery), p.BaseLoadW, p.BaseLoadSource, p.MonthKwh, p.NetIfaces, now)
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
	return addMonthKwhExec(db.SQL, serverID, delta)
}

// QueueMonthKwh 月累计 kWh 增量入本轮缓冲（CommitRound 统一提交）。
func (db *DB) QueueMonthKwh(serverID int64, delta float64) error {
	db.queue(func(tx *sql.Tx) error { return addMonthKwhExec(tx, serverID, delta) })
	return nil
}

func addMonthKwhExec(e execer, serverID int64, delta float64) error {
	_, err := e.Exec(`UPDATE server_profile SET month_kwh = COALESCE(month_kwh,0) + ?
		WHERE server_id = ?`, delta, serverID)
	return err
}

// ResetAllMonthKwh 月初清零全节点月累计（后台任务按月翻转触发）。
func (db *DB) ResetAllMonthKwh() error {
	_, err := db.SQL.Exec(`UPDATE server_profile SET month_kwh = 0 WHERE COALESCE(month_kwh,0) != 0`)
	return err
}

// ---------- metrics ----------

// UpsertLatest 最新指标 upsert（试连等单发路径立即执行）。
func (db *DB) UpsertLatest(m *Metric) error {
	return upsertLatestExec(db.SQL, m)
}

// QueueLatest 最新指标 upsert 入本轮缓冲（CommitRound 统一提交）。
func (db *DB) QueueLatest(m *Metric) error {
	db.queue(func(tx *sql.Tx) error { return upsertLatestExec(tx, m) })
	return nil
}

const latestUpsertSQL = `INSERT INTO latest_metric
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
		temp_c=excluded.temp_c, freq_mhz=excluded.freq_mhz, power_src=excluded.power_src`

func upsertLatestExec(e execer, m *Metric) error {
	_, err := e.Exec(latestUpsertSQL,
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

// LatestWithServer 快照联查：server + profile + latest_metric 单查询。
// 不查 credential：快照链路用不到凭据，原实现每节点多一次查询还拉三个密文 BLOB。
type SnapshotRow struct {
	Srv  *Server
	Prof *Profile
	M    *Metric // 可能为 nil（从未采集）
}

const snapshotJoinSQL = `SELECT s.id, s.name, s.region, s.region_source, s.tags,
		s.note_public, s.note_private, s.sort_order, s.hidden, s.enabled, s.is_self, s.created_at,
		COALESCE(p.hostname,''), COALESCE(p.os_name,''), COALESCE(p.os_version,''),
		COALESCE(p.kernel,''), COALESCE(p.arch,''), COALESCE(p.cpu_model,''), COALESCE(p.cpu_cores,0),
		COALESCE(p.mem_total,0), COALESCE(p.swap_total,0), COALESCE(p.disk_total,0), COALESCE(p.disks_json,''),
		COALESCE(p.virt,''), COALESCE(p.public_ip,''), COALESCE(p.geo_country,''), COALESCE(p.geo_city,''),
		COALESCE(p.power_rapl,0), COALESCE(p.power_battery,0), COALESCE(p.base_load_w,0),
		COALESCE(p.base_load_source,'default'), COALESCE(p.month_kwh,0), COALESCE(p.net_ifaces,''),
		l.server_id, l.ts, l.status, COALESCE(l.cpu_pct,0),
		COALESCE(l.mem_used,0), COALESCE(l.mem_total,0), COALESCE(l.swap_used,0), COALESCE(l.swap_total,0),
		COALESCE(l.disk_used,0), COALESCE(l.disk_total,0),
		COALESCE(l.net_in_bps,0), COALESCE(l.net_out_bps,0),
		COALESCE(l.net_in_total,0), COALESCE(l.net_out_total,0),
		COALESCE(l.tcp_conns,0), COALESCE(l.udp_conns,0),
		COALESCE(l.load1,0), COALESCE(l.load5,0), COALESCE(l.load15,0),
		COALESCE(l.uptime_s,0), COALESCE(l.processes,0),
		l.power_w, l.cpu_w, l.dram_w, l.temp_c, l.freq_mhz, l.power_src
	FROM server s
	LEFT JOIN server_profile p ON p.server_id = s.id
	LEFT JOIN latest_metric l ON l.server_id = s.id
	ORDER BY s.sort_order, s.id`

func (db *DB) SnapshotRows() ([]*SnapshotRow, error) {
	rows, err := db.SQL.Query(snapshotJoinSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SnapshotRow
	for rows.Next() {
		var tags string
		var hidden, enabled, isSelf int
		s := &Server{}
		p := &Profile{}
		m := &Metric{}
		var mServerID sql.NullInt64
		if err := rows.Scan(&s.ID, &s.Name, &s.Region, &s.RegionSource, &tags,
			&s.NotePublic, &s.NotePrivate, &s.SortOrder, &hidden, &enabled, &isSelf, &s.CreatedAt,
			&p.Hostname, &p.OsName, &p.OsVersion,
			&p.Kernel, &p.Arch, &p.CpuModel, &p.CpuCores,
			&p.MemTotal, &p.SwapTotal, &p.DiskTotal, &p.DisksJSON,
			&p.Virt, &p.PublicIP, &p.GeoCountry, &p.GeoCity,
			&p.PowerRapL, &p.PowerBattery, &p.BaseLoadW,
			&p.BaseLoadSource, &p.MonthKwh, &p.NetIfaces,
			&mServerID, &m.Ts, &m.Status, &m.CpuPct,
			&m.MemUsed, &m.MemTotal, &m.SwapUsed, &m.SwapTotal,
			&m.DiskUsed, &m.DiskTotal,
			&m.NetInBps, &m.NetOutBps,
			&m.NetInTotal, &m.NetOutTotal,
			&m.TcpConns, &m.UdpConns,
			&m.Load1, &m.Load5, &m.Load15,
			&m.UptimeS, &m.Processes,
			&m.PowerW, &m.CpuW, &m.DramW, &m.TempC, &m.FreqMhz, &m.PowerSrc); err != nil {
			return nil, err
		}
		s.Tags = decodeTags(tags)
		s.Hidden, s.Enabled, s.IsSelf = hidden == 1, enabled == 1, isSelf == 1
		p.ServerID = s.ID
		row := &SnapshotRow{Srv: s, Prof: p}
		if mServerID.Valid {
			m.ServerID = mServerID.Int64
			row.M = m
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SampleWindows 时间窗分桶聚合：把 [from,to] 等分为 n 桶，每桶返回桶内均值与桶起始 ts。
// 曲线点数可控且全程覆盖（替代 LIMIT 截断——截断会让 6h/24h 曲线只画到半程）。
func (db *DB) SampleWindows(serverID int64, from, to int64, n int) ([]map[string]any, error) {
	if n <= 0 {
		n = 60
	}
	span := (to - from) / int64(n)
	if span < 1 {
		span = 1
	}
	rows, err := db.SQL.Query(`SELECT (? + (ts - ?)/?) AS bucket_ts, AVG(COALESCE(cpu_pct,0)),
		AVG(COALESCE(mem_used,0)), AVG(COALESCE(disk_used,0)),
		AVG(COALESCE(net_in_bps,0)), AVG(COALESCE(net_out_bps,0)),
		AVG(CASE WHEN power_w IS NOT NULL THEN power_w END)
		FROM metric_sample
		WHERE server_id = ? AND ts >= ? AND ts <= ?
		GROUP BY bucket_ts ORDER BY bucket_ts`,
		from, from, span, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var ts int64
		var cpu, mem, disk, netIn, netOut float64
		var power sql.NullFloat64
		if err := rows.Scan(&ts, &cpu, &mem, &disk, &netIn, &netOut, &power); err != nil {
			return nil, err
		}
		row := map[string]any{
			"ts": ts, "cpu": roundF1(cpu), "mem": mem, "disk": disk,
			"net_in": netIn, "net_out": netOut,
		}
		if power.Valid {
			row["power"] = power.Float64
		} else {
			row["power"] = nil
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func roundF1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// ---------- daily traffic（按日流量记录） ----------

// UpsertDailyTraffic 幂等写某日流量（聚合任务重复跑安全）。
func (db *DB) UpsertDailyTraffic(serverID, dayTs int64, inTotal, outTotal int64, now int64) error {
	_, err := db.SQL.Exec(`INSERT INTO metric_daily (server_id, day_ts, in_total, out_total, updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(server_id, day_ts) DO UPDATE SET in_total=excluded.in_total,
			out_total=excluded.out_total, updated_at=excluded.updated_at`,
		serverID, dayTs, inTotal, outTotal, now)
	return err
}

// DailyTrafficRange 按日流量（dayTs 升序，含首尾）。
func (db *DB) DailyTrafficRange(serverID int64, fromDay, toDay int64) ([]map[string]any, error) {
	rows, err := db.SQL.Query(`SELECT day_ts, COALESCE(in_total,0), COALESCE(out_total,0)
		FROM metric_daily WHERE server_id = ? AND day_ts >= ? AND day_ts <= ? ORDER BY day_ts`,
		serverID, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var ts, inT, outT int64
		if err := rows.Scan(&ts, &inT, &outT); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"ts": ts, "in_total": inT, "out_total": outT})
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, rows.Err()
}

// DailyTrafficSumAll 全节点某日收发字节合计，按 server_id 分组（快照一次取全）。
func (db *DB) DailyTrafficSumAll(dayTs int64) (map[int64][2]int64, error) {
	rows, err := db.SQL.Query(`SELECT server_id, COALESCE(in_total,0), COALESCE(out_total,0)
		FROM metric_daily WHERE day_ts = ?`, dayTs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][2]int64{}
	for rows.Next() {
		var id, inT, outT int64
		if err := rows.Scan(&id, &inT, &outT); err != nil {
			return nil, err
		}
		out[id] = [2]int64{inT, outT}
	}
	return out, rows.Err()
}

// DailyTrafficSum 全节点某日收发字节合计（公开 summary 用）。
func (db *DB) DailyTrafficSum(dayTs int64) (inTotal, outTotal int64, err error) {
	err = db.SQL.QueryRow(`SELECT COALESCE(SUM(in_total),0), COALESCE(SUM(out_total),0)
		FROM metric_daily WHERE day_ts = ?`, dayTs).Scan(&inTotal, &outTotal)
	return
}

// DeleteDailyBefore 清理过期日流量行（retention 与小时聚合同参）。
func (db *DB) DeleteDailyBefore(dayTs int64) (int64, error) {
	res, err := db.SQL.Exec(`DELETE FROM metric_daily WHERE day_ts < ?`, dayTs)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertSample 原始采样落库（试连等单发路径立即执行）。
func (db *DB) InsertSample(m *Metric) error {
	return insertSampleExec(db.SQL, m)
}

// QueueSample 原始采样落库入本轮缓冲（CommitRound 统一提交）。
func (db *DB) QueueSample(m *Metric) error {
	db.queue(func(tx *sql.Tx) error { return insertSampleExec(tx, m) })
	return nil
}

const sampleInsertSQL = `INSERT INTO metric_sample
	(server_id, ts, cpu_pct, mem_used, swap_used, disk_used, net_in_bps, net_out_bps,
	 net_in_total, net_out_total,
	 tcp_conns, udp_conns, load1, load5, load15, uptime_s, processes,
	 power_w, cpu_w, dram_w, temp_c, freq_mhz, power_src)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

func insertSampleExec(e execer, m *Metric) error {
	_, err := e.Exec(sampleInsertSQL,
		m.ServerID, m.Ts, m.CpuPct, m.MemUsed, m.SwapUsed, m.DiskUsed,
		m.NetInBps, m.NetOutBps, m.NetInTotal, m.NetOutTotal,
		m.TcpConns, m.UdpConns,
		m.Load1, m.Load5, m.Load15, m.UptimeS, m.Processes,
		nullFloat(m.PowerW), nullFloat(m.CpuW), nullFloat(m.DramW),
		nullFloat(m.TempC), nullInt64(m.FreqMhz), nullStr(m.PowerSrc))
	return err
}

// SamplesInRange 原始采样（访客短期曲线 / 登录全范围 / 小时与日聚合输入）。
// limit<=0 表示不限制（聚合任务需要完整小时样本做首尾差）。
func (db *DB) SamplesInRange(serverID int64, from, to int64, limit int) ([]*Metric, error) {
	q := `SELECT ts, COALESCE(cpu_pct,0), COALESCE(mem_used,0), COALESCE(swap_used,0),
		COALESCE(disk_used,0), COALESCE(net_in_bps,0), COALESCE(net_out_bps,0),
		COALESCE(net_in_total,0), COALESCE(net_out_total,0),
		COALESCE(tcp_conns,0), COALESCE(udp_conns,0),
		COALESCE(load1,0), COALESCE(load5,0), COALESCE(load15,0),
		COALESCE(uptime_s,0), COALESCE(processes,0),
		power_w, cpu_w, dram_w, temp_c, freq_mhz, power_src
		FROM metric_sample WHERE server_id = ? AND ts >= ? AND ts <= ? ORDER BY ts`
	args := []any{serverID, from, to}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.SQL.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Metric
	for rows.Next() {
		m := &Metric{ServerID: serverID}
		if err := rows.Scan(&m.Ts, &m.CpuPct, &m.MemUsed, &m.SwapUsed, &m.DiskUsed,
			&m.NetInBps, &m.NetOutBps, &m.NetInTotal, &m.NetOutTotal,
			&m.TcpConns, &m.UdpConns,
			&m.Load1, &m.Load5, &m.Load15, &m.UptimeS, &m.Processes,
			&m.PowerW, &m.CpuW, &m.DramW, &m.TempC, &m.FreqMhz, &m.PowerSrc); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// TrafficWindowEdge 窗口端点计数：[from,to] 内首个（可选：跳过未初始化的
// NULL/0 计数样本）与最后一个有累计流量的样本。供日流量差值计算用，
// 替代把整窗样本拉进内存——10s 采样下一天 8640 行，端点两条索引查询足够。
type TrafficWindowEdge struct {
	NetInTotal  int64
	NetOutTotal int64
}

// FirstTrafficSample 窗口内第一条有效计数样本（net_in_total>0 且 net_out_total>0，
// 按时间升序）。无有效样本返回 false。
func (db *DB) FirstTrafficSample(serverID int64, from, to int64) (TrafficWindowEdge, bool) {
	var e TrafficWindowEdge
	err := db.SQL.QueryRow(`SELECT net_in_total, net_out_total FROM metric_sample
		WHERE server_id = ? AND ts >= ? AND ts <= ? AND net_in_total > 0 AND net_out_total > 0
		ORDER BY ts LIMIT 1`, serverID, from, to).Scan(&e.NetInTotal, &e.NetOutTotal)
	return e, err == nil
}

// LastTrafficSample 窗口内最后一条样本的累计计数（不筛有效性，
// 有效性由调用方按回绕/零头规则判定）。无样本返回 false。
func (db *DB) LastTrafficSample(serverID int64, from, to int64) (TrafficWindowEdge, bool) {
	var e TrafficWindowEdge
	err := db.SQL.QueryRow(`SELECT net_in_total, net_out_total FROM metric_sample
		WHERE server_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts DESC LIMIT 1`, serverID, from, to).Scan(&e.NetInTotal, &e.NetOutTotal)
	return e, err == nil
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

// DeleteAuditBefore 清理早于 ts 的审计日志（audit_log 无界增长会拖慢
// ListAudit 的 COUNT(*)，保留期由 tasks.cleanup 控制）。
func (db *DB) DeleteAuditBefore(ts int64) (int64, error) {
	res, err := db.SQL.Exec(`DELETE FROM audit_log WHERE ts < ?`, ts)
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
