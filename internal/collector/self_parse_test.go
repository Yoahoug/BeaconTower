package collector

import "testing"

func TestMeminfoParse(t *testing.T) {
	data := []byte(`MemTotal:       12154788 kB
MemFree:         4567890 kB
MemAvailable:    7890123 kB
Buffers:          123456 kB
SwapTotal:       2097148 kB
SwapFree:        2097148 kB
HugePages_Total:       0
`)
	total, avail, free, swapT, swapF := meminfoParse(data)
	if total != 12154788 || avail != 7890123 || free != 4567890 || swapT != 2097148 || swapF != 2097148 {
		t.Fatalf("got %d %d %d %d %d", total, avail, free, swapT, swapF)
	}
}

func TestApplyMemWiring(t *testing.T) {
	// 服务器实况量级：total 12G、avail 8.2G → used 应为 ~3.3G（28%），而非 8.2G
	s := &RawSample{}
	applyMemWiring(s, 12154788, 8635676, 4567890, 2097148, 2097148)
	if s.MemTotal != 12154788*1024 {
		t.Fatalf("total=%d", s.MemTotal)
	}
	wantUsed := int64(12154788-8635676) * 1024
	if s.MemUsed != wantUsed {
		t.Fatalf("used=%d want %d", s.MemUsed, wantUsed)
	}
	if s.SwapUsed != 0 {
		t.Fatalf("swapUsed=%d", s.SwapUsed)
	}
	// 无 MemAvailable 时回落 MemFree（fixMem 在采集尾部统一结算，此处等价验证）
	s2 := &RawSample{}
	applyMemWiring(s2, 1000, 0, 400, 0, 0)
	s2.fixMem()
	if s2.MemUsed != 600*1024 {
		t.Fatalf("fallback used=%d", s2.MemUsed)
	}
}

func TestNetDevSum(t *testing.T) {
	data := []byte(`Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 12345678    98765    0    0    0     0          0         0 12345678    98765    0    0    0     0       0          0
  eth0: 1000000000  500000    0    0    0     0          0         0 2000000000  400000    0    0    0     0       0          0
eth0.100: 500 10 0 0 0 0 0 0 600 8 0 0 0 0 0 0
`)
	rx, tx := netDevSum(data)
	if rx != 1000000500 || tx != 2000000600 {
		t.Fatalf("rx=%d tx=%d", rx, tx)
	}
}

func TestProcConnCount(t *testing.T) {
	data := []byte("  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1\n" +
		"   1: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12346 1\n")
	if n := procConnCount(data); n != 2 {
		t.Fatalf("n=%d", n)
	}
}

func TestOsReleaseGet(t *testing.T) {
	data := []byte(`PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
ID=ubuntu
`)
	name, id, ver := osReleaseGet(data)
	if name != "Ubuntu" || id != "ubuntu" || ver != "22.04" {
		t.Fatalf("got %q %q %q", name, id, ver)
	}
}

func TestCpuinfoCoresModel(t *testing.T) {
	x86 := []byte(`processor	: 0
model name	: Intel(R) N100
model		: 190
processor	: 1
model name	: Intel(R) N100
model		: 190
`)
	if n := cpuinfoCores(x86); n != 2 {
		t.Fatalf("cores=%d", n)
	}
	if m := cpuinfoModel(x86); m != "Intel(R) N100" {
		t.Fatalf("model=%q", m)
	}
	arm := []byte("model name\t: N/A\nModel\t\t: Raspberry Pi 4\n")
	if m := cpuinfoModel(arm); m != "Raspberry Pi 4" {
		t.Fatalf("arm model=%q", m)
	}
}

func TestMountsFilter(t *testing.T) {
	data := []byte(`/dev/sda1 / ext4 rw,relatime 0 0
proc /proc proc rw 0 0
tmpfs /tmp tmpfs rw 0 0
overlay /app overlay rw 0 0
sysfs /sys sysfs rw 0 0
/dev/sdb1 /data ext4 rw 0 0
cgroup /sys/fs/cgroup cgroup2 rw 0 0
`)
	out := mountsFilter(data)
	if len(out) != 2 || out[0] != "/" || out[1] != "/data" {
		t.Fatalf("out=%v", out)
	}
}

func TestDmiVirt(t *testing.T) {
	cases := map[string]string{
		"QEMU":                   "kvm",
		"Microsoft Corporation":  "hyperv",
		"VMware, Inc.":           "vmware",
		"innotek GmbH":           "vbox",
		"To Be Filled By O.E.M.": "unknown",
	}
	for in, want := range cases {
		if got := dmiVirt(in); got != want {
			t.Errorf("dmiVirt(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIp4Extract(t *testing.T) {
	if ip := ip4Extract(`{"query":"1.2.3.4"}`); ip != "1.2.3.4" {
		t.Fatalf("ip=%q", ip)
	}
	if ip := ip4Extract("117.173.198.51\n"); ip != "117.173.198.51" {
		t.Fatalf("ip=%q", ip)
	}
}

func TestPubIPCacheRoundtrip(t *testing.T) {
	t.Setenv("BEACON_CACHE_DIR", t.TempDir())
	if got := readPubIPCache(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	writePubIPCache("1.2.3.4")
	if got := readPubIPCache(); got != "1.2.3.4" {
		t.Fatalf("got %q", got)
	}
	writePubIPCache("not-an-ip")
	if got := readPubIPCache(); got != "1.2.3.4" {
		t.Fatalf("invalid ip should not overwrite, got %q", got)
	}
}
