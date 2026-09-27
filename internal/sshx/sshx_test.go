package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// fakeSSHServer 进程内假 SSH 服务端：密码认证，exec 请求交给本机 sh 执行
//（sh -s 与 sh -c <cmd> 均真实跑，stdin/stdout/stderr 直通），
// 用于无外网依赖地回归 Run/PushFile/TOFU/错误分类全链路。
type fakeSSHServer struct {
	ln      net.Listener
	hostKey ssh.Signer
	user    string
	pass    string
}

func newFakeSSHServer(t *testing.T) *fakeSSHServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成 host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("签名 host key: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听: %v", err)
	}
	s := &fakeSSHServer{ln: ln, hostKey: signer, user: "testuser", pass: "testpass"}
	go s.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *fakeSSHServer) addr() string { return s.ln.Addr().String() }

func (s *fakeSSHServer) fingerprint() string {
	return FingerprintSHA256(s.hostKey.PublicKey())
}

func (s *fakeSSHServer) serve() {
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == s.user && string(password) == s.pass {
				return nil, nil
			}
			return nil, fmt.Errorf("认证失败")
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
		command := string(payload[4 : 4+n])
		_ = req.Reply(true, nil)
		s.execCommand(channel, command)
		return
	}
}

// execCommand 用本机 sh 真实执行命令：sh -s（stdin 脚本）或 sh -c <cmd>。
func (s *fakeSSHServer) execCommand(channel ssh.Channel, command string) {
	var cmd *exec.Cmd
	if strings.HasPrefix(command, "sh -s") {
		cmd = exec.Command("sh", "-s")
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprint(channel.Stderr(), "exec failed")
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
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = 127
		}
	}
	s.finish(channel, code)
}

func (s *fakeSSHServer) finish(channel ssh.Channel, code int) {
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
	_ = channel.CloseWrite()
}

// dialCred 构造连接假服务器的凭据。
func dialCred(addr string) *Cred {
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return &Cred{Host: host, Port: port, Username: "testuser", AuthType: "password", Password: "testpass"}
}

func TestRunScriptEcho(t *testing.T) {
	srv := newFakeSSHServer(t)
	res, err := RunScript(context.Background(), dialCred(srv.addr()), "", false, "echo hello; echo world")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if got := strings.TrimSpace(res.Out); got != "hello\nworld" {
		t.Fatalf("输出不符: %q", got)
	}
	if res.HostKeyFP != srv.fingerprint() {
		t.Fatalf("指纹不符: %q vs %q", res.HostKeyFP, srv.fingerprint())
	}
}

func TestRunScriptBadPassword(t *testing.T) {
	srv := newFakeSSHServer(t)
	cred := dialCred(srv.addr())
	cred.Password = "wrong"
	_, err := RunScript(context.Background(), cred, "", false, "echo x")
	if err == nil || !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("期望认证失败错误，得到: %v", err)
	}
}

func TestPushFile(t *testing.T) {
	srv := newFakeSSHServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "wg0.conf")
	conn, err := Dial(context.Background(), dialCred(srv.addr()), "", false)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	// 复用连接执行两次操作：验证 Conn 连续可用
	if _, err := conn.Run(context.Background(), "true"); err != nil {
		t.Fatalf("预热 Run: %v", err)
	}
	if err := conn.PushFile(context.Background(), path, []byte("[Interface]\nPrivateKey=abc\n")); err != nil {
		t.Fatalf("PushFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回: %v", err)
	}
	if string(data) != "[Interface]\nPrivateKey=abc\n" {
		t.Fatalf("内容不符: %q", data)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("权限应为 600，得到 %o", perm)
	}
}

func TestPushFileRejectsBadPath(t *testing.T) {
	srv := newFakeSSHServer(t)
	conn, err := Dial(context.Background(), dialCred(srv.addr()), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, p := range []string{"", "relative/path", "/tmp/x'y", "/tmp/a\nb"} {
		if err := conn.PushFile(context.Background(), p, []byte("x")); err == nil {
			t.Fatalf("路径 %q 应被拒绝", p)
		}
	}
}

func TestRemoteErrorNonZeroExit(t *testing.T) {
	srv := newFakeSSHServer(t)
	_, err := RunScript(context.Background(), dialCred(srv.addr()), "", false, "echo oops-tail >&2; exit 3")
	var re *RemoteError
	if err == nil || !errors.As(err, &re) {
		t.Fatalf("期望 RemoteError，得到: %v", err)
	}
	if re.ExitCode != 3 {
		t.Fatalf("退出码应为 3，得到 %d", re.ExitCode)
	}
	if !strings.Contains(re.StderrTail, "oops-tail") {
		t.Fatalf("stderr 尾部应含 oops-tail: %q", re.StderrTail)
	}
}

func TestConnReuseAcrossRuns(t *testing.T) {
	srv := newFakeSSHServer(t)
	conn, err := Dial(context.Background(), dialCred(srv.addr()), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i := 0; i < 3; i++ {
		out, err := conn.Run(context.Background(), fmt.Sprintf("echo n%d", i))
		if err != nil {
			t.Fatalf("第 %d 次 Run: %v", i, err)
		}
		if strings.TrimSpace(out) != fmt.Sprintf("n%d", i) {
			t.Fatalf("第 %d 次输出不符: %q", i, out)
		}
	}
}

func TestStrictHostKeyMismatch(t *testing.T) {
	srv := newFakeSSHServer(t)
	wrongFP := "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, err := RunScript(context.Background(), dialCred(srv.addr()), wrongFP, true, "echo x")
	if err == nil || !strings.Contains(err.Error(), "指纹不匹配") {
		t.Fatalf("严格模式下应拒绝指纹不匹配，得到: %v", err)
	}
}

func TestTOFURecordsFingerprint(t *testing.T) {
	srv := newFakeSSHServer(t)
	res, err := RunScript(context.Background(), dialCred(srv.addr()), "", false, "echo ok")
	if err != nil {
		t.Fatal(err)
	}
	// 再次用记录到的指纹连接（应成功）
	res2, err := RunScript(context.Background(), dialCred(srv.addr()), res.HostKeyFP, false, "echo ok2")
	if err != nil {
		t.Fatalf("TOFU 二连失败: %v", err)
	}
	if strings.TrimSpace(res2.Out) != "ok2" {
		t.Fatalf("输出不符: %q", res2.Out)
	}
}

func TestContextCancelMidRun(t *testing.T) {
	srv := newFakeSSHServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := RunScript(ctx, dialCred(srv.addr()), "", false, "sleep 5")
	if err == nil {
		t.Fatal("ctx 超时后应返回错误")
	}
}
