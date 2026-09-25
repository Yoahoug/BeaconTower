package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

// ---------- Argon2id 密码哈希（参数见 doc/05 §2.2） ----------

const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64MB
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword 生成 argon2id 编码串，参数内嵌以便日后平滑升级。
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := io.ReadFull(crand.Reader, salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$m=65536,t=3,p=2$%s$%s",
		hex.EncodeToString(salt), hex.EncodeToString(hash)), nil
}

// VerifyPassword 校验密码。
func VerifyPassword(password, encoded string) bool {
	// 手动切分，避免 Sscanf 对 $ 的处理差异
	parts := split3(encoded)
	if len(parts) != 4 || parts[0] != "argon2id" {
		return false
	}
	saltHex, hashHex := parts[2], parts[3]
	salt, err1 := hex.DecodeString(saltHex)
	want, err2 := hex.DecodeString(hashHex)
	if err1 != nil || err2 != nil || len(salt) == 0 || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, uint32(len(want)))
	return subtleEqual(got, want)
}

func split3(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '$' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	out = append(out, cur)
	return out
}

func subtleEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// SubtleEqualFold CSRF 双提交比较（定长比较防时序；token 为随机串，大小写敏感比较）。
func SubtleEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtleEqual([]byte(a), []byte(b))
}

// ---------- 会话 token ----------

// NewSessionToken 生成 32 字节随机 token（hex）。
func NewSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(crand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken 会话库中只存 token 的 SHA-256（doc/05 §2.2）。
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// HashIP 来源 IP 只存 SHA-256（doc/05 §3 审计）。
func HashIP(ip string) string {
	sum := sha256.Sum256([]byte("beacontower-ip:" + ip))
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

// ---------- AES-256-GCM 凭据加密（doc/05 §2.1） ----------

// MasterKey 解析主密钥：优先环境变量 hex；否则生成 data/master.key（0600）。
func MasterKey(dataDir, hexKey string) ([]byte, error) {
	if len(hexKey) > 0 {
		k, err := hex.DecodeString(hexKey)
		if err != nil || len(k) != 32 {
			return nil, errors.New("BEACON_MASTER_KEY 必须为 32 字节 hex（64 个十六进制字符）")
		}
		return k, nil
	}
	path := filepath.Join(dataDir, "master.key")
	if raw, err := os.ReadFile(path); err == nil {
		k, derr := hex.DecodeString(string(raw))
		if derr != nil || len(k) != 32 {
			return nil, errors.New("data/master.key 损坏，无法解析")
		}
		return k, nil
	}
	k := make([]byte, 32)
	if _, err := io.ReadFull(crand.Reader, k); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(k)), 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

// Encrypt 加密，输出 nonce(12B)||ciphertext||tag(16B)。
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt 解密。
func Decrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("密文过短")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// EncryptString 空串返回 nil（与"留空即无值"语义对齐）。
func EncryptString(key []byte, s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return Encrypt(key, []byte(s))
}

// DecryptString nil/空返回 ""。
func DecryptString(key, data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	b, err := Decrypt(key, data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
