package local_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

type dutyEnv struct {
	db   *storage.DB
	log  *eventlog.Log
	reg  *settings.Registry
	runs *run.Store
	cps  *gates.Checkpoints
	fake *local.FakeServer
	duty *local.Duty
}

func newDutyEnv(t *testing.T) *dutyEnv {
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
	cps := gates.NewCheckpoints(db, log)
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)
	duty := local.NewDuty(local.DutyDeps{
		Registry:    local.NewRegistry(reg),
		Client:      local.NewClient(fake.URL),
		Checkpoints: cps,
		Events:      log,
	})
	return &dutyEnv{db: db, log: log, reg: reg, runs: run.NewStore(db, log), cps: cps, fake: fake, duty: duty}
}

func (e *dutyEnv) runningRun(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: id, UserID: "alice", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := e.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition %s→%s: %v", id, st, err)
		}
	}
}

// TestDutyCallShapesAndMetersZeroDollar exercises R16/R18/R19 end-to-end: the
// client shapes the request (temp-0, engine json_schema, logprobs); the call
// lands ONE D7 checkpoint row on the consuming run with the local marker; and
// the metering fold prices it TRUE $0 (zero-allowance), never UNPRICED.
func TestDutyCallShapesAndMetersZeroDollar(t *testing.T) {
	ctx := context.Background()
	e := newDutyEnv(t)
	e.runningRun(t, "task1.intake")

	// intake-triage → fast seat → Qwen3.5-4B.
	e.fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{
		Content:      `{"family":"software","stakes":"low","size":"small","data_bearing":false,"abstain":false}`,
		InputTokens:  100,
		OutputTokens: 5,
		Logprobs:     []local.TokenLogprob{{Token: "software", Logprob: -0.1}},
	})

	schema := local.TriageSchema([]string{"software"}, []string{"low"}, []string{"small"})
	res, err := e.duty.Call(ctx, "task1.intake", local.DutyRequest{
		Alias: local.AliasIntakeTriage, System: "classify", User: "a request",
		Schema: schema, Name: "intake-triage", MaxTokens: 512, Classification: true,
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res.Model != "Qwen3.5-4B" {
		t.Errorf("model = %q, want Qwen3.5-4B", res.Model)
	}
	if len(res.Logprobs) == 0 {
		t.Error("no logprobs exposed (the only v0 logprob lane, R16)")
	}

	// Request shaping (S12.4 cross-cutting rules).
	lr := e.fake.LastRequest()
	if lr.Temperature != 0 {
		t.Errorf("temperature = %v, want 0 (greedy, S12.4)", lr.Temperature)
	}
	if !lr.HasJSONSchema || lr.SchemaName != "intake-triage" {
		t.Error("json_schema not enforced at the engine (S12.4)")
	}
	if !lr.Logprobs {
		t.Error("logprobs not requested (R16)")
	}
	if lr.MaxTokens != 512 {
		t.Errorf("max_tokens = %d, want the per-duty length cap 512", lr.MaxTokens)
	}

	// R18/R19: exactly one $0 zero-allowance local line on the consuming run.
	led := metering.NewLedger(e.db, nil, metering.NoMeteredExceptions(), e.reg)
	rc, err := led.RunConsumption(ctx, "task1.intake")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}
	var localLine *metering.LineItem
	for i := range rc.Items {
		if rc.Items[i].Lane == metering.LaneLocal {
			localLine = &rc.Items[i]
		}
	}
	if localLine == nil {
		t.Fatalf("no local-lane line in the receipt (the marker override did not fire, §8 reading 4); items=%+v", rc.Items)
	}
	if localLine.Model != "Qwen3.5-4B" {
		t.Errorf("local line model = %q, want Qwen3.5-4B", localLine.Model)
	}
	if localLine.Unpriced() {
		t.Error("local line is UNPRICED — must be TRUE $0 zero-allowance (R19)")
	}
	if localLine.PricedUSD != 0 || localLine.PricedCalls == 0 {
		t.Errorf("local line = $%v over %d priced calls, want $0 over ≥1 (R19)", localLine.PricedUSD, localLine.PricedCalls)
	}
	if !localLine.ZeroAllowance() {
		t.Error("local line not labeled zero-allowance (R19)")
	}
}

// TestDutyAbsentStackDegrades: a nil client (unconfigured stack) makes every
// call ErrStackAbsent so the seams degrade per the S12.4/S06 rows (R17).
func TestDutyAbsentStackDegrades(t *testing.T) {
	var d *local.Duty // nil = unconfigured
	if _, err := d.Call(context.Background(), "r.intake", local.DutyRequest{Alias: local.AliasUtility}); !errors.Is(err, local.ErrStackAbsent) {
		t.Errorf("nil duty: got %v, want ErrStackAbsent", err)
	}
}

// TestRunlessCallIsSurfacedDefect: a call on a non-checkpointable run fails
// loudly and records a defect event — never a silently unmetered call (R18).
func TestRunlessCallIsSurfacedDefect(t *testing.T) {
	ctx := context.Background()
	e := newDutyEnv(t)
	// No run created → the checkpoint write fails.
	e.fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{Content: `{"abstain":true}`, InputTokens: 5, OutputTokens: 1})
	_, err := e.duty.Call(ctx, "ghost.intake", local.DutyRequest{
		Alias: local.AliasIntakeTriage, User: "x",
		Schema: local.TriageSchema([]string{"software"}, []string{"low"}, []string{"small"}), Classification: true,
	})
	if err == nil {
		t.Fatal("run-less call should fail loudly, never be silently unmetered (R18)")
	}
	// The defect event was recorded (platform-scope).
	if !hasEvent(t, e.db, local.EventLocalUnmeteredDefect) {
		t.Error("no local.unmetered_defect event recorded (R18)")
	}
}

// TestEagerUnload exercises R12: admission-stop + unload, idempotent, event-
// recorded; refuse new calls while engaged; symmetric resume.
func TestEagerUnload(t *testing.T) {
	ctx := context.Background()
	e := newDutyEnv(t)
	e.runningRun(t, "task2.intake")
	unload := local.NewEagerUnload(e.duty, e.log, nil)

	if err := unload.Engage(ctx, "operator"); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	if !unload.Stopped() {
		t.Error("admissions not stopped after Engage (R12)")
	}
	if u := e.fake.Unloaded(); len(u) == 0 || u[0] != "*" {
		t.Errorf("unload endpoint not called (got %v)", u)
	}
	// New calls refuse while engaged.
	if _, err := e.duty.Call(ctx, "task2.intake", local.DutyRequest{Alias: local.AliasUtility, User: "x"}); !errors.Is(err, local.ErrAdmissionsStopped) {
		t.Errorf("call while engaged: got %v, want ErrAdmissionsStopped", err)
	}
	// Idempotent.
	if err := unload.Engage(ctx, "operator"); err != nil {
		t.Fatalf("Engage (idempotent): %v", err)
	}
	// Resume is symmetric.
	if err := unload.Resume(ctx, "operator"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if unload.Stopped() {
		t.Error("admissions still stopped after Resume (R12)")
	}
	if !hasEvent(t, e.db, local.EventLocalAdmissionsStopped) || !hasEvent(t, e.db, local.EventLocalAdmissionsResumed) {
		t.Error("eager-unload events not recorded (R12)")
	}
}

func hasEvent(t *testing.T, db *storage.DB, typ string) bool {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE type = ?`, typ).Scan(&n); err != nil {
		t.Fatalf("count events %q: %v", typ, err)
	}
	return n > 0
}
