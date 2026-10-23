package ingest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestDebugDumpBuffer writes the elementary stream the puller produced and
// checks it with ffprobe. It exists to diagnose stream corruption, and is
// skipped unless VIDEOAI_DEBUG_DUMP is set.
func TestDebugDumpBuffer(t *testing.T) {
	if os.Getenv("VIDEOAI_DEBUG_DUMP") == "" {
		t.Skip("set VIDEOAI_DEBUG_DUMP=1 to run")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	raw := filepath.Join(dir, "src.h264")
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=3",
		"-c:v", "libx264", "-g", "10", "-pix_fmt", "yuv420p", "-f", "h264", raw).CombinedOutput(); err != nil {
		t.Skipf("cannot encode: %v %s", err, out)
	}
	srcBytes, _ := os.ReadFile(raw)
	t.Logf("source bytes=%d", len(srcBytes))

	url := startRTSP(t, srcBytes)
	m := NewManager(dir)
	defer m.StopAll()
	if err := m.Ensure(77, url); err != nil {
		t.Fatal(err)
	}
	// Let the stream accumulate for a fixed window.
	time.Sleep(10 * time.Second)
	src, cleanup, err := func() (string, func(), error) {
		m.mu.Lock()
		s := m.streams[77]
		m.mu.Unlock()
		return s.freezeBuffer()
	}()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	dumped, _ := os.ReadFile(src)
	t.Logf("buffered bytes=%d", len(dumped))
	if st, ok := m.State(77); ok {
		t.Logf("state: connected=%v packets=%d decodeErrors=%d frames=%d bytes=%d",
			st.Connected, st.Packets, st.DecodeErrors, st.Frames, st.Bytes)
	}
	if err := os.WriteFile("/tmp/videoai-debug.h264", dumped, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-hide_banner", "-loglevel", "warning",
		"-f", "h264", "-i", "/tmp/videoai-debug.h264", "-show_entries", "stream=codec_name,width,height", "-of", "csv=p=0").CombinedOutput()
	t.Logf("ffprobe buffered: err=%v out=%s", err, string(out))

	srcOut, srcErr := exec.Command("ffprobe", "-hide_banner", "-loglevel", "warning",
		"-f", "h264", "-i", raw, "-show_entries", "stream=codec_name,width,height", "-of", "csv=p=0").CombinedOutput()
	t.Logf("ffprobe source: err=%v out=%s", srcErr, string(srcOut))

	// How many frames can actually be decoded out of the buffer?
	decOut, decErr := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "h264", "-r", "25", "-i", "/tmp/videoai-debug.h264", "-an", "-f", "null", "-").CombinedOutput()
	t.Logf("ffmpeg decode buffered: err=%v out=%s", decErr, string(decOut))
	srcDec, srcDecErr := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "h264", "-r", "25", "-i", raw, "-an", "-f", "null", "-").CombinedOutput()
	t.Logf("ffmpeg decode source: err=%v out=%s", srcDecErr, string(srcDec))
}
