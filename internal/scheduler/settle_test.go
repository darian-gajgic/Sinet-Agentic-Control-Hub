package scheduler

// settle_test.go — P3-RW-6 §7 packet B: the queue rows of ENDED runs settle
// (T-B4/T-B5, checklist 13/14).
//
// `IsTerminal(crashed)` is deliberately false (crashed is supersedable), so the
// post-dispatch settle correctly skips a crashed run — but when the ladder later
// tombstones, finalizes or harvests it, NOBODY settled the queue row: recovery
// never touches the queue, `SettleRun` refuses non-terminals, and `settleTerminal`
// only fires on the dispatch path. The parent of every recovery fork leaked its
// claimed row the same way (the fork gets its OWN row from reconcileQueueRows).
// The live-world copy carried 16 such rows.
//
// The fix is level-triggered and scheduler-owned (S10.7 keeps the queue family
// single-owner): the claim pass settles any queued/claimed row whose run has
// ENDED — which self-heals rows that predate the fix, and is why these tests
// seed the leak rather than provoking it through a dispatch.

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// idleDispatcher claims and does nothing else: the run stays `claimed`, which is
// exactly the state a leaked queue row is written in.
type idleDispatcher struct{}

func (idleDispatcher) Dispatch(context.Context, run.Run) error { return nil }

func (e *schedEnv) queueStatus(t *testing.T, runID string) string {
	t.Helper()
	var status string
	err := e.db.QueryRowContext(context.Background(),
		`SELECT status FROM queue WHERE run_id = ? ORDER BY queue_id DESC LIMIT 1`, runID).Scan(&status)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		t.Fatalf("queue status %s: %v", runID, err)
	}
	return status
}

