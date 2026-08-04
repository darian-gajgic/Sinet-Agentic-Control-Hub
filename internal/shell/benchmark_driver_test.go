package shell

// benchmark_driver_test.go — the BENCH-REG §3 dispatch→render DRIVER (P3-B6-2C,
// R18), end to end over the REAL composition: the shell's driver pass, the real
// scheduler, the real dispatcher's `.direct` leg, the real capture column and
// the real DirectText seam. The only fake is the ENGINE. $0: nothing spawns a
// process, dials a provider or reaches a seat.
//
// This is the level nothing else can see. internal/benchmark proves the verbs
// refuse out-of-order acts and internal/stage proves the leg runs one unshaped
// session, but only here does a SAMPLED pair actually become a RENDERED one —
// which is the gap §35 D4 named and this packet closes.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

const (
	driverStatement    = "FROZEN-STATEMENT: the confirmed specification of record."
	driverArmAnswer    = "DIRECT-ARM-TEXT: one shot, take what comes."
	driverPlatformText = "PLATFORM-ARM-TEXT: the accepted deliverable."
)

// ── the fake engine ─────────────────────────────────────────────────────────

type driverSession struct {
	text string
	kind adapters.OutcomeKind
}

func (d driverSession) Events() <-chan adapters.Event {
	ch := make(chan adapters.Event)
	close(ch)
	return ch
}
func (d driverSession) Cursor() adapters.Cursor      { return adapters.Cursor{} }
func (d driverSession) Fingerprint() string          { return "fp" }
func (d driverSession) Pause(context.Context) error  { return nil }
func (d driverSession) Cancel(context.Context) error { return nil }
func (d driverSession) Wait(context.Context) (adapters.Outcome, error) {
	kind := d.kind
	if kind == "" {
		kind = adapters.OutcomeCompleted
	}
	return adapters.Outcome{Kind: kind, ResultText: d.text}, nil
}

type driverEngine struct {
	mu     sync.Mutex
	starts int
	text   string
	kind   adapters.OutcomeKind
	// startErr crashes the spawn — the arm never answers at all.
	startErr error
}

func (e *driverEngine) Substrate() string { return adapters.SubstrateClaudeCLI }
func (e *driverEngine) Start(_ context.Context, req adapters.StartRequest) (adapters.Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.starts++
	// The one thing worth asserting inside the engine: the §2 arm answers the
	// frozen statement and nothing else reached it.
	if req.Worker.Prompt != driverStatement {
		return nil, errors.New("the direct arm was handed something other than the frozen statement: " + req.Worker.Prompt)
	}
	if e.startErr != nil {
		return nil, e.startErr
	}
	return driverSession{text: e.text, kind: e.kind}, nil
}
func (e *driverEngine) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("the §2 arm never resumes")
}
func (e *driverEngine) started() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.starts
}

// ── the environment ─────────────────────────────────────────────────────────

type driverEnv struct {
	db     *storage.DB
	log    *eventlog.Log
	runs   *run.Store
	sched  *scheduler.Scheduler
	ladder *recovery.Ladder
	bs     *benchmarkSurface
	engine *driverEngine

	// platformErr, when set, fails the platform-arm text seam — the fixture for
	// a render that must be retried rather than half-applied.
	platformErr error
}

func newDriverEnv(t *testing.T, engine *driverEngine) *driverEnv {
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
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	runs := run.NewStore(db, log)
	e := &driverEnv{db: db, log: log, runs: runs, engine: engine}

	// The seam holder exactly as shell.Run composes it: created before the
	// skeleton, its store bound after the practice exists.
	bseams := &directArmSeams{}
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: engine},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		Model:        "fake-model-1",
		// The statement seam is a fixture: resolving it for real needs an S06
		// artifact of record on disk, and briefFromState's own hash-verified
		// resolution is asserted separately. The CAPTURE seam is the real one.
		BenchmarkStatement: func(context.Context, string) (string, error) { return driverStatement, nil },
		BenchmarkCapture:   bseams.Capture,
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	meterLedger := metering.NewLedger(db, priceTable, exceptions, reg)
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, meterLedger, exceptions),
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour, // manual only: nothing ticks in this file
		Logger:       testLogger(),
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	e.sched = sched

	// The recovery ladder, composed with the B6-2C fork-refusal predicate exactly
	// as shell.Run composes it.
	journal, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg})
	if err != nil {
		t.Fatalf("gates.NewJournal: %v", err)
	}
	ladder, err := recovery.New(recovery.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Effects: journal, Settings: reg, NoFork: stage.IsDirectArmRun,
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	e.ladder = ladder

	evalStore := evals.NewStore(db, conformance.NewStore(db, log, reg), reg)
	bs, err := buildBenchmarkSurface(ctx, db, log, reg, runs, sched, meterLedger, evalStore, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("buildBenchmarkSurface: %v", err)
	}
	bseams.store = bs.Store
	// The two seams a nil pipeline/review store leave unwired. Direct stays the
	// REAL read of the capture column — that is half of what this file proves.
	bs.Practice.Briefs = func(context.Context, string) (benchmark.TaskBrief, error) {
		return benchmark.TaskBrief{Statement: driverStatement, StatementSource: "s06:fixture@sha256:abc"}, nil
	}
	bs.Practice.Platform = func(context.Context, string) (string, error) {
		if e.platformErr != nil {
			return "", e.platformErr
		}
		return driverPlatformText, nil
	}
	e.bs = bs
	return e
}

