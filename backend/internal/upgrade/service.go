package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"baseplatform/internal/license"
	"baseplatform/internal/settings"
	"gorm.io/gorm"
)

type Service struct {
	DB       *gorm.DB
	License  *license.Service
	Version  string
	mu       sync.Mutex
	status   Status
}

type Status struct {
	Phase   string `json:"phase"`
	Percent int    `json:"percent"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

type Manifest struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Signature   string `json:"signature"`
	Changelog   string `json:"changelog"`
}

func NewService(db *gorm.DB, lic *license.Service, version string) *Service {
	return &Service{DB: db, License: lic, Version: version}
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) Check() (*Manifest, bool, error) {
	cfg := settings.LoadLicense(s.DB)
	base := strings.TrimRight(strings.TrimSpace(cfg.LicenseServerURL), "/")
	if base == "" {
		return nil, false, errors.New("license server not configured")
	}
	resp, err := http.Get(base + "/api/v1/releases/latest")
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Success int `json:"success"`
		Data    struct {
			Version     string `json:"version"`
			DownloadURL string `json:"download_url"`
			SHA256      string `json:"sha256"`
			Signature   string `json:"signature"`
			Changelog   string `json:"changelog"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &wrap) != nil || wrap.Success != 1 {
		return nil, false, errors.New("check update failed")
	}
	m := &Manifest{
		Version:     wrap.Data.Version,
		DownloadURL: wrap.Data.DownloadURL,
		SHA256:      wrap.Data.SHA256,
		Signature:   wrap.Data.Signature,
		Changelog:   wrap.Data.Changelog,
	}
	return m, m.Version != s.Version, nil
}

func (s *Service) Apply() error {
	if !s.License.HasFeature(license.FeatureOnlineUpgrade) {
		return errors.New("需要企业版在线升级授权")
	}
	s.setStatus("checking", 5, "检查更新...")
	m, has, err := s.Check()
	if err != nil {
		s.setStatus("error", 0, "", err.Error())
		return err
	}
	if !has {
		s.setStatus("done", 100, "已是最新版本")
		return nil
	}
	manifestRaw, _ := json.Marshal(map[string]string{
		"version": m.Version, "download_url": m.DownloadURL, "sha256": m.SHA256,
	})
	if err := licenseVerifyManifest(manifestRaw, m.Signature); err != nil {
		s.setStatus("error", 0, "", "签名校验失败")
		return err
	}
	staging := filepath.Join(os.TempDir(), "baseplatform-upgrade")
	_ = os.MkdirAll(staging, 0o755)
	target := filepath.Join(staging, "baseplatform.new")
	s.setStatus("downloading", 20, "下载升级包...")
	if err := downloadFile(m.DownloadURL, target); err != nil {
		s.setStatus("error", 0, "", err.Error())
		return err
	}
	s.setStatus("verifying", 60, "校验文件...")
	sum, err := fileSHA256(target)
	if err != nil || !strings.EqualFold(sum, m.SHA256) {
		s.setStatus("error", 0, "", "SHA256 校验失败")
		return errors.New("sha256 mismatch")
	}
	exe, err := os.Executable()
	if err != nil {
		s.setStatus("error", 0, "", err.Error())
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	backup := exe + ".bak"
	newPath := exe + ".new"
	_ = os.Remove(backup)
	s.setStatus("applying", 80, "替换二进制...")
	_ = copyFile(exe, backup)
	if err := copyFile(target, newPath); err != nil {
		s.setStatus("error", 0, "", err.Error())
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = copyFile(backup, exe)
		s.setStatus("error", 0, "", err.Error())
		return err
	}
	_ = os.Chmod(exe, 0o755)
	s.setStatus("restarting", 95, "重启服务...")
	go s.restart(exe)
	s.setStatus("done", 100, "升级完成，服务正在重启")
	return nil
}

func (s *Service) setStatus(phase string, pct int, msg string, errMsg ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = Status{Phase: phase, Percent: pct, Message: msg}
	if len(errMsg) > 0 {
		s.status.Error = errMsg[0]
	}
}

func (s *Service) restart(exe string) {
	time.Sleep(2 * time.Second)
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return
	}
	os.Exit(0)
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download http %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func licenseVerifyManifest(raw []byte, sig string) error {
	return license.VerifyManifest(raw, sig)
}
