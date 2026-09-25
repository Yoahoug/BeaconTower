package config

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// Config 服务配置，全部来自环境变量（见 doc/07 §3）。
type Config struct {
	Port              string
	DataDir           string
	MasterKeyHex      string
	SetupToken        string
	CollectInterval   int
	SiteTitle         string
	TrustedProxies    []*net.IPNet
	TrustedProxiesRaw string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load 读取环境变量并做合法性收敛。
func Load() *Config {
	interval, err := strconv.Atoi(getenv("BEACON_COLLECT_INTERVAL", "10"))
	if err != nil || interval < 5 {
		interval = 10
	}
	c := &Config{
		Port:            getenv("BEACON_PORT", "8080"),
		DataDir:         getenv("BEACON_DATA_DIR", "/app/data"),
		MasterKeyHex:    strings.TrimSpace(os.Getenv("BEACON_MASTER_KEY")),
		SetupToken:      os.Getenv("BEACON_SETUP_TOKEN"),
		CollectInterval: interval,
		SiteTitle:       getenv("BEACON_SITE_TITLE", "BeaconTower"),
	}
	raw := strings.TrimSpace(os.Getenv("BEACON_TRUSTED_PROXIES"))
	c.TrustedProxiesRaw = raw
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			if ip := net.ParseIP(part); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				part = part + "/" + strconv.Itoa(bits)
			} else {
				continue
			}
		}
		_, n, err := net.ParseCIDR(part)
		if err == nil {
			c.TrustedProxies = append(c.TrustedProxies, n)
		}
	}
	return c
}
