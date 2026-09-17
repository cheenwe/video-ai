package storage

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"baseplatform/internal/config"
	"baseplatform/internal/models"

	"gorm.io/gorm"
)

var supportedDrivers = map[string]bool{
	"local":  true,
	"s3":     true,
	"oss":    true,
	"ftp":    true,
	"sftp":   true,
	"webdav": true,
	"nfs":    true,
	"smb":    true,
}

func IsSupportedDriver(driver string) bool {
	if driver == "" {
		return true
	}
	return supportedDrivers[driver]
}

type Manager struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func (m *Manager) GetStorage(id uint) (*models.Storage, error) {
	var s models.Storage
	if err := m.DB.First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (m *Manager) DriverFor(s *models.Storage) (Driver, error) {
	switch s.Driver {
	case "local", "":
		basePath, err := m.localBasePath(s)
		if err != nil {
			return nil, err
		}
		return NewLocalDriver(basePath, m.protectedRoots()...)
	case "s3", "oss":
		cfg := parseConfig[S3Config](s.Config)
		return NewS3Driver(cfg)
	case "ftp":
		cfg := parseConfig[FTPConfig](s.Config)
		return NewFTPDriver(cfg)
	case "sftp":
		cfg := parseConfig[FTPConfig](s.Config)
		if cfg.Port == 0 {
			cfg.Port = 22
		}
		return NewSFTPDriver(cfg)
	case "webdav":
		cfg := parseConfig[WebDAVConfig](s.Config)
		return NewWebDAVDriver(cfg)
	case "nfs":
		return nil, ErrUnsupported
	case "smb":
		return nil, ErrUnsupported
	default:
		return nil, ErrUnsupported
	}
}

func (m *Manager) protectedRoots() []string {
	if m == nil || m.Cfg == nil {
		return nil
	}
	return m.Cfg.ProtectedAbsRoots()
}

func (m *Manager) LocalBasePath(s *models.Storage) (string, error) {
	return m.localBasePath(s)
}

func (m *Manager) localBasePath(s *models.Storage) (string, error) {
	cfg := parseConfig[LocalConfig](s.Config)
	base := cfg.BasePath
	if base == "" {
		base = m.Cfg.StorageLocalRoot
	}
	if !filepath.IsAbs(base) {
		abs, err := filepath.Abs(base)
		if err != nil {
			return "", err
		}
		base = abs
	}
	return base, nil
}

func (m *Manager) EncodeLocalConfig(basePath string) (string, error) {
	if basePath == "" {
		basePath = m.Cfg.StorageLocalRoot
	}
	if !filepath.IsAbs(basePath) {
		abs, err := filepath.Abs(basePath)
		if err != nil {
			return "", err
		}
		basePath = abs
	}
	return encodeConfig(LocalConfig{BasePath: basePath})
}

func (m *Manager) EncodeDriverConfig(driver string, raw map[string]interface{}) (string, error) {
	switch driver {
	case "local", "":
		basePath := ""
		if raw != nil {
			if v, ok := raw["base_path"].(string); ok {
				basePath = v
			}
		}
		return m.EncodeLocalConfig(basePath)
	case "s3":
		cfg := S3Config{}
		if raw != nil {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &cfg)
		}
		return encodeConfig(cfg)
	case "ftp":
		cfg := FTPConfig{Port: 21}
		if raw != nil {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &cfg)
		}
		if cfg.Port == 0 {
			cfg.Port = 21
		}
		return encodeConfig(cfg)
	case "sftp":
		cfg := FTPConfig{Port: 22}
		if raw != nil {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &cfg)
		}
		if cfg.Port == 0 {
			cfg.Port = 22
		}
		return encodeConfig(cfg)
	case "webdav":
		cfg := WebDAVConfig{}
		if raw != nil {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &cfg)
		}
		return encodeConfig(cfg)
	case "nfs", "smb":
		return "", ErrUnsupported
	default:
		return "", ErrUnsupported
	}
}

func (m *Manager) ParseLocalConfig(raw string) LocalConfig {
	return parseConfig[LocalConfig](raw)
}

func (m *Manager) ParseDriverConfig(driver, raw string) map[string]interface{} {
	out := map[string]interface{}{}
	switch driver {
	case "local", "":
		cfg := parseConfig[LocalConfig](raw)
		out["base_path"] = cfg.BasePath
	case "s3":
		cfg := parseConfig[S3Config](raw)
		out["endpoint"] = cfg.Endpoint
		out["bucket"] = cfg.Bucket
		out["access_key"] = cfg.AccessKey
		out["secret_key"] = cfg.SecretKey
		out["region"] = cfg.Region
		out["use_ssl"] = cfg.UseSSL
		out["prefix"] = cfg.Prefix
	case "ftp", "sftp":
		cfg := parseConfig[FTPConfig](raw)
		out["host"] = cfg.Host
		out["port"] = cfg.Port
		out["username"] = cfg.Username
		out["password"] = cfg.Password
		out["base_path"] = cfg.BasePath
	case "webdav":
		cfg := parseConfig[WebDAVConfig](raw)
		out["url"] = cfg.URL
		out["username"] = cfg.Username
		out["password"] = cfg.Password
	case "nfs":
		cfg := parseConfig[NFSConfig](raw)
		out["server"] = cfg.Server
		out["export"] = cfg.Export
		out["mount_point"] = cfg.MountPoint
		out["sub_path"] = cfg.SubPath
		if cfg.Version == 0 {
			cfg.Version = 4
		}
		out["version"] = cfg.Version
		out["options"] = cfg.Options
	case "smb":
		cfg := parseConfig[SMBConfig](raw)
		out["host"] = cfg.Host
		if cfg.Port == 0 {
			cfg.Port = 445
		}
		out["port"] = cfg.Port
		out["username"] = cfg.Username
		out["password"] = cfg.Password
		out["domain"] = cfg.Domain
		out["share"] = cfg.Share
		out["base_path"] = cfg.BasePath
	}
	return out
}

func FileItemOut(item FileItem) map[string]interface{} {
	out := map[string]interface{}{
		"name":     item.Name,
		"path":     item.Path,
		"size":     item.Size,
		"is_dir":   item.IsDir,
		"modified": FormatModified(item.Modified),
	}
	if item.MimeType != "" {
		out["mime_type"] = item.MimeType
	}
	return out
}


