package intake_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
)

// route_test.go — S08.8 as intake surfaces it: the selection block on the
// Stage-4 approval card (visible PRE-execution), the re-route/pin answer
// with its recorded actor, pin survival across re-planning, and the no-fit
// two-stage card content. The router is faked (selection mechanics are the
// worker package's battery); this battery pins the SURFACE contract.

type fakeRouter struct {
	calls int
	block intake.RouteBlock
}

func (f *fakeRouter) RouteTask(_ context.Context, q intake.RouteQuery) (intake.RouteBlock, error) {
	f.calls++
	b := f.block
	// Echo requirements so tests can assert the plan-declared inputs
	// reached the router (boundary: the plan declares, S08.8 selects).
	b.Signals = append(append([]string{}, b.Signals...), "family="+q.Family)
	return b, nil
}

func workerBackedBlock() intake.RouteBlock {
	return intake.RouteBlock{
		Cause: "selector-match", Score: 3,
		TemplateID: "wt-alpha", TemplateName: "alpha", VersionID: "wtv-alpha-1",
		Model: "claude-haiku-4-5", Lane: "anthropic", WindowTokens: 200_000,
		PlainReason: "Specialist \"alpha\" matched: family selector software.",
		Candidates: []intake.RouteCandidate{
			{TemplateID: "wt-alpha", Name: "alpha", VersionID: "wtv-alpha-1", Score: 3, Reason: "family selector"},
			{TemplateID: "wt-beta", Name: "beta", VersionID: "wtv-beta-2", Score: 2, Reason: "trigger match"},
		},
	}
}

// approvalCard walks a standard task to its approval card.
func approvalCard(t *testing.T, f *fix) (*intake.State, string, intake.Card) {
	t.Helper()
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected approval card, got %q", st.OpenAskKind)
	}
	askID, card := f.openAsk(st.RunID)
	return st, askID, card
}

func TestApprovalCardCarriesRoutingBlock(t *testing.T) {
	f := newFix(t)
	router := &fakeRouter{block: workerBackedBlock()}
	f.p.Router = router

	st, _, card := approvalCard(t, f)
	if card.Approval == nil || card.Approval.Routing == nil {
		t.Fatal("approval card missing the S08.8 routing block")
	}
	rb := card.Approval.Routing
	if rb.TemplateName != "alpha" || rb.PlainReason == "" {
		t.Fatalf("routing block = %+v", rb)
	}
	if len(rb.Candidates) != 2 {
		t.Fatalf("candidates = %d, want the re-route targets", len(rb.Candidates))
	}
	if st.Routing == nil || st.Routing.TemplateID != "wt-alpha" {
		t.Fatalf("state routing = %+v", st.Routing)
	}
	// The plan-declared requirements reached the router (S08.8 boundary).
	joined := strings.Join(rb.Signals, ",")
	if !strings.Contains(joined, "family=software") {
		t.Fatalf("router inputs missing: %v", rb.Signals)
	}
}

func TestRerouteAndPinRecordedWithActor(t *testing.T) {
	f := newFix(t)
	f.p.Router = &fakeRouter{block: workerBackedBlock()}

	st, askID, _ := approvalCard(t, f)
	st = f.answer("u1", askID, intake.Answer{
		Action: intake.ActionApprove,
		Route:  &intake.RouteOverride{Target: "wt-beta", Pin: true},
	})
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("phase = %s", st.Phase)
	}
	if st.Routing.TemplateID != "wt-beta" || st.Routing.VersionID != "wtv-beta-2" {
		t.Fatalf("override not applied: %+v", st.Routing)
	}
	if st.Routing.Cause != "override" || !st.Routing.Pinned || st.Routing.OverriddenBy != "u1" {
		t.Fatalf("override bookkeeping: %+v", st.Routing)
	}
	// The override is a recorded human decision with its actor (S08.8).
	doc := f.ledgerDoc(st.TaskID)
	found := false
	for _, d := range doc.Decisions {
		if strings.Contains(d.Text, "routing override") && d.Author == ledger.AuthorHuman {
			found = true
		}
	}
	if !found {
		t.Fatalf("no override decision recorded: %+v", doc.Decisions)
	}
}

