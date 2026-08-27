package intake_test

// gf8red_test.go — P3-GF8 acceptance tests, committed RED with the grounding
// brief (P3/briefs/P3-GF8.md §9; Amendment-A carve-out, CONVENTIONS §38 area).
// The window opens at the grounding commit and closes at the P3-GF8
// implementation commit. Every test here compiles against the CURRENT type
// surface (the grounding commit adds the inert [A15] fields on Step and
// ApprovalLayer2 — sanctioned inert type surface, no behavior) and fails only
// on behavior that does not exist yet.
//
// Binding sources: Spec S06.6 per-step approach [A15] + S00.9 A15 row;
// Spec S06.9 (structured Re-plan entry, delta re-approval);
// operator records P3/design/b6-gate-operator-findings-r5-2026-08-23.md
// (§B.1, §C rule 7, §D.1) and ...-r4-2026-08-23.md (§F3);
// harvest P3/design/w1-nexus-live-harvest-2026-08-27.md (H9, H17).

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf8ApproachPlanner returns a draft hook emitting the base pair with the
// [A15] per-step approach members filled — the post-A15 planner's shape.
func gf8ApproachPlanner() func(in intake.DraftInput) (intake.Pair, error) {
	return func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		for i := range p.Plan.Steps {
			p.Plan.Steps[i].Approach = "I edit the widget module directly and keep the public surface unchanged."
			p.Plan.Steps[i].Decisions = []intake.StepDecision{{
				Decision:     "edit in place rather than rewrite",
				Alternatives: []string{"rewrite the module from scratch"},
				Why:          "the module is small and a rewrite risks the parts that already work",
			}}
		}
		p.Plan.Steps[0].OrderingRationale = "the change must exist before anything can verify it"
		return p, nil
	}
}

// gf8Approve drives a standard-tier task to an approved plan with the
// approach-carrying planner and returns the fixture and its state.
func gf8Approve(t *testing.T) (*fix, *intake.State) {
	t.Helper()
	f := newFix(t)
	f.planner.draft = gf8ApproachPlanner()
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected approval card, got %q", st.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})
	return f, st
}

// TestGF8ApproachRequiredAtValidation (brief R10/R11; S06.6 [A15]): a PLAN
// step without its approach fails artifact validation at the boundary, and a
// listed material decision must be complete (decision + at least one
// alternative + why). RED pre-A15: Validate knows no approach member.
func TestGF8ApproachRequiredAtValidation(t *testing.T) {
	base := func() intake.Plan {
		return intake.Plan{
			TaskID: "t1", Owner: "u1", Version: 1, SpecVersion: 1,
			Status: intake.StatusDraft,
			Steps: []intake.Step{{
				ID: "S-1", Title: "Do the work", DoneWhen: "tests pass", Class: "C1",
				Approach: "straightforward edit of the one module involved",
			}},
			Coverage: map[string][]string{},
		}
	}

	missing := base()
	missing.Steps[0].Approach = ""
	if err := missing.Validate(); !errors.Is(err, intake.ErrBadArtifact) {
		t.Errorf("a step without its approach must fail validation (S06.6 [A15]); got %v", err)
	}

	incomplete := base()
	incomplete.Steps[0].Decisions = []intake.StepDecision{{Decision: "use X"}}
	if err := incomplete.Validate(); !errors.Is(err, intake.ErrBadArtifact) {
		t.Errorf("a material decision without alternatives+why must fail validation (S06.6 [A15]); got %v", err)
	}

	complete := base()
	complete.Steps[0].Decisions = []intake.StepDecision{{
		Decision: "use X", Alternatives: []string{"Y"}, Why: "X is already in the repo",
	}}
	if err := complete.Validate(); err != nil {
		t.Errorf("a complete step must validate; got %v", err)
	}
}

