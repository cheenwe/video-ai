package videoai

import (
	"testing"
	"time"
)

func TestHysteresisTriggerHold(t *testing.T) {
	h := newHysteresis(3, 5) // 3s trigger, 5s recover
	t0 := time.Now()

	// First violation arms the pending state, no trigger yet.
	if d := h.Update(ResultNear, RuleNearEdge, t0); d.Trigger {
		t.Fatal("triggered on first frame")
	}
	// Within the hold window: still no trigger.
	if d := h.Update(ResultNear, RuleNearEdge, t0.Add(2*time.Second)); d.Trigger {
		t.Fatal("triggered before hold elapsed")
	}
	// After the hold window: trigger.
	d := h.Update(ResultNear, RuleNearEdge, t0.Add(3*time.Second))
	if !d.Trigger || d.Rule != RuleNearEdge {
		t.Fatalf("expected trigger, got %+v", d)
	}
	// Continued violation keeps the event alive (refresh), not a recover.
	if d := h.Update(ResultNear, RuleNearEdge, t0.Add(4*time.Second)); !d.Trigger || d.Recover {
		t.Fatalf("expected refresh trigger, got %+v", d)
	}
}

func TestHysteresisRecoverHold(t *testing.T) {
	h := newHysteresis(0, 5)
	t0 := time.Now()

	if d := h.Update(ResultCross, RuleCrossBoundary, t0); !d.Trigger {
		t.Fatal("zero trigger hold should fire immediately")
	}
	// Normal frames must persist for the recover window before closing.
	if d := h.Update(ResultNormal, "", t0.Add(1*time.Second)); d.Recover {
		t.Fatal("recovered too early")
	}
	// A violation inside the window re-arms, recovery restarts.
	if d := h.Update(ResultCross, RuleCrossBoundary, t0.Add(2*time.Second)); !d.Trigger {
		t.Fatal("expected re-trigger")
	}
	if d := h.Update(ResultNormal, "", t0.Add(3*time.Second)); d.Recover {
		t.Fatal("recovered too early after re-trigger")
	}
	if d := h.Update(ResultNormal, "", t0.Add(8*time.Second)); !d.Recover {
		t.Fatal("expected recover after hold")
	}
}

func TestHysteresisEscalationAndUnknown(t *testing.T) {
	h := newHysteresis(0, 0)
	t0 := time.Now()

	if d := h.Update(ResultNear, RuleNearEdge, t0); !d.Trigger {
		t.Fatal("expected immediate near trigger")
	}
	// Escalation near → cross closes the old rule and triggers the new one.
	d := h.Update(ResultCross, RuleCrossBoundary, t0.Add(1*time.Second))
	if !d.Trigger || !d.Escalated || !d.Recover || d.Rule != RuleCrossBoundary {
		t.Fatalf("expected escalation, got %+v", d)
	}
	// Unknown frames must not recover an active event.
	if d := h.Update(ResultUnknown, "", t0.Add(2*time.Second)); d.Recover {
		t.Fatal("unknown frame must not recover")
	}
	if d := h.Update(ResultOffline, "", t0.Add(3*time.Second)); d.Recover {
		t.Fatal("offline frame must not recover")
	}
	if d := h.Update(ResultNormal, "", t0.Add(4*time.Second)); !d.Recover {
		t.Fatal("expected recover")
	}
	// Unknown frames also clear a pending (not yet held) trigger.
	h2 := newHysteresis(10, 0)
	h2.Update(ResultNear, RuleNearEdge, t0)
	if d := h2.Update(ResultUnknown, "", t0.Add(1*time.Second)); d.Trigger {
		t.Fatal("unknown cleared pending state")
	}
	if d := h2.Update(ResultNear, RuleNearEdge, t0.Add(2*time.Second)); d.Trigger {
		t.Fatal("pending hold restarted but should not have fired yet")
	}
}

func TestAnalysisInterval(t *testing.T) {
	cases := []struct {
		fps  float64
		want time.Duration
	}{
		{0, time.Second}, // default 1 fps
		{2, 500 * time.Millisecond},
		{0.5, 2 * time.Second},
		{30, 100 * time.Millisecond}, // clamped min
		{0.01, 10 * time.Second},     // clamped max
	}
	for _, c := range cases {
		if got := analysisInterval(c.fps); got != c.want {
			t.Errorf("analysisInterval(%v) = %v, want %v", c.fps, got, c.want)
		}
	}
}

func TestAdapterRegistry(t *testing.T) {
	mock, err := AdapterByName("mock")
	if err != nil || !mock.Demo() {
		t.Fatalf("mock adapter: %v demo=%v", err, mock != nil && mock.Demo())
	}
	onnx, err := AdapterByName("onnx")
	if err != nil || onnx.Demo() {
		t.Fatalf("onnx adapter registered: %v", err)
	}
	// The onnx placeholder must refuse to fabricate detections.
	if dets, err := onnx.Infer(1, time.Now(), 1280, 720); err == nil || dets != nil {
		t.Fatal("onnx placeholder must fail loudly, not synthesize results")
	}
	if _, err := AdapterByName("nope"); err == nil {
		t.Fatal("unknown adapter should error")
	}
}
