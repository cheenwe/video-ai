package handlers

import (
	"fmt"
	"io"
	"strings"

	"baseplatform/internal/license"
	"baseplatform/internal/middleware"
	"baseplatform/internal/response"

	"github.com/gin-gonic/gin"
)

type SiteInfoHandler struct {
	License *license.Service
}

func (h *SiteInfoHandler) Info(c *gin.Context) {
	response.OK(c, h.License.SiteInfo(), "")
}

type LicenseHandler struct {
	License *license.Service
}

func (h *LicenseHandler) Status(c *gin.Context) {
	response.OK(c, h.License.Meta(), "")
}

func (h *LicenseHandler) Fingerprint(c *gin.Context) {
	response.OK(c, h.License.Fingerprint(), "")
}

func (h *LicenseHandler) Import(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		var req struct {
			Content string `json:"content"`
		}
		if c.ShouldBindJSON(&req) == nil && req.Content != "" {
			key := c.ClientIP() + ":" + clientUserID(c)
			if err := h.License.Import([]byte(req.Content), key); err != nil {
				response.BizFail(c, err.Error(), 403)
				return
			}
			response.OK(c, h.License.Meta(), "授权已导入")
			return
		}
		response.BizFail(c, "缺少授权文件", 400)
		return
	}
	f, err := file.Open()
	if err != nil {
		response.BizFail(c, "读取文件失败", 400)
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		response.BizFail(c, "读取文件失败", 400)
		return
	}
	key := c.ClientIP() + ":" + clientUserID(c)
	if err := h.License.Import(raw, key); err != nil {
		response.BizFail(c, err.Error(), 403)
		return
	}
	response.OK(c, h.License.Meta(), "授权已导入")
}

func (h *LicenseHandler) Activate(c *gin.Context) {
	var req struct {
		LicenseServerURL string `json:"license_server_url"`
		LicenseID       string `json:"license_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if err := h.License.Activate(req.LicenseServerURL, req.LicenseID); err != nil {
		response.BizFail(c, err.Error(), 403)
		return
	}
	response.OK(c, h.License.Meta(), "激活成功")
}

func (h *LicenseHandler) Sync(c *gin.Context) {
	msg, err := h.License.SyncLicense()
	if err != nil {
		response.BizFail(c, err.Error(), 500)
		return
	}
	response.OK(c, h.License.Meta(), msg)
}

func (h *LicenseHandler) Remove(c *gin.Context) {
	if err := h.License.Remove(); err != nil {
		response.BizFail(c, err.Error(), 500)
		return
	}
	response.OK(c, h.License.Meta(), "授权已移除")
}

func (h *LicenseHandler) TrialApply(c *gin.Context) {
	var req struct {
		LicenseServerURL string `json:"license_server_url"`
		TeamName         string `json:"team_name"`
		Contact          string `json:"contact"`
		Edition          string `json:"edition"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	res, err := h.License.ApplyTrial(req.LicenseServerURL, req.TeamName, req.Contact, req.Edition)
	if err != nil {
		response.BizFail(c, err.Error(), 403)
		return
	}
	response.OK(c, res, "申请已提交")
}

func (h *LicenseHandler) TrialStatus(c *gin.Context) {
	serverURL := strings.TrimSpace(c.Query("license_server_url"))
	if serverURL == "" {
		var req struct {
			LicenseServerURL string `json:"license_server_url"`
		}
		_ = c.ShouldBindJSON(&req)
		serverURL = strings.TrimSpace(req.LicenseServerURL)
	}
	res, err := h.License.TrialStatus(serverURL)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	response.OK(c, res, "ok")
}

func clientUserID(c *gin.Context) string {
	u := middleware.CurrentUser(c)
	if u.ID == 0 {
		return "0"
	}
	return fmt.Sprintf("%d", u.ID)
}
