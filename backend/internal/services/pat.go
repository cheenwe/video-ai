package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/settings"

	"gorm.io/gorm"
)

const TokenPrefixTag = "va_"

var (
	ErrTokenNotFound = errors.New("令牌不存在")
	ErrTokenDisabled  = errors.New("令牌已禁用")
	ErrTokenExpired   = errors.New("令牌已过期")
	ErrTokenInvalid   = errors.New("令牌无效")
)

type PATService struct {
	DB *gorm.DB
}

func NewPATService(db *gorm.DB) *PATService {
	return &PATService{DB: db}
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func IsPAT(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), TokenPrefixTag)
}

func generateRawToken() (raw, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	raw = TokenPrefixTag + hex.EncodeToString(b)
	prefix = raw
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	hash = HashToken(raw)
	return raw, prefix, hash, nil
}

type CreateTokenInput struct {
	Name      string
	ExpiresAt *time.Time
	CreatedBy *uint
}

func (s *PATService) Create(user models.User, in CreateTokenInput) (*models.PersonalAccessToken, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, "", errors.New("名称不能为空")
	}
	raw, prefix, hash, err := generateRawToken()
	if err != nil {
		return nil, "", err
	}
	row := models.PersonalAccessToken{
		UserID:      user.ID,
		Name:        name,
		TokenPrefix: prefix,
		TokenHash:   hash,
		Enabled:     1,
		ExpiresAt:   in.ExpiresAt,
		CreatedBy:   in.CreatedBy,
	}
	if err := s.DB.Create(&row).Error; err != nil {
		return nil, "", err
	}
	return &row, raw, nil
}

func (s *PATService) ListForUser(userID uint) ([]models.PersonalAccessToken, error) {
	var rows []models.PersonalAccessToken
	err := s.DB.Where("user_id = ?", userID).Order("id desc").Find(&rows).Error
	return rows, err
}

func (s *PATService) GetOwned(userID, tokenID uint) (*models.PersonalAccessToken, error) {
	var row models.PersonalAccessToken
	if err := s.DB.Where("id = ? AND user_id = ?", tokenID, userID).First(&row).Error; err != nil {
		return nil, ErrTokenNotFound
	}
	return &row, nil
}

type UpdateTokenInput struct {
	Name      *string
	Enabled   *bool
	ExpiresAt **time.Time
}

func (s *PATService) Update(user models.User, tokenID uint, in UpdateTokenInput) (*models.PersonalAccessToken, error) {
	row, err := s.GetOwned(user.ID, tokenID)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, errors.New("名称不能为空")
		}
		updates["name"] = name
	}
	if in.Enabled != nil {
		if *in.Enabled {
			updates["enabled"] = 1
		} else {
			updates["enabled"] = 0
		}
	}
	if in.ExpiresAt != nil {
		updates["expires_at"] = *in.ExpiresAt
	}
	if err := s.DB.Model(row).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetOwned(user.ID, tokenID)
}

func (s *PATService) Delete(userID, tokenID uint) error {
	res := s.DB.Where("id = ? AND user_id = ?", tokenID, userID).Delete(&models.PersonalAccessToken{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (s *PATService) Authenticate(raw string) (*models.User, *models.PersonalAccessToken, error) {
	raw = strings.TrimSpace(raw)
	if !IsPAT(raw) {
		return nil, nil, ErrTokenInvalid
	}
	if !settings.LoadGateway(s.DB).PersonalTokensEnabled {
		return nil, nil, ErrTokenDisabled
	}
	hash := HashToken(raw)
	var row models.PersonalAccessToken
	if err := s.DB.Where("token_hash = ?", hash).First(&row).Error; err != nil {
		return nil, nil, ErrTokenInvalid
	}
	if row.Enabled == 0 {
		return nil, nil, ErrTokenDisabled
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
		return nil, nil, ErrTokenExpired
	}
	var user models.User
	if err := s.DB.First(&user, row.UserID).Error; err != nil {
		return nil, nil, ErrTokenInvalid
	}
	if user.Disabled != 0 {
		return nil, nil, ErrTokenDisabled
	}
	now := time.Now()
	_ = s.DB.Model(&row).Update("last_used_at", now)
	return &user, &row, nil
}

func TokenView(row models.PersonalAccessToken) map[string]interface{} {
	return map[string]interface{}{
		"id":           row.ID,
		"name":         row.Name,
		"token_prefix": row.TokenPrefix,
		"enabled":      row.Enabled != 0,
		"expires_at":   row.ExpiresAt,
		"last_used_at": row.LastUsedAt,
		"created_by":   row.CreatedBy,
		"created_at":   row.CreatedAt,
	}
}

func (s *PATService) AdminListForUser(userID uint) ([]models.PersonalAccessToken, error) {
	return s.ListForUser(userID)
}

func (s *PATService) AdminDeleteForUser(userID, tokenID uint) error {
	return s.Delete(userID, tokenID)
}

var _ = fmt.Sprintf
