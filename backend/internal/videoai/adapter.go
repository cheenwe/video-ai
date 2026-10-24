package videoai

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Frame is one decoded analysis frame. Inferencers must consume the pixels;
// a tick without a fresh frame is reported offline by the runtime instead of
// being answered with synthetic detections.
type Frame struct {
	TS   time.Time
	JPEG []byte
	W    int
	H    int
}

// ModelConfig carries the per-version inference settings resolved from the
// AlgoModelVersion row (and its schemas) at deployment start.
type ModelConfig struct {
	VersionID  uint
	Adapter    string
	ModelPath  string
	Backend    string   // cpu | gpu -> ORT execution providers
	Classes    []string // label table used to decode outputs
	InputW     int      // letterbox target width
	InputH     int      // letterbox target height
	Confidence float64
	IoU        float64
}

// Inferencer produces detections for one analysis frame. Implementations must
// be safe for concurrent use across deployment loops.
type Inferencer interface {
	// Name matches AlgoModelVersion.Adapter.
	Name() string
	// Demo reports whether results are synthetic. Production adapters must
	// return false; demo adapters must never be presented as real accuracy.
	Demo() bool
	// Available reports whether inference can run right now, with a
	// human-readable reason when it cannot. Self-check surfaces it.
	Available() (bool, string)
	// Infer returns detections for one decoded frame. Real adapters return an
	// error instead of fabricating results.
	Infer(cameraID uint, f Frame, cfg ModelConfig) ([]Detection, error)
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

// closableAdapter is implemented by adapters that hold external resources
// (worker processes, sessions).
type closableAdapter interface {
	CloseAll()
}

// CloseAdapters releases external resources held by registered adapters;
// called when the analysis manager stops.
func CloseAdapters() {
	adapterMu.RLock()
	impls := make([]Inferencer, 0, len(adapterImpls))
	for _, impl := range adapterImpls {
		impls = append(impls, impl)
	}
	adapterMu.RUnlock()
	for _, impl := range impls {
		if c, ok := impl.(closableAdapter); ok {
			c.CloseAll()
		}
	}
}

// SelfCheckRunner is implemented by adapters that can verify a concrete model
// version end-to-end (load + sample inference). Self-check uses it so a
// published version is proven runnable instead of assumed.
type SelfCheckRunner interface {
	SelfCheck(cfg ModelConfig) SelfCheckResult
}

// SelfCheckResult reports load / sample-inference outcomes for one version.
type SelfCheckResult struct {
	LoadOK       bool
	LoadDetail   string
	SampleOK     bool
	SampleDetail string
}

// onnxAdapter runs real inference through an external ONNX Runtime worker
// (see backend/workers/onnx/worker.py). The platform owns frame intake,
// pre-processing and post-processing; the worker owns the ORT session, so no
// CGO/ORT binding is compiled into this binary.
// workerPoolSize is how many worker processes a single model key may run.
//
// One worker handles one inference at a time, so a single session serialises
// every camera bound to that model: with 8 cameras at 2 FPS and a 60 ms model,
// the last camera would wait half a second behind the first. The pool is sized
// from GOMAXPROCS (capped, since each worker is a Python process with its own
// ORT session and memory) and can be pinned with
// BASE_VIDEO_AI_ONNX_WORKERS.
var workerPoolSize = func() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		n = 1
	}
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("BASE_VIDEO_AI_ONNX_WORKERS"))); err == nil && v > 0 {
		n = v
	}
	if n > 8 {
		n = 8
	}
	return n
}()

// modelWorkers is the set of sessions serving one model key. Sessions are
// handed out round-robin; a busy one simply makes the caller wait for the next
// free slot instead of for the whole model.
type modelWorkers struct {
	sessions []*workerSession
	next     uint64
}

func (mw *modelWorkers) pick() *workerSession {
	if len(mw.sessions) == 0 {
		return nil
	}
	// Cheap round-robin: strict least-loaded tracking would need per-session
	// counters that go stale immediately anyway.
	i := atomic.AddUint64(&mw.next, 1)
	return mw.sessions[int(i)%len(mw.sessions)]
}

type onnxAdapter struct {
	mu       sync.Mutex
	sessions map[string]*modelWorkers
}

func (a *onnxAdapter) Name() string { return "onnx" }
func (a *onnxAdapter) Demo() bool   { return false }

func (a *onnxAdapter) Available() (bool, string) {
	argv, why := resolveWorkerCmd()
	if argv == nil {
		return false, why
	}
	if err := pythonHasOnnxruntime(); err != nil {
		return false, err.Error()
	}
	return true, strings.Join(argv, " ")
}

