package videoai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/services"
	"baseplatform/internal/videoai/ingest"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// Manager runs in-process analysis loops for active deployments. The
// inferencer is resolved per deployment from its business template's model
// version; unknown/failed inference paths degrade to unknown results instead
// of fabricating detections. A tick without a fresh real frame is reported
// offline — analysis never runs on synthetic pixels.
type Manager struct {
	DB       *gorm.DB
	DataHome string
	Bus      *services.EventBus
	// Ingest pulls real video. Nil means the platform runs without frames.
	Ingest *ingest.Manager
	// frames is the frame source used by analysis loops. It defaults to
	// Ingest; tests replace it to inject deterministic frames.
	frames FrameSource
	// Hub broadcasts analysis updates to live subscribers. Nil disables push.
	Hub    *Hub
	logger *log.Logger
	parser cron.Parser

	mu      sync.Mutex
	cancels map[uint]context.CancelFunc
	hys     map[uint]*hysteresis
	wg      sync.WaitGroup

	// Per-tick lookups that only change on operator edits. See tickCache.
	cacheStream *tickCache // cameraID -> stream URL
	cacheModel  *tickCache // templateID -> *resolvedModel
	cacheGeom   *tickCache // boundaryVersionID -> *models.BoundaryVersion

	// openOutages tracks cameras with an unclosed media outage so a healthy
	// tick does not query the outage table at all.
	outageMu    sync.Mutex
	openOutages map[uint]bool

	// lastOverlay remembers what was persisted for each deployment so the
	// overlay is only rewritten when it actually changes.
	overlayMu   sync.Mutex
	lastOverlay map[uint]string
	// camStatus remembers the last persisted camera status.
	camStatus map[uint]string
	// appliedRev remembers the last persisted applied revision.
	appliedRev map[uint]int
}

// FrameSource is the ingest surface the analysis loop needs.
type FrameSource interface {
	Ensure(cameraID uint, url string) error
	Latest(cameraID uint) (ingest.Frame, bool)
}

// SetFrameSource replaces where analysis reads frames from. Production keeps
// the ingest manager; tests and dry-run harnesses inject deterministic frames
// through this seam instead of faking detections.
func (m *Manager) SetFrameSource(fs FrameSource) { m.frames = fs }

func NewManager(db *gorm.DB, dataHome string, bus *services.EventBus) *Manager {
	ing := ingest.NewManager(dataHome)
	return &Manager{
		DB:       db,
		DataHome: dataHome,
		Bus:      bus,
		Ingest:   ing,
		frames:   ing,
		Hub:      NewHub(),
		logger:   log.New(os.Stdout, "[videoai] ", log.LstdFlags|log.Lmsgprefix),
		parser:   cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		cancels:  map[uint]context.CancelFunc{},
		hys:      map[uint]*hysteresis{},
		// Stream URLs and published geometry move on operator edits (minutes
		// to days), while a model can be re-published mid-shift: a short TTL
		// bounds staleness even if a handler forgets to invalidate.
		cacheStream: newTickCache(30 * time.Second),
		cacheModel:  newTickCache(5 * time.Second),
		cacheGeom:   newTickCache(30 * time.Second),
		openOutages: map[uint]bool{},
		lastOverlay: map[uint]string{},
		camStatus:   map[uint]string{},
		appliedRev:  map[uint]int{},
	}
}

// freshFrameWindow is how old a frame may be before it stops counting as live.
const freshFrameWindow = 5 * time.Second

// cameraStreamURL returns the camera's main (or first) configured stream URL.
// The result is cached briefly: the URL only changes when an operator edits the
// camera, and this runs once per tick per camera.
func (m *Manager) cameraStreamURL(cameraID uint) string {
	if c := m.cacheStream; c != nil {
		if v, ok := c.get(cacheKeyStream(cameraID)); ok {
			if u, _ := v.(string); u != "" {
				return u
			}
		}
	}
	var streams []models.StreamProfile
	m.DB.Where("camera_id = ? AND enabled = 1", cameraID).Order("id asc").Find(&streams)
	url := ""
	for _, s := range streams {
		if s.Role == "main" && s.URL != "" {
			url = s.URL
			break
		}
	}
	if url == "" {
		for _, s := range streams {
			if s.URL != "" {
				url = s.URL
				break
			}
		}
	}
	// Only cache a usable URL: caching "" would keep a freshly configured
	// stream invisible for the whole TTL.
	if url != "" && m.cacheStream != nil {
		m.cacheStream.set(cacheKeyStream(cameraID), url)
	}
	return url
}

// FrameFor exposes the freshest decoded frame for a camera. The AI boundary
// candidate uses it because a candidate must be derived from real pixels — if
// there is no live frame the caller has to fail instead of guessing.
func (m *Manager) FrameFor(cameraID uint) (ingest.Frame, bool) {
	var cam models.Camera
	if err := m.DB.First(&cam, cameraID).Error; err != nil {
		return ingest.Frame{}, false
	}
	return m.frameFor(cam)
}

