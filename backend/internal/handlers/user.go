package handlers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/security"
	"baseplatform/internal/services"
	"baseplatform/internal/settings"
	"baseplatform/internal/validation"
)

type UserHandler struct {
	DB *gorm.DB
}

type userCreateReq struct {
	Username        string  `json:"username" binding:"required,min=3,max=64"`
	Email           string  `json:"email" binding:"required,email,max=128"`
	Password        string  `json:"password" binding:"required,min=5,max=128"`
	Role            string  `json:"role"`
	RealName        *string `json:"real_name"`
	DepartmentID    *uint   `json:"department_id"`
	Timezone        string  `json:"timezone"`
	WebLoginEnabled *bool   `json:"web_login_enabled"`
}

type userUpdateReq struct {
	Username        *string `json:"username"`
	Email           *string `json:"email"`
	Role            *string `json:"role"`
	RealName        *string `json:"real_name"`
	DepartmentID    *uint   `json:"department_id"`
	NewPassword     *string `json:"new_password"`
	Timezone        *string `json:"timezone"`
	WebLoginEnabled *bool   `json:"web_login_enabled"`
}

type disabledReq struct {
	Disabled bool `json:"disabled"`
}

func userOut(db *gorm.DB, u models.User) gin.H {
	out := gin.H{
		"id":                u.ID,
		"username":          u.Username,
		"email":             u.Email,
		"role":              u.Role,
		"disabled":          u.Disabled,
		"web_login_enabled": u.WebLoginEnabled != 0,
		"real_name":         u.RealName,
		"department_id":     u.DepartmentID,
		"timezone":          u.Timezone,
		"avatar_url":        u.AvatarURL,
		"created_at":        u.CreatedAt,
		"is_system":         u.IsSystemUser(),
		"auth_source":       u.AuthSource,
	}
	if u.DepartmentID != nil {
		var dept models.Department
		if err := db.First(&dept, *u.DepartmentID).Error; err == nil {
			out["department_name"] = dept.Name
		}
	}
	return out
}

func validAssignableRole(role string) bool {
	return role == models.RoleUser || role == models.RoleAdmin
}

func (h *UserHandler) List(c *gin.Context) {
	page, pageSize := services.ParsePagination(c, h.DB)
	keyword := c.Query("keyword")
	q := services.NewRBACService(h.DB).ApplyDataScope(h.DB.Model(&models.User{}), middleware.CurrentUser(c), services.ScopeOptions{UserIDColumn: "id", DeptIDColumn: "department_id"})
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("username LIKE ? OR real_name LIKE ? OR email LIKE ?", like, like, like)
	}
	var total int64
	q.Count(&total)
	var items []models.User
	q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, u := range items {
		out = append(out, userOut(h.DB, u))
	}
	response.OK(c, services.PaginateResult(out, total, page, pageSize), "")
}

