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
	"go/ast"
	"go/parser"
	"go/token"
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

	// The two halves of R5(b) composed on the one state where the FSM lets both
	// be seen at once: from `claimed` the terminal edges are illegal (so the
	// write fails for a reason that is not the context) and `crashed` is legal
	// (so the posture's corpse lands).
	t.Run("a failed terminal write still admits the corpse", func(t *testing.T) {
		e := newCancelEnv(t)
		const runID, taskID = "t-terminal-residual.verify", "t-terminal-residual"
		e.seedRun(t, runID, taskID, owner, run.StateClaimed)

		err := e.sk.verifyTerminal(context.Background(), runID, taskID, verify.Outcome{
			Verdict: verify.VerdictShip,
		})
		if err == nil {
			t.Fatal("the terminal write must report a refused transition")
		}
		e.sk.crash(context.Background(), runID, "verification terminal record: "+err.Error())
		if got := e.state(t, runID); got != run.StateCrashed {
			t.Fatalf("run is %s, want crashed — a run holding an unlanded outcome must stay classifiable", got)
		}
	})
}

// TestVerifyTerminalErrorsTakeTheCrashPostureAtEverySite is the STRUCTURAL half
// of R5(b), and it exists because behavior cannot reach it: `verifyTerminal`
// only fails when its transition is refused, and every state that refuses
// running→completed / running→parked (parked, terminal, tombstoned) refuses
// running→crashed as well — the sole exception, `claimed`, is unreachable once
// a dispatch leg has taken the run to running. So the difference a deleted
// `crash` call makes at the DISPATCH site shows up only under a transient
// storage failure, which no hermetic fixture produces deterministically.
//
// The source is therefore the evidence, as it already is for the NO-AUTO-KILL
// proof (cancel_internal_test.go) and the watchdog import wall: every
// verifyTerminal call must have its error checked, and every such check must
// leave the ladder a corpse. Deleting either crash call, or reverting a site to
// a bare `return s.verifyTerminal(...)`, fails this test.
func TestVerifyTerminalErrorsTakeTheCrashPostureAtEverySite(t *testing.T) {
	const wantSites = 2 // dispatchVerify (the drain leg) + answerRevise (the S07.7 resume leg)
	fset := token.NewFileSet()
	sites, guarded := 0, 0
	for _, path := range []string{"skeleton.go", "answer.go"} {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if calleeName(x) == "verifyTerminal" {
					sites++
				}
			case *ast.IfStmt:
				if x.Init == nil || !callsFunc(x.Init, "verifyTerminal") {
					return true
				}
				guarded++
				if !callsFunc(x.Body, "crash") {
					t.Errorf("%s: a verifyTerminal error is handled without crashing the run — a run holding an unlanded outcome would strand exactly as a dead drive does (R5b, CONVENTIONS §16/§56)", path)
				}
			}
			return true
		})
	}
	if sites != wantSites {
		t.Errorf("found %d verifyTerminal call sites, want %d — a new site owes the same posture", sites, wantSites)
	}
	if guarded != sites {
		t.Errorf("%d of %d verifyTerminal call sites check their error; an unchecked one swallows a lost outcome", guarded, sites)
	}
}

// calleeName is the called function's own name (the selector's, for a method).
func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

// callsFunc reports whether the node contains a call to the named function.
func callsFunc(n ast.Node, name string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && calleeName(call) == name {
			found = true
		}
		return !found
	})
	return found
}