// seedPair creates the owner, the task, the platform run and the SAMPLED pair —
// the state the §4 hook leaves behind, which is where the driver picks up.
func (e *driverEnv) seedPair(t *testing.T, pairID, taskID, owner string) benchmark.Pair {
	t.Helper()
	ctx := context.Background()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO users (user_id, display_name, role, created_ts) VALUES (?,?,?,?)`,
			owner, owner, "member", ts); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO tasks (task_id, user_id, title, created_ts) VALUES (?,?,?,?)`,
			taskID, owner, "T", ts)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	platformRun := taskID + ".execute"
	if _, err := e.runs.Create(ctx, run.NewRun{
		ID: platformRun, UserID: owner, TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	}); err != nil {
		t.Fatalf("create platform run: %v", err)
	}
	var pair benchmark.Pair
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		pair, err = e.bs.Store.CreateTx(ctx, tx, benchmark.NewPair{
			PairID: pairID, UserID: owner, Domain: benchmark.DomainSoftwareDevelopment,
			DomainSource: "fixture", TaskID: taskID, DeliverableID: taskID + ".d1",
			TaskClass: "software/standard", PlatformRunID: platformRun,
			Phase: benchmark.PhasePreGate, RatePct: 100,
		})
		return err
	}); err != nil {
		t.Fatalf("create pair: %v", err)
	}
	return pair
}

func (e *driverEnv) pass(t *testing.T) {
	t.Helper()
	benchmarkDriverPass(context.Background(), e.bs, testLogger())
}

// claim runs one scheduler pass, which is what carries a queued direct arm into
// the dispatcher, and waits for the dispatch to finish. It is called explicitly
// so each phase of the walk is visible; dispatch itself is asynchronous in
// production, so the wait is how a test observes a phase boundary the scheduler
// does not announce.
func (e *driverEnv) claim(t *testing.T) int {
	t.Helper()
	n, err := e.sched.Tick(context.Background())
	if err != nil {
		t.Fatalf("scheduler tick: %v", err)
	}
	if n > 0 {
		e.waitIdle(t)
	}
	return n
}

// waitIdle blocks until no run is in flight. Dueness in this file is stored
// state, never a sleep: the loop polls the FSM the dispatch actually moves.
func (e *driverEnv) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := e.db.QueryRowContext(context.Background(),
			`SELECT count(*) FROM runs WHERE state IN ('claimed', 'running')`).Scan(&n); err != nil {
			t.Fatalf("read in-flight runs: %v", err)
		}
		if n == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("a dispatched run never ended — the leg is wedged")
}

func (e *driverEnv) pair(t *testing.T, pairID string) benchmark.Pair {
	t.Helper()
	p, err := e.bs.Store.Pair(context.Background(), pairID)
	if err != nil {
		t.Fatalf("read pair: %v", err)
	}
	return p
}

// ── the walk (rubric 15) ────────────────────────────────────────────────────

