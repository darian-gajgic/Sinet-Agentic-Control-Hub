package intake_test

// v41_test.go — P3-V41 acceptance tests (brief P3/briefs/P3-V41.md §9; Spec
// S06.5, S06.4). Committed RED by the grounding agent (Amendment A) and green at
// the implementation commit.
//
// The headline these assert: the software question set's quality_bar slot
// weighs 12 at v4.1 — equal to, never above, the three measured 12s — so at the
// unchanged ⚙ clearance floors it is on the FIRST card at every tier; the
// Clearance meter keeps its S06.5 law over the new total; the v4 record states
// that the operator ratified it on 2026-09-17; and nothing but the weight moved.

import (
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// v41Software is the v4.1 software set: ids, weights, order and ask posture.
// Every row but quality_bar is VERBATIM from v4 (the v3 prefix, then the two
// GF7 additions appended); quality_bar moves 8 → 12 on the operator's A2
// ruling and nothing else moves.
var v41Software = []struct {
	id    string
	w     int
	never bool
}{
	{"behavior", 10, true}, {"terminology", 10, true}, {"edge_cases", 10, false},
	{"collection_semantics", 12, false}, {"comparison_rules", 12, false}, {"ordering_atomicity", 12, false},
	{"indices_ranges", 8, true}, {"output_format", 8, false}, {"units", 6, false}, {"numerical_precision", 6, true},
	{"technology_stack", 11, false}, {"assets_media", 10, false}, {"look_feel", 10, false},
	{"language_locale", 9, false}, {"quality_bar", 12, false},
}

// v41Total is the v4.1 software set's total weight: 142 at v4, +4 for the
// quality_bar move. v41Entry is the weight resolved at interview ENTRY — the
// four never-asked slots (10+10+8+6), unchanged.
const (
	v41Total = 146
	v41Entry = 34
)

// TestV41SeedWeightAndVersion (brief R1/R2): the seed is v4.1, quality_bar
// weighs 12, and every other slot keeps its id, weight, order and posture. RED
// at v4: version "v4", weight 8, total 142.
func TestV41SeedWeightAndVersion(t *testing.T) {
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	if soft.Version != "v4.1" {
		t.Errorf("software seed version = %q, want %q (gate item A2 option (a), 2026-09-17)", soft.Version, "v4.1")
	}
	if len(soft.Slots) != len(v41Software) {
		t.Fatalf("software carries %d slots, want %d — the count is verbatim from v4", len(soft.Slots), len(v41Software))
	}
	total := 0
	for i, w := range v41Software {
		s := soft.Slots[i]
		total += s.Weight
		if s.ID != w.id || s.Weight != w.w {
			t.Errorf("slot %d = %s/%d, want %s/%d — only quality_bar moves, and only its weight", i, s.ID, s.Weight, w.id, w.w)
		}
		if never := s.Ask == intake.AskNever; never != w.never {
			t.Errorf("slot %s ask posture = %q, want never=%v (verbatim from v4)", s.ID, s.Ask, w.never)
		}
	}
	if total != v41Total {
		t.Errorf("total weight = %d, want %d (142 at v4 + 4 for quality_bar 8 → 12)", total, v41Total)
	}
	// 12 EQUALS the measured 12s; it never outranks them (the v4 record's rule,
	// restated by A2).
	qb := soft.Slot("quality_bar")
	if qb == nil {
		t.Fatal("no quality_bar slot")
	}
	for _, id := range []string{"collection_semantics", "comparison_rules", "ordering_atomicity"} {
		if qb.Weight > soft.Slot(id).Weight {
			t.Errorf("quality_bar (%d) outranks the measured %s (%d) — 12 equals, it does not outrank", qb.Weight, id, soft.Slot(id).Weight)
		}
	}
}

// v41Walk drives a real interview through the pipeline at the given tier and
// returns the question ids of every card in delivery order plus the Clearance
// each card was issued at, then the state's Clearance when the interview
// stopped. Nothing is read off the seed slice: the cards come from the ask
// rows the pipeline wrote.
func v41Walk(t *testing.T, tier intake.Tier) (cards [][]string, atIssue []float64, floor float64, final float64) {
	t.Helper()
	f := newFix(t)
	f.p.Registry = nil
	f.class.prop.Tier = tier
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	for round := 0; round < 8; round++ {
		if st.OpenAskID == "" || st.OpenAskKind != intake.CardInterview {
			break
		}
		askID, card := f.openAsk(st.RunID)
		if round == 0 {
			floor = card.ClearanceFloor
		}
		var ids []string
		var answers []intake.SlotAnswer
		for _, q := range card.Questions {
			ids = append(ids, q.ID)
			answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
		}
		cards = append(cards, ids)
		atIssue = append(atIssue, card.Clearance)
		st = f.answer("u1", askID, intake.Answer{Answers: answers})
	}
	return cards, atIssue, floor, st.Clearance
}

func v41Pct(resolved int) float64 { return 100 * float64(resolved) / float64(v41Total) }

// TestV41QualityBarOnTheFirstCardAtEveryTier (brief R3; S06.5 highest-weight-
// first delivery, up to 4 per card; S06.4 floors 60/75/90): at v4.1 the first
// card at every tier is the four 12s — the three measured ones in taxonomy
// order, then quality_bar — and the arithmetic that follows is exactly the
// brief §4 table: entry 34/146 = 23.3 → 82/146 = 56.2 after card 1 (below every
// floor, so every tier issues card 2) → 123/146 = 84.2 after card 2 (low and
// standard stop) → 146/146 after card 3 (high stops). RED at v4: card 1 is the
// three 12s plus technology_stack, and standard never reaches quality_bar.
func TestV41QualityBarOnTheFirstCardAtEveryTier(t *testing.T) {
	card1 := []string{"collection_semantics", "comparison_rules", "ordering_atomicity", "quality_bar"}
	card2 := []string{"technology_stack", "edge_cases", "assets_media", "look_feel"}
	card3 := []string{"language_locale", "output_format", "units"}
	cases := []struct {
		tier  intake.Tier
		floor float64
		cards [][]string
	}{
		{intake.TierLow, 60, [][]string{card1, card2}},
		{intake.TierStandard, 75, [][]string{card1, card2}},
		{intake.TierHigh, 90, [][]string{card1, card2, card3}},
	}
	for _, c := range cases {
		t.Run(string(c.tier), func(t *testing.T) {
			cards, atIssue, floor, final := v41Walk(t, c.tier)
			if floor != c.floor {
				t.Fatalf("clearance floor served on the first card = %v, want the ⚙ default %v (a floor change is S18 — STOP)", floor, c.floor)
			}
			if !reflect.DeepEqual(cards, c.cards) {
				t.Fatalf("cards = %v, want %v", cards, c.cards)
			}
			// The Clearance each card was issued at, and where the interview
			// stopped: the S06.5 law over the v4.1 total.
			wantAt := []float64{v41Pct(v41Entry), v41Pct(v41Entry + 48), v41Pct(v41Entry + 48 + 41)}
			for i := range cards {
				if math.Abs(atIssue[i]-wantAt[i]) > 1e-9 {
					t.Errorf("card %d issued at clearance %.4f, want %.4f", i+1, atIssue[i], wantAt[i])
				}
			}
			wantFinal := wantAt[len(cards)]
			if len(cards) == 3 {
				wantFinal = 100
			}
			if math.Abs(final-wantFinal) > 1e-9 {
				t.Errorf("clearance when the interview stopped = %.4f, want %.4f", final, wantFinal)
			}
			if final < c.floor {
				t.Errorf("interview stopped at %.2f below the %s floor %v", final, c.tier, c.floor)
			}
		})
	}
}

// TestV41ClearanceProperty (brief R4; S06.5 "Clearance = 100 × resolved weight
// / total weight"): over random subsets of the v4.1 set, the meter is exactly
// that ratio over the new total. RED at v4: the total is 142.
func TestV41ClearanceProperty(t *testing.T) {
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	total := 0
	for _, s := range soft.Slots {
		total += s.Weight
	}
	if total != v41Total {
		t.Fatalf("total weight = %d, want %d", total, v41Total)
	}
	rng := rand.New(rand.NewSource(20260917))
	for iter := 0; iter < 256; iter++ {
		resolved := map[string]bool{}
		sum := 0
		for _, s := range soft.Slots {
			if rng.Intn(2) == 1 {
				resolved[s.ID] = true
				sum += s.Weight
			}
		}
		want := 100 * float64(sum) / float64(v41Total)
		if got := soft.Clearance(resolved); math.Abs(got-want) > 1e-9 {
			t.Fatalf("iter %d: Clearance(%d resolved) = %.6f, want %.6f", iter, len(resolved), got, want)
		}
	}
	if got := soft.Clearance(nil); got != 0 {
		t.Errorf("Clearance(nil) = %v, want 0", got)
	}
}

// TestV41RecordStatesRatified (brief R5/R6): the seed's Source — the v4
// drafting record — no longer says ratification is PENDING; it says the
// operator RATIFIED v4 on 2026-09-17 in the operator's own words, names the
// gate record as the ratification object, and the appended v4.1 record names
// A2's evidence (the webshop FAIL) and its reasoning (12 equals, never
// outranks). The earlier chain stays intact. RED at v4: PENDING.
func TestV41RecordStatesRatified(t *testing.T) {
	src := intake.SeedTaxonomies()[intake.FamilySoftware].Source
	if strings.Contains(src, "ratification PENDING") {
		t.Errorf("the v4 record still says ratification is pending: %q", src)
	}
	for _, want := range []string{
		"RATIFIED", "2026-09-17", "rework-sitting-gate.md",
		"ok for now, will need some refinement later", // A1, the operator's words
		"P3-V41", "quality_bar", "webshop", "equals", // A2: the packet, the slot, the evidence, the rule
		// the chain before it, untouched
		"P3-GF7", "2026-08-27", "P3-GF3-BE1", "P3-RW-12", "2607.00711",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("software Source is missing %q", want)
		}
	}
}

