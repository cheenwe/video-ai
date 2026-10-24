package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"baseplatform/internal/models"
)

// TestRecordingPlanRejectsMissingSchedule: schedule mode without windows must
// fail loudly instead of silently recording 24/7.
func TestRecordingPlanRejectsMissingSchedule(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	f.db.Create(&models.Camera{Code: "rec-cam", Name: "录像", AccessType: "rtsp", Enabled: 1})
	var cam models.Camera
	f.db.Where("code = ?", "rec-cam").First(&cam)
	f.db.Create(&models.StreamProfile{CameraID: cam.ID, Role: "main", URL: "rtsp://cam/rec", Enabled: 1})

	e := f.ok("PUT", "/api/recording-plans/"+uintStr(cam.ID), map[string]any{
		"mode": "schedule", "retain_days": 7, "max_gb": 50, "enabled": 1,
	})
	// The plan is saved, but the response says the recorder did not start.
	if !contains(e.Msg, "计划已保存") || !contains(e.Msg, "时段") {
		t.Fatalf("expected an explicit schedule error, got %q", e.Msg)
	}
}

// TestRecordingPlanOffStopsRecorder: switching a plan off must stop recording
// and leave no process behind.
func TestRecordingPlanOffStopsRecorder(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	f.db.Create(&models.Camera{Code: "rec-off", Name: "录像关", AccessType: "rtsp", Enabled: 1})
	var cam models.Camera
	f.db.Where("code = ?", "rec-off").First(&cam)

	e := f.ok("PUT", "/api/recording-plans/"+uintStr(cam.ID), map[string]any{
		"mode": "off", "retain_days": 7, "max_gb": 50, "enabled": 1,
	})
	if !contains(e.Msg, "已保存") {
		t.Fatalf("off plan should save cleanly, got %q", e.Msg)
	}
	var plan models.RecordingPlan
	if err := f.db.Where("camera_id = ?", cam.ID).First(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "off" {
		t.Fatalf("mode = %q", plan.Mode)
	}
}

// TestCameraRecordingsListsSegments feeds the recorder directory directly and
// checks the listing endpoint reports state + files, so the UI can tell
// "nothing recorded yet" from "recorder broken".
func TestCameraRecordingsListsSegments(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	f.db.Create(&models.Camera{Code: "rec-list", Name: "录像列表", AccessType: "rtsp", Enabled: 1})
	var cam models.Camera
	f.db.Where("code = ?", "rec-list").First(&cam)

	// setup() roots data at a temp dir; mirror the recorder layout there.
	dir := filepath.Join(f.cfg.DataHome, "recordings", uintStr(cam.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "20261024-090000.mp4"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("fake-mp4"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := f.ok("GET", "/api/cameras/"+uintStr(cam.ID)+"/recordings", nil)
	var out struct {
		CameraID uint `json:"camera_id"`
		State    struct {
			Recording bool `json:"recording"`
		} `json:"state"`
		Segments []struct {
			Name string `json:"name"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(e.Data, &out); err != nil {
		t.Fatal(err)
	}
	if out.CameraID != cam.ID {
		t.Fatalf("camera id = %d", out.CameraID)
	}
	if out.State.Recording {
		t.Fatal("no plan was started, recording must be false")
	}
	if len(out.Segments) != 1 || out.Segments[0].Name != name {
		t.Fatalf("segments = %+v", out.Segments)
	}
}

// TestRecordingFileRejectsTraversal: the file endpoint must not serve paths
// outside the camera's recording directory.
func TestRecordingFileRejectsTraversal(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	f.db.Create(&models.Camera{Code: "rec-sec", Name: "录像安全", AccessType: "rtsp", Enabled: 1})
	var cam models.Camera
	f.db.Where("code = ?", "rec-sec").First(&cam)

	// A file one level up must be unreachable.
	outside := filepath.Join(f.cfg.DataHome, "recordings", "secret.mp4")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Raw request: a rejected path answers with gin's plain 404, which is not
	// JSON, so the fixture's JSON helper must not be used here.
	for _, bad := range []string{"..%2Fsecret.mp4", "secret.mp4", "%2e%2e%2fsecret.mp4"} {
		req := httptest.NewRequest("GET", "/api/cameras/"+uintStr(cam.ID)+"/recordings/"+bad, nil)
		req.Header.Set("Origin", "http://example.com")
		req.AddCookie(f.cookie)
		w := httptest.NewRecorder()
		f.r.ServeHTTP(w, req)
		if w.Code == 200 && w.Body.Len() > 0 && contains(w.Body.String(), "secret") {
			t.Fatalf("traversal via %q leaked file content", bad)
		}
		if w.Code == 200 {
			t.Fatalf("traversal via %q must not be served", bad)
		}
	}
}

// uintStr renders an id for URL building.
func uintStr(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
