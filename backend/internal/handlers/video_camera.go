package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CameraHandler struct {
	DB   *gorm.DB
	RBAC *services.RBACService
}

func cameraOut(c models.Camera, streams []models.StreamProfile) gin.H {
	out := gin.H{
		"id": c.ID, "code": c.Code, "name": c.Name,
		"project_id": c.ProjectID, "group_id": c.GroupID,
		"access_type": c.AccessType, "vendor": c.Vendor, "model": c.Model,
		"remark": c.Remark, "enabled": c.Enabled != 0, "archived": c.Archived != 0,
		"nvr_agent_id": c.NVRAgentID, "agent_camera_id": c.AgentCameraID,
		"device_status": c.DeviceStatus, "media_status": c.MediaStatus,
		"analysis_status": c.AnalysisStatus, "last_error": c.LastError,
		"created_at": c.CreatedAt, "updated_at": c.UpdatedAt,
	}
	if streams != nil {
		arr := make([]gin.H, 0, len(streams))
		for _, s := range streams {
			arr = append(arr, gin.H{
				"id": s.ID, "role": s.Role, "url": s.URL, "protocol": s.Protocol,
				"revision": s.Revision, "fov_changed": s.FOVChanged != 0, "enabled": s.Enabled != 0,
			})
		}
		out["streams"] = arr
	}
	return out
}

func (h *CameraHandler) scopeCameras(c *gin.Context) *gorm.DB {
	u := middleware.CurrentUser(c)
	return h.RBAC.ApplyDataScope(h.DB.Model(&models.Camera{}), u, services.ScopeOptions{
		ProjectIDColumn: "project_id",
	}).Where("archived = 0")
}

func (h *CameraHandler) ListGroups(c *gin.Context) {
	u := middleware.CurrentUser(c)
	q := h.RBAC.ApplyDataScope(h.DB.Model(&models.CameraGroup{}), u, services.ScopeOptions{
		ProjectIDColumn: "project_id",
	})
	var items []models.CameraGroup
	q.Order("id desc").Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, g := range items {
		out = append(out, gin.H{
			"id": g.ID, "code": g.Code, "name": g.Name, "project_id": g.ProjectID,
			"remark": g.Remark, "enabled": g.Enabled != 0,
		})
	}
	response.OK(c, out, "")
}

type groupReq struct {
	Code      string `json:"code" binding:"required,max=64"`
	Name      string `json:"name" binding:"required,max=128"`
	ProjectID *uint  `json:"project_id"`
	Remark    string `json:"remark"`
	Enabled   *int   `json:"enabled"`
}

func (h *CameraHandler) CreateGroup(c *gin.Context) {
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	g := models.CameraGroup{
		Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name),
		ProjectID: req.ProjectID, Remark: req.Remark, Enabled: 1,
	}
	if err := h.DB.Create(&g).Error; err != nil {
		response.BizFail(c, "创建失败（编码可能重复）", 400)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.group.create", &u, g.Code)
	response.OK(c, g, "已创建")
}

func (h *CameraHandler) UpdateGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var g models.CameraGroup
	if h.DB.First(&g, id).Error != nil {
		response.BizFail(c, "分组不存在", 404)
		return
	}
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	updates := map[string]any{"name": strings.TrimSpace(req.Name), "remark": req.Remark, "project_id": req.ProjectID}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	_ = h.DB.Model(&g).Updates(updates).Error
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.group.update", &u, g.Code)
	response.OK(c, g, "已更新")
}

func (h *CameraHandler) DeleteGroup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var n int64
	h.DB.Model(&models.Camera{}).Where("group_id = ? AND archived = 0", id).Count(&n)
	if n > 0 {
		response.BizFail(c, "分组下仍有摄像头", 400)
		return
	}
	if err := h.DB.Delete(&models.CameraGroup{}, id).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.group.delete", &u, fmt.Sprintf("%d", id))
	response.OK(c, nil, "已删除")
}

func (h *CameraHandler) List(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	q := h.scopeCameras(c)
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR code LIKE ?", like, like)
	}
	if pid := c.Query("project_id"); pid != "" {
		q = q.Where("project_id = ?", pid)
	}
	if gid := c.Query("group_id"); gid != "" {
		q = q.Where("group_id = ?", gid)
	}
	var total int64
	q.Count(&total)
	var items []models.Camera
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, cam := range items {
		var streams []models.StreamProfile
		h.DB.Where("camera_id = ?", cam.ID).Find(&streams)
		out = append(out, cameraOut(cam, streams))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

type cameraReq struct {
	Code          string `json:"code" binding:"required,max=64"`
	Name          string `json:"name" binding:"required,max=128"`
	ProjectID     *uint  `json:"project_id"`
	GroupID       *uint  `json:"group_id"`
	AccessType    string `json:"access_type"`
	Vendor        string `json:"vendor"`
	Model         string `json:"model"`
	Remark        string `json:"remark"`
	Enabled       *int   `json:"enabled"`
	NVRAgentID    *uint  `json:"nvr_agent_id"`
	AgentCameraID string `json:"agent_camera_id"`
	MainURL       string `json:"main_url"`
	SubURL        string `json:"sub_url"`
}

func (h *CameraHandler) Create(c *gin.Context) {
	var req cameraReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	at := strings.TrimSpace(req.AccessType)
	if at == "" {
		at = "nvr_agent"
	}
	cam := models.Camera{
		Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name),
		ProjectID: req.ProjectID, GroupID: req.GroupID, AccessType: at,
		Vendor: req.Vendor, Model: req.Model, Remark: req.Remark, Enabled: 1,
		NVRAgentID: req.NVRAgentID, AgentCameraID: strings.TrimSpace(req.AgentCameraID),
		DeviceStatus: "registered", MediaStatus: "unknown", AnalysisStatus: "idle",
	}
	if req.Enabled != nil {
		cam.Enabled = *req.Enabled
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&cam).Error; err != nil {
			return err
		}
		return h.upsertStreams(tx, cam.ID, req.MainURL, req.SubURL, false)
	}); err != nil {
		response.BizFail(c, "创建失败（编号可能重复）", 400)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.create", &u, cam.Code)
	var streams []models.StreamProfile
	h.DB.Where("camera_id = ?", cam.ID).Find(&streams)
	response.OK(c, cameraOut(cam, streams), "已创建")
}

