package frp

import (
	"strings"
	"testing"
)

func TestMergeFrpcConfigs(t *testing.T) {
	a := `[common]
user = abc

sakura_mode = true
login_fail_exit = false

server_addr = frp-oil.com
server_port = 8088

[ssh]
# id = 1
type = tcp
local_ip = localhost
local_port = 22
remote_port = 29377
`
	b := `[common]
user = abc
sakura_mode = true
server_addr = frp-oil.com
server_port = 8088

[web]
type = tcp
local_ip = 127.0.0.1
local_port = 8091
remote_port = 34282
`
	out, err := MergeFrpcConfigs([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	// [common] 只保留一份，两个隧道段都在
	if n := strings.Count(out, "[common]"); n != 1 {
		t.Fatalf("[common] 应只保留一份，实际 %d: %s", n, out)
	}
	if !strings.Contains(out, "[ssh]") || !strings.Contains(out, "[web]") {
		t.Fatalf("两个隧道段都应保留: %s", out)
	}
	if !strings.Contains(out, "remote_port = 29377") || !strings.Contains(out, "remote_port = 34282") {
		t.Fatalf("隧道参数应完整保留: %s", out)
	}
	if strings.Contains(out, "\n\n\n") {
		t.Fatalf("不应出现连续空行堆积: %q", out)
	}

	// 同名段去重（同一隧道被重复选中）
	out2, err := MergeFrpcConfigs([]string{a, a})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out2, "[ssh]"); n != 1 {
		t.Fatalf("同名段应去重，实际 %d 份", n)
	}

	// 缺 [common] 段 → 报错（不能默默生成一份无效配置）
	if _, err := MergeFrpcConfigs([]string{"[ssh]\ntype = tcp\n"}); err == nil {
		t.Fatal("缺 [common] 段应报错")
	}
}

func TestContainerNameAndPath(t *testing.T) {
	if got := ContainerName("chmlfrp", 7); got != "beacontower-frpc-chmlfrp-7" {
		t.Fatalf("容器名不符合约定: %s", got)
	}
	if !containerRe.MatchString(ContainerName("natfrp", 12)) {
		t.Fatal("生成的容器名应通过白名单校验")
	}
	// 防御：只有面板前缀 + label 的容器才允许被操作
	if containerRe.MatchString("bt-frpc; rm -rf /") {
		t.Fatal("容器名白名单不应放行注入")
	}
	if got := ConfigPath("chmlfrp"); got != "/etc/beacontower-frpc/chmlfrp/frpc.ini" {
		t.Fatalf("配置路径不符合约定: %s", got)
	}
}

func TestDefaultImage(t *testing.T) {
	if DefaultImage("natfrp") != "ghcr.io/yoahoug/beacontower-frpc-natfrp:latest" {
		t.Fatal("NATFRP 默认镜像不对")
	}
	if DefaultImage("chmlfrp") != "ghcr.io/yoahoug/beacontower-frpc-chmlfrp:latest" {
		t.Fatal("ChmlFrp 默认镜像不对")
	}
	if DefaultImage("other") != "" {
		t.Fatal("未知平台不应有默认镜像")
	}
	for _, k := range []string{"natfrp", "chmlfrp"} {
		if !imageRe.MatchString(DefaultImage(k)) {
			t.Fatalf("%s 默认镜像应通过白名单校验", k)
		}
	}
	if imageRe.MatchString("img:latest -v /:/host") {
		t.Fatal("镜像名白名单不应放行空格参数注入")
	}
}

func TestLastLines(t *testing.T) {
	long := strings.Repeat("x", 400) + "\nreal error line\n"
	got := lastLines(long, 1)
	if got != "real error line" {
		t.Fatalf("应取最后一行: %q", got)
	}
	if n := len(lastLines(strings.Repeat("y", 1000), 3)); n > 300 {
		t.Fatalf("错误摘要应截断到 300 字符内，实际 %d", n)
	}
}

func TestNormStatus(t *testing.T) {
	cases := map[string]string{
		"running": "running", "restarting": "running",
		"exited": "stopped", "created": "stopped", "paused": "stopped",
		"missing": "missing", "": "missing",
		"dead": "error", "weird": "weird",
		" running\n": "running",
	}
	for in, want := range cases {
		if got := normStatus(in); got != want {
			t.Errorf("normStatus(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
