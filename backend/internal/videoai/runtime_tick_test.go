package videoai

import (
	"testing"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/videoai/ingest"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// testEchoAdapter is a deterministic, frame-consuming test double. It is
// registered under a non-production name; production registers only onnx.
type testEchoAdapter struct{}

func (testEchoAdapter) Name() string { return "test-echo" }
func (testEchoAdapter) Demo() bool   { return true }

func (testEchoAdapter) Available() (bool, string) { return true, "test" }

func (testEchoAdapter) Infer(cameraID uint, f Frame, cfg ModelConfig) ([]Detection, error) {
	if len(f.JPEG) == 0 {
		return nil, errEmptyFrame
	}
	// A bucket in the upper half of a 1280x720 calibration frame: above a
	// pit_side=below boundary, i.e. a crossing.
	return []Detection{{
		Label:      "grab_bucket",
		Confidence: 0.92,
		Centroid:   Point{X: 640, Y: 100},
		Contour:    []Point{{X: 600, Y: 60}, {X: 680, Y: 60}, {X: 680, Y: 140}, {X: 600, Y: 140}},
	}}, nil
}

// fakeFrames injects deterministic frames into the analysis loop.
type fakeFrames struct {
	frames map[uint]ingest.Frame
}

func (f *fakeFrames) Ensure(cameraID uint, url string) error { return nil }

func (f *fakeFrames) Latest(cameraID uint) (ingest.Frame, bool) {
	fr, ok := f.frames[cameraID]
	return fr, ok
}

func tickTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Camera{}, &models.StreamProfile{},
		&models.BoundaryVersion{}, &models.AlgoModel{}, &models.AlgoModelVersion{},
		&models.BusinessTemplate{}, &models.Deployment{}, &models.RuntimeStatus{},
		&models.VideoEvent{}, &models.EventAction{}, &models.Evidence{},
		&models.MediaOutage{}, &models.AnalysisStat{}, &models.RuleVersion{},
	); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// tickFixture wires camera + boundary + template + deployment for tick tests.
func tickFixture(t *testing.T, db *gorm.DB) (camID, depID uint) {
	t.Helper()
	cam := models.Camera{Code: "tick-cam", Name: "tick", AccessType: "rtsp", Enabled: 1}
	if err := db.Create(&cam).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&models.StreamProfile{CameraID: cam.ID, Role: "main", URL: "rtsp://127.0.0.1:554/x", Enabled: 1})
	bound := models.BoundaryVersion{
		CameraID: cam.ID, Version: 1, Status: "published", Name: "edge",
		RefWidth: 1280, RefHeight: 720, Source: "manual",
		GeometryJSON: `{"kind":"polyline","pit_side":"below","buffer_norm":0.02,"points":[{"x":100,"y":400},{"x":640,"y":420},{"x":1180,"y":400}]}`,
	}
	if err := db.Create(&bound).Error; err != nil {
		t.Fatal(err)
	}
	model := models.AlgoModel{Code: "echo_model", Name: "echo", Framework: "test"}
	if err := db.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	mv := models.AlgoModelVersion{ModelID: model.ID, Version: "v1", Status: "published", Adapter: "test-echo", ClassesJSON: `["grab_bucket"]`}
	if err := db.Create(&mv).Error; err != nil {
		t.Fatal(err)
	}
	tpl := models.BusinessTemplate{Code: "echo_tpl", Name: "echo tpl", Version: "1", Status: "published", ModelVersionID: &mv.ID, RuleCodesJSON: `["grab_near_edge","grab_cross_boundary"]`}
	if err := db.Create(&tpl).Error; err != nil {
		t.Fatal(err)
	}
	dep := models.Deployment{
		Name: "tick-dep", CameraID: cam.ID, TemplateID: &tpl.ID, BoundaryVersionID: &bound.ID,
		Status: "running", DesiredRevision: 1, AnalysisFPS: 2,
		ThresholdNear: 0.15, ThresholdCross: 0, TriggerHoldSec: 1, RecoverHoldSec: 2,
	}
	if err := db.Create(&dep).Error; err != nil {
		t.Fatal(err)
	}
	return cam.ID, dep.ID
}

