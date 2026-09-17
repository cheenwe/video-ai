package scheduler

import (
	"context"
	"errors"
	"fmt"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
	"baseplatform/internal/config"
	"baseplatform/internal/models"
	"baseplatform/internal/services"
)

type Manager struct {
	DB       *gorm.DB
	Cfg      *config.Config
	Backup   *services.OpsBackupService
	parser   cron.Parser
	cron     *cron.Cron
	mu       sync.Mutex
	entries  map[uint]cron.EntryID
	running  map[uint]bool
	stopping bool
	started  bool
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}
type JobView struct {
	models.ScheduleJob
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
}

func NewManager(db *gorm.DB, cfg *config.Config, backup *services.OpsBackupService, _ *services.LDAPAuth) *Manager {
	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{DB: db, Cfg: cfg, Backup: backup, parser: p, cron: cron.New(cron.WithParser(p), cron.WithLocation(time.UTC)), entries: map[uint]cron.EntryID{}, running: map[uint]bool{}, ctx: ctx, cancel: cancel}
}
func validAction(key string) bool {
	return key == "backup" || key == "database_check" || key == "session_cleanup" || key == "ntp_sync"
}
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || m.stopping {
		return errors.New("调度器已启动或停止")
	}
	if err := seedPresets(m.DB); err != nil {
		return err
	}
	if err := m.DB.Model(&models.ScheduleJob{}).Where("last_status = ?", "running").Updates(map[string]any{"last_status": "failed", "last_error": "上次进程退出导致任务中断，错过的执行不补跑"}).Error; err != nil {
		return err
	}
	if err := m.DB.Model(&models.ScheduleRun{}).Where("status = ?", "running").Updates(map[string]any{"status": "failed", "detail": "进程重启，任务中断", "finished_at": time.Now().UTC()}).Error; err != nil {
		return err
	}
	var jobs []models.ScheduleJob
	if err := m.DB.Find(&jobs).Error; err != nil {
		return err
	}
	for _, j := range jobs {
		if j.JobType != "builtin" || !validAction(j.BuiltinKey) {
			if err := m.DB.Model(&j).Updates(map[string]any{"enabled": 0, "last_status": "failed", "last_error": "此动作不在平台内置任务白名单，请重新配置"}).Error; err != nil {
				return err
			}
			continue
		}
		if j.Enabled != 0 {
			if err := m.addLocked(j); err != nil {
				return fmt.Errorf("任务 %d: %w", j.ID, err)
			}
		}
	}
	m.started = true
	m.cron.Start()
	return nil
}
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopping = true
	m.cancel()
	m.mu.Unlock()
	<-m.cron.Stop().Done()
	m.wg.Wait()
}
func (m *Manager) ValidateCron(expr string) error {
	if strings.Contains(expr, "TZ=") || strings.HasPrefix(expr, "@every") {
		return errors.New("请使用五段 Cron 或 @daily 等日历表达式，时区单独配置")
	}
	_, err := m.parser.Parse(expr)
	return err
}
func (m *Manager) validate(j *models.ScheduleJob) error {
	j.Name = strings.TrimSpace(j.Name)
	if j.Name == "" || len([]rune(j.Name)) > 100 {
		return errors.New("任务名称需为1-100字")
	}
	if j.JobType != "builtin" || !validAction(j.BuiltinKey) {
		return errors.New("仅支持平台内置动作")
	}
	if j.Enabled != 0 && j.Enabled != 1 {
		return errors.New("启用状态无效")
	}
	if err := m.ValidateCron(j.Cron); err != nil {
		return err
	}
	if j.Timezone == "" {
		j.Timezone = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(j.Timezone); err != nil {
		return errors.New("无效时区")
	}
	j.CommandID = nil
	j.Args = "{}"
	j.Options = "{}"
	return nil
}
func (m *Manager) addLocked(j models.ScheduleJob) error {
	if err := m.validate(&j); err != nil {
		return err
	}
	id := j.ID
	entry, err := m.cron.AddFunc("CRON_TZ="+j.Timezone+" "+j.Cron, func() {
		if err := m.RunJob(id, "scheduled"); err != nil {
			services.LogSystem(m.DB, "schedule", "skip", "schedule.skipped", fmt.Sprintf("job=%d %s", id, err))
		}
	})
	if err == nil {
		m.entries[id] = entry
	}
	return err
}
func (m *Manager) reloadLocked(j models.ScheduleJob) error {
	if id, ok := m.entries[j.ID]; ok {
		m.cron.Remove(id)
		delete(m.entries, j.ID)
	}
	if j.Enabled != 0 {
		return m.addLocked(j)
	}
	return nil
}
func (m *Manager) NextRun(id uint) *time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.cron.Entry(m.entries[id])
	if entry.Next.IsZero() {
		return nil
	}
	t := entry.Next
	return &t
}
func (m *Manager) ListJobs() ([]JobView, error) {
	var jobs []models.ScheduleJob
	if err := m.DB.Order("id ASC").Find(&jobs).Error; err != nil {
		return nil, err
	}
	out := []JobView{}
	for _, j := range jobs {
		out = append(out, JobView{j, m.NextRun(j.ID)})
	}
	return out, nil
}
func (m *Manager) GetJob(id uint) (*JobView, error) {
	var j models.ScheduleJob
	if err := m.DB.First(&j, id).Error; err != nil {
		return nil, err
	}
	return &JobView{j, m.NextRun(id)}, nil
}
func (m *Manager) CreateJob(j *models.ScheduleJob) error {
	if err := m.validate(j); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return errors.New("服务正在停止")
	}
	*j = models.ScheduleJob{Name: j.Name, JobType: "builtin", BuiltinKey: j.BuiltinKey, Cron: j.Cron, Timezone: j.Timezone, Enabled: j.Enabled, Args: "{}", Options: "{}"}
	if err := m.DB.Create(j).Error; err != nil {
		return err
	}
	return m.reloadLocked(*j)
}
func (m *Manager) UpdateJob(id uint, patch map[string]any) (*models.ScheduleJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running[id] || m.stopping {
		return nil, errors.New("任务运行中或服务停止中")
	}
	var j models.ScheduleJob
	if err := m.DB.First(&j, id).Error; err != nil {
		return nil, err
	}
	for k, v := range patch {
		switch k {
		case "name":
			x, ok := v.(string)
			if !ok {
				return nil, errors.New("名称无效")
			}
			j.Name = x
		case "cron":
			x, ok := v.(string)
			if !ok {
				return nil, errors.New("Cron无效")
			}
			j.Cron = x
		case "timezone":
			x, ok := v.(string)
			if !ok {
				return nil, errors.New("时区无效")
			}
			j.Timezone = x
		case "enabled":
			switch x := v.(type) {
			case bool:
				if x {
					j.Enabled = 1
				} else {
					j.Enabled = 0
				}
			case float64:
				if x != 0 && x != 1 {
					return nil, errors.New("启用状态无效")
				}
				j.Enabled = int(x)
			default:
				return nil, errors.New("启用状态无效")
			}
		default:
			return nil, fmt.Errorf("不支持修改字段: %s", k)
		}
	}
	if err := m.validate(&j); err != nil {
		return nil, err
	}
	if err := m.DB.Save(&j).Error; err != nil {
		return nil, err
	}
	return &j, m.reloadLocked(j)
}
func (m *Manager) DeleteJob(id uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running[id] {
		return errors.New("任务运行中")
	}
	var j models.ScheduleJob
	if err := m.DB.First(&j, id).Error; err != nil {
		return err
	}
	if j.IsBuiltin != 0 {
		return errors.New("预置任务不可删除，可停用")
	}
	if err := m.DB.Delete(&j).Error; err != nil {
		return err
	}
	if entry, ok := m.entries[id]; ok {
		m.cron.Remove(entry)
		delete(m.entries, id)
	}
	return nil
}

