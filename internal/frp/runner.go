package frp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/store"
)

// ErrNotFound 平台不存在。
var ErrNotFound = errors.New("平台不存在或已解绑")

// ErrUnsupported 该平台不支持此操作（两平台能力不等价，由 handler 转成友好提示）。
var ErrUnsupported = errors.New("该平台不支持此操作")

// ConfigTarget 下载 frpc 配置所需的定位信息。两平台取参不同：
// Sakura 认隧道 ID，ChmlFrp 要「节点名 + 隧道名」。
type ConfigTarget struct {
	RemoteID string
	NodeName string
	Name     string
}

// platform 两平台客户端的公共能力。写操作只收 AGPL/文档里明确可用的子集；
// 平台特有操作（锁定/迁移/下线/认证）由 Runner 按 kind 分派。
type platform interface {
	UserInfo(ctx context.Context) (*Account, error)
	Tunnels(ctx context.Context) ([]*Tunnel, error)
	Nodes(ctx context.Context) ([]*Node, error)
	CreateTunnel(ctx context.Context, in TunnelInput) (string, error)
	UpdateTunnel(ctx context.Context, id string, in TunnelInput) error
	DeleteOne(ctx context.Context, id string) error
	ConfigFor(ctx context.Context, tgt ConfigTarget) (string, error)
}

// Runner 穿透平台协调层：凭据解密、令牌续期、同步落库、写操作分派。
type Runner struct {
	DB     *store.DB
	Master []byte
	Flow   *DeviceFlow

	// DialForProbe 本地连接计数的节点拨号钩子（由 Deployer 注入实现，
	// Runner 不直接持有凭据解密）。nil 时只采本机节点。
	DialForProbe ProbeDialer

	mu    sync.Mutex
	locks map[int64]*sync.Mutex

	// syncAllRuns SyncAll 轮次计数（周期性 full 同步用，见 syncAllFullEvery）
	syncAllRuns int
}

func NewRunner(db *store.DB, master []byte, clientID string) *Runner {
	return &Runner{
		DB:     db,
		Master: master,
		Flow:   NewDeviceFlow(clientID),
		locks:  map[int64]*sync.Mutex{},
	}
}

// lockFor 每个平台一把锁。ChmlFrp 的 refresh_token 是一次性的：
// 两个 goroutine 同时刷新会把令牌烧掉（一个成功、另一个拿到 invalid_grant，
// 且失败方的落库可能覆盖成功方的），因此续期与同步必须串行到平台粒度。
func (r *Runner) lockFor(id int64) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.locks[id]; ok {
		return m
	}
	m := &sync.Mutex{}
	r.locks[id] = m
	return m
}

// ---------- 凭据 ----------

func (r *Runner) decrypt(blob []byte) string {
	if len(blob) == 0 {
		return ""
	}
	s, err := crypto.DecryptString(r.Master, blob)
	if err != nil {
		log.Printf("[frp] 凭据解密失败（master key 与密文不匹配？）: %v", err)
		return ""
	}
	return s
}

func (r *Runner) encrypt(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := crypto.EncryptString(r.Master, s)
	if err != nil {
		return nil
	}
	return b
}

// tokenFresh 判断当前 access_token 是否还够用（留 90s 余量覆盖一次请求往返）。
// Sakura 访问密钥与 Cloudflare API Token 都是长期有效的静态凭据，视为恒新鲜。
func tokenFresh(p *store.FRPPlatform) bool {
	if p.Kind != KindChmlfrp {
		return true
	}
	if p.TokenExpireAt == 0 {
		return false
	}
	return time.Now().Unix() < p.TokenExpireAt-90
}

