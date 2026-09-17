package handlers

import (
	"errors"
	"fmt"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"net/http"
	"strconv"
	"strings"
	"time"
	"baseplatform/internal/config"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/security"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB      *gorm.DB
	Cfg     *config.Config
	Limiter *services.SecurityLoginLimiter
	Captcha *services.CaptchaStore
	LDAP    *services.LDAPAuth
}

type profileReq struct {
	RealName *string `json:"real_name"`
	Timezone *string `json:"timezone"`
}
type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (h *AuthHandler) Config(c *gin.Context) {
	sec := settings.LoadSecurity(h.DB)
	oauth := settings.LoadOAuth(h.DB)
	providers := make([]settings.PublicOAuthProvider, 0, len(oauth.Providers))
	if oauth.Enabled {
		for _, p := range oauth.Providers {
			if p.Enabled {
				providers = append(providers, settings.PublicOAuthProvider{ID: p.ID, Name: p.Name, Type: p.Type, IconURL: p.IconURL})
			}
		}
	}
	response.OK(c, gin.H{
		"allow_register":  false,
		"captcha_enabled": sec.CaptchaEnabled,
		"oauth_enabled":   oauth.Enabled && len(providers) > 0,
		"oauth_providers": providers,
		"password_rules":  services.PasswordRulesText(sec),
		"admin_2fa":       sec.Admin2FAEnabled,
	}, "")
}

