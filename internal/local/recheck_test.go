package local

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// recheckEnv wires a real duty (fake /v1), calibration store, and a running run.
type recheckEnv struct {
	duty  *Duty
	store *CalStore
	fake  *FakeServer
}

func newRecheckEnv(t *testing.T) *recheckEnv {
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
	if err := db.ReapplySettings(ctx); err != nil {
		t.Fatalf("Reapply: %v", err)
	}
	runs := run.NewStore(db, log)
	if _, err := runs.Create(ctx, run.NewRun{ID: "task.intake", UserID: "alice", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, "task.intake", st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}
	fake := NewFakeServer()
	t.Cleanup(fake.Close)
	duty := NewDuty(DutyDeps{Registry: NewRegistry(reg), Client: NewClient(fake.URL), Checkpoints: gates.NewCheckpoints(db, log), Events: log})
	return &recheckEnv{duty: duty, store: NewCalStore(db), fake: fake}
}

// lowMarginTriageResult builds a fast-tier DutyResult on the fast seat whose
// family/stakes label margin is `margin`.
func lowMarginTriageResult(margin float64) DutyResult {
	fast, _ := SeatByKey("fast")
	lps := []TokenLogprob{
		TokenLogprobFixture(`{"reason":"ok"`, 3.0),
		TokenLogprobFixture(`,"family":"`, 3.0),
		TokenLogprobFixture(`software`, margin),
		TokenLogprobFixture(`","stakes":"`, 3.0),
		TokenLogprobFixture(`low`, margin+1.0),
		TokenLogprobFixture(`","size":"small","abstain":false}`, 3.0),
	}
	return DutyResult{Alias: AliasIntakeTriage, Model: fast.Model, Seat: fast, Content: `{"family":"software"}`, Logprobs: lps}
}

func TestRecheckerFiresBelowThreshold(t *testing.T) {
	ctx := context.Background()
	e := newRecheckEnv(t)
	fast, _ := SeatByKey("fast")
	// Calibration with a high threshold (5.0) so a margin-1.0 answer re-checks.
	key := CalibrationKey{Duty: AliasIntakeTriage, ModelHash: fast.ModelHash(), EngineBuild: LlamaCppPin}
	if err := e.store.SaveCalibration(ctx, Calibration{Key: key, Threshold: 5.0, AcceptanceBar: 0.2, LabeledN: 40, MeetsBar: true}); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}
	// The workhorse (9B) returns a distinct confident answer.
	e.fake.SetModelResponse("Qwen3.5-9B", FakeResponse{Content: `{"family":"software","stakes":"high","size":"medium","abstain":false}`, InputTokens: 50, OutputTokens: 6})

	rc := NewReChecker(e.duty, e.store)
	first := lowMarginTriageResult(1.0)
	in := DutyRequest{Alias: AliasIntakeTriage, System: "classify", User: "x", Schema: TriageSchema([]string{"software"}, []string{"low", "high"}, []string{"small", "medium"}), Name: "intake-triage", MaxTokens: 512, Classification: true}
	out, err := rc.MarginRecheck(ctx, "task.intake", AliasIntakeTriage, in, first, []string{"family", "stakes"})
	if err != nil {
		t.Fatalf("MarginRecheck: %v", err)
	}
	if !out.Rechecked {
		t.Fatalf("expected a re-check below the threshold; got %+v", out)
	}
	if out.Result.Model != "Qwen3.5-9B" {
		t.Errorf("re-check should run on the workhorse; model = %q", out.Result.Model)
	}
}

func TestRecheckerAcceptsAboveThreshold(t *testing.T) {
	ctx := context.Background()
	e := newRecheckEnv(t)
	fast, _ := SeatByKey("fast")
	key := CalibrationKey{Duty: AliasIntakeTriage, ModelHash: fast.ModelHash(), EngineBuild: LlamaCppPin}
	// Low threshold (0.5): a margin-1.0 answer is accepted, no re-check.
	if err := e.store.SaveCalibration(ctx, Calibration{Key: key, Threshold: 0.5, AcceptanceBar: 0.2, LabeledN: 40, MeetsBar: true}); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}
	rc := NewReChecker(e.duty, e.store)
	out, err := rc.MarginRecheck(ctx, "task.intake", AliasIntakeTriage, DutyRequest{Alias: AliasIntakeTriage}, lowMarginTriageResult(1.0), []string{"family", "stakes"})
	if err != nil {
		t.Fatalf("MarginRecheck: %v", err)
	}
	if out.Rechecked {
		t.Errorf("margin above the threshold should accept the fast answer; got %+v", out)
	}
	if out.Result.Model != fast.Model {
		t.Errorf("the fast answer should stand; model = %q", out.Result.Model)
	}
}

func TestRecheckerUncalibratedNoGate(t *testing.T) {
	ctx := context.Background()
	e := newRecheckEnv(t)
	// No calibration saved ⇒ no re-check gate (the fast answer stands, R3/R4).
	rc := NewReChecker(e.duty, e.store)
	out, err := rc.MarginRecheck(ctx, "task.intake", AliasIntakeTriage, DutyRequest{Alias: AliasIntakeTriage}, lowMarginTriageResult(0.1), []string{"family", "stakes"})
	if err != nil {
		t.Fatalf("MarginRecheck: %v", err)
	}
	if out.Rechecked {
		t.Error("uncalibrated ⇒ no re-check gate (honest-uncalibrated posture)")
	}
}

func TestRecheckerNilSafe(t *testing.T) {
	var rc *ReChecker
	out, err := rc.MarginRecheck(context.Background(), "r", AliasIntakeTriage, DutyRequest{}, lowMarginTriageResult(0.1), []string{"family"})
	if err != nil || out.Rechecked {
		t.Errorf("a nil ReChecker must be a no-op, got %+v %v", out, err)
	}
}
