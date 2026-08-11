package intake_test

// rebind_test.go — P3-RW-6 §7 T-A3: the dispatch-entry rebind and its LINEAGE
// GUARD (Spec S02.5 steps 2–3; migration 0002 lineage columns).
//
// The rebind is what makes a recovery fork actually supersede its parent: the
// pipeline's state carries the run id it was born with (`<task>.intake`), and a
// fork dispatch must move that identity onto the fork before anything else
// happens. Because the rebind moves an identity, it is GUARDED: the dispatched
// run must be a real descendant of the state's current run, by the
// `runs.parent_run_id` chain, and must belong to the same task. Anything else
// is refused loudly — a rebind onto an unrelated run would hand one task's
// interview to another task's run.

import (
	"context"
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// stateEvents counts the task's intake.state events (the rebind appends exactly
// one; a refusal appends none).
func (f *fix) stateEvents(taskID string) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM run_events e JOIN runs r ON r.run_id = e.run_id
		 WHERE r.task_id = ? AND e.type = ?`, taskID, intake.EventState).Scan(&n); err != nil {
		f.t.Fatalf("count intake.state: %v", err)
	}
	return n
}

// forkRun creates a fork-shaped successor row the way internal/recovery does
// (Spec S02.5 step 3: `<parent>.g<newGen>`, queued, parent_run_id set), then
// walks it to running — the scheduler's claim, which the dev harness performs
// directly (CONVENTIONS §14).
func (f *fix) forkRun(parentID, taskID, owner string, gen int64, withLineage bool) string {
	f.t.Helper()
	ctx := context.Background()
	id := parentID + ".g" + itoa(gen)
	n := run.NewRun{ID: id, UserID: owner, TaskID: taskID, State: run.StateQueued, Generation: gen}
	if withLineage {
		n.ParentRunID = parentID
	}
	if _, err := f.runs.Create(ctx, n); err != nil {
		f.t.Fatalf("create fork %s: %v", id, err)
	}
	for _, s := range []run.State{run.StateClaimed, run.StateRunning} {
		if _, err := f.runs.Transition(ctx, id, s, run.TransitionOptions{
			Reason: "test claim", Actor: run.ActorPlatform,
		}); err != nil {
			f.t.Fatalf("claim fork %s→%s: %v", id, s, err)
		}
	}
	return id
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestForkRebindLineageGuard(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)

	// Task A: an interview card is open (the run is parked on its gate).
	stA := f.start(stdRequest())
	f.admit(stA.RunID)
	stA = f.advance(stA.TaskID)
	if stA.OpenAskID == "" {
		t.Fatalf("expected an open interview gate, got %+v", stA.Phase)
	}
	askID := stA.OpenAskID

	// Task B: an unrelated task with its own intake run.
	reqB := stdRequest()
	reqB.TaskID = "t-other-task"
	stB := f.start(reqB)
	f.admit(stB.RunID)

	// (1) A dispatch of ANOTHER task's run is refused: no rebind, no event.
	before := f.stateEvents(stA.TaskID)
	if _, err := f.p.AdvanceDispatched(ctx, stA.TaskID, stB.RunID); err == nil {
		t.Fatal("rebinding task A's state onto task B's run must be refused")
	} else if !errors.Is(err, intake.ErrLineage) {
		t.Fatalf("refusal error = %v, want an ErrLineage refusal", err)
	}
	if got := f.stateEvents(stA.TaskID); got != before {
		t.Fatalf("%d state events appended by a refused rebind, want 0", got-before)
	}
	if cur, err := f.p.LoadState(ctx, stA.TaskID); err != nil || cur.RunID != stA.RunID {
		t.Fatalf("state run id moved on a refused rebind: %v (%v)", cur, err)
	}

	// (2) A run WEARING the fork id shape but with no lineage row is refused too
	// — the guard walks parent_run_id, it does not parse names.
	orphan := f.forkRun(stA.RunID, stA.TaskID, stA.Owner, 9, false)
	if _, err := f.p.AdvanceDispatched(ctx, stA.TaskID, orphan); err == nil {
		t.Fatal("a fork-shaped run with no parent_run_id lineage must be refused")
	}
	if got := f.stateEvents(stA.TaskID); got != before {
		t.Fatalf("%d state events appended by the orphan refusal, want 0", got-before)
	}

	// (3) The SAME-id dispatch is a no-op: no rebind event, and the open gate
	// still waits (gates wait, S06.1).
	if _, err := f.p.AdvanceDispatched(ctx, stA.TaskID, stA.RunID); err != nil {
		t.Fatalf("same-id dispatch: %v", err)
	}
	if got := f.stateEvents(stA.TaskID); got != before {
		t.Fatalf("the same-id dispatch appended %d events, want 0 (R13: the non-fork path is unchanged)", got-before)
	}

	// (4) A genuine fork REBINDS: one state event on the FORK naming the fork,
	// the open ask row re-points to it (OQ1 second limb), and the fork lands
	// PARKED on the gate it inherited rather than running idle (R4).
	fork := f.forkRun(stA.RunID, stA.TaskID, stA.Owner, 2, true)
	st, err := f.p.AdvanceDispatched(ctx, stA.TaskID, fork)
	if err != nil {
		t.Fatalf("AdvanceDispatched on a genuine fork: %v", err)
	}
	if st.RunID != fork {
		t.Fatalf("state run id = %q, want the fork %q", st.RunID, fork)
	}
	if got := f.stateEvents(stA.TaskID); got != before+1 {
		t.Fatalf("rebind appended %d events, want exactly 1", got-before)
	}
	var evRun string
	if err := f.db.QueryRowContext(ctx,
		`SELECT run_id FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		intake.EventState).Scan(&evRun); err != nil {
		t.Fatalf("read rebind event: %v", err)
	}
	if evRun != fork {
		t.Fatalf("rebind event landed on %q, want the fork %q", evRun, fork)
	}
	var askRun string
	if err := f.db.QueryRowContext(ctx, `SELECT run_id FROM asks WHERE ask_id = ?`, askID).Scan(&askRun); err != nil {
		t.Fatalf("read ask: %v", err)
	}
	if askRun != fork {
		t.Fatalf("open ask row still names %q, want the fork %q", askRun, fork)
	}
	if got := f.runState(fork); got != run.StateParked {
		t.Fatalf("fork state = %s, want parked on the inherited gate (R4)", got)
	}

	// (5) MULTI-HOP lineage rebinds too — the live-world t-80eff shape: a fork
	// crashed before its own rebind committed, so the state still names an
	// ANCESTOR two hops above the run being dispatched.
	mid := f.forkRun(fork, stA.TaskID, stA.Owner, 3, true)
	deep := f.forkRun(mid, stA.TaskID, stA.Owner, 4, true)
	if _, err := f.p.AdvanceDispatched(ctx, stA.TaskID, deep); err != nil {
		t.Fatalf("multi-hop rebind: %v", err)
	}
	cur, err := f.p.LoadState(ctx, stA.TaskID)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if cur.RunID != deep {
		t.Fatalf("state run id = %q, want the deep fork %q", cur.RunID, deep)
	}
}
