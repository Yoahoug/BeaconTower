package sshkey

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
)

// fakeSSHServer 进程内假 SSH 服务端（与 sshx 测试同思路，但额外支持公钥认证，
// 以回归「装机→验证」全链路；exec 交给本机 sh 真实执行）。
type fakeSSHServer struct {
	ln       net.Listener
	hostKey  ssh.Signer
	user     string
	pass     string
	authKeys string // 内存态 authorized_keys（PublicKeyCallback 逐行比对）
}

func (s *fakeSSHServer) addr() string { return s.ln.Addr().String() }

func (s *fakeSSHServer) fingerprint() string {
	return sshx.FingerprintSHA256(s.hostKey.PublicKey())
}

// newFakeSSHServerWithKeys 建服务端；pubKeys 为初始 authorized_keys（多行拼接）。
// 公钥认证直接比对 pubKeys 文本是否包含 "ssh-ed25519 <base64>" 前缀段。
func newFakeSSHServerWithKeys(t *testing.T, pubKeys string) *fakeSSHServer {
	t.Helper()
	_, priv, err := genEd25519()
	if err != nil {
		t.Fatalf("生成 host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听: %v", err)
	}
	s := &fakeSSHServer{ln: ln, hostKey: signer, user: "testuser", pass: "testpass", authKeys: pubKeys}
	go s.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func genEd25519() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (s *fakeSSHServer) serve() {
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == s.user && string(password) == s.pass {
				return nil, nil
			}
			return nil, fmt.Errorf("认证失败")
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() != s.user {
				return nil, fmt.Errorf("认证失败")
			}
			line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
			// 与真实 authorized_keys 语义对齐：逐行匹配（去除注释后的 key 段）
			for _, l := range strings.Split(s.authKeys, "\n") {
				if strings.TrimSpace(l) == line {
					return nil, nil
				}
			}
			return nil, fmt.Errorf("公钥不在 authorized_keys")
		},
	}
	cfg.AddHostKey(s.hostKey)
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
			if err != nil {
				return
			}
			defer sconn.Close()
			go ssh.DiscardRequests(reqs)
			for ch := range chans {
				if ch.ChannelType() != "session" {
					_ = ch.Reject(ssh.UnknownChannelType, "unsupported")
					continue
				}
				go s.handleSession(ch)
			}
		}()
	}
}

func (s *fakeSSHServer) handleSession(ch ssh.NewChannel) {
	channel, requests, err := ch.Accept()
	if err != nil {
		return
	}
	defer channel.Close()
	for req := range requests {
		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}
		payload := req.Payload
		if len(payload) < 4 {
			_ = req.Reply(false, nil)
			continue
		}
		n := int(uint32(payload[0])<<24 | uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3]))
		if 4+n > len(payload) {
			_ = req.Reply(false, nil)
			continue
		}
		_ = req.Reply(true, nil)
		s.execCommand(channel, string(payload[4:4+n]))
		return
	}
}

func (s *fakeSSHServer) execCommand(channel ssh.Channel, command string) {
	var cmd *exec.Cmd
	if strings.HasPrefix(command, "sh -s") {
		cmd = exec.Command("sh", "-s")
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.finish(channel, 127)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.finish(channel, 127)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.finish(channel, 127)
		return
	}
	if err := cmd.Start(); err != nil {
		s.finish(channel, 127)
		return
	}
	go func() {
		_, _ = io.Copy(stdin, channel)
		_ = stdin.Close()
	}()
	doneOut := make(chan struct{})
	doneErr := make(chan struct{})
	go func() { _, _ = io.Copy(channel, stdout); close(doneOut) }()
	go func() { _, _ = io.Copy(channel.Stderr(), stderr); close(doneErr) }()
	<-doneOut
	<-doneErr
	code := 0
	if err := cmd.Wait(); err != nil {
		code = 127
	}
	s.finish(channel, code)
}

func (s *fakeSSHServer) finish(channel ssh.Channel, code int) {
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
	_ = channel.CloseWrite()
}

// addPubKey 模拟装机成功：往 authorized_keys 追加一行（测试里手动执行装钥脚本，
// 与 Bootstrap 真实路径的脚本执行解耦——这里测编排逻辑，脚本幂等在 TestInstallScriptIdempotent）。
func (s *fakeSSHServer) addPubKey(pub string) {
	if !strings.Contains(s.authKeys, strings.TrimSpace(pub)) {
		s.authKeys += strings.TrimRight(pub, "\n") + "\n"
	}
}

