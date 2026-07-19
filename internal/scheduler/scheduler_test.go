package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

type schedEnv struct {
	db   *storage.DB
	log  *eventlog.Log
	reg  *settings.Registry
	runs *run.Store
	cps  *gates.Checkpoints
}

func newSchedEnv(t *testing.T) *schedEnv {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	return &schedEnv{db: db, log: log, reg: reg, runs: run.NewStore(db, log), cps: gates.NewCheckpoints(db, log)}
}

func (e *schedEnv) newRun(t *testing.T, id, user, lane string) {
	t.Helper()
	if _, err := e.runs.Create(context.Background(), run.NewRun{ID: id, UserID: user, Lane: lane, Substrate: "claude-cli"}); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
}

// fakeDispatcher drives a claimed run through claimed→running→(hold)→completed,
// writing one checkpoint so the receipt has content. hold blocks each dispatch
// in running until release is closed, so lane-slot occupancy is observable.
type fakeDispatcher struct {
	runs    *run.Store
	cps     *gates.Checkpoints
	hold    bool
	release chan struct{}

	mu         sync.Mutex
	dispatched []string
}

func (d *fakeDispatcher) Dispatch(ctx context.Context, r run.Run) error {
	// Record before the transition so an observer that sees state==running is
	// guaranteed to also see the dispatch recorded.
	d.mu.Lock()
	d.dispatched = append(d.dispatched, r.ID)
	d.mu.Unlock()
	if _, err := d.runs.Transition(ctx, r.ID, run.StateRunning, run.TransitionOptions{Reason: "fake engine start", Actor: run.ActorPlatform}); err != nil {
		return err
	}
	if d.hold {
		<-d.release
	}
	// If the run was parked out from under us (maintenance drain, S01.6),
	// respect it — the disposition is the scheduler's, never forced here.
	cur, err := d.runs.Get(ctx, r.ID)
	if err != nil {
		return err
	}
	if cur.State != run.StateRunning {
		return nil
	}
	if d.cps != nil {
		if _, err := d.cps.Write(ctx, gates.NewCheckpoint{
			RunID: r.ID, ModelID: "claude-haiku-4-5",
			Usage: json.RawMessage(`{"input_tokens":10,"output_tokens":4}`), SessionSubstrate: "claude-cli",
		}); err != nil {
			return err
		}
	}
	_, err = d.runs.Transition(ctx, r.ID, run.StateCompleted, run.TransitionOptions{Reason: "fake engine done", Actor: run.ActorPlatform})
	return err
}

func (e *schedEnv) scheduler(t *testing.T, disp Dispatcher) *Scheduler {
	t.Helper()
	led := metering.NewLedger(e.db, nil, metering.NoMeteredExceptions(), e.reg)
	s, err := New(Config{
		DB: e.db, Runs: e.runs, Settings: e.reg, Dispatcher: disp,
		Pressure: metering.NewPressureGauge(e.db, e.reg),
		Receipts: metering.NewReceipts(e.db, led, metering.NoMeteredExceptions()),
		Logger:   discardLogger(),
	})
	if err != nil {
		t.Fatalf("New scheduler: %v", err)
	}
	return s
}

