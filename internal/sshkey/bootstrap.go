// 引导/轮换编排：用一次性凭据建连 → 采集画像 → 生成密钥 → 幂等装公钥 →
// 新私钥重拨验证 → 返回密钥对与画像。调用方（handler）负责落库：
// 验证通过才写 auth_type='key' + private_key_enc 并清空密码字段；
// 任何一步失败返回 err，密码路径原样保留（绝不半切）。
package sshkey

import (
	"context"
	"fmt"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/collector"
	"github.com/Yoahoug/BeaconTower/internal/sshx"
)

// sshxDial 透传建连（独立小包装，便于测试时拦截）。
func sshxDial(ctx context.Context, cred *collector.SSHCred, hostKeyFP string, strict bool) (*sshx.Conn, error) {
	return sshx.Dial(ctx, cred, hostKeyFP, strict)
}

// Budget 各阶段超时。建连+采集沿用试连既有预算；装钥/验证各给独立份额，
// 总和不超过试连接口的 30s 前端超时。
const (
	collectBudget = 20 * time.Second
	installBudget = 6 * time.Second
	verifyBudget  = 10 * time.Second
)

// BootResult 一次成功引导的产物。Raw 画像沿用试连返回结构，前端零改动；
// HostKeyFP 取引导连接的 TOFU 指纹（装机与验证同机同 host key）。
type BootResult struct {
	Pair      *KeyPair
	Raw       *collector.RawSample
	HostKeyFP string
	LatencyMs int64
}

// Bootstrap 用一次性密码凭据完成密钥装机 + 验证 + 画像采集。
// hostKeyFP 为空 = TOFU 首次记录；strict 透传站点设置。
func Bootstrap(ctx context.Context, cred *collector.SSHCred, hostKeyFP string, strict bool, serverID int64) (*BootResult, error) {
	if cred == nil || cred.Password == "" {
		return nil, fmt.Errorf("参数错误：引导需要一次性 SSH 密码")
	}
	// 1) 密码建连 + 采集画像（与普通试连同链路，失败分类口径一致）
	ctxC, cancelC := context.WithTimeout(ctx, collectBudget)
	defer cancelC()
	probe, err := collector.DialAndCollect(ctxC, cred, hostKeyFP, strict)
	if err != nil {
		return nil, fmt.Errorf("密码连接失败：%w", err)
	}
	// 2) 生成节点专属密钥并沿同一条连接装公钥（脚本幂等）
	pair, err := Generate(serverID)
	if err != nil {
		return nil, err
	}
	conn, err := sshxDial(ctx, cred, probe.HostKeyFP, strict)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ctxI, cancelI := context.WithTimeout(ctx, installBudget)
	defer cancelI()
	if _, err := InstallViaConn(ctxI, conn, pair.PublicKey); err != nil {
		return nil, err
	}
	// 3) 新私钥独立重拨验证（安全底线：验证不过不算装好）
	ctxV, cancelV := context.WithTimeout(ctx, verifyBudget)
	defer cancelV()
	res, err := Verify(ctxV, cred.Host, cred.Port, cred.Username, pair.PrivatePEM, probe.HostKeyFP, strict)
	if err != nil {
		return nil, fmt.Errorf("密钥验证失败（密码路径未受影响）：%w", err)
	}
	if !VerifyOK(res) {
		return nil, fmt.Errorf("密钥验证输出异常（密码路径未受影响）")
	}
	return &BootResult{
		Pair:      pair,
		Raw:       probe.Raw,
		HostKeyFP: probe.HostKeyFP,
		LatencyMs: res.LatencyMs,
	}, nil
}

// Repair 密钥失联自愈：节点密钥失效（重装系统/清库误删 authorized_keys 行）后，
// 用用户重输的密码重建密钥。流程与 Bootstrap 完全一致——旧公钥行如仍在
// authorized_keys，注释可辨识后人工清理；新行独立追加，互不影响。
func Repair(ctx context.Context, cred *collector.SSHCred, hostKeyFP string, strict bool, serverID int64) (*BootResult, error) {
	return Bootstrap(ctx, cred, hostKeyFP, strict, serverID)
}
