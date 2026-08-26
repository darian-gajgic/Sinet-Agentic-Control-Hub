package worker

// lanepin_ln9_test.go — P3-LN-9 (S08.8, S00.9 A13, S12.1): the per-task lane
// pin as SELECTION sees it.
//
// In-package because resolveSeat and the Coverage predicate are unexported,
// following zai_ln2b_test.go and routing_local_test.go.
//
// The through-line is the inversion the packet exists to make: routing
// DEGRADES when the platform chose a lane it cannot use, and REFUSES when a
// person did. Every guard below is written so that reverting the arm it holds
// turns it red — a pin that quietly landed on another lane is exactly the
// silent substitution the operator's ordered head-to-head cannot survive.
//
// $0: no store, no database, no engine. Selection is pure over its inputs.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// explodingPressure fails the test if the consumption gauge is consulted at
// all. It is the guard for "chooseFlatLane is not consulted when a pin binds"
// (brief §7): asserting the CHOSEN lane alone cannot see the difference
// between a pin that replaced the comparison and a pin that happened to agree
// with it.
type explodingPressure struct{ t *testing.T }

func (p explodingPressure) Pressure(_ context.Context, _, lane string) (LanePressure, error) {
	p.t.Errorf("the consumption gauge was read for lane %q while a lane pin was bound — a bound pin REPLACES "+
		"the pressure comparison (S08.8 step 3 [S00.9 A13]), it does not enter it", lane)
	return LanePressure{}, nil
}

// twoLaneCoverage is the world the operator's ordered comparison needs: the
// configured lane plus a commissioned one, both held flat-rate.
func twoLaneCoverage() Coverage {
	return Coverage{
		FlatRateLanes: []string{"anthropic", "zai"},
		LocalLane:     "local",
	}
}

func twoLaneRouter(t *testing.T) *Router {
	return &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: AlternateSeatsFor(LaneSeat{Lane: "zai", Model: "glm-5.3"}),
		Coverage:   twoLaneCoverage(),
		Pressure:   explodingPressure{t},
	}
}

// ── The predicate: one rule, and the two refusals say different things ──────

func TestLN9LanePinRefusalIsOnePredicateWithDistinctDetail(t *testing.T) {
	cov := twoLaneCoverage()

	for _, lane := range []string{"anthropic", "zai"} {
		if got := LanePinRefusal(cov, lane); got != "" {
			t.Errorf("LanePinRefusal(%q) = %q, want no refusal — the lane is held flat-rate", lane, got)
		}
	}
	if got := LanePinRefusal(cov, ""); got != "" {
		t.Errorf("LanePinRefusal(\"\") = %q — an absent pin is not a refused pin, and an unpinned task must be "+
			"byte-identical to today", got)
	}

	// The local ENGINE lane refuses IN ITS OWN WORDS. Borrowing the 2.7
	// subscription-gap sentence would tell an operator to buy a subscription
	// they already hold; the absent thing is a commissioned local provider
	// entry, not a subscription (S12.1 class (a)).
	local := LanePinRefusal(cov, "local")
	if local == "" {
		t.Fatal("a pin to the local engine lane was accepted — it carries no v0 consumer, so honoring it could " +
			"only mean riding the paid seat (brief R8(b))")
	}
	for _, want := range []string{"local", "S12.1 class (a)", "no local provider entry"} {
		if !strings.Contains(local, want) {
			t.Errorf("the local refusal does not say %q: %q", want, local)
		}
	}
	if strings.Contains(local, "Subscription gap") || strings.Contains(local, "2.7") {
		t.Errorf("the local refusal borrowed the subscription-gap wording, which tells an operator to buy "+
			"something they already hold: %q", local)
	}

	// An uncovered lane refuses with the coverage sentence, and a different
	// one — one code, distinct DETAIL (OQ-5).
	uncovered := LanePinRefusal(cov, "kimi-cli")
	if uncovered == "" {
		t.Fatal("a pin to an uncovered lane was accepted — subscription coverage binds every choice (S08.8 step 3)")
	}
	if uncovered == local {
		t.Error("the uncovered-lane refusal and the local-lane refusal are the same sentence — they are different " +
			"situations with different remedies (OQ-5: one code, distinct detail)")
	}
	if !strings.Contains(uncovered, "flat-rate") {
		t.Errorf("the uncovered refusal does not name coverage as the reason: %q", uncovered)
	}
	// Both name what IS pinnable, the way the plan-budget verb names the
	// windows that exist rather than only saying no.
	for name, msg := range map[string]string{"local": local, "uncovered": uncovered} {
		if !strings.Contains(msg, `"anthropic"`) || !strings.Contains(msg, `"zai"`) {
			t.Errorf("the %s refusal does not enumerate the pinnable lanes: %q", name, msg)
		}
	}
}

