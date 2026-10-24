package ingest

import (
	"bytes"
	"testing"
)

func fakeJPEG(payload string) []byte {
	return append(append([]byte{0xFF, 0xD8}, []byte(payload)...), 0xFF, 0xD9)
}

// TestJpegScannerSplitAcrossReads feeds frames in awkward chunks, including a
// read that ends mid-marker, because that is exactly what a pipe delivers.
func TestJpegScannerSplitAcrossReads(t *testing.T) {
	frames := []string{"aaa", "bb", "cccc", "d"}
	var stream []byte
	for _, p := range frames {
		stream = append(stream, fakeJPEG(p)...)
	}
	// Junk before the first frame must be skipped.
	stream = append([]byte("garbage\x00\x01"), stream...)

	got := []string{}
	sc := &jpegScanner{}
	// 1-byte chunks: the worst case for marker detection.
	for i := 0; i < len(stream); i++ {
		sc.feed(stream[i:i+1], func(j []byte) { got = append(got, string(j)) })
	}
	if len(got) != len(frames) {
		t.Fatalf("frames = %d, want %d (%q)", len(got), len(frames), got)
	}
	for i, f := range frames {
		if want := string(fakeJPEG(f)); got[i] != want {
			t.Fatalf("frame %d = %q, want %q", i, got[i], want)
		}
	}
}

// TestJpegScannerPartialStartMarker keeps a trailing 0xFF so a marker split
// across two reads is still recognised.
func TestJpegScannerPartialStartMarker(t *testing.T) {
	sc := &jpegScanner{}
	got := []string{}
	sc.feed([]byte("xx\xFF"), func(j []byte) { got = append(got, string(j)) })
	sc.feed(append([]byte{0xD8}, []byte("body")...), func(j []byte) { got = append(got, string(j)) })
	sc.feed([]byte{0xFF, 0xD9}, func(j []byte) { got = append(got, string(j)) })
	if len(got) != 1 {
		t.Fatalf("frames = %d (%q), want 1", len(got), got)
	}
	if want := string(fakeJPEG("body")); got[0] != want {
		t.Fatalf("frame = %q, want %q", got[0], want)
	}
}

// TestJpegScannerNoUnboundedGrowth guards the offset bookkeeping: feeding many
// frames must not grow the accumulation buffer without bound.
func TestJpegScannerNoUnboundedGrowth(t *testing.T) {
	sc := &jpegScanner{}
	one := fakeJPEG("payload-payload-payload")
	for i := 0; i < 500; i++ {
		sc.feed(one, func([]byte) {})
	}
	if len(sc.buf) > 8192 {
		t.Fatalf("accumulation buffer grew to %d bytes after 500 complete frames", len(sc.buf))
	}
}

// TestJpegScannerEmitsInOrder is a whole-stream sanity check.
func TestJpegScannerEmitsInOrder(t *testing.T) {
	var stream []byte
	want := []string{}
	for i := 0; i < 50; i++ {
		p := bytes.Repeat([]byte{byte('a' + i%26)}, i+1)
		stream = append(stream, fakeJPEG(string(p))...)
		want = append(want, string(fakeJPEG(string(p))))
	}
	got := []string{}
	sc := &jpegScanner{}
	sc.feed(stream, func(j []byte) { got = append(got, string(j)) })
	if len(got) != len(want) {
		t.Fatalf("frames = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frame %d mismatch (%d vs %d bytes)", i, len(got[i]), len(want[i]))
		}
	}
}

// BenchmarkJpegScannerFeed measures the scanner against a realistic MJPEG
// stream: ~40 KB frames delivered in 64 KB reads.
func BenchmarkJpegScannerFeed(b *testing.B) {
	frame := fakeJPEG(string(bytes.Repeat([]byte{0x5A}, 40*1024)))
	stream := make([]byte, 0, len(frame)*3)
	stream = append(stream, frame...)
	stream = append(stream, frame...)
	stream = append(stream, frame...)
	drop := func([]byte) {}
	b.SetBytes(int64(len(stream)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc := &jpegScanner{}
		// 64 KB reads, matching drainFrames.
		for off := 0; off < len(stream); off += 64 * 1024 {
			end := off + 64*1024
			if end > len(stream) {
				end = len(stream)
			}
			sc.feed(stream[off:end], drop)
		}
	}
}
