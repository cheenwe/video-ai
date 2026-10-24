package videoai

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/videoai/ingest"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqlCounter counts statements by kind so a test can assert that the analysis
// loop stopped rewriting rows that did not change instead of trusting a comment.
type sqlCounter struct {
	gormlogger.Interface
	mu       sync.Mutex
	updates  int
	inserts  int
	selects  int
	traceSQL []string
}

func (c *sqlCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	c.mu.Lock()
	c.traceSQL = append(c.traceSQL, sql)
	up := strings.ToUpper(sql)
	switch {
	case strings.HasPrefix(up, "UPDATE"):
		c.updates++
	case strings.HasPrefix(up, "INSERT"):
		c.inserts++
	case strings.HasPrefix(up, "SELECT"):
		c.selects++
	}
	c.mu.Unlock()
}

func (c *sqlCounter) snapshot() (u, i, s int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.updates, c.inserts, c.selects
}

// TestTickDoesNotRewriteUnchangedRows locks the write amplification fix: a
// second tick on a healthy deployment must not repeat the camera status,
// applied revision or overlay writes, because nothing about them changed.
func TestTickDoesNotRewriteUnchangedRows(t *testing.T) {
	RegisterAdapter("test-echo", testEchoAdapter{})
	db := tickTestDB(t)
	camID, depID := tickFixture(t, db)

	m := NewManager(db, t.TempDir(), nil)
	m.frames = &fakeFrames{frames: map[uint]ingest.Frame{
		camID: {TS: time.Now(), JPEG: syntheticFrameJPEG(), W: 1280, H: 720},
	}}

	counter := &sqlCounter{Interface: gormlogger.Discard}
	session := db.Session(&gorm.Session{Logger: counter})
	m.DB = session

	var dep models.Deployment
	db.First(&dep, depID)
	m.ensureRuntime(dep)

	run := func() {
		var d models.Deployment
		if err := session.First(&d, depID).Error; err != nil {
			t.Fatal(err)
		}
		// Frame must stay fresh for the 5s window.
		m.frames.(*fakeFrames).frames[camID] = ingest.Frame{TS: time.Now(), JPEG: syntheticFrameJPEG(), W: 1280, H: 720}
		if err := m.tick(depID, time.Now()); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}

	run()
	firstU, _, _ := counter.snapshot()
	run()
	secondU, _, secondS := counter.snapshot()

	delta := secondU - firstU
	t.Logf("first tick UPDATEs=%d, second tick UPDATEs=%d (heartbeat + stat rollup only)", firstU, delta)
	// The heartbeat (runtime row) and the per-tick stat rollup must still be
	// written every tick; everything else must not.
	if delta > 3 {
		t.Fatalf("second tick issued %d UPDATEs; expected at most 3 (heartbeat + stat), trace:\n%s",
			delta, strings.Join(counter.traceSQL, "\n"))
	}
	if secondS == 0 {
		t.Fatal("expected the tick to still read its configuration")
	}
}

// TestStreamURLCacheInvalidates checks the operator path: after the URL is
// edited and invalidated, the next lookup sees the new value rather than a
// value pinned for the whole TTL.
func TestStreamURLCacheInvalidates(t *testing.T) {
	db := tickTestDB(t)
	cam := models.Camera{Code: "cache-cam", Name: "c", AccessType: "rtsp", Enabled: 1}
	if err := db.Create(&cam).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&models.StreamProfile{CameraID: cam.ID, Role: "main", URL: "rtsp://old/1", Enabled: 1})

	m := NewManager(db, t.TempDir(), nil)
	if got := m.cameraStreamURL(cam.ID); got != "rtsp://old/1" {
		t.Fatalf("first lookup = %q", got)
	}
	// Still cached even though the row changed underneath.
	db.Model(&models.StreamProfile{}).Where("camera_id = ?", cam.ID).Update("url", "rtsp://new/1")
	if got := m.cameraStreamURL(cam.ID); got != "rtsp://old/1" {
		t.Fatalf("cache must be warm, got %q", got)
	}
	m.InvalidateStream(cam.ID)
	if got := m.cameraStreamURL(cam.ID); got != "rtsp://new/1" {
		t.Fatalf("after invalidate = %q, want rtsp://new/1", got)
	}
}

