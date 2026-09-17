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

type RBACHandler struct {
	DB   *gorm.DB
	RBAC *services.RBACService
}

type roleCreateReq struct {
	Code        string `json:"code" binding:"required,max=64"`
	Name        string `json:"name" binding:"required,max=128"`
	Description string `json:"description"`
	DataScope   string `json:"data_scope"`
}

type roleUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	DataScope   *string `json:"data_scope"`
	Enabled     *int    `json:"enabled"`
}

type assignPermissionsReq struct {
	Permissions []string `json:"permissions"`
}

func roleOut(r models.Role) gin.H {
	return gin.H{
		"id":          r.ID,
		"code":        r.Code,
		"name":        r.Name,
		"description": r.Description,
		"data_scope":  r.DataScope,
		"is_builtin":  r.IsBuiltin != 0,
		"sort_order":  r.SortOrder,
		"enabled":     r.Enabled != 0,
		"created_at":  r.CreatedAt,
		"updated_at":  r.UpdatedAt,
	}
}

// List 角色列表。
func (h *RBACHandler) List(c *gin.Context) {
	var roles []models.Role
	if err := h.DB.Order("sort_order asc, id asc").Find(&roles).Error; err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	out := make([]gin.H, 0, len(roles))
	for _, r := range roles {
		out = append(out, roleOut(r))
	}
	response.OK(c, gin.H{"items": out}, "")
}

// Create 新建角色。
func (h *RBACHandler) Create(c *gin.Context) {
	var req roleCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if !validRoleCode(req.Code) {
		response.BizFail(c, "角色编码只能包含字母、数字、下划线和连字符", 400)
		return
	}
	dataScope := req.DataScope
	if !validDataScope(dataScope) {
		dataScope = models.DataScopeSelf
	}
	role := models.Role{
		Code:        req.Code,
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		DataScope:   dataScope,
		IsBuiltin:   0,
		Enabled:     1,
	}
	if err := h.DB.Create(&role).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			response.BizFail(c, "角色编码已存在", 400)
			return
		}
		response.BizFail(c, "创建失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "role.create", &u, role.Name)
	response.OK(c, roleOut(role), "已创建")
}

// Update 编辑角色（内置角色仅可改名称/描述/数据范围/启用状态）。
func (h *RBACHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var role models.Role
	if err := h.DB.First(&role, id).Error; err != nil {
		response.BizFail(c, "角色不存在", 404)
		return
	}
	var req roleUpdateReq
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
	if req.DataScope != nil {
		if !validDataScope(*req.DataScope) {
			response.BizFail(c, "无效的数据范围", 400)
			return
		}
		updates["data_scope"] = *req.DataScope
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) > 0 {
		if err := h.DB.Model(&role).Updates(updates).Error; err != nil {
			response.BizFail(c, "更新失败", 500)
			return
		}
	}
	if err := h.DB.First(&role, id).Error; err != nil {
		response.BizFail(c, "角色不存在", 404)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "role.update", &u, role.Name)
	response.OK(c, roleOut(role), "已更新")
}

// Delete 删除角色（内置角色不可删除）。
func (h *RBACHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var role models.Role
	if err := h.DB.First(&role, id).Error; err != nil {
		response.BizFail(c, "角色不存在", 404)
		return
	}
	if role.IsBuiltin != 0 {
		response.BizFail(c, "内置角色不可删除", 400)
		return
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", id).Delete(&models.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&models.UserRole{}).Error; err != nil {
			return err
		}
		return tx.Delete(&role).Error
	}); err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "role.delete", &u, role.Name)
	response.OK(c, nil, "已删除")
}

// Permissions 返回全部权限点（按模块分组）。
func (h *RBACHandler) Permissions(c *gin.Context) {
	var perms []models.Permission
	if err := h.DB.Order("module asc, sort_order asc, id asc").Find(&perms).Error; err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	type permItem struct {
		ID        uint   `json:"id"`
		Code      string `json:"code"`
		Name      string `json:"name"`
		Module    string `json:"module"`
		Type      string `json:"type"`
		SortOrder int    `json:"sort_order"`
	}
	groups := map[string][]permItem{}
	order := []string{}
	for _, p := range perms {
		if _, ok := groups[p.Module]; !ok {
			order = append(order, p.Module)
		}
		groups[p.Module] = append(groups[p.Module], permItem{
			ID: p.ID, Code: p.Code, Name: p.Name, Module: p.Module, Type: p.Type, SortOrder: p.SortOrder,
		})
	}
	modules := make([]gin.H, 0, len(order))
	for _, m := range order {
		modules = append(modules, gin.H{"module": m, "items": groups[m]})
	}
	response.OK(c, gin.H{"modules": modules}, "")
}

