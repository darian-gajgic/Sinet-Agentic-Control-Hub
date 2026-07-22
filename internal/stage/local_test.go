package stage_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// local_test.go — the S12.1 class-(b) duty-seam adapters (brief R20/R21): the
// intake Classifier/Utility/SpotCheck and the worker TieBreaker bridge into
// internal/local against a fake /v1, mapping results to the seam types and
// degrading per the S12.4/S06 rows.

func localSeamEnv(t *testing.T) (*local.Duty, *local.FakeServer, *storage.DB) {
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
	// The consuming intake run must be running (the seams checkpoint on it).
	if _, err := runs.Create(ctx, run.NewRun{ID: "t1.intake", UserID: "alice", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, "t1.intake", st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)
	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(fake.URL),
		Checkpoints: gates.NewCheckpoints(db, log), Events: log,
	})
	return duty, fake, db
}

func TestLocalClassifierMapsTriage(t *testing.T) {
	ctx := context.Background()
	duty, fake, _ := localSeamEnv(t)
	fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{
		Content:     `{"family":"software","stakes":"high","size":"large","data_bearing":true,"abstain":false}`,
		InputTokens: 50, OutputTokens: 4,
	})
	c := stage.NewLocalClassifier(duty)
	p, err := c.Classify(ctx, intake.Request{TaskID: "t1", Title: "fix", Text: "fix the parser"}, nil)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if p.Family != intake.FamilySoftware || p.Tier != intake.TierHigh {
		t.Errorf("proposal = family %q tier %q, want software/high", p.Family, p.Tier)
	}
	if len(p.DataHits) != 1 || p.DataHits[0].Source != "classifier" {
		t.Errorf("data-bearing add-only not mapped: %+v", p.DataHits)
	}
	if p.Est.Known {
		t.Error("classifier must NOT price (Est.Known should stay false — S10 prices)")
	}
}

func TestLocalClassifierAbstainFailsClosed(t *testing.T) {
	ctx := context.Background()
	duty, fake, _ := localSeamEnv(t)
	fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{Content: `{"abstain":true}`, InputTokens: 5, OutputTokens: 1})
	c := stage.NewLocalClassifier(duty)
	if _, err := c.Classify(ctx, intake.Request{TaskID: "t1", Text: "??"}, nil); err == nil {
		t.Error("abstain should error so the pipeline fails closed to high (S06.2)")
	}
}

func TestLocalTieBreakerPicksAndDegrades(t *testing.T) {
	ctx := context.Background()
	duty, fake, _ := localSeamEnv(t)
	cands := []worker.Candidate{{TemplateID: "w-1", Name: "one"}, {TemplateID: "w-2", Name: "two"}}
	tb := stage.NewLocalTieBreaker(duty)

	// utility → workhorse → Qwen3.5-9B; pick w-2.
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{Content: `{"pick":"w-2","reason":"better fit"}`, InputTokens: 30, OutputTokens: 5})
	pick, reason, err := tb.Break(ctx, worker.RouteQuery{TaskID: "t1", TaskText: "x"}, cands)
	if err != nil {
		t.Fatalf("Break: %v", err)
	}
	if pick != 1 {
		t.Errorf("pick = %d, want 1 (w-2)", pick)
	}
	if reason == "" {
		t.Error("tie-break reason empty")
	}

	// Abstain → deterministic order (pick 0), no error (R21).
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{Content: `{"pick":"abstain","reason":""}`, InputTokens: 30, OutputTokens: 2})
	pick2, reason2, err2 := tb.Break(ctx, worker.RouteQuery{TaskID: "t1", TaskText: "x"}, cands)
	if err2 != nil {
		t.Fatalf("Break (abstain): %v", err2)
	}
	if pick2 != 0 {
		t.Errorf("abstain pick = %d, want 0 (deterministic order)", pick2)
	}
	if reason2 == "" {
		t.Error("abstain reason not recorded")
	}
}

func TestLocalUtilityAndSpotCheck(t *testing.T) {
	ctx := context.Background()
	duty, fake, _ := localSeamEnv(t)
	pair := intake.Pair{Spec: intake.Spec{TaskID: "t1", Restatement: "do the thing", ACs: []intake.AC{{N: 1, Plain: "works"}}}}

	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{Content: `{"what":"w","wrong":"x","recommend":"y"}`, InputTokens: 40, OutputTokens: 8})
	h, err := stage.NewLocalUtility(duty).Help(ctx, pair)
	if err != nil {
		t.Fatalf("Help: %v", err)
	}
	if h.What != "w" || h.Wrong != "x" || h.Recommend != "y" {
		t.Errorf("help = %+v, want {w,x,y}", h)
	}

	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{Content: `{"uncovered":["AC-1"],"abstain":false}`, InputTokens: 30, OutputTokens: 4})
	uncov, err := stage.NewLocalSpotCheck(duty).Check(ctx, pair)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(uncov) != 1 || uncov[0] != "AC-1" {
		t.Errorf("uncovered = %v, want [AC-1]", uncov)
	}
}
