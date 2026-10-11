package geoip

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
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

var httpClient = &http.Client{
	Timeout:   6 * time.Second,
	Transport: proxyTransport(),
}

// proxyTransport 出站代理：BEACON_PROXY_URL 已配置时经其回显（服务器直连
// 外网被墙的部署形态），为空直连。与 frp 包的代理口径一致但独立实现，
// 避免 geoip 反向依赖 frp。
func proxyTransport() http.RoundTripper {
	if raw := os.Getenv("BEACON_PROXY_URL"); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
			return &http.Transport{Proxy: http.ProxyURL(u)}
		}
	}
	return nil
}

// onlineProvider 在线回显源：返回 nil 表示该源不可用/不认识这个 IP。
type onlineProvider struct {
	name  string
	url   func(ip string) string
	parse func(body []byte) *Result
}

// onlineProviders 在线回显源列表（只取 country/city，与采集脚本同源）。
// ip-api 免费接口只提供 HTTP——HTTPS 是付费版，写 https 会稳定失败（v2.7.2
// 实测：面板宿主直连「Empty reply」、经代理 502，解析链整体失效）。
// 多源兜底：单一回显源在墙内/机房出口上随时可能不可达，逐个试、谁先成就用谁。
var onlineProviders = []onlineProvider{
	{"ip-api", func(ip string) string {
		return "http://ip-api.com/json/" + ip + "?fields=status,countryCode,city"
	}, parseIPAPI},
	{"ipinfo", func(ip string) string { return "https://ipinfo.io/" + ip + "/json" }, parseIPInfo},
	{"ipapi.co", func(ip string) string { return "https://ipapi.co/" + ip + "/json/" }, parseIPAPICo},
}

func parseIPAPI(body []byte) *Result {
	var v struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.Status != "success" || v.CountryCode == "" {
		return nil
	}
	return &Result{Country: v.CountryCode, City: v.City, Region: regionText(v.CountryCode, v.City)}
}

func parseIPInfo(body []byte) *Result {
	var v struct {
		Country string `json:"country"`
		City    string `json:"city"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.Country == "" {
		return nil
	}
	return &Result{Country: v.Country, City: v.City, Region: regionText(v.Country, v.City)}
}

func parseIPAPICo(body []byte) *Result {
	var v struct {
		CountryCode string `json:"country_code"`
		City        string `json:"city"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.CountryCode == "" {
		return nil
	}
	return &Result{Country: v.CountryCode, City: v.City, Region: regionText(v.CountryCode, v.City)}
}

// onlineLookupFn 供测试替换（默认在线回显），避免单测依赖外网。
var onlineLookupFn = onlineLookup

// onlineLookup 依次尝试在线回显源。全部失败返回 nil，由上层降级。
// 总预算 6s（单源 2.5s）：上游故障期不把持锁的采集链路拖住（旧实现单源 6s）。
// 负缓存：失败结果缓存 5 分钟，故障期不反复重试。
func onlineLookup(ip string) *Result {
	const negTTL = 5 * time.Minute
	cacheMu.Lock()
	if e, ok := cache[ip]; ok {
		if e.res != nil && time.Since(e.at) < cacheTTL {
			cacheMu.Unlock()
			return e.res
		}
		if e.res == nil && time.Since(e.at) < negTTL {
			cacheMu.Unlock()
			return nil
		}
	}
	cacheMu.Unlock()

	deadline := time.Now().Add(6 * time.Second)
	for _, p := range onlineProviders {
		remain := time.Until(deadline)
		if remain <= 0 {
			break
		}
		budget := 2500 * time.Millisecond
		if remain < budget {
			budget = remain
		}
		res := fetchOnlineGeo(p, ip, budget)
		if res == nil {
			continue
		}
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
	cacheMu.Lock()
	cache[ip] = cacheEntry{res: nil, at: time.Now()}
	cacheMu.Unlock()
	return nil
}

// fetchOnlineGeo 单源请求（含预算与状态码校验）；任何失败返回 nil。
func fetchOnlineGeo(p onlineProvider, ip string, budget time.Duration) *Result {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", p.url(ip), nil)
	if err != nil {
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil
	}
	return p.parse(body)
}

// ---------- 主入口 ----------

// UnknownText 归属未知时的展示文案。**仅用于展示/提示，不写存储**——
// 把占位文案当解析结果写进 region 正是 v2.7.2「未定位」线上事故的成因（doc/17）。
const UnknownText = "未定位"

// IsPlaceholder 地区文本是否为「未定位」占位文案：创建/编辑节点时应视作空值，
// 否则会被记成「管理员指定」（region_source=manual），自动定位永久失效。
func IsPlaceholder(s string) bool { return strings.TrimSpace(s) == UnknownText }

// Lookup 解析链：内网直返 → 离线 mmdb → 在线回显（缓存） → 静态映射。
// **解析不出来时返回 nil**。旧实现返回 Result{Region:"未定位"} 占位对象且永不返回
// nil，调用方无法区分「解析失败」与「解析成功」——占位文案被当值落库、被回传固化。
func Lookup(ip string) *Result {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil
	}
	if isPrivateIP(ip) {
		return &Result{Country: "LAN", Region: "内网"}
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil
	}
	if r := offlineLookup(parsed); r != nil {
		return r
	}
	if r := onlineLookupFn(ip); r != nil {
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
	return nil
}

// RegionText 解析结果转展示文案；未解析（nil 或字段全空）返回空串。
// 空串由展示层各自回退（前端 region 显示有 '未定位' 兜底），存储层必须留空。
func RegionText(r *Result) string {
	if r == nil {
		return ""
	}
	if r.Region != "" {
		return r.Region
	}
	if r.Country != "" {
		return r.Country
	}
	return ""
}

// regionText ISO 国家码 → 中文展示文案（公开链路只见文案）。
// 未收录的国家码回退为城市名，再回退为国家码本身；两者皆无返回空串。
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
	return ""
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
