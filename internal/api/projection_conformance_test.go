package api

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// projection_conformance_test.go — the B5-4 red→card inbox derivation (R15/R16,
// S14.5): a RED conformance-registry row surfaces as an inbox card by
// derivation, flag-now exactly on lane/storage-affecting rows; green raises
// nothing; a member never sees platform-scope cards. Internal test (package
// api) driving the projector directly — internal/api never imports
// internal/conformance, so the rows are inserted as raw table state (the
// watchdog-flags derive-from-log precedent).

func insertConfRow(t *testing.T, db *storage.DB, id, affect, lastResult string) {
	t.Helper()
	ctx := context.Background()
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		var lr any
		if lastResult != "" {
			lr = lastResult
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO conformance_registry
			   (row_id, owning_section, fixtures, trigger_set, schedule, cadence, affect_class, last_run, last_result)
			 VALUES (?, 'S14.5', 'go test ./x/', 'quarterly', 'quarterly sweep', 'quarterly', ?, '2026-07-23T00:00:00Z', ?)`,
			id, affect, lr)
		return err
	}); err != nil {
		t.Fatalf("insert conformance row %q: %v", id, err)
	}
}

func TestConformanceRedCardsInboxOwnerScoped(t *testing.T) {
	ctx := context.Background()
	db, _, _ := wdTestDB(t)
	p := &projector{db: db}

	insertConfRow(t, db, "adapter-anthropic", "lane", "red")      // lane-affecting → flag-now
	insertConfRow(t, db, "kill9-suspend-cycle", "storage", "red") // storage-affecting → flag-now
	insertConfRow(t, db, "forced-escalation-e2e", "none", "red")  // neither → ordinary card
	insertConfRow(t, db, "adapter-local", "lane", "green")        // green → nothing
	insertConfRow(t, db, "compaction-canary", "lane", "")         // never run → nothing (OQ3)

	op, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("operator inbox: %v", err)
	}
	if len(op.ConformanceCards) != 3 {
		t.Fatalf("operator should see 3 RED conformance cards, got %d: %+v", len(op.ConformanceCards), op.ConformanceCards)
	}
	byID := map[string]InboxConformanceCard{}
	for _, c := range op.ConformanceCards {
		byID[c.RowID] = c
		if c.LastResult != "red" {
			t.Errorf("card %q last_result = %q, want red (green/never-run raise nothing)", c.RowID, c.LastResult)
		}
	}
	if !byID["adapter-anthropic"].FlagNow {
		t.Error("a lane-affecting red row must be flag-now (R14)")
	}
	if !byID["kill9-suspend-cycle"].FlagNow {
		t.Error("a storage-affecting red row must be flag-now (R14)")
	}
	if byID["forced-escalation-e2e"].FlagNow {
		t.Error("a neither-affecting red row must NOT be flag-now — an ordinary approval-class card (R14)")
	}
	if _, ok := byID["adapter-local"]; ok {
		t.Error("a GREEN row raised a card — green must raise nothing (S14.5)")
	}
	if _, ok := byID["compaction-canary"]; ok {
		t.Error("a NEVER-RUN row raised a card — dueness raises nothing at v0 (OQ3)")
	}

	// A member sees NO platform-scope conformance cards (R16, S01.9).
	m, err := p.inbox(ctx, ownerScope{UserID: "alice"})
	if err != nil {
		t.Fatalf("member inbox: %v", err)
	}
	if len(m.ConformanceCards) != 0 {
		t.Fatalf("a member must see no platform-scope conformance cards, got %d", len(m.ConformanceCards))
	}
}

// TestRegressionSweepRedRaisesTheOwnerCard pins the B5-5 claim that a red
// S14.8 regression sweep reaches its owner: the sweep row's resting state IS
// the sweep verdict (red if any asset came back red), and a red RECORDED row
// surfaces through this same derivation — never a bare asks row, since a suite
// run has no platform run (Spec S14.8 ¶3/¶5; S14.5).
func TestRegressionSweepRedRaisesTheOwnerCard(t *testing.T) {
	ctx := context.Background()
	const sweepRow = "regression-eval-sweep" // internal/api never imports internal/conformance

	red, _, _ := wdTestDB(t)
	insertConfRow(t, red, sweepRow, "none", "red")
	op, err := (&projector{db: red}).inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(op.ConformanceCards) != 1 || op.ConformanceCards[0].RowID != sweepRow {
		t.Fatalf("a red regression sweep must raise its card: %+v", op.ConformanceCards)
	}
	if op.ConformanceCards[0].FlagNow {
		t.Error("the sweep row is neither lane- nor storage-affecting — an ordinary approval-class card")
	}

	// Non-tautological: the same row green raises nothing.
	green, _, _ := wdTestDB(t)
	insertConfRow(t, green, sweepRow, "none", "green")
	op, err = (&projector{db: green}).inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(op.ConformanceCards) != 0 {
		t.Fatalf("a green sweep must raise nothing: %+v", op.ConformanceCards)
	}
}
