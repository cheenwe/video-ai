package videoai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"baseplatform/internal/models"
	"baseplatform/internal/services"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// Manager runs in-process mock analysis loops for active deployments.
type Manager struct {
	DB       *gorm.DB
	DataHome string
	Bus      *services.EventBus
	Adapter  MockAdapter
	logger   *log.Logger
	parser   cron.Parser

	mu      sync.Mutex
	cancels map[uint]context.CancelFunc
	wg      sync.WaitGroup
}

func NewManager(db *gorm.DB, dataHome string, bus *services.EventBus) *Manager {
	return &Manager{
		DB:       db,
		DataHome: dataHome,
		Bus:      bus,
		Adapter:  MockAdapter{},
		logger:   log.New(os.Stdout, "[videoai] ", log.LstdFlags|log.Lmsgprefix),
		parser:   cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		cancels:  map[uint]context.CancelFunc{},
	}
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
	}
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
			Backend:      "mock",
			ModelLoaded:  1,
			UpdatedAt:    time.Now(),
		}
		_ = m.DB.Create(&rt).Error
		return
	}
	_ = m.DB.Model(&rt).Updates(map[string]any{
		"state":        "running",
		"backend":      "mock",
		"model_loaded": 1,
		"updated_at":   time.Now(),
	}).Error
}

func (m *Manager) runLoop(ctx context.Context, id uint) {
	interval := time.Second
	ticker := time.NewTicker(interval)
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
	if !m.inSchedule(d.ScheduleCron, now) {
		if d.Status != "out_of_schedule" {
			_ = m.DB.Model(&d).Update("status", "out_of_schedule").Error
			d.Status = "out_of_schedule"
		}
		m.bumpRuntime(d, ResultUnknown, 0, "计划外时段，不分析", now)
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
		m.bumpRuntime(d, ResultOffline, 0, "摄像头已停用", now)
		return nil
	}

	var bound models.BoundaryVersion
	mediaOK := true
	geom := Geometry{}
	if d.BoundaryVersionID != nil {
		if err := m.DB.First(&bound, *d.BoundaryVersionID).Error; err != nil || bound.Status != "published" {
			mediaOK = false
			m.bumpRuntime(d, ResultUnknown, 0, "边界未发布或无效", now)
			_ = m.DB.Model(&cam).Update("analysis_status", "fault").Error
			return nil
		}
		g, err := ParseGeometry(bound.GeometryJSON)
		if err != nil {
			m.bumpRuntime(d, ResultUnknown, 0, err.Error(), now)
			return nil
		}
		geom = g
	} else {
		m.bumpRuntime(d, ResultUnknown, 0, "未绑定边界版本", now)
		return nil
	}

	fw, fh := float64(bound.RefWidth), float64(bound.RefHeight)
	if fw <= 0 {
		fw = 1280
	}
	if fh <= 0 {
		fh = 720
	}
	dets := m.Adapter.Infer(cam.ID, now, fw, fh)
	out := EvaluateGrabRules(RuleInput{
		Detections:     dets,
		Geometry:       geom,
		ThresholdNear:  d.ThresholdNear,
		ThresholdCross: d.ThresholdCross,
		FrameW:         fw,
		FrameH:         fh,
		MediaOK:        mediaOK,
	})
	out.Overlay["demo"] = true
	out.Overlay["adapter"] = "mock"

	m.bumpRuntime(d, out.Result, out.Distance, out.Reason, now)
	_ = m.DB.Model(&cam).Updates(map[string]any{
		"analysis_status": "running",
		"media_status":    "online",
		"device_status":   "online",
	}).Error
	_ = m.DB.Model(&d).Update("applied_revision", d.DesiredRevision).Error

	if out.Result == ResultNear || out.Result == ResultCross {
		return m.mergeEvent(d, cam, bound, out, now)
	}
	return m.recoverActiveEvents(d.ID, now)
}

func (m *Manager) bumpRuntime(d models.Deployment, result string, dist float64, reason string, now time.Time) {
	hb := now
	_ = m.DB.Model(&models.RuntimeStatus{}).Where("deployment_id = ?", d.ID).Updates(map[string]any{
		"state":        d.Status,
		"applied_rev":  d.DesiredRevision,
		"actual_fps":   d.AnalysisFPS,
		"frame_age_ms": 200,
		"last_result":  result,
		"last_error":   reason,
		"updated_at":   now,
		"heartbeat_at": &hb,
		"backend":      "mock",
		"model_loaded": 1,
	}).Error
	_ = dist
}

