package collector

import (
	"testing"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// TestDiffMetricsBaselineWriteBack 回归：applySample 曾漏掉差分基线写回
// （p.ts/p.cpuTotal/p.netRx 从不赋值），导致 CPU 使用率与网络速率恒为 0——
// 线上表现为「服务器的资源信息读取不到」（内存/磁盘/负载正常，CPU 与网速全 0）。
func TestDiffMetricsBaselineWriteBack(t *testing.T) {
	c := New(nil, nil, nil)
	srv := &store.Server{ID: 7, Name: "t"}
	p := &prevState{rapl: map[string]uint64{}}

	// 首轮：无基线，只建基线不产出数值
	first := &RawSample{CpuTotal: 1000, CpuIdle: 400, NetRx: 5000, NetTx: 6000}
	m1 := &store.Metric{}
	c.diffMetrics(p, first, srv, 1000, m1)
	if p.ts != 1000 || p.cpuTotal != 1000 || p.cpuIdle != 400 || p.netRx != 5000 || p.netTx != 6000 {
		t.Fatalf("首轮基线未写回: %+v", p)
	}
	if m1.CpuPct != 0 || m1.NetInBps != 0 || m1.NetOutBps != 0 {
		t.Fatalf("首轮不应产出数值: %+v", m1)
	}

	// 第二轮（+10s）：CPU 增量 1000 tick、其中 idle 600 → 使用率 40%；
	// 网络增量 20000/30000 字节 / 10s = 2000/3000 字节每秒
	second := &RawSample{CpuTotal: 2000, CpuIdle: 1000, NetRx: 25000, NetTx: 36000}
	m2 := &store.Metric{}
	c.diffMetrics(p, second, srv, 1010, m2)
	if m2.CpuPct != 40 {
		t.Fatalf("CPU 使用率应为 40%%，得到 %v", m2.CpuPct)
	}
	if m2.NetInBps != 2000 || m2.NetOutBps != 3000 {
		t.Fatalf("网速应为 2000/3000 字节每秒，得到 %v/%v", m2.NetInBps, m2.NetOutBps)
	}
	if p.ts != 1010 || p.cpuTotal != 2000 || p.netRx != 25000 {
		t.Fatalf("第二轮基线未推进: %+v", p)
	}
}

// TestDiffMetricsSkipsStaleWindow 间隔超限（长时间离线/进程暂停）时只重建基线，
// 不产出把长时间平均值当瞬时值的假曲线。
func TestDiffMetricsSkipsStaleWindow(t *testing.T) {
	c := New(nil, nil, nil)
	srv := &store.Server{ID: 1}
	p := &prevState{rapl: map[string]uint64{}, ts: 1000, cpuTotal: 1000, cpuIdle: 400, netRx: 1000, netTx: 1000}
	m := &store.Metric{}
	// 间隔 1 小时 > maxDeltaSec()
	c.diffMetrics(p, &RawSample{CpuTotal: 9_000_000, CpuIdle: 1000, NetRx: 1 << 30, NetTx: 1 << 30}, srv, 4600, m)
	if m.NetInBps != 0 || m.NetOutBps != 0 {
		t.Fatalf("超限间隔不应产出网速: %+v", m)
	}
	if p.ts != 4600 {
		t.Fatalf("基线仍应更新: %+v", p)
	}
}

// TestDiffMetricsCounterReset 计数器回绕/重启归零时跳过本轮网速。
func TestDiffMetricsCounterReset(t *testing.T) {
	c := New(nil, nil, nil)
	srv := &store.Server{ID: 1}
	p := &prevState{rapl: map[string]uint64{}, ts: 1000, cpuTotal: 2000, cpuIdle: 1000, netRx: 9000, netTx: 9000}
	m := &store.Metric{}
	c.diffMetrics(p, &RawSample{CpuTotal: 100, CpuIdle: 50, NetRx: 10, NetTx: 10}, srv, 1010, m)
	if m.NetInBps != 0 || m.NetOutBps != 0 || m.CpuPct != 0 {
		t.Fatalf("计数器归零后不应产出数值: %+v", m)
	}
	if p.netRx != 10 || p.cpuTotal != 100 {
		t.Fatalf("基线应被重置为新值: %+v", p)
	}
}
