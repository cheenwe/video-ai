package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// maxBufferBytes caps the rolling encoded buffer per camera.
const maxBufferBytes = 48 << 20

// ------------------------------------------------------------- decoder -----

// ffmpegDecoder pipes the elementary stream into ffmpeg and reads back JPEG
// frames. ffmpeg is used because a native decoder would require CGO plus
// platform-specific toolchains.
type ffmpegDecoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex
	closed bool
}

// startFFmpegDecoder launches one decoder process. The elementary stream must
// be written to Write; JPEG frames come back on Frames.
func startFFmpegDecoder(ctx context.Context, codec string) (*ffmpegDecoder, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("未找到 ffmpeg，无法解码视频帧")
	}
	demuxer := "h264"
	if codec == "h265" {
		demuxer = "hevc"
	}
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", demuxer, "-r", "25", "-i", "pipe:0",
		"-an",
		// Never upscale: mjpeg refuses to open for some tiny sources, and
		// blowing up a low-res sub-stream gains nothing.
		"-vf", "fps=2,scale='min(960,iw)':-2",
		"-threads", "1", "-pix_fmt", "yuvj420p",
		"-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "3",
		"pipe:1",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &ffmpegDecoder{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

// Write feeds encoded access units to the decoder.
func (d *ffmpegDecoder) Write(p []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("decoder closed")
	}
	_, err := d.stdin.Write(p)
	return err
}

// Frames exposes the JPEG stream produced by ffmpeg.
func (d *ffmpegDecoder) Frames() io.Reader { return d.stdout }

func (d *ffmpegDecoder) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	_ = d.stdin.Close()
	return d.cmd.Wait()
}

// --------------------------------------------------------- jpeg scanner ----

// jpegScanner splits a concatenated JPEG stream into individual frames.
//
// It keeps a read offset instead of re-slicing the accumulation buffer on every
// emitted frame: at 2 FPS a 16-camera host pushes ~2 MB/s through here, and
// reallocating the tail after each frame turned that into quadratic copying.
type jpegScanner struct {
	buf     []byte
	off     int // consumed prefix of buf
	frame   []byte
	inFrame bool
}

var (
	soi = []byte{0xFF, 0xD8}
	eoi = []byte{0xFF, 0xD9}
)

// compact drops the consumed prefix once it is worth the copy.
func (s *jpegScanner) compact() {
	if s.off == 0 {
		return
	}
	if s.off >= len(s.buf) {
		s.buf = s.buf[:0]
	} else if s.off >= 4096 {
		copy(s.buf, s.buf[s.off:])
		s.buf = s.buf[:len(s.buf)-s.off]
	} else {
		return
	}
	s.off = 0
}

func (s *jpegScanner) feed(b []byte, emit func([]byte)) {
	s.buf = append(s.buf, b...)
	for {
		if !s.inFrame {
			i := bytes.Index(s.buf[s.off:], soi)
			if i < 0 {
				// Keep a possible partial start marker (a trailing 0xFF)
				// instead of dropping it, or frames split across reads
				// would never be recognised.
				keep := len(soi) - 1
				if len(s.buf)-s.off > keep {
					tail := s.buf[len(s.buf)-keep:]
					copy(s.buf, tail)
					s.buf = s.buf[:keep]
					s.off = 0
				} else {
					s.compact()
				}
				return
			}
			s.off += i + len(soi)
			s.inFrame = true
			s.frame = append(s.frame[:0], soi...)
			continue
		}
		i := bytes.Index(s.buf[s.off:], eoi)
		if i < 0 {
			s.compact()
			return
		}
		end := s.off + i + len(eoi)
		s.frame = append(s.frame, s.buf[s.off:end]...)
		s.off = end
		s.inFrame = false
		emit(s.frame)
		s.frame = nil
	}
}

// jpegSize reads SOF dimensions without decoding the whole image.
func jpegSize(b []byte) (int, int) {
	for i := 2; i+9 <= len(b); {
		if b[i] != 0xFF {
			i++
			continue
		}
		marker := b[i+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		segLen := int(b[i+2])<<8 | int(b[i+3])
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			h := int(b[i+5])<<8 | int(b[i+6])
			w := int(b[i+7])<<8 | int(b[i+8])
			return w, h
		}
		i += 2 + segLen
	}
	return 0, 0
}

// ------------------------------------------------------------- buffer ------

