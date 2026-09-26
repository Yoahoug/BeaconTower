package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Yoahoug/BeaconTower/internal/collector"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/geoip"
	"github.com/Yoahoug/BeaconTower/internal/middleware"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/gin-gonic/gin"
)

// ---------- 节点列表（管理端全量，含敏感字段但凭据不回显） ----------

// GET /api/v1/admin/servers
func (a *App) ListServers(c *gin.Context) {
	servers, err := a.DB.ListServers()
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	out := []map[string]any{}
	for _, s := range servers {
		cred, _ := a.DB.GetCredential(s.ID)
		prof, _ := a.DB.GetProfile(s.ID)
		item := map[string]any{
			"id": s.ID, "name": s.Name, "region": s.Region,
			"region_source": s.RegionSource, "tags": s.Tags,
			"note_public": s.NotePublic, "note_private": s.NotePrivate,
			"sort_order": s.SortOrder, "hidden": s.Hidden, "enabled": s.Enabled,
			"is_self": s.IsSelf, "created_at": s.CreatedAt,
			"ssh":     sshView(cred),
			"profile": profileView(prof),
			"power":   powerAdminView(prof, cred),
		}
		if cred != nil {
			item["last_success_at"] = nullableInt(cred.LastSuccessAt)
		}
		out = append(out, item)
	}
	middleware.OK(c, out)
}

// sshView 凭据不回显：只返回 has_password/has_key 标记。
func sshView(cred *store.Credential) any {
	if cred == nil {
		// 本机节点无凭据：采集走本地进程
		return map[string]any{"local": true}
	}
	return map[string]any{
		"host": cred.Host, "port": cred.Port, "username": cred.Username,
		"auth_type":    cred.AuthType,
		"has_password": len(cred.PasswordEnc) > 0,
		"has_key":      len(cred.PrivateKeyEnc) > 0,
		"host_key_fp":  cred.HostKeyFP,
	}
}

func profileView(p *store.Profile) any {
	if p == nil {
		return nil
	}
	return map[string]any{
		"hostname": p.Hostname, "os_name": p.OsName, "os_version": p.OsVersion,
		"kernel": p.Kernel, "arch": p.Arch, "cpu_model": p.CpuModel,
		"cpu_cores": p.CpuCores, "mem_total": p.MemTotal, "disk_total": p.DiskTotal,
		"virt": p.Virt, "public_ip": p.PublicIP,
		"geo_country": p.GeoCountry, "geo_city": p.GeoCity,
		"power": map[string]any{
			"rapl": p.PowerRapL, "battery": p.PowerBattery,
			"base_load_w": p.BaseLoadW, "base_load_source": p.BaseLoadSource,
		},
	}
}

func powerAdminView(p *store.Profile, cred *store.Credential) any {
	lastErr := ""
	if cred != nil {
		lastErr = cred.LastError
	}
	if p == nil {
		return map[string]any{"rapl": false, "battery": false, "base_load_w": 0, "base_load_source": "default", "last_error": lastErr}
	}
	return map[string]any{
		"rapl": p.PowerRapL, "battery": p.PowerBattery,
		"base_load_w": p.BaseLoadW, "base_load_source": p.BaseLoadSource, "last_error": lastErr,
	}
}

// ---------- 新建 / 编辑 ----------