// client 构造平台客户端。ChmlFrp 令牌临近过期时在此自动续期并落库——
// 续期结果必须立刻写回，否则进程重启或并发请求都会拿着已失效的旧令牌。
func (r *Runner) client(ctx context.Context, p *store.FRPPlatform) (platform, error) {
	token := r.decrypt(p.TokenEnc)
	if token == "" {
		return nil, fmt.Errorf("%w（未保存访问密钥）", ErrAuth)
	}

	switch p.Kind {
	case KindNatfrp:
		return NewNatfrp("", token), nil

	case KindCloudflared:
		creds, err := ParseCloudflaredCreds(token)
		if err != nil {
			return nil, err
		}
		if creds.AccountID == "" {
			return nil, fmt.Errorf("%w（Cloudflare 凭据缺少 account_id，请重新绑定）", ErrAuth)
		}
		c := NewCloudflared("", creds.Token, creds.AccountID)
		c.ZoneID = creds.ZoneID
		c.ManagedTunnelID = creds.UnifiedTunnelID
		return c, nil

	case KindChmlfrp:
		if tokenFresh(p) {
			return NewChmlfrp("", token), nil
		}
		refresh := r.decrypt(p.RefreshEnc)
		if refresh == "" {
			return nil, fmt.Errorf("%w（缺少刷新令牌，请重新授权）", ErrAuth)
		}
		tok, err := RefreshChmlfrpToken(ctx, r.Flow.clientID, refresh)
		if err != nil {
			return nil, err
		}
		if err := r.DB.UpdateFRPPlatformCreds(p.ID, r.encrypt(tok.AccessToken), r.encrypt(tok.RefreshToken), tok.ExpiresAt()); err != nil {
			// 落库失败不阻断本次调用，但下次还会用旧令牌再来一遍，必须留痕
			log.Printf("[frp] 刷新后的令牌落库失败（平台 %d）: %v", p.ID, err)
		}
		p.TokenEnc = r.encrypt(tok.AccessToken)
		p.RefreshEnc = r.encrypt(tok.RefreshToken)
		p.TokenExpireAt = tok.ExpiresAt()
		log.Printf("[frp] 平台 %d 的 ChmlFrp 令牌已自动续期（%ds）", p.ID, tok.ExpiresIn)
		return NewChmlfrp("", tok.AccessToken), nil

	default:
		return nil, fmt.Errorf("未知平台类型: %s", p.Kind)
	}
}

// ---------- 绑定 ----------

// BindNatfrp 绑定 Sakura 账号：先用访问密钥拉一次用户信息验活，成功才落库。
func (r *Runner) BindNatfrp(ctx context.Context, name, token string) (*store.FRPPlatform, error) {
	c := NewNatfrp("", token)
	acc, err := c.UserInfo(ctx)
	if err != nil {
		return nil, err
	}
	p := &store.FRPPlatform{
		Kind:     KindNatfrp,
		Name:     name,
		Status:   "ok",
		UID:      acc.UID,
		Username: acc.Username,
	}
	p.TokenEnc = r.encrypt(token)
	id, err := r.DB.InsertFRPPlatform(p)
	if err != nil {
		return nil, err
	}
	p.ID = id
	return r.DB.GetFRPPlatform(id)
}

// BindCloudflared 绑定 Cloudflare 账号：只需 API Token——Account ID 用 token
// 自动发现（GET /accounts），Zone ID 延迟到首次同步时按隧道 hostname 发现
// （cfEnhance）。token/zone 缺一不可时才要求用户手填。creds.AccountID 可空。
func (r *Runner) BindCloudflared(ctx context.Context, name string, creds CloudflaredCreds) (*store.FRPPlatform, error) {
	if creds.Token == "" {
		return nil, errors.New("请填写 Cloudflare API Token")
	}
	c := NewCloudflared("", creds.Token, creds.AccountID)
	if creds.AccountID == "" {
		accID, accounts, err := DiscoverAccount(ctx, creds.Token)
		if err != nil {
			return nil, err
		}
		creds.AccountID = accID
		c.AccountID = accID
		if len(accounts) > 1 {
			log.Printf("[frp] 该 API Token 关联 %d 个 Cloudflare 账户，已选用第一个（%s）", len(accounts), accounts[0])
		}
	}
	email, err := c.Verify(ctx)
	if err != nil {
		return nil, err
	}
	p := &store.FRPPlatform{
		Kind:     KindCloudflared,
		Name:     name,
		Status:   "ok",
		UID:      creds.AccountID,
		Username: email,
	}
	encoded, err := EncodeCloudflaredCreds(creds)
	if err != nil {
		return nil, err
	}
	p.TokenEnc = r.encrypt(encoded)
	id, err := r.DB.InsertFRPPlatform(p)
	if err != nil {
		return nil, err
	}
	p.ID = id
	return r.DB.GetFRPPlatform(id)
}

