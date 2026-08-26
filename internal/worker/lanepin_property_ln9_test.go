package worker_test

// lanepin_property_ln9_test.go — P3-LN-9 §9.3/§9.5 (S08.8, S00.9 A13, D5).
//
// R10 as a PROPERTY, not as an example — and stated precisely, because the
// first cut overclaimed it (r1 F13).
//
// WHAT IT PROVES: over arbitrary draws, an unpinned Decision is INERT to
// everything this packet added — identical field for field between a world
// carrying the new Coverage members and one without them, with `LanePin` empty
// and no pin vocabulary anywhere in the reason. WHAT IT DOES NOT PROVE: that
// the result equals a pre-packet binary, since both arms run post-packet code.
// The pre-vs-post half is carried by the two assertions below it — every new
// arm in selection is gated on `q.PinnedLane`, and `lane_pin` is absent from
// an unpinned Decision's JSON — plus the held-out mutation, which is what stops
// this being a property that can only pass. §66 D8 is the record of exactly
// this property passing while comparing too few fields, so the comparison here
// is over the WHOLE Decision.
//
// $0: a real store over a temp database, a fake tie-break, no engine, no
// provider, no network.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ln9Pressure is a deterministic gauge: it makes both lanes comparable so the
// unpinned path exercises the branch a pin replaces.
type ln9Pressure map[string]float64

func (p ln9Pressure) Pressure(_ context.Context, _, lane string) (worker.LanePressure, error) {
	v, ok := p[lane]
	if !ok {
		return worker.LanePressure{Applicable: false}, nil
	}
	return worker.LanePressure{Ratio: v, Applicable: true, Unit: "test units"}, nil
}

// ln9Router composes the router the property draws over. `pinAware` decides
// whether the NEW Coverage members are present at all, which is what makes the
// comparison a statement about this packet rather than about itself.
func ln9Router(f *fix, pinAware bool, lanes []string, pressure ln9Pressure) *worker.Router {
	cov := worker.Coverage{FlatRateLanes: lanes}
	if pinAware {
		cov.LocalLane = "local"
		cov.PinNotes = map[string]string{"zai": "Note: a pooled allowance."}
	}
	return &worker.Router{
		Store:      f.store,
		DutyMap:    worker.DefaultDutyMap(),
		Alternates: worker.AlternateSeatsFor(worker.LaneSeat{Lane: "zai", Model: "glm-5.3"}),
		Coverage:   cov,
		Pressure:   pressure,
	}
}

// TestLN9UnpinnedIsByteIdentical — brief R10 / §9.3.
func TestLN9UnpinnedIsByteIdentical(t *testing.T) {
	// TWO worlds, one call each per draw. Route WRITES gap records, so routing
	// the same query twice through one store makes the second call see a
	// recurrence the first did not — a difference produced by the test rather
	// than by the diff, and one that would have been read as a real one.
	before, after := newFix(t), newFix(t)
	for _, f := range []*fix{before, after} {
		f.user("alice", "member")
		approveReviewer(t, f, "alice")
	}
	ctx := context.Background()

	families := []string{"read-analyze", "write-produce", "build-change", "chore", "generic"}
	domains := []string{"software", "chore", "", "writing"}
	laneShapes := [][]string{
		{"anthropic"},
		{"anthropic", "zai"},
		{"zai"},
	}
	texts := []string{
		"Please review my code for defects",
		"Draft the release notes for this change",
		"Rebuild the notes index",
	}

	// One fixed seed, so a failure is reproducible from the transcript alone.
	rng := rand.New(rand.NewSource(0x1e9))
	const draws = 240
	for i := 0; i < draws; i++ {
		lanes := laneShapes[rng.Intn(len(laneShapes))]
		pressure := ln9Pressure{}
		for _, l := range lanes {
			if rng.Intn(4) > 0 {
				pressure[l] = rng.Float64()
			}
		}
		q := worker.RouteQuery{
			Requester:  "alice",
			TaskID:     fmt.Sprintf("t-ln9-%03d", i),
			TaskText:   texts[rng.Intn(len(texts))],
			Family:     families[rng.Intn(len(families))],
			Domain:     domains[rng.Intn(len(domains))],
			Research:   rng.Intn(2) == 0,
			Writes:     rng.Intn(2) == 0,
			Mechanical: rng.Intn(4) == 0,
		}
		if rng.Intn(3) == 0 {
			q.Classes = []string{"C1"}
			q.Tools = []string{"Read"}
		}

		was, errBefore := ln9Router(before, false, lanes, pressure).Route(ctx, q)
		now, errAfter := ln9Router(after, true, lanes, pressure).Route(ctx, q)

		if (errBefore == nil) != (errAfter == nil) {
			t.Fatalf("draw %d: error disagreement — before=%v after=%v", i, errBefore, errAfter)
		}
		if errAfter != nil {
			continue
		}
		if now.LanePin != "" {
			t.Fatalf("draw %d: an UNPINNED task carries LanePin=%q — the absent case must serve exactly the "+
				"bytes it served before (R10)", i, now.LanePin)
		}
		// The two stores mint their own random template/version ids, so those
		// are the one class of field that CANNOT match across independent
		// worlds. They are compared for PRESENCE and then normalized away, so
		// nothing else hides behind the exemption.
		if (was.TemplateID == "") != (now.TemplateID == "") ||
			(was.VersionID == "") != (now.VersionID == "") ||
			len(was.Candidates) != len(now.Candidates) {
			t.Fatalf("draw %d: worker identity presence disagrees — before=%+v after=%+v", i, was, now)
		}
		normalizeIDs(&was)
		normalizeIDs(&now)

		if now.PlainReason != was.PlainReason {
			t.Fatalf("draw %d: PlainReason moved on an unpinned task.\nbefore: %q\nafter:  %q",
				i, was.PlainReason, now.PlainReason)
		}
		if !reflect.DeepEqual(was, now) {
			t.Fatalf("draw %d: the Decision moved on an unpinned task (§66 D8: compare ALL the fields).\n"+
				"before: %+v\nafter:  %+v", i, was, now)
		}
		// The reason must carry no pin vocabulary at all: a sentence that
		// mentions a pin on a task that has none is its own lie.
		for _, banned := range []string{"pinned on this task", "REPLACED", "SUPERSEDES"} {
			if strings.Contains(now.PlainReason, banned) {
				t.Fatalf("draw %d: an unpinned decision's reason says %q: %q", i, banned, now.PlainReason)
			}
		}
	}
}

