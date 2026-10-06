package store

import "testing"

// 迁移 v13（doc/16）：frp_platform.kind 的 CHECK 约束从 ('natfrp','chmlfrp')
// 扩为 ('natfrp','chmlfrp','cloudflared')。SQLite 不能 ALTER CHECK，走的是
// 官方 12 步重建表流程。本测试锁两件事：
//  1. 旧数据（natfrp/chmlfrp 平台 + 各列含 NULL/默认值）在重建后原样保留；
//  2. 重建后能插入 kind='cloudflared' 的新平台，且 (kind,name) 唯一索引仍在。
func TestMigrationV13CloudflaredKind(t *testing.T) {
	db := newTestDB(t)

	// 老库形态：v10~v12 迁移已跑过，此时 kind CHECK 尚不含 cloudflared。
	// 直接插两条旧平台，列上带 NULL（last_error）与非默认值，拷贝语义才有区分度。
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.SQL.Exec(q, args...); err != nil {
			t.Fatalf("exec %s: %v", q[:60], err)
		}
	}
	mustExec(`INSERT INTO frp_platform
		(id, kind, name, token_enc, uid, username, tunnel_used, tunnel_quota, conns, conns_src,
		 traffic_day_used, status, last_error, last_sync_at, created_at, updated_at)
		VALUES (1, 'natfrp', 'Sakura', x'0102', 'u1', 'sakura-user', 2, 2, 8, 'local',
			123456789, 'ok', NULL, 1700000000, 1700000000, 1700000000)`)
	mustExec(`INSERT INTO frp_platform
		(id, kind, name, token_enc, refresh_enc, token_expire_at, uid, username, tunnel_used, tunnel_quota,
		 conns, conns_src, traffic_up, traffic_down, status, last_sync_at, created_at, updated_at)
		VALUES (2, 'chmlfrp', 'ChmlFrp', x'0304', x'0506', 1700001000, 'u2', 'chml-user', 1, 4,
			3, 'platform', 100, 200, 'error', 1700000500, 1700000000, 1700000000)`)

	// 迁移在 Open 时已跑（newTestDB 打开即到最新版）。验证旧数据保留。
	p1, err := db.GetFRPPlatform(1)
	if err != nil || p1 == nil {
		t.Fatalf("读回 natfrp 平台失败: %v", err)
	}
	if p1.Kind != "natfrp" || p1.Username != "sakura-user" || p1.TunnelUsed != 2 || p1.Conns != 8 || p1.ConnsSrc != "local" {
		t.Fatalf("natfrp 平台数据在重建后失真: %+v", p1)
	}
	if p1.LastError != "" {
		t.Fatalf("NULL last_error 应归一为空串，got %q", p1.LastError)
	}
	p2, err := db.GetFRPPlatform(2)
	if err != nil || p2 == nil {
		t.Fatalf("读回 chmlfrp 平台失败: %v", err)
	}
	if p2.Kind != "chmlfrp" || p2.TokenExpireAt != 1700001000 || p2.TrafficUp != 100 || p2.Status != "error" {
		t.Fatalf("chmlfrp 平台数据在重建后失真: %+v", p2)
	}

	// 重建后的表应接受 cloudflared kind
	id, err := db.InsertFRPPlatform(&FRPPlatform{Kind: "cloudflared", Name: "Cloudflare", Status: "ok"})
	if err != nil {
		t.Fatalf("插入 cloudflared 平台被拒（CHECK 约束未生效？）: %v", err)
	}
	p3, _ := db.GetFRPPlatform(id)
	if p3 == nil || p3.Kind != "cloudflared" {
		t.Fatalf("cloudflared 平台读回失败: %+v", p3)
	}

	// (kind, name) 唯一索引在重建后仍生效
	if _, err := db.InsertFRPPlatform(&FRPPlatform{Kind: "cloudflared", Name: "Cloudflare", Status: "ok"}); err == nil {
		t.Fatal("(kind,name) 重复插入应报唯一约束冲突")
	}
}
