package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// DB SQLite 访问层封装。
type DB struct {
	SQL *sql.DB
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
const schemaVersion = 3

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
	}
	return fmt.Errorf("unknown migration %d", v)
}