type loginReq struct {
	Username  string `json:"username" binding:"required,max=64"`
	Password  string `json:"password" binding:"required,max=128"`
	CaptchaID string `json:"captcha_id"`
	Captcha   string `json:"captcha"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	ip := c.ClientIP()
	if locked, wait := h.Limiter.AssertAllowed(ip); locked {
		response.Fail(c, 429, "登录失败过多，请 "+strconv.Itoa(wait)+" 秒后重试", 429)
		return
	}
	sec := settings.LoadSecurity(h.DB)
	needCaptcha := sec.CaptchaEnabled || h.Limiter.RequireCaptcha(req.Username)
	if needCaptcha {
		if !h.Captcha.Verify(req.CaptchaID, req.Captcha) {
			response.BizFailData(c, "验证码错误", 422, gin.H{"captcha_required": true})
			return
		}
	}
	username := strings.TrimSpace(req.Username)
	var u models.User
	err := h.DB.Where("username = ?", username).First(&u).Error
	authed := err == nil && security.VerifyPassword(req.Password, u.PasswordHash)
	if !authed {
		// LDAP fallback
		if !authed && h.LDAP != nil {
			ldapCfg := settings.LoadLDAP(h.DB)
			if ldapCfg.Enabled {
				if lu, e := h.LDAP.Authenticate(ldapCfg, username, req.Password); e == nil && lu != nil {
					if u.ID == 0 {
						email := lu.Email
						u = models.User{Username: lu.Username, Email: &email, Role: models.RoleUser, AuthSource: models.AuthSourceLDAP, LDAPDN: lu.DN, Timezone: "UTC", WebLoginEnabled: 1}
						_ = h.DB.Create(&u).Error
					} else if u.AuthSource != models.AuthSourceLDAP {
						_ = h.DB.Model(&u).Updates(map[string]any{"auth_source": models.AuthSourceLDAP, "ldap_dn": lu.DN}).Error
					}
					authed = true
				}
			}
		}
	}
	if !authed || u.Disabled != 0 || !u.CanWebLogin() {
		h.Limiter.RecordFailure(ip)
		h.Limiter.RecordAccountFailure(username)
		services.LogAction(h.DB, c, "login", "failed", "login", nil, "用户名或密码错误，或账号不可用")
		response.BizFail(c, "用户名或密码错误，或账号不可用", 400)
		return
	}
	if u.TOTPEnabled != 0 {
		temp, _, err := security.CreatePurposeToken(h.Cfg.SecretKey, h.Cfg.JWTAlgorithm, 5, u.ID, u.Username, u.Role, "totp", u.SessionVersion)
		if err != nil {
			response.BizFail(c, "签发临时凭证失败", 500)
			return
		}
		h.Limiter.Clear(ip)
		h.Limiter.ClearAccount(username)
		services.LogAction(h.DB, c, "login", "success", "login.totp_challenge", &u, "")
		response.OK(c, gin.H{"totp_required": true, "temp_token": temp}, "需要二次验证")
		return
	}
	token, _, err := security.CreateToken(h.Cfg.SecretKey, h.Cfg.JWTAlgorithm, h.Cfg.JWTExpireDays, u.ID, u.Username, u.Role, u.SessionVersion)
	if err != nil {
		response.BizFail(c, "签发凭证失败", 500)
		return
	}
	if err = h.DB.Model(&u).Update("last_login_at", time.Now()).Error; err != nil {
		response.BizFail(c, "保存登录记录失败", 500)
		return
	}
	h.Limiter.Clear(ip)
	h.Limiter.ClearAccount(username)
	setSessionCookie(c, token, h.Cfg.JWTExpireDays*86400)
	services.LogAction(h.DB, c, "login", "success", "login", &u, "")
	response.OK(c, gin.H{"user": userPayload(h.DB, u)}, "登录成功")
}

type totpVerifyReq struct {
	TempToken string `json:"temp_token" binding:"required"`
	Code      string `json:"code" binding:"required"`
}

// LoginTOTP 完成二次验证：用临时令牌 + TOTP 验证码换取会话令牌。
func (h *AuthHandler) LoginTOTP(c *gin.Context) {
	var req totpVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	claims, err := security.ParseToken(h.Cfg.SecretKey, h.Cfg.JWTAlgorithm, req.TempToken)
	if err != nil || claims.Purpose != "totp" {
		response.BizFail(c, "临时凭证无效或已过期", 401)
		return
	}
	var u models.User
	if err := h.DB.First(&u, claims.UserID).Error; err != nil || u.Disabled != 0 || !u.CanWebLogin() || u.SessionVersion != claims.SessionVersion {
		response.BizFail(c, "账号不可用", 403)
		return
	}
	if u.TOTPSecret == nil || u.TOTPEnabled == 0 {
		response.BizFail(c, "未启用二次验证", 400)
		return
	}
	key := fmt.Sprintf("totp:%d", u.ID)
	for _, k := range []string{key, "totp-ip:" + c.ClientIP()} {
		if locked, _ := h.Limiter.AssertAllowed(k); locked {
			response.Fail(c, 429, "二次验证尝试过多，请稍后重试", 429)
			return
		}
	}
	step := int64(0)
	now := time.Now()
	for delta := -1; delta <= 1; delta++ {
		at := now.Add(time.Duration(delta) * 30 * time.Second)
		if valid, _ := totp.ValidateCustom(req.Code, *u.TOTPSecret, at, totp.ValidateOpts{Period: 30, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}); valid {
			step = at.Unix() / 30
			break
		}
	}
	if step == 0 {
		h.Limiter.RecordFailure(key)
		h.Limiter.RecordFailure("totp-ip:" + c.ClientIP())
		response.BizFail(c, "验证码错误", 400)
		return
	}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		exp := claims.ExpiresAt.Time
		if err := tx.Create(&models.AuthRevokedToken{JTI: claims.ID, RevokedAt: now, ExpiresAt: &exp}).Error; err != nil {
			return err
		}
		result := tx.Model(&models.User{}).Where("id = ? AND last_totp_step < ? AND session_version = ?", u.ID, step, claims.SessionVersion).Update("last_totp_step", step)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("验证码已使用")
		}
		return nil
	})
	if err != nil {
		response.BizFail(c, "凭据已使用，请重新登录或等待下一组验证码", 401)
		return
	}
	h.Limiter.Clear(key)

	token, _, err := security.CreateToken(h.Cfg.SecretKey, h.Cfg.JWTAlgorithm, h.Cfg.JWTExpireDays, u.ID, u.Username, u.Role, u.SessionVersion)
	if err != nil {
		response.BizFail(c, "签发凭证失败", 500)
		return
	}
	_ = h.DB.Model(&u).Update("last_login_at", time.Now())
	setSessionCookie(c, token, h.Cfg.JWTExpireDays*86400)
	services.LogAction(h.DB, c, "login", "success", "login.totp", &u, "")
	response.OK(c, gin.H{"user": userPayload(h.DB, u)}, "登录成功")
}

// CaptchaCreate 生成登录验证码图片。
func (h *AuthHandler) CaptchaCreate(c *gin.Context) {
	sec := settings.LoadSecurity(h.DB)
	ch, err := h.Captcha.Create(sec.Captcha)
	if err != nil {
		response.BizFail(c, "验证码生成失败", 500)
		return
	}
	response.OK(c, ch, "")
}

// CaptchaPreview 管理员预览验证码样式。
func (h *AuthHandler) CaptchaPreview(c *gin.Context) {
	var req struct {
		Type       string `json:"type"`
		Length     int    `json:"length"`
		NoiseLevel string `json:"noise_level"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
	}
	_ = c.ShouldBindJSON(&req)
	cfg := settings.CaptchaConfig{
		Type: req.Type, Length: req.Length, NoiseLevel: req.NoiseLevel,
		Width: req.Width, Height: req.Height,
	}
	img, err := h.Captcha.Preview(cfg)
	if err != nil {
		response.BizFail(c, "预览失败", 500)
		return
	}
	response.OK(c, gin.H{"image_base64": img}, "")
}

