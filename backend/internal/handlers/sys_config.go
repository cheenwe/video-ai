package handlers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
)

type SysConfigHandler struct{ DB *gorm.DB }

type sysConfigCreateReq struct {
	Category string `json:"category" binding:"required"`
	Code     string `json:"code" binding:"required"`
	Value    string `json:"value"`
	Status   string `json:"status"`
	Version  string `json:"version"`
}

type sysConfigUpdateReq struct {
	Value   *string `json:"value"`
	Status  *string `json:"status"`
	Version *string `json:"version"`
}

func configOut(c models.SysConfig) gin.H {
	return gin.H{
		"id": c.ID, "category": c.Category, "code": c.Code, "value": c.Value,
		"status": c.Status, "version": c.Version, "updated_by": c.UpdatedBy, "updated_at": c.UpdatedAt,
	}
}

func (h *SysConfigHandler) List(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	q := h.DB.Model(&models.SysConfig{}).Where("category <> ?", "settings")
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("category LIKE ? OR code LIKE ? OR value LIKE ?", like, like, like)
	}
	if cat := c.Query("category"); cat != "" {
		q = q.Where("category = ?", cat)
	}
	var total int64
	q.Count(&total)
	var items []models.SysConfig
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, configOut(item))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

func (h *SysConfigHandler) GetPublicValue(c *gin.Context) {
	category := c.Query("category")
	code := c.Query("code")
	if category == "system" {
		if code != services.ConfigCodePageSize {
			response.BizFail(c, "无权限读取该配置", 403)
			return
		}
		response.OK(c, gin.H{
			"category": category,
			"code":     code,
			"value":    strconv.Itoa(services.PageSizeFromDB(h.DB)),
		}, "")
		return
	}
	// 白名单：与 lab_system 一致，可扩展
	if category != "site" && category != "inspection" {
		response.BizFail(c, "无权限读取该配置", 403)
		return
	}
	var cfg models.SysConfig
	if err := h.DB.Where("category = ? AND code = ? AND status = ?", category, code, "active").First(&cfg).Error; err != nil {
		response.BizFail(c, "配置不存在", 404)
		return
	}
	items := []string{}
	for _, p := range strings.Split(cfg.Value, ",") {
		if s := strings.TrimSpace(p); s != "" {
			items = append(items, s)
		}
	}
	response.OK(c, gin.H{"category": category, "code": code, "items": items}, "")
}

func (h *SysConfigHandler) Create(c *gin.Context) {
	var req sysConfigCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if strings.TrimSpace(req.Category) == "settings" {
		response.BizFail(c, "内置设置请使用专用设置页", 422)
		return
	}
	req.Category = strings.TrimSpace(req.Category)
	req.Code = strings.TrimSpace(req.Code)
	if req.Category == "" || req.Code == "" {
		response.BizFail(c, "分类与编码不能为空", 422)
		return
	}
	status := req.Status
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "disabled" {
		response.BizFail(c, "状态无效", 422)
		return
	}
	me := middleware.CurrentUser(c)
	uid := me.ID
	row := models.SysConfig{Category: req.Category, Code: req.Code, Value: req.Value, Status: status, Version: req.Version, UpdatedBy: &uid}
	if err := h.DB.Create(&row).Error; err != nil {
		response.BizFail(c, "配置已存在", 400)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "sys_config.create", &me, req.Category+"."+req.Code)
	response.OK(c, configOut(row), "创建成功")
}

func (h *SysConfigHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("cid"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var row models.SysConfig
	if err := h.DB.First(&row, id).Error; err != nil {
		response.BizFail(c, "配置不存在", 404)
		return
	}
	if row.Category == "settings" {
		response.BizFail(c, "内置设置不可直接修改", 403)
		return
	}
	var req sysConfigUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	updates := map[string]interface{}{}
	if req.Value != nil {
		updates["value"] = *req.Value
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "disabled" {
			response.BizFail(c, "状态无效", 422)
			return
		}
		updates["status"] = *req.Status
	}
	if req.Version != nil {
		updates["version"] = *req.Version
	}
	me := middleware.CurrentUser(c)
	updates["updated_by"] = me.ID
	if err := h.DB.Model(&row).Updates(updates).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	h.DB.First(&row, id)
	services.LogAction(h.DB, c, "audit", "success", "sys_config.update", &me, row.Category+"."+row.Code)
	response.OK(c, configOut(row), "更新成功")
}

func (h *SysConfigHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("cid"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var row models.SysConfig
	if err := h.DB.First(&row, id).Error; err != nil {
		response.BizFail(c, "配置不存在", 404)
		return
	}
	if row.Category == "settings" {
		response.BizFail(c, "内置设置不可直接删除", 403)
		return
	}
	if err := h.DB.Delete(&row).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "sys_config.delete", &me, row.Category+"."+row.Code)
	response.OK(c, nil, "已删除")
}