// BindChmlfrp 用设备码会话换取正式绑定。reuseID 非空表示「重新授权」已有平台，
func (r *Runner) BindChmlfrp(ctx context.Context, sessionID string, reuseID int64) (*store.FRPPlatform, error) {
	tok := r.Flow.Token(sessionID)
	if tok == nil {
		return nil, errors.New("授权会话无效或尚未完成，请重新发起授权")
	}
	c := NewChmlfrp("", tok.AccessToken)
	acc, err := c.UserInfo(ctx)
	if err != nil {
		return nil, err
	}

	if reuseID > 0 {
		if err := r.DB.UpdateFRPPlatformCreds(reuseID, r.encrypt(tok.AccessToken), r.encrypt(tok.RefreshToken), tok.ExpiresAt()); err != nil {
			return nil, err
		}
		p, _ := r.DB.GetFRPPlatform(reuseID)
		if p == nil {
			return nil, ErrNotFound
		}
		p.Username = acc.Username
		p.UID = acc.UID
		p.Status = "ok"
		p.LastError = ""
		_ = r.DB.SaveFRPPlatformProfile(p)
		return r.DB.GetFRPPlatform(reuseID)
	}

	// 展示名用平台名而非用户名：账号画像（username）属于隐私面，
	// 不该出现在卡片标题与游客可见的位置；多账号靠改名区分
	name := "ChmlFrp"
	// 同名账号已存在时直接复用（避免重复绑定报 UNIQUE 冲突）
	if exist, _ := r.DB.GetFRPPlatformByName(KindChmlfrp, name); exist != nil {
		if err := r.DB.UpdateFRPPlatformCreds(exist.ID, r.encrypt(tok.AccessToken), r.encrypt(tok.RefreshToken), tok.ExpiresAt()); err != nil {
			return nil, err
		}
		return r.DB.GetFRPPlatform(exist.ID)
	}

	p := &store.FRPPlatform{Kind: KindChmlfrp, Name: name, Status: "ok", UID: acc.UID, Username: acc.Username}
	p.TokenEnc = r.encrypt(tok.AccessToken)
	p.RefreshEnc = r.encrypt(tok.RefreshToken)
	p.TokenExpireAt = tok.ExpiresAt()
	id, err := r.DB.InsertFRPPlatform(p)
	if err != nil {
		return nil, err
	}
	r.Flow.SetUsername(sessionID, acc.Username)
	return r.DB.GetFRPPlatform(id)
}

// ---------- 同步 ----------

