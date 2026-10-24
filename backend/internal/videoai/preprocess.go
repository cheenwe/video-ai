package videoai

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/draw"
)

// errEmptyFrame guards against degenerate inputs.
var errEmptyFrame = errors.New("空帧")

// letterbox is the affine transform that fits a source frame into the network
// input: uniform scale plus symmetric padding. Detections are mapped back to
// source coordinates through it.
type letterbox struct {
	gain float64
	padX float64
	padY float64
	inW  int
	inH  int
}

// toSource maps a point in letterboxed input space back to source pixels.
func (lb letterbox) toSource(x, y float64) (float64, float64) {
	if lb.gain <= 0 {
		return x, y
	}
	return (x - lb.padX) / lb.gain, (y - lb.padY) / lb.gain
}

// ----------------------------------------------------------- scratch ----
//
// Analysis is a per-frame hot path: at 2 FPS a 16-camera deployment asks for
// 32 tensors of 3*640*640 float32 per second (≈236 MB/s). Allocating that
// fresh every tick makes the GC the bottleneck rather than the arithmetic, so
// the letterbox canvas and the tensor are recycled. Ownership is explicit:
// preprocess hands out a tensor that the caller MUST return with putTensor
// once the inference round-trip is done; nothing may retain it.

type scratchCanvas struct {
	img  *image.RGBA
	w, h int
}

var canvasPool = sync.Pool{New: func() any { return &scratchCanvas{} }}

// getCanvas returns a letterbox canvas of exactly (w,h), cleared to the YOLO
// gray padding value.
func getCanvas(w, h int) *image.RGBA {
	sc := canvasPool.Get().(*scratchCanvas)
	if sc.img == nil || sc.w != w || sc.h != h {
		sc.img = image.NewRGBA(image.Rect(0, 0, w, h))
		sc.w, sc.h = w, h
	}
	// Gray 114 padding, matching YOLO training preprocessing.
	pix := sc.img.Pix
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = 114, 114, 114, 255
	}
	return sc.img
}

func putCanvas(img *image.RGBA) {
	if img == nil {
		return
	}
	canvasPool.Put(&scratchCanvas{img: img, w: img.Bounds().Dx(), h: img.Bounds().Dy()})
}

// ------------------------------------------------------- scaler cache ----
//
// draw.BiLinear.Scale rebuilds its contribution table and allocates a
// dw*sh temporary buffer on EVERY call — 14.7 MB of churn per 1280x720 frame.
// NewScaler returns a Scaler that pools that buffer and computes the table
// once, so it is cached per (source,target) geometry: cameras run at a handful
// of resolutions, so the cache stays tiny and the hot path stops allocating.

type scalerKey struct{ dw, dh, sw, sh int }

const maxCachedScalers = 64

var (
	scalerMu    sync.Mutex
	scalerCache = map[scalerKey]draw.Scaler{}
)

// resizeQuality selects the interpolator used to letterbox a frame.
//
// Measured on a 1280x720 frame scaled into 640x640 (Apple M1 Pro):
//
//	approx   (ApproxBiLinear)   4.5 ms   24 KB   mean |diff| 0.68/255
//	bilinear (Kernel tent)     14.4 ms   16 MB   reference
//
// Frames are downscaled, where ApproxBiLinear blends the four nearest source
// pixels instead of point-sampling them — closer to area averaging and less
// prone to aliasing, at ~0.3% mean deviation from true bilinear. Set
// BASE_VIDEO_AI_RESIZE=bilinear for the exact tent kernel when a model is
// known to be sensitive to resampling.
var resizeQuality = func() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASE_VIDEO_AI_RESIZE"))) {
	case "bilinear", "exact":
		return "bilinear"
	default:
		return "approx"
	}
}()

