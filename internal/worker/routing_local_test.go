package worker

import (
	"context"
	"strings"
	"testing"
)

// routing_local_test.go — the R22 consumer-class split: with the S12 local
// stack serving (Coverage.LocalAvailable true), a class-(a) ENGINE dispatch
// onto duty utility/mechanical still degrades to the paid execution seat with
// the refined honest reason (no local engine adapter this cut) — the B3-3
// ERROR stub is GONE, and it is NEVER an error (absent duties degrade, never
// faked).

func TestResolveSeatLocalAvailableDegradesNotError(t *testing.T) {
	r := &Router{
		DutyMap:  DefaultDutyMap(),
		Coverage: Coverage{FlatRateLanes: []string{"anthropic"}, LocalAvailable: true},
	}
	seat, _, reason, gap, err := r.resolveSeat(context.Background(), RouteQuery{}, ExecutionProfile{Duty: DutyUtility})
	if err != nil {
		t.Fatalf("resolveSeat errored — the removed ERROR stub is back (R22): %v", err)
	}
	if gap != "" {
		t.Fatalf("unexpected gap advice: %q", gap)
	}
	exec := DefaultDutyMap()[DutyExecution]
	if seat.Model != exec.Model || seat.Lane != exec.Lane {
		t.Errorf("seat = %+v, want the paid execution seat %+v", seat, exec)
	}
	if !strings.Contains(reason, "engine lane carries no v0 consumer") {
		t.Errorf("reason = %q, want the refined class-(a) honest reason", reason)
	}
	// Mechanical helper duties degrade the same way.
	_, _, reason2, _, err2 := r.resolveSeat(context.Background(), RouteQuery{Mechanical: true}, ExecutionProfile{Duty: DutyExecution})
	if err2 != nil {
		t.Fatalf("mechanical dispatch errored: %v", err2)
	}
	if !strings.Contains(reason2, "engine lane carries no v0 consumer") {
		t.Errorf("mechanical reason = %q, want the refined reason", reason2)
	}
}

func TestResolveSeatLocalAbsentDegradesWithAbsenceReason(t *testing.T) {
	r := &Router{
		DutyMap:  DefaultDutyMap(),
		Coverage: Coverage{FlatRateLanes: []string{"anthropic"}, LocalAvailable: false},
	}
	seat, _, reason, _, err := r.resolveSeat(context.Background(), RouteQuery{}, ExecutionProfile{Duty: DutyUtility})
	if err != nil {
		t.Fatalf("resolveSeat: %v", err)
	}
	exec := DefaultDutyMap()[DutyExecution]
	if seat.Model != exec.Model {
		t.Errorf("seat model = %q, want the execution seat %q", seat.Model, exec.Model)
	}
	if !strings.Contains(reason, "not configured") {
		t.Errorf("reason = %q, want the not-configured reason", reason)
	}
}