// openBuffer creates the rolling elementary-stream file for a camera. The
// buffer starts at a keyframe so ffmpeg can decode it from byte zero.
func (s *stream) openBuffer() error {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if s.bufCur != nil {
		return nil
	}
	if err := os.MkdirAll(s.bufDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.bufDir, "buffer.h264")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	s.bufCur = f
	s.bufSize = 0
	return nil
}

// writeBuffer appends one access unit, rotating at keyframes once the buffer
// exceeds its cap. The previous generation is kept so a clip can span it.
func (s *stream) writeBuffer(au []byte, keyframe bool) {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if s.bufCur == nil {
		return
	}
	if keyframe && s.bufSize > maxBufferBytes {
		_ = s.bufCur.Close()
		if s.bufPrev != "" {
			_ = os.Remove(s.bufPrev)
		}
		s.bufPrev = filepath.Join(s.bufDir, "buffer.h264")
		newPath := filepath.Join(s.bufDir, "buffer_next.h264")
		_ = os.Rename(s.bufPrev, newPath)
		f, err := os.OpenFile(newPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			s.bufCur = nil
			return
		}
		s.bufCur = f
		s.bufSize = 0
	}
	if _, err := s.bufCur.Write(au); err != nil {
		return
	}
	s.bufSize += int64(len(au))
}

func (s *stream) closeBuffer() {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if s.bufCur != nil {
		_ = s.bufCur.Close()
		s.bufCur = nil
	}
}

// freezeBuffer snapshots the rolling buffer into a stable temp file so a clip
// can be produced while ingest keeps writing. Returns the path and cleanup.
func (s *stream) freezeBuffer() (string, func(), error) {
	s.bufMu.Lock()
	cur := filepath.Join(s.bufDir, "buffer.h264")
	prev := s.bufPrev
	if s.bufCur != nil {
		_ = s.bufCur.Sync()
	}
	s.bufMu.Unlock()

	tmp, err := os.CreateTemp(s.bufDir, "clip-*.h264")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}
	if prev != "" {
		f, err := os.Open(prev)
		if err == nil {
			_, _ = io.Copy(tmp, f)
			_ = f.Close()
		}
	}
	f, err := os.Open(cur)
	if err != nil {
		cleanup()
		return "", func() {}, errors.New("尚无可用取流缓冲")
	}
	if _, err := io.Copy(tmp, f); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, err
	}
	_ = f.Close()
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmp.Name(), cleanup, nil
}

// muxToMP4 converts a raw elementary stream to MP4 by remuxing (no re-encode).
func muxToMP4(ctx context.Context, src string, seconds int, dest string) error {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("未找到 ffmpeg，无法生成视频片段")
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if seconds > 0 {
		args = append(args, "-sseof", fmt.Sprintf("-%d", seconds))
	}
	args = append(args, "-f", "h264", "-r", "25", "-i", src,
		"-c", "copy", "-movflags", "+faststart", dest)
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("ffmpeg 生成片段失败: %v %s", err, msg)
	}
	return nil
}

// ------------------------------------------------------------ annex-B ------

// toAnnexB converts an access unit into Annex-B bytestream form.
func toAnnexB(au [][]byte) []byte {
	size := 0
	for _, n := range au {
		size += 4 + len(n)
	}
	out := make([]byte, 0, size)
	for _, n := range au {
		out = append(out, 0x00, 0x00, 0x00, 0x01)
		out = append(out, n...)
	}
	return out
}

// isKeyframe reports whether an access unit can be decoded standalone.
func isKeyframe(au [][]byte, codec string) bool {
	for _, n := range au {
		if len(n) == 0 {
			continue
		}
		if codec == "h265" {
			if typ := (int(n[0]) >> 1) & 0x3F; typ == 19 || typ == 20 {
				return true
			}
			continue
		}
		if typ := int(n[0]) & 0x1F; typ == 5 {
			return true
		}
	}
	return false
}

// rtpDecoder turns RTP packets into access units for either codec.
type rtpDecoder interface {
	Decode(pkt *rtp.Packet) ([][]byte, error)
}

// frameSink records decoded frames; kept small for use by the stream.
type frameSink func(jpeg []byte, ts time.Time)

func drainFrames(r io.Reader, sink frameSink) {
	sc := &jpegScanner{}
	buf := make([]byte, 64*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			// feed copies into its own accumulation buffer, so the chunk can
			// be handed over directly: allocating and copying 64 KB per read
			// only to copy it again inside the scanner was pure overhead.
			sc.feed(buf[:n], func(j []byte) {
				sink(j, time.Now())
			})
		}
		if err != nil {
			return
		}
	}
}
