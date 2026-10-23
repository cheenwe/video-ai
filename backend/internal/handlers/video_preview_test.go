package handlers

import "testing"

// TestPreviewSessionCap verifies the MJPEG concurrency guard: the cap is
// configurable, acquisitions beyond it are rejected, and releases free slots.
func TestPreviewSessionCap(t *testing.T) {
	t.Setenv("BASE_VIDEO_AI_MAX_PREVIEW", "1")
	if maxPreviewSessions() != 1 {
		t.Fatalf("env override ignored: %d", maxPreviewSessions())
	}
	if !acquirePreviewSession() {
		t.Fatal("first acquire should succeed")
	}
	if acquirePreviewSession() {
		t.Fatal("acquire beyond the cap must be rejected")
	}
	releasePreviewSession()
	if !acquirePreviewSession() {
		t.Fatal("slot freed by release was not reusable")
	}
	releasePreviewSession()

	// Default cap when unset/invalid.
	t.Setenv("BASE_VIDEO_AI_MAX_PREVIEW", "")
	if maxPreviewSessions() != 32 {
		t.Fatalf("default cap: %d", maxPreviewSessions())
	}
	t.Setenv("BASE_VIDEO_AI_MAX_PREVIEW", "not-a-number")
	if maxPreviewSessions() != 32 {
		t.Fatalf("invalid env should fall back to default: %d", maxPreviewSessions())
	}
}
