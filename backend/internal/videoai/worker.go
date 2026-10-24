package videoai

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// inferTimeout bounds one worker round-trip; a hung worker is killed and
// respawned on the next tick instead of stalling the analysis loop. It is a
// var so tests can shorten it.
var inferTimeout = 15 * time.Second

// resolveWorkerCmd returns argv for the ONNX worker, or why it cannot run.
// Order: BASE_VIDEO_AI_ONNX_WORKER override, then the worker bundled next to
// the executable (dev layout: backend/workers/onnx/worker.py).
func resolveWorkerCmd() ([]string, string) {
	override := strings.TrimSpace(os.Getenv("BASE_VIDEO_AI_ONNX_WORKER"))
	if override != "" {
		return strings.Fields(override), ""
	}
	if _, err := exec.LookPath("python3"); err != nil {
		return nil, "PATH 中找不到 python3（ONNX worker 需要 Python + onnxruntime）"
	}
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "workers", "onnx", "worker.py"),
			filepath.Join(filepath.Dir(dir), "workers", "onnx", "worker.py"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "workers", "onnx", "worker.py"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return []string{"python3", c}, ""
		}
	}
	return nil, "未找到 ONNX worker 脚本（workers/onnx/worker.py）；可用 BASE_VIDEO_AI_ONNX_WORKER 指定"
}

// pythonHasOnnxruntime reports whether python3 can import onnxruntime;
// self-check surfaces the concrete import error instead of a vague failure.
func pythonHasOnnxruntime() error {
	out, err := exec.Command("python3", "-c", "import onnxruntime").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("python3 无法导入 onnxruntime：%s", msg)
	}
	return nil
}

// pipeRW joins a reader and a write-closer into one stream.
type pipeRW struct {
	io.Reader
	io.WriteCloser
}

// workerSession is one long-lived worker process bound to a model config.
type workerSession struct {
	cfg  ModelConfig
	cmd  *exec.Cmd
	rw   io.ReadWriteCloser
	mu   sync.Mutex
	dead bool
}

// newWorkerSession spawns the worker and performs the init handshake.
func newWorkerSession(cfg ModelConfig) (*workerSession, error) {
	argv, why := resolveWorkerCmd()
	if argv == nil {
		return nil, fmt.Errorf("推理 worker 不可用：%s", why)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stderr = os.Stderr // worker diagnostics stay visible in service logs
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("worker 启动失败: %w", err)
	}
	s := &workerSession{cfg: cfg, cmd: cmd, rw: pipeRW{Reader: stdout, WriteCloser: stdin}}
	if err := s.handshake(); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

// newWorkerSessionWith builds a session over an existing stream (tests).
func newWorkerSessionWith(cfg ModelConfig, rw io.ReadWriteCloser) *workerSession {
	return &workerSession{cfg: cfg, rw: rw}
}

// handshake sends the session config and waits for the worker's metadata.
func (s *workerSession) handshake() error {
	cfgJSON, err := json.Marshal(map[string]any{
		"model_path": s.cfg.ModelPath,
		"providers":  providersFor(s.cfg.Backend),
		"input_name": "images",
		"input_size": []int{s.cfg.InputW, s.cfg.InputH},
		"classes":    s.cfg.Classes,
	})
	if err != nil {
		return err
	}
	if err := writeFrame(s.rw, map[string]any{"op": "init"}, cfgJSON); err != nil {
		return fmt.Errorf("worker init 写入失败: %w", err)
	}
	reply, _, err := readFrame(s.rw)
	if err != nil {
		return fmt.Errorf("worker init 读取失败: %w", err)
	}
	if ok, _ := reply["ok"].(bool); !ok {
		return fmt.Errorf("worker init 失败: %s", replyErr(reply))
	}
	return nil
}

// run sends one tensor and decodes the output tensors.
//
// The returned tensors borrow pooled buffers: the caller must call
// releaseTensors once it is done reading them (postprocess only reads).
func (s *workerSession) run(tensor []float32, cfg ModelConfig) ([]rawTensor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return nil, fmt.Errorf("推理 worker 已退出")
	}
	payload := encodeFloats(tensor)
	defer putBytes(payload)
	hdr := map[string]any{
		"op": "infer", "n": len(payload),
		"name": "images", "shape": []int{1, 3, cfg.InputH, cfg.InputW},
	}
	if err := writeFrame(s.rw, hdr, payload); err != nil {
		s.dead = true
		return nil, fmt.Errorf("推理请求写入失败: %w", err)
	}

	type result struct {
		outs []rawTensor
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		outs, err := readInferReply(s.rw)
		ch <- result{outs, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			s.dead = true
			return nil, r.err
		}
		return r.outs, nil
	case <-time.After(inferTimeout):
		s.dead = true
		s.kill()
		return nil, fmt.Errorf("推理超时（%s），worker 已终止", inferTimeout)
	}
}

// ------------------------------------------------------ wire buffers ----
//
// One inference moves a 3x640x640 float32 tensor in (~4.9 MB) and a
// [1,84,8400] output out (~2.8 MB). Encoding those on the heap per frame makes
// the allocator, not the model, the limit on how many cameras a host can run,
// so both sides draw from pools. Buffers are only ever live between run() and
// postprocess, both synchronous, so reuse cannot race.

var bytePool = sync.Pool{New: func() any { return new([]byte) }}

func getBytes(n int) []byte {
	p := bytePool.Get().(*[]byte)
	if cap(*p) < n {
		*p = make([]byte, n)
	}
	return (*p)[:n]
}

func putBytes(b []byte) {
	if cap(b) == 0 {
		return
	}
	bytePool.Put(&b)
}

