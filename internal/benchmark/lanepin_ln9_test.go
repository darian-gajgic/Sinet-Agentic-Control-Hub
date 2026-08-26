package benchmark_test

// lanepin_ln9_test.go — P3-LN-9 §9.9 / R8(c) (BENCH-REG §2, S00.9 A13).
//
// The direct arm is DELIBERATELY EXEMPT from the per-task lane pin. Its
// substrate and lane are structural constants naming which of the platform's
// surfaces IS the requester's own frontier one, and the arm is born without
// consulting selection at all — the arm's identity IS its lane. A task pin
// moving it would not be honoring a preference, it would be corrupting the
// blind-pair protocol: the comparison would no longer be against the surface
// the record says it was against.
//
// $0: no engine, no provider.

import (
	"context"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// TestLN9BenchmarkDirectArmIgnoresThePin dispatches the direct arm on a task
// carrying a lane pin and asserts the arm's own run row is untouched.
//
// The pin reaches this task the way every pin does — it rides the intake
// request, which the benchmark package never reads — so what this guard really
// holds is that the arm's birth stays independent of selection. If a future
// packet routes the direct arm through Route, this fails and says why.
func TestLN9BenchmarkDirectArmIgnoresThePin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)

	pair := h.sampledPair(t, "t-ln9-pinned")
	if _, err := h.practice(t).DispatchDirectArm(ctx, pair.PairID); err != nil {
		t.Fatalf("DispatchDirectArm: %v", err)
	}

	runID := benchmark.DirectRunID(pair.PairID)
	var lane, substrate string
	if err := h.db.QueryRowContext(ctx,
		`SELECT lane, substrate FROM runs WHERE run_id = ?`, runID).Scan(&lane, &substrate); err != nil {
		t.Fatalf("read the direct arm's run row: %v", err)
	}
	if lane != "anthropic" || substrate != "claude-cli" {
		t.Fatalf("the direct arm was born on lane %q / substrate %q, want anthropic / claude-cli — the arm's "+
			"identity IS its lane, and a task pin moving it would mean the blind pair compared against a "+
			"different surface than its own record names (BENCH-REG §2; brief R8(c))", lane, substrate)
	}

	// And the arm's row is stable across its own admission. Stated for what it
	// is (r1 F12): this drives NO dispatch, so it does not prove where the R9
	// stamp was installed — that is held by the execute-dispatch guards in
	// internal/stage. What it proves is narrower and still worth holding: the
	// direct arm's lane and substrate are set once, at birth, from the
	// structural constants, and nothing on the arm's own path moves them.
	if err := h.exec(ctx, `UPDATE runs SET state = 'running' WHERE run_id = ?`, runID); err != nil {
		t.Fatalf("advance the arm: %v", err)
	}
	if err := h.db.QueryRowContext(ctx,
		`SELECT lane, substrate FROM runs WHERE run_id = ?`, runID).Scan(&lane, &substrate); err != nil {
		t.Fatalf("re-read the direct arm's run row: %v", err)
	}
	if lane != "anthropic" || substrate != "claude-cli" {
		t.Errorf("the direct arm's lane/substrate moved to %q/%q after dispatch", lane, substrate)
	}
}
