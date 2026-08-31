package intake_test

// gf14red_test.go — P3-GF14 acceptance tests, committed RED with the grounding
// brief (P3/briefs/P3-GF14.md §7; Amendment-A carve-out, CONVENTIONS §3). The
// window opens at the grounding commit and closes at the P3-GF14 implementation
// commit. Every test compiles against the CURRENT type surface and fails only
// on behavior that does not exist yet.
//
// The defects these tests kill were witnessed live on the GF9 review evidence
// world (~/.sinet-gf9-review, KEPT; control.log + platform.db + the task-2
// artifacts, read at grounding 2026-08-31):
//
//   (T1) a page reload cancelled the answer beat's context mid-drive and the
//        run crashed for the recovery ladder ("stage: intake answer died
//        mid-drive … err=storage: begin immediate: context canceled",
//        03:28:01.680Z) — a ~12-minute re-billed heal for the exact act the
//        card invites ("You can leave; nothing is lost").
//   (T2) six prices supplied as bare numbers ("Honey Glow 8, … Winter Pine
//        11") reached the drafted plan as "price (8, 9, 9, 10, 8, 11
//        respectively)" with the currency unstated ANYWHERE — no open point,
//        no out-loud assumption; only the requester's contest made it euros.
//   (T3/T4/T5) task 2 served one card with two stakes: the chip said HIGH
//        (the classifier-abstain fail-closed posture, never revisited after
//        the requester answered the family card) while the plan's own spec
//        said "treated as a light task" — because the Stage-3 critique's
//        "TIER MISMATCH … downgrade to 'low'" blocker was handed to the
//        PLANNER, which "resolved" the platform's band by prose.
//
// Binding sources: Spec S02.3/S02.5 (resume semantics; the ladder is the
// fallback, not the routine cure), S06.1 ("gates wait; answering resumes the
// pipeline in place"), S06.2 (classifier failure fails closed), S06.4
// (lowering only by explicit requester action, never below a floor), S06.5
// (consequential ambiguities are asked), S06.6 (markers/assumptions contract),
// S06.8 (verdicts), S15.6; operator findings r5 §C1/C5/C6 (currency is the
// canonical case); CONVENTIONS §14, §16, §54, §56 (first bullet).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// TestGF14AnswerDriveOutlivesItsCaller is R1: once the answer beat's resume
// commit has taken the run to `running`, the drive completes to its next gate
// whatever became of the viewer's connection. The reload is staged
// deterministically: the planner seam cancels the caller's context mid-drive
// (strictly after the resume commit — the drive is what calls Draft), which is
// byte-for-byte the witnessed failure ("begin immediate: context canceled" on
// the first post-draft write).
func TestGF14AnswerDriveOutlivesItsCaller(t *testing.T) {
	f := newFix(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		cancel() // the page navigated away mid-drive
		return basePair(in), nil
	}

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("walk precondition: first card = %q, want the interview", st.OpenAskKind)
	}

	for i := 0; i < 10 && st.OpenAskKind == intake.CardInterview; i++ {
		askID, card := f.openAsk(st.RunID)
		var answers []intake.SlotAnswer
		for _, q := range card.Questions {
			answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
		}
		raw, err := json.Marshal(intake.Answer{Answers: answers})
		if err != nil {
			t.Fatal(err)
		}
		st, err = f.p.Answer(ctx, "u1", askID, raw)
		if err != nil {
			t.Fatalf("R1: the answer beat died with its caller — the drive must outlive the "+
				"viewer's connection and park on its next gate (Spec S02.3, S06.1; brief R1): %v", err)
		}
	}

	if ctx.Err() == nil {
		t.Fatal("staging defect: the draft hook never ran, so nothing was cancelled")
	}
	if got := f.planner.draftCalls; got != 1 {
		t.Fatalf("draft ran %d times, want exactly 1 (no re-pay, no ladder fork)", got)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after the detached drive the approval card must be open, got %q", st.OpenAskKind)
	}
	if got := f.runState(st.RunID); got != run.StateParked {
		t.Fatalf("run state = %q, want parked on the approval gate — never crashed for the ladder", got)
	}
}