// normalizeIDs blanks the store-minted identity fields. They are random per
// store, so two independent worlds never agree on them; everything the
// property is actually about — cause, seat, reason, signals, gap bookkeeping —
// survives untouched.
func normalizeIDs(d *worker.Decision) {
	d.TemplateID, d.VersionID = "", ""
	for i := range d.Candidates {
		d.Candidates[i].TemplateID, d.Candidates[i].VersionID = "", ""
	}
}

// TestLN9UnpinnedPropertyCanFail is the HELD-OUT MUTATION §9.3 requires. It
// fabricates a pin on an otherwise identical query and asserts the very
// comparison above goes red. A property that can only pass is not a property.
func TestLN9UnpinnedPropertyCanFail(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	approveReviewer(t, f, "alice")
	ctx := context.Background()

	lanes := []string{"anthropic", "zai"}
	pressure := ln9Pressure{"anthropic": 0.9, "zai": 0.1}
	base := worker.RouteQuery{
		Requester: "alice", TaskID: "t-ln9-mutation",
		TaskText: "Please review my code for defects",
		Family:   "read-analyze", Domain: "software",
		Classes: []string{"C1"}, Tools: []string{"Read"},
	}
	r := ln9Router(f, true, lanes, pressure)

	unpinned, err := r.Route(ctx, base)
	if err != nil {
		t.Fatalf("Route(unpinned): %v", err)
	}
	pinned := base
	pinned.PinnedLane = "anthropic"
	pinned.TaskID = "t-ln9-mutation-pinned"
	withPin, err := r.Route(ctx, pinned)
	if err != nil {
		t.Fatalf("Route(pinned): %v", err)
	}

	if withPin.LanePin != "anthropic" {
		t.Errorf("Decision.LanePin = %q, want the pinned lane — this is the structured member the picker binds to",
			withPin.LanePin)
	}
	if reflect.DeepEqual(unpinned, withPin) {
		t.Fatal("a pinned decision is field-for-field identical to an unpinned one — the byte-identity property " +
			"above would pass whatever the pin did, which is the §66 D8 failure exactly")
	}
	if withPin.PlainReason == unpinned.PlainReason {
		t.Error("the plain reason did not move under a pin — the pin reaches the approval card, the Workforce row " +
			"and routing.decided through this string alone")
	}
	if !strings.Contains(withPin.PlainReason, "REPLACED") {
		t.Errorf("the pinned reason does not say the pressure comparison was replaced: %q", withPin.PlainReason)
	}
	// The other direction of the pin's own claim: the unpinned twin, in this
	// world, is chosen by the GAUGE — so the pin genuinely replaced something.
	if !strings.Contains(unpinned.PlainReason, "Chosen among") {
		t.Errorf("the unpinned control did not go through the pressure comparison, so this test does not prove "+
			"the pin replaced it: %q", unpinned.PlainReason)
	}
	if unpinned.Lane != "zai" || withPin.Lane != "anthropic" {
		t.Errorf("unpinned lane = %q (want zai, the less consumed), pinned lane = %q (want anthropic, the pin)",
			unpinned.Lane, withPin.Lane)
	}
}

