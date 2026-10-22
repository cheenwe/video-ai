package handlers

import (
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct {
	DB *gorm.DB
}

func (h *ReportHandler) Summary(c *gin.Context) {
	days := 7
	from := time.Now().AddDate(0, 0, -days)

	var total, confirmed, falsePos, pending, activeScene int64
	h.DB.Model(&models.VideoEvent{}).Where("started_at >= ?", from).Count(&total)
	h.DB.Model(&models.VideoEvent{}).Where("started_at >= ? AND handle_status = ?", from, "confirmed").Count(&confirmed)
	h.DB.Model(&models.VideoEvent{}).Where("started_at >= ? AND conclusion = ?", from, "false_positive").Count(&falsePos)
	h.DB.Model(&models.VideoEvent{}).Where("handle_status = ?", "pending").Count(&pending)
	h.DB.Model(&models.VideoEvent{}).Where("scene_status = ?", "active").Count(&activeScene)

	type camRow struct {
		CameraID uint
		Cnt      int64
	}
	var byCam []camRow
	h.DB.Model(&models.VideoEvent{}).Select("camera_id, count(*) as cnt").
		Where("started_at >= ?", from).Group("camera_id").Order("cnt desc").Limit(20).Scan(&byCam)

	var running, cameras, publishedBound int64
	h.DB.Model(&models.Deployment{}).Where("status = ?", "running").Count(&running)
	h.DB.Model(&models.Camera{}).Where("archived = 0 AND enabled = 1").Count(&cameras)
	h.DB.Model(&models.BoundaryVersion{}).Where("status = ?", "published").Count(&publishedBound)

	// Daily trend
	type dayRow struct {
		Day string
		Cnt int64
	}
	var trend []dayRow
	h.DB.Model(&models.VideoEvent{}).
		Select("date(started_at) as day, count(*) as cnt").
		Where("started_at >= ?", from).
		Group("date(started_at)").Order("day asc").Scan(&trend)

	response.OK(c, gin.H{
		"range_days": days, "event_total": total, "confirmed": confirmed,
		"false_positive": falsePos, "pending": pending, "active_scene": activeScene,
		"running_deployments": running, "cameras": cameras, "published_boundaries": publishedBound,
		"by_camera": byCam, "trend": trend,
		"note": "人工误报比例是运营指标，不等于模型总体误报率",
	}, "")
}

func (h *ReportHandler) RuntimeMetrics(c *gin.Context) {
	var items []models.RuntimeStatus
	h.DB.Order("updated_at desc").Limit(100).Find(&items)
	response.OK(c, items, "")
}