// TestGF14CanonicalCurrencyCaseIsCaught is R3: bare-number prices with no
// currency token anywhere in the request, the answers, the pair's assumptions
// or its clarifications must NOT reach the approval card unremarked. The fake
// planner plays the model that missed the gap (the witnessed case): the
// deterministic backstop is what this test exercises. PASS is either arm the
// spec allows — an open currency point served with a finite choice set, or a
// standing out-loud currency assumption (S06.5/S06.6; r5 §C5/C6: currency is
// the canonical finite set).
func TestGF14CanonicalCurrencyCaseIsCaught(t *testing.T) {
	f := newFix(t)
	req := stdRequest()
	req.Title = "A one-page price list for my candle shop"
	req.Text = "A one-page price list for my candle shop. We sell six candles - each should get its name, a short one-line description, and its price. I will give you the exact prices."

	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Spec.ACs = append(p.Spec.ACs, intake.AC{
			N:     len(p.Spec.ACs) + 1,
			Plain: "the price list shows each candle with its name, description and price (8, 9, 9, 10, 8, 11 respectively)",
		})
		p.Plan.Coverage[p.Spec.ACs[len(p.Spec.ACs)-1].Key()] = []string{"S-1"}
		return p, nil
	}

	st := f.start(req)
	if len(st.DataBearing) == 0 {
		t.Fatal("walk precondition: the price cue must set the P47-1 data-bearing flag")
	}
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)

	currencyTokens := []string{"currenc", "euro", "dollar", "€", "$", "bare number"}
	containsAny := func(s string) bool {
		ls := strings.ToLower(s)
		for _, tok := range currencyTokens {
			if strings.Contains(ls, tok) {
				return true
			}
		}
		return false
	}

	switch st.OpenAskKind {
	case intake.CardClarification:
		// The backstop asked. The question must name the currency gap and
		// carry a finite choice set (free text is always available anyway).
		_, card := f.openAsk(st.RunID)
		for _, q := range card.Questions {
			if containsAny(q.Text) || containsAny(q.Phrased) {
				if len(q.Options) < 2 {
					t.Fatalf("the currency open point must offer a finite choice set (r5 §C5 — "+
						"currency is the canonical case), got %d options", len(q.Options))
				}
				return
			}
		}
		t.Fatal("a clarification card is open but no question addresses the unstated currency")
	case intake.CardApproval:
		pair, err := f.p.CurrentPair(context.Background(), st.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range pair.Spec.Assumptions {
			if containsAny(a.Text) {
				return // resolved out loud — the other honest arm
			}
		}
		for _, c := range pair.Spec.Clarifications {
			if containsAny(c) {
				return
			}
		}
		t.Fatal("R3: six bare-number prices reached the approval card with the currency unstated " +
			"anywhere — no open point, no out-loud assumption (the operator-named canonical case, " +
			"r5 §C6; Spec S06.5/S06.6; witnessed on t-3df81201c0ab0cbe)")
	default:
		t.Fatalf("unexpected card %q at the end of the walk", st.OpenAskKind)
	}
}

