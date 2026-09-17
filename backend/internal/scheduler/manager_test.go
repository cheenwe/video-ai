package scheduler_test

import (
	"testing"
	"time"
	"baseplatform/internal/config"
	"baseplatform/internal/database"
	"baseplatform/internal/models"
	"baseplatform/internal/scheduler"
	"baseplatform/internal/services"
)

func TestSchedulerLifecycleAndHistory(t *testing.T) {
	cfg := &config.Config{DataHome: t.TempDir(), DBDriver: "sqlite"}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	m := scheduler.NewManager(db, cfg, services.NewOpsBackupService(cfg, db), nil)
	if err = m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	jobs, err := m.ListJobs()
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Enabled != 0 {
			t.Fatal("preset enabled without operator action")
		}
	}
	j := models.ScheduleJob{Name: "UTC health", JobType: "builtin", BuiltinKey: "database_check", Cron: "0 0 * * *", Timezone: "UTC", Enabled: 1}
	if err = m.CreateJob(&j); err != nil {
		t.Fatal(err)
	}
	next := m.NextRun(j.ID)
	if next == nil || next.UTC().Hour() != 0 || next.UTC().Minute() != 0 {
		t.Fatalf("wrong UTC next execution: %v", next)
	}
	if err = m.RunJob(j.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var run models.ScheduleRun
		db.Where("job_id = ?", j.ID).First(&run)
		if run.Status == "success" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %+v", run)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = m.UpdateJob(j.ID, map[string]any{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	if m.RunJob(j.ID, "manual") == nil {
		t.Fatal("disabled job executed")
	}
	if m.ValidateCron("@every 1s") == nil {
		t.Fatal("unbounded interval allowed")
	}
	if err = m.CreateJob(&models.ScheduleJob{Name: "external", JobType: "command", Cron: "0 0 * * *", Enabled: 1}); err == nil {
		t.Fatal("external command accepted")
	}
	m.Stop()
	if m.RunJob(j.ID, "manual") == nil {
		t.Fatal("job accepted after stop")
	}
}
func TestInvalidStoredScheduleFailsStartup(t *testing.T) {
	cfg := &config.Config{DataHome: t.TempDir(), DBDriver: "sqlite"}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	db.Create(&models.ScheduleJob{Name: "broken", JobType: "builtin", BuiltinKey: "database_check", Cron: "bad", Timezone: "UTC", Enabled: 1})
	m := scheduler.NewManager(db, cfg, nil, nil)
	defer m.Stop()
	if m.Start() == nil {
		t.Fatal("invalid schedule startup error ignored")
	}
}
