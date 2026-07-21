package ledger

import "testing"

func TestSubStageNaming(t *testing.T) {
	cases := []struct {
		planned string
		k       int
		name    string
	}{
		{"S-1", 1, "S-1"},
		{"S-1", 0, "S-1"},
		{"S-1", 2, "S-1#2"},
		{"S-1", 3, "S-1#3"},
		{"plan", 2, "plan#2"},
	}
	for _, c := range cases {
		if got := SubStageName(c.planned, c.k); got != c.name {
			t.Fatalf("SubStageName(%q,%d) = %q, want %q", c.planned, c.k, got, c.name)
		}
		if got := PlannedStage(c.name); got != c.planned {
			t.Fatalf("PlannedStage(%q) = %q, want %q", c.name, got, c.planned)
		}
	}
}

func TestPlannedStageConservative(t *testing.T) {
	// Non-ordinal shapes are never rewritten (a plan step id is what it is).
	for _, s := range []string{"S-1", "S-1#", "S-1#x", "#2", "revise-r2", "S-1#2b"} {
		want := s
		if s == "S-1#2b" || s == "S-1#x" || s == "S-1#" {
			want = s // explicit: trailing non-digit suffixes stay
		}
		if got := PlannedStage(s); got != want {
			t.Fatalf("PlannedStage(%q) = %q, want %q", s, got, want)
		}
	}
}
