package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/security"
	"baseplatform/internal/settings"

	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// CloudOAuthService 处理云盘（OneDrive/Google Drive/百度网盘）的 OAuth 授权流程。
type CloudOAuthService struct {
	DB *gorm.DB
}

// cloudOAuthState 是写入签名 state 的载荷；redirect_uri 一并放入，
// 这样 Callback 在交换 token 时无需再由调用方传入回调地址。
type cloudOAuthState struct {
	StorageID   uint   `json:"storage_id"`
	Platform    string `json:"platform"`
	RedirectURI string `json:"redirect_uri,omitempty"`
	Nonce       string `json:"nonce"`
	Exp         int64  `json:"exp"`
}

// cloudDriverPlatforms 把云盘存储驱动映射到 OAuth 平台标识。
var cloudDriverPlatforms = map[string]string{
	"onedrive":     "microsoft",
	"google_drive": "google",
	"baidu_pan":    "baidu",
}

// CloudPlatformForDriver 返回云盘驱动对应的 OAuth 平台标识。
func CloudPlatformForDriver(driver string) (string, bool) {
	p, ok := cloudDriverPlatforms[driver]
	return p, ok
}

// IsCloudStorageDriver 判断驱动是否为云盘类（需 OAuth 授权）。
func IsCloudStorageDriver(driver string) bool {
	_, ok := cloudDriverPlatforms[driver]
	return ok
}

// FindApp 在云盘配置中查找指定平台的应用凭证。
func (s *CloudOAuthService) FindApp(cfg settings.CloudStorage, platform string) (*settings.CloudStorageApp, error) {
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if a.Platform == platform {
			return a, nil
		}
	}
	return nil, fmt.Errorf("未配置云盘应用: %s", platform)
}

func (s *CloudOAuthService) oauthConfig(app *settings.CloudStorageApp, redirectURI string) *oauth2.Config {
	authURL, tokenURL := app.AuthURL, app.TokenURL
	scopes := splitOAuthScopes(app.Scopes)
	switch app.Platform {
	case "microsoft":
		if authURL == "" {
			authURL = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
		}
		if tokenURL == "" {
			tokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
		}
		if len(scopes) == 0 {
			scopes = []string{"offline_access", "Files.ReadWrite.All", "User.Read"}
		}
	case "google":
		if authURL == "" {
			authURL = "https://accounts.google.com/o/oauth2/v2/auth"
		}
		if tokenURL == "" {
			tokenURL = "https://oauth2.googleapis.com/token"
		}
		if len(scopes) == 0 {
			scopes = []string{"https://www.googleapis.com/auth/drive.file", "offline_access"}
		}
	case "baidu":
		if authURL == "" {
			authURL = "https://openapi.baidu.com/oauth/2.0/authorize"
		}
		if tokenURL == "" {
			tokenURL = "https://openapi.baidu.com/oauth/2.0/token"
		}
		if len(scopes) == 0 {
			scopes = []string{"basic", "netdisk"}
		}
		scopes = filterBaiduOAuthScopes(scopes)
	}
	return &oauth2.Config{
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}
}

// CloudOAuthRedirectURI 生成云盘 OAuth 回调地址。
func CloudOAuthRedirectURI(baseURL, platform string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	return baseURL + "/api/storages/cloud-oauth/" + url.PathEscape(platform) + "/callback"
}

// BuildAuthURL 构建云盘授权 URL。state 中携带 storageID/platform/redirectURI 并签名。
func (s *CloudOAuthService) BuildAuthURL(storageID uint, platform, redirectURI, secretKey string) (string, error) {
	cfg := settings.LoadCloudStorage(s.DB)
	if !cfg.Enabled {
		return "", fmt.Errorf("云盘对接未启用，请先在系统配置中开启")
	}
	app, err := s.FindApp(cfg, platform)
	if err != nil {
		return "", err
	}
	if !app.Enabled || app.ClientID == "" || app.ClientSecret == "" {
		return "", fmt.Errorf("请先在「云盘对接」中启用并填写 %s 的应用凭证", app.Name)
	}
	state, err := s.newState(storageID, platform, redirectURI, secretKey)
	if err != nil {
		return "", err
	}
	ocfg := s.oauthConfig(app, redirectURI)
	var opts []oauth2.AuthCodeOption
	switch platform {
	case "baidu":
		opts = append(opts,
			oauth2.SetAuthURLParam("confirm_login", "1"),
			oauth2.SetAuthURLParam("display", "page"),
		)
	default:
		opts = append(opts, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	}
	return ocfg.AuthCodeURL(state, opts...), nil
}

// filterBaiduOAuthScopes 移除百度当前应用未开放的 scope（如 share）。
func filterBaiduOAuthScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{"basic", "netdisk"}
	}
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if s == "share" {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{"basic", "netdisk"}
	}
	return out
}

