package worker

import (
	"strings"
	"testing"
)

// tq5_laneorder_test.go — P3-TQ-5 acceptance (Spec S08.8 step 3: "resolves
// against the requester's duty maps", "subscription coverage binds every
// choice", "among flat-rate lanes, selection uses consumption pressure";
// gate record P3/gates/rework-sitting-gate.md B7 + §Answers "First Kimi K3
// CLI"). The execution duty carries an ORDERED preference of lanes — kimi-cli,
// then kimi, then anthropic — as worker DATA beside the duty map. Lane NAMES
// only: which model a lane fronts stays the lane document's fact and reaches
// the router through AlternateSeatsFor when the lane is commissioned (§63 D5,
// the `"k3"` constant scan). The first COVERED seat in that order wins when
// the gauge cannot separate the lanes; a preferred lane that is not covered is
// skipped and NAMED in the plain reason — never a gap, never an error.
//
// RED-BY-COMPILE at grounding: the identifiers LaneOrder (type + Router field)
// and DefaultLaneOrder do not exist yet.

func TestTQ5DefaultLaneOrderIsKimiCLIThenKimiThenAnthropic(t *testing.T) {
	order := DefaultLaneOrder()
	got := order[DutyExecution]
	want := []string{"kimi-cli", "kimi", "anthropic"}
	if len(got) != len(want) {
		t.Fatalf("execution lane order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("execution lane order = %v, want %v (B7: first Kimi K3 CLI, then K3 on kimi, then Opus 5 on anthropic)", got, want)
		}
	}
	// Planning and judging are anthropic-only by ratification (AlternateSeatsFor
	// is EXECUTION ONLY): no order entry exists for them, so their single seat
	// resolves exactly as before.
	for _, duty := range []string{DutyPlanning, DutyJudge} {
		if _, ok := order[duty]; ok {
			t.Errorf("duty %s carries a lane order — planning and judging stay on their anthropic seat (the untouched AlternateSeatsFor ratification)", duty)
		}
	}
}

func TestTQ5NothingCommissionedResolvesToOpus5AndNamesTheSkippedLanes(t *testing.T) {
	r := &Router{
		DutyMap:   DefaultDutyMap(),
		LaneOrder: DefaultLaneOrder(),
		Coverage:  Coverage{FlatRateLanes: []string{"anthropic"}},
	}
	seat, _, reason, gap, err := r.resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("the interim must never be an error: %v", err)
	}
	if gap != "" {
		t.Fatalf("the interim must never be a park (2.7 gap advice): %q", gap)
	}
	if seat.Model != "claude-opus-5" || seat.Lane != "anthropic" {
		t.Fatalf("seat = %+v, want claude-opus-5 on anthropic (the third choice, the only one available)", seat)
	}
	for _, skipped := range []string{"kimi-cli", "kimi"} {
		if !strings.Contains(reason, skipped) {
			t.Errorf("the reason does not name the skipped lane %q: %q", skipped, reason)
		}
	}
	if !strings.Contains(reason, "first choice") {
		t.Errorf("the reason does not say a first choice was skipped: %q", reason)
	}
	if strings.Contains(reason, "Not covered by a subscription") {
		t.Errorf("a resolved seat carries the 2.7 gap sentence: %q", reason)
	}
	// Requester copy carries no spec citations (CONVENTIONS §38, P3-GF13).
	for _, cite := range []string{"S08", "S07", "S06", "A13", "D5"} {
		if strings.Contains(reason, cite) {
			t.Errorf("the reason cites %q on a requester surface: %q", cite, reason)
		}
	}
}

func TestTQ5CommissionedKimiLanesWinInTheConfiguredOrder(t *testing.T) {
	// The alternates are handed in the WRONG order on purpose (kimi before
	// kimi-cli — the alphabetical order the lane documents load in): the order
	// that binds is the lane order's, never the commissioning read's.
	alts := AlternateSeatsFor(LaneSeat{Lane: "kimi", Model: "k3"}, LaneSeat{Lane: "kimi-cli", Model: "k3"})
	cases := []struct {
		name      string
		covered   []string
		wantLane  string
		wantModel string
		skipped   []string
	}{
		{"both kimi lanes commissioned", []string{"anthropic", "kimi", "kimi-cli"}, "kimi-cli", "k3", nil},
		{"only kimi commissioned", []string{"anthropic", "kimi"}, "kimi", "k3", []string{"kimi-cli"}},
		{"nothing commissioned", []string{"anthropic"}, "anthropic", "claude-opus-5", []string{"kimi-cli", "kimi"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Pressure nil: the gauge cannot separate the lanes, so the
			// configured order decides (the LN-4 D7 reading, unchanged).
			r := &Router{DutyMap: DefaultDutyMap(), Alternates: alts, LaneOrder: DefaultLaneOrder(),
				Coverage: Coverage{FlatRateLanes: c.covered}}
			seat, _, reason, gap, err := r.resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
			if err != nil || gap != "" {
				t.Fatalf("resolveSeat: err=%v gap=%q", err, gap)
			}
			if seat.Lane != c.wantLane || seat.Model != c.wantModel {
				t.Fatalf("seat = %s on %s, want %s on %s", seat.Model, seat.Lane, c.wantModel, c.wantLane)
			}
			for _, s := range c.skipped {
				if !strings.Contains(reason, s) {
					t.Errorf("the reason does not name the skipped lane %q: %q", s, reason)
				}
			}
		})
	}
}

// The S08.8 step-3 gauge is untouched: when it CAN separate two covered lanes
// it still decides, and the reason says so. The lane order is the tie-break
// and the fallback, exactly what "configured order" has meant since LN-2B.
func TestTQ5LaneOrderIsTheTieBreakNotAGaugeOverride(t *testing.T) {
	alts := AlternateSeatsFor(LaneSeat{Lane: "kimi-cli", Model: "k3"})
	r := &Router{DutyMap: DefaultDutyMap(), Alternates: alts, LaneOrder: DefaultLaneOrder(),
		Coverage: Coverage{FlatRateLanes: []string{"anthropic", "kimi-cli"}},
		Pressure: fixedPressure{"anthropic": 0.10, "kimi-cli": 0.90}}
	seat, _, reason, gap, err := r.resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat: err=%v gap=%q", err, gap)
	}
	if seat.Lane != "anthropic" {
		t.Errorf("with comparable readings the gauge must decide (anthropic has the most left), got %q", seat.Lane)
	}
	if !strings.Contains(reason, "how much of each is left") {
		t.Errorf("the reason does not say the gauge decided: %q", reason)
	}
}
