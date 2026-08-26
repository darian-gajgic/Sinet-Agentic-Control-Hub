package intake

// lanepin_ln9_test.go — P3-LN-9 (S00.9 A13, S15.2, S08.8): the lane pin at the
// system BOUNDARY, and the re-plan behaviour that makes the worker pin's
// freeze mechanic unnecessary.
//
// In-package because refuseLanePin, routeQueryFor and computeRouting are
// unexported — the three things worth pinning here all sit behind them.
//
// $0: no database, no engine, no provider. Everything below is pure over the
// pipeline's own inputs.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ln9Pinnable is the seam value the composition root fills: the covered lanes
// plus the local ENGINE lane carried as a KNOWN-and-refused row, so the
// boundary can answer it in its own words.
func ln9Pinnable() []LanePinOption {
	return []LanePinOption{
		{Lane: "anthropic", Pinnable: true},
		{Lane: "zai", Pinnable: true},
		{Lane: "local", NotPinnable: "lane \"local\" is the local ENGINE lane, which carries no v0 consumer: " +
			"no local provider entry is commissioned (S12.1 class (a))"},
	}
}

func TestLN9BoundaryAdmitsOnlyWhatSelectionWouldHonor(t *testing.T) {
	p := &Pipeline{PinnableLanes: ln9Pinnable()}

	// The ordinary case is untouched: no pin, no question asked.
	if err := p.refuseLanePin(""); err != nil {
		t.Fatalf("an UNPINNED request was refused (%v) — the absent case must behave exactly as before", err)
	}
	for _, lane := range []string{"anthropic", "zai"} {
		if err := p.refuseLanePin(lane); err != nil {
			t.Errorf("refuseLanePin(%q) = %v, want admitted — the lane is one selection would honor", lane, err)
		}
	}

	// The local engine lane refuses with the SELECTION layer's own sentence,
	// quoted rather than re-composed here (§66 D1: the boundary never
	// re-derives a domain verdict).
	err := p.refuseLanePin("local")
	if err == nil {
		t.Fatal("a pin to the local engine lane was admitted (brief R8(b))")
	}
	if !errors.Is(err, ErrLanePinRefused) {
		t.Fatalf("err = %v, want ErrLanePinRefused so the surface can map ONE code", err)
	}
	if !strings.Contains(err.Error(), "S12.1 class (a)") {
		t.Errorf("the local refusal is not the carried verdict — it must be quoted, not re-derived: %v", err)
	}
	if strings.Contains(err.Error(), "Subscription gap") {
		t.Errorf("the local refusal borrowed the 2.7 subscription-gap wording: %v", err)
	}

	// A lane the platform does not offer at all refuses with the OTHER detail
	// sentence, and names what IS pinnable. Lane names ship in the lane
	// documents and are not secret, so unlike the project pin this may
	// enumerate rather than answer a deliberate not-found.
	unknown := p.refuseLanePin("kimi-cli")
	if unknown == nil {
		t.Fatal("a pin to a lane this platform cannot dispatch to was admitted")
	}
	if !errors.Is(unknown, ErrLanePinRefused) {
		t.Fatalf("err = %v, want ErrLanePinRefused", unknown)
	}
	if unknown.Error() == err.Error() {
		t.Error("the unknown-lane refusal and the local-lane refusal are the same sentence — one code, DISTINCT " +
			"detail (OQ-5)")
	}
	for _, want := range []string{`"anthropic"`, `"zai"`, "kimi-cli"} {
		if !strings.Contains(unknown.Error(), want) {
			t.Errorf("the refusal does not mention %s: %v", want, unknown)
		}
	}
	if strings.Contains(unknown.Error(), `"local"`) {
		t.Errorf("a lane the platform refuses is listed as pinnable: %v", unknown)
	}
}

// The nil seam is FAIL-CLOSED. The hazard this packet closes is a pin silently
// dropped, so a pipeline whose composition root never filled the seam must
// refuse every pin rather than admit every pin — and a mutation that stops the
// root filling it turns the ACCEPTED case red rather than leaving the platform
// quietly permissive (§12: the only default is consent; §65 D5).
func TestLN9UnwiredPinSeamRefusesRatherThanAdmits(t *testing.T) {
	p := &Pipeline{}
	if err := p.refuseLanePin(""); err != nil {
		t.Fatalf("an unpinned request on an unwired pipeline was refused (%v) — the absent case must be inert", err)
	}
	err := p.refuseLanePin("anthropic")
	if err == nil {
		t.Fatal("an unwired lane-pin seam ADMITTED a pin — with nothing composed, nothing is known to be " +
			"dispatchable, and admitting is the silent-drop hazard wearing a different hat")
	}
	if !errors.Is(err, ErrLanePinRefused) {
		t.Fatalf("err = %v, want ErrLanePinRefused", err)
	}
	if !strings.Contains(err.Error(), "none") {
		t.Errorf("the refusal does not say that nothing is pinnable here: %v", err)
	}
}

