package collector

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// errNativeUnsupported 平台无原生采集路径。
var errNativeUnsupported = errors.New("native collect unsupported")

// pubIPCachePath 本机公网 IP 缓存（6h）。SSH 节点缓存在目标机 /tmp/.bt_pub_ip；
// 本机节点（原生路径）读这里。
func pubIPCachePath() string { return cacheDir() + "/pub_ip" }

func cacheDir() string {
	if v := os.Getenv("BEACON_CACHE_DIR"); v != "" {
		return v
	}
	return "/tmp/beacontower"
}

// readPubIPCache 读缓存；格式 "<ip> <unix_ts>"，超 6h 或非法返回空。
func readPubIPCache() string {
	b, err := os.ReadFile(pubIPCachePath())
	if err != nil {
		return ""
	}
	f := strings.Fields(strings.TrimSpace(string(b)))
	if len(f) != 2 || net.ParseIP(f[0]) == nil {
		return ""
	}
	ts, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil || ts <= 0 {
		return ""
	}
	if time.Now().Unix()-ts > 6*3600 {
		return ""
	}
	return f[0]
}

// writePubIPCache 探测成功后原子回写缓存。
func writePubIPCache(ip string) {
	if net.ParseIP(ip) == nil {
		return
	}
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	content := ip + " " + strconv.FormatInt(time.Now().Unix(), 10)
	tmp := dir + "/.pub_ip.tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, pubIPCachePath())
}

// pubIPCacheStale 缓存缺失/过期（过期才触发探测）。
func pubIPCacheStale() bool { return readPubIPCache() == "" }

// fetchURL curl 或 busybox wget 抓取（运行镜像内可能只有 wget）。
func fetchURL(ctx context.Context, url string) (string, bool) {
	if p, err := exec.LookPath("curl"); err == nil {
		if out, err := exec.CommandContext(ctx, p, "-sS", "-m", "5", "-4", url).Output(); err == nil {
			return string(out), true
		}
	}
	if p, err := exec.LookPath("wget"); err == nil {
		if out, err := exec.CommandContext(ctx, p, "-qO-", "-T", "5", url).Output(); err == nil {
			return string(out), true
		}
	}
	return "", false
}

// probePubIP 依次尝试两个源，成功即回写 6h 缓存并返回。
func probePubIP(ctx context.Context) {
	for _, u := range []string{"https://ip.sb", "http://ip-api.com/json/?fields=query"} {
		if ctx.Err() != nil {
			return
		}
		if body, ok := fetchURL(ctx, u); ok {
			if ip := ip4Extract(body); ip != "" {
				writePubIPCache(ip)
				return
			}
		}
	}
}
