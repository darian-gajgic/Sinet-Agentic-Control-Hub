package worker

import (
	"strings"
	"testing"
)

// tq5_drain_r1_internal_test.go — P3-TQ-5 drain r1, F2 and F4 (Spec S08.8
// step 3 + accountability). Both pin behaviour the packet's own acceptance
// tests left free: a mutation could satisfy every one of them and still tell a
// requester something false.

// F2 · A lane the household DOES hold is never called unavailable. The
// skipped-lane sentence exists to explain a preference that could not be
// honored; a covered lane the gauge simply passed over was honored and lost on
// the measurement, which the gauge's own sentence says. Naming it as "not set
// up on this platform yet" would be a false statement about the person's own
// subscriptions.
func TestTQ5AGaugeChoiceNeverCallsACoveredLaneUnavailable(t *testing.T) {
	r := &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: AlternateSeatsFor(LaneSeat{Lane: "kimi-cli", Model: "k3"}),
		LaneOrder:  DefaultLaneOrder(),
		Coverage:   Coverage{FlatRateLanes: []string{"anthropic", "kimi-cli"}},
		// Comparable readings, and the always-configured lane has the most
		// left: the gauge decides, against the configured order.
		Pressure: fixedPressure{"anthropic": 0.10, "kimi-cli": 0.90},
	}
	seat, _, reason, gap, err := r.resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat: err=%v gap=%q", err, gap)
	}
	if seat.Lane != "anthropic" {
		t.Fatalf("the gauge must decide here, got lane %q", seat.Lane)
	}
	if !strings.Contains(reason, "how much of each is left") {
		t.Errorf("the reason does not say the gauge decided: %q", reason)
	}
	// kimi-cli is COMMISSIONED and covered — it lost on the gauge, not for
	// want of a subscription.
	if strings.Contains(reason, "the kimi-cli lane") {
		t.Errorf("a covered lane the gauge passed over is named as a skipped one: %q", reason)
	}
	// kimi is genuinely not held, sits ahead of the winner, and IS named —
	// so the assertion above is discriminating and not just an empty sentence.
	if !strings.Contains(reason, "the kimi lane") {
		t.Errorf("the uncovered preferred lane is not named: %q", reason)
	}
}

// F4 · A duty nobody assigned degrades onto the EXECUTION seat, so it rides
// that seat's alternates and lane order too. Resolving it under its own
// unknown name found neither (both maps are keyed by duty class), which left
// an unknown duty on the always-configured lane while the very same router
// sent the execution duty to a commissioned one — two answers to "which lane
// does this platform use" on one router.
func TestTQ5UnknownDutyDegradeRidesTheExecutionLaneOrder(t *testing.T) {
	newRouter := func() *Router {
		return &Router{
			DutyMap:    DefaultDutyMap(),
			Alternates: AlternateSeatsFor(LaneSeat{Lane: "kimi-cli", Model: "k3"}),
			LaneOrder:  DefaultLaneOrder(),
			Coverage:   Coverage{FlatRateLanes: []string{"anthropic", "kimi-cli"}},
		}
	}
	known, _, _, _, err := newRouter().resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("execution duty: %v", err)
	}
	if known.Lane != "kimi-cli" {
		t.Fatalf("the execution duty resolved to %q — the comparison below would prove nothing", known.Lane)
	}

	seat, _, reason, gap, err := newRouter().resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: "archaeology"})
	if err != nil || gap != "" {
		t.Fatalf("an unknown duty must degrade, never fail: err=%v gap=%q", err, gap)
	}
	if seat.Lane != known.Lane || seat.Model != known.Model {
		t.Errorf("the degraded duty resolved to %s on %s, want the execution seat's own answer %s on %s",
			seat.Model, seat.Lane, known.Model, known.Lane)
	}
	// The degrade is still STATED: riding the execution seat's order does not
	// make the substitution silent.
	if !strings.Contains(reason, "No model is assigned to work of this kind") {
		t.Errorf("the degrade is not recorded in the reason: %q", reason)
	}
}
