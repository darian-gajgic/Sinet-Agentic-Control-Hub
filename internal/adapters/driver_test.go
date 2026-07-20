package adapters_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// Contract-level Driver suite: a fake in-memory Adapter drives the real
// storage/eventlog/run/gates machinery, asserting the B1 end-to-end bar —
// spawn → engine events land in run_events (fenced) → checkpoint rows per
// S02.4 → engine_sessions maintained → ask rows durable on observation.

type env struct {
	db   *storage.DB
	log  *eventlog.Log
	runs *run.Store
	cps  *gates.Checkpoints
	drv  *adapters.Driver
}

func newEnv(t *testing.T) *env {
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
	e := &env{db: db, log: log, runs: runs, cps: gates.NewCheckpoints(db, log)}
	e.drv = &adapters.Driver{
		Runs: runs, Checkpoints: e.cps, Log: log, DB: db,
		CopyAsideDir: filepath.Join(t.TempDir(), "copy-aside"),
	}
	return e
}

// claimedRun creates a run and walks it new→queued→claimed (Spec S02.3).
func (e *env) claimedRun(t *testing.T, id string) run.Run {
	t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: id, UserID: "u1", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := e.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition to %s: %v", st, err)
		}
	}
	r, err := e.runs.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return r
}

// fakeSession replays a scripted event stream.
type fakeSession struct {
	events      chan adapters.Event
	cursor      adapters.Cursor
	fingerprint string
	outcome     adapters.Outcome
}

func (s *fakeSession) Events() <-chan adapters.Event { return s.events }
func (s *fakeSession) Cursor() adapters.Cursor       { return s.cursor }
func (s *fakeSession) Fingerprint() string           { return s.fingerprint }
func (s *fakeSession) Pause(context.Context) error   { return nil }
func (s *fakeSession) Cancel(context.Context) error  { return nil }
func (s *fakeSession) Wait(ctx context.Context) (adapters.Outcome, error) {
	return s.outcome, nil
}

type fakeAdapter struct {
	script  []adapters.Event
	cursor  adapters.Cursor
	outcome adapters.Outcome

	started *adapters.StartRequest
	resumed *adapters.ParkRecord
	answer  *adapters.Answer

	startErr error
}

func (a *fakeAdapter) Substrate() string { return adapters.SubstrateClaudeCLI }

func (a *fakeAdapter) session() *fakeSession {
	s := &fakeSession{
		events:      make(chan adapters.Event, len(a.script)+1),
		cursor:      a.cursor,
		fingerprint: "fp-test",
		outcome:     a.outcome,
	}
	for _, ev := range a.script {
		s.events <- ev
	}
	close(s.events)
	return s
}

func (a *fakeAdapter) Start(ctx context.Context, req adapters.StartRequest) (adapters.Session, error) {
	if a.startErr != nil {
		return nil, a.startErr
	}
	a.started = &req
	return a.session(), nil
}

func (a *fakeAdapter) Resume(ctx context.Context, rec adapters.ParkRecord, ans *adapters.Answer) (adapters.Session, error) {
	a.resumed = &rec
	a.answer = ans
	return a.session(), nil
}

func usageEvent(id string, index int64) adapters.Event {
	u := &adapters.Usage{
		ModelID: "claude-haiku-4-5", MessageID: id, MessageIndex: index,
		InputTokens: 10, OutputTokens: 4, CacheCreationTokens: 120,
		CacheCreationByTTL: map[string]int64{"ephemeral_5m_input_tokens": 120},
		Raw:                json.RawMessage(`{"input_tokens":10,"output_tokens":4}`),
	}
	return adapters.Event{Kind: adapters.KindUsage, Payload: json.RawMessage(`{"message_id":"` + id + `"}`), Usage: u}
}

