package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
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

type BoundaryHandler struct {
	DB *gorm.DB
	// Runtime is optional; when set, publishing a boundary invalidates the
	// cached geometry so analysis picks it up on the next tick.
	Runtime *videoai.Manager
}

func (h *BoundaryHandler) List(c *gin.Context) {
	camID, _ := strconv.Atoi(c.Query("camera_id"))
	q := h.DB.Model(&models.BoundaryVersion{})
	if camID > 0 {
		q = q.Where("camera_id = ?", camID)
	}
	var items []models.BoundaryVersion
	q.Order("camera_id asc, version desc").Limit(200).Find(&items)
	response.OK(c, items, "")
}

type boundaryReq struct {
	CameraID  uint            `json:"camera_id" binding:"required"`
	Name      string          `json:"name"`
	Geometry  json.RawMessage `json:"geometry" binding:"required"`
	RefWidth  int             `json:"ref_width"`
	RefHeight int             `json:"ref_height"`
	StreamRev int             `json:"stream_rev"`
	Source    string          `json:"source"`
}

func (h *BoundaryHandler) Create(c *gin.Context) {
	var req boundaryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	g, err := videoai.ParseGeometry(string(req.Geometry))
	if err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	if err := videoai.ValidateGeometry(g, req.RefWidth, req.RefHeight); err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	var cam models.Camera
	if h.DB.First(&cam, req.CameraID).Error != nil || cam.Archived == 1 {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}
	var maxVer int
	h.DB.Model(&models.BoundaryVersion{}).Where("camera_id = ?", req.CameraID).
		Select("COALESCE(MAX(version),0)").Scan(&maxVer)
	u := middleware.CurrentUser(c)
	src := req.Source
	if src == "" {
		src = "manual"
	}
	bv := models.BoundaryVersion{
		CameraID: req.CameraID, Version: maxVer + 1, Status: "draft", Name: req.Name,
		GeometryJSON: string(req.Geometry), RefWidth: req.RefWidth, RefHeight: req.RefHeight,
		StreamRev: req.StreamRev, Source: src, CreatedBy: &u.ID,
	}
	if err := h.DB.Create(&bv).Error; err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "boundary.create", &u, fmt.Sprintf("cam=%d v%d", bv.CameraID, bv.Version))
	response.OK(c, bv, "已创建草稿")
}

func (h *BoundaryHandler) Publish(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var bv models.BoundaryVersion
	if h.DB.First(&bv, id).Error != nil {
		response.BizFail(c, "版本不存在", 404)
		return
	}
	g, err := videoai.ParseGeometry(bv.GeometryJSON)
	if err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	if err := videoai.ValidateGeometry(g, bv.RefWidth, bv.RefHeight); err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	u := middleware.CurrentUser(c)
	now := time.Now()
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.BoundaryVersion{}).
			Where("camera_id = ? AND status = ? AND id <> ?", bv.CameraID, "published", bv.ID).
			Updates(map[string]any{"status": "archived"}).Error; err != nil {
			return err
		}
		return tx.Model(&bv).Updates(map[string]any{
			"status": "published", "published_at": now, "published_by": u.ID, "invalid_reason": "",
		}).Error
	})
	if err != nil {
		response.BizFail(c, "发布失败", 500)
		return
	}
	if h.Runtime != nil {
		h.Runtime.InvalidateBoundary(bv.ID)
	}
	_ = h.DB.First(&bv, id)
	services.LogAction(h.DB, c, "audit", "success", "boundary.publish", &u, fmt.Sprintf("id=%d", id))
	response.OK(c, bv, "已发布")
}

