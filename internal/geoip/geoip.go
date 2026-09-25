package geoip

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

// Result IP 归属解析结果（只到地区级，公开链路只见文案不见 IP）。
type Result struct {
	Country string // HK / JP / CN …
	City    string
	Region  string // 中文展示文案：香港 / 东京 …
}

// 内网段直接标记内网（doc/05 §5）。
func isPrivateIP(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() {
		return true
	}
	if parsed.To4() != nil {
		for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8"} {
			_, n, _ := net.ParseCIDR(cidr)
			if n.Contains(parsed) {
				return true
			}
		}
	}
	return false
}

// ---------- 离线 mmdb（优先） ----------

var (
	mu        sync.RWMutex
	cityDB    *geoip2.Reader
	countryDB *geoip2.Reader
)

// Init 载入离线库（可选）：BEACON_GEOIP_CITY / BEACON_GEOIP_COUNTRY，
// 或数据目录下的 geolite2-city.mmdb / geolite2-country.mmdb。
// 库文件缺失不报错，仅记录日志并降级到在线回显 + 静态映射。
func Init(dataDir string) {
	cityPaths := []string{
		os.Getenv("BEACON_GEOIP_CITY"),
		dataDir + "/geolite2-city.mmdb",
	}
	countryPaths := []string{
		os.Getenv("BEACON_GEOIP_COUNTRY"),
		dataDir + "/geolite2-country.mmdb",
	}
	for _, p := range cityPaths {
		if p == "" {
			continue
		}
		if r, err := geoip2.Open(p); err == nil {
			mu.Lock()
			cityDB = r
			mu.Unlock()
			log.Printf("[geoip] city db loaded: %s", p)
			break
		} else if p == os.Getenv("BEACON_GEOIP_CITY") {
			log.Printf("[geoip] city db %s: %v（降级到在线回显）", p, err)
		}
	}
	for _, p := range countryPaths {
		if p == "" {
			continue
		}
		if r, err := geoip2.Open(p); err == nil {
			mu.Lock()
			countryDB = r
			mu.Unlock()
			log.Printf("[geoip] country db loaded: %s", p)
			break
		} else if p == os.Getenv("BEACON_GEOIP_COUNTRY") {
			log.Printf("[geoip] country db %s: %v（降级到在线回显）", p, err)
		}
	}
}

func offlineLookup(ip net.IP) *Result {
	mu.RLock()
	defer mu.RUnlock()
	if cityDB != nil {
		if rec, err := cityDB.City(ip); err == nil {
			iso := rec.Country.IsoCode
			if iso == "" {
				iso = rec.RegisteredCountry.IsoCode
			}
			if iso != "" {
				city := pickName(rec.City.Names)
				return &Result{Country: iso, City: city, Region: regionText(iso, city)}
			}
		}
	}
	if countryDB != nil {
		if rec, err := countryDB.Country(ip); err == nil {
			iso := rec.Country.IsoCode
			if iso == "" {
				iso = rec.RegisteredCountry.IsoCode
			}
			if iso != "" {
				return &Result{Country: iso, Region: regionText(iso, "")}
			}
		}
	}
	return nil
}

func pickName(names map[string]string) string {
	if names == nil {
		return ""
	}
	for _, lang := range []string{"zh-CN", "zh", "en"} {
		if v := names[lang]; v != "" {
			return v
		}
	}
	for _, v := range names {
		return v
	}
	return ""
}

// ---------- 在线回显（回退，带缓存） ----------

type cacheEntry struct {
	res *Result
	at  time.Time
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cacheEntry{}
)

const cacheTTL = 24 * time.Hour

var httpClient = &http.Client{Timeout: 6 * time.Second}

// onlineLookup 经 ip-api.com 回显解析（只读 countryCode/city，与采集脚本同源）。
// 失败返回 nil，由上层降级静态映射。
func onlineLookup(ip string) *Result {
	cacheMu.Lock()
	if e, ok := cache[ip]; ok && time.Since(e.at) < cacheTTL {
		cacheMu.Unlock()
		return e.res
	}
	cacheMu.Unlock()

	req, err := http.NewRequest("GET", "http://ip-api.com/json/"+ip+"?fields=status,countryCode,city", nil)
	if err != nil {
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil
	}
	var v struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.Status != "success" || v.CountryCode == "" {
		return nil
	}
	res := &Result{Country: v.CountryCode, City: v.City, Region: regionText(v.CountryCode, v.City)}
	cacheMu.Lock()
	cache[ip] = cacheEntry{res: res, at: time.Now()}
	if len(cache) > 4096 {
		// 简单淘汰：清空一半（低频操作，无需 LRU）
		n := 0
		for k := range cache {
			delete(cache, k)
			if n++; n > 2048 {
				break
			}
		}
	}
	cacheMu.Unlock()
	return res
}

// ---------- 主入口 ----------

// Lookup 解析链：内网直返 → 离线 mmdb → 在线回显（缓存） → 静态映射 → 未定位。
func Lookup(ip string) *Result {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return &Result{Region: "未定位"}
	}
	if isPrivateIP(ip) {
		return &Result{Country: "LAN", Region: "内网"}
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return &Result{Region: "未定位"}
	}
	if r := offlineLookup(parsed); r != nil {
		return r
	}
	if r := onlineLookup(ip); r != nil {
		return r
	}
	// 静态映射兜底（文档网段，便于联调离线环境）
	switch {
	case strings.HasPrefix(ip, "203.0.113."):
		return &Result{Country: "HK", City: "Hong Kong", Region: "香港"}
	case strings.HasPrefix(ip, "198.51.100."):
		return &Result{Country: "JP", City: "Tokyo", Region: "东京"}
	case strings.HasPrefix(ip, "192.0.2."):
		return &Result{Country: "SG", City: "Singapore", Region: "新加坡"}
	}
	return &Result{Region: "未定位"}
}

// RegionText 解析结果转展示文案。
func RegionText(r *Result) string {
	if r == nil {
		return "未定位"
	}
	if r.Region != "" {
		return r.Region
	}
	if r.Country != "" {
		return r.Country
	}
	return "未定位"
}

// regionText ISO 国家码 → 中文展示文案（公开链路只见文案）。
// 未收录的国家码回退为城市名，再回退为国家码本身。
func regionText(iso, city string) string {
	if v, ok := countryRegion[strings.ToUpper(iso)]; ok {
		return v
	}
	if city != "" {
		return city
	}
	if iso != "" {
		return strings.ToUpper(iso)
	}
	return "未定位"
}

// 常见国家/地区码 → 中文文案（按需扩展）。
var countryRegion = map[string]string{
	"CN": "中国内地",
	"HK": "香港",
	"MO": "澳门",
	"TW": "台湾",
	"JP": "日本",
	"KR": "韩国",
	"SG": "新加坡",
	"US": "美国",
	"CA": "加拿大",
	"GB": "英国",
	"DE": "德国",
	"FR": "法国",
	"NL": "荷兰",
	"AU": "澳大利亚",
	"IN": "印度",
	"RU": "俄罗斯",
	"BR": "巴西",
	"AE": "阿联酋",
	"MY": "马来西亚",
	"TH": "泰国",
	"VN": "越南",
	"ID": "印度尼西亚",
	"PH": "菲律宾",
}
