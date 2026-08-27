package intake_test

// gf12red_test.go — P3-GF12 acceptance tests, committed RED with the grounding
// brief (P3/briefs/P3-GF12.md §9; Amendment-A carve-out, CONVENTIONS §5).
// The window opens at the grounding commit and closes at the P3-GF12
// implementation commit. Every test compiles against the CURRENT type surface
// (the grounding commit adds the inert `CardEmission` kind and the exported
// A15 cap aliases — sanctioned inert type surface, no behavior) and fails
// only on behavior that does not exist yet.
//
// The two defects these tests kill were witnessed live on the GF9 evidence
// world (~/.sinet-gf9-builder, control.log 2026-08-27T23:33..2026-08-28T01:14):
//
//   (1) eleven over-cap refusals — "step S-1 approach is 1294 characters
//       (cap 1200)" and kin (1246..1569) — each crashing the intake drive
//       ("stage: intake answer died mid-drive") or the fork's dispatch, the
//       recovery ladder forking ⚙ recovery.max_attempts times with ZERO new
//       information per re-drive, then tombstoning the lineage. Twice.
//   (2) the content-family planner re-emitting NEEDS-CLARIFICATION markers
//       for facts the requester had already supplied and confirmed — asks
//       4/5/6 of t-4b8ed0297f821433 re-confirm the same shop facts, ask 6
//       byte-identical to the answered ask 5.
//
// Binding sources: Spec S06.6 (markers: "each marker is either asked (S06.5)
// or converted to a listed assumption"; [A15] per-step approach), S06.7(a)
// (bounded auto-fix, then a decision card — the requester-granted-round
// precedent), S06.10 (ceremony economics: bounded single revisions);
// CONVENTIONS §60 (bounded in-session re-ask is the house pattern; content is
// never repaired or trimmed), §14, §16.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// overCapRefusal reproduces the exact error shape the landed seam returns for
// an over-cap emission: pairSession wraps Plan.Validate's ErrBadArtifact chain
// in "planner output: ..." (internal/stage/engines.go pairSession; witnessed
// verbatim in the GF9 world's control.log).
func overCapRefusal(runes int) error {
	return fmt.Errorf("planner output: %w",
		fmt.Errorf("%w: step S-1 approach is %d characters (cap 1200)", intake.ErrBadArtifact, runes))
}

