package ingest

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

// ------------------------------------------------------- annex-B helpers ----

// splitAnnexB turns an elementary stream into access units, each prefixed with
// SPS/PPS when it starts a keyframe so a decoder can start from it.
func splitAnnexB(raw []byte) [][][]byte {
	var nals [][]byte
	for i := 0; i < len(raw); {
		// find next start code
		j := i
		for j+2 < len(raw) && !(raw[j] == 0 && raw[j+1] == 0 && raw[j+2] == 1) {
			j++
		}
		if j+2 >= len(raw) {
			break
		}
		start := j + 3
		k := start
		for k+2 < len(raw) && !(raw[k] == 0 && raw[k+1] == 0 && raw[k+2] == 1) {
			k++
		}
		nals = append(nals, raw[start:k])
		i = k
	}
	var sps, pps []byte
	var aus [][][]byte
	var cur [][]byte
	flush := func() {
		if len(cur) > 0 {
			aus = append(aus, cur)
			cur = nil
		}
	}
	for _, n := range nals {
		if len(n) == 0 {
			continue
		}
		typ := n[0] & 0x1F
		switch typ {
		case 7:
			sps = n
			continue
		case 8:
			pps = n
			continue
		case 5: // IDR: start a new access unit incl. parameter sets
			flush()
			cur = [][]byte{}
			if sps != nil {
				cur = append(cur, sps)
			}
			if pps != nil {
				cur = append(cur, pps)
			}
			cur = append(cur, n)
			flush()
		case 1: // non-IDR slice
			flush()
			cur = [][]byte{n}
		default:
			if len(cur) > 0 {
				cur = append(cur, n)
			}
		}
	}
	flush()
	return aus
}

// ------------------------------------------------------------ RTSP server ----

// rtspTestServer serves a recorded H264 clip over RTSP so the ingest path can
// be exercised for real rather than mocked.
type rtspTestServer struct {
	mu     sync.RWMutex
	stream *gortsplib.ServerStream
	medi   *description.Media
	forma  *format.H264
	aus    [][][]byte
	closed chan struct{}
}

func (s *rtspTestServer) OnDescribe(_ *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &base.Response{StatusCode: base.StatusOK}, s.stream, nil
}

func (s *rtspTestServer) OnSetup(_ *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &base.Response{StatusCode: base.StatusOK}, s.stream, nil
}

func (s *rtspTestServer) OnPlay(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return &base.Response{StatusCode: base.StatusOK}, nil
}

// serve loops the clip as RTP packets until the test finishes.
func (s *rtspTestServer) serve() {
	enc, err := s.forma.CreateEncoder()
	if err != nil {
		return
	}
	seq := uint16(0)
	ts := uint32(0)
	for {
		select {
		case <-s.closed:
			return
		default:
		}
		for _, au := range s.aus {
			pkts, err := enc.Encode(au)
			if err != nil {
				continue
			}
			s.mu.RLock()
			st := s.stream
			s.mu.RUnlock()
			if st == nil {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			for _, p := range pkts {
				p.SequenceNumber = seq
				p.Timestamp = ts
				seq++
				if err := st.WritePacketRTP(s.medi, p); err != nil {
					// No reader yet, or connection gone: keep looping.
					break
				}
			}
			ts += 3000
			time.Sleep(40 * time.Millisecond)
		}
	}
}

// startRTSP pushes a generated clip as an RTSP stream and returns its URL.
func startRTSP(t *testing.T, raw []byte) string {
	t.Helper()
	aus := splitAnnexB(raw)
	if len(aus) == 0 {
		t.Skip("no access units in test clip")
	}
	var sps, pps []byte
	for _, au := range aus {
		for _, n := range au {
			switch n[0] & 0x1F {
			case 7:
				sps = n
			case 8:
				pps = n
			}
		}
		if sps != nil && pps != nil {
			break
		}
	}
	if sps == nil || pps == nil {
		t.Skip("test clip has no SPS/PPS")
	}
	forma := &format.H264{PayloadTyp: 96, SPS: sps, PPS: pps, PacketizationMode: 1}
	medi := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{forma},
	}

	h := &rtspTestServer{medi: medi, forma: forma, aus: aus, closed: make(chan struct{})}
	srv := &gortsplib.Server{Handler: h, RTSPAddress: "127.0.0.1:8557"}
	// Block readers until the stream exists.
	h.mu.Lock()
	if err := srv.Start(); err != nil {
		h.mu.Unlock()
		t.Skipf("cannot start RTSP server: %v", err)
	}
	h.stream = &gortsplib.ServerStream{
		Server: srv,
		Desc:   &description.Session{Medias: []*description.Media{medi}},
	}
	if err := h.stream.Initialize(); err != nil {
		h.mu.Unlock()
		srv.Close()
		t.Skipf("cannot init stream: %v", err)
	}
	h.mu.Unlock()
	go h.serve()

	t.Cleanup(func() {
		close(h.closed)
		srv.Close()
	})
	return "rtsp://127.0.0.1:8557/stream"
}

// TestIngestPullsRealStream is the end-to-end proof that ingest connects to a
// live RTSP source, decodes it and exposes frames. It is skipped when ffmpeg
// is unavailable because decoding is delegated to it.
func TestIngestPullsRealStream(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	raw := filepath.Join(dir, "src.h264")
	out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=3",
		"-c:v", "libx264", "-g", "10", "-pix_fmt", "yuv420p", "-f", "h264", raw).CombinedOutput()
	if err != nil {
		t.Skipf("cannot encode test clip: %v %s", err, out)
	}
	rawBytes, err := os.ReadFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	url := startRTSP(t, rawBytes)

	m := NewManager(dir)
	defer m.StopAll()
	if err := m.Ensure(42, url); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if f, ok := m.Latest(42); ok && len(f.JPEG) > 0 {
			if f.W <= 0 || f.H <= 0 {
				t.Fatalf("frame without dimensions: %+v", f)
			}
			if !bytes.HasPrefix(f.JPEG, []byte{0xFF, 0xD8}) {
				t.Fatal("frame is not a JPEG")
			}
			st, _ := m.State(42)
			if !st.Connected {
				t.Fatal("stream reports disconnected while frames arrive")
			}
			if st.BufferBytes <= 0 {
				t.Fatal("rolling buffer stayed empty")
			}
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	st, _ := m.State(42)
	t.Fatalf("no frame arrived; state=%+v", st)
}

// TestIngestClipFromRealStream proves an MP4 clip can be produced from what
// the puller buffered, not from a synthetic file.
func TestIngestClipFromRealStream(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	raw := filepath.Join(dir, "src.h264")
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=3",
		"-c:v", "libx264", "-g", "10", "-pix_fmt", "yuv420p", "-f", "h264", raw).CombinedOutput(); err != nil {
		t.Skipf("cannot encode test clip: %v %s", err, out)
	}
	rawBytes, err := os.ReadFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	url := startRTSP(t, rawBytes)

	m := NewManager(dir)
	defer m.StopAll()
	if err := m.Ensure(43, url); err != nil {
		t.Fatal(err)
	}

	// Wait until the buffer has real content.
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := m.State(43); ok && st.BufferBytes > 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	dest := filepath.Join(dir, "event.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := m.Clip(ctx, 43, 5, 0, dest); err != nil {
		t.Fatalf("Clip: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Size() == 0 {
		t.Fatalf("clip not produced: %v", err)
	}
}
