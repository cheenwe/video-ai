package recorder

import (
	"testing"
	"time"
)

func at(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.Local)
}

// TestParseScheduleValidatesInput: bad JSON or bad clocks are errors, never a
// silent "always record".
func TestParseScheduleValidatesInput(t *testing.T) {
	if s, err := ParseSchedule(""); err != nil || s != nil {
		t.Fatalf("empty schedule must mean always-on, got %v / %v", s, err)
	}
	bad := []string{
		`not json`,
		`{"windows":[]}`,
		`{"windows":[{"from":"8","to":"18:00"}]}`,
		`{"windows":[{"from":"08:00","to":"24:00"}]}`,
		`{"windows":[{"from":"08:00","to":"18:00","days":[0]}]}`,
		`{"windows":[{"from":"08:00","to":"18:00","days":[8]}]}`,
	}
	for _, raw := range bad {
		if _, err := ParseSchedule(raw); err == nil {
			t.Fatalf("%s should be rejected", raw)
		}
	}
	good := `{"windows":[{"from":"08:00","to":"18:00","days":[1,2,3,4,5]}]}`
	if _, err := ParseSchedule(good); err != nil {
		t.Fatal(err)
	}
}

// TestScheduleActiveSameDayWindow: 2026-10-24 is a Saturday (ISO 6), so a
// Mon–Fri 08:00–18:00 window is closed all day.
func TestScheduleActiveSameDayWindow(t *testing.T) {
	s, err := ParseSchedule(`{"windows":[{"from":"08:00","to":"18:00","days":[1,2,3,4,5]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	// Friday 2026-10-23.
	friday := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	if !s.Active(at(friday.Year(), friday.Month(), friday.Day(), 9, 0)) {
		t.Fatal("09:00 Friday should be inside the window")
	}
	if s.Active(at(friday.Year(), friday.Month(), friday.Day(), 7, 59)) {
		t.Fatal("07:59 is before the window")
	}
	if s.Active(at(friday.Year(), friday.Month(), friday.Day(), 18, 0)) {
		t.Fatal("18:00 is the exclusive end")
	}
	saturday := time.Date(2026, 10, 24, 0, 0, 0, 0, time.Local)
	if s.Active(at(saturday.Year(), saturday.Month(), saturday.Day(), 10, 0)) {
		t.Fatal("Saturday is not in the day list")
	}
}

// TestScheduleActiveOvernightWindow: 22:00–06:00 spans midnight, so early
// morning belongs to the previous day's window.
func TestScheduleActiveOvernightWindow(t *testing.T) {
	s, err := ParseSchedule(`{"windows":[{"from":"22:00","to":"06:00","days":[5]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	// Friday 2026-10-23 22:00 opens the window; Saturday 05:00 is still inside.
	if !s.Active(time.Date(2026, 10, 23, 23, 0, 0, 0, time.Local)) {
		t.Fatal("Friday 23:00 should be inside")
	}
	if !s.Active(time.Date(2026, 10, 24, 5, 0, 0, 0, time.Local)) {
		t.Fatal("Saturday 05:00 should still be inside Friday's window")
	}
	if s.Active(time.Date(2026, 10, 24, 7, 0, 0, 0, time.Local)) {
		t.Fatal("Saturday 07:00 is after the window")
	}
	// Monday 02:00 is not inside a Friday window (Sunday was not selected).
	if s.Active(time.Date(2026, 10, 26, 2, 0, 0, 0, time.Local)) {
		t.Fatal("Monday 02:00 must not match a Friday window")
	}
}

// TestScheduleEveryDayWindow: no days list means every day.
func TestScheduleEveryDayWindow(t *testing.T) {
	s, err := ParseSchedule(`{"windows":[{"from":"00:00","to":"12:00"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Active(time.Date(2026, 10, 24, 6, 30, 0, 0, time.Local)) {
		t.Fatal("06:30 should be inside an every-day morning window")
	}
	if s.Active(time.Date(2026, 10, 24, 13, 0, 0, 0, time.Local)) {
		t.Fatal("13:00 should be outside")
	}
}

// TestScheduleAllDayWindow: 00:00–00:00 means the whole day.
func TestScheduleAllDayWindow(t *testing.T) {
	s, err := ParseSchedule(`{"windows":[{"from":"00:00","to":"00:00"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []int{0, 6, 12, 23} {
		if !s.Active(time.Date(2026, 10, 24, h, 0, 0, 0, time.Local)) {
			t.Fatalf("%02d:00 should be inside an all-day window", h)
		}
	}
}

// TestNilScheduleIsAlwaysActive keeps continuous mode working.
func TestNilScheduleIsAlwaysActive(t *testing.T) {
	var s *Schedule
	if !s.Active(time.Now()) {
		t.Fatal("a nil schedule must mean always on")
	}
}