// TestLN9PlantedPinCannotSteerDispatch — §9.5, at Route rather than at the
// resolver: a query carrying a pin to an uncovered lane, handed straight to
// selection with the boundary bypassed entirely.
func TestLN9PlantedPinCannotSteerDispatch(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	approveReviewer(t, f, "alice")

	r := ln9Router(f, true, []string{"anthropic"}, ln9Pressure{"anthropic": 0.2})
	d, err := r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: "t-ln9-planted",
		TaskText: "Please review my code for defects",
		Family:   "read-analyze", Domain: "software",
		Classes: []string{"C1"}, Tools: []string{"Read"},
		PinnedLane: "kimi-cli",
	})
	if err == nil {
		t.Fatalf("Route accepted a planted pin to an uncovered lane and answered %+v — selection must refuse, "+
			"never silently pick another lane (brief R3 layer 3)", d)
	}
	if !errors.Is(err, worker.ErrLanePinUnhonorable) {
		t.Fatalf("err = %v, want ErrLanePinUnhonorable", err)
	}
	if !strings.Contains(err.Error(), "kimi-cli") {
		t.Errorf("the refusal does not name the pin: %v", err)
	}
	if d.Lane != "" || d.Model != "" {
		t.Errorf("a refused pin still produced a routable decision: %+v", d)
	}
}

// TestLN9R1UnpinnedDecisionSerializesWithoutTheMember is the evaluator's own
// probe, committed (r1 F13). `omitempty` is a claim about BYTES, and the
// property above compares Go values — so this is the half that actually holds
// the wire contract: an unpinned task's routing.decided payload must not carry
// the key at all, or every consumer sees a member the platform said it would
// only send when it meant something.
func TestLN9R1UnpinnedDecisionSerializesWithoutTheMember(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	approveReviewer(t, f, "alice")

	r := ln9Router(f, true, []string{"anthropic", "zai"}, ln9Pressure{"anthropic": 0.2, "zai": 0.4})
	q := worker.RouteQuery{
		Requester: "alice", TaskID: "t-ln9r1-wire",
		TaskText: "Please review my code for defects",
		Family:   "read-analyze", Domain: "software",
		Classes: []string{"C1"}, Tools: []string{"Read"},
	}
	unpinned, err := r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	raw, err := json.Marshal(unpinned)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "lane_pin") {
		t.Errorf("an UNPINNED decision serializes the lane_pin key: %s", raw)
	}

	// The inverse, so the absence above is a fact about the pin and not about
	// the marshaller.
	q.PinnedLane, q.TaskID = "zai", "t-ln9r1-wire-pinned"
	pinned, err := r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route(pinned): %v", err)
	}
	raw, err = json.Marshal(pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"lane_pin":"zai"`) {
		t.Errorf("a PINNED decision does not serialize the member: %s", raw)
	}
}

// TestLN9R1EveryNewSelectionArmIsGatedOnThePin is the structural half of the
// pre-vs-post claim: the packet's additions to routing.go are reachable ONLY
// through a non-empty PinnedLane. A source scan is a weak instrument in
// general; here it is the right one, because the claim is precisely "no new
// code runs when the field is empty" and that is a statement about the SOURCE
// rather than about any one draw.
func TestLN9R1EveryNewSelectionArmIsGatedOnThePin(t *testing.T) {
	body := readWorkerSourceExternal(t, "routing.go")

	// The lane-pin arm in resolveSeat is entered only under the guard.
	if !strings.Contains(body, `if q.PinnedLane != "" {`) {
		t.Error("the lane-pin arm is no longer gated on a non-empty PinnedLane — an ungated arm runs on every " +
			"unpinned task, which is what R10 exists to forbid")
	}
	// resolveLanePin is reached from exactly one place: that arm.
	if n := strings.Count(body, "r.resolveLanePin("); n != 1 {
		t.Errorf("resolveLanePin has %d call sites, want exactly 1 (the guarded arm) — a second caller is a "+
			"second door the property above does not cover", n)
	}
	// And the two decision sites carry the member straight from the query, so
	// an empty query member cannot produce a non-empty decision member.
	if n := strings.Count(body, "LanePin:      q.PinnedLane"); n < 1 {
		if n := strings.Count(body, "LanePin:       q.PinnedLane"); n < 1 {
			t.Error("Decision.LanePin is no longer taken directly from the query")
		}
	}
}

func readWorkerSourceExternal(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}
