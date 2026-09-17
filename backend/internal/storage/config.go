package storage

import "encoding/json"

type LocalConfig struct {
	BasePath string `json:"base_path"`
}

type S3Config struct {
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"`
	UseSSL    bool   `json:"use_ssl"`
	Prefix    string `json:"prefix"`
}

type FTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	BasePath string `json:"base_path"`
}

type WebDAVConfig struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type NFSConfig struct {
	Server     string `json:"server"`
	Export     string `json:"export"`
	MountPoint string `json:"mount_point"`
	SubPath    string `json:"sub_path"`
	Version    int    `json:"version"`
	Options    string `json:"options"`
}

type SMBConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain"`
	Share    string `json:"share"`
	BasePath string `json:"base_path"`
}

type CloudStorageConfig struct {
	Platform     string `json:"platform"`
	RemotePath   string `json:"remote_path"`
	Connected    bool   `json:"connected"`
	AccountName  string `json:"account_name"`
	TokenExpiry  string `json:"token_expiry"`
	OAuthScope   string `json:"oauth_scope,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func ParseCloudStorageConfig(raw string) map[string]interface{} {
	cfg := parseConfig[CloudStorageConfig](raw)
	out := map[string]interface{}{
		"platform":     cfg.Platform,
		"remote_path":  cfg.RemotePath,
		"account_name": cfg.AccountName,
		"token_expiry": cfg.TokenExpiry,
		"oauth_scope":  cfg.OAuthScope,
		"connected":    cfg.Connected || cfg.AccessToken != "",
	}
	return out
}

func parseConfig[T any](raw string) T {
	var cfg T
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

func encodeConfig(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
