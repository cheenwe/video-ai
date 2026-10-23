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

	"github.com/gin-gonic/gin/binding"

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

type templateReq struct {
	Code           string   `json:"code" binding:"required"`
	Name           string   `json:"name" binding:"required"`
	ModelVersionID *uint    `json:"model_version_id"`
	RuleCodes      []string `json:"rule_codes"`
	Description    string   `json:"description"`
}

// templateUpdateReq is the partial-update shape for PUT: every field is
// optional so an operator can rebind a published model version (or swap rule
// codes) without resending the whole template.
type templateUpdateReq struct {
	Name           string   `json:"name"`
	ModelVersionID *uint    `json:"model_version_id"`
	RuleCodes      []string `json:"rule_codes"`
	Description    string   `json:"description"`
}

// UpdateTemplate rebinds a business template (model version, rules). Rebinding
// is how an operator points a template at a real published model after the
// seed's draft version.
func (h *AlgorithmHandler) UpdateTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var tpl models.BusinessTemplate
	if h.DB.First(&tpl, id).Error != nil {
		response.BizFail(c, "模板不存在", 404)
		return
	}
	var req templateUpdateReq
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if req.ModelVersionID != nil {
		var mv models.AlgoModelVersion
		if err := h.DB.First(&mv, *req.ModelVersionID).Error; err != nil {
			response.BizFail(c, "模型版本不存在", 422)
			return
		}
		if mv.Status != "published" {
			response.BizFail(c, "模型版本未发布，不能绑定", 422)
			return
		}
	}
	// ShouldBindBodyWith caches the body so we can also see which keys the
	// caller actually sent; a rebind-only PUT must not wipe other columns.
	var sent map[string]json.RawMessage
	_ = c.ShouldBindBodyWith(&sent, binding.JSON)
	_, descSent := sent["description"]
	upd := map[string]any{}
	if n := strings.TrimSpace(req.Name); n != "" {
		upd["name"] = n
	}
	if descSent {
		upd["description"] = req.Description
	}
	if req.ModelVersionID != nil {
		upd["model_version_id"] = *req.ModelVersionID
	}
	if len(upd) == 0 && len(req.RuleCodes) == 0 {
		response.BizFail(c, "没有需要更新的字段", 422)
		return
	}
	if len(req.RuleCodes) > 0 {
		codes, _ := json.Marshal(req.RuleCodes)
		upd["rule_codes_json"] = string(codes)
	}
	if err := h.DB.Model(&tpl).Updates(upd).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "algorithm.template.update", &u, tpl.Code)
	_ = h.DB.First(&tpl, id)
	response.OK(c, tpl, "已更新")
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
	adapter := strings.TrimSpace(req.Adapter)
	if adapter == "" {
		adapter = "onnx"
	}
	// The adapter must exist in the runtime; there is no demo fallback.
	if _, err := videoai.AdapterByName(adapter); err != nil {
		response.BizFail(c, "未知或未注册的推理适配器: "+adapter, 422)
		return
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
	// The adapter must be registered; publishing a version the runtime cannot
	// run would only surface later as a runtime fault.
	if _, err := videoai.AdapterByName(mv.Adapter); err != nil {
		response.BizFail(c, "推理适配器不可用: "+mv.Adapter, 422)
		return
	}
	if mv.FilePath == "" {
		response.BizFail(c, "真实模型必须配置文件路径", 422)
		return
	}
	if info, err := os.Stat(mv.FilePath); err != nil || info.IsDir() || info.Size() == 0 {
		response.BizFail(c, "模型文件不存在或为空："+mv.FilePath, 422)
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
	checks := []gin.H{}
	pass := true
	add := func(name string, ok bool, detail string) {
		checks = append(checks, gin.H{"name": name, "ok": ok, "detail": detail})
		if !ok {
			pass = false
		}
	}

	// Adapter must be registered with the runtime before a version can be used.
	impl, aerr := videoai.AdapterByName(mv.Adapter)
	if aerr != nil {
		add("adapter", false, aerr.Error())
	} else {
		add("adapter", true, mv.Adapter)
		if impl.Demo() {
			add("demo", false, "演示适配器不得用于生产推理")
		} else {
			add("demo", true, "真实推理适配器")
		}
	}

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
	// Load + sample inference go through the real adapter: it must boot the
	// worker with this model and run one frame end-to-end.
	if aerr != nil {
		add("load", false, "适配器不可用，跳过加载验证")
		add("sample_infer", false, "适配器不可用，跳过样例推理")
	} else if runner, ok := impl.(videoai.SelfCheckRunner); ok {
		res := runner.SelfCheck(videoai.ConfigForVersion(mv))
		add("load", res.LoadOK, res.LoadDetail)
		add("sample_infer", res.SampleOK, res.SampleDetail)
	} else {
		add("load", false, "适配器不支持加载自检")
		add("sample_infer", false, "适配器不支持样例推理自检")
	}

	report := gin.H{
		"ok": pass, "adapter": mv.Adapter, "backend": mv.Backend,
		"demo":   impl != nil && impl.Demo(),
		"checks": checks,
	}
	if !pass {
		report["note"] = "自检未通过；真实模型需可加载并完成样例推理后才能发布使用"
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
