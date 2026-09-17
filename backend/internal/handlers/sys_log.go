package handlers

import (
	"encoding/csv"
	"encoding/json"
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

type SysLogHandler struct{ DB *gorm.DB }

func logOut(l models.SysLog) gin.H {
	out := gin.H{
		"id": l.ID, "created_at": l.CreatedAt, "ip": l.IP,
		"user_id": l.UserID, "username": l.Username, "tag": l.Tag,
		"result": l.Result, "action": l.Action, "request_id": l.RequestID, "remark": l.Remark,
	}
	if l.Source != 0 {
		out["source"] = l.Source
		out["source_name"] = models.FileSourceName(l.Source)
	}
	return out
}

func (h *SysLogHandler) List(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	q := h.DB.Model(&models.SysLog{})
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("action LIKE ? OR username LIKE ? OR remark LIKE ?", like, like, like)
	}
	if uid := c.Query("user_id"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if tag := c.Query("tag"); tag != "" {
		q = q.Where("tag = ?", tag)
	}
	if category := c.Query("category"); category != "" {
		switch category {
		case "login":
			q = q.Where("tag = ?", "login")
		case "audit":
			q = q.Where("tag = ?", "audit")
		case "sync":
			q = q.Where("tag = ?", "sync")
		case "schedule":
			q = q.Where("tag = ?", "schedule")
		case "log":
			q = q.Where("tag NOT IN ?", []string{"login", "audit", "sync", "schedule"})
		}
	}
	if result := c.Query("result"); result != "" {
		switch result {
		case "ok":
			q = q.Where("result IN ?", []string{"ok", "success"})
		case "fail":
			q = q.Where("result IN ?", []string{"fail", "failed"})
		default:
			q = q.Where("result = ?", result)
		}
	}
	if src := c.Query("source"); src != "" {
		v := strings.ToLower(strings.TrimSpace(src))
		switch v {
		case "desktop":
			q = q.Where("source IN ?", models.FileSourceDesktopIDs())
		default:
			if id, err := parseFileSourceFilter(src); err == nil && id != 0 {
				q = q.Where("source = ?", id)
			}
		}
	}
	var total int64
	q.Count(&total)
	var items []models.SysLog
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, logOut(item))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

// parseJSON 安全解析 JSON 字符串为 map（失败返回 nil）。
func parseJSON(s string) map[string]interface{} {
	if s == "" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// Get 获取单条审计日志详情，包含变更前后数据。
func (h *SysLogHandler) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var l models.SysLog
	if err := h.DB.First(&l, id).Error; err != nil {
		response.BizFail(c, "日志不存在", 404)
		return
	}
	out := logOut(l)
	if before := parseJSON(l.BeforeData); before != nil {
		out["before"] = before
	}
	if after := parseJSON(l.AfterData); after != nil {
		out["after"] = after
	}
	response.OK(c, out, "")
}

// Export 导出审计日志为 CSV。支持 time_start / time_end / tag / result / keyword 筛选。
func (h *SysLogHandler) Export(c *gin.Context) {
	q := h.DB.Model(&models.SysLog{})
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("action LIKE ? OR username LIKE ? OR remark LIKE ?", like, like, like)
	}
	if tag := c.Query("tag"); tag != "" {
		q = q.Where("tag = ?", tag)
	}
	if result := c.Query("result"); result != "" {
		q = q.Where("result = ?", result)
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

	var items []models.SysLog
	q.Order("id desc").Limit(10000).Find(&items)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="audit-logs-%s.csv"`, time.Now().Format("20060102-150405")))
	c.Status(200)

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"ID", "时间", "用户", "IP", "标签", "动作", "结果", "备注"})
	for _, l := range items {
		_ = w.Write([]string{
			strconv.Itoa(int(l.ID)),
			l.CreatedAt.Format("2006-01-02 15:04:05"),
			l.Username,
			l.IP,
			l.Tag,
			l.Action,
			l.Result,
			l.Remark,
		})
	}
	w.Flush()
}

func parseFileSourceFilter(v string) (uint8, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if n, err := strconv.ParseUint(v, 10, 8); err == nil && n > 0 {
		return uint8(n), nil
	}
	if id := middleware.ParseNextFileSourceHeader(v); id != 0 {
		return id, nil
	}
	switch v {
	case "desktop":
		return models.FileSourceDesktop, nil
	case "desktop-linux", "desktop_linux":
		return models.FileSourceDesktopLinux, nil
	case "desktop-darwin", "desktop_darwin", "desktop-macos", "desktop_macos":
		return models.FileSourceDesktopDarwin, nil
	case "desktop-windows", "desktop_windows":
		return models.FileSourceDesktopWindows, nil
	case "smb":
		return models.FileSourceSMB, nil
	case "webdav":
		return models.FileSourceWebDAV, nil
	case "web":
		return models.FileSourceWeb, nil
	case "api":
		return models.FileSourceAPI, nil
	}
	return 0, fmt.Errorf("unknown source")
}
