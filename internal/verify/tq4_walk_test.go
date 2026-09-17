package verify_test

// tq4_walk_test.go — P3-TQ-4a acceptance battery, walk-schema half: the
// interface the platform-authored acceptance walk plugs into (Spec S07.3
// [A16, 2026-09-17]). TQ-4a ships the schema, the deterministic clause split
// and the honest absence; TQ-4b ships the driver once a browser adoption is
// gate-ratified (Spec S16.4).
//
// The live defect these bind (TQ-F3/TQ-F7): the plan's given/when/then lines
// were machine-walkable and nobody walked them; and when a human did look, the
// app was dead on screen while its DOM was entirely correct — which is why the
// walk asserts DOM state and never rendered pixels.
//
// Committed RED at grounding: WalkPlanFor returns nothing today.

import (
	"reflect"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// webshopSpec is the frozen AC set from the TQ-F3 task, verbatim in shape: two
// criteria with given/when/then sub-lines, two with none.
func webshopSpec() intake.Spec {
	s := spec()
	s.ACs = []intake.AC{
		{N: 1, Plain: "The shop lists at least 30 car parts on the main view, and every one shows a picture and a price."},
		{N: 3, Plain: "Clicking a part opens its details.",
			Structured:     "Given a visitor viewing the list, when they click a part, then a detail view opens showing that part's information, price and compatible cars.",
			StructuredKind: "gwt"},
		{N: 5, Plain: "A visitor can add parts to a cart and see what is in it.",
			Structured:     "Given a visitor viewing a part, when they add it to the cart, then the cart shows the added part and no payment step is presented.",
			StructuredKind: "gwt"},
		{N: 9, Plain: "All text a visitor sees is in English."},
	}
	return s
}

func walkFor(t *testing.T, plan []verify.ACWalk, key string) verify.ACWalk {
	t.Helper()
	for _, w := range plan {
		if w.ACKey == key {
			return w
		}
	}
	t.Fatalf("no walk entry for %s in %+v", key, plan)
	panic("unreachable")
}

// TestTQ4WalkPlanSplitsTheFrozenGwtSubLine [R11]: every AC carrying a
// given/when/then sub-line is decomposed into navigate → act → assert steps,
// each carrying the verbatim clause it came from. The split is over the
// STRUCTURED sub-line only (Spec S06.6: verification binds to the sub-line
// where one exists).
func TestTQ4WalkPlanSplitsTheFrozenGwtSubLine(t *testing.T) {
	plan := verify.WalkPlanFor(webshopSpec())
	if len(plan) == 0 {
		t.Fatal("WalkPlanFor produced no plan — every frozen AC gets an entry, walkable or not (Spec S07.3 [A16])")
	}
	w := walkFor(t, plan, "AC-3")
	if !w.Walkable {
		t.Fatalf("AC-3 not walkable: %+v — it carries a gwt sub-line", w)
	}
	if len(w.Steps) != 3 {
		t.Fatalf("AC-3 produced %d steps, want 3 (given → when → then): %+v", len(w.Steps), w.Steps)
	}
	kinds := []verify.WalkStepKind{verify.WalkNavigate, verify.WalkAct, verify.WalkAssert}
	for i, want := range kinds {
		if w.Steps[i].Kind != want {
			t.Fatalf("AC-3 step %d kind %q, want %q", i, w.Steps[i].Kind, want)
		}
		if w.Steps[i].Clause == "" {
			t.Fatalf("AC-3 step %d carries no verbatim clause", i)
		}
	}
	if got := w.Steps[1].Clause; got == w.Steps[0].Clause {
		t.Fatalf("the when-clause is the given-clause (%q) — the split did not split", got)
	}
}

// TestTQ4ACWithoutAGwtSubLineIsNotAWalkFailure [R11]: an AC with no structured
// sub-line is recorded as not walkable with its reason. It is a criterion the
// walk does not cover — the judge weighs the plain line at V2 (Spec S06.6) —
// never a walk that failed.
func TestTQ4ACWithoutAGwtSubLineIsNotAWalkFailure(t *testing.T) {
	plan := verify.WalkPlanFor(webshopSpec())
	w := walkFor(t, plan, "AC-9")
	if w.Walkable {
		t.Fatalf("AC-9 reported walkable: %+v — it carries no given/when/then sub-line", w)
	}
	if len(w.Steps) != 0 {
		t.Fatalf("AC-9 produced steps from a plain line: %+v — the walk never invents a criterion's machine form", w.Steps)
	}
	if w.Reason == "" {
		t.Fatal("AC-9 carries no reason — an uncovered criterion says why in plain words")
	}
}

// TestTQ4WalkPlanIsPureOverTheSpec [R11, R13]: same spec, same plan. The walk
// plan is derived from the frozen ACs alone — never from the tree, the
// executor's report or the deliverable text.
func TestTQ4WalkPlanIsPureOverTheSpec(t *testing.T) {
	s := webshopSpec()
	a := verify.WalkPlanFor(s)
	b := verify.WalkPlanFor(s)
	if len(a) == 0 {
		t.Fatal("WalkPlanFor produced no plan")
	}
	// The WHOLE plan, not its shape: clauses, targets, expectations and
	// reasons are what a second call has to reproduce. Comparing only keys and
	// step counts would let the clause text vary between identical calls
	// (drain r1 F6).
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two calls over the same spec produced different plans:\n%+v\nvs\n%+v", a, b)
	}
}

// TestTQ4UncomposableStepIsNeverAPass [R11]: a step the platform can describe
// but not compose into something it can execute is marked uncomposable, and the
// schema offers no way to report it as passed (Spec S07.3 [A16]: "a walk step
// the platform cannot compose records UNVERIFIABLE-HERE with its reason, never
// PASS").
func TestTQ4UncomposableStepIsNeverAPass(t *testing.T) {
	plan := verify.WalkPlanFor(webshopSpec())
	w := walkFor(t, plan, "AC-5")
	if !w.Walkable {
		t.Fatalf("AC-5 not walkable: %+v", w)
	}
	// NON-VACUITY (drain r1 F6): the loop below says what an uncomposable step
	// may not carry, and a plan in which every step happened to be composable
	// would satisfy it by having nothing to check. With no browser adopted
	// nothing can be composed, so at least one uncomposable step must exist
	// for this test to be testing anything.
	uncomposable := 0
	for _, s := range w.Steps {
		if !s.Composable {
			uncomposable++
		}
	}
	if uncomposable == 0 {
		t.Fatalf("AC-5's steps are all composable: %+v — no browser is adopted, so nothing can be composed, and this test would assert nothing", w.Steps)
	}
	for i, s := range w.Steps {
		if s.Composable {
			continue
		}
		if s.Target != "" || s.Expect != "" {
			t.Fatalf("AC-5 step %d is not composable but carries a target/expectation (%q/%q) — an uncomposable step names no state it could not have read", i, s.Target, s.Expect)
		}
		if w.Reason == "" {
			t.Fatal("AC-5 holds an uncomposable step and states no reason")
		}
	}
}
