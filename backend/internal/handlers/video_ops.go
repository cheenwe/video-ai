package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/videoai"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type VideoOpsHandler struct {
	DB       *gorm.DB
	DataHome string
	Runtime  *videoai.Manager
}

// SelfCheck runs platform-level video AI readiness checks (M5).
func (h *VideoOpsHandler) SelfCheck(c *gin.Context) {
	checks := []gin.H{}
	okAll := true
	add := func(name string, ok bool, detail string) {
		checks = append(checks, gin.H{"name": name, "ok": ok, "detail": detail})
		if !ok {
			okAll = false
		}
	}

	var camN, runN, boundN, tplN, pendingN int64
	h.DB.Model(&models.Camera{}).Where("archived = 0").Count(&camN)
	h.DB.Model(&models.Deployment{}).Where("status = ?", "running").Count(&runN)
	h.DB.Model(&models.BoundaryVersion{}).Where("status = ?", "published").Count(&boundN)
	h.DB.Model(&models.BusinessTemplate{}).Where("status = ?", "published").Count(&tplN)
	h.DB.Model(&models.VideoEvent{}).Where("handle_status = ?", "pending").Count(&pendingN)
	add("cameras", true, fmt.Sprintf("%d registered", camN))
	add("running_deployments", true, fmt.Sprintf("%d", runN))
	add("published_boundaries", true, fmt.Sprintf("%d", boundN))
	add("business_templates", tplN > 0, fmt.Sprintf("%d", tplN))
	add("pending_alerts", true, fmt.Sprintf("%d", pendingN))

	evid := filepath.Join(h.DataHome, "evidence")
	if err := os.MkdirAll(evid, 0o755); err != nil {
		add("evidence_dir", false, err.Error())
	} else {
		add("evidence_dir", true, evid)
	}
	add("runtime", h.Runtime != nil, "in-process mock manager")
	add("go_runtime", true, runtime.Version())
	add("os", true, runtime.GOOS+"/"+runtime.GOARCH)

	response.OK(c, gin.H{
		"ok": okAll, "checks": checks, "generated_at": time.Now(),
		"note": "四路长跑与断流恢复需在部署环境按 docs/video-ai.md 清单执行",
	}, "")
}