// Sync 拉取平台数据并刷新本地镜像。full=true 时同时同步节点列表
// （节点集合变动很慢，周期性轻同步只更新账号与隧道，省掉两次外部请求）。
//
// 单次同步内任一环节失败都不会覆盖既有镜像，只标记 last_error，
// 保证平台临时抖动时面板仍能展示最近一次有效数据。
func (r *Runner) Sync(ctx context.Context, platformID int64, full bool) error {
	lock := r.lockFor(platformID)
	lock.Lock()
	defer lock.Unlock()

	p, err := r.DB.GetFRPPlatform(platformID)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrNotFound
	}
	cli, err := r.client(ctx, p)
	if err != nil {
		r.markError(p, err)
		return err
	}

	acc, err := cli.UserInfo(ctx)
	if err != nil {
		r.markError(p, err)
		return err
	}

	tuns, err := cli.Tunnels(ctx)
	if err != nil {
		r.markError(p, err)
		return err
	}
	now := time.Now().Unix()
	online := 0
	for _, t := range tuns {
		if t.Online {
			online++
		}
	}
	// Sakura 的 /user/info 只返回隧道数上限（tunnels），不返回已用数，
	// 直接用刚拉到的隧道条数补上，否则面板会显示成「0/2」这种自相矛盾的配额。
	// ChmlFrp 自己报了准确值（tunnelCount），不要覆盖。
	// Cloudflare 无配额概念（TunnelQuota=-1 表示不限），已用数同样按条数补。
	if p.Kind == KindNatfrp || p.Kind == KindCloudflared {
		acc.TunnelUsed = len(tuns)
	}
	ids, err := r.DB.ReplaceFRPTunnels(platformID, toStoreTunnels(platformID, tuns, now))
	if err != nil {
		r.markError(p, err)
		return err
	}
	// 回填节点名：Sakura 的隧道只带节点 ID，展示时要名称。
	// Cloudflare 的 NodeID/NodeName 都是 tunnel 名（connector 视图），无需回填。
	r.fillNodeNames(platformID, tuns, ids)

	if full {
		if nodes, err := cli.Nodes(ctx); err == nil {
			if err := r.DB.ReplaceFRPNodes(platformID, toStoreNodes(platformID, nodes, now)); err != nil {
				log.Printf("[frp] 平台 %d 节点镜像写入失败: %v", platformID, err)
			}
		} else {
			log.Printf("[frp] 平台 %d 节点同步失败（保留旧镜像）: %v", platformID, err)
		}
	}

	// 账号画像 + 用量快照
	p.UID = acc.UID
	p.Username = acc.Username
	p.GroupName = acc.GroupName
	p.SpeedLimit = acc.SpeedLimit
	p.Realname = acc.Realname
	p.TunnelUsed = acc.TunnelUsed
	p.TunnelQuota = acc.TunnelQuota
	p.Conns = acc.Conns
	// 连接数口径：ChmlFrp 的 totalCurConns 是平台真实值；Sakura 的 API 无此
	// 字段（acc.Conns 恒 0），先随同步写 0/空，本地计数管线随后覆盖为
	// socket 计数并标 local——两条管线都走 SaveFRPPlatformProfile，避免
	// 这里写空后 local 值被下次平台同步意外清掉时无标记可辨。
	// Cloudflare 的「连接数」口径是 connector↔边缘的活跃连接（conns_active，
	// 各隧道之和），与 frp 的访客并发不同但同样是平台真实值，标 platform。
	if p.Kind == KindChmlfrp {
		p.ConnsSrc = connSrcPlatform
	} else if p.Kind == KindCloudflared {
		sum := 0
		for _, t := range tuns {
			sum += t.Conns
		}
		p.Conns = sum
		p.ConnsSrc = connSrcPlatform
	} else {
		p.ConnsSrc = ""
	}
	p.TrafficDayUsed = acc.TrafficDayUsed
	p.TrafficRemain = acc.TrafficRemain
	p.TrafficUp = acc.TrafficUp
	p.TrafficDown = acc.TrafficDown
	if acc.Extra != nil {
		if b, e := json.Marshal(acc.Extra); e == nil {
			p.ProfileJSON = string(b)
		}
	}
	p.Status = "ok"
	p.LastError = ""
	p.LastSyncAt = now
	if err := r.DB.SaveFRPPlatformProfile(p); err != nil {
		return err
	}
	_ = r.DB.AppendFRPUsage(platformID, &store.FRPUsage{
		TS:             now,
		TrafficDayUsed: acc.TrafficDayUsed,
		TrafficRemain:  acc.TrafficRemain,
		TrafficUp:      acc.TrafficUp,
		TrafficDown:    acc.TrafficDown,
		Conns:          p.Conns,
		TunnelOnline:   online,
		TunnelTotal:    len(tuns),
	})
	// Cloudflare 特有增强，必须在 SaveFRPPlatformProfile 之后：
	//   1) zone 缺失时自动发现（绑定只给了 token 的旧记录 / 只读绑定升级）；
	//   2) 按 ingress hostname 从 GraphQL Analytics 回填今日流量——CF 的
	//      UserInfo 不带流量值（acc.TrafficDayUsed 恒 0），上面那次画像保存
	//      会把它写成 0，流量只能在画像落库后再覆盖，否则会被清掉。
	//   失败静默（面板显示「—」），不影响主镜像。
	if p.Kind == KindCloudflared {
		r.cfEnhance(ctx, p, cli.(*CloudflaredClient), tuns)
	}
	return nil
}

// cfEnhance Cloudflare 平台的同步增强：补 zone（自动发现）+ 补统一容器 ID
// （按名查找已建隧道）+ 回填隧道流量。任何失败只记日志——增强数据缺失时
// 面板显示「—」/写操作时报可读错误，不影响主镜像。
func (r *Runner) cfEnhance(ctx context.Context, p *store.FRPPlatform, cli *CloudflaredClient, tuns []*Tunnel) {
	creds, err := ParseCloudflaredCreds(r.decrypt(p.TokenEnc))
	if err != nil {
		return
	}
	dirty := false
	if creds.ZoneID == "" && len(tuns) > 0 {
		if zid, err := cli.DiscoverZone(ctx, tuns[0].Remote); err == nil && zid != "" {
			creds.ZoneID = zid
			dirty = true
			log.Printf("[frp] 平台 %d 自动发现 Cloudflare zone（%s…）", p.ID, zid[:8])
		}
	}
	if creds.UnifiedTunnelID == "" {
		// 统一容器按名查找：面板以前建过（或回写丢失）直接复用，不重复创建
		if id, err := cli.FindUnifiedTunnel(ctx); err == nil && id != "" {
			creds.UnifiedTunnelID = id
			dirty = true
			log.Printf("[frp] 平台 %d 找到既有统一容器隧道 %s", p.ID, id)
		}
	}
	if dirty {
		if enc, err := EncodeCloudflaredCreds(*creds); err == nil {
			if err := r.DB.UpdateFRPPlatformCreds(p.ID, r.encrypt(enc), nil, 0); err != nil {
				log.Printf("[frp] 平台 %d 的 Cloudflare 凭据增强结果落库失败: %v", p.ID, err)
			}
		}
	}
	cli.ZoneID = creds.ZoneID
	if cli.ZoneID == "" {
		return
	}
	updates := map[string]int64{} // remote_id → 24h 边缘字节数
	total := int64(0)
	for _, t := range tuns {
		if t.Remote == "" {
			continue
		}
		sum, err := cli.TunnelDayTraffic(ctx, t.Remote, 24)
		if err != nil {
			log.Printf("[frp] 平台 %d 隧道「%s」流量查询失败（本轮显示 —）: %v", p.ID, t.Remote, err)
			continue
		}
		if sum == nil {
			continue
		}
		updates[t.RemoteID] = sum.Bytes
		total += sum.Bytes
	}
	if len(updates) > 0 {
		if err := r.DB.SetFRPTunnelTraffic(p.ID, updates); err != nil {
			log.Printf("[frp] 平台 %d 隧道流量回写失败: %v", p.ID, err)
		}
	}
	// 平台级今日流量：各 ingress 域名之和（与其他平台的 TrafficDayUsed 对齐）
	if total > 0 {
		if err := r.DB.SetFRPPlatformDayTraffic(p.ID, total); err != nil {
			log.Printf("[frp] 平台 %d 今日流量回写失败: %v", p.ID, err)
		}
	}
}