func TestLN9PinnableLanesCarriesTheVerdictAcrossTheSeam(t *testing.T) {
	opts := PinnableLanes(twoLaneCoverage())

	byLane := map[string]PinnableLane{}
	for _, o := range opts {
		if _, dup := byLane[o.Lane]; dup {
			t.Errorf("lane %q appears twice in the pinnable set", o.Lane)
		}
		byLane[o.Lane] = o
	}
	for _, lane := range []string{"anthropic", "zai"} {
		o, ok := byLane[lane]
		if !ok || !o.Pinnable {
			t.Errorf("covered lane %q is not offered as pinnable: %+v", lane, o)
		}
		if o.NotPinnable != "" {
			t.Errorf("pinnable lane %q carries a refusal: %q", lane, o.NotPinnable)
		}
	}
	// The local lane rides the set as a KNOWN-and-refused row, so the boundary
	// can answer with its own words instead of falling through to the generic
	// "not one this platform can pin to".
	l, ok := byLane["local"]
	if !ok {
		t.Fatal("the local engine lane is absent from the pinnable set entirely — the boundary then has no way " +
			"to refuse it in its own words (brief R8(b))")
	}
	if l.Pinnable {
		t.Error("the local engine lane is offered as PINNABLE")
	}
	if l.NotPinnable != LanePinRefusal(twoLaneCoverage(), "local") {
		t.Error("the carried refusal is not the predicate's own sentence — the verdict must be computed once and " +
			"carried, never re-derived (§66 D1)")
	}

	// Nothing commissioned: exactly the configured lane, and the local row.
	bare := PinnableLanes(Coverage{FlatRateLanes: []string{"anthropic"}, LocalLane: "local"})
	if len(bare) != 2 || bare[0].Lane != "anthropic" || !bare[0].Pinnable {
		t.Errorf("the single-lane world offers %+v, want just the configured lane plus the refused local row", bare)
	}
}

// ── The pin binds, and the pressure comparison is REPLACED ─────────────────

func TestLN9BoundPinSkipsTheFlatLaneComparison(t *testing.T) {
	ctx := context.Background()
	r := twoLaneRouter(t)

	// explodingPressure fails the test if it is read at all, so this asserts
	// the skip structurally rather than by inspecting the outcome.
	seat, _, reason, gap, err := r.resolveSeat(ctx,
		RouteQuery{PinnedLane: "zai"}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat(pin=zai): err=%v gap=%q", err, gap)
	}
	if seat.Lane != "zai" {
		t.Fatalf("the pin named zai and selection seated %q", seat.Lane)
	}
	if seat.Model == "" || seat.WindowTokens == 0 {
		t.Errorf("the pinned seat carries no model or window: %+v", seat)
	}
	// The reason must NAME the lane and say the comparison was REPLACED —
	// "skipped" would leave a reader thinking the gauge still had a say.
	if !strings.Contains(reason, `"zai"`) {
		t.Errorf("the reason does not name the pinned lane: %q", reason)
	}
	if !strings.Contains(reason, "REPLACED") || !strings.Contains(reason, "consumption-pressure") {
		t.Errorf("the reason does not say the pressure comparison was replaced: %q", reason)
	}
	if !strings.Contains(reason, "pinned on this task") {
		t.Errorf("the reason does not say the lane was pinned on this task: %q", reason)
	}
	// D5: the pin REPLACES the comparison, it does not add a currency to it.
	// The scan is for a money FIGURE or a price word, not for the standing
	// "never dollars — D5" disclaimer the flat-lane reasons already carry —
	// that sentence asserts the invariant rather than breaching it.
	for _, banned := range []string{"$", "usd", "price", "cost"} {
		if strings.Contains(strings.ToLower(reason), banned) {
			t.Errorf("the pin reason names %q — a pin is a person's named choice and carries no money (D5): %q",
				banned, reason)
		}
	}
	if !strings.Contains(reason, "never dollars — D5") {
		t.Errorf("the pin reason drops the D5 disclaimer the flat-lane reasons carry: %q", reason)
	}
}

