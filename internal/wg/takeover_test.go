package wg

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

func step(seq int64, serverID int64, title string) *store.WGTaskStep {
	return &store.WGTaskStep{Seq: seq, ServerID: sql.NullInt64{Int64: serverID, Valid: serverID > 0},
		Title: title, Status: "pending"}
}

func TestSplitTakeoverSteps(t *testing.T) {
	// 真实建步骤顺序：0 准备、1 实测、2.. 成员、1000 提交
	steps := []*store.WGTaskStep{
		step(0, 9, "准备目标机"),
		step(1, 9, "面板侧实测"),
		step(2, 5, "翻转 · a"),
		step(3, 7, "金丝雀 · c"),
		step(7, 2, "翻转 · 本机"),
		step(TakeoverCommitSeq, 9, "提交"),
	}
	prep, members, commit := splitTakeoverSteps(steps, 7)
	if len(prep) != 2 || prep[0].Seq != 0 || prep[1].Seq != 1 {
		t.Fatalf("准备段应为 seq 0/1，得到 %d 条", len(prep))
	}
	if commit == nil || commit.Seq != TakeoverCommitSeq {
		t.Fatal("提交段应被单独摘出")
	}
	if len(members) != 3 {
		t.Fatalf("成员段应 3 条，得到 %d", len(members))
	}
	// 金丝雀被挪到最前（这一步错了会先翻别的成员再验证，等于没有金丝雀）
	if members[0].ServerID.Int64 != 7 {
		t.Fatalf("金丝雀应排最前，实际 %d", members[0].ServerID.Int64)
	}
	// 本机仍排在最后
	if members[2].ServerID.Int64 != 2 {
		t.Fatalf("本机应仍排最后，实际 %d", members[2].ServerID.Int64)
	}
	// 没指定金丝雀时不重排
	if _, m2, _ := splitTakeoverSteps(steps, 0); m2[0].ServerID.Int64 != 5 {
		t.Fatal("未指定金丝雀不应重排成员")
	}
}

func TestSelfFlipRisk(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "takeover.db"))
	if err != nil {
		t.Fatalf("打开库: %v", err)
	}
	defer db.Close()
	r := NewRunner(db, []byte("0123456789abcdef0123456789abcdef"))
	now := time.Now().Unix()
	netRow := &store.WGNetwork{Subnet: "10.66.66.0/24", HubIP: "10.66.66.2", Iface: "wg0",
		Keepalive: 25, MTU: 1420, CreatedAt: now, UpdatedAt: now}
	if err := db.EnsureWGNetwork(netRow); err != nil {
		t.Fatalf("建网: %v", err)
	}
	selfID, _ := db.CreateServer(&store.Server{Name: "本机", IsSelf: true, Enabled: true, CreatedAt: now})
	peer := &store.WGPeer{Kind: "server", Name: "本机", WgIP: "10.66.66.66",
		ServerID: sql.NullInt64{Int64: selfID, Valid: true}, Status: "online", CreatedAt: now}

	// 凭据走 WG 网段：翻转会把自己掐断 → 必须拦下
	if err := db.UpsertCredential(&store.Credential{ServerID: selfID, Host: "10.66.66.66",
		Port: 22, Username: "root", AuthType: "password"}); err != nil {
		t.Fatalf("写凭据: %v", err)
	}
	if msg := r.selfFlipRisk(peer, netRow); msg == "" || !contains(msg, "WG 网段") {
		t.Fatalf("WG 网段地址应被拦下: %q", msg)
	}

	// 容器可达地址：放行
	if err := db.UpsertCredential(&store.Credential{ServerID: selfID, Host: "172.17.0.1",
		Port: 22, Username: "root", AuthType: "password"}); err != nil {
		t.Fatalf("写凭据: %v", err)
	}
	if msg := r.selfFlipRisk(peer, netRow); msg != "" {
		t.Fatalf("容器可达地址不该被拦: %q", msg)
	}
}

func TestAbortTakeoverMarksStepsSkipped(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "abort.db"))
	if err != nil {
		t.Fatalf("打开库: %v", err)
	}
	defer db.Close()
	r := NewRunner(db, []byte("0123456789abcdef0123456789abcdef"))
	taskID, err := db.InsertWGTask(&store.WGTask{Kind: "takeover", Status: "running",
		Payload: "{}", CreatedAt: time.Now().Unix()})
	if err != nil {
		t.Fatalf("建任务: %v", err)
	}
	st1, _ := db.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: 0, Title: "准备", Status: "ok"})
	st2, _ := db.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: 2, Title: "翻转", Status: "pending"})
	st3, _ := db.InsertWGTaskStep(&store.WGTaskStep{TaskID: taskID, Seq: 1000, Title: "提交", Status: "running"})
	// 中止按 DB 现状筛选：跑过的步骤（ok）必须原样保留状态与日志，
	// pending/running 才标 skipped——中途中止不等于「什么都没发生」
	_ = db.FinishWGTaskStep(st1, "ok", "已迁移中心身份（公钥 abcd1234，端口 51830）并拉起接口", time.Now().Unix())
	r.abortTakeover(taskID, "目标机准备失败，成员未动")

	steps, _ := db.ListWGTaskSteps(taskID)
	for _, st := range steps {
		switch st.ID {
		case st1:
			if st.Status != "ok" {
				t.Fatalf("已完成的步骤不该被改写: %s", st.Status)
			}
			if !contains(st.Log, "已迁移中心身份") {
				t.Fatalf("已完成的步骤日志不该被覆盖: %q", st.Log)
			}
		case st2, st3:
			if st.Status != "skipped" {
				t.Fatalf("未执行的步骤应标 skipped: %s", st.Status)
			}
		}
	}
	task, _ := db.GetWGTask(taskID)
	if task == nil || task.Status != "failed" || !contains(task.Result, "成员未动") {
		t.Fatalf("任务应置失败并带上原因: %+v", task)
	}
}
