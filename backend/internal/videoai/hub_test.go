package videoai

import (
	"fmt"
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
		TS: time.Now(),
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

// BenchmarkHubPublishWithSubscribers measures fan-out cost. The hub encodes
// once per update, so adding subscribers must not multiply the JSON work.
func BenchmarkHubPublishWithSubscribers(b *testing.B) {
	for _, subs := range []int{0, 1, 8, 16} {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			h := NewHub()
			drain := make([]func(), 0, subs)
			for i := 0; i < subs; i++ {
				ch, unsub := h.Subscribe(64)
				drain = append(drain, unsub)
				go func() {
					for range ch {
					}
				}()
			}
			u := Update{
				Type: "analysis", DeploymentID: 1, CameraID: 1, CameraName: "cam",
				Result: "near", Rule: "grab_near_edge", Level: "warning",
				Reason: "接近边界", TS: time.Now(),
				Overlay: map[string]any{"ref_width": 1280.0, "ref_height": 720.0, "adapter": "onnx"},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.Publish(u)
			}
			b.StopTimer()
			for _, f := range drain {
				f()
			}
		})
	}
}
