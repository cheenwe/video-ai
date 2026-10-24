package videoai

import (
	"fmt"
	"sort"
	"strings"
)

// edgeLabelMarkers are the label fragments that identify a detection as the
// pit edge itself rather than the grab bucket. A model trained only for
// buckets cannot produce a boundary, and inventing one from bucket boxes would
// be exactly the fabricated geometry this platform removed.
var edgeLabelMarkers = []string{
	"edge", "border", "boundary", "kerb", "curb", "rim", "坑边", "边界",
}

// isEdgeLabel reports whether a detection class describes the pit edge.
func isEdgeLabel(label string) bool {
	l := strings.ToLower(strings.TrimSpace(label))
	if l == "" {
		return false
	}
	for _, m := range edgeLabelMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}

// candidatePoint is a detection centroid clamped to the frame.
type candidatePoint struct {
	x, y  float64
	score float64
}

// BoundaryCandidate is a suggested boundary derived from one real frame.
type BoundaryCandidate struct {
	Geometry Geometry
	// EdgeDetections is how many detections fed the polyline; Confidence is
	// their mean score. Both are reported so an operator can judge the
	// suggestion instead of trusting a black box.
	EdgeDetections int
	Confidence     float64
}

// DeriveBoundary turns edge-class detections from one frame into a candidate
// polyline in frame pixels. It never invents points: with fewer than two edge
// detections it returns an error, which the API surfaces as "model has no pit
// edge output" instead of a fake line.
//
// Vertices are ordered left→right in the source frame, matching how a manual
// calibration polyline is drawn, and the pit side is inferred from where the
// non-edge detections (the grab bucket) sit relative to the line.
func DeriveBoundary(dets []Detection, frameW, frameH int) (BoundaryCandidate, error) {
	cand := BoundaryCandidate{}
	if len(dets) == 0 {
		return cand, fmt.Errorf("模型未输出任何检测，无法生成坑边候选")
	}

	var edge, others []candidatePoint
	for _, d := range dets {
		p := candidatePoint{x: d.Centroid.X, y: d.Centroid.Y, score: d.Confidence}
		// Clamp into the frame: a detection outside the picture would produce
		// a polyline the boundary validator rejects later.
		if frameW > 0 {
			p.x = clamp(p.x, 0, float64(frameW))
		}
		if frameH > 0 {
			p.y = clamp(p.y, 0, float64(frameH))
		}
		if isEdgeLabel(d.Label) {
			edge = append(edge, p)
		} else {
			others = append(others, p)
		}
	}
	if len(edge) < 2 {
		return cand, fmt.Errorf("模型输出中没有坑边类检测（坑边 %d 个、其他目标 %d 个）；"+
			"自动候选需要能输出坑边/边界类的分割或检测模型，请换用已发布模型或手动绘制", len(edge), len(others))
	}

	sort.SliceStable(edge, func(i, j int) bool { return edge[i].x < edge[j].x })

	sum := 0.0
	points := make([]Point, 0, len(edge))
	for _, p := range edge {
		// Drop points that do not advance in x: two detections of the same
		// segment would otherwise create a zero-width spike.
		if n := len(points); n > 0 && p.x <= points[n-1].X {
			continue
		}
		points = append(points, Point{X: p.x, Y: p.y})
		sum += p.score
	}
	if len(points) < 2 {
		return cand, fmt.Errorf("坑边检测集中在同一竖直位置，无法构成有效折线；请调整视角后重试")
	}

	cand.Geometry = Geometry{
		Kind:       "polyline",
		Points:     points,
		PitSide:    inferPitSide(points, others),
		BufferNorm: 0.02,
		SourceHint: "ai_candidate",
	}
	cand.EdgeDetections = len(edge)
	cand.Confidence = sum / float64(len(edge))
	return cand, nil
}

// inferPitSide decides which side of the polyline the pit is on by comparing
// where the non-edge detections sit relative to the line at their own x.
// Returns "below" when there is no evidence either way, matching the manual
// editor default.
func inferPitSide(line []Point, others []candidatePoint) string {
	if len(line) < 2 || len(others) == 0 {
		return "below"
	}
	above, below := 0, 0
	for _, o := range others {
		ly, ok := lineYAt(line, o.x)
		if !ok {
			continue
		}
		switch {
		case o.y < ly:
			above++
		case o.y > ly:
			below++
		}
	}
	if above > below {
		return "above"
	}
	return "below"
}

// lineYAt interpolates the polyline's y at x, clamping to the end vertices for
// points outside the span.
func lineYAt(line []Point, x float64) (float64, bool) {
	if len(line) < 2 {
		return 0, false
	}
	if x <= line[0].X {
		return line[0].Y, true
	}
	last := line[len(line)-1]
	if x >= last.X {
		return last.Y, true
	}
	for i := 1; i < len(line); i++ {
		a, b := line[i-1], line[i]
		if x <= b.X {
			if b.X == a.X {
				return b.Y, true
			}
			t := (x - a.X) / (b.X - a.X)
			return a.Y + t*(b.Y-a.Y), true
		}
	}
	return last.Y, true
}
