package api

import (
	"strings"
	"testing"
)

// TestTaskIntakeStateQuerySeeks is the P3-RW-17 R8 half of the derive-queries-
// are-served-by-an-index discipline (§38 drain D7): the Stage-0 triage read hops
// task → runs → run_events, which is the hop migration 0022 measured for its pin
// edge, and it must SEEK on the event leg. That leg is the one whose cost would
// otherwise grow with the whole event log rather than with the task.
//
// The assertion is against the PRODUCTION text, not a paraphrase — a query
// edited into a full scan is exactly what a paraphrase cannot catch.
func TestTaskIntakeStateQuerySeeks(t *testing.T) {
	db, log, runs := wdTestDB(t)
	seedIndexFixture(t, log, runs)

	detail := planFor(t, db, QueryTaskIntakeState, "t1")
	if !strings.Contains(detail, "run_events_run_type_idx") {
		t.Errorf("the intake.state seek is not served by 0015's run_events_run_type_idx:\n%s", detail)
	}
	if strings.Contains(detail, "SCAN run_events") {
		t.Errorf("the triage read full-scans run_events:\n%s", detail)
	}
	// The CROSS JOIN is what pins `runs` as the outer loop; a plain JOIN lets the
	// planner reverse the order and walk the type index instead (0022's measured
	// finding, carried over rather than re-guessed).
	if !strings.Contains(QueryTaskIntakeState, "CROSS JOIN") {
		t.Error("the join order is no longer pinned — the outer loop must stay `runs`")
	}
}
