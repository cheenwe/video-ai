// Package ingest pulls real video from RTSP streams and keeps a decoded
// latest-frame queue plus a rolling encoded buffer. It is what lets analysis,
// preview and evidence stop being purely synthetic.
//
// Decoding is delegated to ffmpeg (a subprocess) because a native H264/H265
// decoder would drag in CGO and platform-specific toolchains. When ffmpeg is
// unavailable the stream still connects and buffers, but no JPEG frames and no
// clips can be produced — that is reported, never faked.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
)

// reconnectDelay is the backoff between reconnect attempts.
const reconnectDelay = 3 * time.Second

// Frame is one decoded image held in memory.
type Frame struct {
	TS   time.Time
	JPEG []byte
	W    int
	H    int
}

// State describes one ingest stream for UI and self-check.
type State struct {
	CameraID    uint       `json:"camera_id"`
	URL         string     `json:"url"`
	Codec       string     `json:"codec"`
	Running     bool       `json:"running"`
	Connected   bool       `json:"connected"`
	Frames      int64      `json:"frames"`
	Bytes       int64      `json:"bytes"`
	// Packets / DecodeErrors help ops tell "no packets arriving" from
	// "packets arrive but cannot be assembled into frames".
	Packets      int64 `json:"packets"`
	DecodeErrors int64 `json:"decode_errors"`
	FrameAgeMS  int64      `json:"frame_age_ms"`
	BufferBytes int64      `json:"buffer_bytes"`
	LastFrameAt *time.Time `json:"last_frame_at"`
	LastError   string     `json:"last_error"`
	StartedAt   time.Time  `json:"started_at"`
}

// Manager owns the per-camera ingest streams.
type Manager struct {
	dataHome string
	logger   *log.Logger

	mu      sync.Mutex
	streams map[uint]*stream
}

// NewManager builds an ingest manager rooted at dataHome.
func NewManager(dataHome string) *Manager {
	return &Manager{
		dataHome: dataHome,
		logger:   log.New(os.Stdout, "[ingest] ", log.LstdFlags|log.Lmsgprefix),
		streams:  map[uint]*stream{},
	}
}

// Ensure starts ingest for a camera, restarting it when the URL changed.
func (m *Manager) Ensure(cameraID uint, rawURL string) error {
	if rawURL == "" {
		return errors.New("码流地址为空")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.streams[cameraID]; ok {
		if s.rawURL == rawURL {
			return nil
		}
		s.stop()
		delete(m.streams, cameraID)
	}
	s := newStream(cameraID, rawURL, m.dataHome, m.logger)
	m.streams[cameraID] = s
	s.start()
	return nil
}

// Stop tears down ingest for a camera.
func (m *Manager) Stop(cameraID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.streams[cameraID]; ok {
		s.stop()
		delete(m.streams, cameraID)
	}
}

// StopAll tears down every running stream.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.streams {
		s.stop()
		delete(m.streams, id)
	}
}

// Latest returns the most recent decoded frame, if any has arrived yet.
func (m *Manager) Latest(cameraID uint) (Frame, bool) {
	m.mu.Lock()
	s, ok := m.streams[cameraID]
	m.mu.Unlock()
	if !ok {
		return Frame{}, false
	}
	return s.latest()
}

// State reports one stream's liveness.
func (m *Manager) State(cameraID uint) (State, bool) {
	m.mu.Lock()
	s, ok := m.streams[cameraID]
	m.mu.Unlock()
	if !ok {
		return State{}, false
	}
	return s.state(), true
}

// States lists every known stream.
func (m *Manager) States() []State {
	m.mu.Lock()
	list := make([]*stream, 0, len(m.streams))
	for _, s := range m.streams {
		list = append(list, s)
	}
	m.mu.Unlock()
	out := make([]State, 0, len(list))
	for _, s := range list {
		out = append(out, s.state())
	}
	return out
}

// Subscribe returns a channel of decoded frames for a camera plus an
// unsubscribe func. Slow consumers drop frames instead of blocking ingest.
func (m *Manager) Subscribe(cameraID uint, buf int) (<-chan Frame, func()) {
	m.mu.Lock()
	s, ok := m.streams[cameraID]
	m.mu.Unlock()
	if !ok {
		return nil, func() {}
	}
	if buf <= 0 {
		buf = 4
	}
	return s.subscribe(buf)
}

// Snapshot writes the latest decoded frame to dest as a JPEG.
func (m *Manager) Snapshot(cameraID uint, dest string) error {
	f, ok := m.Latest(cameraID)
	if !ok || len(f.JPEG) == 0 {
		return errors.New("尚无可用视频帧（码流未连通或 ffmpeg 不可用）")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, f.JPEG, 0o644)
}

