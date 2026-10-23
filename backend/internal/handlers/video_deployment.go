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
	"baseplatform/internal/videoai"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DeploymentHandler struct {
	DB      *gorm.DB
	Runtime *videoai.Manager
	RBAC    *services.RBACService
}

func (h *DeploymentHandler) List(c *gin.Context) {
	u := middleware.CurrentUser(c)
	q := h.RBAC.ApplyDataScope(h.DB.Model(&models.Deployment{}), u, services.ScopeOptions{
		ProjectIDColumn: "project_id",
	})
	if cam := c.Query("camera_id"); cam != "" {
		q = q.Where("camera_id = ?", cam)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	var items []models.Deployment
	q.Order("id desc").Limit(200).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, d := range items {
		row := deploymentOut(d)
		var rt models.RuntimeStatus
		if h.DB.Where("deployment_id = ?", d.ID).First(&rt).Error == nil {
			row["runtime"] = rt
		}
		out = append(out, row)
	}
	response.OK(c, out, "")
}

func deploymentOut(d models.Deployment) gin.H {
	return gin.H{
		"id": d.ID, "name": d.Name, "camera_id": d.CameraID, "project_id": d.ProjectID,
		"template_id": d.TemplateID, "boundary_version_id": d.BoundaryVersionID,
		"desired_revision": d.DesiredRevision, "applied_revision": d.AppliedRevision,
		"status": d.Status, "analysis_fps": d.AnalysisFPS,
		"threshold_near": d.ThresholdNear, "threshold_cross": d.ThresholdCross,
		"trigger_hold_sec": d.TriggerHoldSec, "recover_hold_sec": d.RecoverHoldSec,
		"schedule_cron": d.ScheduleCron, "mute_notify": d.MuteNotify != 0,
		"osd_enabled": d.OSDEnabled != 0, "last_error": d.LastError,
		"last_self_check": d.LastSelfCheck, "started_at": d.StartedAt, "stopped_at": d.StoppedAt,
		"created_at": d.CreatedAt, "updated_at": d.UpdatedAt,
	}
}

type deploymentReq struct {
	Name              string   `json:"name" binding:"required,max=128"`
	CameraID          uint     `json:"camera_id" binding:"required"`
	ProjectID         *uint    `json:"project_id"`
	TemplateID        *uint    `json:"template_id"`
	BoundaryVersionID *uint    `json:"boundary_version_id"`
	AnalysisFPS       *float64 `json:"analysis_fps"`
	ThresholdNear     *float64 `json:"threshold_near"`
	ThresholdCross    *float64 `json:"threshold_cross"`
	TriggerHoldSec    *float64 `json:"trigger_hold_sec"`
	RecoverHoldSec    *float64 `json:"recover_hold_sec"`
	ScheduleCron      string   `json:"schedule_cron"`
	MuteNotify        *int     `json:"mute_notify"`
	OSDEnabled        *int     `json:"osd_enabled"`
	ConfigJSON        string   `json:"config_json"`
}

func (h *DeploymentHandler) Create(c *gin.Context) {
	var req deploymentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if err := videoai.ValidateScheduleCron(req.ScheduleCron); err != nil {
		response.BizFail(c, "时段表达式无效："+err.Error(), 422)
		return
	}
	var cam models.Camera
	if h.DB.First(&cam, req.CameraID).Error != nil || cam.Archived == 1 {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}
	u := middleware.CurrentUser(c)
	d := models.Deployment{
		Name: strings.TrimSpace(req.Name), CameraID: req.CameraID,
		ProjectID: firstUint(req.ProjectID, cam.ProjectID), TemplateID: req.TemplateID,
		BoundaryVersionID: req.BoundaryVersionID, DesiredRevision: 1, Status: "draft",
		AnalysisFPS: 2, ThresholdNear: 0.15, ThresholdCross: 0,
		TriggerHoldSec: 1, RecoverHoldSec: 2, OSDEnabled: 1, CreatedBy: &u.ID,
		ScheduleCron: req.ScheduleCron, ConfigJSON: req.ConfigJSON,
	}
	applyDeploymentReq(&d, req)
	if err := h.DB.Create(&d).Error; err != nil {
		response.BizFail(c, "创建失败", 500)
		return
	}
	h.saveRevision(d, &u.ID)
	services.LogAction(h.DB, c, "audit", "success", "deployment.create", &u, d.Name)
	response.OK(c, deploymentOut(d), "已创建")
}

func (h *DeploymentHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var d models.Deployment
	if h.DB.First(&d, id).Error != nil {
		response.BizFail(c, "布控不存在", 404)
		return
	}
	var req deploymentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if err := videoai.ValidateScheduleCron(req.ScheduleCron); err != nil {
		response.BizFail(c, "时段表达式无效："+err.Error(), 422)
		return
	}
	d.Name = strings.TrimSpace(req.Name)
	d.TemplateID = req.TemplateID
	d.BoundaryVersionID = req.BoundaryVersionID
	d.ScheduleCron = req.ScheduleCron
	d.ConfigJSON = req.ConfigJSON
	if req.ProjectID != nil {
		d.ProjectID = req.ProjectID
	}
	applyDeploymentReq(&d, req)
	d.DesiredRevision++
	if err := h.DB.Save(&d).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	h.saveRevision(d, &u.ID)
	services.LogAction(h.DB, c, "audit", "success", "deployment.update", &u, d.Name)
	response.OK(c, deploymentOut(d), "已更新")
}

func applyDeploymentReq(d *models.Deployment, req deploymentReq) {
	if req.AnalysisFPS != nil {
		d.AnalysisFPS = *req.AnalysisFPS
	}
	if req.ThresholdNear != nil {
		d.ThresholdNear = *req.ThresholdNear
	}
	if req.ThresholdCross != nil {
		d.ThresholdCross = *req.ThresholdCross
	}
	if req.TriggerHoldSec != nil {
		d.TriggerHoldSec = *req.TriggerHoldSec
	}
	if req.RecoverHoldSec != nil {
		d.RecoverHoldSec = *req.RecoverHoldSec
	}
	if req.MuteNotify != nil {
		d.MuteNotify = *req.MuteNotify
	}
	if req.OSDEnabled != nil {
		d.OSDEnabled = *req.OSDEnabled
	}
}

func (h *DeploymentHandler) saveRevision(d models.Deployment, by *uint) {
	snap, _ := json.Marshal(deploymentOut(d))
	_ = h.DB.Create(&models.DeploymentRevision{
		DeploymentID: d.ID, Revision: d.DesiredRevision, SnapshotJSON: string(snap), CreatedBy: by,
	}).Error
}

func (h *DeploymentHandler) SelfCheck(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var d models.Deployment
	if h.DB.First(&d, id).Error != nil {
		response.BizFail(c, "布控不存在", 404)
		return
	}
	ok, report := videoai.SelfCheck(h.DB, d)
	b, _ := json.Marshal(report)
	_ = h.DB.Model(&d).Update("last_self_check", string(b)).Error
	response.OK(c, gin.H{"ok": ok, "report": report}, "")
}

func (h *DeploymentHandler) Start(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var d models.Deployment
	if h.DB.First(&d, id).Error != nil {
		response.BizFail(c, "布控不存在", 404)
		return
	}
	ok, report := videoai.SelfCheck(h.DB, d)
	b, _ := json.Marshal(report)
	_ = h.DB.Model(&d).Update("last_self_check", string(b)).Error
	if !ok {
		_ = h.DB.Model(&d).Updates(map[string]any{"status": "fault", "last_error": "自检未通过"}).Error
		response.BizFail(c, "自检未通过，不能启动", 422)
		return
	}
	if h.Runtime == nil {
		response.BizFail(c, "分析运行时未就绪", 503)
		return
	}
	_ = h.DB.Model(&d).Updates(map[string]any{"status": "starting", "last_error": ""}).Error
	if err := h.Runtime.StartDeployment(uint(id)); err != nil {
		response.BizFail(c, err.Error(), 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "deployment.start", &u, fmt.Sprintf("%d", id))
	_ = h.DB.First(&d, id)
	response.OK(c, deploymentOut(d), "已启动")
}

func (h *DeploymentHandler) Stop(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if h.Runtime != nil {
		h.Runtime.StopDeployment(uint(id), "stopped")
	} else {
		_ = h.DB.Model(&models.Deployment{}).Where("id = ?", id).Update("status", "stopped")
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "deployment.stop", &u, fmt.Sprintf("%d", id))
	var d models.Deployment
	_ = h.DB.First(&d, id)
	response.OK(c, deploymentOut(d), "已停止")
}

func (h *DeploymentHandler) Pause(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if h.Runtime != nil {
		h.Runtime.PauseDeployment(uint(id))
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "deployment.pause", &u, fmt.Sprintf("%d", id))
	var d models.Deployment
	_ = h.DB.First(&d, id)
	response.OK(c, deploymentOut(d), "已暂停分析")
}

func firstUint(a, b *uint) *uint {
	if a != nil {
		return a
	}
	return b
}
