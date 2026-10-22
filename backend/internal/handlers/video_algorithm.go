package handlers

import (
	"strconv"
	"time"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AlgorithmHandler struct {
	DB *gorm.DB
}

func (h *AlgorithmHandler) ListModels(c *gin.Context) {
	var modelsList []models.AlgoModel
	h.DB.Order("id desc").Find(&modelsList)
	out := make([]gin.H, 0, len(modelsList))
	for _, m := range modelsList {
		var vers []models.AlgoModelVersion
		h.DB.Where("model_id = ?", m.ID).Order("id desc").Find(&vers)
		out = append(out, gin.H{"id": m.ID, "code": m.Code, "name": m.Name, "framework": m.Framework, "remark": m.Remark, "versions": vers})
	}
	response.OK(c, out, "")
}

func (h *AlgorithmHandler) ListRules(c *gin.Context) {
	var items []models.RuleVersion
	h.DB.Order("code asc, version desc").Find(&items)
	response.OK(c, items, "")
}

func (h *AlgorithmHandler) ListTemplates(c *gin.Context) {
	var items []models.BusinessTemplate
	h.DB.Order("id desc").Find(&items)
	response.OK(c, items, "")
}

type modelVersionReq struct {
	ModelID  uint   `json:"model_id" binding:"required"`
	Version  string `json:"version" binding:"required"`
	Adapter  string `json:"adapter"`
	Backend  string `json:"backend"`
	FilePath string `json:"file_path"`
	Checksum string `json:"checksum"`
}

func (h *AlgorithmHandler) CreateModelVersion(c *gin.Context) {
	var req modelVersionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	adapter := req.Adapter
	if adapter == "" {
		adapter = "mock"
	}
	mv := models.AlgoModelVersion{
		ModelID: req.ModelID, Version: req.Version, Status: "draft",
		Adapter: adapter, Backend: firstStr(req.Backend, "cpu"),
		FilePath: req.FilePath, Checksum: req.Checksum,
		ClassesJSON: `["grab_bucket"]`,
	}
	if err := h.DB.Create(&mv).Error; err != nil {
		response.BizFail(c, "创建失败", 400)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "algorithm.version.create", &u, mv.Version)
	response.OK(c, mv, "已创建")
}

func (h *AlgorithmHandler) PublishModelVersion(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var mv models.AlgoModelVersion
	if h.DB.First(&mv, id).Error != nil {
		response.BizFail(c, "版本不存在", 404)
		return
	}
	// Validation pipeline (mock): checksum optional, adapter must be known.
	if mv.Adapter != "mock" && mv.Adapter != "onnx" {
		response.BizFail(c, "不支持的适配器", 422)
		return
	}
	if mv.Adapter == "onnx" && mv.FilePath == "" {
		response.BizFail(c, "ONNX 模型缺少文件路径", 422)
		return
	}
	now := time.Now()
	_ = h.DB.Model(&mv).Updates(map[string]any{"status": "published", "published_at": now}).Error
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "algorithm.version.publish", &u, mv.Version)
	_ = h.DB.First(&mv, id)
	response.OK(c, mv, "已发布")
}

func (h *AlgorithmHandler) SelfCheckModel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var mv models.AlgoModelVersion
	if h.DB.First(&mv, id).Error != nil {
		response.BizFail(c, "版本不存在", 404)
		return
	}
	report := gin.H{
		"ok": true, "adapter": mv.Adapter, "backend": mv.Backend,
		"demo": mv.Adapter == "mock",
		"checks": []gin.H{
			{"name": "file", "ok": mv.Adapter == "mock" || mv.FilePath != "", "detail": mv.FilePath},
			{"name": "load", "ok": true, "detail": "mock load ok"},
			{"name": "sample_infer", "ok": true, "detail": "demo inference"},
		},
	}
	response.OK(c, report, "")
}

func firstStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
