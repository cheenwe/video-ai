package videoai

import (
	"sync"
	"time"
)

// Update is one analysis tick broadcast to live subscribers. It carries the
// geometry overlay so the browser can draw the boundary and detections on top
// of the video instead of only showing a badge.
type Update struct {
	Type         string         `json:"type"` // analysis | event | runtime
	DeploymentID uint           `json:"deployment_id"`
	CameraID     uint           `json:"camera_id"`
	CameraName   string         `json:"camera_name,omitempty"`
	Result       string         `json:"result"`
	Rule         string         `json:"rule"`
	Level        string         `json:"level"`
	Distance     float64        `json:"distance"`
	Reason       string         `json:"reason"`
	Overlay      map[string]any `json:"overlay,omitempty"`
	EventID      uint           `json:"event_id,omitempty"`
	TS           time.Time      `json:"ts"`
}

// Hub fans out analysis updates to WebSocket subscribers. It keeps the latest
// update per deployment so a client that connects mid-stream is immediately in
// sync rather than staring at an empty panel.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan Update]struct{}
	last map[uint]Update
}

// NewHub builds an empty hub.
func NewHub() *Hub {
	return &Hub{subs: map[chan Update]struct{}{}, last: map[uint]Update{}}
}

// Publish broadcasts an update and remembers it as the deployment's latest.
func (h *Hub) Publish(u Update) {
	h.mu.Lock()
	if u.DeploymentID != 0 {
		h.last[u.DeploymentID] = u
	}
	h.fanout(u)
	h.mu.Unlock()
}

// PublishStatus broadcasts a status-only update (no geometry) while keeping
// the deployment's last known overlay. Paths that never reach inference —
// offline, waiting_video, out_of_schedule, fault — use it so live viewers see
// those transitions, and a client connecting afterwards still gets the last
// overlay to draw instead of a blank panel.
func (h *Hub) PublishStatus(u Update) {
	h.mu.Lock()
	if u.DeploymentID != 0 {
		if prev, ok := h.last[u.DeploymentID]; ok && u.Overlay == nil {
			u.Overlay = prev.Overlay
		}
		h.last[u.DeploymentID] = u
	}
	h.fanout(u)
	h.mu.Unlock()
}

// fanout delivers to every subscriber; slow clients drop frames rather than
// stalling analysis. Caller holds h.mu.
func (h *Hub) fanout(u Update) {
	for ch := range h.subs {
		select {
		case ch <- u:
		default:
			// A slow client drops frames rather than stalling analysis.
		}
	}
}

// Subscribe registers a consumer and returns its unsubscribe func.
func (h *Hub) Subscribe(buf int) (<-chan Update, func()) {
	if buf <= 0 {
		buf = 16
	}
	ch := make(chan Update, buf)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
}

// Latest returns the most recent update for every deployment.
func (h *Hub) Latest() []Update {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Update, 0, len(h.last))
	for _, u := range h.last {
		out = append(out, u)
	}
	return out
}

// SubscriberCount is exposed for self-check and ops.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
