package license

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"baseplatform/internal/settings"
	"gorm.io/gorm"
)

type Service struct {
	DB       *gorm.DB
	Store    *Store
	Version  string
	mu       sync.RWMutex
	cache    Meta
	payload  *LicensePayload
	importRL *RateLimiter
}

func NewService(db *gorm.DB, version string) *Service {
	s := &Service{
		DB:       db,
		Store:    NewStore(db),
		Version:  version,
		cache:    CommunityMeta(),
		importRL: NewRateLimiter(15, time.Hour),
	}
	s.Refresh()
	s.ensureLicenseServerConfigured()
	go s.backgroundLoop()
	go s.bootstrapInstallSync()
	return s
}

func (s *Service) Meta() Meta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

func (s *Service) Fingerprint() MachineInfo {
	return CollectMachineInfo()
}

func (s *Service) Activate(serverURL, licenseID string) error {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return errors.New("license server url required")
	}
	body, _ := json.Marshal(map[string]string{
		"license_id": licenseID,
		"machine_id": MachineID(),
		"hostname":   hostname(),
	})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/activate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Success int `json:"success"`
		Data    struct {
			LicenseFile json.RawMessage `json:"license_file"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if json.Unmarshal(raw, &wrap) != nil || wrap.Success != 1 {
		if strings.TrimSpace(wrap.Msg) != "" {
			return errors.New(wrap.Msg)
		}
		return errors.New("activation failed")
	}
	licRaw, err := parseActivationLicenseFile(wrap.Data.LicenseFile)
	if err != nil {
		return err
	}
	return s.Import(licRaw, "activate:"+licenseID)
}

func parseActivationLicenseFile(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty license file")
	}
	if raw[0] == '"' {
		var lic []byte
		if err := json.Unmarshal(raw, &lic); err != nil {
			return nil, errors.New("invalid license file")
		}
		return lic, nil
	}
	if raw[0] == '{' {
		return raw, nil
	}
	dec, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, errors.New("invalid license file")
	}
	return dec, nil
}

func (s *Service) SyncCRL() error {
	cfg := settings.LoadLicense(s.DB)
	url := strings.TrimRight(strings.TrimSpace(cfg.LicenseServerURL), "/")
	if url == "" {
		return errors.New("license server not configured")
	}
	return s.syncCRLFromURL(url)
}

// SyncLicense pulls CRL, refreshes the license file from the license server when possible,
// runs heartbeat, and updates last_sync_at.
func (s *Service) SyncLicense() (string, error) {
	cfg := settings.LoadLicense(s.DB)
	url := strings.TrimRight(strings.TrimSpace(cfg.LicenseServerURL), "/")
	if url == "" {
		return "", errors.New("请先配置授权服务器地址")
	}
	if err := s.syncCRLFromURL(url); err != nil {
		return "", err
	}

	meta := s.Meta()
	licenseID := strings.TrimSpace(meta.LicenseID)
	pulled := false
	var pullNote string

	if licenseID != "" {
		if err := s.Activate(url, licenseID); err != nil {
			pullNote = "吊销列表已同步，但未能从服务器拉取最新授权：" + err.Error()
		} else {
			pulled = true
		}
	} else if trial, err := s.TrialStatus(url); err == nil && trial != nil && trial.Status == "approved" && strings.TrimSpace(trial.LicenseID) != "" {
		if err := s.Activate(url, trial.LicenseID); err != nil {
			pullNote = "试用授权同步失败：" + err.Error()
		} else {
			pulled = true
		}
	}

	_ = s.Heartbeat()
	_ = s.RegisterInstall()

	now := time.Now().UTC().Format(time.RFC3339)
	s.Refresh()
	s.mu.Lock()
	s.cache.LastSyncAt = now
	metaToSave := s.cache
	s.mu.Unlock()
	_ = s.Store.SaveMeta(metaToSave)

	switch {
	case pulled:
		return "授权信息已更新", nil
	case pullNote != "":
		return pullNote, nil
	default:
		return "授权信息已同步", nil
	}
}

func (s *Service) syncCRLFromURL(url string) error {
	resp, err := http.Get(url + "/api/v1/crl")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Success int `json:"success"`
		Data    struct {
			Payload   string `json:"payload"`
			Signature string `json:"signature"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if json.Unmarshal(raw, &wrap) != nil || wrap.Success != 1 {
		if strings.TrimSpace(wrap.Msg) != "" {
			return errors.New(wrap.Msg)
		}
		return errors.New("crl sync failed")
	}
	if err := verifyCRL(wrap.Data.Payload, wrap.Data.Signature); err != nil {
		return err
	}
	_ = s.Store.SaveCRL(wrap.Data.Payload, wrap.Data.Signature)
	var crl struct {
		Revoked []string `json:"revoked"`
	}
	_ = json.Unmarshal([]byte(wrap.Data.Payload), &crl)
	meta := s.Meta()
	for _, id := range crl.Revoked {
		if id == meta.LicenseID {
			_ = s.Store.Invalidate("revoked")
			s.Refresh()
			return nil
		}
	}
	s.Refresh()
	return nil
}

