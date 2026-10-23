package ingest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

// TestDebugEncode checks whether the RTP encoder accepts the access units we
// feed the test server. Kept as a diagnostic; skipped by default.
func TestDebugEncode(t *testing.T) {
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
	b, _ := os.ReadFile(raw)
	aus := splitAnnexB(b)
	t.Logf("aus=%d", len(aus))

	// Collect parameter sets like startRTSP does.
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
	forma := &format.H264{PayloadTyp: 96, SPS: sps, PPS: pps, PacketizationMode: 1}
	enc, err := forma.CreateEncoder()
	if err != nil {
		t.Fatalf("CreateEncoder: %v", err)
	}
	ok, bad := 0, 0
	for i, au := range aus {
		pkts, err := enc.Encode(au)
		if err != nil {
			if bad < 3 {
				t.Logf("AU %d encode error: %v (nals=%d)", i, err, len(au))
			}
			bad++
			continue
		}
		ok++
		_ = pkts
	}
	t.Logf("encoded ok=%d bad=%d", ok, bad)
	if ok == 0 {
		t.Fatal("encoder rejected every access unit")
	}
}
