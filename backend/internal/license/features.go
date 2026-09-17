package license

import "strings"

func HasFeature(meta *Meta, code string) bool {
	if meta == nil || !meta.Valid {
		return false
	}
	for _, f := range meta.Features {
		if f == code {
			return true
		}
	}
	return false
}

func CommunityMeta() Meta {
	return Meta{
		Edition:  EditionCommunity,
		Valid:    true,
		Features: []string{},
		MaxUsers: DefaultCommunityMaxUsers,
	}
}

func EffectiveMaxUsers(meta Meta) int {
	if !meta.Valid || meta.Edition == EditionCommunity {
		if meta.MaxUsers > 0 {
			return ClampMaxUsers(meta.MaxUsers)
		}
		return DefaultCommunityMaxUsers
	}
	if meta.MaxUsers == 0 {
		return 0 // 旗舰版不限
	}
	return ClampMaxUsers(meta.MaxUsers)
}

// ClampMaxUsers enforces the minimum seat limit for licensed editions.
func ClampMaxUsers(n int) int {
	if n == 0 {
		return 0
	}
	if n < MinLicensedMaxUsers {
		return MinLicensedMaxUsers
	}
	return n
}

func NormalizePayload(payload *LicensePayload) {
	if payload == nil {
		return
	}
	edition := strings.TrimSpace(payload.Edition)
	if edition == "" {
		edition = EditionFlagship
	}
	payload.Edition = edition

	if payload.MaxUsers == 0 && edition == EditionCommunity {
		payload.MaxUsers = DefaultCommunityMaxUsers
	}
	if payload.MaxUsers > 0 {
		payload.MaxUsers = ClampMaxUsers(payload.MaxUsers)
	}

	if len(payload.Features) == 0 {
		if edition == EditionFlagship {
			payload.Features = append([]string{}, FlagshipDefaultFeatures...)
		}
		return
	}

	if edition == EditionFlagship {
		payload.Features = ensureFeatures(payload.Features, FlagshipDefaultFeatures)
	}
}

func ensureFeatures(current, defaults []string) []string {
	set := map[string]bool{}
	out := append([]string{}, current...)
	for _, f := range out {
		set[f] = true
	}
	for _, f := range defaults {
		if !set[f] {
			out = append(out, f)
			set[f] = true
		}
	}
	return out
}

func EditionLabel(edition string) string {
	switch edition {
	case EditionFlagship:
		return "旗舰版"
	default:
		return "社区版"
	}
}

func IsCommunityEdition(meta Meta) bool {
	return !meta.Valid || meta.Edition == EditionCommunity || meta.Edition == ""
}