// TestTickWithoutFrameReportsOfflineAndNoEvents locks the core no-mock
// contract: a tick with no fresh frame is offline and fabricates nothing.
func TestTickWithoutFrameReportsOfflineAndNoEvents(t *testing.T) {
	RegisterAdapter("test-echo", testEchoAdapter{})
	db := tickTestDB(t)
	camID, depID := tickFixture(t, db)

	m := NewManager(db, t.TempDir(), nil)
	m.frames = &fakeFrames{frames: map[uint]ingest.Frame{}}
	var dep models.Deployment
	db.First(&dep, depID)
	m.ensureRuntime(dep)

	if err := m.tick(depID, time.Now()); err != nil {
		t.Fatal(err)
	}
	var rt models.RuntimeStatus
	if err := db.Where("deployment_id = ?", depID).First(&rt).Error; err != nil {
		t.Fatal(err)
	}
	if rt.LastResult != ResultOffline {
		t.Fatalf("last_result = %q, want offline", rt.LastResult)
	}
	if rt.LastError != "等待视频帧" {
		t.Fatalf("last_error = %q", rt.LastError)
	}
	if rt.FrameAgeMS != -1 {
		t.Fatalf("frame_age_ms = %d, want -1 sentinel", rt.FrameAgeMS)
	}
	if rt.HeartbeatAt == nil {
		t.Fatal("heartbeat must still update while offline")
	}
	var events int64
	db.Model(&models.VideoEvent{}).Where("camera_id = ?", camID).Count(&events)
	if events != 0 {
		t.Fatalf("offline tick fabricated %d events", events)
	}
	var cam models.Camera
	db.First(&cam, camID)
	if cam.AnalysisStatus != "waiting_video" {
		t.Fatalf("analysis_status = %q", cam.AnalysisStatus)
	}
}

// TestTickWithFrameRunsRealInference proves the happy path consumes pixels:
// the adapter receives the JPEG, and a crossing produces a real event with
// overlay evidence.
func TestTickWithFrameRunsRealInference(t *testing.T) {
	RegisterAdapter("test-echo", testEchoAdapter{})
	db := tickTestDB(t)
	camID, depID := tickFixture(t, db)

	fresh := time.Now()
	m := NewManager(db, t.TempDir(), nil)
	m.frames = &fakeFrames{frames: map[uint]ingest.Frame{
		camID: {TS: fresh, JPEG: syntheticFrameJPEG(), W: 640, H: 480},
	}}
	var dep models.Deployment
	db.First(&dep, depID)
	m.ensureRuntime(dep)

	// First tick evaluates the frame; hysteresis fires on the next tick once
	// the trigger hold has elapsed.
	now := fresh
	var fired bool
	for i := 0; i < 6 && !fired; i++ {
		now = now.Add(1500 * time.Millisecond)
		if err := m.tick(depID, now); err != nil {
			t.Fatal(err)
		}
		var n int64
		db.Model(&models.VideoEvent{}).Where("camera_id = ?", camID).Count(&n)
		fired = n > 0
	}
	if !fired {
		t.Fatal("crossing frame never produced an event")
	}
	var ev models.VideoEvent
	db.Where("camera_id = ?", camID).First(&ev)
	if ev.OverlayJSON == "" {
		t.Fatal("event overlay missing")
	}
	var evidences []models.Evidence
	db.Where("event_id = ?", ev.ID).Find(&evidences)
	kinds := map[string]bool{}
	for _, e := range evidences {
		kinds[e.Kind] = true
	}
	if !kinds["overlay"] {
		t.Fatalf("overlay evidence missing: %+v", evidences)
	}
	var rt models.RuntimeStatus
	db.Where("deployment_id = ?", depID).First(&rt)
	if rt.LastResult != ResultCross {
		t.Fatalf("runtime last_result = %q, want cross", rt.LastResult)
	}
	if rt.Backend != "test-echo" {
		t.Fatalf("runtime backend = %q", rt.Backend)
	}
	var stats models.AnalysisStat
	if err := db.Where("deployment_id = ?", depID).First(&stats).Error; err != nil {
		t.Fatalf("analysis stat missing: %v", err)
	}
	if stats.Ticks == 0 || stats.Cross == 0 {
		t.Fatalf("stat counters %+v", stats)
	}
}

// TestResolveModelRejectsUnpublished keeps the fail-closed model gate.
func TestResolveModelRejectsUnpublished(t *testing.T) {
	db := tickTestDB(t)
	m := NewManager(db, t.TempDir(), nil)
	_, depID := tickFixture(t, db)

	var dep models.Deployment
	db.First(&dep, depID)
	var tpl models.BusinessTemplate
	db.First(&tpl, *dep.TemplateID)
	var mv models.AlgoModelVersion
	db.First(&mv, *tpl.ModelVersionID)
	db.Model(&mv).Update("status", "draft")

	if _, err := m.resolveModel(dep); err == nil {
		t.Fatal("draft model version must not resolve")
	}
}

