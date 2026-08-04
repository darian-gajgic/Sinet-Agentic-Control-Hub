package adapters_test

// Interaction-boundary crash cases (P3-B3-4; Research/18 §7-C3 — the
// kill-point-matrix test shape adopted as ordinary S03 conformance-suite
// growth): the platform dies at the two nastiest moments of the ask
// lifecycle and every later act reconstructs from DB state ALONE — no
// in-memory carryover, the moral equivalent of a boot reconcile.
//
// Dated engine-fact evidence notes (Research/18 §3-C3, codor probes at
// commit 305670fb554, recorded 2026-07-20 on the Agent-SDK path — verify
// at Sinet's own pin before relying on any of them):
//
//  1. Re-raised interactions carry FRESH native request/tool_use ids
//     after a crash-resume; correlation must ride a durable semantic
//     record, never native ids. (Sinet's own -p defer path measured the
//     OPPOSITE — --resume re-fires PreToolUse on the SAME tool_use_id,
//     SPIKE G1-S2 — and the durable ask row is authoritative either way:
//     P-T02-1, S03.4.)
//  2. Answered approvals are never auto-resent; answered asks replay
//     idempotently (codor's boot-reconcile kill-point matrix).
//  3. Empty/error output never speaks as the agent — the named negative
//     row lives at the lane: claudecli's
//     TestErrorOutputNeverBecomesStageOutput.

import (
	"context"
	"encoding/json"
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

// envAt builds the driver environment over an EXPLICIT DB path so a test
// can close it and re-open the same file — the platform-restart
// simulation.
func envAt(t *testing.T, dbPath string) *env {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, dbPath, settings.New())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, settings.New())
	runs := run.NewStore(db, log)
	e := &env{db: db, log: log, runs: runs, cps: gates.NewCheckpoints(db, log)}
	e.drv = &adapters.Driver{Runs: runs, Checkpoints: e.cps, Log: log, DB: db,
		CopyAsideDir: filepath.Join(t.TempDir(), "copy-aside")}
	return e
}

func gateParkFixture(runID string) (*adapters.ParkRecord, *adapters.Ask) {
	park := &adapters.ParkRecord{
		RunID: runID, Substrate: adapters.SubstrateClaudeCLI,
		Cursor:      adapters.Cursor{Substrate: adapters.SubstrateClaudeCLI, SessionID: "sid-cb"},
		Reason:      adapters.ParkReasonGateAsk,
		Start:       adapters.StartRequest{RunID: runID, UserID: "u1", Model: "m"},
		Fingerprint: "fp-cb", ParkedAt: time.Now(),
	}
	ask := &adapters.Ask{
		ID: "toolu_cb1", ToolName: "Bash",
		ToolInput:    json.RawMessage(`{"command":"echo GATED"}`),
		EngineExpiry: time.Now().Add(365 * 24 * time.Hour),
		Park:         park,
	}
	return park, ask
}

// TestRestartWithPendingAskResumesFromRowAlone: the platform dies AFTER
// the ask was observed and the run parked. On "reboot" the durable ask
// row is the only surviving artifact; the resume must reconstruct the
// FULL invocation from its snapshot (S03.4 obligation) and land the
// answer exactly once.
func TestRestartWithPendingAskResumesFromRowAlone(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), storage.DBFileName)

	// Life 1: drive to the gate park, then die (close the DB).
	e1 := envAt(t, dbPath)
	e1.claimedRun(t, "r-cb1")
	park, ask := gateParkFixture("r-cb1")
	fa := &fakeAdapter{
		cursor: park.Cursor,
		script: []adapters.Event{
			usageEvent("msg_1", 1),
			{Kind: adapters.KindGateAsk, Payload: json.RawMessage(`{"ask_id":"toolu_cb1"}`), Ask: ask},
		},
		outcome: adapters.Outcome{Kind: adapters.OutcomeParked, Park: park, Ask: ask},
	}
	if out, err := e1.drv.Drive(ctx, fa, park.Start); err != nil || out.Kind != adapters.OutcomeParked {
		t.Fatalf("Drive: out=%v err=%v", out.Kind, err)
	}
	if err := e1.db.Close(); err != nil {
		t.Fatalf("simulated death: %v", err)
	}

	// Life 2: fresh process state over the same DB. The pending ask is
	// found open; its snapshot alone rebuilds the park record.
	e2 := envAt(t, dbPath)
	t.Cleanup(func() { e2.db.Close() })
	var status, snapshot string
	if err := e2.db.QueryRowContext(ctx,
		`SELECT status, snapshot FROM asks WHERE ask_id = 'toolu_cb1'`).Scan(&status, &snapshot); err != nil {
		t.Fatalf("ask row after restart: %v", err)
	}
	if status != "open" {
		t.Fatalf("ask status %q after restart, want open (restart-with-pending)", status)
	}
	var recovered adapters.Ask
	if err := json.Unmarshal([]byte(snapshot), &recovered); err != nil {
		t.Fatalf("snapshot decode: %v", err)
	}
	if recovered.Park == nil || recovered.Park.Fingerprint != "fp-cb" ||
		recovered.Park.Start.Model != "m" || recovered.Park.Cursor.SessionID != "sid-cb" {
		t.Fatalf("snapshot does not reconstruct the invocation: %+v", recovered.Park)
	}

	fa2 := &fakeAdapter{
		cursor:  recovered.Park.Cursor,
		script:  []adapters.Event{usageEvent("msg_2", 2)},
		outcome: adapters.Outcome{Kind: adapters.OutcomeCompleted},
	}
	ans := &adapters.Answer{AskID: "toolu_cb1", UpdatedInput: json.RawMessage(`{"command":"echo OK"}`)}
	out, err := e2.drv.DriveResume(ctx, fa2, *recovered.Park, ans)
	if err != nil || out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("resume from recovered row: out=%v err=%v", out.Kind, err)
	}
	if fa2.resumed == nil || fa2.resumed.Fingerprint != "fp-cb" {
		t.Errorf("adapter resumed without the reconstructed record: %+v", fa2.resumed)
	}
	if err := e2.db.QueryRowContext(ctx,
		`SELECT status FROM asks WHERE ask_id = 'toolu_cb1'`).Scan(&status); err != nil || status != "answered" {
		t.Errorf("ask status = %q err=%v, want answered exactly once", status, err)
	}
	if r, _ := e2.runs.Get(ctx, "r-cb1"); r.State != run.StateCompleted {
		t.Errorf("run state %s, want completed", r.State)
	}
}

