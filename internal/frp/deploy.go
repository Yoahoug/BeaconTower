// 节点侧穿透客户端托管（doc/13 §14）：面板经 SSH 在任意节点上拉取 frpc 镜像、
// 写配置、起容器，并在隧道变化后同步配置、重启/停止/删除。
//
// 安全边界：
//   - 只操作固定前缀 + 固定 label 的容器（beacontower.managed=frpc），绝不碰用户的其它容器；
//   - 配置含平台令牌，节点上落盘 0600，经 SSH stdin 推送（不出现在命令行/进程表）；
//   - 容器名/镜像/路径全部由面板生成，节点侧不解析用户输入。
package frp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/sshx"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

const (
	// 节点侧配置目录（root 专用，0600）。与 WG 的 /etc/wireguard 同风格。
	deployRootDir = "/etc/beacontower-frpc"
	// 容器名前缀；只管理这个前缀 + managed label 的容器。
	containerPrefix = "beacontower-frpc-"
	// 容器内配置文件挂载点（两个客户端镜像的 entrypoint 默认都读这里）。
	containerConfPath = "/etc/frp/frpc.ini"
	managedLabel      = "beacontower.managed=frpc"
)

var (
	containerRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,62}$`)
	imageRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,199}$`)
)

// DefaultImage 各平台默认客户端镜像（与 deploy/frpc-* 发布的一致；
// cloudflared 用官方镜像，无需自建）。
func DefaultImage(kind string) string {
	switch kind {
	case "natfrp":
		return "ghcr.io/yoahoug/beacontower-frpc-natfrp:latest"
	case "chmlfrp":
		return "ghcr.io/yoahoug/beacontower-frpc-chmlfrp:latest"
	case KindCloudflared:
		return "docker.io/cloudflare/cloudflared:latest"
	}
	return ""
}

// ContainerName 固定命名：一个 (平台, 节点) 一个容器。
func ContainerName(kind string, serverID int64) string {
	return fmt.Sprintf("%s%s-%d", containerPrefix, kind, serverID)
}

// ConfigPath 节点侧配置文件路径。cloudflared 无配置文件（token 作启动参数），
// 返回占位路径仅用于目录归置，实际不落盘任何文件。
func ConfigPath(kind string) string {
	if kind == KindCloudflared {
		return deployRootDir + "/" + kind + "/README.txt"
	}
	return deployRootDir + "/" + kind + "/frpc.ini"
}

// Deployer 节点侧客户端托管执行器。
type Deployer struct {
	DB     *store.DB
	Master []byte
	Runner *Runner
}

func NewDeployer(db *store.DB, master []byte, r *Runner) *Deployer {
	d := &Deployer{DB: db, Master: master, Runner: r}
	// Runner 需要在非托管节点上数 socket（本地连接计数）时借用本执行器的
	// SSH 通道；Deployer 自身不持有 Runner 之外的引用，无环。
	if r != nil {
		r.DialForProbe = d.dial
	}
	return d
}

// ImageFor 读取镜像（设置里可覆盖，缺省用 DefaultImage）。
func (d *Deployer) ImageFor(kind string) string {
	def := DefaultImage(kind)
	s, err := d.DB.GetSettings()
	if err != nil || s == nil {
		return def
	}
	v := strings.TrimSpace(s["frp_image_"+kind])
	if v == "" {
		return def
	}
	if !imageRe.MatchString(v) {
		return def
	}
	return v
}

// ---------- SSH ----------

func (d *Deployer) credFor(serverID int64) (*sshx.Cred, string, error) {
	cred, err := d.DB.GetCredential(serverID)
	if err != nil {
		return nil, "", err
	}
	if cred == nil {
		return nil, "", errors.New("节点无 SSH 凭据，请先在节点管理中录入")
	}
	if cred.Host == "local" || cred.Port <= 0 {
		return nil, "", errors.New("本机节点暂不支持 SSH 托管（可改用容器外部手动部署）")
	}
	c := &sshx.Cred{Host: cred.Host, Port: cred.Port, Username: cred.Username, AuthType: cred.AuthType}
	switch cred.AuthType {
	case "password":
		pw, err := crypto.DecryptString(d.Master, cred.PasswordEnc)
		if err != nil {
			return nil, "", errors.New("凭据解密失败")
		}
		c.Password = pw
	case "key":
		key, err := crypto.DecryptString(d.Master, cred.PrivateKeyEnc)
		if err != nil {
			return nil, "", errors.New("凭据解密失败")
		}
		c.PrivateKey = key
		if len(cred.PassphraseEnc) > 0 {
			if pp, err := crypto.DecryptString(d.Master, cred.PassphraseEnc); err == nil {
				c.Passphrase = pp
			}
		}
	default:
		return nil, "", fmt.Errorf("未知认证方式 %q", cred.AuthType)
	}
	return c, cred.HostKeyFP, nil
}

