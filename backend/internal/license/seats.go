package license

import (
	"errors"
	"time"

	"baseplatform/internal/models"
	"gorm.io/gorm"
)

var ErrUserLimitReached = errors.New("USER_LIMIT_REACHED")

func CountEnabledUsers(db *gorm.DB) (int64, error) {
	var n int64
	err := db.Model(&models.User{}).Where("disabled = ?", 0).Count(&n).Error
	return n, err
}

func CanEnableMoreUsers(db *gorm.DB, meta Meta, additional int64) (bool, int64, error) {
	limit := EffectiveMaxUsers(meta)
	if limit == 0 {
		return true, 0, nil
	}
	count, err := CountEnabledUsers(db)
	if err != nil {
		return false, count, err
	}
	return count+additional <= int64(limit), count, nil
}

func CheckSeatAvailable(db *gorm.DB, meta Meta) error {
	ok, _, err := CanEnableMoreUsers(db, meta, 1)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUserLimitReached
	}
	return nil
}

func seatProtected(u models.User) bool {
	return u.IsSystemUser()
}

// EnsureSystemAccountsEnabled re-enables built-in admin accounts wrongly disabled by seat reconcile.
func EnsureSystemAccountsEnabled(db *gorm.DB) error {
	return db.Model(&models.User{}).
		Where("username IN ?", []string{"admin", "superadmin"}).
		Where("disabled = ?", 1).
		Update("disabled", 0).Error
}

// ReconcileUserSeats disables least-recently-used enabled users until within limit.
// System accounts (admin / superadmin / guest) are never auto-disabled.
func ReconcileUserSeats(db *gorm.DB, meta Meta) (int, error) {
	_ = EnsureSystemAccountsEnabled(db)

	limit := EffectiveMaxUsers(meta)
	if limit == 0 {
		return 0, nil
	}
	var users []models.User
	if err := db.Where("disabled = ?", 0).
		Order("CASE WHEN last_login_at IS NULL THEN 0 ELSE 1 END").
		Order("last_login_at ASC").
		Order("id ASC").
		Find(&users).Error; err != nil {
		return 0, err
	}
	var regular []models.User
	for _, u := range users {
		if seatProtected(u) {
			continue
		}
		regular = append(regular, u)
	}
	protected := len(users) - len(regular)
	disabled := 0
	for protected+len(regular) > limit && len(regular) > 0 {
		u := regular[0]
		regular = regular[1:]
		if err := db.Model(&u).Update("disabled", 1).Error; err != nil {
			return disabled, err
		}
		disabled++
	}
	return disabled, nil
}

func TouchUserLogin(db *gorm.DB, userID uint) {
	now := time.Now().UTC()
	db.Model(&models.User{}).Where("id = ?", userID).Update("last_login_at", now)
}