// RolePermissions 返回角色拥有的权限 code 列表。
func (h *RBACHandler) RolePermissions(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	codes, err := h.RBAC.RolePermissionCodes(uint(id))
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	response.OK(c, gin.H{"permissions": codes}, "")
}

// AssignPermissions 为角色分配权限（全量替换）。
func (h *RBACHandler) AssignPermissions(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var role models.Role
	if err := h.DB.First(&role, id).Error; err != nil {
		response.BizFail(c, "角色不存在", 404)
		return
	}
	var req assignPermissionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	for _, code := range req.Permissions {
		var count int64
		if h.DB.Model(&models.Permission{}).Where("code = ?", code).Count(&count).Error != nil || count != 1 {
			response.BizFail(c, "存在未知权限点", 422)
			return
		}
	}
	if err := h.RBAC.AssignRolePermissions(uint(id), req.Permissions); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "role.assign_permissions", &u, role.Name)
	response.OK(c, gin.H{"permissions": req.Permissions}, "权限已更新")
}

// AssignUserRole 为用户分配角色（全量替换用户的自定义角色，保留内置 User.Role）。
func (h *RBACHandler) AssignUserRole(c *gin.Context) {
	uid, err := strconv.Atoi(c.Param("uid"))
	if err != nil || uid < 1 {
		response.BizFail(c, "无效用户 ID", 400)
		return
	}
	var user models.User
	if err := h.DB.First(&user, uid).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	var req struct {
		RoleIDs []uint `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	seen := map[uint]bool{}
	for _, id := range req.RoleIDs {
		var role models.Role
		if seen[id] || h.DB.First(&role, id).Error != nil || role.Enabled == 0 || role.IsBuiltin != 0 {
			response.BizFail(c, "请选择有效且不重复的自定义角色", 422)
			return
		}
		seen[id] = true
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", uid).Delete(&models.UserRole{}).Error; err != nil {
			return err
		}
		if len(req.RoleIDs) == 0 {
			return nil
		}
		urs := make([]models.UserRole, 0, len(req.RoleIDs))
		for _, rid := range req.RoleIDs {
			urs = append(urs, models.UserRole{UserID: uint(uid), RoleID: rid})
		}
		return tx.Create(&urs).Error
	}); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "user.assign_role", &me, user.Username)
	response.OK(c, nil, "角色已更新")
}

// UserRoles 返回用户的角色 ID 列表（含内置角色与 UserRole）。
func (h *RBACHandler) UserRoles(c *gin.Context) {
	uid, err := strconv.Atoi(c.Param("uid"))
	if err != nil || uid < 1 {
		response.BizFail(c, "无效用户 ID", 400)
		return
	}
	var user models.User
	if err := h.DB.First(&user, uid).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	var urs []models.UserRole
	if err := h.DB.Where("user_id = ?", uid).Find(&urs).Error; err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	ids := make([]uint, 0, len(urs))
	for _, ur := range urs {
		ids = append(ids, ur.RoleID)
	}
	response.OK(c, gin.H{"builtin_role": user.Role, "role_ids": ids}, "")
}

// MyPermissions 返回当前用户的权限 code 列表，供前端菜单/按钮控制。
func (h *RBACHandler) MyPermissions(c *gin.Context) {
	u := middleware.CurrentUser(c)
	codes, err := h.RBAC.UserPermissionCodes(u)
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	response.OK(c, gin.H{"permissions": codes}, "")
}

func validRoleCode(code string) bool {
	if code == "" {
		return false
	}
	for _, r := range code {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func validDataScope(scope string) bool {
	switch scope {
	case models.DataScopeAll, models.DataScopeProject, models.DataScopeDept, models.DataScopeDeptAndSub, models.DataScopeSelf:
		return true
	}
	return false
}