// TestGF8ApproachRendersOnThePlanOfRecord (brief R12; S06.6 [A15]): the
// accepted emission's markdown of record carries the approach, the material
// decisions and the ordering rationale — the artifact, not just the sidecar,
// says HOW. RED: renderPlanMD does not render the [A15] members.
func TestGF8ApproachRendersOnThePlanOfRecord(t *testing.T) {
	f := newFix(t)
	f.planner.draft = gf8ApproachPlanner()
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.PlanRef == nil {
		t.Fatal("no plan artifact after drafting")
	}
	md, err := os.ReadFile(st.PlanRef.Path)
	if err != nil {
		t.Fatalf("read plan of record: %v", err)
	}
	for _, want := range []string{
		"I edit the widget module directly",
		"edit in place rather than rewrite",
		"rewrite the module from scratch",
		"the change must exist before anything can verify it",
	} {
		if !strings.Contains(string(md), want) {
			t.Errorf("plan of record does not carry %q (S06.6 [A15] renders on the artifact)", want)
		}
	}
}

// TestGF8ApprovalCardServesConstraints (brief R4; r5 §B.1 + §C rule 7): the
// drafted-plan surface serves the WHOLE derived understanding — the SPEC's
// constraints join the approval card's expandable layer, per-field data, not
// prose. RED: ApprovalLayer2.Constraints is never populated.
func TestGF8ApprovalCardServesConstraints(t *testing.T) {
	f := newFix(t)
	f.planner.draft = gf8ApproachPlanner()
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected approval card, got %q", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	if card.Approval == nil {
		t.Fatal("approval card without body")
	}
	got := card.Approval.Layer2.Constraints
	if len(got) != 1 || got[0] != "stay within the repo" {
		t.Errorf("approval layer 2 does not serve the SPEC constraints (r5 §B.1): got %v", got)
	}
}

// TestGF8ContestCarriesTheFieldNotABlob (brief R5/R6/R7; S06.9 structured
// entry; r5 §C rule 7): a Re-plan contest naming an understanding field by
// its target key reaches the reviser as a finding carrying the field's
// CURRENT text — field identity travels, nothing arrives as a blob. RED:
// replanContest folds the raw target string without expansion.
func TestGF8ContestCarriesTheFieldNotABlob(t *testing.T) {
	f := newFix(t)
	f.planner.draft = gf8ApproachPlanner()
	var captured *intake.ReviseInput
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		captured = &in
		return baseRevise(in), nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	f.answer("u1", askID, intake.Answer{
		Action:   intake.ActionRePlan,
		Contests: []intake.ContestRef{{Target: "out_of_scope:1", Note: "keep this, and also no telemetry"}},
	})
	if captured == nil {
		t.Fatal("contest did not reach the reviser")
	}
	if captured.Reason != intake.ReviseContest {
		t.Fatalf("revise reason %q", captured.Reason)
	}
	found := false
	for _, finding := range captured.Findings {
		if strings.Contains(finding, "no deploys") && strings.Contains(finding, "no telemetry") {
			found = true
		}
	}
	if !found {
		t.Errorf("the contest finding does not carry the contested field's current text (\"no deploys\"): %v", captured.Findings)
	}
}

// TestGF8ContestRefusesAFieldThatIsNotThere (brief R6): a grammar-shaped
// contest target naming a field the pair does not hold is refused — the §43
// one-list discipline, extended to field targets. RED: any non-empty target
// is accepted today.
func TestGF8ContestRefusesAFieldThatIsNotThere(t *testing.T) {
	f := newFix(t)
	f.planner.draft = gf8ApproachPlanner()
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	raw := []byte(`{"action":"replan","contests":[{"target":"out_of_scope:99","note":"?"}]}`)
	if _, err := f.p.Answer(context.Background(), "u1", askID, raw); !errors.Is(err, intake.ErrBadAnswer) {
		t.Errorf("a contest naming out_of_scope:99 against a 1-item list must be refused (brief R6); got %v", err)
	}
}

