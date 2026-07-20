package intake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The interview-taxonomy seed (Spec S06.5 TBD-P3): both v1 seeds validate,
// Clearance is the deterministic G1 P8 formula, and operator files load
// strictly.

func TestSeedTaxonomiesValidate(t *testing.T) {
	seeds := SeedTaxonomies()
	soft, ok := seeds[FamilySoftware]
	if !ok {
		t.Fatal("no software seed")
	}
	if err := soft.Validate(); err != nil {
		t.Fatalf("software seed: %v", err)
	}
	if len(soft.Slots) != 10 {
		t.Fatalf("software seed carries %d slots, want the ClarifyCodeBench 10", len(soft.Slots))
	}
	if !strings.Contains(soft.Source, "2607.00711") {
		t.Fatalf("software seed must cite its source: %s", soft.Source)
	}
	// The empirically failed types outweigh the natively handled ones
	// (R03 §2.3 evidence reading).
	if soft.Slot("collection_semantics").Weight <= soft.Slot("units").Weight {
		t.Fatal("collection_semantics must outweigh units (S40 evidence)")
	}
	gen, ok := seeds[FamilyGeneric]
	if !ok {
		t.Fatal("no generic fallback seed (S06.2: v0 ships software + generic)")
	}
	if err := gen.Validate(); err != nil {
		t.Fatalf("generic seed: %v", err)
	}
}

func TestClearance(t *testing.T) {
	tax := &Taxonomy{ID: "x", Family: FamilyGeneric, Version: "v1", Slots: []Slot{
		{ID: "a", Name: "A", Weight: 60, Question: "?"},
		{ID: "b", Name: "B", Weight: 40, Question: "?"},
	}}
	if err := tax.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := tax.Clearance(nil); got != 0 {
		t.Fatalf("empty clearance = %v", got)
	}
	if got := tax.Clearance(map[string]bool{"a": true}); got != 60 {
		t.Fatalf("clearance = %v, want 60", got)
	}
	if got := tax.Clearance(map[string]bool{"a": true, "b": true}); got != 100 {
		t.Fatalf("clearance = %v, want 100", got)
	}
}

func TestUnresolvedOrdering(t *testing.T) {
	tax := &Taxonomy{ID: "x", Family: FamilyGeneric, Version: "v1", Slots: []Slot{
		{ID: "low", Name: "L", Weight: 5, Question: "?"},
		{ID: "high", Name: "H", Weight: 12, Question: "?"},
		{ID: "mid", Name: "M", Weight: 8, Question: "?"},
	}}
	got := tax.Unresolved(map[string]bool{})
	if got[0].ID != "high" || got[1].ID != "mid" || got[2].ID != "low" {
		t.Fatalf("unresolved order = %v, want highest weight first", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestTaxonomyValidation(t *testing.T) {
	bad := []Taxonomy{
		{ID: "x", Version: "v1"}, // no slots
		{ID: "x", Version: "v1", Slots: []Slot{{ID: "a", Weight: 0, Question: "?"}}},                                                // weight
		{ID: "x", Version: "v1", Slots: []Slot{{ID: "a", Weight: 1, Question: "?"}, {ID: "a", Weight: 1, Question: "?"}}},           // dup
		{ID: "x", Version: "v1", Slots: []Slot{{ID: "a", Weight: 1, Question: "?", Options: []Option{{Label: "one", Value: "1"}}}}}, // 1 option
	}
	for i, tax := range bad {
		if err := tax.Validate(); err == nil {
			t.Fatalf("case %d: invalid taxonomy accepted", i)
		}
	}
}

func TestLoadTaxonomyStrict(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "tax.json")
	os.WriteFile(good, []byte(`{"id":"custom","family":"software","version":"v2","source":"operator edit","slots":[{"id":"a","name":"A","must_know":"m","weight":3,"question":"?"}]}`), 0o644)
	tax, err := LoadTaxonomy(good)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if tax.Version != "v2" || tax.Slots[0].Weight != 3 {
		t.Fatalf("loaded %+v", tax)
	}
	badPath := filepath.Join(dir, "bad.json")
	os.WriteFile(badPath, []byte(`{"id":"x","version":"v1","slots":[],"surprise":true}`), 0o644)
	if _, err := LoadTaxonomy(badPath); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
}
