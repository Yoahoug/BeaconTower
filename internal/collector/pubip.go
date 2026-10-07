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

// pubIPFailPath 探测失败退避标记（时间戳文件）：网络受限环境下每 10s 空转
// fork 两个进程毫无意义，失败后指数退避（上限 1h），成功即清。
func pubIPFailPath() string { return cacheDir() + "/pub_ip_fail" }

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

// pubIPCacheStale 缓存缺失/过期，且未处于失败退避窗口内。
func pubIPCacheStale() bool {
	if readPubIPCache() != "" {
		return false
	}
	if b, err := os.ReadFile(pubIPFailPath()); err == nil {
		if lastFail, e := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); e == nil {
			// 退避：距上次失败的时间须超过 2^n 分钟（n=失败次数，简化为按上次失败
			// 时间与缓存过期时长推算），上限 1h
			elapsed := time.Now().Unix() - lastFail
			if elapsed < 0 {
				return false
			}
			backoff := int64(2 * time.Minute / time.Second)
			for i := 0; i < 5 && elapsed > backoff; i++ {
				backoff *= 2
			}
			if elapsed < backoff {
				return false
			}
		}
	}
	return true
}

// markPubIPFail 记录探测失败时间戳（退避起点）。
func markPubIPFail() {
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(pubIPFailPath(), []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644)
}

// clearPubIPFail 探测成功后清除退避标记。
func clearPubIPFail() { _ = os.Remove(pubIPFailPath()) }

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

// probePubIP 依次尝试两个源，成功回写 6h 缓存并清退避标记；失败记退避起点。
func probePubIP(ctx context.Context) {
	for _, u := range []string{"https://ip.sb", "https://ipwho.is/"} {
		if ctx.Err() != nil {
			return
		}
		if body, ok := fetchURL(ctx, u); ok {
			if ip := ip4Extract(body); ip != "" {
				writePubIPCache(ip)
				clearPubIPFail()
				return
			}
		}
	}
	markPubIPFail()
}
