package handlers

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
)

// ---------- 站内通知（当前用户） ----------

type NotificationHandler struct {
	DB  *gorm.DB
	Svc *services.NotificationService
}

func (h *NotificationHandler) List(c *gin.Context) {
	user := middleware.CurrentUser(c)
	page, pageSize := services.ParsePagination(c, h.DB)
	q := h.DB.Model(&models.Notification{}).Where("user_id = ?", user.ID)
	if r := c.Query("is_read"); r != "" {
		if r == "1" || r == "true" {
			q = q.Where("is_read = ?", 1)
		} else {
			q = q.Where("is_read = ?", 0)
		}
	}
	var total int64
	q.Count(&total)
	var items []models.Notification
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	response.OK(c, services.PaginateResult(items, total, page, pageSize), "")
}

func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	user := middleware.CurrentUser(c)
	var cnt int64
	h.DB.Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", user.ID, 0).Count(&cnt)
	response.OK(c, gin.H{"count": cnt}, "")
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	user := middleware.CurrentUser(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	now := time.Now()
	res := h.DB.Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, user.ID).
		Updates(map[string]interface{}{"is_read": 1, "read_at": now})
	if res.RowsAffected == 0 {
		response.BizFail(c, "通知不存在", 404)
		return
	}
	response.OK(c, nil, "已标记为已读")
}

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	user := middleware.CurrentUser(c)
	now := time.Now()
	h.DB.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ?", user.ID, 0).
		Updates(map[string]interface{}{"is_read": 1, "read_at": now})
	response.OK(c, nil, "已全部标记为已读")
}

