package videoai

import (
	"strings"
	"testing"
)

func det(label string, x, y, score float64) Detection {
	return Detection{
		Label: label, Confidence: score,
		Centroid: Point{X: x, Y: y},
		Contour:  []Point{{X: x - 5, Y: y - 5}, {X: x + 5, Y: y - 5}, {X: x + 5, Y: y + 5}, {X: x - 5, Y: y + 5}},
	}
}

// TestDeriveBoundaryFromEdgeDetections is the happy path: two edge detections
// become a 2-point polyline in frame pixels, ordered left to right.
func TestDeriveBoundaryFromEdgeDetections(t *testing.T) {
	dets := []Detection{
		det("pit_edge", 300, 500, 0.8),
		det("pit_edge", 900, 520, 0.9),
	}
	cand, err := DeriveBoundary(dets, 1280, 720)
	if err != nil {
		t.Fatal(err)
	}
	if cand.Geometry.Kind != "polyline" || len(cand.Geometry.Points) != 2 {
		t.Fatalf("geometry = %+v", cand.Geometry)
	}
	if cand.Geometry.Points[0].X >= cand.Geometry.Points[1].X {
		t.Fatalf("points not left-to-right: %+v", cand.Geometry.Points)
	}
	if cand.Geometry.SourceHint != "ai_candidate" {
		t.Fatalf("source hint = %q", cand.Geometry.SourceHint)
	}
	if cand.EdgeDetections != 2 {
		t.Fatalf("edge detections = %d", cand.EdgeDetections)
	}
	if got, want := cand.Confidence, 0.85; got < want-0.001 || got > want+0.001 {
		t.Fatalf("confidence = %v, want %v", got, want)
	}
	// The produced geometry must pass the same validator the create API uses.
	if err := ValidateGeometry(cand.Geometry, 1280, 720); err != nil {
		t.Fatalf("candidate rejected by validator: %v", err)
	}
}

// TestDeriveBoundaryRejectsBucketOnlyModel is the honesty guard: a model that
// only knows about grab buckets must not be turned into a line.
func TestDeriveBoundaryRejectsBucketOnlyModel(t *testing.T) {
	dets := []Detection{
		det("grab_bucket", 300, 200, 0.9),
		det("grab_bucket", 700, 220, 0.9),
		det("grab_bucket", 1000, 210, 0.9),
	}
	_, err := DeriveBoundary(dets, 1280, 720)
	if err == nil {
		t.Fatal("bucket-only detections must not produce a boundary")
	}
	if !strings.Contains(err.Error(), "坑边") {
		t.Fatalf("error should explain the missing edge class: %v", err)
	}
}

// TestDeriveBoundaryRejectsEmptyAndSingle covers the degenerate inputs.
func TestDeriveBoundaryRejectsEmptyAndSingle(t *testing.T) {
	if _, err := DeriveBoundary(nil, 1280, 720); err == nil {
		t.Fatal("no detections must fail")
	}
	if _, err := DeriveBoundary([]Detection{det("pit_edge", 100, 100, 0.9)}, 1280, 720); err == nil {
		t.Fatal("a single edge detection must fail")
	}
}

// TestDeriveBoundaryDropsNonAdvancingPoints: two detections at the same x would
// create a zero-width spike that the geometry validator rejects.
func TestDeriveBoundaryDropsNonAdvancingPoints(t *testing.T) {
	dets := []Detection{
		det("pit_edge", 400, 300, 0.9),
		det("pit_edge", 400, 340, 0.9),
		det("pit_edge", 800, 320, 0.9),
	}
	cand, err := DeriveBoundary(dets, 1280, 720)
	if err != nil {
		t.Fatal(err)
	}
	if len(cand.Geometry.Points) != 2 {
		t.Fatalf("expected 2 advancing points, got %+v", cand.Geometry.Points)
	}
	if err := ValidateGeometry(cand.Geometry, 1280, 720); err != nil {
		t.Fatalf("validator: %v", err)
	}
}

// TestDeriveBoundaryInfersPitSide: with the bucket above the line, the pit is
// above.
func TestDeriveBoundaryInfersPitSide(t *testing.T) {
	above := []Detection{
		det("pit_edge", 200, 500, 0.9),
		det("pit_edge", 1000, 500, 0.9),
		det("grab_bucket", 600, 150, 0.9),
		det("grab_bucket", 700, 120, 0.9),
	}
	cand, err := DeriveBoundary(above, 1280, 720)
	if err != nil {
		t.Fatal(err)
	}
	if cand.Geometry.PitSide != "above" {
		t.Fatalf("pit side = %q, want above", cand.Geometry.PitSide)
	}

	below := []Detection{
		det("pit_edge", 200, 300, 0.9),
		det("pit_edge", 1000, 300, 0.9),
		det("grab_bucket", 600, 600, 0.9),
	}
	cand, err = DeriveBoundary(below, 1280, 720)
	if err != nil {
		t.Fatal(err)
	}
	if cand.Geometry.PitSide != "below" {
		t.Fatalf("pit side = %q, want below", cand.Geometry.PitSide)
	}

	// No evidence → manual editor default.
	noBuckets := []Detection{
		det("pit_edge", 200, 300, 0.9),
		det("pit_edge", 1000, 300, 0.9),
	}
	cand, _ = DeriveBoundary(noBuckets, 1280, 720)
	if cand.Geometry.PitSide != "below" {
		t.Fatalf("default pit side = %q", cand.Geometry.PitSide)
	}
}

// TestDeriveBoundaryClampsToFrame: a detection outside the picture must not
// produce out-of-range vertices.
func TestDeriveBoundaryClampsToFrame(t *testing.T) {
	dets := []Detection{
		det("pit_edge", -50, 400, 0.9),
		det("pit_edge", 2000, 900, 0.9),
	}
	cand, err := DeriveBoundary(dets, 1280, 720)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range cand.Geometry.Points {
		if p.X < 0 || p.X > 1280 || p.Y < 0 || p.Y > 720 {
			t.Fatalf("point outside frame: %+v", p)
		}
	}
	if err := ValidateGeometry(cand.Geometry, 1280, 720); err != nil {
		t.Fatalf("validator: %v", err)
	}
}

// TestIsEdgeLabelAlsoMatchesChinese keeps a Chinese-labelled edge class usable.
func TestIsEdgeLabelAlsoMatchesChinese(t *testing.T) {
	for _, l := range []string{"pit_edge", "EDGE", "边界线", "坑边内侧", "curb_stone"} {
		if !isEdgeLabel(l) {
			t.Fatalf("%q should be treated as an edge class", l)
		}
	}
	for _, l := range []string{"grab_bucket", "抓斗", "", "person"} {
		if isEdgeLabel(l) {
			t.Fatalf("%q must not be treated as an edge class", l)
		}
	}
}
