package intake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The interview-taxonomy seeds (Spec S06.5): every family carries its own
// Deep-Plan question set, Clearance is the deterministic G1 P8 formula, and
// operator files load strictly.

// ccbSlotIDs are the ten ClarifyCodeBench clarity types (arXiv:2607.00711
// Table 2) the software set was seeded from at P3-B2-2 — the evidence pin
// P3-RW-12 keeps whole while adding Deep-Plan depth around it.
var ccbSlotIDs = []string{
	"behavior", "terminology", "edge_cases", "collection_semantics", "comparison_rules",
	"ordering_atomicity", "indices_ranges", "output_format", "units", "numerical_precision",
}

// rw12SoftwareSlotIDs are the three Deep-Plan slots P3-RW-12 adds: the
// technical and visual choices a requester who is not a programmer cannot be
// expected to volunteer (operator directive, 2026-08-12).
var rw12SoftwareSlotIDs = []string{"technology_stack", "assets_media", "look_feel"}

// TestSeedTaxonomiesCoverSixFamilies (P3-RW-12 §7 T1; rewrites the B2-era
// TestSeedTaxonomiesValidate, which pinned software to exactly the ten CCB
// slots): every family in the vocabulary has its OWN validating question set,
// software is v2 with the ten CCB slots plus the three Deep-Plan ones, and
// every set records where its content came from.
func TestSeedTaxonomiesCoverSixFamilies(t *testing.T) {
	seeds := SeedTaxonomies()
	if len(seeds) != len(Families()) {
		t.Fatalf("SeedTaxonomies covers %d families, want all %d of Families()", len(seeds), len(Families()))
	}
	for _, fam := range Families() {
		tax, ok := seeds[fam]
		if !ok {
			t.Fatalf("no question set seeded for the %s family (S06.5 per-kind-of-task question sets)", fam)
		}
		if err := tax.Validate(); err != nil {
			t.Fatalf("%s seed: %v", fam, err)
		}
		if tax.ID != string(fam) || tax.Family != fam {
			t.Errorf("%s seed identifies as id=%q family=%q", fam, tax.ID, tax.Family)
		}
		if tax.Version == "" || tax.Source == "" {
			t.Errorf("%s seed has no version/provenance: %q / %q", fam, tax.Version, tax.Source)
		}
		for _, s := range tax.Slots {
			if s.MustKnow == "" {
				t.Errorf("%s slot %q states no MustKnow — the reason it is asked at all (S06.5)", fam, s.ID)
			}
			if s.Name == "" {
				t.Errorf("%s slot %q has no plain-language name (it is rendered back in the understanding block)", fam, s.ID)
			}
		}
	}

	soft := seeds[FamilySoftware]
	if soft.Version != "v2" {
		t.Errorf("software seed version = %q, want v2 (the Deep-Plan revision)", soft.Version)
	}
	for _, id := range append(append([]string{}, ccbSlotIDs...), rw12SoftwareSlotIDs...) {
		if soft.Slot(id) == nil {
			t.Errorf("software v2 lost slot %q", id)
		}
	}
	// The CCB citation stays — the ten clarity slots are benchmark-derived and
	// the drafting provenance is additive, never a replacement.
	if !strings.Contains(soft.Source, "2607.00711") {
		t.Errorf("software seed must keep citing ClarifyCodeBench: %s", soft.Source)
	}
	for _, want := range []string{"P3-RW-12", "claude-opus-5", "2026-08-13"} {
		if !strings.Contains(soft.Source, want) {
			t.Errorf("software seed source is missing the RW-12 drafting provenance %q: %s", want, soft.Source)
		}
	}
	// The evidence pin: the empirically failed types outweigh the natively
	// handled ones (R03 §2.3), and the reasoned Deep-Plan slots sit BELOW the
	// measured failure types — reasoning never outranks measurement.
	if soft.Slot("collection_semantics").Weight <= soft.Slot("units").Weight {
		t.Error("collection_semantics must outweigh units (S40 evidence)")
	}
	for _, id := range rw12SoftwareSlotIDs {
		if soft.Slot(id).Weight >= soft.Slot("collection_semantics").Weight {
			t.Errorf("reasoned slot %q outweighs the benchmark-failed collection_semantics", id)
		}
		if soft.Slot(id).Weight <= soft.Slot("units").Weight {
			t.Errorf("slot %q does not outrank the natively-handled units type", id)
		}
	}
}

