package videoai

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestInScheduleAlwaysOn(t *testing.T) {
	m := &Manager{parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)}
	if !m.inSchedule("", time.Now()) {
		t.Fatal("empty cron should always be in schedule")
	}
}

func TestInScheduleBusinessHours(t *testing.T) {
	m := &Manager{parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)}
	// Monday 10:00 should match "* 8-18 * * 1-5"
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.Local) // Monday
	if !m.inSchedule("* 8-18 * * 1-5", now) {
		t.Fatal("expected in schedule")
	}
	night := time.Date(2026, 10, 12, 22, 0, 0, 0, time.Local)
	if m.inSchedule("* 8-18 * * 1-5", night) {
		t.Fatal("expected out of schedule at night")
	}
}