func (d *Deployer) strictHostKey() bool {
	s, err := d.DB.GetSettings()
	if err != nil {
		return false
	}
	return s["strict_host_key"] == "true"
}

func (d *Deployer) dial(ctx context.Context, serverID int64) (*sshx.Conn, func(), error) {
	cred, fp, err := d.credFor(serverID)
	if err != nil {
		return nil, nil, err
	}
	conn, err := sshx.Dial(ctx, cred, fp, d.strictHostKey())
	if err != nil {
		return nil, nil, err
	}
	if conn.HostKey() != "" && conn.HostKey() != fp && !d.strictHostKey() {
		_ = d.DB.SetCredentialFP(serverID, conn.HostKey())
	}
	return conn, func() { _ = conn.Close() }, nil
}

// ---------- Docker ----------

// DockerStatus 节点 Docker 现状。
type DockerStatus struct {
	Present  bool   `json:"present"`
	Version  string `json:"version"`
	DaemonOK bool   `json:"daemon_ok"`
	Err      string `json:"err,omitempty"`
}

// DockerCheck 探测节点 Docker（连得上面板 SSH 才有结果）。
func (d *Deployer) DockerCheck(ctx context.Context, serverID int64) (*DockerStatus, error) {
	conn, closer, err := d.dial(ctx, serverID)
	if err != nil {
		return nil, err
	}
	defer closer()
	return dockerCheck(ctx, conn), nil
}

func dockerCheck(ctx context.Context, conn *sshx.Conn) *DockerStatus {
	st := &DockerStatus{}
	out, err := conn.Run(ctx, `command -v docker >/dev/null 2>&1 && docker --version 2>/dev/null || echo missing`)
	if err != nil {
		st.Err = err.Error()
		return st
	}
	v := strings.TrimSpace(out)
	if v == "" || v == "missing" {
		return st
	}
	st.Present = true
	st.Version = v
	// daemon 是否在跑（无权限/未启动都会失败）
	if _, err := conn.Run(ctx, `docker info >/dev/null 2>&1`); err != nil {
		st.Err = "Docker 已安装但守护进程不可用（未启动或无权限）"
		return st
	}
	st.DaemonOK = true
	return st
}

// InstallDocker 在节点上安装 Docker 并启动（apt/dnf/yum/apk/pacman）。
// 冷机器上 apt-get update + 安装可能要几分钟，调用方给足超时。
func (d *Deployer) InstallDocker(ctx context.Context, serverID int64) (*DockerStatus, error) {
	conn, closer, err := d.dial(ctx, serverID)
	if err != nil {
		return nil, err
	}
	defer closer()
	if st := dockerCheck(ctx, conn); st.Present && st.DaemonOK {
		return st, nil
	}
	pkg, err := conn.Run(ctx, `if command -v apt-get >/dev/null 2>&1; then echo apt;
elif command -v dnf >/dev/null 2>&1; then echo dnf;
elif command -v yum >/dev/null 2>&1; then echo yum;
elif command -v apk >/dev/null 2>&1; then echo apk;
elif command -v pacman >/dev/null 2>&1; then echo pacman; else echo unsupported; fi`)
	if err != nil {
		return nil, err
	}
	switch strings.TrimSpace(pkg) {
	case "apt":
		if _, err := conn.Run(ctx, `export DEBIAN_FRONTEND=noninteractive
apt-get update -qq >/dev/null 2>&1 || true
apt-get install -y -qq docker.io >/dev/null 2>&1 || apt-get install -y docker.io
systemctl enable --now docker 2>/dev/null || service docker start 2>/dev/null || true`); err != nil {
			return nil, fmt.Errorf("安装 Docker 失败: %w", err)
		}
	case "dnf", "yum":
		if _, err := conn.Run(ctx, pkg+` install -y docker
systemctl enable --now docker 2>/dev/null || true`); err != nil {
			return nil, fmt.Errorf("安装 Docker 失败: %w", err)
		}
	case "apk":
		if _, err := conn.Run(ctx, `apk add --no-cache docker >/dev/null 2>&1 || apk add docker
rc-update add docker default 2>/dev/null || true
service docker start 2>/dev/null || true`); err != nil {
			return nil, fmt.Errorf("安装 Docker 失败: %w", err)
		}
	case "pacman":
		if _, err := conn.Run(ctx, `pacman -Sy --noconfirm docker
systemctl enable --now docker 2>/dev/null || true`); err != nil {
			return nil, fmt.Errorf("安装 Docker 失败: %w", err)
		}
	default:
		return nil, errors.New("无法识别包管理器，请手动安装 Docker 后重试")
	}
	// daemon 起来需要一点时间：轮询等待
	deadline := time.Now().Add(20 * time.Second)
	for {
		st := dockerCheck(ctx, conn)
		if st.Present && st.DaemonOK {
			return st, nil
		}
		if time.Now().After(deadline) {
			if st.Err == "" {
				st.Err = "安装完成但 Docker 守护进程未能启动"
			}
			return st, errors.New(st.Err)
		}
		time.Sleep(2 * time.Second)
	}
}

