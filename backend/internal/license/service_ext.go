package license

import (
	"errors"
	"time"

	"baseplatform/internal/settings"
)

func (s *Service) applyPayload(payload *LicensePayload) {
	if payload == nil {
		return
	}
	NormalizePayload(payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload = payload
	s.cache = Meta{
		Edition:      payload.Edition,
		Valid:        true,
		CustomerName: payload.CustomerName,
		LicenseID:    payload.LicenseID,
		ExpiresAt:    payload.ExpiresAt.Format(time.RFC3339),
		Features:     append([]string{}, payload.Features...),
		MaxUsers:     payload.MaxUsers,
	}
}

func metaFromPayload(payload *LicensePayload) Meta {
	return Meta{
		Edition:      payload.Edition,
		Valid:        true,
		CustomerName: payload.CustomerName,
		LicenseID:    payload.LicenseID,
		ExpiresAt:    payload.ExpiresAt.Format(time.RFC3339),
		Features:     append([]string{}, payload.Features...),
		MaxUsers:     payload.MaxUsers,
	}
}

func invalidMeta(reason string) Meta {
	meta := CommunityMeta()
	meta.Valid = false
	meta.Reason = reason
	return meta
}

func (s *Service) Refresh() {
	prev := s.Store.LoadMeta()
	cache := CommunityMeta()
	var payload *LicensePayload

	if _, p, err := s.Store.LoadActive(); err == nil {
		NormalizePayload(p)
		switch {
		case time.Now().After(p.ExpiresAt):
			cache = invalidMeta("expired")
			cache.LicenseID = p.LicenseID
		case s.checkCRL(p.LicenseID) != nil:
			cache = invalidMeta("revoked")
			cache.LicenseID = p.LicenseID
		default:
			payload = p
			cache = metaFromPayload(p)
		}
	}

	s.mu.Lock()
	s.cache = cache
	s.payload = payload
	if prev.LastSyncAt != "" {
		s.cache.LastSyncAt = prev.LastSyncAt
	}
	s.mu.Unlock()

	_ = s.Store.SaveMeta(s.Meta())
	_, _ = ReconcileUserSeats(s.DB, s.Meta())
}

func (s *Service) SiteInfo() SiteInfo {
	meta := s.Meta()
	count, _ := CountEnabledUsers(s.DB)
	return SiteInfo{
		Version:   s.Version,
		Edition:   meta.Edition,
		MaxUsers:  EffectiveMaxUsers(meta),
		UserCount: int(count),
		License:   meta,
		Watermark: s.watermarkInfo(),
	}
}

func (s *Service) watermarkInfo() map[string]interface{} {
	if !s.HasFeature(FeatureWatermark) {
		return nil
	}
	wm := settings.NormalizeWatermark(settings.LoadWatermark(s.DB))
	return map[string]interface{}{
		"enabled":   wm.Enabled,
		"text":      wm.Text,
		"opacity":   wm.Opacity,
		"font_size": wm.FontSize,
		"angle":     wm.Angle,
		"density":   wm.Density,
		"scope":     wm.Scope,
	}
}

func (s *Service) Import(raw []byte, clientKey string) error {
	if s.importRL.Exceeded(clientKey) {
		return errors.New("导入失败次数过多，请 1 小时后再试")
	}
	lic, err := decodeLicenseFile(raw)
	if err != nil {
		s.importRL.RecordFailure(clientKey)
		return errors.New("授权文件格式无效")
	}
	payload, err := tryDecryptLicense(lic)
	if err != nil {
		s.importRL.RecordFailure(clientKey)
		return err
	}
	if err := ValidateMachineBinding(payload); err != nil {
		s.importRL.RecordFailure(clientKey)
		return err
	}
	NormalizePayload(payload)
	if time.Now().After(payload.ExpiresAt) {
		s.importRL.RecordFailure(clientKey)
		return errors.New("授权已过期")
	}
	if err := s.checkCRL(payload.LicenseID); err != nil {
		s.importRL.RecordFailure(clientKey)
		return err
	}
	if err := s.Store.SaveActive(lic, payload); err != nil {
		s.importRL.RecordFailure(clientKey)
		return err
	}
	s.importRL.Reset(clientKey)
	s.Refresh()
	return nil
}

// Remove clears the locally installed license and reverts to community edition.
func (s *Service) Remove() error {
	if err := s.Store.Clear(); err != nil {
		return err
	}
	s.Refresh()
	return nil
}

func (s *Service) CheckSeatAvailable() error {
	return CheckSeatAvailable(s.DB, s.Meta())
}

func (s *Service) IsCommunity() bool {
	return IsCommunityEdition(s.Meta())
}

func (s *Service) Edition() string {
	m := s.Meta()
	if !m.Valid || m.Edition == "" {
		return EditionCommunity
	}
	return m.Edition
}
