package videoai

import (
	"fmt"
	"math"
	"sort"
)

// rawTensor is one worker output tensor: name, shape and float32 payload.
type rawTensor struct {
	Name  string
	Shape []int
	Data  []float32
}

// box is an axis-aligned box in source frame coordinates.
type box struct {
	x1, y1, x2, y2 float64
	score          float64
	class          int
}

// postprocess decodes YOLO-style detect outputs into detections in source
// frame coordinates. It accepts channel-first [1, 4+nc, N] (v5/v8 exports),
// anchor-first [1, N, 4+nc] and plain [N, 4+nc] layouts.
func postprocess(outs []rawTensor, cfg ModelConfig, lb letterbox, origW, origH int) ([]Detection, error) {
	if len(outs) == 0 {
		return nil, fmt.Errorf("推理未返回输出张量")
	}
	det := pickDetectOutput(outs, len(cfg.Classes))
	nc := len(cfg.Classes)
	if nc == 0 {
		nc = 1
	}
	cols := 4 + nc
	channelFirst := len(det.Shape) == 3 && det.Shape[1] == cols
	var n int
	switch {
	case len(det.Shape) == 3 && det.Shape[1] == cols:
		n = det.Shape[2]
	case len(det.Shape) == 3 && det.Shape[2] == cols:
		n = det.Shape[1]
	case len(det.Shape) == 2 && det.Shape[1] == cols:
		n = det.Shape[0]
	default:
		return nil, fmt.Errorf("无法识别的输出形状 %v（期望含 %d 列）", det.Shape, cols)
	}
	if n <= 0 || len(det.Data) < n*cols {
		return nil, fmt.Errorf("输出数据长度 %d 与形状 %v 不符", len(det.Data), det.Shape)
	}

	conf := cfg.Confidence
	if conf <= 0 {
		conf = 0.25
	}
	var boxes []box
	for i := 0; i < n; i++ {
		cx, cy, bw, bh, scores := anchorAt(det.Data, i, n, cols, nc, channelFirst)
		if bw <= 0 || bh <= 0 {
			continue
		}
		best, bestScore := 0, scores[0]
		for c := 1; c < nc; c++ {
			if scores[c] > bestScore {
				best, bestScore = c, scores[c]
			}
		}
		if float64(bestScore) < conf {
			continue
		}
		// Letterboxed pixels -> source pixels.
		x1, y1 := lb.toSource(cx-bw/2, cy-bh/2)
		x2, y2 := lb.toSource(cx+bw/2, cy+bh/2)
		if origW > 0 {
			x1 = clamp(x1, 0, float64(origW))
			x2 = clamp(x2, 0, float64(origW))
		}
		if origH > 0 {
			y1 = clamp(y1, 0, float64(origH))
			y2 = clamp(y2, 0, float64(origH))
		}
		if x2 <= x1 || y2 <= y1 {
			continue
		}
		boxes = append(boxes, box{x1: x1, y1: y1, x2: x2, y2: y2, score: float64(bestScore), class: best})
	}
	boxes = nms(boxes, cfg.IoU)

	out := make([]Detection, 0, len(boxes))
	for _, b := range boxes {
		label := fmt.Sprintf("class%d", b.class)
		if b.class < len(cfg.Classes) {
			label = cfg.Classes[b.class]
		}
		out = append(out, Detection{
			Label:      label,
			Confidence: b.score,
			Centroid:   Point{X: (b.x1 + b.x2) / 2, Y: (b.y1 + b.y2) / 2},
			Contour: []Point{
				{X: b.x1, Y: b.y1}, {X: b.x2, Y: b.y1},
				{X: b.x2, Y: b.y2}, {X: b.x1, Y: b.y2},
			},
		})
	}
	return out, nil
}

// pickDetectOutput selects the detect tensor: an explicit "detect"/"output0"
// name wins, otherwise the first output with a plausible detect shape.
func pickDetectOutput(outs []rawTensor, classes int) rawTensor {
	cols := 4 + classes
	for _, o := range outs {
		if o.Name == "output0" || o.Name == "detect" || o.Name == "output" {
			return o
		}
	}
	for _, o := range outs {
		if len(o.Shape) == 3 && (o.Shape[1] == cols || o.Shape[2] == cols) {
			return o
		}
		if len(o.Shape) == 2 && o.Shape[1] == cols {
			return o
		}
	}
	return outs[0]
}

// anchorAt reads one anchor from the flat output payload.
func anchorAt(data []float32, i, n, cols, nc int, channelFirst bool) (cx, cy, bw, bh float64, scores []float64) {
	scores = make([]float64, nc)
	if channelFirst {
		cx = float64(data[i])
		cy = float64(data[n+i])
		bw = float64(data[2*n+i])
		bh = float64(data[3*n+i])
		for c := 0; c < nc; c++ {
			scores[c] = float64(data[(4+c)*n+i])
		}
		return
	}
	base := i * cols
	cx = float64(data[base])
	cy = float64(data[base+1])
	bw = float64(data[base+2])
	bh = float64(data[base+3])
	for c := 0; c < nc; c++ {
		scores[c] = float64(data[base+4+c])
	}
	return
}

// nms greedily keeps the highest-confidence boxes, suppressing overlaps.
func nms(boxes []box, iou float64) []box {
	if iou <= 0 {
		iou = 0.45
	}
	sort.SliceStable(boxes, func(i, j int) bool { return boxes[i].score > boxes[j].score })
	keep := make([]box, 0, len(boxes))
	drop := make([]bool, len(boxes))
	for i := range boxes {
		if drop[i] {
			continue
		}
		keep = append(keep, boxes[i])
		for j := i + 1; j < len(boxes); j++ {
			if drop[j] {
				continue
			}
			if iouOf(boxes[i], boxes[j]) > iou {
				drop[j] = true
			}
		}
	}
	return keep
}

func iouOf(a, b box) float64 {
	x1 := math.Max(a.x1, b.x1)
	y1 := math.Max(a.y1, b.y1)
	x2 := math.Min(a.x2, b.x2)
	y2 := math.Min(a.y2, b.y2)
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	inter := (x2 - x1) * (y2 - y1)
	areaA := (a.x2 - a.x1) * (a.y2 - a.y1)
	areaB := (b.x2 - b.x1) * (b.y2 - b.y1)
	return inter / (areaA + areaB - inter)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
