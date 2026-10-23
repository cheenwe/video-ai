package handlers

import (
	"net/http"
	"time"

	"baseplatform/internal/middleware"
	"baseplatform/internal/videoai"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// VideoWSHandler streams live analysis updates over WebSocket.
type VideoWSHandler struct {
	Runtime *videoai.Manager
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Origin is already constrained by the CORS middleware.
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	wsWriteWait  = 10 * time.Second
	wsPingPeriod = 25 * time.Second
)

// Stream upgrades the connection and pushes analysis updates. The caller is
// expected to wrap this with RequirePermission so only authorised viewers
// receive a live feed.
func (h *VideoWSHandler) Stream(c *gin.Context) {
	if h.Runtime == nil || h.Runtime.Hub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "实时推送未启用"})
		return
	}
	if middleware.CurrentUser(c).ID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch, unsub := h.Runtime.Hub.Subscribe(32)
	defer unsub()

	// Optional camera filter: ?camera_id=1,2
	var allow map[uint]bool
	if raw := c.Query("camera_id"); raw != "" {
		allow = map[uint]bool{}
		for _, id := range parseInts(raw) {
			allow[id] = true
		}
	}

	// Replay the latest state so a new panel is not blank.
	for _, u := range h.Runtime.Hub.Latest() {
		if allow != nil && !allow[u.CameraID] {
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
		if err := conn.WriteJSON(u); err != nil {
			return
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Reading keeps the connection alive and detects client disconnects.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case u, ok := <-ch:
			if !ok {
				return
			}
			if allow != nil && !allow[u.CameraID] {
				continue
			}
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteJSON(u); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// parseInts parses a comma separated list of ids.
func parseInts(raw string) []uint {
	out := []uint{}
	cur := uint(0)
	seen := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c >= '0' && c <= '9' {
			cur = cur*10 + uint(c-'0')
			seen = true
			continue
		}
		if seen {
			out = append(out, cur)
			cur, seen = 0, false
		}
	}
	if seen {
		out = append(out, cur)
	}
	return out
}