// TestModelCacheInvalidateOnRepublish checks that rebinding a template to a
// different published model takes effect on the next resolution.
func TestModelCacheInvalidateOnRepublish(t *testing.T) {
	RegisterAdapter("test-echo", testEchoAdapter{})
	db := tickTestDB(t)
	_, depID := tickFixture(t, db)
	m := NewManager(db, t.TempDir(), nil)

	var dep models.Deployment
	db.First(&dep, depID)
	rm1, err := m.resolveModel(dep)
	if err != nil {
		t.Fatal(err)
	}

	var model models.AlgoModel
	db.Where("code = ?", "echo_model").First(&model)
	mv2 := models.AlgoModelVersion{ModelID: model.ID, Version: "v2", Status: "published", Adapter: "test-echo"}
	if err := db.Create(&mv2).Error; err != nil {
		t.Fatal(err)
	}
	db.Model(&models.BusinessTemplate{}).Where("id = ?", *dep.TemplateID).
		Update("model_version_id", mv2.ID)

	// Cached: still v1 until invalidated.
	if rm2, _ := m.resolveModel(dep); rm2.VersionID != rm1.VersionID {
		t.Fatalf("cache should still serve v1, got %d", rm2.VersionID)
	}
	m.InvalidateModel(*dep.TemplateID)
	rm3, err := m.resolveModel(dep)
	if err != nil {
		t.Fatal(err)
	}
	if rm3.VersionID != mv2.ID {
		t.Fatalf("after invalidate got version %d, want %d", rm3.VersionID, mv2.ID)
	}
}

// TestNoteMediaDownReusesOutageAfterRestart guards against double counting:
// an outage left open by a previous process must be reused, not duplicated.
func TestNoteMediaDownReusesOutageAfterRestart(t *testing.T) {
	db := tickTestDB(t)
	cam := models.Camera{Code: "outage-cam", Name: "o", AccessType: "rtsp", Enabled: 1}
	if err := db.Create(&cam).Error; err != nil {
		t.Fatal(err)
	}

	// Process 1 opens an outage.
	m1 := NewManager(db, t.TempDir(), nil)
	now := time.Now()
	m1.noteMediaDown(cam, "初始原因", now)

	// Process 2 (restart) sees the open row and must not create another.
	m2 := NewManager(db, t.TempDir(), nil)
	m2.noteMediaDown(cam, "新原因", now.Add(time.Second))

	var open int64
	db.Model(&models.MediaOutage{}).Where("camera_id = ? AND ended_at IS NULL", cam.ID).Count(&open)
	if open != 1 {
		t.Fatalf("expected exactly 1 open outage after restart, got %d", open)
	}
	var row models.MediaOutage
	db.Where("camera_id = ? AND ended_at IS NULL", cam.ID).First(&row)
	if row.Reason != "新原因" {
		t.Fatalf("reason not updated: %q", row.Reason)
	}

	// Recovery closes it, and a second healthy tick does not touch the table.
	m2.noteMediaOK(cam, now.Add(2*time.Second))
	m2.noteMediaOK(cam, now.Add(3*time.Second))
	db.Model(&models.MediaOutage{}).Where("camera_id = ? AND ended_at IS NULL", cam.ID).Count(&open)
	if open != 0 {
		t.Fatalf("outage not closed, %d still open", open)
	}
}

// TestOverlayPersistSkipsUnchanged guards the volatile-key stripping: frame
// age changes every tick but must not trigger a database write on its own.
func TestOverlayPersistSkipsUnchanged(t *testing.T) {
	db := tickTestDB(t)
	m := NewManager(db, t.TempDir(), nil)
	db.Create(&models.RuntimeStatus{DeploymentID: 7})

	writeCount := 0
	m.DB = db.Session(&gorm.Session{Logger: &countUpdates{fn: func() { writeCount++ }}})

	base := map[string]any{"adapter": "onnx", "ref_width": 1280, "frame_age_ms": 12}
	m.persistOverlay(7, base)
	if writeCount != 1 {
		t.Fatalf("first persist should write once, got %d", writeCount)
	}
	m.persistOverlay(7, map[string]any{"adapter": "onnx", "ref_width": 1280, "frame_age_ms": 900})
	if writeCount != 1 {
		t.Fatalf("volatile-only change must not write, got %d", writeCount)
	}
	m.persistOverlay(7, map[string]any{"adapter": "onnx", "ref_width": 1920, "frame_age_ms": 900})
	if writeCount != 2 {
		t.Fatalf("stable change must write, got %d", writeCount)
	}
	var rt models.RuntimeStatus
	db.Where("deployment_id = ?", 7).First(&rt)
	if strings.Contains(rt.LastOverlayJSON, "frame_age_ms") {
		t.Fatalf("volatile key persisted: %s", rt.LastOverlayJSON)
	}
}

// countUpdates is a minimal gorm logger that counts UPDATE statements.
type countUpdates struct {
	fn func()
}

func (c *countUpdates) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *countUpdates) Info(context.Context, string, ...any)             {}
func (c *countUpdates) Warn(context.Context, string, ...any)             {}
func (c *countUpdates) Error(context.Context, string, ...any)            {}
func (c *countUpdates) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	if strings.HasPrefix(strings.ToUpper(sql), "UPDATE") {
		c.fn()
	}
}
