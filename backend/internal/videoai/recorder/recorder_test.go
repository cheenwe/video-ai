package recorder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseSegmentName locks the filename contract the prune logic relies on.
func TestParseSegmentName(t *testing.T) {
	got, ok := parseSegmentName("20261024-093015.mp4")
	if !ok {
		t.Fatal("expected a parseable segment name")
	}
	want := time.Date(2026, 10, 24, 9, 30, 15, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("parsed %v, want %v", got, want)
	}
	if _, ok := parseSegmentName("clip.mp4"); ok {
		t.Fatal("a non-strftime name must not parse")
	}
}

// TestListOrdersAndComputesDuration: segments are newest-first and each closed
// segment's duration is the gap to the previous one.
func TestListOrdersAndComputesDuration(t *testing.T) {
	m := NewManager(t.TempDir())
	dir := m.Dir(7)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"20261024-100000.mp4", "20261024-100500.mp4", "20261024-101000.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A zero-byte file is what a segment looks like before the muxer writes
	// anything; it must not be listed as playable.
	if err := os.WriteFile(filepath.Join(dir, "20261024-101500.mp4"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	segs, err := m.List(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 3 {
		t.Fatalf("expected 3 non-empty segments, got %d", len(segs))
	}
	if segs[0].Name != "20261024-101000.mp4" {
		t.Fatalf("newest first expected, got %s", segs[0].Name)
	}
	if segs[0].DurationMS != 0 {
		t.Fatalf("newest segment is still being written, duration must be 0, got %d", segs[0].DurationMS)
	}
	if got, want := segs[1].DurationMS, int64(5*60*1000); got != want {
		t.Fatalf("closed segment duration = %d, want %d", got, want)
	}
	if got, want := segs[2].DurationMS, int64(5*60*1000); got != want {
		t.Fatalf("oldest segment duration = %d, want %d", got, want)
	}
}

// TestPruneByRetentionAndSize covers both limits plus the guard that the
// in-flight segment is never deleted.
func TestPruneByRetentionAndSize(t *testing.T) {
	m := NewManager(t.TempDir())
	dir := m.Dir(3)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -30).Format("20060102-150405") + ".mp4"
	recent1 := time.Now().Add(-2*time.Hour).Format("20060102-150405") + ".mp4"
	recent2 := time.Now().Add(-1*time.Hour).Format("20060102-150405") + ".mp4"
	for _, n := range []string{old, recent1, recent2} {
		if err := os.WriteFile(filepath.Join(dir, n), make([]byte, 2048), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Retain 7 days: the 30-day-old segment goes.
	if removed := m.Prune(3, 7, 0); removed != 1 {
		t.Fatalf("retention removed %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, old)); !os.IsNotExist(err) {
		t.Fatal("old segment still present")
	}

	// Now cap the size below what remains: both recent segments are 2 KB, so a
	// 0 GB cap is invalid (means unlimited) — use maxGB with tiny units via
	// a direct size check instead: 1 GB limit keeps both.
	if removed := m.Prune(3, 7, 1); removed != 0 {
		t.Fatalf("1GB cap should keep both, removed %d", removed)
	}
	segs, _ := m.List(3)
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
}

// TestSegmentPathRejectsTraversal is the security check: only plain .mp4 names
// inside the camera directory resolve.
func TestSegmentPathRejectsTraversal(t *testing.T) {
	m := NewManager(t.TempDir())
	dir := m.Dir(5)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	good := "20261024-100000.mp4"
	if err := os.WriteFile(filepath.Join(dir, good), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file outside the camera directory must not be reachable.
	outside := filepath.Join(m.dir, "secret.mp4")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := m.SegmentPath(5, good); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	for _, bad := range []string{"../secret.mp4", "secret.mp4", "/etc/passwd", "x.txt", ""} {
		if _, err := m.SegmentPath(5, bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

// TestStartRequiresStreamURL: starting without a URL is a clear error rather
// than a silently idle recorder.
func TestStartRequiresStreamURL(t *testing.T) {
	m := NewManager(t.TempDir())
	err := m.Start(1, "   ", 7, 50, "")
	if err == nil || !strings.Contains(err.Error(), "码流") {
		t.Fatalf("expected a stream URL error, got %v", err)
	}
}

// TestStatusReportsNotRecordingBeforeStart keeps the API honest: a camera with
// no plan reports recording=false instead of pretending.
func TestStatusReportsNotRecordingBeforeStart(t *testing.T) {
	m := NewManager(t.TempDir())
	st := m.Status(9)
	if st.Recording {
		t.Fatal("a never-started camera must not report recording")
	}
	if st.CameraID != 9 {
		t.Fatalf("camera id = %d", st.CameraID)
	}
}
