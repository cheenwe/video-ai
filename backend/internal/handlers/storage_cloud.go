package handlers

import (
	"net/http"
	"os"
	"strings"

	"baseplatform/internal/config"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type StorageCloudOAuthHandler struct {
	DB         *gorm.DB
	Cfg        *config.Config
	CloudOAuth *services.CloudOAuthService
}

func (h *StorageCloudOAuthHandler) cloudOAuthBaseURL(c *gin.Context) string {
	if h.Cfg != nil {
		if v := strings.TrimSpace(os.Getenv("BASE_PUBLIC_URL")); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return publicBaseURL(c, h.Cfg)
}

func publicBaseURL(c *gin.Context, cfg *config.Config) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// CallbackURLPreview 返回当前请求下将使用的 OAuth 回调地址（供管理页展示与复制）。
func (h *StorageCloudOAuthHandler) CallbackURLPreview(c *gin.Context) {
	platform := strings.TrimSpace(c.Query("platform"))
	if platform == "" {
		response.BizFail(c, "缺少 platform 参数", 400)
		return
	}
	baseURL := h.cloudOAuthBaseURL(c)
	redirectURI := services.CloudOAuthRedirectURI(baseURL, platform)
	response.OK(c, gin.H{
		"redirect_uri": redirectURI,
		"base_url":     baseURL,
		"request_host": c.Request.Host,
	}, "")
}

// AuthURL 返回云厂商授权页 URL（需管理员鉴权；供 SPA 先请求再跳转）。
func (h *StorageCloudOAuthHandler) AuthURL(c *gin.Context) {
	idStr := c.Param("id")
	id, err := services.StorageIDFromParam(idStr)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var storage models.Storage
	if err := h.DB.First(&storage, id).Error; err != nil {
		response.BizFail(c, "存储源不存在", 404)
		return
	}
	platform, ok := services.CloudPlatformForDriver(storage.Driver)
	if !ok {
		response.BizFail(c, "该存储驱动不支持云盘 OAuth 授权", 400)
		return
	}
	baseURL := h.cloudOAuthBaseURL(c)
	redirectURI := services.CloudOAuthRedirectURI(baseURL, platform)
	url, err := h.CloudOAuth.BuildAuthURL(id, platform, redirectURI, h.Cfg.SecretKey)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	response.OK(c, gin.H{"url": url, "redirect_uri": redirectURI}, "")
}

// Start 浏览器直接跳转入口；认证使用会话 Cookie 或 Authorization 请求头。
func (h *StorageCloudOAuthHandler) Start(c *gin.Context) {
	idStr := c.Param("id")
	id, err := services.StorageIDFromParam(idStr)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var storage models.Storage
	if err := h.DB.First(&storage, id).Error; err != nil {
		response.BizFail(c, "存储源不存在", 404)
		return
	}
	platform, ok := services.CloudPlatformForDriver(storage.Driver)
	if !ok {
		response.BizFail(c, "该存储驱动不支持云盘 OAuth 授权", 400)
		return
	}
	baseURL := h.cloudOAuthBaseURL(c)
	redirectURI := services.CloudOAuthRedirectURI(baseURL, platform)
	url, err := h.CloudOAuth.BuildAuthURL(id, platform, redirectURI, h.Cfg.SecretKey)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	c.Redirect(http.StatusFound, url)
}

// Callback 云盘 OAuth 回调，交换 token 并写回存储源配置。
func (h *StorageCloudOAuthHandler) Callback(c *gin.Context) {
	platform := strings.TrimSpace(c.Param("platform"))
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		response.BizFail(c, "授权码或状态缺失", 400)
		return
	}
	st, err := h.CloudOAuth.ParseState(state, h.Cfg.SecretKey)
	if err != nil {
		response.BizFail(c, "授权状态无效或已过期", 400)
		return
	}
	if platform != "" && st.Platform != platform {
		response.BizFail(c, "平台不匹配", 400)
		return
	}
	if err := h.CloudOAuth.Callback(st.StorageID, st.Platform, code, state, h.Cfg.SecretKey); err != nil {
		response.BizFail(c, err.Error(), 502)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "storage.cloud.oauth.callback", nil, st.Platform)
	response.OK(c, nil, "授权成功")
}
