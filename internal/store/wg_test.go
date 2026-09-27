package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestWGMigrationAndCRUD(t *testing.T) {
	db := newTestDB(t)
	// 网络初始为空
	if n, _ := db.GetWGNetwork(); n != nil {
		t.Fatal("初始网络应为空")
	}
	// 建网
	now := time.Now().Unix()
	n := &WGNetwork{Subnet: "10.66.66.0/24", HubIP: "10.66.66.2", Iface: "wg0",
		Keepalive: 25, MTU: 1420, CreatedAt: now, UpdatedAt: now}
	if err := db.EnsureWGNetwork(n); err != nil {
		t.Fatalf("EnsureWGNetwork: %v", err)
	}
	if n2, _ := db.GetWGNetwork(); n2 == nil || n2.Subnet != "10.66.66.0/24" || n2.HubIP != "10.66.66.2" {
		t.Fatalf("网络读取不符: %+v", n2)
	}
	// 依赖的服务器行（外键）
	hubSrvID, err := db.CreateServer(&Server{Name: "wg1", Region: "", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	spokeSrvID, err := db.CreateServer(&Server{Name: "ops", Region: "", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetActiveHub(hubSrvID, now); err != nil {
		t.Fatal(err)
	}
	n2, _ := db.GetWGNetwork()
	if n2.ActiveHubServerID != hubSrvID {
		t.Fatalf("active hub 应为 %d: %d", hubSrvID, n2.ActiveHubServerID)
	}
	// hub 槽位
	hub := &WGHub{ServerID: hubSrvID, ListenPort: 51820, PublicKey: "abc",
		PrivateKeyEnc: []byte{1, 2}, Endpoint: "1.2.3.4:51820", Status: "ok",
		QuotaGB: sql.NullFloat64{Float64: 1024, Valid: true}}
	if err := db.UpsertWGHub(hub); err != nil {
		t.Fatalf("UpsertWGHub: %v", err)
	}
	h2, _ := db.GetWGHub(hubSrvID)
	if h2 == nil || h2.ListenPort != 51820 || h2.Endpoint != "1.2.3.4:51820" || !h2.QuotaGB.Valid {
		t.Fatalf("hub 读取不符: %+v", h2)
	}
	// 更新（保留私钥）
	h2.PrivateKeyEnc = nil
	h2.Status = "error"
	if err := db.UpsertWGHub(h2); err != nil {
		t.Fatal(err)
	}
	h3, _ := db.GetWGHub(hubSrvID)
	if len(h3.PrivateKeyEnc) != 2 {
		t.Fatal("私钥应保留")
	}
	// peer
	pid, err := db.InsertWGPeer(&WGPeer{Kind: "server", ServerID: sql.NullInt64{Int64: spokeSrvID, Valid: true},
		Name: "ops", WgIP: "10.66.66.66", PublicKey: "pub1", Managed: true,
		Status: "pending", CreatedAt: now})
	if err != nil {
		t.Fatalf("InsertWGPeer: %v", err)
	}
	if _, err := db.InsertWGPeer(&WGPeer{Kind: "device", Name: "Mac",
		WgIP: "10.66.66.66", PublicKey: "pub2", Managed: true, Status: "pending", CreatedAt: now}); err == nil {
		t.Fatal("重复 IP 应违反唯一约束")
	}
	did, err := db.InsertWGPeer(&WGPeer{Kind: "device", Name: "Mac",
		WgIP: "10.66.66.11", PublicKey: "pub2", Managed: true, Status: "pending", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	peers, _ := db.ListWGPeers()
	if len(peers) != 2 {
		t.Fatalf("应有 2 个 peer: %d", len(peers))
	}
	bySrv, _ := db.GetWGPeerByServer(spokeSrvID)
	if bySrv == nil || bySrv.WgIP != "10.66.66.66" || !bySrv.Managed {
		t.Fatalf("按服务器查 peer 不符: %+v", bySrv)
	}
	p1, _ := db.GetWGPeer(pid)
	p1.Status = "online"
	p1.LastHandshake = sql.NullInt64{Int64: now, Valid: true}
	if err := db.UpdateWGPeer(p1); err != nil {
		t.Fatal(err)
	}
	p1b, _ := db.GetWGPeer(pid)
	if p1b.Status != "online" || !p1b.LastHandshake.Valid {
		t.Fatalf("peer 更新不符: %+v", p1b)
	}
	// 任务
	tid, err := db.InsertWGTask(&WGTask{Kind: "apply", Status: "running", Payload: `{"allocations":[]}`, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := db.HasRunningWGTask(); !ok {
		t.Fatal("应有运行中任务")
	}
	sid, err := db.InsertWGTaskStep(&WGTaskStep{TaskID: tid, Seq: 0, ServerID: sql.NullInt64{Int64: 7, Valid: true},
		Title: "中心节点", Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.StartWGTaskStep(sid, now); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishWGTaskStep(sid, "ok", "完成", now); err != nil {
		t.Fatal(err)
	}
	steps, _ := db.ListWGTaskSteps(tid)
	if len(steps) != 1 || steps[0].Status != "ok" || steps[0].Log != "完成" {
		t.Fatalf("步骤不符: %+v", steps)
	}
	if err := db.FinishWGTask(tid, "done", "成功 1", now); err != nil {
		t.Fatal(err)
	}
	if ok, _ := db.HasRunningWGTask(); ok {
		t.Fatal("任务完结后不应有运行中任务")
	}
	tk, _ := db.GetWGTask(tid)
	if tk.Status != "done" || tk.Result != "成功 1" {
		t.Fatalf("任务不符: %+v", tk)
	}
	// hub 流量累加
	day := now / 86400 * 86400
	if err := db.AddWGHubTraffic(hubSrvID, day, 100, 200, now); err != nil {
		t.Fatal(err)
	}
	if err := db.AddWGHubTraffic(hubSrvID, day, 50, 25, now); err != nil {
		t.Fatal(err)
	}
	rx, tx, err := db.WGHubTrafficRange(hubSrvID, day, day)
	if err != nil || rx != 150 || tx != 225 {
		t.Fatalf("流量累加不符: rx=%d tx=%d err=%v", rx, tx, err)
	}
	// 删除 hub 槽位
	_ = did
	if err := db.DeleteWGHub(hubSrvID); err != nil {
		t.Fatal(err)
	}
	if h, _ := db.GetWGHub(hubSrvID); h != nil {
		t.Fatal("hub 槽位应删除")
	}
}