// frameFor returns the freshest decoded frame for a camera, if ingest is
// running for it and the frame is recent enough to be meaningful.
func (m *Manager) frameFor(cam models.Camera) (ingest.Frame, bool) {
	if m.frames == nil {
		return ingest.Frame{}, false
	}
	u := m.cameraStreamURL(cam.ID)
	if u == "" {
		return ingest.Frame{}, false
	}
	if err := m.frames.Ensure(cam.ID, u); err != nil {
		return ingest.Frame{}, false
	}
	f, ok := m.frames.Latest(cam.ID)
	if !ok || time.Since(f.TS) > freshFrameWindow {
		return ingest.Frame{}, false
	}
	return f, true
}

// boundaryFor loads a boundary version, caching it briefly. Published boundary
// versions are immutable, and the geometry JSON is parsed on every tick, so
// this removes one query plus one JSON parse per tick.
func (m *Manager) boundaryFor(id uint) (models.BoundaryVersion, error) {
	if c := m.cacheGeom; c != nil {
		if v, ok := c.get(cacheKeyGeom(id)); ok {
			if b, _ := v.(*models.BoundaryVersion); b != nil {
				return *b, nil
			}
		}
	}
	var bound models.BoundaryVersion
	err := m.DB.First(&bound, id).Error
	if err == nil && m.cacheGeom != nil {
		cp := bound
		m.cacheGeom.set(cacheKeyGeom(id), &cp)
	}
	return bound, err
}

// resolvedModel is the model version selected for a deployment, via its
// business template's ModelVersionID, together with the inference config.
type resolvedModel struct {
	VersionID uint
	Adapter   string
	Demo      bool
	Config    ModelConfig
}

// resolveModel loads the published model version bound to the deployment's
// template. Returns nil when nothing is bound — the caller then reports
// unknown instead of silently falling back to demo inference.
func (m *Manager) resolveModel(d models.Deployment) (*resolvedModel, error) {
	if d.TemplateID == nil {
		return nil, fmt.Errorf("布控未绑定业务模板")
	}
	// Only successful resolutions are cached: a transient failure (model file
	// moved, worker down) must be retried on the next tick, not remembered.
	if c := m.cacheModel; c != nil {
		if v, ok := c.get(cacheKeyModel(*d.TemplateID)); ok {
			if rm, _ := v.(*resolvedModel); rm != nil {
				return rm, nil
			}
		}
	}
	var tpl models.BusinessTemplate
	if err := m.DB.First(&tpl, *d.TemplateID).Error; err != nil {
		return nil, fmt.Errorf("业务模板不存在")
	}
	if tpl.ModelVersionID == nil {
		return nil, fmt.Errorf("业务模板 %s 未绑定模型版本", tpl.Code)
	}
	var mv models.AlgoModelVersion
	if err := m.DB.First(&mv, *tpl.ModelVersionID).Error; err != nil {
		return nil, fmt.Errorf("模型版本不存在")
	}
	if mv.Status != "published" {
		return nil, fmt.Errorf("模型版本 %s 未发布", mv.Version)
	}
	cfg := ConfigForVersion(mv)
	rm := &resolvedModel{VersionID: mv.ID, Adapter: mv.Adapter, Config: cfg}
	if mv.ClassesJSON != "" {
		_ = json.Unmarshal([]byte(mv.ClassesJSON), &cfg.Classes)
		rm.Config.Classes = cfg.Classes
	}
	if impl, err := AdapterByName(mv.Adapter); err != nil {
		return nil, err
	} else {
		rm.Demo = impl.Demo()
	}
	if c := m.cacheModel; c != nil {
		c.set(cacheKeyModel(*d.TemplateID), rm)
	}
	return rm, nil
}

// ConfigForVersion derives inference settings from a model version row.
// InputSchema may carry {"width":640,"height":640,"confidence":0.25,"iou":0.45};
// unset fields fall back to YOLO defaults.
func ConfigForVersion(mv models.AlgoModelVersion) ModelConfig {
	cfg := ModelConfig{
		VersionID:  mv.ID,
		Adapter:    mv.Adapter,
		ModelPath:  mv.FilePath,
		Backend:    mv.Backend,
		InputW:     640,
		InputH:     640,
		Confidence: 0.25,
		IoU:        0.45,
	}
	if mv.InputSchema != "" {
		var schema struct {
			Width      int     `json:"width"`
			Height     int     `json:"height"`
			Confidence float64 `json:"confidence"`
			IoU        float64 `json:"iou"`
		}
		if json.Unmarshal([]byte(mv.InputSchema), &schema) == nil {
			if schema.Width > 0 {
				cfg.InputW = schema.Width
			}
			if schema.Height > 0 {
				cfg.InputH = schema.Height
			}
			if schema.Confidence > 0 {
				cfg.Confidence = schema.Confidence
			}
			if schema.IoU > 0 {
				cfg.IoU = schema.IoU
			}
		}
	}
	return cfg
}

func (m *Manager) Start() {
	m.RestoreRunning()
	go m.watchLoop()
}

func (m *Manager) Stop() {
	m.mu.Lock()
	for id, cancel := range m.cancels {
		cancel()
		delete(m.cancels, id)
	}
	m.mu.Unlock()
	m.wg.Wait()
	CloseAdapters()
	if m.Ingest != nil {
		m.Ingest.StopAll()
	}
}

func (m *Manager) RestoreRunning() {
	var list []models.Deployment
	m.DB.Where("status IN ?", []string{"running", "starting", "pending"}).Find(&list)
	for _, d := range list {
		_ = m.StartDeployment(d.ID)
	}
}