func (h *UserHandler) Create(c *gin.Context) {
	var req userCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, validation.UserCreateBindMessage(err), 422)
		return
	}
	username := strings.TrimSpace(req.Username)
	if err := validation.UsernameOK(username); err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	if models.IsSystemUsername(username) {
		response.BizFail(c, "系统用户名不可使用", 400)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !validation.EmailOK(email) {
		response.BizFail(c, "邮箱格式不正确", 422)
		return
	}
	sec := settings.LoadSecurity(h.DB)
	if err := services.ValidatePassword(req.Password, sec); err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	if req.Role == "admin" && !middleware.CurrentUser(c).IsSuperAdmin() {
		response.BizFail(c, "仅超级管理员可授予管理角色", 403)
		return
	}
	role := req.Role
	if role == "" {
		role = models.RoleUser
	}
	if !validAssignableRole(role) {
		response.BizFail(c, "role 须为 admin 或 user", 400)
		return
	}

	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	var exists int64
	h.DB.Model(&models.User{}).Where("username = ?", username).Count(&exists)
	if exists > 0 {
		response.BizFail(c, "用户名已存在", 409)
		return
	}
	h.DB.Model(&models.User{}).Where("email = ?", email).Count(&exists)
	if exists > 0 {
		response.BizFail(c, "邮箱已被注册", 409)
		return
	}
	hash, err := security.HashPassword(req.Password)
	if err != nil {
		response.BizFail(c, "密码处理失败", 500)
		return
	}
	u := models.User{Username: username, Email: &email, PasswordHash: hash, Role: role, RealName: req.RealName, DepartmentID: req.DepartmentID, Timezone: tz, WebLoginEnabled: 1, MustChangePassword: 1}
	if req.WebLoginEnabled != nil && !*req.WebLoginEnabled {
		u.WebLoginEnabled = 0
	}
	if req.DepartmentID != nil {
		var dept models.Department
		if err := h.DB.First(&dept, *req.DepartmentID).Error; err != nil {
			response.BizFail(c, "部门不存在", 400)
			return
		}
	}
	if err := h.DB.Create(&u).Error; err != nil {
		response.BizFail(c, "创建失败", 400)
		return
	}

	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "user.create", &me, "创建用户 "+u.Username+" role="+role)
	response.OK(c, userOut(h.DB, u), "创建成功")
}

func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("uid"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), "user", uint(id), true) {
		response.BizFail(c, "资源不存在或无权限", 403)
		return
	}

	var u models.User
	if err := h.DB.First(&u, id).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	meActor := middleware.CurrentUser(c)
	if (u.IsSuperAdmin() && !meActor.IsSuperAdmin()) || u.ID == meActor.ID {
		response.BizFail(c, "请在个人中心修改本人资料；不能修改更高权限账号", 403)
		return
	}
	var req userUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if u.IsSystemUser() && req.Role != nil && *req.Role != u.Role {
		response.BizFail(c, "系统用户不允许修改角色", 403)
		return
	}
	if u.IsSystemUser() && req.Username != nil && *req.Username != u.Username {
		response.BizFail(c, "系统用户不允许修改用户名", 403)
		return
	}
	if u.IsSystemUser() && req.NewPassword != nil {
		response.BizFail(c, "系统用户不允许修改密码", 403)
		return
	}
	updates := map[string]interface{}{}
	if req.Username != nil {
		name := strings.TrimSpace(*req.Username)
		if err := validation.UsernameOK(name); err != nil {
			response.BizFail(c, err.Error(), 422)
			return
		}
		if models.IsSystemUsername(name) && name != u.Username {
			response.BizFail(c, "系统用户名不可使用", 400)
			return
		}
		updates["username"] = name
	}
	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if email == "" {
			response.BizFail(c, "邮箱不能为空", 422)
			return
		}
		if !validation.EmailOK(email) {
			response.BizFail(c, "邮箱格式不正确", 422)
			return
		}
		var exists int64
		h.DB.Model(&models.User{}).Where("email = ? AND id != ?", email, u.ID).Count(&exists)
		if exists > 0 {
			response.BizFail(c, "邮箱已被注册", 409)
			return
		}
		updates["email"] = email
	}
	if req.Role != nil {
		if *req.Role != u.Role && !meActor.IsSuperAdmin() {
			response.BizFail(c, "仅超级管理员可变更角色", 403)
			return
		}
		if !validAssignableRole(*req.Role) {
			response.BizFail(c, "role 须为 admin 或 user", 400)
			return
		}
		updates["role"] = *req.Role
		updates["session_version"] = gorm.Expr("session_version + 1")
	}
	if req.RealName != nil {
		updates["real_name"] = req.RealName
	}
	if req.DepartmentID != nil {
		if *req.DepartmentID != 0 {
			var dept models.Department
			if err := h.DB.First(&dept, *req.DepartmentID).Error; err != nil {
				response.BizFail(c, "部门不存在", 400)
				return
			}
		}
		if *req.DepartmentID == 0 {
			updates["department_id"] = nil
		} else {
			updates["department_id"] = req.DepartmentID
		}
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}
	if req.WebLoginEnabled != nil {
		if u.ID == meActor.ID && !*req.WebLoginEnabled {
			response.BizFail(c, "不能关闭本人的 Web 登录", 403)
			return
		}
		if u.Username == "admin" && !*req.WebLoginEnabled {
			response.BizFail(c, "不能关闭初始管理员的 Web 登录", 403)
			return
		}
		enabled := 0
		if *req.WebLoginEnabled {
			enabled = 1
		}
		if enabled != u.WebLoginEnabled {
			updates["web_login_enabled"] = enabled
			if enabled == 0 {
				updates["session_version"] = gorm.Expr("session_version + 1")
			}
		}
	}
	if req.NewPassword != nil {
		pwd := strings.TrimSpace(*req.NewPassword)
		if pwd == "" {
			response.BizFail(c, "新密码不能为空", 422)
			return
		}
		sec := settings.LoadSecurity(h.DB)
		if err := services.ValidatePassword(pwd, sec); err != nil {
			response.BizFail(c, err.Error(), 422)
			return
		}
		hash, err := security.HashPassword(pwd)
		if err != nil {
			response.BizFail(c, "密码处理失败", 500)
			return
		}
		updates["password_hash"] = hash
		updates["must_change_password"] = 1
		updates["session_version"] = gorm.Expr("session_version + 1")
	}
	if len(updates) > 0 {
		if err := h.DB.Model(&u).Updates(updates).Error; err != nil {
			response.BizFail(c, "更新失败，用户名或邮箱可能已存在", 409)
			return
		}
		h.DB.First(&u, u.ID)
	}
	me := middleware.CurrentUser(c)
	remark := "更新用户 " + u.Username
	if req.Role != nil {
		remark += " role=" + *req.Role
	}
	if req.NewPassword != nil {
		remark += " password_changed"
	}
	services.LogAction(h.DB, c, "audit", "success", "user.update", &me, remark)
	response.OK(c, userOut(h.DB, u), "更新成功")
}