// markError 记录失败状态；凭据类错误额外降级为 unbound，前端据此弹重新授权。
func (r *Runner) markError(p *store.FRPPlatform, err error) {
	status := "error"
	if errors.Is(err, ErrAuth) {
		status = "unbound"
	}
	if e := r.DB.SetFRPPlatformStatus(p.ID, status, trimErr(err.Error())); e != nil {
		log.Printf("[frp] 平台状态回写失败: %v", e)
	}
	log.Printf("[frp] 平台 %d(%s) 同步失败: %v", p.ID, p.Name, err)
}

// SyncAll 周期任务入口：逐个平台轻同步，单平台失败不影响其余。
// 同步完成后追加一轮本地连接计数（Sakura 无平台值的连接数由此补齐），
// 失败只记日志，绝不影响同步结果。
// syncAllRuns 计数周期全量同步：节点镜像（ Sakura 71 节点 / ChmlFrp 39 节点）
// 体积大且变动慢，不值得每 2 分钟拉；但只靠手动「同步」按钮的话隧道行的节点名
// 与在用节点表会一直空着（线上实测踩坑）。折中：每 10 轮（约 20 分钟）做一次
// full 同步刷新节点镜像。
const syncAllFullEvery = 10

func (r *Runner) SyncAll(ctx context.Context) {
	list, err := r.DB.ListFRPPlatforms()
	if err != nil {
		log.Printf("[frp] 读取平台列表失败: %v", err)
		return
	}
	r.syncAllRuns++
	full := r.syncAllRuns%syncAllFullEvery == 1 // 首轮立即 full，之后每 10 轮一次
	for _, p := range list {
		if err := r.Sync(ctx, p.ID, full); err != nil {
			// Sync 内部已记状态，这里静默继续
			continue
		}
	}
	r.CollectLocalConns(ctx)
}

// fillNodeNames 补齐隧道与节点的对应关系：
//   - Sakura 的隧道只带节点 ID，反查节点表补名称；
//   - ChmlFrp 的隧道只带节点名（/tunnel 不返回节点 ID），反查节点表补
//     remote_id —— in_use 判定、迁移、配置下载都依赖它。
//     节点镜像可能还没同步过（首次同步隧道在节点拉取之前），此时留空，
//     下一次 full 同步会补上。
func (r *Runner) fillNodeNames(platformID int64, tuns []*Tunnel, ids map[string]int64) {
	nodes, err := r.DB.ListFRPNodes()
	if err != nil {
		return
	}
	byID := map[string]string{}
	byName := map[string]string{}
	for _, n := range nodes {
		if n.PlatformID != platformID {
			continue
		}
		byID[n.RemoteID] = n.Name
		byName[n.Name] = n.RemoteID
	}
	for _, t := range tuns {
		if id, ok := ids[t.RemoteID]; ok {
			switch {
			case t.NodeName == "" && t.NodeID != "":
				// Sakura：有 ID 补名称
				if name, ok := byID[t.NodeID]; ok {
					_, _ = r.DB.SQL.Exec(`UPDATE frp_tunnel SET node_name=? WHERE id=?`, name, id)
				}
			case t.NodeName != "" && t.NodeID == "":
				// ChmlFrp：有名称补 ID
				if rid, ok := byName[t.NodeName]; ok {
					_, _ = r.DB.SQL.Exec(`UPDATE frp_tunnel SET node_id=? WHERE id=?`, rid, id)
				}
			}
		}
	}
}

