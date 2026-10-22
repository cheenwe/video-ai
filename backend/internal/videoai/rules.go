package videoai

import "fmt"

const (
	RuleNearEdge      = "grab_near_edge"
	RuleCrossBoundary = "grab_cross_boundary"
	ResultNormal      = "normal"
	ResultNear        = "near"
	ResultCross       = "cross"
	ResultUnknown     = "unknown"
	ResultOffline     = "offline"
)

// Detection is a mock/real grab bucket instance.
type Detection struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Centroid   Point   `json:"centroid"`
	Contour    []Point `json:"contour,omitempty"`
}

// RuleInput feeds the geometric rules.
type RuleInput struct {
	Detections     []Detection
	Geometry       Geometry
	ThresholdNear  float64 // normalized distance
	ThresholdCross float64 // <=0 means any outside counts as cross
	FrameW         float64
	FrameH         float64
	MediaOK        bool
}

// RuleOutput is the per-frame verdict.
type RuleOutput struct {
	Result   string             `json:"result"`
	Distance float64            `json:"distance"`
	RuleCode string             `json:"rule_code"`
	Metrics  map[string]float64 `json:"metrics"`
	Overlay  map[string]any     `json:"overlay"`
	Reason   string             `json:"reason"`
}

// EvaluateGrabRules applies grab_near_edge / grab_cross_boundary.
func EvaluateGrabRules(in RuleInput) RuleOutput {
	out := RuleOutput{Metrics: map[string]float64{}, Overlay: map[string]any{}}
	if !in.MediaOK {
		out.Result = ResultOffline
		out.Reason = "视频不可用"
		return out
	}
	if len(in.Detections) == 0 {
		out.Result = ResultUnknown
		out.Reason = "未检测到抓斗"
		return out
	}
	if err := ValidateGeometry(in.Geometry, int(in.FrameW), int(in.FrameH)); err != nil {
		out.Result = ResultUnknown
		out.Reason = err.Error()
		return out
	}

	best := in.Detections[0]
	for _, d := range in.Detections[1:] {
		if d.Confidence > best.Confidence {
			best = d
		}
	}
	dist := SignedDistanceToPolyline(best.Centroid, in.Geometry)
	// Normalize roughly by frame diagonal.
	diag := hypot(in.FrameW, in.FrameH)
	if diag < 1 {
		diag = 1
	}
	norm := dist / diag
	out.Distance = norm
	out.Metrics["distance_norm"] = norm
	out.Metrics["confidence"] = best.Confidence
	out.Overlay["centroid"] = best.Centroid
	out.Overlay["contour"] = best.Contour
	out.Overlay["boundary"] = in.Geometry.Points

	crossTh := in.ThresholdCross
	near := in.ThresholdNear
	if near <= 0 {
		near = 0.15
	}
	if norm > crossTh {
		out.Result = ResultCross
		out.RuleCode = RuleCrossBoundary
		out.Reason = fmt.Sprintf("抓斗越过坑边（归一化距离 %.3f）", norm)
		return out
	}
	if norm > -near {
		out.Result = ResultNear
		out.RuleCode = RuleNearEdge
		out.Reason = fmt.Sprintf("抓斗临边（归一化距离 %.3f）", norm)
		return out
	}
	out.Result = ResultNormal
	out.Reason = "正常"
	return out
}