// pairJSON renders the pair for disposition-agnostic content assertions.
func pairJSON(t *testing.T, p *intake.Pair) string {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// driveToInterviewCard starts a standard-tier task and returns the fixture,
// state, and the open interview ask.
func driveToInterviewCard(t *testing.T) (*fix, *intake.State, string) {
	t.Helper()
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("expected the interview card, got %q", st.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	return f, st, askID
}

// TestGF12OverCapDraftLandsOnACardNeverACrash (brief R1/R2; S06.6 [A15];
// CONVENTIONS §60): when the planner seat's emission is refused for the A15
// cap and the seam's bounded re-emission is exhausted, the intake drive must
// LAND — a served decision card on a parked run — never error up into the
// mapDriveErr crash that feeds the recovery ladder. RED today: p.Answer
// returns the refusal as a drive error, the surface crashes the run, and the
// GF9 world shows where that ends (fork chain → tombstone, twice).
func TestGF12OverCapDraftLandsOnACardNeverACrash(t *testing.T) {
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		return intake.Pair{}, overCapRefusal(1294)
	}

	raw, _ := json.Marshal(intake.Answer{ForceProceed: true})
	st, err := f.p.Answer(context.Background(), "u1", askID, raw)
	if err != nil {
		t.Fatalf("the over-cap emission crashed the drive instead of landing on a card (the witnessed GF9 shape): %v", err)
	}
	if st.OpenAskKind != intake.CardEmission {
		t.Fatalf("open ask kind = %q, want %q (the honest landing)", st.OpenAskKind, intake.CardEmission)
	}
	if got := f.runState(st.RunID); got != run.StateParked {
		t.Fatalf("run state = %q, want parked — gates wait (S06.1)", got)
	}
	_, card := f.openAsk(st.RunID)
	if card.Decision == nil || strings.TrimSpace(card.Decision.Summary) == "" {
		t.Fatal("the emission card must carry a DecisionBody with a plain-words summary of what happened")
	}
	replan := false
	for _, c := range card.Decision.Choices {
		if c.Value == intake.ChoiceReplan {
			replan = true
		}
	}
	if !replan {
		t.Fatalf("the emission card offers no %q choice — the requester must be able to grant one more round (the S06.7(a) precedent)", intake.ChoiceReplan)
	}
	// Never a silent trim (CONVENTIONS §60): the refused emission must not
	// have been accepted in any form.
	if st.SpecRef != nil || st.PlanVersion != 0 {
		t.Fatalf("a refused emission left an accepted artifact behind (spec ref %v, plan v%d) — model output is never repaired",
			st.SpecRef, st.PlanVersion)
	}
}

// TestGF12EmissionCardRetryGrantsOneMoreRound (brief R3): answering the
// emission card with ChoiceReplan is an explicit human grant of one more paid
// round — the pipeline re-drives the SAME draft with nothing lost, and a
// compliant emission then walks on to approval. RED today: the card does not
// exist, so there is nothing to answer.
func TestGF12EmissionCardRetryGrantsOneMoreRound(t *testing.T) {
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		return intake.Pair{}, overCapRefusal(1475)
	}
	raw, _ := json.Marshal(intake.Answer{ForceProceed: true})
	st, err := f.p.Answer(context.Background(), "u1", askID, raw)
	if err != nil {
		t.Fatalf("the over-cap emission crashed the drive instead of landing on a card: %v", err)
	}
	if st.OpenAskKind != intake.CardEmission {
		t.Fatalf("open ask kind = %q, want %q", st.OpenAskKind, intake.CardEmission)
	}
	before := f.planner.draftCalls

	f.planner.draft = nil // the granted round emits a compliant pair
	emissionAsk, _ := f.openAsk(st.RunID)
	st = f.answer("u1", emissionAsk, intake.Answer{Choice: intake.ChoiceReplan})
	if f.planner.draftCalls != before+1 {
		t.Fatalf("draft calls = %d, want %d — the retry choice grants exactly one more round", f.planner.draftCalls, before+1)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after the granted round, open ask kind = %q, want the approval card — the journey completes", st.OpenAskKind)
	}
}

// TestGF12OverCapReviseKeepsTheContestAndLandsHonestly (brief R2/R3; the GF9
// t-33c01d7c crash at 00:03:59 was exactly this leg — the redraft after the
// approval card): an over-cap REVISE lands on the emission card with the
// pending contest preserved durably, and the granted round re-drives the
// revise with the requester's findings intact. RED today: the contest answer
// errors out of p.Answer and the run goes to the ladder.
func TestGF12OverCapReviseKeepsTheContestAndLandsHonestly(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected the approval card, got %q", st.OpenAskKind)
	}

	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		return intake.Pair{}, overCapRefusal(1400)
	}
	approvalAsk, _ := f.openAsk(st.RunID)
	raw, _ := json.Marshal(intake.Answer{Action: intake.ActionRePlan, Note: "make it simpler"})
	st, err := f.p.Answer(context.Background(), "u1", approvalAsk, raw)
	if err != nil {
		t.Fatalf("the over-cap revise crashed the contest drive instead of landing on a card: %v", err)
	}
	if st.OpenAskKind != intake.CardEmission {
		t.Fatalf("open ask kind = %q, want %q", st.OpenAskKind, intake.CardEmission)
	}

	var second *intake.ReviseInput
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		second = &in
		return baseRevise(in), nil
	}
	emissionAsk, _ := f.openAsk(st.RunID)
	st = f.answer("u1", emissionAsk, intake.Answer{Choice: intake.ChoiceReplan})
	if second == nil {
		t.Fatal("the granted round never re-drove the revise")
	}
	found := false
	for _, fd := range second.Findings {
		if strings.Contains(fd, "make it simpler") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the re-driven revise lost the requester's contest — findings %v must still carry it (resume-in-place, nothing lost)", second.Findings)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after the granted round, open ask kind = %q, want the approval card back", st.OpenAskKind)
	}
}

