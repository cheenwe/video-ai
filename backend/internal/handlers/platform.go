package handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
	"baseplatform/internal/config"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlatformHandler struct {
	DB       *gorm.DB
	Started  time.Time
	LDAPSvc  *services.LDAPAuth
	Mailer   *services.Mailer
	Cfg      *config.Config
	Version  string
}
type Branding struct {
	LoginCopy           map[string]string `json:"login_copy"`
	SiteName            string            `json:"site_name"`
	Tagline             string            `json:"tagline"`
	PrimaryColor        string            `json:"primary_color"`
	LogoURL             string            `json:"logo_url"`
	FaviconURL          string            `json:"favicon_url"`
	LogoText            string            `json:"logo_text"`
	LoginDescription    string            `json:"login_description"`
	FooterPoweredByName string            `json:"footer_powered_by_name"`
	FooterICP           string            `json:"footer_icp"`
	FooterPoweredByURL  string            `json:"footer_powered_by_url"`
	FooterWechat        string            `json:"footer_wechat"`
}

func defaultBranding() Branding {
	return Branding{
		LoginCopy:           defaultLoginCopy(),
		SiteName:            "Video AI",
		Tagline:             "视频智能分析管理平台",
		PrimaryColor:        "#176b4d",
		LogoText:            "VA",
		LoginDescription:    "统一管理视频源、分析任务与现场事件",
		FooterPoweredByName: "Video AI",
	}
}
func (h *PlatformHandler) Branding(c *gin.Context) {
	response.OK(c, loadBranding(h.DB), "")
}
func (h *PlatformHandler) SaveBranding(c *gin.Context) {
	b := loadBranding(h.DB)
	if c.ShouldBindJSON(&b) != nil || strings.TrimSpace(b.SiteName) == "" || utf8.RuneCountInString(b.SiteName) > 80 || !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(b.PrimaryColor) {
		response.BizFail(c, "名称或主题色不合法", 422)
		return
	}
	if b.LoginCopy == nil {
		response.BizFail(c, "登录文案不能为空", 422)
		return
	}
	defaults := defaultLoginCopy()
	for k, v := range b.LoginCopy {
		if _, ok := defaults[k]; !ok {
			response.BizFail(c, "未知登录文案字段", 422)
			return
		}
		if utf8.RuneCountInString(v) > 500 {
			response.BizFail(c, "每项登录文案不得超过500字", 422)
			return
		}
	}
	for _, k := range requiredLoginCopy {
		if strings.TrimSpace(b.LoginCopy[k]) == "" {
			response.BizFail(c, "必填登录文案不能为空", 422)
			return
		}
	}
	if utf8.RuneCountInString(b.Tagline) > 500 || utf8.RuneCountInString(b.LoginDescription) > 500 {
		response.BizFail(c, "平台说明不得超过500字", 422)
		return
	}
	for _, v := range []string{b.LogoURL, b.FaviconURL} {
		if v != "" && (!strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || strings.Contains(v, "\\")) {
			response.BizFail(c, "图标仅支持本站相对地址", 422)
			return
		}
	}
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "branding", b, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.branding", &u, "")
	response.OK(c, b, "已保存")
}
func (h *PlatformHandler) General(c *gin.Context) {
	response.OK(c, gin.H{"page_size": services.PageSizeFromDB(h.DB)}, "")
}
func (h *PlatformHandler) SaveGeneral(c *gin.Context) {
	var req struct {
		PageSize int `json:"page_size"`
	}
	if c.ShouldBindJSON(&req) != nil || req.PageSize < 1 || req.PageSize > 200 {
		response.BizFail(c, "分页大小应为 1–200", 422)
		return
	}
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "general", settings.General{PageSize: req.PageSize}, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.general", &u, "")
	h.General(c)
}
func (h *PlatformHandler) Security(c *gin.Context) { response.OK(c, settings.LoadSecurity(h.DB), "") }
func (h *PlatformHandler) SaveSecurity(c *gin.Context) {
	var s settings.Security
	if c.ShouldBindJSON(&s) != nil || s.PasswordMinLen < 8 || s.PasswordMinLen > 72 || s.LoginFailMax < 3 || s.LoginFailMax > 100 || s.LoginFailWindowSec < 30 || s.LoginFailWindowSec > 3600 || s.LoginBanSec < 60 || s.LoginBanSec > 86400 {
		response.BizFail(c, "密码或登录限流参数超出范围", 422)
		return
	}
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "security", s, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.security", &u, "")
	h.Security(c)
}
func (h *PlatformHandler) Overview(c *gin.Context) {
	var users, depts, logs, storages, scheduleJobs int64
	h.DB.Model(&models.User{}).Count(&users)
	h.DB.Model(&models.Department{}).Count(&depts)
	h.DB.Model(&models.SysLog{}).Count(&logs)
	h.DB.Model(&models.Storage{}).Where("enabled = ?", 1).Count(&storages)
	h.DB.Model(&models.ScheduleJob{}).Where("enabled = ?", 1).Count(&scheduleJobs)

	// 系统统计
	var sysStats services.SystemStats
	sysStats = services.CollectSystemStats("/", "/data", "/var/lib/baseplatform")

	// 最近活动
	var recentLogs []models.SysLog
	h.DB.Order("id desc").Limit(10).Find(&recentLogs)
	activity := make([]gin.H, 0, len(recentLogs))
	for _, item := range recentLogs {
		activity = append(activity, gin.H{
			"id":         item.ID,
			"action":     item.Action,
			"username":   item.Username,
			"remark":     item.Remark,
			"status":     item.Result,
			"created_at": item.CreatedAt,
		})
	}

	// 12 天趋势
	chart := overviewChart(h.DB, 12)

	response.OK(c, gin.H{
		"stats": gin.H{
			"users":         users,
			"departments":   depts,
			"logs":          logs,
			"storages":      storages,
			"schedule_jobs": scheduleJobs,
		},
		"system": gin.H{
			"hostname":    sysStats.Hostname,
			"os":          sysStats.OS,
			"platform":    sysStats.Platform,
			"arch":        sysStats.Arch,
			"uptime_sec":  sysStats.UptimeSec,
			"cpu_cores":   sysStats.CPUCores,
			"cpu_percent": sysStats.CPUPercent,
			"mem_total":   sysStats.MemTotal,
			"mem_used":    sysStats.MemUsed,
			"mem_percent": sysStats.MemPercent,
			"disks":       sysStats.Disks,
			"go_version":  runtime.Version(),
		},
		"activity": activity,
		"chart":    chart,
	}, "")
}

