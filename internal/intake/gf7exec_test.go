package intake_test

// gf7exec_test.go — the executor half of the P3-GF7 acceptance battery
// (P3/briefs/P3-GF7.md §9 T6–T11 + the two property marks P1/P2, and the §4
// per-slot matrix). Committed RED with the reds already in gf7red_test.go and
// green at the P3-GF7 implementation commit.
//
// Everything here compiles against the CURRENT type surface and fails only on
// v4 behavior that does not exist yet: the new slot vocabulary (ask posture,
// per-option effect, per-slot recommendation) is read through JSON rather than
// through Go fields, which is also how a surface will read it.
//
// Binding records: P3/design/b6-gate-operator-findings-r5-2026-08-23.md
// (§A per-slot verdicts, §C the seven hard rules, §D W2), harvest
// P3/design/w1-nexus-live-harvest-2026-08-27.md (H2/H3/H10/H18/H23),
// Spec S06.5/S06.6/S06.9, S09.10.

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf7NeverAsked are the v4 slots the platform settles itself and never asks:
// the three the operator killed as questions (r5 §D W2 — behavior and
// terminology invert into stated understanding, indices_ranges resolves
// internally) plus numerical_precision, ruled ask-never at OQ1 (same
// natively-handled band, fails C.2 the same way).
var gf7NeverAsked = map[string]bool{
	"behavior":            true,
	"terminology":         true,
	"indices_ranges":      true,
	"numerical_precision": true,
}

// gf7AskedSoftware is the v4 software set's ASKED slots with their weights:
// the nine survivors verbatim from v3 plus the two H23 additions.
var gf7AskedSoftware = map[string]int{
	"edge_cases": 10, "collection_semantics": 12, "comparison_rules": 12,
	"ordering_atomicity": 12, "output_format": 8, "units": 6,
	"technology_stack": 11, "assets_media": 10, "look_feel": 10,
	"language_locale": 9, "quality_bar": 8,
}

// ---- the v4 vocabulary, read the way a surface reads it ----

type gf7Option struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

type gf7SlotView struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	MustKnow    string      `json:"must_know"`
	Weight      int         `json:"weight"`
	Question    string      `json:"question"`
	Why         string      `json:"why"`
	Ask         string      `json:"ask"`
	Recommended string      `json:"recommended"`
	Options     []gf7Option `json:"options"`
}

type gf7TaxView struct {
	ID      string        `json:"id"`
	Version string        `json:"version"`
	Source  string        `json:"source"`
	Slots   []gf7SlotView `json:"slots"`
}

// gf7View renders a shipped question set through its own JSON — the exact
// bytes the governed file holds and LoadTaxonomy round-trips.
func gf7View(t *testing.T, fam intake.Family) gf7TaxView {
	t.Helper()
	raw, err := json.Marshal(intake.SeedTaxonomies()[fam])
	if err != nil {
		t.Fatalf("marshal %s seed: %v", fam, err)
	}
	var view gf7TaxView
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("decode %s seed: %v", fam, err)
	}
	return view
}

// ---- §4: the per-slot acceptance matrix ----

