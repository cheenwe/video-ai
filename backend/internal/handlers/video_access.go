package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AccessHandler manages GB28181 / push-stream device skeletons (M6).
type AccessHandler struct {
	DB *gorm.DB
}

func (h *AccessHandler) List(c *gin.Context) {
	q := h.DB.Model(&models.AccessDevice{})
	if kind := c.Query("kind"); kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var items []models.AccessDevice
	q.Order("id desc").Limit(200).Find(&items)
	response.OK(c, items, "")
}

type accessReq struct {
	Kind       string          `json:"kind" binding:"required"`
	Code       string          `json:"code" binding:"required"`
	Name       string          `json:"name" binding:"required"`
	CameraID   *uint           `json:"camera_id"`
	ConfigJSON json.RawMessage `json:"config"`
	Enabled    *int            `json:"enabled"`
}

func (h *AccessHandler) Create(c *gin.Context) {
	var req accessReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	kind := strings.TrimSpace(req.Kind)
	switch kind {
	case "gb28181", "rtmp_push", "http_flv", "hls_in":
	default:
		response.BizFail(c, "不支持的接入类型", 422)
		return
	}
	cfg := "{}"
	if len(req.ConfigJSON) > 0 {
		cfg = string(req.ConfigJSON)
	}
	d := models.AccessDevice{
		Kind: kind, Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name),
		CameraID: req.CameraID, ConfigJSON: cfg, Status: "registered", Enabled: 1,
	}
	if req.Enabled != nil {
		d.Enabled = *req.Enabled
	}
	if err := h.DB.Create(&d).Error; err != nil {
		response.BizFail(c, "创建失败（编码可能重复）", 400)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "access.create", &u, d.Code)
	response.OK(c, d, "已登记（适配器骨架，需真机联调）")
}

func (h *AccessHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var d models.AccessDevice
	if h.DB.First(&d, id).Error != nil {
		response.BizFail(c, "设备不存在", 404)
		return
	}
	var req accessReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	upd := map[string]any{"name": strings.TrimSpace(req.Name), "camera_id": req.CameraID}
	if len(req.ConfigJSON) > 0 {
		upd["config_json"] = string(req.ConfigJSON)
	}
	if req.Enabled != nil {
		upd["enabled"] = *req.Enabled
	}
	_ = h.DB.Model(&d).Updates(upd).Error
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "access.update", &u, d.Code)
	_ = h.DB.First(&d, id)
	response.OK(c, d, "已更新")
}

func (h *AccessHandler) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.DB.Delete(&models.AccessDevice{}, id).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "access.delete", &u, fmt.Sprintf("%d", id))
	response.OK(c, nil, "已删除")
}

func (h *AccessHandler) Probe(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var d models.AccessDevice
	if h.DB.First(&d, id).Error != nil {
		response.BizFail(c, "设备不存在", 404)
		return
	}
	// Skeleton: mark as needing adapter implementation.
	msg := "适配器尚未对接真机信令/媒体，当前仅保存配置"
	_ = h.DB.Model(&d).Updates(map[string]any{"status": "pending_adapter", "last_error": msg}).Error
	response.OK(c, gin.H{
		"id": d.ID, "kind": d.Kind, "status": "pending_adapter",
		"ok": false, "message": msg,
		"next_steps": []string{"配置 SIP/国标平台参数", "联调目录与邀请", "绑定平台摄像头通道"},
	}, "")
}

// RecordingPlanHandler manages per-camera retain policies.
type RecordingPlanHandler struct {
	DB *gorm.DB
}

func (h *RecordingPlanHandler) Get(c *gin.Context) {
	camID, _ := strconv.Atoi(c.Param("cameraId"))
	var plan models.RecordingPlan
	err := h.DB.Where("camera_id = ?", camID).First(&plan).Error
	if err == gorm.ErrRecordNotFound {
		response.OK(c, models.RecordingPlan{CameraID: uint(camID), Mode: "off", RetainDays: 7, MaxGB: 50, Enabled: 1}, "")
		return
	}
	response.OK(c, plan, "")
}

func (h *RecordingPlanHandler) Save(c *gin.Context) {
	camID, _ := strconv.Atoi(c.Param("cameraId"))
	var req models.RecordingPlan
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	var plan models.RecordingPlan
	err := h.DB.Where("camera_id = ?", camID).First(&plan).Error
	if err == gorm.ErrRecordNotFound {
		plan = models.RecordingPlan{CameraID: uint(camID), Mode: req.Mode, RetainDays: req.RetainDays, MaxGB: req.MaxGB, ScheduleJSON: req.ScheduleJSON, Enabled: 1}
		if plan.Mode == "" {
			plan.Mode = "off"
		}
		_ = h.DB.Create(&plan).Error
	} else {
		_ = h.DB.Model(&plan).Updates(map[string]any{
			"mode": req.Mode, "retain_days": req.RetainDays, "max_gb": req.MaxGB,
			"schedule_json": req.ScheduleJSON, "enabled": req.Enabled,
		}).Error
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "recording.plan.save", &u, fmt.Sprintf("cam=%d", camID))
	_ = h.DB.Where("camera_id = ?", camID).First(&plan)
	response.OK(c, plan, "已保存")
}
