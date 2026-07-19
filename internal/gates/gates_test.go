package gates_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

type env struct {
	db    *storage.DB
	log   *eventlog.Log
	runs  *run.Store
	cps   *gates.Checkpoints
	jrnl  *gates.Journal
	clock *fakeClock
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

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
	clock := &fakeClock{t: time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)}
	jrnl, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg, Now: clock.now})
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	return &env{db: db, log: log, runs: run.NewStore(db, log), cps: gates.NewCheckpoints(db, log), jrnl: jrnl, clock: clock}
}

func (e *env) runningRun(t *testing.T, id string) run.Run {
	t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: id, UserID: "u1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var r run.Run
	var err error
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if r, err = e.runs.Transition(ctx, id, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("→%s: %v", to, err)
		}
	}
	return r
}

func TestCheckpointWriteLinksEvent(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun(t, "r1")

	cp, err := e.cps.Write(ctx, gates.NewCheckpoint{
		RunID:                 "r1",
		Usage:                 json.RawMessage(`{"input_tokens":10,"output_tokens":5,"cost_usd":0.01}`),
		SessionSubstrate:      "claude",
		SessionID:             "sess-1",
		MessageIndex:          3,
		CwdKey:                "/w",
		TranscriptPath:        "/w/t.jsonl",
		LedgerRevision:        "rev-1",
		ArtifactSnapshotRef:   "snap-1",
		ModelID:               "model-x",
		InvocationFingerprint: "fp-1",
		ToolSchemaVersion:     "ts-1",
		PromptSchemaVersion:   "ps-1",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if cp.UserID != "u1" {
		t.Fatalf("checkpoint user = %q, want the run owner u1 (15.6)", cp.UserID)
	}
	// The row links to a real run.checkpoint event appended in the same tx.
	evs, err := e.log.After(ctx, cp.EventSeq-1, 1)
	if err != nil || len(evs) != 1 {
		t.Fatalf("event at %d: %v %v", cp.EventSeq, evs, err)
	}
	if evs[0].Type != gates.EventCheckpoint || evs[0].RunID != "r1" {
		t.Fatalf("linked event = %+v, want run.checkpoint on r1", evs[0])
	}

	// Last returns the newest by event order.
	cp2, err := e.cps.Write(ctx, gates.NewCheckpoint{RunID: "r1", ModelID: "model-x"})
	if err != nil {
		t.Fatalf("second Write: %v", err)
	}
	last, ok, err := e.cps.Last(ctx, "r1")
	if err != nil || !ok {
		t.Fatalf("Last: %v %v", ok, err)
	}
	if last.ID != cp2.ID {
		t.Fatalf("Last = %d, want %d", last.ID, cp2.ID)
	}

	// Not checkpointable outside running/draining (Spec S02.3/S02.4).
	if _, err := e.runs.Transition(ctx, "r1", run.StateParked, run.TransitionOptions{}); err != nil {
		t.Fatalf("park: %v", err)
	}
	if _, err := e.cps.Write(ctx, gates.NewCheckpoint{RunID: "r1"}); !errors.Is(err, gates.ErrNotCheckpointable) {
		t.Fatalf("checkpoint on parked err = %v, want ErrNotCheckpointable", err)
	}
	if _, err := e.cps.Write(ctx, gates.NewCheckpoint{RunID: "nope"}); !errors.Is(err, gates.ErrRunNotFound) {
		t.Fatalf("checkpoint on missing run err = %v, want ErrRunNotFound", err)
	}
}

func TestEffectLifecycle(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun(t, "r1")

	// Key order normalizes: the stored payload is canonical.
	eff, err := e.jrnl.Propose(ctx, gates.Proposal{
		RunID: "r1", UserID: "u1", Class: gates.ClassB,
		Payload:           json.RawMessage(`{"to":"x@example.com", "amount": 5}`),
		ProviderWindowRef: "resend:24h",
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if eff.State != gates.EffectProposed || eff.PayloadHash == "" {
		t.Fatalf("proposed = %+v", eff)
	}
	if string(eff.Payload) != `{"amount":5,"to":"x@example.com"}` {
		t.Fatalf("payload not canonical: %s", eff.Payload)
	}

	eff, err = e.jrnl.Approve(ctx, eff.ID, "operator")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if eff.State != gates.EffectApproved || eff.ApprovedBy != "operator" || eff.ApprovedTS.IsZero() {
		t.Fatalf("approved = %+v", eff)
	}

	eff, err = e.jrnl.BeginExecute(ctx, eff.ID)
	if err != nil {
		t.Fatalf("BeginExecute: %v", err)
	}
	if eff.State != gates.EffectExecuting || eff.Attempts != 1 {
		t.Fatalf("executing = %+v", eff)
	}
	if eff.IdempotencyKey != eff.ID {
		t.Fatalf("idempotency_key = %q, want the effect UUID %q (Spec S02.7)", eff.IdempotencyKey, eff.ID)
	}

	eff, err = e.jrnl.Succeed(ctx, eff.ID, json.RawMessage(`{"provider_id":"msg_1"}`))
	if err != nil {
		t.Fatalf("Succeed: %v", err)
	}
	if eff.State != gates.EffectSucceeded {
		t.Fatalf("state = %s, want succeeded", eff.State)
	}

	// Lifecycle order is enforced.
	if _, err := e.jrnl.Approve(ctx, eff.ID, "operator"); !errors.Is(err, gates.ErrBadState) {
		t.Fatalf("re-approve err = %v, want ErrBadState", err)
	}
	if _, err := e.jrnl.BeginExecute(ctx, eff.ID); !errors.Is(err, gates.ErrBadState) {
		t.Fatalf("re-execute err = %v, want ErrBadState", err)
	}
}

func TestApprovalExpiryReturnsToProposed(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	eff, err := e.jrnl.Propose(ctx, gates.Proposal{UserID: "u1", Class: gates.ClassA, Payload: json.RawMessage(`{"op":1}`)})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if _, err := e.jrnl.Approve(ctx, eff.ID, "operator"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	e.clock.advance(7*24*time.Hour + time.Minute) // past ⚙ effects.approval_expiry (7 d)
	if _, err := e.jrnl.BeginExecute(ctx, eff.ID); !errors.Is(err, gates.ErrApprovalExpired) {
		t.Fatalf("err = %v, want ErrApprovalExpired", err)
	}
	got, err := e.jrnl.Get(ctx, eff.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != gates.EffectProposed || got.ApprovedBy != "" {
		t.Fatalf("after expiry = %+v, want proposed with approval cleared (Terraform saved-plan semantics)", got)
	}
}

func TestPayloadDriftReturnsToProposed(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	eff, err := e.jrnl.Propose(ctx, gates.Proposal{UserID: "u1", Class: gates.ClassC, Payload: json.RawMessage(`{"body":"hello"}`)})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if _, err := e.jrnl.Approve(ctx, eff.ID, "operator"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	// Tamper with the stored payload behind the journal's back.
	err = e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE effects SET payload = ? WHERE effect_id = ?`,
			`{"body":"tampered"}`, eff.ID)
		return err
	})
	if err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := e.jrnl.BeginExecute(ctx, eff.ID); !errors.Is(err, gates.ErrPayloadDrift) {
		t.Fatalf("err = %v, want ErrPayloadDrift", err)
	}
	got, err := e.jrnl.Get(ctx, eff.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != gates.EffectProposed || got.ApprovedBy != "" {
		t.Fatalf("after drift = %+v, want proposed with approval cleared", got)
	}
}

func TestReconcileInDoubtPerClass(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ids := map[gates.Class]string{}
	for _, class := range []gates.Class{gates.ClassA, gates.ClassB, gates.ClassC, gates.ClassD} {
		eff, err := e.jrnl.Propose(ctx, gates.Proposal{UserID: "u1", Class: class, Payload: json.RawMessage(`{"c":"` + string(class) + `"}`)})
		if err != nil {
			t.Fatalf("Propose %s: %v", class, err)
		}
		if _, err := e.jrnl.Approve(ctx, eff.ID, "operator"); err != nil {
			t.Fatalf("Approve %s: %v", class, err)
		}
		if _, err := e.jrnl.BeginExecute(ctx, eff.ID); err != nil {
			t.Fatalf("BeginExecute %s: %v", class, err)
		}
		ids[class] = eff.ID
	}
	res, err := e.jrnl.ReconcileInDoubt(ctx)
	if err != nil {
		t.Fatalf("ReconcileInDoubt: %v", err)
	}
	if len(res) != 4 {
		t.Fatalf("resolutions = %d, want 4", len(res))
	}
	for _, class := range []gates.Class{gates.ClassA, gates.ClassB, gates.ClassC} {
		got, err := e.jrnl.Get(ctx, ids[class])
		if err != nil {
			t.Fatalf("Get %s: %v", class, err)
		}
		if got.State != gates.EffectApproved {
			t.Fatalf("class %s after reconcile = %s, want approved (replay path)", class, got.State)
		}
		if got.IdempotencyKey != got.ID || got.Attempts != 1 {
			t.Fatalf("class %s lost replay identity: key %q attempts %d", class, got.IdempotencyKey, got.Attempts)
		}
	}
	gotD, err := e.jrnl.Get(ctx, ids[gates.ClassD])
	if err != nil {
		t.Fatalf("Get D: %v", err)
	}
	if gotD.State != gates.EffectUnknown {
		t.Fatalf("class D after reconcile = %s, want unknown (P-T07-3)", gotD.State)
	}
	var card struct{ Card, Note string }
	if err := json.Unmarshal(gotD.Result, &card); err != nil || card.Card == "" {
		t.Fatalf("class D card = %s (%v), want a decision card", gotD.Result, err)
	}
	// A second pass finds nothing in doubt.
	res, err = e.jrnl.ReconcileInDoubt(ctx)
	if err != nil || len(res) != 0 {
		t.Fatalf("second reconcile = %v %v, want empty", res, err)
	}
}
