package recorder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is one recording window. From/To are "HH:MM" in local time and Days
// are ISO weekdays (1=Mon … 7=Sun); empty Days means every day. A window whose
// To is earlier than From crosses midnight.
type Window struct {
	From string `json:"from"`
	To   string `json:"to"`
	Days []int  `json:"days,omitempty"`
}

// Schedule is the parsed form of RecordingPlan.ScheduleJSON. The shape is:
//
//	{"windows":[{"from":"08:00","to":"18:00","days":[1,2,3,4,5]}]}
//
// An empty schedule means "always", which is what continuous mode uses.
type Schedule struct {
	Windows []Window `json:"windows"`
}

// ParseSchedule validates a schedule_json payload. It returns nil for an empty
// string so callers can treat "no schedule" as always-on; malformed input is an
// error rather than a silent fallback to always recording.
func ParseSchedule(raw string) (*Schedule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var s Schedule
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, fmt.Errorf("时段定义不是合法 JSON: %w", err)
	}
	if len(s.Windows) == 0 {
		return nil, fmt.Errorf("时段定义缺少 windows")
	}
	for i, w := range s.Windows {
		if _, _, err := parseClock(w.From); err != nil {
			return nil, fmt.Errorf("第 %d 个时段的开始时间无效: %w", i+1, err)
		}
		if _, _, err := parseClock(w.To); err != nil {
			return nil, fmt.Errorf("第 %d 个时段的结束时间无效: %w", i+1, err)
		}
		for _, d := range w.Days {
			if d < 1 || d > 7 {
				return nil, fmt.Errorf("第 %d 个时段的星期取值无效: %d（1=周一 … 7=周日）", i+1, d)
			}
		}
	}
	return &s, nil
}

// Active reports whether t falls inside any window. nil means always on.
func (s *Schedule) Active(t time.Time) bool {
	if s == nil || len(s.Windows) == 0 {
		return true
	}
	for _, w := range s.Windows {
		if w.contains(t) {
			return true
		}
	}
	return false
}

// contains evaluates one window against the wall clock. `weekdayOK` answers
// "does the window open on this ISO weekday"; for an overnight window the
// early-morning part is matched against the weekday the window started on.
func (w Window) contains(t time.Time) bool {
	fromH, fromM, err1 := parseClock(w.From)
	toH, toM, err2 := parseClock(w.To)
	if err1 != nil || err2 != nil {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	from := fromH*60 + fromM
	to := toH*60 + toM
	today := isoWeekday(t)
	weekdayOK := func(d int) bool { return len(w.Days) == 0 || containsInt(w.Days, d) }

	if from == to {
		// 00:00–00:00 reads as "the whole day".
		return weekdayOK(today)
	}
	if from < to {
		return weekdayOK(today) && cur >= from && cur < to
	}
	// Overnight window: [from, 24:00) belongs to today's window, [00:00, to)
	// to the one that opened yesterday.
	if cur >= from {
		return weekdayOK(today)
	}
	if cur < to {
		return weekdayOK(isoWeekday(t.AddDate(0, 0, -1)))
	}
	return false
}

// isoWeekday maps Go's Sunday=0 to ISO Sunday=7.
func isoWeekday(t time.Time) int {
	if d := int(t.Weekday()); d != 0 {
		return d
	}
	return 7
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// parseClock turns "HH:MM" into hour/minute.
func parseClock(s string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("需要 HH:MM 格式，收到 %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("小时无效: %q", parts[0])
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("分钟无效: %q", parts[1])
	}
	return h, m, nil
}