func (m *Manager) watchLoop() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for range t.C {
		var list []models.Deployment
		m.DB.Where("status IN ?", []string{"running", "starting", "pending", "out_of_schedule"}).Find(&list)
		for _, d := range list {
			m.mu.Lock()
			_, ok := m.cancels[d.ID]
			m.mu.Unlock()
			if !ok && (d.Status == "running" || d.Status == "starting" || d.Status == "pending") {
				_ = m.StartDeployment(d.ID)
			}
		}
	}
}

func (m *Manager) StartDeployment(id uint) error {
	var d models.Deployment
	if err := m.DB.First(&d, id).Error; err != nil {
		return err
	}
	m.mu.Lock()
	if cancel, ok := m.cancels[id]; ok {
		cancel()
		delete(m.cancels, id)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[id] = cancel
	m.mu.Unlock()

	now := time.Now()
	_ = m.DB.Model(&d).Updates(map[string]any{
		"status":     "running",
		"started_at": now,
		"last_error": "",
	}).Error
	m.ensureRuntime(d)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runLoop(ctx, id)
	}()
	return nil
}

func (m *Manager) StopDeployment(id uint, status string) {
	if status == "" {
		status = "stopped"
	}
	m.mu.Lock()
	if cancel, ok := m.cancels[id]; ok {
		cancel()
		delete(m.cancels, id)
	}
	delete(m.hys, id)
	m.mu.Unlock()
	now := time.Now()
	_ = m.DB.Model(&models.Deployment{}).Where("id = ?", id).Updates(map[string]any{
		"status":     status,
		"stopped_at": now,
	}).Error
	_ = m.DB.Model(&models.RuntimeStatus{}).Where("deployment_id = ?", id).Updates(map[string]any{
		"state": status,
	}).Error
	var d models.Deployment
	if m.DB.Select("camera_id").First(&d, id).Error == nil {
		_ = m.DB.Model(&models.Camera{}).Where("id = ?", d.CameraID).Update("analysis_status", "idle").Error
		// Stop pulling this camera's stream unless another deployment needs it.
		if m.Ingest != nil && !m.cameraHasRunningDeployment(d.CameraID) {
			m.Ingest.Stop(d.CameraID)
		}
	}
}

// cameraHasRunningDeployment reports whether any deployment still needs frames.
func (m *Manager) cameraHasRunningDeployment(cameraID uint) bool {
	var n int64
	m.DB.Model(&models.Deployment{}).
		Where("camera_id = ? AND status IN ?", cameraID, []string{"running", "starting", "pending"}).
		Count(&n)
	return n > 0
}

func (m *Manager) PauseDeployment(id uint) {
	m.StopDeployment(id, "paused")
}

func (m *Manager) ensureRuntime(d models.Deployment) {
	var rt models.RuntimeStatus
	err := m.DB.Where("deployment_id = ?", d.ID).First(&rt).Error
	if err == gorm.ErrRecordNotFound {
		rt = models.RuntimeStatus{
			DeploymentID: d.ID,
			CameraID:     d.CameraID,
			State:        "running",
			UpdatedAt:    time.Now(),
		}
		_ = m.DB.Create(&rt).Error
		return
	}
	_ = m.DB.Model(&rt).Updates(map[string]any{
		"state":      "running",
		"updated_at": time.Now(),
	}).Error
}

// analysisInterval derives the tick period from AnalysisFPS (frames per
// second). Defaults apply for non-positive values; the interval is clamped to
// [100ms, 10s] so a misconfigured deployment cannot spin or stall.
func analysisInterval(fps float64) time.Duration {
	if fps <= 0 {
		fps = 1
	}
	d := time.Duration(float64(time.Second) / fps)
	if d < 100*time.Millisecond {
		d = 100 * time.Millisecond
	}
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

func (m *Manager) runLoop(ctx context.Context, id uint) {
	var d models.Deployment
	if err := m.DB.First(&d, id).Error; err != nil {
		return
	}
	ticker := time.NewTicker(analysisInterval(d.AnalysisFPS))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := m.tick(id, now); err != nil {
				m.logger.Printf("deployment %d tick: %v", id, err)
			}
		}
	}
}

