package services

import (
	"errors"
	"sync"

	"baseplatform/internal/models"

	"gorm.io/gorm"
)

func init() {
	RegisterPlatformPermissions()
}

// PermissionDef 权限点定义（注册时使用）。
type PermissionDef struct {
	Code      string
	Name      string
	Module    string
	Type      string // menu | action
	Parent    string // 父权限 code（可选）
	SortOrder int
}

var (
	permissionRegistry = map[string]PermissionDef{}
	registryMu         sync.RWMutex
)

// RegisterPermission 注册一个权限点（重复注册覆盖）。
func RegisterPermission(def PermissionDef) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if def.Type == "" {
		def.Type = "action"
	}
	permissionRegistry[def.Code] = def
}

// RegisteredPermissions 返回所有已注册的权限定义。
func RegisteredPermissions() []PermissionDef {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]PermissionDef, 0, len(permissionRegistry))
	for _, d := range permissionRegistry {
		out = append(out, d)
	}
	return out
}

// RegisterPlatformPermissions 注册平台基础权限点。
// 与视频 AI 业务解耦，仅包含平台管理类权限。
func RegisterPlatformPermissions() {
	perms := []PermissionDef{
		// 用户管理
		{Code: "user:list", Name: "查看用户", Module: "user", Type: "action", SortOrder: 1},
		{Code: "user:create", Name: "创建用户", Module: "user", Type: "action", SortOrder: 2},
		{Code: "user:update", Name: "编辑用户", Module: "user", Type: "action", SortOrder: 3},
		{Code: "user:delete", Name: "删除用户", Module: "user", Type: "action", SortOrder: 4},
		{Code: "user:disable", Name: "禁用/启用用户", Module: "user", Type: "action", SortOrder: 5},
		// 部门管理
		{Code: "department:list", Name: "查看部门", Module: "department", Type: "action", SortOrder: 1},
		{Code: "department:create", Name: "创建部门", Module: "department", Type: "action", SortOrder: 2},
		{Code: "department:update", Name: "编辑部门", Module: "department", Type: "action", SortOrder: 3},
		{Code: "department:delete", Name: "删除部门", Module: "department", Type: "action", SortOrder: 4},
		// 系统配置
		{Code: "config:list", Name: "查看系统配置", Module: "config", Type: "action", SortOrder: 1},
		{Code: "config:create", Name: "新增配置项", Module: "config", Type: "action", SortOrder: 2},
		{Code: "config:update", Name: "修改配置项", Module: "config", Type: "action", SortOrder: 3},
		{Code: "config:delete", Name: "删除配置项", Module: "config", Type: "action", SortOrder: 4},
		// 审计日志
		{Code: "log:list", Name: "查看操作日志", Module: "log", Type: "action", SortOrder: 1},
		{Code: "log:export", Name: "导出操作日志", Module: "log", Type: "action", SortOrder: 2},
		// 字典管理
		{Code: "dict:list", Name: "查看字典", Module: "dict", Type: "action", SortOrder: 1},
		{Code: "dict:create", Name: "新增字典", Module: "dict", Type: "action", SortOrder: 2},
		{Code: "dict:update", Name: "编辑字典", Module: "dict", Type: "action", SortOrder: 3},
		{Code: "dict:delete", Name: "删除字典", Module: "dict", Type: "action", SortOrder: 4},
		// Webhook
		{Code: "webhook:list", Name: "查看 Webhook", Module: "webhook", Type: "action", SortOrder: 1},
		{Code: "webhook:create", Name: "新增 Webhook", Module: "webhook", Type: "action", SortOrder: 2},
		{Code: "webhook:update", Name: "编辑 Webhook", Module: "webhook", Type: "action", SortOrder: 3},
		{Code: "webhook:delete", Name: "删除 Webhook", Module: "webhook", Type: "action", SortOrder: 4},
		// 存储对接
		{Code: "storage:list", Name: "查看存储", Module: "storage", Type: "action", SortOrder: 1},
		{Code: "storage:create", Name: "新增存储", Module: "storage", Type: "action", SortOrder: 2},
		{Code: "storage:update", Name: "编辑存储", Module: "storage", Type: "action", SortOrder: 3},
		{Code: "storage:delete", Name: "删除存储", Module: "storage", Type: "action", SortOrder: 4},
		// 计划任务
		{Code: "schedule:list", Name: "查看计划任务", Module: "schedule", Type: "action", SortOrder: 1},
		{Code: "schedule:create", Name: "新增任务", Module: "schedule", Type: "action", SortOrder: 2},
		{Code: "schedule:update", Name: "编辑任务", Module: "schedule", Type: "action", SortOrder: 3},
		{Code: "schedule:delete", Name: "删除任务", Module: "schedule", Type: "action", SortOrder: 4},
		// 角色权限
		{Code: "role:list", Name: "查看角色", Module: "role", Type: "action", SortOrder: 1},
		{Code: "role:create", Name: "新增角色", Module: "role", Type: "action", SortOrder: 2},
		{Code: "role:update", Name: "编辑角色", Module: "role", Type: "action", SortOrder: 3},
		{Code: "role:delete", Name: "删除角色", Module: "role", Type: "action", SortOrder: 4},
		{Code: "role:assign", Name: "分配权限", Module: "role", Type: "action", SortOrder: 5},
		// API 调用日志
		{Code: "apilog:list", Name: "查看 API 调用日志", Module: "apilog", Type: "action", SortOrder: 1},
		// 系统设置
		{Code: "settings:general", Name: "基本设置", Module: "settings", Type: "action", SortOrder: 1},
		{Code: "settings:security", Name: "安全策略", Module: "settings", Type: "action", SortOrder: 2},
		{Code: "settings:email", Name: "邮件设置", Module: "settings", Type: "action", SortOrder: 3},
		{Code: "settings:ldap", Name: "LDAP 设置", Module: "settings", Type: "action", SortOrder: 4},
		{Code: "settings:oauth", Name: "OAuth 设置", Module: "settings", Type: "action", SortOrder: 5},
		{Code: "settings:gateway", Name: "网关设置", Module: "settings", Type: "action", SortOrder: 6},
		// 视频监控（NVR 接入）
		{Code: "nvr:list", Name: "查看 NVR 节点", Module: "nvr", Type: "action", SortOrder: 1},
		{Code: "nvr:create", Name: "新增 NVR 节点", Module: "nvr", Type: "action", SortOrder: 2},
		{Code: "nvr:update", Name: "编辑 NVR 节点", Module: "nvr", Type: "action", SortOrder: 3},
		{Code: "nvr:delete", Name: "删除 NVR 节点", Module: "nvr", Type: "action", SortOrder: 4},
		{Code: "nvr:config", Name: "NVR 设备配置与联调", Module: "nvr", Type: "action", SortOrder: 5},
		{Code: "camera:list", Name: "查看摄像头", Module: "camera", Type: "action", SortOrder: 1},
		{Code: "camera:create", Name: "新增摄像头", Module: "camera", Type: "action", SortOrder: 2},
		{Code: "camera:update", Name: "编辑摄像头", Module: "camera", Type: "action", SortOrder: 3},
		{Code: "camera:delete", Name: "归档摄像头", Module: "camera", Type: "action", SortOrder: 4},
		{Code: "camera:live", Name: "实时预览", Module: "camera", Type: "action", SortOrder: 5},
		{Code: "camera:record", Name: "录制控制与录像下载任务", Module: "camera", Type: "action", SortOrder: 6},
		{Code: "recording:list", Name: "查看与回放录像", Module: "recording", Type: "action", SortOrder: 1},
		{Code: "recording:download", Name: "下载录像文件", Module: "recording", Type: "action", SortOrder: 2},
		{Code: "boundary:list", Name: "查看坑边边界", Module: "boundary", Type: "action", SortOrder: 1},
		{Code: "boundary:edit", Name: "编辑坑边边界", Module: "boundary", Type: "action", SortOrder: 2},
		{Code: "boundary:publish", Name: "发布坑边边界", Module: "boundary", Type: "action", SortOrder: 3},
		{Code: "algorithm:list", Name: "查看算法资产", Module: "algorithm", Type: "action", SortOrder: 1},
		{Code: "algorithm:publish", Name: "发布算法版本", Module: "algorithm", Type: "action", SortOrder: 2},
		{Code: "deployment:list", Name: "查看布控", Module: "deployment", Type: "action", SortOrder: 1},
		{Code: "deployment:edit", Name: "编辑布控", Module: "deployment", Type: "action", SortOrder: 2},
		{Code: "deployment:control", Name: "启停布控", Module: "deployment", Type: "action", SortOrder: 3},
		{Code: "event:list", Name: "查看告警事件", Module: "event", Type: "action", SortOrder: 1},
		{Code: "event:handle", Name: "处置告警事件", Module: "event", Type: "action", SortOrder: 2},
		{Code: "report:list", Name: "查看统计报表", Module: "report", Type: "action", SortOrder: 1},
		{Code: "access:list", Name: "查看扩展接入", Module: "access", Type: "action", SortOrder: 1},
		{Code: "access:edit", Name: "管理扩展接入", Module: "access", Type: "action", SortOrder: 2},
	}
	for _, p := range perms {
		RegisterPermission(p)
	}
}

