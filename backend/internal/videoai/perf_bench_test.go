package videoai

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// benchFrame builds a realistic 1280x720 JPEG frame so the benchmark measures
// the same work the runtime does per tick.
func benchFrame(tb testing.TB) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func BenchmarkPreprocess(b *testing.B) {
	frame := benchFrame(b)
	b.SetBytes(int64(len(frame)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Release the tensor exactly like onnxAdapter.Infer does, otherwise
		// the benchmark measures a pool that never recycles.
		t, _, err := preprocess(frame, 640, 640)
		if err != nil {
			b.Fatal(err)
		}
		putTensor(t)
	}
}

// BenchmarkPreprocessParallel reflects the real deployment: several cameras
// analyse concurrently, so allocator contention shows up here rather than in
// the serial benchmark.
func BenchmarkPreprocessParallel(b *testing.B) {
	frame := benchFrame(b)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			t, _, err := preprocess(frame, 640, 640)
			if err != nil {
				b.Fatal(err)
			}
			putTensor(t)
		}
	})
}

// benchOutput builds a YOLOv8-shaped output. Only every 300th anchor clears
// the confidence gate, which matches a real frame: a model that returns
// thousands of confident boxes is a broken model, and benchmarking that case
// would measure the result slice rather than the decode loop.
func benchOutput(nc, n int, channelFirst bool) rawTensor {
	cols := 4 + nc
	data := make([]float32, n*cols)
	score := func(i, c int) float32 {
		if i%300 == 0 {
			return 0.9
		}
		return 0.05
	}
	for i := 0; i < n; i++ {
		if channelFirst {
			data[i] = float32(i % 640)
			data[n+i] = float32(i % 480)
			data[2*n+i] = 40
			data[3*n+i] = 60
			for c := 0; c < nc; c++ {
				data[(4+c)*n+i] = score(i, c)
			}
		} else {
			base := i * cols
			data[base] = float32(i % 640)
			data[base+1] = float32(i % 480)
			data[base+2] = 40
			data[base+3] = 60
			for c := 0; c < nc; c++ {
				data[base+4+c] = score(i, c)
			}
		}
	}
	return rawTensor{Name: "output0", Shape: []int{1, cols, n}, Data: data}
}

func BenchmarkPostprocess(b *testing.B) {
	cfg := ModelConfig{Classes: []string{"grab_bucket"}, InputW: 640, InputH: 640, Confidence: 0.25, IoU: 0.45}
	det := benchOutput(1, 8400, true)
	lb := letterbox{gain: 0.5, padX: 0, padY: 80, inW: 640, inH: 640}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := postprocess([]rawTensor{det}, cfg, lb, 1280, 720); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPostprocessWorstCase keeps the pathological shape — nearly every
// anchor confident — to show the cost ceiling rather than the typical cost.
func BenchmarkPostprocessWorstCase(b *testing.B) {
	cfg := ModelConfig{Classes: []string{"grab_bucket"}, InputW: 640, InputH: 640, Confidence: 0.01, IoU: 0.45}
	det := benchOutput(1, 8400, true)
	lb := letterbox{gain: 0.5, padX: 0, padY: 80, inW: 640, inH: 640}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := postprocess([]rawTensor{det}, cfg, lb, 1280, 720); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNMS(b *testing.B) {
	boxes := make([]box, 0, 500)
	for i := 0; i < 500; i++ {
		x := float64(i % 40 * 16)
		y := float64(i / 40 * 16)
		boxes = append(boxes, box{x1: x, y1: y, x2: x + 60, y2: y + 80, score: float64(i%50) / 50})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make([]box, len(boxes))
		copy(in, boxes)
		_ = nms(in, 0.45)
	}
}
