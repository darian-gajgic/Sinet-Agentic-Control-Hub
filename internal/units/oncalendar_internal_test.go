package units

import "testing"

// The OnCalendar derivation (Spec S01.7 persistent calendar timers) covers the
// ⚙ backup.interval clamp (6 h – 7 d) and the drill months, and errors LOUDLY
// on a non-expressible interval rather than silently mis-scheduling.
func TestOnCalendarFromSeconds(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
		err  bool
	}{
		{21600, "*-*-* 0/6:00:00", false},     // 6 h (clamp floor)
		{43200, "*-*-* 0/12:00:00", false},    // 12 h
		{86400, "*-*-* 00:00:00", false},      // daily (default)
		{604800, "Mon *-*-* 00:00:00", false}, // 7 d (clamp ceiling)
		{172800, "*-*-1/2 00:00:00", false},   // 2 d (approximate day step)
		{5400, "", true},                      // 90 min — not a whole-hour day divisor
		{0, "", true},
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

func TestOnCalendarFromMonths(t *testing.T) {
	if got := onCalendarFromMonths(1); got != "*-*-01 00:00:00" {
		t.Errorf("monthly = %q", got)
	}
	if got := onCalendarFromMonths(3); got != "*-1/3-01 00:00:00" {
		t.Errorf("quarterly = %q", got)
	}
}