// TestGF14AbstainedStakesSettleWhenTheFamilyAnswerArrives is R4.1: the
// classifier-abstain HIGH is a fail-closed POSTURE (Spec S06.2), not a
// classification; when the requester's family answer removes the abstain's
// cause, the classification completes once and its proposal assigns the tier
// (floors still clamp upward; the band is never re-entered). Witnessed
// inverse: t-cccd5dc7d00a4f64 carried tier=high, floor=none to its grave while
// its own spec said "treated as a light task".
func TestGF14AbstainedStakesSettleWhenTheFamilyAnswerArrives(t *testing.T) {
	f := newFix(t)
	f.class.err = errors.New("cannot read this request")

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardFamily {
		t.Fatalf("walk precondition: abstain must raise the family card, got %q", st.OpenAskKind)
	}
	if st.Tier != intake.TierHigh {
		t.Fatalf("walk precondition: the abstain posture is HIGH (S06.2 fail-closed), got %q", st.Tier)
	}

	// The requester names the kind — the fact whose absence caused the
	// abstain. The classifier is healthy again and, given the family, reads
	// the task as low-stakes.
	f.class.err = nil
	f.class.prop = intake.TriageProposal{
		Family: intake.FamilyContent, Tier: intake.TierLow,
		Est: intake.Estimate{SizeClass: "S", USD: 0.2, Known: true, Basis: "fake"},
	}
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Choice: string(intake.FamilyContent)})

	if st.Tier != intake.TierLow {
		t.Fatalf("R4.1: tier = %q after the family answer, want %q — the fail-closed posture must "+
			"settle when its cause is removed (Spec S06.2; the served first-paint promise "+
			"'it settles as the questions are answered/chosen')", st.Tier, intake.TierLow)
	}
}

// TestGF14LowerStakesIsARequesterDoorOnTheApprovalCard is R4.5: S06.4's one
// downward move — "explicit requester action" — must be reachable from the
// card. Pipeline.LowerTier has existed since its packet with ZERO callers
// (grounding grep, 2026-08-31): a verb no wire reaches is not a door.
func TestGF14LowerStakesIsARequesterDoorOnTheApprovalCard(t *testing.T) {
	f := newFix(t)
	f.class.prop.Tier = intake.TierHigh

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("walk precondition: approval card, got %q", st.OpenAskKind)
	}
	if st.Tier != intake.TierHigh {
		t.Fatalf("walk precondition: classified HIGH, got %q", st.Tier)
	}

	askID, _ := f.openAsk(st.RunID)
	st2, err := f.p.Answer(context.Background(), "u1", askID,
		[]byte(`{"action":"lower_stakes","tier":"standard"}`))
	if err != nil {
		t.Fatalf("R4.5: the approval card must accept the lower_stakes action (Spec S06.4 explicit "+
			"requester action; Pipeline.LowerTier is the landed verb): %v", err)
	}
	if st2.Tier != intake.TierStandard {
		t.Fatalf("tier = %q after lowering, want %q", st2.Tier, intake.TierStandard)
	}
	if st2.OpenAskKind != intake.CardApproval {
		t.Fatalf("the approval door must still stand after lowering, got %q", st2.OpenAskKind)
	}
}

// TestGF14CritiqueDownwardProposalReachesTheRequesterNotTheDrain is R4.2/R4.3:
// a critique that judges the tier TOO HIGH is a decision only the requester
// may take (S06.4), so it must ride the approval card's platform-authored
// stakes block — never the REVISE package (where task 2's planner "resolved"
// the platform's band by prose), and never the tier itself (monotone rule).
func TestGF14CritiqueDownwardProposalReachesTheRequesterNotTheDrain(t *testing.T) {
	f := newFix(t)
	f.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictTierUp, ProposedTier: intake.TierLow}}

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("walk precondition: approval card, got %q", st.OpenAskKind)
	}
	if st.Tier != intake.TierStandard {
		t.Fatalf("the downward proposal must not move the tier by itself (S06.4), got %q", st.Tier)
	}
	if f.planner.reviseCalls != 0 {
		t.Fatalf("the tier objection entered the REVISE drain (%d revise calls) — the planner "+
			"cannot author the band", f.planner.reviseCalls)
	}

	askID, _ := f.openAsk(st.RunID)
	var snapshot string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT snapshot FROM asks WHERE ask_id = ?`, askID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, `"stakes"`) || !strings.Contains(snapshot, `"proposed_lower":"low"`) {
		t.Fatalf("R4.2/R4.3: the approval card must serve the platform-authored stakes block with "+
			"the pending downward proposal (brief §1 R4.3: {tier, origin, plain_reason, "+
			"proposed_lower, can_lower}); snapshot carries neither: %.400s", snapshot)
	}
}