// Clip produces an MP4 covering the last beforeSec seconds plus afterSec of
// extra wait, so the tail of an incident is included. It blocks.
func (m *Manager) Clip(ctx context.Context, cameraID uint, beforeSec, afterSec int, dest string) error {
	if afterSec > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(afterSec) * time.Second):
		}
	}
	m.mu.Lock()
	s, ok := m.streams[cameraID]
	m.mu.Unlock()
	if !ok {
		return errors.New("该摄像头未建立取流")
	}
	src, cleanup, err := s.freezeBuffer()
	if err != nil {
		return err
	}
	defer cleanup()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return muxToMP4(ctx, src, beforeSec+afterSec, dest)
}

// ---------------------------------------------------------------- stream ----

type stream struct {
	cameraID uint
	rawURL   string
	dataHome string
	logger   *log.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu          sync.RWMutex
	codec       string
	connected   bool
	frame       Frame
	hasFrame    bool
	frames       int64
	bytes        int64
	packets      int64
	decodeErrors int64
	lastFrameAt  time.Time
	lastErr     string
	startedAt   time.Time

	bufMu   sync.Mutex
	bufDir  string
	bufCur  *os.File
	bufPrev string
	bufSize int64

	subsMu sync.Mutex
	subs   map[chan Frame]struct{}
}

// subscribe registers a frame consumer.
func (s *stream) subscribe(buf int) (<-chan Frame, func()) {
	ch := make(chan Frame, buf)
	s.subsMu.Lock()
	if s.subs == nil {
		s.subs = map[chan Frame]struct{}{}
	}
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.subsMu.Lock()
			delete(s.subs, ch)
			s.subsMu.Unlock()
			close(ch)
		})
	}
}

// fanout delivers a frame to every subscriber without blocking.
func (s *stream) fanout(f Frame) {
	s.subsMu.Lock()
	for ch := range s.subs {
		select {
		case ch <- f:
		default:
		}
	}
	s.subsMu.Unlock()
}

func newStream(cameraID uint, rawURL, dataHome string, logger *log.Logger) *stream {
	return &stream{
		cameraID:  cameraID,
		rawURL:    rawURL,
		dataHome:  dataHome,
		logger:    logger,
		startedAt: time.Now(),
		bufDir:    filepath.Join(dataHome, "ingest", fmt.Sprintf("%d", cameraID)),
	}
}

func (s *stream) start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop(ctx)
	}()
}

func (s *stream) stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.closeBuffer()
}

func (s *stream) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		err := s.runOnce(ctx)
		s.setError(err)
		if ctx.Err() != nil {
			return
		}
		s.logger.Printf("camera %d 取流中断: %v（%s 后重连）", s.cameraID, err, reconnectDelay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (s *stream) setError(err error) {
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	}
	s.connected = false
	s.mu.Unlock()
}