// ---------- 写操作分派 ----------

// CreateTunnel 建隧道，返回平台侧隧道 ID（调用方随后再同步一次以刷新镜像）。
func (r *Runner) CreateTunnel(ctx context.Context, platformID int64, in TunnelInput) (string, error) {
	lock := r.lockFor(platformID)
	lock.Lock()
	defer lock.Unlock()
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return "", err
	}
	if p.Kind == KindChmlfrp {
		in.NodeID = r.chmlNodeName(platformID, in.NodeID)
	}
	if p.Kind == KindCloudflared {
		// 统一容器模型：节点（ingress 规则）挂到面板专属隧道下；
		// 首次创建时懒建物理隧道并把 ID 回写进凭据。
		c := cli.(*CloudflaredClient)
		tunnelID, err := c.EnsureUnifiedTunnel(ctx, func(id string) error {
			return r.persistUnifiedTunnelID(p, id)
		})
		if err != nil {
			return "", err
		}
		c.ManagedTunnelID = tunnelID
		return c.CreateTunnel(ctx, in)
	}
	return cli.CreateTunnel(ctx, in)
}

// persistUnifiedTunnelID 把统一容器隧道 ID 合并进凭据 JSON 落库（懒创建回写）。
func (r *Runner) persistUnifiedTunnelID(p *store.FRPPlatform, tunnelID string) error {
	creds, err := ParseCloudflaredCreds(r.decrypt(p.TokenEnc))
	if err != nil {
		return err
	}
	creds.UnifiedTunnelID = tunnelID
	enc, err := EncodeCloudflaredCreds(*creds)
	if err != nil {
		return err
	}
	if err := r.DB.UpdateFRPPlatformCreds(p.ID, r.encrypt(enc), nil, 0); err != nil {
		return err
	}
	log.Printf("[frp] 平台 %d 统一容器隧道已创建并回写（%s）", p.ID, tunnelID)
	return nil
}

// chmlNodeName 把面板内部的节点标识（remote_id）翻成 ChmlFrp 要求的节点名。
// 真机实测：/create_tunnel 的 node 参数只认节点名，传数字 ID 一律回
// 「节点不存在」；而面板内部（列表/迁移/编辑）一律用 remote_id，所以
// 出网前在这里翻一次。翻不到就原样透传，让平台自己的报错说话。
func (r *Runner) chmlNodeName(platformID int64, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ref
	}
	nodes, err := r.DB.ListFRPNodes()
	if err != nil {
		return ref
	}
	return pickChmlNodeName(nodes, platformID, ref)
}

// pickChmlNodeName 纯函数便于单测：优先按 remote_id 匹配，其次按名字
// （调用方可能已经传的是名字，比如 URL 里手填的节点名）。
func pickChmlNodeName(nodes []*store.FRPNode, platformID int64, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ref
	}
	for _, n := range nodes {
		if n == nil || n.PlatformID != platformID {
			continue
		}
		if n.RemoteID == ref || n.Name == ref {
			return n.Name
		}
	}
	return ref
}

func (r *Runner) UpdateTunnel(ctx context.Context, platformID int64, remoteID string, in TunnelInput) error {
	lock := r.lockFor(platformID)
	lock.Lock()
	defer lock.Unlock()
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return err
	}
	if p.Kind == KindChmlfrp {
		in.NodeID = r.chmlNodeName(platformID, in.NodeID)
	}
	return cli.UpdateTunnel(ctx, remoteID, in)
}

func (r *Runner) DeleteTunnel(ctx context.Context, platformID int64, remoteID string) error {
	lock := r.lockFor(platformID)
	lock.Lock()
	defer lock.Unlock()
	_, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return err
	}
	return cli.DeleteOne(ctx, remoteID)
}

// TunnelConfig 取 frpc 配置文本（两个平台都返回 INI：Sakura 按
// NatfrpFrpcVersion 声明的樱花分支版本取 sakura INI，ChmlFrp 直接给 ini）。
func (r *Runner) TunnelConfig(ctx context.Context, platformID int64, tgt ConfigTarget) (string, error) {
	_, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return "", err
	}
	return cli.ConfigFor(ctx, tgt)
}

