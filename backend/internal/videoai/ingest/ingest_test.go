package ingest

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestToAnnexB(t *testing.T) {
	au := [][]byte{{0x65, 0x01}, {0x41, 0x02, 0x03}}
	got := toAnnexB(au)
	want := []byte{0, 0, 0, 1, 0x65, 0x01, 0, 0, 0, 1, 0x41, 0x02, 0x03}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func TestIsKeyframe(t *testing.T) {
	// H264: NAL type 5 is an IDR slice.
	if !isKeyframe([][]byte{{0x65, 0x88}}, "h264") {
		t.Fatal("h264 IDR not detected")
	}
	if isKeyframe([][]byte{{0x41, 0x9a}}, "h264") {
		t.Fatal("h264 non-IDR wrongly flagged")
	}
	// H265: NAL type 19 (IDR_W_RADL).
	if !isKeyframe([][]byte{{0x26, 0x01}}, "h265") {
		t.Fatal("h265 IDR not detected")
	}
	if isKeyframe([][]byte{{0x02, 0x01}}, "h265") {
		t.Fatal("h265 non-IDR wrongly flagged")
	}
}

func TestJPEGScannerSplitsConcatenatedFrames(t *testing.T) {
	f1 := []byte{0xFF, 0xD8, 0x01, 0x02, 0xFF, 0xD9}
	f2 := []byte{0xFF, 0xD8, 0x03, 0xFF, 0xD9}
	var got [][]byte
	sc := &jpegScanner{}
	sc.feed(append(append([]byte{}, f1...), f2...), func(b []byte) { got = append(got, b) })
	if len(got) != 2 || !bytes.Equal(got[0], f1) || !bytes.Equal(got[1], f2) {
		t.Fatalf("unexpected split: %v", got)
	}
}

func TestJPEGScannerHandlesChunkBoundaries(t *testing.T) {
	frame := []byte{0xFF, 0xD8, 0x0A, 0x0B, 0x0C, 0xFF, 0xD9}
	var got [][]byte
	sc := &jpegScanner{}
	for i := 0; i < len(frame); i++ {
		sc.feed([]byte{frame[i]}, func(b []byte) { got = append(got, b) })
	}
	if len(got) != 1 || !bytes.Equal(got[0], frame) {
		t.Fatalf("frame lost across chunk boundary: %v", got)
	}
}

func TestJPEGSize(t *testing.T) {
	// Minimal SOF0 marker: FF C0 <len> <prec> <h> <w>
	b := []byte{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x11, 0x08, 0x01, 0xE0, 0x02, 0x80}
	w, h := jpegSize(b)
	if w != 640 || h != 480 {
		t.Fatalf("got %dx%d want 640x480", w, h)
	}
}

func TestBufferRotatesAtKeyframe(t *testing.T) {
	dir := t.TempDir()
	s := newStream(7, "rtsp://example/1", dir, nil)
	if err := s.openBuffer(); err != nil {
		t.Fatal(err)
	}
	defer s.closeBuffer()
	// Exceed the cap, then feed a keyframe to trigger rotation.
	chunk := make([]byte, 1<<20)
	s.writeBuffer(chunk, false)
	for i := 0; i < 50; i++ {
		s.writeBuffer(chunk, false)
	}
	before := s.bufSize
	if before <= maxBufferBytes {
		t.Fatalf("buffer did not grow past cap: %d", before)
	}
	s.writeBuffer([]byte{0x00, 0x00, 0x00, 0x01, 0x65}, true)
	if s.bufSize >= before {
		t.Fatalf("rotation did not reset size: %d -> %d", before, s.bufSize)
	}
}

// TestClipRequiresFFmpeg checks that clip generation degrades honestly when
// ffmpeg is absent instead of pretending to have produced video.
func TestClipRequiresFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	// Build a short H264 elementary stream with real keyframes.
	raw := filepath.Join(dir, "in.h264")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=6",
		"-c:v", "libx264", "-g", "10", "-f", "h264", raw)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot encode test stream: %v %s", err, out)
	}
	dest := filepath.Join(dir, "out.mp4")
	if err := muxToMP4(context.Background(), raw, 0, dest); err != nil {
		t.Fatalf("muxToMP4: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Size() == 0 {
		t.Fatalf("clip not produced: %v", err)
	}
}

// TestMuxToMP4RejectsGarbage ensures an unusable buffer reports failure rather
// than emitting an empty clip.
func TestMuxToMP4RejectsGarbage(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	raw := filepath.Join(dir, "junk.h264")
	if err := os.WriteFile(raw, binary.BigEndian.AppendUint32(nil, 0xdeadbeef), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := muxToMP4(context.Background(), raw, 5, filepath.Join(dir, "out.mp4")); err == nil {
		t.Fatal("expected failure for undecodable input")
	}
}

func TestStateReportsFrameAge(t *testing.T) {
	dir := t.TempDir()
	s := newStream(3, "rtsp://example/3", dir, nil)
	if st := s.state(); st.FrameAgeMS != -1 {
		t.Fatalf("expected -1 without frames, got %d", st.FrameAgeMS)
	}
	s.onFrame([]byte{0xFF, 0xD8, 0xFF, 0xD9}, time.Now().Add(-2*time.Second))
	st := s.state()
	if st.FrameAgeMS < 1500 || st.Frames != 1 {
		t.Fatalf("unexpected state: age=%d frames=%d", st.FrameAgeMS, st.Frames)
	}
}
