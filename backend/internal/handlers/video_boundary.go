package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
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

func (h *BoundaryHandler) Candidate(c *gin.Context) {
	camID, _ := strconv.Atoi(c.Query("camera_id"))
	w, _ := strconv.Atoi(c.DefaultQuery("width", "1280"))
	hgt, _ := strconv.Atoi(c.DefaultQuery("height", "720"))
	if camID < 1 {
		response.BizFail(c, "需要 camera_id", 422)
		return
	}
	g := videoai.SuggestBoundary(w, hgt)
	response.OK(c, gin.H{
		"camera_id": camID, "geometry": g, "ref_width": w, "ref_height": hgt,
		"demo": true, "note": "演示候选边界，需人工校准后发布",
	}, "")
}
