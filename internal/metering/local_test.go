package metering

import (
	"context"
	"testing"
	"time"
)

// local_test.go — the S12.1 free-tier metering behaviors (brief R18/R19; §8
// readings 4+5). The local marker + the $0 pricing + the flat-lane guard are
// pinned SIDE BY SIDE so the two are never conflated.

// TestLocalZeroDollarAndFlatGuardSideBySide: a local-marked row prices a TRUE
// $0 (zero-allowance), while a PAID flat-rate lane's all-zero row still trips
// the S10.3 never-price-from-$0 guard (UNPRICED) — both bind at once (R19).
func TestLocalZeroDollarAndFlatGuardSideBySide(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	// Paid flat-rate row (no local marker) — the S10.3 guard target.
	e.checkpoint(t, "r1", "paid-model", `{"input_tokens":100,"output_tokens":50}`)
	// Local row (marker) — the free tier.
	e.checkpoint(t, "r1", "Qwen3.5-4B", `{"input_tokens":80,"output_tokens":6,"local":{"lane":"local","duty":"intake-triage","model":"Qwen3.5-4B","model_sha256":"abc123","engine_build":"b10085"}}`)

	// A price table with an ALL-ZERO row for the paid flat-rate lane (the
	// coding-plan $0 shape the guard exists for).
	tbl := NewEffectiveDatedTable("test")
	tbl.Add(PriceRow{Model: "paid-model", Lane: "anthropic", Prices: UnitPrices{}, EffectiveFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
	led := NewLedger(e.db, tbl, NoMeteredExceptions(), e.reg)
	rc, err := led.RunConsumption(ctx, "r1")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}

	var paid, local *LineItem
	for i := range rc.Items {
		switch rc.Items[i].Lane {
		case "anthropic":
			paid = &rc.Items[i]
		case LaneLocal:
			local = &rc.Items[i]
		}
	}
	if paid == nil || local == nil {
		t.Fatalf("expected an anthropic line AND a local line; got %+v", rc.Items)
	}
	// The paid flat-rate lane trips the S10.3 guard → UNPRICED, never $0.
	if !paid.Unpriced() {
		t.Error("paid flat-rate all-zero row did NOT trip the S10.3 guard (must stay UNPRICED)")
	}
	// The local lane prices a TRUE $0, zero-allowance, never UNPRICED.
	if local.Unpriced() {
		t.Error("local row is UNPRICED — must be TRUE $0 zero-allowance (R19)")
	}
	if local.PricedUSD != 0 || local.PricedCalls == 0 || !local.ZeroAllowance() {
		t.Errorf("local line = $%v priced=%d zeroAllowance=%v, want $0/≥1/true", local.PricedUSD, local.PricedCalls, local.ZeroAllowance())
	}
	if local.Model != "Qwen3.5-4B" {
		t.Errorf("local line model = %q, want the resolved local model", local.Model)
	}
}

// TestPressureGaugeHonorsLocalLaneOverride: a local-marked row counts under
// lane "local" (the D5 floor), not the consuming run's paid lane (R19).
func TestPressureGaugeHonorsLocalLaneOverride(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	e.checkpoint(t, "r1", "paid-model", `{"input_tokens":100,"output_tokens":50}`)
	e.checkpoint(t, "r1", "Qwen3.5-4B", `{"input_tokens":80,"output_tokens":6,"local":{"lane":"local","duty":"utility","model":"Qwen3.5-4B","model_sha256":"x","engine_build":"b10085"}}`)

	g := NewPressureGauge(e.db, e.reg)
	localGauge, err := g.Read(ctx, "alice", "local", UndeclaredBudget())
	if err != nil {
		t.Fatalf("Read local: %v", err)
	}
	if localGauge.WeightedConsumption == 0 {
		t.Error("local lane pressure is 0 — the marker override did not surface the local row on the D5 floor (R19)")
	}
	anthropicGauge, err := g.Read(ctx, "alice", "anthropic", UndeclaredBudget())
	if err != nil {
		t.Fatalf("Read anthropic: %v", err)
	}
	// The local row must NOT inflate the paid lane's pressure.
	if anthropicGauge.WeightedConsumption != 150 { // 100 input + 50 output, no cache
		t.Errorf("anthropic weighted consumption = %v, want 150 (local row must not count here)", anthropicGauge.WeightedConsumption)
	}
}