func (s *Service) Heartbeat() error {
	meta := s.Meta()
	if !meta.Valid || meta.LicenseID == "" {
		return nil
	}
	cfg := settings.LoadLicense(s.DB)
	url := strings.TrimRight(strings.TrimSpace(cfg.LicenseServerURL), "/")
	if url == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{
		"license_id": meta.LicenseID,
		"machine_id": MachineID(),
	})
	req, _ := http.NewRequest(http.MethodPost, url+"/api/v1/heartbeat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Success int `json:"success"`
		Data    struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &wrap)
	if wrap.Data.Status == "revoked" || wrap.Data.Status == "expired" {
		_ = s.Store.Invalidate(wrap.Data.Status)
		s.Refresh()
	}
	return nil
}

func (s *Service) HasFeature(code string) bool {
	m := s.Meta()
	return HasFeature(&m, code)
}

func (s *Service) checkCRL(licenseID string) error {
	payload, sig, err := s.Store.LoadCRL()
	if err != nil {
		return nil
	}
	if err := verifyCRL(payload, sig); err != nil {
		return nil
	}
	var crl struct {
		Revoked []string `json:"revoked"`
	}
	_ = json.Unmarshal([]byte(payload), &crl)
	for _, id := range crl.Revoked {
		if id == licenseID {
			return errors.New("授权已被吊销")
		}
	}
	return nil
}

func (s *Service) backgroundLoop() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.Refresh()
		_ = s.SyncCRL()
		_ = s.Heartbeat()
		_ = s.RegisterInstall()
	}
}

// ensureLicenseServerConfigured persists the default license URL when unset (fresh install / empty config).
func (s *Service) ensureLicenseServerConfigured() {
	raw := settings.LoadSection(s.DB, "license", settings.DefaultLicense())
	if strings.TrimSpace(raw.LicenseServerURL) != "" {
		return
	}
	cfg := settings.NormalizeLicense(raw)
	_ = settings.SaveSection(s.DB, "license", cfg, nil)
}

// bootstrapInstallSync runs once after startup: register deployment basics + CRL sync.
func (s *Service) bootstrapInstallSync() {
	// Brief delay so DB/settings seed and network are ready.
	time.Sleep(2 * time.Second)
	_ = s.RegisterInstall()
	_, _ = s.SyncLicense()
}

// RegisterInstall posts machine fingerprint to license server deployments/register (no license required).
func (s *Service) RegisterInstall() error {
	cfg := settings.LoadLicense(s.DB)
	url := strings.TrimRight(strings.TrimSpace(cfg.LicenseServerURL), "/")
	if url == "" {
		return nil
	}
	info := CollectMachineInfo()
	body, _ := json.Marshal(map[string]string{
		"machine_id": info.MachineID,
		"hostname":   info.Hostname,
		"version":    s.Version,
		"product":    "baseplatform",
	})
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url+"/api/v1/deployments/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return errors.New("deployment register failed")
	}
	return nil
}
