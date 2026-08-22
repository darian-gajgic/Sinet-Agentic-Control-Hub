package intake_test

// gf3be1_test.go — the P3-GF3-BE1 acceptance battery, committed RED by the
// grounding session per the tests-first pipeline (CONVENTIONS §3 Amendment-A;
// brief P3/briefs/P3-GF3-BE1.md §8). The fixtures and fakes are
// pipeline_test.go's and deepplan_test.go's (same test package); no existing
// test file is modified.
//
// The headline these assert (design note §2 pieces A–E + the floor serve):
// every v3 software/generic slot carries concrete options and a plain-words
// why; the utility seat may decorate each asked question with a task-grounded
// suggestion under the exact fold-by-id containment; a person can skip any
// asked question and the platform takes the served suggestion as an explicit
// assumption; Re-interview re-presents the WHOLE question set with each
// slot's current resolution attached; Re-plan takes many objections at once
// plus free text; and every card says where its questions stop.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf3Suggesting returns a fakePhraser that serves "SUGGEST-<id>" for every
// asked id, plus per-test extras folded in by the caller's fn wrapper.
func gf3Suggesting(extra func(in intake.PhraseInput, res *intake.PhraseResult)) *fakePhraser {
	return &fakePhraser{fn: func(in intake.PhraseInput) (intake.PhraseResult, error) {
		res := intake.PhraseResult{Suggestions: map[string]string{}}
		for _, q := range in.Questions {
			res.Suggestions[q.ID] = "SUGGEST-" + q.ID
		}
		if extra != nil {
			extra(in, &res)
		}
		return res, nil
	}}
}

// gf3ToApproval walks a fresh standard-tier software task to its approval
// card and returns the open ask id.
func gf3ToApproval(t *testing.T, f *fix) (string, *intake.State) {
	t.Helper()
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("walk did not reach the approval card: %q (phase %s)", st.OpenAskKind, st.Phase)
	}
	askID, _ := f.openAsk(st.RunID)
	return askID, st
}

// ---- T1: taxonomy v3 delivery contract (brief R1; design §2.A; S06.5) ----

// TestGF3TaxonomyV3EverySlotCarriesOptionsAndWhy: the S06.5 delivery contract
// — "each with 2–4 labeled options plus free text" — holds on EVERY slot of
// the software and generic v3 seeds, and every slot carries the plain-words
// why line the FE renders. (The Version=="v3" assertion is deliberately
// absent until the OQ-1 sanction lands — brief §8 T1.)
func TestGF3TaxonomyV3EverySlotCarriesOptionsAndWhy(t *testing.T) {
	seeds := intake.SeedTaxonomies()
	for _, fam := range []intake.Family{intake.FamilySoftware, intake.FamilyGeneric} {
		tax := seeds[fam]
		for _, s := range tax.Slots {
			if len(s.Options) < 2 || len(s.Options) > 4 {
				t.Errorf("%s/%s carries %d options, want 2–4 on every v3 slot (S06.5; design §2.A(ii))",
					fam, s.ID, len(s.Options))
			}
			if strings.TrimSpace(s.Why) == "" {
				t.Errorf("%s/%s has no plain-words why line (design §2.A(iii))", fam, s.ID)
			}
		}
	}
}

// ---- T7: the verbatim wall (brief R1) — GREEN guard, must stay green ----

// TestGF3WeightsAndIDsStayVerbatim pins ids, weights, slot count and order
// for both revised sets: the measured/ratified thing the v3 redraft may not
// move.
func TestGF3WeightsAndIDsStayVerbatim(t *testing.T) {
	want := map[intake.Family][]struct {
		id string
		w  int
	}{
		intake.FamilySoftware: {
			{"behavior", 10}, {"terminology", 10}, {"edge_cases", 10},
			{"collection_semantics", 12}, {"comparison_rules", 12}, {"ordering_atomicity", 12},
			{"indices_ranges", 8}, {"output_format", 8}, {"units", 6}, {"numerical_precision", 6},
			{"technology_stack", 11}, {"assets_media", 10}, {"look_feel", 10},
		},
		intake.FamilyGeneric: {
			{"goal", 12}, {"deliverable", 10}, {"scope", 10}, {"inputs", 8},
			{"constraints", 8}, {"audience", 6}, {"quality_bar", 6}, {"deadline", 4},
		},
	}
	seeds := intake.SeedTaxonomies()
	for fam, slots := range want {
		tax := seeds[fam]
		if len(tax.Slots) != len(slots) {
			t.Fatalf("%s carries %d slots, ratified %d — count is verbatim", fam, len(tax.Slots), len(slots))
		}
		for i, w := range slots {
			if tax.Slots[i].ID != w.id || tax.Slots[i].Weight != w.w {
				t.Errorf("%s slot %d = %s/%d, ratified %s/%d — ids, weights and order are verbatim",
					fam, i, tax.Slots[i].ID, tax.Slots[i].Weight, w.id, w.w)
			}
		}
	}
}

