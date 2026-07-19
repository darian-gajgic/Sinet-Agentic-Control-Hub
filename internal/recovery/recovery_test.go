package recovery_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

func TestAliveRunUntouched(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	mustRunning(t, s, "r1")
	l := newLadder(t, s, nil, fakeUnits{"r1": {State: recovery.UnitActive}}, nil)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Alive != 1 || rpt.Forked != 0 || rpt.Wedged != 0 {
		t.Fatalf("report = %+v, want 1 alive", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateRunning {
		t.Fatalf("state = %s, want running (untouched)", r.State)
	}
}

func TestWedgedFlaggedOnceNeverKilled(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	mustRunning(t, s, "r1")
	clock := &fakeClock{t: time.Now().Add(10 * time.Minute)} // cursor stalled past ⚙ recovery.dead_after (5 m) + wake grace
	l := newLadder(t, s, clock, fakeUnits{"r1": {State: recovery.UnitActive}}, nil)

	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	if rpt.Wedged != 1 {
		t.Fatalf("report = %+v, want 1 wedged", rpt)
	}
	// Pause-and-flag, never auto-kill (D1.3): still stored running.
	if r := getRun(t, s, "r1"); r.State != run.StateRunning {
		t.Fatalf("state = %s, want running", r.State)
	}
	if got := countType(t, s, "r1", run.EventWedged); got != 1 {
		t.Fatalf("wedged events = %d, want 1", got)
	}
	// The flag is once per stall episode, not per sweep.
	clock.advance(5 * time.Minute)
	if _, err := l.ReconcilePass(ctx); err != nil {
		t.Fatalf("pass 2: %v", err)
	}
	if got := countType(t, s, "r1", run.EventWedged); got != 1 {
		t.Fatalf("wedged events after second pass = %d, want 1", got)
	}
}

func TestFinishedDuringOutageHarvested(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	mustRunning(t, s, "r1")
	recs := []recovery.HarvestRecord{
		{SessionID: "sess-1", MessageID: "m1", Payload: json.RawMessage(`{"text":"step"}`)},
		{SessionID: "sess-1", MessageID: "m2", Terminal: true, Payload: json.RawMessage(`{"text":"done"}`), Usage: json.RawMessage(`{"output_tokens":9}`)},
	}
	l := newLadder(t, s, nil,
		fakeUnits{"r1": {State: recovery.UnitCorpseExit0, Result: "exited"}},
		fakeHarvest{"r1": recs})

	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Harvested != 1 {
		t.Fatalf("report = %+v, want 1 harvested", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateCompleted {
		t.Fatalf("state = %s, want completed (deliver the result instead of redoing the work)", r.State)
	}
	if got := countType(t, s, "r1", run.EventHarvest); got != 2 {
		t.Fatalf("harvest events = %d, want 2", got)
	}
	// Idempotent: a replayed pass re-appends nothing (dedup by
	// (session_id, message_id)) and completed runs are not rescanned.
	if _, err := l.ReconcilePass(ctx); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if got := countType(t, s, "r1", run.EventHarvest); got != 2 {
		t.Fatalf("harvest events after second pass = %d, want 2 (dedup)", got)
	}
}

func TestDeadForkFromCheckpointAndFencing(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	mustRunning(t, s, "r1")
	cp, err := s.cps.Write(ctx, mkCheckpoint("r1"))
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	l := newLadder(t, s, nil, nil, nil) // B0 defaults: unit gone, nothing to harvest

	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Forked != 1 {
		t.Fatalf("report = %+v, want 1 forked", rpt)
	}
	parent := getRun(t, s, "r1")
	if parent.State != run.StateCrashed || parent.Generation != 1 {
		t.Fatalf("parent = %s gen %d, want crashed gen 1 (takeover fence)", parent.State, parent.Generation)
	}
	succ := getRun(t, s, "r1.g1")
	if succ.State != run.StateQueued || succ.Generation != 1 ||
		succ.ParentRunID != "r1" || succ.DispatchID != "r1#g1" || succ.RecoveryAttempts != 1 {
		t.Fatalf("successor = %+v", succ)
	}
	// The fork event cites the checkpoint (re-spend bounded to
	// work-since-last-checkpoint, D7).
	ev := lastOfType(t, s, "r1.g1", run.EventForked)
	var p struct {
		Detail struct {
			FromCheckpointID  int64 `json:"from_checkpoint_id"`
			FromCheckpointSeq int64 `json:"from_checkpoint_seq"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("fork payload: %v", err)
	}
	if p.Detail.FromCheckpointID != cp.ID || p.Detail.FromCheckpointSeq != cp.EventSeq {
		t.Fatalf("fork cites checkpoint %d/%d, want %d/%d", p.Detail.FromCheckpointID, p.Detail.FromCheckpointSeq, cp.ID, cp.EventSeq)
	}
	// Fenced double-resume is inert: the old incarnation's generation is
	// stale (Spec S02.5 step 4, P-T02-3).
	_, err = s.log.Append(ctx, eventlog.Append{
		RunID: "r1", Generation: 0, UserID: "u1", Type: "zombie.write",
		SchemaVersion: 1, Payload: json.RawMessage(`{}`),
	})
	if !errors.Is(err, eventlog.ErrStaleGeneration) {
		t.Fatalf("stale append err = %v, want ErrStaleGeneration", err)
	}
	// Re-running the pass does not double-fork (one durable dispatch id per
	// interruption).
	rpt, err = l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if rpt.Forked != 0 {
		t.Fatalf("second pass forked %d, want 0", rpt.Forked)
	}
}

func TestStaleInterruptionFinalized(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	mustRunning(t, s, "r1")
	clock := &fakeClock{t: time.Now().Add(25 * time.Hour)} // interrupted past ⚙ recovery.stale_finalize (24 h)
	l := newLadder(t, s, clock, nil, nil)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Finalized != 1 || rpt.Forked != 0 {
		t.Fatalf("report = %+v, want 1 finalized", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateFinalized {
		t.Fatalf("state = %s, want finalized (finalize-with-card, Spec S02.5 step 3)", r.State)
	}
}

func TestRepeatOffenderTombstoned(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	mustRunning(t, s, "r1")
	// Lineage already at ⚙ recovery.max_attempts (3) forks.
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE runs SET recovery_attempts = 3 WHERE run_id = 'r1'`)
		return err
	})
	if err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	l := newLadder(t, s, nil, nil, nil)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Tombstoned != 1 || rpt.Forked != 0 {
		t.Fatalf("report = %+v, want 1 tombstoned", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateTombstoned {
		t.Fatalf("state = %s, want tombstoned (repeat offender, Spec S02.5 step 3)", r.State)
	}
}

// TestSuspendCycleWakeGrace is the S02.9 companion test: inject clock
// deltas and assert the suspend-aware lease evaluation — on wake, ⚙
// recovery.wake_grace applies before any lease is declared dead (P-T07-4).
func TestSuspendCycleWakeGrace(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	t0 := time.Now()
	if _, err := s.runs.Create(ctx, run.NewRun{ID: "r1", UserID: "u1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := s.runs.Transition(ctx, "r1", to, run.TransitionOptions{}); err != nil {
			t.Fatalf("→%s: %v", to, err)
		}
	}
	deadline := t0.Add(time.Hour)
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE runs SET lease_holder = 'w1', lease_deadline_ts = ? WHERE run_id = 'r1'`,
			deadline.UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	clock := &fakeClock{t: t0}
	l := newLadder(t, s, clock, nil, nil)

	// Pass 1 at t0: first pass (crash-equivalent start) — lease live.
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	if !rpt.GraceApplied || rpt.Pending != 1 || rpt.Forked != 0 {
		t.Fatalf("pass 1 = %+v, want grace applied, 1 pending", rpt)
	}

	// Nap: the wall clock jumps past the deadline. The lease is 60 s
	// expired but within ⚙ recovery.wake_grace (120 s) — NOT dead.
	clock.t = deadline.Add(60 * time.Second)
	rpt, err = l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("pass 2: %v", err)
	}
	if !rpt.GraceApplied {
		t.Fatal("pass 2: clock jump not detected as wake")
	}
	if rpt.Pending != 1 || rpt.Forked != 0 {
		t.Fatalf("pass 2 = %+v, want lease honored within wake grace", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateClaimed {
		t.Fatalf("state after wake = %s, want claimed (never declared dead inside the grace)", r.State)
	}

	// Next sweep, 5 minutes later (no jump): grace is over, the holder
	// never re-asserted — DEAD → crashed + forked.
	clock.advance(5 * time.Minute)
	rpt, err = l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("pass 3: %v", err)
	}
	if rpt.GraceApplied {
		t.Fatal("pass 3: grace still applied without a jump")
	}
	if rpt.Forked != 1 {
		t.Fatalf("pass 3 = %+v, want 1 forked", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateCrashed {
		t.Fatalf("state = %s, want crashed", r.State)
	}
	if r := getRun(t, s, "r1.g1"); r.State != run.StateQueued || r.Generation != 1 {
		t.Fatalf("successor = %+v, want queued gen 1", r)
	}
}

func TestAsksReObserved(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	r := mustRunning(t, s, "r1")
	if _, err := s.runs.Transition(ctx, "r1", run.StateParked, run.TransitionOptions{Reason: "gate"}); err != nil {
		t.Fatalf("park: %v", err)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts, engine_expiry_ts)
			 VALUES ('a1', ?, ?, '{"invocation":"full"}', 'open', ?, ?)`,
			r.ID, r.UserID, past, past); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
			 VALUES ('a2', ?, ?, '{"invocation":"full"}', 'open', ?)`,
			r.ID, r.UserID, past)
		return err
	})
	if err != nil {
		t.Fatalf("seed asks: %v", err)
	}
	l := newLadder(t, s, nil, nil, nil)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.AsksObserved != 2 || rpt.AsksEngineExpired != 1 {
		t.Fatalf("report = %+v, want 2 observed / 1 engine-expired", rpt)
	}
	// The rows are authoritative and untouched (re-issue is B1/B2).
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM asks WHERE answered_ts IS NULL`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("open asks = %d (%v), want 2", n, err)
	}
}

// countType counts run events of one type.
func countType(t *testing.T, s *stack, runID, typ string) int {
	t.Helper()
	var n int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ?`, runID, typ).Scan(&n)
	if err != nil {
		t.Fatalf("countType: %v", err)
	}
	return n
}

// lastOfType returns the newest event of one type for a run.
func lastOfType(t *testing.T, s *stack, runID, typ string) eventlog.Event {
	t.Helper()
	evs, err := s.log.After(context.Background(), 0, 10000)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].RunID == runID && evs[i].Type == typ {
			return evs[i]
		}
	}
	t.Fatalf("no %s event for %s", typ, runID)
	return eventlog.Event{}
}
