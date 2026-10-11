package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yoahoug/BeaconTower/internal/config"
	"github.com/Yoahoug/BeaconTower/internal/crypto"
	"github.com/Yoahoug/BeaconTower/internal/store"
	"github.com/gin-gonic/gin"
)

// newServerTestApp 造带 DB + Master 的 App（凭据保存路径不触 SSH）。
func newServerTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	master, err := crypto.MasterKey(t.TempDir(), "")
	if err != nil {
		t.Fatalf("MasterKey: %v", err)
	}
	gin.SetMode(gin.TestMode)
	return &App{DB: db, Cfg: &config.Config{SiteTitle: "BeaconTower"}, Master: master}
}

// callHandler 构造测试请求并调用 handler（带路由参数），返回响应记录器。
func callHandler(a *App, h func(*gin.Context), method, path, body string, params gin.Params) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	h(c)
	return w
}

func respCodeMsg(t *testing.T, w *httptest.ResponseRecorder) (int, string) {
	t.Helper()
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析响应: %v（body: %s）", err, w.Body.String())
	}
	return out.Code, out.Msg
}

// TestCreateServerRejectsKeyWithoutPrivateKey v2.7.0 事故回归：
// auth_type=key 且无私钥（引导标记丢失的形态）必须在落库前拒绝，
// 不允许再造出「key 认证 + 私钥为空」的失联节点。
func TestCreateServerRejectsKeyWithoutPrivateKey(t *testing.T) {
	a := newServerTestApp(t)
	w := callHandler(a, a.CreateServer, http.MethodPost, "/api/v1/admin/servers",
		`{"name":"bad-node","ssh":{"host":"192.0.2.10","port":22,"username":"root","auth_type":"key"}}`, nil)
	code, msg := respCodeMsg(t, w)
	if code != 1001 {
		t.Fatalf("应拒绝（1001），got code=%d body=%s", code, w.Body.String())
	}
	if !strings.Contains(msg, "私钥") {
		t.Fatalf("错误信息应指向私钥缺失: %q", msg)
	}
	// 节点不应被创建
	servers, _ := a.DB.ListServers()
	for _, s := range servers {
		if s.Name == "bad-node" {
			t.Fatalf("坏节点不应落库")
		}
	}
}

// TestCreateServerAcceptsPanelKey 引导成功路径：panel_private_key 存在时
// 归一为 key 认证落库，密码不存。
func TestCreateServerAcceptsPanelKey(t *testing.T) {
	a := newServerTestApp(t)
	body := `{"name":"boot-node","ssh":{"host":"192.0.2.11","port":22,"username":"root",
		"auth_type":"key","password":"should-not-store",
		"panel_private_key":"-----BEGIN OPENSSH PRIVATE KEY-----\\nb3BlbnNzaC1rZXktdjEAAAAA\\n-----END OPENSSH PRIVATE KEY-----\\n"}}`
	w := callHandler(a, a.CreateServer, http.MethodPost, "/api/v1/admin/servers", body, nil)
	code, _ := respCodeMsg(t, w)
	if code != 0 {
		t.Fatalf("引导密钥落库应成功，got code=%d body=%s", code, w.Body.String())
	}
	servers, _ := a.DB.ListServers()
	var id int64
	for _, s := range servers {
		if s.Name == "boot-node" {
			id = s.ID
		}
	}
	if id == 0 {
		t.Fatalf("节点未创建")
	}
	cred, _ := a.DB.GetCredential(id)
	if cred.AuthType != "key" || len(cred.PrivateKeyEnc) == 0 {
		t.Fatalf("应为 key 认证且有私钥: auth=%s keyLen=%d", cred.AuthType, len(cred.PrivateKeyEnc))
	}
	if len(cred.PasswordEnc) != 0 {
		t.Fatalf("密码不应被存储")
	}
}

// TestUpdateServerRejectsKeyFlipWithoutKey 编辑路径同源事故回归：
// 引导标记没送达后端时（ssh.auth_type=key 且无任何私钥），不允许把已有
// 节点翻转成 key 认证——原凭据必须原样保留。
func TestUpdateServerRejectsKeyFlipWithoutKey(t *testing.T) {
	a := newServerTestApp(t)
	id, err := a.DB.CreateServer(&store.Server{Name: "victim", CreatedAt: nowUnix()})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if err := a.DB.UpsertCredential(&store.Credential{
		ServerID: id, Host: "192.0.2.12", Port: 22, Username: "root", AuthType: "password",
	}); err != nil {
		t.Fatalf("UpsertCredential: %v", err)
	}
	// 恶意/异常前端行为：只翻转 auth_type，不带任何私钥
	w := callHandler(a, a.UpdateServer, http.MethodPut, "/api/v1/admin/servers/"+itoa(id),
		`{"ssh":{"auth_type":"key"}}`,
		gin.Params{{Key: "id", Value: itoa(id)}})
	code, _ := respCodeMsg(t, w)
	if code != 1001 {
		t.Fatalf("翻转 key 且无私钥应被拒绝，got code=%d body=%s", code, w.Body.String())
	}
	cred, _ := a.DB.GetCredential(id)
	if cred.AuthType != "password" {
		t.Fatalf("原凭据应保留 password，got %s", cred.AuthType)
	}
}

// TestRotateKeySelfAllowsBrokenCredential 本机节点凭据坏档（key 无私钥）时
// rotate-key 必须放行进入自愈（组网依赖宿主 SSH）；凭据健康时仍拒绝。
func TestRotateKeySelfAllowsBrokenCredential(t *testing.T) {
	a := newServerTestApp(t)
	id, err := a.DB.EnsureSelfServer(nowUnix())
	if err != nil {
		t.Fatalf("EnsureSelfServer: %v", err)
	}
	// 健康凭据（密码认证）：拒绝
	if err := a.DB.UpsertCredential(&store.Credential{
		ServerID: id, Host: "192.168.240.1", Port: 22, Username: "root",
		AuthType: "password",
	}); err != nil {
		t.Fatalf("UpsertCredential: %v", err)
	}
	w := callHandler(a, a.RotateServerKey, http.MethodPost, "/api/v1/admin/servers/"+itoa(id)+"/rotate-key",
		`{"password":"secret"}`, gin.Params{{Key: "id", Value: itoa(id)}})
	code, _ := respCodeMsg(t, w)
	if code != 1001 {
		t.Fatalf("健康凭据不应允许 rotate，got code=%d body=%s", code, w.Body.String())
	}
	// 坏档凭据（key 无私钥）→ 放行进入引导（此处无 SSH 可达，预期 2001 连接失败而非 1001「本机不走 SSH」）
	if err := a.DB.UpsertCredential(&store.Credential{
		ServerID: id, Host: "192.168.240.1", Port: 22, Username: "root",
		AuthType: "key",
	}); err != nil {
		t.Fatalf("UpsertCredential: %v", err)
	}
	w2 := callHandler(a, a.RotateServerKey, http.MethodPost, "/api/v1/admin/servers/"+itoa(id)+"/rotate-key",
		`{"password":"secret"}`, gin.Params{{Key: "id", Value: itoa(id)}})
	code2, msg2 := respCodeMsg(t, w2)
	if code2 == 1001 && strings.Contains(msg2, "本机节点") {
		t.Fatalf("坏档本机凭据应允许自愈，被旧逻辑拒绝: %s", msg2)
	}
}