// TestGF7SoftwareV4MeetsThePerSlotMatrix (brief §4; r5 §C rules 2–5): every
// ASKED v4 software slot carries a plain purpose, 2–4 concrete options, an
// effect line on every option, and exactly one recommended default that names
// one of its own options; every NEVER-asked slot carries its planner
// instruction and ships no question at all.
func TestGF7SoftwareV4MeetsThePerSlotMatrix(t *testing.T) {
	soft := gf7View(t, intake.FamilySoftware)
	if soft.Version != "v4" {
		t.Fatalf("software seed version = %q, want v4 (the W2 revision, r5 §D)", soft.Version)
	}
	seen := map[string]bool{}
	for _, s := range soft.Slots {
		seen[s.ID] = true
		if s.MustKnow == "" {
			t.Errorf("%s states no MustKnow — the planner-read reason it is covered at all", s.ID)
		}
		if gf7NeverAsked[s.ID] {
			if s.Ask != "never" {
				t.Errorf("%s ask posture = %q, want %q — it is never delivered as a question (r5 §C rules 2+3)", s.ID, s.Ask, "never")
			}
			if s.Question != "" || len(s.Options) > 0 || s.Why != "" {
				t.Errorf("%s still carries asked-slot content (question=%q options=%d why=%q) — a never-asked slot ships none",
					s.ID, s.Question, len(s.Options), s.Why)
			}
			continue
		}
		wantWeight, asked := gf7AskedSoftware[s.ID]
		if !asked {
			t.Errorf("unexpected slot %q in the v4 software set", s.ID)
			continue
		}
		if s.Weight != wantWeight {
			t.Errorf("%s weight = %d, want %d", s.ID, s.Weight, wantWeight)
		}
		if s.Ask != "" {
			t.Errorf("%s ask posture = %q, want the asked default", s.ID, s.Ask)
		}
		if strings.TrimSpace(s.Question) == "" {
			t.Errorf("%s asks nothing", s.ID)
		}
		if strings.TrimSpace(s.Why) == "" {
			t.Errorf("%s carries no plain purpose line (C.4: what breaks if unanswered)", s.ID)
		}
		if len(s.Options) < 2 || len(s.Options) > 4 {
			t.Errorf("%s carries %d options, want 2–4 a non-programmer can pick between (C.4/C.5)", s.ID, len(s.Options))
		}
		recommended := 0
		for _, o := range s.Options {
			if strings.TrimSpace(o.Effect) == "" {
				t.Errorf("%s/%s carries no effect line — every option says what picking it does (C.4)", s.ID, o.Value)
			}
			if o.Value == s.Recommended {
				recommended++
			}
		}
		if recommended != 1 {
			t.Errorf("%s recommends %q, which names %d of its own options — want exactly one (C.4)", s.ID, s.Recommended, recommended)
		}
	}
	for id := range gf7AskedSoftware {
		if !seen[id] {
			t.Errorf("the v4 software set has no %q slot", id)
		}
	}
	for id := range gf7NeverAsked {
		if !seen[id] {
			t.Errorf("never-asked slot %q was dropped — killed slots KEEP their id and weight so coverage stays accounted (H10)", id)
		}
	}
}

// TestGF7OrderingAtomicityRecommendsSystemDecides (r5 §A round 1): a non-IT
// requester cannot answer this one, so the platform deciding and showing what
// it decided is the only honest default — first option AND recommended.
func TestGF7OrderingAtomicityRecommendsSystemDecides(t *testing.T) {
	soft := gf7View(t, intake.FamilySoftware)
	for _, s := range soft.Slots {
		if s.ID != "ordering_atomicity" {
			continue
		}
		if len(s.Options) == 0 || s.Options[0].Value != "planner_chooses" {
			t.Fatalf("ordering_atomicity does not lead with the planner-chooses arm: %+v", s.Options)
		}
		if s.Recommended != "planner_chooses" {
			t.Errorf("ordering_atomicity recommends %q, want the system-decides arm (r5 §A round 1)", s.Recommended)
		}
		return
	}
	t.Fatal("ordering_atomicity is missing — it is on the operator's KEEP list")
}

// TestGF7ComparisonRulesOptionsNameTheirReferent (brief R5; r5 §A round 1):
// the condemned option is gone and the question binds the order to a named
// surface, so no option is left whose referent the requester cannot tell.
func TestGF7ComparisonRulesOptionsNameTheirReferent(t *testing.T) {
	soft := gf7View(t, intake.FamilySoftware)
	for _, s := range soft.Slots {
		if s.ID != "comparison_rules" {
			continue
		}
		const condemned = "Closest to what the person was looking for"
		for _, o := range s.Options {
			if o.Label == condemned {
				t.Errorf("the referent-free option survives: %q (r5 §A round 1 — unanswerable)", o.Label)
			}
		}
		if !strings.Contains(strings.ToLower(s.Question), "search") &&
			!strings.Contains(strings.ToLower(s.Question), "filter") {
			t.Errorf("comparison_rules names no surface the order governs: %q", s.Question)
		}
		return
	}
	t.Fatal("comparison_rules is missing — it is a FIX, not a kill")
}