func (h *BoundaryHandler) Rollback(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var src models.BoundaryVersion
	if h.DB.First(&src, id).Error != nil {
		response.BizFail(c, "版本不存在", 404)
		return
	}
	var maxVer int
	h.DB.Model(&models.BoundaryVersion{}).Where("camera_id = ?", src.CameraID).
		Select("COALESCE(MAX(version),0)").Scan(&maxVer)
	u := middleware.CurrentUser(c)
	nv := models.BoundaryVersion{
		CameraID: src.CameraID, Version: maxVer + 1, Status: "draft",
		Name:         "rollback-from-v" + strconv.Itoa(src.Version),
		GeometryJSON: src.GeometryJSON, RefWidth: src.RefWidth, RefHeight: src.RefHeight,
		StreamRev: src.StreamRev, Source: "manual", CreatedBy: &u.ID,
	}
	if err := h.DB.Create(&nv).Error; err != nil {
		response.BizFail(c, "回滚失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "boundary.rollback", &u, fmt.Sprintf("from=%d to=%d", src.ID, nv.ID))
	response.OK(c, nv, "已生成回滚草稿，请重新发布")
}

// Candidate suggests a pit-edge polyline from one real live frame. It runs the
// published model on a fresh frame and derives geometry only from detections
// the model calls "edge" — a bucket-only model produces an explicit 422 rather
// than an invented line. The result is a draft suggestion: the operator still
// reviews and posts it through the normal create endpoint.
func (h *BoundaryHandler) Candidate(c *gin.Context) {
	camID, _ := strconv.Atoi(c.Query("camera_id"))
	if camID < 1 {
		response.BizFail(c, "需要 camera_id", 422)
		return
	}
	var cam models.Camera
	if h.DB.First(&cam, camID).Error != nil || cam.Archived == 1 {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}
	if h.Runtime == nil {
		response.BizFail(c, "分析运行时未启用，无法取帧推理", http.StatusServiceUnavailable)
		return
	}
	frame, ok := h.Runtime.FrameFor(cam.ID)
	if !ok {
		response.BizFail(c, "暂无新鲜视频帧（码流未配置或取流未连通），自动候选需要实时画面；请先确认预览正常或手动绘制", http.StatusServiceUnavailable)
		return
	}

	mv, err := h.candidateModelVersion(c)
	if err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	impl, err := videoai.AdapterByName(mv.Adapter)
	if err != nil {
		response.BizFail(c, err.Error(), http.StatusServiceUnavailable)
		return
	}
	cfg := videoai.ConfigForVersion(mv)
	cfg.Classes = nil
	if mv.ClassesJSON != "" {
		_ = json.Unmarshal([]byte(mv.ClassesJSON), &cfg.Classes)
	}
	dets, err := impl.Infer(cam.ID, videoai.Frame{TS: frame.TS, JPEG: frame.JPEG, W: frame.W, H: frame.H}, cfg)
	if err != nil {
		// Worker missing / model failed: honest 503, never a fallback line.
		response.BizFail(c, "推理失败，无法生成候选："+err.Error(), http.StatusServiceUnavailable)
		return
	}
	cand, err := videoai.DeriveBoundary(dets, frame.W, frame.H)
	if err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	geom, _ := json.Marshal(cand.Geometry)
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "boundary.candidate", &u,
		fmt.Sprintf("cam=%d edge=%d conf=%.2f", cam.ID, cand.EdgeDetections, cand.Confidence))
	response.OK(c, gin.H{
		"geometry":         json.RawMessage(geom),
		"ref_width":        frame.W,
		"ref_height":       frame.H,
		"model_version_id": mv.ID,
		"edge_detections":  cand.EdgeDetections,
		"confidence":       cand.Confidence,
		"note":             "候选由当前帧推理生成，需人工确认后再保存为边界版本",
	}, "")
}

// candidateModelVersion resolves the model to run: an explicit
// model_version_id wins, otherwise the published version bound to the first
// published business template. Only published versions are candidates — a
// draft has no proven output.
func (h *BoundaryHandler) candidateModelVersion(c *gin.Context) (models.AlgoModelVersion, error) {
	var mv models.AlgoModelVersion
	if raw := c.Query("model_version_id"); raw != "" {
		id, _ := strconv.Atoi(raw)
		if h.DB.First(&mv, id).Error != nil {
			return mv, fmt.Errorf("模型版本不存在")
		}
		if mv.Status != "published" {
			return mv, fmt.Errorf("模型版本未发布，不能用于生成候选")
		}
		return mv, nil
	}
	// Walk published templates and take the first whose version is also
	// published. The seeded template binds a draft (no model file yet), so
	// stopping at the first template would report "unpublished" even when a
	// usable model exists elsewhere.
	var tpls []models.BusinessTemplate
	h.DB.Where("status = ? AND model_version_id IS NOT NULL", "published").
		Order("id asc").Find(&tpls)
	if len(tpls) == 0 {
		return mv, fmt.Errorf("没有可用的业务模板；请先发布绑定模型的模板，或用 model_version_id 指定模型版本")
	}
	var unpublished []string
	for _, tpl := range tpls {
		var cand models.AlgoModelVersion
		if err := h.DB.First(&cand, *tpl.ModelVersionID).Error; err != nil {
			continue
		}
		if cand.Status == "published" {
			return cand, nil
		}
		unpublished = append(unpublished, tpl.Code+"→"+cand.Version)
	}
	return mv, fmt.Errorf("已发布的业务模板都没有绑定已发布的模型版本（%s）；请先发布模型版本，或用 model_version_id 指定",
		strings.Join(unpublished, "、"))
}