// encodeFloats serialises the tensor as little-endian float32, the layout the
// worker's numpy.frombuffer expects.
func encodeFloats(t []float32) []byte {
	out := getBytes(len(t) * 4)
	for i, v := range t {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

// releaseTensors returns the data buffers of a run() result. Call it once
// postprocess has read them; nothing may retain rawTensor.Data afterwards.
func releaseTensors(outs []rawTensor) {
	for i := range outs {
		putFloats(outs[i].Data)
		outs[i].Data = nil
	}
}

// decodeFloats turns a wire payload into a pooled float32 slice and hands the
// byte buffer back.
func decodeFloats(payload []byte) []float32 {
	if len(payload) == 0 {
		return nil
	}
	out := getFloats(len(payload) / 4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(payload[i*4:]))
	}
	putBytes(payload)
	return out
}

var floatPool = sync.Pool{New: func() any { return new([]float32) }}

func getFloats(n int) []float32 {
	p := floatPool.Get().(*[]float32)
	if cap(*p) < n {
		*p = make([]float32, n)
	}
	return (*p)[:n]
}

func putFloats(f []float32) {
	if cap(f) == 0 {
		return
	}
	floatPool.Put(&f)
}

// readInferReply reads the infer reply header plus one framed payload per
// declared output tensor.
func readInferReply(r io.Reader) ([]rawTensor, error) {
	reply, _, err := readFrame(r)
	if err != nil {
		return nil, fmt.Errorf("推理响应读取失败: %w", err)
	}
	if ok, _ := reply["ok"].(bool); !ok {
		return nil, fmt.Errorf("推理失败: %s", replyErr(reply))
	}
	rawOutputs, _ := reply["outputs"].([]any)
	outs := make([]rawTensor, 0, len(rawOutputs))
	for _, item := range rawOutputs {
		m, _ := item.(map[string]any)
		if m == nil {
			return nil, fmt.Errorf("推理响应 outputs 格式错误")
		}
		name, _ := m["name"].(string)
		shape := intSlice(m["shape"])
		_, payload, err := readFrame(r)
		if err != nil {
			return nil, fmt.Errorf("输出张量 %s 读取失败: %w", name, err)
		}
		if len(payload)%4 != 0 {
			return nil, fmt.Errorf("输出张量 %s 长度不是 4 的倍数", name)
		}
		data := decodeFloats(payload)
		outs = append(outs, rawTensor{Name: name, Shape: shape, Data: data})
	}
	return outs, nil
}

func (s *workerSession) alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.dead
}

func (s *workerSession) kill() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

func (s *workerSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return
	}
	s.dead = true
	_ = s.rw.Close()
	s.kill()
	if s.cmd != nil {
		go func() { _ = s.cmd.Wait() }()
	}
}

// sessionKey identifies a worker by everything that affects its session.
func sessionKey(cfg ModelConfig) string {
	return fmt.Sprintf("%s|%s|%dx%d|%g|%g|%s",
		cfg.ModelPath, cfg.Backend, cfg.InputW, cfg.InputH,
		cfg.Confidence, cfg.IoU, strings.Join(cfg.Classes, ","))
}

func providersFor(backend string) []string {
	if strings.EqualFold(backend, "gpu") || strings.EqualFold(backend, "cuda") {
		return []string{"CUDAExecutionProvider", "CPUExecutionProvider"}
	}
	return []string{"CPUExecutionProvider"}
}

func intSlice(v any) []int {
	items, _ := v.([]any)
	out := make([]int, 0, len(items))
	for _, it := range items {
		if f, ok := it.(float64); ok {
			out = append(out, int(f))
		}
	}
	return out
}

func replyErr(reply map[string]any) string {
	if msg, _ := reply["error"].(string); msg != "" {
		return msg
	}
	return "未知错误"
}

// ------------------------------------------------------------ framing ----

// writeFrame writes a length-prefixed JSON header plus an optional payload.
// The payload length is declared in the header's "n" field so the reader
// knows to consume it; forgetting that desynchronises the stream.
func writeFrame(w io.Writer, header any, payload []byte) error {
	h := header
	if payload != nil {
		if m, ok := header.(map[string]any); ok {
			cp := make(map[string]any, len(m)+1)
			for k, v := range m {
				cp[k] = v
			}
			cp["n"] = len(payload)
			h = cp
		}
	}
	hb, err := json.Marshal(h)
	if err != nil {
		return err
	}
	// Header and payload are written separately so a multi-megabyte tensor is
	// never copied into an intermediate buffer first. The stream is guarded by
	// the session mutex, so splitting the write cannot interleave.
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(hb)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := w.Write(hb); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err = w.Write(payload)
	}
	return err
}

// readFrame reads one framed message and returns its header and payload.
func readFrame(r io.Reader) (map[string]any, []byte, error) {
	hdr, err := readN(r, 4)
	if err != nil {
		return nil, nil, err
	}
	hlen := binary.BigEndian.Uint32(hdr)
	if hlen == 0 || hlen > 1<<20 {
		return nil, nil, fmt.Errorf("非法头长度 %d", hlen)
	}
	hb, err := readN(r, int(hlen))
	if err != nil {
		return nil, nil, err
	}
	var header map[string]any
	if err := json.Unmarshal(hb, &header); err != nil {
		return nil, nil, fmt.Errorf("头 JSON 解析失败: %w", err)
	}
	var payload []byte
	if n, ok := header["n"].(float64); ok && n > 0 {
		if n > 1<<30 {
			return nil, nil, fmt.Errorf("非法负载长度 %v", n)
		}
		payload, err = readN(r, int(n))
		if err != nil {
			return nil, nil, err
		}
	}
	return header, payload, nil
}

func readN(r io.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
