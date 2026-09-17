package handlers

import (
	"strconv"
	"strings"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ProjectHandler struct {
	DB   *gorm.DB
	RBAC *services.RBACService
}

type projectCreateReq struct {
	Code        string `json:"code" binding:"required,max=64"`
	Name        string `json:"name" binding:"required,max=128"`
	Description string `json:"description"`
	OwnerID     *uint  `json:"owner_id"`
}

type projectUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	OwnerID     *uint   `json:"owner_id"`
	Enabled     *int    `json:"enabled"`
}

type memberAddReq struct {
	UserID uint   `json:"user_id" binding:"required"`
	Role   string `json:"role"`
}

func projectOut(p models.Project) gin.H {
	return gin.H{
		"id":          p.ID,
		"code":        p.Code,
		"name":        p.Name,
		"description": p.Description,
		"owner_id":    p.OwnerID,
		"enabled":     p.Enabled != 0,
		"created_at":  p.CreatedAt,
		"updated_at":  p.UpdatedAt,
	}
}

// List 项目列表（按数据范围过滤）。
func (h *ProjectHandler) List(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	u := middleware.CurrentUser(c)
	q := h.RBAC.ApplyDataScope(h.DB.Model(&models.Project{}), u, services.ScopeOptions{
		UserIDColumn:    "owner_id",
		ProjectIDColumn: "id",
	})
	// 项目列表按 owner_id 过滤即可
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR code LIKE ? OR description LIKE ?", like, like, like)
	}
	var total int64
	q.Count(&total)
	var items []models.Project
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, p := range items {
		out = append(out, projectOut(p))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

// Create 新建项目，创建者自动成为 owner 成员。
func (h *ProjectHandler) Create(c *gin.Context) {
	var req projectCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	u := middleware.CurrentUser(c)
	ownerID := u.ID
	if req.OwnerID != nil {
		ownerID = *req.OwnerID
	}
	var owner models.User
	if h.DB.First(&owner, ownerID).Error != nil {
		response.BizFail(c, "负责人不存在", 422)
		return
	}
	project := models.Project{
		Code:        req.Code,
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		OwnerID:     &ownerID,
		Enabled:     1,
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		// 创建者加入项目
		member := models.ProjectMember{
			ProjectID: project.ID,
			UserID:    ownerID,
			Role:      "owner",
		}
		return tx.Create(&member).Error
	}); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			response.BizFail(c, "项目编码已存在", 400)
			return
		}
		response.BizFail(c, "创建失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "project.create", &u, project.Name)
	response.OK(c, projectOut(project), "已创建")
}

// Update 编辑项目。
func (h *ProjectHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if !h.RBAC.CanAccessEntity(middleware.CurrentUser(c), "project", uint(id), true) {
		response.BizFail(c, "项目不存在或无权限", 403)
		return
	}

	var project models.Project
	if err := h.DB.First(&project, id).Error; err != nil {
		response.BizFail(c, "项目不存在", 404)
		return
	}
	var req projectUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			response.BizFail(c, "名称不能为空", 400)
			return
		}
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.OwnerID != nil {
		var owner models.User
		if h.DB.First(&owner, *req.OwnerID).Error != nil {
			response.BizFail(c, "负责人不存在", 422)
			return
		}

		updates["owner_id"] = *req.OwnerID
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) > 0 {
		if err := h.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&project).Updates(updates).Error; err != nil {
				return err
			}
			if req.OwnerID != nil {
				if err := tx.Model(&models.ProjectMember{}).Where("project_id = ? AND role = ?", project.ID, "owner").Update("role", "member").Error; err != nil {
					return err
				}
				var member models.ProjectMember
				err := tx.Where("project_id = ? AND user_id = ?", project.ID, *req.OwnerID).First(&member).Error
				if err == gorm.ErrRecordNotFound {
					return tx.Create(&models.ProjectMember{ProjectID: project.ID, UserID: *req.OwnerID, Role: "owner"}).Error
				}
				if err != nil {
					return err
				}
				return tx.Model(&member).Update("role", "owner").Error
			}
			return nil
		}); err != nil {
			response.BizFail(c, "更新失败", 500)
			return
		}
	}
	if err := h.DB.First(&project, id).Error; err != nil {
		response.BizFail(c, "项目不存在", 404)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "project.update", &u, project.Name)
	response.OK(c, projectOut(project), "已更新")
}