// ---------- 配置 ----------

// MergeFrpcConfigs 把逐条隧道的 frpc 配置合成为一份：保留第一个 [common] 段，
// 其余隧道段按入参顺序拼接（同名段去重）。两平台的取配置接口都返回「[common] + 单隧道段」。
func MergeFrpcConfigs(cfgs []string) (string, error) {
	var common string
	var sections []string
	seen := map[string]bool{}
	for _, cfg := range cfgs {
		cur := ""
		var buf []string
		flush := func() {
			if cur == "" {
				return
			}
			body := strings.TrimRight(strings.Join(buf, "\n"), "\n")
			if cur == "common" {
				if common == "" {
					common = body
				}
				return
			}
			if seen[cur] {
				return
			}
			seen[cur] = true
			sections = append(sections, body)
		}
		for _, line := range strings.Split(cfg, "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
				flush()
				cur = strings.ToLower(strings.Trim(t, "[]"))
				buf = []string{t}
				continue
			}
			if cur != "" {
				buf = append(buf, line)
			}
		}
		flush()
	}
	if strings.TrimSpace(common) == "" {
		return "", errors.New("平台返回的配置缺少 [common] 段")
	}
	out := common
	if len(sections) > 0 {
		out += "\n\n" + strings.Join(sections, "\n\n")
	}
	return out + "\n", nil
}

// RenderConfig 拉取并合成该托管所辖隧道的客户端配置。
func (d *Deployer) RenderConfig(ctx context.Context, platform *store.FRPPlatform, tunnels []*store.FRPTunnel) (string, error) {
	if len(tunnels) == 0 {
		return "", errors.New("该托管未包含任何隧道")
	}
	cfgs := make([]string, 0, len(tunnels))
	for _, t := range tunnels {
		cfg, err := d.Runner.TunnelConfig(ctx, platform.ID, ConfigTarget{
			RemoteID: t.RemoteID, NodeName: t.NodeName, Name: t.Name,
		})
		if err != nil {
			return "", fmt.Errorf("取隧道「%s」配置失败: %w", t.Name, err)
		}
		if strings.TrimSpace(cfg) == "" {
			return "", fmt.Errorf("隧道「%s」的平台配置为空", t.Name)
		}
		cfgs = append(cfgs, cfg)
	}
	return MergeFrpcConfigs(cfgs)
}

// ---------- 容器生命周期 ----------

// ApplyResult 一次部署/同步的执行结果。
type ApplyResult struct {
	Status    string // running/stopped/missing/error
	DockerVer string
	LogTail   string
}

