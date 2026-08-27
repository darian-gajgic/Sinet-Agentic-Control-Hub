package intake_test

// familygate_test.go — the P3-RW-11 acceptance battery for interview family
// reachability inside the pipeline (brief §7 T1, T4, T5, T6, T8; Spec S06.2,
// S06.5, S06.4, S13.7).
//
// The headline these assert: every task resolves its question-set family
// through REAL machinery — a registered project's captured family, the local
// classifier, or a first-class question to the requester — and never through a
// silent generic fallback. The fixtures and fakes are pipeline_test.go's (same
// test package), so this file adds cases, never a second harness.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// softwareSlotIDs is the seeded software question set's slot ids, read from the
// SHIPPING seed rather than a copy of it.
func softwareSlotIDs(t *testing.T) map[string]bool {
	t.Helper()
	tax, ok := intake.SeedTaxonomies()[intake.FamilySoftware]
	if !ok {
		t.Fatal("SeedTaxonomies has no software set")
	}
	ids := map[string]bool{}
	for _, s := range tax.Slots {
		ids[s.ID] = true
	}
	return ids
}

func generation(t *testing.T, f *fix, runID string) int64 {
	t.Helper()
	r, err := f.runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("run %s: %v", runID, err)
	}
	return r.Generation
}

// askStatus reads one ask row's status ("" when the row is absent).
func askStatus(t *testing.T, f *fix, askID string) string {
	t.Helper()
	var status string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT status FROM asks WHERE ask_id = ?`, askID).Scan(&status); err != nil {
		t.Fatalf("ask %s: %v", askID, err)
	}
	return status
}

// cardKindsIssued lists every ask kind issued on a run, oldest first.
func cardKindsIssued(t *testing.T, f *fix, runID string) []string {
	t.Helper()
	rows, err := f.db.QueryContext(context.Background(),
		`SELECT snapshot FROM asks WHERE run_id = ? ORDER BY rowid`, runID)
	if err != nil {
		t.Fatalf("asks for %s: %v", runID, err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var snapshot string
		if err := rows.Scan(&snapshot); err != nil {
			t.Fatal(err)
		}
		var card intake.Card
		if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, string(card.Kind))
	}
	return kinds
}

// humanDecisionTexts returns the ledger's human-authored decision texts.
func humanDecisionTexts(t *testing.T, f *fix, taskID string) []string {
	t.Helper()
	var out []string
	for _, d := range f.ledgerDoc(taskID).Decisions {
		if d.Author == ledger.AuthorHuman {
			out = append(out, d.Text)
		}
	}
	return out
}

// platformDecisionTexts returns the ledger's platform-authored decision texts.
func platformDecisionTexts(t *testing.T, f *fix, taskID string) []string {
	t.Helper()
	var out []string
	for _, d := range f.ledgerDoc(taskID).Decisions {
		if d.Author == ledger.AuthorPlatform {
			out = append(out, d.Text)
		}
	}
	return out
}

// TestStartRegistryFamilySelectsTaxonomy (T1; R1, S06.2 step 2, S13.7, S1.6):
// a task in a project whose capture declares a family gets THAT family's
// question set at Start — no family question, no classifier, and the
// fail-closed high tier stands because no classifier ran.
func TestStartRegistryFamilySelectsTaxonomy(t *testing.T) {
	f := newFix(t)
	f.p.Classifier = nil
	f.p.Registry = &fakeRegistry{ok: true, slice: intake.RegistrySlice{
		Project: "shop", Ref: "shop@capture-v1", Family: intake.FamilySoftware,
	}}

	st := f.start(stdRequest())
	if st.Family != intake.FamilySoftware {
		t.Errorf("family = %q, want software (the registry's captured family, R1)", st.Family)
	}
	if st.TaxonomyID != "software" {
		t.Errorf("taxonomy = %q, want software", st.TaxonomyID)
	}
	if st.FamilySource != intake.FamilySourceRegistry {
		t.Errorf("family source = %q, want %q", st.FamilySource, intake.FamilySourceRegistry)
	}
	if st.Tier != intake.TierHigh {
		t.Errorf("tier = %q, want high (no classifier ⇒ fail closed, S06.2)", st.Tier)
	}

	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("first card kind = %q, want interview (a resolved family is never asked)", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	ids := softwareSlotIDs(t)
	if len(card.Questions) == 0 {
		t.Fatal("interview card carries no questions")
	}
	for _, q := range card.Questions {
		if !ids[q.ID] {
			t.Errorf("question %q is not a software slot — the wrong taxonomy fired", q.ID)
		}
	}
	for _, k := range cardKindsIssued(t, f, st.RunID) {
		if k == string(intake.CardFamily) {
			t.Error("a family question was issued for a family the registry already resolved (R1)")
		}
	}
}

// TestFamilyCardFiresWhenUnresolved (T4; R4, S06.5 + 1.7 ask-don't-assume,
// S06.1 gates wait, D10): with no registry family and no classifier the FIRST
// card is the family question; answering it swaps the taxonomy and the SAME
// answer-drive asks that family's questions next.
func TestFamilyCardFiresWhenUnresolved(t *testing.T) {
	f := newFix(t)
	f.p.Classifier = nil
	f.p.Registry = nil

	st := f.start(stdRequest())
	if st.FamilySource != intake.FamilySourceDefault {
		t.Fatalf("family source at Start = %q, want %q (nothing resolved it)", st.FamilySource, intake.FamilySourceDefault)
	}
	f.admit(st.RunID)
	st = f.advance(st.TaskID)

	if st.OpenAskKind != intake.CardFamily {
		t.Fatalf("first card kind = %q, want %q (never a silent generic, R4/R5)", st.OpenAskKind, intake.CardFamily)
	}
	askID, card := f.openAsk(st.RunID)
	if card.Decision == nil {
		t.Fatal("family card carries no decision body")
	}
	if card.Decision.Summary == "" {
		t.Error("family card has no summary")
	}
	want := []string{
		string(intake.FamilySoftware), string(intake.FamilyResearch), string(intake.FamilyContent),
		string(intake.FamilyData), string(intake.FamilyChore), string(intake.FamilyGeneric),
	}
	if len(card.Decision.Choices) != len(want) {
		t.Fatalf("family card offers %d choices, want %d", len(card.Decision.Choices), len(want))
	}
	for i, c := range card.Decision.Choices {
		if c.Value != want[i] {
			t.Errorf("choice %d value = %q, want %q (the six family constants, one list two readers)", i, c.Value, want[i])
		}
		if c.Label == "" {
			t.Errorf("choice %q has no label — a surface would draw an unnamed button", c.Value)
		}
	}
	h := card.Decision.Help
	if h.What == "" || h.Wrong == "" || h.Recommend == "" {
		t.Errorf("family card help block incomplete: %+v (the 13.5 block)", h)
	}
	if f.runState(st.RunID) != run.StateParked {
		t.Errorf("run state = %q, want parked (gates wait, S06.1)", f.runState(st.RunID))
	}
	genBefore := generation(t, f, st.RunID)

	st = f.answer("u1", askID, intake.Answer{Choice: string(intake.FamilySoftware)})

	if got := askStatus(t, f, askID); got != "answered" {
		t.Errorf("family ask status = %q, want answered", got)
	}
	if st.Family != intake.FamilySoftware {
		t.Errorf("family after the answer = %q, want software", st.Family)
	}
	if st.FamilySource != intake.FamilySourceRequester {
		t.Errorf("family source = %q, want %q (attributed, never silent)", st.FamilySource, intake.FamilySourceRequester)
	}
	if st.TaxonomyID != "software" {
		t.Errorf("taxonomy after the swap = %q, want software", st.TaxonomyID)
	}
	if gen := generation(t, f, st.RunID); gen != genBefore+1 {
		t.Errorf("generation = %d, want %d (parked→running bumps once, the landed fence)", gen, genBefore+1)
	}
	var found bool
	for _, txt := range humanDecisionTexts(t, f, st.TaskID) {
		if strings.Contains(txt, "software") {
			found = true
		}
	}
	if !found {
		t.Errorf("no human-authored ledger decision records the chosen family: %v", humanDecisionTexts(t, f, st.TaskID))
	}

	// The SAME answer-drive asks the software questions next, with clearance
	// recomputed over the software set (the landed advance loop, §1.2).
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("card after the family answer = %q, want interview", st.OpenAskKind)
	}
	_, next := f.openAsk(st.RunID)
	ids := softwareSlotIDs(t)
	for _, q := range next.Questions {
		if !ids[q.ID] {
			t.Errorf("post-swap question %q is not a software slot", q.ID)
		}
	}
	tax := intake.SeedTaxonomies()[intake.FamilySoftware]
	// Nothing was answered yet, but the slots the software set settles without
	// asking resolved when the set was keyed, so the meter starts at their share
	// rather than at zero (P3-GF7 R3; harvest H10 — inferred slots are COUNTED).
	settled := map[string]bool{}
	for _, s := range tax.Slots {
		if s.Ask == intake.AskNever {
			settled[s.ID] = true
		}
	}
	if want := tax.Clearance(settled); st.Clearance != want {
		t.Errorf("clearance = %v, want %v (recomputed over the software set)", st.Clearance, want)
	}
}

// TestFamilyCardChoiceGenericExplicit (T5; R5/R6): a requester-chosen generic
// is ATTRIBUTED, a chosen family with no seeded question set keeps the true
// family and records the disclosed fallback, and a value outside the
// vocabulary is refused with the card left open.
func TestFamilyCardChoiceGenericExplicit(t *testing.T) {
	t.Run("explicit generic is attributed", func(t *testing.T) {
		f := newFix(t)
		f.p.Classifier = nil
		f.p.Registry = nil
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)
		askID, _ := f.openAsk(st.RunID)

		st = f.answer("u1", askID, intake.Answer{Choice: string(intake.FamilyGeneric)})
		if st.Family != intake.FamilyGeneric || st.TaxonomyID != "generic" {
			t.Errorf("family/taxonomy = %q/%q, want generic/generic", st.Family, st.TaxonomyID)
		}
		if st.FamilySource != intake.FamilySourceRequester {
			t.Errorf("family source = %q, want %q — a chosen generic is attributed, not silent (R5)",
				st.FamilySource, intake.FamilySourceRequester)
		}
		if st.OpenAskKind != intake.CardInterview {
			t.Fatalf("card after the answer = %q, want interview", st.OpenAskKind)
		}
		genericIDs := map[string]bool{}
		for _, s := range intake.SeedTaxonomies()[intake.FamilyGeneric].Slots {
			genericIDs[s.ID] = true
		}
		_, card := f.openAsk(st.RunID)
		for _, q := range card.Questions {
			if !genericIDs[q.ID] {
				t.Errorf("question %q is not a generic slot", q.ID)
			}
		}
	})

	// P3-RW-12 retarget: every vocabulary family now ships its own seeded set,
	// so this subtest's original premise — a family the seeds do not cover —
	// no longer exists in the shipping map. The DISCLOSED-fallback machinery
	// it guards has not gone anywhere: it is exercised against a custom
	// Taxonomies map missing a family, which is exactly the case it exists for
	// (an operator-supplied override set, or a family seeded later).
	t.Run("unseeded family keeps the true family and discloses the fallback", func(t *testing.T) {
		f := newFix(t)
		f.p.Classifier = nil
		f.p.Registry = nil
		custom := intake.SeedTaxonomies()
		delete(custom, intake.FamilyResearch)
		f.p.Taxonomies = custom
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)
		askID, _ := f.openAsk(st.RunID)

		st = f.answer("u1", askID, intake.Answer{Choice: string(intake.FamilyResearch)})
		if st.Family != intake.FamilyResearch {
			t.Errorf("family = %q, want research (the TRUE family stays durable, R6)", st.Family)
		}
		if st.TaxonomyID != "generic" {
			t.Errorf("taxonomy = %q, want generic (this map seeds no research set)", st.TaxonomyID)
		}
		var disclosed bool
		for _, txt := range platformDecisionTexts(t, f, st.TaskID) {
			if strings.Contains(txt, "research") && strings.Contains(txt, "generic") {
				disclosed = true
			}
		}
		if !disclosed {
			t.Errorf("the generic fallback was not RECORDED — a silent fallback survives (R6): %v",
				platformDecisionTexts(t, f, st.TaskID))
		}
	})

	t.Run("a value outside the vocabulary is refused", func(t *testing.T) {
		f := newFix(t)
		f.p.Classifier = nil
		f.p.Registry = nil
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)
		askID, _ := f.openAsk(st.RunID)

		raw, err := json.Marshal(intake.Answer{Choice: "bogus"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.p.Answer(context.Background(), "u1", askID, raw)
		if !errors.Is(err, intake.ErrBadAnswer) {
			t.Fatalf("Answer(bogus) error = %v, want ErrBadAnswer", err)
		}
		if !strings.Contains(err.Error(), string(intake.FamilySoftware)) {
			t.Errorf("refusal does not name the vocabulary it accepts: %v", err)
		}
		if got := askStatus(t, f, askID); got != "open" {
			t.Errorf("ask status after a refused answer = %q, want open", got)
		}
	})
}

// TestClassifierFamilyResolves (T6; OQ3 precedence, R5/R7): a VALID classifier
// family resolves and is attributed; a classifier ERROR fails closed to high
// AND leaves the family unresolved, so the question fires.
func TestClassifierFamilyResolves(t *testing.T) {
	t.Run("valid family resolves", func(t *testing.T) {
		f := newFix(t)
		f.p.Registry = nil
		f.class.prop = intake.TriageProposal{Family: intake.FamilySoftware, Tier: intake.TierStandard}
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)
		if st.Family != intake.FamilySoftware || st.FamilySource != intake.FamilySourceClassifier {
			t.Errorf("family/source = %q/%q, want software/%q", st.Family, st.FamilySource, intake.FamilySourceClassifier)
		}
		if st.OpenAskKind == intake.CardFamily {
			t.Error("the family question fired over a family the classifier resolved (OQ3)")
		}
	})

	t.Run("classifier error fails closed and asks", func(t *testing.T) {
		f := newFix(t)
		f.p.Registry = nil
		f.class.err = errors.New("stack absent")
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)
		if st.Tier != intake.TierHigh {
			t.Errorf("tier = %q, want high (fail closed, S06.2/R7)", st.Tier)
		}
		if st.FamilySource != intake.FamilySourceDefault {
			t.Errorf("family source = %q, want %q", st.FamilySource, intake.FamilySourceDefault)
		}
		if st.OpenAskKind != intake.CardFamily {
			t.Errorf("card kind = %q, want %q (an unresolved family is ASKED)", st.OpenAskKind, intake.CardFamily)
		}
	})
}

// TestBandNeverAsksFamily (T8; R4/S06.4): a trivial band task never gains a
// click — no card of any kind is issued, and its generic family is attributed
// to whatever actually resolved it.
func TestBandNeverAsksFamily(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil
	f.class.prop = intake.TriageProposal{
		Family: intake.FamilyGeneric, Tier: intake.TierTrivial, ReadOnly: true,
		Est: intake.Estimate{SizeClass: "XS", USD: 0.10, Known: true, Basis: "fake"},
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if !st.Band {
		t.Fatalf("task did not enter the zero-interaction band: tier=%q band=%v", st.Tier, st.Band)
	}
	if st.FamilySource != intake.FamilySourceClassifier {
		t.Errorf("family source = %q, want %q (the classifier resolved generic explicitly)",
			st.FamilySource, intake.FamilySourceClassifier)
	}
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("band phase = %q, want approved", st.Phase)
	}
	if kinds := cardKindsIssued(t, f, st.RunID); len(kinds) != 0 {
		t.Errorf("band task issued cards %v — the band never requires a click (S06.4)", kinds)
	}
	if st.Family != intake.FamilyGeneric {
		t.Errorf("band family = %q, want generic", st.Family)
	}
}
