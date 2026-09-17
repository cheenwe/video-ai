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

type DepartmentHandler struct{ DB *gorm.DB }

type departmentCreateReq struct {
	Name      string `json:"name" binding:"required"`
	ParentID  *uint  `json:"parent_id"`
	SortOrder *int   `json:"sort_order"`
	Remark    string `json:"remark"`
}

type departmentUpdateReq struct {
	Name      *string `json:"name"`
	ParentID  *uint   `json:"parent_id"`
	SortOrder *int    `json:"sort_order"`
	Remark    *string `json:"remark"`
}

func departmentOut(d models.Department) gin.H {
	return gin.H{
		"id":         strconv.FormatUint(uint64(d.ID), 10),
		"name":       d.Name,
		"parent_id":  d.ParentID,
		"sort_order": d.SortOrder,
		"remark":     d.Remark,
		"created_at": d.CreatedAt,
		"updated_at": d.UpdatedAt,
	}
}

func (h *DepartmentHandler) List(c *gin.Context) {
	q := services.NewRBACService(h.DB).ApplyDataScope(h.DB.Model(&models.Department{}), middleware.CurrentUser(c), services.ScopeOptions{DeptIDColumn: "id"})
	if c.Query("all") == "1" {
		var items []models.Department
		q.Order("sort_order asc, id asc").Find(&items)
		out := make([]gin.H, 0, len(items))
		for _, item := range items {
			out = append(out, departmentOut(item))
		}
		response.OK(c, services.PaginateResult(out, int64(len(items)), 1, len(items)), "")
		return
	}
	page, pageSize := services.ParsePagination(c, h.DB)
	var total int64
	q.Count(&total)
	var items []models.Department
	q.Order("sort_order asc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, departmentOut(item))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

func (h *DepartmentHandler) Create(c *gin.Context) {
	var req departmentCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 400)
		return
	}
	if req.ParentID != nil && *req.ParentID == 0 {
		req.ParentID = nil
	}
	if req.ParentID != nil {
		var parent models.Department
		if err := h.DB.First(&parent, *req.ParentID).Error; err != nil {
			response.BizFail(c, "上级部门不存在", 400)
			return
		}
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 128 {
		response.BizFail(c, "部门名称不能为空且不得超过128字节", 422)
		return
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	item := models.Department{
		Name:      req.Name,
		ParentID:  req.ParentID,
		SortOrder: sortOrder,
		Remark:    req.Remark,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.BizFail(c, "创建失败", 500)
		return
	}
	user := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "department", "ok", "department.create", &user, item.Name)
	response.OK(c, departmentOut(item), "已创建")
}

func (h *DepartmentHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), "department", uint(id), true) {
		response.BizFail(c, "资源不存在或无权限", 403)
		return
	}

	var item models.Department
	if err := h.DB.First(&item, id).Error; err != nil {
		response.BizFail(c, "部门不存在", 404)
		return
	}
	var req departmentUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 400)
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 128 {
			response.BizFail(c, "部门名称不合法", 422)
			return
		}
		item.Name = name
	}
	if req.ParentID != nil {
		if *req.ParentID == item.ID {
			response.BizFail(c, "不能将自己设为上级部门", 400)
			return
		}
		if *req.ParentID != 0 {
			var parent models.Department
			if err := h.DB.First(&parent, *req.ParentID).Error; err != nil {
				response.BizFail(c, "上级部门不存在", 400)
				return
			}
		}
		if *req.ParentID == 0 {
			item.ParentID = nil
		} else {
			seen := map[uint]bool{item.ID: true}
			cursor := *req.ParentID
			for cursor != 0 {
				if seen[cursor] {
					response.BizFail(c, "部门层级不能形成循环", 400)
					return
				}
				seen[cursor] = true
				var ancestor models.Department
				if h.DB.First(&ancestor, cursor).Error != nil {
					response.BizFail(c, "上级部门不存在", 400)
					return
				}
				if ancestor.ParentID == nil {
					break
				}
				cursor = *ancestor.ParentID
			}
			item.ParentID = req.ParentID
		}
	}
	if req.SortOrder != nil {
		item.SortOrder = *req.SortOrder
	}
	if req.Remark != nil {
		item.Remark = *req.Remark
	}
	if err := h.DB.Save(&item).Error; err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	user := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "department", "ok", "department.update", &user, item.Name)
	response.OK(c, departmentOut(item), "已保存")
}

func (h *DepartmentHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), "department", uint(id), true) {
		response.BizFail(c, "资源不存在或无权限", 403)
		return
	}

	var item models.Department
	if err := h.DB.First(&item, id).Error; err != nil {
		response.BizFail(c, "部门不存在", 404)
		return
	}
	var childCount int64
	h.DB.Model(&models.Department{}).Where("parent_id = ?", item.ID).Count(&childCount)
	if childCount > 0 {
		response.BizFail(c, "请先删除或移出子部门", 400)
		return
	}
	var userCount int64
	h.DB.Model(&models.User{}).Where("department_id = ?", item.ID).Count(&userCount)
	if userCount > 0 {
		response.BizFail(c, "部门下仍有用户，请先调整用户归属", 400)
		return
	}
	if err := h.DB.Delete(&item).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	user := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "department", "ok", "department.delete", &user, item.Name)
	response.OK(c, nil, "已删除")
}