// TestGF8PreDraftCorrectionOfAResolvedSlot (brief R2; r5 §C rule 3, harvest
// H9): during the interview, a slot already resolved — shown on this card's
// understood block — accepts a corrected answer; last write wins, nothing is
// silently discarded (H17 posture). RED: applyInterviewAnswer refuses any
// slot not asked on the current card.
func TestGF8PreDraftCorrectionOfAResolvedSlot(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	askID, card1 := f.openAsk(st.RunID)
	if card1.Kind != intake.CardInterview {
		t.Fatalf("expected interview card, got %q", card1.Kind)
	}
	correctedSlot := card1.Questions[0].ID
	var answers []intake.SlotAnswer
	for _, q := range card1.Questions {
		answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "first answer: " + q.ID})
	}
	st = f.answer("u1", askID, intake.Answer{Answers: answers})
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("expected a second interview card, got %q", st.OpenAskKind)
	}
	askID, card2 := f.openAsk(st.RunID)
	for _, q := range card2.Questions {
		if q.ID == correctedSlot {
			t.Fatalf("slot %q re-asked on card 2 — fixture assumption broken", correctedSlot)
		}
	}
	// The card SHOWS the resolved slot in its understood block; correcting it
	// alongside this card's own answers must be accepted.
	body := intake.Answer{Answers: []intake.SlotAnswer{{ID: correctedSlot, Value: "corrected answer"}}}
	for _, q := range card2.Questions {
		body.Answers = append(body.Answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	st2, err := f.p.Answer(context.Background(), "u1", askID, raw)
	if err != nil {
		t.Fatalf("correcting a resolved slot shown on the card must be accepted (brief R2): %v", err)
	}
	got := ""
	for _, r := range st2.Resolutions {
		if r.SlotID == correctedSlot {
			got = r.Value
		}
	}
	if got != "corrected answer" {
		t.Errorf("resolution for %q = %q, want the correction (last write wins)", correctedSlot, got)
	}
}

// TestGF8UnderstandingDeltaProperty (brief R8; S06.9 delta vocabulary; A15
// freeze mechanics) — PROPERTY: for EVERY requester-facing content field, a
// post-approval revision changing only that field produces a delta card with
// an item naming that field — a content change can never be invisible on a
// delta card, and a pure understanding-correction is never "changes
// nothing". RED: diffPairs diffs only ACs, steps (title/done-when/class) and
// assumptions.
func TestGF8UnderstandingDeltaProperty(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *intake.Pair)
		target string
	}{
		{"restatement", func(p *intake.Pair) { p.Spec.Restatement = "corrected restatement" }, "restatement"},
		{"outcome", func(p *intake.Pair) { p.Spec.Outcome = []string{"a corrected outcome"} }, "outcome:1"},
		{"constraint", func(p *intake.Pair) { p.Spec.Constraints = []string{"stay in src/ only"} }, "constraint:1"},
		{"out_of_scope", func(p *intake.Pair) { p.Spec.OutOfScope = []string{"no deploys, no telemetry"} }, "out_of_scope:1"},
		{"risk", func(p *intake.Pair) { p.Plan.Risks = []string{"a corrected risk"} }, "risk:1"},
		{"approach", func(p *intake.Pair) { p.Plan.Steps[0].Approach = "a corrected approach" }, "approach:S-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, st := gf8Approve(t)
			ctx := context.Background()
			pair, _, err := f.p.ApprovedPair(ctx, st.TaskID)
			if err != nil {
				t.Fatalf("ApprovedPair: %v", err)
			}
			next := pair
			tc.mutate(&next)
			st2, _, err := f.p.ProposeDelta(ctx, st.TaskID, "contested_card", next)
			if err != nil {
				t.Fatalf("a pure %s correction must be expressible as a delta (S06.9): %v", tc.name, err)
			}
			rec := st2.Deltas[len(st2.Deltas)-1]
			found := false
			for _, item := range rec.Items {
				if item.Target == tc.target {
					found = true
				}
			}
			if !found {
				t.Errorf("delta items carry no %q target: %+v", tc.target, rec.Items)
			}
		})
	}
}
