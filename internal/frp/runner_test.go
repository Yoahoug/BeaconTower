package frp

import (
	"testing"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// TestPickChmlNodeName 回归：ChmlFrp 的 create/edit 只认节点名，面板内部
// 用 remote_id；真机实测传数字 ID 一律「节点不存在」，所以在出网前必须翻名。
func TestPickChmlNodeName(t *testing.T) {
	nodes := []*store.FRPNode{
		{PlatformID: 2, RemoteID: "8", Name: "英国伦敦"},
		{PlatformID: 2, RemoteID: "24", Name: "东莞三线PLUS3"},
		{PlatformID: 1, RemoteID: "8", Name: "别的平台的同 ID 节点"},
		nil,
	}
	cases := []struct {
		name       string
		platformID int64
		ref        string
		want       string
	}{
		{"remote_id 翻成节点名", 2, "8", "英国伦敦"},
		{"24 翻成对应节点名", 2, "24", "东莞三线PLUS3"},
		{"已经是名字则原样返回", 2, "英国伦敦", "英国伦敦"},
		{"跨平台同 remote_id 不误伤", 1, "24", "24"},
		{"翻不到原样透传（交给平台报错）", 2, "999", "999"},
		{"空串原样返回", 2, "  ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickChmlNodeName(nodes, c.platformID, c.ref); got != c.want {
				t.Fatalf("pickChmlNodeName(%d, %q) = %q, want %q", c.platformID, c.ref, got, c.want)
			}
		})
	}
	if got := pickChmlNodeName(nil, 2, "8"); got != "8" {
		t.Fatalf("节点镜像为空时应原样透传，得到 %q", got)
	}
}