// TestDriverWalksASampledPairToRendered is the §35 D4 gap, closed: with the
// driver running, a sampled pair reaches the state whose card its requester can
// actually vote on. Each pass is driven explicitly so the ORDER is the assertion
// — nothing advances until the state that entitles it to advance is stored.
func TestDriverWalksASampledPairToRendered(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.seedPair(t, "bp-1", "t-1", "alice")
	ctx := context.Background()

	// Pass 1: the sampled pair is dispatched. It is NOT rendered — its arm has
	// not run, and the driver has no authority to pretend otherwise.
	e.pass(t)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDispatched {
		t.Fatalf("after the first pass the pair is %s, want dispatched", got.State)
	}
	if got := e.pair(t, pair.PairID).DirectRunID; got != benchmark.DirectRunID(pair.PairID) {
		t.Fatalf("the pair names direct run %q", got)
	}

	// Pass 2 with the arm still queued: nothing moves. The driver waits for the
	// run to END rather than for a timer.
	e.pass(t)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDispatched {
		t.Fatalf("the pair advanced while its arm was still queued: %s", got.State)
	}
	if e.engine.started() != 0 {
		t.Fatal("the engine ran before the scheduler claimed the run")
	}

	// The scheduler claims and the dispatcher runs the single shot.
	if n := e.claim(t); n != 1 {
		t.Fatalf("the claim pass dispatched %d runs, want the direct arm", n)
	}
	armRun, err := e.runs.Get(ctx, benchmark.DirectRunID(pair.PairID))
	if err != nil {
		t.Fatal(err)
	}
	if armRun.State != run.StateCompleted {
		t.Fatalf("the direct arm ended %s, want completed under the ordinary FSM", armRun.State)
	}
	if e.engine.started() != 1 {
		t.Fatalf("the engine ran %d times — §2 is single-shot", e.engine.started())
	}
	// The capture is real and readable through the seam the practice uses.
	body, err := e.bs.Store.CapturedDirectText(ctx, armRun.ID)
	if err != nil || body != driverArmAnswer {
		t.Fatalf("CapturedDirectText = %q, %v — want the arm's own text", body, err)
	}

	// Pass 3: the arm has ended, so the pair renders blind.
	e.pass(t)
	got := e.pair(t, pair.PairID)
	if got.State != benchmark.StateRendered {
		t.Fatalf("after the arm ended the pair is %s, want rendered", got.State)
	}
	both := got.RenderA + "\n" + got.RenderB
	if !strings.Contains(both, driverPlatformText) || !strings.Contains(both, driverArmAnswer) {
		t.Fatalf("the blind pair does not carry both arms:\nA=%q\nB=%q", got.RenderA, got.RenderB)
	}
	// And the requester can now see the form.
	pending, err := e.bs.Store.PendingVerdicts(ctx, "alice")
	if err != nil || len(pending) != 1 {
		t.Fatalf("PendingVerdicts = %d, %v — want the requester's one card", len(pending), err)
	}
}

// TestDriverDispatchIsSingleShotUnderReEntry: re-entry is SAFE because the verbs
// check state, not because the driver remembers anything. A second pass over a
// dispatched pair asks nothing; asking anyway hits the state guard; and the runs
// table's primary key is the structural backstop behind both.
func TestDriverDispatchIsSingleShotUnderReEntry(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.seedPair(t, "bp-2", "t-2", "alice")
	ctx := context.Background()

	e.pass(t)
	e.pass(t)
	e.pass(t)

	// The state guard refuses a direct re-dispatch.
	_, err := e.bs.Practice.DispatchDirectArm(ctx, pair.PairID)
	if !errors.Is(err, benchmark.ErrPairState) {
		t.Fatalf("re-dispatch of a dispatched pair: %v, want the state guard", err)
	}
	// Exactly one direct-arm run exists, and exactly one queue row for it: the
	// PK makes a second arm structurally impossible even if a guard were lost.
	var runRows, queueRows int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM runs WHERE run_id LIKE ?`, pair.PairID+".direct%").Scan(&runRows); err != nil {
		t.Fatal(err)
	}
	if runRows != 1 {
		t.Errorf("%d direct-arm runs exist for one pair", runRows)
	}
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM queue WHERE run_id = ?`, benchmark.DirectRunID(pair.PairID)).Scan(&queueRows); err != nil {
		t.Fatal(err)
	}
	if queueRows != 1 {
		t.Errorf("%d queue rows for one direct arm", queueRows)
	}
}

