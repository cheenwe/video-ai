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
		return 0 // 企业版/旗舰版不限
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

// CanonicalEdition maps legacy aliases onto nextlicense editions.
func CanonicalEdition(edition string) string {
	switch strings.ToLower(strings.TrimSpace(edition)) {
	case EditionTeam:
		return EditionTeam
	case EditionEnterprise, EditionFlagship:
		return EditionEnterprise
	case EditionCommunity, "":
		return EditionCommunity
	default:
		return strings.ToLower(strings.TrimSpace(edition))
	}
}

func IsCommercialEdition(edition string) bool {
	switch CanonicalEdition(edition) {
	case EditionTeam, EditionEnterprise:
		return true
	default:
		return false
	}
}

func NormalizePayload(payload *LicensePayload) {
	if payload == nil {
		return
	}
	edition := strings.TrimSpace(payload.Edition)
	if edition == "" {
		edition = EditionEnterprise
	}
	// Preserve flagship label if that is what was issued historically; otherwise canonicalize.
	if strings.EqualFold(edition, EditionFlagship) {
		payload.Edition = EditionFlagship
	} else {
		payload.Edition = CanonicalEdition(edition)
		if payload.Edition != EditionCommunity && payload.Edition != EditionTeam && payload.Edition != EditionEnterprise {
			payload.Edition = edition
		}
	}
	edition = CanonicalEdition(payload.Edition)

	if payload.MaxUsers == 0 && edition == EditionTeam {
		payload.MaxUsers = DefaultTeamMaxUsers
	}
	if payload.MaxUsers == 0 && edition == EditionCommunity {
		payload.MaxUsers = DefaultCommunityMaxUsers
	}
	if payload.MaxUsers > 0 {
		payload.MaxUsers = ClampMaxUsers(payload.MaxUsers)
	}

	if len(payload.Features) == 0 {
		switch edition {
		case EditionTeam:
			payload.Features = append([]string{}, TeamDefaultFeatures...)
		case EditionEnterprise:
			payload.Features = append([]string{}, EnterpriseDefaultFeatures...)
		}
		return
	}

	if edition == EditionEnterprise {
		payload.Features = ensureFeatures(payload.Features, EnterpriseDefaultFeatures)
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
	switch CanonicalEdition(edition) {
	case EditionTeam:
		return "团队版"
	case EditionEnterprise:
		return "旗舰版"
	default:
		return "社区版"
	}
}

func IsCommunityEdition(meta Meta) bool {
	return !meta.Valid || CanonicalEdition(meta.Edition) == EditionCommunity || meta.Edition == ""
}