func (e *schedEnv) receiptCount(t *testing.T, runID string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM receipts WHERE run_id = ?`, runID).Scan(&n); err != nil {
		t.Fatalf("count receipts %s: %v", runID, err)
	}
	return n
}

// journal builds the effect journal the ladder requires (the scheduler itself
// has no use for one).
func (e *schedEnv) journal(t *testing.T) *gates.Journal {
	t.Helper()
	j, err := gates.NewJournal(gates.JournalConfig{DB: e.db, Settings: e.reg})
	if err != nil {
		t.Fatalf("gates.NewJournal: %v", err)
	}
	return j
}

func (e *schedEnv) drive(t *testing.T, runID string, states ...run.State) {
	t.Helper()
	for _, st := range states {
		if _, err := e.runs.Transition(context.Background(), runID, st, run.TransitionOptions{
			Reason: "fixture", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("drive %s→%s: %v", runID, st, err)
		}
	}
}

func TestLadderTerminalQueueRowsSettle(t *testing.T) {
	ctx := context.Background()
	e := newSchedEnv(t)
	s := e.scheduler(t, idleDispatcher{})

	// Each run gets its OWN owner so one claim pass can claim them all under the
	// per-(user, lane) cap of 1.
	type seed struct {
		id     string
		user   string
		states []run.State // driven AFTER the claim
	}
	seeds := []seed{
		{"t-done.execute", "u-done", []run.State{run.StateRunning, run.StateCompleted}},
		{"t-crash.execute", "u-crash", []run.State{run.StateCrashed}},
		{"t-final.execute", "u-final", []run.State{run.StateCrashed, run.StateFinalized}},
		{"t-tomb.execute", "u-tomb", []run.State{run.StateCrashed, run.StateTombstoned}},
		{"t-gate.execute", "u-gate", []run.State{run.StateRunning, run.StateParked, run.StateDiedAtGate}},
		// The two rows that must be LEFT ALONE: an open-gate park (the
		// documented admission that keeps its claim) and a live queued row.
		{"t-park.execute", "u-park", []run.State{run.StateRunning, run.StateParked}},
	}
	for _, sd := range seeds {
		e.newRun(t, sd.id, sd.user, "anthropic")
		if err := s.Enqueue(ctx, sd.id, ClassInteractive); err != nil {
			t.Fatalf("enqueue %s: %v", sd.id, err)
		}
	}
	// The control that never gets claimed: its lane is serialized to 0.
	e.newRun(t, "t-waiting.execute", "u-wait", "anthropic")
	if err := s.SetLaneCap(ctx, "u-wait", "anthropic", 0); err != nil {
		t.Fatalf("SetLaneCap: %v", err)
	}
	if err := s.Enqueue(ctx, "t-waiting.execute", ClassInteractive); err != nil {
		t.Fatalf("enqueue control: %v", err)
	}

	if n, err := s.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	} else if n != len(seeds) {
		t.Fatalf("claim pass dispatched %d, want %d", n, len(seeds))
	}
	s.WaitInFlight()
	for _, sd := range seeds {
		if got := e.queueStatus(t, sd.id); got != queueClaimed {
			t.Fatalf("%s queue row = %q before the ladder ran, want claimed", sd.id, got)
		}
		e.drive(t, sd.id, sd.states...)
	}

	// ONE claim pass — the settlement is level-triggered, not event-driven.
	if _, err := s.Tick(ctx); err != nil {
		t.Fatalf("settling Tick: %v", err)
	}
	s.WaitInFlight()

	for _, id := range []string{"t-done.execute", "t-crash.execute", "t-final.execute",
		"t-tomb.execute", "t-gate.execute"} {
		if got := e.queueStatus(t, id); got != queueDone {
			t.Errorf("%s queue row = %q, want done — an ended run's admission is over (S10.7)", id, got)
		}
	}
	if got := e.queueStatus(t, "t-park.execute"); got != queueClaimed {
		t.Errorf("the open-gate park's row = %q, want claimed — that admission is documented and its resume continues under it", got)
	}
	if got := e.queueStatus(t, "t-waiting.execute"); got != queueQueued {
		t.Errorf("the waiting run's row = %q, want queued — a live admission is untouched", got)
	}

	// Receipts: the four IsTerminal states materialize one each; a CRASHED run
	// settles queue-only (its lineage's spend rides its checkpoints, and each
	// successor ends with its own receipt) — never a fabricated measurement (D4).
	for _, id := range []string{"t-done.execute", "t-final.execute", "t-tomb.execute", "t-gate.execute"} {
		if got := e.receiptCount(t, id); got != 1 {
			t.Errorf("%s has %d receipts, want exactly 1", id, got)
		}
	}
	if got := e.receiptCount(t, "t-crash.execute"); got != 0 {
		t.Errorf("the crashed run has %d receipts, want 0 (queue-only settle)", got)
	}

	// Idempotence: a second pass changes nothing at all.
	if _, err := s.Tick(ctx); err != nil {
		t.Fatalf("second settling Tick: %v", err)
	}
	s.WaitInFlight()
	for _, id := range []string{"t-done.execute", "t-final.execute", "t-tomb.execute", "t-gate.execute"} {
		if got := e.receiptCount(t, id); got != 1 {
			t.Errorf("%s has %d receipts after a second pass, want 1 (idempotent)", id, got)
		}
	}
	if got := e.queueStatus(t, "t-park.execute"); got != queueClaimed {
		t.Errorf("the parked run's row moved to %q on a later pass", got)
	}
}

func TestForkParentQueueRowSettles(t *testing.T) {
	ctx := context.Background()
	e := newSchedEnv(t)
	s := e.scheduler(t, idleDispatcher{})

	e.newRun(t, "t-fork.execute", "u-fork", "anthropic")
	if err := s.Enqueue(ctx, "t-fork.execute", ClassInteractive); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := s.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	s.WaitInFlight()
	e.drive(t, "t-fork.execute", run.StateCrashed)

	// The REAL ladder supersedes it (recovery imports no scheduler; the
	// composition is the shell's, and this test stands in for it).
	l, err := recovery.New(recovery.Config{
		DB: e.db, Log: e.log, Runs: e.runs, Checkpoints: e.cps,
		Effects: e.journal(t), Settings: e.reg, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	if rpt, err := l.ReconcilePass(ctx); err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	} else if rpt.Forked != 1 {
		t.Fatalf("pass forked %d, want 1", rpt.Forked)
	}
	parent, err := e.runs.Get(ctx, "t-fork.execute")
	if err != nil {
		t.Fatal(err)
	}
	forkID := "t-fork.execute.g" + itoa(parent.Generation)

	if _, err := s.Tick(ctx); err != nil {
		t.Fatalf("settling Tick: %v", err)
	}
	s.WaitInFlight()

	if got := e.queueStatus(t, "t-fork.execute"); got != queueDone {
		t.Errorf("the superseded parent's queue row = %q, want done — the fork re-admits under its OWN row", got)
	}
	if got := e.queueStatus(t, forkID); got == "" {
		t.Fatalf("the fork %s got no queue row (reconcileQueueRows)", forkID)
	}
	fork, err := e.runs.Get(ctx, forkID)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if fork.State != run.StateClaimed {
		t.Errorf("the fork is %s, want claimed — the settle must not starve it", fork.State)
	}
	var rows int
	if err := e.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM queue WHERE run_id IN (?, ?) AND status IN (?, ?)`,
		"t-fork.execute", forkID, queueQueued, queueClaimed).Scan(&rows); err != nil {
		t.Fatalf("count live rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("%d live queue rows across the lineage, want exactly 1 (the fork's)", rows)
	}
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
