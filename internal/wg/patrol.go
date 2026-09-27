package wg

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// Patrol WG 巡检（5min ticker 调用）：
// - 逐个 hub/备胎建连读 dump → 更新成员握手/流量/在线状态（hub 侧视角覆盖全网成员）；
// - hub 当日转发流量按累计计数器差值入 wg_hub_traffic（月度汇总驱动额度提醒）；
// - hub 公钥与面板记录不符 → 标注异常（配置被外部改动的信号）。
// 全程尽力而为：单点失败只记日志/状态，不影响其他节点。
func (r *Runner) Patrol() {
	network, err := r.DB.GetWGNetwork()
	if err != nil || network == nil {
		return
	}
	hubs, err := r.DB.ListWGHub()
	if err != nil || len(hubs) == 0 {
		return
	}
	now := time.Now().Unix()
	day := dayFloor(now)
	peers, _ := r.DB.ListWGPeers()
	byPub := map[string]*store.WGPeer{}
	for _, p := range peers {
		byPub[p.PublicKey] = p
	}
	for _, hub := range hubs {
		r.patrolHub(network, hub, byPub, day, now)
	}
}

func (r *Runner) patrolHub(network *store.WGNetwork, hub *store.WGHub, byPub map[string]*store.WGPeer, day, now int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	strict := r.strictHostKey()
	conn, closer, err := r.dial(ctx, hub.ServerID, strict)
	if err != nil {
		hub.Status = "error"
		hub.LastError = "巡检连接失败: " + err.Error()
		hub.CheckedAt.Scan(now)
		_ = r.DB.UpsertWGHub(hub)
		return
	}
	defer closer()
	devs, err := ShowDump(ctx, conn)
	if err != nil {
		hub.Status = "error"
		hub.LastError = "巡检读取失败: " + err.Error()
		hub.CheckedAt.Scan(now)
		_ = r.DB.UpsertWGHub(hub)
		return
	}
	var dumpDev *DevState
	for _, d := range devs {
		if d.Interface == network.Iface {
			dumpDev = &d
		}
	}
	if dumpDev == nil {
		hub.Status = "error"
		hub.LastError = "接口 " + network.Iface + " 未运行"
		hub.CheckedAt.Scan(now)
		_ = r.DB.UpsertWGHub(hub)
		return
	}
	// 密钥一致性：conf 被外部改动（如手工重配）时公钥会漂移
	hub.LastError = ""
	hub.Status = "ok"
	if dumpDev.PublicKey != "" && dumpDev.PublicKey != hub.PublicKey {
		hub.LastError = "接口公钥与面板记录不符（配置可能被外部修改）"
	}
	hub.CheckedAt.Scan(now)
	// 流量差值：dump rx/tx 为自接口启动的累计值
	var cumRX, cumTX uint64
	for _, p := range dumpDev.Peers {
		cumRX += p.RX
		cumTX += p.TX
	}
	drx := int64(cumRX) - hub.RxCum
	dtx := int64(cumTX) - hub.TxCum
	if drx < 0 || dtx < 0 {
		// 接口重启计数器归零：重置基线，不计入当日
		drx, dtx = 0, 0
	}
	hub.RxCum = int64(cumRX)
	hub.TxCum = int64(cumTX)
	if drx > 0 || dtx > 0 {
		if err := r.DB.AddWGHubTraffic(hub.ServerID, day, drx, dtx, now); err != nil {
			log.Printf("[wg] patrol hub %d 流量写入失败: %v", hub.ServerID, err)
		}
	}
	if err := r.DB.UpsertWGHub(hub); err != nil {
		log.Printf("[wg] patrol hub %d 状态写入失败: %v", hub.ServerID, err)
	}
	// 成员状态（hub 侧视角）
	seen := map[string]bool{}
	for _, dp := range dumpDev.Peers {
		p := byPub[dp.PublicKey]
		if p == nil {
			continue // 未知 peer（面板外手工加入）——概览页以 hub 实况为准
		}
		seen[dp.PublicKey] = true
		p.CheckedAt.Scan(now)
		p.LastHandshake.Scan(dp.LastHandshakeA)
		p.RxBytes = int64(dp.RX)
		p.TxBytes = int64(dp.TX)
		if OnlineByHandshake(time.Unix(dp.LastHandshakeA, 0), time.Unix(now, 0)) {
			if p.Status != "online" {
				p.Status = "online"
				p.LastError = ""
			}
		} else {
			p.Status = "offline"
		}
		if err := r.DB.UpdateWGPeer(p); err != nil {
			log.Printf("[wg] patrol peer %s 更新失败: %v", p.Name, err)
		}
	}
	// DB 中在线成员未出现在 hub dump → 标记离线
	for _, p := range byPub {
		if seen[p.PublicKey] || p.Status == "left" || p.Kind == "" {
			continue
		}
		if p.Status == "online" {
			p.Status = "offline"
			p.CheckedAt.Scan(now)
			_ = r.DB.UpdateWGPeer(p)
		}
	}
}

// PatrolOnce 供外部触发一次巡检（手动刷新用）。
func (r *Runner) PatrolOnce() error {
	network, err := r.DB.GetWGNetwork()
	if err != nil || network == nil {
		return errors.New("尚未初始化组网")
	}
	hubs, _ := r.DB.ListWGHub()
	if len(hubs) == 0 {
		return errors.New("尚无中心节点")
	}
	r.Patrol()
	return nil
}

func dayFloor(now int64) int64 {
	t := time.Unix(now, 0)
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return t.Unix()
}
