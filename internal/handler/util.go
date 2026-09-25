package handler

import (
	"encoding/json"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/crypto"
)

// hashToken 会话 token 哈希。
func hashToken(token string) string { return crypto.HashToken(token) }

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// intervalTicker 轻量封装（SSE 心跳用）。
type intervalTicker struct {
	t *time.Ticker
	C <-chan time.Time
}

func newIntervalTicker(d time.Duration) *intervalTicker {
	t := time.NewTicker(d)
	return &intervalTicker{t: t, C: t.C}
}

func (t *intervalTicker) Stop() { t.t.Stop() }
