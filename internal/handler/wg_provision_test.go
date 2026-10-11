package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/Yoahoug/BeaconTower/internal/wg"
)

// 一键配置中心 / 重新签发凭证的接口级用例：只断言 handler 同步落库与鉴权分支，
// 真正的 SSH 链路在后台任务里跑（这里的目标机地址不可达，任务会失败，不影响断言）。

const provMaster = "0123456789abcdef0123456789abcdef"

func newWGProvisionTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "wg-prov.db"))
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	key := []byte(provMaster)
	return &App{DB: db, Cfg: &config.Config{SiteTitle: "BeaconTower"},
		Master: key, WG: wg.NewRunner(db, key)}
}

// callWG 调 handler 并解包统一响应 {code,msg,data}。
func callWG(t *testing.T, h gin.HandlerFunc, method, path string, params gin.Params, body string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	h(c)
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v (body=%s)", err, w.Body.String())
	}
	return w.Code, resp
}

func respData(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	d, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("响应没有 data 对象: %v", resp)
	}
	return d
}

// seedProvision 造「一台现役 hub + 一台网外新机（带 SSH 凭据）」的最小现场。
func seedProvision(t *testing.T, app *App) (hubID, freshID int64) {
	t.Helper()
	now := time.Now().Unix()
	var err error
	hubID, err = app.DB.CreateServer(&store.Server{Name: "阿里云-My", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatalf("建 hub 节点: %v", err)
	}
	freshID, err = app.DB.CreateServer(&store.Server{Name: "阿里云-Z", Enabled: true, CreatedAt: now})
	if err != nil {
		t.Fatalf("建新机节点: %v", err)
	}
	kp, err := wg.GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成 hub 密钥: %v", err)
	}
	if err := app.DB.EnsureWGNetwork(&store.WGNetwork{
		Subnet: "10.66.66.0/24", HubIP: "10.66.66.2", Iface: "wg0",
		Keepalive: 25, MTU: 1420, ActiveHubServerID: hubID, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("建网络: %v", err)
	}
	if err := app.DB.UpsertWGHub(&store.WGHub{
		ServerID: hubID, ListenPort: 51820, PublicKey: kp.Public,
		PrivateKeyEnc: app.WG.Encrypt(kp.Private), Endpoint: "47.10.0.1:51820", Status: "ok",
	}); err != nil {
		t.Fatalf("建 hub 槽位: %v", err)
	}
	setCred(t, app, freshID, "192.0.2.9")
	return hubID, freshID
}

func setCred(t *testing.T, app *App, serverID int64, host string) {
	t.Helper()
	pw, _ := crypto.EncryptString([]byte(provMaster), "secret")
	if err := app.DB.UpsertCredential(&store.Credential{
		ServerID: serverID, Host: host, Port: 1, Username: "root",
		AuthType: "password", PasswordEnc: pw,
	}); err != nil {
		t.Fatalf("写凭据: %v", err)
	}
}

// 新机一键配置：落 hub 槽位（含新建密钥）、任务角色 standby、非重下发。
func TestWGHubProvisionFreshMachine(t *testing.T) {
	app := newWGProvisionTestApp(t)
	hubID, freshID := seedProvision(t, app)

	_, resp := callWG(t, app.WGHubProvision, http.MethodPost, "/api/v1/admin/wg/hubs/provision",
		nil, `{"server_id":`+itoa(freshID)+`}`)
	if code := resp["code"].(float64); code != 0 {
		t.Fatalf("期望成功，得到 %v（%v）", code, resp["msg"])
	}
	data := respData(t, resp)
	if data["role"] != string(wg.RoleStandby) {
		t.Errorf("新机角色应为 standby，得到 %v", data["role"])
	}
	hub, _ := app.DB.GetWGHub(freshID)
	if hub == nil {
		t.Fatal("槽位没有落库")
	}
	if !wg.ValidKey(hub.PublicKey) || len(hub.PrivateKeyEnc) == 0 {
		t.Error("面板没有为新机生成密钥（一键配置的关键前提）")
	}
	if hub.Endpoint == "" {
		t.Error("端点未解析（新机没有画像公网 IP 时应回落到凭据 host）")
	}
	if hub.ServerID == hubID {
		t.Error("应新建槽位而不是复用现役")
	}
	task, _ := app.DB.GetWGTask(int64(data["task_id"].(float64)))
	if task == nil || task.Kind != "provision" {
		t.Fatalf("任务未落库或 kind 不对: %+v", task)
	}
	if strings.Contains(task.Payload, `"reprovision":true`) {
		t.Errorf("全新机器不应是重下发: %s", task.Payload)
	}
	steps, _ := app.DB.ListWGTaskSteps(task.ID)
	if len(steps) != 1 || !strings.HasPrefix(steps[0].Title, "配置备援中心") {
		t.Fatalf("步骤不符合预期: %+v", steps)
	}
}

// 已有槽位（如手工登记过的、或退役后复用）：按重下发处理，保留原密钥。
func TestWGHubProvisionExistingSlotKeepsKeys(t *testing.T) {
	app := newWGProvisionTestApp(t)
	_, freshID := seedProvision(t, app)
	kp, _ := wg.GenerateKeyPair()
	if err := app.DB.UpsertWGHub(&store.WGHub{ServerID: freshID, ListenPort: 51999,
		PublicKey: kp.Public, PrivateKeyEnc: app.WG.Encrypt(kp.Private), Status: "ok"}); err != nil {
		t.Fatalf("预置槽位: %v", err)
	}

	_, resp := callWG(t, app.WGHubProvision, http.MethodPost, "/api/v1/admin/wg/hubs/provision",
		nil, `{"server_id":`+itoa(freshID)+`}`)
	data := respData(t, resp)
	task, _ := app.DB.GetWGTask(int64(data["task_id"].(float64)))
	if !strings.Contains(task.Payload, `"reprovision":true`) {
		t.Errorf("已存在槽位应标记重下发: %s", task.Payload)
	}
	hub, _ := app.DB.GetWGHub(freshID)
	if hub.PublicKey != kp.Public {
		t.Error("重下发不该换掉现有身份密钥")
	}
	if hub.ListenPort != 51999 {
		t.Errorf("未显式改端口时应沿用槽位端口，得到 %d", hub.ListenPort)
	}
}

// 拒绝分支：已在网内的成员、无 SSH 凭据、本机节点。
func TestWGHubProvisionGuards(t *testing.T) {
	app := newWGProvisionTestApp(t)
	_, freshID := seedProvision(t, app)
	now := time.Now().Unix()

	// 已是网内成员（kind=server 的 peer 记录）→ 必须先移出
	if _, err := app.DB.InsertWGPeer(&store.WGPeer{Kind: "server",
		ServerID: sql.NullInt64{Int64: freshID, Valid: true}, Name: "阿里云-Z",
		WgIP: "10.66.66.9", PublicKey: "pubkeyZZZZ", Managed: true,
		Status: "online", CreatedAt: now}); err != nil {
		t.Fatalf("预置成员: %v", err)
	}
	_, resp := callWG(t, app.WGHubProvision, http.MethodPost, "/api/v1/admin/wg/hubs/provision",
		nil, `{"server_id":`+itoa(freshID)+`}`)
	if code := resp["code"].(float64); code == 0 {
		t.Error("网内成员不应被允许再配置为中心")
	} else if !strings.Contains(resp["msg"].(string), "移出") {
		t.Errorf("提示应指向「移出」这条路: %v", resp["msg"])
	}

	// 无凭据的新机
	other, _ := app.DB.CreateServer(&store.Server{Name: "无凭据机", Enabled: true, CreatedAt: now})
	_, resp = callWG(t, app.WGHubProvision, http.MethodPost, "/api/v1/admin/wg/hubs/provision",
		nil, `{"server_id":`+itoa(other)+`}`)
	if code := resp["code"].(float64); code == 0 || !strings.Contains(resp["msg"].(string), "SSH") {
		t.Errorf("无凭据应被拦下并提示录凭据: %v", resp)
	}

	// 本机节点
	self, _ := app.DB.CreateServer(&store.Server{Name: "本机", Enabled: true, IsSelf: true, CreatedAt: now})
	_, resp = callWG(t, app.WGHubProvision, http.MethodPost, "/api/v1/admin/wg/hubs/provision",
		nil, `{"server_id":`+itoa(self)+`}`)
	if code := resp["code"].(float64); code == 0 {
		t.Error("本机不能作为中心节点")
	}

	// 未初始化组网（清掉网络后）
	if err := app.DB.EnsureWGNetwork(&store.WGNetwork{}); err != nil {
		t.Fatalf("重置网络: %v", err)
	}
	if _, err := app.DB.SQL.Exec(`DELETE FROM wg_network`); err != nil {
		t.Fatalf("删网络: %v", err)
	}
	_, resp = callWG(t, app.WGHubProvision, http.MethodPost, "/api/v1/admin/wg/hubs/provision",
		nil, `{"server_id":`+itoa(other)+`}`)
	if code := resp["code"].(float64); code == 0 || !strings.Contains(resp["msg"].(string), "尚未初始化") {
		t.Errorf("未初始化应被拦下: %v", resp)
	}
}

// 导入设备（面板无私钥）重新签发：换新密钥、转面板托管、返回可直接出码的 conf。
func TestWGPeerReissueImportedDevice(t *testing.T) {
	app := newWGProvisionTestApp(t)
	seedProvision(t, app)
	now := time.Now().Unix()
	imported, err := app.DB.InsertWGPeer(&store.WGPeer{Kind: "device", Name: "iPhone",
		WgIP: "10.66.66.13", PublicKey: "importedPubKey", Managed: false,
		Status: "offline", CreatedAt: now})
	if err != nil {
		t.Fatalf("预置导入设备: %v", err)
	}

	_, resp := callWG(t, app.WGPeerReissue, http.MethodPost,
		"/api/v1/admin/wg/peers/"+itoa(imported)+"/reissue", gin.Params{{Key: "id", Value: itoa(imported)}}, `{}`)
	if code := resp["code"].(float64); code != 0 {
		t.Fatalf("期望成功，得到 %v（%v）", code, resp["msg"])
	}
	data := respData(t, resp)
	if data["rotated"] != true {
		t.Error("导入成员应换新密钥")
	}
	p, _ := app.DB.GetWGPeer(imported)
	if !p.Managed || len(p.PrivateKeyEnc) == 0 {
		t.Error("没有转成面板托管")
	}
	if p.PublicKey == "importedPubKey" {
		t.Error("公钥没有轮换")
	}
	if p.WgIP != "10.66.66.13" {
		t.Errorf("重新签发必须保留原 WG IP，得到 %s", p.WgIP)
	}
	conf, _ := data["conf"].(string)
	// 客户端 conf 里没有自己的公钥：它带的是本机私钥与中心公钥/端点
	if !strings.Contains(conf, "PrivateKey = ") || !strings.Contains(conf, "Endpoint = 47.10.0.1:51820") {
		t.Errorf("返回的 conf 不是新凭证（前端要靠它直接出二维码）: %s", conf)
	}
	priv, _ := app.WG.DecryptBlob(p.PrivateKeyEnc)
	if !strings.Contains(conf, priv) {
		t.Error("conf 里的私钥与库中不一致")
	}
	// 中心不可达：逐台记账返回，不能让整个请求失败
	if data["synced"].(float64) != 0 || data["total"].(float64) != 1 {
		t.Errorf("结果计数不对: synced=%v total=%v", data["synced"], data["total"])
	}
}

// 已托管成员再次调用 = 重新下发（不换钥），且服务器成员被拒。
func TestWGPeerReissueManagedAndServerGuard(t *testing.T) {
	app := newWGProvisionTestApp(t)
	seedProvision(t, app)
	now := time.Now().Unix()
	kp, _ := wg.GenerateKeyPair()
	psk, _ := wg.GeneratePSK()
	managed, err := app.DB.InsertWGPeer(&store.WGPeer{Kind: "device", Name: "Mac",
		WgIP: "10.66.66.11", PublicKey: kp.Public, PrivateKeyEnc: app.WG.Encrypt(kp.Private),
		PskEnc: app.WG.Encrypt(psk), Managed: true, Status: "online", CreatedAt: now})
	if err != nil {
		t.Fatalf("预置托管设备: %v", err)
	}
	_, resp := callWG(t, app.WGPeerReissue, http.MethodPost,
		"/api/v1/admin/wg/peers/"+itoa(managed)+"/reissue", gin.Params{{Key: "id", Value: itoa(managed)}}, `{}`)
	data := respData(t, resp)
	if data["rotated"] != false {
		t.Error("已托管成员不该换钥")
	}
	p, _ := app.DB.GetWGPeer(managed)
	if p.PublicKey != kp.Public {
		t.Error("公钥被意外改动")
	}

	srvPeer, err := app.DB.InsertWGPeer(&store.WGPeer{Kind: "server", Name: "成员机",
		WgIP: "10.66.66.5", PublicKey: "srvPub", Managed: true, Status: "online", CreatedAt: now})
	if err != nil {
		t.Fatalf("预置服务器成员: %v", err)
	}
	_, resp = callWG(t, app.WGPeerReissue, http.MethodPost,
		"/api/v1/admin/wg/peers/"+itoa(srvPeer)+"/reissue", gin.Params{{Key: "id", Value: itoa(srvPeer)}}, `{}`)
	if code := resp["code"].(float64); code == 0 {
		t.Error("服务器成员不应允许重新签发")
	}
}
