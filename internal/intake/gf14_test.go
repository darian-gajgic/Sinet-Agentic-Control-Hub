package intake_test

// gf14_test.go — P3-GF14, the executor-materialized half of §7: the properties
// the named acceptance tests state by example.
//
//   - R1 is a PER-CANCELLATION-POINT invariant, not one staged moment: past the
//     resume commit, the drive runs to its own end from WHEREVER the caller
//     happens to die. The table below cancels the request inside each seam the
//     drive calls, in drive order.
//   - R3's suppression is a property over EVERY location the platform scans for
//     a currency; the end-to-end negative lives here, the location-by-location
//     sweep beside the backstop itself (gf14currency_internal_test.go).
//   - R4's stakes block is the one truth a card serves about how careful the
//     platform is being: tier, who set it, and whether the requester's own
//     downward move is legal.
//
// Every test here is hermetic: the seams are this package's fakes, no engine
// and no paid call.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// cancelSpotCheck / cancelUtility are the drive's later seams as abort points:
// each runs the caller's cancel on its first call and then answers honestly.
type cancelSpotCheck struct{ cancel context.CancelFunc }

func (s cancelSpotCheck) Check(context.Context, intake.Pair) ([]string, error) {
	s.cancel()
	return nil, nil
}

type cancelUtility struct{ cancel context.CancelFunc }

func (u cancelUtility) Help(_ context.Context, pair intake.Pair) (intake.HelpBlock, error) {
	u.cancel()
	return intake.HelpBlock{What: "w", Wrong: "x", Recommend: "y"}, nil
}

// cancelCritic cancels the caller while judging — the post-spine point.
type cancelCritic struct{ cancel context.CancelFunc }

func (c cancelCritic) Critique(context.Context, intake.Pair) (intake.Verdict, error) {
	c.cancel()
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}

func (c cancelCritic) Recheck(context.Context, intake.Pair, []string) (intake.Verdict, error) {
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}

// TestGF14DriveOutlivesItsCallerAtEveryCancellationPoint is R1 as the property
// it is: the answered card resumed the run, so the drive owes that run its own
// ending — the next gate — no matter which seam the caller's death lands in.
func TestGF14DriveOutlivesItsCallerAtEveryCancellationPoint(t *testing.T) {
	points := []struct {
		name string
		arm  func(f *fix, cancel context.CancelFunc)
	}{
		{"in the plan draft (the witnessed point)", func(f *fix, cancel context.CancelFunc) {
			f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
				cancel()
				return basePair(in), nil
			}
		}},
		{"in the spine's advisory spot-check", func(f *fix, cancel context.CancelFunc) {
			f.p.SpotCheck = cancelSpotCheck{cancel: cancel}
		}},
		{"in the Stage-3 critique", func(f *fix, cancel context.CancelFunc) {
			f.p.Critic = cancelCritic{cancel: cancel}
		}},
		{"in the approval card's help seat", func(f *fix, cancel context.CancelFunc) {
			f.p.Utility = cancelUtility{cancel: cancel}
		}},
	}
	for _, point := range points {
		t.Run(point.name, func(t *testing.T) {
			f := newFix(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			point.arm(f, cancel)

			st := f.start(stdRequest())
			f.admit(st.RunID)
			st = f.advance(st.TaskID)

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
					t.Fatalf("the drive died with its caller at this point: %v", err)
				}
			}

			if ctx.Err() == nil {
				t.Fatal("staging defect: the armed seam never ran, so nothing was cancelled")
			}
			if st.OpenAskKind != intake.CardApproval {
				t.Fatalf("card = %q after the detached drive, want the approval gate", st.OpenAskKind)
			}
			if got := f.runState(st.RunID); got != run.StateParked {
				t.Fatalf("run state = %q, want parked on its gate — never crashed for the ladder", got)
			}
			if got := f.planner.draftCalls; got != 1 {
				t.Fatalf("draft ran %d times, want exactly 1 (no re-pay, no ladder fork)", got)
			}
		})
	}
}

// TestGF14CurrencyBackstopStandsDownWhenTheRequestNamesOne is R3's companion
// negative: a settled fact stays settled, so a currency written in the request
// suppresses the backstop entirely — no card, no assumption, no second asking.
func TestGF14CurrencyBackstopStandsDownWhenTheRequestNamesOne(t *testing.T) {
	f := newFix(t)
	req := stdRequest()
	req.Title = "A one-page price list for my candle shop"
	req.Text = "A one-page price list for my candle shop. Six candles, each with its name and its price — " +
		"the cheapest is €8 and I will give you the rest."

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
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)

	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("card = %q, want the approval card — the currency is already named, so nothing may be asked", st.OpenAskKind)
	}
	pair, err := f.p.CurrentPair(context.Background(), st.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range pair.Spec.Clarifications {
		if strings.Contains(strings.ToLower(c), "currency") {
			t.Fatalf("the backstop asked about a currency the request already named: %q", c)
		}
	}
}

