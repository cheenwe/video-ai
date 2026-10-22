package videoai

import (
	"encoding/json"
	"fmt"
	"math"
)

// Point is a normalized or pixel coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Geometry describes a pit-edge boundary.
type Geometry struct {
	Kind       string  `json:"kind"` // polyline | polygon
	Points     []Point `json:"points"`
	PitSide    string  `json:"pit_side"` // left | right | above | below relative to directed polyline
	BufferNorm float64 `json:"buffer_norm"`
	ROI        []Point `json:"roi,omitempty"`
	SourceHint string  `json:"source_hint,omitempty"` // demo | ai_candidate_demo
}

func ParseGeometry(raw string) (Geometry, error) {
	var g Geometry
	if raw == "" {
		return g, fmt.Errorf("边界几何为空")
	}
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return g, fmt.Errorf("边界几何无效: %w", err)
	}
	if len(g.Points) < 2 {
		return g, fmt.Errorf("边界至少需要 2 个点")
	}
	if g.Kind == "" {
		g.Kind = "polyline"
	}
	if g.PitSide == "" {
		g.PitSide = "below"
	}
	return g, nil
}

func ValidateGeometry(g Geometry, width, height int) error {
	if len(g.Points) < 2 {
		return fmt.Errorf("边界至少需要 2 个点")
	}
	if g.Kind == "polygon" && len(g.Points) < 3 {
		return fmt.Errorf("多边形至少需要 3 个点")
	}
	for i, p := range g.Points {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) {
			return fmt.Errorf("第 %d 个点坐标无效", i+1)
		}
		if width > 0 && height > 0 {
			if p.X < 0 || p.Y < 0 || p.X > float64(width) || p.Y > float64(height) {
				return fmt.Errorf("第 %d 个点超出参考帧范围", i+1)
			}
		}
	}
	if selfIntersects(g.Points, g.Kind == "polygon") {
		return fmt.Errorf("边界线段自交")
	}
	return nil
}

func selfIntersects(pts []Point, closed bool) bool {
	n := len(pts)
	segCount := n - 1
	if closed {
		segCount = n
	}
	for i := 0; i < segCount; i++ {
		a1, a2 := pts[i], pts[(i+1)%n]
		for j := i + 1; j < segCount; j++ {
			b1, b2 := pts[j], pts[(j+1)%n]
			if closed {
				if i == 0 && j == segCount-1 {
					continue
				}
			}
			if j == i+1 || (closed && i == 0 && j == n-1) {
				continue
			}
			if segmentsIntersect(a1, a2, b1, b2) {
				return true
			}
		}
	}
	return false
}

func segmentsIntersect(a, b, c, d Point) bool {
	o1 := orient(a, b, c)
	o2 := orient(a, b, d)
	o3 := orient(c, d, a)
	o4 := orient(c, d, b)
	if o1 != o2 && o3 != o4 {
		return true
	}
	return false
}

func orient(a, b, c Point) int {
	v := (b.Y-a.Y)*(c.X-b.X) - (b.X-a.X)*(c.Y-b.Y)
	if v > 1e-9 {
		return 1
	}
	if v < -1e-9 {
		return -1
	}
	return 0
}

// SignedDistanceToPolyline returns signed distance of point to polyline (positive = outside pit).
func SignedDistanceToPolyline(p Point, g Geometry) float64 {
	if len(g.Points) < 2 {
		return math.Inf(1)
	}
	minAbs := math.Inf(1)
	var bestSigned float64
	for i := 0; i < len(g.Points)-1; i++ {
		d, side := distToSegmentSigned(p, g.Points[i], g.Points[i+1], g.PitSide)
		if math.Abs(d) < minAbs {
			minAbs = math.Abs(d)
			bestSigned = d
			_ = side
		}
	}
	return bestSigned
}

func distToSegmentSigned(p, a, b Point, pitSide string) (float64, float64) {
	vx, vy := b.X-a.X, b.Y-a.Y
	wx, wy := p.X-a.X, p.Y-a.Y
	c1 := vx*wx + vy*wy
	if c1 <= 0 {
		return hypot(p.X-a.X, p.Y-a.Y) * sideSign(p, a, b, pitSide), 0
	}
	c2 := vx*vx + vy*vy
	if c2 <= c1 {
		return hypot(p.X-b.X, p.Y-b.Y) * sideSign(p, a, b, pitSide), 0
	}
	t := c1 / c2
	proj := Point{X: a.X + t*vx, Y: a.Y + t*vy}
	return hypot(p.X-proj.X, p.Y-proj.Y) * sideSign(p, a, b, pitSide), t
}

func sideSign(p, a, b Point, pitSide string) float64 {
	cross := (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
	if cross == 0 {
		return 1
	}
	// Image coordinates (y down): for AB left→right, cross<0 means above on screen.
	var outside bool
	switch pitSide {
	case "below":
		outside = cross < 0
	case "above":
		outside = cross > 0
	case "right":
		outside = cross > 0
	case "left":
		outside = cross < 0
	default:
		outside = cross < 0
	}
	if outside {
		return 1
	}
	return -1
}

func hypot(x, y float64) float64 { return math.Hypot(x, y) }

// PointInPolygon ray casting.
func PointInPolygon(p Point, poly []Point) bool {
	if len(poly) < 3 {
		return false
	}
	inside := false
	j := len(poly) - 1
	for i := 0; i < len(poly); i++ {
		pi, pj := poly[i], poly[j]
		if ((pi.Y > p.Y) != (pj.Y > p.Y)) &&
			(p.X < (pj.X-pi.X)*(p.Y-pi.Y)/(pj.Y-pi.Y+1e-12)+pi.X) {
			inside = !inside
		}
		j = i
	}
	return inside
}
