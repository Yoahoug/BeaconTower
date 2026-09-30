package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// FRPDeploy 节点侧客户端托管记录（doc/13 §14）：某节点上跑一个 frpc 容器，
// 承载某平台的若干条隧道。一个 (平台, 节点) 只允许一条记录。
type FRPDeploy struct {
	ID         int64
	PlatformID int64
	ServerID   int64
	TunnelIDs  string // JSON 数组（frp_tunnel.id）
	Image      string
	Container  string
	ConfigPath string
	Status     string // running/stopped/missing/error/pending
	LastError  string
	DockerVer  string
	Dirty      bool // 平台侧隧道变动后置位：当前容器里的配置已过期，待同步
	LastSyncAt int64
	CreatedAt  int64
	UpdatedAt  int64
}

// TunnelIDList 解析隧道 ID 列表。
func (d *FRPDeploy) TunnelIDList() []int64 {
	var out []int64
	if d == nil || d.TunnelIDs == "" {
		return out
	}
	_ = json.Unmarshal([]byte(d.TunnelIDs), &out)
	return out
}

// SetTunnelIDList 写入隧道 ID 列表（去重、保持顺序）。
func (d *FRPDeploy) SetTunnelIDList(ids []int64) {
	seen := map[int64]bool{}
	uniq := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	b, _ := json.Marshal(uniq)
	d.TunnelIDs = string(b)
}

// HasTunnel 该托管是否包含某隧道。
func (d *FRPDeploy) HasTunnel(id int64) bool {
	for _, x := range d.TunnelIDList() {
		if x == id {
			return true
		}
	}
	return false
}

func scanFRPDeploy(row interface{ Scan(...any) error }) (*FRPDeploy, error) {
	d := &FRPDeploy{}
	var dirty int
	err := row.Scan(&d.ID, &d.PlatformID, &d.ServerID, &d.TunnelIDs, &d.Image, &d.Container,
		&d.ConfigPath, &d.Status, &d.LastError, &d.DockerVer, &dirty, &d.LastSyncAt, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	d.Dirty = dirty == 1
	return d, nil
}

const frpDeployCols = `id, platform_id, server_id, tunnel_ids, image, container, config_path,
	status, last_error, docker_ver, dirty, last_sync_at, created_at, updated_at`

// ListFRPDeploys 全部托管记录（按 id 升序）。
func (db *DB) ListFRPDeploys() ([]*FRPDeploy, error) {
	rows, err := db.SQL.Query(`SELECT ` + frpDeployCols + ` FROM frp_deploy ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FRPDeploy
	for rows.Next() {
		d, err := scanFRPDeploy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetFRPDeploy 按主键取。
func (db *DB) GetFRPDeploy(id int64) (*FRPDeploy, error) {
	row := db.SQL.QueryRow(`SELECT `+frpDeployCols+` FROM frp_deploy WHERE id = ?`, id)
	d, err := scanFRPDeploy(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetFRPDeployByPair 按 (平台, 节点) 取（唯一）。
func (db *DB) GetFRPDeployByPair(platformID, serverID int64) (*FRPDeploy, error) {
	row := db.SQL.QueryRow(`SELECT `+frpDeployCols+` FROM frp_deploy WHERE platform_id = ? AND server_id = ?`, platformID, serverID)
	d, err := scanFRPDeploy(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// UpsertFRPDeploy 新建或更新（按 (平台, 节点) 唯一键），返回记录 id。
func (db *DB) UpsertFRPDeploy(d *FRPDeploy) (int64, error) {
	now := time.Now().Unix()
	dirty := 0
	if d.Dirty {
		dirty = 1
	}
	old, err := db.GetFRPDeployByPair(d.PlatformID, d.ServerID)
	if err != nil {
		return 0, err
	}
	if old == nil {
		res, err := db.SQL.Exec(`INSERT INTO frp_deploy
			(platform_id, server_id, tunnel_ids, image, container, config_path, status, last_error, docker_ver, dirty, last_sync_at, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			d.PlatformID, d.ServerID, d.TunnelIDs, d.Image, d.Container, d.ConfigPath,
			d.Status, d.LastError, d.DockerVer, dirty, d.LastSyncAt, now, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	if _, err := db.SQL.Exec(`UPDATE frp_deploy SET tunnel_ids=?, image=?, container=?, config_path=?,
		status=?, last_error=?, docker_ver=?, dirty=?, last_sync_at=?, updated_at=? WHERE id=?`,
		d.TunnelIDs, d.Image, d.Container, d.ConfigPath, d.Status, d.LastError, d.DockerVer,
		dirty, d.LastSyncAt, now, old.ID); err != nil {
		return 0, err
	}
	return old.ID, nil
}

// SetFRPDeployResult 落库一次同步/动作的结果。
func (db *DB) SetFRPDeployResult(id int64, status, lastErr, dockerVer string, synced bool, dirty bool) error {
	now := time.Now().Unix()
	lastSync := int64(0)
	if synced {
		lastSync = now
	}
	_, err := db.SQL.Exec(`UPDATE frp_deploy SET status=?, last_error=?, docker_ver=?,
		dirty=?, last_sync_at=CASE WHEN ? > 0 THEN ? ELSE last_sync_at END, updated_at=? WHERE id=?`,
		status, lastErr, dockerVer, boolToInt(dirty), lastSync, lastSync, now, id)
	return err
}

// MarkFRPDeploysDirtyByTunnel 隧道被改动/删除后，把包含它的托管标记为待同步。
func (db *DB) MarkFRPDeploysDirtyByTunnel(tunnelID int64) (int, error) {
	list, err := db.ListFRPDeploys()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range list {
		if !d.HasTunnel(tunnelID) {
			continue
		}
		if _, err := db.SQL.Exec(`UPDATE frp_deploy SET dirty=1, updated_at=? WHERE id=?`, time.Now().Unix(), d.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// DeleteFRPDeploy 删除记录。
func (db *DB) DeleteFRPDeploy(id int64) error {
	_, err := db.SQL.Exec(`DELETE FROM frp_deploy WHERE id=?`, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