// getScaler returns a reusable scaler for one geometry. Scalers are stateless
// during Scale (they only read the precomputed distribution and use an
// internal sync.Pool), so they are safe to share across analysis loops.
func getScaler(dw, dh, sw, sh int) draw.Scaler {
	k := scalerKey{dw, dh, sw, sh}
	scalerMu.Lock()
	defer scalerMu.Unlock()
	if s, ok := scalerCache[k]; ok {
		return s
	}
	if len(scalerCache) >= maxCachedScalers {
		// Bounded: a misconfigured fleet of odd resolutions must not grow the
		// map forever. Dropping entries only costs one rebuild.
		for k := range scalerCache {
			delete(scalerCache, k)
			if len(scalerCache) <= maxCachedScalers/2 {
				break
			}
		}
	}
	var s draw.Scaler
	if resizeQuality == "bilinear" {
		// NewScaler (not Scale) so the 14 MB temporary buffer is pooled and
		// the contribution table is built once per geometry.
		s = draw.BiLinear.NewScaler(dw, dh, sw, sh)
	} else {
		// ApproxBiLinear has no kernel scaler and therefore no temporary
		// buffer to pool; it is already allocation-free.
		s = draw.ApproxBiLinear
	}
	scalerCache[k] = s
	return s
}

type tensorBuf struct {
	data []float32
}

var tensorPool = sync.Pool{New: func() any { return &tensorBuf{} }}

// getTensor returns a zero-length slice with at least n float32 of capacity.
func getTensor(n int) []float32 {
	b := tensorPool.Get().(*tensorBuf)
	if cap(b.data) < n {
		b.data = make([]float32, n)
	}
	return b.data[:n]
}

// putTensor returns a tensor obtained from getTensor. Passing a slice that did
// not come from getTensor corrupts the pool, so only call it on the exact
// slice preprocess returned.
func putTensor(t []float32) {
	if cap(t) == 0 {
		return
	}
	tensorPool.Put(&tensorBuf{data: t[:cap(t)]})
}

// --------------------------------------------------------- preprocess ----

// preprocess decodes a JPEG frame, letterboxes it into (tw, th) and returns
// the NCHW float32 tensor (RGB channels, /255) plus the transform used to map
// detections back to source coordinates.
//
// The returned tensor comes from a pool: the caller must hand it back with
// putTensor as soon as the inference call is finished.
func preprocess(jpegBytes []byte, tw, th int) ([]float32, letterbox, error) {
	if tw <= 0 {
		tw = 640
	}
	if th <= 0 {
		th = 640
	}
	src, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, letterbox{}, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, letterbox{}, errEmptyFrame
	}
	gain := math.Min(float64(tw)/float64(w), float64(th)/float64(h))
	nw := int(math.Round(float64(w) * gain))
	nh := int(math.Round(float64(h) * gain))
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	padX := float64(tw-nw) / 2
	padY := float64(th-nh) / 2
	offX := int(math.Round(padX))
	offY := int(math.Round(padY))

	canvas := getCanvas(tw, th)
	getScaler(nw, nh, w, h).Scale(canvas, image.Rect(offX, offY, offX+nw, offY+nh), src, b, draw.Over, nil)
	tensor := preprocessTo(canvas, tw, th)
	putCanvas(canvas)

	return tensor, letterbox{gain: gain, padX: padX, padY: padY, inW: tw, inH: th}, nil
}

// preprocessTo converts a letterboxed RGBA canvas into the NCHW float32
// tensor, reading the pixel buffer directly. Going through At()/RGBA() costs
// an interface call plus 8→16 bit expansion per pixel — for 640x640 that is
// ~400k avoidable calls per frame.
func preprocessTo(canvas *image.RGBA, tw, th int) []float32 {
	tensor := getTensor(3 * tw * th)
	plane := tw * th
	pix := canvas.Pix
	stride := canvas.Stride
	const inv255 = 1.0 / 255.0
	for y := 0; y < th; y++ {
		row := y * stride
		off := y * tw
		for x := 0; x < tw; x++ {
			i := row + x*4
			tensor[off+x] = float32(pix[i]) * inv255
			tensor[plane+off+x] = float32(pix[i+1]) * inv255
			tensor[2*plane+off+x] = float32(pix[i+2]) * inv255
		}
	}
	return tensor
}

var (
	syntheticOnce sync.Once
	syntheticJPEG []byte
)

// syntheticFrameJPEG builds a deterministic gray frame used by onnx
// self-check sample inference. It is a neutral input: any runnable model
// must handle it without error.
func syntheticFrameJPEG() []byte {
	syntheticOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 640, 480))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{128, 128, 128, 255}), image.Point{}, draw.Src)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
			return
		}
		syntheticJPEG = append([]byte(nil), buf.Bytes()...)
	})
	return syntheticJPEG
}