// ── The pin reaches the router query, and is re-read on every recompute ────

func TestLN9RouteQueryCarriesTheTasksPin(t *testing.T) {
	st := &State{
		Owner: "alice", TaskID: "t-pin", RunID: "t-pin.intake",
		Req: Request{Title: "compare the lanes", Text: "run it twice", PinnedLane: "zai"},
	}
	q := routeQueryFor(st, &Pair{})
	if q.PinnedLane != "zai" {
		t.Fatalf("routeQueryFor dropped the task's lane pin (%q) — a correct selector reached through a dropped "+
			"argument is a broken feature (§63 R1)", q.PinnedLane)
	}
	// And an unpinned task carries nothing, so the query is what it was.
	st.Req.PinnedLane = ""
	if q := routeQueryFor(st, &Pair{}); q.PinnedLane != "" {
		t.Errorf("an unpinned task produced PinnedLane=%q", q.PinnedLane)
	}
}

// recordingRouter captures every query selection was asked, so a test can say
// what the pipeline actually sent rather than what it hoped it sent.
type recordingRouter struct {
	seen  []RouteQuery
	block RouteBlock
}

func (r *recordingRouter) RouteTask(_ context.Context, q RouteQuery) (RouteBlock, error) {
	r.seen = append(r.seen, q)
	b := r.block
	b.LanePin = q.PinnedLane
	return b, nil
}

// TestLN9PinSurvivesReplanAndCarriesNoWorkerFreeze — §9.6 / brief R8(a).
func TestLN9PinSurvivesReplanAndCarriesNoWorkerFreeze(t *testing.T) {
	ctx := context.Background()
	rec := &recordingRouter{block: RouteBlock{
		Cause: "selector-match", Model: "glm-5.3", Lane: "zai",
		WindowTokens: 200000, PlainReason: "seeded",
	}}
	p := &Pipeline{Router: rec, PinnableLanes: ln9Pinnable()}
	st := &State{
		Owner: "alice", TaskID: "t-pin", RunID: "t-pin.intake",
		Req: Request{Title: "compare the lanes", Text: "run it twice", PinnedLane: "zai"},
	}

	// First selection, then a RE-PLAN recompute. The pin must reach both.
	for round := 1; round <= 2; round++ {
		if err := p.computeRouting(ctx, st, &Pair{}); err != nil {
			t.Fatalf("computeRouting round %d: %v", round, err)
		}
	}
	if len(rec.seen) != 2 {
		t.Fatalf("selection ran %d times, want 2", len(rec.seen))
	}
	for i, q := range rec.seen {
		if q.PinnedLane != "zai" {
			t.Fatalf("recompute %d asked selection with PinnedLane=%q — the pin is a fact about the TASK, so a "+
				"recompute that re-reads the task re-reads the pin (brief R8(a))", i+1, q.PinnedLane)
		}
	}

	// THE TRAP, pinned as a test (§1.2): a lane pin must not set the WORKER
	// pin. `Pinned` means "freeze the worker choice against a re-plan
	// recompute", and nobody asked for that — setting it would freeze a choice
	// the requester never touched.
	if st.Routing == nil {
		t.Fatal("no routing block was recorded")
	}
	if st.Routing.Pinned {
		t.Error("a LANE-pinned task carries RouteBlock.Pinned — that flag is the WORKER axis and freezes the " +
			"worker choice against re-planning, which no lane pin asked for")
	}
	if st.Routing.OverriddenBy != "" {
		t.Errorf("a lane pin recorded an override actor (%q) — no card override happened", st.Routing.OverriddenBy)
	}
	if st.Routing.LanePin != "zai" {
		t.Errorf("RouteBlock.LanePin = %q, want the pinned lane — it is the structured member the picker binds to",
			st.Routing.LanePin)
	}

	// The control: an unpinned task records neither, so the member is empty
	// exactly where it should be.
	st.Req.PinnedLane = ""
	st.Routing = nil
	rec.seen = nil
	if err := p.computeRouting(ctx, st, &Pair{}); err != nil {
		t.Fatalf("computeRouting(unpinned): %v", err)
	}
	if st.Routing.LanePin != "" || st.Routing.Pinned {
		t.Errorf("an unpinned task recorded LanePin=%q Pinned=%v", st.Routing.LanePin, st.Routing.Pinned)
	}
}