// Sync 全量同步：拉配置 → 写盘（0600）→ 拉镜像 → 重建容器 → 校验运行。
// cloudflared 平台无 INI 配置：改为取 tunnel token 作为容器启动参数（见 syncCloudflared）。
func (d *Deployer) Sync(ctx context.Context, dep *store.FRPDeploy, platform *store.FRPPlatform, tunnels []*store.FRPTunnel) (*ApplyResult, error) {
	if platform.Kind == KindCloudflared {
		return d.syncCloudflared(ctx, dep, platform, tunnels)
	}
	conf, err := d.RenderConfig(ctx, platform, tunnels)
	if err != nil {
		return nil, err
	}
	conn, closer, err := d.dial(ctx, dep.ServerID)
	if err != nil {
		return nil, err
	}
	defer closer()
	st := dockerCheck(ctx, conn)
	if !st.Present {
		return nil, errors.New("节点未安装 Docker（可让面板自动安装后重试）")
	}
	if !st.DaemonOK {
		return nil, errors.New("节点 Docker 守护进程不可用：" + st.Err)
	}
	if !containerRe.MatchString(dep.Container) {
		return nil, fmt.Errorf("容器名非法: %q", dep.Container)
	}
	if !imageRe.MatchString(dep.Image) {
		return nil, fmt.Errorf("镜像名非法: %q", dep.Image)
	}
	if err := sshx.ValidatePath(dep.ConfigPath); err != nil {
		return nil, fmt.Errorf("配置路径非法: %w", err)
	}
	// 1) 写配置：临时文件 → 0600 → 原子替换（配置含平台令牌，不能落到 0644）
	dir := strings.TrimSuffix(dep.ConfigPath, "/frpc.ini")
	if _, err := conn.Run(ctx, "mkdir -p "+dir+" && chmod 700 "+dir); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := conn.PushFile(ctx, dep.ConfigPath+".tmp", []byte(conf)); err != nil {
		return nil, fmt.Errorf("推送配置失败: %w", err)
	}
	if _, err := conn.Run(ctx, "chmod 600 "+dep.ConfigPath+".tmp && mv -f "+dep.ConfigPath+".tmp "+dep.ConfigPath); err != nil {
		return nil, fmt.Errorf("落盘配置失败: %w", err)
	}
	// 2) 拉镜像（本地已有则跳过）
	if _, err := conn.Run(ctx, "docker image inspect "+dep.Image+" >/dev/null 2>&1 || docker pull "+dep.Image); err != nil {
		return nil, fmt.Errorf("拉取镜像 %s 失败（节点需能访问 GHCR）: %w", dep.Image, err)
	}
	// 3) 重建容器（host 网络：客户端要访问本机服务；restart 策略保证开机自启）
	run := "docker rm -f " + dep.Container + " >/dev/null 2>&1 || true\n" +
		"docker run -d --name " + dep.Container + " --restart unless-stopped --network host" +
		" --label " + managedLabel + " --label beacontower.kind=" + platform.Kind +
		" --label beacontower.deploy=" + strconv.FormatInt(dep.ID, 10) +
		" -v " + dep.ConfigPath + ":" + containerConfPath + ":ro " + dep.Image
	if _, err := conn.Run(ctx, run); err != nil {
		return nil, fmt.Errorf("启动容器失败: %w", err)
	}
	// 4) 校验：等 2s 看容器是否还在跑，并回读日志尾部（失败时给出可读原因）
	time.Sleep(2 * time.Second)
	res := &ApplyResult{DockerVer: st.Version}
	state, _ := conn.Run(ctx, `docker inspect -f '{{.State.Status}}' `+dep.Container+` 2>/dev/null || echo missing`)
	res.Status = normStatus(state)
	tail, _ := conn.Run(ctx, "docker logs --tail 30 "+dep.Container+" 2>&1 || true")
	res.LogTail = strings.TrimSpace(tail)
	if res.Status != "running" {
		return res, fmt.Errorf("容器未处于运行状态（%s）：%s", res.Status, lastLines(res.LogTail, 3))
	}
	return res, nil
}

