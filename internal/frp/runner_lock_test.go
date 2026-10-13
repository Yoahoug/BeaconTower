package frp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// 回归（线上 v2.8.3 事故）：Sync/建改删都是「先持平台操作锁、再调 client()」，
// 而 client() 在 ChmlFrp 令牌过期时要续期。续期曾复用平台操作锁 → sync.Mutex
// 不可重入 → 首轮同步即自锁死，整个 FRP 平台永久卡住（goroutine 栈实测：
// client → lockSlow，9 个请求协程排队在同一把锁上），只能重启进程恢复。
//
// 本用例在同 goroutine 内先持有平台操作锁，再调 client()：
//   - 修复前：卡死在 runner.client 的 lock.Lock()，5s 看门狗判失败；
//   - 修复后：续期走独立的 tokenLockFor，正常完成并落库。
func TestChmlTokenRefreshUnderPlatformLockDoesNotDeadlock(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	master := []byte("beacontower-test-master-key-32b!")
	r := NewRunner(db, master, "test-client")

	// 令牌已过期（tokenFresh 留 90s 余量，这里放到过去一小时）
	encTok := r.encrypt("stale-access-token")
	encRef := r.encrypt("stale-refresh-token")
	if _, err := db.SQL.Exec(`INSERT INTO frp_platform
		(id, kind, name, token_enc, refresh_enc, token_expire_at, uid, username,
		 tunnel_used, tunnel_quota, conns, conns_src, traffic_day_used, traffic_remain,
		 traffic_up, traffic_down, status, last_error, last_sync_at, created_at, updated_at)
		VALUES (1, 'chmlfrp', 'ChmlFrp', ?, ?, ?, 'u1', 'chml-user',
		 1, 4, 3, 'platform', 0, 0, 100, 200, 'ok', NULL, 0, 0, 0)`,
		encTok, encRef, time.Now().Unix()-3600); err != nil {
		t.Fatalf("插入平台行: %v", err)
	}

	// 假令牌端点：返回可用的新令牌
	var refreshHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		refreshHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "fresh-access-token",
			"refresh_token": "fresh-refresh-token",
			"token_type":    "bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()
	origEP := QzhuaTokenEP
	QzhuaTokenEP = srv.URL + "/oauth2/token"
	defer func() { QzhuaTokenEP = origEP }()

	p, err := db.GetFRPPlatform(1)
	if err != nil || p == nil {
		t.Fatalf("读取平台行: %v", err)
	}

	// 模拟 Sync/建改删：先持有平台操作锁，再构造客户端（触发续期）
	opLock := r.lockFor(1)
	opLock.Lock()
	defer opLock.Unlock()

	type result struct {
		cli platform
		err error
	}
	done := make(chan result, 1)
	go func() {
		cli, err := r.client(context.Background(), p)
		done <- result{cli, err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("续期应成功，实际返回错误: %v", got.err)
		}
		if got.cli == nil {
			t.Fatal("client() 未返回云平台客户端")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client() 在持有平台操作锁时自锁死：令牌续期复用了平台操作锁（死锁回归）")
	}

	if refreshHits != 1 {
		t.Fatalf("令牌端点应被调用 1 次，实际 %d 次", refreshHits)
	}
	// 续期结果必须落库（下轮调用/进程重启都靠它）
	cur, err := db.GetFRPPlatform(1)
	if err != nil || cur == nil {
		t.Fatalf("回读平台行: %v", err)
	}
	if !tokenFresh(cur) {
		t.Fatalf("新令牌未落库：expire_at=%d", cur.TokenExpireAt)
	}
	if got := r.decrypt(cur.TokenEnc); got != "fresh-access-token" {
		t.Fatalf("落库的是旧令牌: %q", got)
	}
	if got := r.decrypt(cur.RefreshEnc); got != "fresh-refresh-token" {
		t.Fatalf("刷新令牌未更新: %q", got)
	}

	// 再次构造（仍持有平台操作锁）：库里令牌已新鲜，不应再触发续期，且同样不许卡死
	done2 := make(chan result, 1)
	go func() {
		cli, err := r.client(context.Background(), p)
		done2 <- result{cli, err}
	}()
	select {
	case got := <-done2:
		if got.err != nil {
			t.Fatalf("二次构造应直接复用新鲜令牌，实际错误: %v", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("二次构造 client() 卡死")
	}
	if refreshHits != 1 {
		t.Fatalf("令牌新鲜时不应再次续期，累计调用 %d 次", refreshHits)
	}
}