// TestV41QualityBarCopyIsByteUnchanged (brief R2): the slot's purpose,
// question, why-line, recommended default and options are exactly what v4
// shipped — only the weight moves. GREEN at v4 and must stay green; it is the
// guard against a redraft riding in on a data change.
func TestV41QualityBarCopyIsByteUnchanged(t *testing.T) {
	got := intake.SeedTaxonomies()[intake.FamilySoftware].Slot("quality_bar")
	if got == nil {
		t.Fatal("no quality_bar slot")
	}
	want := intake.Slot{
		ID: "quality_bar", Name: "What finished has to pass", Weight: got.Weight,
		MustKnow: "What the finished work must pass to count as done — feature-correctness alone, or a stated polish, performance or " +
			"accessibility bar — is unstated. It decides what verification gates on: write the acceptance criteria against this " +
			"answer. Precise form: name the checks the deliverable must pass and whether any of them are measured rather than judged.",
		Question:    "What does the finished thing have to pass before you would call it done?",
		Why:         "This is what the work is checked against at the end, so it decides what gets built along the way — a bar added afterwards means going back.",
		Recommended: "works_and_polished",
		Options: []intake.Option{
			{
				Label: "Everything I asked for works", Value: "feature_correct",
				Effect: "Checking asks one question per thing you asked for: does it do that? Quickest route to something usable, and nothing is judged on looks, speed or accessibility.",
			},
			{
				Label: "Everything works, and it looks and behaves properly", Value: "works_and_polished",
				Effect: "On top of the feature checks, the finished thing is judged on readable text and contrast, nothing jumping around as it " +
					"loads, and no errors running underneath. Recommended: it catches what you would notice in the first minute of using it, without a formal audit.",
			},
			{
				Label: "It has to pass a named standard, and I will say which", Value: "audited",
				Effect: "The standard you name becomes part of the acceptance criteria and is measured on the built thing rather than assumed. " +
					"Slower, and it can send back work that already does everything you asked for.",
			},
		},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("quality_bar copy changed — only the weight may move:\n got %+v\nwant %+v", *got, want)
	}
}
