package metering

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// spend_test.go — the S14.4 spend read (brief R6): the daily per-person priced
// total + trailing median. The v0 posture is DORMANT — an empty price table
// prices every row UNPRICED, so the read reports AnyUnpriced with zero dollars;
// it activates when a price table is seeded.

func TestSpendWindowDormantOnEmptyTable(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun(t, "job.execute", "alice", "anthropic", "claude-cli")
	// A real paid-lane checkpoint with token usage — UNPRICED under the empty
	// v0 table (§11).
	e.checkpoint(t, "job.execute", "claude-x", `{"input_tokens":1000,"output_tokens":500}`)

	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg) // nil ⇒ empty table
	win, err := led.SpendWindow(ctx, "alice", time.Now(), 14)
	if err != nil {
		t.Fatalf("SpendWindow: %v", err)
	}
	if !win.AnyUnpriced {
		t.Errorf("empty price table must mark AnyUnpriced (the dormancy signal, R6)")
	}
	if win.TodayUSD != 0 {
		t.Errorf("today's priced total = $%v, want $0 (no dollars at v0)", win.TodayUSD)
	}
	if win.UserID != "alice" {
		t.Errorf("UserID = %q, want alice", win.UserID)
	}
}

// TestSpendWindowLocalRowsAreZeroNotUnpriced: a local-marker checkpoint prices a
// TRUE $0 (not UNPRICED), so a person doing only local work does not trip the
// dormancy signal for those rows (the free tier is priced, §26).
func TestSpendWindowLocalRowsAreZeroNotUnpriced(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun(t, "job.intake", "bob", "anthropic", "claude-cli")
	// A local duty D7 row carries the local marker (the §26 wire contract).
	e.checkpoint(t, "job.intake", "Qwen3.5-4B",
		`{"input_tokens":100,"output_tokens":5,"local":{"lane":"local","duty":"intake-triage","model":"Qwen3.5-4B","model_sha256":"abc","engine_build":"b10085"}}`)

	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg)
	win, err := led.SpendWindow(ctx, "bob", time.Now(), 14)
	if err != nil {
		t.Fatalf("SpendWindow: %v", err)
	}
	if win.AnyUnpriced {
		t.Errorf("a local-only person must not be marked AnyUnpriced (local rows price a TRUE $0, §26)")
	}
	if win.TodayUSD != 0 {
		t.Errorf("local today total = $%v, want $0 (zero-allowance)", win.TodayUSD)
	}
}

// datedCheckpoint inserts a checkpoint row dated at ts (bypassing cps.Write,
// which hardcodes created_ts=now and gates on running state) so the spend read's
// day-bucketing can be exercised across real prior days. It reuses the run's
// first event_seq (the checkpoints.event_seq FK; the column is not unique).
func (e *env) datedCheckpoint(t *testing.T, runID, user, model, usageJSON string, ts time.Time) {
	t.Helper()
	ctx := context.Background()
	var seq int64
	if err := e.db.QueryRowContext(ctx,
		`SELECT event_seq FROM run_events WHERE run_id = ? ORDER BY event_seq LIMIT 1`, runID).Scan(&seq); err != nil {
		t.Fatalf("datedCheckpoint seq %s: %v", runID, err)
	}
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO checkpoints (run_id, user_id, event_seq, usage_json, session_substrate, session_id, model_id, created_ts)
			 VALUES (?, ?, ?, ?, 'claude-cli', ?, ?, ?)`,
			runID, user, seq, usageJSON, "sid-"+runID, model, ts.UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("datedCheckpoint %s: %v", runID, err)
	}
}

// TestSpendWindowPricedDayBucketing (drain D11): with a price table loaded, the
// read buckets real ledger rows by UTC day — today's total, the trailing median
// of the prior days, and the history depth.
func TestSpendWindowPricedDayBucketing(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun(t, "r.execute", "alice", "anthropic", "claude-cli")

	pt := NewEffectiveDatedTable("v1")
	pt.Add(PriceRow{Model: "m", Lane: "anthropic", EffectiveFrom: day(2000, 1, 1),
		Prices: UnitPrices{InputUSD: 3e-6, OutputUSD: 15e-6}})
	led := NewLedger(e.db, pt, NoMeteredExceptions(), e.reg)

	asOf := day(2026, 6, 15).Add(12 * time.Hour) // noon, deterministic
	// today: 1e6 in + 1e6 out = $3 + $15 = $18
	e.datedCheckpoint(t, "r.execute", "alice", "m", `{"input_tokens":1000000,"output_tokens":1000000}`, asOf)
	// prior day 1: 1e6 in = $3
	e.datedCheckpoint(t, "r.execute", "alice", "m", `{"input_tokens":1000000,"output_tokens":0}`, day(2026, 6, 14).Add(time.Hour))
	// prior day 2: 2e6 in = $6
	e.datedCheckpoint(t, "r.execute", "alice", "m", `{"input_tokens":2000000,"output_tokens":0}`, day(2026, 6, 13).Add(time.Hour))

	win, err := led.SpendWindow(ctx, "alice", asOf, 14)
	if err != nil {
		t.Fatalf("SpendWindow: %v", err)
	}
	if win.AnyUnpriced {
		t.Errorf("priced table must not mark AnyUnpriced: %+v", win)
	}
	if win.TodayUSD != 18 {
		t.Errorf("TodayUSD = %v, want 18 ($3 in + $15 out)", win.TodayUSD)
	}
	if win.DaysOfHistory != 2 {
		t.Errorf("DaysOfHistory = %d, want 2 (the two prior days)", win.DaysOfHistory)
	}
	if win.MedianUSD != 4.5 {
		t.Errorf("MedianUSD = %v, want 4.5 (median of $3 and $6)", win.MedianUSD)
	}
}

// TestSpendOwnersDerivesFromUsageNotActiveRuns (drain D11): the owner set is the
// day's spenders (usage rows), so a person whose run COMPLETED today still
// appears — the gap where a completed-run owner escaped that day's spend check.
func TestSpendOwnersDerivesFromUsageNotActiveRuns(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	// alice: a run that completed today but left a usage row.
	e.runningRun(t, "done.execute", "alice", "anthropic", "claude-cli")
	e.datedCheckpoint(t, "done.execute", "alice", "m", `{"input_tokens":100,"output_tokens":50}`, time.Now())
	if _, err := e.runs.Transition(ctx, "done.execute", run.StateCompleted, run.TransitionOptions{Actor: "platform"}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// bob: an active run today with a usage row.
	e.runningRun(t, "live.execute", "bob", "anthropic", "claude-cli")
	e.datedCheckpoint(t, "live.execute", "bob", "m", `{"input_tokens":100,"output_tokens":50}`, time.Now())

	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg)
	owners, err := led.SpendOwners(ctx, time.Now())
	if err != nil {
		t.Fatalf("SpendOwners: %v", err)
	}
	got := map[string]bool{}
	for _, o := range owners {
		got[o] = true
	}
	if !got["alice"] {
		t.Errorf("SpendOwners missed alice, whose run completed today — the exact gap D11 closes: %v", owners)
	}
	if !got["bob"] {
		t.Errorf("SpendOwners missed bob (active): %v", owners)
	}
}

func TestMedian(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{5}, 5},
		{[]float64{3, 1, 2}, 2},      // odd → middle
		{[]float64{4, 1, 3, 2}, 2.5}, // even → average of the two central
		{[]float64{10, 10, 10}, 10},  // constant
	}
	for _, c := range cases {
		if got := median(c.in); got != c.want {
			t.Errorf("median(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
