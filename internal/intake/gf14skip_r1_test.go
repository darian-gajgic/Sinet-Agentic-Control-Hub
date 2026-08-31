package intake_test

// gf14skip_r1_test.go — P3-GF14 drain r1 F3: the currency point's SKIP arm,
// walked end to end.
//
// The brief's R3 gives the open point two honest endings, and the landing only
// pinned one of them: answering with a currency. The other is the person
// declining to name one — "leave the numbers as they are" — which must not
// silently drop the question. It converts to the out-loud assumption S06.6's
// second arm describes, listed on the approval card's centerpiece where a
// Re-plan can contest it, and it is never asked again.

import (
	"context"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

func TestGF14CurrencySkipBecomesAnOutLoudAssumption(t *testing.T) {
	f := newFix(t)
	req := stdRequest()
	req.Title = "A one-page price list for my candle shop"
	req.Text = "A one-page price list for my candle shop, six candles with a name and a price each."

	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Spec.ACs = append(p.Spec.ACs, intake.AC{
			N: len(p.Spec.ACs) + 1, Plain: "each candle shows its price (8, 9, 9, 10, 8, 11)",
		})
		p.Plan.Coverage[p.Spec.ACs[len(p.Spec.ACs)-1].Key()] = []string{"S-1"}
		return p, nil
	}

	st := f.start(req)
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardClarification {
		t.Fatalf("card = %q, want the currency open point", st.OpenAskKind)
	}

	askID, card := f.openAsk(st.RunID)
	q := card.Questions[0]
	skip := q.Options[len(q.Options)-1] // the leave-them-bare arm
	if !strings.Contains(strings.ToLower(skip.Value), "no currency") {
		t.Fatalf("the last option is meant to be the skip arm, got %+v", skip)
	}

	st = f.answer("u1", askID, intake.Answer{Answers: []intake.SlotAnswer{{ID: q.ID, Value: skip.Value}}})

	// The point is not re-asked, and approval is reachable — an open marker
	// would have blocked it.
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("card = %q after the skip, want the approval card", st.OpenAskKind)
	}
	pair, err := f.p.CurrentPair(context.Background(), st.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pair.Spec.Clarifications) != 0 {
		t.Fatalf("the skipped point is still open on the artifact: %v", pair.Spec.Clarifications)
	}

	// It is SAID, on the card's centerpiece, in the requester's own answer.
	var listed string
	for _, a := range pair.Spec.Assumptions {
		if strings.Contains(strings.ToLower(a.Text), "currency") {
			listed = a.Text
		}
	}
	if listed == "" {
		t.Fatalf("the skip vanished instead of becoming a listed assumption: %+v", pair.Spec.Assumptions)
	}
	if !strings.Contains(strings.ToLower(listed), strings.ToLower(skip.Value)) {
		t.Errorf("the assumption must carry what the person actually chose (%q): %q", skip.Value, listed)
	}
	for _, token := range []string{"S06", "§", "Spec/", "NEEDS-CLARIFICATION"} {
		if strings.Contains(listed, token) {
			t.Errorf("the listed assumption carries the platform's own vocabulary (%q): %q", token, listed)
		}
	}

	// And the approval card the person reads carries it too.
	_, approval := f.openAsk(st.RunID)
	if approval.Approval == nil {
		t.Fatal("no approval body on the approval card")
	}
	found := false
	for _, a := range approval.Approval.Layer1.Assumptions {
		if strings.Contains(strings.ToLower(a.Text), "currency") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the skipped currency point is not on the card's assumptions list: %+v", approval.Approval.Layer1.Assumptions)
	}
}
