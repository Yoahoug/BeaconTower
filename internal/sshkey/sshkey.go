// Package sshkey 面板代管 SSH 密钥：为每个节点生成独立 Ed25519 密钥对，
// 经一次性凭据（密码）建连后幂等安装公钥，再以新私钥重拨验证，验证通过才
// 允许翻转 auth_type='key' 并丢弃密码（doc/05 §2.1 凭据静态加密语义不变）。
// 私钥只在面板内存/库里出现，永不回传浏览器、不入日志、不落临时盘。
package sshkey

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/Yoahoug/BeaconTower/internal/sshx"
)

// KeyPair 面板代管密钥对（PEM 均为 OpenSSH 格式，可直接入 private_key_enc）。
type KeyPair struct {
	PrivatePEM string // -----BEGIN OPENSSH PRIVATE KEY-----
	PublicKey  string // ssh-ed25519 AAAA... comment
	Comment    string
}

// Generate 生成节点专属 Ed25519 密钥对。注释打上面板标识 + 节点 ID + 时间，
// 便于日后在 authorized_keys 里人工辨识/清理（面板库丢失后的救援入口）。
func Generate(serverID int64) (*KeyPair, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成密钥失败: %w", err)
	}
	comment := fmt.Sprintf("beacontower-panel-server-%d-%s", serverID, time.Now().UTC().Format("20060102T150405Z"))
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, fmt.Errorf("编码私钥失败: %w", err)
	}
	privatePEM := pem.EncodeToMemory(block)
	pub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		PrivatePEM: string(privatePEM),
		PublicKey:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
		Comment:    comment,
	}, nil
}

// installScript 幂等安装脚本：mkdir -p ~/.ssh → grep 去重追加公钥 → 收紧权限。
// 公钥由面板生成（base64 字符集 + 固定注释），无注入面。输出 bt_key_ok=1
// 供面板确认脚本执行成功。
func installScript(publicKey string) string {
	// 单引号安全：公钥恒为 base64 字符 + 注释（面板生成，无单引号）
	pk := strings.ReplaceAll(publicKey, "'", "")
	return `umask 077
mkdir -p ~/.ssh
chmod 700 ~/.ssh
touch ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
if ! grep -qF '` + pk + `' ~/.ssh/authorized_keys 2>/dev/null; then
  echo '` + pk + `' >> ~/.ssh/authorized_keys
fi
echo bt_key_ok=1
`
}

// InstallViaConn 在已建立的 SSH 连接里幂等安装公钥。
func InstallViaConn(ctx context.Context, conn *sshx.Conn, publicKey string) (bool, error) {
	out, err := conn.Run(ctx, installScript(publicKey))
	if err != nil {
		return false, fmt.Errorf("公钥安装失败: %w", err)
	}
	if !strings.Contains(out, "bt_key_ok=1") {
		return false, fmt.Errorf("公钥安装脚本未正常收尾（输出不含 bt_key_ok）")
	}
	return true, nil
}

// Verify 用新私钥独立重拨：认证 + 真实执行一条只读命令，证明密钥链路可用。
// 这是切换前的安全底线——验证不过，密码路径原样保留，节点绝不失联。
func Verify(ctx context.Context, host string, port int, username, privatePEM, storedFP string, strict bool) (*sshx.RunResult, error) {
	cred := &sshx.Cred{
		Host: host, Port: port, Username: username,
		AuthType: "key", PrivateKey: privatePEM,
	}
	return sshx.RunScript(ctx, cred, storedFP, strict, "echo bt_verify=$((6*7))")
}

// VerifyOK 判定验证输出。
func VerifyOK(res *sshx.RunResult) bool {
	return res != nil && strings.Contains(res.Out, "bt_verify=42")
}
