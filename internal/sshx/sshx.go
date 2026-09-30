// Package sshx 通用 SSH 执行器：从 collector 抽出的建连/执行/传文件通道，
// 供 WG 组网等需要向节点推送与执行脚本的模块复用（doc/12 §3）。
// 约定与采集链路一致：8s 建连超时、host key TOFU/严格模式、stdin→sh -s 免转义、
// 输出 256KB 上限、错误中文分类。
package sshx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// DialTimeout 建连超时（与采集链路一致）。
	DialTimeout = 8 * time.Second
	// MaxOutputLen 单次执行输出上限（与采集链路一致，防恶意输出撑爆内存）。
	MaxOutputLen = 256 * 1024
	// MaxPayloadLen PushFile 单文件上限（WG conf 量级远小于此）。
	MaxPayloadLen = 32 * 1024 * 1024
)

// Cred 解密后的连接凭据（内存态，不落盘、不入日志）。
type Cred struct {
	Host       string
	Port       int
	Username   string
	AuthType   string // password | key
	Password   string
	PrivateKey string
	Passphrase string
}

// FingerprintSHA256 计算 host key 指纹（SHA256:...，TOFU 展示用）。
func FingerprintSHA256(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// authMethods 组装认证方式。
func authMethods(c *Cred) ([]ssh.AuthMethod, error) {
	switch c.AuthType {
	case "password":
		if c.Password == "" {
			return nil, errors.New("认证失败：密码为空")
		}
		return []ssh.AuthMethod{ssh.Password(c.Password)}, nil
	case "key":
		if c.PrivateKey == "" {
			return nil, errors.New("认证失败：私钥为空")
		}
		var signer ssh.Signer
		var err error
		if c.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(c.PrivateKey), []byte(c.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(c.PrivateKey))
		}
		if err != nil {
			return nil, errors.New("认证失败：私钥无法解析")
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("参数错误：未知认证方式 %q", c.AuthType)
	}
}

// ClassifyErr SSH 错误分类（doc/04 错误码 2001 的 msg 分类）。
func ClassifyErr(err error) error {
	if err == nil {
		return nil
	}
	var re *RemoteError
	if errors.As(err, &re) {
		return re
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unable to authenticate"),
		strings.Contains(msg, "Authentication failed"),
		strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "no supported methods"),
		strings.Contains(msg, "认证失败"):
		return fmt.Errorf("认证失败：%s", trimErr(msg))
	case strings.Contains(msg, "connection refused"):
		return errors.New("连接被拒绝：目标端口未监听 SSH")
	case strings.Contains(msg, "no route"),
		strings.Contains(msg, "unreachable"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "不可达"):
		return fmt.Errorf("连接超时：主机不可达（%s 超时）", DialTimeout)
	case strings.Contains(msg, "指纹不匹配"):
		return err
	default:
		if len(msg) > 160 {
			msg = msg[:160]
		}
		return fmt.Errorf("SSH 连接失败：%s", msg)
	}
}

func trimErr(msg string) string {
	// 去掉 Go ssh 库的前缀噪音，保留关键原因
	if strings.Contains(msg, "unable to authenticate") {
		return "用户名或密码/密钥错误"
	}
	if strings.Contains(msg, "permission denied") {
		return "Permission denied（用户名或密码/密钥错误）"
	}
	if len(msg) > 160 {
		return msg[:160]
	}
	return msg
}

// RemoteError 远端命令非零退出（区别于连接层错误：命令已送达并执行）。
type RemoteError struct {
	ExitCode   int
	StderrTail string
}

func (e *RemoteError) Error() string {
	tail := strings.TrimSpace(e.StderrTail)
	if tail == "" {
		return fmt.Sprintf("远端命令失败（退出码 %d）", e.ExitCode)
	}
	if len(tail) > 300 {
		tail = tail[:300]
	}
	return fmt.Sprintf("远端命令失败（退出码 %d）：%s", e.ExitCode, tail)
}

// Conn 已建立的 SSH 连接，可连续多次 Run/PushFile（组网编排避免反复握手）。
type Conn struct {
	client    *ssh.Client
	HostKeyFP string
	LatencyMs int64
}

// HostKey 返回本次连接对端 host key 指纹（TOFU 记录用）。
func (c *Conn) HostKey() string { return c.HostKeyFP }

// Latency 返回建连耗时毫秒。
func (c *Conn) Latency() int64 { return c.LatencyMs }