func (h *NotificationHandler) Delete(c *gin.Context) {
	user := middleware.CurrentUser(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	res := h.DB.Where("id = ? AND user_id = ?", id, user.ID).Delete(&models.Notification{})
	if res.RowsAffected == 0 {
		response.BizFail(c, "通知不存在", 404)
		return
	}
	response.OK(c, nil, "已删除")
}

// ---------- Webhook 端点管理 ----------

type WebhookHandler struct {
	DB *gorm.DB
}

func (h *WebhookHandler) List(c *gin.Context) {
	var items []models.WebhookEndpoint
	h.DB.Order("id desc").Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, ep := range items {
		out = append(out, gin.H{
			"id": ep.ID, "name": ep.Name, "url": ep.URL,
			"event_types": splitTypes(ep.EventTypes),
			"enabled":     ep.Enabled, "timeout_sec": ep.TimeoutSec,
			"max_retries": ep.MaxRetries,
			"created_at":  ep.CreatedAt, "updated_at": ep.UpdatedAt,
		})
	}
	response.OK(c, gin.H{"items": out}, "")
}

func (h *WebhookHandler) Create(c *gin.Context) {
	var req struct {
		Name       string   `json:"name" binding:"required"`
		URL        string   `json:"url" binding:"required"`
		Secret     string   `json:"secret"`
		EventTypes []string `json:"event_types"`
		Enabled    *int     `json:"enabled"`
		TimeoutSec int      `json:"timeout_sec"`
		MaxRetries *int     `json:"max_retries"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	enabled := 1
	if req.Enabled != nil {
		if *req.Enabled != 0 && *req.Enabled != 1 {
			response.BizFail(c, "启用状态无效", 422)
			return
		}
		enabled = *req.Enabled
	}
	timeout := req.TimeoutSec
	if timeout <= 0 {
		timeout = 10
	}
	maxRetries := 5
	if req.MaxRetries != nil {
		maxRetries = *req.MaxRetries
	}
	if maxRetries < 0 || maxRetries > 5 || timeout > 30 {
		response.BizFail(c, "重试次数应为0-5，超时应为1-30秒", 422)
		return
	}
	if !validWebhookURL(req.URL) || strings.TrimSpace(req.Name) == "" {
		response.BizFail(c, "名称或HTTP(S)地址无效", 422)
		return
	}
	ep := models.WebhookEndpoint{
		Name:       strings.TrimSpace(req.Name),
		URL:        strings.TrimSpace(req.URL),
		Secret:     req.Secret,
		EventTypes: strings.Join(req.EventTypes, ","),
		Enabled:    enabled,
		TimeoutSec: timeout,
		MaxRetries: maxRetries,
	}
	if err := h.DB.Create(&ep).Error; err != nil {
		response.BizFail(c, "创建失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "webhook.create", &me, ep.Name)
	response.OK(c, gin.H{"id": ep.ID}, "创建成功")
}

func (h *WebhookHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var ep models.WebhookEndpoint
	if err := h.DB.First(&ep, id).Error; err != nil {
		response.BizFail(c, "Webhook 不存在", 404)
		return
	}
	var req struct {
		Name       *string  `json:"name"`
		URL        *string  `json:"url"`
		Secret     *string  `json:"secret"`
		EventTypes []string `json:"event_types"`
		Enabled    *int     `json:"enabled"`
		TimeoutSec *int     `json:"timeout_sec"`
		MaxRetries *int     `json:"max_retries"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.URL != nil {
		if !validWebhookURL(*req.URL) {
			response.BizFail(c, "HTTP(S)地址无效", 422)
			return
		}
		updates["url"] = strings.TrimSpace(*req.URL)
	}
	if req.Secret != nil {
		updates["secret"] = *req.Secret
	}
	if req.EventTypes != nil {
		updates["event_types"] = strings.Join(req.EventTypes, ",")
	}
	if req.Enabled != nil {
		if *req.Enabled != 0 && *req.Enabled != 1 {
			response.BizFail(c, "启用状态无效", 422)
			return
		}
		updates["enabled"] = *req.Enabled
	}
	if req.TimeoutSec != nil {
		if *req.TimeoutSec < 1 || *req.TimeoutSec > 30 {
			response.BizFail(c, "超时应为1-30秒", 422)
			return
		}
		updates["timeout_sec"] = *req.TimeoutSec
	}
	if req.MaxRetries != nil {
		if *req.MaxRetries < 0 || *req.MaxRetries > 5 {
			response.BizFail(c, "重试次数应为0-5", 422)
			return
		}
		updates["max_retries"] = *req.MaxRetries
	}
	if len(updates) > 0 {
		if h.DB.Model(&ep).Updates(updates).Error != nil {
			response.BizFail(c, "更新失败", 500)
			return
		}
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "webhook.update", &me, ep.Name)
	response.OK(c, nil, "更新成功")
}

func (h *WebhookHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var ep models.WebhookEndpoint
	if err := h.DB.First(&ep, id).Error; err != nil {
		response.BizFail(c, "Webhook 不存在", 404)
		return
	}
	if h.DB.Delete(&ep).Error != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "webhook.delete", &me, ep.Name)
	response.OK(c, nil, "已删除")
}

// Deliveries 查询某端点的投递记录。
func (h *WebhookHandler) Deliveries(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	page, pageSize := services.ParsePagination(c, h.DB)
	q := h.DB.Model(&models.WebhookDelivery{}).Where("endpoint_id = ?", id)
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	q.Count(&total)
	var items []models.WebhookDelivery
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	response.OK(c, services.PaginateResult(items, total, page, pageSize), "")
}

// Retry 手动重试一条失败的投递。
func (h *WebhookHandler) Retry(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效投递 ID", 400)
		return
	}
	var rec models.WebhookDelivery
	if err := h.DB.First(&rec, id).Error; err != nil {
		response.BizFail(c, "投递记录不存在", 404)
		return
	}
	var ep models.WebhookEndpoint
	if err := h.DB.First(&ep, rec.EndpointID).Error; err != nil {
		response.BizFail(c, "端点不存在", 404)
		return
	}
	svc := services.NewNotificationService(h.DB, nil)
	if ep.Enabled == 0 {
		response.BizFail(c, "端点已停用", 409)
		return
	}
	if err := svc.Retry(rec.ID); err != nil {
		response.BizFail(c, err.Error(), 409)
		return
	}
	response.OK(c, nil, "已加入重试队列")
}

func splitTypes(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	out := []string{}
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func validWebhookURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
