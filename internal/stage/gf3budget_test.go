package stage

// gf3budget_test.go — the P3-GF3-BE1 T6b leg: the phrase seat's length budget
// provably covers the biggest card the pipeline can now build.
//
// PH-1 is why this is a test and not a comment. A cap that was right for the
// card it was measured on went wrong the moment the card changed, and the
// failure was silent: the JSON stopped mid-object, every question fell back to
// its taxonomy wording, and nothing anywhere said so. This packet grows both
// dimensions at once — a suggestion per question, and a review card carrying
// every slot of a family instead of four — so the budget is now derived from
// the question count, and the derivation is pinned here.

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// The measured basis (P3/design/ph1-phrase-fallback-diagnosis-2026-08-17.md,
// carried in the phraseMaxTokens comment): a working four-question card's JSON
// is 350 to 500 output tokens with the think phase off. That is at most ~125
// tokens per question of rewording, plus the fixed reason and summary region.
//
// GF3 adds, per question, a one-line suggestion and an option value. A
// suggestion is a sentence like the rewording it sits beside, so the modelled
// per-question cost doubles and takes a little more for the option token.
const (
	modelledFixedTokens       = 150 // reason + summary + object scaffolding
	modelledPerQuestionTokens = 260 // rewording + suggestion + option value
	// modelledHeadroomFactor is the same discipline the live leg asserts with
	// liveCapHeadroomPct: a reply may use at most half its budget, because a
	// call that fits exactly is indistinguishable from one that fits by luck.
	modelledHeadroomFactor = 2
)

func modelledOutput(questions int) int64 {
	return int64(modelledFixedTokens + questions*modelledPerQuestionTokens)
}

// PhraseBudgetForTest exposes the derivation to the external test package (the
// export_test.go pattern), so the opt-in live leg can measure a real call
// against the budget it was actually given.
func PhraseBudgetForTest(questions int) int64 { return phraseBudget(questions) }

// TestPhraseBudgetKeepsTheMeasuredDeliveryCardUnchanged: a full S06.5 delivery
// card (up to 4 questions) asks the engine for exactly the budget PH-1
// measured. This packet grows the budget for bigger cards; it does not re-open
// the number that was measured for this one.
func TestPhraseBudgetKeepsTheMeasuredDeliveryCardUnchanged(t *testing.T) {
	const maxQuestionsPerCard = 4 // Spec S06.5, ratified text
	if got := phraseBudget(maxQuestionsPerCard); got != phraseMaxTokens {
		t.Errorf("a full delivery card asks for %d tokens, want the measured %d unchanged", got, phraseMaxTokens)
	}
	for n := 1; n <= maxQuestionsPerCard; n++ {
		if got := phraseBudget(n); got != phraseMaxTokens {
			t.Errorf("a %d-question card asks for %d, want the measured floor %d — nothing here may shrink a call that worked",
				n, got, phraseMaxTokens)
		}
	}
}

// TestPhraseBudgetCoversTheReviewCard (T6b): the S06.9 review card carries
// EVERY slot of its family, so the budget has to cover the largest seeded set
// with the same headroom the live leg demands of a delivery card. A seed set
// that grows past what the budget can carry fails here rather than on a
// requester's card.
func TestPhraseBudgetCoversTheReviewCard(t *testing.T) {
	largest, where := 0, intake.Family("")
	for fam, tax := range intake.SeedTaxonomies() {
		if len(tax.Slots) > largest {
			largest, where = len(tax.Slots), fam
		}
	}
	if largest == 0 {
		t.Fatal("no seeded question sets")
	}
	budget := phraseBudget(largest)
	need := modelledOutput(largest) * modelledHeadroomFactor
	if budget < need {
		t.Errorf("the review card for %s (%d slots) budgets %d tokens; the modelled reply is %d and needs %dx headroom (%d) — "+
			"the card would truncate and every question would silently fall back to its taxonomy wording (PH-1)",
			where, largest, budget, modelledOutput(largest), modelledHeadroomFactor, need)
	}
	t.Logf("largest seeded set: %s with %d slots — budget %d against a modelled %d (%.1fx headroom)",
		where, largest, budget, modelledOutput(largest), float64(budget)/float64(modelledOutput(largest)))
}

// TestPhraseBudgetGrowsWithTheCard: the budget is a line in the question count,
// not a constant with an exception. A card one question bigger than the largest
// seeded set is already provisioned, which is what keeps a future family seed
// from arriving at a cap nobody re-derived.
func TestPhraseBudgetGrowsWithTheCard(t *testing.T) {
	prev := phraseBudget(4)
	for n := 5; n <= 40; n++ {
		got := phraseBudget(n)
		if got <= prev {
			t.Fatalf("budget for %d questions = %d, not above the %d for %d — the line stopped growing", n, got, prev, n-1)
		}
		if got < modelledOutput(n)*modelledHeadroomFactor {
			t.Fatalf("budget for %d questions = %d, below the modelled reply plus headroom %d",
				n, got, modelledOutput(n)*modelledHeadroomFactor)
		}
		prev = got
	}
}