type totpSetupResp struct {
	Secret string `json:"secret"`
	OTPURL string `json:"otp_url"`
	QRURL  string `json:"qr_url"`
}

// TOTPSetup 生成新的 TOTP 密钥（未启用，需验证后生效）。
func (h *AuthHandler) TOTPSetup(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u.TOTPEnabled != 0 {
		response.BizFail(c, "已启用二次验证，请先禁用", 400)
		return
	}
	secret, otpURL, err := services.GenerateTOTPSecret("VideoAI", u.Username)
	if err != nil {
		response.BizFail(c, "生成密钥失败", 500)
		return
	}
	if err := h.DB.Model(&u).Update("totp_secret", secret).Error; err != nil {
		response.BizFail(c, "保存密钥失败", 500)
		return
	}
	response.OK(c, totpSetupResp{Secret: secret, OTPURL: otpURL, QRURL: services.QRCodeURL(otpURL)}, "")
}

type totpEnableReq struct {
	Code string `json:"code" binding:"required"`
}

// TOTPEnable 验证 TOTP 验证码并启用二次验证。
func (h *AuthHandler) TOTPEnable(c *gin.Context) {
	var req totpEnableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	if u.TOTPSecret == nil {
		response.BizFail(c, "请先生成密钥", 400)
		return
	}
	if !services.VerifyTOTP(*u.TOTPSecret, req.Code) {
		response.BizFail(c, "验证码错误", 400)
		return
	}
	if err := h.DB.Model(&u).Update("totp_enabled", 1).Error; err != nil {
		response.BizFail(c, "启用失败", 500)
		return
	}
	u.TOTPEnabled = 1
	services.LogAction(h.DB, c, "audit", "success", "auth.totp.enable", &u, "")
	response.OK(c, userPayload(h.DB, u), "二次验证已启用")
}

type totpDisableReq struct {
	Code string `json:"code" binding:"required"`
}

// TOTPDisable 验证 TOTP 验证码并禁用二次验证。
func (h *AuthHandler) TOTPDisable(c *gin.Context) {
	var req totpDisableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	if settings.LoadSecurity(h.DB).Admin2FAEnabled && u.IsAdmin() {
		response.BizFail(c, "管理员强制双因素认证期间不能关闭", 403)
		return
	}

	if u.TOTPEnabled == 0 || u.TOTPSecret == nil {
		response.BizFail(c, "未启用二次验证", 400)
		return
	}
	if !services.VerifyTOTP(*u.TOTPSecret, req.Code) {
		response.BizFail(c, "验证码错误", 400)
		return
	}
	if err := h.DB.Model(&u).Updates(map[string]any{"totp_enabled": 0, "totp_secret": nil, "last_totp_step": 0, "session_version": gorm.Expr("session_version + 1")}).Error; err != nil {
		response.BizFail(c, "禁用失败", 500)
		return
	}
	u.TOTPEnabled = 0
	u.TOTPSecret = nil
	services.LogAction(h.DB, c, "audit", "success", "auth.totp.disable", &u, "")
	response.OK(c, userPayload(h.DB, u), "二次验证已禁用")
}

// TOTPStatus 返回当前用户 TOTP 绑定状态与全局强制策略。
func (h *AuthHandler) TOTPStatus(c *gin.Context) {
	u := middleware.CurrentUser(c)
	sec := settings.LoadSecurity(h.DB)
	response.OK(c, gin.H{
		"enabled":            u.TOTPEnabled != 0,
		"enforced":           sec.Admin2FAEnabled && u.IsAdmin(),
		"admin_2fa_required": sec.Admin2FAEnabled && u.IsAdmin(),
	}, "")
}