func (m *Manager) tick(id uint, now time.Time) error {
	var d models.Deployment
	if err := m.DB.First(&d, id).Error; err != nil {
		return err
	}
	if d.Status == "paused" || d.Status == "stopped" || d.Status == "draft" {
		return nil
	}
	inWindow, valid := m.inSchedule(d.ScheduleCron, now)
	if !valid {
		// Invalid schedule_cron is a configuration fault: never analyse and
		// never silently fall back to always-on (self-check flags it too).
		m.bumpRuntime(d, ResultUnknown, 0, "schedule_cron 无效："+strings.TrimSpace(d.ScheduleCron), now, nil, -1, nil)
		return nil
	}
	if !inWindow {
		if d.Status != "out_of_schedule" {
			_ = m.DB.Model(&d).Update("status", "out_of_schedule").Error
			d.Status = "out_of_schedule"
		}
		m.bumpRuntime(d, ResultUnknown, 0, "计划外时段，不分析", now, nil, -1, nil)
		return nil
	}
	if d.Status == "out_of_schedule" {
		_ = m.DB.Model(&d).Update("status", "running").Error
		d.Status = "running"
	}
	var cam models.Camera
	if err := m.DB.First(&cam, d.CameraID).Error; err != nil {
		return err
	}
	if cam.Enabled == 0 || cam.Archived == 1 {
		m.noteMediaDown(cam, "摄像头已停用或已归档", now)
		m.bumpRuntime(d, ResultOffline, 0, "摄像头已停用", now, nil, -1, nil)
		return nil
	}

	// Real video only: ingest is started when the camera has a stream, and a
	// tick without a fresh frame is reported offline. Analysis never runs on
	// synthetic pixels, so there is no demo fallback path.
	frame, hasFrame := m.frameFor(cam)
	if !hasFrame {
		m.noteMediaDown(cam, "无可用视频帧（未配置码流或取流未连通）", now)
		m.bumpRuntime(d, ResultOffline, 0, "等待视频帧", now, nil, -1, nil)
		_ = m.DB.Model(&cam).Update("analysis_status", "waiting_video").Error
		return nil
	}
	frameAgeMS := time.Since(frame.TS).Milliseconds()
	frameW, frameH := frame.W, frame.H
	m.noteMediaOK(cam, now)

	var bound models.BoundaryVersion
	mediaOK := true
	geom := Geometry{}
	if d.BoundaryVersionID != nil {
		loaded, err := m.boundaryFor(*d.BoundaryVersionID)
		if err == nil {
			bound = loaded
		}
		if err != nil || bound.Status != "published" {
			mediaOK = false
			m.bumpRuntime(d, ResultUnknown, 0, "边界未发布或无效", now, nil, frameAgeMS, nil)
			_ = m.setCameraStatus(cam, "fault")
			return nil
		}
		g, gerr := ParseGeometry(bound.GeometryJSON)
		if gerr != nil {
			m.bumpRuntime(d, ResultUnknown, 0, gerr.Error(), now, nil, frameAgeMS, nil)
			return nil
		}
		geom = g
	} else {
		m.bumpRuntime(d, ResultUnknown, 0, "未绑定边界版本", now, nil, frameAgeMS, nil)
		return nil
	}

	rm, err := m.resolveModel(d)
	if err != nil {
		m.bumpRuntime(d, ResultUnknown, 0, err.Error(), now, nil, frameAgeMS, nil)
		_ = m.DB.Model(&cam).Update("analysis_status", "fault").Error
		return nil
	}
	impl, err := AdapterByName(rm.Adapter)
	if err != nil {
		m.bumpRuntime(d, ResultUnknown, 0, err.Error(), now, nil, frameAgeMS, nil)
		return nil
	}

	fw, fh := float64(bound.RefWidth), float64(bound.RefHeight)
	if fw <= 0 {
		fw = 1280
	}
	if fh <= 0 {
		fh = 720
	}
	dets, err := impl.Infer(cam.ID, Frame{TS: now, JPEG: frame.JPEG, W: frame.W, H: frame.H}, rm.Config)
	if err != nil {
		m.bumpRuntime(d, ResultUnknown, 0, err.Error(), now, rm, frameAgeMS, nil)
		_ = m.DB.Model(&cam).Update("analysis_status", "fault").Error
		return nil
	}
	out := EvaluateGrabRules(RuleInput{
		Detections:     dets,
		Geometry:       geom,
		ThresholdNear:  d.ThresholdNear,
		ThresholdCross: d.ThresholdCross,
		FrameW:         fw,
		FrameH:         fh,
		MediaOK:        mediaOK,
	})
	out.Overlay["adapter"] = rm.Adapter
	out.Overlay["model_version_id"] = rm.VersionID
	// The boundary is stored in its calibration frame size; publishing it lets
	// the live view scale the overlay correctly instead of guessing.
	out.Overlay["ref_width"] = fw
	out.Overlay["ref_height"] = fh
	out.Overlay["frame_age_ms"] = frameAgeMS
	out.Overlay["frame_source"] = "rtsp_ingest"
	if frameW > 0 {
		out.Overlay["frame_width"] = frameW
		out.Overlay["frame_height"] = frameH
		if bound.RefWidth > 0 && frameW > 0 && absDiff(frameW, bound.RefWidth) > bound.RefWidth/10 {
			out.Overlay["frame_size_mismatch"] = true
		}
	}
	if rm.Demo {
		out.Overlay["demo"] = true
	}

	// Hysteresis state lives with the loop goroutine; it is recreated on
	// restart, while active events persist and merge by event_key.
	hys := m.hysteresisFor(id, d)
	dec := hys.Update(out.Result, out.RuleCode, now)
	// Keep the geometry of this tick so the live view can draw it. Volatile
	// values (frame age) are pushed over the websocket instead of being
	// persisted, so the row is only rewritten when the stable part changes.
	m.persistOverlay(d.ID, out.Overlay)
	// One broadcast per tick: the analysis update already carries the result,
	// so bumpRuntime pushes it instead of emitting a second status frame.
	full := buildUpdate(d, cam, out, dec, now)
	m.bumpRuntime(d, out.Result, out.Distance, out.Reason, now, rm, frameAgeMS, &full)
	_ = m.setCameraStatus(cam, "running")
	if d.AppliedRevision != d.DesiredRevision {
		_ = m.DB.Model(&d).Update("applied_revision", d.DesiredRevision).Error
		m.noteAppliedRev(d.ID, d.DesiredRevision)
	}

	if dec.Escalated {
		_ = m.recoverActiveEventsFor(d.ID, dec.Rule, now)
	}
	if dec.Trigger {
		return m.mergeEvent(d, cam, bound, out, dec, rm, now)
	}
	if dec.Recover {
		return m.recoverActiveEvents(d.ID, now)
	}
	return nil
}

