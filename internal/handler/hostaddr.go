package handler

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// hostSuggestAddr 猜测「面板容器可达的宿主机地址」= 容器默认路由的网关。
// bridge 网络下该地址即宿主机在网桥上的接口地址（192.168.240.1 / 172.17.0.1 等），
// 面板要把本机纳入 WG 组网时，SSH 就该连它——不能填 10.66.66.x，
// 否则翻转成员会将面板自己掐断。
//
// 不做任何硬编码：容器网关由 Docker 分配，实测同一台机器上 docker0 可能处于
// DOWN（172.17.0.1 仍在但不可依赖），只有默认网关是稳定可用的。
// 读不到（非 Linux 或异常）时返回空串，前端退回通用提示。
func hostSuggestAddr() string {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	return parseProcRoute(string(b))
}

// parseProcRoute 从 /proc/net/route 内容里取出默认路由的网关（IPv4 点分十进制）。
// 格式：Iface Destination Gateway Flags ...，Gateway 为该 32 位地址的小端十六进制。
// 例：Gateway=0101A8C0 → 192.168.1.1。
func parseProcRoute(content string) string {
	for _, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] == "Iface" {
			continue
		}
		// Destination 全 0 = 默认路由；Flags 需含 RTF_UP(0x1)（默认路由通常是 0003）
		if f[1] != "00000000" {
			continue
		}
		if flags, err := strconv.ParseUint(f[3], 16, 32); err != nil || flags&0x1 == 0 {
			continue
		}
		v, err := strconv.ParseUint(f[2], 16, 32)
		if err != nil || v == 0 {
			continue
		}
		ip := net.IPv4(byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		if ip.IsUnspecified() {
			continue
		}
		return ip.String()
	}
	return ""
}