// syncCloudflared cloudflared 托管：取统一容器隧道 token → 拉官方镜像 →
// 以 token 作启动参数起容器。与 frpc 托管的差异：
//   - 无配置文件：token 经 docker run 参数传入（--env-file 方式会在节点上落盘），
//     面板与 SSH 通道之外不留任何含凭据的文件；token 出现在节点 `docker inspect`
//     的 Cmd 里，这与官方 Dashboard 的部署方式一致，可接受；
//   - 一条托管对应统一容器（面板专属物理隧道）：ingress 规则云端即改即生效，
//     新增/删除节点无需重启容器，也没有「配置过期」概念（脏标逻辑对 CF 无意义，
//     见 handler 侧 no-op）；独立专线（如 ops 上的 New-api）有自己的连接器，
//     不归面板托管。
func (d *Deployer) syncCloudflared(ctx context.Context, dep *store.FRPDeploy, platform *store.FRPPlatform, tunnels []*store.FRPTunnel) (*ApplyResult, error) {
	creds, err := ParseCloudflaredCreds(d.Runner.decrypt(platform.TokenEnc))
	if err != nil {
		return nil, err
	}
	tunnelID := creds.UnifiedTunnelID
	if tunnelID == "" {
		// 凭据里没有统一容器 ID（首次托管前还没建过节点）：从托管隧道反解
		for _, t := range tunnels {
			if id, _ := SplitCFRemoteID(t.RemoteID); id != "" {
				tunnelID = id
				break
			}
		}
	}
	if tunnelID == "" {
		return nil, errors.New("统一容器隧道尚未创建：请先在面板新建一条 Cloudflare 隧道（节点）")
	}
	cli := NewCloudflared("", creds.Token, creds.AccountID)
	tok, err := cli.TunnelToken(ctx, tunnelID)
	if err != nil {
		return nil, fmt.Errorf("取隧道运行令牌失败: %w", err)
	}
	if !tokenRe.MatchString(tok) {
		return nil, errors.New("隧道运行令牌格式异常（疑似平台返回内容变化），已中止部署")
	}

	conn, closer, err := d.dial(ctx, dep.ServerID)
	if err != nil {
		return nil, err
	}
	defer closer()
	st := dockerCheck(ctx, conn)
	if !st.Present {
		return nil, errors.New("节点未安装 Docker（可让面板自动安装后重试）")
	}
	if !st.DaemonOK {
		return nil, errors.New("节点 Docker 守护进程不可用：" + st.Err)
	}
	if !containerRe.MatchString(dep.Container) {
		return nil, fmt.Errorf("容器名非法: %q", dep.Container)
	}
	if !imageRe.MatchString(dep.Image) {
		return nil, fmt.Errorf("镜像名非法: %q", dep.Image)
	}
	// 拉镜像（cloudflared 官方镜像在 Docker Hub；国内节点不通时用 ImageFor 换镜像源）
	if _, err := conn.Run(ctx, "docker image inspect "+dep.Image+" >/dev/null 2>&1 || docker pull "+dep.Image); err != nil {
		return nil, fmt.Errorf("拉取镜像 %s 失败: %w", dep.Image, err)
	}
	// 重建容器（host 网络：ingress service 指向宿主 localhost 时需要；
	// 指向 LAN IP 时亦无冲突）。token 只经 SSH 通道进命令行，不落盘。
	run := "docker rm -f " + dep.Container + " >/dev/null 2>&1 || true\n" +
		"docker run -d --name " + dep.Container + " --restart unless-stopped --network host" +
		" --label " + managedLabel + " --label beacontower.kind=" + platform.Kind +
		" --label beacontower.deploy=" + strconv.FormatInt(dep.ID, 10) +
		" " + dep.Image + " tunnel --no-autoupdate run --token " + tok
	if _, err := conn.Run(ctx, run); err != nil {
		return nil, fmt.Errorf("启动容器失败: %w", err)
	}
	time.Sleep(2 * time.Second)
	res := &ApplyResult{DockerVer: st.Version}
	state, _ := conn.Run(ctx, `docker inspect -f '{{.State.Status}}' `+dep.Container+` 2>/dev/null || echo missing`)
	res.Status = normStatus(state)
	tail, _ := conn.Run(ctx, "docker logs --tail 30 "+dep.Container+" 2>&1 | grep -v -E 'eyJ|token' || true")
	res.LogTail = strings.TrimSpace(tail)
	if res.Status != "running" {
		return res, fmt.Errorf("容器未处于运行状态（%s）：%s", res.Status, lastLines(res.LogTail, 3))
	}
	return res, nil
}

// normStatus 把 Docker 的原始状态收敛成四种对外语义（前端标签、dirty 提示都按这套走）：
// running / stopped / missing / error。restarting 归 running（frpc 在自愈重试，别惊动用户），
// created/exited/paused 归 stopped，空值与 missing 归 missing，dead 归 error。
func normStatus(s string) string {
	switch strings.TrimSpace(s) {
	case "running", "restarting":
		return "running"
	case "created", "exited", "paused", "removing":
		return "stopped"
	case "", "missing":
		return "missing"
	case "dead":
		return "error"
	}
	return strings.TrimSpace(s)
}

