package store

import (
	"database/sql"
	"encoding/json"
)

// ---------- WG 组网领域模型（doc/12 §4） ----------

// WGNetwork 单张组网（v1 恒 id=1）。
type WGNetwork struct {
	Subnet            string `json:"subnet"`             // 10.66.66.0/24
	HubIP             string `json:"hub_ip"`             // hub 虚拟 IP（浮动身份，裸 IP）
	Iface             string `json:"iface"`              // wg0
	Keepalive         int    `json:"keepalive"`          // 秒
	MTU               int    `json:"mtu"`
	ActiveHubServerID int64  `json:"active_hub_server_id"` // 0 = 尚无
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

// WGHub hub/备胎槽位（server_id 为主键，公网机）。
type WGHub struct {
	ServerID     int64           `json:"server_id"`
	ListenPort   int             `json:"listen_port"`
	PublicKey    string          `json:"public_key"`
	PrivateKeyEnc []byte         `json:"-"`
	Endpoint     string          `json:"endpoint"` // 对外宣告 host:port（可 DDNS 域名）
	Status       string          `json:"status"`   // pending/ok/error
	LastError    string          `json:"last_error"`
	QuotaGB      sql.NullFloat64 `json:"quota_gb"` // 月流量提醒阈值（GB），NULL=不限
	CheckedAt    sql.NullInt64   `json:"checked_at"`
}

// WGPeer 网内成员：服务器（SSH 管理）或设备（面板发凭证）。
type WGPeer struct {
	ID            int64
	Kind          string // server | device
	ServerID      sql.NullInt64
	Name          string
	WgIP          string
	PublicKey     string
	PrivateKeyEnc []byte // 面板代管（可再出配置）时非空；纯导入为空
	PskEnc        []byte
	Managed       bool
	Status        string // pending/joining/online/offline/error/left
	LastError     string
	LastHandshake sql.NullInt64
	RxBytes       int64
	TxBytes       int64
	CheckedAt     sql.NullInt64
	CreatedAt     int64
}

// WGTask 组网任务（apply/switch_hub/remove）。
type WGTask struct {
	ID         int64
	Kind       string
	Status     string // running/done/failed/partial
	Payload    string
	Result     string
	CreatedAt  int64
	FinishedAt sql.NullInt64
}

// WGTaskStep 任务步骤。
type WGTaskStep struct {
	ID         int64
	TaskID     int64
	Seq        int64
	ServerID   sql.NullInt64
	Title      string
	Status     string // pending/running/ok/failed/skipped
	Log        string
	StartedAt  sql.NullInt64
	FinishedAt sql.NullInt64
}

// ---------- network ----------

func (db *DB) GetWGNetwork() (*WGNetwork, error) {
	n := &WGNetwork{}
	var active sql.NullInt64
	err := db.SQL.QueryRow(`SELECT subnet, hub_ip, iface, keepalive, mtu, active_hub_server_id,
		created_at, COALESCE(updated_at,0) FROM wg_network WHERE id = 1`).
		Scan(&n.Subnet, &n.HubIP, &n.Iface, &n.Keepalive, &n.MTU, &active, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n.ActiveHubServerID = active.Int64
	return n, nil
}

func (db *DB) EnsureWGNetwork(n *WGNetwork) error {
	_, err := db.SQL.Exec(`INSERT INTO wg_network (id, subnet, hub_ip, iface, keepalive, mtu,
		active_hub_server_id, created_at, updated_at) VALUES (1,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET subnet=excluded.subnet, hub_ip=excluded.hub_ip,
			iface=excluded.iface, keepalive=excluded.keepalive, mtu=excluded.mtu,
			active_hub_server_id=excluded.active_hub_server_id, updated_at=excluded.updated_at`,
		n.Subnet, n.HubIP, n.Iface, n.Keepalive, n.MTU, nullInt64(sql.NullInt64{Int64: n.ActiveHubServerID, Valid: n.ActiveHubServerID > 0}),
		n.CreatedAt, n.UpdatedAt)
	return err
}

func (db *DB) SetActiveHub(serverID int64, now int64) error {
	_, err := db.SQL.Exec(`UPDATE wg_network SET active_hub_server_id = ?, updated_at = ? WHERE id = 1`,
		nullInt64(sql.NullInt64{Int64: serverID, Valid: serverID > 0}), now)
	return err
}

// ---------- hub ----------

func (db *DB) ListWGHub() ([]*WGHub, error) {
	rows, err := db.SQL.Query(`SELECT server_id, listen_port, COALESCE(public_key,''),
		COALESCE(private_key_enc,x''), COALESCE(endpoint,''), status, COALESCE(last_error,''),
		quota_gb, checked_at FROM wg_hub ORDER BY server_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WGHub
	for rows.Next() {
		h := &WGHub{}
		if err := rows.Scan(&h.ServerID, &h.ListenPort, &h.PublicKey, &h.PrivateKeyEnc,
			&h.Endpoint, &h.Status, &h.LastError, &h.QuotaGB, &h.CheckedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (db *DB) GetWGHub(serverID int64) (*WGHub, error) {
	h := &WGHub{}
	err := db.SQL.QueryRow(`SELECT server_id, listen_port, COALESCE(public_key,''),
		COALESCE(private_key_enc,x''), COALESCE(endpoint,''), status, COALESCE(last_error,''),
		quota_gb, checked_at FROM wg_hub WHERE server_id = ?`, serverID).
		Scan(&h.ServerID, &h.ListenPort, &h.PublicKey, &h.PrivateKeyEnc, &h.Endpoint,
			&h.Status, &h.LastError, &h.QuotaGB, &h.CheckedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return h, nil
}

// UpsertWGHub 写入/更新 hub 槽位（private_key_enc 为空则保留原值）。
func (db *DB) UpsertWGHub(h *WGHub) error {
	_, err := db.SQL.Exec(`INSERT INTO wg_hub
		(server_id, listen_port, public_key, private_key_enc, endpoint, status, last_error, quota_gb, checked_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(server_id) DO UPDATE SET listen_port=excluded.listen_port,
			public_key=excluded.public_key,
			private_key_enc=CASE WHEN excluded.private_key_enc IS NULL THEN wg_hub.private_key_enc
				ELSE excluded.private_key_enc END,
			endpoint=excluded.endpoint, status=excluded.status, last_error=excluded.last_error,
			quota_gb=excluded.quota_gb, checked_at=excluded.checked_at`,
		h.ServerID, h.ListenPort, h.PublicKey, h.PrivateKeyEnc, h.Endpoint,
		h.Status, h.LastError, nullFloat(h.QuotaGB), h.CheckedAt)
	return err
}

func (db *DB) DeleteWGHub(serverID int64) error {
	_, err := db.SQL.Exec(`DELETE FROM wg_hub WHERE server_id = ?`, serverID)
	return err
}

// ---------- peer ----------

const wgPeerCols = `id, kind, server_id, name, wg_ip, public_key, private_key_enc, psk_enc,
	managed, status, COALESCE(last_error,''), last_handshake,
	COALESCE(rx_bytes,0), COALESCE(tx_bytes,0), checked_at, created_at`

func scanWGPeer(p *WGPeer, row interface {
	Scan(dest ...any) error
}) error {
	var srvID sql.NullInt64
	var managed int
	if err := row.Scan(&p.ID, &p.Kind, &srvID, &p.Name, &p.WgIP, &p.PublicKey,
		&p.PrivateKeyEnc, &p.PskEnc, &managed, &p.Status, &p.LastError,
		&p.LastHandshake, &p.RxBytes, &p.TxBytes, &p.CheckedAt, &p.CreatedAt); err != nil {
		return err
	}
	p.ServerID = srvID
	p.Managed = managed == 1
	return nil
}

func (db *DB) ListWGPeers() ([]*WGPeer, error) {
	rows, err := db.SQL.Query(`SELECT ` + wgPeerCols + ` FROM wg_peer ORDER BY wg_ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WGPeer
	for rows.Next() {
		p := &WGPeer{}
		if err := scanWGPeer(p, rows); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// scanWGPeerManaged scanWGPeer 不解析 managed 布尔（占位），统一在此修正。
// （SQLite bool 以整数存取，Scan 进中间变量后置位）

func (db *DB) GetWGPeer(id int64) (*WGPeer, error) {
	p := &WGPeer{}
	if err := scanWGPeer(p, db.SQL.QueryRow(`SELECT `+wgPeerCols+` FROM wg_peer WHERE id = ?`, id)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (db *DB) GetWGPeerByServer(serverID int64) (*WGPeer, error) {
	p := &WGPeer{}
	if err := scanWGPeer(p, db.SQL.QueryRow(`SELECT `+wgPeerCols+` FROM wg_peer WHERE kind='server' AND server_id = ?`, serverID)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (db *DB) InsertWGPeer(p *WGPeer) (int64, error) {
	managed := 0
	if p.Managed {
		managed = 1
	}
	var srv any
	if p.ServerID.Valid {
		srv = p.ServerID.Int64
	}
	res, err := db.SQL.Exec(`INSERT INTO wg_peer
		(kind, server_id, name, wg_ip, public_key, private_key_enc, psk_enc, managed, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.Kind, srv, p.Name, p.WgIP, p.PublicKey, p.PrivateKeyEnc, p.PskEnc, managed, p.Status, p.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) UpdateWGPeer(p *WGPeer) error {
	managed := 0
	if p.Managed {
		managed = 1
	}
	var srv any
	if p.ServerID.Valid {
		srv = p.ServerID.Int64
	}
	_, err := db.SQL.Exec(`UPDATE wg_peer SET kind=?, server_id=?, name=?, wg_ip=?, public_key=?,
		private_key_enc=?, psk_enc=?, managed=?, status=?, last_error=?,
		last_handshake=?, rx_bytes=?, tx_bytes=?, checked_at=? WHERE id=?`,
		p.Kind, srv, p.Name, p.WgIP, p.PublicKey, p.PrivateKeyEnc, p.PskEnc, managed,
		p.Status, p.LastError, nullInt(p.LastHandshake), p.RxBytes, p.TxBytes, p.CheckedAt, p.ID)
	return err
}

func (db *DB) DeleteWGPeer(id int64) error {
	_, err := db.SQL.Exec(`DELETE FROM wg_peer WHERE id = ?`, id)
	return err
}

// ---------- task ----------

func (db *DB) InsertWGTask(t *WGTask) (int64, error) {
	res, err := db.SQL.Exec(`INSERT INTO wg_task (kind, status, payload, created_at)
		VALUES (?,?,?,?)`, t.Kind, t.Status, t.Payload, t.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) FinishWGTask(id int64, status, result string, now int64) error {
	_, err := db.SQL.Exec(`UPDATE wg_task SET status=?, result=?, finished_at=? WHERE id=?`,
		status, result, now, id)
	return err
}

func (db *DB) GetWGTask(id int64) (*WGTask, error) {
	t := &WGTask{}
	err := db.SQL.QueryRow(`SELECT id, kind, status, payload, COALESCE(result,''),
		created_at, finished_at FROM wg_task WHERE id = ?`, id).
		Scan(&t.ID, &t.Kind, &t.Status, &t.Payload, &t.Result, &t.CreatedAt, &t.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (db *DB) ListWGTasks(limit int) ([]*WGTask, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}
	rows, err := db.SQL.Query(`SELECT id, kind, status, payload, COALESCE(result,''),
		created_at, finished_at FROM wg_task ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WGTask
	for rows.Next() {
		t := &WGTask{}
		if err := rows.Scan(&t.ID, &t.Kind, &t.Status, &t.Payload, &t.Result, &t.CreatedAt, &t.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// HasRunningWGTask 是否存在未完结任务（同一时刻只允许一个组网任务）。
func (db *DB) HasRunningWGTask() (bool, error) {
	var n int
	err := db.SQL.QueryRow(`SELECT COUNT(*) FROM wg_task WHERE status = 'running'`).Scan(&n)
	return n > 0, err
}

func (db *DB) InsertWGTaskStep(s *WGTaskStep) (int64, error) {
	var srv any
	if s.ServerID.Valid {
		srv = s.ServerID.Int64
	}
	res, err := db.SQL.Exec(`INSERT INTO wg_task_step (task_id, seq, server_id, title, status, started_at)
		VALUES (?,?,?,?,?,?)`, s.TaskID, s.Seq, srv, s.Title, s.Status, s.StartedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// StartWGTaskStep 步骤开跑。
func (db *DB) StartWGTaskStep(id int64, now int64) error {
	_, err := db.SQL.Exec(`UPDATE wg_task_step SET status='running', started_at=? WHERE id=?`, now, id)
	return err
}

// FinishWGTaskStep 步骤收尾（log 非空时覆盖追加）。
func (db *DB) FinishWGTaskStep(id int64, status, log string, now int64) error {
	if log != "" {
		_, err := db.SQL.Exec(`UPDATE wg_task_step SET status=?, log=?, finished_at=? WHERE id=?`,
			status, log, now, id)
		return err
	}
	_, err := db.SQL.Exec(`UPDATE wg_task_step SET status=?, finished_at=? WHERE id=?`, status, now, id)
	return err
}

// AppendWGTaskStepLog 追加一行步骤日志（保留既有内容）。
func (db *DB) AppendWGTaskStepLog(id int64, line string) error {
	_, err := db.SQL.Exec(`UPDATE wg_task_step SET log = COALESCE(log,'') || ? WHERE id=?`, line+"\n", id)
	return err
}

func (db *DB) ListWGTaskSteps(taskID int64) ([]*WGTaskStep, error) {
	rows, err := db.SQL.Query(`SELECT id, task_id, seq, server_id, title, status, COALESCE(log,''),
		started_at, finished_at FROM wg_task_step WHERE task_id = ? ORDER BY seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WGTaskStep
	for rows.Next() {
		s := &WGTaskStep{}
		if err := rows.Scan(&s.ID, &s.TaskID, &s.Seq, &s.ServerID, &s.Title, &s.Status,
			&s.Log, &s.StartedAt, &s.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---------- hub 流量 ----------

// AddWGHubTraffic 累加 hub 当日 WG 转发流量（巡检差值写入，幂等累加）。
func (db *DB) AddWGHubTraffic(hubServerID, dayTs int64, rx, tx int64, now int64) error {
	_, err := db.SQL.Exec(`INSERT INTO wg_hub_traffic (hub_server_id, day_ts, rx, tx, updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(hub_server_id, day_ts) DO UPDATE SET
			rx = COALESCE(wg_hub_traffic.rx,0) + excluded.rx,
			tx = COALESCE(wg_hub_traffic.tx,0) + excluded.tx,
			updated_at = excluded.updated_at`,
		hubServerID, dayTs, rx, tx, now)
	return err
}

// WGHubTrafficRange 月度流量汇总（[fromDay,toDay] 闭区间）。
func (db *DB) WGHubTrafficRange(hubServerID, fromDay, toDay int64) (rx, tx int64, err error) {
	err = db.SQL.QueryRow(`SELECT COALESCE(SUM(rx),0), COALESCE(SUM(tx),0)
		FROM wg_hub_traffic WHERE hub_server_id = ? AND day_ts >= ? AND day_ts <= ?`,
		hubServerID, fromDay, toDay).Scan(&rx, &tx)
	return
}

// ---------- JSON 辅助 ----------

// WGTaskPayload 任务载荷（JSON 存 wg_task.payload）。
type WGTaskPayload struct {
	Allocations []WGAlloc `json:"allocations"`
}

type WGAlloc struct {
	ServerID int64  `json:"server_id"`
	WgIP     string `json:"wg_ip"`
	Role     string `json:"role"` // hub/standby/spoke
}

// DecodeWGPayload 解析任务载荷（容错：非法 JSON 返回空载荷）。
func DecodeWGPayload(raw string) *WGTaskPayload {
	var p WGTaskPayload
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &p)
	}
	return &p
}

// SetCredentialFP 回写 host key 指纹（TOFU 记录/非严格更新；不动 last_error）。
func (db *DB) SetCredentialFP(serverID int64, fp string) error {
	_, err := db.SQL.Exec(`UPDATE server_credential SET host_key_fp = ? WHERE server_id = ?`, fp, serverID)
	return err
}