// ---- T2: suggestion decoration under containment (brief R3/R5) ----

// TestGF3SuggestionsFoldBySlotIDUnderContainment: the seat's suggestions fold
// by ASKED slot id exactly like Phrased — an unasked id reaches nothing, a
// suggested_option that names no option of its question is dropped (the text
// suggestion kept), and question count/order/ids/options/text stay the
// pipeline's own selection.
func TestGF3SuggestionsFoldBySlotIDUnderContainment(t *testing.T) {
	f := newFix(t)
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	realOption := soft.Slot("technology_stack").Options[0].Value
	f.p.Phraser = gf3Suggesting(func(_ intake.PhraseInput, res *intake.PhraseResult) {
		res.Suggestions["a_slot_nobody_asked"] = "SMUGGLED"
		res.SuggestedOptions = map[string]string{
			"technology_stack":     realOption,       // an existing option value → kept
			"collection_semantics": "no_such_option", // names no option → dropped
		}
	})

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("card = %q, want interview", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)

	// Containment first: the selection is the taxonomy's, untouched.
	wantSlots := soft.Unresolved(nil)
	if len(wantSlots) > 4 {
		wantSlots = wantSlots[:4]
	}
	if len(card.Questions) != len(wantSlots) {
		t.Fatalf("card carries %d questions, want %d — decoration may not change the count",
			len(card.Questions), len(wantSlots))
	}
	for i, q := range card.Questions {
		slot := wantSlots[i]
		if q.ID != slot.ID || q.Text != slot.Question {
			t.Fatalf("question %d = %s/%q, want %s/%q — decoration may not touch id or text",
				i, q.ID, q.Text, slot.ID, slot.Question)
		}
		if q.Suggested != "SUGGEST-"+q.ID {
			t.Errorf("question %s suggested = %q, want the seat's %q folded by slot id",
				q.ID, q.Suggested, "SUGGEST-"+q.ID)
		}
		if q.Suggested == "SMUGGLED" || q.ID == "a_slot_nobody_asked" {
			t.Fatal("a suggestion for a question the pipeline never selected reached the card")
		}
		switch q.ID {
		case "technology_stack":
			if q.SuggestedOption != realOption {
				t.Errorf("technology_stack suggested_option = %q, want the existing option value %q",
					q.SuggestedOption, realOption)
			}
		case "collection_semantics":
			if q.SuggestedOption != "" {
				t.Errorf("collection_semantics suggested_option = %q — a value naming no option must be dropped",
					q.SuggestedOption)
			}
		}
	}
}

// ---- T3: per-slot skip (brief R6/R7; design §2.C; S06.5 assumption arm) ----