func (h *UserHandler) SetDisabled(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("uid"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var req disabledReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	disabled := 0
	if req.Disabled {
		disabled = 1
	}
	if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), "user", uint(id), true) {
		response.BizFail(c, "资源不存在或无权限", 403)
		return
	}

	var u models.User
	if err := h.DB.First(&u, id).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	if u.ID == middleware.CurrentUser(c).ID {
		response.BizFail(c, "不能禁用或删除自己", 400)
		return
	}
	if u.IsSystemUser() {
		response.BizFail(c, "系统用户不允许禁用", 403)
		return
	}
	if !req.Disabled && u.Disabled != 0 {

	}
	if err := h.DB.Model(&u).Updates(map[string]any{"disabled": disabled, "session_version": gorm.Expr("session_version + 1")}).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	u.Disabled = disabled
	me := middleware.CurrentUser(c)
	action := "user.enabled"
	if req.Disabled {
		action = "user.disabled"
	}
	services.LogAction(h.DB, c, "audit", "success", action, &me, u.Username)
	response.OK(c, userOut(h.DB, u), "已更新")
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("uid"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	me := middleware.CurrentUser(c)
	if uint(id) == me.ID {
		response.BizFail(c, "不能删除自己", 400)
		return
	}
	if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), "user", uint(id), true) {
		response.BizFail(c, "资源不存在或无权限", 403)
		return
	}

	var u models.User
	if err := h.DB.First(&u, id).Error; err != nil {
		response.BizFail(c, "用户不存在", 404)
		return
	}
	if u.ID == middleware.CurrentUser(c).ID {
		response.BizFail(c, "不能禁用或删除自己", 400)
		return
	}
	if u.IsSystemUser() {
		response.BizFail(c, "系统用户不允许删除", 403)
		return
	}
	if err := h.DB.Delete(&u).Error; err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	services.LogAction(h.DB, c, "audit", "success", "user.delete", &me, u.Username)
	response.OK(c, nil, "已删除")
}