func TestDriveHappyPath(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.claimedRun(t, "r1")

	transcript := filepath.Join(t.TempDir(), "sess.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"line":1}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed transcript: %v", err)
	}
	fa := &fakeAdapter{
		cursor: adapters.Cursor{
			Substrate: adapters.SubstrateClaudeCLI, SessionID: "reported-sid",
			MessageIndex: 1, CwdKey: "/tmp/run-cwd", TranscriptPath: transcript,
		},
		script: []adapters.Event{
			{Kind: adapters.KindMessage, Payload: json.RawMessage(`{"excerpt":"ok"}`)},
			usageEvent("msg_1", 1),
			{Kind: adapters.KindRateLimit, Payload: json.RawMessage(`{"status":"allowed"}`)},
			{Kind: adapters.KindDone, Payload: json.RawMessage(`{"subtype":"success"}`)},
		},
		outcome: adapters.Outcome{Kind: adapters.OutcomeCompleted},
	}
	e.drv.Snapshot = func(ctx context.Context, runID string) (string, error) { return "snap-1", nil }

	out, err := e.drv.Drive(ctx, fa, adapters.StartRequest{
		RunID: "r1", UserID: "u1", Model: "claude-haiku-4-5",
		Worker: adapters.CompiledWorker{ToolSchemaVersion: "ts1", PromptSchemaVersion: "ps1"},
	})
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome %q, want completed", out.Kind)
	}
	r, err := e.runs.Get(ctx, "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if r.State != run.StateCompleted {
		t.Fatalf("run state %s, want completed", r.State)
	}

	// Engine events landed, in order, fenced at the running generation.
	types := eventTypes(t, e, "r1")
	want := []string{"run.created", "run.state", "run.state", "run.state", // create + queued + claimed + running
		"engine.message", "engine.usage", "run.checkpoint", // paid call: usage event + checkpoint event, one tx (S02.3/S02.4)
		"engine.rate_limit", "engine.done",
		"run.state"} // completed
	if len(types) != len(want) {
		t.Fatalf("event types %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("event[%d] = %s, want %s (all: %v)", i, types[i], want[i], types)
		}
	}

	// One checkpoint row per paid call, all five S02.4 blocks.
	cp, ok, err := e.cps.Last(ctx, "r1")
	if err != nil || !ok {
		t.Fatalf("Last checkpoint: ok=%v err=%v", ok, err)
	}
	if string(cp.Usage) != `{"input_tokens":10,"output_tokens":4}` {
		t.Errorf("usage block = %s", cp.Usage)
	}
	if cp.SessionSubstrate != adapters.SubstrateClaudeCLI || cp.SessionID != "reported-sid" ||
		cp.MessageIndex != 1 || cp.CwdKey != "/tmp/run-cwd" || cp.TranscriptPath != transcript {
		t.Errorf("session cursor block = %+v", cp)
	}
	if cp.LedgerRevision != "" { // S05's seam stays empty at B1 (B2 owns it)
		t.Errorf("ledger revision = %q, want empty seam", cp.LedgerRevision)
	}
	if cp.ArtifactSnapshotRef != "snap-1" {
		t.Errorf("artifact snapshot ref = %q, want snap-1", cp.ArtifactSnapshotRef)
	}
	if cp.ModelID != "claude-haiku-4-5" || cp.InvocationFingerprint != "fp-test" ||
		cp.ToolSchemaVersion != "ts1" || cp.PromptSchemaVersion != "ps1" {
		t.Errorf("version fields = %+v", cp)
	}

	// engine_sessions row maintained + transcript copied aside.
	var sid, copyPath string
	err = e.db.QueryRowContext(ctx,
		`SELECT engine_session_id, transcript_copy_path FROM engine_sessions WHERE run_id = 'r1'`).
		Scan(&sid, &copyPath)
	if err != nil {
		t.Fatalf("engine_sessions: %v", err)
	}
	if sid != "reported-sid" {
		t.Errorf("engine_session_id = %q", sid)
	}
	if copyPath == "" {
		t.Fatalf("transcript_copy_path empty, want copy-aside")
	}
	got, err := os.ReadFile(copyPath)
	if err != nil || string(got) != `{"line":1}`+"\n" {
		t.Errorf("copy-aside content %q err=%v", got, err)
	}
}

