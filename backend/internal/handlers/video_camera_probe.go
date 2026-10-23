package handlers

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// probeAgentCam is injected from server.go so the platform camera probe can
// consult the bound nvr-agent without importing the NVR client stack here.
type probeAgentCam func(agentID uint, agentCamID string) (found bool, live bool, err error)

// streamURL returns the configured URL for the camera's main (or first) stream.
func streamURL(db *gorm.DB, cameraID uint) string {
	var streams []models.StreamProfile
	db.Where("camera_id = ?", cameraID).Order("id asc").Find(&streams)
	for _, s := range streams {
		if s.Role == "main" && s.URL != "" {
			return s.URL
		}
	}
	for _, s := range streams {
		if s.URL != "" {
			return s.URL
		}
	}
	return ""
}

// probeRTSP dials the RTSP TCP endpoint (default port 554). It validates
// transport reachability, not a full DESCRIBE handshake.
func probeRTSP(raw string, timeout time.Duration) (time.Duration, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return 0, fmt.Errorf("码流地址无效")
	}
	host := u.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		port := "554"
		if u.Scheme == "rtsps" || u.Scheme == "https" {
			port = "322"
		}
		host = net.JoinHostPort(host, port)
	}
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(context.Background(), "tcp", host)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(start), nil
}

// Probe tests stream reachability for a platform camera and updates its
// device/media status accordingly (M1: RTSP/码流连接测试).
func (h *CameraHandler) Probe(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cam models.Camera
	if h.scopeCameras(c).First(&cam, id).Error != nil {
		response.BizFail(c, "摄像头不存在", 404)
		return
	}

	report := gin.H{"camera_id": cam.ID, "access_type": cam.AccessType}
	now := time.Now()
	mediaStatus, deviceStatus := "offline", "online"
	var lastErr string

	switch {
	case cam.AccessType == "nvr_agent" && h.ProbeAgent != nil && cam.NVRAgentID != nil && cam.AgentCameraID != "":
		found, live, err := h.ProbeAgent(*cam.NVRAgentID, cam.AgentCameraID)
		if err != nil {
			mediaStatus, lastErr = "offline", "NVR 节点不可达: "+err.Error()
		} else if !found {
			mediaStatus, lastErr = "no_frame", "NVR 节点上未找到通道 "+cam.AgentCameraID
		} else if !live {
			mediaStatus, lastErr = "no_frame", "通道已登记但预览拉流未运行"
		} else {
			mediaStatus = "online"
			report["detail"] = "agent 通道在线"
		}
	default:
		raw := streamURL(h.DB, cam.ID)
		if raw == "" {
			mediaStatus, lastErr = "unknown", "未配置码流地址"
		} else if cost, err := probeRTSP(raw, 5*time.Second); err != nil {
			mediaStatus, lastErr = "offline", "码流连接失败: "+err.Error()
		} else {
			mediaStatus = "online"
			report["detail"] = fmt.Sprintf("TCP 连接成功（%.0f ms）", float64(cost.Milliseconds()))
		}
		report["stream_url_host"] = maskStreamURL(streamURL(h.DB, cam.ID))
	}

	if mediaStatus == "offline" || mediaStatus == "no_frame" {
		deviceStatus = "offline"
	}
	report["media_status"] = mediaStatus
	report["ok"] = mediaStatus == "online"
	if lastErr != "" {
		report["error"] = lastErr
	}
	_ = h.DB.Model(&cam).Updates(map[string]any{
		"media_status":  mediaStatus,
		"device_status": deviceStatus,
		"last_error":    lastErr,
		"updated_at":    now,
	}).Error
	u := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", map[bool]string{true: "success", false: "failure"}[report["ok"] == true],
		"camera.probe", &u, fmt.Sprintf("cam=%d media=%s", cam.ID, mediaStatus))
	response.OK(c, report, "")
}

// maskStreamURL hides credentials before echoing a stream URL back.
func maskStreamURL(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		username := u.User.Username()
		if username != "" {
			u.User = url.User(strings.Repeat("*", len(username)))
		}
		return u.String()
	}
	return raw
}