// TestGF12SettledMarkerIsNeverReAsked (brief R5/R6; S06.5 "a resolved slot
// must never re-ask", S06.6 marker rules; the witnessed ask-5→ask-6
// byte-identical repeat): a re-emitted NEEDS-CLARIFICATION marker whose text
// matches one the requester already answered is settled from the record —
// disclosed, never re-carded — and the pipeline walks on. RED today: the
// pipeline cards ANY open marker, every round, unboundedly.
func TestGF12SettledMarkerIsNeverReAsked(t *testing.T) {
	const marker = "The shop name and opening hours on the page are placeholders; the real ones are needed before publishing."
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Spec.Clarifications = []string{marker}
		return p, nil
	}
	// The witnessed planner behavior: the revise re-emits the SAME marker
	// after the requester answered it (t-4b8ed0297f821433 ask 6 ≡ ask 5).
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		p := baseRevise(in)
		p.Spec.Clarifications = []string{marker}
		return p, nil
	}

	st := f.answer("u1", askID, intake.Answer{ForceProceed: true})
	if st.OpenAskKind != intake.CardClarification {
		t.Fatalf("expected the first clarification card (a genuinely open point), got %q", st.OpenAskKind)
	}
	clarAsk, card := f.openAsk(st.RunID)
	st = f.answer("u1", clarAsk, intake.Answer{Answers: []intake.SlotAnswer{
		{ID: card.Questions[0].ID, Value: "Spoke & Sprocket, Mon-Fri 9-18, Sat 9-13, closed Sunday"},
	}})

	if st.OpenAskKind == intake.CardClarification {
		t.Fatal("the settled fact was re-asked — the answered marker came back as a fresh clarification card (the witnessed confirm-loop)")
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("open ask kind = %q, want the approval card — the re-emitted settled marker is settled from the record", st.OpenAskKind)
	}
	pair, err := f.p.CurrentPair(context.Background(), st.TaskID)
	if err != nil {
		t.Fatalf("CurrentPair: %v", err)
	}
	if len(pair.Spec.Clarifications) > 0 {
		t.Fatalf("the served pair still carries open markers %v — an artifact with open markers cannot reach approval (S06.6)", pair.Spec.Clarifications)
	}
	if !strings.Contains(pairJSON(t, pair), "Spoke & Sprocket") {
		t.Fatal("the requester's answer is nowhere on the served pair — settling from the record must keep the answer visible, never drop it")
	}
}

// TestGF12MarkerRoundsAreBoundedByConversion (brief R7; S06.6's second arm:
// "each marker is either asked (S06.5) or converted to a listed assumption"):
// a planner that keeps inventing FRESH markers gets a bounded number of
// clarification rounds; what is still open at the bound converts to listed
// assumptions on the SPEC — visible on the approval card's centerpiece,
// contestable via Re-plan — instead of an unbounded human treadmill (4 rounds
// witnessed live). RED today: the loop has no bound at all.
func TestGF12MarkerRoundsAreBoundedByConversion(t *testing.T) {
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Spec.Clarifications = []string{"fresh doubt 0: is the widget really a widget?"}
		return p, nil
	}
	round := 0
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		round++
		p := baseRevise(in)
		p.Spec.Clarifications = []string{fmt.Sprintf("fresh doubt %d: and is doubt %d fully resolved?", round, round-1)}
		return p, nil
	}

	st := f.answer("u1", askID, intake.Answer{ForceProceed: true})
	cards := 0
	for st.OpenAskKind == intake.CardClarification {
		cards++
		if cards > 2 {
			t.Fatalf("clarification card %d issued — the marker loop must be bounded (2 rounds, then conversion to listed assumptions)", cards)
		}
		clarAsk, card := f.openAsk(st.RunID)
		var answers []intake.SlotAnswer
		for _, q := range card.Questions {
			answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.Text})
		}
		st = f.answer("u1", clarAsk, intake.Answer{Answers: answers})
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("open ask kind = %q, want the approval card after the bounded rounds", st.OpenAskKind)
	}
	pair, err := f.p.CurrentPair(context.Background(), st.TaskID)
	if err != nil {
		t.Fatalf("CurrentPair: %v", err)
	}
	if len(pair.Spec.Clarifications) > 0 {
		t.Fatalf("open markers %v survived to approval (S06.6)", pair.Spec.Clarifications)
	}
	if !strings.Contains(pairJSON(t, pair), "fresh doubt") {
		t.Fatal("the unconverted leftover marker vanished — conversion must LIST it as an assumption, never drop it silently")
	}
}
