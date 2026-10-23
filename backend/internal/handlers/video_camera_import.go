package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
)

// importRow is one line of a bulk camera import.
type importRow struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	AccessType    string  `json:"access_type"`
	Vendor        string  `json:"vendor"`
	Model         string  `json:"model"`
	Remark        string  `json:"remark"`
	ProjectID     *uint   `json:"project_id"`
	GroupID       *uint   `json:"group_id"`
	NVRAgentID    *uint   `json:"nvr_agent_id"`
	AgentCameraID string  `json:"agent_camera_id"`
	MainURL       string  `json:"main_url"`
	SubURL        string  `json:"sub_url"`
	Enabled       *int    `json:"enabled"`
}

type importReq struct {
	ProjectID *uint       `json:"project_id"`
	GroupID   *uint       `json:"group_id"`
	Rows      []importRow `json:"rows" binding:"required,min=1"`
}

// rowResult reports a single row so the UI can show exactly which lines
// failed and why, instead of a generic "import failed".
type rowResult struct {
	Index int    `json:"index"`
	Code  string `json:"code"`
	OK    bool   `json:"ok"`
	ID    uint   `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

// ImportCameras creates cameras in bulk, continuing past bad rows.
func (h *CameraHandler) ImportCameras(c *gin.Context) {
	var req importReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误：需要 rows 数组", http.StatusUnprocessableEntity)
		return
	}
	if len(req.Rows) > 500 {
		response.BizFail(c, "单次导入最多 500 行", http.StatusUnprocessableEntity)
		return
	}

	results := make([]rowResult, 0, len(req.Rows))
	created := 0
	for i, row := range req.Rows {
		res := h.importOne(row, req.ProjectID, req.GroupID)
		res.Index = i + 1
		if res.OK {
			created++
		}
		results = append(results, res)
	}

	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "camera.import", &u,
		fmt.Sprintf("rows=%d created=%d failed=%d", len(req.Rows), created, len(req.Rows)-created))

	response.OK(c, gin.H{
		"total":   len(req.Rows),
		"created": created,
		"failed":  len(req.Rows) - created,
		"results": results,
	}, fmt.Sprintf("导入完成：成功 %d，失败 %d", created, len(req.Rows)-created))
}

func (h *CameraHandler) importOne(row importRow, defaultProject, defaultGroup *uint) rowResult {
	code := strings.TrimSpace(row.Code)
	name := strings.TrimSpace(row.Name)
	res := rowResult{Code: code}
	if code == "" || name == "" {
		res.Error = "编码与名称不能为空"
		return res
	}
	at := strings.TrimSpace(row.AccessType)
	if at == "" {
		at = "rtsp"
	}
	switch at {
	case "nvr_agent", "rtsp", "gb28181", "push":
	default:
		res.Error = "接入方式无效：" + at
		return res
	}
	mainURL := strings.TrimSpace(row.MainURL)
	subURL := strings.TrimSpace(row.SubURL)
	agentID := strings.TrimSpace(row.AgentCameraID)
	if mainURL == "" && subURL == "" && (at != "nvr_agent" || agentID == "") {
		res.Error = "缺少码流地址或 NVR 通道 ID"
		return res
	}
	if at == "nvr_agent" && (row.NVRAgentID == nil || agentID == "") {
		res.Error = "nvr_agent 接入需要 nvr_agent_id 与 agent_camera_id"
		return res
	}

	projectID := row.ProjectID
	if projectID == nil {
		projectID = defaultProject
	}
	groupID := row.GroupID
	if groupID == nil {
		groupID = defaultGroup
	}

	var dup int64
	h.DB.Model(&models.Camera{}).Where("code = ?", code).Count(&dup)
	if dup > 0 {
		res.Error = "编码已存在"
		return res
	}

	enabled := 1
	if row.Enabled != nil && *row.Enabled == 0 {
		enabled = 0
	}
	cam := models.Camera{
		Code: code, Name: name, ProjectID: projectID, GroupID: groupID,
		AccessType: at, Vendor: row.Vendor, Model: row.Model, Remark: row.Remark,
		Enabled: enabled, NVRAgentID: row.NVRAgentID, AgentCameraID: agentID,
		DeviceStatus: "registered", MediaStatus: "unknown", AnalysisStatus: "idle",
	}
	if err := h.DB.Create(&cam).Error; err != nil {
		res.Error = "创建失败：" + err.Error()
		return res
	}
	streams := []models.StreamProfile{}
	if mainURL != "" {
		streams = append(streams, models.StreamProfile{CameraID: cam.ID, Role: "main", URL: mainURL, Protocol: "rtsp", Revision: 1, Enabled: 1})
	}
	if subURL != "" {
		streams = append(streams, models.StreamProfile{CameraID: cam.ID, Role: "sub", URL: subURL, Protocol: "rtsp", Revision: 1, Enabled: 1})
	}
	for i := range streams {
		if err := h.DB.Create(&streams[i]).Error; err != nil {
			res.Error = "摄像头已创建，但码流保存失败：" + err.Error()
			res.OK = true
			res.ID = cam.ID
			return res
		}
	}
	res.OK = true
	res.ID = cam.ID
	return res
}
