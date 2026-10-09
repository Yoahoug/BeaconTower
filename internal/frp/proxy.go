// 出站代理：服务器直连外网被墙/被 TUN 接管建连不稳时，平台外呼与 GeoIP
// 在线回显统一走 BEACON_PROXY_URL 指定的 HTTP 代理（运维 override 注入，
// 如 http://172.17.0.1:7890 指向宿主 mihomo）。变量为空 = 全部直连。
//
// 在 newHTTPClient 阶段注入而非全局 transport 默认值，避免波及 SSH 隧道、
// 面板自身 HTTP 服务等不该走代理的链路。
package frp

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

var (
	proxyOnce    sync.Once
	proxyBaseURL *url.URL // nil = 未配置，直连
)

// outboundProxy 解析并缓存 BEACON_PROXY_URL（首次调用时一次性生效）。
func outboundProxy() *url.URL {
	proxyOnce.Do(func() {
		raw := os.Getenv("BEACON_PROXY_URL")
		if raw == "" {
			return
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			log.Printf("[frp] BEACON_PROXY_URL=%q 不是合法代理地址，忽略并直连", raw)
			return
		}
		proxyBaseURL = u
		log.Printf("[frp] 出站代理已启用: %s", u.String())
	})
	return proxyBaseURL
}

// newProxiedHTTPClient 统一客户端工厂：超时 + TLS 下限 + 显式代理（未配置
// 时走直连）。所有平台客户端与 OAuth 端点都从这里拿客户端。
func newProxiedHTTPClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if u := outboundProxy(); u != nil {
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}