// TestCrashAfterAnswerNeverReasksOrDowngrades: the platform dies after
// the answer transaction committed but before the resumed engine did any
// work (spawn failure = the crash-at-resume window). The answered ask
// must stay answered — never re-opened, never auto-re-asked (§3-C3
// note 2) — and the run is crashed for the recovery ladder, whose fork
// lineage re-verifies into its OWN records rather than resurrecting the
// consumed interaction.
func TestCrashAfterAnswerNeverReasksOrDowngrades(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), storage.DBFileName)

	e1 := envAt(t, dbPath)
	e1.claimedRun(t, "r-cb2")
	park, ask := gateParkFixture("r-cb2")
	park.RunID, park.Start.RunID, ask.ID = "r-cb2", "r-cb2", "toolu_cb2"
	fa := &fakeAdapter{
		cursor: park.Cursor,
		script: []adapters.Event{
			{Kind: adapters.KindGateAsk, Payload: json.RawMessage(`{"ask_id":"toolu_cb2"}`), Ask: ask},
		},
		outcome: adapters.Outcome{Kind: adapters.OutcomeParked, Park: park, Ask: ask},
	}
	if out, err := e1.drv.Drive(ctx, fa, park.Start); err != nil || out.Kind != adapters.OutcomeParked {
		t.Fatalf("Drive: out=%v err=%v", out.Kind, err)
	}

	// The resume's answer tx commits; the engine never comes up (the
	// adapter refuses — the tightest crash window after the commit).
	fa2 := &resumeRefusingAdapter{}
	ans := &adapters.Answer{AskID: "toolu_cb2"}
	out, err := e1.drv.DriveResume(ctx, fa2, *park, ans)
	if err != nil || out.Kind != adapters.OutcomeCrashed {
		t.Fatalf("resume with dead engine: out=%v err=%v", out.Kind, err)
	}
	if err := e1.db.Close(); err != nil {
		t.Fatalf("simulated death: %v", err)
	}

	// Restart: the answer SURVIVES — status answered, payload intact; the
	// interaction is consumed, not re-raisable; the crashed run waits for
	// the S02.5 ladder.
	e2 := envAt(t, dbPath)
	t.Cleanup(func() { e2.db.Close() })
	var status, answer string
	if err := e2.db.QueryRowContext(ctx,
		`SELECT status, COALESCE(answer,'') FROM asks WHERE ask_id = 'toolu_cb2'`).Scan(&status, &answer); err != nil {
		t.Fatalf("ask row after restart: %v", err)
	}
	if status != "answered" {
		t.Errorf("ask status %q after crash-past-answer, want answered (never re-opened, never auto-resent)", status)
	}
	if answer == "" {
		t.Error("answer payload lost across the crash")
	}
	if r, _ := e2.runs.Get(ctx, "r-cb2"); r.State != run.StateCrashed {
		t.Errorf("run state %s, want crashed (recovery ladder territory, S02.5)", r.State)
	}
}

// resumeRefusingAdapter fails exactly the Resume spawn — the crash window
// immediately after the answer transaction committed.
type resumeRefusingAdapter struct{ fakeAdapter }

func (a *resumeRefusingAdapter) Resume(ctx context.Context, rec adapters.ParkRecord, ans *adapters.Answer) (adapters.Session, error) {
	return nil, errSpawnRefused
}

var errSpawnRefused = &spawnRefusedError{}

type spawnRefusedError struct{}

func (*spawnRefusedError) Error() string { return "engine refused to spawn (simulated crash window)" }
