package stage

// cancel_internal_test.go — the ratified S02.3 cancel mapping (feature 4.5)
// asserted against REAL FSM rows, a REAL queue and a fake engine session. $0:
// nothing here spawns a process or dials anything.
//
// The last test in the file is the NO-AUTO-KILL assertion: a source scan
// proving the cancel machinery is reachable from the HTTP verbs alone.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// fakeSession is a live engine session that records the S03.1 ladder running
// on it. Cancel is the ONLY verb the cancel path may reach.
type fakeSession struct {
	cancelled int
	paused    int
	err       error
}

func (f *fakeSession) Events() <-chan adapters.Event {
	ch := make(chan adapters.Event)
	close(ch)
	return ch
}
func (f *fakeSession) Cursor() adapters.Cursor { return adapters.Cursor{} }
func (f *fakeSession) Fingerprint() string     { return "fp" }
func (f *fakeSession) Pause(context.Context) error {
	f.paused++
	return nil
}
func (f *fakeSession) Cancel(context.Context) error {
	f.cancelled++
	return f.err
}
func (f *fakeSession) Wait(context.Context) (adapters.Outcome, error) {
	return adapters.Outcome{Kind: adapters.OutcomeCanceled}, nil
}

// cancelEnv is a real spine with no engine: the FSM store, the queue, and a
// skeleton whose live-session registry the test fills by hand.
type cancelEnv struct {
	db    *storage.DB
	log   *eventlog.Log
	runs  *run.Store
	sk    *Skeleton
	sched *scheduler.Scheduler
}

func newCancelEnv(t *testing.T) *cancelEnv {
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
	runs := run.NewStore(db, log)
	root := t.TempDir()
	sk, err := New(Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		// One registered adapter satisfies the constructor; no session is ever
		// spawned in this file.
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: nopAdapter{}},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour, // manual only: nothing ticks in this file
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	return &cancelEnv{db: db, log: log, runs: runs, sk: sk, sched: sched}
}

type nopAdapter struct{}

func (nopAdapter) Substrate() string { return adapters.SubstrateClaudeCLI }
func (nopAdapter) Start(context.Context, adapters.StartRequest) (adapters.Session, error) {
	return nil, errors.New("no engine in the cancel tests ($0)")
}
func (nopAdapter) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("no engine in the cancel tests ($0)")
}

// seedRun creates a task + run and walks it to the wanted state through the
// ratified edges, so the fixture is never a hand-written illegal row.
func (e *cancelEnv) seedRun(t *testing.T, runID, taskID, owner string, to run.State) run.Run {
	t.Helper()
	ctx := context.Background()
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tasks (task_id, user_id, title, created_ts) VALUES (?,?,?,?)`,
			taskID, owner, "T", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	r, err := e.runs.Create(ctx, run.NewRun{ID: runID, UserID: owner, TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: "anthropic"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	path := map[run.State][]run.State{
		run.StateNew:       {},
		run.StateQueued:    {run.StateQueued},
		run.StateClaimed:   {run.StateQueued, run.StateClaimed},
		run.StateRunning:   {run.StateQueued, run.StateClaimed, run.StateRunning},
		run.StateParked:    {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked},
		run.StateDraining:  {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateDraining},
		run.StateCompleted: {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateCompleted},
	}[to]
	for _, st := range path {
		if r, err = e.runs.Transition(ctx, runID, st, run.TransitionOptions{Reason: "fixture", Actor: "platform"}); err != nil {
			t.Fatalf("fixture transition %s: %v", st, err)
		}
	}
	return r
}

func (e *cancelEnv) state(t *testing.T, runID string) run.State {
	t.Helper()
	r, err := e.runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run %q: %v", runID, err)
	}
	return r.State
}

func (e *cancelEnv) kanban(t *testing.T, taskID string) string {
	t.Helper()
	var k string
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT kanban_status FROM tasks WHERE task_id = ?`, taskID).Scan(&k); err != nil {
		t.Fatalf("read kanban: %v", err)
	}
	return k
}

