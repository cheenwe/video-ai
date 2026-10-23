package videoai

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
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

// preprocess decodes a JPEG frame, letterboxes it into (tw, th) and returns
// the NCHW float32 tensor (RGB channels, /255) plus the transform used to map
// detections back to source coordinates.
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

	// YOLO-style gray padding (114) instead of black, matching training.
	canvas := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{114, 114, 114, 255}), image.Point{}, draw.Src)
	draw.BiLinear.Scale(canvas, image.Rect(offX, offY, offX+nw, offY+nh), src, b, draw.Over, nil)

	// NCHW float32, RGB, normalized to [0,1].
	tensor := make([]float32, 3*tw*th)
	plane := tw * th
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			r16, g16, b16, _ := canvas.At(x, y).RGBA()
			off := y*tw + x
			tensor[off] = float32(r16>>8) / 255
			tensor[plane+off] = float32(g16>>8) / 255
			tensor[2*plane+off] = float32(b16>>8) / 255
		}
	}
	return tensor, letterbox{gain: gain, padX: padX, padY: padY, inW: tw, inH: th}, nil
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