// TestGF14CurrencyPointIsAskedOnceAndSettles is R3's other half: the open point
// is asked ONCE. Answering it settles it on the record, and the next emission
// carries the answer as a listed assumption instead of asking again.
func TestGF14CurrencyPointIsAskedOnceAndSettles(t *testing.T) {
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
	if len(card.Questions) != 1 {
		t.Fatalf("the clarification card carries %d questions, want the one open point", len(card.Questions))
	}
	q := card.Questions[0]
	if len(q.Options) < 2 {
		t.Fatalf("the currency question must offer a finite choice set, got %d options", len(q.Options))
	}
	if q.Why == "" {
		t.Fatal("a platform-authored question says why it is worth answering")
	}
	for _, o := range q.Options {
		if o.Label == "" || o.Value == "" {
			t.Fatalf("an offered option must carry both a label and a value: %+v", o)
		}
	}

	// The requester answers with the option the platform offered.
	st = f.answer("u1", askID, intake.Answer{Answers: []intake.SlotAnswer{{ID: q.ID, Value: q.Options[0].Value}}})
	if st.OpenAskKind == intake.CardClarification {
		_, again := f.openAsk(st.RunID)
		t.Fatalf("the settled point was asked a second time: %+v", again.Questions)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("card = %q after the answer, want the approval card", st.OpenAskKind)
	}
}

// TestGF14DownwardProposalRidesAnyVerdictKind is R4.2's general form: the tier
// opinion travels on `proposed_tier` whatever the verdict was, so a REVISE
// round fixes what it was asked to fix while the stakes question goes to the
// person who owns it. The tier itself never moves down on its own (S06.4).
func TestGF14DownwardProposalRidesAnyVerdictKind(t *testing.T) {
	f := newFix(t)
	f.critic.verdicts = []intake.Verdict{{
		Kind:         intake.VerdictRevise,
		Findings:     []string{"AC-2 has no done-when a machine could check"},
		ProposedTier: intake.TierLow,
	}}

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("walk precondition: approval card, got %q", st.OpenAskKind)
	}
	if st.Tier != intake.TierStandard {
		t.Fatalf("tier = %q — a proposal is not a move (S06.4: no downward verdict exists)", st.Tier)
	}
	if f.planner.reviseCalls != 1 {
		t.Fatalf("%d revise calls, want the one round the findings earned", f.planner.reviseCalls)
	}

	_, card := f.openAsk(st.RunID)
	if card.Stakes == nil || card.Stakes.ProposedLower != intake.TierLow {
		t.Fatalf("the pending downward proposal must reach the requester on the stakes block: %+v", card.Stakes)
	}
	if !card.Stakes.CanLower {
		t.Fatal("a served proposal the requester cannot act on is a dead end")
	}
}

