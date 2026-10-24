package recorder

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestRecorderProducesRealSegments drives the real ffmpeg binary against a
// locally generated clip. It proves the segmenter invocation is valid and that
// files land where List() looks — not just that the bookkeeping compiles.
// Skipped when ffmpeg is unavailable, because that is an environment gap, not
// a code failure.
func TestRecorderProducesRealSegments(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg 不可用，跳过真实分段集成验证")
	}
	src := filepath.Join(t.TempDir(), "src.mp4")
	mk := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=10", "-t", "4",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", src)
	if out, err := mk.CombinedOutput(); err != nil {
		t.Skipf("无法生成测试素材（缺 libx264？）：%v %s", err, out)
	}

	m := NewManager(t.TempDir())
	if err := m.Start(42, src, 7, 50, ""); err != nil {
		t.Fatal(err)
	}
	defer m.Stop(42)

	deadline := time.Now().Add(20 * time.Second)
	var segs []Segment
	for time.Now().Before(deadline) {
		segs, _ = m.List(42)
		if len(segs) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(segs) == 0 {
		t.Fatal("ffmpeg 未产出任何分段（分段命令可能非法）")
	}
	if segs[0].SizeBytes == 0 {
		t.Fatal("分段文件为空")
	}
	// The segment must be a real MP4 container: ffprobe-free check via header.
	path := filepath.Join(m.Dir(42), segs[0].Name)
	head := make([]byte, 12)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Read(head); err != nil {
		t.Fatal(err)
	}
	if string(head[4:8]) != "ftyp" {
		t.Fatalf("产出文件不是 MP4（head=%q）", head)
	}
}