// TestGF3PerSlotSkipConvertsToAssumptionCarryingTheServedSuggestion:
// {id, skip:true} converts THAT slot to an explicit assumption via the
// existing resolution machinery, carrying the card's served suggestion; the
// clearance recomputes exactly as landed; the slot is never re-asked; the
// assumption reaches the approval centerpiece and the recap, origin-labeled.
// force_proceed stays untouched.
func TestGF3PerSlotSkipConvertsToAssumptionCarryingTheServedSuggestion(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.p.Phraser = gf3Suggesting(nil)
	tax := intake.SeedTaxonomies()[intake.FamilySoftware]

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	askID, card := f.openAsk(st.RunID)
	skipped := card.Questions[0].ID

	ans := intake.Answer{Answers: []intake.SlotAnswer{{ID: skipped, Skip: true}}}
	for _, q := range card.Questions[1:] {
		ans.Answers = append(ans.Answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
	}
	raw, err := json.Marshal(ans)
	if err != nil {
		t.Fatal(err)
	}
	st, err = f.p.Answer(ctx, "u1", askID, raw)
	if err != nil {
		t.Fatalf("skip answer refused: %v — {id, skip:true} is the per-slot assumption arm (design §2.C)", err)
	}

	var got *intake.SlotResolution
	for i := range st.Resolutions {
		if st.Resolutions[i].SlotID == skipped {
			got = &st.Resolutions[i]
		}
	}
	if got == nil || got.How != intake.ResolvedAssumption {
		t.Fatalf("skipped slot resolution = %+v, want an explicit assumption (S06.5 third arm)", got)
	}
	if !strings.Contains(got.Assumption, "SUGGEST-"+skipped) {
		t.Errorf("skip assumption = %q, want it to carry the served suggestion %q",
			got.Assumption, "SUGGEST-"+skipped)
	}
	if st.ForceProceeded {
		t.Error("a per-slot skip must not trip the whole-interview force_proceed arm")
	}
	resolved := map[string]bool{}
	for _, r := range st.Resolutions {
		resolved[r.SlotID] = true
	}
	if want := tax.Clearance(resolved); st.Clearance != want {
		t.Errorf("clearance = %v, want the landed computation's %v — skip changes no clearance math", st.Clearance, want)
	}

	// The skipped slot is resolved, so it is NEVER re-picked (operator F4).
	if st.OpenAskKind == intake.CardInterview {
		_, next := f.openAsk(st.RunID)
		for _, q := range next.Questions {
			if q.ID == skipped {
				t.Fatalf("slot %q was skipped and asked again — a resolved slot is never re-picked", skipped)
			}
		}
		st = f.answerInterviewToFloor("u1", st.RunID)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("walk did not reach approval: %q", st.OpenAskKind)
	}

	// The assumption lands on the centerpiece and in the recap, labeled.
	_, appr := f.openAsk(st.RunID)
	foundAssumption := false
	for _, a := range appr.Approval.Layer1.Assumptions {
		if strings.Contains(a.Text, "SUGGEST-"+skipped) {
			foundAssumption = true
		}
	}
	if !foundAssumption {
		t.Error("the skipped slot's suggestion did not reach the approval card's assumptions centerpiece (S06.9)")
	}
	foundRecap := false
	if u := appr.Approval.Layer1.Understood; u != nil {
		for _, it := range u.Items {
			if it.SlotID == skipped && it.How == intake.ResolvedAssumption {
				foundRecap = true
			}
		}
	}
	if !foundRecap {
		t.Error("the skipped slot is missing from the understood recap, or mislabeled")
	}
}

// TestGF3SkipRefusals: skip may not smuggle a value, and stays inside the
// card's asked set.
func TestGF3SkipRefusals(t *testing.T) {
	ctx := context.Background()

	t.Run("skip with a value refuses", func(t *testing.T) {
		f := newFix(t)
		st := f.start(stdRequest())
		f.admit(st.RunID)
		f.advance(st.TaskID)
		askID, card := f.openAsk(st.RunID)
		raw, _ := json.Marshal(intake.Answer{Answers: []intake.SlotAnswer{
			{ID: card.Questions[0].ID, Value: "x", Skip: true},
		}})
		if _, err := f.p.Answer(ctx, "u1", askID, raw); !errors.Is(err, intake.ErrBadAnswer) {
			t.Fatalf("skip+value answered %v, want ErrBadAnswer — the two arms are exclusive", err)
		}
	})

	t.Run("skip of an unasked slot refuses", func(t *testing.T) {
		f := newFix(t)
		st := f.start(stdRequest())
		f.admit(st.RunID)
		f.advance(st.TaskID)
		askID, card := f.openAsk(st.RunID)
		unasked := "units" // weight 6 — never on the first standard card
		for _, q := range card.Questions {
			if q.ID == unasked {
				t.Fatalf("fixture drift: %q is on the first card", unasked)
			}
		}
		raw, _ := json.Marshal(intake.Answer{Answers: []intake.SlotAnswer{{ID: unasked, Skip: true}}})
		if _, err := f.p.Answer(ctx, "u1", askID, raw); !errors.Is(err, intake.ErrBadAnswer) {
			t.Fatalf("unasked skip answered %v, want ErrBadAnswer (asked-set containment)", err)
		}
	})
}

// ---- T4: the re-interview review card (brief R8/R9; design §2.D) ----

// TestGF3ReinterviewIssuesTheFullReviewCard: Re-interview returns to Stage 1
// with artifacts intact (S06.9) and re-presents the WHOLE question set —
// every slot, taxonomy order, each resolved one carrying its CURRENT
// resolution so the surface renders review-and-adjust instead of re-asking
// blind. The NORMAL interview selection is untouched (pipeline_test.go's ≤4
// pin keeps guarding it).
func TestGF3ReinterviewIssuesTheFullReviewCard(t *testing.T) {
	f := newFix(t)
	askID, st := gf3ToApproval(t, f)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionReInterview})
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("after Re-interview: %q, want the interview review card", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	tax := intake.SeedTaxonomies()[intake.FamilySoftware]
	if len(card.Questions) != len(tax.Slots) {
		t.Fatalf("review card carries %d questions, want ALL %d family slots (design §2.D)",
			len(card.Questions), len(tax.Slots))
	}
	res := map[string]intake.SlotResolution{}
	for _, r := range st.Resolutions {
		res[r.SlotID] = r
	}
	for i, q := range card.Questions {
		if q.ID != tax.Slots[i].ID {
			t.Fatalf("review card question %d = %q, want taxonomy order %q", i, q.ID, tax.Slots[i].ID)
		}
		r, resolved := res[q.ID]
		switch {
		case resolved && q.Resolution == nil:
			t.Errorf("slot %q is settled but the review card attaches no resolution", q.ID)
		case resolved && (q.Resolution.How != r.How || q.Resolution.Value != r.Value ||
			q.Resolution.Assumption != r.Assumption):
			t.Errorf("slot %q resolution = %+v, state holds %+v — the card must serve the record",
				q.ID, q.Resolution, r)
		case !resolved && q.Resolution != nil:
			t.Errorf("slot %q is unresolved but carries %+v", q.ID, q.Resolution)
		}
		if q.Why != tax.Slots[i].Why {
			t.Errorf("slot %q why = %q, want the taxonomy's %q", q.ID, q.Why, tax.Slots[i].Why)
		}
	}

	// Adjusting one answer merges through the landed ReviseInterview path.
	var reasons []string
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		reasons = append(reasons, in.Reason)
		return baseRevise(in), nil
	}
	changed := card.Questions[0].ID
	reviewAsk, _ := f.openAsk(st.RunID)
	st = f.answer("u1", reviewAsk, intake.Answer{Answers: []intake.SlotAnswer{{ID: changed, Value: "changed"}}})
	if len(reasons) != 1 || reasons[0] != intake.ReviseInterview {
		t.Fatalf("revise reasons = %v, want exactly one %q merge", reasons, intake.ReviseInterview)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after the adjusted answer: %q, want a fresh approval card", st.OpenAskKind)
	}
}