// Delete 删除项目（同时清理成员）。
func (h *ProjectHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if !h.RBAC.CanAccessEntity(middleware.CurrentUser(c), "project", uint(id), true) {
		response.BizFail(c, "项目不存在或无权限", 403)
		return
	}

	var project models.Project
	if err := h.DB.First(&project, id).Error; err != nil {
		response.BizFail(c, "项目不存在", 404)
		return
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", id).Delete(&models.ProjectMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&project).Error
	}); err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "project.delete", &u, project.Name)
	response.OK(c, nil, "已删除")
}

// Members 项目成员列表。
func (h *ProjectHandler) Members(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if !h.RBAC.CanAccessEntity(middleware.CurrentUser(c), "project", uint(id), false) {
		response.BizFail(c, "项目不存在或无权限", 403)
		return
	}

	var members []models.ProjectMember
	if err := h.DB.Where("project_id = ?", id).Order("id asc").Find(&members).Error; err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	out := make([]gin.H, 0, len(members))
	for _, m := range members {
		var user models.User
		_ = h.DB.Select("id, username, real_name").First(&user, m.UserID).Error
		out = append(out, gin.H{
			"id":         m.ID,
			"project_id": m.ProjectID,
			"user_id":    m.UserID,
			"username":   user.Username,
			"real_name":  user.RealName,
			"role":       m.Role,
			"created_at": m.CreatedAt,
		})
	}
	response.OK(c, gin.H{"items": out}, "")
}

// AddMember 添加项目成员。
func (h *ProjectHandler) AddMember(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if !h.RBAC.CanAccessEntity(middleware.CurrentUser(c), "project", uint(id), true) {
		response.BizFail(c, "项目不存在或无权限", 403)
		return
	}

	var req memberAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "member"
	}
	if role != "owner" && role != "admin" && role != "member" {
		response.BizFail(c, "角色无效（owner/admin/member）", 400)
		return
	}
	var target models.User
	if h.DB.First(&target, req.UserID).Error != nil {
		response.BizFail(c, "用户不存在", 422)
		return
	}
	if role == "owner" {
		response.BizFail(c, "负责人由项目创建时确定", 422)
		return
	}
	// 检查是否已存在
	var existing models.ProjectMember
	if err := h.DB.Where("project_id = ? AND user_id = ?", id, req.UserID).First(&existing).Error; err == nil {
		response.BizFail(c, "该用户已是项目成员", 400)
		return
	}
	member := models.ProjectMember{
		ProjectID: uint(id),
		UserID:    req.UserID,
		Role:      role,
	}
	if err := h.DB.Create(&member).Error; err != nil {
		response.BizFail(c, "添加失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "project.add_member", &u, strconv.Itoa(id))
	response.OK(c, gin.H{"id": member.ID}, "已添加")
}

// UpdateMember 更新成员角色。
func (h *ProjectHandler) UpdateMember(c *gin.Context) {
	mid, err := strconv.Atoi(c.Param("mid"))
	if err != nil || mid < 1 {
		response.BizFail(c, "无效成员 ID", 400)
		return
	}
	var member models.ProjectMember
	if h.DB.First(&member, mid).Error != nil || !h.RBAC.CanAccessEntity(middleware.CurrentUser(c), "project", member.ProjectID, true) {
		response.BizFail(c, "成员不存在或无权限", 403)
		return
	}
	if member.Role == "owner" {
		response.BizFail(c, "项目负责人需通过转移负责人操作修改", 409)
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	role := strings.TrimSpace(req.Role)
	if role != "owner" && role != "admin" && role != "member" {
		response.BizFail(c, "角色无效（owner/admin/member）", 400)
		return
	}
	if role == "owner" {
		response.BizFail(c, "不能通过成员角色变更负责人", 422)
		return
	}
	if err := h.DB.Model(&models.ProjectMember{}).Where("id = ?", mid).Update("role", role).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "project.update_member", &u, strconv.Itoa(mid))
	response.OK(c, nil, "已更新")
}

// RemoveMember 移除项目成员。
func (h *ProjectHandler) RemoveMember(c *gin.Context) {
	mid, err := strconv.Atoi(c.Param("mid"))
	if err != nil || mid < 1 {
		response.BizFail(c, "无效成员 ID", 400)
		return
	}
	var member models.ProjectMember
	if h.DB.First(&member, mid).Error != nil || !h.RBAC.CanAccessEntity(middleware.CurrentUser(c), "project", member.ProjectID, true) {
		response.BizFail(c, "成员不存在或无权限", 403)
		return
	}
	if member.Role == "owner" {
		response.BizFail(c, "项目负责人需通过转移负责人操作修改", 409)
		return
	}

	if err := h.DB.Delete(&models.ProjectMember{}, mid).Error; err != nil {
		response.BizFail(c, "移除失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "project.remove_member", &u, strconv.Itoa(mid))
	response.OK(c, nil, "已移除")
}
