package stage

// Budget-watcher unit tests (Spec S05.3 ⚙ consumption): stage-fit at
// session start, overflow → stage-split proposal, second overflow within
// one planned stage → re-plan — all as durable context.overflow events.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

func newWatcherHarness(t *testing.T) (*Skeleton, run.Run) {
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
	r, err := runs.Create(ctx, run.NewRun{ID: "t-w.execute", UserID: "u1", Substrate: "claude-cli", Lane: "anthropic"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Internal construction: the watcher needs only Settings + Log.
	return &Skeleton{cfg: Config{DB: db, Log: log, Settings: reg}}, r
}

func usageEvent(prompt, output int64) adapters.Event {
	return adapters.Event{Kind: adapters.KindUsage, Usage: &adapters.Usage{
		InputTokens: prompt, OutputTokens: output,
	}}
}

func overflowEvents(t *testing.T, s *Skeleton, runID string) []map[string]any {
	t.Helper()
	rows, err := s.cfg.DB.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq`, runID, EventContextOverflow)
	if err != nil {
		t.Fatalf("query overflow events: %v", err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, m)
	}
	return out
}

// Defaults: window 200k, ⚙ fit 0.5 → 100k, ⚙ overflow 0.7 → 140k.

func TestBudgetWatcherUnderThresholdsIsSilent(t *testing.T) {
	s, r := newWatcherHarness(t)
	w := s.newBudgetWatcher(context.Background(), r, "S-1", 0)
	w.observe(usageEvent(50_000, 1_000))
	w.observe(usageEvent(90_000, 2_000))
	rep := w.report()
	if rep.Overflowed || rep.FitExceeded || rep.OverflowSeq != 0 {
		t.Fatalf("report = %+v, want silent", rep)
	}
	if evs := overflowEvents(t, s, r.ID); len(evs) != 0 {
		t.Fatalf("%d overflow events, want 0", len(evs))
	}
}

func TestBudgetWatcherOverflowProposesStageSplit(t *testing.T) {
	s, r := newWatcherHarness(t)
	w := s.newBudgetWatcher(context.Background(), r, "S-1", 0)
	w.observe(usageEvent(80_000, 1_000))  // fine at start
	w.observe(usageEvent(150_000, 5_000)) // crosses 140k
	w.observe(usageEvent(160_000, 5_000)) // still over: no second event per session
	rep := w.report()
	if !rep.Overflowed || rep.Proposal != "stage-split" || rep.OverflowSeq == 0 {
		t.Fatalf("report = %+v, want stage-split overflow", rep)
	}
	evs := overflowEvents(t, s, r.ID)
	if len(evs) != 1 {
		t.Fatalf("%d overflow events, want exactly 1 per session", len(evs))
	}
	if evs[0]["proposal"] != "stage-split" || evs[0]["stage"] != "S-1" {
		t.Fatalf("payload = %v", evs[0])
	}
}

func TestBudgetWatcherSecondOverflowEscalatesToReplan(t *testing.T) {
	s, r := newWatcherHarness(t)
	// PriorOverflows=1: an earlier session of this planned stage already
	// proposed a split (Spec S05.3 "a second overflow within one planned
	// stage escalates to a re-plan proposal").
	w := s.newBudgetWatcher(context.Background(), r, "S-1", 1)
	w.observe(usageEvent(80_000, 0))
	w.observe(usageEvent(150_000, 0))
	rep := w.report()
	if !rep.Overflowed || rep.Proposal != "re-plan" {
		t.Fatalf("report = %+v, want re-plan", rep)
	}
}

func TestBudgetWatcherOverweightBriefIsAPlanShapeDefect(t *testing.T) {
	s, r := newWatcherHarness(t)
	w := s.newBudgetWatcher(context.Background(), r, "S-1", 0)
	// The FIRST call's footprint already exceeds the fit target: the brief
	// cannot fit — a plan-shape defect raising a re-plan proposal
	// (Spec S05.3, G1 Def.11).
	w.observe(usageEvent(120_000, 0))
	rep := w.report()
	if !rep.FitExceeded {
		t.Fatalf("report = %+v, want FitExceeded", rep)
	}
	evs := overflowEvents(t, s, r.ID)
	if len(evs) != 1 || evs[0]["proposal"] != "re-plan" {
		t.Fatalf("events = %v, want one re-plan proposal", evs)
	}
}