// The pin that names the duty seat's OWN lane still binds, and still replaces
// the comparison. Without this the guard above could pass on a router that
// only ever honors ALTERNATE lanes.
func TestLN9PinOnTheConfiguredLaneAlsoBinds(t *testing.T) {
	r := twoLaneRouter(t)
	seat, _, reason, gap, err := r.resolveSeat(context.Background(),
		RouteQuery{PinnedLane: "anthropic"}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat(pin=anthropic): err=%v gap=%q", err, gap)
	}
	if seat.Lane != "anthropic" {
		t.Fatalf("seated lane %q, want anthropic", seat.Lane)
	}
	if !strings.Contains(reason, "REPLACED") {
		t.Errorf("a pin on the configured lane did not replace the comparison: %q", reason)
	}
}

// ── Layer 3: a planted pin cannot steer dispatch ───────────────────────────

func TestLN9PlantedPinRefusesAtSelection(t *testing.T) {
	r := twoLaneRouter(t)
	for _, pin := range []string{"kimi-cli", "local", "no-such-lane"} {
		t.Run(pin, func(t *testing.T) {
			seat, _, _, gap, err := r.resolveSeat(context.Background(),
				RouteQuery{PinnedLane: pin}, ExecutionProfile{Duty: DutyExecution})
			if err == nil {
				t.Fatalf("selection accepted a planted pin to %q and seated %+v (gap=%q) — a pin arriving by any "+
					"route other than the boundary must still not steer dispatch (brief R3 layer 3)", pin, seat, gap)
			}
			if !errors.Is(err, ErrLanePinUnhonorable) {
				t.Fatalf("err = %v, want ErrLanePinUnhonorable so callers can tell a refused pin from a broken router", err)
			}
			if !strings.Contains(err.Error(), pin) {
				t.Errorf("the refusal does not name the pin it refused: %v", err)
			}
			if seat.Lane != "" {
				t.Errorf("a refused pin still handed back a seat on lane %q — selection must route NOWHERE, "+
					"never quietly onto another lane", seat.Lane)
			}
		})
	}
}

// A lane that IS covered but on which this duty has no seat is the third
// detail sentence: refused, because riding the duty's own seat would silently
// give the requester a lane they did not ask for.
func TestLN9CoveredLaneWithNoSeatForTheDutyRefuses(t *testing.T) {
	r := &Router{
		DutyMap: DefaultDutyMap(),
		// zai is covered, and no alternate seat is registered for it.
		Coverage: Coverage{FlatRateLanes: []string{"anthropic", "zai"}, LocalLane: "local"},
	}
	_, _, _, _, err := r.resolveSeat(context.Background(),
		RouteQuery{PinnedLane: "zai"}, ExecutionProfile{Duty: DutyExecution})
	if err == nil {
		t.Fatal("a pin to a covered lane with no seat for the duty was honored — there is no model to seat there, " +
			"so honoring it could only mean riding a different lane")
	}
	if !errors.Is(err, ErrLanePinUnhonorable) {
		t.Fatalf("err = %v, want ErrLanePinUnhonorable", err)
	}
	// The message names the TRUE cause. r1 F1 corrected it: it used to blame
	// the duty ("this duty resolves to no model there"), which sent a reader to
	// the template when the missing thing is a commissioned execution seat.
	if !strings.Contains(err.Error(), "no execution seat on it") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
	if strings.Contains(err.Error(), "this duty resolves to no model there") {
		t.Errorf("the refusal still blames the duty: %v", err)
	}
}

// ── OQ-4: the task pin wins over a template model pin, and says so ─────────

