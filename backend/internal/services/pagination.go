package services

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"baseplatform/internal/settings"
)

const (
	ConfigCategorySystem = "system"
	ConfigCodePageSize   = "page_size"
	DefaultPageSize      = 20
	MaxPageSize          = 200
)

func PageSizeFromDB(db *gorm.DB) int {
	n := settings.LoadGeneral(db).PageSize
	if n < 1 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return n
}

func ParsePagination(c *gin.Context, db *gorm.DB) (page int, pageSize int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	defaultSize := PageSizeFromDB(db)
	if raw := c.Query("page_size"); raw != "" {
		pageSize, _ = strconv.Atoi(raw)
	} else {
		pageSize = defaultSize
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}

func PaginateResult(items any, total int64, page, pageSize int) gin.H {
	return gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}
}
