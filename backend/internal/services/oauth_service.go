package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/security"
	"baseplatform/internal/settings"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
)

type OAuthService struct {
	DB *gorm.DB
}

type oauthState struct {
	ProviderID string `json:"provider_id"`
	Nonce      string `json:"nonce"`
	Exp        int64  `json:"exp"`
}

func (s *OAuthService) FindProvider(cfg settings.OAuth, id string) (*settings.OAuthProvider, error) {
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if p.ID == id && p.Enabled {
			return p, nil
		}
	}
	return nil, fmt.Errorf("OAuth 提供商不存在或未启用")
}

func (s *OAuthService) BuildAuthURL(cfg settings.OAuth, provider *settings.OAuthProvider, redirectURI, secret string) (string, error) {
	state, err := s.newState(provider.ID, secret)
	if err != nil {
		return "", err
	}
	ocfg := s.oauthConfig(provider, redirectURI)
	return ocfg.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
}

func (s *OAuthService) oauthConfig(p *settings.OAuthProvider, redirectURI string) *oauth2.Config {
	authURL, tokenURL := p.AuthURL, p.TokenURL
	scopes := splitOAuthScopes(p.Scopes)
	switch p.Type {
	case "github":
		authURL = "https://github.com/login/oauth/authorize"
		tokenURL = "https://github.com/login/oauth/access_token"
		if len(scopes) == 0 {
			scopes = []string{"read:user", "user:email"}
		}
	case "microsoft":
		authURL = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
		tokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
		if len(scopes) == 0 {
			scopes = []string{"openid", "profile", "email", "User.Read", "offline_access"}
		}
	case "google":
		authURL = google.Endpoint.AuthURL
		tokenURL = google.Endpoint.TokenURL
		if len(scopes) == 0 {
			scopes = []string{"openid", "profile", "email"}
		}
	}
	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}
}

func splitOAuthScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, ",") && !strings.Contains(raw, " ") {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	}
	return strings.Fields(raw)
}

func (s *OAuthService) newState(providerID, secret string) (string, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	st := oauthState{ProviderID: providerID, Nonce: hex.EncodeToString(nonce), Exp: time.Now().Add(10 * time.Minute).Unix()}
	raw, _ := json.Marshal(st)
	return security.SignState(string(raw), secret)
}

func (s *OAuthService) ParseState(state, secret string) (*oauthState, error) {
	raw, err := security.VerifyState(state, secret)
	if err != nil {
		return nil, err
	}
	var st oauthState
	if json.Unmarshal([]byte(raw), &st) != nil {
		return nil, fmt.Errorf("invalid state")
	}
	if time.Now().Unix() > st.Exp {
		return nil, fmt.Errorf("state expired")
	}
	return &st, nil
}

func (s *OAuthService) Exchange(cfg settings.OAuth, provider *settings.OAuthProvider, redirectURI, code string) (*oauth2.Token, error) {
	ocfg := s.oauthConfig(provider, redirectURI)
	return ocfg.Exchange(context.Background(), code)
}

type OAuthProfile struct {
	ID       string
	Username string
	Email    string
	Name     string
}

func (s *OAuthService) FetchProfile(provider *settings.OAuthProvider, token *oauth2.Token) (*OAuthProfile, error) {
	switch provider.Type {
	case "github":
		return s.fetchGitHub(token.AccessToken)
	case "microsoft":
		return s.fetchMicrosoft(token.AccessToken)
	case "google":
		return s.fetchGoogle(token.AccessToken)
	default:
		if provider.UserInfoURL == "" {
			return nil, fmt.Errorf("未配置用户信息 URL")
		}
		return s.fetchGeneric(provider.UserInfoURL, token.AccessToken)
	}
}

func (s *OAuthService) fetchMicrosoft(accessToken string) (*OAuthProfile, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Microsoft Graph 错误: %s", string(body))
	}
	var data struct {
		ID                string `json:"id"`
		DisplayName       string `json:"displayName"`
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
	}
	if json.Unmarshal(body, &data) != nil {
		return nil, fmt.Errorf("解析 Microsoft 用户信息失败")
	}
	email := data.Mail
	if email == "" {
		email = data.UserPrincipalName
	}
	username := email
	if at := strings.Index(username, "@"); at > 0 {
		username = username[:at]
	}
	return &OAuthProfile{ID: data.ID, Username: username, Email: email, Name: data.DisplayName}, nil
}

func (s *OAuthService) fetchGoogle(accessToken string) (*OAuthProfile, error) {
	return s.fetchGeneric("https://www.googleapis.com/oauth2/v3/userinfo", accessToken)
}

func (s *OAuthService) fetchGitHub(accessToken string) (*OAuthProfile, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitHub API 错误: %s", string(body))
	}
	var data struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if json.Unmarshal(body, &data) != nil {
		return nil, fmt.Errorf("解析 GitHub 用户信息失败")
	}
	email := data.Email
	if email == "" {
		email = s.fetchGitHubPrimaryEmail(accessToken)
	}
	return &OAuthProfile{ID: fmt.Sprintf("%d", data.ID), Username: data.Login, Email: email, Name: data.Name}, nil
}

func (s *OAuthService) fetchGitHubPrimaryEmail(accessToken string) string {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if json.NewDecoder(resp.Body).Decode(&emails) != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email
		}
	}
	return ""
}

func (s *OAuthService) fetchGeneric(userInfoURL, accessToken string) (*OAuthProfile, error) {
	req, _ := http.NewRequest(http.MethodGet, userInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return nil, fmt.Errorf("解析用户信息失败")
	}
	id := fmt.Sprint(data["id"])
	if id == "" {
		id = fmt.Sprint(data["sub"])
	}
	username := fmt.Sprint(data["login"])
	if username == "" {
		username = fmt.Sprint(data["username"])
	}
	if username == "" {
		username = fmt.Sprint(data["preferred_username"])
	}
	email := fmt.Sprint(data["email"])
	name := fmt.Sprint(data["name"])
	return &OAuthProfile{ID: id, Username: username, Email: email, Name: name}, nil
}

func (s *OAuthService) FindOrCreateUser(providerID string, profile *OAuthProfile) (*models.User, error) {
	var link models.OAuthAccount
	err := s.DB.Where("provider_id = ? AND subject = ?", providerID, profile.ID).First(&link).Error
	if err == nil {
		var user models.User
		if err := s.DB.First(&user, link.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}
	username := sanitizeUsername(profile.Username)
	if username == "" {
		username = "oauth_" + profile.ID
	}
	base := username
	for i := 0; ; i++ {
		var count int64
		s.DB.Model(&models.User{}).Where("username = ?", username).Count(&count)
		if count == 0 {
			break
		}
		username = fmt.Sprintf("%s_%d", base, i+1)
	}
	hash, _ := security.HashPassword(hex.EncodeToString(mustRand(16)))
	var email *string
	if profile.Email != "" {
		e := strings.ToLower(profile.Email)
		email = &e
	}
	user := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		Role:         "user",
		Timezone:     "UTC",
	}
	if err := s.DB.Create(&user).Error; err != nil {
		return nil, err
	}
	if err := s.DB.Create(&models.OAuthAccount{
		UserID:     user.ID,
		ProviderID: providerID,
		Subject:    profile.ID,
	}).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func sanitizeUsername(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func mustRand(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func OAuthRedirectURI(baseURL, providerID string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	return baseURL + "/api/auth/oauth/" + url.PathEscape(providerID) + "/callback"
}