// Action 容器动作：restart / stop / start。
func (d *Deployer) Action(ctx context.Context, dep *store.FRPDeploy, action string) (*ApplyResult, error) {
	if !containerRe.MatchString(dep.Container) {
		return nil, fmt.Errorf("容器名非法: %q", dep.Container)
	}
	var cmd string
	switch action {
	case "restart":
		cmd = "docker restart " + dep.Container
	case "stop":
		cmd = "docker stop " + dep.Container
	case "start":
		cmd = "docker start " + dep.Container
	default:
		return nil, errors.New("不支持的动作: " + action)
	}
	conn, closer, err := d.dial(ctx, dep.ServerID)
	if err != nil {
		return nil, err
	}
	defer closer()
	// 先确认这确实是面板托管的容器，避免误操作同名/他人容器
	if _, err := conn.Run(ctx, `docker inspect -f '{{index .Config.Labels "beacontower.managed"}}' `+dep.Container+` 2>/dev/null | grep -q '^frpc$'`); err != nil {
		return nil, errors.New("节点上找不到面板托管的该容器（可能已被手动删除）")
	}
	if _, err := conn.Run(ctx, cmd); err != nil {
		return nil, fmt.Errorf("%s 失败: %w", action, err)
	}
	return d.statusWithConn(ctx, conn, dep)
}

// Status 读取容器状态（running/stopped/missing/error）。
func (d *Deployer) Status(ctx context.Context, dep *store.FRPDeploy) (*ApplyResult, error) {
	conn, closer, err := d.dial(ctx, dep.ServerID)
	if err != nil {
		return nil, err
	}
	defer closer()
	return d.statusWithConn(ctx, conn, dep)
}

func (d *Deployer) statusWithConn(ctx context.Context, conn *sshx.Conn, dep *store.FRPDeploy) (*ApplyResult, error) {
	if !containerRe.MatchString(dep.Container) {
		return nil, fmt.Errorf("容器名非法: %q", dep.Container)
	}
	res := &ApplyResult{}
	out, err := conn.Run(ctx, `docker inspect -f '{{.State.Status}}' `+dep.Container+` 2>/dev/null || echo missing`)
	if err != nil {
		return nil, err
	}
	res.Status = normStatus(out)
	return res, nil
}

// Logs 容器日志尾部。
func (d *Deployer) Logs(ctx context.Context, dep *store.FRPDeploy, tail int) (string, error) {
	if tail <= 0 || tail > 500 {
		tail = 200
	}
	if !containerRe.MatchString(dep.Container) {
		return "", fmt.Errorf("容器名非法: %q", dep.Container)
	}
	conn, closer, err := d.dial(ctx, dep.ServerID)
	if err != nil {
		return "", err
	}
	defer closer()
	out, err := conn.Run(ctx, "docker logs --tail "+strconv.Itoa(tail)+" "+dep.Container+" 2>&1 || true")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Remove 删除容器（可选连配置文件一起删；配置文件含平台令牌，默认删）。
func (d *Deployer) Remove(ctx context.Context, dep *store.FRPDeploy, purge bool) error {
	conn, closer, err := d.dial(ctx, dep.ServerID)
	if err != nil {
		return err
	}
	defer closer()
	if containerRe.MatchString(dep.Container) {
		if _, err := conn.Run(ctx, "docker rm -f "+dep.Container+" >/dev/null 2>&1 || true"); err != nil {
			return fmt.Errorf("删除容器失败: %w", err)
		}
	}
	if purge && dep.ConfigPath != "" {
		if err := sshx.ValidatePath(dep.ConfigPath); err != nil {
			return fmt.Errorf("配置路径非法: %w", err)
		}
		dir := strings.TrimSuffix(dep.ConfigPath, "/frpc.ini")
		if _, err := conn.Run(ctx, "rm -rf "+dir); err != nil {
			return fmt.Errorf("清理配置失败: %w", err)
		}
	}
	return nil
}

// lastLines 取日志尾部的若干行（错误摘要用，避免把整段日志塞进错误）。
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := strings.TrimSpace(strings.Join(lines, "；"))
	if len(out) > 300 {
		out = out[len(out)-300:]
	}
	return out
}

// MarshalLogTail 供 handler 把日志尾部塞进 JSON（保留换行）。
func MarshalLogTail(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
