package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"baseplatform/internal/config"
	"baseplatform/internal/database"
	"baseplatform/internal/models"
	"baseplatform/internal/security"
	"baseplatform/internal/settings"
)

type envelope struct {
	Success int             `json:"success"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Msg     string          `json:"msg"`
}
type fixture struct {
	r      http.Handler
	db     *gorm.DB
	cookie *http.Cookie
	t      *testing.T
}

func setup(t *testing.T) *fixture {
	t.Helper()
	cfg := &config.Config{DataHome: t.TempDir(), SecretKey: "test-only-secret-with-at-least-32-bytes", JWTAlgorithm: "HS256", JWTExpireDays: 1}
	db, e := database.Open(cfg)
	if e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	hash, _ := security.HashPassword("Adminpass123")
	if e = db.Create(&models.User{Username: "admin", PasswordHash: hash, Role: "superadmin", WebLoginEnabled: 1, Timezone: "Asia/Shanghai"}).Error; e != nil {
		t.Fatal(e)
	}
	return &fixture{r: New(cfg, db, ""), db: db, t: t}
}
func (f *fixture) call(method, path string, body any, cookie *http.Cookie, origin string) (envelope, *httptest.ResponseRecorder) {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.r.ServeHTTP(w, req)
	var e envelope
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		f.t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
	}
	return e, w
}
func (f *fixture) login(name, pwd string) *http.Cookie {
	f.t.Helper()
	e, w := f.call("POST", "/api/auth/login", map[string]string{"username": name, "password": pwd}, nil, "")
	if e.Success != 1 {
		f.t.Fatal(e)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "baseplatform_session" {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				f.t.Fatal("unsafe cookie")
			}
			return c
		}
	}
	f.t.Fatal("missing cookie")
	return nil
}
func (f *fixture) ok(method, path string, body any) envelope {
	f.t.Helper()
	e, _ := f.call(method, path, body, f.cookie, "http://example.com")
	if e.Success != 1 {
		f.t.Fatalf("%s %s: %s", method, path, e.Msg)
	}
	return e
}
func TestMigratedAdministrationFlow(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")
	e := f.ok("POST", "/api/departments", map[string]any{"name": "现场运维", "parent_id": 0})
	var dept struct {
		ID string `json:"id"`
	}
	json.Unmarshal(e.Data, &dept)
	var deptID uint
	fmt.Sscan(dept.ID, &deptID)
	e = f.ok("POST", "/api/users", map[string]any{"username": "operator", "email": "operator@example.test", "password": "Operator123", "department_id": deptID})
	var user struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(e.Data, &user)
	member := f.login("operator", "Operator123")
	e, w := f.call("GET", "/api/users", nil, member, "")
	if w.Code != 403 || e.Success == 1 {
		t.Fatal("ordinary user accessed administration")
	}
	e, w = f.call("PUT", "/api/auth/profile", map[string]string{"real_name": "攻击"}, member, "https://other.example")
	if w.Code != 403 {
		t.Fatal("cross-origin write allowed")
	}
	f.ok("PUT", "/api/users/"+fmt.Sprint(user.ID), map[string]any{"role": "admin", "department_id": 0})
	e, _ = f.call("GET", "/api/auth/me", nil, member, "")
	if e.Success == 1 {
		t.Fatal("old role session remains valid")
	}
	member = f.login("operator", "Operator123")
	f.ok("POST", "/api/users/"+fmt.Sprint(user.ID)+"/disabled", map[string]bool{"disabled": true})
	e, _ = f.call("GET", "/api/auth/me", nil, member, "")
	if e.Success == 1 {
		t.Fatal("disabled session remains valid")
	}
	f.ok("POST", "/api/sys-configs", map[string]string{"category": "video", "code": "name", "value": "坑边监测"})
	f.ok("GET", "/api/sys-logs?category=audit", nil)
	e, _ = f.call("POST", "/api/sys-configs", map[string]string{"category": "settings", "code": "security", "value": "{}"}, f.cookie, "http://example.com")
	if e.Success == 1 {
		t.Fatal("reserved settings bypass")
	}
	f.ok("DELETE", "/api/departments/"+dept.ID, nil)
	f.ok("POST", "/api/auth/logout", nil)
	e, _ = f.call("GET", "/api/auth/me", nil, f.cookie, "")
	if e.Code != 401 {
		t.Fatal("logout failed to revoke session")
	}
}
func TestDepartmentCycleAndPasswordRevocation(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")
	a := f.ok("POST", "/api/departments", map[string]any{"name": "A"})
	var x struct {
		ID string `json:"id"`
	}
	json.Unmarshal(a.Data, &x)
	var aid uint
	fmt.Sscan(x.ID, &aid)
	b := f.ok("POST", "/api/departments", map[string]any{"name": "B", "parent_id": aid})
	json.Unmarshal(b.Data, &x)
	var bid uint
	fmt.Sscan(x.ID, &bid)
	e, _ := f.call("PUT", fmt.Sprintf("/api/departments/%d", aid), map[string]any{"parent_id": bid}, f.cookie, "http://example.com")
	if e.Success == 1 {
		t.Fatal("cycle accepted")
	}
	other := f.login("admin", "Adminpass123")
	f.ok("POST", "/api/auth/change-password", map[string]string{"old_password": "Adminpass123", "new_password": "Updatedpass456"})
	e, _ = f.call("GET", "/api/auth/me", nil, other, "")
	if e.Success == 1 {
		t.Fatal("other session survived password change")
	}
	f.login("admin", "Updatedpass456")
}
func TestInitializationDoesNotResetExistingAccount(t *testing.T) {
	f := setup(t)
	t.Setenv("BASE_ADMIN_PASSWORD", "Different123")
	if e := database.BootstrapAdmin(f.db); e != nil {
		t.Fatal(e)
	}
	f.login("admin", "Adminpass123")
}

func TestLoginCopyConfiguration(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")
	// Older branding records and partial updates must retain usable defaults.
	if err := settings.SaveSection(f.db, "branding", map[string]any{"site_name": "现场视频平台"}, nil); err != nil {
		t.Fatal(err)
	}
	e := f.ok("PUT", "/api/site/branding", map[string]any{"login_copy": map[string]string{"title": "欢迎登录", "help": ""}})
	var branding struct {
		SiteName string            `json:"site_name"`
		Copy     map[string]string `json:"login_copy"`
	}
	if err := json.Unmarshal(e.Data, &branding); err != nil {
		t.Fatal(err)
	}
	if branding.SiteName != "现场视频平台" || branding.Copy["title"] != "欢迎登录" || branding.Copy["help"] != "" || branding.Copy["submit"] != "登录" {
		t.Fatalf("unexpected branding: %+v", branding)
	}
	for _, copy := range []map[string]string{{"title": " "}, {"unknown": "x"}, {"title": strings.Repeat("文", 501)}} {
		e, _ = f.call("PUT", "/api/site/branding", map[string]any{"login_copy": copy}, f.cookie, "http://example.com")
		if e.Success == 1 {
			t.Fatalf("accepted invalid copy: %v", copy)
		}
	}
	e, _ = f.call("PUT", "/api/site/branding", map[string]any{"login_copy": map[string]string{"title": "unauthorized"}}, nil, "")
	if e.Success == 1 {
		t.Fatal("anonymous update allowed")
	}
	e, _ = f.call("GET", "/api/site/branding", nil, nil, "")
	json.Unmarshal(e.Data, &branding)
	if branding.Copy["title"] != "欢迎登录" {
		t.Fatal("rejected update changed persisted copy")
	}
}
