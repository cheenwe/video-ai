package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"baseplatform/internal/avatars"
	"baseplatform/internal/config"
	"baseplatform/internal/handlers"
	"baseplatform/internal/license"
	"baseplatform/internal/middleware"
	"baseplatform/internal/scheduler"
	"baseplatform/internal/services"
	"baseplatform/internal/storage"
	"baseplatform/internal/upgrade"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Application struct {
	Router        *gin.Engine
	Scheduler     *scheduler.Manager
	Notifications *services.NotificationService
}

func (a *Application) Start() error {
	if err := a.Scheduler.Start(); err != nil {
		a.Scheduler.Stop()
		return err
	}
	a.Notifications.Start()
	return nil
}
func (a *Application) Stop() { a.Scheduler.Stop(); a.Notifications.Stop() }

// New builds routes without starting background workers (used by API tests).
func New(cfg *config.Config, db *gorm.DB, frontend string) *gin.Engine {
	app, err := NewApplication(cfg, db, frontend)
	if err != nil {
		panic(err)
	}
	return app.Router
}
func NewApplication(cfg *config.Config, db *gorm.DB, frontend string) (*Application, error) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Recovery(), middleware.RequestID(), middleware.SecurityHeaders())
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
		}
		c.Next()
	})

	// --- Services ---
	captchaStore := services.NewCaptchaStore()
	ldapAuth := &services.LDAPAuth{}
	mailer := &services.Mailer{}
	oauthSvc := &services.OAuthService{DB: db}
	patSvc := services.NewPATService(db)
	backupSvc := services.NewOpsBackupService(cfg, db)
	avatarSvc := &avatars.Service{Root: filepath.Join(cfg.DataHome, "avatars")}
	if err := avatarSvc.EnsureDirs(); err != nil {
		return nil, err
	}
	schedMgr := scheduler.NewManager(db, cfg, backupSvc, ldapAuth)
	license.InitPublicKey(cfg.DataHome)
	licSvc := license.NewService(db, "1.0.0")
	upgradeSvc := upgrade.NewService(db, licSvc, "0.1.0")
	middleware.SetLicenseService(licSvc)
	storageMgr := &storage.Manager{DB: db, Cfg: cfg}
	cloudOAuth := &services.CloudOAuthService{DB: db}
	eventBus := services.NewEventBus(db)
	notifSvc := services.NewNotificationService(db, eventBus)

	// --- Handlers ---
	auth := &handlers.AuthHandler{DB: db, Cfg: cfg, Limiter: services.NewSecurityLoginLimiter(db), Captcha: captchaStore, CaptchaLimiter: services.NewIPRateLimiter(20, time.Minute), LDAP: ldapAuth}
	p := &handlers.PlatformHandler{DB: db, Started: time.Now(), LDAPSvc: ldapAuth, Mailer: mailer, Cfg: cfg, Version: "1.0.0"}
	u := &handlers.UserHandler{DB: db}
	d := &handlers.DepartmentHandler{DB: db}
	sc := &handlers.SysConfigHandler{DB: db}
	sl := &handlers.SysLogHandler{DB: db}
	av := &handlers.AvatarHandler{DB: db, Avatar: avatarSvc}
	tok := &handlers.TokenHandler{DB: db, PAT: patSvc}
	sch := &handlers.ScheduleHandler{DB: db, M: schedMgr}
	ops := &handlers.OpsHandler{DB: db, Backup: backupSvc}
	baseURL := strings.TrimRight(os.Getenv("BASE_PUBLIC_URL"), "/")
	oa := &handlers.OAuthHandler{DB: db, Cfg: cfg, OAuth: oauthSvc, BaseURL: baseURL}
	site := &handlers.SiteInfoHandler{License: licSvc}
	lic := &handlers.LicenseHandler{License: licSvc}
	upg := &handlers.UpgradeHandler{Upgrade: upgradeSvc}
	stor := &handlers.StorageHandler{DB: db, Manager: storageMgr}
	storageCloud := &handlers.StorageCloudOAuthHandler{DB: db, Cfg: cfg, CloudOAuth: cloudOAuth}
	dict := &handlers.DictHandler{DB: db}
	cf := &handlers.CustomFieldHandler{DB: db}
	notif := &handlers.NotificationHandler{DB: db, Svc: notifSvc}
	webhook := &handlers.WebhookHandler{DB: db}
	apiLog := &handlers.ApiCallLogHandler{DB: db}
	rbacSvc := services.NewRBACService(db)
	rbac := &handlers.RBACHandler{DB: db, RBAC: rbacSvc}
	project := &handlers.ProjectHandler{DB: db, RBAC: rbacSvc}

	// --- Public routes ---
	r.GET("/api/health", p.Health)
	r.GET("/api/health/live", p.Live)
	r.GET("/api/health/ready", p.Ready)
	r.GET("/api/site/branding", p.Branding)
	r.GET("/api/auth/config", auth.Config)
	r.POST("/api/auth/login", auth.Login)
	r.POST("/api/auth/totp/verify", auth.LoginTOTP)
	r.GET("/api/auth/captcha", auth.CaptchaCreate)
	r.GET("/api/avatars/presets", av.ListPresets)
	r.GET("/api/auth/oauth/:provider", oa.Begin)
	r.GET("/api/auth/oauth/:provider/callback", oa.Callback)
	r.GET("/api/site/info", site.Info)
	r.GET("/api/storage/cloud/callback/:platform", storageCloud.Callback)

	// --- Authenticated routes ---
	a := r.Group("/api", middleware.Auth(cfg, db, patSvc), middleware.AccountSetupGate(db), middleware.APICallLogger(db))
	a.GET("/auth/me", auth.Me)
	a.POST("/auth/logout", auth.Logout)
	a.PUT("/auth/profile", auth.Profile)
	a.POST("/auth/change-password", auth.ChangePassword)
	a.GET("/auth/timezones", auth.Timezones)
	a.GET("/auth/totp/status", auth.TOTPStatus)
	a.POST("/auth/totp/setup", auth.TOTPSetup)
	a.POST("/auth/totp/enable", auth.TOTPEnable)
	a.POST("/auth/totp/disable", auth.TOTPDisable)
	a.GET("/sys-configs/value", sc.GetPublicValue)

	// Current user permissions (for frontend menu/button control)
	a.GET("/auth/permissions", rbac.MyPermissions)

	// Dictionaries (authenticated users can read enabled dict items)
	a.GET("/dicts/by-code/:code/items", dict.GetDictItemsByCode)

	// Custom field values: read field definitions and values (authenticated users)
	// 用独立前缀避免与 admin 的 /custom-fields/:id 参数路由冲突
	a.GET("/cf-data/modules", cf.ListModules)
	a.GET("/cf-data/:module/definitions", cf.GetDefinitions)
	a.GET("/cf-data/:module/batch", cf.GetBatchValues)
	a.GET("/cf-data/:module/:entityId", cf.GetValues)
	a.PUT("/cf-data/:module/:entityId", cf.SaveValues)

	// Notifications (current user)
	a.GET("/notifications", notif.List)
	a.GET("/notifications/unread-count", notif.UnreadCount)
	a.PUT("/notifications/:id/read", notif.MarkRead)
	a.PUT("/notifications/read-all", notif.MarkAllRead)
	a.DELETE("/notifications/:id", notif.Delete)

	// Avatar management (self)
	a.POST("/avatars/upload", av.UploadUserAvatar)
	a.POST("/avatars/preset", av.SetUserAvatarPreset)
	a.DELETE("/avatars", av.DeleteUserAvatar)

	// Personal tokens (self)
	a.GET("/tokens", tok.List)
	a.POST("/tokens", tok.Create)
	a.PUT("/tokens/:id", tok.Update)
	a.DELETE("/tokens/:id", tok.Delete)
	a.GET("/tokens/gateway", tok.GatewayStatus)
	a.GET("/tokens/:id/call-logs", apiLog.ListForToken)

	// Storage list (authenticated users see enabled storages)
	a.GET("/storages", stor.List)
	a.GET("/storages/drivers", stor.Drivers)
	a.GET("/storages/config-schema", stor.ConfigSchema)

	// --- Admin routes ---
	admin := a.Group("")
	admin.GET("/users", middleware.RequirePermission(db, "user:list"), u.List)
	admin.POST("/users", middleware.RequirePermission(db, "user:create"), u.Create)
	admin.PUT("/users/:uid", middleware.RequirePermission(db, "user:update"), u.Update)
	admin.POST("/users/:uid/disabled", middleware.RequirePermission(db, "user:disable"), u.SetDisabled)
	admin.DELETE("/users/:uid", middleware.RequirePermission(db, "user:delete"), middleware.RequireSuperAdmin(), u.Delete)
	admin.GET("/users/:uid/tokens", middleware.RequirePermission(db, "user:list"), middleware.RequireSuperAdmin(), tok.AdminListForUser)
	admin.DELETE("/users/:uid/tokens/:id", middleware.RequirePermission(db, "user:delete"), middleware.RequireSuperAdmin(), tok.AdminDeleteForUser)

	admin.POST("/users/:uid/avatar/upload", middleware.RequirePermission(db, "user:create"), av.UploadUserAvatarAdmin)
	admin.POST("/users/:uid/avatar/preset", middleware.RequirePermission(db, "user:create"), av.SetUserAvatarPresetAdmin)
	admin.DELETE("/users/:uid/avatar", middleware.RequirePermission(db, "user:delete"), av.DeleteUserAvatarAdmin)

	admin.GET("/departments", middleware.RequirePermission(db, "department:list"), d.List)
	admin.POST("/departments", middleware.RequirePermission(db, "department:create"), d.Create)
	admin.PUT("/departments/:id", middleware.RequirePermission(db, "department:update"), d.Update)
	admin.DELETE("/departments/:id", middleware.RequirePermission(db, "department:delete"), middleware.RequireSuperAdmin(), d.Delete)

	admin.GET("/sys-configs", middleware.RequirePermission(db, "config:list"), sc.List)
	admin.POST("/sys-configs", middleware.RequirePermission(db, "config:create"), sc.Create)
	admin.PUT("/sys-configs/:cid", middleware.RequirePermission(db, "config:update"), sc.Update)
	admin.DELETE("/sys-configs/:cid", middleware.RequirePermission(db, "config:delete"), middleware.RequireSuperAdmin(), sc.Delete)

	// Dictionaries
	admin.GET("/dicts", middleware.RequirePermission(db, "dict:list"), dict.ListDicts)
	admin.GET("/dicts/:id", middleware.RequirePermission(db, "dict:list"), dict.GetDict)
	admin.POST("/dicts", middleware.RequirePermission(db, "dict:create"), dict.CreateDict)
	admin.PUT("/dicts/:id", middleware.RequirePermission(db, "dict:update"), dict.UpdateDict)
	admin.DELETE("/dicts/:id", middleware.RequirePermission(db, "dict:delete"), middleware.RequireSuperAdmin(), dict.DeleteDict)
	admin.GET("/dicts/:id/items", middleware.RequirePermission(db, "dict:list"), dict.ListItems)
	admin.POST("/dicts/:id/items", middleware.RequirePermission(db, "dict:create"), dict.CreateItem)
	admin.PUT("/dicts/:id/items/:itemId", middleware.RequirePermission(db, "dict:update"), dict.UpdateItem)
	admin.DELETE("/dicts/:id/items/:itemId", middleware.RequirePermission(db, "dict:delete"), middleware.RequireSuperAdmin(), dict.DeleteItem)

	// Custom fields: field definition CRUD (admin)
	admin.GET("/custom-fields", middleware.RequirePermission(db, "custom_field:list"), cf.ListDefs)
	admin.POST("/custom-fields", middleware.RequirePermission(db, "custom_field:create"), cf.CreateDef)
	admin.PUT("/custom-fields/:id", middleware.RequirePermission(db, "custom_field:update"), cf.UpdateDef)
	admin.DELETE("/custom-fields/:id", middleware.RequirePermission(db, "custom_field:delete"), middleware.RequireSuperAdmin(), cf.DeleteDef)

	admin.GET("/sys-logs", middleware.RequirePermission(db, "log:list"), sl.List)
	admin.GET("/sys-logs/:id", middleware.RequirePermission(db, "log:list"), sl.Get)
	admin.GET("/sys-logs/export", middleware.RequirePermission(db, "log:export"), sl.Export)

	// API call logs (admin)
	admin.GET("/api-call-logs", middleware.RequirePermission(db, "apilog:list"), apiLog.List)

	// RBAC: 角色与权限管理
	admin.GET("/roles", middleware.RequirePermission(db, "role:list"), rbac.List)
	admin.POST("/roles", middleware.RequirePermission(db, "role:create"), middleware.RequireSuperAdmin(), rbac.Create)
	admin.PUT("/roles/:id", middleware.RequirePermission(db, "role:update"), middleware.RequireSuperAdmin(), rbac.Update)
	admin.DELETE("/roles/:id", middleware.RequirePermission(db, "role:delete"), middleware.RequireSuperAdmin(), middleware.RequireSuperAdmin(), rbac.Delete)
	admin.GET("/permissions", middleware.RequirePermission(db, "role:list"), rbac.Permissions)
	admin.GET("/roles/:id/permissions", middleware.RequirePermission(db, "role:list"), rbac.RolePermissions)
	admin.PUT("/roles/:id/permissions", middleware.RequirePermission(db, "role:assign"), middleware.RequireSuperAdmin(), rbac.AssignPermissions)
	admin.GET("/users/:uid/roles", middleware.RequirePermission(db, "role:list"), rbac.UserRoles)
	admin.PUT("/users/:uid/roles", middleware.RequirePermission(db, "role:assign"), middleware.RequireSuperAdmin(), rbac.AssignUserRole)

	// Project management (data scope)
	admin.GET("/projects", middleware.RequirePermission(db, "project:list"), project.List)
	admin.POST("/projects", middleware.RequirePermission(db, "project:create"), project.Create)
	admin.PUT("/projects/:id", middleware.RequirePermission(db, "project:update"), project.Update)
	admin.DELETE("/projects/:id", middleware.RequirePermission(db, "project:delete"), middleware.RequireSuperAdmin(), project.Delete)
	admin.GET("/projects/:id/members", middleware.RequirePermission(db, "project:list"), project.Members)
	admin.POST("/projects/:id/members", middleware.RequirePermission(db, "project:create"), project.AddMember)
	admin.PUT("/projects/members/:mid", middleware.RequirePermission(db, "project:update"), project.UpdateMember)
	admin.DELETE("/projects/members/:mid", middleware.RequirePermission(db, "project:delete"), project.RemoveMember)

	// Webhook management
	admin.GET("/webhooks", middleware.RequirePermission(db, "webhook:list"), webhook.List)
	admin.POST("/webhooks", middleware.RequirePermission(db, "webhook:create"), webhook.Create)
	admin.PUT("/webhooks/:id", middleware.RequirePermission(db, "webhook:update"), webhook.Update)
	admin.DELETE("/webhooks/:id", middleware.RequirePermission(db, "webhook:delete"), webhook.Delete)
	admin.GET("/webhooks/:id/deliveries", middleware.RequirePermission(db, "webhook:list"), webhook.Deliveries)
	admin.POST("/webhooks/deliveries/:id/retry", middleware.RequirePermission(db, "webhook:update"), webhook.Retry)
	admin.GET("/admin/overview", middleware.RequirePermission(db, "overview:list"), p.Overview)
	admin.PUT("/site/branding", middleware.RequirePermission(db, "settings:general"), p.SaveBranding)
	admin.GET("/admin/settings/general", middleware.RequirePermission(db, "settings:general"), p.General)
	admin.PUT("/admin/settings/general", middleware.RequirePermission(db, "settings:general"), p.SaveGeneral)
	admin.GET("/admin/settings/security", middleware.RequirePermission(db, "settings:security"), middleware.RequireSuperAdmin(), p.Security)
	admin.PUT("/admin/settings/security", middleware.RequirePermission(db, "settings:security"), middleware.RequireSuperAdmin(), p.SaveSecurity)
	admin.POST("/admin/settings/captcha/preview", middleware.RequirePermission(db, "settings:captcha"), auth.CaptchaPreview)

	// Email settings
	admin.GET("/admin/settings/email", middleware.RequirePermission(db, "settings:email"), p.Email)
	admin.PUT("/admin/settings/email", middleware.RequirePermission(db, "settings:email"), p.SaveEmail)
	admin.POST("/admin/settings/email/test", middleware.RequirePermission(db, "settings:email"), p.EmailTest)

	// LDAP settings
	admin.GET("/admin/settings/ldap", middleware.RequirePermission(db, "settings:ldap"), middleware.RequireSuperAdmin(), p.LDAP)
	admin.PUT("/admin/settings/ldap", middleware.RequirePermission(db, "settings:ldap"), middleware.RequireSuperAdmin(), p.SaveLDAP)
	admin.POST("/admin/settings/ldap/test", middleware.RequirePermission(db, "settings:ldap"), middleware.RequireSuperAdmin(), p.LDAPTest)

	// OAuth settings
	admin.GET("/admin/settings/oauth", middleware.RequirePermission(db, "settings:oauth"), middleware.RequireSuperAdmin(), oa.AdminList)
	admin.PUT("/admin/settings/oauth", middleware.RequirePermission(db, "settings:oauth"), middleware.RequireSuperAdmin(), oa.AdminSave)

	// Gateway settings
	admin.GET("/admin/settings/gateway", middleware.RequirePermission(db, "settings:gateway"), middleware.RequireSuperAdmin(), p.Gateway)
	admin.PUT("/admin/settings/gateway", middleware.RequirePermission(db, "settings:gateway"), middleware.RequireSuperAdmin(), p.SaveGateway)

	// NTP settings
	admin.GET("/admin/settings/ntp", middleware.RequirePermission(db, "settings:ntp"), p.NTP)
	admin.PUT("/admin/settings/ntp", middleware.RequirePermission(db, "settings:ntp"), p.SaveNTP)
	admin.GET("/admin/ops/ntp", middleware.RequirePermission(db, "ops:list"), ops.NTPQuery)

	// Schedule management
	admin.GET("/admin/schedule/jobs", middleware.RequirePermission(db, "schedule:list"), sch.ListJobs)
	admin.GET("/admin/schedule/jobs/:id/runs", middleware.RequirePermission(db, "schedule:list"), sch.Runs)
	admin.GET("/admin/schedule/jobs/:id", middleware.RequirePermission(db, "schedule:list"), sch.GetJob)
	admin.POST("/admin/schedule/jobs", middleware.RequirePermission(db, "schedule:create"), sch.CreateJob)
	admin.PUT("/admin/schedule/jobs/:id", middleware.RequirePermission(db, "schedule:update"), sch.UpdateJob)
	admin.DELETE("/admin/schedule/jobs/:id", middleware.RequirePermission(db, "schedule:delete"), sch.DeleteJob)
	admin.POST("/admin/schedule/jobs/:id/run", middleware.RequirePermission(db, "schedule:run"), sch.RunJob)
	admin.GET("/admin/schedule/commands", middleware.RequirePermission(db, "schedule:list"), sch.ListCommands)
	admin.POST("/admin/schedule/commands", middleware.RequirePermission(db, "schedule:create"), sch.CreateCommand)
	admin.PUT("/admin/schedule/commands/:id", middleware.RequirePermission(db, "schedule:update"), sch.UpdateCommand)
	admin.DELETE("/admin/schedule/commands/:id", middleware.RequirePermission(db, "schedule:delete"), sch.DeleteCommand)
	admin.GET("/admin/schedule/stats", middleware.RequirePermission(db, "schedule:list"), sch.Stats)
	admin.POST("/admin/schedule/validate-cron", middleware.RequirePermission(db, "schedule:update"), sch.ValidateCron)

	// Backup & restore
	admin.GET("/admin/ops/backups", middleware.RequirePermission(db, "backup:list"), ops.List)
	admin.GET("/admin/ops/backups/status", middleware.RequirePermission(db, "backup:list"), ops.Status)
	admin.POST("/admin/ops/backups", middleware.RequirePermission(db, "backup:create"), middleware.RequireSuperAdmin(), ops.Create)
	admin.GET("/admin/ops/backups/:id/download", middleware.RequirePermission(db, "backup:download"), ops.Download)
	admin.DELETE("/admin/ops/backups/:id", middleware.RequirePermission(db, "backup:delete"), middleware.RequireSuperAdmin(), ops.Delete)
	admin.POST("/admin/ops/backups/:id/restore", middleware.RequirePermission(db, "backup:restore"), middleware.RequireSuperAdmin(), ops.Restore)

	// License management
	admin.GET("/admin/license/status", middleware.RequirePermission(db, "license:list"), lic.Status)
	admin.GET("/admin/license/fingerprint", middleware.RequirePermission(db, "license:list"), lic.Fingerprint)
	admin.POST("/admin/license/import", middleware.RequirePermission(db, "license:create"), middleware.RequireSuperAdmin(), lic.Import)
	admin.POST("/admin/license/activate", middleware.RequirePermission(db, "license:create"), middleware.RequireSuperAdmin(), lic.Activate)
	admin.POST("/admin/license/sync", middleware.RequirePermission(db, "license:create"), middleware.RequireSuperAdmin(), lic.Sync)
	admin.POST("/admin/license/remove", middleware.RequirePermission(db, "license:create"), middleware.RequireSuperAdmin(), lic.Remove)
	admin.POST("/admin/license/trial/apply", middleware.RequirePermission(db, "license:create"), middleware.RequireSuperAdmin(), lic.TrialApply)
	admin.GET("/admin/license/trial/status", middleware.RequirePermission(db, "license:list"), lic.TrialStatus)

	// Upgrade management
	admin.GET("/admin/upgrade/check", middleware.RequirePermission(db, "upgrade:list"), upg.Check)
	admin.GET("/admin/upgrade/status", middleware.RequirePermission(db, "upgrade:list"), upg.Status)
	admin.POST("/admin/upgrade/apply", middleware.RequirePermission(db, "upgrade:create"), middleware.RequireSuperAdmin(), upg.Apply)

	// Storage management (admin)
	admin.GET("/admin/storages", middleware.RequirePermission(db, "storage:list"), stor.List)
	admin.GET("/admin/storages/:id", middleware.RequirePermission(db, "storage:list"), stor.Get)
	admin.POST("/admin/storages", middleware.RequirePermission(db, "storage:create"), stor.Create)
	admin.PUT("/admin/storages/:id", middleware.RequirePermission(db, "storage:update"), stor.Update)
	admin.DELETE("/admin/storages/:id", middleware.RequirePermission(db, "storage:delete"), stor.Delete)
	admin.POST("/admin/storages/:id/test", middleware.RequirePermission(db, "storage:create"), stor.Test)
	admin.POST("/admin/storages/test-config", middleware.RequirePermission(db, "storage:create"), stor.TestConfig)
	admin.GET("/admin/storages/:id/browse", middleware.RequirePermission(db, "storage:list"), stor.Browse)
	admin.GET("/admin/storages/:id/cloud-oauth-url", middleware.RequirePermission(db, "storage:list"), storageCloud.AuthURL)
	admin.GET("/admin/storages/:id/cloud-oauth/start", middleware.RequirePermission(db, "storage:list"), storageCloud.Start)
	admin.GET("/admin/storage/cloud/callback-url", middleware.RequirePermission(db, "storage:list"), storageCloud.CallbackURLPreview)

	// Cloud storage app settings
	admin.GET("/admin/settings/cloud-storage", middleware.RequirePermission(db, "settings:cloud-storage"), middleware.RequireSuperAdmin(), p.CloudStorageSettings)
	admin.PUT("/admin/settings/cloud-storage", middleware.RequirePermission(db, "settings:cloud-storage"), middleware.RequireSuperAdmin(), p.SaveCloudStorageSettings)

	// Watermark settings (flagship feature)
	admin.GET("/admin/settings/watermark", middleware.RequirePermission(db, "settings:watermark"), middleware.RequireFeature(license.FeatureWatermark), p.Watermark)
	admin.PUT("/admin/settings/watermark", middleware.RequirePermission(db, "settings:watermark"), middleware.RequireFeature(license.FeatureWatermark), p.SaveWatermark)

	// License server settings
	admin.GET("/admin/settings/license", middleware.RequirePermission(db, "settings:license"), middleware.RequireSuperAdmin(), p.LicenseSettings)
	admin.PUT("/admin/settings/license", middleware.RequirePermission(db, "settings:license"), middleware.RequireSuperAdmin(), p.SaveLicenseSettings)

	if frontend != "" {
		if info, e := os.Stat(filepath.Join(frontend, "index.html")); e == nil && !info.IsDir() {
			r.Static("/assets", filepath.Join(frontend, "assets"))
			r.StaticFile("/favicon.svg", filepath.Join(frontend, "favicon.svg"))
			r.NoRoute(func(c *gin.Context) {
				if strings.HasPrefix(c.Request.URL.Path, "/api/") {
					c.Status(404)
					return
				}
				if c.Request.Method != "GET" {
					c.Status(405)
					return
				}
				c.File(filepath.Join(frontend, "index.html"))
			})
			r.GET("/", func(c *gin.Context) { c.File(filepath.Join(frontend, "index.html")) })
		}
	}
	return &Application{r, schedMgr, notifSvc}, nil
}
