package units

import "testing"

// The OnCalendar derivation (Spec S01.7 persistent calendar timers) is TOTAL
// over the ⚙ backup.interval clamp (6 h – 7 d) — every value renders, including
// whole-hour NON-divisors of a day (F7) — and only a non-positive (never
// clamp-valid) value errors.
func TestOnCalendarFromSeconds(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
		err  bool
	}{
		{21600, "*-*-* 0/6:00:00", false},     // 6 h (clamp floor)
		{25200, "*-*-* 0/7:00:00", false},     // 7 h — a NON-divisor of a day (F7): renders, never errors
		{32400, "*-*-* 0/9:00:00", false},     // 9 h — another non-divisor
		{43200, "*-*-* 0/12:00:00", false},    // 12 h
		{86400, "*-*-* 00:00:00", false},      // daily (default)
		{604800, "Mon *-*-* 00:00:00", false}, // 7 d (clamp ceiling)
		{172800, "*-*-1/2 00:00:00", false},   // 2 d (day step)
		{90000, "*-*-1/1 00:00:00", false},    // 25 h — nearest whole day (no failure inside the clamp)
		{0, "", true},                         // non-positive — never clamp-valid
	}
	for _, c := range cases {
		got, err := onCalendarFromSeconds(c.sec)
		if c.err {
			if err == nil {
				t.Errorf("onCalendarFromSeconds(%d) = %q, want error", c.sec, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("onCalendarFromSeconds(%d) = %q, %v; want %q", c.sec, got, err, c.want)
		}
	}
}

// TestOnCalendarTotalOverClamp: no ⚙ backup.interval value in the clamp
// (6 h – 7 d) ever fails generation (F7).
func TestOnCalendarTotalOverClamp(t *testing.T) {
	for sec := int64(21600); sec <= 604800; sec += 137 { // prime-step sweep
		if _, err := onCalendarFromSeconds(sec); err != nil {
			t.Fatalf("onCalendarFromSeconds(%d) failed inside the clamp: %v", sec, err)
		}
	}
}

func TestOnCalendarFromMonths(t *testing.T) {
	if got := onCalendarFromMonths(1); got != "*-*-01 00:00:00" {
		t.Errorf("monthly = %q", got)
	}
	if got := onCalendarFromMonths(3); got != "*-1/3-01 00:00:00" {
		t.Errorf("quarterly = %q", got)
	}
}