// volatileOverlayKeys are recomputed every tick and pushed to live viewers
// over the websocket; writing them to the database would defeat the change
// check in persistOverlay.
var volatileOverlayKeys = map[string]bool{"frame_age_ms": true}

// persistOverlay stores the stable part of the overlay, skipping the write when
// it is unchanged. The overlay carries the boundary geometry, which only
// changes when an operator publishes a new boundary — once per tick per
// deployment was pure write amplification.
func (m *Manager) persistOverlay(deploymentID uint, overlay map[string]any) {
	if len(overlay) == 0 {
		return
	}
	stable := make(map[string]any, len(overlay))
	for k, v := range overlay {
		if !volatileOverlayKeys[k] {
			stable[k] = v
		}
	}
	payload := mustJSON(stable)
	m.overlayMu.Lock()
	prev, seen := m.lastOverlay[deploymentID]
	same := seen && prev == payload
	if !same {
		m.lastOverlay[deploymentID] = payload
	}
	m.overlayMu.Unlock()
	if same {
		return
	}
	_ = m.DB.Model(&models.RuntimeStatus{}).Where("deployment_id = ?", deploymentID).
		Update("last_overlay_json", payload).Error
}

func (m *Manager) noteAppliedRev(deploymentID uint, rev int) {
	m.overlayMu.Lock()
	m.appliedRev[deploymentID] = rev
	m.overlayMu.Unlock()
}

// setCameraStatus writes the camera status only when it changed. A healthy
// camera runs this on every tick, which used to mean one UPDATE per tick per
// camera rewriting three unchanged columns.
func (m *Manager) setCameraStatus(cam models.Camera, status string) error {
	m.overlayMu.Lock()
	prev, seen := m.camStatus[cam.ID]
	same := seen && prev == status && cam.MediaStatus == "online" && cam.DeviceStatus == "online"
	if !same {
		m.camStatus[cam.ID] = status
	}
	m.overlayMu.Unlock()
	if same {
		return nil
	}
	return m.DB.Model(&cam).Updates(map[string]any{
		"analysis_status": status,
		"media_status":    "online",
		"device_status":   "online",
	}).Error
}

// buildUpdate assembles the tick's websocket frame so live viewers can draw it
// immediately. It is handed to bumpRuntime, which publishes it once.
func buildUpdate(d models.Deployment, cam models.Camera, out RuleOutput, dec hysteresisDecision, now time.Time) Update {
	result := out.Result
	rule := out.RuleCode
	if rule == "" {
		rule = dec.Rule
	}
	return Update{
		Type:         "analysis",
		DeploymentID: d.ID,
		CameraID:     cam.ID,
		CameraName:   cam.Name,
		Result:       result,
		Rule:         rule,
		Level:        levelFor(dec.Level),
		Distance:     out.Distance,
		Reason:       out.Reason,
		Overlay:      out.Overlay,
		TS:           now,
	}
}

// hysteresisFor returns the per-deployment hysteresis state, creating it on
// first use. Guarded by m.mu (cancels map shares the lock).

func (m *Manager) hysteresisFor(id uint, d models.Deployment) *hysteresis {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hys[id]
	if !ok {
		h = newHysteresis(d.TriggerHoldSec, d.RecoverHoldSec)
		m.hys[id] = h
	}
	return h
}

// recordStat increments the daily rollup for a deployment. Counters, not
// events, are the only way to express how often analysis was undecidable.
func (m *Manager) recordStat(d models.Deployment, result string, now time.Time) {
	day := now.Format("2006-01-02")
	updates := map[string]any{"ticks": gorm.Expr("ticks + 1")}
	switch result {
	case ResultNormal:
		updates["normal"] = gorm.Expr("normal + 1")
	case ResultNear:
		updates["near"] = gorm.Expr("near + 1")
	case ResultCross:
		updates["cross"] = gorm.Expr("cross + 1")
	case ResultOffline:
		updates["offline"] = gorm.Expr("offline + 1")
	default:
		updates["unknown"] = gorm.Expr("unknown + 1")
	}
	updates["updated_at"] = now
	res := m.DB.Model(&models.AnalysisStat{}).
		Where("day = ? AND camera_id = ? AND deployment_id = ?", day, d.CameraID, d.ID).
		Updates(updates)
	if res.Error == nil && res.RowsAffected == 0 {
		stat := models.AnalysisStat{Day: day, CameraID: d.CameraID, DeploymentID: d.ID}
		switch result {
		case ResultNormal:
			stat.Normal = 1
		case ResultNear:
			stat.Near = 1
		case ResultCross:
			stat.Cross = 1
		case ResultOffline:
			stat.Offline = 1
		default:
			stat.Unknown = 1
		}
		stat.Ticks = 1
		_ = m.DB.Create(&stat).Error
	}
}