func overviewChart(db *gorm.DB, days int) []gin.H {
	if days < 1 {
		days = 12
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	buckets := make([]int, days)
	labels := make([]string, days)
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i)
		labels[i] = fmt.Sprintf("%02d-%02d", d.Month(), d.Day())
	}

	var logs []models.SysLog
	db.Select("created_at").Where("created_at >= ?", start).Find(&logs)
	for _, item := range logs {
		idx := int(item.CreatedAt.In(now.Location()).Sub(start).Hours() / 24)
		if idx >= 0 && idx < days {
			buckets[idx]++
		}
	}

	out := make([]gin.H, days)
	max := 0
	for _, n := range buckets {
		if n > max {
			max = n
		}
	}
	for i := 0; i < days; i++ {
		height := 0
		if max > 0 {
			height = int(float64(buckets[i]) / float64(max) * 100)
			if buckets[i] > 0 && height < 12 {
				height = 12
			}
		}
		out[i] = gin.H{
			"label":  labels[i],
			"count":  buckets[i],
			"height": height,
		}
	}
	return out
}
// healthComponent 单个子系统的健康状态。
type healthComponent struct {
	Status string `json:"status"` // ok | degraded | down
	Detail string `json:"detail,omitempty"`
}

// Health 就绪性检查：数据库、磁盘等关键依赖。任一关键依赖故障返回 503。
func (h *PlatformHandler) Health(c *gin.Context) {
	components := map[string]healthComponent{}
	overall := "ok"

	// 数据库
	dbComp := healthComponent{Status: "ok"}
	if sqlDB, err := h.DB.DB(); err != nil {
		dbComp = healthComponent{Status: "down", Detail: err.Error()}
		overall = "down"
	} else if err := sqlDB.Ping(); err != nil {
		dbComp = healthComponent{Status: "down", Detail: err.Error()}
		overall = "down"
	}
	components["database"] = dbComp

	// 磁盘（数据目录）
	diskComp := healthComponent{Status: "ok"}
	if h.Cfg != nil {
		stats := services.CollectSystemStats(h.Cfg.DataHome)
		for _, d := range stats.Disks {
			if d.Percent >= 95 {
				diskComp = healthComponent{Status: "down", Detail: fmt.Sprintf("磁盘 %s 使用率 %.1f%%", d.Path, d.Percent)}
				overall = "down"
				break
			} else if d.Percent >= 85 {
				diskComp = healthComponent{Status: "degraded", Detail: fmt.Sprintf("磁盘 %s 使用率 %.1f%%", d.Path, d.Percent)}
				if overall == "ok" {
					overall = "degraded"
				}
			}
		}
	}
	components["disk"] = diskComp

	out := gin.H{
		"status":     overall,
		"version":    h.Version,
		"uptime_sec": int64(time.Since(h.Started).Seconds()),
		"components": components,
	}
	if overall == "down" {
		response.FailData(c, 503, "服务不可用", 503, out)
		return
	}
	response.OK(c, out, "")
}