func TestDriveGateAskThenResume(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.claimedRun(t, "r2")

	park := &adapters.ParkRecord{
		RunID: "r2", Substrate: adapters.SubstrateClaudeCLI,
		Cursor:      adapters.Cursor{Substrate: adapters.SubstrateClaudeCLI, SessionID: "sid-2"},
		Reason:      adapters.ParkReasonGateAsk,
		Start:       adapters.StartRequest{RunID: "r2", UserID: "u1", Model: "m"},
		Fingerprint: "fp-2", ParkedAt: time.Now(),
	}
	ask := &adapters.Ask{
		ID: "toolu_ask1", ToolName: "Bash",
		ToolInput:    json.RawMessage(`{"command":"echo HELLO"}`),
		EngineExpiry: time.Now().Add(365 * 24 * time.Hour),
		Park:         park,
	}
	fa := &fakeAdapter{
		cursor: park.Cursor,
		script: []adapters.Event{
			usageEvent("msg_1", 1),
			{Kind: adapters.KindGateAsk, Payload: json.RawMessage(`{"ask_id":"toolu_ask1"}`), Ask: ask},
		},
		outcome: adapters.Outcome{Kind: adapters.OutcomeParked, Park: park, Ask: ask},
	}
	out, err := e.drv.Drive(ctx, fa, park.Start)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeParked {
		t.Fatalf("outcome %q, want parked", out.Kind)
	}
	r, _ := e.runs.Get(ctx, "r2")
	if r.State != run.StateParked {
		t.Fatalf("run state %s, want parked", r.State)
	}
	genAtPark := r.Generation

	// Durable ask-record from the moment observed (S03.4).
	var status, snapshot string
	if err := e.db.QueryRowContext(ctx,
		`SELECT status, snapshot FROM asks WHERE ask_id = 'toolu_ask1'`).Scan(&status, &snapshot); err != nil {
		t.Fatalf("ask row: %v", err)
	}
	if status != "open" {
		t.Errorf("ask status %q, want open", status)
	}
	var snap adapters.Ask
	if err := json.Unmarshal([]byte(snapshot), &snap); err != nil {
		t.Fatalf("snapshot round-trip: %v", err)
	}
	if snap.Park == nil || snap.Park.Fingerprint != "fp-2" || snap.Park.Start.Model != "m" {
		t.Errorf("snapshot lost the invocation-reconstruction record: %+v", snap.Park)
	}

	// Resume: answer recorded, generation bumped, stale appends fenced.
	fa2 := &fakeAdapter{
		cursor:  park.Cursor,
		script:  []adapters.Event{usageEvent("msg_2", 2)},
		outcome: adapters.Outcome{Kind: adapters.OutcomeCompleted},
	}
	ans := &adapters.Answer{AskID: "toolu_ask1", UpdatedInput: json.RawMessage(`{"command":"echo ANSWER-42"}`)}
	out, err = e.drv.DriveResume(ctx, fa2, *park, ans)
	if err != nil {
		t.Fatalf("DriveResume: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("resume outcome %q", out.Kind)
	}
	if fa2.answer == nil || string(fa2.answer.UpdatedInput) != `{"command":"echo ANSWER-42"}` {
		t.Errorf("adapter did not receive the answer: %+v", fa2.answer)
	}
	if err := e.db.QueryRowContext(ctx,
		`SELECT status FROM asks WHERE ask_id = 'toolu_ask1'`).Scan(&status); err != nil {
		t.Fatalf("ask row after resume: %v", err)
	}
	if status != "answered" {
		t.Errorf("ask status %q, want answered", status)
	}
	r, _ = e.runs.Get(ctx, "r2")
	if r.State != run.StateCompleted {
		t.Fatalf("run state %s, want completed", r.State)
	}
	if r.Generation != genAtPark+1 {
		t.Fatalf("generation %d, want %d (parked→running bumps, S02.3)", r.Generation, genAtPark+1)
	}
	// The fence: an append at the pre-resume generation is rejected.
	_, err = e.log.Append(ctx, eventlog.Append{
		RunID: "r2", Generation: genAtPark, UserID: "u1",
		Type: "engine.message", SchemaVersion: 1, Payload: json.RawMessage(`{}`),
	})
	if !errors.Is(err, eventlog.ErrStaleGeneration) {
		t.Fatalf("stale append error = %v, want ErrStaleGeneration", err)
	}
}

func TestDriveDiedAtGate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.claimedRun(t, "r3")
	park := &adapters.ParkRecord{
		RunID: "r3", Substrate: adapters.SubstrateClaudeCLI, Reason: adapters.ParkReasonGateAsk,
		Start: adapters.StartRequest{RunID: "r3", UserID: "u1", Model: "m"},
	}
	fa := &fakeAdapter{
		outcome: adapters.Outcome{Kind: adapters.OutcomeDiedAtGate, Park: park,
			Detail: "engine budget preempted the park"},
	}
	out, err := e.drv.Drive(ctx, fa, park.Start)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeDiedAtGate {
		t.Fatalf("outcome %q", out.Kind)
	}
	r, _ := e.runs.Get(ctx, "r3")
	if r.State != run.StateDiedAtGate {
		t.Fatalf("run state %s, want died-at-gate (via parked, S02.3)", r.State)
	}
}

func TestDriveCrashAndSpawnFailure(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	e.claimedRun(t, "r4")
	fa := &fakeAdapter{outcome: adapters.Outcome{Kind: adapters.OutcomeCrashed, Detail: "boom"}}
	if _, err := e.drv.Drive(ctx, fa, adapters.StartRequest{RunID: "r4", UserID: "u1", Model: "m"}); err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if r, _ := e.runs.Get(ctx, "r4"); r.State != run.StateCrashed {
		t.Fatalf("run state %s, want crashed", r.State)
	}

	e.claimedRun(t, "r5")
	fb := &fakeAdapter{startErr: errors.New("no binary")}
	out, err := e.drv.Drive(ctx, fb, adapters.StartRequest{RunID: "r5", UserID: "u1", Model: "m"})
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCrashed {
		t.Fatalf("outcome %q, want crashed", out.Kind)
	}
	if r, _ := e.runs.Get(ctx, "r5"); r.State != run.StateCrashed {
		t.Fatalf("run state %s, want crashed after spawn failure", r.State)
	}
}

