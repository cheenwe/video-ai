package videoai

import (
	"testing"
	"time"
)

// TestHubPublishStatusKeepsOverlay locks the live-push contract for the paths
// that never run inference (offline / waiting_video / out_of_schedule): the
// status update must reach subscribers and must not wipe the last overlay a
// late joiner would replay.
func TestHubPublishStatusKeepsOverlay(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(8)
	defer unsub()

	rich := Update{
		Type: "analysis", DeploymentID: 7, CameraID: 3,
		Result: "near", Overlay: map[string]any{"ref_width": 1280.0},
		TS:     time.Now(),
	}
	h.Publish(rich)
	if got := <-ch; got.Result != "near" || got.Overlay == nil {
		t.Fatalf("rich publish not fanned out: %+v", got)
	}

	// A status-only update replaces result but keeps the overlay.
	h.PublishStatus(Update{Type: "analysis", DeploymentID: 7, CameraID: 3, Result: "offline", Reason: "等待视频帧", TS: time.Now()})
	got := <-ch
	if got.Result != "offline" || got.Overlay == nil || got.Overlay["ref_width"] != 1280.0 {
		t.Fatalf("status publish dropped the overlay: %+v", got)
	}

	// A client connecting afterwards still gets the geometry back.
	last := h.Latest()
	if len(last) != 1 {
		t.Fatalf("latest entries: %d", len(last))
	}
	if last[0].Result != "offline" || last[0].Overlay == nil {
		t.Fatalf("latest replay lost state: %+v", last[0])
	}

	// Status-only with no history stays overlay-free.
	h.PublishStatus(Update{Type: "analysis", DeploymentID: 9, CameraID: 4, Result: "unknown", TS: time.Now()})
	got = <-ch
	if got.Overlay != nil {
		t.Fatalf("unexpected overlay on first status: %+v", got)
	}
}