// TestConfigForVersionDefaults covers schema parsing and YOLO defaults.
func TestConfigForVersionDefaults(t *testing.T) {
	mv := models.AlgoModelVersion{Adapter: "onnx", FilePath: "/m.onnx", Backend: "cpu"}
	cfg := ConfigForVersion(mv)
	if cfg.InputW != 640 || cfg.InputH != 640 || cfg.Confidence != 0.25 || cfg.IoU != 0.45 {
		t.Fatalf("defaults %+v", cfg)
	}
	mv.InputSchema = `{"width":320,"height":320,"confidence":0.4,"iou":0.5}`
	cfg = ConfigForVersion(mv)
	if cfg.InputW != 320 || cfg.InputH != 320 || cfg.Confidence != 0.4 || cfg.IoU != 0.5 {
		t.Fatalf("schema override %+v", cfg)
	}
	mv.Backend = "gpu"
	if got := providersFor(mv.Backend); len(got) == 0 || got[0] != "CUDAExecutionProvider" {
		t.Fatalf("gpu providers %v", got)
	}
}

// TestSeedDefaultsHasNoMockVersion locks the no-mock seed contract.
func TestSeedDefaultsHasNoMockVersion(t *testing.T) {
	db := tickTestDB(t)
	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}
	var mockVersions int64
	db.Model(&models.AlgoModelVersion{}).Where("adapter = ?", "mock").Count(&mockVersions)
	if mockVersions != 0 {
		t.Fatalf("seed created %d mock versions", mockVersions)
	}
	var tpl models.BusinessTemplate
	if err := db.Where("code = ?", "grab_edge_guard").First(&tpl).Error; err != nil {
		t.Fatal(err)
	}
	if tpl.ModelVersionID == nil {
		t.Fatal("template must bind the onnx version")
	}
	var mv models.AlgoModelVersion
	db.First(&mv, *tpl.ModelVersionID)
	if mv.Adapter != "onnx" || mv.Status != "draft" {
		t.Fatalf("bound version %+v", mv)
	}
}

// TestSeedMigratesLegacyMockBinding: a template bound to a legacy mock
// version is re-pointed at the onnx version and the mock row is archived.
func TestSeedMigratesLegacyMockBinding(t *testing.T) {
	db := tickTestDB(t)
	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}
	var model models.AlgoModel
	db.Where("code = ?", "grab_bucket_seg").First(&model)
	legacy := models.AlgoModelVersion{ModelID: model.ID, Version: "demo-1.0.0", Status: "published", Adapter: "mock"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	var tpl models.BusinessTemplate
	db.Where("code = ?", "grab_edge_guard").First(&tpl)
	db.Model(&tpl).Update("model_version_id", legacy.ID)

	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}
	db.First(&legacy, legacy.ID)
	if legacy.Status != "archived" {
		t.Fatalf("legacy mock version not archived: %q", legacy.Status)
	}
	db.First(&tpl, tpl.ID)
	var mv models.AlgoModelVersion
	db.First(&mv, *tpl.ModelVersionID)
	if mv.Adapter != "onnx" {
		t.Fatalf("template still bound to %q", mv.Adapter)
	}
}

// TestSeedCorrectsLegacyMockFramework: a legacy AlgoModel row labelled
// framework=mock is corrected to onnx. The label is what the algorithm page
// shows, so a stale one makes a real onnx model look like a demo.
func TestSeedCorrectsLegacyMockFramework(t *testing.T) {
	db := tickTestDB(t)
	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}
	var model models.AlgoModel
	db.Where("code = ?", "grab_bucket_seg").First(&model)
	if err := db.Model(&model).Update("framework", "mock").Error; err != nil {
		t.Fatal(err)
	}

	if err := SeedDefaults(db); err != nil {
		t.Fatal(err)
	}
	var after models.AlgoModel
	db.First(&after, model.ID)
	if after.Framework != "onnx" {
		t.Fatalf("framework not corrected: %q", after.Framework)
	}
	// Correction must not fabricate a version or touch the existing one.
	var drafts int64
	db.Model(&models.AlgoModelVersion{}).Where("model_id = ? AND adapter = ?", model.ID, "mock").Count(&drafts)
	if drafts != 0 {
		t.Fatalf("seed created %d mock versions", drafts)
	}
	var versions int64
	db.Model(&models.AlgoModelVersion{}).Where("model_id = ?", model.ID).Count(&versions)
	if versions != 1 {
		t.Fatalf("expected the single seeded v1, got %d versions", versions)
	}
}
