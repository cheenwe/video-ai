package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
	"baseplatform/internal/videoai/recorder"

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
	// ConfigJSONAlias accepts the field name the model serializes back
	// (config_json). Without it, a client that reads a device and posts the
	// same object back would silently drop the whole config.
	ConfigJSONAlias json.RawMessage `json:"config_json"`
	Enabled         *int            `json:"enabled"`
}

// effectiveConfig returns whichever config alias the caller used, normalized
// so the stored form is always a JSON object.
func (r accessReq) effectiveConfig() json.RawMessage {
	raw := r.ConfigJSON
	if len(raw) == 0 {
		raw = r.ConfigJSONAlias
	}
	return normalizeConfigJSON(raw)
}

// normalizeConfigJSON accepts both an object and a quoted JSON string. The
// column stores text and the UI edits it as a textarea, so a client that
// round-trips config_json naturally sends a string; storing it verbatim would
// double-encode it and silently break probing.
func normalizeConfigJSON(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return json.RawMessage(bytes.TrimSpace([]byte(s)))
		}
	}
	return raw
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
	if raw := req.effectiveConfig(); len(raw) > 0 {
		cfg = string(raw)
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
	if raw := req.effectiveConfig(); len(raw) > 0 {
		upd["config_json"] = string(raw)
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

// RecordingPlanHandler manages per-camera retain policies and, for cameras the
// platform can pull itself, starts/stops platform-side recording.
type RecordingPlanHandler struct {
	DB *gorm.DB
	// Recorder is optional; nil keeps plans advisory-only (NVR agent does the
	// recording) which is how the platform behaves without it.
	Recorder *recorder.Manager
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

	// Apply the plan to the platform recorder. Failures are reported in the
	// response instead of being swallowed: a plan that says "continuous" while
	// nothing records is worse than an explicit error.
	msg := "已保存"
	if h.Recorder != nil {
		if err := h.applyRecordingPlan(plan); err != nil {
			msg = "计划已保存，但平台录制未启动：" + err.Error()
		} else if plan.Mode == "continuous" || plan.Mode == "schedule" {
			msg = "已保存，平台录制已按计划应用"
		}
	}
	response.OK(c, plan, msg)
}

// applyRecordingPlan reconciles the recorder with a saved plan. Only direct
// streams are recorded here: NVR-bound cameras record on the agent side, and
// event_only/off mean no continuous process.
func (h *RecordingPlanHandler) applyRecordingPlan(plan models.RecordingPlan) error {
	camID := plan.CameraID
	if plan.Enabled == 0 || (plan.Mode != "continuous" && plan.Mode != "schedule") {
		h.Recorder.Stop(camID)
		return nil
	}
	var cam models.Camera
	if err := h.DB.First(&cam, camID).Error; err != nil {
		return fmt.Errorf("摄像头不存在")
	}
	if cam.NVRAgentID != nil && cam.AgentCameraID != "" {
		// NVR-bound: the agent owns recording; do not double-record.
		h.Recorder.Stop(camID)
		return nil
	}
	url := streamURL(h.DB, camID)
	if url == "" {
		return fmt.Errorf("摄像头未配置码流地址")
	}
	scheduleJSON := ""
	if plan.Mode == "schedule" {
		if plan.ScheduleJSON == "" {
			return fmt.Errorf("schedule 模式需要 schedule_json 时段定义")
		}
		scheduleJSON = plan.ScheduleJSON
	}
	return h.Recorder.Start(camID, url, plan.RetainDays, plan.MaxGB, scheduleJSON)
}

// Recordings lists the platform-recorded segments for a camera, plus the live
// recorder state so the UI can tell "nothing recorded" from "recorder broken".
func (h *RecordingPlanHandler) Recordings(c *gin.Context) {
	camID, _ := strconv.Atoi(c.Param("id"))
	var cam models.Camera
	if h.DB.First(&cam, camID).Error != nil {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}
	if h.Recorder == nil {
		response.BizFail(c, "平台录制未启用", http.StatusServiceUnavailable)
		return
	}
	segs, err := h.Recorder.List(uint(camID))
	if err != nil {
		response.BizFail(c, "读取录像目录失败："+err.Error(), 500)
		return
	}
	response.OK(c, gin.H{
		"camera_id": uint(camID),
		"state":     h.Recorder.Status(uint(camID)),
		"segments":  segs,
		"nvr_bound": cam.NVRAgentID != nil && cam.AgentCameraID != "",
	}, "")
}

// RecordingFile serves one recorded segment. Names are validated by the
// recorder so a crafted name cannot escape the camera directory.
func (h *RecordingPlanHandler) RecordingFile(c *gin.Context) {
	camID, _ := strconv.Atoi(c.Param("id"))
	name := c.Param("name")
	if h.Recorder == nil {
		response.BizFail(c, "平台录制未启用", http.StatusServiceUnavailable)
		return
	}
	path, err := h.Recorder.SegmentPath(uint(camID), name)
	if err != nil {
		response.BizFail(c, err.Error(), 404)
		return
	}
	c.Header("Content-Type", "video/mp4")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", name))
	c.File(path)
}