// ---- T5: multi-contest Re-plan (brief R10; design §2.E; S06.9) ----

// TestGF3MultiContestReplanMergesIntoOneRevision: a SET of contested targets
// plus one free-text note merge into ONE bounded ReviseContest revision — one
// revise call, spine re-run, no second paid critique (§14 reading 5).
func TestGF3MultiContestReplanMergesIntoOneRevision(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	askID, st := gf3ToApproval(t, f)

	var got []intake.ReviseInput
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		got = append(got, in)
		return baseRevise(in), nil
	}
	raw, _ := json.Marshal(intake.Answer{
		Action: intake.ActionRePlan,
		Contests: []intake.ContestRef{
			{Target: "AC-1", Note: "wrong"},
			{Target: "S-2"},
		},
		Note: "and generally: softer colors, in my own words",
	})
	st, err := f.p.Answer(ctx, "u1", askID, raw)
	if err != nil {
		t.Fatalf("multi-contest replan refused: %v — contests[] is the S06.9 structured SET entry", err)
	}
	if len(got) != 1 {
		t.Fatalf("revise calls = %d, want exactly ONE bounded delta re-plan", len(got))
	}
	if got[0].Reason != intake.ReviseContest {
		t.Fatalf("revise reason = %q, want %q", got[0].Reason, intake.ReviseContest)
	}
	findings := strings.Join(got[0].Findings, "\n")
	for _, want := range []string{"AC-1: wrong", "S-2", "and generally: softer colors, in my own words"} {
		if !strings.Contains(findings, want) {
			t.Errorf("findings %q are missing %q — every contest and the target-less note merge in",
				got[0].Findings, want)
		}
	}
	if f.critic.calls != 1 {
		t.Errorf("critique calls = %d after the contested re-plan, want 1 — no second paid pass (S06.10)", f.critic.calls)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after the re-plan: %q, want a fresh approval card", st.OpenAskKind)
	}
	doc := f.ledgerDoc(st.TaskID)
	var decisions []string
	for _, d := range doc.Decisions {
		decisions = append(decisions, d.Text)
	}
	all := strings.Join(decisions, "\n")
	if !strings.Contains(all, "AC-1") || !strings.Contains(all, "S-2") {
		t.Errorf("the ledger decision names no contested targets: %q", decisions)
	}
}