// TestGF7NewSlotsRecordTheirReasonedWeights (brief R8): the two H23 additions
// enter with weights that are REASONED and say so — reasoning never outranks
// the measured 12s.
func TestGF7NewSlotsRecordTheirReasonedWeights(t *testing.T) {
	soft := gf7View(t, intake.FamilySoftware)
	byID := map[string]gf7SlotView{}
	for _, s := range soft.Slots {
		byID[s.ID] = s
	}
	for _, id := range []string{"language_locale", "quality_bar"} {
		s, ok := byID[id]
		if !ok {
			t.Errorf("the v4 software set has no %q slot (W1 H23 + §D.1)", id)
			continue
		}
		if s.Weight >= byID["collection_semantics"].Weight {
			t.Errorf("%s (%d) outweighs the measured collection_semantics (%d) — reasoning does not outrank measurement",
				id, s.Weight, byID["collection_semantics"].Weight)
		}
		if !strings.Contains(soft.Source, id) {
			t.Errorf("the Source records no weight reasoning for the new %q slot", id)
		}
	}
	if !strings.Contains(soft.Source, "P3-GF7") || !strings.Contains(soft.Source, "2026-08-27") {
		t.Errorf("the v4 Source does not say which packet drafted it and when (S06.5): %q", soft.Source)
	}
	if !strings.Contains(strings.ToLower(soft.Source), "opus") {
		t.Errorf("the v4 Source names no drafting model (S06.5 strongest-available-frontier-model rule): %q", soft.Source)
	}
}

// ---- T10: vocabulary round-trip and Validate ----

func gf7WriteTaxonomy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tax.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGF7VocabularyRoundTripsAndValidates (brief §9 T10; R7/R2): the v4
// vocabulary survives the strict LoadTaxonomy decode both ways, and Validate
// refuses the three shapes that would make a card unrenderable.
func TestGF7VocabularyRoundTripsAndValidates(t *testing.T) {
	raw, err := json.MarshalIndent(intake.SeedTaxonomies()[intake.FamilySoftware], "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := intake.LoadTaxonomy(gf7WriteTaxonomy(t, string(raw)+"\n"))
	if err != nil {
		t.Fatalf("the v4 software set does not survive its own strict decode: %v", err)
	}
	if loaded.Version != "v4" {
		t.Errorf("round-tripped version = %q, want v4", loaded.Version)
	}
	var back gf7TaxView
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	carriesAsk, carriesEffect, carriesRecommended := false, false, false
	for _, s := range back.Slots {
		carriesAsk = carriesAsk || s.Ask == "never"
		carriesRecommended = carriesRecommended || s.Recommended != ""
		for _, o := range s.Options {
			carriesEffect = carriesEffect || o.Effect != ""
		}
	}
	if !carriesAsk || !carriesEffect || !carriesRecommended {
		t.Errorf("the round-tripped file loses vocabulary: ask=%v effect=%v recommended=%v",
			carriesAsk, carriesEffect, carriesRecommended)
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "recommended names no option",
			body: `{"id":"t","family":"software","version":"v4","source":"s","slots":[
			  {"id":"a","name":"A","must_know":"m","weight":1,"question":"q","recommended":"nope",
			   "options":[{"label":"L1","value":"v1","effect":"e1"},{"label":"L2","value":"v2","effect":"e2"}]}]}`,
			want: "recommend",
		},
		{
			name: "a recommendation with no options to name",
			body: `{"id":"t","family":"software","version":"v4","source":"s","slots":[
			  {"id":"a","name":"A","must_know":"m","weight":1,"question":"q","recommended":"v1"}]}`,
			want: "recommend",
		},
		{
			name: "an ask-never slot with no must-know",
			body: `{"id":"t","family":"software","version":"v4","source":"s","slots":[
			  {"id":"a","name":"A","weight":1,"ask":"never"}]}`,
			want: "must",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := intake.LoadTaxonomy(gf7WriteTaxonomy(t, tc.body))
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("refusal does not name the defect (%q): %v", tc.want, err)
			}
		})
	}

	// And the valid ask-never shape LOADS: no question, no options, no why.
	ok := `{"id":"t","family":"software","version":"v4","source":"s","slots":[
	  {"id":"a","name":"A","must_know":"the platform settles this itself","weight":1,"ask":"never"},
	  {"id":"b","name":"B","must_know":"m","weight":2,"question":"q","why":"w","recommended":"v1",
	   "options":[{"label":"L1","value":"v1","effect":"e1"},{"label":"L2","value":"v2","effect":"e2"}]}]}`
	if _, err := intake.LoadTaxonomy(gf7WriteTaxonomy(t, ok)); err != nil {
		t.Errorf("a well-formed v4 set was refused: %v", err)
	}
}

