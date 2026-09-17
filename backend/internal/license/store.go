package license

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"

	"baseplatform/internal/models"
	"gorm.io/gorm"
)

const (
	categoryLicense = "license"
	codeActive      = "active"
	codeMeta        = "meta"
	codeCRL         = "crl_cache"
)

var storeKey = deriveStoreKey()

func deriveStoreKey() []byte {
	secret := os.Getenv("LICENSE_STORE_SECRET")
	if secret == "" {
		secret = "baseplatform-local-dev-store-key"
	}
	h := sha256.Sum256([]byte(secret + ":store"))
	return h[:]
}

type Store struct {
	DB *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{DB: db}
}

func (s *Store) encryptLocal(raw []byte) (string, error) {
	block, err := aes.NewCipher(storeKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, raw, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func (s *Store) decryptLocal(encB64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(storeKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("store blob too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

func (s *Store) SaveActive(lic *SignedLicense, payload *LicensePayload) error {
	raw, err := json.Marshal(lic)
	if err != nil {
		return err
	}
	enc, err := s.encryptLocal(raw)
	if err != nil {
		return err
	}
	if err := s.upsertConfig(codeActive, enc); err != nil {
		return err
	}
	meta := Meta{
		Edition:      payload.Edition,
		Valid:        true,
		CustomerName: payload.CustomerName,
		LicenseID:    payload.LicenseID,
		ExpiresAt:    payload.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		Features:     payload.Features,
		MaxUsers:     payload.MaxUsers,
	}
	mb, _ := json.Marshal(meta)
	return s.upsertConfig(codeMeta, string(mb))
}

func (s *Store) LoadMeta() Meta {
	var row models.SysConfig
	if err := s.DB.Where("category = ? AND code = ?", categoryLicense, codeMeta).First(&row).Error; err != nil {
		return CommunityMeta()
	}
	var meta Meta
	if json.Unmarshal([]byte(row.Value), &meta) != nil {
		return CommunityMeta()
	}
	return meta
}

func (s *Store) LoadActive() (*SignedLicense, *LicensePayload, error) {
	var row models.SysConfig
	if err := s.DB.Where("category = ? AND code = ?", categoryLicense, codeActive).First(&row).Error; err != nil {
		return nil, nil, err
	}
	raw, err := s.decryptLocal(row.Value)
	if err != nil {
		return nil, nil, err
	}
	var lic SignedLicense
	if err := json.Unmarshal(raw, &lic); err != nil {
		return nil, nil, err
	}
	payload, err := tryDecryptLicense(&lic)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateMachineBinding(payload); err != nil {
		return nil, nil, err
	}
	return &lic, payload, nil
}

func (s *Store) Clear() error {
	s.DB.Where("category = ?", categoryLicense).Delete(&models.SysConfig{})
	return nil
}

func (s *Store) SaveCRL(payload, signature string) error {
	raw, _ := json.Marshal(map[string]string{"payload": payload, "signature": signature})
	return s.upsertConfig(codeCRL, string(raw))
}

func (s *Store) LoadCRL() (string, string, error) {
	var row models.SysConfig
	if err := s.DB.Where("category = ? AND code = ?", categoryLicense, codeCRL).First(&row).Error; err != nil {
		return "", "", err
	}
	var m map[string]string
	if json.Unmarshal([]byte(row.Value), &m) != nil {
		return "", "", errors.New("invalid crl cache")
	}
	return m["payload"], m["signature"], nil
}

func (s *Store) upsertConfig(code, value string) error {
	var row models.SysConfig
	err := s.DB.Where("category = ? AND code = ?", categoryLicense, code).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.DB.Create(&models.SysConfig{
			Category: categoryLicense,
			Code:     code,
			Value:    value,
			Status:   "active",
		}).Error
	}
	if err != nil {
		return err
	}
	row.Value = value
	row.Status = "active"
	return s.DB.Save(&row).Error
}

func (s *Store) SaveMeta(meta Meta) error {
	mb, _ := json.Marshal(meta)
	return s.upsertConfig(codeMeta, string(mb))
}

func (s *Store) Invalidate(reason string) error {
	_ = s.DB.Where("category = ? AND code = ?", categoryLicense, codeActive).Delete(&models.SysConfig{}).Error
	meta := CommunityMeta()
	if reason != "" {
		meta.Valid = false
		meta.Reason = reason
	}
	mb, _ := json.Marshal(meta)
	return s.upsertConfig(codeMeta, string(mb))
}
