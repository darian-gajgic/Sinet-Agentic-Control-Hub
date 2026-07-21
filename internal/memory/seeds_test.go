package memory_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// Spec S09.10: the B2 seed objects live as governed house-scope L2 entries
// with the B2-gate ratification as provenance.

func TestB2SeedGovernance(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()

	// House scope needs its D10 holder.
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")

	created, err := f.gate.EnsureB2SeedGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	if created != 6 {
		t.Fatalf("created = %d, want the 6 B2 seed objects", created)
	}

	// Every governed entry: house scope, imported origin, the B2-gate
	// ratification recorded, file-backed under knowledge/house/.
	for _, id := range []string{
		"seed-intake-taxonomy-software", "seed-intake-taxonomy-generic",
		"seed-intake-p47-triggers", "seed-verify-rubric-software",
		"seed-verify-golden-software", "seed-verify-calibration-entailment",
	} {
		e, err := f.store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if e.Scope != memory.ScopeHouse || e.Origin != memory.OriginImported ||
			e.Status != memory.StatusActive || e.ApprovedBy != "darian" {
			t.Fatalf("%s = %+v", id, e)
		}
		if !strings.Contains(e.OriginRef, "B2-report.md") || !strings.Contains(e.OriginRef, "2026-07-20") {
			t.Fatalf("%s provenance = %q, want the B2-gate ratification record", id, e.OriginRef)
		}
		if e.FilePath == "" || !strings.HasPrefix(e.FilePath, "house/") {
			t.Fatalf("%s file path = %q", id, e.FilePath)
		}
		// House objects carry the ⚙ re-verify interval (default 180 d).
		if e.ReverifyIntervalDays != 180 {
			t.Fatalf("%s reverify = %d, want 180", id, e.ReverifyIntervalDays)
		}
	}

	// The governed files ARE the operator-editable strict-JSON forms: the
	// owning packages' Load* paths parse and validate them byte-for-byte
	// (CONVENTIONS §14/§15 override machinery unchanged).
	if _, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-software.json")); err != nil {
		t.Fatalf("governed software taxonomy fails LoadTaxonomy: %v", err)
	}
	if _, err := intake.LoadTriggers(filepath.Join(f.root, "house", "intake-p47-triggers.json")); err != nil {
		t.Fatalf("governed trigger file fails LoadTriggers: %v", err)
	}
	if _, err := verify.LoadRubric(filepath.Join(f.root, "house", "verify-rubric-software.json")); err != nil {
		t.Fatalf("governed rubric fails LoadRubric: %v", err)
	}
	if _, err := verify.LoadGoldenSet(filepath.Join(f.root, "house", "verify-golden-software.json")); err != nil {
		t.Fatalf("governed golden set fails LoadGoldenSet: %v", err)
	}
	if _, err := verify.LoadCalibrationSet(filepath.Join(f.root, "house", "verify-calibration-entailment.json")); err != nil {
		t.Fatalf("governed calibration set fails LoadCalibrationSet: %v", err)
	}

	// Idempotent across boots.
	created, err = f.gate.EnsureB2SeedGovernance(ctx)
	if err != nil || created != 0 {
		t.Fatalf("second ensure: created=%d err=%v", created, err)
	}

	// Governance is real: removal sticks — a later boot never resurrects
	// a removed object (no private update path around the gate).
	if _, err := f.gate.Remove(ctx, "darian", "seed-verify-golden-software", "superseded by B4 set"); err != nil {
		t.Fatalf("Remove seed: %v", err)
	}
	created, err = f.gate.EnsureB2SeedGovernance(ctx)
	if err != nil || created != 0 {
		t.Fatalf("ensure after removal: created=%d err=%v", created, err)
	}
	e, err := f.store.Get(ctx, "seed-verify-golden-software")
	if err != nil || e.Status != memory.StatusRemoved {
		t.Fatalf("removed seed = %+v (%v)", e, err)
	}

	// Machinery objects never inject into stage briefs: their task-type
	// selector has no confirmable stage fact (conservative matching) —
	// the golden set's planted defects must never reach a prompt.
	f.task("t1", "darian")
	items, err := (&memory.Source{S: f.store}).Items(ctx,
		ledger.SliceQuery{TaskID: "t1", Owner: "darian", Stage: "step-1"})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	for _, it := range items {
		if strings.HasPrefix(it.ItemID, "knowledge/seed-") {
			t.Fatalf("governed machinery object leaked into a stage brief: %s", it.ItemID)
		}
	}

	// Still searchable (title index) under house scope.
	f.run("r1", "t1", "darian")
	hits, err := f.store.Search(ctx, "r1", "taxonomy")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var sawSeed bool
	for _, h := range hits {
		if strings.HasPrefix(h.EntryID, "seed-intake-taxonomy") {
			sawSeed = true
		}
	}
	if !sawSeed {
		t.Fatalf("governed seeds unsearchable: %+v", hits)
	}
}

