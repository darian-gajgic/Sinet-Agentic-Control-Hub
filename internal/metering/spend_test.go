package metering

import (
	"context"
	"testing"
	"time"
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