// TestGF14StakesBlockNamesWhoSetTheTier is R4.3/R4.4: every card a person
// decides at carries the whole stakes truth, and the origin is the record's,
// never a guess.
func TestGF14StakesBlockNamesWhoSetTheTier(t *testing.T) {
	t.Run("the classifier's reading", func(t *testing.T) {
		f := newFix(t)
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)

		_, card := f.openAsk(st.RunID)
		if card.Stakes == nil {
			t.Fatal("the interview card carries no stakes block")
		}
		if card.Stakes.Tier != card.Tier {
			t.Fatalf("stakes block tier %q disagrees with the card's own %q", card.Stakes.Tier, card.Tier)
		}
		if card.Stakes.Origin != intake.TierSourceClassifier {
			t.Fatalf("origin = %q, want %q", card.Stakes.Origin, intake.TierSourceClassifier)
		}
		if !card.Stakes.CanLower {
			t.Fatal("a standard-tier task above no floor can be lowered — the card must say so")
		}
		if card.Stakes.PlainReason == "" {
			t.Fatal("the stakes block owes a plain-words reason")
		}
		for _, token := range []string{"S06", "§", "Spec/", "tier"} {
			if strings.Contains(card.Stakes.PlainReason, token) {
				t.Errorf("the served reason carries the platform's own vocabulary (%q): %q", token, card.Stakes.PlainReason)
			}
		}
	})

	t.Run("the fail-closed posture says so", func(t *testing.T) {
		f := newFix(t)
		f.class.err = errors.New("cannot read this request")
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)
		if st.OpenAskKind != intake.CardFamily {
			t.Fatalf("walk precondition: family card, got %q", st.OpenAskKind)
		}

		// The family card asks what is unresolved and carries no stakes claim;
		// the interview card that follows carries the posture and names it.
		askID, _ := f.openAsk(st.RunID)
		f.class.err = nil
		f.class.prop = intake.TriageProposal{}
		st = f.answer("u1", askID, intake.Answer{Choice: string(intake.FamilyContent)})
		if st.Tier != intake.TierHigh {
			t.Fatalf("tier = %q, want high — a still-abstaining classifier leaves the posture standing", st.Tier)
		}
		_, card := f.openAsk(st.RunID)
		if card.Stakes == nil || card.Stakes.Origin != intake.TierSourceFailClosed {
			t.Fatalf("stakes block = %+v, want the fail-closed origin", card.Stakes)
		}
	})

	t.Run("a floor owns the tier and closes the door", func(t *testing.T) {
		f := newFix(t)
		f.class.prop.Floors = []intake.FloorReason{{Class: "new_spend", Source: "classifier", Detail: "buys a domain"}}
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)

		_, card := f.openAsk(st.RunID)
		if card.Stakes == nil || card.Stakes.Origin != intake.TierSourceFloor {
			t.Fatalf("stakes block = %+v, want the floor origin", card.Stakes)
		}
		if card.Stakes.CanLower {
			t.Fatal("a task held at its floor cannot be lowered — the card must not offer it")
		}
		if !strings.Contains(card.Stakes.PlainReason, "spends money") {
			t.Fatalf("the floor's reason must name what tripped it in plain words: %q", card.Stakes.PlainReason)
		}
	})
}

// TestGF14LowerStakesRefusesWhatTheVerbRefuses is R4.5's other half: the door
// is the verb's, so it takes the verb's walls with it.
func TestGF14LowerStakesRefusesWhatTheVerbRefuses(t *testing.T) {
	f := newFix(t)
	f.class.prop.Tier = intake.TierHigh
	f.class.prop.Floors = []intake.FloorReason{{Class: "credential_touch", Source: "classifier", Detail: "reads a saved login"}}

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("walk precondition: approval card, got %q", st.OpenAskKind)
	}

	askID, _ := f.openAsk(st.RunID)
	_, err := f.p.Answer(context.Background(), "u1", askID,
		[]byte(`{"action":"lower_stakes","tier":"low"}`))
	if !errors.Is(err, intake.ErrBelowFloor) {
		t.Fatalf("lowering below a floor = %v, want the floor refusal", err)
	}
	if _, err := f.p.Answer(context.Background(), "u1", askID,
		[]byte(`{"action":"lower_stakes"}`)); !errors.Is(err, intake.ErrBadAnswer) {
		t.Fatalf("a lowering that names no tier = %v, want the bad-answer refusal", err)
	}
	st2, err := f.p.LoadState(context.Background(), st.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Tier != intake.TierHigh || st2.OpenAskID != askID {
		t.Fatalf("a refused lowering moved something: tier %q, ask %q", st2.Tier, st2.OpenAskID)
	}
}

// TestGF14LoweringRefreshesTheServedCard is R4.5's snapshot half: the ask row's
// card — which is what the surfaces read, the step-up demand included — must
// carry the tier the task actually has after the lowering.
func TestGF14LoweringRefreshesTheServedCard(t *testing.T) {
	f := newFix(t)
	f.class.prop.Tier = intake.TierHigh

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)

	st, err := f.p.Answer(context.Background(), "u1", askID,
		[]byte(`{"action":"lower_stakes","tier":"standard"}`))
	if err != nil {
		t.Fatalf("lower_stakes: %v", err)
	}
	if st.TierSource != intake.TierSourceRequester {
		t.Fatalf("tier source = %q after the requester's own move, want %q", st.TierSource, intake.TierSourceRequester)
	}

	_, card := f.openAsk(st.RunID)
	if card.Tier != intake.TierStandard {
		t.Fatalf("the served card still says %q — the step-up demand reads this snapshot", card.Tier)
	}
	if card.Stakes == nil || card.Stakes.Tier != intake.TierStandard ||
		card.Stakes.Origin != intake.TierSourceRequester {
		t.Fatalf("the served stakes block did not follow the lowering: %+v", card.Stakes)
	}
	if card.Approval == nil {
		t.Fatal("the approval card lost its body on the re-serve")
	}
	doc := f.ledgerDoc(st.TaskID)
	found := false
	for _, d := range doc.Decisions {
		if strings.Contains(d.Text, "lowered the stakes") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the human decision was not recorded: %+v", doc.Decisions)
	}
}
