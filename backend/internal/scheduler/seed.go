package scheduler

import (
	"errors"
	"gorm.io/gorm"
	"baseplatform/internal/models"
)

func seedPresets(db *gorm.DB) error {
	for _, j := range []models.ScheduleJob{
		{Name: "数据库检查", JobType: "builtin", BuiltinKey: "database_check", Cron: "*/5 * * * *", Timezone: "Asia/Shanghai", Enabled: 0, IsBuiltin: 1},
		{Name: "过期会话清理", JobType: "builtin", BuiltinKey: "session_cleanup", Cron: "0 3 * * *", Timezone: "Asia/Shanghai", Enabled: 0, IsBuiltin: 1},
		{Name: "定期备份", JobType: "builtin", BuiltinKey: "backup", Cron: "0 2 * * *", Timezone: "Asia/Shanghai", Enabled: 0, IsBuiltin: 1},
	} {
		var old models.ScheduleJob
		err := db.Where("is_builtin = 1 AND builtin_key = ?", j.BuiltinKey).First(&old).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err = db.Create(&j).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return db.Model(&models.ScheduleJob{}).Where("builtin_key = ?", "ntp_sync").Updates(map[string]any{"enabled": 0, "last_error": "旧NTP任务已停用，请使用运维中心时间查询"}).Error
}
