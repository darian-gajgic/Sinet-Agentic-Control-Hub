package stage

// abort_internal_test.go — P3-RW-10 T4/T5(b): the two stage-layer writes that
// record an ENDING must be writable at the moment the ending happens, whatever
// the caller's context did.
//
//   - `crash` is the corpse the recovery ladder forks from (S02.5 step 2). Every
//     dispatch leg, both intake beats and both verify-answer sites call it, so
//     detaching it INSIDE covers them all — and the cancel-suppression consult
//     still runs, so a leg unwound by a human cancel stays corpse-free (drain D1).
//   - `verifyTerminal` lands a drain's PAID outcome (S07.7). A drain that
//     finished its work and lost its requester must still land the verdict: no
//     paid work is lost (D7), no finding dies silently (S07.7).
//
// $0: nothing here spawns a process or dials anything.

import (
	"context"
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// deadContext is a request context that has already been aborted.
func deadContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func crashedCause(t *testing.T, e *cancelEnv, runID string) string {
	t.Helper()
	var cause string
	if err := e.db.QueryRowContext(context.Background(), `
		SELECT COALESCE(json_extract(payload, '$.detail.cause'), '')
		  FROM run_events
		 WHERE run_id = ? AND type = ? AND json_extract(payload, '$.to') = 'crashed'
		 ORDER BY event_seq LIMIT 1`, runID, run.EventState).Scan(&cause); err != nil {
		t.Fatalf("crash cause for %s: %v", runID, err)
	}
	return cause
}

func TestCrashHelperSurvivesCanceledContext(t *testing.T) {
	e := newCancelEnv(t)
	const runID, taskID, owner = "t-crash-abort.intake", "t-crash-abort", "u-abort"
	e.seedRun(t, runID, taskID, owner, run.StateRunning)

	e.sk.crash(deadContext(), runID, "intake answer: planner session: context canceled")

	if got := e.state(t, runID); got != run.StateCrashed {
		t.Fatalf("run is %s after crash on a dead context, want crashed (R2)", got)
	}
	if cause := crashedCause(t, e, runID); cause == "" {
		t.Fatal("the corpse carries no cause — unclassifiable (§8: reason+actor+detail)")
	}

	t.Run("cancel suppression still suppresses", func(t *testing.T) {
		e := newCancelEnv(t)
		const runID, taskID = "t-crash-suppressed.intake", "t-crash-suppressed"
		r := e.seedRun(t, runID, taskID, owner, run.StateRunning)

		// A human cancel is in flight at this generation: the unwind it caused
		// must NOT file a corpse for the ladder to fork (drain D1).
		e.sk.cancels.markRequested(runID, r.Generation)
		e.sk.crash(deadContext(), runID, "dispatch leg unwound by the cancel")

		if got := e.state(t, runID); got != run.StateRunning {
			t.Fatalf("run is %s, want still running — the suppression consult must work detached too", got)
		}
		if e.sk.cancels.consumeCancel(runID, r.Generation) {
			t.Fatal("the suppression mark was not consumed — it would swallow a later, genuine crash")
		}
	})
}

func TestVerifyTerminalSurvivesCanceledContext(t *testing.T) {
	const owner = "u-abort-verify"

	t.Run("ship lands", func(t *testing.T) {
		e := newCancelEnv(t)
		const runID, taskID = "t-terminal-ship.verify", "t-terminal-ship"
		e.seedRun(t, runID, taskID, owner, run.StateRunning)

		if err := e.sk.verifyTerminal(deadContext(), runID, taskID, verify.Outcome{
			Verdict: verify.VerdictShip, VerifiedItems: []string{"S-1"},
		}); err != nil {
			t.Fatalf("verifyTerminal on a dead context: %v", err)
		}
		if got := e.state(t, runID); got != run.StateCompleted {
			t.Fatalf("run is %s, want completed — a finished drain's outcome outlives its request (R5a)", got)
		}
		if got := e.kanban(t, taskID); got != "done" {
			t.Fatalf("kanban = %q, want done", got)
		}
	})

	t.Run("card re-parks", func(t *testing.T) {
		e := newCancelEnv(t)
		const runID, taskID = "t-terminal-card.verify", "t-terminal-card"
		e.seedRun(t, runID, taskID, owner, run.StateRunning)

		if err := e.sk.verifyTerminal(deadContext(), runID, taskID, verify.Outcome{
			Verdict: verify.VerdictEscalate,
		}); err != nil {
			t.Fatalf("verifyTerminal on a dead context: %v", err)
		}
		if got := e.state(t, runID); got != run.StateParked {
			t.Fatalf("run is %s, want parked on its card (S07.7/S02.3)", got)
		}
		if got := e.kanban(t, taskID); got != "attention" {
			t.Fatalf("kanban = %q, want attention", got)
		}
	})

	// The residual-error posture: a terminal write that fails REGARDLESS (here:
	// no such run) must leave the dispatch leg's caller an error, never a silent
	// swallow — the crash posture at the call sites covers the run itself.
	t.Run("a residual failure is still reported", func(t *testing.T) {
		e := newCancelEnv(t)
		err := e.sk.verifyTerminal(deadContext(), "t-nope.verify", "t-nope", verify.Outcome{
			Verdict: verify.VerdictShip,
		})
		if err == nil {
			t.Fatal("a terminal write on a nonexistent run must error, not pass silently")
		}
		if errors.Is(err, context.Canceled) {
			t.Fatalf("the terminal write still rides the request context: %v", err)
		}
	})
}
