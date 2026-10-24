package server

import (
	"encoding/json"
	"testing"
	"time"

	"baseplatform/internal/models"
)

// TestReportSummaryCountsMatchFilters locks the consolidated summary query:
// collapsing five COUNTs into one scan must not change any number. The
// date-filtered counters (total/confirmed/false positive) and the
// unfiltered ones (pending/active) have different guards, so both are checked.
func TestReportSummaryCountsMatchFilters(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	var cam models.Camera
	f.db.Create(&models.Camera{Code: "sum-cam", Name: "汇总", AccessType: "rtsp", Enabled: 1})
	f.db.Where("code = ?", "sum-cam").First(&cam)

	recent := time.Now().Add(-24 * time.Hour)
	old := time.Now().Add(-60 * 24 * time.Hour)

	mk := func(key string, started time.Time, handle, scene, conclusion string) {
		f.db.Create(&models.VideoEvent{
			EventKey: key, CameraID: cam.ID, Category: "grab_near_edge", Level: "warning",
			HandleStatus: handle, SceneStatus: scene, Conclusion: conclusion,
			StartedAt: started,
		})
	}
	mk("sum-1", recent, "confirmed", "recovered", "real")        // recent + confirmed
	mk("sum-2", recent, "pending", "active", "")                 // recent + pending + active
	mk("sum-3", recent, "closed", "recovered", "false_positive") // recent + false positive
	mk("sum-4", old, "confirmed", "recovered", "real")           // outside the 7-day window
	mk("sum-5", old, "pending", "active", "")                    // old, still pending/active

	e := f.ok("GET", "/api/reports/summary", nil)
	var out struct {
		EventTotal    int64 `json:"event_total"`
		Confirmed     int64 `json:"confirmed"`
		FalsePositive int64 `json:"false_positive"`
		Pending       int64 `json:"pending"`
		ActiveScene   int64 `json:"active_scene"`
	}
	if err := json.Unmarshal(e.Data, &out); err != nil {
		t.Fatal(err)
	}
	// In-range: sum-1..3.
	if out.EventTotal != 3 {
		t.Fatalf("event_total = %d, want 3", out.EventTotal)
	}
	// In-range confirmed: sum-1 only (sum-4 is out of range).
	if out.Confirmed != 1 {
		t.Fatalf("confirmed = %d, want 1", out.Confirmed)
	}
	// In-range false positive: sum-3 only.
	if out.FalsePositive != 1 {
		t.Fatalf("false_positive = %d, want 1", out.FalsePositive)
	}
	// Pending is not date filtered: sum-2 and sum-5.
	if out.Pending != 2 {
		t.Fatalf("pending = %d, want 2", out.Pending)
	}
	// Active scene is not date filtered: sum-2 and sum-5.
	if out.ActiveScene != 2 {
		t.Fatalf("active_scene = %d, want 2", out.ActiveScene)
	}
}
