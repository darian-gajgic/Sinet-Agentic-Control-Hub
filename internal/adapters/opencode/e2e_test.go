package opencode

// e2e_test.go — tier F, Driver integration (R18): the adapter EMITS and the
// existing internal/adapters Driver persists. Nothing in this package writes a
// checkpoint, an ask row, or an engine_sessions row of its own; this suite is
// how that stays true.
//
// Harness shape is the claudecli e2e precedent (newE2E): storage + run store +
// checkpoints + event log + Driver, all on t.TempDir().

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

type e2eEnv struct {
	db   *storage.DB
	log  *eventlog.Log
	runs *run.Store
	drv  *adapters.Driver
}

func newE2E(t *testing.T) *e2eEnv {
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
	return &e2eEnv{
		db: db, log: log, runs: runs,
		drv: &adapters.Driver{
			Runs: runs, Checkpoints: gates.NewCheckpoints(db, log), Log: log, DB: db,
			CopyAsideDir: filepath.Join(t.TempDir(), "copy-aside"),
		},
	}
}

func (e *e2eEnv) claimedRun(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{
		ID: id, UserID: "u1", Substrate: adapters.SubstrateOpencode, Lane: adapters.LaneZAI,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := e.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}
}

func TestDriverIntegrationCheckpointPerPaidCall(t *testing.T) {
	e := newE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.claimedRun(t, "r1")

	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("tooltrun.sse") }
	a := fakeAdapter(t, f)
	req := testRequest(t)

	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s)", out.Kind, out.Detail)
	}
	if r, _ := e.runs.Get(ctx, "r1"); r.State != run.StateCompleted {
		t.Fatalf("run state = %s, want completed", r.State)
	}

	// One checkpoint row per PAID CALL (D7), never per stream frame.
	var checkpoints int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM checkpoints WHERE run_id = 'r1'`).Scan(&checkpoints); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if checkpoints != 2 {
		t.Errorf("checkpoint rows = %d, want 2 (one per paid call in a tool-using turn)", checkpoints)
	}
	cps := gates.NewCheckpoints(e.db, e.log)
	cp, ok, err := cps.Last(ctx, "r1")
	if err != nil || !ok {
		t.Fatalf("last checkpoint: ok=%v err=%v", ok, err)
	}
	if cp.SessionSubstrate != adapters.SubstrateOpencode {
		t.Errorf("checkpoint substrate = %q", cp.SessionSubstrate)
	}
	if cp.SessionID != fixtureSessionID {
		t.Errorf("checkpoint session id = %q, want the engine-REPORTED id (S02.4b)", cp.SessionID)
	}
	if cp.TranscriptPath != "" {
		t.Errorf("checkpoint transcript path = %q, want empty (opencode keeps no JSONL transcript)", cp.TranscriptPath)
	}
	if cp.InvocationFingerprint == "" {
		t.Error("checkpoint lost the invocation fingerprint (S02.4e)")
	}

	// engine_sessions row present; copy-aside naturally inert (no transcript).
	var sid, copyPath string
	if err := e.db.QueryRowContext(ctx,
		`SELECT engine_session_id, transcript_copy_path FROM engine_sessions WHERE run_id = 'r1'`).
		Scan(&sid, &copyPath); err != nil {
		t.Fatalf("engine_sessions: %v", err)
	}
	if sid != fixtureSessionID {
		t.Errorf("engine_sessions id = %q", sid)
	}
	if copyPath != "" {
		t.Errorf("copy-aside path = %q, want empty — there is no transcript to copy on this substrate", copyPath)
	}

	// The usage events landed under the S14.2 canonical type, and the adapter
	// wrote no parallel persistence of its own.
	rows, err := e.db.QueryContext(ctx, `SELECT type FROM run_events WHERE run_id = 'r1' ORDER BY event_seq`)
	if err != nil {
		t.Fatalf("run_events: %v", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatalf("scan: %v", err)
		}
		counts[typ]++
	}
	if counts["usage.recorded"] != 2 {
		t.Errorf("usage.recorded events = %d, want 2", counts["usage.recorded"])
	}
	if counts["engine.done"] != 1 {
		t.Errorf("engine.done events = %d, want 1", counts["engine.done"])
	}
	if counts["checkpoint.written"] != 2 {
		t.Errorf("checkpoint.written events = %d, want 2 (same tx as the usage append)", counts["checkpoint.written"])
	}
}

func TestDriverIntegrationGateParkOpensDurableAsk(t *testing.T) {
	e := newE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.claimedRun(t, "r2")

	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) {
		f.setPermissions(fixtureAsk())
		f.publishFixture("gate.sse")
	}
	a := fakeAdapter(t, f)
	req := testRequest(t)
	req.RunID = "r2"

	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeParked || out.Ask == nil {
		t.Fatalf("outcome = %q ask=%+v, want parked on the gate", out.Kind, out.Ask)
	}
	if r, _ := e.runs.Get(ctx, "r2"); r.State != run.StateParked {
		t.Fatalf("run state = %s, want parked", r.State)
	}
	// The durable ask-record is a platform record from the moment the ask is
	// observed (S03.4) — the Driver upserts it; this package never does.
	var status string
	if err := e.db.QueryRowContext(ctx,
		`SELECT status FROM asks WHERE ask_id = ?`, out.Ask.ID).Scan(&status); err != nil {
		t.Fatalf("ask row for %q: %v", out.Ask.ID, err)
	}
	if status != "open" {
		t.Errorf("ask status = %q, want open", status)
	}

	// Resume answers it: same session id, one reply, run back to a terminal
	// state, ask marked answered in the resume transaction.
	f2 := newFakeServe(t)
	f2.setPermissions(fixtureAsk())
	f2.onReply = func(f *fakeServe, _ fakeReply) { f.publishFixture("gate-replied.sse") }
	a2 := fakeAdapter(t, f2)
	out2, err := e.drv.DriveResume(ctx, a2, *out.Park, ApproveAnswer(out.Ask.ID))
	if err != nil {
		t.Fatalf("DriveResume: %v", err)
	}
	if out2.Kind != adapters.OutcomeCompleted {
		t.Fatalf("resume outcome = %q (%s)", out2.Kind, out2.Detail)
	}
	if err := e.db.QueryRowContext(ctx,
		`SELECT status FROM asks WHERE ask_id = ?`, out.Ask.ID).Scan(&status); err != nil {
		t.Fatalf("ask row after resume: %v", err)
	}
	if status != "answered" {
		t.Errorf("ask status after resume = %q, want answered", status)
	}
	replies := f2.repliesSeen()
	if len(replies) != 1 || replies[0].Reply != "once" {
		t.Errorf("wire replies = %+v, want exactly one \"once\"", replies)
	}
}
