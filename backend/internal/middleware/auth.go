package middleware

import (
	"net/http"
	"strings"
	"time"

	"baseplatform/internal/config"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/security"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const ContextUserKey = "current_user"
const ContextClaimsKey = "claims"
const ContextTokenConstraintsKey = "token_constraints"
const ContextAuthViaPATKey = "auth_via_pat"
const ContextPATTokenIDKey = "pat_token_id"
const sessionCookieName = "baseplatform_session"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.New().String()
		}
		c.Header("X-Request-ID", rid)
		c.Set("request_id", rid)
		c.Next()
	}
}

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; media-src 'self' blob:; frame-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'")
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

func CORS(cfg *config.Config) gin.HandlerFunc {
	allowed := make(map[string]struct{})
	wildcard := false
	for _, raw := range strings.Split(cfg.CORSOrigins, ",") {
		origin := strings.TrimSpace(raw)
		if origin == "*" {
			wildcard = true
		} else if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		requestOrigin := strings.TrimSpace(c.GetHeader("Origin"))
		if wildcard {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if _, ok := allowed[requestOrigin]; ok && requestOrigin != "" {
			c.Header("Access-Control-Allow-Origin", requestOrigin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-API-Key")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func ExtractToken(c *gin.Context) string {
	if key := strings.TrimSpace(c.GetHeader("X-API-Key")); key != "" {
		return key
	}
	auth := c.GetHeader("Authorization")
	if auth != "" {
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			return strings.TrimSpace(auth[7:])
		}
		return strings.TrimSpace(auth)
	}
	if token, err := c.Cookie(sessionCookieName); err == nil {
		return strings.TrimSpace(token)
	}
	return ""
}

func authUsesCookie(c *gin.Context) bool {
	return strings.TrimSpace(c.GetHeader("Authorization")) == "" && strings.TrimSpace(c.GetHeader("X-API-Key")) == ""
}

func validCookieOrigin(c *gin.Context) bool {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin == "" {
		return false
	}
	wantHTTP := "http://" + c.Request.Host
	wantHTTPS := "https://" + c.Request.Host
	return origin == wantHTTP || origin == wantHTTPS
}

func Auth(cfg *config.Config, db *gorm.DB, pat *services.PATService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ExtractToken(c)
		if token == "" {
			response.Fail(c, http.StatusUnauthorized, "未登录", 401)
			c.Abort()
			return
		}
		if authUsesCookie(c) && !validCookieOrigin(c) {
			response.Fail(c, http.StatusForbidden, "请求来源无效", 403)
			c.Abort()
			return
		}
		claims, err := security.ParseToken(cfg.SecretKey, cfg.JWTAlgorithm, token)
		if err == nil {
			if claims.Purpose != "" {
				response.Fail(c, http.StatusUnauthorized, "令牌用途无效", 401)
				c.Abort()
				return
			}
			var revoked []models.AuthRevokedToken
			if err := db.Select("id").Where("jti = ?", claims.ID).Limit(1).Find(&revoked).Error; err != nil {
				response.Fail(c, http.StatusInternalServerError, "认证检查失败", 500)
				c.Abort()
				return
			}
			if len(revoked) > 0 {
				response.Fail(c, http.StatusUnauthorized, "登录已失效，请重新登录", 401)
				c.Abort()
				return
			}
			var user models.User
			if err := db.First(&user, claims.UserID).Error; err != nil {
				response.Fail(c, http.StatusUnauthorized, "用户不存在", 401)
				c.Abort()
				return
			}
			if user.Disabled != 0 || user.SessionVersion != claims.SessionVersion {
				response.Fail(c, http.StatusForbidden, "账号已禁用", 403)
				c.Abort()
				return
			}
			c.Set(ContextUserKey, user)
			c.Set(ContextClaimsKey, claims)
			c.Next()
			return
		}
		// JWT 解析失败，尝试 PAT 认证
		if pat != nil && services.IsPAT(token) {
			user, patRow, perr := pat.Authenticate(token)
			if perr == nil && user != nil && patRow != nil {
				c.Set(ContextUserKey, *user)
				c.Set(ContextAuthViaPATKey, true)
				c.Set(ContextPATTokenIDKey, patRow.ID)
				c.Next()
				return
			}
			if perr == services.ErrTokenDisabled {
				response.Fail(c, http.StatusForbidden, "令牌已禁用", 403)
				c.Abort()
				return
			}
			if perr == services.ErrTokenExpired {
				response.Fail(c, http.StatusUnauthorized, "令牌已过期", 401)
				c.Abort()
				return
			}
			response.Fail(c, http.StatusUnauthorized, "令牌无效", 401)
			c.Abort()
			return
		}
		response.Fail(c, http.StatusUnauthorized, "登录已过期", 401)
		c.Abort()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := c.MustGet(ContextUserKey).(models.User)
		if !ok || !u.IsAdmin() {
			response.Fail(c, http.StatusForbidden, "需要管理员权限", 403)
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireSuperAdmin 管理后台删除类操作：仅超级管理员。
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := c.MustGet(ContextUserKey).(models.User)
		if !ok || !u.IsSuperAdmin() {
			response.Fail(c, http.StatusForbidden, "需要超级管理员权限", 403)
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequirePermission 检查当前用户是否拥有指定权限点。
// 超级管理员内置拥有全部权限；其他用户按角色-权限关联判定。
func RequirePermission(db *gorm.DB, code string) gin.HandlerFunc {
	rbac := services.NewRBACService(db)
	return func(c *gin.Context) {
		u, ok := c.MustGet(ContextUserKey).(models.User)
		if !ok {
			response.Fail(c, http.StatusUnauthorized, "未登录", 401)
			c.Abort()
			return
		}
		if !rbac.HasPermission(u, code) {
			response.Fail(c, http.StatusForbidden, "无操作权限", 403)
			c.Abort()
			return
		}
		c.Next()
	}
}

func ClientIP(c *gin.Context) string {
	return c.ClientIP()
}

func RequestIDFromContext(c *gin.Context) string {
	if v, ok := c.Get("request_id"); ok {
		return v.(string)
	}
	return ""
}

func CurrentUser(c *gin.Context) models.User {
	return c.MustGet(ContextUserKey).(models.User)
}

// RequestFileSource 解析操作来源：优先 X-NextFile-Source，其次 Web/PAT。
func RequestFileSource(c *gin.Context) uint8 {
	if src := ParseNextFileSourceHeader(c.GetHeader("X-NextFile-Source")); src != 0 {
		return src
	}
	return models.FileSourceWeb
}

// ParseNextFileSourceHeader 解析客户端自报来源（见 docs/rest-api.md）。
func ParseNextFileSourceHeader(v string) uint8 {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "desktop", "desktop_sync", "sync-drive", "sync_drive":
		return models.FileSourceDesktop
	case "desktop-linux", "desktop_linux":
		return models.FileSourceDesktopLinux
	case "desktop-darwin", "desktop_darwin", "desktop-macos", "desktop_macos":
		return models.FileSourceDesktopDarwin
	case "desktop-windows", "desktop_windows":
		return models.FileSourceDesktopWindows
	case "webdav":
		return models.FileSourceWebDAV
	case "smb":
		return models.FileSourceSMB
	case "ftp":
		return models.FileSourceFTP
	case "sftp":
		return models.FileSourceSFTP
	case "s3":
		return models.FileSourceS3
	case "nfs":
		return models.FileSourceNFS
	case "web":
		return models.FileSourceWeb
	case "api":
		return models.FileSourceAPI
	}
	return 0
}

// APICallLogger 记录 PAT 令牌调用日志。仅对通过 PAT 认证的请求写入记录，
// 避免对 Web 端正常请求产生写库压力。日志写入在 goroutine 中异步执行。
func APICallLogger(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		viaPAT, _ := c.Get(ContextAuthViaPATKey)
		if viaPAT == true {
			tokenID, _ := c.Get(ContextPATTokenIDKey)
			user := CurrentUser(c)
			ua := c.GetHeader("User-Agent")
			if len(ua) > 512 {
				ua = ua[:512]
			}
			path := c.Request.URL.Path
			if len(path) > 512 {
				path = path[:512]
			}
			var tid uint
			if v, ok := tokenID.(uint); ok {
				tid = v
			}
			log := models.ApiCallLog{
				TokenID:    tid,
				UserID:     user.ID,
				Method:     c.Request.Method,
				Path:       path,
				StatusCode: c.Writer.Status(),
				DurationMs: time.Since(start).Milliseconds(),
				IP:         c.ClientIP(),
				UserAgent:  ua,
				CreatedAt:  start,
			}
			go func() {
				_ = db.Create(&log).Error
			}()
		}
	}
}

// AccountSetupGate applies equally to browser and API-token authentication.
func AccountSetupGate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		path := c.FullPath()
		basic := path == "/api/auth/me" || path == "/api/auth/logout" || path == "/api/auth/change-password" || path == "/api/auth/permissions" || path == "/api/auth/timezones"
		if u.MustChangePassword != 0 && !basic {
			response.Fail(c, 403, "请先修改初始密码", 403)
			c.Abort()
			return
		}
		if settings.LoadSecurity(db).Admin2FAEnabled && u.IsAdmin() && u.TOTPEnabled == 0 && !basic && path != "/api/auth/totp/status" && path != "/api/auth/totp/setup" && path != "/api/auth/totp/enable" {
			response.Fail(c, 403, "请先在个人中心绑定双因素认证", 403)
			c.Abort()
			return
		}
		c.Next()
	}
}
