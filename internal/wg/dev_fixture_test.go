package wg

import (
	"database/sql"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// TestDevFixture 只在本机开发库上造数据（不改源码、不进生产）：
//
//	BT_FIXTURE_DB     本机开发库路径（如 ./data/beacontower.db）
//	BT_FIXTURE_MASTER 主密钥 hex
//	BT_FIXTURE_PEER_PRIV 真实 hub 认识的成员私钥（面板侧握手探测用）
//	BT_FIXTURE_Z_PUB 备援 hub 公钥；BT_FIXTURE_Z_EP 备援端点
//
// 用 `go test ./internal/wg -run TestDevFixture` 触发后删除本文件。
func TestDevFixture(t *testing.T) {
	dbPath := os.Getenv("BT_FIXTURE_DB")
	hexKey := os.Getenv("BT_FIXTURE_MASTER")
	priv := os.Getenv("BT_FIXTURE_PEER_PRIV")
	zPub := os.Getenv("BT_FIXTURE_Z_PUB")
	zEp := os.Getenv("BT_FIXTURE_Z_EP")
	if dbPath == "" || hexKey == "" {
		t.Skip("未设置 BT_FIXTURE_*：跳过本机开发库造数")
	}
	master, err := hex.DecodeString(hexKey)
	if err != nil || len(master) != 32 {
		t.Fatalf("主密钥无效: %v", err)
	}
	db, err := store.Open(dbPath) // Open 内已跑迁移
	if err != nil {
		t.Fatalf("打开库: %v", err)
	}
	defer db.Close()
	r := NewRunner(db, master)
	now := time.Now().Unix()

	// 1) 备援 hub 槽位（模拟真实 Z）
	if zPub != "" && zEp != "" {
		srvID, err := db.CreateServer(&store.Server{Name: "阿里云-Z", Enabled: true, CreatedAt: now})
		if err != nil {
			t.Fatalf("建节点: %v", err)
		}
		if err := db.UpsertWGHub(&store.WGHub{ServerID: srvID, ListenPort: endpointPort(zEp),
			PublicKey: zPub, Endpoint: zEp, Status: "ok",
			CheckedAt: sql.NullInt64{Int64: now, Valid: true}}); err != nil {
			t.Fatalf("写备援: %v", err)
		}
		t.Logf("已建备援 hub server_id=%d", srvID)
	}

	// 2) 面板托管设备（真机 hub 已知的那把私钥）——让真握手探测可用
	if priv != "" {
		pub, err := PublicKeyFromPrivate(priv)
		if err != nil {
			t.Fatalf("私钥无效: %v", err)
		}
		psk, err := GeneratePSK()
		if err != nil {
			t.Fatalf("生成 PSK: %v", err)
		}
		peers, _ := db.ListWGPeers()
		for _, p := range peers {
			if p.PublicKey == pub {
				p.PskEnc = r.Encrypt(psk) // 补齐合法 PSK（首次造数时随手写的不合法）
				p.PrivateKeyEnc = r.Encrypt(priv)
				if err := db.UpdateWGPeer(p); err != nil {
					t.Fatalf("更新托管成员: %v", err)
				}
				t.Logf("托管成员已存在 id=%d，PSK 已补齐", p.ID)
				return
			}
		}
		id, err := db.InsertWGPeer(&store.WGPeer{Kind: "device", Name: "探针手机（开发库）",
			WgIP: "10.66.66.14", PublicKey: pub, PrivateKeyEnc: r.Encrypt(priv),
			PskEnc: r.Encrypt(psk), Managed: true, Status: "offline", CreatedAt: now})
		if err != nil {
			t.Fatalf("写托管成员: %v", err)
		}
		t.Logf("已建托管设备 id=%d", id)
	}
}
