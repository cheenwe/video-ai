package videoai

import (
	"hash/fnv"
	"math"
	"time"
)

// MockAdapter produces deterministic synthetic detections for demos.
// Results are labeled as demo in overlays; never claim real model accuracy.
type MockAdapter struct{}

func (MockAdapter) Name() string { return "mock" }

func (MockAdapter) Infer(cameraID uint, frameTS time.Time, w, h float64) []Detection {
	if w <= 0 {
		w = 1280
	}
	if h <= 0 {
		h = 720
	}
	// Slow oscillation across the lower third of the frame so distance to a
	// typical horizontal pit edge changes over time (demo only).
	phase := float64(frameTS.UnixMilli()%20000) / 20000 * 2 * math.Pi
	seed := hashU32(cameraID)
	offset := float64(seed%17) / 100
	cx := w * (0.35 + 0.3*math.Sin(phase+offset))
	cy := h * (0.55 + 0.25*math.Sin(phase*0.7+offset))
	r := math.Min(w, h) * 0.06
	contour := []Point{
		{X: cx - r, Y: cy - r},
		{X: cx + r, Y: cy - r},
		{X: cx + r, Y: cy + r},
		{X: cx - r, Y: cy + r},
	}
	return []Detection{{
		Label:      "grab_bucket",
		Confidence: 0.85 + 0.1*math.Sin(phase),
		Centroid:   Point{X: cx, Y: cy},
		Contour:    contour,
	}}
}

func hashU32(v uint) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	return h.Sum32()
}

// SuggestBoundary returns a simple horizontal candidate polyline.
func SuggestBoundary(w, h int) Geometry {
	if w <= 0 {
		w = 1280
	}
	if h <= 0 {
		h = 720
	}
	y := float64(h) * 0.62
	return Geometry{
		Kind:       "polyline",
		PitSide:    "below",
		BufferNorm: 0.02,
		Points: []Point{
			{X: float64(w) * 0.1, Y: y},
			{X: float64(w) * 0.5, Y: y + float64(h)*0.02},
			{X: float64(w) * 0.9, Y: y},
		},
		SourceHint: "ai_candidate_demo",
	}
}
