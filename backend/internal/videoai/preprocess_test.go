package videoai

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// solidJPEG builds a deterministic JPEG of a solid color.
func solidJPEG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	drawRect(img, img.Bounds(), c)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func drawRect(img *image.RGBA, r image.Rectangle, c color.Color) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

// TestPreprocessSameSizeExact verifies the tensor pipeline is near-lossless
// when no scaling is needed: pixels become RGB/255 in NCHW order (JPEG is
// lossy, so values are compared with tolerance).
func TestPreprocessSameSizeExact(t *testing.T) {
	src := solidJPEG(t, 2, 2, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	tensor, lb, err := preprocess(src, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tensor) != 3*4 {
		t.Fatalf("tensor len %d", len(tensor))
	}
	for i := 0; i < 4; i++ {
		if diff(tensor[i], 1.0) > 0.02 {
			t.Fatalf("R plane[%d] = %v, want ~1", i, tensor[i])
		}
		if tensor[4+i] > 0.02 || tensor[8+i] > 0.02 {
			t.Fatalf("G/B plane[%d] not near zero: %v %v", i, tensor[4+i], tensor[8+i])
		}
	}
	if lb.gain != 1 || lb.padX != 0 || lb.padY != 0 {
		t.Fatalf("unexpected letterbox %+v", lb)
	}
}

// TestPreprocessLetterboxGeometry checks scale/pad math and that the padding
// area carries the YOLO gray value, not black.
func TestPreprocessLetterboxGeometry(t *testing.T) {
	src := solidJPEG(t, 100, 50, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	tensor, lb, err := preprocess(src, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	// gain = min(200/100, 200/50) = 2 -> image fills width, pads vertically.
	if lb.gain != 2 {
		t.Fatalf("gain %v", lb.gain)
	}
	if lb.padY != 50 || lb.padX != 0 {
		t.Fatalf("pad %+v", lb)
	}
	// Corner pixel is padding: 114/255 on every channel.
	const want = 114.0 / 255
	corner := tensor[0]
	if diff(corner, want) > 0.01 {
		t.Fatalf("padding R = %v, want ~%v", corner, want)
	}
	if diff(tensor[200*200], want) > 0.01 {
		t.Fatalf("padding G = %v", tensor[200*200])
	}
}

// TestPreprocessRejectsGarbage keeps decode failures loud.
func TestPreprocessRejectsGarbage(t *testing.T) {
	if _, _, err := preprocess([]byte("not a jpeg"), 64, 64); err == nil {
		t.Fatal("expected decode error")
	}
}

// TestLetterboxToSource verifies detections map back to source coordinates.
func TestLetterboxToSource(t *testing.T) {
	lb := letterbox{gain: 0.5, padX: 10, padY: 20, inW: 640, inH: 640}
	x, y := lb.toSource(110, 120)
	if x != 200 || y != 200 {
		t.Fatalf("toSource = (%v,%v), want (200,200)", x, y)
	}
}

func diff(a, b float32) float32 {
	if a > b {
		return a - b
	}
	return b - a
}
