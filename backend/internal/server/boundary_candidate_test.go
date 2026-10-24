package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"baseplatform/internal/handlers"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/videoai"
	"baseplatform/internal/videoai/ingest"

	"github.com/gin-gonic/gin"
)

// edgeAdapter answers with pit-edge detections, standing in for a segmentation
// model that can actually see the boundary.
type edgeAdapter struct{ labels []string }

func (edgeAdapter) Name() string              { return "test-edge-src" }
func (edgeAdapter) Demo() bool                { return false }
func (edgeAdapter) Available() (bool, string) { return true, "test" }
func (a edgeAdapter) Infer(uint, videoai.Frame, videoai.ModelConfig) ([]videoai.Detection, error) {
	out := make([]videoai.Detection, 0, len(a.labels))
	for i, l := range a.labels {
		out = append(out, videoai.Detection{
			Label: l, Confidence: 0.9,
			Centroid: videoai.Point{X: float64(200 + i*400), Y: 400},
			Contour:  []videoai.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}},
		})
	}
	return out, nil
}

// candFrames injects one fresh frame (or none, to exercise the honest 503).
type candFrames struct {
	frame *ingest.Frame
}

func (f *candFrames) Ensure(uint, string) error { return nil }
func (f *candFrames) Latest(uint) (ingest.Frame, bool) {
	if f.frame == nil {
		return ingest.Frame{}, false
	}
	return *f.frame, true
}

// candRig builds the candidate endpoint over a real DB with an injectable frame
// source, so the test exercises the whole handler instead of the pure function.
func candRig(t *testing.T) (*gin.Engine, *handlers.BoundaryHandler, *videoai.Manager, *candFrames) {
	t.Helper()
	f := setup(t)

	var cam models.Camera
	if err := f.db.Create(&models.Camera{Code: "cand-cam", Name: "候选", AccessType: "rtsp", Enabled: 1}).Error; err != nil {
		t.Fatal(err)
	}
	f.db.Where("code = ?", "cand-cam").First(&cam)
	if err := f.db.Create(&models.StreamProfile{CameraID: cam.ID, Role: "main", URL: "rtsp://cam/1", Enabled: 1}).Error; err != nil {
		t.Fatal(err)
	}

	mgr := videoai.NewManager(f.db, t.TempDir(), nil)
	frames := &candFrames{}
	mgr.SetFrameSource(frames)

	h := &handlers.BoundaryHandler{DB: f.db, Runtime: mgr}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// The handler reads the current user for the audit trail; a real request
	// arrives through Auth, which this rig replaces with a fixed identity.
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextUserKey, models.User{ID: 1, Username: "admin", Role: "superadmin"})
	})
	r.GET("/api/boundaries/candidate", h.Candidate)
	return r, h, mgr, frames
}

func postCandidate(r *gin.Engine, query string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/boundaries/candidate?"+query, nil)
	r.ServeHTTP(w, req)
	return w
}

// TestCandidateRequiresFreshFrame: without a live frame the endpoint must not
// guess — it returns 503 and the operator is told to draw manually.
func TestCandidateRequiresFreshFrame(t *testing.T) {
	r, _, _, frames := candRig(t)
	frames.frame = nil

	w := postCandidate(r, "camera_id=1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (%s)", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "新鲜视频帧") {
		t.Fatalf("error should mention the missing frame: %s", w.Body.String())
	}
}

// TestCandidateRejectsBucketOnlyModel: a bucket-only output is a 422 with an
// explanation, never a fabricated polyline.
func TestCandidateRejectsBucketOnlyModel(t *testing.T) {
	r, h, _, frames := candRig(t)
	frames.frame = &ingest.Frame{TS: time.Now(), JPEG: []byte{0xFF, 0xD8, 0xFF, 0xD9}, W: 1280, H: 720}

	videoai.RegisterAdapter("test-bucket-src", edgeAdapter{labels: []string{"grab_bucket", "grab_bucket"}})
	mkPublishedModel(t, h, "test-bucket-src", `["grab_bucket"]`)

	w := postCandidate(r, "camera_id=1")
	if w.Code != 422 {
		t.Fatalf("status = %d, want 422 (%s)", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "坑边") {
		t.Fatalf("error should explain the missing edge class: %s", w.Body.String())
	}
}

// TestCandidateReturnsDraftGeometry is the happy path: edge detections from a
// real published model become a suggested polyline the operator can save.
func TestCandidateReturnsDraftGeometry(t *testing.T) {
	r, h, _, frames := candRig(t)
	frames.frame = &ingest.Frame{TS: time.Now(), JPEG: []byte{0xFF, 0xD8, 0xFF, 0xD9}, W: 1280, H: 720}

	videoai.RegisterAdapter("test-edge-src", edgeAdapter{labels: []string{"pit_edge", "pit_edge", "pit_edge"}})
	mkPublishedModel(t, h, "test-edge-src", `["pit_edge","grab_bucket"]`)

	w := postCandidate(r, "camera_id=1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	var env struct {
		Data struct {
			Geometry       map[string]any `json:"geometry"`
			RefWidth       int            `json:"ref_width"`
			RefHeight      int            `json:"ref_height"`
			EdgeDetections int            `json:"edge_detections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	pts, _ := env.Data.Geometry["points"].([]any)
	if len(pts) != 3 {
		t.Fatalf("expected 3 candidate vertices, got %d (%v)", len(pts), env.Data.Geometry)
	}
	if env.Data.RefWidth != 1280 || env.Data.RefHeight != 720 {
		t.Fatalf("candidate must report the source frame size, got %dx%d", env.Data.RefWidth, env.Data.RefHeight)
	}
	if env.Data.EdgeDetections != 3 {
		t.Fatalf("edge detections = %d", env.Data.EdgeDetections)
	}
	// The returned geometry must be savable through the normal create path.
	raw, _ := json.Marshal(env.Data.Geometry)
	g, err := videoai.ParseGeometry(string(raw))
	if err != nil {
		t.Fatalf("candidate not parseable: %v", err)
	}
	if err := videoai.ValidateGeometry(g, env.Data.RefWidth, env.Data.RefHeight); err != nil {
		t.Fatalf("candidate not validatable: %v", err)
	}
}

// TestCandidateUnknownCamera is a plain 404 rather than a 500.
func TestCandidateUnknownCamera(t *testing.T) {
	r, _, _, _ := candRig(t)
	w := postCandidate(r, "camera_id=9999")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
}

// mkPublishedModel creates the model row, a published version and a published
// template bound to it, so candidateModelVersion has something to resolve.
func mkPublishedModel(t *testing.T, h *handlers.BoundaryHandler, adapter, classes string) {
	t.Helper()
	model := models.AlgoModel{Code: "m-" + adapter, Name: adapter, Framework: "onnx"}
	if err := h.DB.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	mv := models.AlgoModelVersion{
		ModelID: model.ID, Version: "v-cand", Status: "published", Adapter: adapter,
		ClassesJSON: classes,
	}
	if err := h.DB.Create(&mv).Error; err != nil {
		t.Fatal(err)
	}
	tpl := models.BusinessTemplate{
		Code: "t-" + adapter, Name: adapter, Version: "1", Status: "published",
		ModelVersionID: &mv.ID, RuleCodesJSON: `[]`,
	}
	if err := h.DB.Create(&tpl).Error; err != nil {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