func TestLN9TaskPinSupersedesTheTemplateModelPin(t *testing.T) {
	r := twoLaneRouter(t)
	profile := ExecutionProfile{
		Duty:           DutyExecution,
		ModelPin:       "claude-opus-4-8",
		ModelPinReason: "the golden set was calibrated on this model",
	}

	// Conflict: the template pins a model that lives on the duty seat's lane,
	// and the task pins a DIFFERENT lane. The person's act on this task wins.
	seat, _, reason, gap, err := r.resolveSeat(context.Background(),
		RouteQuery{PinnedLane: "zai"}, profile)
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat(pin=zai, model pin): err=%v gap=%q", err, gap)
	}
	if seat.Lane != "zai" {
		t.Fatalf("seated lane %q — the TASK pin wins over a standing template default (OQ-4)", seat.Lane)
	}
	if seat.Model == "claude-opus-4-8" {
		t.Error("the template's pinned model was carried onto the pinned lane — the lane's seat is the lane " +
			"document's own, and moving a model between lanes invents a fact")
	}
	if !strings.Contains(reason, "SUPERSEDES") {
		t.Errorf("the reason does not record the supersession: %q", reason)
	}
	if !strings.Contains(reason, profile.ModelPinReason) {
		t.Errorf("the reason does not quote the template's OWN recorded reason, so the person who set it cannot "+
			"see why it was overridden: %q", reason)
	}

	// No conflict: the pin names the duty seat's own lane, so the template's
	// model pin still stands and is not reported as superseded.
	seat, _, reason, gap, err = r.resolveSeat(context.Background(),
		RouteQuery{PinnedLane: "anthropic"}, profile)
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat(pin=anthropic, model pin): err=%v gap=%q", err, gap)
	}
	if seat.Model != "claude-opus-4-8" || seat.Lane != "anthropic" {
		t.Errorf("seat = %+v, want the template's pinned model on its own lane — the two pins do not conflict here", seat)
	}
	if strings.Contains(reason, "SUPERSEDES") {
		t.Errorf("a non-conflicting model pin was reported as superseded: %q", reason)
	}
}

// ── The pool note: a pin between two lanes on one allowance says so ────────

func TestLN9PinNoteRidesTheReasonForAPooledLane(t *testing.T) {
	cov := twoLaneCoverage()
	cov.FlatRateLanes = []string{"anthropic", "kimi", "kimi-cli"}
	const note = "Note: lanes kimi and kimi-cli draw one membership allowance."
	cov.PinNotes = map[string]string{"kimi-cli": note}

	r := &Router{
		DutyMap: DefaultDutyMap(),
		Alternates: AlternateSeatsFor(
			LaneSeat{Lane: "kimi", Model: "k-model"},
			LaneSeat{Lane: "kimi-cli", Model: "k-model"},
		),
		Coverage: cov,
		Pressure: explodingPressure{t},
	}
	_, _, reason, _, err := r.resolveSeat(context.Background(),
		RouteQuery{PinnedLane: "kimi-cli"}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat: %v", err)
	}
	if !strings.Contains(reason, note) {
		t.Errorf("the pooled-allowance note did not reach the reason — an operator pinning between two lanes on "+
			"ONE membership would read the pin as buying a separate quota: %q", reason)
	}
	// And an unpooled lane says nothing extra.
	_, _, plain, _, err := r.resolveSeat(context.Background(),
		RouteQuery{PinnedLane: "kimi"}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat(kimi): %v", err)
	}
	if strings.Contains(plain, note) {
		t.Errorf("a lane with no note of its own borrowed another lane's: %q", plain)
	}
}

// ── D5 stays structural with the new members present ──────────────────────

func TestLN9NewSelectionInputsCarryNoMoney(t *testing.T) {
	// The reflective scan in zai_ln2b_test.go covers the input TYPES; this is
	// the packet's own restatement over the members it added, so a future
	// reader sees the claim was checked for THIS diff and not merely inherited.
	for typ, members := range map[string][]string{
		"Coverage":   {"LocalLane", "PinNotes"},
		"RouteQuery": {"PinnedLane"},
		"Decision":   {"LanePin"},
	} {
		for _, m := range members {
			lower := strings.ToLower(m)
			for _, needle := range []string{"price", "cost", "usd", "dollar", "spend"} {
				if strings.Contains(lower, needle) {
					t.Errorf("%s.%s is money-shaped — dollar-based routing between flat-rate lanes is a D5 "+
						"violation and is NEVER done (S10.2)", typ, m)
				}
			}
		}
	}
	body := readWorkerSource(t, "routing.go")
	for _, banned := range []string{"PricedUSD", "CostUSD", "PriceTable", "metering."} {
		if strings.Contains(body, banned) {
			t.Errorf("routing.go names %q after the lane pin landed — no price, cost or dollar figure enters "+
				"this package's selection inputs (D5)", banned)
		}
	}
}
