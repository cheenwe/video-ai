package server

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"baseplatform/internal/models"
)

func TestVideoAICameraBoundaryDeploymentFlow(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e := f.ok("POST", "/api/cameras", map[string]any{
		"code": "cam-demo-1", "name": "Demo Cam", "access_type": "rtsp", "main_url": "rtsp://example/1",
	})
	var cam struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(e.Data, &cam)
	if cam.ID == 0 {
		t.Fatal("camera id missing")
	}

	e = f.ok("POST", "/api/boundaries", map[string]any{
		"camera_id": cam.ID, "name": "edge", "ref_width": 1280, "ref_height": 720,
		"geometry": map[string]any{
			"kind": "polyline", "pit_side": "below", "buffer_norm": 0.02,
			"points": []map[string]float64{{"x": 100, "y": 400}, {"x": 640, "y": 420}, {"x": 1180, "y": 400}},
		},
	})
	var bound struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(e.Data, &bound)

	f.ok("POST", fmt.Sprintf("/api/boundaries/%d/publish", bound.ID), nil)

	var tpl models.BusinessTemplate
	if err := f.db.Where("code = ?", "grab_edge_guard").First(&tpl).Error; err != nil {
		t.Fatal(err)
	}

	e = f.ok("POST", "/api/deployments", map[string]any{
		"name": "demo", "camera_id": cam.ID, "template_id": tpl.ID, "boundary_version_id": bound.ID,
	})
	var dep struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(e.Data, &dep)

	f.ok("POST", fmt.Sprintf("/api/deployments/%d/start", dep.ID), nil)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		f.db.Model(&models.VideoEvent{}).Where("camera_id = ?", cam.ID).Count(&n)
		if n > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	var n int64
	f.db.Model(&models.VideoEvent{}).Where("camera_id = ?", cam.ID).Count(&n)
	if n == 0 {
		t.Fatal("expected video event from mock runtime")
	}

	f.ok("GET", "/api/reports/summary", nil)
	f.ok("GET", "/api/video/self-check", nil)
	f.ok("POST", "/api/access-devices", map[string]any{
		"kind": "gb28181", "code": "gb-1", "name": "国标设备",
		"config": map[string]any{"sip_id": "34020000001320000001"},
	})

	// Cleanup runtime goroutine for this deployment.
	f.ok("POST", fmt.Sprintf("/api/deployments/%d/stop", dep.ID), nil)
}