func (h *CameraHandler) Get(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cam models.Camera
	if h.scopeCameras(c).First(&cam, id).Error != nil {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}
	var streams []models.StreamProfile
	h.DB.Where("camera_id = ?", cam.ID).Find(&streams)
	response.OK(c, cameraOut(cam, streams), "")
}

func (h *CameraHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cam models.Camera
	if h.scopeCameras(c).First(&cam, id).Error != nil {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}
	var req cameraReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	fovChanged := false
	var oldMain models.StreamProfile
	if h.DB.Where("camera_id = ? AND role = ?", cam.ID, "main").First(&oldMain).Error == nil {
		if req.MainURL != "" && req.MainURL != oldMain.URL {
			fovChanged = true
		}
	}
	updates := map[string]any{
		"name": strings.TrimSpace(req.Name), "project_id": req.ProjectID, "group_id": req.GroupID,
		"vendor": req.Vendor, "model": req.Model, "remark": req.Remark,
		"nvr_agent_id": req.NVRAgentID, "agent_camera_id": strings.TrimSpace(req.AgentCameraID),
	}
	if req.AccessType != "" {
		updates["access_type"] = req.AccessType
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&cam).Updates(updates).Error; err != nil {
			return err
		}
		if err := h.upsertStreams(tx, cam.ID, req.MainURL, req.SubURL, fovChanged); err != nil {
			return err
		}
		if fovChanged {
			return tx.Model(&models.BoundaryVersion{}).
				Where("camera_id = ? AND status = ?", cam.ID, "published").
				Updates(map[string]any{"status": "invalid", "invalid_reason": "码流/视场变更，需重新校准边界"}).Error
		}
		return nil
	}); err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.update", &u, cam.Code)
	_ = h.DB.First(&cam, id)
	var streams []models.StreamProfile
	h.DB.Where("camera_id = ?", cam.ID).Find(&streams)
	response.OK(c, cameraOut(cam, streams), "已更新")
}

func (h *CameraHandler) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cam models.Camera
	if h.scopeCameras(c).First(&cam, id).Error != nil {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}
	var running int64
	h.DB.Model(&models.Deployment{}).Where("camera_id = ? AND status IN ?", cam.ID, []string{"running", "starting", "pending"}).Count(&running)
	if running > 0 {
		response.BizFail(c, "请先停止相关布控", 409)
		return
	}
	_ = h.DB.Model(&cam).Updates(map[string]any{"archived": 1, "enabled": 0, "updated_at": time.Now()}).Error
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.archive", &u, cam.Code)
	response.OK(c, nil, "已归档")
}

func (h *CameraHandler) upsertStreams(tx *gorm.DB, cameraID uint, mainURL, subURL string, bump bool) error {
	for _, item := range []struct{ role, url string }{{"main", mainURL}, {"sub", subURL}} {
		if item.url == "" && !bump {
			continue
		}
		var sp models.StreamProfile
		err := tx.Where("camera_id = ? AND role = ?", cameraID, item.role).First(&sp).Error
		if err == gorm.ErrRecordNotFound {
			if item.url == "" {
				continue
			}
			sp = models.StreamProfile{CameraID: cameraID, Role: item.role, URL: item.url, Protocol: "rtsp", Revision: 1, Enabled: 1}
			if err := tx.Create(&sp).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		upd := map[string]any{}
		if item.url != "" {
			upd["url"] = item.url
		}
		if bump {
			upd["revision"] = sp.Revision + 1
			upd["fov_changed"] = 1
		}
		if len(upd) > 0 {
			if err := tx.Model(&sp).Updates(upd).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// SyncFromNVR imports agent cameras that are not yet registered on the platform.
func (h *CameraHandler) SyncFromNVR(c *gin.Context) {
	agentID, _ := strconv.Atoi(c.Param("agentId"))
	var agent models.NVRAgent
	if h.DB.First(&agent, agentID).Error != nil {
		response.BizFail(c, "节点不存在", 404)
		return
	}
	type body struct {
		Cameras []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"cameras"`
		ProjectID *uint `json:"project_id"`
	}
	var req body
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Cameras) == 0 {
		response.BizFail(c, "请提供 cameras 列表", 422)
		return
	}
	created := 0
	for _, item := range req.Cameras {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		var n int64
		h.DB.Model(&models.Camera{}).Where("nvr_agent_id = ? AND agent_camera_id = ? AND archived = 0", agent.ID, id).Count(&n)
		if n > 0 {
			continue
		}
		code := fmt.Sprintf("nvr%d_%s", agent.ID, id)
		name := item.Name
		if name == "" {
			name = code
		}
		cam := models.Camera{
			Code: code, Name: name, ProjectID: req.ProjectID, AccessType: "nvr_agent",
			NVRAgentID: &agent.ID, AgentCameraID: id, Enabled: 1,
			DeviceStatus: "registered", MediaStatus: "unknown", AnalysisStatus: "idle",
		}
		if err := h.DB.Create(&cam).Error; err == nil {
			created++
		}
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.sync_nvr", &u, fmt.Sprintf("agent=%d created=%d", agent.ID, created))
	response.OK(c, gin.H{"created": created}, "同步完成")
}
