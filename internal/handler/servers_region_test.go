package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Yoahoug/BeaconTower/internal/store"
)

// 「未定位」占位文案守卫：创建/编辑节点都不允许把它以 manual 落库——
// 落库即永久跳过自动定位（采集自愈与「重定位」都只认 auto），doc/17 §1.2。
func TestUpdateServerPlaceholderRegionResetsAuto(t *testing.T) {
	app := newWGProvisionTestApp(t)
	now := time.Now().Unix()
	id, err := app.DB.CreateServer(&store.Server{
		Name: "Rack", Region: "未定位", RegionSource: "manual", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	// 模拟前端编辑保存把预填的「未定位」原样回传
	_, resp := callWG(t, app.UpdateServer, "PUT", "/api/v1/admin/servers/1",
		gin.Params{{Key: "id", Value: "1"}}, `{"region":"未定位"}`)
	if code := resp["code"].(float64); code != 0 {
		t.Fatalf("更新应成功，得到 %v（%v）", code, resp["msg"])
	}
	srv, _ := app.DB.GetServer(id)
	if srv.Region != "" || srv.RegionSource != "auto" {
		t.Fatalf("占位文案应被清掉并交还自动定位: region=%q source=%q", srv.Region, srv.RegionSource)
	}
}

func TestUpdateServerManualRegionKept(t *testing.T) {
	app := newWGProvisionTestApp(t)
	now := time.Now().Unix()
	id, err := app.DB.CreateServer(&store.Server{Name: "香港机", Region: "", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	_, resp := callWG(t, app.UpdateServer, "PUT", "/api/v1/admin/servers/1",
		gin.Params{{Key: "id", Value: "1"}}, `{"region":"香港"}`)
	if code := resp["code"].(float64); code != 0 {
		t.Fatalf("更新应成功，得到 %v（%v）", code, resp["msg"])
	}
	srv, _ := app.DB.GetServer(id)
	if srv.Region != "香港" || srv.RegionSource != "manual" {
		t.Fatalf("人工指定应保留为 manual: region=%q source=%q", srv.Region, srv.RegionSource)
	}
}

// 重定位：解析成功即覆盖 auto 节点的 region，并回填画像 geo（Rack 的一次点击修复路径）。
// 用内网地址走「内网直返」分支，避免单测依赖外网回显源（文档网段会被在线源解析成真实归属）。
func TestRelocateBackfillsRegionAndProfileGeo(t *testing.T) {
	app := newWGProvisionTestApp(t)
	now := time.Now().Unix()
	id, err := app.DB.CreateServer(&store.Server{Name: "内网侧节点", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.DB.UpdateRegion(id, "", "auto"); err != nil {
		t.Fatal(err)
	}
	if err := app.DB.UpsertProfile(&store.Profile{
		ServerID: id, PublicIP: "192.168.1.50", BaseLoadSource: "default"}, 0); err != nil {
		t.Fatal(err)
	}
	_, resp := callWG(t, app.Relocate, "POST", "/api/v1/admin/servers/1/locate",
		gin.Params{{Key: "id", Value: "1"}}, ``)
	data := respData(t, resp)
	if data["region"] != "内网" || data["region_source"] != "manual" || data["updated"] != true {
		t.Fatalf("auto 节点应被覆盖为解析结果（内网按设计钉为 manual）: %v", data)
	}
	srv, _ := app.DB.GetServer(id)
	if srv.Region != "内网" {
		t.Fatalf("region 未落库: %q", srv.Region)
	}
	prof, _ := app.DB.GetProfile(id)
	if prof == nil || prof.GeoCountry != "LAN" {
		t.Fatalf("画像 geo 未回填: %+v", prof)
	}
}

// 重定位对 manual 节点：保留人工标签（updated=false），但画像 geo 仍回填
// （原始归属数据，与展示标签的取舍无关）。
func TestRelocateManualKeepsRegionButBackfillsGeo(t *testing.T) {
	app := newWGProvisionTestApp(t)
	now := time.Now().Unix()
	id, err := app.DB.CreateServer(&store.Server{
		Name: "Rack", Region: "美国", RegionSource: "manual", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.DB.UpsertProfile(&store.Profile{
		ServerID: id, PublicIP: "192.168.1.51", BaseLoadSource: "default"}, 0); err != nil {
		t.Fatal(err)
	}
	_, resp := callWG(t, app.Relocate, "POST", "/api/v1/admin/servers/1/locate",
		gin.Params{{Key: "id", Value: "1"}}, ``)
	data := respData(t, resp)
	if data["updated"] != false || data["region"] != "美国" {
		t.Fatalf("manual 节点不应被覆盖: %v", data)
	}
	prof, _ := app.DB.GetProfile(id)
	if prof == nil || prof.GeoCountry != "LAN" {
		t.Fatalf("画像 geo 应照常回填: %+v", prof)
	}
}

// 解析不出来必须显式报错且不写任何值——旧实现会把占位文案「未定位」写进库。
// 用非法 IP（主机名）触发解析链全败，不依赖外网。
func TestRelocateUnresolvableFailsWithoutWriting(t *testing.T) {
	app := newWGProvisionTestApp(t)
	now := time.Now().Unix()
	id, err := app.DB.CreateServer(&store.Server{
		Name: "Rack", Region: "未定位", RegionSource: "auto", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	setCred(t, app, id, "rack.example.invalid")
	_, resp := callWG(t, app.Relocate, "POST", "/api/v1/admin/servers/1/locate",
		gin.Params{{Key: "id", Value: "1"}}, ``)
	if code := resp["code"].(float64); code == 0 {
		t.Fatalf("解析失败不应报成功: %v", resp)
	}
	if msg, _ := resp["msg"].(string); !strings.Contains(msg, "定位失败") {
		t.Fatalf("应给出可照做的失败文案: %v", resp["msg"])
	}
	srv, _ := app.DB.GetServer(id)
	if srv.Region != "未定位" {
		t.Fatalf("失败时不应改写 region: %q", srv.Region)
	}
}
