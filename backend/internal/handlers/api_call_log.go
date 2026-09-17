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

type ApiCallLogHandler struct{ DB *gorm.DB }

func apiCallLogOut(l models.ApiCallLog) gin.H {
	return gin.H{
		"id":          l.ID,
		"token_id":    l.TokenID,
		"user_id":     l.UserID,
		"method":      l.Method,
		"path":        l.Path,
		"status_code": l.StatusCode,
		"duration_ms": l.DurationMs,
		"ip":          l.IP,
		"user_agent":  l.UserAgent,
		"created_at":  l.CreatedAt,
	}
}

// applyApiCallFilters 应用通用筛选条件，返回带条件的查询。
func applyApiCallFilters(q *gorm.DB, c *gin.Context) *gorm.DB {
	if tid := c.Query("token_id"); tid != "" {
		q = q.Where("token_id = ?", tid)
	}
	if uid := c.Query("user_id"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if method := c.Query("method"); method != "" {
		q = q.Where("method = ?", method)
	}
	if status := c.Query("status_code"); status != "" {
		q = q.Where("status_code = ?", status)
	}
	if path := c.Query("path"); path != "" {
		q = q.Where("path LIKE ?", "%"+path+"%")
	}
	if ts := c.Query("time_start"); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if te := c.Query("time_end"); te != "" {
		if t, err := time.Parse(time.RFC3339, te); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}
	return q
}

// List 管理员查看全部 API 调用日志，支持多条件筛选。
func (h *ApiCallLogHandler) List(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	q := applyApiCallFilters(h.DB.Model(&models.ApiCallLog{}), c)
	var total int64
	q.Count(&total)
	var items []models.ApiCallLog
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, apiCallLogOut(item))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

// ListForToken 当前用户查看自己某个令牌的调用日志。
func (h *ApiCallLogHandler) ListForToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效令牌 ID", 400)
		return
	}
	u := middleware.CurrentUser(c)
	// 校验令牌归属
	var token models.PersonalAccessToken
	if err := h.DB.Where("id = ? AND user_id = ?", id, u.ID).First(&token).Error; err != nil {
		response.BizFail(c, "令牌不存在", 404)
		return
	}
	page, pageSize := services.ParsePagination(c, h.DB)
	q := h.DB.Model(&models.ApiCallLog{}).Where("token_id = ?", id)
	q = applyApiCallFilters(q, c)
	var total int64
	q.Count(&total)
	var items []models.ApiCallLog
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, apiCallLogOut(item))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}
