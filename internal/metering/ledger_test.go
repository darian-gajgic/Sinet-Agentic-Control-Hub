package metering

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

type env struct {
	db   *storage.DB
	log  *eventlog.Log
	reg  *settings.Registry
	runs *run.Store
	cps  *gates.Checkpoints
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
	return &env{db: db, log: log, reg: reg, runs: run.NewStore(db, log), cps: gates.NewCheckpoints(db, log)}
}

// runningRun creates a run and walks it new→queued→claimed→running so
// checkpoints are writable (Spec S02.4: checkpoints only in running/draining).
func (e *env) runningRun(t *testing.T, id, user, lane, substrate string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: id, UserID: user, Lane: lane, Substrate: substrate}); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := e.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition %s→%s: %v", id, st, err)
		}
	}
}

func (e *env) checkpoint(t *testing.T, runID, model, usageJSON string) {
	t.Helper()
	if _, err := e.cps.Write(context.Background(), gates.NewCheckpoint{
		RunID: runID, ModelID: model, Usage: json.RawMessage(usageJSON),
		SessionSubstrate: "claude-cli", SessionID: "sid-" + runID,
	}); err != nil {
		t.Fatalf("checkpoint %s: %v", runID, err)
	}
}

func TestLedgerAggregatesCheckpointsUnpricedAtV0(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	e.checkpoint(t, "r1", "claude-sonnet-4-5", `{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":8000}`)
	e.checkpoint(t, "r1", "claude-sonnet-4-5", `{"input_tokens":20,"output_tokens":10}`)

	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg) // nil price table → empty
	rc, err := led.RunConsumption(ctx, "r1")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}
	if rc.UserID != "alice" {
		t.Errorf("billed to %q, want alice (3.4 bill-the-requester)", rc.UserID)
	}
	if len(rc.Items) != 1 {
		t.Fatalf("items = %d, want 1 (one model×purpose)", len(rc.Items))
	}
	li := rc.Items[0]
	if li.Model != "claude-sonnet-4-5" || li.Purpose != PurposeExecution || li.Lane != "anthropic" {
		t.Errorf("line = %+v", li)
	}
	if li.Calls != 2 || li.PromptTokens != (8000+100)+20 || li.BilledOutputTokens != 60 {
		t.Errorf("aggregation wrong: %+v", li)
	}
	if li.Currency != CurrencyAPIEquiv {
		t.Errorf("currency = %s, want api-equivalent (flat lane, empty exceptions)", li.Currency)
	}
	// Empty price table → every call UNPRICED, never a silent $0 (S10.1).
	if li.UnpricedCalls != 2 || !li.Unpriced() || rc.TotalPricedUSD != 0 {
		t.Errorf("expected UNPRICED at v0: line=%+v totalPriced=%v", li, rc.TotalPricedUSD)
	}
}

func TestLedgerPricesWhenTableLoaded(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	e.checkpoint(t, "r1", "m", `{"input_tokens":1000000,"output_tokens":1000000}`)

	pt := NewEffectiveDatedTable("v1")
	pt.Add(PriceRow{Model: "m", Lane: "anthropic", EffectiveFrom: day(2000, 1, 1),
		Prices: UnitPrices{InputUSD: 3e-6, OutputUSD: 15e-6}})
	led := NewLedger(e.db, pt, NoMeteredExceptions(), e.reg)
	rc, err := led.RunConsumption(ctx, "r1")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}
	if rc.TotalUnpricedCalls != 0 || rc.Items[0].PricedUSD != 3.0+15.0 {
		t.Errorf("priced = %+v, want 18.0", rc.Items[0])
	}
}

