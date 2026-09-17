package services

import (
	"gorm.io/gorm"
	"baseplatform/internal/models"
)

// EntityQuery is shared by detail, update and custom-field endpoints.
// Unknown resource modules are denied until an authorization adapter exists.
func (s *RBACService) EntityQuery(user models.User, module string, write bool) (*gorm.DB, bool) {
	action := "list"
	if write {
		action = "update"
	}
	switch module {
	case "user":
		q := s.DB.Model(&models.User{})
		if !s.HasPermission(user, "user:"+action) {
			return q.Where("id = ?", user.ID), true
		}
		return s.ApplyDataScope(q, user, ScopeOptions{UserIDColumn: "id", DeptIDColumn: "department_id"}), true
	case "department":
		if !s.HasPermission(user, "department:"+action) {
			return nil, false
		}
		return s.ApplyDataScope(s.DB.Model(&models.Department{}), user, ScopeOptions{DeptIDColumn: "id"}), true
	case "project":
		if !s.HasPermission(user, "project:"+action) {
			return nil, false
		}
		return s.ApplyDataScope(s.DB.Model(&models.Project{}), user, ScopeOptions{UserIDColumn: "owner_id", ProjectIDColumn: "id"}), true
	default:
		return nil, false
	}
}
func (s *RBACService) CanAccessEntity(user models.User, module string, id uint, write bool) bool {
	q, ok := s.EntityQuery(user, module, write)
	if !ok {
		return false
	}
	var count int64
	return q.Where("id = ?", id).Count(&count).Error == nil && count == 1
}
