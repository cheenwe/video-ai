package license

// Branding field groups per edition plan.
var (
	BrandingBasicKeys = []string{
		"site_name", "tagline", "primary_color", "logo_text", "login_description",
		"footer_powered_by_name", "footer_icp",
	}
	BrandingCustomKeys = []string{
		"logo_url", "favicon_url", "share_ad_image_url", "share_ad_url",
		"footer_powered_by_url", "footer_wechat",
	}
)

func brandingKeySet(keys []string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

var brandingBasicSet = brandingKeySet(BrandingBasicKeys)
var brandingCustomSet = brandingKeySet(BrandingCustomKeys)

// CanUpdateBrandingField returns whether the current license may write a branding field.
func CanUpdateBrandingField(meta Meta, code string) bool {
	if IsCommunityEdition(meta) {
		return false
	}
	if brandingCustomSet[code] {
		return HasFeature(&meta, FeatureBrandingCustom)
	}
	if brandingBasicSet[code] {
		return true // 旗舰版基础品牌字段可编辑
	}
	return false
}