func dialCred(addr string) *sshx.Cred {
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return &sshx.Cred{Host: host, Port: port, Username: "testuser", AuthType: "password", Password: "testpass"}
}

func TestGenerateKeyPairFormat(t *testing.T) {
	pair, err := Generate(42)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(pair.PrivatePEM, "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Fatalf("私钥非 OpenSSH PEM: %q", pair.PrivatePEM[:40])
	}
	if !strings.HasPrefix(pair.PublicKey, "ssh-ed25519 ") {
		t.Fatalf("公钥格式异常: %q", pair.PublicKey)
	}
	if !strings.Contains(pair.Comment, "beacontower-panel-server-42-") {
		t.Fatalf("注释缺节点标识: %q", pair.Comment)
	}
	// 私钥可被 sshx 正常解析（authMethods 兼容性）
	signer, err := ssh.ParsePrivateKey([]byte(pair.PrivatePEM))
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	if pubLine != pair.PublicKey {
		t.Fatalf("公私钥不匹配: %q vs %q", pubLine, pair.PublicKey)
	}
}

// TestInstallScriptIdempotent 真实执行装钥脚本两遍：第一遍追加，第二遍不重复。
func TestInstallScriptIdempotent(t *testing.T) {
	home := t.TempDir()
	// sh 环境的 HOME 覆盖：脚本用 ~ 展开，依赖 HOME 环境变量（sh 真实语义）
	script := installScript("ssh-ed25519 AAAATEST test-comment")
	for i := 0; i < 2; i++ {
		cmd := exec.Command("sh", "-s")
		cmd.Env = append(os.Environ(), "HOME="+home)
		cmd.Stdin = strings.NewReader(script)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("脚本执行 #%d: %v\n%s", i, err, out)
		}
		if !strings.Contains(string(out), "bt_key_ok=1") {
			t.Fatalf("缺 ok 标记: %s", out)
		}
	}
	raw, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("读 authorized_keys: %v", err)
	}
	if got := strings.Count(string(raw), "AAAATEST"); got != 1 {
		t.Fatalf("幂等失败：出现 %d 次，应为 1 次\n%s", got, raw)
	}
	// 权限 600（umask 077 生效）。Windows 侧 Git Bash 的 sh 映射到 exe 的
	// 权限位与 POSIX 不同，权限断言只在真实 POSIX 语义下校验。
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(home, ".ssh", "authorized_keys"))
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("authorized_keys 权限 %v，期望 600", st.Mode().Perm())
		}
	}
}

// TestBootstrapEndToEnd 密码建连 → 装钥 → 新私钥验证 全链路。
// 装钥脚本在假服务器侧真实执行于本进程 sh，但 authorized_keys 是服务端内存态——
// 因此把脚本的文件操作映射到内存：通过 InstallViaConn 的输出来登记公钥。
// 为此用可变 authKeys 的服务端（addPubKey 由脚本输出的公钥行驱动）。
func TestBootstrapEndToEnd(t *testing.T) {
	srv := newFakeSSHServerWithKeys(t, "")
	cred := dialCred(srv.addr())
	pair, err := Generate(7)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// 1) 密码连接装钥：脚本真实执行会写本机 HOME 的 authorized_keys（无副作用语义不变），
	//    服务端公钥库同步登记，模拟节点 sshd 真实读到新公钥
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := sshx.Dial(ctx, cred, "", false)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if _, err := InstallViaConn(ctx, conn, pair.PublicKey); err != nil {
		t.Fatalf("InstallViaConn: %v", err)
	}
	srv.addPubKey(pair.PublicKey)
	// 2) 新私钥验证
	res, err := Verify(ctx, cred.Host, cred.Port, cred.Username, pair.PrivatePEM, srv.fingerprint(), false)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !VerifyOK(res) {
		t.Fatalf("验证输出异常: %q", res.Out)
	}
}

// TestBootstrapRejectsWhenKeyNotInstalled 公钥未装时 Verify 必须失败（安全底线）。
func TestBootstrapRejectsWhenKeyNotInstalled(t *testing.T) {
	srv := newFakeSSHServerWithKeys(t, "")
	cred := dialCred(srv.addr())
	pair, err := Generate(9)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := Verify(ctx, cred.Host, cred.Port, cred.Username, pair.PrivatePEM, srv.fingerprint(), false); err == nil {
		t.Fatalf("公钥未装，验证不应通过")
	}
}
