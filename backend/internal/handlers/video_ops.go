package handlers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// IngestStates reports every active puller for the ops/live views.
func (h *VideoOpsHandler) IngestStates(c *gin.Context) {
	if h.Runtime == nil || h.Runtime.Ingest == nil {
		response.OK(c, []any{}, "")
		return
	}
	response.OK(c, h.Runtime.Ingest.States(), "")
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
	add("runtime", h.Runtime != nil, "in-process analysis manager")

	// Report the adapters actually registered, instead of assuming mock.
	adapters := videoai.RegisteredAdapters()
	names := make([]string, 0, len(adapters))
	demoOnly := true
	for _, a := range adapters {
		if n, ok := a["name"].(string); ok {
			names = append(names, n)
		}
		if d, ok := a["demo"].(bool); ok && !d {
			demoOnly = false
		}
	}
	add("adapters", len(adapters) > 0, strings.Join(names, ","))
	add("demo_only", true, fmt.Sprintf("demo_only=%v", demoOnly))

	// Frame ingest drives real analysis; report whether the transcoder is present.
	if ffmpegPath, err := exec.LookPath("ffmpeg"); err != nil {
		add("ffmpeg", false, "未找到 ffmpeg，证据截图/片段与直连预览转码不可用")
	} else {
		add("ffmpeg", true, ffmpegPath)
	}

	// Frame ingest is what separates real analysis from a demo loop.
	if h.Runtime != nil && h.Runtime.Ingest != nil {
		states := h.Runtime.Ingest.States()
		live := 0
		for _, st := range states {
			if st.Connected {
				live++
			}
		}
		add("ingest", true, fmt.Sprintf("%d 路取流，%d 路已连通", len(states), live))
		if h.Runtime.Hub != nil {
			add("live_push", true, fmt.Sprintf("WebSocket 订阅者 %d", h.Runtime.Hub.SubscriberCount()))
		}
	} else {
		add("ingest", false, "未启用取流，分析不读取真实视频帧")
	}

	add("go_runtime", true, runtime.Version())
	add("os", true, runtime.GOOS+"/"+runtime.GOARCH)

	response.OK(c, gin.H{
		"ok": okAll, "checks": checks, "generated_at": time.Now(),
		"adapters": adapters,
		"note":     "四路长跑与断流恢复需在部署环境按 docs/video-ai.md 清单执行",
	}, "")
}
