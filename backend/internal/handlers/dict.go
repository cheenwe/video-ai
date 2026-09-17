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

type DictHandler struct{ DB *gorm.DB }

// ---------- 字典类型 ----------

type dictCreateReq struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
	Enabled     *int   `json:"enabled"`
}

type dictUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	SortOrder   *int    `json:"sort_order"`
	Enabled     *int    `json:"enabled"`
}

func (h *DictHandler) ListDicts(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	q := h.DB.Model(&models.SysDict{})
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("code LIKE ? OR name LIKE ?", like, like)
	}
	var total int64
	q.Count(&total)
	var items []models.SysDict
	q.Order("sort_order asc, id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	response.OK(c, services.PaginateResult(items, total, page, pageSize), "")
}

func (h *DictHandler) GetDict(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var d models.SysDict
	if err := h.DB.First(&d, id).Error; err != nil {
		response.BizFail(c, "字典不存在", 404)
		return
	}
	response.OK(c, d, "")
}

func (h *DictHandler) CreateDict(c *gin.Context) {
	var req dictCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" {
		response.BizFail(c, "编码与名称不能为空", 422)
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
	d := models.SysDict{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
		Enabled:     enabled,
	}
	if err := h.DB.Create(&d).Error; err != nil {
		response.BizFail(c, "字典编码已存在", 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "dict.create", &me, d.Code)
	response.OK(c, d, "创建成功")
}

func (h *DictHandler) UpdateDict(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var d models.SysDict
	if err := h.DB.First(&d, id).Error; err != nil {
		response.BizFail(c, "字典不存在", 404)
		return
	}
	var req dictUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			response.BizFail(c, "名称不能为空", 422)
			return
		}
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.Enabled != nil {
		if *req.Enabled != 0 && *req.Enabled != 1 {
			response.BizFail(c, "启用状态无效", 422)
			return
		}
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		response.OK(c, d, "")
		return
	}
	if err := h.DB.Model(&d).Updates(updates).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	h.DB.First(&d, id)
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "dict.update", &me, d.Code)
	response.OK(c, d, "更新成功")
}

func (h *DictHandler) DeleteDict(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var d models.SysDict
	if err := h.DB.First(&d, id).Error; err != nil {
		response.BizFail(c, "字典不存在", 404)
		return
	}
	// 已被引用（存在字典项）时只能停用不能删除
	var itemCount int64
	h.DB.Model(&models.SysDictItem{}).Where("dict_id = ?", id).Count(&itemCount)
	if itemCount > 0 {
		response.BizFail(c, "该字典下存在字典项，请先停用或删除字典项", 400)
		return
	}
	if err := h.DB.Delete(&d).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "dict.delete", &me, d.Code)
	response.OK(c, nil, "已删除")
}

// ---------- 字典项 ----------

type dictItemCreateReq struct {
	Code      string `json:"code" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Value     string `json:"value" binding:"required"`
	SortOrder int    `json:"sort_order"`
	Enabled   *int   `json:"enabled"`
	Remark    string `json:"remark"`
}

type dictItemUpdateReq struct {
	Name      *string `json:"name"`
	Value     *string `json:"value"`
	SortOrder *int    `json:"sort_order"`
	Enabled   *int    `json:"enabled"`
	Remark    *string `json:"remark"`
}

func (h *DictHandler) ListItems(c *gin.Context) {
	dictID, err := strconv.Atoi(c.Param("id"))
	if err != nil || dictID < 1 {
		response.BizFail(c, "无效字典 ID", 400)
		return
	}
	var items []models.SysDictItem
	h.DB.Where("dict_id = ?", dictID).Order("sort_order asc, id asc").Find(&items)
	response.OK(c, gin.H{"items": items, "total": len(items)}, "")
}

func (h *DictHandler) CreateItem(c *gin.Context) {
	dictID, err := strconv.Atoi(c.Param("id"))
	if err != nil || dictID < 1 {
		response.BizFail(c, "无效字典 ID", 400)
		return
	}
	var d models.SysDict
	if err := h.DB.First(&d, dictID).Error; err != nil {
		response.BizFail(c, "字典不存在", 404)
		return
	}
	var req dictItemCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.Value = strings.TrimSpace(req.Value)
	if req.Code == "" || req.Name == "" || req.Value == "" {
		response.BizFail(c, "编码、名称与值不能为空", 422)
		return
	}
	// 同一字典下编码唯一
	var dup int64
	h.DB.Model(&models.SysDictItem{}).Where("dict_id = ? AND code = ?", dictID, req.Code).Count(&dup)
	if dup > 0 {
		response.BizFail(c, "字典项编码已存在", 400)
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
	item := models.SysDictItem{
		DictID:    uint(dictID),
		Code:      req.Code,
		Name:      req.Name,
		Value:     req.Value,
		SortOrder: req.SortOrder,
		Enabled:   enabled,
		Remark:    req.Remark,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.BizFail(c, "创建失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "dict_item.create", &me, d.Code+":"+item.Code)
	response.OK(c, item, "创建成功")
}

func (h *DictHandler) UpdateItem(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("itemId"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效字典项 ID", 400)
		return
	}
	var item models.SysDictItem
	if err := h.DB.First(&item, id).Error; err != nil {
		response.BizFail(c, "字典项不存在", 404)
		return
	}
	var req dictItemUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			response.BizFail(c, "名称不能为空", 422)
			return
		}
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Value != nil {
		if strings.TrimSpace(*req.Value) == "" {
			response.BizFail(c, "值不能为空", 422)
			return
		}
		updates["value"] = strings.TrimSpace(*req.Value)
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.Enabled != nil {
		if *req.Enabled != 0 && *req.Enabled != 1 {
			response.BizFail(c, "启用状态无效", 422)
			return
		}
		updates["enabled"] = *req.Enabled
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if len(updates) == 0 {
		response.OK(c, item, "")
		return
	}
	if err := h.DB.Model(&item).Updates(updates).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	h.DB.First(&item, id)
	var d models.SysDict
	h.DB.First(&d, item.DictID)
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "dict_item.update", &me, d.Code+":"+item.Code)
	response.OK(c, item, "更新成功")
}

func (h *DictHandler) DeleteItem(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("itemId"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效字典项 ID", 400)
		return
	}
	var item models.SysDictItem
	if err := h.DB.First(&item, id).Error; err != nil {
		response.BizFail(c, "字典项不存在", 404)
		return
	}
	if err := h.DB.Delete(&item).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	var d models.SysDict
	h.DB.First(&d, item.DictID)
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "dict_item.delete", &me, d.Code+":"+item.Code)
	response.OK(c, nil, "已删除")
}

// ---------- 公开查询（已认证用户可获取启用的字典项） ----------

// GetDictItemsByCode 按字典编码获取启用的字典项列表（供前端表单下拉使用）。
func (h *DictHandler) GetDictItemsByCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.BizFail(c, "字典编码不能为空", 400)
		return
	}
	var d models.SysDict
	if err := h.DB.Where("code = ? AND enabled = ?", code, 1).First(&d).Error; err != nil {
		response.BizFail(c, "字典不存在或已停用", 404)
		return
	}
	var items []models.SysDictItem
	h.DB.Where("dict_id = ? AND enabled = ?", d.ID, 1).Order("sort_order asc, id asc").Find(&items)
	response.OK(c, gin.H{"code": d.Code, "name": d.Name, "items": items}, "")
}
