package services

import (
	"baseplatform/internal/models"

	"gorm.io/gorm"
)

// ScopeOptions 指定资源表中用于数据范围过滤的列名。
// 不同资源表的列名不同（如 user_id / created_by / department_id 等），
// 由调用方按需传入，空字符串表示该维度不适用。
type ScopeOptions struct {
	UserIDColumn    string // 所属用户列，如 "user_id"、"created_by"
	DeptIDColumn    string // 所属部门列，如 "department_id"
	ProjectIDColumn string // 所属项目列，如 "project_id"
}

// scopeRank 数据范围权限优先级，数值越大权限越大。
var scopeRank = map[string]int{
	models.DataScopeSelf:       1,
	models.DataScopeCustom:     2,
	models.DataScopeProject:    3,
	models.DataScopeDept:       4,
	models.DataScopeDeptAndSub: 5,
	models.DataScopeAll:        6,
}

// EffectiveDataScope 返回用户所有角色中权限最大的数据范围。
func (s *RBACService) EffectiveDataScope(user models.User) string {
	if user.Role == models.RoleSuperAdmin {
		return models.DataScopeAll
	}
	roleIDs, err := s.userRoleIDs(user)
	if err != nil || len(roleIDs) == 0 {
		return models.DataScopeSelf
	}
	var roles []models.Role
	if err := s.DB.Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
		return models.DataScopeSelf
	}
	best := models.DataScopeSelf
	bestRank := scopeRank[best]
	for _, r := range roles {
		if rank, ok := scopeRank[r.DataScope]; ok && rank > bestRank {
			best = r.DataScope
			bestRank = rank
		}
	}
	return best
}

// subDeptIDs 返回指定部门及其所有下级部门 ID（递归）。
func (s *RBACService) subDeptIDs(deptID uint) ([]uint, error) {
	ids := []uint{}
	seen := map[uint]bool{}
	queue := []uint{deptID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		var children []uint
		if err := s.DB.Model(&models.Department{}).Where("parent_id = ?", id).Pluck("id", &children).Error; err != nil {
			return nil, err
		}
		queue = append(queue, children...)
	}
	return ids, nil
}

// userProjectIDs 返回用户作为成员的所有项目 ID。
func (s *RBACService) userProjectIDs(userID uint) ([]uint, error) {
	var members []models.ProjectMember
	if err := s.DB.Where("user_id = ?", userID).Find(&members).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.ProjectID)
	}
	return ids, nil
}

// ApplyDataScope 根据用户的数据范围为查询添加 WHERE 条件。
// 返回修改后的查询，调用方可继续链式调用。
// 对于 superadmin（all 范围），不添加任何条件。
// Roles grant a union of applicable scopes; missing dimensions fail closed.
func (s *RBACService) ApplyDataScope(db *gorm.DB, user models.User, opts ScopeOptions) *gorm.DB {
	if user.IsSuperAdmin() {
		return db
	}
	ids, err := s.userRoleIDs(user)
	if err != nil || len(ids) == 0 {
		return db.Where("1 = 0")
	}
	var roles []models.Role
	if s.DB.Where("id IN ? AND enabled = ?", ids, 1).Find(&roles).Error != nil {
		return db.Where("1 = 0")
	}
	predicates := s.DB.Session(&gorm.Session{NewDB: true}).Where("1 = 0")
	for _, role := range roles {
		switch role.DataScope {
		case models.DataScopeAll:
			return db
		case models.DataScopeSelf:
			if opts.UserIDColumn != "" {
				predicates = predicates.Or(opts.UserIDColumn+" = ?", user.ID)
			}
		case models.DataScopeDept, models.DataScopeDeptAndSub:
			if opts.DeptIDColumn != "" && user.DepartmentID != nil {
				depts := []uint{*user.DepartmentID}
				if role.DataScope == models.DataScopeDeptAndSub {
					depts, err = s.subDeptIDs(*user.DepartmentID)
					if err != nil {
						continue
					}
				}
				predicates = predicates.Or(opts.DeptIDColumn+" IN ?", depts)
			}
		case models.DataScopeProject:
			if opts.ProjectIDColumn != "" {
				projects, e := s.userProjectIDs(user.ID)
				if e == nil && len(projects) > 0 {
					predicates = predicates.Or(opts.ProjectIDColumn+" IN ?", projects)
				}
			}
		case models.DataScopeCustom:
			// 未实现自定义部门成员表；显式忽略，保持 fail-closed（不扩大权限）。
		}
	}
	return db.Where(predicates)
}