// LockTunnel Sakura 专属：锁定编辑/删除/迁移。
func (r *Runner) LockTunnel(ctx context.Context, platformID int64, remoteID string, edit, del, migrate bool) error {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return err
	}
	if p.Kind != KindNatfrp {
		return fmt.Errorf("%w：锁定是 Sakura 特有功能", ErrUnsupported)
	}
	return cli.(*NatfrpClient).LockTunnel(ctx, remoteID, edit, del, migrate)
}

// MigrateTunnel Sakura 专属：把隧道迁到另一节点。
func (r *Runner) MigrateTunnel(ctx context.Context, platformID int64, remoteID, nodeID string) error {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return err
	}
	if p.Kind != KindNatfrp {
		return fmt.Errorf("%w：节点迁移是 Sakura 特有功能", ErrUnsupported)
	}
	return cli.(*NatfrpClient).MigrateTunnel(ctx, remoteID, nodeID)
}

// OfflineTunnel ChmlFrp 专属：强制断开当前连接（平台没有「启动」接口）。
func (r *Runner) OfflineTunnel(ctx context.Context, platformID int64, name string) error {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return err
	}
	if p.Kind != KindChmlfrp {
		return fmt.Errorf("%w：强制下线是 ChmlFrp 特有功能", ErrUnsupported)
	}
	return cli.(*ChmlfrpClient).OfflineTunnel(ctx, name)
}

// TunnelAuth Sakura 专属：通过隧道访问认证。
func (r *Runner) TunnelAuth(ctx context.Context, platformID int64, remoteID, ip string) (string, error) {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return "", err
	}
	if p.Kind != KindNatfrp {
		return "", fmt.Errorf("%w：访问认证是 Sakura 特有功能", ErrUnsupported)
	}
	return cli.(*NatfrpClient).TunnelAuth(ctx, remoteID, ip)
}

// TunnelTraffic 单隧道流量曲线，两平台实现不同但都归一到 TrafficPoint。
func (r *Runner) TunnelTraffic(ctx context.Context, platformID int64, t store.FRPTunnel) ([]TrafficPoint, error) {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	switch p.Kind {
	case KindNatfrp:
		m, err := cli.(*NatfrpClient).TunnelTraffic(ctx, t.RemoteID)
		if err != nil {
			return nil, err
		}
		// Sakura 返回 {unix 时间戳: 字节}，转成按时间升序的点
		pts := make([]TrafficPoint, 0, len(m))
		for ts, v := range m {
			pts = append(pts, TrafficPoint{Label: ts, Used: v})
		}
		sortTrafficPoints(pts)
		return pts, nil
	case KindChmlfrp:
		return cli.(*ChmlfrpClient).TunnelLast7Days(ctx, t.RemoteID)
	case KindCloudflared:
		return nil, fmt.Errorf("%w：Cloudflare API 不提供隧道级流量数据", ErrUnsupported)
	}
	return nil, ErrUnsupported
}

// TrafficHistory Sakura 专属：账号级日/周/月流量历史。
// ChmlFrp 用 /flow_last_7_days 代替，由 FlowHistory 处理。
func (r *Runner) TrafficHistory(ctx context.Context, platformID int64, kind string) ([]TrafficPoint, error) {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	if p.Kind != KindNatfrp {
		return nil, fmt.Errorf("%w：日/周/月流量历史是 Sakura 特有功能", ErrUnsupported)
	}
	return cli.(*NatfrpClient).TrafficHistory(ctx, kind)
}

// FlowHistory 账号级流量曲线，两平台统一入口。
func (r *Runner) FlowHistory(ctx context.Context, platformID int64, kind string) ([]TrafficPoint, error) {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	switch p.Kind {
	case KindNatfrp:
		return cli.(*NatfrpClient).TrafficHistory(ctx, kind)
	case KindChmlfrp:
		return cli.(*ChmlfrpClient).FlowLast7Days(ctx)
	case KindCloudflared:
		return nil, fmt.Errorf("%w：Cloudflare API 不提供流量历史", ErrUnsupported)
	}
	return nil, ErrUnsupported
}

// Subdomains ChmlFrp 专属：用户免费二级域名列表。
func (r *Runner) Subdomains(ctx context.Context, platformID int64) ([]*Subdomain, error) {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	if p.Kind != KindChmlfrp {
		return nil, fmt.Errorf("%w：免费二级域名是 ChmlFrp 特有功能", ErrUnsupported)
	}
	return cli.(*ChmlfrpClient).Subdomains(ctx)
}