type sshInput struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	AuthType   string `json:"auth_type"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
	Passphrase string `json:"passphrase"`
}

type probeInput struct {
	Profile struct {
		Hostname  string `json:"hostname"`
		OsName    string `json:"os_name"`
		OsVersion string `json:"os_version"`
		Kernel    string `json:"kernel"`
		Arch      string `json:"arch"`
		CpuModel  string `json:"cpu_model"`
		CpuCores  int    `json:"cpu_cores"`
		MemTotal  int64  `json:"mem_total"`
		DiskTotal int64  `json:"disk_total"`
		Virt      string `json:"virt"`
		Power     *struct {
			Rapl           bool    `json:"rapl"`
			Battery        bool    `json:"battery"`
			BaseLoadW      float64 `json:"base_load_w"`
			BaseLoadSource string  `json:"base_load_source"`
		} `json:"power"`
	} `json:"profile"`
	Geo *struct {
		PublicIP string `json:"public_ip"`
		Country  string `json:"country"`
		City     string `json:"city"`
	} `json:"geo"`
	HostKeyFP string `json:"host_key_fp"`
}

type serverInput struct {
	Name        string      `json:"name"`
	Region      *string     `json:"region"`
	Tags        []string    `json:"tags"`
	NotePublic  string      `json:"note_public"`
	NotePrivate string      `json:"note_private"`
	Hidden      bool        `json:"hidden"`
	Enabled     *bool       `json:"enabled"`
	SSH         sshInput    `json:"ssh"`
	Probe       *probeInput `json:"probe"`
}

func validateSSH(s sshInput, isCreate bool) (port int, msg string) {
	port = s.Port
	if port == 0 {
		port = 22
	}
	if strings.TrimSpace(s.Host) == "" || strings.TrimSpace(s.Username) == "" {
		return 0, "参数错误：请填写 SSH 地址与用户名"
	}
	if len(s.Host) > 253 || len(s.Username) > 64 {
		return 0, "参数错误：地址或用户名过长"
	}
	if port < 1 || port > 65535 {
		return 0, "参数错误：端口须为 1–65535"
	}
	if s.AuthType != "password" && s.AuthType != "key" {
		return 0, "参数错误：auth_type 须为 password|key"
	}
	if isCreate {
		if s.AuthType == "password" && s.Password == "" {
			return 0, "参数错误：请填写 SSH 密码"
		}
		if s.AuthType == "key" && s.PrivateKey == "" {
			return 0, "参数错误：请粘贴 SSH 私钥"
		}
	}
	return port, ""
}

// POST /api/v1/admin/servers
func (a *App) CreateServer(c *gin.Context) {
	var in serverInput
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	port, msg := validateSSH(in.SSH, true)
	if msg != "" {
		middleware.Fail(c, 1001, msg)
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		middleware.Fail(c, 1001, "参数错误：请填写节点名称")
		return
	}
	if len(name) > 64 {
		middleware.Fail(c, 1001, "参数错误：节点名称过长（≤64）")
		return
	}
	pwEnc, err := crypto.EncryptString(a.Master, in.SSH.Password)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	keyEnc, err := crypto.EncryptString(a.Master, in.SSH.PrivateKey)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	ppEnc, err := crypto.EncryptString(a.Master, in.SSH.Passphrase)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	region, source := "", "auto"
	if in.Region != nil && strings.TrimSpace(*in.Region) != "" {
		region, source = strings.TrimSpace(*in.Region), "manual"
	} else if in.Probe != nil && in.Probe.Geo != nil {
		if r := geoip.Lookup(in.Probe.Geo.PublicIP); r != nil {
			region = geoip.RegionText(r)
		}
		if in.Probe.Geo.Country != "" && region == "" {
			region = in.Probe.Geo.Country
		}
	}
	servers, _ := a.DB.ListServers()
	s := &store.Server{
		Name: name, Region: region, RegionSource: source, Tags: in.Tags,
		NotePublic: in.NotePublic, NotePrivate: in.NotePrivate,
		SortOrder: len(servers), Hidden: in.Hidden, Enabled: true,
		CreatedAt: nowUnix(),
	}
	id, err := a.DB.CreateServer(s)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	cred := &store.Credential{
		ServerID: id, Host: strings.TrimSpace(in.SSH.Host), Port: port,
		Username: strings.TrimSpace(in.SSH.Username), AuthType: in.SSH.AuthType,
		PasswordEnc: pwEnc, PrivateKeyEnc: keyEnc, PassphraseEnc: ppEnc,
	}
	if in.Probe != nil {
		cred.HostKeyFP = in.Probe.HostKeyFP
	}
	if err := a.DB.UpsertCredential(cred); err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	if in.Probe != nil {
		a.applyProbe(id, in.Probe, source)
	}
	a.audit(a.actorOf(c), "server_create", "server:"+itoa(id),
		"添加节点 "+name+"（"+cred.Host+":"+itoa(int64(port))+"）", ipOf(c))
	middleware.OK(c, gin.H{"id": id})
}

// PUT /api/v1/admin/servers/:id（部分更新：仅覆盖请求中出现的键；凭据留空 = 保留原值）
func (a *App) UpdateServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	srv, err := a.DB.GetServer(id)
	if err != nil || srv == nil {
		middleware.Fail(c, 2002, "节点不存在")
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	// 本机节点：名称/备注等基本信息可改，SSH 凭据不可改（采集走本地进程）
	if srv.IsSelf {
		if _, ok := in["ssh"]; ok {
			middleware.Fail(c, 1001, "本机节点通过本地进程采集，无需 SSH 凭据")
			return
		}
	}
	if v, ok := in["name"]; ok {
		if s := strings.TrimSpace(toStr(v)); s != "" {
			if len(s) > 64 {
				middleware.Fail(c, 1001, "参数错误：节点名称过长（≤64）")
				return
			}
			srv.Name = s
		}
	}
	if v, ok := in["tags"]; ok {
		srv.Tags = toStrSlice(v)
	}
	if v, ok := in["note_public"]; ok {
		srv.NotePublic = toStr(v)
	}
	if v, ok := in["note_private"]; ok {
		srv.NotePrivate = toStr(v)
	}
	if v, ok := in["hidden"]; ok {
		srv.Hidden = toBool(v)
	}
	if v, ok := in["enabled"]; ok {
		srv.Enabled = toBool(v)
	}
	if v, ok := in["region"]; ok {
		if s := strings.TrimSpace(toStr(v)); s != "" {
			srv.Region, srv.RegionSource = s, "manual"
		} else {
			srv.RegionSource = "auto"
		}
	}
	// SSH（允许部分更新；凭据留空 = 保留原值）
	cred, _ := a.DB.GetCredential(id)
	if cred == nil {
		cred = &store.Credential{ServerID: id, Port: 22, AuthType: "password"}
	}
	if v, ok := in["ssh"]; ok {
		if m, ok := v.(map[string]any); ok {
			if hv, ok := m["host"]; ok {
				if s := strings.TrimSpace(toStr(hv)); s != "" {
					cred.Host = s
				}
			}
			if pv, ok := m["port"]; ok {
				if p := toInt(pv); p >= 1 && p <= 65535 {
					cred.Port = p
				}
			}
			if uv, ok := m["username"]; ok {
				if s := strings.TrimSpace(toStr(uv)); s != "" {
					cred.Username = s
				}
			}
			if av, ok := m["auth_type"]; ok {
				if s := toStr(av); s == "password" || s == "key" {
					cred.AuthType = s
				}
			}
			if pv, ok := m["password"]; ok && toStr(pv) != "" {
				enc, err := crypto.EncryptString(a.Master, toStr(pv))
				if err != nil {
					middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
					return
				}
				cred.PasswordEnc = enc
			}
			if kv, ok := m["private_key"]; ok && toStr(kv) != "" {
				enc, err := crypto.EncryptString(a.Master, toStr(kv))
				if err != nil {
					middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
					return
				}
				cred.PrivateKeyEnc = enc
			}
			if ppv, ok := m["passphrase"]; ok && toStr(ppv) != "" {
				enc, err := crypto.EncryptString(a.Master, toStr(ppv))
				if err != nil {
					middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
					return
				}
				cred.PassphraseEnc = enc
			}
		}
	}
	if err := a.DB.UpdateServer(srv); err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	if err := a.DB.UpsertCredential(cred); err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	a.audit(a.actorOf(c), "server_update", "server:"+itoa(id), "编辑节点 "+srv.Name, ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// DELETE /api/v1/admin/servers/:id
func (a *App) DeleteServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	srv, _ := a.DB.GetServer(id)
	if srv == nil {
		middleware.Fail(c, 2002, "节点不存在")
		return
	}
	if srv.IsSelf {
		middleware.Fail(c, 1001, "本机节点不可删除（面板自身数据源）")
		return
	}
	if err := a.DB.DeleteServer(id); err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	a.audit(a.actorOf(c), "server_delete", "server:"+itoa(id), "删除节点 "+srv.Name, ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// PUT /api/v1/admin/servers/order（前端 admin.js reorder(ids) → {ids}；doc 写 sort_order 单节点，此处兼容两种）
func (a *App) ReorderServers(c *gin.Context) {
	var in struct {
		IDs       []int64 `json:"ids"`
		ID        *int64  `json:"id"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	if len(in.IDs) > 0 {
		if err := a.DB.ReorderServers(in.IDs); err != nil {
			middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
			return
		}
	} else if in.ID != nil && in.SortOrder != nil {
		servers, _ := a.DB.ListServers()
		ids := []int64{}
		moved := false
		for _, s := range servers {
			if s.ID == *in.ID {
				moved = true
				continue
			}
			ids = append(ids, s.ID)
		}
		if !moved {
			middleware.Fail(c, 2002, "节点不存在")
			return
		}
		pos := *in.SortOrder
		if pos < 0 {
			pos = 0
		}
		if pos > len(ids) {
			pos = len(ids)
		}
		ids = append(ids[:pos], append([]int64{*in.ID}, ids[pos:]...)...)
		if err := a.DB.ReorderServers(ids); err != nil {
			middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
			return
		}
	} else {
		middleware.Fail(c, 1001, "参数错误：须提供 ids 或 id+sort_order")
		return
	}
	a.audit(a.actorOf(c), "server_order", "servers", "调整节点排序", ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// PUT /api/v1/admin/servers/:id/power-calibration
func (a *App) RecalibratePower(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	srv, _ := a.DB.GetServer(id)
	if srv == nil {
		middleware.Fail(c, 2002, "节点不存在")
		return
	}
	var in struct {
		BaseLoadW float64 `json:"base_load_w"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	if in.BaseLoadW < 0 || in.BaseLoadW > 200 {
		middleware.Fail(c, 1001, "参数错误：base_load_w 须为 0–200")
		return
	}
	prof, _ := a.DB.GetProfile(id)
	if prof == nil || !prof.PowerRapL {
		middleware.Fail(c, 1001, "该节点未暴露 RAPL 功率计接口，无法校准")
		return
	}
	if err := a.DB.UpdatePowerCalibration(id, in.BaseLoadW, "manual"); err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	a.audit(a.actorOf(c), "power_calibrate", "server:"+itoa(id),
		"设置节点 "+srv.Name+" 基础功耗 "+ftoa(in.BaseLoadW)+"W", ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// POST /api/v1/admin/servers/:id/locate（强制重新定位；仅 auto 覆盖 region）
func (a *App) Relocate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		middleware.Fail(c, 1001, "参数错误：id 非法")
		return
	}
	srv, _ := a.DB.GetServer(id)
	if srv == nil {
		middleware.Fail(c, 2002, "节点不存在")
		return
	}
	cred, _ := a.DB.GetCredential(id)
	prof, _ := a.DB.GetProfile(id)
	ip := ""
	if prof != nil && prof.PublicIP != "" {
		ip = prof.PublicIP
	} else if cred != nil {
		ip = cred.Host
	}
	r := geoip.Lookup(ip)
	region := geoip.RegionText(r)
	source := "auto"
	if r != nil && r.Country == "LAN" {
		source = "manual"
	}
	if srv.RegionSource == "auto" {
		_ = a.DB.UpdateRegion(id, region, source)
		srv.Region, srv.RegionSource = region, source
	}
	a.audit(a.actorOf(c), "server_locate", "server:"+itoa(id),
		"重新定位节点 "+srv.Name+" → "+region, ipOf(c))
	middleware.OK(c, gin.H{"ok": true, "region": srv.Region, "region_source": srv.RegionSource})
}

// ---------- 试连 ----------

// POST /api/v1/admin/servers/test（30s 超时由前端控制；后端 8s SSH 超时 + 解析）
func (a *App) TestConnection(c *gin.Context) {
	var in struct {
		SSH sshInput `json:"ssh"`
		// 兼容直接平铺 {host,port,...}
		Host       string `json:"host"`
		Port       int    `json:"port"`
		Username   string `json:"username"`
		AuthType   string `json:"auth_type"`
		Password   string `json:"password"`
		PrivateKey string `json:"private_key"`
		Passphrase string `json:"passphrase"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	sshIn := in.SSH
	if sshIn.Host == "" {
		sshIn = sshInput{Host: in.Host, Port: in.Port, Username: in.Username,
			AuthType: in.AuthType, Password: in.Password,
			PrivateKey: in.PrivateKey, Passphrase: in.Passphrase}
	}
	port, msg := validateSSH(sshIn, true)
	if msg != "" {
		middleware.Fail(c, 1001, msg)
		return
	}
	cred := &collector.SSHCred{
		Host: strings.TrimSpace(sshIn.Host), Port: port,
		Username: strings.TrimSpace(sshIn.Username), AuthType: sshIn.AuthType,
		Password: sshIn.Password, PrivateKey: sshIn.PrivateKey, Passphrase: sshIn.Passphrase,
	}
	settings, _ := a.DB.GetSettings()
	strict := settings["strict_host_key"] == "true"
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	res, err := collector.DialAndCollect(ctx, cred, "", strict)
	if err != nil {
		middleware.Fail(c, 2001, err.Error())
		return
	}
	raw := res.Raw
	geo := geoip.Lookup(raw.PubIP)
	middleware.OK(c, gin.H{
		"ok": true, "latency_ms": res.LatencyMs,
		"profile": gin.H{
			"hostname": raw.Hostname, "os_name": raw.OsName, "os_version": raw.OsVer,
			"kernel": raw.Kernel, "arch": raw.Arch, "cpu_model": raw.CpuModel,
			"cpu_cores": raw.CpuCores, "mem_total": raw.MemTotal, "disk_total": raw.DiskTotal,
			"virt": raw.Virt,
			"power": gin.H{"rapl": raw.HasRapl, "battery": raw.HasBat,
				"base_load_w": 0, "base_load_source": "default"},
		},
		"geo": gin.H{"public_ip": raw.PubIP, "country": geo.Country,
			"city": geo.City, "region_text": geoip.RegionText(geo)},
		"host_key_fp": res.HostKeyFP,
	})
}

// applyProbe 保存试连画像（新建/编辑时 probe 透传）。
func (a *App) applyProbe(serverID int64, pb *probeInput, regionSource string) {
	p := &store.Profile{
		ServerID: serverID, Hostname: pb.Profile.Hostname,
		OsName: pb.Profile.OsName, OsVersion: pb.Profile.OsVersion,
		Kernel: pb.Profile.Kernel, Arch: pb.Profile.Arch,
		CpuModel: pb.Profile.CpuModel, CpuCores: pb.Profile.CpuCores,
		MemTotal: pb.Profile.MemTotal, DiskTotal: pb.Profile.DiskTotal,
		Virt: pb.Profile.Virt, BaseLoadSource: "default",
	}
	if pb.Geo != nil {
		p.PublicIP, p.GeoCountry, p.GeoCity = pb.Geo.PublicIP, pb.Geo.Country, pb.Geo.City
		if regionSource == "auto" {
			// region 已在 CreateServer 写入，此处不再覆盖
		}
	}
	if pb.Profile.Power != nil {
		p.PowerRapL, p.PowerBattery = pb.Profile.Power.Rapl, pb.Profile.Power.Battery
		p.BaseLoadW = pb.Profile.Power.BaseLoadW
		if pb.Profile.Power.BaseLoadSource != "" {
			p.BaseLoadSource = pb.Profile.Power.BaseLoadSource
		}
	}
	_ = a.DB.UpsertProfile(p, nowUnix())
	_ = a.DB.SetCollectResult(serverID, pb.HostKeyFP, "", nowUnix())
}

// ---------- 设置 / 审计 ----------

// GET /api/v1/admin/settings
func (a *App) GetSettings(c *gin.Context) {
	s, err := a.DB.GetSettings()
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	middleware.OK(c, settingsView(s))
}

// PUT /api/v1/admin/settings
func (a *App) SaveSettings(c *gin.Context) {
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		middleware.Fail(c, 1001, "参数错误：请求体须为 JSON")
		return
	}
	cur, _ := a.DB.GetSettings()
	next := map[string]string{}
	for k, v := range cur {
		next[k] = v
	}
	get := func(key string) (any, bool) { v, ok := in[key]; return v, ok }
	if v, ok := get("site_title"); ok {
		s := strings.TrimSpace(toStr(v))
		if s == "" || len(s) > 64 {
			middleware.Fail(c, 1001, "参数错误：site_title 须为 1–64 字符")
			return
		}
		next["site_title"] = s
	}
	if v, ok := get("interval_s"); ok {
		n := toInt(v)
		if n < 5 || n > 3600 {
			middleware.Fail(c, 1001, "参数错误：interval_s 须为 5–3600")
			return
		}
		next["interval_s"] = itoa(int64(n))
		a.Cfg.CollectInterval = n
	}
	if v, ok := get("retention_days"); ok {
		n := toInt(v)
		if n < 1 || n > 365 {
			middleware.Fail(c, 1001, "参数错误：retention_days 须为 1–365")
			return
		}
		next["retention_days"] = itoa(int64(n))
	}
	for _, k := range []string{"open_7d_history", "show_power_public", "show_cost_public", "strict_host_key", "private_mode"} {
		if v, ok := get(k); ok {
			next[k] = boolStr(toBool(v))
		}
	}
	if v, ok := get("electric_price"); ok {
		f := toFloat(v)
		if f < 0 || f > 99 {
			middleware.Fail(c, 1001, "参数错误：electric_price 须为 0–99")
			return
		}
		next["electric_price"] = ftoa(f)
	}
	if err := a.DB.SaveSettings(next); err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	a.audit(a.actorOf(c), "settings", "panel", "更新站点设置", ipOf(c))
	middleware.OK(c, gin.H{"ok": true})
}

// GET /api/v1/admin/audit?page=&size=
func (a *App) ListAudit(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	total, items, err := a.DB.ListAudit(page, size)
	if err != nil {
		middleware.AbortCode(c, http.StatusOK, 5000, "服务器内部错误")
		return
	}
	rows := []map[string]any{}
	for _, e := range items {
		rows = append(rows, map[string]any{
			"id": e.ID, "ts": e.Ts, "actor": e.Actor, "action": e.Action,
			"target": e.Target, "detail": e.Detail, "source_ip_hash": e.SourceIPHash,
		})
	}
	middleware.OK(c, gin.H{"total": total, "items": rows})
}

// settingsView DB map → 前端 settings 形态（与 mock seedSettings 同键）。
func settingsView(s map[string]string) map[string]any {
	return map[string]any{
		"site_title": s["site_title"], "interval_s": toInt(s["interval_s"]),
		"retention_days":    toInt(s["retention_days"]),
		"open_7d_history":   s["open_7d_history"] == "true",
		"show_power_public": s["show_power_public"] == "true",
		"show_cost_public":  s["show_cost_public"] == "true",
		"electric_price":    toFloat(s["electric_price"]),
		"strict_host_key":   s["strict_host_key"] == "true",
		"private_mode":      s["private_mode"] == "true",
	}
}