// ---- T7: the family guess is correctable from an interview card ----

// TestGF7InterviewAnswerCorrectsTheFamilyGuess (brief R9; W1 H18/H17): a
// requester who sees the guess on the card can correct it in the same answer.
// The question set re-scopes, every resolution already given is RETAINED (no
// silent loss), the source becomes the requester, and the platform records the
// switch as a decision.
func TestGF7InterviewAnswerCorrectsTheFamilyGuess(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.Family != intake.FamilySoftware || st.FamilySource != intake.FamilySourceClassifier {
		t.Fatalf("fixture drift: family=%s source=%s", st.Family, st.FamilySource)
	}
	askID, card := f.openAsk(st.RunID)
	answers := []map[string]any{}
	for _, q := range card.Questions {
		answers = append(answers, map[string]any{"id": q.ID, "value": "answered: " + q.ID})
	}
	before := len(st.Resolutions)

	st = f.answer("u1", askID, map[string]any{"answers": answers, "family": "research"})

	if st.Family != intake.FamilyResearch {
		t.Errorf("family = %q, want the requester's correction %q (H18)", st.Family, intake.FamilyResearch)
	}
	if st.FamilySource != intake.FamilySourceRequester {
		t.Errorf("family source = %q, want %q", st.FamilySource, intake.FamilySourceRequester)
	}
	if st.TaxonomyID != "research" {
		t.Errorf("taxonomy id = %q, want the corrected family's set", st.TaxonomyID)
	}
	if len(st.Resolutions) < before+len(answers) {
		t.Errorf("resolutions = %d, want at least %d retained — a family switch loses nothing (H17 REJECT)",
			len(st.Resolutions), before+len(answers))
	}
	found := false
	for _, txt := range humanDecisionTexts(t, f, st.TaskID) {
		if strings.Contains(txt, "research") {
			found = true
		}
	}
	if !found {
		t.Errorf("no human-authored decision records the family correction: %v", humanDecisionTexts(t, f, st.TaskID))
	}
	if next := cardKindsIssued(t, f, st.RunID); next[0] == string(intake.CardFamily) {
		t.Error("the classifier path issued a family card — RW-13 zero-touch must not regress")
	}
}

// TestGF7BadFamilyCorrectionIsRefused: the interview answer validates the
// correction against the SAME vocabulary the family card offers, and a refused
// body changes nothing.
func TestGF7BadFamilyCorrectionIsRefused(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)

	raw, err := json.Marshal(map[string]any{"family": "wizardry"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.Answer(context.Background(), "u1", askID, raw); err == nil {
		t.Fatal("an unknown family was accepted")
	}
	after, err := f.p.LoadState(context.Background(), st.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Family != intake.FamilySoftware {
		t.Errorf("a refused correction moved the family to %q", after.Family)
	}
}

// ---- T8: the re-interview review card ----

// TestGF7ReviewCardOmitsTheNeverAskedSlots (brief R2, OQ5): a re-interview is
// still a question card, so it must not present the guess-my-misread question
// either. The never-asked slots' current resolution rides the understood recap
// instead, and Assume still reaches them.
func TestGF7ReviewCardOmitsTheNeverAskedSlots(t *testing.T) {
	f := newFix(t)
	askID, st := gf3ToApproval(t, f)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionReInterview})
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("after Re-interview: %q, want the review card", st.OpenAskKind)
	}
	reviewID, card := f.openAsk(st.RunID)
	for _, q := range card.Questions {
		if gf7NeverAsked[q.ID] {
			t.Errorf("the review card asks %q — a never-asked slot is never a question on ANY card kind (R2, OQ5)", q.ID)
		}
	}
	if card.Understood == nil {
		t.Fatal("the review card carries no understanding block")
	}
	recap := map[string]bool{}
	for _, item := range card.Understood.Items {
		recap[item.SlotID] = true
	}
	for id := range gf7NeverAsked {
		if !recap[id] {
			t.Errorf("%q is neither asked nor recapped — its resolution must ride the understood block (R2)", id)
		}
	}

	// Correction stays available: Assume reaches a never-asked slot.
	st = f.answer("u1", reviewID, intake.Answer{
		Assume: []intake.SlotAnswer{{ID: "terminology", Value: "compatibility means which cars a part fits"}},
	})
	for _, r := range st.Resolutions {
		if r.SlotID == "terminology" {
			if r.Assumption != "compatibility means which cars a part fits" {
				t.Errorf("the assumed correction did not land: %+v", r)
			}
			return
		}
	}
	t.Error("Assume on a never-asked slot reached nothing")
}

