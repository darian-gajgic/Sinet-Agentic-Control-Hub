package stage

// direct_internal_test.go — the BENCH-REG §2 direct-arm leg (P3-B6-2C, R19),
// asserted against a REAL FSM, a REAL checkpoint store and a FAKE engine. $0:
// nothing here spawns a process, dials a provider or reaches a real seat.
//
// The assertions are mostly NEGATIVE, because §2 is mostly a list of things the
// direct arm must not receive. The positive half is small: one session, the
// frozen statement as the whole prompt, checkpoints and metering intact, the
// capture written, the run ended by the ordinary FSM.

import (
	"context"
	"database/sql"
	"errors"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The §2 frozen statement this file's fixtures answer, and the fake engine's
// reply. Both are deliberately unmistakable strings so a prompt that carried
// anything else is visible in the assertion.
const (
	fixtureStatement = "FROZEN-STATEMENT: build the thing described in the confirmed specification of record."
	fixtureArmAnswer = "DIRECT-ARM-ANSWER: here is what one shot produced."
)

// directSession is one fake engine invocation: it emits ONE paid-call usage
// event (so the D7 checkpoint machinery runs for real) and then ends with the
// arm's text.
type directSession struct {
	text    string
	kind    adapters.OutcomeKind
	events  []adapters.Event
	paused  int
	cancels int
}

func (d *directSession) Events() <-chan adapters.Event {
	ch := make(chan adapters.Event, len(d.events))
	for _, ev := range d.events {
		ch <- ev
	}
	close(ch)
	return ch
}
func (d *directSession) Cursor() adapters.Cursor {
	return adapters.Cursor{Substrate: adapters.SubstrateClaudeCLI, SessionID: "sess-direct"}
}
func (d *directSession) Fingerprint() string { return "fp-direct" }
func (d *directSession) Pause(context.Context) error {
	d.paused++
	return nil
}
func (d *directSession) Cancel(context.Context) error {
	d.cancels++
	return nil
}
func (d *directSession) Wait(context.Context) (adapters.Outcome, error) {
	kind := d.kind
	if kind == "" {
		kind = adapters.OutcomeCompleted
	}
	return adapters.Outcome{Kind: kind, ResultText: d.text}, nil
}

// directAdapter records every StartRequest it is handed. The recording IS the
// test: what the direct arm receives is the whole §2 claim.
type directAdapter struct {
	mu       sync.Mutex
	starts   []adapters.StartRequest
	resumes  int
	sessions []*directSession
	// text/kind shape the session's disposition.
	text string
	kind adapters.OutcomeKind
	// usage, when true, emits one paid call so checkpoints and metering run.
	usage bool
	// startErr fails the spawn.
	startErr error
}

func (a *directAdapter) Substrate() string { return adapters.SubstrateClaudeCLI }

func (a *directAdapter) Start(_ context.Context, req adapters.StartRequest) (adapters.Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.starts = append(a.starts, req)
	if a.startErr != nil {
		return nil, a.startErr
	}
	s := &directSession{text: a.text, kind: a.kind}
	if a.usage {
		s.events = append(s.events, adapters.Event{
			Kind: adapters.KindUsage,
			Usage: &adapters.Usage{
				ModelID: "fake-model-1", InputTokens: 900_000, OutputTokens: 900_000,
				MessageID: "m1", MessageIndex: 0,
			},
			Payload: []byte(`{"model":"fake-model-1"}`),
		})
	}
	a.sessions = append(a.sessions, s)
	return s, nil
}

func (a *directAdapter) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	a.mu.Lock()
	a.resumes++
	a.mu.Unlock()
	return nil, errors.New("the §2 direct arm never resumes: no retries, no follow-up turns")
}

func (a *directAdapter) started() []adapters.StartRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]adapters.StartRequest{}, a.starts...)
}

