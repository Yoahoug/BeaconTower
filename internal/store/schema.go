package store

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// DB SQLite 访问层封装。
type DB struct {
	SQL *sql.DB

	// roundMu/roundCmds 采集轮写缓冲：同一轮内所有采集写入合并为单个事务，
	// 每轮一次 fsync（原为每节点 4~5 条独立 Exec 各自 fsync）。
	// 缓冲只由采集器逐轮 flush（CommitRound），多节点 goroutine 并发登记。
	roundMu   sync.Mutex
	roundCmds []func(*sql.Tx) error
}

// execer SQL.Exec 的公共接口（*sql.DB 与 *sql.Tx 皆满足），
// 使单发/批量事务两条路径共用同一 SQL 构造函数。
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// queue 采集轮缓冲登记。须在 CommitRound 之前调用（collector 每轮结束时调用）。
func (db *DB) queue(fn func(*sql.Tx) error) {
	db.roundMu.Lock()
	defer db.roundMu.Unlock()
	db.roundCmds = append(db.roundCmds, fn)
}

// CommitRound 将本轮缓冲的写入以单事务提交；无缓冲写入时为 no-op。
// 事务内任一语句失败不影响其他语句（失败语句被跳过，下轮重写自会覆盖）。
func (db *DB) CommitRound() {
	db.roundMu.Lock()
	cmds := db.roundCmds
	db.roundCmds = nil
	db.roundMu.Unlock()
	if len(cmds) == 0 {
		return
	}
	tx, err := db.SQL.Begin()
	if err != nil {
		return
	}
	var firstErr error
	for _, fn := range cmds {
		// 单条失败不中断整轮（SQLite 语句级隔离，其余写入照常提交，下轮重写自会覆盖），
		// 但必须留痕：静默吞掉会让约束冲突/类型错误长期无人发现
		if err := fn(tx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// 单条语句失败不回滚整轮：提交其余成功写入
	_ = tx.Commit()
	if firstErr != nil {
		log.Printf("[store] 采集轮写入有语句失败（其余已提交）: %v", firstErr)
	}
}

// Open 打开数据库并执行建表 + 增量迁移。
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{SQL: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error { return db.SQL.Close() }

// 当前 schema 版本
const schemaVersion = 10

func (db *DB) migrate() error {
	if _, err := db.SQL.Exec(`CREATE TABLE IF NOT EXISTS schema_migration (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var v int
	err := db.SQL.QueryRow(`SELECT version FROM schema_migration LIMIT 1`).Scan(&v)
	if err == sql.ErrNoRows {
		if _, err := db.SQL.Exec(`INSERT INTO schema_migration (version) VALUES (0)`); err != nil {
			return err
		}
		v = 0
	} else if err != nil {
		return err
	}
	for v < schemaVersion {
		v++
		if err := applyMigration(db.SQL, v); err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
		if _, err := db.SQL.Exec(`UPDATE schema_migration SET version = ?`, v); err != nil {
			return err
		}
	}
	return nil
}

// v1：doc/03 全表；v2：doc/09 功耗列；v3：审计/会话/设置表 + 画像 geo_at。
func applyMigration(sqlDB *sql.DB, v int) error {
	exec := func(stmts ...string) error {
		for _, s := range stmts {
			if strings.TrimSpace(s) == "" {
				continue
			}
			if _, err := sqlDB.Exec(s); err != nil {
				return fmt.Errorf("%w (stmt: %.80s)", err, s)
			}
		}
		return nil
	}
	switch v {
	case 1:
		return exec(
			`CREATE TABLE IF NOT EXISTS admin (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				username TEXT NOT NULL,
				password_hash TEXT NOT NULL,
				created_at INTEGER NOT NULL,
				last_login_at INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS server (
				id INTEGER PRIMARY KEY,
				name TEXT NOT NULL,
				region TEXT DEFAULT '',
				region_source TEXT DEFAULT 'manual',
				tags TEXT DEFAULT '[]',
				note_public TEXT DEFAULT '',
				note_private TEXT DEFAULT '',
				sort_order INTEGER DEFAULT 0,
				hidden INTEGER DEFAULT 0,
				enabled INTEGER DEFAULT 1,
				created_at INTEGER NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS server_credential (
				server_id INTEGER PRIMARY KEY REFERENCES server(id) ON DELETE CASCADE,
				host TEXT NOT NULL,
				port INTEGER NOT NULL DEFAULT 22,
				username TEXT NOT NULL,
				auth_type TEXT NOT NULL CHECK (auth_type IN ('password','key')),
				password_enc BLOB,
				private_key_enc BLOB,
				passphrase_enc BLOB,
				host_key_fp TEXT,
				last_error TEXT,
				last_success_at INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS server_profile (
				server_id INTEGER PRIMARY KEY REFERENCES server(id) ON DELETE CASCADE,
				hostname TEXT,
				os_name TEXT,
				os_version TEXT,
				kernel TEXT,
				arch TEXT,
				cpu_model TEXT,
				cpu_cores INTEGER,
				mem_total INTEGER,
				swap_total INTEGER,
				disk_total INTEGER,
				disks_json TEXT,
				virt TEXT,
				public_ip TEXT,
				geo_country TEXT,
				geo_city TEXT,
				geo_at INTEGER,
				collected_at INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS latest_metric (
				server_id INTEGER PRIMARY KEY REFERENCES server(id) ON DELETE CASCADE,
				ts INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'online',
				cpu_pct REAL,
				mem_used INTEGER, mem_total INTEGER,
				swap_used INTEGER, swap_total INTEGER,
				disk_used INTEGER, disk_total INTEGER,
				net_in_bps REAL, net_out_bps REAL,
				net_in_total INTEGER, net_out_total INTEGER,
				tcp_conns INTEGER, udp_conns INTEGER,
				load1 REAL, load5 REAL, load15 REAL,
				uptime_s INTEGER,
				processes INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS metric_sample (
				id INTEGER PRIMARY KEY,
				server_id INTEGER NOT NULL REFERENCES server(id) ON DELETE CASCADE,
				ts INTEGER NOT NULL,
				cpu_pct REAL, mem_used INTEGER, swap_used INTEGER,
				disk_used INTEGER, net_in_bps REAL, net_out_bps REAL,
				tcp_conns INTEGER, udp_conns INTEGER,
				load1 REAL, load5 REAL, load15 REAL, uptime_s INTEGER, processes INTEGER
			)`,
			`CREATE INDEX IF NOT EXISTS idx_sample_srv_ts ON metric_sample(server_id, ts)`,
			`CREATE TABLE IF NOT EXISTS metric_hourly (
				server_id INTEGER NOT NULL,
				hour_ts INTEGER NOT NULL,
				cpu_avg REAL, cpu_max REAL,
				mem_avg INTEGER, mem_max INTEGER,
				net_in_avg REAL, net_out_avg REAL,
				net_in_total INTEGER, net_out_total INTEGER,
				PRIMARY KEY (server_id, hour_ts)
			)`,
			`CREATE TABLE IF NOT EXISTS session (
				token_hash TEXT PRIMARY KEY,
				expires_at INTEGER NOT NULL,
				created_at INTEGER NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS audit_log (
				id INTEGER PRIMARY KEY,
				ts INTEGER NOT NULL,
				actor TEXT NOT NULL,
				action TEXT NOT NULL,
				target TEXT NOT NULL,
				detail TEXT NOT NULL,
				source_ip_hash TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_log(ts DESC)`,
			`CREATE TABLE IF NOT EXISTS setting (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)`,
		)
	case 2: // doc/09 功耗增量
		return exec(
			`ALTER TABLE latest_metric ADD COLUMN power_w REAL`,
			`ALTER TABLE latest_metric ADD COLUMN cpu_w REAL`,
			`ALTER TABLE latest_metric ADD COLUMN dram_w REAL`,
			`ALTER TABLE latest_metric ADD COLUMN temp_c REAL`,
			`ALTER TABLE latest_metric ADD COLUMN freq_mhz INTEGER`,
			`ALTER TABLE latest_metric ADD COLUMN power_src TEXT`,
			`ALTER TABLE metric_sample ADD COLUMN power_w REAL`,
			`ALTER TABLE metric_sample ADD COLUMN cpu_w REAL`,
			`ALTER TABLE metric_sample ADD COLUMN dram_w REAL`,
			`ALTER TABLE metric_sample ADD COLUMN temp_c REAL`,
			`ALTER TABLE metric_sample ADD COLUMN freq_mhz INTEGER`,
			`ALTER TABLE metric_sample ADD COLUMN power_src TEXT`,
			`ALTER TABLE metric_hourly ADD COLUMN power_avg REAL`,
			`ALTER TABLE metric_hourly ADD COLUMN kwh REAL`,
			`ALTER TABLE server_profile ADD COLUMN power_rapl INTEGER DEFAULT 0`,
			`ALTER TABLE server_profile ADD COLUMN power_battery INTEGER DEFAULT 0`,
			`ALTER TABLE server_profile ADD COLUMN base_load_w REAL`,
			`ALTER TABLE server_profile ADD COLUMN base_load_source TEXT DEFAULT 'default'`,
		)
	case 3: // energy 累计（月度 kWh/电费由聚合推算，此处补 kwh 累计列）
		return exec(
			`ALTER TABLE server_profile ADD COLUMN month_kwh REAL DEFAULT 0`,
		)
	case 4: // 本机节点：is_self 标记（0/1），面板自身默认占首位
		return exec(
			`ALTER TABLE server ADD COLUMN is_self INTEGER DEFAULT 0`,
			`CREATE INDEX IF NOT EXISTS idx_server_self ON server(is_self)`,
		)
	case 5: // 采样表补流量累计计数（小时聚合/日流量的首尾差基线）+ 清理任务可走索引
		return exec(
			`ALTER TABLE metric_sample ADD COLUMN net_in_total INTEGER`,
			`ALTER TABLE metric_sample ADD COLUMN net_out_total INTEGER`,
			`CREATE INDEX IF NOT EXISTS idx_sample_ts ON metric_sample(ts)`,
		)
	case 6: // 流量监控：按日收发字节记录（首尾累计计数器差聚合）
		return exec(
			`CREATE TABLE IF NOT EXISTS metric_daily (
				server_id INTEGER NOT NULL REFERENCES server(id) ON DELETE CASCADE,
				day_ts INTEGER NOT NULL,
				in_total INTEGER DEFAULT 0,
				out_total INTEGER DEFAULT 0,
				updated_at INTEGER,
				PRIMARY KEY (server_id, day_ts)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_daily_ts ON metric_daily(day_ts)`,
		)
	case 7: // WG 组网：单网 + 主备 hub + peer（服务器/设备）+ 任务步骤 + hub 日流量
		return exec(
			`CREATE TABLE IF NOT EXISTS wg_network (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				subnet TEXT NOT NULL,
				hub_ip TEXT NOT NULL,
				iface TEXT NOT NULL DEFAULT 'wg0',
				keepalive INTEGER NOT NULL DEFAULT 25,
				mtu INTEGER NOT NULL DEFAULT 1420,
				active_hub_server_id INTEGER,
				created_at INTEGER NOT NULL,
				updated_at INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS wg_hub (
				server_id INTEGER PRIMARY KEY REFERENCES server(id) ON DELETE CASCADE,
				listen_port INTEGER NOT NULL DEFAULT 51820,
				public_key TEXT NOT NULL DEFAULT '',
				private_key_enc BLOB,
				endpoint TEXT DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending',
				last_error TEXT,
				quota_gb REAL,
				checked_at INTEGER,
				rx_cum INTEGER DEFAULT 0,
				tx_cum INTEGER DEFAULT 0
			)`,
			`CREATE TABLE IF NOT EXISTS wg_peer (
				id INTEGER PRIMARY KEY,
				kind TEXT NOT NULL CHECK (kind IN ('server','device')),
				server_id INTEGER REFERENCES server(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				wg_ip TEXT NOT NULL,
				public_key TEXT NOT NULL,
				private_key_enc BLOB,
				psk_enc BLOB,
				managed INTEGER NOT NULL DEFAULT 1,
				status TEXT NOT NULL DEFAULT 'pending',
				last_error TEXT,
				last_handshake INTEGER,
				rx_bytes INTEGER DEFAULT 0,
				tx_bytes INTEGER DEFAULT 0,
				checked_at INTEGER,
				created_at INTEGER NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_wg_peer_ip ON wg_peer(wg_ip)`,
			`CREATE INDEX IF NOT EXISTS idx_wg_peer_srv ON wg_peer(server_id)`,
			`CREATE TABLE IF NOT EXISTS wg_task (
				id INTEGER PRIMARY KEY,
				kind TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'running',
				payload TEXT NOT NULL DEFAULT '{}',
				result TEXT,
				created_at INTEGER NOT NULL,
				finished_at INTEGER
			)`,
			`CREATE TABLE IF NOT EXISTS wg_task_step (
				id INTEGER PRIMARY KEY,
				task_id INTEGER NOT NULL REFERENCES wg_task(id) ON DELETE CASCADE,
				seq INTEGER NOT NULL,
				server_id INTEGER,
				title TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'pending',
				log TEXT,
				started_at INTEGER,
				finished_at INTEGER
			)`,
			`CREATE INDEX IF NOT EXISTS idx_wg_step_task ON wg_task_step(task_id, seq)`,
			`CREATE TABLE IF NOT EXISTS wg_hub_traffic (
				hub_server_id INTEGER NOT NULL REFERENCES server(id) ON DELETE CASCADE,
				day_ts INTEGER NOT NULL,
				rx INTEGER DEFAULT 0,
				tx INTEGER DEFAULT 0,
				updated_at INTEGER,
				PRIMARY KEY (hub_server_id, day_ts)
			)`,
		)
	case 8: // 资产中转（doc/12 §8）：国内难以直连 GitHub 的二进制/脚本
		// 由面板测速镜像后缓存，再经 SSH 推送到节点
		return exec(
			`CREATE TABLE IF NOT EXISTS wg_asset (
				id INTEGER PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				version TEXT DEFAULT '',
				arch TEXT DEFAULT 'x86_64',
				sha256 TEXT DEFAULT '',
				sources TEXT NOT NULL DEFAULT '[]',
				path TEXT DEFAULT '',
				size INTEGER DEFAULT 0,
				note TEXT DEFAULT '',
				updated_at INTEGER
			)`,
		)
	case 9: // wg_task 心跳：watchdog 按 heartbeat_at 判活，长任务不再被误判中断
		return exec(`ALTER TABLE wg_task ADD COLUMN heartbeat_at INTEGER DEFAULT 0`)
	case 10: // 内网穿透平台管理（doc/13）：NATFRP / ChmlFrp 账号、隧道镜像、节点镜像、用量快照
		return exec(
			`CREATE TABLE IF NOT EXISTS frp_platform (
				id INTEGER PRIMARY KEY,
				kind TEXT NOT NULL CHECK (kind IN ('natfrp','chmlfrp')),
				name TEXT NOT NULL,
				token_enc BLOB,
				refresh_enc BLOB,
				token_expire_at INTEGER DEFAULT 0,
				uid TEXT DEFAULT '',
				username TEXT DEFAULT '',
				group_name TEXT DEFAULT '',
				speed_limit TEXT DEFAULT '',
				realname TEXT DEFAULT '',
				tunnel_used INTEGER DEFAULT 0,
				tunnel_quota INTEGER DEFAULT 0,
				conns INTEGER DEFAULT 0,
				traffic_day_used INTEGER DEFAULT 0,
				traffic_remain INTEGER DEFAULT 0,
				traffic_up INTEGER DEFAULT 0,
				traffic_down INTEGER DEFAULT 0,
				status TEXT NOT NULL DEFAULT 'unbound',
				last_error TEXT,
				last_sync_at INTEGER DEFAULT 0,
				profile_json TEXT DEFAULT '{}',
				created_at INTEGER NOT NULL,
				updated_at INTEGER DEFAULT 0
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_frp_platform_kind_name ON frp_platform(kind, name)`,
			`CREATE TABLE IF NOT EXISTS frp_usage (
				id INTEGER PRIMARY KEY,
				platform_id INTEGER NOT NULL REFERENCES frp_platform(id) ON DELETE CASCADE,
				ts INTEGER NOT NULL,
				traffic_day_used INTEGER DEFAULT 0,
				traffic_remain INTEGER DEFAULT 0,
				traffic_up INTEGER DEFAULT 0,
				traffic_down INTEGER DEFAULT 0,
				conns INTEGER DEFAULT 0,
				tunnel_online INTEGER DEFAULT 0,
				tunnel_total INTEGER DEFAULT 0
			)`,
			`CREATE INDEX IF NOT EXISTS idx_frp_usage ON frp_usage(platform_id, ts)`,
			`CREATE TABLE IF NOT EXISTS frp_tunnel (
				id INTEGER PRIMARY KEY,
				platform_id INTEGER NOT NULL REFERENCES frp_platform(id) ON DELETE CASCADE,
				remote_id TEXT NOT NULL DEFAULT '',
				name TEXT NOT NULL DEFAULT '',
				proto TEXT NOT NULL DEFAULT 'tcp',
				node_id TEXT DEFAULT '',
				node_name TEXT DEFAULT '',
				local_ip TEXT DEFAULT '',
				local_port INTEGER DEFAULT 0,
				remote TEXT DEFAULT '',
				online INTEGER DEFAULT 0,
				status TEXT NOT NULL DEFAULT 'unknown',
				status_reason TEXT DEFAULT '',
				conns INTEGER DEFAULT 0,
				today_up INTEGER DEFAULT 0,
				today_down INTEGER DEFAULT 0,
				uptime INTEGER DEFAULT 0,
				client_ver TEXT DEFAULT '',
				extra TEXT DEFAULT '',
				lock_edit INTEGER DEFAULT 0,
				lock_delete INTEGER DEFAULT 0,
				lock_migrate INTEGER DEFAULT 0,
				synced_at INTEGER DEFAULT 0
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_frp_tunnel_remote ON frp_tunnel(platform_id, remote_id)`,
			`CREATE TABLE IF NOT EXISTS frp_node (
				id INTEGER PRIMARY KEY,
				platform_id INTEGER NOT NULL REFERENCES frp_platform(id) ON DELETE CASCADE,
				remote_id TEXT NOT NULL DEFAULT '',
				name TEXT NOT NULL DEFAULT '',
				host TEXT DEFAULT '',
				area TEXT DEFAULT '',
				group_name TEXT DEFAULT '',
				caps TEXT DEFAULT '[]',
				online INTEGER DEFAULT 0,
				load REAL DEFAULT 0,
				uptime INTEGER DEFAULT 0,
				description TEXT DEFAULT '',
				synced_at INTEGER DEFAULT 0
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_frp_node_remote ON frp_node(platform_id, remote_id)`,
		)
	}
	return fmt.Errorf("unknown migration %d", v)
}