// Spec S09.10 (B3-5): the composer playbook as a governed house object —
// idempotent seeding pending the B3-gate ratification, and HouseObject
// serving the CURRENT approved version to the composer seam.
func TestComposerPlaybookGovernance(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()

	if _, err := f.gate.EnsureComposerPlaybook(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")

	created, err := f.gate.EnsureComposerPlaybook(ctx)
	if err != nil || !created {
		t.Fatalf("EnsureComposerPlaybook: created=%v err=%v", created, err)
	}
	// Idempotent: a second boot creates nothing.
	if created, err := f.gate.EnsureComposerPlaybook(ctx); err != nil || created {
		t.Fatalf("second ensure: created=%v err=%v", created, err)
	}

	e, err := f.store.Get(ctx, memory.ComposerPlaybookEntryID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.Scope != memory.ScopeHouse || e.Kind != memory.KindPlaybook ||
		e.Status != memory.StatusActive || e.Origin != memory.OriginImported {
		t.Fatalf("entry = %+v", e)
	}
	if !strings.Contains(e.OriginRef, "B3") || !strings.Contains(e.OriginRef, "pending") {
		t.Fatalf("provenance = %q, want the pending B3-gate ratification record", e.OriginRef)
	}
	if e.TopicKey != worker.ComposerPlaybookTopicKey {
		t.Fatalf("topic key = %q", e.TopicKey)
	}

	// HouseObject returns the current approved version WITH file content.
	cur, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if err != nil {
		t.Fatalf("HouseObject: %v", err)
	}
	if cur.ID != memory.ComposerPlaybookEntryID || !strings.Contains(cur.Content, "one-shot") {
		t.Fatalf("house object = id %q, content %d bytes", cur.ID, len(cur.Content))
	}

	// A gated new version supersedes; the composer reads the NEW current
	// approved version (Spec S09.10: edits operator-gated, no private path).
	v2, err := f.gate.NewVersion(ctx, "darian", cur.ID, memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindPlaybook,
		Title:    worker.ComposerPlaybookTitle + " (v2)",
		Content:  "# Composer playbook v2\nRevised practice.\n",
		TopicKey: worker.ComposerPlaybookTopicKey,
	})
	if err != nil {
		t.Fatalf("NewVersion: %v", err)
	}
	cur2, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if err != nil {
		t.Fatalf("HouseObject v2: %v", err)
	}
	if cur2.ID != v2.ID || !strings.Contains(cur2.Content, "Revised practice") {
		t.Fatalf("current approved version = %q, want the superseding %q", cur2.ID, v2.ID)
	}
	// Never resurrected after supersession (the retired seed stays retired).
	if created, err := f.gate.EnsureComposerPlaybook(ctx); err != nil || created {
		t.Fatalf("ensure after supersession: created=%v err=%v", created, err)
	}
}