// ---- T9: a skip and a system resolution are different things ----

// TestGF7SkipAndSystemResolutionAreDistinguishable (brief R3; r5 §A round 1):
// the operator accepted "you skipped this one" for a question they were ASKED.
// A slot they were never asked must not claim they skipped it — and the record
// must say which is which without reading the prose.
func TestGF7SkipAndSystemResolutionAreDistinguishable(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	askID, card := f.openAsk(st.RunID)
	skipped := card.Questions[0].ID
	st = f.answer("u1", askID, intake.Answer{Answers: []intake.SlotAnswer{{ID: skipped, Skip: true}}})

	byID := map[string]intake.SlotResolution{}
	for _, r := range st.Resolutions {
		byID[r.SlotID] = r
	}
	skip, ok := byID[skipped]
	if !ok {
		t.Fatalf("the skipped slot %q has no resolution", skipped)
	}
	if !strings.Contains(skip.Assumption, "skipped") {
		t.Errorf("the skip prose no longer says the requester skipped it: %q", skip.Assumption)
	}
	for id := range gf7NeverAsked {
		sys, ok := byID[id]
		if !ok {
			t.Errorf("never-asked slot %q has no resolution at interview entry (R3)", id)
			continue
		}
		if sys.How != intake.ResolvedAssumption {
			t.Errorf("%q resolved as %q, want an assumption (S06.5 third arm)", id, sys.How)
		}
		if strings.Contains(sys.Assumption, "skipped") {
			t.Errorf("%q claims the requester skipped a question they were never asked: %q", id, sys.Assumption)
		}
		if strings.TrimSpace(sys.Assumption) == "" {
			t.Errorf("%q resolved silently — an inferred slot is a CONFIRMABLE assumption, never silent state (H10)", id)
		}
	}

	// The marker, not the prose, is what a surface reads.
	var payload string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq DESC LIMIT 1`,
		st.RunID, intake.EventState).Scan(&payload); err != nil {
		t.Fatalf("read state event: %v", err)
	}
	var recorded struct {
		Resolutions []struct {
			SlotID string `json:"slot_id"`
			Via    string `json:"via"`
		} `json:"resolutions"`
	}
	if err := json.Unmarshal([]byte(payload), &recorded); err != nil {
		t.Fatal(err)
	}
	via := map[string]string{}
	for _, r := range recorded.Resolutions {
		via[r.SlotID] = r.Via
	}
	if via[skipped] != "" {
		t.Errorf("a requester skip is marked %q — the marker names the PLATFORM's own resolutions", via[skipped])
	}
	for id := range gf7NeverAsked {
		if via[id] != "system" {
			t.Errorf("%q is recorded via %q, want %q — a system resolution must be distinguishable from a skip on the record (R3)",
				id, via[id], "system")
		}
	}
}

// ---- T11: the utility seat folds on the v4 set ----

// TestGF7SeatFoldsOnTheV4Set (brief R5): per-goal binding still rides the
// EXISTING suggestion seam on the new ids, under the same containment — an id
// nobody asked reaches nothing, and a suggested option naming no option of its
// question is dropped while the text survives.
func TestGF7SeatFoldsOnTheV4Set(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	cmp := soft.Slot("comparison_rules")
	if cmp == nil || len(cmp.Options) == 0 {
		t.Fatal("comparison_rules carries no options to bind a suggestion to")
	}
	real := cmp.Options[0].Value
	f.p.Phraser = gf3Suggesting(func(_ intake.PhraseInput, res *intake.PhraseResult) {
		res.SuggestedOptions = map[string]string{
			"comparison_rules": real,
			"behavior":         "smuggled", // never asked at v4 — must reach nothing
		}
	})
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	_, card := f.openAsk(st.RunID)

	sawComparison := false
	for _, q := range card.Questions {
		if gf7NeverAsked[q.ID] {
			t.Errorf("a never-asked slot reached the card through the seat: %q", q.ID)
		}
		if q.Suggested != "SUGGEST-"+q.ID {
			t.Errorf("%s suggestion = %q, want the seat's own text", q.ID, q.Suggested)
		}
		if q.ID == "comparison_rules" {
			sawComparison = true
			if q.SuggestedOption != real {
				t.Errorf("comparison_rules suggested_option = %q, want the existing option %q", q.SuggestedOption, real)
			}
		}
	}
	if !sawComparison {
		t.Fatal("comparison_rules was not on the first v4 card — it is a weight-12 slot")
	}
}

// ---- P1: delivery never asks the unaskable, and never ships an empty card ----

// TestGF7PropertyDeliveryIsAskableAndNonEmpty (brief §9 P1): over randomly
// generated resolution states — every question on every card independently
// answered, skipped, or left alone — the interview never delivers a never-asked
// slot and never issues a card with no questions on it. The second half is the
// askable-exhaustion trap: a set whose only unresolved slots are never-asked
// ones must stop interviewing rather than ship an empty card.
func TestGF7PropertyDeliveryIsAskableAndNonEmpty(t *testing.T) {
	rng := rand.New(rand.NewSource(20260827))
	for iter := 0; iter < 16; iter++ {
		f := newFix(t)
		f.p.Registry = nil
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)

		for round := 0; round < 8; round++ {
			if st.OpenAskID == "" || st.OpenAskKind != intake.CardInterview {
				break
			}
			askID, card := f.openAsk(st.RunID)
			if len(card.Questions) == 0 {
				t.Fatalf("iteration %d round %d: an interview card shipped with no questions (askable exhaustion, P1)", iter, round)
			}
			var answers []intake.SlotAnswer
			for _, q := range card.Questions {
				if gf7NeverAsked[q.ID] {
					t.Fatalf("iteration %d round %d: delivery emitted the never-asked slot %q", iter, round, q.ID)
				}
				switch rng.Intn(3) {
				case 0:
					answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
				case 1:
					answers = append(answers, intake.SlotAnswer{ID: q.ID, Skip: true})
				}
			}
			st = f.answer("u1", askID, intake.Answer{Answers: answers})
		}
	}
}

// ---- P2: Clearance with never-asked mass ----

// TestGF7PropertyClearanceCountsTheNeverAskedMass (brief §9 P2): Clearance is
// monotone under resolution, reads 100 only when every slot is resolved, and
// its value at interview entry is exactly the never-asked weight share — the
// H10 rule that inferred slots are COUNTED by the meter, never silently filled.
func TestGF7PropertyClearanceCountsTheNeverAskedMass(t *testing.T) {
	tax := intake.SeedTaxonomies()[intake.FamilySoftware]
	n := len(tax.Slots)
	if n > 20 {
		t.Fatalf("the exhaustive enumeration below assumes a small set, got %d slots", n)
	}
	total, never := 0, 0
	for _, s := range tax.Slots {
		total += s.Weight
		if gf7NeverAsked[s.ID] {
			never += s.Weight
		}
	}
	resolved := make(map[string]bool, n)
	for mask := 0; mask < 1<<n; mask++ {
		for i, s := range tax.Slots {
			resolved[s.ID] = mask&(1<<i) != 0
		}
		got := tax.Clearance(resolved)
		if (got == 100) != (mask == 1<<n-1) {
			t.Fatalf("mask %d: clearance %v — 100 iff every slot is resolved", mask, got)
		}
		for i, s := range tax.Slots {
			if mask&(1<<i) != 0 {
				continue
			}
			resolved[s.ID] = true
			if next := tax.Clearance(resolved); next < got {
				t.Fatalf("mask %d: resolving %q LOWERED clearance %v → %v", mask, s.ID, got, next)
			}
			resolved[s.ID] = false
		}
	}
	entry := make(map[string]bool, len(gf7NeverAsked))
	for id := range gf7NeverAsked {
		entry[id] = true
	}
	want := 100 * float64(never) / float64(total)
	if got := tax.Clearance(entry); got != want {
		t.Errorf("the never-asked share reads %v, want %v", got, want)
	}

	// And the pipeline's own entry value is that share, before anything is asked.
	f := newFix(t)
	f.p.Registry = nil
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.Clearance != want {
		t.Errorf("interview entry clearance = %v, want the never-asked share %v (R3: counted from the start)", st.Clearance, want)
	}
}
