package intake_test

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf8exec_test.go — the P3-GF8 executor-owed acceptance (brief §9): the second
// half of the drafted-plan understanding surface, driven through the machinery
// that actually produces it.

// TestGF8ApprovalCardServesTheSuppliedInputs (brief R4; operator record r5
// §B.1 "data/integrations"): the requester-supplied inputs that dismissed a
// research trigger are part of what the platform understood, so they belong on
// the drafted-plan surface beside the constraints. They reach Layer 2 through
// the real supply-fact path (S06.3), never hand-placed.
func TestGF8ApprovalCardServesTheSuppliedInputs(t *testing.T) {
	f := newFix(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Plan.ResearchNodes = nil // the policy violation that cards the requester
		return p, nil
	}
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		if in.Reason == intake.ReviseResearch {
			return in.Pair, nil // the bounce fails to add the node
		}
		return baseRevise(in), nil
	}
	req := stdRequest()
	req.Text = "use the latest dependency versions"
	st := f.start(req)
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardResearch {
		t.Fatalf("expected the research decision card, got %q", st.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	var facts []intake.SuppliedFact
	for _, rule := range st.Spine.MissingResearch {
		facts = append(facts, intake.SuppliedFact{RuleID: rule, Fact: "dependency X is at 2.4.1 as of today"})
	}
	st = f.answer("u1", askID, intake.Answer{Choice: intake.ChoiceSupplyFact, Facts: facts})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after supply_fact: %q", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	if card.Approval == nil {
		t.Fatal("approval card without body")
	}
	supplied := card.Approval.Layer2.Supplied
	if len(supplied) != len(facts) {
		t.Fatalf("approval layer 2 serves %d supplied input(s), want %d (r5 §B.1 data/integrations)", len(supplied), len(facts))
	}
	for _, f := range supplied {
		if f.RuleID == "" || !strings.Contains(f.Fact, "2.4.1") {
			t.Errorf("a served supplied input lost its identity or its text: %+v", f)
		}
	}
}
