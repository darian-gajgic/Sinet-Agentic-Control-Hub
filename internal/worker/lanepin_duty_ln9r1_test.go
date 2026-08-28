package worker

// lanepin_duty_ln9r1_test.go — P3-LN-9 drain r1 F1/F2 (S08.8, S00.9 A13, S07.5).
//
// THE EVALUATOR'S TWO PROBES, committed. A pin naming a lane the owner holds
// flat-rate was REFUSED whenever the matched template's duty was not execution
// — and the refusal escaped the boundary unmapped, so a bad request answered as
// a platform defect. Two duty shapes reach it, and they get there differently:
//
//   · duty `planning`  — the duty map HAS that seat, so nothing degrades and
//                        the search looked in Alternates["planning"], which is
//                        empty by ratified design;
//   · duty `reviewer`  — an unknown duty, which selection already degrades onto
//                        the execution seat WITHOUT reassigning `duty`, so the
//                        search looked in Alternates["reviewer"], also empty.
//
// Both are the same root cause wearing different clothes: `AlternateSeatsFor`
// only ever populates DutyExecution, because a commissioned lane seats
// execution only. The pin names a LANE, so it is honored on that lane's
// execution seat and the substitution is stated.
//
// $0: pure over selection's inputs. No store, no database, no engine.

import (
	"context"
	"strings"
	"testing"
)

// ln9r1Router is the two-lane commissioned world with an execution-only
// alternate — the shape the composition root actually produces.
func ln9r1Router() *Router {
	return &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: AlternateSeatsFor(LaneSeat{Lane: "zai", Model: "glm-5.3"}),
		Coverage: Coverage{
			FlatRateLanes: []string{"anthropic", "zai"},
			LocalLane:     "local",
		},
	}
}

func TestLN9R1PinIsHonoredForANonExecutionDuty(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, duty string
		wantSaid   []string
		wantAbsent []string
	}{
		// The duty map HAS a planning seat, so nothing degrades and the old
		// search simply found nothing on that duty. Riding the lane's
		// execution seat IS a substitution here, so the reason must say so.
		{"a template declaring the planning duty", DutyPlanning,
			[]string{"instead of comparing", "only set up for doing the work",
				"measured against the quality bars", plainDuty(DutyPlanning)}, nil},
		// An unknown duty degrades onto the execution SEAT while `duty` keeps
		// its original value — the second door to the same refusal. Here the
		// substitution is the degrade selection ALREADY records in its own
		// sentence, so the pin adds no second claim: the effective duty is
		// execution before the pin is consulted at all.
		{"a template declaring an unknown duty", "reviewer",
			[]string{"instead of comparing", "No model is assigned to work of this kind (reviewer)"},
			// And NOT the execution-seat substitution clause. Selection already
			// degraded this duty onto the execution seat and said so in its own
			// sentence, so the effective duty IS execution by the time the pin
			// is consulted. Saying it a second time would describe two
			// substitutions where one happened — which is what tracking the
			// EFFECTIVE duty (rather than re-deriving from the template's) buys.
			[]string{"only set up for doing the work"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ln9r1Router()
			seat, _, reason, gap, err := r.resolveSeat(ctx,
				RouteQuery{PinnedLane: "zai"}, ExecutionProfile{Duty: tc.duty})
			if err != nil {
				t.Fatalf("a pin to a COVERED lane was refused for duty %q: %v\n\n"+
					"Coverage holds the lane; what was missing is a seat row for THIS duty, which is a fact "+
					"about the platform's own bookkeeping and not a coverage verdict (r1 F1)", tc.duty, err)
			}
			if gap != "" {
				t.Fatalf("duty %q produced the 2.7 subscription-gap leg on a covered lane: %q", tc.duty, gap)
			}
			if seat.Lane != "zai" {
				t.Fatalf("duty %q seated lane %q, want zai — the pin names a LANE", tc.duty, seat.Lane)
			}
			if seat.Model != "glm-5.3" {
				t.Errorf("duty %q seated model %q, want the lane document's own execution model", tc.duty, seat.Model)
			}
			if seat.WindowTokens == 0 {
				t.Errorf("duty %q seated no window", tc.duty)
			}
			// HONEST REASON: the substitution is stated, not smuggled. A
			// planning-duty worker running on a lane that seats execution only
			// is a fact its reader is entitled to.
			for _, want := range tc.wantSaid {
				if !strings.Contains(reason, want) {
					t.Errorf("duty %q: the reason does not say %q: %q", tc.duty, want, reason)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(reason, absent) {
					t.Errorf("duty %q: the reason says %q, which describes a substitution that did NOT happen "+
						"a second time — selection had already degraded this duty onto the execution seat: %q",
						tc.duty, absent, reason)
				}
			}
			// Whichever door it came through, the reader is told the duty is
			// not running on its own duty-map seat. A pin that quietly moved a
			// planning-duty worker onto another lane's model would be the
			// silent substitution this packet exists to end.
			if !strings.Contains(reason, tc.duty) {
				t.Errorf("duty %q: the reason never names the duty at all: %q", tc.duty, reason)
			}
		})
	}
}

// The control that keeps the fix from becoming "honor everything": a pin to a
// covered lane that NOTHING on this platform seats still refuses — and the
// refusal now names the true cause. The old message blamed the duty ("this duty
// resolves to no model there") and sent a reader to the template, when the
// missing thing is a commissioned execution seat.
func TestLN9R1RefusalNamesTheTrueCauseNotTheDuty(t *testing.T) {
	r := &Router{
		DutyMap: DefaultDutyMap(),
		// `zai` is covered and NO lane document contributed a seat for it.
		Coverage: Coverage{FlatRateLanes: []string{"anthropic", "zai"}, LocalLane: "local"},
	}
	for _, duty := range []string{DutyExecution, DutyPlanning, "reviewer"} {
		_, _, _, _, err := r.resolveSeat(context.Background(),
			RouteQuery{PinnedLane: "zai"}, ExecutionProfile{Duty: duty})
		if err == nil {
			t.Fatalf("duty %q: a pin to a lane this platform seats nothing on was honored — there is no model "+
				"to run there, and riding another lane's seat is the substitution the pin exists to end", duty)
		}
		if !strings.Contains(err.Error(), "no model on that lane has been set up here") {
			t.Errorf("duty %q: the refusal does not name the true cause: %v", duty, err)
		}
		if strings.Contains(err.Error(), "this duty resolves to no model there") {
			t.Errorf("duty %q: the refusal still blames the duty, which sends a reader to the template when "+
				"the missing thing is a commissioned seat: %v", duty, err)
		}
	}
}

// F6 · the covered-lane COUNT in the pin reason agrees with the names beside
// it. The composition root prepends the configured lane to the commissioned
// set, so a commissioned lane that IS the configured one appears twice.
func TestLN9R1PinReasonCountsDistinctLanes(t *testing.T) {
	r := ln9r1Router()
	r.Coverage.FlatRateLanes = []string{"anthropic", "zai", "zai"}

	_, _, reason, _, err := r.resolveSeat(context.Background(),
		RouteQuery{PinnedLane: "zai"}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat: %v", err)
	}
	if strings.Contains(reason, "across the 3 covered") {
		t.Errorf("the reason counted a duplicated lane twice: %q", reason)
	}
	if !strings.Contains(reason, "across the 2 covered") {
		t.Errorf("the reason does not report 2 distinct covered lanes: %q", reason)
	}
	if got, want := coveredLaneCount(r.Coverage), 2; got != want {
		t.Errorf("coveredLaneCount = %d, want %d", got, want)
	}
}
