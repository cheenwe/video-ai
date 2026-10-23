package services_test

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
	"baseplatform/internal/config"
	"baseplatform/internal/database"
	"baseplatform/internal/models"
	"baseplatform/internal/services"
)

func testDB(t *testing.T) (*gorm.DB, *config.Config) {
	t.Helper()
	cfg := &config.Config{DataHome: t.TempDir(), DBDriver: "sqlite"}
	db, e := database.Open(cfg)
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	return db, cfg
}
func TestBackupRecoveryIsolatedAndChecksummed(t *testing.T) {
	db, cfg := testDB(t)
	u := models.User{Username: "before", Role: "user", PasswordHash: "test"}
	db.Create(&u)
	service := services.NewOpsBackupService(cfg, db)
	m, e := service.Create()
	if e != nil {
		t.Fatal(e)
	}
	db.Model(&u).Update("username", "after")
	target := filepath.Join(t.TempDir(), "recovered")
	if e = service.RestoreTo(m.ID, target); e != nil {
		t.Fatal(e)
	}
	restored, e := database.Open(&config.Config{DataHome: target, DBDriver: "sqlite"})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := restored.DB()
	defer sql.Close()
	var recovered models.User
	restored.First(&recovered, u.ID)
	if recovered.Username != "before" {
		t.Fatal("snapshot was not restored")
	}
	db.First(&u, u.ID)
	if u.Username != "after" {
		t.Fatal("live database was modified")
	}
	if service.RestoreTo(m.ID, target) == nil {
		t.Fatal("existing recovery directory overwritten")
	}
	path, _ := service.ArchivePath(m.ID)
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("corrupt")
	f.Close()
	if service.RestoreTo(m.ID, filepath.Join(t.TempDir(), "corrupt")) == nil {
		t.Fatal("corrupted archive accepted")
	}
}
func TestDurableNotificationsAndStableRetries(t *testing.T) {
	t.Setenv("BASE_WEBHOOK_ALLOW_PRIVATE", "1")
	db, _ := testDB(t)
	u := models.User{Username: "recipient", Role: "admin", PasswordHash: "test"}
	db.Create(&u)
	var ids []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids = append(ids, r.Header.Get("X-Delivery-ID"))
		if len(ids) == 1 {
			w.WriteHeader(500)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer remote.Close()
	ep := models.WebhookEndpoint{Name: "test", URL: remote.URL, Enabled: 1, MaxRetries: 2, TimeoutSec: 2}
	db.Create(&ep)
	bus := services.NewEventBus(db)
	if e := bus.Emit("user.created", &u, map[string]interface{}{"title": "created", "secret": "do-not-send"}); e != nil {
		t.Fatal(e)
	}
	svc := services.NewNotificationService(db, bus)
	if e := svc.ProcessOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	var n int64
	db.Model(&models.Notification{}).Count(&n)
	if n != 1 {
		t.Fatal("typed user ID did not create notification")
	}
	var row models.WebhookDelivery
	db.First(&row)
	if row.Status != "pending" {
		t.Fatalf("retry state: %s", row.Status)
	}
	var payload map[string]any
	json.Unmarshal([]byte(row.Payload), &payload)
	if payload["data"].(map[string]any)["secret"] != "***" {
		t.Fatal("secret escaped masking")
	}
	db.Model(&row).Update("next_retry_at", time.Now().Add(-time.Minute))
	// A new worker resumes the persisted queue, without any in-memory state.
	svc = services.NewNotificationService(db, nil)
	if e := svc.ProcessOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	db.First(&row, row.ID)
	if row.Status != "success" || len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("unstable delivery: %+v %v", row, ids)
	}
	db.Model(&row).Update("status", "failed")
	if e := svc.Retry(row.ID); e != nil {
		t.Fatal(e)
	}
	if e := svc.ProcessOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(ids) != 3 || ids[2] != ids[0] {
		t.Fatal("manual retry changed delivery identity")
	}
	var count int64
	db.Model(&models.WebhookDelivery{}).Count(&count)
	if count != 1 {
		t.Fatal("retry created duplicate delivery")
	}
}

// TestBuiltinRolePermissionContract 锁死规格中「菜单与权限」章节的约定：
// 1. 新建库时内置 admin 角色带上视频 AI 全量权限点；
// 2. 已有库撤销授权后，重启（重新 Seed）不得恢复；
// 3. 已有库不会自动获得规格新增的权限点（需角色页勾选）。
func TestBuiltinRolePermissionContract(t *testing.T) {
	db, _ := testDB(t)
	rbac := services.NewRBACService(db)
	admin := models.User{Username: "permadmin", Role: models.RoleAdmin, PasswordHash: "x"}

	// 1. 新建库：admin 角色带齐视频 AI 菜单权限。
	for _, code := range []string{
		"camera:live", "camera:list", "camera:create", "camera:update", "camera:delete",
		"boundary:list", "boundary:edit", "boundary:publish",
		"algorithm:list", "algorithm:publish",
		"deployment:list", "deployment:edit", "deployment:control",
		"event:list", "event:handle", "report:list",
		"access:list", "access:edit",
	} {
		if !rbac.HasPermission(admin, code) {
			t.Fatalf("fresh database missing builtin admin permission %s", code)
		}
	}
	// user 角色不得越权拿到管理权限。
	plain := models.User{Username: "permuser", Role: models.RoleUser, PasswordHash: "x"}
	if rbac.HasPermission(plain, "camera:live") {
		t.Fatal("builtin user role leaked admin permission")
	}

	// 2. 已有库：撤销全部授权后重启，授权不得被恢复。
	var role models.Role
	if err := db.Where("code = ?", models.RoleAdmin).First(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := rbac.AssignRolePermissions(role.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := rbac.SeedBuiltinRoles(); err != nil {
		t.Fatal(err)
	}
	if rbac.HasPermission(admin, "user:list") {
		t.Fatal("restart resurrected revoked administrator grants")
	}

	// 3. 已有库：规格后续新增的权限点不会自动授予，需角色页勾选。
	services.RegisterPermission(services.PermissionDef{Code: "video:future", Name: "未来权限", Module: "video", Type: "action"})
	if err := rbac.SyncPermissions(); err != nil {
		t.Fatal(err)
	}
	if err := rbac.SeedBuiltinRoles(); err != nil {
		t.Fatal(err)
	}
	if rbac.HasPermission(admin, "video:future") {
		t.Fatal("existing database auto-granted a newly added permission")
	}
	// 角色页勾选后立即生效（全量替换语义保留）。
	if err := rbac.AssignRolePermissions(role.ID, []string{"video:future"}); err != nil {
		t.Fatal(err)
	}
	if !rbac.HasPermission(admin, "video:future") {
		t.Fatal("manual grant via role page did not take effect")
	}
}