// countEvents counts one run's events of a type.
func (e *cancelEnv) countEvents(t *testing.T, runID, typ string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ?`, runID, typ).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", typ, err)
	}
	return n
}

// setKanbanFixture puts a task in a known board column.
func (e *cancelEnv) setKanbanFixture(t *testing.T, taskID, status string) {
	t.Helper()
	ctx := context.Background()
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET kanban_status = ? WHERE task_id = ?`, status, taskID)
		return err
	}); err != nil {
		t.Fatalf("set kanban: %v", err)
	}
}

// lastTransition returns the payload of the run's most recent state change.
func (e *cancelEnv) lastTransition(t *testing.T, runID string) map[string]any {
	t.Helper()
	var payload string
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq DESC LIMIT 1`,
		runID, run.EventState).Scan(&payload); err != nil {
		t.Fatalf("read transition: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatalf("decode transition: %v", err)
	}
	return m
}

// TestCancelRunningInvokesTheLadderThenCompletes is the running limb: the S03.1
// ladder runs on the LIVE session, and the run completes carrying the cancel
// reason (§14 reading 9).
func TestCancelRunningInvokesTheLadderThenCompletes(t *testing.T) {
	e := newCancelEnv(t)
	r := e.seedRun(t, "t-1.execute", "t-1", "alice", run.StateRunning)
	sess := &fakeSession{}
	e.sk.cancels.register(r.ID, sess)

	out, err := e.sk.CancelRun(context.Background(), "alice", r.ID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	switch {
	case sess.cancelled != 1:
		t.Errorf("the cancel ladder ran %d times on the live session, want exactly 1 (S03.1)", sess.cancelled)
	case sess.paused != 0:
		t.Error("the cancel path must not use the pause verb — the ladder is Cancel's")
	case !out.LadderInvoked || !out.Applied:
		t.Errorf("outcome = %+v", out)
	case e.state(t, r.ID) != run.StateCompleted:
		t.Errorf("running cancel landed on %s, want completed (§14 reading 9)", e.state(t, r.ID))
	}
	tr := e.lastTransition(t, r.ID)
	if tr["to"] != "completed" || !strings.Contains(tr["reason"].(string), "cancelled by alice") {
		t.Errorf("the transition must carry the cancel reason and its actor; got %v", tr)
	}
	if tr["actor"] != "alice" {
		t.Errorf("the acting principal must be the person, not the platform; got %v", tr["actor"])
	}
	if e.kanban(t, "t-1") != "cancelled" {
		t.Errorf("kanban = %q, want cancelled", e.kanban(t, "t-1"))
	}
	// A repeat is the honest already-ended answer, never a second cancellation.
	again, err := e.sk.CancelRun(context.Background(), "alice", r.ID)
	if err != nil {
		t.Fatalf("repeat cancel: %v", err)
	}
	if again.Applied || sess.cancelled != 1 {
		t.Errorf("a repeat re-fired the ladder (%d) or re-applied (%v)", sess.cancelled, again.Applied)
	}
}

// TestCancelDrainingCompletesLikeRunning: draining is running finishing its
// current paid call, so it lands on the same terminal.
func TestCancelDrainingCompletesLikeRunning(t *testing.T) {
	e := newCancelEnv(t)
	r := e.seedRun(t, "t-2.execute", "t-2", "alice", run.StateDraining)
	if _, err := e.sk.CancelRun(context.Background(), "alice", r.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if got := e.state(t, r.ID); got != run.StateCompleted {
		t.Errorf("draining cancel landed on %s, want completed", got)
	}
}

// TestCancelParkedFinalizesWithItsCard: parked → finalized, generation
// UNCHANGED (no resume edge was taken), and the open card closes in the same
// act so it cannot outlive the run it was parked on.
func TestCancelParkedFinalizesWithItsCard(t *testing.T) {
	e := newCancelEnv(t)
	r := e.seedRun(t, "t-3.verify", "t-3", "alice", run.StateParked)
	genBefore := r.Generation
	ctx := context.Background()
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
			"ask-3", r.ID, "alice", "{}", "open", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed ask: %v", err)
	}

	if _, err := e.sk.CancelRun(ctx, "alice", r.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if got := e.state(t, r.ID); got != run.StateFinalized {
		t.Errorf("parked cancel landed on %s, want finalized (finalize-with-card)", got)
	}
	after, _ := e.runs.Get(ctx, r.ID)
	if after.Generation != genBefore {
		t.Errorf("generation moved %d→%d: a cancel takes no resume edge (§14 reading 9)", genBefore, after.Generation)
	}
	var status string
	var answered *string
	if err := e.db.QueryRowContext(ctx,
		`SELECT status, answered_ts FROM asks WHERE ask_id = 'ask-3'`).Scan(&status, &answered); err != nil {
		t.Fatalf("read ask: %v", err)
	}
	if status != "cancelled" || answered == nil {
		t.Errorf("the open card must close with the run; got status %q answered %v", status, answered)
	}
}

// TestCancelQueuedFinalizesAndSettlesTheQueueRow is the OQ1 edge: a run that
// never ran finalizes, and its queue row settles in the SAME transaction so the
// claim loop can never pick it up afterwards.
func TestCancelQueuedFinalizesAndSettlesTheQueueRow(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	e.seedRun(t, "t-4.execute", "t-4", "alice", run.StateNew)
	if err := e.sched.Enqueue(ctx, "t-4.execute", scheduler.ClassBackground); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if got := e.state(t, "t-4.execute"); got != run.StateQueued {
		t.Fatalf("fixture: run is %s, want queued", got)
	}

	out, err := e.sk.CancelRun(ctx, "alice", "t-4.execute")
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if !out.Applied || out.To != string(run.StateFinalized) {
		t.Fatalf("queued cancel outcome = %+v, want finalized (OQ1)", out)
	}
	var qstatus string
	if err := e.db.QueryRowContext(ctx,
		`SELECT status FROM queue WHERE run_id = 't-4.execute'`).Scan(&qstatus); err != nil {
		t.Fatalf("read queue row: %v", err)
	}
	if qstatus != "done" {
		t.Errorf("queue row status = %q, want done (settled in the same tx)", qstatus)
	}
	// And the claim loop finds nothing to claim.
	n, err := e.sched.Tick(ctx)
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.sched.WaitInFlight()
	if n != 0 {
		t.Errorf("the claim loop dispatched %d runs after the cancel committed", n)
	}
}

// TestCancelClaimedIsA409Retry: the transient CAS window is never raced.
func TestCancelClaimedIsA409Retry(t *testing.T) {
	e := newCancelEnv(t)
	r := e.seedRun(t, "t-5.execute", "t-5", "alice", run.StateClaimed)
	if _, err := e.sk.CancelRun(context.Background(), "alice", r.ID); !errors.Is(err, ErrCancelInFlight) {
		t.Fatalf("claimed cancel error = %v, want ErrCancelInFlight", err)
	}
	if got := e.state(t, r.ID); got != run.StateClaimed {
		t.Errorf("a refused cancel moved the run to %s", got)
	}
}

// TestCancelTaskCoversEveryNonTerminalRun.
func TestCancelTaskCoversEveryNonTerminalRun(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	e.seedRun(t, "t-6.intake", "t-6", "alice", run.StateCompleted)
	e.seedRun(t, "t-6.execute", "t-6", "alice", run.StateRunning)
	e.seedRun(t, "t-6.verify", "t-6", "alice", run.StateParked)

	out, err := e.sk.CancelTask(ctx, "alice", "t-6")
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if len(out.Runs) != 3 || !out.Applied {
		t.Fatalf("task cancel outcome = %+v", out)
	}
	want := map[string]run.State{
		"t-6.intake":  run.StateCompleted, // already terminal: untouched
		"t-6.execute": run.StateCompleted,
		"t-6.verify":  run.StateFinalized,
	}
	for id, st := range want {
		if got := e.state(t, id); got != st {
			t.Errorf("run %s = %s, want %s", id, got, st)
		}
	}
	if e.kanban(t, "t-6") != "cancelled" {
		t.Errorf("kanban = %q, want cancelled", e.kanban(t, "t-6"))
	}
}

// TestCancelSuppressionIsSingleUseAtItsOwnGeneration is the LOAD-BEARING race
// (drain D1): the dispatch leg's crash() arriving BEFORE the cancel's terminal
// transition, while the run is still `running`.
//
// This is the scenario the guard exists for, and it is non-tautological by
// construction: running→crashed is a perfectly legal edge, so with the guard
// removed the crash lands and the assertion below fails. (The first cut asserted
// the post-transition case, where completed→crashed is refused by the FSM
// anyway — it passed with the guard disabled and proved nothing.)
func TestCancelSuppressionIsSingleUseAtItsOwnGeneration(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	r := e.seedRun(t, "t-8.execute", "t-8", "alice", run.StateRunning)
	sess := &fakeSession{}
	e.sk.cancels.register(r.ID, sess)

	// The cancel takes its mark and runs the ladder; the leg unwinds and calls
	// crash BEFORE the terminal transition commits.
	e.sk.runLadder(ctx, r)
	e.sk.crash(ctx, r.ID, "session ended canceled")

	if got := e.state(t, r.ID); got != run.StateRunning {
		t.Fatalf("the leg's crash landed on a still-running cancelled run (state %s): the suppression did not hold", got)
	}
	if n := e.countEvents(t, r.ID, run.EventState); n != 3 { // queued, claimed, running
		t.Fatalf("a crash transition was recorded: %d state events, want the 3 fixture ones", n)
	}

	// SINGLE-USE: the mark is consumed, so a SECOND unwind — a genuine crash
	// this cancel did not cause — files its corpse.
	e.sk.crash(ctx, r.ID, "a real failure after the cancel")
	if got := e.state(t, r.ID); got != run.StateCrashed {
		t.Fatalf("the second crash was swallowed (state %s): the suppression is not single-use", got)
	}
}

// TestStaleCancelMarkCannotSwallowALaterCrash: a mark that outlived its cancel
// is rendered inert by the resume's generation bump (§8), so the run's next
// genuine crash still leaves a corpse for the recovery ladder.
func TestStaleCancelMarkCannotSwallowALaterCrash(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	r := e.seedRun(t, "t-9.execute", "t-9", "alice", run.StateRunning)
	e.sk.cancels.register(r.ID, &fakeSession{})
	e.sk.runLadder(ctx, r) // mark taken at generation 0; the cancel then fails to commit

	// The run parks and is resumed by a human: parked→running bumps generation.
	if _, err := e.runs.Transition(ctx, r.ID, run.StateParked, run.TransitionOptions{Reason: "gate", Actor: "platform"}); err != nil {
		t.Fatalf("park: %v", err)
	}
	resumed, err := e.runs.Transition(ctx, r.ID, run.StateRunning, run.TransitionOptions{Reason: "resume", Actor: "alice"})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Generation == r.Generation {
		t.Fatalf("fixture invariant broken: the resume did not bump the generation")
	}

	// Now the run genuinely crashes. The stale mark is at the OLD generation and
	// must not match.
	e.sk.crash(ctx, r.ID, "a real crash after the resume")
	if got := e.state(t, r.ID); got != run.StateCrashed {
		t.Fatalf("a stale cancel mark swallowed a real crash (state %s) — the zombie of D1", got)
	}
}

// TestFailedCancelClearsItsSuppression: when the terminal transition does NOT
// commit, the mark is cleared, so the run's next crash is recorded.
func TestFailedCancelClearsItsSuppression(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	// A terminal hook that fails FAILS the transition (§8) — the deterministic
	// way to make a cancel not commit.
	failing := true
	if err := e.runs.SetTerminalHook(func(context.Context, *sql.Tx, run.Run) error {
		if failing {
			return errors.New("terminal hook refuses")
		}
		return nil
	}); err != nil {
		t.Fatalf("SetTerminalHook: %v", err)
	}
	r := e.seedRun(t, "t-10.execute", "t-10", "alice", run.StateRunning)
	e.sk.cancels.register(r.ID, &fakeSession{})

	if _, err := e.sk.CancelRun(ctx, "alice", r.ID); err == nil {
		t.Fatal("the cancel must fail when its terminal transition does")
	}
	if got := e.state(t, r.ID); got != run.StateRunning {
		t.Fatalf("the failed cancel still moved the run to %s", got)
	}
	// The suppression must not have outlived the failed attempt.
	failing = false
	e.sk.crash(ctx, r.ID, "a real crash after the failed cancel")
	if got := e.state(t, r.ID); got != run.StateCrashed {
		t.Fatalf("a failed cancel's mark swallowed a real crash (state %s) — drain D1", got)
	}
}

// TestCancelTaskLeavesKanbanAloneWhenNothingWasCancelled (drain D4): a task
// whose runs had all already ended is not "cancelled" — it finished — and a
// no-op cancel must not rewrite the board's record of that.
func TestCancelTaskLeavesKanbanAloneWhenNothingWasCancelled(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	e.seedRun(t, "t-11.execute", "t-11", "alice", run.StateCompleted)
	e.setKanbanFixture(t, "t-11", "done")

	out, err := e.sk.CancelTask(ctx, "alice", "t-11")
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if out.Applied {
		t.Errorf("nothing was cancellable; Applied = true")
	}
	if got := e.kanban(t, "t-11"); got != "done" {
		t.Errorf("kanban = %q, want the untouched \"done\" — a no-op cancel rewrote the board (drain D4)", got)
	}
	if out.Kanban != "" {
		t.Errorf("the outcome claims kanban %q for a no-op cancel", out.Kanban)
	}
	// The control: a task with a live run DOES move.
	e.seedRun(t, "t-12.execute", "t-12", "alice", run.StateRunning)
	e.setKanbanFixture(t, "t-12", "executing")
	if _, err := e.sk.CancelTask(ctx, "alice", "t-12"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if got := e.kanban(t, "t-12"); got != kanbanCancelled {
		t.Errorf("a real cancel must move the column; got %q", got)
	}
}

// TestRacedCancelIsAConflictNotAnInternalError (drain D8): the run's state moved
// between the read and the transition. Staged deterministically by handing the
// mapping a STALE run value, which is exactly what a race produces.
func TestRacedCancelIsAConflictNotAnInternalError(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	stale := e.seedRun(t, "t-13.execute", "t-13", "alice", run.StateRunning)
	// The world moves on: the run completes on its own.
	if _, err := e.runs.Transition(ctx, stale.ID, run.StateCompleted, run.TransitionOptions{
		Reason: "finished first", Actor: "platform"}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	_, err := e.sk.cancelOne(ctx, "alice", stale) // stale.State is still `running`
	if !errors.Is(err, ErrCancelRaced) {
		t.Fatalf("a raced cancel returned %v, want ErrCancelRaced (drain D8)", err)
	}
	if got := e.state(t, stale.ID); got != run.StateCompleted {
		t.Errorf("the raced cancel changed the run to %s — nothing may double-fire", got)
	}
	// The classifier discriminates: a NON-raced failure stays itself.
	if got := raced(errors.New("disk on fire")); errors.Is(got, ErrCancelRaced) {
		t.Error("raced() swallowed an unrelated failure into a conflict")
	}
}

// TestQueuedCancelWithoutASchedulerIsRefusedFailClosed (drain D11): the OQ1
// edge and its queue settle commit together or not at all, so a process that
// cannot settle the queue row must not half-commit the transition.
func TestQueuedCancelWithoutASchedulerIsRefusedFailClosed(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	e.sk.sched = nil // a process with no scheduler bound
	r := e.seedRun(t, "t-14.execute", "t-14", "alice", run.StateQueued)

	_, err := e.sk.CancelRun(ctx, "alice", r.ID)
	if !errors.Is(err, ErrCancelQueueUnsettleable) {
		t.Fatalf("error = %v, want ErrCancelQueueUnsettleable", err)
	}
	if got := e.state(t, r.ID); got != run.StateQueued {
		t.Errorf("the refused cancel still moved the run to %s — it half-committed (drain D11)", got)
	}
}

// ── NO AUTO-KILL: the source assertion ──────────────────────────────────────

// TestCancelIsReachableFromTheHTTPVerbsOnly proves the S14.4 / G1 D1.3
// invariant structurally rather than by inspection: the cancel entry points are
// called from the HTTP surface adapter and from tests, and from nowhere else in
// the tree. No watchdog tier, no recovery pass, no scheduler loop and no
// benchmark driver can reach them.
func TestCancelIsReachableFromTheHTTPVerbsOnly(t *testing.T) {
	// The sanctioned callers, by file — exactly three, each a link in the ONE
	// human chain: the HTTP verb handlers (api/actions.go), the
	// api.CancelSurface adapter they call through (stage/surface.go), and the
	// machinery itself (stage/cancel.go). Anything else appearing here is an
	// automated path acquiring a cancel, which is what D1.3 forbids.
	sanctioned := map[string]bool{
		filepath.Join("..", "..", "internal", "stage", "cancel.go"):  true,
		filepath.Join("..", "..", "internal", "stage", "surface.go"): true,
		filepath.Join("..", "..", "internal", "api", "actions.go"):   true,
	}
	targets := map[string]bool{
		"CancelRun": true, "CancelTask": true, "cancelOne": true,
		"runLadder": true, "markRequested": true,
	}
	// The walk covers the WHOLE tree — internal/ including nested packages, plus
	// cmd/ and tools/ (drain D5). The tree is clean today; the wall is what
	// keeps it clean tomorrow, and a wall that only looked at one directory
	// level would not have seen a caller added one level down.
	var offenders []string
	var scanned int
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		clean := filepath.Clean(path)
		if sanctioned[clean] {
			return nil
		}
		src, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		scanned++
		ast.Inspect(src, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			case *ast.Ident:
				name = fn.Name
			}
			if targets[name] {
				offenders = append(offenders, clean+": "+name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	// The walk must actually have covered the tree, cmd/ and tools/ included —
	// an empty or truncated walk would make the wall pass by doing nothing.
	if scanned < 100 {
		t.Fatalf("the wall scanned only %d files: it is not covering the tree", scanned)
	}
	for _, must := range []string{
		filepath.Join("..", "..", "cmd"), filepath.Join("..", "..", "tools"),
		filepath.Join("..", "..", "internal", "adapters", "claudecli"), // a NESTED package
	} {
		if fi, serr := os.Stat(must); serr != nil || !fi.IsDir() {
			t.Fatalf("the wall expects %s to exist and be walked; got %v", must, serr)
		}
	}
	if len(offenders) != 0 {
		t.Fatalf("the cancel machinery is reachable outside the HTTP verbs — NO AUTO-KILL (S14.4 / G1 D1.3): %v", offenders)
	}
	// Non-tautological: the scan DOES see the sanctioned caller when it is not
	// excluded, so an empty offender list is a real result and not a broken walk.
	src, err := parser.ParseFile(fset, filepath.Join("..", "..", "internal", "stage", "surface.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse surface.go: %v", err)
	}
	seen := false
	ast.Inspect(src, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if fn, ok := call.Fun.(*ast.SelectorExpr); ok && targets[fn.Sel.Name] {
				seen = true
			}
		}
		return true
	})
	if !seen {
		t.Fatal("the probe found no call in the sanctioned caller: the scan is not measuring what it claims")
	}
}
