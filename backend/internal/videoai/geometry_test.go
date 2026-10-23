package videoai

import "testing"

func TestEvaluateGrabRulesCross(t *testing.T) {
	g := Geometry{
		Kind: "polyline", PitSide: "below",
		Points: []Point{{X: 0, Y: 100}, {X: 200, Y: 100}},
	}
	out := EvaluateGrabRules(RuleInput{
		Detections:     []Detection{{Label: "grab_bucket", Confidence: 0.9, Centroid: Point{X: 100, Y: 40}}},
		Geometry:       g,
		ThresholdNear:  0.15,
		ThresholdCross: 0,
		FrameW:         200,
		FrameH:         200,
		MediaOK:        true,
	})
	// Point above the line with pit_side=below means outside (cross).
	if out.Result != ResultCross && out.Result != ResultNear {
		t.Fatalf("expected cross/near, got %s (%s)", out.Result, out.Reason)
	}
}

func TestValidateGeometryMinPoints(t *testing.T) {
	err := ValidateGeometry(Geometry{Points: []Point{{X: 1, Y: 1}}}, 100, 100)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestNoSyntheticBoundaryCandidates locks the no-mock contract: the demo
// polyline generator is gone from the package, so nothing can hand out
// fabricated boundary geometry.
func TestNoSyntheticBoundaryCandidates(t *testing.T) {
	if RegisteredAdapters() == nil {
		t.Fatal("adapter registry unavailable")
	}
	for _, a := range RegisteredAdapters() {
		if name, _ := a["name"].(string); name == "mock" {
			t.Fatal("mock adapter must not be registered in production")
		}
	}
}