func TestRerouteRejectsTargetOutsideCard(t *testing.T) {
	f := newFix(t)
	f.p.Router = &fakeRouter{block: workerBackedBlock()}

	_, askID, _ := approvalCard(t, f)
	raw := []byte(`{"action":"approve","route":{"target":"wt-nope"}}`)
	if _, err := f.p.Answer(context.Background(), "u1", askID, raw); err == nil {
		t.Fatal("override outside the card's candidate set must reject")
	}
	// The card stays open — the failed answer changed nothing.
	_, card := f.openAsk((mustState(t, f, askID)).RunID)
	if card.Kind != intake.CardApproval {
		t.Fatalf("card kind = %q", card.Kind)
	}
}

func mustState(t *testing.T, f *fix, askID string) *intake.State {
	t.Helper()
	// askID format intake:<task>:<n> (state.go); recover the task id.
	parts := strings.Split(askID, ":")
	st, err := f.p.LoadState(context.Background(), parts[1])
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestPinSurvivesReplanRecompute(t *testing.T) {
	f := newFix(t)
	router := &fakeRouter{block: workerBackedBlock()}
	f.p.Router = router

	_, askID, _ := approvalCard(t, f)
	// Re-plan WITH a pin on beta: the revised card must keep beta pinned
	// even though the fresh selection would pick alpha again.
	st := f.answer("u1", askID, intake.Answer{
		Action:  intake.ActionRePlan,
		Contest: &intake.ContestRef{Target: "AC-1", Note: "tighten"},
		Route:   &intake.RouteOverride{Target: "wt-beta", Pin: true},
	})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected recomputed approval card, got %q", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	rb := card.Approval.Routing
	if rb == nil || rb.TemplateID != "wt-beta" || !rb.Pinned || rb.Cause != "pinned" {
		t.Fatalf("pin did not survive the replan recompute: %+v", rb)
	}
	if rb.OverriddenBy != "u1" {
		t.Fatalf("pin actor lost: %+v", rb)
	}
	if router.calls < 2 {
		t.Fatalf("router calls = %d; re-planning must recompute (pin filters, not skips)", router.calls)
	}
}

func TestNoFitTwoStageCardContent(t *testing.T) {
	f := newFix(t)
	f.p.Router = &fakeRouter{block: intake.RouteBlock{
		Cause: "no-fit-generalist", Generalist: true, Degraded: true,
		Model: "claude-haiku-4-5", Lane: "anthropic", WindowTokens: 200_000,
		PlainReason:   "No specialist fits; running as generalist-with-injected-knowledge, DEGRADED-MARKED.",
		GapSignature:  "family=software;domain=software;classes=C1;tools=",
		ComposeEarned: true,
		GapAdvice:     "Subscription gap (2.7): config-derived advice.",
	}}

	_, _, card := approvalCard(t, f)
	rb := card.Approval.Routing
	if rb == nil || !rb.Generalist {
		t.Fatalf("no-fit block missing: %+v", rb)
	}
	// Stage 1 (interpretation confirmed) rides the same card: the S06
	// restatement is Layer1's first element.
	if card.Approval.Layer1.Restatement == "" {
		t.Fatal("restatement missing — the no-fit stage-1 surface")
	}
	// Stage 2: the one card offers generalist default (degraded-marked),
	// compose-when-earned, and the gap advice.
	if !rb.Degraded || !strings.Contains(rb.PlainReason, "DEGRADED") {
		t.Fatalf("degraded marking missing: %+v", rb)
	}
	if !rb.ComposeEarned || rb.GapSignature == "" {
		t.Fatalf("compose-when-earned/gap record refs missing: %+v", rb)
	}
	if rb.GapAdvice == "" {
		t.Fatalf("gap advice leg missing: %+v", rb)
	}
}