func (a *onnxAdapter) Infer(cameraID uint, f Frame, cfg ModelConfig) ([]Detection, error) {
	if len(f.JPEG) == 0 {
		return nil, fmt.Errorf("推理缺少视频帧：ingest 未提供像素")
	}
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("模型版本未配置文件路径")
	}
	tensor, lb, err := preprocess(f.JPEG, cfg.InputW, cfg.InputH)
	if err != nil {
		return nil, fmt.Errorf("帧预处理失败: %w", err)
	}
	// The tensor is pooled; it only has to outlive the synchronous worker
	// round-trip, after which the buffer goes back for the next frame.
	defer putTensor(tensor)
	sess, err := a.session(cfg)
	if err != nil {
		return nil, err
	}
	outs, err := sess.run(tensor, cfg)
	if err != nil {
		// A dead worker (model reload, OOM) must not stick: drop it so the
		// next tick respawns.
		a.dropSession(cfg)
		return nil, err
	}
	defer releaseTensors(outs)
	return postprocess(outs, cfg, lb, f.W, f.H)
}

// session returns a live worker for the config, spawning one on demand and
// growing the pool up to workerPoolSize as cameras contend for it.
//
// The pool grows lazily: a deployment with one camera pays for one worker.
// Growth is what makes multi-camera deployments parallel instead of queued.
func (a *onnxAdapter) session(cfg ModelConfig) (*workerSession, error) {
	key := sessionKey(cfg)

	a.mu.Lock()
	if a.sessions == nil {
		a.sessions = map[string]*modelWorkers{}
	}
	mw := a.sessions[key]
	if mw != nil {
		if s := mw.pick(); s != nil && s.alive() {
			a.mu.Unlock()
			return s, nil
		}
	}
	a.mu.Unlock()

	// Spawning costs a Python + ORT startup, so it happens outside the lock;
	// concurrent spawns are bounded by demand (one per contending camera).
	s, err := newWorkerSession(cfg)
	if err != nil {
		return nil, err
	}

	// Every mutation of mw.sessions happens under a.mu; readers (pick) hold it
	// too, so the slice is never written while another goroutine walks it.
	a.mu.Lock()
	mw = a.sessions[key]
	if mw == nil {
		a.sessions[key] = &modelWorkers{sessions: []*workerSession{s}}
		a.mu.Unlock()
		return s, nil
	}
	for i, old := range mw.sessions {
		if !old.alive() {
			mw.sessions[i] = s
			a.mu.Unlock()
			old.close()
			return s, nil
		}
	}
	if len(mw.sessions) < workerPoolSize {
		mw.sessions = append(mw.sessions, s)
		a.mu.Unlock()
		return s, nil
	}
	// At capacity: this spawn raced another one, so retire the extra process.
	live := mw.pick()
	a.mu.Unlock()
	s.close()
	return live, nil
}

// dropSession retires the workers of a model key after a failure so the next
// tick respawns them against a freshly loaded model.
func (a *onnxAdapter) dropSession(cfg ModelConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if mw, ok := a.sessions[sessionKey(cfg)]; ok {
		for _, s := range mw.sessions {
			s.close()
		}
		delete(a.sessions, sessionKey(cfg))
	}
}

// CloseAll shuts every worker down; called on manager stop.
func (a *onnxAdapter) CloseAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, mw := range a.sessions {
		for _, s := range mw.sessions {
			s.close()
		}
		delete(a.sessions, k)
	}
}

// SelfCheck proves the version is runnable: it boots the worker with the real
// model and runs one inference on a synthetic frame.
func (a *onnxAdapter) SelfCheck(cfg ModelConfig) SelfCheckResult {
	res := SelfCheckResult{}
	if cfg.ModelPath == "" {
		res.LoadDetail = "未配置模型文件路径"
		return res
	}
	if ok, why := a.Available(); !ok {
		res.LoadDetail = "推理 worker 不可用：" + why
		return res
	}
	sess, err := a.session(cfg)
	if err != nil {
		res.LoadDetail = "worker 启动失败: " + err.Error()
		return res
	}
	res.LoadOK = true
	res.LoadDetail = "worker 已加载模型 " + cfg.ModelPath

	tensor, _, err := preprocess(syntheticFrameJPEG(), cfg.InputW, cfg.InputH)
	if err != nil {
		res.SampleDetail = "样例帧构造失败: " + err.Error()
		return res
	}
	defer putTensor(tensor)
	dets, err := sess.run(tensor, cfg)
	if err != nil {
		a.dropSession(cfg)
		res.SampleDetail = "样例推理失败: " + err.Error()
		return res
	}
	defer releaseTensors(dets)
	res.SampleOK = true
	res.SampleDetail = fmt.Sprintf("样例推理完成，输出 %d 个检测", len(dets))
	return res
}

func init() {
	RegisterAdapter("onnx", &onnxAdapter{})
}
