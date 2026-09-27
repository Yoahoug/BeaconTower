package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildVariants(t *testing.T) {
	gh := "https://github.com/owner/repo/releases/download/v1/x.bin"
	vs := BuildVariants(gh)
	if len(vs) != len(MirrorPrefixes)+1 {
		t.Fatalf("应有 %d 个变体: %d", len(MirrorPrefixes)+1, len(vs))
	}
	if vs[0] != gh {
		t.Fatal("原 URL 应排首位")
	}
	if !strings.HasPrefix(vs[1], "https://") || !strings.Contains(vs[1], gh) {
		t.Fatalf("镜像变体格式不符: %q", vs[1])
	}
	// 非 GitHub 链接不生成变体
	if got := BuildVariants("https://example.com/x.bin"); len(got) != 1 {
		t.Fatalf("非 GitHub 链接不应生成变体: %v", got)
	}
}

func TestProbeOrdersUsableFirst(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow-ok"))
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fast-ok"))
	}))
	defer fast.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer bad.Close()

	res := Probe(context.Background(), []string{slow.URL, bad.URL, fast.URL}, 3*time.Second)
	if len(res) != 3 {
		t.Fatalf("应有 3 个结果")
	}
	if res[0].URL != fast.URL {
		t.Fatalf("最快可用源应排首位: %+v", res)
	}
	if res[len(res)-1].usable() {
		t.Fatal("403 源不应可用")
	}
}

func TestFetchWithSHA256AndFallback(t *testing.T) {
	content := "asset-content-123"
	sum := sha256.Sum256([]byte(content))
	hexSum := hex.EncodeToString(sum[:])

	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer badSrv.Close()

	dest := filepath.Join(t.TempDir(), "asset.bin")
	// 首源不可用 → 降级到第二源
	probe := Probe(context.Background(), []string{badSrv.URL, okSrv.URL}, 2*time.Second)
	via, err := Fetch(context.Background(), probe, dest, hexSum, 1<<20)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if via != okSrv.URL {
		t.Fatalf("应经 okSrv 下载: %s", via)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != content {
		t.Fatalf("内容不符: %q %v", got, err)
	}
	// sha 不匹配应报错
	if _, err := Fetch(context.Background(), probe, filepath.Join(t.TempDir(), "x"), "deadbeef", 1<<20); err == nil {
		t.Fatal("sha 不匹配应报错")
	}
}

func TestFetchRejectsOversize(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer big.Close()
	probe := Probe(context.Background(), []string{big.URL}, 2*time.Second)
	if _, err := Fetch(context.Background(), probe, filepath.Join(t.TempDir(), "big"), "", 1024); err == nil {
		t.Fatal("超限应报错")
	}
}

func TestProbeTimeoutMarksUnavailable(t *testing.T) {
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer hang.Close()
	res := Probe(context.Background(), []string{hang.URL}, 200*time.Millisecond)
	if res[0].usable() || res[0].Err == "" {
		t.Fatalf("超时源应不可用: %+v", res[0])
	}
}

func TestFetchNoUsableSource(t *testing.T) {
	res := []ProbeResult{{URL: "https://x/y", Status: 403, Err: "HTTP 403"}}
	if _, err := Fetch(context.Background(), res, filepath.Join(t.TempDir(), "z"), "", 1024); err == nil || !strings.Contains(err.Error(), "无可用") {
		if err == nil {
			t.Fatal("无可用源应报错")
		}
	}
	var _ = fmt.Sprintf
}