// noteMediaDown opens a media outage for the camera when none is open, so
// offline duration can be summed later instead of guessed.
func (m *Manager) hasOpenOutage(cameraID uint) bool {
	m.outageMu.Lock()
	defer m.outageMu.Unlock()
	return m.openOutages[cameraID]
}

// noteMediaDown opens a media outage for the camera when none is open, so
// offline duration can be summed later instead of guessed. The open set is
// tracked in memory so a camera that stays offline does not query the outage
// table on every tick.
func (m *Manager) noteMediaDown(cam models.Camera, reason string, now time.Time) {
	m.outageMu.Lock()
	known, open := m.openOutages[cam.ID]
	m.outageMu.Unlock()
	if open {
		return
	}
	if !known {
		// First offline tick for this camera in this process: an outage left
		// open by a previous run must be reused, not duplicated, or reported
		// offline time would be double counted after every restart.
		var existing models.MediaOutage
		if err := m.DB.Where("camera_id = ? AND ended_at IS NULL", cam.ID).
			Order("id desc").First(&existing).Error; err == nil {
			m.outageMu.Lock()
			m.openOutages[cam.ID] = true
			m.outageMu.Unlock()
			if existing.Reason != reason {
				_ = m.DB.Model(&existing).Update("reason", reason).Error
			}
			return
		}
	}
	m.outageMu.Lock()
	m.openOutages[cam.ID] = true
	m.outageMu.Unlock()
	_ = m.DB.Create(&models.MediaOutage{
		CameraID:  cam.ID,
		ProjectID: cam.ProjectID,
		StartedAt: now,
		Reason:    reason,
	}).Error
}

// noteMediaOK closes the camera's open outage, recording its real duration.
func (m *Manager) noteMediaOK(cam models.Camera, now time.Time) {
	m.outageMu.Lock()
	if !m.openOutages[cam.ID] {
		m.outageMu.Unlock()
		return
	}
	delete(m.openOutages, cam.ID)
	m.outageMu.Unlock()

	var open models.MediaOutage
	if err := m.DB.Where("camera_id = ? AND ended_at IS NULL", cam.ID).Order("id desc").First(&open).Error; err != nil {
		return
	}
	secs := int64(now.Sub(open.StartedAt).Seconds())
	_ = m.DB.Model(&open).Updates(map[string]any{
		"ended_at":     now,
		"duration_sec": secs,
		"updated_at":   now,
	}).Error
}

// bumpRuntime writes the runtime row and broadcasts the outcome. push carries
// the full analysis frame when the tick reached inference; other paths pass
// nil and get a status-only frame, so live viewers still see offline / fault /
// out-of-schedule transitions.
func (m *Manager) bumpRuntime(d models.Deployment, result string, dist float64, reason string, now time.Time, rm *resolvedModel, frameAgeMS int64, push *Update) {
	m.recordStat(d, result, now)
	hb := now
	backend := ""
	modelLoaded := 0
	if rm != nil {
		backend = rm.Adapter
		modelLoaded = 1
	}
	if frameAgeMS < 0 {
		// No real frame: report a sentinel instead of a fabricated 200ms.
		frameAgeMS = -1
	}
	_ = m.DB.Model(&models.RuntimeStatus{}).Where("deployment_id = ?", d.ID).Updates(map[string]any{
		"state":        d.Status,
		"applied_rev":  d.DesiredRevision,
		"actual_fps":   d.AnalysisFPS,
		"frame_age_ms": frameAgeMS,
		"last_result":  result,
		"last_error":   reason,
		"updated_at":   now,
		"heartbeat_at": &hb,
		"backend":      backend,
		"model_loaded": modelLoaded,
	}).Error
	// Every state transition reaches live viewers, including the paths that
	// never run inference (offline / waiting_video / out_of_schedule / fault).
	// The overlay of the previous tick is preserved so panels keep drawing it.
	if m.Hub != nil {
		if push != nil {
			m.Hub.Publish(*push)
		} else {
			m.Hub.PublishStatus(Update{
				Type:         "analysis",
				DeploymentID: d.ID,
				CameraID:     d.CameraID,
				Result:       result,
				Reason:       reason,
				TS:           now,
			})
		}
	}
	_ = dist
}