// RBACService 角色与权限服务。
type RBACService struct {
	DB *gorm.DB
}

func NewRBACService(db *gorm.DB) *RBACService {
	return &RBACService{DB: db}
}

// SyncPermissions 将已注册的权限点同步到数据库（不存在则创建，已存在保留）。
func (s *RBACService) SyncPermissions() error {
	defs := RegisteredPermissions()
	for _, d := range defs {
		var p models.Permission
		err := s.DB.Where("code = ?", d.Code).First(&p).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		perm := models.Permission{
			Code:      d.Code,
			Name:      d.Name,
			Module:    d.Module,
			Type:      d.Type,
			SortOrder: d.SortOrder,
		}
		if d.Parent != "" {
			var parent models.Permission
			if e := s.DB.Where("code = ?", d.Parent).First(&parent).Error; e == nil {
				perm.ParentID = &parent.ID
			}
		}
		if err := s.DB.Create(&perm).Error; err != nil {
			return err
		}
	}
	return nil
}

// HasPermission 检查用户是否拥有指定权限。
// superadmin 内置角色拥有全部权限。
func (s *RBACService) HasPermission(user models.User, code string) bool {
	if user.Role == models.RoleSuperAdmin {
		return true
	}
	// 收集用户的所有角色 ID：内置角色（基于 User.Role）+ UserRole 表
	roleIDs, err := s.userRoleIDs(user)
	if err != nil || len(roleIDs) == 0 {
		return false
	}
	// 查询这些角色是否拥有该权限
	var perm models.Permission
	if err := s.DB.Where("code = ?", code).First(&perm).Error; err != nil {
		return false
	}
	var count int64
	s.DB.Model(&models.RolePermission{}).
		Where("role_id IN ? AND permission_id = ?", roleIDs, perm.ID).
		Count(&count)
	return count > 0
}