// TestGF3ReplanBackCompatAndRefusals: the legacy single contest stays
// byte-compatible; an empty re-plan still refuses; a note-only re-plan is the
// free-text channel on its own (brief OQ-4).
func TestGF3ReplanBackCompatAndRefusals(t *testing.T) {
	ctx := context.Background()

	t.Run("single contest still works", func(t *testing.T) {
		f := newFix(t)
		askID, _ := gf3ToApproval(t, f)
		var got []intake.ReviseInput
		f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
			got = append(got, in)
			return baseRevise(in), nil
		}
		f.answer("u1", askID, intake.Answer{
			Action:  intake.ActionRePlan,
			Contest: &intake.ContestRef{Target: "AC-1", Note: "off"},
		})
		if len(got) != 1 || strings.Join(got[0].Findings, "\n") != "AC-1: off" {
			t.Fatalf("single-contest revise = %+v, want the landed behavior byte-identical", got)
		}
	})

	t.Run("an empty re-plan refuses", func(t *testing.T) {
		f := newFix(t)
		askID, _ := gf3ToApproval(t, f)
		raw, _ := json.Marshal(intake.Answer{Action: intake.ActionRePlan})
		if _, err := f.p.Answer(ctx, "u1", askID, raw); !errors.Is(err, intake.ErrBadAnswer) {
			t.Fatalf("empty re-plan answered %v, want ErrBadAnswer", err)
		}
	})

	t.Run("a note-only re-plan is the free-text channel", func(t *testing.T) {
		f := newFix(t)
		askID, _ := gf3ToApproval(t, f)
		var got []intake.ReviseInput
		f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
			got = append(got, in)
			return baseRevise(in), nil
		}
		raw, _ := json.Marshal(intake.Answer{Action: intake.ActionRePlan, Note: "do it my way"})
		if _, err := f.p.Answer(ctx, "u1", askID, raw); err != nil {
			t.Fatalf("note-only re-plan refused: %v (brief OQ-4)", err)
		}
		if len(got) != 1 || !strings.Contains(strings.Join(got[0].Findings, "\n"), "do it my way") {
			t.Fatalf("note-only revise = %+v, want the note as a target-less finding", got)
		}
	})
}

// ---- T6a: the served clearance floor (brief R11) ----

// TestGF3CardsServeTheClearanceFloor: the tier's ⚙ clearance floor rides
// every intake card next to the computed clearance — served, never derived,
// value and consumption untouched (verified unserved at grounding).
func TestGF3CardsServeTheClearanceFloor(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	_, card := f.openAsk(st.RunID)
	if card.ClearanceFloor != 75 {
		t.Errorf("interview card clearance_floor = %v, want the standard tier's ⚙ 75 served beside clearance",
			card.ClearanceFloor)
	}
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("walk did not reach approval: %q", st.OpenAskKind)
	}
	_, appr := f.openAsk(st.RunID)
	if appr.ClearanceFloor != 75 {
		t.Errorf("approval card clearance_floor = %v, want 75 — one issue site serves every kind",
			appr.ClearanceFloor)
	}
}
