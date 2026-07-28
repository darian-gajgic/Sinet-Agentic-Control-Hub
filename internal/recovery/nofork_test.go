package recovery_test

// nofork_test.go — the P3-B6-2C drain-r1 fork-refusal seam (D1).
//
// The S02.5 step-3 default for a crashed run is FORK-from-last-checkpoint. For a
// single-shot run class that default is a protocol breach rather than a
// recovery: BENCH-REG §2 freezes the direct arm as "run once … no retries, no
// follow-up turns, take what comes", the fork would be a SECOND billed engine
// run on the same frozen statement, and its output could never be consumed
// anyway (the capture keys the parent run id). The predicate seam is how the
// class contract outranks the default without internal/recovery learning what a
// benchmark is.
//
// The seam must not OVER-match either — an ordinary crashed run still forks, and
// that half is asserted in the same fixture so a predicate that swallowed
// everything could not pass.

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// noForkLadder builds a ladder with the predicate under test.
func noForkLadder(t *testing.T, s *stack, noFork func(string) bool) *recovery.Ladder {
	t.Helper()
	l, err := recovery.New(recovery.Config{
		DB: s.db, Log: s.log, Runs: s.runs, Checkpoints: s.cps, Effects: s.jrnl,
		Settings: s.reg, NoFork: noFork,
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	return l
}

// suffixNoFork is the shell's own composition shape: the run-id convention, and
// nothing about benchmarks inside this package.
func suffixNoFork(runID string) bool { return strings.HasSuffix(runID, ".direct") }

// TestNoForkClassIsFinalizedNotForked is D1: a crashed single-shot run is
// finalized, an ordinary crashed run beside it still forks, and the two are
// driven by ONE pass over ONE fixture so the predicate cannot pass by matching
// everything or nothing.
func TestNoForkClassIsFinalizedNotForked(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	direct := mustRunning(t, s, "bp-1.direct")
	ordinary := mustRunning(t, s, "t-1.execute")
	for _, r := range []run.Run{direct, ordinary} {
		if _, err := s.runs.Transition(ctx, r.ID, run.StateCrashed, run.TransitionOptions{
			Reason: "fixture crash", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("crash %s: %v", r.ID, err)
		}
	}

	rpt, err := noForkLadder(t, s, suffixNoFork).ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Finalized != 1 || rpt.Forked != 1 {
		t.Fatalf("pass report finalized=%d forked=%d, want exactly one of each", rpt.Finalized, rpt.Forked)
	}

	// The single-shot arm: FINALIZED, and with no successor at all.
	got, err := s.runs.Get(ctx, direct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != run.StateFinalized {
		t.Errorf("the crashed direct arm is %s, want finalized — §2 is single-shot", got.State)
	}
	if hasSuccessor(t, s, direct.ID) {
		t.Error("the crashed direct arm was forked — a second billed run on the same frozen statement")
	}
	if got.Generation != direct.Generation {
		t.Errorf("the no-fork finalize bumped the generation %d→%d — nothing was fenced", direct.Generation, got.Generation)
	}

	// The ordinary run: still forked. The predicate did not over-match.
	if !hasSuccessor(t, s, ordinary.ID) {
		t.Error("an ordinary crashed run was NOT forked — the seam must change one class only")
	}
}

// TestNoForkRevertProbe is the drain's own guard: with the predicate absent, the
// ladder returns to the default fork and the assertion above would fail. A
// property that cannot fail when its mechanism is removed guarantees nothing.
func TestNoForkRevertProbe(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	direct := mustRunning(t, s, "bp-2.direct")
	if _, err := s.runs.Transition(ctx, direct.ID, run.StateCrashed, run.TransitionOptions{
		Reason: "fixture crash", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatal(err)
	}

	// nil predicate = the pre-drain behavior, exactly.
	rpt, err := noForkLadder(t, s, nil).ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Forked != 1 {
		t.Fatalf("without the predicate the pass forked %d runs, want 1 — the probe proves the mechanism is load-bearing", rpt.Forked)
	}
	if !hasSuccessor(t, s, direct.ID) {
		t.Fatal("without the predicate the direct arm was not forked — this probe no longer proves anything")
	}
}

// TestNoForkFinalizeNamesItsReason: the record has to SAY why this run was not
// forked, or the next reader sees an unexplained terminal and re-derives the
// wrong lesson from it.
func TestNoForkFinalizeNamesItsReason(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	direct := mustRunning(t, s, "bp-3.direct")
	if _, err := s.runs.Transition(ctx, direct.ID, run.StateCrashed, run.TransitionOptions{
		Reason: "fixture crash", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := noForkLadder(t, s, suffixNoFork).ReconcilePass(ctx); err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}

	var payload string
	if err := s.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = 'run.state_changed'
		  ORDER BY event_seq DESC LIMIT 1`, direct.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"finalized", "never forked", "BENCH-REG §2", "no_fork_reason"} {
		if !strings.Contains(payload, want) {
			t.Errorf("the finalize record does not carry %q: %s", want, payload)
		}
	}
}

// hasSuccessor reports whether the ladder forked a run. It reads the runs table
// directly (the store's successor lookup is transaction-scoped), which is also
// the shape a reader would use to audit a fork after the fact.
func hasSuccessor(t *testing.T, s *stack, parentRunID string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM runs WHERE parent_run_id = ?`, parentRunID).Scan(&n); err != nil {
		t.Fatalf("read successors of %q: %v", parentRunID, err)
	}
	return n > 0
}
