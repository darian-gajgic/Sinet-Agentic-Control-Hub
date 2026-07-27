package shell

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// history_seams_test.go — the B5-8B composition. Tier F: no local stack is
// dialed, so this is also the ABSENT-STACK case, which is the one that matters
// — the query surface must be fully usable with no model anywhere in the path.

func historyTestDB(t *testing.T) (*storage.DB, *eventlog.Log, *settings.Registry) {
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
	return db, log, reg
}

// TestHistorySurfaceComposesWithoutALocalStack: nil duty and nil calibration
// store are HONEST states, not failures. Composition succeeds and both
// model-free layers work — S14.10's Layer 0 has no model in it by definition,
// and Layer 1's named verbs do not need one either.
func TestHistorySurfaceComposesWithoutALocalStack(t *testing.T) {
	ctx := context.Background()
	db, log, _ := historyTestDB(t)
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)

	st, err := buildHistorySurface(db, log, runs, cps, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("buildHistorySurface with no local stack: %v", err)
	}
	if st == nil {
		t.Fatal("nil store")
	}
	scope := history.Scope{Operator: true}

	// Layer 0 — every registered view is selectable through the composed store.
	for _, v := range history.Views() {
		if v.OwnerColumn == "" {
			continue
		}
		if _, err := st.SelectView(ctx, v.Name, scope, 5); err != nil {
			t.Errorf("SelectView(%q): %v", v.Name, err)
		}
	}
	// Layer 1 — the named verb, with no model in the path.
	if _, err := st.RunQuery(ctx, "status.runs_active", nil, scope, 5); err != nil {
		t.Errorf("RunQuery: %v", err)
	}
	// The natural-language path degrades to the card rather than to an error.
	a, err := st.Ask(ctx, "what is running?", scope, 5)
	if err != nil {
		t.Fatalf("Ask with no stack returned an error instead of the floor: %v", err)
	}
	if a.Card == nil {
		t.Error("Ask with no stack did not produce a disambiguation card")
	}
	// Search works over an empty corpus without erroring.
	if _, err := st.Search(ctx, "anything", scope, 5); err != nil {
		t.Errorf("Search: %v", err)
	}
}

// TestHistoryAdvisoryOpensAMeteredRun: the S12.1 R18 rule is that every local
// duty call writes ONE D7 checkpoint on a consuming run. A question has no run
// of its own, so the seam opens a short-lived platform-scope advisory run — the
// same shape the S14.9 narrator uses.
func TestHistoryAdvisoryOpensAMeteredRun(t *testing.T) {
	ctx := context.Background()
	db, log, _ := historyTestDB(t)
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)

	adv := historyAdvisory(advisoryMeter(runs, cps))
	id, settle := adv(ctx, "history-intent")
	if id == "" {
		t.Fatal("the advisory seam yielded no run id")
	}
	r, err := runs.Get(ctx, id)
	if err != nil {
		t.Fatalf("the advisory run was not created: %v", err)
	}
	if r.UserID != run.ActorPlatform {
		t.Errorf("advisory run owner = %q, want the platform scope", r.UserID)
	}
	if r.State != run.StateRunning {
		t.Errorf("advisory run state = %q, want running (it must be checkpointable)", r.State)
	}
	if settle == nil {
		t.Fatal("no settle func — the advisory run would never end")
	}
	settle()
	after, err := runs.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != run.StateCompleted {
		t.Errorf("after settle the advisory run is %q, want completed", after.State)
	}
}
