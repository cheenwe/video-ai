package handlers

import (
	"database/sql"
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

	// One pass over the events table instead of five COUNTs that each scan it.
	// pending and active_scene are deliberately NOT date-filtered (they are
	// "open right now"), so the WHERE is their union and each counter carries
	// its own guard — the totals are identical to the five separate queries.
	type eventCounts struct {
		Total       int64
		Confirmed   int64
		FalsePos    int64
		Pending     int64
		ActiveScene int64
	}
	var ec eventCounts
	h.DB.Model(&models.VideoEvent{}).
		Select(`coalesce(sum(case when started_at >= @from then 1 else 0 end),0) as total,
			coalesce(sum(case when started_at >= @from and handle_status = @confirmed then 1 else 0 end),0) as confirmed,
			coalesce(sum(case when started_at >= @from and conclusion = @false_positive then 1 else 0 end),0) as false_pos,
			coalesce(sum(case when handle_status = @pending then 1 else 0 end),0) as pending,
			coalesce(sum(case when scene_status = @active then 1 else 0 end),0) as active_scene`,
			sql.Named("from", from),
			sql.Named("confirmed", "confirmed"),
			sql.Named("false_positive", "false_positive"),
			sql.Named("pending", "pending"),
			sql.Named("active", "active")).
		Where("started_at >= ? OR handle_status = ? OR scene_status = ?", from, "pending", "active").
		Scan(&ec)
	total, confirmed, falsePos, pending, activeScene := ec.Total, ec.Confirmed, ec.FalsePos, ec.Pending, ec.ActiveScene

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

	// Analysis quality: undecidable rate comes from tick counters, because
	// frames that yield no verdict never become events.
	type tickRow struct {
		Ticks   int64
		Unknown int64
		Offline int64
		Normal  int64
		Near    int64
		Cross   int64
	}
	var tk tickRow
	h.DB.Model(&models.AnalysisStat{}).
		Select("coalesce(sum(ticks),0) as ticks, coalesce(sum(unknown),0) as unknown, coalesce(sum(offline),0) as offline, coalesce(sum(normal),0) as normal, coalesce(sum(near),0) as near, coalesce(sum(cross),0) as cross").
		Where("day >= ?", from.Format("2006-01-02")).Scan(&tk)
	unknownRate := 0.0
	offlineRate := 0.0
	if tk.Ticks > 0 {
		unknownRate = float64(tk.Unknown) / float64(tk.Ticks)
		offlineRate = float64(tk.Offline) / float64(tk.Ticks)
	}

	// Offline duration: closed outages in range plus still-open ones.
	type outageRow struct {
		CameraID  uint
		Secs      int64
		OpenCount int64
	}
	var closedSecs int64
	h.DB.Model(&models.MediaOutage{}).
		Select("coalesce(sum(duration_sec),0)").
		Where("ended_at IS NOT NULL AND started_at >= ?", from).Scan(&closedSecs)

	var openOutages []models.MediaOutage
	h.DB.Where("ended_at IS NULL").Find(&openOutages)
	openSecs := int64(0)
	openNow := len(openOutages)
	for _, o := range openOutages {
		openSecs += int64(time.Since(o.StartedAt).Seconds())
	}

	type outageByCam struct {
		CameraID uint
		Secs     float64
	}
	var outageRows []outageByCam
	h.DB.Model(&models.MediaOutage{}).
		Select("camera_id, coalesce(sum(duration_sec),0) as secs").
		Where("ended_at IS NOT NULL AND started_at >= ?", from).
		Group("camera_id").Order("secs desc").Limit(20).Scan(&outageRows)

	response.OK(c, gin.H{
		"range_days": days, "event_total": total, "confirmed": confirmed,
		"false_positive": falsePos, "pending": pending, "active_scene": activeScene,
		"running_deployments": running, "cameras": cameras, "published_boundaries": publishedBound,
		"by_camera": byCam, "trend": trend,
		"analysis_quality": gin.H{
			"ticks": tk.Ticks, "normal": tk.Normal, "near": tk.Near, "cross": tk.Cross,
			"unknown": tk.Unknown, "offline": tk.Offline,
			"unknown_rate": unknownRate, "offline_rate": offlineRate,
			"availability": 1 - unknownRate - offlineRate,
		},
		"offline": gin.H{
			"closed_sec": closedSecs, "open_sec": openSecs,
			"total_sec": closedSecs + openSecs, "open_now": openNow,
			"by_camera": outageRows,
		},
		"note": "人工误报比例是运营指标，不等于模型总体误报率；不可判定率按分析拍数统计，非按事件统计",
	}, "")
}

func (h *ReportHandler) RuntimeMetrics(c *gin.Context) {
	var items []models.RuntimeStatus
	h.DB.Order("updated_at desc").Limit(100).Find(&items)
	response.OK(c, items, "")
}
