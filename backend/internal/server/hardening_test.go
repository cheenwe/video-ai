package server

import (
	"encoding/json"
	"fmt"
	"github.com/pquerna/otp/totp"
	"strings"
	"testing"
	"time"
	"baseplatform/internal/database"
	"baseplatform/internal/models"
	"baseplatform/internal/security"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"
)

func auditUser(t *testing.T, f *fixture, name, role string) models.User {
	t.Helper()
	hash, _ := security.HashPassword("Auditpass123")
	u := models.User{Username: name, Role: role, PasswordHash: hash, WebLoginEnabled: 1}
	if err := f.db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}
func TestPermissionEnforcementAndSetupGate(t *testing.T) {
	f := setup(t)
	u := auditUser(t, f, "reviewmanager", "admin")
	f.cookie = f.login(u.Username, "Auditpass123")
	var role models.Role
	f.db.Where("code = ?", "admin").First(&role)
	rbac := services.NewRBACService(f.db)
	if err := rbac.AssignRolePermissions(role.ID, nil); err != nil {
		t.Fatal(err)
	}
	e, _ := f.call("GET", "/api/users", nil, f.cookie, "")
	if e.Code != 403 {
		t.Fatalf("cleared permissions not enforced: %+v", e)
	}
	if err := rbac.SeedBuiltinRoles(); err != nil {
		t.Fatal(err)
	}
	if rbac.HasPermission(u, "user:list") {
		t.Fatal("restart reset administrator grants")
	}
	rbac.AssignRolePermissions(role.ID, []string{"user:list"})
	f.db.Model(&u).Update("must_change_password", 1)
	e, _ = f.call("GET", "/api/users", nil, f.cookie, "")
	if e.Code != 403 {
		t.Fatal("pending password change bypassed")
	}
	f.ok("GET", "/api/auth/me", nil)
	f.ok("POST", "/api/auth/change-password", map[string]string{"old_password": "Auditpass123", "new_password": "Changedpass123"})
	e, _ = f.call("GET", "/api/auth/me", nil, f.cookie, "")
	if e.Code != 403 && e.Code != 401 {
		t.Fatal("old session survived password change")
	}
}
func TestEntityAuthorizationAndCustomRole(t *testing.T) {
	f := setup(t)
	u := auditUser(t, f, "reviewuser", "user")
	f.cookie = f.login(u.Username, "Auditpass123")
	def := models.CustomFieldDef{Module: "user", FieldCode: "note", FieldName: "Note", FieldType: "text", Enabled: 1}
	f.db.Create(&def)
	payload := map[string]any{"values": []map[string]any{{"field_id": def.ID, "value": "test"}}}
	e, _ := f.call("PUT", "/api/cf-data/user/1", payload, f.cookie, "http://example.com")
	if e.Code != 403 {
		t.Fatal("cross-user write allowed")
	}
	e, _ = f.call("GET", "/api/cf-data/user/batch?entity_ids=1", nil, f.cookie, "")
	if e.Code != 403 {
		t.Fatal("batch read leaked another user")
	}
	f.ok("PUT", fmt.Sprintf("/api/cf-data/user/%d", u.ID), payload)
	role := models.Role{Code: "project_reviewer", Name: "reviewer", DataScope: "project", Enabled: 1}
	f.db.Create(&role)
	services.NewRBACService(f.db).AssignRolePermissions(role.ID, []string{"project:list", "project:update"})
	f.db.Create(&models.UserRole{UserID: u.ID, RoleID: role.ID})
	owner := uint(1)
	a := models.Project{Code: "allowed", Name: "allowed", OwnerID: &owner, Enabled: 1}
	b := models.Project{Code: "other", Name: "other", OwnerID: &owner, Enabled: 1}
	f.db.Create(&a)
	f.db.Create(&b)
	f.db.Create(&models.ProjectMember{UserID: u.ID, ProjectID: a.ID, Role: "member"})
	e = f.ok("GET", "/api/projects", nil)
	var page struct {
		Total int `json:"total"`
	}
	json.Unmarshal(e.Data, &page)
	if page.Total != 1 {
		t.Fatalf("project membership scope: %s", e.Data)
	}
	f.ok("PUT", fmt.Sprintf("/api/projects/%d", a.ID), map[string]string{"name": "renamed"})
	e, _ = f.call("PUT", fmt.Sprintf("/api/projects/%d", b.ID), map[string]string{"name": "denied"}, f.cookie, "http://example.com")
	if e.Code != 403 {
		t.Fatal("cross-project edit allowed")
	}
	f.db.Model(&role).Update("enabled", 0)
	e, _ = f.call("GET", "/api/projects", nil, f.cookie, "")
	if e.Code != 403 {
		t.Fatal("disabled role still grants permissions")
	}
}
func TestTOTPProtection(t *testing.T) {
	f := setup(t)
	u := auditUser(t, f, "totpuser", "user")
	secret, uri, err := services.GenerateTOTPSecret("Test", "totpuser")
	if err != nil {
		t.Fatal(err)
	}
	if qr := services.QRCodeURL(uri); !strings.HasPrefix(qr, "data:image/png;base64,") {
		t.Fatal("QR is not generated locally")
	}
	f.db.Model(&u).Updates(map[string]any{"totp_enabled": 1, "totp_secret": secret, "session_version": 3})
	e, _ := f.call("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "Auditpass123"}, nil, "")
	var pending struct {
		Temp string `json:"temp_token"`
	}
	json.Unmarshal(e.Data, &pending)
	if pending.Temp == "" {
		t.Fatal("missing challenge")
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	body := map[string]string{"temp_token": pending.Temp, "code": code}
	e, _ = f.call("POST", "/api/auth/totp/verify", body, nil, "")
	if e.Success != 1 {
		t.Fatalf("valid TOTP failed: %+v", e)
	}
	e, _ = f.call("POST", "/api/auth/totp/verify", body, nil, "")
	if e.Success == 1 {
		t.Fatal("replayed challenge accepted")
	}
	f.cookie = f.login("admin", "Adminpass123")
	sec := settings.DefaultSecurity()
	sec.Admin2FAEnabled = true
	settings.SaveSection(f.db, "security", sec, nil)
	e, _ = f.call("GET", "/api/users", nil, f.cookie, "")
	if e.Code != 403 {
		t.Fatal("required enrollment not enforced")
	}
	e, _ = f.call("POST", "/api/auth/totp/disable", map[string]string{"code": "123456"}, f.cookie, "http://example.com")
	if e.Code != 403 {
		t.Fatal("forced TOTP disable allowed")
	}
}
func TestBootstrapNoSharedPassword(t *testing.T) {
	f := setup(t)
	if err := f.db.Where("1 = 1").Delete(&models.User{}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASE_ADMIN_PASSWORD", "")
	if err := database.BootstrapAdmin(f.db); err != nil {
		t.Fatal(err)
	}
	var users []models.User
	f.db.Find(&users)
	if len(users) != 1 || users[0].MustChangePassword != 1 {
		t.Fatal("unexpected bootstrap accounts")
	}
	if !security.VerifyPassword(database.DefaultAdminPassword, users[0].PasswordHash) {
		t.Fatal("default admin password mismatch")
	}
	if err := f.db.Where("1 = 1").Delete(&models.User{}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASE_ADMIN_PASSWORD", "short")
	if database.BootstrapAdmin(f.db) == nil {
		t.Fatal("initial password skipped policy")
	}
	t.Setenv("BASE_ADMIN_PASSWORD", "Initialpass123")
	if err := database.BootstrapAdmin(f.db); err != nil {
		t.Fatal(err)
	}
}