func (m *Manager) mergeEvent(d models.Deployment, cam models.Camera, bound models.BoundaryVersion, out RuleOutput, dec hysteresisDecision, rm *resolvedModel, now time.Time) error {
	key := fmt.Sprintf("cam%d-%s-active", cam.ID, dec.Rule)
	metric, _ := json.Marshal(out.Metrics)
	overlay, _ := json.Marshal(out.Overlay)
	snap, _ := json.Marshal(map[string]any{
		"deployment_id":       d.ID,
		"desired_revision":    d.DesiredRevision,
		"boundary_version_id": bound.ID,
		"boundary_version":    bound.Version,
		"template_id":         d.TemplateID,
		"model_version_id":    rm.VersionID,
		"adapter":             rm.Adapter,
		"demo":                rm.Demo,
		"rule_code":           dec.Rule,
		"trigger_hold_sec":    d.TriggerHoldSec,
	})

	var ev models.VideoEvent
	err := m.DB.Where("event_key = ? AND scene_status = ?", key, "active").First(&ev).Error
	if err == gorm.ErrRecordNotFound {
		ev = models.VideoEvent{
			EventKey:           key,
			CameraID:           cam.ID,
			ProjectID:          d.ProjectID,
			DeploymentID:       &d.ID,
			Category:           dec.Rule,
			Level:              levelFor(dec.Level),
			SceneStatus:        "active",
			HandleStatus:       "pending",
			StartedAt:          now,
			LastEvidenceAt:     now,
			TriggerMetricJSON:  string(metric),
			ConfigSnapshotJSON: string(snap),
			OverlayJSON:        string(overlay),
		}
		if err := m.DB.Create(&ev).Error; err != nil {
			return err
		}
		m.createEvidence(ev, out, rm)
		m.scheduleClip(ev)
		if m.Hub != nil {
			m.Hub.Publish(Update{
				Type: "event", DeploymentID: d.ID, CameraID: cam.ID, CameraName: cam.Name,
				Result: out.Result, Rule: dec.Rule, Level: ev.Level,
				Distance: out.Distance, Overlay: out.Overlay, EventID: ev.ID, TS: now,
			})
		}
		if d.MuteNotify == 0 {
			m.notify(ev, cam, "新的视频分析事件")
		}
		return nil
	}
	_ = m.DB.Model(&ev).Updates(map[string]any{
		"last_evidence_at":     now,
		"level":                levelFor(dec.Level),
		"category":             dec.Rule,
		"trigger_metric_json":  string(metric),
		"overlay_json":         string(overlay),
		"config_snapshot_json": string(snap),
	}).Error
	return nil
}

func (m *Manager) recoverActiveEvents(deploymentID uint, now time.Time) error {
	return m.DB.Model(&models.VideoEvent{}).
		Where("deployment_id = ? AND scene_status = ?", deploymentID, "active").
		Updates(map[string]any{
			"scene_status": "recovered",
			"ended_at":     now,
		}).Error
}

// recoverActiveEventsFor closes only the previous rule's active events during
// an escalation (near → cross), so the new event key starts fresh.
func (m *Manager) recoverActiveEventsFor(deploymentID uint, keepRule string, now time.Time) error {
	return m.DB.Model(&models.VideoEvent{}).
		Where("deployment_id = ? AND scene_status = ? AND category <> ?", deploymentID, "active", keepRule).
		Updates(map[string]any{
			"scene_status": "recovered",
			"ended_at":     now,
		}).Error
}

// createEvidence writes the overlay and, when real frames are available, a
// screenshot. Missing frames are recorded as failed evidence rather than
// silently dropped, so operators can tell "no frame" from "no event".
func (m *Manager) createEvidence(ev models.VideoEvent, out RuleOutput, rm *resolvedModel) {
	dir := filepath.Join(m.DataHome, "evidence", fmt.Sprintf("%d", ev.ID))
	_ = os.MkdirAll(dir, 0o755)
	meta, _ := json.Marshal(map[string]any{
		"demo":   rm.Demo,
		"result": out.Result,
		"reason": out.Reason,
	})
	path := filepath.Join(dir, "overlay.json")
	_ = os.WriteFile(path, []byte(mustJSON(out.Overlay)), 0o644)
	_ = m.DB.Create(&models.Evidence{
		EventID:  ev.ID,
		Kind:     "overlay",
		Status:   "ready",
		Path:     path,
		MetaJSON: string(meta),
	}).Error

	shot := filepath.Join(dir, "snapshot.jpg")
	if err := m.snapshot(ev.CameraID, shot); err != nil {
		_ = m.DB.Create(&models.Evidence{
			EventID: ev.ID, Kind: "screenshot", Status: "failed", Error: err.Error(),
		}).Error
		return
	}
	_ = m.DB.Create(&models.Evidence{
		EventID: ev.ID, Kind: "screenshot", Status: "ready", Path: shot,
	})
}

// snapshot stores the latest decoded frame as JPEG evidence.
func (m *Manager) snapshot(cameraID uint, dest string) error {
	if m.Ingest == nil {
		return errors.New("未启用取流，无法截取画面")
	}
	return m.Ingest.Snapshot(cameraID, dest)
}

// scheduleClip produces an MP4 covering the moments around an event. It runs
// in the background because it must wait for the tail of the incident.
func (m *Manager) scheduleClip(ev models.VideoEvent) {
	if m.Ingest == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		dest := filepath.Join(m.DataHome, "evidence", fmt.Sprintf("%d", ev.ID), "clip.mp4")
		before, after := clipBeforeSec, clipAfterSec
		err := m.Ingest.Clip(ctx, ev.CameraID, before, after, dest)
		e := models.Evidence{EventID: ev.ID, Kind: "clip"}
		if err != nil {
			e.Status = "failed"
			e.Error = err.Error()
		} else {
			e.Status = "ready"
			e.Path = dest
			e.MetaJSON = mustJSON(map[string]any{"before_sec": before, "after_sec": after})
		}
		_ = m.DB.Create(&e).Error
	}()
}

// clipBeforeSec / clipAfterSec frame the incident window for event clips.
const (
	clipBeforeSec = 15
	clipAfterSec  = 10
)