// Live 存活性检查：仅验证进程存活，不检查依赖。
func (h *PlatformHandler) Live(c *gin.Context) {
	response.OK(c, gin.H{"status": "ok"}, "")
}

// Ready 就绪性检查：等价于 Health，供编排系统区分 liveness/readiness。
func (h *PlatformHandler) Ready(c *gin.Context) {
	h.Health(c)
}
func (h *PlatformHandler) Roles(c *gin.Context) {
	response.OK(c, gin.H{"items": []gin.H{{"code": "superadmin", "name": "超级管理员", "description": "系统初始化账号；全部基础管理权限"}, {"code": "admin", "name": "管理员", "description": "用户、部门、配置及日志管理；无删除权限"}, {"code": "user", "name": "普通用户", "description": "个人资料与密码管理"}}, "mode": fmt.Sprint("fixed")}, "")
}

// --- Email settings ---

func (h *PlatformHandler) Email(c *gin.Context) { response.OK(c, settings.LoadEmail(h.DB), "") }
func (h *PlatformHandler) SaveEmail(c *gin.Context) {
	var e settings.Email
	if c.ShouldBindJSON(&e) != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if e.Enabled {
		if e.SMTPHost == "" || e.SMTPPort < 1 || e.SMTPPort > 65535 || e.From == "" {
			response.BizFail(c, "启用邮件需填写 SMTP 主机、端口和发件人", 422)
			return
		}
	}
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "email", e, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.email", &u, "")
	response.OK(c, e, "已保存")
}

type emailTestReq struct {
	To string `json:"to" binding:"required,email"`
}

func (h *PlatformHandler) EmailTest(c *gin.Context) {
	var req emailTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "邮箱格式不正确", 422)
		return
	}
	cfg := settings.LoadEmail(h.DB)
	if !cfg.Enabled {
		response.BizFail(c, "邮件服务未启用", 400)
		return
	}
	body, _ := services.RenderTemplate(cfg.Templates.Test, map[string]string{"site_name": loadBranding(h.DB).SiteName})
	if err := h.Mailer.Send(cfg, req.To, "Video AI 测试邮件", body); err != nil {
		response.BizFail(c, err.Error(), 502)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "settings.email.test", &me, req.To)
	response.OK(c, nil, "测试邮件已发送")
}

// --- LDAP settings ---

func (h *PlatformHandler) LDAP(c *gin.Context) { response.OK(c, settings.LoadLDAP(h.DB), "") }
func (h *PlatformHandler) SaveLDAP(c *gin.Context) {
	var l settings.LDAP
	if c.ShouldBindJSON(&l) != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if l.Enabled {
		if l.Host == "" || l.BaseDN == "" {
			response.BizFail(c, "启用 LDAP 需填写 Host 与 Base DN", 422)
			return
		}
	}
	u := middleware.CurrentUser(c)
	l = settings.MergeLDAPDefaults(l)
	if err := settings.SaveSection(h.DB, "ldap", l, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.ldap", &u, "")
	response.OK(c, l, "已保存")
}

func (h *PlatformHandler) LDAPTest(c *gin.Context) {
	cfg := settings.LoadLDAP(h.DB)
	if !cfg.Enabled {
		response.BizFail(c, "LDAP 未启用", 400)
		return
	}
	msg, err := h.LDAPSvc.Test(cfg)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "settings.ldap.test", &me, msg)
	response.OK(c, gin.H{"message": msg}, "")
}

// --- Gateway settings (personal tokens) ---

func (h *PlatformHandler) Gateway(c *gin.Context) { response.OK(c, settings.LoadGateway(h.DB), "") }
func (h *PlatformHandler) SaveGateway(c *gin.Context) {
	var g settings.Gateway
	if c.ShouldBindJSON(&g) != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "gateway", g, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.gateway", &u, "")
	response.OK(c, g, "已保存")
}

// --- NTP settings ---

