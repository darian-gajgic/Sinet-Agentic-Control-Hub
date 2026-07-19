package scheduler

import (
	"testing"
	"time"
)

func cand(id string, class WorkloadClass, ageHours float64, now time.Time) candidate {
	return candidate{
		queueID:    int64(len(id)),
		runID:      id,
		userID:     "u",
		lane:       "anthropic",
		class:      class,
		enqueuedTS: now.Add(-time.Duration(ageHours * float64(time.Hour))),
	}
}

func TestInteractiveNeverStarvedByAutomation(t *testing.T) {
	now := time.Now()
	interactive := cand("i", ClassInteractive, 0, now)  // brand new
	background := cand("b", ClassBackground, 1000, now) // aged 1000h
	// 3.3: interactive is CRITICAL_PLUS — it outranks any automation class
	// regardless of how long the automation has waited.
	if !priorityLess(interactive, background, now) {
		t.Fatal("fresh interactive must outrank aged background (3.3)")
	}
	if priorityLess(background, interactive, now) {
		t.Fatal("aged background must NOT outrank interactive (3.3)")
	}
}

func TestAgingPreventsBackgroundStarvation(t *testing.T) {
	now := time.Now()
	// A background item that has waited long enough overtakes fresh higher-
	// class automation work — "aging so background never starves" (S10.7).
	oldBackground := cand("b", ClassBackground, 3, now) // 3h old
	freshHumanBlocked := cand("h", ClassHumanBlocked, 0, now)
	if !priorityLess(oldBackground, freshHumanBlocked, now) {
		t.Fatal("a long-waiting background item must eventually overtake fresh human-blocked work (aging, S10.7)")
	}
	// But a FRESH background does not jump a fresh higher class.
	freshBackground := cand("b2", ClassBackground, 0, now)
	if priorityLess(freshBackground, freshHumanBlocked, now) {
		t.Fatal("fresh background must not outrank fresh human-blocked (ladder order, S10.7)")
	}
}

func TestFIFOWithinClass(t *testing.T) {
	now := time.Now()
	older := cand("older", ClassBackground, 2, now)
	newer := cand("newer", ClassBackground, 1, now)
	if !priorityLess(older, newer, now) {
		t.Fatal("within a class, the older enqueue is admitted first (FIFO)")
	}
}

func TestSortCandidatesOrdering(t *testing.T) {
	now := time.Now()
	cands := []candidate{
		cand("bg-fresh", ClassBackground, 0, now),
		cand("interactive", ClassInteractive, 0, now),
		cand("scheduled", ClassScheduled, 0, now),
		cand("bg-old", ClassBackground, 5, now), // aged past scheduled
	}
	sortCandidates(cands, now)
	if cands[0].runID != "interactive" {
		t.Fatalf("first = %q, want interactive (never starved by automation)", cands[0].runID)
	}
	// The 5h-aged background should have overtaken fresh scheduled work.
	pos := map[string]int{}
	for i, c := range cands {
		pos[c.runID] = i
	}
	if pos["bg-old"] >= pos["scheduled"] {
		t.Fatalf("aged background did not overtake fresh scheduled: order %v", runIDs(cands))
	}
	if pos["bg-fresh"] <= pos["scheduled"] {
		t.Fatalf("fresh background outranked scheduled: order %v", runIDs(cands))
	}
}

func runIDs(cs []candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.runID
	}
	return out
}
