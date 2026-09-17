package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"baseplatform/internal/middleware"
	"baseplatform/internal/response"
	"baseplatform/internal/services"
)

type OpsHandler struct {
	DB     *gorm.DB
	Backup *services.OpsBackupService
}

func (h *OpsHandler) List(c *gin.Context) {
	list, err := h.Backup.List()
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	response.OK(c, gin.H{"items": list}, "")
}

func (h *OpsHandler) Status(c *gin.Context) {
	response.OK(c, h.Backup.Status(), "")
}

func (h *OpsHandler) Create(c *gin.Context) {
	manifest, err := h.Backup.Create()
	if err != nil {
		response.BizFail(c, err.Error(), 409)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "ops.backup.create", &me, manifest.ID)
	response.OK(c, manifest, "备份已创建")
}

func (h *OpsHandler) Download(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BizFail(c, "缺少备份 ID", 400)
		return
	}
	path, err := h.Backup.ArchivePath(id)
	if err != nil {
		response.BizFail(c, "备份文件不存在", 404)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "ops.backup.download", &me, id)
	c.Header("Content-Disposition", "attachment; filename=baseplatform-"+id+".tar.gz")
	c.File(path)
}

func (h *OpsHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BizFail(c, "缺少备份 ID", 400)
		return
	}
	if err := h.Backup.Delete(id); err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "ops.backup.delete", &me, id)
	response.OK(c, nil, "已删除")
}

func (h *OpsHandler) Restore(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BizFail(c, "缺少备份 ID", 400)
		return
	}
	if err := h.Backup.Restore(id); err != nil {
		response.BizFail(c, err.Error(), 409)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "ops.backup.restore", &me, id)
	response.OK(c, h.Backup.Status(), "恢复目录已准备，需停止服务后切换数据目录")
}

// NTPQuery 手动触发 NTP 查询（不修改系统时间）。
func (h *OpsHandler) NTPQuery(c *gin.Context) {
	server := strings.TrimSpace(c.Query("server"))
	if server == "" {
		server = "ntp.aliyun.com"
	}
	res, err := services.QueryNTP(server)
	if err != nil {
		response.Fail(c, http.StatusServiceUnavailable, err.Error(), 503)
		return
	}
	response.OK(c, res, "")
}
