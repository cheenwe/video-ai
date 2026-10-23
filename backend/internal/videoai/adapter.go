package videoai

import (
	"fmt"
	"sync"
	"time"
)

// Inferencer produces detections for one analysis frame. Implementations must
// be safe for concurrent use across deployment loops.
type Inferencer interface {
	// Name matches AlgoModelVersion.Adapter (mock | onnx | ...).
	Name() string
	// Demo reports whether results are synthetic. Demo adapters must never be
	// presented as real detection accuracy.
	Demo() bool
	// Infer returns detections for a frame of the given size. Real adapters
	// should return an error instead of fabricating results.
	Infer(cameraID uint, frameTS time.Time, w, h float64) ([]Detection, error)
}

var (
	adapterMu    sync.RWMutex
	adapterImpls = map[string]Inferencer{}
)

// RegisterAdapter makes an inferencer available by adapter name. Registering
// again replaces the previous implementation (e.g. swapping the onnx
// placeholder for a real ONNX Runtime worker).
func RegisterAdapter(name string, impl Inferencer) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	adapterImpls[name] = impl
}

// AdapterByName resolves a registered inferencer.
func AdapterByName(name string) (Inferencer, error) {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	if impl, ok := adapterImpls[name]; ok {
		return impl, nil
	}
	return nil, fmt.Errorf("推理适配器 %q 未注册", name)
}

// RegisteredAdapters lists adapter names with their demo flag, for
// self-check reports.
func RegisteredAdapters() []map[string]any {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	out := make([]map[string]any, 0, len(adapterImpls))
	for _, impl := range adapterImpls {
		out = append(out, map[string]any{"name": impl.Name(), "demo": impl.Demo()})
	}
	return out
}

// onnxPlaceholder refuses to infer until a real ONNX Runtime worker is wired
// in. Failing loudly is deliberate: publishing an onnx version must not make
// the platform pretend to run real inference.
type onnxPlaceholder struct{}

func (onnxPlaceholder) Name() string { return "onnx" }
func (onnxPlaceholder) Demo() bool   { return false }
func (onnxPlaceholder) Infer(uint, time.Time, float64, float64) ([]Detection, error) {
	return nil, fmt.Errorf("ONNX Runtime 未接入：请实现真实推理 Adapter/Worker 后再使用 onnx 版本")
}

func init() {
	RegisterAdapter("mock", MockAdapter{})
	RegisterAdapter("onnx", onnxPlaceholder{})
}
