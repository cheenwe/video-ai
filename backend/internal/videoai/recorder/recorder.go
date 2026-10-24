// Package recorder implements platform-side continuous recording for cameras
// whose stream the platform can pull itself (direct RTSP). It shells out to
// ffmpeg in stream-copy mode, so recording costs no transcode, and writes
// fixed-length segments under the data directory.
//
// NVR-bound cameras are deliberately out of scope: their recording belongs to
// the NVR agent, and duplicating that here would record through the agent's
// HLS output at a second quality.
package recorder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// segmentSeconds is how long one file covers. Short enough that a crash loses
// little, long enough that a 24h recording is not thousands of files.
const segmentSeconds = 300

// retryDelay is how long the supervisor waits before restarting a recorder
// whose ffmpeg exited (stream down, network blip).
const retryDelay = 10 * time.Second

// windowPollInterval is how often a paused (off-shift) recorder re-checks its
// schedule. Coarse on purpose: nothing is being recorded anyway.
const windowPollInterval = 20 * time.Second

// Segment is one recorded file on disk.
type Segment struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"` // 0 when it is the segment still being written
}

// State is the live status of one camera's recorder.
type State struct {
	CameraID  uint `json:"camera_id"`
	Recording bool `json:"recording"`
	// Paused means the plan's schedule window is currently closed.
	Paused    bool      `json:"paused"`
	Dir       string    `json:"dir"`
	StartedAt time.Time `json:"started_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	Segments  int       `json:"segments"`
	Bytes     int64     `json:"bytes"`
}

// Manager owns one ffmpeg segmenter per camera. It is safe for concurrent use.
type Manager struct {
	dir string

	mu      sync.Mutex
	workers map[uint]*worker
	logger  func(format string, args ...any)
}

type worker struct {
	cameraID uint
	url      string
	retain   int
	maxGB    int
	dir      string
	// schedule is nil for continuous mode (always on). When set, ffmpeg only
	// runs inside a window; outside one the process is stopped so an idle
	// camera costs nothing.
	schedule *Schedule

	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	startedAt time.Time
	lastError string
	stopped   bool
	done      chan struct{}
	// activeName is the segment ffmpeg is currently writing; pruning must
	// never delete it (the file would disappear mid-write).
	activeName string
	// paused is true while a schedule window is closed.
	paused bool
}

// NewManager creates a recorder rooted at dataHome/recordings.
func NewManager(dataHome string) *Manager {
	return &Manager{
		dir:     filepath.Join(dataHome, "recordings"),
		workers: map[uint]*worker{},
		logger:  func(string, ...any) {},
	}
}

// SetLogger routes supervisor messages to the caller's logger.
func (m *Manager) SetLogger(fn func(format string, args ...any)) {
	if fn != nil {
		m.logger = fn
	}
}

// Dir returns the per-camera recording directory.
func (m *Manager) Dir(cameraID uint) string {
	return filepath.Join(m.dir, strconv.FormatUint(uint64(cameraID), 10))
}

// Start begins recording a camera. Scheduling is passed as the raw plan JSON:
// empty means continuous, and a malformed schedule is an error rather than a
// silent switch to 24/7 recording. Calling Start again with the same
// URL/schedule is a no-op, so plan re-saves do not duplicate ffmpeg processes.
func (m *Manager) Start(cameraID uint, url string, retainDays, maxGB int, scheduleJSON string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("摄像头未配置码流地址，无法录像")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("未找到 ffmpeg，无法录像")
	}
	sched, err := ParseSchedule(scheduleJSON)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if w, ok := m.workers[cameraID]; ok && w.url == url && w.sameSchedule(sched) && w.running() {
		m.mu.Unlock()
		return nil
	}
	if old, ok := m.workers[cameraID]; ok {
		old.stop()
	}
	w := &worker{
		cameraID: cameraID, url: url, retain: retainDays, maxGB: maxGB,
		dir: m.Dir(cameraID), done: make(chan struct{}), schedule: sched,
	}
	m.workers[cameraID] = w
	m.mu.Unlock()

	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("创建录像目录失败: %w", err)
	}
	w.run(m.logger)
	return nil
}

// Stop ends recording for a camera and waits for ffmpeg to exit so the last
// segment is finalised (moov atom written) rather than left truncated.
func (m *Manager) Stop(cameraID uint) {
	m.mu.Lock()
	w := m.workers[cameraID]
	delete(m.workers, cameraID)
	m.mu.Unlock()
	if w != nil {
		w.stop()
	}
}

// StopAll stops every recorder; called on shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	all := make([]*worker, 0, len(m.workers))
	for id, w := range m.workers {
		all = append(all, w)
		delete(m.workers, id)
	}
	m.mu.Unlock()
	for _, w := range all {
		w.stop()
	}
}

// Status reports whether a camera is recording and what is on disk.
func (m *Manager) Status(cameraID uint) State {
	st := State{CameraID: cameraID, Dir: m.Dir(cameraID)}
	m.mu.Lock()
	w := m.workers[cameraID]
	m.mu.Unlock()
	if w != nil {
		w.mu.Lock()
		st.Recording = w.cmd != nil && !w.stopped
		st.Paused = w.paused
		st.StartedAt = w.startedAt
		st.LastError = w.lastError
		w.mu.Unlock()
	}
	segs, _ := m.List(cameraID)
	st.Segments = len(segs)
	for _, s := range segs {
		st.Bytes += s.SizeBytes
	}
	return st
}

// List returns the camera's segments, newest first.
func (m *Manager) List(cameraID uint) ([]Segment, error) {
	dir := m.Dir(cameraID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Segment{}, nil
		}
		return nil, err
	}
	segs := make([]Segment, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mp4") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() == 0 {
			continue
		}
		started, ok := parseSegmentName(e.Name())
		if !ok {
			started = info.ModTime()
		}
		segs = append(segs, Segment{
			Name: e.Name(), SizeBytes: info.Size(), StartedAt: started,
		})
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].StartedAt.After(segs[j].StartedAt) })
	// A segment runs until the next one starts, so a closed segment's duration
	// is the gap to its successor. The newest file is still being written and
	// therefore keeps 0 rather than a guess.
	for i := 0; i < len(segs)-1; i++ {
		gap := segs[i].StartedAt.Sub(segs[i+1].StartedAt)
		if gap > 0 && gap < time.Hour {
			segs[i+1].DurationMS = gap.Milliseconds()
		}
	}
	return segs, nil
}

// SegmentPath resolves a segment name to a path inside the camera directory,
// rejecting anything that tries to escape it.
func (m *Manager) SegmentPath(cameraID uint, name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if base != name || base == "." || base == ".." || !strings.HasSuffix(base, ".mp4") {
		return "", errors.New("非法的片段名")
	}
	path := filepath.Join(m.Dir(cameraID), base)
	if _, err := os.Stat(path); err != nil {
		return "", errors.New("片段不存在")
	}
	return path, nil
}

// Prune enforces the plan's retention: segments older than retainDays are
// removed, then the oldest are dropped until the directory fits maxGB.
// Returns how many files were deleted.
func (m *Manager) Prune(cameraID uint, retainDays, maxGB int) int {
	segs, err := m.List(cameraID)
	if err != nil || len(segs) == 0 {
		return 0
	}
	removed := 0
	cutoff := time.Time{}
	if retainDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -retainDays)
	}
	keep := make([]Segment, 0, len(segs))
	for _, s := range segs {
		if !cutoff.IsZero() && s.StartedAt.Before(cutoff) {
			// Never delete the segment currently being written.
			if m.isActive(cameraID, s.Name) {
				keep = append(keep, s)
				continue
			}
			if os.Remove(filepath.Join(m.Dir(cameraID), s.Name)) == nil {
				removed++
			}
			continue
		}
		keep = append(keep, s)
	}
	if maxGB > 0 {
		limit := int64(maxGB) * 1000 * 1000 * 1000
		var total int64
		for _, s := range keep {
			total += s.SizeBytes
		}
		// keep is newest-first; drop from the tail until it fits.
		for i := len(keep) - 1; i >= 0 && total > limit; i-- {
			if m.isActive(cameraID, keep[i].Name) {
				continue
			}
			if os.Remove(filepath.Join(m.Dir(cameraID), keep[i].Name)) == nil {
				total -= keep[i].SizeBytes
				removed++
			}
		}
	}
	return removed
}

// PruneAll applies every running worker's policy. Exposed for the periodic
// housekeeping loop.
func (m *Manager) PruneAll() {
	m.mu.Lock()
	type job struct {
		id     uint
		retain int
		maxGB  int
	}
	jobs := make([]job, 0, len(m.workers))
	for id, w := range m.workers {
		jobs = append(jobs, job{id: id, retain: w.retain, maxGB: w.maxGB})
	}
	m.mu.Unlock()
	for _, j := range jobs {
		m.Prune(j.id, j.retain, j.maxGB)
	}
}

// RunHousekeeping prunes on an interval until ctx is cancelled.
func (m *Manager) RunHousekeeping(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.PruneAll()
		}
	}
}

// isActive reports whether the named file is the one ffmpeg is writing.
func (m *Manager) isActive(cameraID uint, name string) bool {
	m.mu.Lock()
	w := m.workers[cameraID]
	m.mu.Unlock()
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cmd != nil && !w.stopped && w.activeName == name
}

func (w *worker) running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cmd != nil && !w.stopped
}

// run starts the segmenter and supervises it until stopped. A dead ffmpeg
// (stream down) is retried after retryDelay with the error kept in State so
// the UI shows why nothing is being recorded.
func (w *worker) run(logger func(string, ...any)) {
	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.cancel = cancel
	w.stopped = false
	w.startedAt = time.Now()
	w.mu.Unlock()

	go func() {
		defer close(w.done)
		for {
			err := w.loop(ctx)
			if ctx.Err() != nil {
				return
			}
			w.mu.Lock()
			switch {
			case errors.Is(err, errOutsideWindow):
				// Scheduled pause: keep last_error clean so the UI does not
				// show a fault for a camera that is simply off-shift.
				w.lastError = ""
				w.paused = true
			default:
				// loop() already recorded the detailed reason (exit status +
				// ffmpeg's stderr tail); overwriting it here with the bare
				// error string is what made failures undiagnosable.
				w.paused = false
			}
			stopped := w.stopped
			w.mu.Unlock()
			if stopped {
				return
			}
			if errors.Is(err, errOutsideWindow) {
				continue // loop already waited one poll interval
			}
			logger("[recorder] camera %d exited, retrying in %s: %v", w.cameraID, retryDelay, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
		}
	}()
}

// errOutsideWindow means the schedule is currently closed; it is a normal
// state, not a failure, so it must not be reported as last_error.
var errOutsideWindow = errors.New("当前不在录像时段内")

// sameSchedule compares the active window set, so a plan re-save that only
// changes retention does not restart ffmpeg.
func (w *worker) sameSchedule(other *Schedule) bool {
	a, b := w.schedule, other
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if len(a.Windows) != len(b.Windows) {
		return false
	}
	for i := range a.Windows {
		if a.Windows[i].From != b.Windows[i].From || a.Windows[i].To != b.Windows[i].To {
			return false
		}
		if len(a.Windows[i].Days) != len(b.Windows[i].Days) {
			return false
		}
		for j := range a.Windows[i].Days {
			if a.Windows[i].Days[j] != b.Windows[i].Days[j] {
				return false
			}
		}
	}
	return true
}

// loop runs one ffmpeg invocation to completion, or waits while the schedule
// is closed.
func (w *worker) loop(ctx context.Context) error {
	if w.schedule != nil && !w.schedule.Active(time.Now()) {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(windowPollInterval):
		}
		return errOutsideWindow
	}
	args := []string{"-hide_banner", "-loglevel", "error"}
	// The transport flag only exists for the RTSP demuxer; passing it to a
	// file or HTTP input makes ffmpeg exit with "Option not found".
	if strings.HasPrefix(strings.ToLower(w.url), "rtsp://") {
		args = append(args, "-rtsp_transport", "tcp")
	}
	args = append(args,
		"-i", w.url,
		"-c", "copy", "-an",
		"-f", "segment",
		"-segment_time", strconv.Itoa(segmentSeconds),
		"-segment_format", "mp4",
		"-reset_timestamps", "1",
		"-strftime", "1",
		filepath.Join(w.dir, "%Y%m%d-%H%M%S.mp4"),
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	// Keep the tail of ffmpeg's stderr: "exit status 195" alone tells an
	// operator nothing, while "Connection refused" points straight at the
	// stream. The buffer is bounded so a misbehaving ffmpeg cannot grow it.
	tail := &tailBuffer{limit: 2048}
	cmd.Stderr = tail
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return nil
	}
	w.cmd = cmd
	w.activeName = newestName(w.dir)
	w.mu.Unlock()

	err := cmd.Run()

	w.mu.Lock()
	w.cmd = nil
	w.activeName = ""
	if err == nil {
		w.lastError = ""
	} else if ctx.Err() == nil {
		w.lastError = "录像进程退出：" + err.Error() + tail.suffix()
	}
	w.mu.Unlock()
	return err
}

// tailBuffer keeps the last `limit` bytes written to it, so the supervisor can
// report why ffmpeg died without buffering an unbounded log.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.limit {
		t.buf = t.buf[len(t.buf)-t.limit:]
	}
	return len(p), nil
}

// suffix returns the last meaningful line, prefixed for inclusion in a status
// message, or "" when ffmpeg said nothing.
func (t *tailBuffer) suffix() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			if len(l) > 200 {
				l = l[:200]
			}
			return "：" + l
		}
	}
	return ""
}

func (w *worker) stop() {
	w.mu.Lock()
	w.stopped = true
	cancel := w.cancel
	cmd := w.cmd
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
	}
}

// newestName returns the most recently created segment name, used to know
// which file must not be pruned while ffmpeg writes it.
func newestName(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var name string
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mp4") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest, name = info.ModTime(), e.Name()
		}
	}
	return name
}

// parseSegmentName reads the strftime timestamp out of a segment filename.
func parseSegmentName(name string) (time.Time, bool) {
	base := strings.TrimSuffix(name, ".mp4")
	t, err := time.ParseInLocation("20060102-150405", base, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