func (h *PlatformHandler) NTP(c *gin.Context) {
	g := settings.NormalizeGeneral(settings.LoadGeneral(h.DB))
	response.OK(c, gin.H{"ntp_enabled": g.NtpEnabled, "ntp_server": g.NtpServer}, "")
}
func (h *PlatformHandler) SaveNTP(c *gin.Context) {
	var req struct {
		NtpEnabled bool   `json:"ntp_enabled"`
		NtpServer  string `json:"ntp_server"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.NtpEnabled && strings.TrimSpace(req.NtpServer) == "") {
		response.BizFail(c, "启用 NTP 需填写服务器地址", 422)
		return
	}
	g := settings.LoadGeneral(h.DB)
	g.NtpEnabled = req.NtpEnabled
	g.NtpServer = strings.TrimSpace(req.NtpServer)
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "general", g, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.ntp", &u, "")
	response.OK(c, g, "已保存")
}

func defaultLoginCopy() map[string]string {
	return map[string]string{
		"badge":                "VIDEO INTELLIGENCE",
		"eyebrow":              "管理控制台",
		"title":                "登录账号",
		"username_label":       "用户名",
		"username_placeholder": "请输入用户名",
		"password_label":       "密码",
		"password_placeholder": "请输入密码",
		"submit":               "登录",
		"submitting":           "正在登录…",
		"show_password":        "显示密码",
		"hide_password":        "隐藏密码",
		"help":                 "账号由管理员分配，如需帮助请联系管理员。",
		"footer":               "",
		"username_required":    "请输入用户名",
		"password_required":    "请输入密码",
		"invalid_credentials":  "用户名或密码错误，或账号不可用",
		"rate_limited":         "登录尝试过于频繁，请稍后再试。",
		"service_unavailable":  "暂时无法连接服务，请稍后重试。",
		"generic_error":        "登录失败，请稍后重试。",
	}
}

var requiredLoginCopy = []string{"title", "username_label", "password_label", "submit", "submitting", "show_password", "hide_password", "username_required", "password_required", "invalid_credentials", "rate_limited", "service_unavailable", "generic_error"}

// Decode into defaults so legacy saved branding keeps all newly introduced text.
// Explicit empty strings remain empty for optional copy; omitted fields retain values.
func loadBranding(db *gorm.DB) Branding {
	b := defaultBranding()
	var row models.SysConfig
	if db.Where("category = ? AND code = ? AND status = ?", "settings", "branding", "active").First(&row).Error != nil {
		return b
	}
	if json.Unmarshal([]byte(row.Value), &b) != nil {
		return defaultBranding()
	}
	if b.LoginCopy == nil {
		b.LoginCopy = defaultLoginCopy()
	}
	for _, key := range requiredLoginCopy {
		if strings.TrimSpace(b.LoginCopy[key]) == "" {
			b.LoginCopy[key] = defaultLoginCopy()[key]
		}
	}
	return b
}

// --- Cloud storage app settings ---

func (h *PlatformHandler) CloudStorageSettings(c *gin.Context) {
	response.OK(c, settings.LoadCloudStorage(h.DB), "")
}

func (h *PlatformHandler) SaveCloudStorageSettings(c *gin.Context) {
	var cfg settings.CloudStorage
	if c.ShouldBindJSON(&cfg) != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	cfg = settings.MergeCloudStorageDefaults(cfg)
	if err := settings.SaveSection(h.DB, "cloud_storage", cfg, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.cloud_storage", &u, "")
	response.OK(c, cfg, "已保存")
}

// --- Watermark settings ---

func (h *PlatformHandler) Watermark(c *gin.Context) {
	response.OK(c, settings.LoadWatermark(h.DB), "")
}

func (h *PlatformHandler) SaveWatermark(c *gin.Context) {
	var w settings.WatermarkSettings
	if c.ShouldBindJSON(&w) != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	w = settings.NormalizeWatermark(w)
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "watermark", w, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.watermark", &u, "")
	response.OK(c, w, "已保存")
}

// --- License server settings ---

func (h *PlatformHandler) LicenseSettings(c *gin.Context) {
	response.OK(c, settings.LoadLicense(h.DB), "")
}

func (h *PlatformHandler) SaveLicenseSettings(c *gin.Context) {
	var l settings.LicenseSettings
	if c.ShouldBindJSON(&l) != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	l = settings.NormalizeLicense(l)
	u := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "license", l, &u.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.license", &u, "")
	response.OK(c, l, "已保存")
}
