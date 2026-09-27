// Package assets 资产中转（doc/12 §8）：国内节点难以直连 GitHub 时，
// 面板侧对同一资产的多个源（官方直链 / GitHub 加速镜像 / 用户自有仓库
// release）并发测速，按延迟择优下载、校验 sha256 后缓存到 data/assets/，
// 再经 SSH 推送到节点——节点本身零外网依赖。
package assets

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MirrorPrefixes 内置 GitHub 加速镜像前缀（按常用度排序，测速后动态择优）。
// 镜像用法：<prefix><原始 URL>。失效镜像测速时自然淘汰。
var MirrorPrefixes = []string{
	"https://ghfast.top/",
	"https://gh-proxy.com/",
	"https://ghproxy.net/",
	"https://ghproxy.cc/",
	"https://gh.ddlc.top/",
}

// IsGitHubURL 判断是否 GitHub 资产链接（可生成镜像变体）。
func IsGitHubURL(u string) bool {
	return strings.HasPrefix(u, "https://github.com/") || strings.HasPrefix(u, "http://github.com/")
}

// BuildVariants 生成候选源列表：原 URL 在前 + 各镜像变体（去重）。
func BuildVariants(u string) []string {
	out := []string{u}
	if IsGitHubURL(u) {
		for _, m := range MirrorPrefixes {
			out = append(out, m+u)
		}
	}
	return out
}

// ProbeResult 单源测速结果。
type ProbeResult struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`            // HTTP 状态码；0 = 失败
	LatencyMs int64 `json:"latency_ms"`
	Err      string `json:"err,omitempty"`
}

// Probe 并发测速：对每个源发起 Range GET（取前 1KB），记录首字节延迟。
// 返回按「可用优先 + 延迟升序」排序的结果。
func Probe(ctx context.Context, urls []string, timeout time.Duration) []ProbeResult {
	results := make([]ProbeResult, len(urls))
	var runWG sync.WaitGroup
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	for i, u := range urls {
		runWG.Add(1)
		go func(i int, u string) {
			defer runWG.Done()
			r := ProbeResult{URL: u}
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				r.Err = err.Error()
				results[i] = r
				return
			}
			req.Header.Set("Range", "bytes=0-1023")
			req.Header.Set("User-Agent", "BeaconTower-asset-probe/1.0")
			resp, err := client.Do(req)
			if err != nil {
				r.Err = trimErr(err.Error())
				results[i] = r
				return
			}
			defer resp.Body.Close()
			r.LatencyMs = time.Since(start).Milliseconds()
			r.Status = resp.StatusCode
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
				r.Err = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			results[i] = r
		}(i, u)
	}
	runWG.Wait()
	sort.Slice(results, func(i, j int) bool {
		okI, okJ := results[i].usable(), results[j].usable()
		if okI != okJ {
			return okI
		}
		return results[i].LatencyMs < results[j].LatencyMs
	})
	return results
}

func (p ProbeResult) usable() bool { return p.Status == http.StatusOK || p.Status == http.StatusPartialContent }

func trimErr(s string) string {
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

// Fetch 按测速排序逐源下载（首个成功即止），写入 dest 并校验 sha256
//（expected 为空则跳过校验）。返回实际使用的 URL。
func Fetch(ctx context.Context, results []ProbeResult, dest, expected string, maxSize int64) (string, error) {
	if maxSize <= 0 {
		maxSize = 512 << 20
	}
	client := &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	var lastErr error
	for _, r := range results {
		if !r.usable() {
			continue
		}
		n, err := download(ctx, client, r.URL, dest, maxSize)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", shortURL(r.URL), err)
			continue
		}
		if expected != "" {
			sum, err := fileSHA256(dest)
			if err != nil {
				return r.URL, fmt.Errorf("校验失败: %w", err)
			}
			if !strings.EqualFold(sum, expected) {
				lastErr = fmt.Errorf("%s: sha256 不匹配（得到 %s）", shortURL(r.URL), sum[:16])
				continue
			}
		}
		_ = n
		return r.URL, nil
	}
	if lastErr == nil {
		lastErr = errors.New("无可用下载源")
	}
	return "", lastErr
}

func download(ctx context.Context, client *http.Client, url, dest string, maxSize int64) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "BeaconTower-asset-fetch/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	// 并发下载同一资产时避免共用 .part 临时文件（交错写入会损坏缓存）
	tmp := dest + ".part." + strconv.FormatInt(time.Now().UnixNano(), 36)
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxSize+1))
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return n, err
	}
	if n > maxSize {
		_ = os.Remove(tmp)
		return n, fmt.Errorf("超过大小上限 %d bytes", maxSize)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return n, closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return n, err
	}
	return n, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func shortURL(u string) string {
	if len(u) > 60 {
		return u[:60] + "…"
	}
	return u
}
