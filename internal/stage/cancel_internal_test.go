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

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
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

// TestCancelledLegFilesNoCrashCorpse: the dispatch leg unwinding BECAUSE of a
// human cancel must not tell the recovery ladder to fork the work the person
// just stopped.
func TestCancelledLegFilesNoCrashCorpse(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	r := e.seedRun(t, "t-7.execute", "t-7", "alice", run.StateRunning)
	e.sk.cancels.register(r.ID, &fakeSession{})
	if _, err := e.sk.CancelRun(ctx, "alice", r.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	// The in-flight dispatch leg now unwinds and calls crash, as it would in
	// production when the ladder ends its session.
	e.sk.crash(ctx, r.ID, "session ended canceled")
	if got := e.state(t, r.ID); got != run.StateCompleted {
		t.Errorf("run = %s after the unwind, want the cancel's own terminal", got)
	}
	var n int
	if err := e.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ?`, r.ID, run.EventWedged).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("the cancelled leg left %d wedge markers", n)
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
		filepath.Join("..", "stage", "cancel.go"):  true,
		filepath.Join("..", "stage", "surface.go"): true,
		filepath.Join("..", "api", "actions.go"):   true,
	}
	targets := map[string]bool{
		"CancelRun": true, "CancelTask": true, "cancelOne": true,
		"runLadder": true, "markRequested": true,
	}
	var offenders []string
	roots, err := filepath.Glob(filepath.Join("..", "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	for _, dir := range roots {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") || sanctioned[f] {
				continue
			}
			src, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				continue
			}
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
					offenders = append(offenders, f+": "+name)
				}
				return true
			})
		}
	}
	if len(offenders) != 0 {
		t.Fatalf("the cancel machinery is reachable outside the HTTP verbs — NO AUTO-KILL (S14.4 / G1 D1.3): %v", offenders)
	}
	// Non-tautological: the scan DOES see the sanctioned caller when it is not
	// excluded, so an empty offender list is a real result and not a broken walk.
	src, err := parser.ParseFile(fset, filepath.Join("..", "stage", "surface.go"), nil, 0)
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
