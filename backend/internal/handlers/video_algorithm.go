package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
	"baseplatform/internal/videoai"

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
	if mv.Adapter == "onnx" {
		if info, err := os.Stat(mv.FilePath); err != nil || info.IsDir() || info.Size() == 0 {
			response.BizFail(c, "ONNX 模型文件不存在或为空："+mv.FilePath, 422)
			return
		}
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
	checks := []gin.H{}
	pass := true
	add := func(name string, ok bool, detail string) {
		checks = append(checks, gin.H{"name": name, "ok": ok, "detail": detail})
		if !ok {
			pass = false
		}
	}

	// Adapter must be registered with the runtime before a version can be used.
	if impl, err := videoai.AdapterByName(mv.Adapter); err != nil {
		add("adapter", false, err.Error())
	} else {
		add("adapter", true, mv.Adapter)
		add("demo", impl.Demo(), fmt.Sprintf("demo=%v", impl.Demo()))
	}

	if mv.Adapter == "mock" {
		add("file", true, "mock 适配器无模型文件")
		add("load", true, "mock load ok")
		add("sample_infer", true, "demo inference")
	} else {
		if mv.FilePath == "" {
			add("file", false, "未配置模型文件路径")
		} else {
			info, err := os.Stat(mv.FilePath)
			if err != nil {
				add("file", false, "模型文件不可访问: "+err.Error())
			} else if info.IsDir() {
				add("file", false, "模型文件路径是目录")
			} else {
				add("file", true, fmt.Sprintf("%d bytes", info.Size()))
			}
		}
		// Checksum verification when provided.
		if mv.FilePath != "" && mv.Checksum != "" {
			if sum, err := fileChecksum(mv.FilePath); err != nil {
				add("checksum", false, err.Error())
			} else if !strings.EqualFold(sum, mv.Checksum) {
				add("checksum", false, fmt.Sprintf("不匹配：期望 %s，实际 %s", mv.Checksum, sum))
			} else {
				add("checksum", true, sum)
			}
		}
		// Classes table must parse — real inference needs it to map outputs.
		var classes []string
		if mv.ClassesJSON == "" || json.Unmarshal([]byte(mv.ClassesJSON), &classes) != nil || len(classes) == 0 {
			add("classes", false, "类别表 classes_json 无效")
		} else {
			add("classes", true, strings.Join(classes, ","))
		}
		// Only mark load/sample_infer as pass when a real inferencer is wired.
		if impl, err := videoai.AdapterByName(mv.Adapter); err == nil && !impl.Demo() {
			add("load", false, "真实推理运行时（ONNX Runtime / GPU Worker）尚未接入，无法验证加载")
			add("sample_infer", false, "真实推理运行时尚未接入，无法样例推理")
			pass = false
		}
	}

	report := gin.H{
		"ok": pass, "adapter": mv.Adapter, "backend": mv.Backend,
		"demo":   mv.Adapter == "mock",
		"checks": checks,
	}
	if !pass {
		report["note"] = "自检未通过；真实模型需在接入 ONNX Runtime/Worker 后重新自检"
	}
	response.OK(c, report, "")
}

// fileChecksum returns the lowercase hex SHA-256 of a file.
func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func firstStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
