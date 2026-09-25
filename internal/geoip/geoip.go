package geoip

import (
	"net"
	"strings"
)

// Result IP 归属解析结果（只到地区级，公开链路只见文案不见 IP）。
type Result struct {
	Country string // HK / JP / CN …
	City    string
	Region  string // 中文展示文案：香港 / 东京 …
}

// 内网段直接标记内网（doc/05 §5）。
func isPrivateIP(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() {
		return true
	}
	if parsed.To4() != nil {
		// RFC1918 + 文档网段
		for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "127.0.0.0/8"} {
			_, n, _ := net.ParseCIDR(cidr)
			if n.Contains(parsed) {
				// 文档网段（TEST-NET）按"演示/未知"处理，由上层映射；此处仅 RFC1918/回环算内网
				if cidr == "10.0.0.0/8" || cidr == "172.16.0.0/12" || cidr == "192.168.0.0/16" || cidr == "127.0.0.0/8" {
					return true
				}
			}
		}
	}
	return false
}

// Lookup 离线解析：内网直返；公网走本地静态映射 + 在线回显服务由上层（collector 经 SSH 回读时）补充。
// 当前为 M1 骨架：提供内网判定与常见地区映射，M2 接入在线回显与 mmdb。
func Lookup(ip string) *Result {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return &Result{Region: "未定位"}
	}
	if isPrivateIP(ip) {
		return &Result{Country: "LAN", Region: "内网"}
	}
	// 演示/文档网段映射（与前端 mock 的 relocate 语义一致，便于联调）
	switch {
	case strings.HasPrefix(ip, "203.0.113."):
		return &Result{Country: "HK", City: "Hong Kong", Region: "香港"}
	case strings.HasPrefix(ip, "198.51.100."):
		return &Result{Country: "JP", City: "Tokyo", Region: "东京"}
	case strings.HasPrefix(ip, "192.0.2."):
		return &Result{Country: "SG", City: "Singapore", Region: "新加坡"}
	}
	return &Result{Region: "未定位"}
}

// RegionText 解析结果转展示文案。
func RegionText(r *Result) string {
	if r == nil {
		return "未定位"
	}
	if r.Region != "" {
		return r.Region
	}
	if r.Country != "" {
		return r.Country
	}
	return "未定位"
}
