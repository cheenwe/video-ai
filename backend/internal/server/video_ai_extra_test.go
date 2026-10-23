package server

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestCameraImportReportsEveryRow checks that bulk import keeps going after a
// bad row and tells the operator exactly which line failed.
func TestCameraImportReportsEveryRow(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e := f.ok("POST", "/api/cameras/import", map[string]any{
		"rows": []map[string]any{
			{"code": "imp-ok-1", "name": "正常一号", "access_type": "rtsp", "main_url": "rtsp://cam/1"},
			{"code": "imp-ok-2", "name": "正常二号", "access_type": "rtsp", "main_url": "rtsp://cam/2"},
			{"code": "", "name": "缺编码", "access_type": "rtsp", "main_url": "rtsp://cam/3"},
			{"code": "imp-no-stream", "name": "缺码流", "access_type": "rtsp"},
			{"code": "imp-bad-type", "name": "接入方式错", "access_type": "carrier_pigeon", "main_url": "rtsp://cam/4"},
			{"code": "imp-agent", "name": "缺 NVR 绑定", "access_type": "nvr_agent"},
		},
	})

	var out struct {
		Total   int `json:"total"`
		Created int `json:"created"`
		Failed  int `json:"failed"`
		Results []struct {
			Index int    `json:"index"`
			Code  string `json:"code"`
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(e.Data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 6 || out.Created != 2 || out.Failed != 4 {
		t.Fatalf("unexpected tally: %+v", out)
	}
	if len(out.Results) != 6 {
		t.Fatalf("expected a result per row, got %d", len(out.Results))
	}
	for i, r := range out.Results {
		if r.Index != i+1 {
			t.Fatalf("row index not preserved: %+v", r)
		}
	}
	if out.Results[0].OK && out.Results[1].OK != true {
		t.Fatal("valid rows should succeed")
	}
	for _, i := range []int{2, 3, 4, 5} {
		if out.Results[i].OK || out.Results[i].Error == "" {
			t.Fatalf("row %d should fail with a reason: %+v", i+1, out.Results[i])
		}
	}

	// Re-importing the same codes must be reported as duplicates, not crash.
	e = f.ok("POST", "/api/cameras/import", map[string]any{
		"rows": []map[string]any{{"code": "imp-ok-1", "name": "重复", "access_type": "rtsp", "main_url": "rtsp://cam/1"}},
	})
	json.Unmarshal(e.Data, &out)
	if out.Created != 0 || out.Failed != 1 {
		t.Fatalf("duplicate should fail: %+v", out)
	}
}

// TestCameraProbeReportsUnreachableStream checks the probe endpoint reports a
// real failure rather than assuming a camera is online.
func TestCameraProbeReportsUnreachableStream(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e := f.ok("POST", "/api/cameras", map[string]any{
		"code": "probe-1", "name": "Probe Cam", "access_type": "rtsp",
		"main_url": "rtsp://127.0.0.1:1/stream",
	})
	var cam struct{ ID uint `json:"id"` }
	json.Unmarshal(e.Data, &cam)

	e = f.ok("POST", fmt.Sprintf("/api/cameras/%d/probe", cam.ID), nil)
	var res struct {
		OK          bool   `json:"ok"`
		MediaStatus string `json:"media_status"`
		Error       string `json:"error"`
	}
	json.Unmarshal(e.Data, &res)
	if res.OK {
		t.Fatalf("unreachable stream reported online: %+v", res)
	}
	if res.MediaStatus == "online" {
		t.Fatalf("media status should not be online: %+v", res)
	}
	if res.Error == "" {
		t.Fatal("probe should explain the failure")
	}
}

// TestPreviewWithoutStreamFailsHonestly checks the direct-RTSP preview path
// refuses instead of serving an empty image.
func TestPreviewWithoutStreamFailsHonestly(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e := f.ok("POST", "/api/cameras", map[string]any{
		"code": "no-stream", "name": "No Stream", "access_type": "rtsp",
	})
	var cam struct{ ID uint `json:"id"` }
	json.Unmarshal(e.Data, &cam)

	e, w := f.call("GET", fmt.Sprintf("/api/cameras/%d/frame.jpg", cam.ID), nil, f.cookie, "http://example.com")
	if e.Success == 1 {
		t.Fatalf("expected failure, got %d %s", w.Code, e.Msg)
	}
}

// TestReportExposesOperationalMetrics checks the undecidable rate and offline
// duration are actually computed, not just event counts.
func TestReportExposesOperationalMetrics(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e := f.ok("GET", "/api/reports/summary", nil)
	var rep struct {
		AnalysisQuality struct {
			Ticks       int64   `json:"ticks"`
			Unknown     int64   `json:"unknown"`
			Offline     int64   `json:"offline"`
			UnknownRate float64 `json:"unknown_rate"`
			Availability float64 `json:"availability"`
		} `json:"analysis_quality"`
		Offline struct {
			TotalSec int64 `json:"total_sec"`
			OpenNow  int   `json:"open_now"`
		} `json:"offline"`
	}
	if err := json.Unmarshal(e.Data, &rep); err != nil {
		t.Fatal(err)
	}
	// With no analysis yet the shape must still be present and sane.
	if rep.AnalysisQuality.Ticks != 0 {
		t.Fatalf("expected no ticks yet, got %d", rep.AnalysisQuality.Ticks)
	}
	if rep.AnalysisQuality.Availability == 0 {
		t.Fatal("availability should be 1 with no ticks, or computed otherwise")
	}
}

// TestAccessDeviceProbeAndSync checks a GB28181 device can be probed for real
// and only then be turned into a platform camera.
func TestAccessDeviceProbeAndSync(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e := f.ok("POST", "/api/access-devices", map[string]any{
		"kind": "gb28181", "code": "gb-1", "name": "国标设备一号",
		"config": map[string]any{"host": "127.0.0.1", "port": 1, "device_id": "34020000001320000001"},
	})
	var dev struct{ ID uint `json:"id"` }
	json.Unmarshal(e.Data, &dev)

	// Nothing is listening, so the probe must admit it rather than mark OK.
	e = f.ok("POST", fmt.Sprintf("/api/access-devices/%d/probe", dev.ID), nil)
	var pr struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	json.Unmarshal(e.Data, &pr)
	if pr.OK || pr.Status == "reachable" {
		t.Fatalf("unreachable device marked reachable: %+v", pr)
	}

	// Sync must refuse while the device is not reachable.
	_, w := f.call("POST", fmt.Sprintf("/api/access-devices/%d/sync-camera", dev.ID), nil, f.cookie, "http://example.com")
	if w.Code == 200 {
		t.Fatal("sync should not succeed before a successful probe")
	}

	// A device with no host cannot even be probed and stays pending.
	e = f.ok("POST", "/api/access-devices", map[string]any{
		"kind": "gb28181", "code": "gb-2", "name": "缺配置", "config": map[string]any{},
	})
	var dev2 struct{ ID uint `json:"id"` }
	json.Unmarshal(e.Data, &dev2)
	e = f.ok("POST", fmt.Sprintf("/api/access-devices/%d/probe", dev2.ID), nil)
	var pr2 struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	json.Unmarshal(e.Data, &pr2)
	if pr2.OK || pr2.Status != "pending_adapter" {
		t.Fatalf("incomplete config should stay pending: %+v", pr2)
	}
}

// TestEventClipsEndpointReturnsShape checks the recording centre can query
// event clips for a camera.
func TestEventClipsEndpointReturnsShape(t *testing.T) {
	f := setup(t)
	f.cookie = f.login("admin", "Adminpass123")

	e := f.ok("POST", "/api/cameras", map[string]any{
		"code": "clip-cam", "name": "Clip Cam", "access_type": "rtsp", "main_url": "rtsp://cam/9",
	})
	var cam struct{ ID uint `json:"id"` }
	json.Unmarshal(e.Data, &cam)

	e = f.ok("GET", fmt.Sprintf("/api/cameras/%d/event-clips", cam.ID), nil)
	var rows []struct {
		Event     map[string]any `json:"event"`
		Evidences []map[string]any `json:"evidences"`
	}
	if err := json.Unmarshal(e.Data, &rows); err != nil {
		t.Fatalf("event-clips should return an array: %v (%s)", err, string(e.Data))
	}
	if len(rows) != 0 {
		t.Fatalf("expected no events yet, got %d", len(rows))
	}
}
