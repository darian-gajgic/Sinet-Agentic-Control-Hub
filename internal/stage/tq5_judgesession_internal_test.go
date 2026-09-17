package stage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// tq5_judgesession_internal_test.go — P3-TQ-5 acceptance checklist 9 (Spec
// S07.5 judge routing; S07.11 "judge model" on every verdict row). Proved at
// the DISPATCH, not at the resolver (CONVENTIONS §63): the model the judge
// session actually sends must be the seat Meta() names, so the verdict row's
// judge_model and the engine that produced the verdict cannot drift apart.
// The duty map here gives planning and judging different models on purpose —
// on the shipped map they are equal by data, and equality hides the bug.
//
// $0: the adapter records the request and starts no engine.

type tq5ModelRecorder struct {
	mu    sync.Mutex
	model string
}

func (a *tq5ModelRecorder) Substrate() string { return adapters.SubstrateClaudeCLI }

func (a *tq5ModelRecorder) Start(_ context.Context, req adapters.StartRequest) (adapters.Session, error) {
	a.mu.Lock()
	a.model = req.Model
	a.mu.Unlock()
	return nil, errors.New("recording adapter starts no engine ($0)")
}

func (a *tq5ModelRecorder) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("recording adapter starts no engine ($0)")
}

func (a *tq5ModelRecorder) seen() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

func TestTQ5JudgeSessionRunsOnTheJudgeSeat(t *testing.T) {
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
	rec := &tq5ModelRecorder{}
	root := t.TempDir()
	sk, err := New(Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
		Adapters: map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: rec},
		DutyMap: worker.DutyMap{
			worker.DutyExecution: {Model: "execution-seat-model", Lane: adapters.LaneAnthropic, WindowTokens: worker.DefaultWindowTokens},
			worker.DutyPlanning:  {Model: "planning-seat-model", Lane: adapters.LaneAnthropic, WindowTokens: worker.DefaultWindowTokens},
			worker.DutyJudge:     {Model: "judge-seat-model", Lane: adapters.LaneAnthropic, WindowTokens: worker.DefaultWindowTokens},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}

	const taskID = "t-tq5-judge"
	runID := taskID + ".verify"
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, 'u1', 'judge seat harness', ?)`,
			taskID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := runs.Create(ctx, run.NewRun{
		ID: runID, UserID: "u1", TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, runID, st, run.TransitionOptions{
			Reason: "test admission", Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("admit: %v", err)
		}
	}

	// The executor was K3 on a commissioned Kimi lane: a cross-family judge,
	// so the flag on the row is false and the judge seat is unambiguous.
	j := newEngineJudge(sk, "k3")
	meta := j.Meta()
	if meta.Model != "judge-seat-model" {
		t.Fatalf("Meta().Model = %q, want the judge seat", meta.Model)
	}
	if meta.SelfFamily {
		t.Errorf("self_family is true although K3 executed and %q judged", meta.Model)
	}

	// The session itself. The recorder refuses to start an engine, so the
	// call errors — what is under test is the request that reached it.
	if _, err := j.Compliance(ctx, verify.JudgeInput{
		Brief:     ledger.Brief{RunID: runID, TaskID: taskID, Stage: "verify", Clean: true},
		BriefText: "the input slice",
	}); err == nil {
		t.Fatal("Compliance returned no error although the recording adapter starts no engine")
	}
	got := rec.seen()
	if got == "" {
		t.Fatal("no session reached the adapter — the dispatch assertion below would be vacuous")
	}
	if got != meta.Model {
		t.Errorf("the judge session dispatched %q while the verdict row says %q — the row and the engine that "+
			"produced it must name the same seat (Spec S07.11)", got, meta.Model)
	}
	if got == sk.seat(worker.DutyPlanning).Model {
		t.Errorf("the judge session ran on the PLANNING seat %q", got)
	}
}