// userRoleIDs 返回用户关联的所有角色 ID（内置角色通过 code 匹配）。
func (s *RBACService) userRoleIDs(user models.User) ([]uint, error) {
	var ids []uint
	// 内置角色
	if user.Role != "" {
		var r models.Role
		if err := s.DB.Where("code = ? AND enabled = ?", user.Role, 1).First(&r).Error; err == nil {
			ids = append(ids, r.ID)
		}
	}
	// UserRole 表
	var urs []models.UserRole
	if err := s.DB.Where("user_id = ? AND role_id IN (?)", user.ID, s.DB.Model(&models.Role{}).Select("id").Where("enabled = ?", 1)).Find(&urs).Error; err != nil {
		return ids, err
	}
	for _, ur := range urs {
		ids = append(ids, ur.RoleID)
	}
	return ids, nil
}

// UserPermissionCodes 返回用户拥有的全部权限 code 列表。
func (s *RBACService) UserPermissionCodes(user models.User) ([]string, error) {
	if user.Role == models.RoleSuperAdmin {
		var codes []string
		s.DB.Model(&models.Permission{}).Pluck("code", &codes)
		return codes, nil
	}
	roleIDs, err := s.userRoleIDs(user)
	if err != nil {
		return nil, err
	}
	if len(roleIDs) == 0 {
		return []string{}, nil
	}
	var codes []string
	s.DB.Model(&models.Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id IN ?", roleIDs).
		Distinct().
		Pluck("permissions.code", &codes)
	return codes, nil
}

// AssignRolePermissions 为角色分配权限（全量替换）。
func (s *RBACService) AssignRolePermissions(roleID uint, permissionCodes []string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&models.RolePermission{}).Error; err != nil {
			return err
		}
		if len(permissionCodes) == 0 {
			return nil
		}
		var perms []models.Permission
		if err := tx.Where("code IN ?", permissionCodes).Find(&perms).Error; err != nil {
			return err
		}
		rps := make([]models.RolePermission, 0, len(perms))
		for _, p := range perms {
			rps = append(rps, models.RolePermission{RoleID: roleID, PermissionID: p.ID})
		}
		if len(rps) == 0 {
			return nil
		}
		return tx.Create(&rps).Error
	})
}

