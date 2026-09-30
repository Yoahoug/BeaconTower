package wg

import (
	"fmt"
	"strings"
)

// Role 节点在网内角色。
type Role string

const (
	RoleHub     Role = "hub"     // 现役中心（公网 UDP 可达，全量 peer）
	RoleStandby Role = "standby" // 备胎中心（常备全量 peer，待切换）
	RoleSpoke   Role = "spoke"   // 辐条（NAT 后，主动拨出）
)

// Probe 预检探测结果（由引擎的探测脚本产出、parseProbe 解析后交 Judge）。
type Probe struct {
	Err            string // 非空 = SSH/执行失败，整体不可用
	OsID           string // ubuntu / debian / centos / rocky / almalinux / alpine ...
	OsVer          string
	Arch           string
	Kernel         string
	Virt           string   // kvm / lxc / openvz / docker / physical...
	UID0           bool     // 是否 root
	PkgManager     string   // apt / dnf / yum / apk / pacman / ""
	HasWg          bool     // wg 命令存在
	HasWgQuick     bool     // wg-quick 命令存在
	KernelModule   bool     // modinfo wireguard 成功（内核态可用）
	WgIfaces       []string // 现存 wg 接口名（wg show all interfaces）
	UfwActive      bool     // ufw 是否启用
	ListenPortBusy bool     // 目标 UDP 端口是否已被占用（hub/standby 才探测）
	Systemd        bool     // systemctl 可用
	HasFirewall    bool     // iptables 或 nft 存在（wg-quick 建规则要用；极简系统/容器里常缺）
}

// IssueLevel 预检问题级别。
type IssueLevel int

const (
	Warn IssueLevel = 1
	Err  IssueLevel = 2
)

// Issue 单条预检结论。
type Issue struct {
	Level IssueLevel
	Msg   string
}

// Judge 预检判定（纯函数）：根据探测结果与目标角色给出阻断/警告清单。
// 有 Err 级问题则不可继续；Warn 不阻断但需在向导中展示。
// reprovision=true 表示节点已是本网成员的重配（校正备胎/重下发），
// 同名接口由引擎备份后重写，不再视为阻断。
func Judge(p *Probe, role Role, ifaceName string, listenPort int, reprovision bool) []Issue {
	var out []Issue
	if p.Err != "" {
		out = append(out, Issue{Err, "SSH 连接或探测失败：" + p.Err})
		return out
	}
	// 权限
	if !p.UID0 {
		out = append(out, Issue{Err, "SSH 账号不是 root（WG 组网需要 root 权限）"})
	}
	// 系统支持
	pm := strings.ToLower(p.PkgManager)
	switch pm {
	case "apt", "dnf", "yum":
		// v1 支持
	case "apk", "pacman":
		out = append(out, Issue{Warn, "系统 " + p.OsID + "（" + pm + "）暂为实验支持，安装步骤可能失败"})
	default:
		out = append(out, Issue{Err, "暂不支持的发行版：" + p.OsID + "（无 apt/dnf/yum）"})
	}
	// 架构
	switch strings.ToLower(p.Arch) {
	case "x86_64", "aarch64", "":
		// 支持或未知（未知给提示）
		if p.Arch == "" {
			out = append(out, Issue{Warn, "未能探测到 CPU 架构"})
		}
	default:
		out = append(out, Issue{Warn, "未验证的架构：" + p.Arch})
	}
	// 已有 WG 接口
	for _, name := range p.WgIfaces {
		if name == ifaceName {
			if reprovision {
				out = append(out, Issue{Warn, "已存在接口 " + name + "：将按面板配置重写（原配置自动备份）"})
			} else {
				out = append(out, Issue{Err, "已存在接口 " + name + "：节点可能已在其他 WG 网络，勿重复配置"})
			}
		} else {
			out = append(out, Issue{Warn, "存在其他 WG 接口 " + name + "（不受影响，仅提示）"})
		}
	}
	// hub/standby 特有
	if role == RoleHub || role == RoleStandby {
		if p.ListenPortBusy {
			out = append(out, Issue{Err, fmt.Sprintf("UDP 端口 %d 已被占用", listenPort)})
		}
	}
	// 环境提示
	v := strings.ToLower(p.Virt)
	if v == "lxc" || v == "openvz" {
		out = append(out, Issue{Warn, "容器环境（" + v + "）：内核模块可能不可用，需宿主支持 wireguard"})
	}
	if p.UfwActive && (role == RoleHub || role == RoleStandby) {
		out = append(out, Issue{Warn, fmt.Sprintf("ufw 已启用：需放行 UDP %d；云服务器还需到控制台安全组入方向放行 UDP %d（放行 TCP 不等于放行 UDP）", listenPort, listenPort)})
	}
	// wireguard 现状（安装缺失不算 Err，安装步骤会处理）
	if !p.HasWg {
		if !p.KernelModule {
			out = append(out, Issue{Warn, "wireguard 未安装且内核模块不可见：将尝试安装（旧内核可能失败）"})
		} else {
			out = append(out, Issue{Warn, "wireguard-tools 未安装：将自动安装"})
		}
	}
	if !p.HasFirewall {
		// 实测：ubuntu:24.04 极简镜像/容器里只有 wireguard-tools 没有 iptables，
		// wg-quick up 会在建规则时 exit 127（iptables: command not found），接口起不来。
		out = append(out, Issue{Warn, "缺少 iptables/nft：wg-quick 建路由规则会失败，将尝试自动安装"})
	}
	return out
}

// HasErr 是否存在阻断级问题。
func HasErr(issues []Issue) bool {
	for _, i := range issues {
		if i.Level == Err {
			return true
		}
	}
	return false
}