// RunJob reserves the execution before responding to the HTTP caller.
func (m *Manager) RunJob(id uint, trigger string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started || m.stopping || m.running[id] || len(m.running) >= 2 {
		return errors.New("任务正在运行、并发已满或服务正在停止")
	}
	var j models.ScheduleJob
	if err := m.DB.First(&j, id).Error; err != nil {
		return err
	}
	if j.Enabled == 0 {
		return errors.New("任务已停用，请先启用")
	}
	if err := m.validate(&j); err != nil {
		return err
	}
	now := time.Now().UTC()
	run := models.ScheduleRun{JobID: id, Status: "running", Trigger: trigger, StartedAt: now}
	if err := m.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		return tx.Model(&j).Updates(map[string]any{"last_status": "running", "last_error": "", "last_run_at": now}).Error
	}); err != nil {
		return err
	}
	m.running[id] = true
	m.wg.Add(1)
	go m.execute(j, run)
	return nil
}
func (m *Manager) execute(j models.ScheduleJob, run models.ScheduleRun) {
	defer m.wg.Done()
	defer func() { m.mu.Lock(); delete(m.running, j.ID); m.mu.Unlock() }()
	start := time.Now()
	detail, err := m.action(j)
	status := "success"
	errorText := ""
	if err != nil {
		status = "failed"
		errorText = err.Error()
		detail = errorText
	}
	end := time.Now().UTC()
	dur := time.Since(start).Milliseconds()
	if e := m.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&run).Updates(map[string]any{"status": status, "detail": detail, "duration_ms": dur, "finished_at": end}).Error; e != nil {
			return e
		}
		updates := map[string]any{"last_status": status, "last_error": errorText, "last_duration": dur, "run_count": gorm.Expr("run_count + 1")}
		if err != nil {
			updates["fail_count"] = gorm.Expr("fail_count + 1")
		}
		return tx.Model(&j).Updates(updates).Error
	}); e != nil {
		services.LogSystem(m.DB, "schedule", "failed", "schedule.persistence_failed", e.Error())
	}
	services.LogSystem(m.DB, "schedule", status, "schedule."+j.BuiltinKey, detail)
}
func (m *Manager) action(j models.ScheduleJob) (string, error) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	defer cancel()
	switch j.BuiltinKey {
	case "database_check":
		db, e := m.DB.DB()
		if e != nil {
			return "", e
		}
		return "数据库连接检查完成", db.PingContext(ctx)
	case "session_cleanup":
		res := m.DB.WithContext(ctx).Where("expires_at IS NOT NULL AND expires_at < ?", time.Now().UTC()).Delete(&models.AuthRevokedToken{})
		return fmt.Sprintf("清理 %d 条过期会话撤销记录", res.RowsAffected), res.Error
	case "backup":
		if m.Backup == nil {
			return "", errors.New("备份服务不可用")
		}
		manifest, e := m.Backup.CreateContext(ctx)
		if e != nil {
			return "", e
		}
		return "备份完成: " + manifest.ID, nil
	case "ntp_sync":
		return "", errors.New("旧NTP任务已停用，请使用运维中心的时间查询；平台不修改操作系统时间")
	}
	return "", errors.New("未知动作")
}
func (m *Manager) StatsSummary() map[string]any {
	jobs, err := m.ListJobs()
	if err != nil {
		return map[string]any{"error": "读取任务失败"}
	}
	enabled, running, failed := 0, 0, 0
	for _, j := range jobs {
		if j.Enabled != 0 {
			enabled++
		}
		if j.LastStatus == "running" {
			running++
		}
		if j.LastStatus == "failed" {
			failed++
		}
	}
	return map[string]any{"total": len(jobs), "enabled": enabled, "running": running, "failed": failed}
}
func (m *Manager) ListCommands() ([]models.ScheduleCommand, error) {
	return []models.ScheduleCommand{}, nil
}
func (m *Manager) CreateCommand(*models.ScheduleCommand) error {
	return errors.New("不支持外部指令，请选择内置任务")
}
func (m *Manager) UpdateCommand(uint, map[string]any) (*models.ScheduleCommand, error) {
	return nil, errors.New("不支持外部指令")
}
func (m *Manager) DeleteCommand(uint) error { return errors.New("不支持外部指令") }