func userPayload(db *gorm.DB, u models.User) gin.H {
	permissions, _ := services.NewRBACService(db).UserPermissionCodes(u)
	out := gin.H{
		"permissions": permissions,
		"user_id":     u.ID, "username": u.Username,
		"is_admin": u.IsAdmin(), "is_superadmin": u.IsSuperAdmin(),
		"role": u.Role, "is_system": u.IsSystemUser(),
		"real_name": u.RealName, "department_id": u.DepartmentID,
		"timezone": u.Timezone, "avatar_url": u.AvatarURL,
		"totp_enabled":         u.TOTPEnabled != 0,
		"auth_source":          u.AuthSource,
		"must_change_password": u.MustChangePassword != 0,
		"must_setup_totp":      settings.LoadSecurity(db).Admin2FAEnabled && u.IsAdmin() && u.TOTPEnabled == 0,
	}
	if u.DepartmentID != nil {
		var dept models.Department
		if err := db.First(&dept, *u.DepartmentID).Error; err == nil {
			out["department_name"] = dept.Name
		}
	}
	return out
}

func (h *AuthHandler) Me(c *gin.Context) {
	u := middleware.CurrentUser(c)
	response.OK(c, userPayload(h.DB, u), "")
}

func (h *AuthHandler) Profile(c *gin.Context) {
	var req profileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	updates := map[string]interface{}{}
	if req.RealName != nil {
		updates["real_name"] = req.RealName
	}
	if req.Timezone != nil && strings.TrimSpace(*req.Timezone) != "" {
		if _, err := time.LoadLocation(strings.TrimSpace(*req.Timezone)); err != nil {
			response.BizFail(c, "无效时区", 422)
			return
		}
		updates["timezone"] = strings.TrimSpace(*req.Timezone)
	}
	if len(updates) > 0 {
		if err := h.DB.Model(&u).Updates(updates).Error; err != nil {
			response.BizFail(c, "保存失败", 500)
			return
		}
		h.DB.First(&u, u.ID)
	}
	services.LogAction(h.DB, c, "operation", "success", "auth.profile.update", &u, "个人信息")
	response.OK(c, userPayload(h.DB, u), "已保存")
}

func (h *AuthHandler) Logout(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if err := h.revokeCurrentToken(c); err != nil {
		response.BizFail(c, "会话注销失败，请重试", 500)
		return
	}
	setSessionCookie(c, "", -1)
	services.LogAction(h.DB, c, "login", "success", "logout", &u, "")
	response.OK(c, nil, "已退出")
}

func setSessionCookie(c *gin.Context, token string, maxAge int) {
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "baseplatform_session", Value: token, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	sec := settings.LoadSecurity(h.DB)
	if err := services.ValidatePassword(req.NewPassword, sec); err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	u := middleware.CurrentUser(c)
	if !security.VerifyPassword(req.OldPassword, u.PasswordHash) {
		response.BizFail(c, "原密码错误", 400)
		return
	}
	hash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		response.BizFail(c, "密码处理失败", 500)
		return
	}
	if err := h.DB.Model(&u).Updates(map[string]interface{}{"password_hash": hash, "session_version": gorm.Expr("session_version + 1"), "must_change_password": 0}).Error; err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	setSessionCookie(c, "", -1)
	if err := h.revokeCurrentToken(c); err != nil {
		response.BizFail(c, "会话注销失败，请重试", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "auth.change_password", &u, "")
	response.OK(c, nil, "密码修改成功，请重新登录")
}

func (h *AuthHandler) revokeCurrentToken(c *gin.Context) error {
	claims, ok := c.Get(middleware.ContextClaimsKey)
	if !ok {
		return nil
	}
	cl := claims.(*security.Claims)
	var exp *time.Time
	if cl.ExpiresAt != nil {
		t := cl.ExpiresAt.Time
		exp = &t
	}
	return h.DB.Create(&models.AuthRevokedToken{JTI: cl.ID, RevokedAt: time.Now(), ExpiresAt: exp}).Error
}

func (h *AuthHandler) Timezones(c *gin.Context) {
	zones := []string{"UTC", "Asia/Shanghai", "Asia/Hong_Kong", "Asia/Tokyo", "America/New_York", "Europe/London"}
	response.OK(c, gin.H{"items": zones}, "")
}