// TestDriverRetriesAFailedRenderWithoutCorruption: a failing render leaves the
// pair EXACTLY where it was — no position assigned, no half-written blind body,
// nothing marked failed — and the next pass simply tries again from the stored
// state. That is what makes "logged and retried" safe rather than hopeful.
func TestDriverRetriesAFailedRenderWithoutCorruption(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.seedPair(t, "bp-3", "t-3", "alice")
	e.platformErr = errors.New("the accepted revision is not readable right now")

	e.pass(t)
	e.claim(t)
	e.pass(t) // the render fails

	got := e.pair(t, pair.PairID)
	if got.State != benchmark.StateDispatched {
		t.Fatalf("a failed render left the pair %s, want it untouched at dispatched", got.State)
	}
	switch {
	case got.Position != "":
		t.Errorf("a failed render assigned a position (%q) — arm identity must not leak from a failure", got.Position)
	case got.RenderA != "" || got.RenderB != "":
		t.Error("a failed render wrote a partial blind body")
	}

	// The next pass, with the source readable again, completes it.
	e.platformErr = nil
	e.pass(t)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateRendered {
		t.Fatalf("the retry did not complete: %s", got.State)
	}
	if e.engine.started() != 1 {
		t.Errorf("the arm ran %d times across a render retry — a render failure must never re-run an arm", e.engine.started())
	}
}

// TestTruncatedArmYieldsTheParityNote: an arm an ordinary platform ceiling cut
// short is an HONEST OUTCOME, not a failure to repair. It renders, it is voted
// on, and the §6 parity note on its record says the run did not end on its own
// terms — flagged for the coordinator, never silently absorbed and never re-run.
func TestTruncatedArmYieldsTheParityNote(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: "", kind: adapters.OutcomeDiedAtGate})
	pair := e.seedPair(t, "bp-4", "t-4", "alice")
	ctx := context.Background()

	e.pass(t)
	e.claim(t)
	armRun, err := e.runs.Get(ctx, benchmark.DirectRunID(pair.PairID))
	if err != nil {
		t.Fatal(err)
	}
	if armRun.State != run.StateDiedAtGate {
		t.Fatalf("the fixture arm ended %s, want the ceiling-preempted state", armRun.State)
	}
	e.pass(t)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateRendered {
		t.Fatalf("a truncated arm did not render: %s — a pair that cannot complete cannot report its own parity gap", got.State)
	}

	answer, err := benchmark.NewAnswer(benchmark.ChoiceA, benchmark.SideA)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := e.bs.Practice.RecordVerdict(ctx, pair.PairID, answer)
	if err != nil {
		t.Fatalf("RecordVerdict: %v", err)
	}
	if recorded.ParityNote == "" {
		t.Fatal("a truncated arm recorded no §6 parity note — a parity gap beyond the declaration is never absorbed")
	}
	if !strings.Contains(recorded.ParityNote, string(run.StateDiedAtGate)) {
		t.Errorf("the parity note does not name the arm's own disposition: %q", recorded.ParityNote)
	}
	if e.engine.started() != 1 {
		t.Errorf("the truncated arm ran %d times — §17: a result is never recomputed", e.engine.started())
	}
}

// TestDriverAdvancesNothingSynthetically: the pass has no protocol authority. A
// pair whose arm never ran, and a pair with no arm at all, are both left exactly
// where the practice put them however many passes run.
func TestDriverAdvancesNothingSynthetically(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.seedPair(t, "bp-5", "t-5", "alice")

	e.pass(t)
	for range 5 {
		e.pass(t) // the arm is queued and never claimed
	}
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDispatched {
		t.Fatalf("five passes moved a pair whose arm never ran: %s", got.State)
	}
	if e.engine.started() != 0 {
		t.Fatal("a driver pass reached the engine — the scheduler is the only ingress (S16.6)")
	}
	// A pair whose arm ENDED but captured nothing renders nothing either: the
	// DirectText seam's absence is honest and the pair waits rather than being
	// rendered against a body that does not exist.
	e.claim(t)
	if err := e.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`UPDATE benchmark_pairs SET direct_text = NULL WHERE pair_id = ?`, pair.PairID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.pass(t)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDispatched {
		t.Fatalf("a pair with no captured arm was rendered anyway: %s", got.State)
	}
}

