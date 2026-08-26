package metering

// lanepin_ln9_test.go — P3-LN-9 §9.7/§9.8 (S10.1, S10.4, R4, R9).
//
// Two claims, and the first is a CORRECTION of a defect the pin made
// load-bearing. The operator was told the receipt's Lane column is the proof of
// which path ran the work; it was not. `runs.lane` is stamped at run BIRTH from
// process config and nothing ever updated it, so a run that routed to a second
// lane still metered and receipted as the first. This file pins that the
// receipt now shows the lane that RAN — and that in a world with nothing to
// correct, nothing moves.
//
// The second claim is the one the pin itself must not break: pinning between
// two lanes on ONE membership changes which client runs the work and changes
// the allowance not at all.
//
// $0: a temp database, no engine, no provider.

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ── §9.7 · the receipt's Lane column shows the lane that RAN ──────────────

// TestLN9ReceiptLaneIsTheLaneThatRan drives the receipt path over a run row
// whose lane was corrected after routing settled — which is what the R9 stamp
// does — and asserts the line item follows it.
//
// The REGRESSION PIN is the control below: the pre-packet behaviour was a
// decision naming one lane and a receipt naming another, and that is what must
// now fail.
func TestLN9ReceiptLaneIsTheLaneThatRan(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	// A run born on the process default, as launchRole stamps one.
	e.runningRun(t, "r-ln9", "alice", "anthropic", "claude-cli")
	// Routing settled on a second lane, and the stamp corrected the row.
	if err := e.runs.SetDecidedLane(ctx, "r-ln9", "zai", "opencode"); err != nil {
		t.Fatalf("SetDecidedLane: %v", err)
	}
	e.checkpoint(t, "r-ln9", "glm-5.3", `{"input_tokens":100,"output_tokens":50}`)

	led := NewLedger(e.db, NewEffectiveDatedTable("empty"), NoMeteredExceptions(), e.reg)
	rcpt, err := NewReceipts(e.db, led, NoMeteredExceptions()).Materialize(ctx, "r-ln9")
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if len(rcpt.Items) != 1 {
		t.Fatalf("receipt has %d line items, want 1", len(rcpt.Items))
	}
	if got := rcpt.Items[0].Lane; got != "zai" {
		t.Fatalf("the receipt's Lane column says %q, want %q — the operator was told this column is the proof "+
			"of which path ran the work, and until the run row carried the DECIDED lane it was not (R9)", got, "zai")
	}
	if got := rcpt.Items[0].Lane; got == "anthropic" {
		t.Fatal("the receipt still reports the creation-time stamp — this is the pre-packet behaviour, and it " +
			"is exactly what must fail now")
	}
}

// The control that keeps R9 a correction: with nothing commissioned the decided
// lane EQUALS the configured one, the stamp writes nothing, and the receipt is
// what it always was.
func TestLN9ReceiptIsUnchangedWhereTheDecisionAgreesWithTheRow(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun(t, "r-plain", "alice", "anthropic", "claude-cli")
	e.checkpoint(t, "r-plain", "claude-sonnet-5", `{"input_tokens":100,"output_tokens":50}`)

	led := NewLedger(e.db, NewEffectiveDatedTable("empty"), NoMeteredExceptions(), e.reg)
	before, err := NewReceipts(e.db, led, NoMeteredExceptions()).Materialize(ctx, "r-plain")
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	// The stamp a default-world dispatch would apply: identical values.
	if err := e.runs.SetDecidedLane(ctx, "r-plain", "anthropic", "claude-cli"); err != nil {
		t.Fatalf("SetDecidedLane: %v", err)
	}
	after, err := NewReceipts(e.db, led, NoMeteredExceptions()).Materialize(ctx, "r-plain")
	if err != nil {
		t.Fatalf("Materialize (after): %v", err)
	}
	if len(after.Items) != len(before.Items) || after.Items[0].Lane != before.Items[0].Lane {
		t.Errorf("a no-op stamp moved the receipt: %+v → %+v", before.Items, after.Items)
	}
	if after.Items[0].Lane != "anthropic" {
		t.Errorf("the default world's receipt lane is %q, want anthropic", after.Items[0].Lane)
	}
}

// ── §9.8 · a pin does not change what the pool holds ──────────────────────

// TestLN9PooledPressureUnchangedUnderAPin is the assertion that stops a pinned
// receipt from being read as evidence of a separate quota. The two Kimi lanes
// draw ONE membership; pinning between them changes which client ran the work
// and changes the allowance not at all.
func TestLN9PooledPressureUnchangedUnderAPin(t *testing.T) {
	ctx := context.Background()
	doc, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi")
	}

	// Two worlds with the SAME total work on the pool, differing only in which
	// lane ran it — which is precisely what a pin decides.
	read := func(t *testing.T, apiCalls, cliCalls int) (int64, float64) {
		t.Helper()
		env := newPoolEnv(t)
		env.checkpoint(t, "kimi", apiCalls)
		env.checkpoint(t, "kimi-cli", cliCalls)
		calls, consumed, err := env.g.planUnits(ctx, env.user, "kimi", doc, time.Time{}, false, env.now)
		if err != nil {
			t.Fatalf("planUnits: %v", err)
		}
		// The requesting lane must not change the answer either.
		cliCallsRead, cliConsumed, err := env.g.planUnits(ctx, env.user, "kimi-cli", doc, time.Time{}, false, env.now)
		if err != nil {
			t.Fatalf("planUnits(kimi-cli): %v", err)
		}
		if cliCallsRead != calls || cliConsumed != consumed {
			t.Fatalf("the two pooled lanes disagree about their own pool: %d/%v vs %d/%v",
				calls, consumed, cliCallsRead, cliConsumed)
		}
		return calls, consumed
	}

	allAPI, consumedAPI := read(t, 10, 0)
	allCLI, consumedCLI := read(t, 0, 10)
	split, consumedSplit := read(t, 6, 4)

	if allAPI != 10 || allCLI != 10 || split != 10 {
		t.Errorf("the pooled reading moved with the pin: all-api=%d all-cli=%d split=%d, want 10 everywhere — "+
			"a pin steers work between two lanes on ONE allowance", allAPI, allCLI, split)
	}
	if consumedAPI != consumedCLI || consumedAPI != consumedSplit {
		t.Errorf("the pooled consumption moved with the pin: %v / %v / %v", consumedAPI, consumedCLI, consumedSplit)
	}
}

// PlanPoolRefusal is UNTOUCHED by the pin: a pin does not make the sibling lane
// budgetable, and the canonical-lane rule still refuses the second declaration
// by name.
func TestLN9PinDoesNotMakeTheSiblingLaneBudgetable(t *testing.T) {
	doc, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi")
	}
	if got := PlanPoolRefusal(doc, "kimi"); got != "" {
		t.Errorf("the canonical lane was refused its own budget: %q", got)
	}
	refusal := PlanPoolRefusal(doc, "kimi-cli")
	if refusal == "" {
		t.Fatal("the pooled sibling lane became budgetable — a pin steers dispatch, it does not split an allowance")
	}
	if !strings.Contains(refusal, doc.Lane) {
		t.Errorf("the refusal does not name the pool's canonical lane: %q", refusal)
	}
}
