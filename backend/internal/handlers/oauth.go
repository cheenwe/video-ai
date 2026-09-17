package handlers

import (
	"net/http"
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

type OAuthHandler struct {
	DB      *gorm.DB
	Cfg     *config.Config
	OAuth   *services.OAuthService
	BaseURL string
}

// Begin 跳转到 OAuth 提供商授权页。
func (h *OAuthHandler) Begin(c *gin.Context) {
	providerID := strings.TrimSpace(c.Param("provider"))
	cfg := settings.LoadOAuth(h.DB)
	if !cfg.Enabled {
		response.BizFail(c, "OAuth 未启用", 403)
		return
	}
	provider, err := h.OAuth.FindProvider(cfg, providerID)
	if err != nil {
		response.BizFail(c, err.Error(), 404)
		return
	}
	redirectURI := services.OAuthRedirectURI(h.BaseURL, provider.ID)
	url, err := h.OAuth.BuildAuthURL(cfg, provider, redirectURI, h.Cfg.SecretKey)
	if err != nil {
		response.BizFail(c, "构建授权链接失败", 500)
		return
	}
	c.Redirect(http.StatusFound, url)
}

// Callback OAuth 提供商回调，交换 token 并登录。
func (h *OAuthHandler) Callback(c *gin.Context) {
	providerID := strings.TrimSpace(c.Param("provider"))
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		response.BizFail(c, "授权码或状态缺失", 400)
		return
	}
	cfg := settings.LoadOAuth(h.DB)
	if !cfg.Enabled {
		response.BizFail(c, "OAuth 未启用", 403)
		return
	}
	provider, err := h.OAuth.FindProvider(cfg, providerID)
	if err != nil {
		response.BizFail(c, err.Error(), 404)
		return
	}
	st, err := h.OAuth.ParseState(state, h.Cfg.SecretKey)
	if err != nil {
		response.BizFail(c, "授权状态无效或已过期", 400)
		return
	}
	if st.ProviderID != provider.ID {
		response.BizFail(c, "提供商不匹配", 400)
		return
	}
	redirectURI := services.OAuthRedirectURI(h.BaseURL, provider.ID)
	token, err := h.OAuth.Exchange(cfg, provider, redirectURI, code)
	if err != nil {
		response.BizFail(c, "交换令牌失败", 502)
		return
	}
	profile, err := h.OAuth.FetchProfile(provider, token)
	if err != nil {
		response.BizFail(c, err.Error(), 502)
		return
	}
	user, err := h.OAuth.FindOrCreateUser(provider.ID, profile)
	if err != nil {
		response.BizFail(c, "登录失败", 500)
		return
	}
	if user.Disabled != 0 || !user.CanWebLogin() {
		response.BizFail(c, "账号不可用", 403)
		return
	}
	if user.TOTPEnabled != 0 {
		temp, _, e := security.CreatePurposeToken(h.Cfg.SecretKey, h.Cfg.JWTAlgorithm, 5, user.ID, user.Username, user.Role, "totp", user.SessionVersion)
		if e != nil {
			response.BizFail(c, "签发二次验证凭据失败", 500)
			return
		}
		response.OK(c, gin.H{"totp_required": true, "temp_token": temp}, "需要二次验证")
		return
	}
	jwt, _, err := security.CreateToken(h.Cfg.SecretKey, h.Cfg.JWTAlgorithm, h.Cfg.JWTExpireDays, user.ID, user.Username, user.Role, user.SessionVersion)
	if err != nil {
		response.BizFail(c, "签发凭证失败", 500)
		return
	}
	h.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("last_login_at", time.Now())
	setSessionCookie(c, jwt, h.Cfg.JWTExpireDays*86400)
	services.LogAction(h.DB, c, "login", "success", "login.oauth", user, provider.ID)
	response.OK(c, gin.H{"user": userPayload(h.DB, *user)}, "登录成功")
}

// AdminListProviders 管理员查看 OAuth 配置。
func (h *OAuthHandler) AdminList(c *gin.Context) {
	cfg := settings.LoadOAuth(h.DB)
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "settings.oauth.view", &me, "")
	response.OK(c, cfg, "")
}

// AdminSave 保存 OAuth 配置。
func (h *OAuthHandler) AdminSave(c *gin.Context) {
	var cfg settings.OAuth
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if p.ID == "" {
			response.BizFail(c, "提供商 ID 不能为空", 422)
			return
		}
		if p.Name == "" {
			response.BizFail(c, "提供商名称不能为空", 422)
			return
		}
	}
	me := middleware.CurrentUser(c)
	if err := settings.SaveSection(h.DB, "oauth", cfg, &me.ID); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "settings.oauth.save", &me, "")
	response.OK(c, cfg, "已保存")
}
