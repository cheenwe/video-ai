package database

import (
	"baseplatform/internal/config"
	"baseplatform/internal/models"
	"baseplatform/internal/security"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"
	"baseplatform/internal/videoai"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DefaultAdminPassword is used when BASE_ADMIN_PASSWORD is unset on first bootstrap.
const DefaultAdminPassword = "ChangeMe123"

func Open(cfg *config.Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	var (
		db  *gorm.DB
		err error
	)
	switch cfg.DBDriver {
	case "mysql":
		db, err = gorm.Open(mysql.Open(cfg.DSN()), gormCfg)
	case "postgres":
		db, err = gorm.Open(postgres.Open(cfg.DSN()), gormCfg)
	default: // sqlite
		db, err = gorm.Open(sqlite.Open(filepath.Join(cfg.DataHome, "baseplatform.db")), gormCfg)
	}
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败（%s）：%w", cfg.DBDriver, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite 用单连接避免写锁冲突；MySQL/PostgreSQL 放开连接池
	if cfg.DBDriver == "sqlite" || cfg.DBDriver == "" {
		sqlDB.SetMaxOpenConns(1)
		for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
			if err = db.Exec(q).Error; err != nil {
				sqlDB.Close()
				return nil, err
			}
		}
	} else {
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
	}
	if err = db.AutoMigrate(
		&models.User{}, &models.Department{}, &models.SysConfig{}, &models.SysLog{}, &models.AuthRevokedToken{}, &models.OAuthAccount{}, &models.PersonalAccessToken{}, &models.ApiCallLog{}, &models.ScheduleCommand{}, &models.ScheduleJob{}, &models.ScheduleRun{}, &models.Storage{}, &models.SysDict{}, &models.SysDictItem{}, &models.Notification{}, &models.PlatformEvent{}, &models.WebhookEndpoint{}, &models.WebhookDelivery{}, &models.Role{}, &models.Permission{}, &models.RolePermission{}, &models.UserRole{}, &models.Project{}, &models.ProjectMember{}, &models.CustomFieldDef{}, &models.CustomFieldValue{}, &models.NVRAgent{},
		&models.CameraGroup{}, &models.Camera{}, &models.StreamProfile{},
		&models.BoundaryVersion{}, &models.AlgoModel{}, &models.AlgoModelVersion{}, &models.RuleVersion{}, &models.BusinessTemplate{},
		&models.Deployment{}, &models.DeploymentRevision{}, &models.RuntimeStatus{},
		&models.VideoEvent{}, &models.EventAction{}, &models.Evidence{},
		&models.RecordingPlan{}, &models.AccessDevice{}, &models.MediaOutage{}, &models.AnalysisStat{},
	); err != nil {
		sqlDB.Close()
		return nil, err
	}
	// 同步权限点与内置角色
	rbac := services.NewRBACService(db)
	if err := rbac.SyncPermissions(); err != nil {
		return nil, fmt.Errorf("同步权限点失败: %w", err)
	}
	if err := rbac.SeedBuiltinRoles(); err != nil {
		return nil, fmt.Errorf("初始化内置角色失败: %w", err)
	}
	if err := videoai.SeedDefaults(db); err != nil {
		return nil, fmt.Errorf("初始化视频 AI 默认数据失败: %w", err)
	}
	return db, nil
}

// BootstrapAdmin initializes only an empty database; existing accounts remain unchanged.
func BootstrapAdmin(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	pwd := os.Getenv("BASE_ADMIN_PASSWORD")
	if pwd == "" {
		pwd = DefaultAdminPassword
	}
	if err := services.ValidatePassword(pwd, settings.DefaultSecurity()); err != nil {
		return err
	}
	hash, err := security.HashPassword(pwd)
	if err != nil {
		return err
	}
	return db.Create(&models.User{Username: "admin", Role: models.RoleSuperAdmin, PasswordHash: hash, Timezone: "Asia/Shanghai", WebLoginEnabled: 1, AuthSource: models.AuthSourceLocal, MustChangePassword: 1}).Error
}
