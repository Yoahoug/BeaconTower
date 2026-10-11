package store

import (
	"strings"
	"testing"
	"time"
)

// 步骤日志：过程行追加、收尾结论接在末尾、中止不覆盖过程。
// 旧实现 FinishWGTaskStep 是整段覆盖——执行期的分阶段日志会被收尾结论吃掉，
// 这就是线上「执行中的步骤没有日志」的结构性成因（doc/17 §2.1）。
func TestWGTaskStepLogAppend(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()
	tid, err := db.InsertWGTask(&WGTask{Kind: "apply", Status: "running", Payload: `{}`, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	// 完成路径：两行过程 + 一行收尾
	sid, err := db.InsertWGTaskStep(&WGTaskStep{TaskID: tid, Seq: 0, Title: "中心节点", Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.StartWGTaskStep(sid, now)
	for _, line := range []string{"[+0ms] 预检通过", "[+120ms] 配置已写入"} {
		if err := db.AppendWGTaskStepLog(sid, line); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.FinishWGTaskStep(sid, "ok", "完成：耗时 1.2s", now); err != nil {
		t.Fatal(err)
	}
	steps, _ := db.ListWGTaskSteps(tid)
	want := "[+0ms] 预检通过\n[+120ms] 配置已写入\n完成：耗时 1.2s"
	if steps[0].Log != want || steps[0].Status != "ok" {
		t.Fatalf("步骤日志不符:\nwant %q\ngot  %q (status=%s)", want, steps[0].Log, steps[0].Status)
	}

	// 中止路径：running 步骤被标 skipped 时，过程日志必须保留
	sid2, err := db.InsertWGTaskStep(&WGTaskStep{TaskID: tid, Seq: 1, Title: "接入 · ops", Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.StartWGTaskStep(sid2, now)
	_ = db.AppendWGTaskStepLog(sid2, "[+0ms] 开始接入")
	if err := db.SkipPendingWGTaskSteps(tid, "未执行（任务中止）", now); err != nil {
		t.Fatal(err)
	}
	steps, _ = db.ListWGTaskSteps(tid)
	if len(steps) != 2 {
		t.Fatalf("应有 2 个步骤: %d", len(steps))
	}
	if !strings.Contains(steps[1].Log, "[+0ms] 开始接入") || !strings.Contains(steps[1].Log, "未执行（任务中止）") {
		t.Fatalf("中止后过程日志被覆盖: %q", steps[1].Log)
	}
	if steps[1].Status != "skipped" {
		t.Fatalf("中止应标 skipped: %s", steps[1].Status)
	}
}
