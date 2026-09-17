package settings

import "strings"

type General struct {
	PageSize       int    `json:"page_size"`
	LocalFilesRoot string `json:"local_files_root"`
	NtpEnabled     bool   `json:"ntp_enabled"`
	NtpServer      string `json:"ntp_server"`
}

type OAuthProvider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // github, microsoft, google, custom
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthURL      string `json:"auth_url,omitempty"`
	TokenURL     string `json:"token_url,omitempty"`
	UserInfoURL  string `json:"user_info_url,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	Enabled      bool   `json:"enabled"`
}

type OAuth struct {
	Enabled   bool            `json:"enabled"`
	Providers []OAuthProvider `json:"providers"`
}

type PublicOAuthProvider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	IconURL string `json:"icon_url,omitempty"`
}

type EmailTemplates struct {
	ResetPassword      string `json:"reset_password"`
	AdminResetPassword string `json:"admin_reset_password"`
	Test               string `json:"test"`
}

type Email struct {
	Enabled   bool           `json:"enabled"`
	SMTPHost  string         `json:"smtp_host"`
	SMTPPort  int            `json:"smtp_port"`
	SMTPUser  string         `json:"smtp_user"`
	SMTPPass  string         `json:"smtp_pass"`
	From      string         `json:"from"`
	UseTLS    bool           `json:"use_tls"`
	Templates EmailTemplates `json:"templates"`
}

type LDAP struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	UseTLS        bool   `json:"use_tls"`
	BindDN        string `json:"bind_dn"`
	BindPass      string `json:"bind_pass"`
	BaseDN        string `json:"base_dn"`
	UserFilter    string `json:"user_filter"`
	UsernameAttr  string `json:"username_attr"`
	EmailAttr     string `json:"email_attr"`
	SyncEnabled   bool   `json:"sync_enabled"`
	DeptBaseDN    string `json:"dept_base_dn"`
	DeptFilter    string `json:"dept_filter"`
	DeptNameAttr  string `json:"dept_name_attr"`
	UserSyncFilter string `json:"user_sync_filter"`
	TestUsername  string `json:"test_username"`
}

type Gateway struct {
	PersonalTokensEnabled bool `json:"personal_tokens_enabled"`
}

type Security struct {
	CaptchaEnabled         bool          `json:"captcha_enabled"`
	Captcha                CaptchaConfig `json:"captcha"`
	LoginFailWindowSec     int           `json:"login_fail_window_sec"`
	LoginFailMax           int           `json:"login_fail_max"`
	LoginBanSec            int           `json:"login_ban_sec"`
	PasswordMinLen         int           `json:"password_min_len"`
	PasswordRequireUpper   bool          `json:"password_require_upper"`
	PasswordRequireLower   bool          `json:"password_require_lower"`
	PasswordRequireDigit   bool          `json:"password_require_digit"`
	PasswordRequireSpecial bool          `json:"password_require_special"`
	Admin2FAEnabled        bool          `json:"admin_2fa_enabled"`
}

type CaptchaConfig struct {
	Type           string `json:"type"`        // digit | string | math
	Length         int    `json:"length"`      // 字符数量（digit/string 生效）
	NoiseLevel     string `json:"noise_level"` // low | medium | high
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	ExpirySeconds  int    `json:"expiry_seconds"`  // 有效期（秒），过期即失效
	CaseSensitive  bool   `json:"case_sensitive"`  // 校验是否区分大小写（string 生效）
	ObfuscateImage bool   `json:"obfuscate_image"` // 下发前对图片字节做混淆，前端还原后再渲染
}

func DefaultGeneral() General {
	return General{
		PageSize:       20,
		LocalFilesRoot: "",
		NtpEnabled:     true,
		NtpServer:      "ntp.aliyun.com",
	}
}

func NormalizeGeneral(g General) General {
	def := DefaultGeneral()
	if g.PageSize < 1 || g.PageSize > 200 {
		g.PageSize = def.PageSize
	}
	if strings.TrimSpace(g.NtpServer) == "" {
		g.NtpServer = def.NtpServer
		if !g.NtpEnabled {
			g.NtpEnabled = def.NtpEnabled
		}
	}
	g.NtpServer = strings.TrimSpace(g.NtpServer)
	return g
}

func DefaultSecurity() Security {
	return Security{
		CaptchaEnabled:         false,
		Captcha:                DefaultCaptcha(),
		LoginFailWindowSec:     60,
		LoginFailMax:           5,
		LoginBanSec:            600,
		PasswordMinLen:         8,
		PasswordRequireUpper:   false,
		PasswordRequireLower:   true,
		PasswordRequireDigit:   true,
		PasswordRequireSpecial: false,
		Admin2FAEnabled:        false,
	}
}

func DefaultCaptcha() CaptchaConfig {
	return CaptchaConfig{
		Type:           "digit",
		Length:         4,
		NoiseLevel:     "low",
		Width:          120,
		Height:         44,
		ExpirySeconds:  180,
		CaseSensitive:  false,
		ObfuscateImage: true,
	}
}

func NormalizeCaptcha(c CaptchaConfig) CaptchaConfig {
	d := DefaultCaptcha()
	switch c.Type {
	case "digit", "string", "math":
	default:
		c.Type = d.Type
	}
	switch c.NoiseLevel {
	case "low", "medium", "high":
	default:
		c.NoiseLevel = d.NoiseLevel
	}
	c.Length = clampInt(c.Length, 2, 8, d.Length)
	c.Width = clampInt(c.Width, 80, 400, d.Width)
	c.Height = clampInt(c.Height, 30, 200, d.Height)
	c.ExpirySeconds = clampInt(c.ExpirySeconds, 30, 1800, d.ExpirySeconds)
	if c.Type != "string" {
		c.CaseSensitive = false
	}
	return c
}

func MergeSecurityDefaults(cfg Security) Security {
	cfg.Captcha = NormalizeCaptcha(cfg.Captcha)
	return cfg
}

func clampInt(v, min, max, fallback int) int {
	if v <= 0 {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func DefaultOAuth() OAuth {
	return OAuth{Enabled: false, Providers: []OAuthProvider{
		{ID: "github", Name: "GitHub", Type: "github", Scopes: "read:user user:email", Enabled: false},
	}}
}

func MergeOAuthDefaults(cfg OAuth) OAuth {
	preset := OAuthProvider{ID: "github", Name: "GitHub", Type: "github", Scopes: "read:user user:email", Enabled: false}
	for _, p := range cfg.Providers {
		if p.ID == preset.ID {
			return cfg
		}
	}
	cfg.Providers = append(cfg.Providers, preset)
	return cfg
}

func DefaultEmail() Email {
	return Email{
		Enabled:  false,
		SMTPPort: 587,
		UseTLS:   true,
		Templates: EmailTemplates{
			ResetPassword:      "您好 {{username}}，请点击链接重置密码：{{link}}",
			AdminResetPassword: "您好 {{username}}，管理员已为您重置 {{site_name}} 登录密码。\n\n新密码：{{password}}\n\n请尽快登录并修改密码。",
			Test:               "这是一封来自 {{site_name}} 的测试邮件。",
		},
	}
}

func DefaultLDAP() LDAP {
	return LDAP{
		Port:           389,
		UserFilter:     "(uid=%s)",
		UsernameAttr:   "uid",
		EmailAttr:      "mail",
		DeptFilter:     "(objectClass=organizationalUnit)",
		DeptNameAttr:   "ou",
		UserSyncFilter: "(|(objectClass=inetOrgPerson)(objectClass=person)(objectClass=user))",
	}
}

func MergeLDAPDefaults(cfg LDAP) LDAP {
	def := DefaultLDAP()
	if cfg.Port == 0 {
		cfg.Port = def.Port
	}
	if strings.TrimSpace(cfg.UserFilter) == "" {
		cfg.UserFilter = def.UserFilter
	}
	if strings.TrimSpace(cfg.UsernameAttr) == "" {
		cfg.UsernameAttr = def.UsernameAttr
	}
	if strings.TrimSpace(cfg.EmailAttr) == "" {
		cfg.EmailAttr = def.EmailAttr
	}
	if strings.TrimSpace(cfg.DeptFilter) == "" {
		cfg.DeptFilter = def.DeptFilter
	}
	if strings.TrimSpace(cfg.DeptNameAttr) == "" {
		cfg.DeptNameAttr = def.DeptNameAttr
	}
	if strings.TrimSpace(cfg.UserSyncFilter) == "" {
		cfg.UserSyncFilter = def.UserSyncFilter
	}
	return cfg
}

func DefaultGateway() Gateway {
	return Gateway{PersonalTokensEnabled: true}
}

// --- License ---

type LicenseSettings struct {
	LicenseServerURL string `json:"license_server_url"`
	HeartbeatHours   int    `json:"heartbeat_hours"`
	AutoSyncCRL      bool   `json:"auto_sync_crl"`
	CRLSyncHours     int    `json:"crl_sync_hours"`
}

func DefaultLicense() LicenseSettings {
	return LicenseSettings{
		LicenseServerURL: "https://license.nextfile.cn",
		HeartbeatHours:   24,
		AutoSyncCRL:      true,
		CRLSyncHours:     12,
	}
}

// NormalizeLicense 补齐空的授权服务器地址等默认值。
func NormalizeLicense(cfg LicenseSettings) LicenseSettings {
	d := DefaultLicense()
	if strings.TrimSpace(cfg.LicenseServerURL) == "" {
		cfg.LicenseServerURL = d.LicenseServerURL
	}
	cfg.LicenseServerURL = strings.TrimRight(strings.TrimSpace(cfg.LicenseServerURL), "/")
	if cfg.HeartbeatHours <= 0 {
		cfg.HeartbeatHours = d.HeartbeatHours
	}
	if cfg.CRLSyncHours <= 0 {
		cfg.CRLSyncHours = d.CRLSyncHours
	}
	return cfg
}

// --- Watermark ---

type WatermarkSettings struct {
	Enabled  bool     `json:"enabled"`
	Text     string   `json:"text"`
	Opacity  float64  `json:"opacity"`
	FontSize int      `json:"font_size"`
	Angle    int      `json:"angle"`
	Density  float64  `json:"density"`
	Scope    []string `json:"scope"`
}

func DefaultWatermark() WatermarkSettings {
	return WatermarkSettings{
		Enabled:  false,
		Text:     "{username} · {datetime}",
		Opacity:  0.15,
		FontSize: 14,
		Angle:    -25,
		Density:  0.7,
		Scope:    []string{"app", "preview", "share", "download"},
	}
}

func NormalizeWatermark(w WatermarkSettings) WatermarkSettings {
	def := DefaultWatermark()
	if strings.TrimSpace(w.Text) == "" {
		w.Text = def.Text
	}
	if w.Opacity <= 0 {
		w.Opacity = def.Opacity
	}
	if w.FontSize <= 0 {
		w.FontSize = def.FontSize
	}
	if len(w.Scope) == 0 {
		w.Scope = append([]string{}, def.Scope...)
	}
	if w.Density <= 0 {
		w.Density = def.Density
	}
	if w.Density < 0.1 {
		w.Density = 0.1
	}
	if w.Density > 2 {
		w.Density = 2
	}
	return w
}

// --- Cloud storage apps (OneDrive/Google Drive/Baidu Pan OAuth) ---

type CloudStorageApp struct {
	Platform     string `json:"platform"` // microsoft, google, baidu
	Name         string `json:"name"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthURL      string `json:"auth_url,omitempty"`
	TokenURL     string `json:"token_url,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
	Enabled      bool   `json:"enabled"`
}

type CloudStorage struct {
	Enabled bool               `json:"enabled"`
	Apps    []CloudStorageApp  `json:"apps"`
}

func DefaultCloudStorage() CloudStorage {
	return CloudStorage{
		Enabled: false,
		Apps: []CloudStorageApp{
			{
				Platform: "microsoft",
				Name:     "Microsoft OneDrive",
				AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
				TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
				Scopes:   "offline_access Files.ReadWrite.All User.Read",
				Enabled:  false,
			},
			{
				Platform: "google",
				Name:     "Google Drive",
				AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
				Scopes:   "https://www.googleapis.com/auth/drive offline_access",
				Enabled:  false,
			},
			{
				Platform: "baidu",
				Name:     "百度网盘",
				AuthURL:  "https://openapi.baidu.com/oauth/2.0/authorize",
				TokenURL: "https://openapi.baidu.com/oauth/2.0/token",
				Scopes:   "basic,netdisk",
				Enabled:  false,
			},
		},
	}
}

func MergeCloudStorageDefaults(cfg CloudStorage) CloudStorage {
	defaults := DefaultCloudStorage()
	existing := map[string]bool{}
	for _, a := range cfg.Apps {
		existing[a.Platform] = true
	}
	for _, a := range defaults.Apps {
		if !existing[a.Platform] {
			cfg.Apps = append(cfg.Apps, a)
		}
	}
	return cfg
}