func TestDivergenceAlarmReadsSettingButNotComparableUnpriced(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	e.checkpoint(t, "r1", "m", `{"input_tokens":100,"output_tokens":50}`)
	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg)
	rc, _ := led.RunConsumption(ctx, "r1")

	al, err := led.CheckDivergence(ctx, rc, 0.02, true)
	if err != nil {
		t.Fatalf("CheckDivergence: %v", err)
	}
	if al.ThresholdFrac != 0.20 { // ⚙ meter.value_divergence_alarm default 20%
		t.Errorf("threshold = %v, want 0.20 (⚙ read)", al.ThresholdFrac)
	}
	if al.Comparable || al.Fired {
		t.Errorf("must not be comparable/fired while UNPRICED at v0: %+v", al)
	}
}

func TestPressureGaugeWeightsCacheReadAndNeedsBudget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	// 8000 cache-read weighted 0.1× = 800; plus 100 input + 50 output = 950.
	e.checkpoint(t, "r1", "m", `{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":8000}`)

	g := NewPressureGauge(e.db, e.reg)

	// Undeclared budget: consumption computed, but not applicable (nothing to
	// gate, S10.4) — honest at v0.
	un, err := g.Read(ctx, "alice", "anthropic", UndeclaredBudget())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if un.Applicable {
		t.Error("undeclared budget must be not-applicable (S10.4)")
	}
	if un.WeightedConsumption != 950 {
		t.Errorf("weighted consumption = %v, want 950 (100+50+8000×0.1)", un.WeightedConsumption)
	}
	if !un.Assumed || un.CacheReadWeight != 0.1 {
		t.Errorf("expected assumed label + ⚙ 0.1 weight, got %+v", un)
	}

	// Declared budget binds: pressure = 950/1900 = 0.5.
	dec, err := g.Read(ctx, "alice", "anthropic", Budget{PeriodTokens: 1900, Declared: true})
	if err != nil {
		t.Fatalf("Read declared: %v", err)
	}
	if !dec.Applicable || dec.Pressure != 0.5 {
		t.Errorf("pressure = %+v, want 0.5 applicable", dec)
	}
}

func TestReceiptMaterializeIsIdempotentWithParkHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	e.checkpoint(t, "r1", "m", `{"input_tokens":100,"output_tokens":50}`)
	// A park episode: running→parked→running (a limit-event/gate park).
	if _, err := e.runs.Transition(ctx, "r1", run.StateParked, run.TransitionOptions{Reason: "blocked_quota (S10.5)", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("park: %v", err)
	}
	if _, err := e.runs.Transition(ctx, "r1", run.StateRunning, run.TransitionOptions{Reason: "resume on provider signal", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if _, err := e.runs.Transition(ctx, "r1", run.StateCompleted, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg)
	rc := NewReceipts(e.db, led, NoMeteredExceptions())

	r1, err := rc.Materialize(ctx, "r1")
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if r1.UserID != "alice" || len(r1.Items) != 1 {
		t.Errorf("receipt = %+v", r1)
	}
	// Done-directly line present verbatim, UNPRICED at v0 (S10.10, §13).
	if r1.DirectUse.Label != DirectUseLabel || !r1.DirectUse.Unpriced {
		t.Errorf("direct-use line = %+v (want labelled + UNPRICED)", r1.DirectUse)
	}
	if r1.DirectUse.FormulaRef != DirectUseFormulaRef {
		t.Errorf("direct-use must cite §13, got %q", r1.DirectUse.FormulaRef)
	}
	// Park history reconstructed from the event log.
	if len(r1.ParkHistory) != 1 || r1.ParkHistory[0].ParkReason == "" || r1.ParkHistory[0].Ongoing {
		t.Errorf("park history = %+v, want one closed episode", r1.ParkHistory)
	}

	// Idempotent: a second materialize returns the same receipt, no duplicate.
	if _, err := rc.Materialize(ctx, "r1"); err != nil {
		t.Fatalf("Materialize (2nd): %v", err)
	}
	var count int
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM receipts WHERE run_id = 'r1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("receipt rows = %d, want 1 (idempotent materialization)", count)
	}
}