// Close 关闭连接。
func (c *Conn) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// Dial 建连并校验 host key。storedFP 为空 = TOFU 记录；strict=true 时指纹不匹配直接拒绝。
func Dial(ctx context.Context, cred *Cred, storedFP string, strict bool) (*Conn, error) {
	methods, err := authMethods(cred)
	if err != nil {
		return nil, ClassifyErr(err)
	}
	var presented ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User:    cred.Username,
		Auth:    methods,
		Timeout: DialTimeout,
		HostKeyCallback: func(host string, remote net.Addr, key ssh.PublicKey) error {
			presented = key
			fp := FingerprintSHA256(key)
			if storedFP == "" {
				return nil // TOFU：由上层持久化
			}
			if fp != storedFP {
				if strict {
					return errors.New("host key 指纹不匹配（严格模式，拒绝连接）")
				}
				return nil // 非严格：记录但放行（上层更新指纹并可告警）
			}
			return nil
		},
	}
	addr := net.JoinHostPort(cred.Host, strconv.Itoa(cred.Port))
	start := time.Now()
	// ssh.Dial 不接受 ctx：用 goroutine + ctx 取消兜底
	type dialOut struct {
		client *ssh.Client
		err    error
	}
	dch := make(chan dialOut, 1)
	go func() {
		cl, err := ssh.Dial("tcp", addr, cfg)
		dch <- dialOut{cl, err}
	}()
	var client *ssh.Client
	select {
	case <-ctx.Done():
		// ctx 取消时 dial goroutine 可能已建连成功：异步回收，避免连接泄漏
		go func() {
			if o := <-dch; o.client != nil {
				_ = o.client.Close()
			}
		}()
		return nil, fmt.Errorf("连接超时：主机不可达（%s 超时）", DialTimeout)
	case o := <-dch:
		if o.err != nil {
			return nil, ClassifyErr(o.err)
		}
		client = o.client
	}
	fp := ""
	if presented != nil {
		fp = FingerprintSHA256(presented)
	}
	return &Conn{client: client, HostKeyFP: fp, LatencyMs: time.Since(start).Milliseconds()}, nil
}

// Run 经 stdin 把脚本喂给远端 sh -s 执行，返回 stdout（上限 MaxOutputLen）。
// script 内允许任意引号（转义由传输层而非 shell 解析承担）。
func (c *Conn) Run(ctx context.Context, script string) (string, error) {
	out, err := c.run(ctx, "sh -s", strings.NewReader(script))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// RunOut 执行脚本并分别返回 stdout/stderr（stderr 用于错误定位，上限 8KB）。
func (c *Conn) RunOut(ctx context.Context, script string) (stdout, stderr string, err error) {
	sout, serr, err := c.runBoth(ctx, "sh -s", strings.NewReader(script))
	return string(sout), string(serr), err
}

// PushFile 将 data 写入远端 path（umask 077，权限 600）。path 须为绝对路径且
// 不含单引号/换行（由面板生成，非用户自由输入）。
func (c *Conn) PushFile(ctx context.Context, path string, data []byte) error {
	return c.push(ctx, path, data, false)
}

// AppendFile 将 data 追加到远端 path 末尾（文件不存在则创建，权限 600）。
func (c *Conn) AppendFile(ctx context.Context, path string, data []byte) error {
	return c.push(ctx, path, data, true)
}

func (c *Conn) push(ctx context.Context, path string, data []byte, isAppend bool) error {
	if err := ValidatePath(path); err != nil {
		return err
	}
	if len(data) > MaxPayloadLen {
		return fmt.Errorf("文件过大（%d bytes > %d）", len(data), MaxPayloadLen)
	}
	op := ">"
	if isAppend {
		op = ">>"
	}
	cmd := "umask 077 && cat " + op + " '" + path + "'"
	_, err := c.run(ctx, cmd, bytes.NewReader(data))
	return err
}

// ValidatePath 校验可安全单引号包裹的绝对路径。
func ValidatePath(path string) error {
	if path == "" {
		return errors.New("路径为空")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("路径须为绝对路径：%q", path)
	}
	if len(path) > 512 {
		return errors.New("路径过长")
	}
	for _, r := range path {
		if r == '\'' || r == '\\' || r < 32 || r == 127 {
			return fmt.Errorf("路径含非法字符 %q", r)
		}
	}
	return nil
}

// run 执行单条远端命令（cmd 由面板拼装，stdin 数据经管道喂入），
// 输出上限 MaxOutputLen，非零退出返回 *RemoteError。
func (c *Conn) run(ctx context.Context, cmd string, stdin io.Reader) ([]byte, error) {
	sout, _, err := c.runBoth(ctx, cmd, stdin)
	return sout, err
}

func (c *Conn) runBoth(ctx context.Context, cmd string, stdin io.Reader) (stdout, stderr []byte, err error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return nil, nil, ClassifyErr(err)
	}
	defer sess.Close()

	stdinPipe, err := sess.StdinPipe()
	if err != nil {
		return nil, nil, ClassifyErr(err)
	}
	stdoutPipe, err := sess.StdoutPipe()
	if err != nil {
		return nil, nil, ClassifyErr(err)
	}
	stderrPipe, err := sess.StderrPipe()
	if err != nil {
		return nil, nil, ClassifyErr(err)
	}
	if err := sess.Start(cmd); err != nil {
		return nil, nil, ClassifyErr(err)
	}

	// stdin：写完即关，远端 cat/sh 收到 EOF
	go func() {
		defer stdinPipe.Close()
		if stdin != nil {
			_, _ = io.Copy(stdinPipe, stdin)
		}
	}()
	// stdout：超限即停读（连接仍会关闭，不留悬挂）
	var outBuf, errBuf bytes.Buffer
	copyDone := make(chan struct{}, 2)
	go copyGuarded(&limitedWriter{&outBuf, MaxOutputLen}, stdoutPipe, copyDone)
	go copyGuarded(&tailWriter{&errBuf, 8 * 1024}, stderrPipe, copyDone)

	waitCh := make(chan error, 1)
	go func() { waitCh <- sess.Wait() }()

	select {
	case <-ctx.Done():
		_ = sess.Close() // 关通道使 Wait/拷贝 goroutine 退出
		// Close 触发 Wait 返回依赖连接可写；半开连接下 Close 可能长时间
		// 阻塞，同步等 waitCh 会把上层流程卡死，这里只等一小段。
		select {
		case <-waitCh:
		case <-time.After(2 * time.Second):
		}
		return nil, nil, fmt.Errorf("执行超时或已取消：%w", ctx.Err())
	case werr := <-waitCh:
		<-copyDone
		<-copyDone
		if werr != nil {
			var ee *ssh.ExitError
			if errors.As(werr, &ee) {
				return outBuf.Bytes(), errBuf.Bytes(), &RemoteError{ExitCode: ee.ExitStatus(), StderrTail: errBuf.String()}
			}
			var em *ssh.ExitMissingError
			if errors.As(werr, &em) {
				return outBuf.Bytes(), errBuf.Bytes(), &RemoteError{ExitCode: -1, StderrTail: errBuf.String()}
			}
			return nil, nil, ClassifyErr(werr)
		}
		return outBuf.Bytes(), errBuf.Bytes(), nil
	}
}

