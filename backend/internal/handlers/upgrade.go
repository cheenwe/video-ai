package handlers

import (
	"baseplatform/internal/response"
	"baseplatform/internal/upgrade"
	"github.com/gin-gonic/gin"
)

type UpgradeHandler struct {
	Upgrade *upgrade.Service
}

func (h *UpgradeHandler) Check(c *gin.Context) {
	m, has, err := h.Upgrade.Check()
	if err != nil {
		response.BizFail(c, err.Error(), 500)
		return
	}
	response.OK(c, gin.H{
		"current_version": h.Upgrade.Version,
		"has_update":      has,
		"manifest":        m,
	}, "")
}

func (h *UpgradeHandler) Status(c *gin.Context) {
	response.OK(c, h.Upgrade.Status(), "")
}

func (h *UpgradeHandler) Apply(c *gin.Context) {
	if err := h.Upgrade.Apply(); err != nil {
		response.BizFail(c, err.Error(), 500)
		return
	}
	response.OK(c, h.Upgrade.Status(), "升级已启动")
}
