package store

import (
	"database/sql"
	"time"
)

// ---------- 内网穿透平台领域模型（doc/13 §4） ----------

// FRPPlatform 一个已接入的穿透平台账号。两个平台的凭据模型不同：
// Sakura 只有长期有效的访问密钥（TokenEnc）；ChmlFrp 走 OAuth2，
// 需要短期 access_token + refresh_token 两个字段，且过期时间必须落库，
// 否则重启后无法判断是否需要先刷新。
type FRPPlatform struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"` // natfrp | chmlfrp
	Name string `json:"name"`

	TokenEnc      []byte `json:"-"`
	RefreshEnc    []byte `json:"-"` // 仅 ChmlFrp
	TokenExpireAt int64  `json:"token_expire_at"`

	// 账号画像（每次同步覆盖）
	UID        string `json:"uid"`
	Username   string `json:"username"`
	GroupName  string `json:"group_name"`
	SpeedLimit string `json:"speed_limit"`
	Realname   string `json:"realname"`

	// 用量快照（每次同步覆盖）
	TunnelUsed     int    `json:"tunnel_used"`
	TunnelQuota    int    `json:"tunnel_quota"`
	Conns          int    `json:"conns"`
	ConnsSrc       string `json:"conns_src"` // platform=平台 API | local=节点侧 socket 计数 | 空=无数据
	TrafficDayUsed int64  `json:"traffic_day_used"`
	TrafficRemain  int64  `json:"traffic_remain"`
	TrafficUp      int64  `json:"traffic_up"`
	TrafficDown    int64  `json:"traffic_down"`
	ProfileJSON    string `json:"profile_json"`

	Status     string `json:"status"` // unbound | ok | error
	LastError  string `json:"last_error"`
	LastSyncAt int64  `json:"last_sync_at"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

// HasToken 前端只说"有没有凭据"，绝不下发凭据本身。
func (p *FRPPlatform) HasToken() bool { return len(p.TokenEnc) > 0 }

// FRPTunnel 平台侧隧道的本地镜像。remote_id 是平台侧主键（Sakura 为数字 ID，
// ChmlFrp 同为数字 ID 但以字符串存，避免两平台类型分叉）。
type FRPTunnel struct {
	ID         int64  `json:"id"`
	PlatformID int64  `json:"platform_id"`
	RemoteID   string `json:"remote_id"`
	Name       string `json:"name"`
	Proto      string `json:"proto"`
	NodeID     string `json:"node_id"`
	NodeName   string `json:"node_name"`
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	Remote     string `json:"remote"`

	Online       bool   `json:"online"`
	Status       string `json:"status"` // normal | banned | unknown
	StatusReason string `json:"status_reason"`
	Conns        int    `json:"conns"`
	LocalConns   int    `json:"local_conns"` // 面板在 frpc 所在节点数到的活跃转发连接（Sakura 无平台值时的口径）
	TodayUp      int64  `json:"today_up"`
	TodayDown    int64  `json:"today_down"`
	Uptime       int64  `json:"uptime"`
	ClientVer    string `json:"client_ver"`
	Extra        string `json:"extra"`

	LockEdit    bool  `json:"lock_edit"`
	LockDelete  bool  `json:"lock_delete"`
	LockMigrate bool  `json:"lock_migrate"`
	SyncedAt    int64 `json:"synced_at"`
}

// FRPNode 节点镜像。caps 用字符串数组承载两平台能力位的并集语义
// （http/udp/web/ipv6/defense/vip/private/beta…），前端只按标签渲染。
type FRPNode struct {
	ID          int64    `json:"id"`
	PlatformID  int64    `json:"platform_id"`
	RemoteID    string   `json:"remote_id"`
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Area        string   `json:"area"`
	GroupName   string   `json:"group_name"`
	Caps        []string `json:"caps"`
	Online      bool     `json:"online"`
	Load        float64  `json:"load"`
	Uptime      int64    `json:"uptime"`
	Description string   `json:"description"`
	SyncedAt    int64    `json:"synced_at"`
}

// FRPUsage 单次同步的用量快照，用于面板自绘趋势（平台侧历史接口各有限制，
// 自留存一份才能做统一的跨平台曲线）。
type FRPUsage struct {
	TS             int64 `json:"ts"`
	TrafficDayUsed int64 `json:"traffic_day_used"`
	TrafficRemain  int64 `json:"traffic_remain"`
	TrafficUp      int64 `json:"traffic_up"`
	TrafficDown    int64 `json:"traffic_down"`
	Conns          int   `json:"conns"`
	TunnelOnline   int   `json:"tunnel_online"`
	TunnelTotal    int   `json:"tunnel_total"`
}

// ---------- frp_platform ----------

const frpPlatformCols = `id, kind, name, token_enc, refresh_enc, COALESCE(token_expire_at,0),
	COALESCE(uid,''), COALESCE(username,''), COALESCE(group_name,''), COALESCE(speed_limit,''),
	COALESCE(realname,''), COALESCE(tunnel_used,0), COALESCE(tunnel_quota,0), COALESCE(conns,0),
	COALESCE(conns_src,''),
	COALESCE(traffic_day_used,0), COALESCE(traffic_remain,0), COALESCE(traffic_up,0), COALESCE(traffic_down,0),
	COALESCE(profile_json,'{}'), status, COALESCE(last_error,''), COALESCE(last_sync_at,0), created_at, COALESCE(updated_at,0)`

func scanFRPPlatform(p *FRPPlatform, row interface {
	Scan(dest ...any) error
}) error {
	return row.Scan(&p.ID, &p.Kind, &p.Name, &p.TokenEnc, &p.RefreshEnc, &p.TokenExpireAt,
		&p.UID, &p.Username, &p.GroupName, &p.SpeedLimit,
		&p.Realname, &p.TunnelUsed, &p.TunnelQuota, &p.Conns, &p.ConnsSrc,
		&p.TrafficDayUsed, &p.TrafficRemain, &p.TrafficUp, &p.TrafficDown,
		&p.ProfileJSON, &p.Status, &p.LastError, &p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt)
}

func (db *DB) ListFRPPlatforms() ([]*FRPPlatform, error) {
	rows, err := db.SQL.Query(`SELECT ` + frpPlatformCols + ` FROM frp_platform ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FRPPlatform
	for rows.Next() {
		p := &FRPPlatform{}
		if err := scanFRPPlatform(p, rows); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (db *DB) GetFRPPlatform(id int64) (*FRPPlatform, error) {
	p := &FRPPlatform{}
	err := scanFRPPlatform(p, db.SQL.QueryRow(`SELECT `+frpPlatformCols+` FROM frp_platform WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (db *DB) GetFRPPlatformByName(kind, name string) (*FRPPlatform, error) {
	p := &FRPPlatform{}
	err := scanFRPPlatform(p, db.SQL.QueryRow(
		`SELECT `+frpPlatformCols+` FROM frp_platform WHERE kind = ? AND name = ?`, kind, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// InsertFRPPlatform 新建平台绑定，返回自增 ID。
func (db *DB) InsertFRPPlatform(p *FRPPlatform) (int64, error) {
	now := time.Now().Unix()
	res, err := db.SQL.Exec(`INSERT INTO frp_platform
		(kind, name, token_enc, refresh_enc, token_expire_at, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Kind, p.Name, p.TokenEnc, p.RefreshEnc, p.TokenExpireAt, p.Status, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateFRPPlatformCreds 只更新凭据（换 Token / OAuth 刷新后回写）。
func (db *DB) UpdateFRPPlatformCreds(id int64, tokenEnc, refreshEnc []byte, expireAt int64) error {
	_, err := db.SQL.Exec(`UPDATE frp_platform SET token_enc = ?, refresh_enc = ?, token_expire_at = ?, updated_at = ? WHERE id = ?`,
		tokenEnc, refreshEnc, expireAt, time.Now().Unix(), id)
	return err
}

func (db *DB) RenameFRPPlatform(id int64, name string) error {
	_, err := db.SQL.Exec(`UPDATE frp_platform SET name = ?, updated_at = ? WHERE id = ?`,
		name, time.Now().Unix(), id)
	return err
}

// SaveFRPPlatformProfile 同步结果回写：账号画像 + 用量快照 + 状态。
// 凭据不在本方法内更新（OAuth 刷新单独走 UpdateFRPPlatformCreds）。
func (db *DB) SaveFRPPlatformProfile(p *FRPPlatform) error {
	_, err := db.SQL.Exec(`UPDATE frp_platform SET
		uid = ?, username = ?, group_name = ?, speed_limit = ?, realname = ?,
		tunnel_used = ?, tunnel_quota = ?, conns = ?, conns_src = ?,
		traffic_day_used = ?, traffic_remain = ?, traffic_up = ?, traffic_down = ?,
		profile_json = ?, status = ?, last_error = ?, last_sync_at = ?, updated_at = ?
		WHERE id = ?`,
		p.UID, p.Username, p.GroupName, p.SpeedLimit, p.Realname,
		p.TunnelUsed, p.TunnelQuota, p.Conns, p.ConnsSrc,
		p.TrafficDayUsed, p.TrafficRemain, p.TrafficUp, p.TrafficDown,
		p.ProfileJSON, p.Status, p.LastError, p.LastSyncAt, time.Now().Unix(), p.ID)
	return err
}

// SetFRPPlatformError 同步失败时的状态回写（不清空上次成功的画像，
// 让面板在平台临时故障时仍能展示最近一次有效数据）。
func (db *DB) SetFRPPlatformError(id int64, msg string) error {
	return db.SetFRPPlatformStatus(id, "error", msg)
}

// SetFRPPlatformStatus 状态流转。凭据失效时置 'unbound'，前端据此弹重新授权。
func (db *DB) SetFRPPlatformStatus(id int64, status, msg string) error {
	_, err := db.SQL.Exec(`UPDATE frp_platform SET status = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		status, msg, time.Now().Unix(), id)
	return err
}

func (db *DB) DeleteFRPPlatform(id int64) error {
	_, err := db.SQL.Exec(`DELETE FROM frp_platform WHERE id = ?`, id)
	return err
}

// ---------- frp_tunnel ----------

const frpTunnelCols = `id, platform_id, remote_id, name, proto, COALESCE(node_id,''), COALESCE(node_name,''),
	COALESCE(local_ip,''), COALESCE(local_port,0), COALESCE(remote,''), COALESCE(online,0), status,
	COALESCE(status_reason,''), COALESCE(conns,0), COALESCE(local_conns,0), COALESCE(today_up,0), COALESCE(today_down,0),
	COALESCE(uptime,0), COALESCE(client_ver,''), COALESCE(extra,''),
	COALESCE(lock_edit,0), COALESCE(lock_delete,0), COALESCE(lock_migrate,0), COALESCE(synced_at,0)`

func scanFRPTunnel(t *FRPTunnel, row interface {
	Scan(dest ...any) error
}) error {
	var online, lEdit, lDel, lMig int
	if err := row.Scan(&t.ID, &t.PlatformID, &t.RemoteID, &t.Name, &t.Proto, &t.NodeID, &t.NodeName,
		&t.LocalIP, &t.LocalPort, &t.Remote, &online, &t.Status,
		&t.StatusReason, &t.Conns, &t.LocalConns, &t.TodayUp, &t.TodayDown,
		&t.Uptime, &t.ClientVer, &t.Extra,
		&lEdit, &lDel, &lMig, &t.SyncedAt); err != nil {
		return err
	}
	t.Online = online == 1
	t.LockEdit = lEdit == 1
	t.LockDelete = lDel == 1
	t.LockMigrate = lMig == 1
	return nil
}

func (db *DB) ListFRPTunnels() ([]*FRPTunnel, error) {
	rows, err := db.SQL.Query(`SELECT ` + frpTunnelCols + ` FROM frp_tunnel ORDER BY platform_id, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FRPTunnel
	for rows.Next() {
		t := &FRPTunnel{}
		if err := scanFRPTunnel(t, rows); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (db *DB) GetFRPTunnel(id int64) (*FRPTunnel, error) {
	t := &FRPTunnel{}
	err := scanFRPTunnel(t, db.SQL.QueryRow(`SELECT `+frpTunnelCols+` FROM frp_tunnel WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ReplaceFRPTunnels 用一次同步的完整结果替换某平台的隧道镜像：
// upsert 命中 remote_id 的行，并删除平台侧已不存在的行（事务内完成，
// 避免同步中途失败留下半新半旧的列表）。返回写回后的本地 ID 映射。
func (db *DB) ReplaceFRPTunnels(platformID int64, list []*FRPTunnel) (map[string]int64, error) {
	tx, err := db.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	keep := make(map[string]bool, len(list))
	ids := make(map[string]int64, len(list))
	for _, t := range list {
		keep[t.RemoteID] = true
		if _, err := tx.Exec(`INSERT INTO frp_tunnel
			(platform_id, remote_id, name, proto, node_id, node_name, local_ip, local_port, remote,
			 online, status, status_reason, conns, today_up, today_down, uptime, client_ver, extra,
			 lock_edit, lock_delete, lock_migrate, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(platform_id, remote_id) DO UPDATE SET
				name=excluded.name, proto=excluded.proto, node_id=excluded.node_id, node_name=excluded.node_name,
				local_ip=excluded.local_ip, local_port=excluded.local_port, remote=excluded.remote,
				online=excluded.online, status=excluded.status, status_reason=excluded.status_reason,
				conns=excluded.conns, today_up=excluded.today_up, today_down=excluded.today_down,
				uptime=excluded.uptime, client_ver=excluded.client_ver, extra=excluded.extra,
				lock_edit=excluded.lock_edit, lock_delete=excluded.lock_delete, lock_migrate=excluded.lock_migrate,
				synced_at=excluded.synced_at`,
			platformID, t.RemoteID, t.Name, t.Proto, t.NodeID, t.NodeName, t.LocalIP, t.LocalPort, t.Remote,
			boolInt(t.Online), t.Status, t.StatusReason, t.Conns, t.TodayUp, t.TodayDown, t.Uptime, t.ClientVer, t.Extra,
			boolInt(t.LockEdit), boolInt(t.LockDelete), boolInt(t.LockMigrate), t.SyncedAt); err != nil {
			return nil, err
		}
	}

	// 删除平台侧已消失的隧道（面板里点删后同步要对齐）
	rows, err := tx.Query(`SELECT id, remote_id FROM frp_tunnel WHERE platform_id = ?`, platformID)
	if err != nil {
		return nil, err
	}
	var stale []int64
	for rows.Next() {
		var id int64
		var rid string
		if err := rows.Scan(&id, &rid); err != nil {
			rows.Close()
			return nil, err
		}
		if !keep[rid] {
			stale = append(stale, id)
		} else {
			ids[rid] = id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range stale {
		if _, err := tx.Exec(`DELETE FROM frp_tunnel WHERE id = ?`, id); err != nil {
			return nil, err
		}
	}
	return ids, tx.Commit()
}

// InsertFRPTunnel 面板侧新建隧道后就地落库（remote_id 由平台返回）。
func (db *DB) InsertFRPTunnel(t *FRPTunnel) (int64, error) {
	now := time.Now().Unix()
	res, err := db.SQL.Exec(`INSERT INTO frp_tunnel
		(platform_id, remote_id, name, proto, node_id, node_name, local_ip, local_port, remote,
		 online, status, status_reason, conns, today_up, today_down, uptime, client_ver, extra,
		 lock_edit, lock_delete, lock_migrate, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.PlatformID, t.RemoteID, t.Name, t.Proto, t.NodeID, t.NodeName, t.LocalIP, t.LocalPort, t.Remote,
		boolInt(t.Online), t.Status, t.StatusReason, t.Conns, t.TodayUp, t.TodayDown, t.Uptime, t.ClientVer, t.Extra,
		boolInt(t.LockEdit), boolInt(t.LockDelete), boolInt(t.LockMigrate), now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) DeleteFRPTunnel(id int64) error {
	_, err := db.SQL.Exec(`DELETE FROM frp_tunnel WHERE id = ?`, id)
	return err
}

// SetFRPTunnelLocalConns 批量回写本地连接计数（key=平台侧 remote_id）。
// 本地计数与平台同步是两条独立管线：这里只动 local_conns，不碰 synced_at，
// 平台同步的 upsert 也不覆盖本列（列不在其 INSERT/UPDATE 清单里）。
func (db *DB) SetFRPTunnelLocalConns(platformID int64, m map[string]int64) error {
	for rid, n := range m {
		if _, err := db.SQL.Exec(`UPDATE frp_tunnel SET local_conns = ? WHERE platform_id = ? AND remote_id = ?`,
			n, platformID, rid); err != nil {
			return err
		}
	}
	return nil
}

// SetFRPPlatformConns 平台级连接数与口径标记（local 计数管线回写用，
// 不更新 last_sync_at——那是平台同步的时间戳，语义不能混）。
func (db *DB) SetFRPPlatformConns(id int64, conns int, src string) error {
	_, err := db.SQL.Exec(`UPDATE frp_platform SET conns = ?, conns_src = ?, updated_at = ? WHERE id = ?`,
		conns, src, time.Now().Unix(), id)
	return err
}

// SetFRPPlatformDayTraffic 平台级今日流量覆盖（Cloudflare GraphQL 管线回写用，
// 与 SaveFRPPlatformProfile 的 traffic_day_used 分离：CF 的 UserInfo 不带流量值，
// 画像保存会写 0，流量必须在其之后由本方法覆盖，两条管线互不覆盖）。
func (db *DB) SetFRPPlatformDayTraffic(id int64, dayUsed int64) error {
	_, err := db.SQL.Exec(`UPDATE frp_platform SET traffic_day_used = ?, updated_at = ? WHERE id = ?`,
		dayUsed, time.Now().Unix(), id)
	return err
}

// SetFRPTunnelTraffic 批量回写隧道今日流量（key=平台侧 remote_id，value=24h
// 边缘字节数）。与 SetFRPTunnelLocalConns 同思路：独立管线，只动 today_up，
// 不碰平台同步 upsert 清单里的其它列。CF 的 edgeResponseBytes 是边缘→源站
// 方向的回源量，无法再拆上下行，统一记 today_up（前端按上+下行合计展示）。
func (db *DB) SetFRPTunnelTraffic(platformID int64, m map[string]int64) error {
	for rid, n := range m {
		if _, err := db.SQL.Exec(`UPDATE frp_tunnel SET today_up = ? WHERE platform_id = ? AND remote_id = ?`,
			n, platformID, rid); err != nil {
			return err
		}
	}
	return nil
}

// DeleteFRPTunnelsOfPlatform 解绑平台时显式清理（外键级联只在 foreign_keys 开启时生效，
// 这里双保险，也便于删平台前后统计）。
func (db *DB) DeleteFRPTunnelsOfPlatform(platformID int64) error {
	_, err := db.SQL.Exec(`DELETE FROM frp_tunnel WHERE platform_id = ?`, platformID)
	return err
}

// ---------- frp_node ----------

const frpNodeCols = `id, platform_id, remote_id, name, COALESCE(host,''), COALESCE(area,''),
	COALESCE(group_name,''), COALESCE(caps,'[]'), COALESCE(online,0), COALESCE(load,0),
	COALESCE(uptime,0), COALESCE(description,''), COALESCE(synced_at,0)`

func scanFRPNode(n *FRPNode, row interface {
	Scan(dest ...any) error
}) error {
	var caps string
	var online int
	if err := row.Scan(&n.ID, &n.PlatformID, &n.RemoteID, &n.Name, &n.Host, &n.Area,
		&n.GroupName, &caps, &online, &n.Load, &n.Uptime, &n.Description, &n.SyncedAt); err != nil {
		return err
	}
	n.Caps = decodeTags(caps)
	n.Online = online == 1
	return nil
}

func (db *DB) ListFRPNodes() ([]*FRPNode, error) {
	rows, err := db.SQL.Query(`SELECT ` + frpNodeCols + ` FROM frp_node ORDER BY platform_id, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FRPNode
	for rows.Next() {
		n := &FRPNode{}
		if err := scanFRPNode(n, rows); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListFRPNodesInUse 只返回「本账号真正在用」的节点：被任一隧道挂载的那些。
// 平台节点表是全网节点（Sakura 实测 71 条），全量下发对使用者毫无信息量，
// 对访客还等于公开未使用的拓扑。隧道记录里的 node_id 存的就是节点 remote_id
// （见 fillNodeNames），所以直接按 remote_id 关联即可。
func (db *DB) ListFRPNodesInUse(platformID int64) ([]*FRPNode, error) {
	rows, err := db.SQL.Query(`SELECT `+frpNodeCols+` FROM frp_node n
		WHERE n.platform_id = ? AND n.remote_id <> ''
		  AND EXISTS (
		    SELECT 1 FROM frp_tunnel t
		    WHERE t.platform_id = n.platform_id AND t.node_id = n.remote_id
		  )
		ORDER BY n.online DESC, n.load ASC, n.name ASC`, platformID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FRPNode
	for rows.Next() {
		n := &FRPNode{}
		if err := scanFRPNode(n, rows); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ReplaceFRPNodes 整表替换某平台的节点镜像（节点集合变动不频繁，直接对比删除最省心）。
func (db *DB) ReplaceFRPNodes(platformID int64, list []*FRPNode) error {
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	keep := make(map[string]bool, len(list))
	for _, n := range list {
		keep[n.RemoteID] = true
		if _, err := tx.Exec(`INSERT INTO frp_node
			(platform_id, remote_id, name, host, area, group_name, caps, online, load, uptime, description, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(platform_id, remote_id) DO UPDATE SET
				name=excluded.name, host=excluded.host, area=excluded.area, group_name=excluded.group_name,
				caps=excluded.caps, online=excluded.online, load=excluded.load, uptime=excluded.uptime,
				description=excluded.description, synced_at=excluded.synced_at`,
			platformID, n.RemoteID, n.Name, n.Host, n.Area, n.GroupName, encodeTags(n.Caps),
			boolInt(n.Online), n.Load, n.Uptime, n.Description, n.SyncedAt); err != nil {
			return err
		}
	}

	rows, err := tx.Query(`SELECT id, remote_id FROM frp_node WHERE platform_id = ?`, platformID)
	if err != nil {
		return err
	}
	var stale []int64
	for rows.Next() {
		var id int64
		var rid string
		if err := rows.Scan(&id, &rid); err != nil {
			rows.Close()
			return err
		}
		if !keep[rid] {
			stale = append(stale, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range stale {
		if _, err := tx.Exec(`DELETE FROM frp_node WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) DeleteFRPNodesOfPlatform(platformID int64) error {
	_, err := db.SQL.Exec(`DELETE FROM frp_node WHERE platform_id = ?`, platformID)
	return err
}

// ---------- frp_usage ----------

// AppendFRPUsage 记一条用量快照。同一分钟内重复同步只保留最后一条，
// 避免手动连点刷新把趋势图打散成一堆同刻点。
func (db *DB) AppendFRPUsage(platformID int64, u *FRPUsage) error {
	if u.TS <= 0 {
		u.TS = time.Now().Unix()
	}
	var last int64
	err := db.SQL.QueryRow(`SELECT COALESCE(MAX(ts),0) FROM frp_usage WHERE platform_id = ?`, platformID).Scan(&last)
	if err != nil {
		return err
	}
	if last > 0 && u.TS-last < 60 {
		_, err = db.SQL.Exec(`UPDATE frp_usage SET ts = ?, traffic_day_used = ?, traffic_remain = ?,
			traffic_up = ?, traffic_down = ?, conns = ?, tunnel_online = ?, tunnel_total = ?
			WHERE platform_id = ? AND ts = ?`,
			u.TS, u.TrafficDayUsed, u.TrafficRemain, u.TrafficUp, u.TrafficDown,
			u.Conns, u.TunnelOnline, u.TunnelTotal, platformID, last)
		return err
	}
	_, err = db.SQL.Exec(`INSERT INTO frp_usage
		(platform_id, ts, traffic_day_used, traffic_remain, traffic_up, traffic_down, conns, tunnel_online, tunnel_total)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		platformID, u.TS, u.TrafficDayUsed, u.TrafficRemain, u.TrafficUp, u.TrafficDown,
		u.Conns, u.TunnelOnline, u.TunnelTotal)
	return err
}

// ListFRPUsage 取某平台最近 sinceTs 之后的快照（升序），用于趋势图。
func (db *DB) ListFRPUsage(platformID int64, sinceTs int64, limit int) ([]*FRPUsage, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := db.SQL.Query(`SELECT ts, COALESCE(traffic_day_used,0), COALESCE(traffic_remain,0),
		COALESCE(traffic_up,0), COALESCE(traffic_down,0), COALESCE(conns,0),
		COALESCE(tunnel_online,0), COALESCE(tunnel_total,0)
		FROM frp_usage WHERE platform_id = ? AND ts >= ? ORDER BY ts DESC LIMIT ?`,
		platformID, sinceTs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FRPUsage
	for rows.Next() {
		u := &FRPUsage{}
		if err := rows.Scan(&u.TS, &u.TrafficDayUsed, &u.TrafficRemain, &u.TrafficUp, &u.TrafficDown,
			&u.Conns, &u.TunnelOnline, &u.TunnelTotal); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 查询按 ts DESC 取"最近 N 条"，返回前翻回升序供图表直接消费
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// PruneFRPUsage 清理过期快照（清理任务调用）。
func (db *DB) PruneFRPUsage(before int64) (int64, error) {
	res, err := db.SQL.Exec(`DELETE FROM frp_usage WHERE ts < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