// TestSeedWeightsMeetTierFloorShape (P3-RW-12 §7 T2; R10): depth scales by
// tier through the EXISTING ⚙ clearance floors alone — so every seeded set's
// weight distribution must let highest-weight-first questioning cross the low
// floor (60) within two cards and the standard floor (75) within three, at the
// ratified defaults. No per-tier question caps exist to rescue a flat set
// (G1 P8 rejected them).
func TestSeedWeightsMeetTierFloorShape(t *testing.T) {
	for fam, tax := range SeedTaxonomies() {
		total := 0
		for _, s := range tax.Slots {
			total += s.Weight
		}
		// Unresolved(nil) IS the pipeline's own ordering: highest weight first.
		order := tax.Unresolved(nil)
		cum := func(n int) float64 {
			if n > len(order) {
				n = len(order)
			}
			sum := 0
			for _, s := range order[:n] {
				sum += s.Weight
			}
			return 100 * float64(sum) / float64(total)
		}
		if got := cum(2 * maxQuestionsPerCard); got < 60 {
			t.Errorf("%s: clearance after 2 cards = %.1f%%, below the low floor 60 (R10)", fam, got)
		}
		if got := cum(3 * maxQuestionsPerCard); got < 75 {
			t.Errorf("%s: clearance after 3 cards = %.1f%%, below the standard floor 75 (R10)", fam, got)
		}
	}
}

// TestStackSlotOffersPlannerChoosesDefault (P3-RW-12 §7 T4; R1): the
// technology choice is asked in words a non-programmer can answer, and its
// FIRST option is the platform choosing and showing what it picked — the
// requester never has to know a stack to get one. The strings are pinned
// exactly: the operator ratifies this content at the packet gate, and a test
// that pins them is what makes a later silent reword impossible.
func TestStackSlotOffersPlannerChoosesDefault(t *testing.T) {
	soft := SeedTaxonomies()[FamilySoftware]
	stack := soft.Slot("technology_stack")
	if stack == nil {
		t.Fatal("software v2 has no technology_stack slot (R1)")
	}
	if stack.Question != "What should this be built with?" {
		t.Errorf("technology_stack question = %q", stack.Question)
	}
	if len(stack.Options) < 2 || len(stack.Options) > 4 {
		t.Fatalf("technology_stack carries %d options, want 2–4 (S06.5)", len(stack.Options))
	}
	if stack.Options[0].Value != plannerChoosesValue {
		t.Errorf("technology_stack default option = %q, want %q first (R1)", stack.Options[0].Value, plannerChoosesValue)
	}
	if stack.Options[0].Label != "You choose for me and show me what you picked" {
		t.Errorf("technology_stack default label = %q", stack.Options[0].Label)
	}
	// Every technical choice across every seeded set offers the same escape
	// hatch, in the same words and under the same canonical value.
	for fam, tax := range SeedTaxonomies() {
		for _, s := range tax.Slots {
			for i, o := range s.Options {
				if o.Value != plannerChoosesValue {
					continue
				}
				if i != 0 {
					t.Errorf("%s/%s: the planner-chooses option is at index %d, want first", fam, s.ID, i)
				}
				if !strings.HasPrefix(o.Label, "You choose") {
					t.Errorf("%s/%s: planner-chooses label = %q, want the shared plain wording", fam, s.ID, o.Label)
				}
			}
		}
	}
	for _, id := range rw12SoftwareSlotIDs {
		s := soft.Slot(id)
		if len(s.Options) == 0 {
			t.Errorf("software slot %q offers no options — a non-programmer needs choices, not a blank box", id)
			continue
		}
		if s.Options[0].Value != plannerChoosesValue {
			t.Errorf("software slot %q does not lead with the planner-chooses default", id)
		}
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
