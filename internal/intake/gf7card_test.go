package intake_test

// gf7card_test.go — what the v4 vocabulary has to REACH for a surface to render
// the operator's bar (P3-GF7 R7/R9). The taxonomy carrying a recommendation and
// per-option effects is only half the requirement: a card that drops them on the
// way out leaves GF9 with nothing to draw, and the drop is invisible to every
// test that reads the seed instead of the ask row.

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// TestGF7CardsServeTheRecommendationAndEffects (R7): every asked question on an
// issued card carries its authored recommended default and an effect line on
// every option, read back from the durable ASK ROW rather than an in-memory
// struct.
func TestGF7CardsServeTheRecommendationAndEffects(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)

	_, card := f.openAsk(st.RunID)
	if len(card.Questions) == 0 {
		t.Fatal("the first interview card carries no questions")
	}
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	for _, q := range card.Questions {
		slot := soft.Slot(q.ID)
		if slot == nil {
			t.Fatalf("card question %q names no slot of the active set", q.ID)
		}
		if q.Recommended != slot.Recommended {
			t.Errorf("%s recommended = %q, want the taxonomy's %q — a surface cannot mark a recommendation it was not served",
				q.ID, q.Recommended, slot.Recommended)
		}
		if !gf7HasOption(q.Options, q.Recommended) {
			t.Errorf("%s recommends %q, which names none of the options served on the card", q.ID, q.Recommended)
		}
		for _, o := range q.Options {
			if o.Effect == "" {
				t.Errorf("%s/%s reached the card with no effect line (C.4)", q.ID, o.Value)
			}
		}
	}
}

// TestGF7EveryIssuedCardCarriesTheFamilyGuess (R9): the family and what resolved
// it ride EVERY card kind the pipeline issues after triage — the interview card,
// the re-interview review card and the approval card alike — so no surface has
// to guess where the chip belongs. The family card itself omits them, because
// family is exactly what is unresolved there.
func TestGF7EveryIssuedCardCarriesTheFamilyGuess(t *testing.T) {
	f := newFix(t)
	askID, st := gf3ToApproval(t, f)
	_, approval := f.openAsk(st.RunID)
	if approval.Family != intake.FamilySoftware || approval.FamilySource != intake.FamilySourceClassifier {
		t.Errorf("approval card family = %q/%q, want software/classifier", approval.Family, approval.FamilySource)
	}
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionReInterview})
	_, review := f.openAsk(st.RunID)
	if review.Family != intake.FamilySoftware || review.FamilySource != intake.FamilySourceClassifier {
		t.Errorf("review card family = %q/%q, want software/classifier", review.Family, review.FamilySource)
	}
}

// TestGF7FamilyCardOmitsTheGuess (R9): the one card where the family is the
// question serves no family — claiming a guess there would be claiming an answer
// to the card's own question.
func TestGF7FamilyCardOmitsTheGuess(t *testing.T) {
	f := newFix(t)
	f.p.Classifier = nil
	f.p.Registry = nil
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardFamily {
		t.Fatalf("card = %q, want the family question", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	if card.Family != "" || card.FamilySource != "" {
		t.Errorf("the family card serves family %q/%q — family is what it is asking about", card.Family, card.FamilySource)
	}
}