func (m *Manager) mergeEvent(d models.Deployment, cam models.Camera, bound models.BoundaryVersion, out RuleOutput, now time.Time) error {
	key := fmt.Sprintf("cam%d-%s-active", cam.ID, out.RuleCode)
	metric, _ := json.Marshal(out.Metrics)
	overlay, _ := json.Marshal(out.Overlay)
	snap, _ := json.Marshal(map[string]any{
		"deployment_id":       d.ID,
		"desired_revision":    d.DesiredRevision,
		"boundary_version_id": bound.ID,
		"boundary_version":    bound.Version,
		"template_id":         d.TemplateID,
		"adapter":             "mock",
		"demo":                true,
		"rule_code":           out.RuleCode,
	})

	var ev models.VideoEvent
	err := m.DB.Where("event_key = ? AND scene_status = ?", key, "active").First(&ev).Error
	if err == gorm.ErrRecordNotFound {
		ev = models.VideoEvent{
			EventKey:           key,
			CameraID:           cam.ID,
			ProjectID:          d.ProjectID,
			DeploymentID:       &d.ID,
			Category:           out.RuleCode,
			Level:              levelFor(out.Result),
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
		_ = m.createEvidence(ev, out)
		if d.MuteNotify == 0 {
			m.notify(ev, cam, "新的视频分析事件")
		}
		return nil
	}
	_ = m.DB.Model(&ev).Updates(map[string]any{
		"last_evidence_at":     now,
		"level":                levelFor(out.Result),
		"category":             out.RuleCode,
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

func (m *Manager) createEvidence(ev models.VideoEvent, out RuleOutput) error {
	dir := filepath.Join(m.DataHome, "evidence", fmt.Sprintf("%d", ev.ID))
	_ = os.MkdirAll(dir, 0o755)
	meta, _ := json.Marshal(map[string]any{
		"demo":   true,
		"result": out.Result,
		"reason": out.Reason,
	})
	path := filepath.Join(dir, "overlay.json")
	_ = os.WriteFile(path, []byte(mustJSON(out.Overlay)), 0o644)
	e := models.Evidence{
		EventID:  ev.ID,
		Kind:     "overlay",
		Status:   "ready",
		Path:     path,
		MetaJSON: string(meta),
	}
	return m.DB.Create(&e).Error
}

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
		"demo":      true,
	})
}

// inSchedule treats empty cron as always-on. Non-empty cron uses standard 5-field
// expressions (e.g. "* 8-18 * * 1-5") matched against the current minute.
func (m *Manager) inSchedule(expr string, now time.Time) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	sched, err := m.parser.Parse(expr)
	if err != nil {
		return true // invalid cron should not silently stop analysis; self-check can surface it
	}
	prev := sched.Next(now.Add(-2 * time.Minute))
	return !prev.After(now) && now.Sub(prev) < 2*time.Minute
}

func levelFor(result string) string {
	if result == ResultCross {
		return "critical"
	}
	return "warning"
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// SelfCheck validates whether a deployment can start.
func SelfCheck(db *gorm.DB, d models.Deployment) (ok bool, report map[string]any) {
	report = map[string]any{"demo_adapter": true}
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
	if d.TemplateID == nil {
		add("template", false, "未选择业务模板")
	} else {
		var t models.BusinessTemplate
		if err := db.First(&t, *d.TemplateID).Error; err != nil || t.Status != "published" {
			add("template", false, "业务模板不可用")
		} else {
			add("template", true, t.Code)
		}
	}
	evidDir := filepath.Join(os.TempDir(), "videoai-check")
	if err := os.MkdirAll(evidDir, 0o755); err != nil {
		add("evidence_path", false, err.Error())
	} else {
		add("evidence_path", true, "writable")
	}
	add("backend", true, "mock")
	if strings.TrimSpace(d.ScheduleCron) != "" {
		p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		if _, err := p.Parse(d.ScheduleCron); err != nil {
			add("schedule", false, "时段表达式无效: "+err.Error())
		} else {
			add("schedule", true, d.ScheduleCron)
		}
	} else {
		add("schedule", true, "always")
	}
	report["checks"] = checks
	report["ok"] = pass
	return pass, report
}
