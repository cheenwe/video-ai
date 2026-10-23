package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

type EventHandler struct {
	DB   *gorm.DB
	RBAC *services.RBACService
}

func (h *EventHandler) List(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	u := middleware.CurrentUser(c)
	q := h.RBAC.ApplyDataScope(h.DB.Model(&models.VideoEvent{}), u, services.ScopeOptions{
		ProjectIDColumn: "project_id",
	})
	if cam := c.Query("camera_id"); cam != "" {
		q = q.Where("camera_id = ?", cam)
	}
	if st := c.Query("handle_status"); st != "" {
		q = q.Where("handle_status = ?", st)
	}
	if lv := c.Query("level"); lv != "" {
		q = q.Where("level = ?", lv)
	}
	if cat := c.Query("category"); cat != "" {
		q = q.Where("category = ?", cat)
	}
	if from := c.Query("from"); from != "" {
		q = q.Where("started_at >= ?", from)
	}
	if to := c.Query("to"); to != "" {
		q = q.Where("started_at <= ?", to)
	}
	var total int64
	q.Count(&total)
	var items []models.VideoEvent
	q.Order("started_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	response.OK(c, services.PaginateResult(items, total, page, pageSize), "")
}

func (h *EventHandler) Get(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var ev models.VideoEvent
	if h.DB.First(&ev, id).Error != nil {
		response.BizFail(c, "事件不存在", 404)
		return
	}
	var actions []models.EventAction
	h.DB.Where("event_id = ?", ev.ID).Order("id asc").Find(&actions)
	var evid []models.Evidence
	h.DB.Where("event_id = ?", ev.ID).Order("id asc").Find(&evid)
	response.OK(c, gin.H{"event": ev, "actions": actions, "evidences": evid}, "")
}

type eventActionReq struct {
	Action     string `json:"action" binding:"required"` // confirm|process|close|false_positive
	Conclusion string `json:"conclusion"`
	Remark     string `json:"remark"`
}

func (h *EventHandler) Act(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var ev models.VideoEvent
	if h.DB.First(&ev, id).Error != nil {
		response.BizFail(c, "事件不存在", 404)
		return
	}
	var req eventActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	from := ev.HandleStatus
	to := from
	switch req.Action {
	case "confirm":
		to = "confirmed"
	case "process":
		to = "processing"
	case "close":
		if strings.TrimSpace(req.Conclusion) == "" || strings.TrimSpace(req.Remark) == "" {
			response.BizFail(c, "关闭时必须填写结论和说明", 422)
			return
		}
		to = "closed"
		ev.Conclusion = req.Conclusion
		ev.ConclusionRemark = req.Remark
	case "false_positive":
		to = "closed"
		ev.Conclusion = "false_positive"
		ev.ConclusionRemark = req.Remark
	default:
		response.BizFail(c, "未知动作", 422)
		return
	}
	// Closing must not change scene_status (algorithm truth).
	ev.HandleStatus = to
	if err := h.DB.Save(&ev).Error; err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	_ = h.DB.Create(&models.EventAction{
		EventID: ev.ID, Action: req.Action, FromStatus: from, ToStatus: to,
		Remark: req.Remark, ActorID: &u.ID,
	}).Error
	services.LogAction(h.DB, c, "audit", "success", "event."+req.Action, &u, fmt.Sprintf("%d", ev.ID))
	response.OK(c, ev, "已处理")
}

func (h *EventHandler) BatchConfirm(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		response.BizFail(c, "参数错误", 422)
		return
	}
	u := middleware.CurrentUser(c)
	results := make([]gin.H, 0, len(req.IDs))
	for _, id := range req.IDs {
		var ev models.VideoEvent
		if h.DB.First(&ev, id).Error != nil {
			results = append(results, gin.H{"id": id, "ok": false, "error": "not found"})
			continue
		}
		from := ev.HandleStatus
		ev.HandleStatus = "confirmed"
		_ = h.DB.Save(&ev).Error
		_ = h.DB.Create(&models.EventAction{
			EventID: ev.ID, Action: "confirm", FromStatus: from, ToStatus: "confirmed", ActorID: &u.ID,
		}).Error
		results = append(results, gin.H{"id": id, "ok": true})
	}
	services.LogAction(h.DB, c, "audit", "success", "event.batch_confirm", &u, fmt.Sprintf("n=%d", len(req.IDs)))
	response.OK(c, results, "")
}

// DownloadEvidence serves a ready evidence file (overlay / screenshot / clip).
// Clips lists recent events for a camera together with their evidences, so the
// recording centre can show event clips beside continuous recordings.
func (h *EventHandler) Clips(c *gin.Context) {
	cameraID, _ := strconv.Atoi(c.Param("id"))
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	var evs []models.VideoEvent
	h.DB.Where("camera_id = ?", cameraID).Order("started_at desc").Limit(limit).Find(&evs)

	ids := make([]uint, 0, len(evs))
	for _, e := range evs {
		ids = append(ids, e.ID)
	}
	byEvent := map[uint][]models.Evidence{}
	if len(ids) > 0 {
		var evid []models.Evidence
		h.DB.Where("event_id IN ?", ids).Order("id asc").Find(&evid)
		for _, e := range evid {
			byEvent[e.EventID] = append(byEvent[e.EventID], e)
		}
	}
	out := make([]gin.H, 0, len(evs))
	for _, e := range evs {
		out = append(out, gin.H{"event": e, "evidences": byEvent[e.ID]})
	}
	response.OK(c, out, "")
}

func (h *EventHandler) DownloadEvidence(c *gin.Context) {
	eventID, _ := strconv.Atoi(c.Param("id"))
	evidID, _ := strconv.Atoi(c.Param("eid"))
	var ev models.VideoEvent
	if h.DB.First(&ev, eventID).Error != nil {
		response.BizFail(c, "事件不存在", 404)
		return
	}
	var e models.Evidence
	if h.DB.Where("id = ? AND event_id = ?", evidID, eventID).First(&e).Error != nil {
		response.BizFail(c, "证据不存在", 404)
		return
	}
	if e.Status == "expired" {
		response.BizFail(c, "证据已过期", 410)
		return
	}
	if e.Status != "ready" || e.Path == "" {
		response.BizFail(c, "证据尚未就绪", 409)
		return
	}
	if _, err := os.Stat(e.Path); err != nil {
		response.BizFail(c, "证据文件不可用", 404)
		return
	}
	c.Header("Content-Disposition", "inline; filename="+filepath.Base(e.Path))
	http.ServeFile(c.Writer, c.Request, e.Path)
}

// CleanupExpiredEvidence marks old evidence expired (scheduler builtin helper).
func CleanupExpiredEvidence(db *gorm.DB, retainDays int) (int64, error) {
	if retainDays < 1 {
		retainDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -retainDays)
	res := db.Model(&models.Evidence{}).
		Where("created_at < ? AND status = ?", cutoff, "ready").
		Updates(map[string]any{"status": "expired"})
	return res.RowsAffected, res.Error
}