// directEnv is a real spine plus the fake engine and the two composed seams.
type directEnv struct {
	db      *storage.DB
	log     *eventlog.Log
	runs    *run.Store
	sk      *Skeleton
	adapter *directAdapter

	mu         sync.Mutex
	captured   []string
	captureErr error
	statement  string
	statErr    error
}

func newDirectEnv(t *testing.T, a *directAdapter) *directEnv {
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
	e := &directEnv{db: db, log: log, runs: run.NewStore(db, log), adapter: a, statement: fixtureStatement}
	root := t.TempDir()
	sk, err := New(Config{
		DB: db, Log: log, Runs: e.runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: a},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		// The dev/test model override, so this file pins no seat name.
		Model:              "fake-model-1",
		BenchmarkStatement: e.statementSeam,
		BenchmarkCapture:   e.captureSeam,
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	e.sk = sk
	return e
}

func (e *directEnv) statementSeam(context.Context, string) (string, error) {
	return e.statement, e.statErr
}

func (e *directEnv) captureSeam(_ context.Context, runID, text string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.captureErr != nil {
		return false, e.captureErr
	}
	e.captured = append(e.captured, runID+"\x1f"+text)
	return true, nil
}

func (e *directEnv) captures() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string{}, e.captured...)
}

// seedDirectRun creates the pair's direct-arm run and walks it to `claimed`
// through the ratified edges, which is where the scheduler hands a run to
// Dispatch. Never a hand-written illegal row.
func (e *directEnv) seedDirectRun(t *testing.T, runID, taskID, owner string) run.Run {
	t.Helper()
	ctx := context.Background()
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO users (user_id, display_name, role, created_ts) VALUES (?,?,?,?)`,
			owner, owner, "member", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO tasks (task_id, user_id, title, created_ts) VALUES (?,?,?,?)`,
			taskID, owner, "T", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r, err := e.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed} {
		r, err = e.runs.Transition(ctx, runID, to, run.TransitionOptions{Reason: "fixture", Actor: run.ActorPlatform})
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
	return r
}

