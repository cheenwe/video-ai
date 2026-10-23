package videoai

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func newScheduleManager() *Manager {
	return &Manager{parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)}
}

func TestInScheduleAlwaysOn(t *testing.T) {
	m := newScheduleManager()
	if active, valid := m.inSchedule("", time.Now()); !active || !valid {
		t.Fatal("empty cron should always be in schedule")
	}
}

func TestInScheduleBusinessHours(t *testing.T) {
	m := newScheduleManager()
	// Monday 10:00 should match "* 8-18 * * 1-5"
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.Local) // Monday
	if active, valid := m.inSchedule("* 8-18 * * 1-5", now); !active || !valid {
		t.Fatal("expected in schedule")
	}
	night := time.Date(2026, 10, 12, 22, 0, 0, 0, time.Local)
	if active, valid := m.inSchedule("* 8-18 * * 1-5", night); active || !valid {
		t.Fatalf("expected out of schedule at night (active=%v valid=%v)", active, valid)
	}
}

// TestInScheduleInvalidCronIsNotAlwaysOn locks P1-2: an unparsable expression
// must be reported invalid so the tick surfaces a fault, instead of silently
// analysing around the clock.
func TestInScheduleInvalidCronIsNotAlwaysOn(t *testing.T) {
	m := newScheduleManager()
	active, valid := m.inSchedule("not a cron", time.Now())
	if active || valid {
		t.Fatalf("invalid cron must not be always-on: active=%v valid=%v", active, valid)
	}
}

// TestValidateScheduleCron covers the shared save/self-check gate: empty stays
// always-on, valid expressions pass, garbage is rejected.
func TestValidateScheduleCron(t *testing.T) {
	if err := ValidateScheduleCron(""); err != nil {
		t.Fatalf("empty cron should be valid: %v", err)
	}
	if err := ValidateScheduleCron("* 8-18 * * 1-5"); err != nil {
		t.Fatalf("business-hours cron should be valid: %v", err)
	}
	if err := ValidateScheduleCron("@daily"); err != nil {
		t.Fatalf("descriptor should be valid: %v", err)
	}
	for _, bad := range []string{"not a cron", "* * *", "99 * * * *", "* 8-18 * * 8"} {
		if err := ValidateScheduleCron(bad); err == nil {
			t.Fatalf("expected rejection for %q", bad)
		}
	}
}
