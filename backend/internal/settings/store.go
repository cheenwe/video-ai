package settings

import (
	"encoding/json"
	"errors"

	"baseplatform/internal/models"

	"gorm.io/gorm"
)

const category = "settings"

func LoadSection[T any](db *gorm.DB, code string, defaults T) T {
	var row models.SysConfig
	err := db.Where("category = ? AND code = ? AND status = ?", category, code, "active").First(&row).Error
	if err != nil {
		return defaults
	}
	var out T
	if json.Unmarshal([]byte(row.Value), &out) != nil {
		return defaults
	}
	return out
}

func SaveSection(db *gorm.DB, code string, data any, updatedBy *uint) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var row models.SysConfig
	err = db.Where("category = ? AND code = ?", category, code).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = models.SysConfig{
			Category:  category,
			Code:      code,
			Value:     string(raw),
			Status:    "active",
			UpdatedBy: updatedBy,
		}
		return db.Create(&row).Error
	}
	if err != nil {
		return err
	}
	row.Value = string(raw)
	row.UpdatedBy = updatedBy
	row.Status = "active"
	return db.Save(&row).Error
}

func LoadGeneral(db *gorm.DB) General { return LoadSection(db, "general", DefaultGeneral()) }

func LoadSecurity(db *gorm.DB) Security {
	return MergeSecurityDefaults(LoadSection(db, "security", DefaultSecurity()))
}

func LoadEmail(db *gorm.DB) Email { return LoadSection(db, "email", DefaultEmail()) }

func LoadLDAP(db *gorm.DB) LDAP {
	return MergeLDAPDefaults(LoadSection(db, "ldap", DefaultLDAP()))
}

func LoadOAuth(db *gorm.DB) OAuth {
	return MergeOAuthDefaults(LoadSection(db, "oauth", DefaultOAuth()))
}

func LoadGateway(db *gorm.DB) Gateway {
	return LoadSection(db, "gateway", DefaultGateway())
}

func LoadLicense(db *gorm.DB) LicenseSettings {
	return NormalizeLicense(LoadSection(db, "license", DefaultLicense()))
}

func LoadWatermark(db *gorm.DB) WatermarkSettings {
	return NormalizeWatermark(LoadSection(db, "watermark", DefaultWatermark()))
}

func LoadCloudStorage(db *gorm.DB) CloudStorage {
	cfg := LoadSection(db, "cloud_storage", DefaultCloudStorage())
	return MergeCloudStorageDefaults(cfg)
}
