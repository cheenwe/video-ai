package models

import "time"

// NVRAgent 是一台运行 nvr-agent 的电脑（可与平台同机，也可在其他电脑上）。
type NVRAgent struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"size:128;not null" json:"name"`
	BaseURL     string     `gorm:"size:255;not null" json:"base_url"`
	TokenCipher string     `gorm:"size:512" json:"-"` // AES-GCM 加密后的 agent api_token
	Enabled     int        `gorm:"not null;default:1" json:"enabled"`
	Remark      string     `gorm:"size:512" json:"remark"`
	LastStatus  string     `gorm:"size:32" json:"last_status"` // online | offline | unauthorized
	LastError   string     `gorm:"size:512" json:"last_error"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	Version     string     `gorm:"size:64" json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (NVRAgent) TableName() string { return "nvr_agents" }