func (s *CloudOAuthService) newState(storageID uint, platform, redirectURI, secretKey string) (string, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	st := cloudOAuthState{
		StorageID:   storageID,
		Platform:    platform,
		RedirectURI: redirectURI,
		Nonce:       hex.EncodeToString(nonce),
		Exp:         time.Now().Add(15 * time.Minute).Unix(),
	}
	raw, _ := json.Marshal(st)
	return security.SignState(string(raw), secretKey)
}

// ParseState 校验并解析 state（供 handler 直接使用）。
func (s *CloudOAuthService) ParseState(state, secretKey string) (*cloudOAuthState, error) {
	raw, err := security.VerifyState(state, secretKey)
	if err != nil {
		return nil, err
	}
	var st cloudOAuthState
	if json.Unmarshal([]byte(raw), &st) != nil {
		return nil, fmt.Errorf("invalid state")
	}
	if time.Now().Unix() > st.Exp {
		return nil, fmt.Errorf("state expired")
	}
	return &st, nil
}

// Callback 处理云盘授权回调：校验 state、交换 token 并写回存储源配置。
// state 由 BuildAuthURL 生成，其中已携带 redirect_uri，故此处无需再传入回调地址。
func (s *CloudOAuthService) Callback(storageID uint, platform, code, state, secretKey string) error {
	st, err := s.ParseState(state, secretKey)
	if err != nil {
		return err
	}
	if st.Platform != platform {
		return fmt.Errorf("state 平台不匹配")
	}
	if st.StorageID != storageID {
		return fmt.Errorf("state 存储源不匹配")
	}
	return s.exchangeAndSave(st.StorageID, platform, st.RedirectURI, code)
}

func (s *CloudOAuthService) exchangeAndSave(storageID uint, platform, redirectURI, code string) error {
	cfg := settings.LoadCloudStorage(s.DB)
	app, err := s.FindApp(cfg, platform)
	if err != nil {
		return err
	}
	ocfg := s.oauthConfig(app, redirectURI)
	token, err := ocfg.Exchange(context.Background(), code)
	if err != nil {
		return err
	}
	var st models.Storage
	if err := s.DB.First(&st, storageID).Error; err != nil {
		return fmt.Errorf("存储源不存在")
	}
	expectedPlatform, ok := CloudPlatformForDriver(st.Driver)
	if !ok || expectedPlatform != platform {
		return fmt.Errorf("存储驱动与云盘平台不匹配")
	}
	config := map[string]interface{}{}
	if st.Config != "" {
		_ = json.Unmarshal([]byte(st.Config), &config)
	}
	config["platform"] = platform
	config["connected"] = true
	config["access_token"] = token.AccessToken
	if token.RefreshToken != "" {
		config["refresh_token"] = token.RefreshToken
	}
	if !token.Expiry.IsZero() {
		config["token_expiry"] = token.Expiry.UTC().Format(time.RFC3339)
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	st.Config = string(raw)
	return s.DB.Save(&st).Error
}

// CloudStorageConfigSummary 返回云盘配置的对外可见摘要（不含 token 明文）。
func CloudStorageConfigSummary(raw string) map[string]interface{} {
	out := map[string]interface{}{}
	if raw == "" {
		return out
	}
	var cfg map[string]interface{}
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		return out
	}
	for _, k := range []string{"platform", "remote_path", "connected", "account_name", "token_expiry", "oauth_scope"} {
		if v, ok := cfg[k]; ok {
			out[k] = v
		}
	}
	if _, ok := out["connected"]; !ok {
		out["connected"] = cfg["access_token"] != nil && cfg["access_token"] != ""
	}
	return out
}

// MergeCloudStorageConfig 合并云盘配置：保留既有 token，仅更新传入的非敏感字段。
func MergeCloudStorageConfig(driver string, raw map[string]interface{}, existing string) (string, error) {
	cfg := map[string]interface{}{}
	if existing != "" {
		_ = json.Unmarshal([]byte(existing), &cfg)
	}
	for k, v := range raw {
		if k == "access_token" || k == "refresh_token" {
			continue
		}
		cfg[k] = v
	}
	if platform, ok := CloudPlatformForDriver(driver); ok {
		cfg["platform"] = platform
	}
	if remote, ok := raw["remote_path"].(string); ok {
		cfg["remote_path"] = strings.TrimSpace(remote)
	}
	b, err := json.Marshal(cfg)
	return string(b), err
}

// StorageIDFromParam 把路由参数解析为存储源 ID。
func StorageIDFromParam(id string) (uint, error) {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(n), nil
}


