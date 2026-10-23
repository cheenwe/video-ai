package videoai

import (
	"math"
	"testing"
)

// channelFirst builds a YOLO v5/v8 style [1, 4+nc, N] payload.
func channelFirst(n, nc int, rows [][]float32) []float32 {
	cols := 4 + nc
	out := make([]float32, cols*n)
	for i, r := range rows {
		for c, v := range r {
			out[c*n+i] = v
		}
	}
	return out
}

// anchorFirst builds a [1, N, 4+nc] payload.
func anchorFirst(rows [][]float32) []float32 {
	var out []float32
	for _, r := range rows {
		out = append(out, r...)
	}
	return out
}

func testCfg() ModelConfig {
	return ModelConfig{
		Adapter: "onnx", InputW: 640, InputH: 640,
		Classes: []string{"grab_bucket"}, Confidence: 0.5, IoU: 0.45,
	}
}

// TestPostprocessChannelFirst decodes a v5/v8 layout, applies the confidence
// threshold and maps boxes back through the letterbox.
func TestPostprocessChannelFirst(t *testing.T) {
	lb := letterbox{gain: 1, padX: 0, padY: 0, inW: 640, inH: 640}
	rows := [][]float32{
		{100, 100, 40, 40, 0.9},  // strong hit
		{300, 300, 20, 20, 0.2},  // below threshold
		{500, 100, 10, 10, 0.05}, // low class score for class 0
	}
	outs := []rawTensor{{Name: "output0", Shape: []int{1, 5, 3}, Data: channelFirst(3, 1, rows)}}
	dets, err := postprocess(outs, testCfg(), lb, 640, 640)
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 detection, got %d (%+v)", len(dets), dets)
	}
	d := dets[0]
	if d.Label != "grab_bucket" || diff32(d.Confidence, 0.9) > 1e-6 {
		t.Fatalf("detection meta %+v", d)
	}
	if d.Centroid.X != 100 || d.Centroid.Y != 100 {
		t.Fatalf("centroid %+v", d.Centroid)
	}
	if len(d.Contour) != 4 {
		t.Fatalf("contour points %d", len(d.Contour))
	}
	if d.Contour[0].X != 80 || d.Contour[0].Y != 80 {
		t.Fatalf("corner %+v", d.Contour[0])
	}
}

// TestPostprocessAnchorFirst accepts the transposed layout too.
func TestPostprocessAnchorFirst(t *testing.T) {
	lb := letterbox{gain: 1, padX: 0, padY: 0, inW: 640, inH: 640}
	rows := [][]float32{
		{100, 100, 40, 40, 0.8},
		{200, 200, 10, 10, 0.1},
	}
	outs := []rawTensor{{Name: "output0", Shape: []int{1, 2, 5}, Data: anchorFirst(rows)}}
	dets, err := postprocess(outs, testCfg(), lb, 640, 640)
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1 detection, got %d", len(dets))
	}
}

// TestPostprocessNMSSuppressesOverlap keeps the higher-confidence box.
func TestPostprocessNMSSuppressesOverlap(t *testing.T) {
	lb := letterbox{gain: 1, padX: 0, padY: 0, inW: 640, inH: 640}
	rows := [][]float32{
		{100, 100, 40, 40, 0.9},
		{102, 101, 38, 41, 0.7}, // heavy overlap with the first
		{400, 400, 30, 30, 0.6}, // separate box survives
	}
	outs := []rawTensor{{Name: "output0", Shape: []int{1, 5, 3}, Data: channelFirst(3, 1, rows)}}
	dets, err := postprocess(outs, testCfg(), lb, 640, 640)
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 2 {
		t.Fatalf("want 2 after NMS, got %d (%+v)", len(dets), dets)
	}
	if diff32(dets[0].Confidence, 0.9) > 1e-6 || diff32(dets[1].Confidence, 0.6) > 1e-6 {
		t.Fatalf("NMS kept wrong boxes: %+v", dets)
	}
}

// diff32 is a float32-tolerant comparison helper.
func diff32(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestPostprocessLetterboxInverse maps a padded box back to source coords.
func TestPostprocessLetterboxInverse(t *testing.T) {
	// 640x640 input holds a 320x160 source scaled 2x, centered.
	lb := letterbox{gain: 2, padX: 0, padY: 240, inW: 640, inH: 640}
	rows := [][]float32{
		{320, 320, 40, 40, 0.9}, // input center
	}
	outs := []rawTensor{{Name: "output0", Shape: []int{1, 5, 1}, Data: channelFirst(1, 1, rows)}}
	dets, err := postprocess(outs, testCfg(), lb, 320, 160)
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 1 {
		t.Fatalf("want 1, got %d", len(dets))
	}
	if math.Abs(dets[0].Centroid.X-160) > 0.01 || math.Abs(dets[0].Centroid.Y-40) > 0.01 {
		t.Fatalf("centroid not mapped to source: %+v", dets[0].Centroid)
	}
	// Below the source frame: clamped, not negative.
	if dets[0].Contour[2].Y > 160+0.01 {
		t.Fatalf("y not clamped: %+v", dets[0].Contour[2])
	}
}

// TestPostprocessRejectsBadShape keeps malformed worker output loud.
func TestPostprocessRejectsBadShape(t *testing.T) {
	outs := []rawTensor{{Name: "output0", Shape: []int{1, 7, 7}, Data: make([]float32, 49)}}
	if _, err := postprocess(outs, testCfg(), letterbox{gain: 1}, 640, 640); err == nil {
		t.Fatal("expected shape error")
	}
	if _, err := postprocess(nil, testCfg(), letterbox{gain: 1}, 640, 640); err == nil {
		t.Fatal("expected empty-output error")
	}
}

// TestPostprocessNoClassesUsesSingleClass covers a missing classes table.
func TestPostprocessNoClassesUsesSingleClass(t *testing.T) {
	cfg := testCfg()
	cfg.Classes = nil
	lb := letterbox{gain: 1, inW: 640, inH: 640}
	rows := [][]float32{{100, 100, 20, 20, 0.9}}
	outs := []rawTensor{{Name: "output0", Shape: []int{1, 5, 1}, Data: channelFirst(1, 1, rows)}}
	dets, err := postprocess(outs, cfg, lb, 640, 640)
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 1 || dets[0].Label != "class0" {
		t.Fatalf("unexpected %+v", dets)
	}
}