func TestEnqueueClaimDispatchCompletesWithReceipt(t *testing.T) {
	e := newSchedEnv(t)
	ctx := context.Background()
	e.newRun(t, "r1", "alice", "anthropic")

	disp := &fakeDispatcher{runs: e.runs, cps: e.cps}
	s := e.scheduler(t, disp)

	if err := s.Enqueue(ctx, "r1", ClassInteractive); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Owner-attributed queue row (15.6).
	var qUser, qStatus, qLane string
	if err := e.db.QueryRowContext(ctx, `SELECT user_id, status, priority_lane FROM queue WHERE run_id='r1'`).Scan(&qUser, &qStatus, &qLane); err != nil {
		t.Fatal(err)
	}
	if qUser != "alice" || qStatus != queueQueued || qLane != string(ClassInteractive) {
		t.Fatalf("queue row = user %q status %q class %q", qUser, qStatus, qLane)
	}

	n, err := s.Tick(ctx)
	if err != nil || n != 1 {
		t.Fatalf("Tick dispatched %d err=%v, want 1", n, err)
	}
	s.WaitInFlight()

	r, _ := e.runs.Get(ctx, "r1")
	if r.State != run.StateCompleted {
		t.Fatalf("run state %s, want completed", r.State)
	}
	// Queue row settled to done.
	if err := e.db.QueryRowContext(ctx, `SELECT status FROM queue WHERE run_id='r1'`).Scan(&qStatus); err != nil {
		t.Fatal(err)
	}
	if qStatus != queueDone {
		t.Errorf("queue status %q, want done", qStatus)
	}
	// Receipt materialized at run-end (S10.1/S10.10).
	var receipts int
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM receipts WHERE run_id='r1'`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Errorf("receipts = %d, want 1", receipts)
	}
	// A claimed→running→completed lease was established (owner attribution).
	if r.HeartbeatSeq != 0 { // heartbeat set only by live dispatch; fake leaves it 0
		t.Logf("heartbeat seq %d", r.HeartbeatSeq)
	}
}

func TestLaneCapSerializes(t *testing.T) {
	e := newSchedEnv(t)
	ctx := context.Background()
	e.newRun(t, "r1", "alice", "anthropic")
	e.newRun(t, "r2", "alice", "anthropic")

	disp := &fakeDispatcher{runs: e.runs, cps: e.cps, hold: true, release: make(chan struct{})}
	s := e.scheduler(t, disp)
	// Cap the (alice, anthropic) lane at 1 concurrent run (Spec S10.7 slots).
	if err := s.SetLaneCap(ctx, "alice", "anthropic", 1); err != nil {
		t.Fatalf("SetLaneCap: %v", err)
	}
	if err := s.Enqueue(ctx, "r1", ClassBackground); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(ctx, "r2", ClassBackground); err != nil {
		t.Fatal(err)
	}

	// First pass claims exactly one — the lane slot is then full.
	if n, err := s.Tick(ctx); err != nil || n != 1 {
		t.Fatalf("Tick 1 = %d err=%v, want 1", n, err)
	}
	// Second pass claims nothing: the one slot is occupied (claimed/running).
	if n, err := s.Tick(ctx); err != nil || n != 0 {
		t.Fatalf("Tick 2 = %d err=%v, want 0 (lane full)", n, err)
	}
	// Release the held run: it completes and frees the slot.
	close(disp.release)
	s.WaitInFlight()
	// Now the second run is claimable.
	if n, err := s.Tick(ctx); err != nil || n != 1 {
		t.Fatalf("Tick 3 = %d err=%v, want 1 (slot freed)", n, err)
	}
	s.WaitInFlight()

	disp.mu.Lock()
	defer disp.mu.Unlock()
	if len(disp.dispatched) != 2 {
		t.Fatalf("dispatched %v, want both runs", disp.dispatched)
	}
}

func TestPriorityInteractiveBeforeBackground(t *testing.T) {
	e := newSchedEnv(t)
	ctx := context.Background()
	e.newRun(t, "bg", "alice", "anthropic")
	e.newRun(t, "iv", "alice", "anthropic")

	disp := &fakeDispatcher{runs: e.runs, cps: e.cps, hold: true, release: make(chan struct{})}
	s := e.scheduler(t, disp)
	if err := s.SetLaneCap(ctx, "alice", "anthropic", 1); err != nil {
		t.Fatal(err)
	}
	// Enqueue background first, then interactive — priority, not arrival, wins.
	if err := s.Enqueue(ctx, "bg", ClassBackground); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(ctx, "iv", ClassInteractive); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Tick(ctx); err != nil || n != 1 {
		t.Fatalf("Tick = %d err=%v", n, err)
	}
	waitState(t, e.runs, "iv", run.StateRunning) // the claimed run is the interactive one
	disp.mu.Lock()
	first := disp.dispatched[0]
	disp.mu.Unlock()
	if first != "iv" {
		t.Fatalf("first dispatched = %q, want iv (interactive first, 3.3)", first)
	}
	close(disp.release)
	s.WaitInFlight()
}

func TestReconcileMaterializesQueueRowForRecoverySuccessor(t *testing.T) {
	e := newSchedEnv(t)
	ctx := context.Background()
	// A recovery fork successor enters 'queued' directly with no queue row
	// (Spec S02.5 step 3; B0-4 note: queue-row materialization is S10's).
	if _, err := e.runs.Create(ctx, run.NewRun{ID: "succ", UserID: "bob", Lane: "anthropic", Substrate: "claude-cli", State: run.StateQueued}); err != nil {
		t.Fatalf("Create queued: %v", err)
	}
	disp := &fakeDispatcher{runs: e.runs, cps: e.cps}
	s := e.scheduler(t, disp)

	if _, err := s.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	s.WaitInFlight()
	// The scheduler materialized a queue row (owner-attributed) and drove it.
	var owner string
	if err := e.db.QueryRowContext(ctx, `SELECT user_id FROM queue WHERE run_id='succ'`).Scan(&owner); err != nil {
		t.Fatalf("no queue row materialized for the successor: %v", err)
	}
	if owner != "bob" {
		t.Errorf("queue owner = %q, want bob", owner)
	}
	if r, _ := e.runs.Get(ctx, "succ"); r.State != run.StateCompleted {
		t.Errorf("successor state = %s, want completed", r.State)
	}
}

func TestEnqueueIdempotentAndRejectsBadState(t *testing.T) {
	e := newSchedEnv(t)
	ctx := context.Background()
	e.newRun(t, "r1", "alice", "anthropic")
	s := e.scheduler(t, &fakeDispatcher{runs: e.runs, cps: e.cps})

	if err := s.Enqueue(ctx, "r1", ClassBackground); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(ctx, "r1", ClassBackground); err != nil {
		t.Fatalf("re-enqueue of a queued run: %v", err)
	}
	var rows int
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue WHERE run_id='r1'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("queue rows = %d, want 1 (idempotent)", rows)
	}
	// Enqueuing a completed run is rejected (not new/queued).
	if _, err := e.runs.Transition(ctx, "r1", run.StateClaimed, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.runs.Transition(ctx, "r1", run.StateRunning, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.runs.Transition(ctx, "r1", run.StateCompleted, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(ctx, "r1", ClassBackground); err == nil {
		t.Fatal("enqueue of a completed run must be rejected")
	}
}

func TestParkInFlightRuns(t *testing.T) {
	e := newSchedEnv(t)
	ctx := context.Background()
	e.newRun(t, "r1", "alice", "anthropic")
	disp := &fakeDispatcher{runs: e.runs, cps: e.cps, hold: true, release: make(chan struct{})}
	s := e.scheduler(t, disp)

	if err := s.Enqueue(ctx, "r1", ClassBackground); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	// Wait until the dispatch goroutine has the run in 'running' and is held.
	waitState(t, e.runs, "r1", run.StateRunning)

	// Maintenance drain-grace expiry parks in-flight runs (never a kill, S01.6).
	if err := s.ParkInFlightRuns(ctx); err != nil {
		t.Fatalf("ParkInFlightRuns: %v", err)
	}
	if r, _ := e.runs.Get(ctx, "r1"); r.State != run.StateParked {
		t.Fatalf("run state %s, want parked", r.State)
	}
	// Release the held dispatch so the goroutine unwinds; its terminal
	// transition (running→completed) is now an invalid edge (parked) and is
	// swallowed, leaving the run parked.
	close(disp.release)
	s.WaitInFlight()
	if r, _ := e.runs.Get(ctx, "r1"); r.State != run.StateParked {
		t.Errorf("run state %s after release, want still parked", r.State)
	}
}