// TestDirectArmSeamsRefuseWhenUnbound: the composition holder is late-bound, and
// an unbound half refuses LOUDLY rather than inventing a prompt or dropping a
// capture on the floor.
func TestDirectArmSeamsRefuseWhenUnbound(t *testing.T) {
	var unbound directArmSeams
	if _, err := unbound.Statement(context.Background(), "t-1"); err == nil {
		t.Error("an unbound statement seam must refuse — the arm is never run on an invented prompt")
	}
	if _, err := unbound.Capture(context.Background(), "bp-1.direct", "text"); err == nil {
		t.Error("an unbound capture seam must refuse — a capture with nowhere to land is a lost arm")
	}

	// Bound to a real store, the capture seam is the real write and the read is
	// the practice's own.
	db, log, _ := watchlistTestDeps(t)
	bound := directArmSeams{store: benchmark.NewStore(db, log)}
	took, err := bound.Capture(context.Background(), "bp-nobody.direct", "text")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if took {
		t.Error("a capture for a run no pair claims must take nothing")
	}
}

// ── drain r1: the crashed arm, end to end (D1 + D2) ─────────────────────────

// TestCrashedArmIsNeverForkedAndItsPairBecomesCloseable is the drain's whole
// story in one walk, over the real composition:
//
//	the §2 arm crashes → the ladder FINALIZES it instead of forking (a fork would
//	be a second billed run on the same frozen statement) → the capture stays
//	absent → the driver stops visiting the pair instead of retrying forever → the
//	requester sees a card that explains it → they decline → the pair closes with
//	its §14 record and leaves every list.
func TestCrashedArmIsNeverForkedAndItsPairBecomesCloseable(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{startErr: errors.New("no engine here")})
	e.operator(t, "op")
	pair := e.seedPair(t, "bp-20", "t-20", "alice")
	e.seedPair(t, "bp-21", "t-21", "bob") // another member's pair, never touched
	ctx := context.Background()

	e.pass(t)
	e.claim(t)

	// The arm crashed and produced nothing.
	armID := benchmark.DirectRunID(pair.PairID)
	armRun, err := e.runs.Get(ctx, armID)
	if err != nil {
		t.Fatal(err)
	}
	if armRun.State != run.StateCrashed {
		t.Fatalf("the arm ended %s, want crashed", armRun.State)
	}
	if _, err := e.bs.Store.CapturedDirectText(ctx, armID); !errors.Is(err, benchmark.ErrNoDirectCapture) {
		t.Fatalf("a crashed arm left a capture (%v) — an engine that never answered must not render as an answer", err)
	}

	// D1: the ladder finalizes it. No fork, no second engine run, ever.
	if _, err := e.ladder.ReconcilePass(ctx); err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	armRun, err = e.runs.Get(ctx, armID)
	if err != nil {
		t.Fatal(err)
	}
	if armRun.State != run.StateFinalized {
		t.Fatalf("the crashed arm is %s after recovery, want finalized (BENCH-REG §2 outranks the fork default)", armRun.State)
	}
	var successors int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM runs WHERE parent_run_id = ?`, armID).Scan(&successors); err != nil {
		t.Fatal(err)
	}
	if successors != 0 {
		t.Fatalf("%d fork(s) of the direct arm exist — §2 is single-shot", successors)
	}

	// D2(a): the driver stops visiting it. Several passes change nothing and the
	// engine is never reached again. (Both members' arms were dispatched, so the
	// count is compared against itself rather than pinned to a literal.)
	attempted := e.engine.started()
	for range 3 {
		e.pass(t)
	}
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDispatched {
		t.Fatalf("the stranded pair moved to %s — nothing may advance it synthetically", got.State)
	}
	if e.engine.started() != attempted {
		t.Fatalf("the engine ran again (%d → %d) — a crashed arm is never re-attempted",
			attempted, e.engine.started())
	}

	// D2(b): the requester sees a card that says what happened; another member
	// does not; the operator does (the landed inbox scoping).
	card := failedPairCard(t, e, "alice", pair.PairID)
	if !card.Answerable {
		t.Error("the requester cannot act on their own failed pair")
	}
	if !containsStr(card.Actions, "decline") {
		t.Errorf("the failed-pair card offers %v, want the landed decline verb", card.Actions)
	}
	for _, want := range []string{"single-shot direct arm", "can never be shown", string(run.StateFinalized)} {
		if !strings.Contains(string(card.Card), want) {
			t.Errorf("the card does not carry the honest story (%q): %s", want, card.Card)
		}
	}
	// Another member sees their OWN stranded pair and never alice's — the
	// non-tautological direction, since bob's arm crashed in the same pass.
	bobSees := listKinds(t, e, "bob", "benchmark_verdict")
	if len(bobSees) != 1 || bobSees[0].ID != "benchmark_verdict:bp-21" {
		t.Errorf("bob's inbox is %v, want exactly his own failed pair", bobSees)
	}
	if opSees := failedPairCard(t, e, "op", pair.PairID); opSees.Answerable {
		t.Error("the operator's copy of the card reads answerable — closing it is the requester's act")
	}

	// D2(c): the decline closes it, with its §14 record.
	e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/decline", `{}`)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDeclined {
		t.Fatalf("the pair is %s after the decline", got.State)
	}
	if n := e.recordedEvents(t, benchmark.EventPairRecorded, pair.PairID); n != 1 {
		t.Fatalf("the closed pair wrote %d §14 records, want exactly 1", n)
	}
	// The crash CAUSE stays derivable without extending the registered §14
	// schema: the record names the direct run, and the run's own state says how
	// it ended.
	rec, err := e.bs.Practice.Reveal(ctx, pair.PairID)
	if err != nil {
		t.Fatalf("Reveal of the decline record: %v", err)
	}
	if rec.DirectRunID != armID {
		t.Errorf("the record names direct run %q — the join to the arm's state is how the cause stays derivable", rec.DirectRunID)
	}
	if !rec.Declined {
		t.Error("the closing record is not marked declined")
	}

	// And the card is gone from every list, for everybody.
	if got := listKinds(t, e, "alice", "benchmark_verdict"); len(got) != 0 {
		t.Errorf("the closed pair still shows %d card(s)", len(got))
	}
	if got := listKinds(t, e, "op", "benchmark_verdict"); len(got) != 1 || got[0].ID != "benchmark_verdict:bp-21" {
		t.Errorf("the operator's inbox is %v, want only the pair still open", got)
	}
	e.pass(t)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDeclined {
		t.Errorf("a driver pass disturbed a closed pair: %s", got.State)
	}
}

// TestDispatchIsSingleShotEvenWhenStateIsFlippedBack is the drain-r1 probe for
// D3. The precise property is NOT "the PK refuses a second row" — pair.go
// deliberately tolerates run.ErrExists for B5-7 crash-resume idempotency — it is
// that no second arm RUN can exist. Flipping a dispatched pair back to `sampled`
// by hand (a corrupted row, an operator at a SQL prompt) re-enters the dispatch,
// and the walk still ends with one run, one queue row and one engine execution.
func TestDispatchIsSingleShotEvenWhenStateIsFlippedBack(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.seedPair(t, "bp-22", "t-22", "alice")
	ctx := context.Background()

	e.pass(t)
	e.claim(t)

	// The hostile fixture: the pair's state is put back by hand.
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE benchmark_pairs SET state = ? WHERE pair_id = ?`, benchmark.StateSampled, pair.PairID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.pass(t)  // re-dispatches
	e.claim(t) // and would re-run the arm if anything let it

	var runRows, queueRows int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM runs WHERE run_id LIKE ?`, pair.PairID+".direct%").Scan(&runRows); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM queue WHERE run_id = ?`, benchmark.DirectRunID(pair.PairID)).Scan(&queueRows); err != nil {
		t.Fatal(err)
	}
	if runRows != 1 || queueRows != 1 {
		t.Errorf("%d run rows / %d queue rows after a state flip-back, want 1 / 1", runRows, queueRows)
	}
	if e.engine.started() != 1 {
		t.Fatalf("the engine executed %d times after a state flip-back — no second arm RUN may exist (BENCH-REG §2)", e.engine.started())
	}
}

// failedPairCard reads one caller's approvals list and returns the card for a
// pair, through the REAL handler.
func failedPairCard(t *testing.T, e *driverEnv, who, pairID string) api.ApprovalItem {
	t.Helper()
	for _, it := range listKinds(t, e, who, "benchmark_verdict") {
		if it.ID == "benchmark_verdict:"+pairID {
			return it
		}
	}
	t.Fatalf("%s sees no card for pair %q", who, pairID)
	return api.ApprovalItem{}
}

func listKinds(t *testing.T, e *driverEnv, who, kind string) []api.ApprovalItem {
	t.Helper()
	var list api.ApprovalList
	if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/approvals", "")), &list); err != nil {
		t.Fatalf("decode approvals: %v", err)
	}
	var out []api.ApprovalItem
	for _, it := range list.Items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

func containsStr(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