func (e *directEnv) eventTypes(t *testing.T, runID string) []string {
	t.Helper()
	rows, err := e.db.QueryContext(context.Background(),
		`SELECT type FROM run_events WHERE run_id = ? ORDER BY event_seq`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// ── the run role ────────────────────────────────────────────────────────────

// TestDirectRunRoutesByItsSuffix: a `.direct` run used to answer "no skeleton
// role" and crash, which is exactly why every sampled pair rested forever.
func TestDirectRunRoutesByItsSuffix(t *testing.T) {
	for _, id := range []string{"bp-1.direct", "bp-42.direct", "bp-7.direct.g1"} {
		rl, ok := runRole(id)
		if !ok || rl != roleDirect {
			t.Errorf("runRole(%q) = %q,%v — want the direct-arm role (recovery forks included)", id, rl, ok)
		}
	}
	if rl, ok := runRole("t-1.execute"); !ok || rl != roleExecute {
		t.Errorf("the new suffix must not shadow an existing role: got %q,%v", rl, ok)
	}
}

// ── the invocation itself (rubric 16) ───────────────────────────────────────

// TestDirectArmPromptIsTheFrozenStatementAlone is the load-bearing assertion of
// the whole leg: what the direct arm receives is the §2 statement and NOTHING
// the platform adds. Field by field, because "the prompt looks right" is not the
// claim — the claim is that no worker template, no knowledge slice, no ledger
// projection and no recitation marker reached it.
func TestDirectArmPromptIsTheFrozenStatementAlone(t *testing.T) {
	a := &directAdapter{text: fixtureArmAnswer, usage: true}
	e := newDirectEnv(t, a)
	r := e.seedDirectRun(t, "bp-1.direct", "t-1", "alice")

	if err := e.sk.Dispatch(context.Background(), r); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	starts := e.started(t)
	if len(starts) != 1 {
		t.Fatalf("the direct arm ran %d sessions — §2 is single-shot", len(starts))
	}
	got := starts[0]
	if got.Worker.Prompt != fixtureStatement {
		t.Fatalf("the prompt is not the frozen statement alone:\n got: %q\nwant: %q", got.Worker.Prompt, fixtureStatement)
	}
	// The stage runner's own prompt frame must be absent: a SINET-STAGE marker
	// line would mean a stage brief was assembled around the statement.
	if strings.Contains(got.Worker.Prompt, "SINET-STAGE") {
		t.Error("the prompt carries the stage marker — a stage brief was assembled around the frozen statement")
	}
	switch {
	case got.Worker.SystemPromptAppend != "":
		t.Errorf("a system-prompt append reached the direct arm: %q", got.Worker.SystemPromptAppend)
	case len(got.Worker.AgentsJSON) != 0:
		t.Errorf("an agent definition reached the direct arm: %s", got.Worker.AgentsJSON)
	case got.Worker.AgentName != "":
		t.Errorf("a worker template named the direct arm's agent: %q", got.Worker.AgentName)
	case len(got.Worker.ToolAllowlist) != 0:
		t.Errorf("a platform toolset reached the direct arm: %v", got.Worker.ToolAllowlist)
	case len(got.Worker.GatedTools) != 0:
		t.Errorf("gated tools reached the direct arm: %v", got.Worker.GatedTools)
	case got.Worker.SessionStartContextPath != "":
		t.Errorf("a pinned-context re-injection path reached the direct arm: %q", got.Worker.SessionStartContextPath)
	case got.Worker.Recitation:
		t.Error("the recitation valve was wired on the direct arm — the platform authors nothing into this session")
	case got.Worker.PermissionMode != "":
		t.Errorf("a permission mode was lowered onto the direct arm: %q", got.Worker.PermissionMode)
	case got.OnEvent != nil:
		t.Error("a budget watcher observed the direct arm — its only purpose is proposing a split, which §2 forbids")
	case got.OnSession != nil:
		t.Error("the live session was registered with a control seam — neither split nor cancel reaches this arm")
	}
	// No ledger revision block and no pinned brief were written for this run: the
	// leg never touches the Task Context Ledger.
	for _, typ := range e.eventTypes(t, r.ID) {
		if typ == "ledger_update" || typ == "context.injected" {
			t.Errorf("the direct arm produced a %q event — no ledger assembly may touch it", typ)
		}
	}
}

// TestDirectArmCheckpointsAndCapturesAndCompletes: the §2 arm is measured like
// any other work (the §14 record needs its consumption), its text lands durably
// through the capture seam, and the run ends on the ORDINARY FSM.
func TestDirectArmCheckpointsAndCapturesAndCompletes(t *testing.T) {
	a := &directAdapter{text: fixtureArmAnswer, usage: true}
	e := newDirectEnv(t, a)
	r := e.seedDirectRun(t, "bp-2.direct", "t-2", "alice")

	if err := e.sk.Dispatch(context.Background(), r); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// The D7 checkpoint per paid call.
	var cps int
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM checkpoints WHERE run_id = ?`, r.ID).Scan(&cps); err != nil {
		t.Fatal(err)
	}
	if cps != 1 {
		t.Errorf("the direct arm recorded %d checkpoints for 1 paid call — metering must measure the arm", cps)
	}
	types := e.eventTypes(t, r.ID)
	if !contains(types, "usage.recorded") {
		t.Errorf("no usage row for the direct arm: %v — the §14 record needs its consumption", types)
	}
	// The capture, verbatim.
	caps := e.captures()
	if len(caps) != 1 {
		t.Fatalf("the leg captured %d times, want exactly 1", len(caps))
	}
	if caps[0] != r.ID+"\x1f"+fixtureArmAnswer {
		t.Errorf("the capture is not the session's final text verbatim: %q", caps[0])
	}
	// The ordinary FSM, no leg-authored transition.
	cur, err := e.runs.Get(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.State != run.StateCompleted {
		t.Errorf("the direct-arm run ended %s, want completed under the ordinary FSM", cur.State)
	}
}

// TestDirectArmTakesWhatComes: an arm that ends short of completion is an
// OUTCOME, not a condition to recover from. Its text — even an empty one — is
// still captured, because the pair's honest completion and its §6 parity note
// both depend on the pair being renderable at all. Nothing re-runs.
func TestDirectArmTakesWhatComes(t *testing.T) {
	a := &directAdapter{text: "", kind: adapters.OutcomeDiedAtGate, usage: true}
	e := newDirectEnv(t, a)
	r := e.seedDirectRun(t, "bp-3.direct", "t-3", "alice")

	if err := e.sk.Dispatch(context.Background(), r); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := len(e.started(t)); got != 1 {
		t.Fatalf("a truncated arm was run %d times — §2 forbids a retry", got)
	}
	if a.resumes != 0 {
		t.Errorf("the arm was resumed %d times — there are no follow-up turns", a.resumes)
	}
	caps := e.captures()
	if len(caps) != 1 || caps[0] != r.ID+"\x1f" {
		t.Fatalf("an arm that produced nothing must still capture its empty answer: %q", caps)
	}
	cur, err := e.runs.Get(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.State == run.StateCompleted {
		t.Error("a died-at-gate arm was recorded as completed — the run's own disposition is what the parity note reads")
	}
}

// TestCrashedArmCapturesNothing is the drain-r1 correction (D2): a crash is
// different in KIND from a truncation. An engine that died — or never spawned —
// did not answer the frozen statement at all, so capturing an empty string would
// render "the platform versus a platform failure" and ask a person to vote on
// it. The capture stays absent, which is what makes the pair a visible
// failed-pair card instead of a fake comparison.
func TestCrashedArmCapturesNothing(t *testing.T) {
	a := &directAdapter{startErr: errors.New("the engine binary is not there")}
	e := newDirectEnv(t, a)
	r := e.seedDirectRun(t, "bp-9.direct", "t-9", "alice")

	if err := e.sk.Dispatch(context.Background(), r); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := e.captures(); len(got) != 0 {
		t.Fatalf("a crashed arm captured %q — an engine that never answered must not render as an empty answer", got)
	}
	cur, err := e.runs.Get(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.State != run.StateCrashed {
		t.Errorf("the crashed arm's run is %s, want crashed — the FSM records what happened", cur.State)
	}
	// Still single-shot: a crash is not a reason to try again.
	if got := len(e.started(t)); got != 1 {
		t.Errorf("the arm was attempted %d times after a crash — §2 has no retry", got)
	}
}

// TestDirectArmRefusesWithoutItsSeams: a `.direct` run in a process with no
// benchmark composition fails LOUDLY. The only thing servable without the
// statement seam is a prompt the platform invented, and an arm answering a
// different question than the platform arm is the §3.1 confound.
func TestDirectArmRefusesWithoutItsSeams(t *testing.T) {
	a := &directAdapter{text: fixtureArmAnswer}
	e := newDirectEnv(t, a)
	e.sk.cfg.BenchmarkStatement = nil
	e.sk.cfg.BenchmarkCapture = nil
	r := e.seedDirectRun(t, "bp-4.direct", "t-4", "alice")

	err := e.sk.Dispatch(context.Background(), r)
	if !errors.Is(err, errNoBenchmarkSeams) {
		t.Fatalf("dispatch without seams: %v, want the loud refusal", err)
	}
	if got := len(e.started(t)); got != 0 {
		t.Errorf("%d sessions started on an invented prompt", got)
	}
	// And a statement seam that REFUSES (a drifted artifact of record) is the
	// same story: nothing is dispatched on a substitute.
	e2 := newDirectEnv(t, &directAdapter{text: fixtureArmAnswer})
	e2.statErr = errors.New("artifact of record has drifted from its pin")
	r2 := e2.seedDirectRun(t, "bp-5.direct", "t-5", "alice")
	if err := e2.sk.Dispatch(context.Background(), r2); err == nil {
		t.Error("a drifted artifact of record must not be papered over")
	}
	if got := len(e2.started(t)); got != 0 {
		t.Errorf("%d sessions started despite a refused statement", got)
	}
}

// TestDirectArmNeverSplitsOrRecites is the runtime probe behind the source scan
// below: a session whose footprint would trip the S05.3 overflow thresholds in
// any OTHER leg raises no proposal, requests no pause and produces no successor,
// because the direct arm is not wired to the machinery that would.
func TestDirectArmNeverSplitsOrRecites(t *testing.T) {
	// The usage event above is deliberately enormous (1.8M tokens against any
	// plausible window), so a wired watcher would certainly fire.
	a := &directAdapter{text: fixtureArmAnswer, usage: true}
	e := newDirectEnv(t, a)
	r := e.seedDirectRun(t, "bp-6.direct", "t-6", "alice")

	if err := e.sk.Dispatch(context.Background(), r); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	for _, typ := range e.eventTypes(t, r.ID) {
		if typ == EventContextOverflow {
			t.Error("the direct arm raised an overflow proposal — the split machinery must not be wired to it")
		}
		if typ == "stage.started" || typ == "stage.finished" {
			t.Errorf("the direct arm minted %q — it is the run's own session, not a stage session (§38 OQ3(a))", typ)
		}
	}
	if len(a.sessions) != 1 {
		t.Fatalf("%d sessions ran — a split successor is a second session", len(a.sessions))
	}
	if a.sessions[0].paused != 0 {
		t.Error("a pause was requested on the direct arm — the only requester is the stage split")
	}
	if a.sessions[0].cancels != 0 {
		t.Error("the cancel ladder ran on the direct arm — no automated path may reach it")
	}
}

// TestDirectLegReachesNoMultiTurnMachinery is the SOURCE assertion behind the
// runtime probe: the leg's own file must not name the constructs that would make
// the arm multi-turn, retried or platform-shaped. A future edit that wires one
// in fails here even if its runtime effect is subtle.
func TestDirectLegReachesNoMultiTurnMachinery(t *testing.T) {
	src, err := os.ReadFile("direct.go")
	if err != nil {
		t.Fatal(err)
	}
	body := stripComments(t, string(src))
	for _, banned := range []string{
		"Session(",           // the stage runner: assembles a brief and watches a budget
		"newBudgetWatcher",   // the overflow proposer
		"splitRequester",     // the split executor
		"newReciter",         // the per-turn recitation channel
		"DriveResume",        // a follow-up turn
		"Assemble",           // a ledger projection
		"CompileForRun",      // a worker template
		"launchRole",         // a successor run
		"cancels.register",   // the human-cancel registry
		"pauseParkPoint",     // a mid-leg park boundary
		"internal/benchmark", // the §35 import wall
	} {
		if strings.Contains(body, banned) {
			t.Errorf("direct.go references %q — the §2 arm is single-shot, unshaped and importless", banned)
		}
	}
	// Non-tautological: the scan must SEE the things the leg does do.
	for _, required := range []string{"driver.Drive(", "BenchmarkStatement", "BenchmarkCapture"} {
		if !strings.Contains(body, required) {
			t.Fatalf("the scan cannot see %q — it would pass vacuously", required)
		}
	}
}

// stripComments returns a file's CODE with its comments removed, so a scan for
// forbidden constructs is not tripped by the prose describing why they are
// forbidden — which this file's subject has a great deal of.
func stripComments(t *testing.T, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "direct.go", src, 0) // no ParseComments: they are dropped
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := printer.Fprint(&b, fset, f); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

func (e *directEnv) started(t *testing.T) []adapters.StartRequest {
	t.Helper()
	return e.adapter.started()
}
