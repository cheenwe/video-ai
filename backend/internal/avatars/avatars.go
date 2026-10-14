package avatars

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"baseplatform/internal/storage"
)

const maxAvatarBytes = 2 << 20 // 2MB

// 用户上传仅允许位图；内置预设仍可使用 SVG。
var allowedUploadExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

type Service struct {
	Root string
}

func (s *Service) EnsureDirs() error {
	dirs := []string{
		s.Root,
		filepath.Join(s.Root, "users"),
		filepath.Join(s.Root, "defaults"),
		filepath.Join(s.Root, "presets"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return s.seedDefaults()
}

func (s *Service) seedDefaults() error {
	files := map[string]string{
		"defaults/user.svg":   defaultUserSVG,
		"presets/user-01.svg": presetUserSVG("#4f86f7"),
		"presets/user-02.svg": presetUserSVG("#2fb344"),
		"presets/user-03.svg": presetUserSVG("#f59f00"),
		"presets/user-04.svg": presetUserSVG("#d63939"),
		"presets/user-05.svg": presetUserSVG("#ae3ec9"),
		"presets/user-06.svg": presetUserSVG("#0ca678"),
	}
	for rel, content := range files {
		p := filepath.Join(s.Root, rel)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) URL(kind string, id uint) string {
	return fmt.Sprintf("/avatars/%s/%d", kind, id)
}

func (s *Service) DefaultURL(kind string) string {
	return "/avatars/defaults/user.svg"
}

func (s *Service) PresetURL(preset string) string {
	return "/avatars/presets/" + preset
}

func (s *Service) IsPresetURL(url string) bool {
	return strings.HasPrefix(url, "/avatars/presets/")
}

func (s *Service) SaveUpload(kind string, id uint, file multipart.File, header *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedUploadExt[ext] {
		return "", fmt.Errorf("仅支持 jpg、png、webp、gif")
	}
	if header.Size > maxAvatarBytes {
		return "", fmt.Errorf("头像不能超过 2MB")
	}
	s.RemoveCustom(kind, id)
	dir := filepath.Join(s.Root, kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, fmt.Sprintf("%d%s", id, ext))
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		os.Remove(dest)
		return "", err
	}
	if written > maxAvatarBytes {
		os.Remove(dest)
		return "", fmt.Errorf("头像不能超过 2MB")
	}
	return s.fileURL(kind, id, ext), nil
}

func (s *Service) SetPreset(preset string) (string, error) {
	preset = strings.TrimSpace(preset)
	if preset == "" {
		return "", fmt.Errorf("请选择头像")
	}
	if strings.Contains(preset, "/") || strings.Contains(preset, "..") {
		return "", fmt.Errorf("无效的头像选项")
	}
	p := filepath.Join(s.Root, "presets", preset)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("头像选项不存在")
	}
	return s.PresetURL(preset), nil
}

func (s *Service) RemoveCustom(kind string, id uint) {
	dir := filepath.Join(s.Root, kind)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".svg"} {
		_ = os.Remove(filepath.Join(dir, fmt.Sprintf("%d%s", id, ext)))
	}
}

func (s *Service) fileURL(kind string, id uint, ext string) string {
	return fmt.Sprintf("/avatars/%s/%d%s", kind, id, ext)
}

func (s *Service) ServeAvatar(c_path string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(c_path))
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", os.ErrNotExist
	}
	full := filepath.Join(s.Root, clean)
	if !storage.IsUnderAnyRoot(full, []string{s.Root}) {
		return "", os.ErrNotExist
	}
	if _, err := os.Stat(full); err != nil {
		return "", err
	}
	return full, nil
}

const defaultUserSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="32" fill="#e8eef8"/><circle cx="32" cy="24" r="12" fill="#4f86f7"/><path d="M12 54c4-14 16-20 20-20s16 6 20 20" fill="#4f86f7"/></svg>`

func presetUserSVG(color string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="32" fill="%s"/><circle cx="32" cy="26" r="11" fill="#fff" opacity=".9"/><path d="M14 52c3-12 14-17 18-17s15 5 18 17" fill="#fff" opacity=".9"/></svg>`, color)
}