func (m *Manager) notify(ev models.VideoEvent, cam models.Camera, title string) {
	if m.Bus == nil {
		return
	}
	level := "warning"
	if ev.Level == "critical" {
		level = "error"
	}
	_ = m.Bus.Emit("video.event.created", nil, map[string]interface{}{
		"title":     title,
		"content":   fmt.Sprintf("%s · 摄像头 %s (#%d) · %s", ev.Category, cam.Name, cam.ID, ev.Level),
		"level":     level,
		"camera_id": cam.ID,
		"event_id":  ev.ID,
	})
}

// ValidateScheduleCron rejects unparsable 5-field cron expressions. An empty
// expression means always-on and stays valid. Save paths and self-check share
// this so an invalid schedule fails loudly instead of silently analysing
// around the clock.
func ValidateScheduleCron(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	_, err := p.Parse(expr)
	return err
}

// inSchedule treats an empty cron as always-on. A non-empty cron uses standard
// 5-field expressions (e.g. "* 8-18 * * 1-5") matched against the current
// minute. The second return value reports whether the expression parsed: an
// invalid cron is a configuration error, so callers must surface it as a fault
// instead of silently degrading to always-on analysis.
func (m *Manager) inSchedule(expr string, now time.Time) (bool, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, true
	}
	sched, err := m.parser.Parse(expr)
	if err != nil {
		return false, false
	}
	prev := sched.Next(now.Add(-2 * time.Minute))
	return !prev.After(now) && now.Sub(prev) < 2*time.Minute, true
}

func levelFor(result string) string {
	if result == ResultCross {
		return "critical"
	}
	return "warning"
}

// absDiff returns |a-b| as an int.
func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// SelfCheck validates whether a deployment can start, including the model
// version bound via its business template.
func SelfCheck(db *gorm.DB, d models.Deployment) (ok bool, report map[string]any) {
	report = map[string]any{}
	checks := []map[string]any{}
	pass := true
	add := func(name string, good bool, detail string) {
		checks = append(checks, map[string]any{"name": name, "ok": good, "detail": detail})
		if !good {
			pass = false
		}
	}

	var cam models.Camera
	if err := db.First(&cam, d.CameraID).Error; err != nil || cam.Enabled == 0 || cam.Archived == 1 {
		add("camera", false, "摄像头不存在或已停用")
	} else {
		add("camera", true, cam.Name)
		if cam.AccessType == "nvr_agent" && (cam.NVRAgentID == nil || cam.AgentCameraID == "") {
			add("stream", false, "未绑定 NVR 通道")
		} else {
			add("stream", true, cam.AccessType)
		}
	}
	if d.BoundaryVersionID == nil {
		add("boundary", false, "未选择边界版本")
	} else {
		var b models.BoundaryVersion
		if err := db.First(&b, *d.BoundaryVersionID).Error; err != nil || b.Status != "published" {
			add("boundary", false, "边界未发布")
		} else if _, err := ParseGeometry(b.GeometryJSON); err != nil {
			add("boundary", false, err.Error())
		} else {
			add("boundary", true, fmt.Sprintf("v%d", b.Version))
		}
	}
	adapter := ""
	demo := false
	if d.TemplateID == nil {
		add("template", false, "未选择业务模板")
	} else {
		var t models.BusinessTemplate
		if err := db.First(&t, *d.TemplateID).Error; err != nil || t.Status != "published" {
			add("template", false, "业务模板不可用")
		} else {
			add("template", true, t.Code)
			if t.ModelVersionID == nil {
				add("model", false, "业务模板未绑定模型版本")
			} else {
				var mv models.AlgoModelVersion
				if err := db.First(&mv, *t.ModelVersionID).Error; err != nil || mv.Status != "published" {
					add("model", false, "模型版本不存在或未发布")
				} else if _, err := AdapterByName(mv.Adapter); err != nil {
					add("model", false, err.Error())
				} else {
					adapter = mv.Adapter
					impl, _ := AdapterByName(mv.Adapter)
					demo = impl.Demo()
					add("model", true, fmt.Sprintf("model %s (adapter=%s)", mv.Version, mv.Adapter))
				}
			}
		}
	}
	evidDir := filepath.Join(os.TempDir(), "videoai-check")
	if err := os.MkdirAll(evidDir, 0o755); err != nil {
		add("evidence_path", false, err.Error())
	} else {
		add("evidence_path", true, "writable")
	}
	add("backend", true, adapter)
	report["demo_adapter"] = demo
	if strings.TrimSpace(d.ScheduleCron) != "" {
		if err := ValidateScheduleCron(d.ScheduleCron); err != nil {
			add("schedule", false, "时段表达式无效: "+err.Error())
		} else {
			add("schedule", true, d.ScheduleCron)
		}
	} else {
		add("schedule", true, "always")
	}
	if d.TriggerHoldSec > 0 || d.RecoverHoldSec > 0 {
		add("hysteresis", true, fmt.Sprintf("trigger=%.1fs recover=%.1fs", d.TriggerHoldSec, d.RecoverHoldSec))
	} else {
		add("hysteresis", true, "单帧判定（未配置滞回）")
	}
	report["checks"] = checks
	report["ok"] = pass
	return pass, report
}
