package worker

import "testing"

// tq5_dutymap_test.go — P3-TQ-5 acceptance (gate record
// P3/gates/rework-sitting-gate.md item B7 + §Answers, 2026-09-17; Spec S06.10
// duty classes, S07.5 judge class). The shipped duty map retires
// claude-sonnet-5 and claude-opus-4-8: every seat the map carries rides
// claude-opus-5 on the anthropic lane (the seat of last resort for execution,
// the only seat for planning and judging — AlternateSeatsFor stays
// EXECUTION ONLY). The model id was live-verified 2026-09-17 against
// https://platform.claude.com/docs/en/models/overview ("Claude API ID:
// claude-opus-5").

func TestTQ5DefaultDutyMapIsOpus5OnEverySeat(t *testing.T) {
	m := DefaultDutyMap()
	for _, duty := range []string{DutyExecution, DutyPlanning, DutyJudge} {
		seat, ok := m[duty]
		if !ok {
			t.Fatalf("duty %s has no seat", duty)
		}
		if seat.Model != "claude-opus-5" || seat.Lane != "anthropic" || seat.WindowTokens != DefaultWindowTokens {
			t.Errorf("seat %s = %+v, want claude-opus-5 on anthropic at the %d budgeting floor (B7 ruling)", duty, seat, DefaultWindowTokens)
		}
	}
	if _, ok := m[DutyUtility]; ok {
		t.Error("utility seat must stay ABSENT (S06.10 pins it local; absent duties degrade, never fake)")
	}
	for duty, seat := range m {
		for _, retired := range []string{"claude-sonnet-5", "claude-opus-4-8"} {
			if seat.Model == retired {
				t.Errorf("seat %s still rides the retired model %q", duty, retired)
			}
		}
	}
}

// A Router built WITHOUT a lane order takes exactly the pre-TQ-5 path: the
// duty-map seat first, then the alternates in the order they were handed in
// (the LN-4 D7 expectation, TestLN4PlacedCredentialIsSelectedByRouting). This
// is green at grounding by design — it guards the nil path the new order must
// not disturb.
func TestTQ5NilLaneOrderKeepsTheDutyMapSeatFirst(t *testing.T) {
	r := &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: AlternateSeatsFor(LaneSeat{Lane: "kimi-cli", Model: "k3"}),
		Coverage:   Coverage{FlatRateLanes: []string{"anthropic", "kimi-cli"}},
	}
	seat, _, _, gap, err := r.resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat: err=%v gap=%q", err, gap)
	}
	if seat.Lane != "anthropic" {
		t.Errorf("with no lane order selection chose %q, want the duty-map seat's lane first (the configured order the nil path keeps)", seat.Lane)
	}
}