func TestDriveCanceledLeavesDisposition(t *testing.T) {
	// No ratified S02.3 cancel edge: the driver reports the outcome and
	// leaves the row to the canceling surface (driver doc).
	e := newEnv(t)
	ctx := context.Background()
	e.claimedRun(t, "r6")
	fa := &fakeAdapter{outcome: adapters.Outcome{Kind: adapters.OutcomeCanceled}}
	out, err := e.drv.Drive(ctx, fa, adapters.StartRequest{RunID: "r6", UserID: "u1", Model: "m"})
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCanceled {
		t.Fatalf("outcome %q", out.Kind)
	}
	if r, _ := e.runs.Get(ctx, "r6"); r.State != run.StateRunning {
		t.Fatalf("run state %s, want running (disposition is the caller's)", r.State)
	}
}

func TestDriveRejectsWrongState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: "r7", UserID: "u1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := e.drv.Drive(ctx, &fakeAdapter{}, adapters.StartRequest{RunID: "r7", UserID: "u1", Model: "m"})
	if !errors.Is(err, adapters.ErrNotDrivable) {
		t.Fatalf("err = %v, want ErrNotDrivable", err)
	}
	_, err = e.drv.DriveResume(ctx, &fakeAdapter{}, adapters.ParkRecord{RunID: "r7"}, nil)
	if !errors.Is(err, adapters.ErrNotDrivable) {
		t.Fatalf("resume err = %v, want ErrNotDrivable", err)
	}
}

func eventTypes(t *testing.T, e *env, runID string) []string {
	t.Helper()
	rows, err := e.db.QueryContext(context.Background(),
		`SELECT type FROM run_events WHERE run_id = ? ORDER BY event_seq`, runID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatalf("scan: %v", err)
		}
		types = append(types, typ)
	}
	return types
}

// TestCheckpointLedgerRevisionSeam fills the S02.4(c) block through the
// real ledger machinery: the checkpoint row records the ledger_version +
// content hash of the run's task ledger (Spec S05.2 duty 1), resolvable
// back to the exact revision — the D7 context payload.
func TestCheckpointLedgerRevisionSeam(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// A task-bearing run (the ledger is a per-task artifact, Spec S05.1).
	err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES ('t1', 'u1', 't', '2026-07-20T00:00:00Z')`)
		return err
	})
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := e.runs.Create(ctx, run.NewRun{ID: "r-led", UserID: "u1", TaskID: "t1", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := e.runs.Transition(ctx, "r-led", st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition to %s: %v", st, err)
		}
	}

	led := ledger.NewStore(e.db, e.log)
	if _, err := led.SetObjective(ctx, "r-led", "platform", ledger.ObjectiveAC{
		Objective:          "checkpointed objective",
		AcceptanceCriteria: []ledger.AcceptanceCriterion{{N: 1, Plain: "done"}},
		SpecVersion:        "spec-v1",
	}); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}
	e.drv.LedgerRevision = led.CheckpointRef

	fa := &fakeAdapter{
		cursor:  adapters.Cursor{Substrate: adapters.SubstrateClaudeCLI, SessionID: "sid"},
		script:  []adapters.Event{usageEvent("msg_1", 1)},
		outcome: adapters.Outcome{Kind: adapters.OutcomeCompleted},
	}
	if _, err := e.drv.Drive(ctx, fa, adapters.StartRequest{RunID: "r-led", UserID: "u1", Model: "m"}); err != nil {
		t.Fatalf("Drive: %v", err)
	}

	cp, ok, err := e.cps.Last(ctx, "r-led")
	if err != nil || !ok {
		t.Fatalf("Last: %v ok=%v", err, ok)
	}
	ref, ok, err := ledger.ParseRevisionRef(cp.LedgerRevision)
	if err != nil || !ok {
		t.Fatalf("checkpoint ledger_revision %q: %v ok=%v", cp.LedgerRevision, err, ok)
	}
	if ref.LedgerVersion != 1 || len(ref.SHA256) != 64 {
		t.Fatalf("revision ref = %+v", ref)
	}
	// The ref resolves to the exact revision content (D7 self-containment).
	doc, err := led.AtVersion(ctx, "t1", ref.LedgerVersion)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	if doc.ObjectiveAC.Objective != "checkpointed objective" {
		t.Fatalf("resolved revision = %+v", doc.ObjectiveAC)
	}
}
