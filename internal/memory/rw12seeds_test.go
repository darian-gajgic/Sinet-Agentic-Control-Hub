package memory_test

// rw12seeds_test.go — the P3-RW-12 §7 T11 leg: the Deep-Plan taxonomies enter
// the 8.3 knowledge gate (Spec S09.10 row 1, S09.4 station 4, S09.8).

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
)

// rw12Families are the four families whose question sets P3-RW-12 seeds.
var rw12Families = []intake.Family{
	intake.FamilyResearch, intake.FamilyContent, intake.FamilyData, intake.FamilyChore,
}

// TestTaxonomyGovernanceCreatesAndSupersedes (§7 T11; R5): the four new
// question sets become governed house objects, the software set moves to v2
// by SUPERSESSION rather than an in-place edit, every governed file is the
// strict JSON the operator can edit, and a boot never resurrects what someone
// removed.
func TestTaxonomyGovernanceCreatesAndSupersedes(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()

	// House scope needs its D10 holder; without one the boot skips and retries.
	if _, _, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")

	// The world as it stands after B2: the software set governed at its
	// B2-ratified v1 content.
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	b2Software, err := f.store.Get(ctx, "seed-intake-taxonomy-software")
	if err != nil {
		t.Fatalf("Get software seed: %v", err)
	}
	if !strings.Contains(b2Software.OriginRef, "B2-report.md") {
		t.Fatalf("B2 entry provenance = %q", b2Software.OriginRef)
	}

	created, superseded, err := f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureRW12TaxonomyGovernance: %v", err)
	}
	if created != 4 {
		t.Errorf("created = %d, want the 4 new family taxonomies", created)
	}
	if superseded != 1 {
		t.Errorf("superseded = %d, want the software set moved to v2", superseded)
	}

	for _, fam := range rw12Families {
		id := "seed-intake-taxonomy-" + string(fam)
		e, err := f.store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if e.Scope != memory.ScopeHouse || e.Kind != memory.KindTaxonomy ||
			e.Status != memory.StatusActive || e.Origin != memory.OriginImported {
			t.Errorf("%s = %+v", id, e)
		}
		if e.TopicKey != "intake/taxonomy/"+string(fam) {
			t.Errorf("%s topic key = %q", id, e.TopicKey)
		}
		if e.Selectors.TaskType != "machinery:intake" {
			t.Errorf("%s selector = %+v, want the machinery marker (never a stage brief)", id, e.Selectors)
		}
		if !strings.Contains(e.OriginRef, "P3-RW-12") {
			t.Errorf("%s provenance = %q, want the RW-12 ratification record", id, e.OriginRef)
		}
		if e.FilePath != filepath.Join("house", "intake-taxonomy-"+string(fam)+".json") {
			t.Errorf("%s file path = %q", id, e.FilePath)
		}
	}

	// Supersession, not an edit: the B2 row is RETIRED and the new active
	// version chains to it (S09.8).
	old, err := f.store.Get(ctx, "seed-intake-taxonomy-software")
	if err != nil {
		t.Fatalf("Get retired software seed: %v", err)
	}
	if old.Status != memory.StatusRetired {
		t.Errorf("B2 software entry status = %q, want retired", old.Status)
	}
	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatalf("HouseObject software: %v", err)
	}
	if cur.Supersedes != old.ID {
		t.Errorf("new software version supersedes %q, want %q", cur.Supersedes, old.ID)
	}
	if cur.Version != old.Version+1 {
		t.Errorf("new software version = %d, want %d", cur.Version, old.Version+1)
	}
	if !strings.Contains(cur.OriginRef, "P3-RW-12") {
		t.Errorf("superseding version provenance = %q", cur.OriginRef)
	}

	// Every governed active file IS the operator-editable override input, and
	// it says exactly what the runtime seed says (the §17 proven-by-test rule).
	seeds := intake.SeedTaxonomies()
	for _, fam := range append([]intake.Family{intake.FamilySoftware, intake.FamilyGeneric}, rw12Families...) {
		path := filepath.Join(f.root, "house", "intake-taxonomy-"+string(fam)+".json")
		got, err := intake.LoadTaxonomy(path)
		if err != nil {
			t.Fatalf("governed %s taxonomy fails LoadTaxonomy: %v", fam, err)
		}
		if !reflect.DeepEqual(got, seeds[fam]) {
			t.Errorf("governed %s file diverges from the in-code seed", fam)
		}
	}

	// Idempotent across boots: nothing created, nothing superseded twice.
	created, superseded, err = f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil || created != 0 || superseded != 0 {
		t.Fatalf("second ensure: created=%d superseded=%d err=%v", created, superseded, err)
	}

	// Governance is real: a removed object stays dead.
	if _, err := f.gate.Remove(ctx, "darian", "seed-intake-taxonomy-research", "not wanted"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	created, _, err = f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil || created != 0 {
		t.Fatalf("ensure after removal: created=%d err=%v", created, err)
	}
	e, err := f.store.Get(ctx, "seed-intake-taxonomy-research")
	if err != nil || e.Status != memory.StatusRemoved {
		t.Fatalf("removed entry = %+v (%v)", e, err)
	}
}

// TestB2GovernedTaxonomyIsTheRatifiedSnapshot (§11 governance-divergence
// trap): the B2 entry records what the B2 gate ACTUALLY ratified. A fresh
// world must not write today's v2 content under the B2 provenance — the
// ratification record is a snapshot, never a live pointer at whatever the
// code says now.
func TestB2GovernedTaxonomyIsTheRatifiedSnapshot(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("darian", "operator")
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	got, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-software.json"))
	if err != nil {
		t.Fatalf("LoadTaxonomy: %v", err)
	}
	if got.Version != "v1" {
		t.Errorf("B2-governed software taxonomy is %q; the B2 gate ratified v1", got.Version)
	}
	if len(got.Slots) != 10 {
		t.Errorf("B2-governed software taxonomy carries %d slots; the B2 gate ratified the ClarifyCodeBench 10", len(got.Slots))
	}
}
