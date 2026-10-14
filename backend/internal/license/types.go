package license

import "time"

const (
	EditionCommunity  = "community"
	EditionTeam       = "team"
	EditionEnterprise = "enterprise"
	// EditionFlagship is a legacy alias for enterprise (kept for older .lic / UI copy).
	EditionFlagship = "flagship"
)

const (
	DefaultCommunityMaxUsers = 5
	DefaultTeamMaxUsers      = 15
	// 授权文件声明的用户上限下限；运行时仅自动创建一个 admin（superadmin 角色）。
	MinLicensedMaxUsers = 1
)

const (
	FeatureBrandingCustom = "branding_custom"
	FeatureWatermark      = "watermark"
	FeatureGateway        = "gateway"
	FeatureStorageSync    = "storage_sync"
	FeatureAdminTOTP      = "admin_totp"
	FeatureSSO            = "sso"
	FeatureOps            = "ops"
	FeatureExtensions     = "extensions"
	FeatureOnlineUpgrade  = "online_upgrade"
	FeatureLoginCaptcha   = "login_captcha"
)

// TeamDefaultFeatures aligns with nextlicense team defaults that this product consumes.
var TeamDefaultFeatures = []string{
	FeatureLoginCaptcha,
	FeatureOnlineUpgrade,
	FeatureAdminTOTP,
}

// EnterpriseDefaultFeatures (and legacy flagship) unlock commercial capabilities.
var EnterpriseDefaultFeatures = []string{
	FeatureBrandingCustom,
	FeatureWatermark,
	FeatureGateway,
	FeatureStorageSync,
	FeatureAdminTOTP,
	FeatureSSO,
	FeatureOps,
	FeatureExtensions,
	FeatureOnlineUpgrade,
	FeatureLoginCaptcha,
}

// FlagshipDefaultFeatures keeps the historical name used by older code paths.
var FlagshipDefaultFeatures = EnterpriseDefaultFeatures

type LicensePayload struct {
	LicenseID    string    `json:"license_id"`
	CustomerName string    `json:"customer_name"`
	Product      string    `json:"product,omitempty"`
	Edition      string    `json:"edition"`
	Features     []string  `json:"features"`
	MaxUsers     int       `json:"max_users"`
	MaxInstances int       `json:"max_instances"`
	ExpiresAt    time.Time `json:"expires_at"`
	MachineID    string    `json:"machine_id,omitempty"`
	BindingMode  string    `json:"binding_mode,omitempty"`
	IssuedAt     time.Time `json:"issued_at"`
	Nonce        string    `json:"nonce,omitempty"`
}

type SignedLicense struct {
	LicenseID string `json:"license_id"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type Meta struct {
	Edition      string   `json:"edition"`
	Valid        bool     `json:"valid"`
	CustomerName string   `json:"customer_name"`
	LicenseID    string   `json:"license_id,omitempty"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	Features     []string `json:"features"`
	MaxUsers     int      `json:"max_users"`
	Reason       string   `json:"reason,omitempty"`
	LastSyncAt   string   `json:"last_sync_at,omitempty"`
}

type SiteInfo struct {
	Version   string                 `json:"version"`
	Edition   string                 `json:"edition"`
	MaxUsers  int                    `json:"max_users"`
	UserCount int                    `json:"user_count"`
	License   Meta                   `json:"license"`
	Watermark map[string]interface{} `json:"watermark,omitempty"`
}