// RolePermissionCodes 返回角色拥有的权限 code 列表。
func (s *RBACService) RolePermissionCodes(roleID uint) ([]string, error) {
	var codes []string
	err := s.DB.Model(&models.Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Pluck("permissions.code", &codes).Error
	return codes, err
}

// SeedBuiltinRoles 创建内置角色（superadmin/admin/user）并分配默认权限。
// superadmin 拥有全部权限；admin 拥有除删除与角色管理外的管理权限；user 仅查看类权限。
func (s *RBACService) SeedBuiltinRoles() error {
	builtins := []struct {
		code        string
		name        string
		desc        string
		dataScope   string
		permissions []string
	}{
		{
			code:      models.RoleSuperAdmin,
			name:      "超级管理员",
			desc:      "系统初始化账号；全部基础管理权限",
			dataScope: models.DataScopeAll,
		},
		{
			code:      models.RoleAdmin,
			name:      "管理员",
			desc:      "用户、部门、配置及日志管理；无删除权限",
			dataScope: models.DataScopeAll,
			permissions: []string{
				"overview:list", "project:list", "project:create", "project:update",
				"custom_field:list", "custom_field:create", "custom_field:update", "schedule:run", "backup:list", "backup:download",
				"user:list", "user:create", "user:update", "user:disable",
				"department:list", "department:create", "department:update",
				"config:list", "config:create", "config:update",
				"log:list", "log:export",
				"dict:list", "dict:create", "dict:update",
				"webhook:list", "webhook:create", "webhook:update",
				"storage:list", "storage:create", "storage:update",
				"schedule:list", "schedule:create", "schedule:update",
				"apilog:list",
				"settings:general", "settings:security", "settings:email",
				"settings:ldap", "settings:oauth", "settings:gateway",
				"nvr:list", "nvr:create", "nvr:update", "nvr:config",
				"camera:list", "camera:create", "camera:update", "camera:delete", "camera:live", "camera:record",
				"recording:list", "recording:download",
				"boundary:list", "boundary:edit", "boundary:publish",
				"algorithm:list", "algorithm:publish",
				"deployment:list", "deployment:edit", "deployment:control",
				"event:list", "event:handle", "report:list",
				"access:list", "access:edit",
			},
		},
		{
			code:      models.RoleUser,
			name:      "普通用户",
			desc:      "个人资料与密码管理",
			dataScope: models.DataScopeSelf,
			permissions: []string{
				"dict:list", "storage:list",
			},
		},
	}
	for _, b := range builtins {
		var role models.Role
		err := s.DB.Where("code = ?", b.code).First(&role).Error
		if err == nil {
			// 已存在的授权由管理员维护；重启不得覆盖。
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		role = models.Role{
			Code:        b.code,
			Name:        b.name,
			Description: b.desc,
			DataScope:   b.dataScope,
			IsBuiltin:   1,
			Enabled:     1,
		}
		if err := s.DB.Create(&role).Error; err != nil {
			return err
		}
		if err := s.AssignRolePermissions(role.ID, b.permissions); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	if _, ok := permissionRegistry["apilog:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "apilog:list", Name: "apilog:list", Module: "apilog", Type: "action"})
	}
	if _, ok := permissionRegistry["backup:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "backup:create", Name: "backup:create", Module: "backup", Type: "action"})
	}
	if _, ok := permissionRegistry["backup:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "backup:delete", Name: "backup:delete", Module: "backup", Type: "action"})
	}
	if _, ok := permissionRegistry["backup:download"]; !ok {
		RegisterPermission(PermissionDef{Code: "backup:download", Name: "backup:download", Module: "backup", Type: "action"})
	}
	if _, ok := permissionRegistry["backup:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "backup:list", Name: "backup:list", Module: "backup", Type: "action"})
	}
	if _, ok := permissionRegistry["backup:restore"]; !ok {
		RegisterPermission(PermissionDef{Code: "backup:restore", Name: "backup:restore", Module: "backup", Type: "action"})
	}
	if _, ok := permissionRegistry["config:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "config:create", Name: "config:create", Module: "config", Type: "action"})
	}
	if _, ok := permissionRegistry["config:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "config:delete", Name: "config:delete", Module: "config", Type: "action"})
	}
	if _, ok := permissionRegistry["config:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "config:list", Name: "config:list", Module: "config", Type: "action"})
	}
	if _, ok := permissionRegistry["config:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "config:update", Name: "config:update", Module: "config", Type: "action"})
	}
	if _, ok := permissionRegistry["custom_field:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "custom_field:create", Name: "custom_field:create", Module: "custom_field", Type: "action"})
	}
	if _, ok := permissionRegistry["custom_field:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "custom_field:delete", Name: "custom_field:delete", Module: "custom_field", Type: "action"})
	}
	if _, ok := permissionRegistry["custom_field:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "custom_field:list", Name: "custom_field:list", Module: "custom_field", Type: "action"})
	}
	if _, ok := permissionRegistry["custom_field:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "custom_field:update", Name: "custom_field:update", Module: "custom_field", Type: "action"})
	}
	if _, ok := permissionRegistry["department:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "department:create", Name: "department:create", Module: "department", Type: "action"})
	}
	if _, ok := permissionRegistry["department:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "department:delete", Name: "department:delete", Module: "department", Type: "action"})
	}
	if _, ok := permissionRegistry["department:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "department:list", Name: "department:list", Module: "department", Type: "action"})
	}
	if _, ok := permissionRegistry["department:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "department:update", Name: "department:update", Module: "department", Type: "action"})
	}
	if _, ok := permissionRegistry["dict:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "dict:create", Name: "dict:create", Module: "dict", Type: "action"})
	}
	if _, ok := permissionRegistry["dict:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "dict:delete", Name: "dict:delete", Module: "dict", Type: "action"})
	}
	if _, ok := permissionRegistry["dict:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "dict:list", Name: "dict:list", Module: "dict", Type: "action"})
	}
	if _, ok := permissionRegistry["dict:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "dict:update", Name: "dict:update", Module: "dict", Type: "action"})
	}
	if _, ok := permissionRegistry["license:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "license:create", Name: "license:create", Module: "license", Type: "action"})
	}
	if _, ok := permissionRegistry["license:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "license:list", Name: "license:list", Module: "license", Type: "action"})
	}
	if _, ok := permissionRegistry["log:export"]; !ok {
		RegisterPermission(PermissionDef{Code: "log:export", Name: "log:export", Module: "log", Type: "action"})
	}
	if _, ok := permissionRegistry["log:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "log:list", Name: "log:list", Module: "log", Type: "action"})
	}
	if _, ok := permissionRegistry["ops:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "ops:list", Name: "ops:list", Module: "ops", Type: "action"})
	}
	if _, ok := permissionRegistry["overview:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "overview:list", Name: "overview:list", Module: "overview", Type: "action"})
	}
	if _, ok := permissionRegistry["project:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "project:create", Name: "project:create", Module: "project", Type: "action"})
	}
	if _, ok := permissionRegistry["project:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "project:delete", Name: "project:delete", Module: "project", Type: "action"})
	}
	if _, ok := permissionRegistry["project:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "project:list", Name: "project:list", Module: "project", Type: "action"})
	}
	if _, ok := permissionRegistry["project:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "project:update", Name: "project:update", Module: "project", Type: "action"})
	}
	if _, ok := permissionRegistry["role:assign"]; !ok {
		RegisterPermission(PermissionDef{Code: "role:assign", Name: "role:assign", Module: "role", Type: "action"})
	}
	if _, ok := permissionRegistry["role:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "role:create", Name: "role:create", Module: "role", Type: "action"})
	}
	if _, ok := permissionRegistry["role:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "role:delete", Name: "role:delete", Module: "role", Type: "action"})
	}
	if _, ok := permissionRegistry["role:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "role:list", Name: "role:list", Module: "role", Type: "action"})
	}
	if _, ok := permissionRegistry["role:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "role:update", Name: "role:update", Module: "role", Type: "action"})
	}
	if _, ok := permissionRegistry["schedule:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "schedule:create", Name: "schedule:create", Module: "schedule", Type: "action"})
	}
	if _, ok := permissionRegistry["schedule:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "schedule:delete", Name: "schedule:delete", Module: "schedule", Type: "action"})
	}
	if _, ok := permissionRegistry["schedule:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "schedule:list", Name: "schedule:list", Module: "schedule", Type: "action"})
	}
	if _, ok := permissionRegistry["schedule:run"]; !ok {
		RegisterPermission(PermissionDef{Code: "schedule:run", Name: "schedule:run", Module: "schedule", Type: "action"})
	}
	if _, ok := permissionRegistry["schedule:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "schedule:update", Name: "schedule:update", Module: "schedule", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:captcha"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:captcha", Name: "settings:captcha", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:cloud-storage"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:cloud-storage", Name: "settings:cloud-storage", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:email"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:email", Name: "settings:email", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:gateway"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:gateway", Name: "settings:gateway", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:general"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:general", Name: "settings:general", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:ldap"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:ldap", Name: "settings:ldap", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:license"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:license", Name: "settings:license", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:ntp"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:ntp", Name: "settings:ntp", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:oauth"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:oauth", Name: "settings:oauth", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:security"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:security", Name: "settings:security", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["settings:watermark"]; !ok {
		RegisterPermission(PermissionDef{Code: "settings:watermark", Name: "settings:watermark", Module: "settings", Type: "action"})
	}
	if _, ok := permissionRegistry["storage:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "storage:create", Name: "storage:create", Module: "storage", Type: "action"})
	}
	if _, ok := permissionRegistry["storage:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "storage:delete", Name: "storage:delete", Module: "storage", Type: "action"})
	}
	if _, ok := permissionRegistry["storage:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "storage:list", Name: "storage:list", Module: "storage", Type: "action"})
	}
	if _, ok := permissionRegistry["storage:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "storage:update", Name: "storage:update", Module: "storage", Type: "action"})
	}
	if _, ok := permissionRegistry["upgrade:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "upgrade:create", Name: "upgrade:create", Module: "upgrade", Type: "action"})
	}
	if _, ok := permissionRegistry["upgrade:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "upgrade:list", Name: "upgrade:list", Module: "upgrade", Type: "action"})
	}
	if _, ok := permissionRegistry["user:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "user:create", Name: "user:create", Module: "user", Type: "action"})
	}
	if _, ok := permissionRegistry["user:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "user:delete", Name: "user:delete", Module: "user", Type: "action"})
	}
	if _, ok := permissionRegistry["user:disable"]; !ok {
		RegisterPermission(PermissionDef{Code: "user:disable", Name: "user:disable", Module: "user", Type: "action"})
	}
	if _, ok := permissionRegistry["user:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "user:list", Name: "user:list", Module: "user", Type: "action"})
	}
	if _, ok := permissionRegistry["user:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "user:update", Name: "user:update", Module: "user", Type: "action"})
	}
	if _, ok := permissionRegistry["webhook:create"]; !ok {
		RegisterPermission(PermissionDef{Code: "webhook:create", Name: "webhook:create", Module: "webhook", Type: "action"})
	}
	if _, ok := permissionRegistry["webhook:delete"]; !ok {
		RegisterPermission(PermissionDef{Code: "webhook:delete", Name: "webhook:delete", Module: "webhook", Type: "action"})
	}
	if _, ok := permissionRegistry["webhook:list"]; !ok {
		RegisterPermission(PermissionDef{Code: "webhook:list", Name: "webhook:list", Module: "webhook", Type: "action"})
	}
	if _, ok := permissionRegistry["webhook:update"]; !ok {
		RegisterPermission(PermissionDef{Code: "webhook:update", Name: "webhook:update", Module: "webhook", Type: "action"})
	}
}