func (s *stream) runOnce(ctx context.Context) error {
	u, err := base.ParseURL(s.rawURL)
	if err != nil {
		return fmt.Errorf("码流地址无效: %w", err)
	}
	tcp := gortsplib.ProtocolTCP
	c := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host, Protocol: &tcp}
	if err := c.Start(); err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer c.Close()
	// c.Wait() only returns when the connection ends, so shutdown must close
	// it explicitly or Stop() would hang forever.
	go func() {
		<-ctx.Done()
		c.Close()
	}()

	desc, _, err := c.Describe(u)
	if err != nil {
		return fmt.Errorf("DESCRIBE 失败: %w", err)
	}

	var medi *description.Media
	var forma format.Format
	var dec rtpDecoder
	// RTP payloads carry parameter sets out of band, so they must be written
	// back into the elementary stream or nothing can decode it.
	var params [][]byte

	var h264Forma *format.H264
	var h265Forma *format.H265
	if m := desc.FindFormat(&h264Forma); m != nil {
		medi, forma = m, h264Forma
		dec, err = h264Forma.CreateDecoder()
		if err != nil {
			return err
		}
		s.setCodec("h264")
		if sps, pps := h264Forma.SafeParams(); len(sps) > 0 || len(pps) > 0 {
			if len(sps) > 0 {
				params = append(params, sps)
			}
			if len(pps) > 0 {
				params = append(params, pps)
			}
		}
	} else if m := desc.FindFormat(&h265Forma); m != nil {
		medi, forma = m, h265Forma
		dec, err = h265Forma.CreateDecoder()
		if err != nil {
			return err
		}
		s.setCodec("h265")
		if len(h265Forma.VPS) > 0 {
			params = append(params, h265Forma.VPS)
		}
		if len(h265Forma.SPS) > 0 {
			params = append(params, h265Forma.SPS)
		}
		if len(h265Forma.PPS) > 0 {
			params = append(params, h265Forma.PPS)
		}
	} else {
		return errors.New("码流中没有 H264/H265 视频轨")
	}
	paramBytes := toAnnexB(params)

	if err := s.openBuffer(); err != nil {
		return err
	}

	// ffmpeg is optional: without it we still buffer the stream, but no frames.
	fd, decErr := startFFmpegDecoder(ctx, s.codec)
	if decErr != nil {
		s.mu.Lock()
		s.lastErr = decErr.Error()
		s.mu.Unlock()
		s.logger.Printf("camera %d: %v", s.cameraID, decErr)
	}

	var decWG sync.WaitGroup
	if fd != nil {
		decWG.Add(1)
		go func() {
			defer decWG.Done()
			drainFrames(fd.Frames(), s.onFrame)
		}()
	}

	// The media must be set up before a packet callback can be attached.
	if _, err := c.Setup(desc.BaseURL, medi, 0, 0); err != nil {
		if fd != nil {
			_ = fd.close()
		}
		decWG.Wait()
		return fmt.Errorf("SETUP 失败: %w", err)
	}

	// Access units are decoded here and forwarded to both the decoder and the
	// rolling buffer. This runs on gortsplib's reader goroutine.
	paramsSent := false
	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		s.mu.Lock()
		s.packets++
		s.mu.Unlock()
		au, err := dec.Decode(pkt)
		if err != nil || len(au) == 0 {
			s.mu.Lock()
			s.decodeErrors++
			s.mu.Unlock()
			return
		}
		key := isKeyframe(au, s.codec)
		annexB := toAnnexB(au)
		// Repeat parameter sets on every keyframe so a decoder (or a clip cut
		// from the buffer) can start anywhere a keyframe begins.
		if len(paramBytes) > 0 && (!paramsSent || key) {
			withParams := make([]byte, 0, len(paramBytes)+len(annexB))
			withParams = append(withParams, paramBytes...)
			withParams = append(withParams, annexB...)
			annexB = withParams
			paramsSent = true
			key = true
		}
		s.mu.Lock()
		s.bytes += int64(len(annexB))
		s.mu.Unlock()
		if fd != nil {
			_ = fd.Write(annexB)
		}
		s.writeBuffer(annexB, key)
	})

	if _, err := c.Play(nil); err != nil {
		if fd != nil {
			_ = fd.close()
		}
		decWG.Wait()
		return fmt.Errorf("PLAY 失败: %w", err)
	}

	s.mu.Lock()
	s.connected = true
	s.lastErr = ""
	s.mu.Unlock()

	waitErr := c.Wait()
	if fd != nil {
		_ = fd.close()
	}
	decWG.Wait()
	return waitErr
}

func (s *stream) setCodec(c string) {
	s.mu.Lock()
	s.codec = c
	s.mu.Unlock()
}

func (s *stream) onFrame(jpeg []byte, ts time.Time) {
	w, h := jpegSize(jpeg)
	s.mu.Lock()
	s.frame = Frame{TS: ts, W: w, H: h}
	s.frame.JPEG = append([]byte(nil), jpeg...)
	s.hasFrame = true
	s.lastFrameAt = ts
	s.frames++
	snapshot := s.frame
	s.mu.Unlock()
	s.fanout(Frame{TS: snapshot.TS, W: w, H: h, JPEG: snapshot.JPEG})
}

func (s *stream) latest() (Frame, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.hasFrame {
		return Frame{}, false
	}
	out := Frame{TS: s.frame.TS, W: s.frame.W, H: s.frame.H}
	out.JPEG = append([]byte(nil), s.frame.JPEG...)
	return out, true
}

func (s *stream) state() State {
	s.mu.RLock()
	st := State{
		CameraID:   s.cameraID,
		URL:        s.rawURL,
		Codec:      s.codec,
		Running:    true,
		Connected:  s.connected,
		Frames:       s.frames,
		Bytes:        s.bytes,
		Packets:      s.packets,
		DecodeErrors: s.decodeErrors,
		LastError:  s.lastErr,
		StartedAt:  s.startedAt,
		FrameAgeMS: -1,
	}
	if s.hasFrame {
		st.FrameAgeMS = time.Since(s.frame.TS).Milliseconds()
		ts := s.frame.TS
		st.LastFrameAt = &ts
	}
	s.mu.RUnlock()

	s.bufMu.Lock()
	st.BufferBytes = s.bufSize
	s.bufMu.Unlock()
	return st
}