// AvailableDomains ChmlFrp 专属：可用的主域名列表。
func (r *Runner) AvailableDomains(ctx context.Context, platformID int64) ([]map[string]any, error) {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	if p.Kind != KindChmlfrp {
		return nil, ErrUnsupported
	}
	return cli.(*ChmlfrpClient).AvailableDomains(ctx)
}

// chmlfrpOnly 取 ChmlFrp 客户端，非该平台直接拒绝。
func (r *Runner) chmlfrpOnly(ctx context.Context, platformID int64) (*ChmlfrpClient, error) {
	p, cli, err := r.platform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	if p.Kind != KindChmlfrp {
		return nil, fmt.Errorf("%w：免费二级域名是 ChmlFrp 特有功能", ErrUnsupported)
	}
	return cli.(*ChmlfrpClient), nil
}

// CreateSubdomain 新建免费二级域名。
func (r *Runner) CreateSubdomain(ctx context.Context, platformID int64, domain, record, typ, target, ttl, remarks string) error {
	c, err := r.chmlfrpOnly(ctx, platformID)
	if err != nil {
		return err
	}
	return c.CreateSubdomain(ctx, domain, record, typ, target, ttl, remarks)
}

// UpdateSubdomain 修改二级域名（平台仅允许改 TTL 与目标）。
func (r *Runner) UpdateSubdomain(ctx context.Context, platformID int64, domain, record, target, ttl, remarks string) error {
	c, err := r.chmlfrpOnly(ctx, platformID)
	if err != nil {
		return err
	}
	return c.UpdateSubdomain(ctx, domain, record, target, ttl, remarks)
}

// DeleteSubdomain 删除二级域名。
func (r *Runner) DeleteSubdomain(ctx context.Context, platformID int64, domain, record string) error {
	c, err := r.chmlfrpOnly(ctx, platformID)
	if err != nil {
		return err
	}
	return c.DeleteSubdomain(ctx, domain, record)
}

// Reauthorize 重新授权入口：解出既有凭据供设备码流程复用（不返回值，仅内部用）。
func (r *Runner) PlatformByID(id int64) (*store.FRPPlatform, error) {
	return r.DB.GetFRPPlatform(id)
}

// platform 取平台记录与客户端（不含锁，调用方负责加锁）。
func (r *Runner) platform(ctx context.Context, id int64) (*store.FRPPlatform, platform, error) {
	p, err := r.DB.GetFRPPlatform(id)
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		return nil, nil, ErrNotFound
	}
	cli, err := r.client(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	return p, cli, nil
}

// ---------- 转换 ----------

func toStoreTunnels(platformID int64, list []*Tunnel, now int64) []*store.FRPTunnel {
	out := make([]*store.FRPTunnel, 0, len(list))
	for _, t := range list {
		out = append(out, &store.FRPTunnel{
			PlatformID:   platformID,
			RemoteID:     t.RemoteID,
			Name:         t.Name,
			Proto:        t.Proto,
			NodeID:       t.NodeID,
			NodeName:     t.NodeName,
			LocalIP:      t.LocalIP,
			LocalPort:    t.LocalPort,
			Remote:       t.Remote,
			Online:       t.Online,
			Status:       t.Status,
			StatusReason: t.StatusReason,
			Conns:        t.Conns,
			TodayUp:      t.TodayUp,
			TodayDown:    t.TodayDown,
			Uptime:       t.Uptime,
			ClientVer:    t.ClientVer,
			Extra:        t.Extra,
			LockEdit:     t.LockEdit,
			LockDelete:   t.LockDelete,
			LockMigrate:  t.LockMigrate,
			SyncedAt:     now,
		})
	}
	return out
}

func toStoreNodes(platformID int64, list []*Node, now int64) []*store.FRPNode {
	out := make([]*store.FRPNode, 0, len(list))
	for _, n := range list {
		out = append(out, &store.FRPNode{
			PlatformID:  platformID,
			RemoteID:    n.RemoteID,
			Name:        n.Name,
			Host:        n.Host,
			Area:        n.Area,
			GroupName:   n.GroupName,
			Caps:        n.Caps,
			Online:      n.Online,
			Load:        n.Load,
			Uptime:      n.Uptime,
			Description: n.Description,
			SyncedAt:    now,
		})
	}
	return out
}

func sortTrafficPoints(pts []TrafficPoint) {
	sort.Slice(pts, func(i, j int) bool {
		a, ea := strconv.ParseInt(pts[i].Label, 10, 64)
		b, eb := strconv.ParseInt(pts[j].Label, 10, 64)
		if ea == nil && eb == nil {
			return a < b
		}
		return pts[i].Label < pts[j].Label
	})
}