// copyGuarded 把远端输出拷进有界缓冲，结束后通知等待方。
// panic 就地 recover：本函数跑在独立 goroutine 里，上层 handler/runner 的
// recover 捕不到同 goroutine 之外的 panic，一旦逃逸整个面板进程直接退出。
func copyGuarded(dst io.Writer, src io.Reader, done chan<- struct{}) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[sshx] 输出拷贝异常（已忽略本轮读侧）: %v", p)
		}
		done <- struct{}{}
	}()
	_, _ = io.Copy(dst, src)
}

// limitedWriter 超限后丢弃后续写入（读侧停工，避免远端大输出撑爆内存）。
type limitedWriter struct {
	w io.Writer
	n int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.w.Write(p)
	l.n -= int64(n)
	return n, err
}

// tailWriter 只保留最后 n 字节（stderr 尾部用于错误定位）。
type tailWriter struct {
	buf *bytes.Buffer
	n   int
}

func (t *tailWriter) Write(p []byte) (int, error) {
	if len(p) >= t.n {
		t.buf.Reset()
		t.buf.Write(p[len(p)-t.n:])
		return len(p), nil
	}
	if t.buf.Len()+len(p) > t.n {
		// 新块比整个上限还大时上面已处理，故 len(p) < t.n。
		// 只需保留「缓冲尾部 keep 字节 + 新块」凑满上限；旧实现写成 b[len(p):]
		// 且不判长度，后一块比缓冲大时切片越界 panic（panic 发生在 io.Copy
		// goroutine 内，recover 捕不到，直接整进程退出）。
		b := t.buf.Bytes()
		keep := t.n - len(p)
		t.buf.Reset()
		t.buf.Write(b[len(b)-keep:])
	}
	t.buf.Write(p)
	return len(p), nil
}

// RunResult 一次性 RunScript 的结果。
type RunResult struct {
	Out       string
	HostKeyFP string
	LatencyMs int64
}

// RunScript 建连 → 执行 → 关闭的一次性封装（试连/巡检等单命令场景）。
func RunScript(ctx context.Context, cred *Cred, storedFP string, strict bool, script string) (*RunResult, error) {
	conn, err := Dial(ctx, cred, storedFP, strict)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out, err := conn.Run(ctx, script)
	if err != nil {
		return nil, err
	}
	return &RunResult{Out: out, HostKeyFP: conn.HostKeyFP, LatencyMs: conn.LatencyMs}, nil
}
