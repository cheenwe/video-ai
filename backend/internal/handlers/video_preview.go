package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/response"

	"github.com/gin-gonic/gin"
)

// maxPreviewSessions caps concurrent MJPEG viewers. Every direct-RTSP panel
// keeps an ffmpeg decode subprocess alive, so an unbounded number of streams
// would exhaust the host. Override with BASE_VIDEO_AI_MAX_PREVIEW.
func maxPreviewSessions() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("BASE_VIDEO_AI_MAX_PREVIEW"))); err == nil && v > 0 {
		return v
	}
	return 32
}

var activePreviewSessions int64

// acquirePreviewSession reserves a viewer slot only while under the cap, so a
// rejected attempt must not leak a count (that would block later acquires).
func acquirePreviewSession() bool {
	cap := int64(maxPreviewSessions())
	for {
		cur := atomic.LoadInt64(&activePreviewSessions)
		if cur >= cap {
			return false
		}
		if atomic.CompareAndSwapInt64(&activePreviewSessions, cur, cur+1) {
			return true
		}
	}
}

func releasePreviewSession() {
	atomic.AddInt64(&activePreviewSessions, -1)
}

// ensureIngest makes sure a puller exists for the camera and returns the URL
// it is using, so direct-RTSP cameras can be previewed without an NVR agent.
func (h *CameraHandler) ensureIngest(camID uint) (string, error) {
	if h.Ingest == nil {
		return "", fmt.Errorf("服务端未启用取流")
	}
	u := streamURL(h.DB, camID)
	if u == "" {
		return "", fmt.Errorf("该摄像头未配置码流地址")
	}
	if err := h.Ingest.Ensure(camID, u); err != nil {
		return "", err
	}
	return u, nil
}

// Frame serves the latest decoded frame as a single JPEG.
func (h *CameraHandler) Frame(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cam models.Camera
	if h.scopeCameras(c).First(&cam, id).Error != nil {
		response.BizFail(c, "摄像头不存在", http.StatusNotFound)
		return
	}
	if _, err := h.ensureIngest(cam.ID); err != nil {
		response.BizFail(c, err.Error(), http.StatusBadRequest)
		return
	}
	// Give a freshly started puller a moment to deliver its first frame.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if f, ok := h.Ingest.Latest(cam.ID); ok {
			c.Header("Cache-Control", "no-store")
			c.Data(http.StatusOK, "image/jpeg", f.JPEG)
			return
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if st, ok := h.Ingest.State(cam.ID); ok && st.LastError != "" {
		response.BizFail(c, "暂无画面："+st.LastError, http.StatusServiceUnavailable)
		return
	}
	response.BizFail(c, "暂无画面：等待取流首帧", http.StatusServiceUnavailable)
}

// MJPEG streams decoded frames as multipart motion-JPEG, which browsers
// render directly in an <img>. This is the preview path for cameras that are
// not bound to an NVR agent.
const mjpegBoundary = "--frame"

// keepalivePart is an empty multipart part, sent on idle streams.
const keepalivePart = "--" + mjpegBoundary + "\r\n\r\n"

func (h *CameraHandler) MJPEG(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cam models.Camera
	if h.scopeCameras(c).First(&cam, id).Error != nil {
		response.BizFail(c, "摄像头不存在", http.StatusNotFound)
		return
	}
	if _, err := h.ensureIngest(cam.ID); err != nil {
		response.BizFail(c, err.Error(), http.StatusBadRequest)
		return
	}
	if !acquirePreviewSession() {
		response.BizFail(c, fmt.Sprintf("预览路数已达上限（%d），请关闭部分画面", maxPreviewSessions()), http.StatusServiceUnavailable)
		return
	}
	defer releasePreviewSession()
	ch, unsub := h.Ingest.Subscribe(cam.ID, 4)
	if ch == nil {
		response.BizFail(c, "取流未启动", http.StatusServiceUnavailable)
		return
	}
	defer unsub()

	c.Header("Content-Type", "multipart/x-mixed-replace; boundary="+mjpegBoundary)
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	w := c.Writer
	// Written directly rather than with fmt.Fprintf: the multipart header is
	// emitted for every frame of every open panel, and Fprintf reparses its
	// format string through reflection each time.
	header := "--" + mjpegBoundary + "\r\nContent-Type: image/jpeg\r\nContent-Length: "
	writePart := func(jpeg []byte) bool {
		if _, err := w.WriteString(header); err != nil {
			return false
		}
		if _, err := w.WriteString(strconv.Itoa(len(jpeg))); err != nil {
			return false
		}
		if _, err := w.WriteString("\r\n\r\n"); err != nil {
			return false
		}
		if _, err := w.Write(jpeg); err != nil {
			return false
		}
		if _, err := w.WriteString("\r\n"); err != nil {
			return false
		}
		w.Flush()
		return true
	}

	// Emit the current frame immediately so the panel is not blank.
	if f, ok := h.Ingest.Latest(cam.ID); ok {
		if !writePart(f.JPEG) {
			return
		}
	}

	ctx := c.Request.Context()
	keepalive := time.NewTicker(10 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case f, ok := <-ch:
			if !ok {
				return
			}
			if !writePart(f.JPEG) {
				return
			}
		case <-keepalive.C:
			// Comment line keeps intermediaries from closing an idle stream.
			if _, err := w.WriteString(keepalivePart); err != nil {
				return
			}
			w.Flush()
		}
	}
}

// IngestState reports puller liveness for the UI.
func (h *CameraHandler) IngestState(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if h.Ingest == nil {
		response.OK(c, gin.H{"enabled": false}, "")
		return
	}
	if st, ok := h.Ingest.State(uint(id)); ok {
		response.OK(c, gin.H{"enabled": true, "state": st}, "")
		return
	}
	u := streamURL(h.DB, uint(id))
	response.OK(c, gin.H{"enabled": true, "running": false, "has_stream": u != ""}, "")
}
