package store

import "testing"

// 流量窗口端点查询：FirstTrafficSample 跳过 NULL/0 头部样本取首条有效计数，
// LastTrafficSample 取窗口最后一条（不筛有效性）。性能审查（2026-09）把
// aggregateDailyTraffic 从整窗拉取改为这两条索引端点查询，此处锁行为。
func TestTrafficWindowEdges(t *testing.T) {
	db := newTestDB(t)
	now := int64(1700000000)
	srv, err := db.CreateServer(&Server{Name: "tw", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	from, to := now-3600, now

	// 空窗口：两端点都不可用
	if _, ok := db.FirstTrafficSample(srv, from, to); ok {
		t.Fatal("空窗口 FirstTrafficSample 应返回 false")
	}
	if _, ok := db.LastTrafficSample(srv, from, to); ok {
		t.Fatal("空窗口 LastTrafficSample 应返回 false")
	}

	// 窗口内三条样本：NULL 头部（计数未初始化）→ 有效 → 最后一条
	mustSample := func(ts int64, in, out int64) {
		t.Helper()
		m := &Metric{ServerID: srv, Ts: ts, Status: "online", NetInTotal: in, NetOutTotal: out}
		if err := db.InsertSample(m); err != nil {
			t.Fatalf("insert sample: %v", err)
		}
	}
	mustSample(from, 0, 0)            // NULL/0 头部，应被跳过
	mustSample(from+1200, 1000, 2000) // 首条有效
	mustSample(to, 1300, 2600)        // 窗口末条

	first, ok := db.FirstTrafficSample(srv, from, to)
	if !ok || first.NetInTotal != 1000 || first.NetOutTotal != 2000 {
		t.Fatalf("FirstTrafficSample 应取首条有效计数，got %+v ok=%v", first, ok)
	}
	last, ok := db.LastTrafficSample(srv, from, to)
	if !ok || last.NetInTotal != 1300 || last.NetOutTotal != 2600 {
		t.Fatalf("LastTrafficSample 应取窗口末条，got %+v ok=%v", last, ok)
	}

	// 窗口裁剪：只看前半段时末条是中间样本
	last2, ok := db.LastTrafficSample(srv, from, from+1200)
	if !ok || last2.NetInTotal != 1000 {
		t.Fatalf("子窗口末条应为中间样本，got %+v ok=%v", last2, ok)
	}
}
