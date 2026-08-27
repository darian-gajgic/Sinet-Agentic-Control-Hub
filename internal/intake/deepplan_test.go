package intake_test

// deepplan_test.go — the P3-RW-12 acceptance battery for Deep-Plan interview
// depth (brief §7 T3, T5, T6, T7, T8, T9a, T10; Spec S06.4/S06.5/S06.6,
// S06.10, S12.4).
//
// The headline these assert: every family is interviewed from its OWN
// question set; a per-request phrasing seat may reword what the taxonomy
// chose to ask and can neither add, drop nor merge a question; every ask
// carries the platform's own record of what it understood so far; and an
// absent seat costs the requester nothing. The fixtures and fakes are
// pipeline_test.go's and familygate_test.go's (same test package).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// ---- The phrase seam fake ----

type fakePhraser struct {
	calls  int
	last   intake.PhraseInput
	fn     func(in intake.PhraseInput) (intake.PhraseResult, error)
	result intake.PhraseResult
	err    error
}

func (f *fakePhraser) PhraseAndSummarize(_ context.Context, in intake.PhraseInput) (intake.PhraseResult, error) {
	f.calls++
	f.last = in
	if f.fn != nil {
		return f.fn(in)
	}
	return f.result, f.err
}

// registryFamily hands the pipeline a project whose capture declares fam,
// optionally with slots the registry already knows.
func registryFamily(fam intake.Family, resolved map[string]string) *fakeRegistry {
	return &fakeRegistry{ok: true, slice: intake.RegistrySlice{
		Project: "proj", Ref: "proj@capture-v1", Family: fam, ResolvedSlots: resolved,
	}}
}

// askCount counts every ask row on a run (open or closed).
func askCount(t *testing.T, f *fix, runID string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM asks WHERE run_id = ?`, runID).Scan(&n); err != nil {
		t.Fatalf("count asks: %v", err)
	}
	return n
}

// TestEveryFamilyUsesItsOwnSetNoFallbackDisclosure (§7 T3; R2): research,
// content, data and chore each resolve to their OWN question set, so the
// disclosed generic fallback stops firing for every value in the vocabulary —
// while the fallback MACHINERY stays intact for a genuinely unseeded family.
func TestEveryFamilyUsesItsOwnSetNoFallbackDisclosure(t *testing.T) {
	for _, fam := range []intake.Family{
		intake.FamilyResearch, intake.FamilyContent, intake.FamilyData, intake.FamilyChore,
	} {
		t.Run(string(fam), func(t *testing.T) {
			f := newFix(t)
			f.p.Classifier = nil
			f.p.Registry = registryFamily(fam, nil)

			st := f.start(stdRequest())
			if st.Family != fam {
				t.Fatalf("family = %q, want %q", st.Family, fam)
			}
			if st.TaxonomyID != string(fam) {
				t.Errorf("taxonomy = %q, want the %s family's own set (R2)", st.TaxonomyID, fam)
			}
			seed, ok := intake.SeedTaxonomies()[fam]
			if !ok {
				t.Fatalf("no seeded set for %s", fam)
			}
			if st.TaxonomyVersion != seed.Version {
				t.Errorf("taxonomy version = %q, want %q", st.TaxonomyVersion, seed.Version)
			}
			for _, txt := range platformDecisionTexts(t, f, st.TaskID) {
				if strings.Contains(txt, "no question set is seeded") {
					t.Errorf("the disclosed fallback still fires for %s: %q", fam, txt)
				}
			}

			// And the questions actually come from that set.
			f.admit(st.RunID)
			st = f.advance(st.TaskID)
			if st.OpenAskKind != intake.CardInterview {
				t.Fatalf("first card = %q, want interview", st.OpenAskKind)
			}
			_, card := f.openAsk(st.RunID)
			for _, q := range card.Questions {
				if seed.Slot(q.ID) == nil {
					t.Errorf("question %q is not a %s slot", q.ID, fam)
				}
			}
		})
	}

	t.Run("machinery retained for a genuinely unseeded family", func(t *testing.T) {
		f := newFix(t)
		f.p.Classifier = nil
		custom := intake.SeedTaxonomies()
		delete(custom, intake.FamilyResearch)
		f.p.Taxonomies = custom
		f.p.Registry = registryFamily(intake.FamilyResearch, nil)

		st := f.start(stdRequest())
		if st.Family != intake.FamilyResearch {
			t.Errorf("family = %q, want research (the TRUE family stays durable)", st.Family)
		}
		if st.TaxonomyID != "generic" {
			t.Errorf("taxonomy = %q, want generic (this map seeds no research set)", st.TaxonomyID)
		}
		var disclosed bool
		for _, txt := range platformDecisionTexts(t, f, st.TaskID) {
			if strings.Contains(txt, "no question set is seeded") {
				disclosed = true
			}
		}
		if !disclosed {
			t.Errorf("the fallback was not disclosed: %v", platformDecisionTexts(t, f, st.TaskID))
		}
	})
}

// TestPhrasingFoldsOnlyAskedSlots (§7 T5; R6): the containment is STRUCTURAL.
// A hostile seat that answers for a slot nobody asked, skips one that was
// asked, and would like to change the options gets exactly one thing: the
// wording of the questions the taxonomy already chose.
func TestPhrasingFoldsOnlyAskedSlots(t *testing.T) {
	f := newFix(t)
	ph := &fakePhraser{fn: func(in intake.PhraseInput) (intake.PhraseResult, error) {
		if len(in.Questions) < 3 {
			return intake.PhraseResult{}, errors.New("fixture expects a full card")
		}
		return intake.PhraseResult{
			Phrasings: map[string]string{
				in.Questions[0].ID: "PHRASED-0",
				in.Questions[1].ID: "PHRASED-1",
				// in.Questions[2] deliberately omitted.
				"a_slot_nobody_asked": "HOSTILE — this question was never selected",
				"behavior":            "HOSTILE — duplicate of a slot that may not be on this card",
			},
			Summary: "SUMMARY-PROSE",
		}, nil
	}}
	f.p.Phraser = ph

	st := f.start(stdRequest()) // the fixture classifier: software, standard
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("card = %q, want interview", st.OpenAskKind)
	}
	if ph.calls != 1 {
		t.Errorf("phrase calls = %d, want exactly 1 per card (OQ2)", ph.calls)
	}

	// Read the card back from the ASK ROW — the durable record is what must
	// carry both texts, not an in-memory struct.
	_, card := f.openAsk(st.RunID)
	tax := intake.SeedTaxonomies()[intake.FamilySoftware]
	want := tax.Unresolved(nil)
	if len(want) > 4 {
		want = want[:4]
	}
	if len(card.Questions) != len(want) {
		t.Fatalf("card carries %d questions, want %d — phrasing may not change the count",
			len(card.Questions), len(want))
	}
	for i, q := range card.Questions {
		slot := want[i]
		if q.ID != slot.ID {
			t.Fatalf("question %d id = %q, want %q (the pipeline's own selection)", i, q.ID, slot.ID)
		}
		if q.Text != slot.Question {
			t.Errorf("question %d canonical text was overwritten: %q", i, q.Text)
		}
		if !reflect.DeepEqual(q.Options, slot.Options) {
			t.Errorf("question %d options changed: %+v want %+v", i, q.Options, slot.Options)
		}
		if q.ID == "a_slot_nobody_asked" {
			t.Fatal("a question the pipeline never selected reached the card")
		}
	}
	if card.Questions[0].Phrased != "PHRASED-0" || card.Questions[1].Phrased != "PHRASED-1" {
		t.Errorf("phrasings not folded: %q / %q", card.Questions[0].Phrased, card.Questions[1].Phrased)
	}
	if card.Questions[2].Phrased != "" {
		t.Errorf("question 2 was not phrased by the seat but carries %q — a missing id keeps verbatim text",
			card.Questions[2].Phrased)
	}
	if card.Understood == nil || card.Understood.Text != "SUMMARY-PROSE" {
		t.Errorf("understood block = %+v, want the seat's summary prose", card.Understood)
	}
	// The seat is offered exactly the selection, and nothing about the slots
	// that were passed over.
	if len(ph.last.Questions) != len(want) {
		t.Errorf("seat was offered %d questions, want the %d selected", len(ph.last.Questions), len(want))
	}
	if ph.last.Family != intake.FamilySoftware || ph.last.Tier != st.Tier {
		t.Errorf("seat input family/tier = %q/%q", ph.last.Family, ph.last.Tier)
	}
}

// TestPhrasingAbsenceIsVerbatimZeroClicks (§7 T6; R12): no seat, or a broken
// seat, costs the requester nothing — the same questions, in the same words,
// on the same number of cards, with the deterministic understanding block
// still present and no invented prose.
func TestPhrasingAbsenceIsVerbatimZeroClicks(t *testing.T) {
	card := func(seam intake.Phraser) (intake.Card, int) {
		f := newFix(t)
		f.p.Classifier = nil
		f.p.Registry = registryFamily(intake.FamilySoftware, map[string]string{"units": "millimetres"})
		f.p.Phraser = seam
		st := f.start(stdRequest())
		f.admit(st.RunID)
		st = f.advance(st.TaskID)
		if st.OpenAskKind != intake.CardInterview {
			t.Fatalf("card = %q, want interview", st.OpenAskKind)
		}
		_, c := f.openAsk(st.RunID)
		return c, askCount(t, f, st.RunID)
	}

	base, baseAsks := card(nil)
	// The registry-resolved slot, plus the ones the v4 set settles without ever
	// asking — those resolve at interview entry and are shown as confirmable
	// assumptions from the first card (P3-GF7 R3; harvest H10).
	wantItems := 1 + gf7NeverAskedCount(intake.FamilySoftware)
	if base.Understood == nil || len(base.Understood.Items) != wantItems {
		t.Fatalf("understood items = %+v, want the registry-resolved slot plus the %d the platform settles itself (R8)",
			base.Understood, wantItems-1)
	}
	if base.Understood.Text != "" {
		t.Errorf("understood prose = %q with no seat — deterministic items are never dressed as model prose", base.Understood.Text)
	}
	for _, q := range base.Questions {
		if q.Phrased != "" {
			t.Errorf("question %q carries a phrasing with no seat wired", q.ID)
		}
	}

	broken, brokenAsks := card(&fakePhraser{err: errors.New("stack absent")})
	if !reflect.DeepEqual(broken.Questions, base.Questions) {
		t.Errorf("an erroring seat changed the questions:\n got %+v\nwant %+v", broken.Questions, base.Questions)
	}
	if !reflect.DeepEqual(broken.Understood, base.Understood) {
		t.Errorf("an erroring seat changed the understanding block: %+v", broken.Understood)
	}
	if brokenAsks != baseAsks {
		t.Errorf("ask rows = %d with a broken seat, want %d — a degraded seat never adds a click", brokenAsks, baseAsks)
	}
}

// TestUnderstoodAccumulatesAcrossRounds (§7 T7; R8/R9): the understanding
// block grows with the record — registry-supplied, requester-answered, and
// finally the force-proceed conversions, each labelled with how it got there.
func TestUnderstoodAccumulatesAcrossRounds(t *testing.T) {
	f := newFix(t)
	f.p.Registry = registryFamily(intake.FamilySoftware, map[string]string{"units": "millimetres"})

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	askID, card := f.openAsk(st.RunID)
	// Round 1 holds the registry slot plus the slots the v4 set settles itself
	// (P3-GF7 R3), and the block is in TAXONOMY order, so the registry item is
	// found by id rather than by position.
	settled := gf7NeverAskedCount(intake.FamilySoftware)
	if card.Understood == nil || len(card.Understood.Items) != 1+settled {
		t.Fatalf("round 1 understood = %+v, want the registry slot plus the %d settled ones", card.Understood, settled)
	}
	round1 := map[string]intake.UnderstoodItem{}
	for _, it := range card.Understood.Items {
		round1[it.SlotID] = it
	}
	if round1["units"].How != intake.ResolvedRegistry {
		t.Errorf("registry item = %+v", round1["units"])
	}
	if round1["units"].Value != "millimetres" {
		t.Errorf("registry item value = %q", round1["units"].Value)
	}

	st = f.answer("u1", askID, intake.Answer{Answers: []intake.SlotAnswer{
		{ID: card.Questions[0].ID, Value: "answer one"},
		{ID: card.Questions[1].ID, Value: "answer two"},
	}})
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("round 2 card = %q, want another interview card", st.OpenAskKind)
	}
	askID2, card2 := f.openAsk(st.RunID)
	if card2.Understood == nil || len(card2.Understood.Items) != 3+settled {
		t.Fatalf("round 2 understood = %+v, want 3 items plus the %d settled ones", card2.Understood, settled)
	}
	byID := map[string]intake.UnderstoodItem{}
	for _, it := range card2.Understood.Items {
		byID[it.SlotID] = it
		if it.Name == "" {
			t.Errorf("item %q has no plain-language name", it.SlotID)
		}
	}
	if byID["units"].How != intake.ResolvedRegistry {
		t.Errorf("units how = %q, want registry", byID["units"].How)
	}
	for _, id := range []string{card.Questions[0].ID, card.Questions[1].ID} {
		if byID[id].How != intake.ResolvedAnswered {
			t.Errorf("slot %q how = %q, want answered", id, byID[id].How)
		}
	}

	// Force-proceed converts the rest; the approval recap carries them as
	// assumptions, each origin-labelled.
	st = f.answer("u1", askID2, intake.Answer{ForceProceed: true})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("card after force-proceed = %q, want approval", st.OpenAskKind)
	}
	_, approval := f.openAsk(st.RunID)
	recap := approval.Approval.Layer1.Understood
	if recap == nil {
		t.Fatal("approval card carries no understanding recap (R9)")
	}
	var assumed int
	for _, it := range recap.Items {
		if it.How == intake.ResolvedAssumption {
			assumed++
			if it.Assumption == "" {
				t.Errorf("assumed slot %q records no assumption text", it.SlotID)
			}
		}
	}
	if assumed == 0 {
		t.Error("no force-proceed conversions in the recap — the vagueness became invisible")
	}
}

// TestApprovalCardCarriesUnderstandingRecap (§7 T8; R9): the recap is the
// platform's own record — every resolution AND every answered escalation —
// complete and beside the planner's restatement, a complement rather than a
// replacement, and never a second blocking card.
//
// Drain r1 (F5) edit, declared: the count assertion read
// `len(Items) == len(st.Resolutions)`, which encodes a resolutions-only recap.
// R9's own prose says "every resolution … plus answered escalations", so the
// original assertion contradicted the requirement it was written to test. The
// bound is now resolutions + escalations, and the walk actually FORCES an
// escalation so the added leg is exercised rather than merely permitted.
func TestApprovalCardCarriesUnderstandingRecap(t *testing.T) {
	f := newFix(t)
	const escalated = "Deleting the cache changes the write-set — proceed?"
	asked := false
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		if !asked {
			asked = true
			return intake.Pair{}, &intake.Escalation{Question: escalated}
		}
		return basePair(in), nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardEscalation {
		t.Fatalf("card = %q, want the forced escalation", st.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Text: "yes, the cache is disposable"})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("card = %q, want approval", st.OpenAskKind)
	}
	if len(st.Escalations) != 1 {
		t.Fatalf("state records %d escalations, want 1", len(st.Escalations))
	}
	_, card := f.openAsk(st.RunID)
	l1 := card.Approval.Layer1
	if l1.Understood == nil {
		t.Fatal("approval layer 1 carries no understanding recap (R9)")
	}
	if l1.Restatement == "" {
		t.Error("the recap replaced the planner's restatement — it complements it")
	}
	if len(l1.Understood.Items) != len(st.Resolutions)+len(st.Escalations) {
		t.Fatalf("recap has %d items, want one per resolution (%d) plus one per answered escalation (%d)",
			len(l1.Understood.Items), len(st.Resolutions), len(st.Escalations))
	}
	recorded := map[string]intake.SlotResolution{}
	for _, r := range st.Resolutions {
		recorded[r.SlotID] = r
	}
	var escalations int
	for _, it := range l1.Understood.Items {
		if it.How == intake.UnderstoodEscalation {
			escalations++
			if it.Name != escalated {
				t.Errorf("escalation item names the question %q, want %q", it.Name, escalated)
			}
			if it.Value != "yes, the cache is disposable" {
				t.Errorf("escalation item value = %q — the requester's own answer is the record", it.Value)
			}
			continue
		}
		r, ok := recorded[it.SlotID]
		if !ok {
			t.Errorf("recap names slot %q, which the state never resolved", it.SlotID)
			continue
		}
		if it.How != r.How || it.Value != r.Value || it.Assumption != r.Assumption {
			t.Errorf("recap item %+v does not match the resolution %+v", it, r)
		}
	}
	if escalations != 1 {
		t.Errorf("recap carries %d escalation items, want 1 (R9 'plus answered escalations')", escalations)
	}
	// No new blocking card anywhere in the walk (S06.4 click discipline).
	for _, k := range cardKindsIssued(t, f, st.RunID) {
		if k == "understanding" || k == "recap" {
			t.Errorf("a new blocking card kind %q appeared", k)
		}
	}
}

// TestPhraseCallRidesTheConsumingRun (§7 T9a; R7): the seat is told which run
// it is working for, explicitly on the input — and after a recovery fork
// rebinds the intake state, that is the FORK, never the superseded parent.
func TestPhraseCallRidesTheConsumingRun(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	ph := &fakePhraser{result: intake.PhraseResult{Summary: "s"}}
	f.p.Phraser = ph

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if ph.last.RunID != st.RunID {
		t.Fatalf("phrase input run = %q, want the consuming run %q", ph.last.RunID, st.RunID)
	}
	parent := st.RunID

	fork := f.forkRun(parent, st.TaskID, st.Owner, 2, true)
	st, err := f.p.AdvanceDispatched(ctx, st.TaskID, fork)
	if err != nil {
		t.Fatalf("AdvanceDispatched: %v", err)
	}
	if st.RunID != fork {
		t.Fatalf("state run = %q, want the fork %q", st.RunID, fork)
	}
	askID, card := f.openAsk(fork)
	var answers []intake.SlotAnswer
	for _, q := range card.Questions {
		answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
	}
	st = f.answer("u1", askID, intake.Answer{Answers: answers})
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("card after the fork answer = %q, want another interview card", st.OpenAskKind)
	}
	if ph.last.RunID != fork {
		t.Errorf("phrase input run = %q after the rebind, want the fork %q", ph.last.RunID, fork)
	}
}

// TestPreRW12SnapshotsDecodeAndAnswer (§7 T10; R11): everything this packet
// adds is additive JSON — a state event and an open card written before it
// decode unchanged, and answering that card still applies.
func TestPreRW12SnapshotsDecodeAndAnswer(t *testing.T) {
	const preCard = `{"kind":"interview","task_id":"t-old","run_id":"t-old.intake","version":1,
		"issued_ts":"2026-08-01T10:00:00Z","clearance":0,"tier":"standard",
		"questions":[{"id":"behavior","text":"What exactly should this do?","weight":10}]}`
	var card intake.Card
	if err := json.Unmarshal([]byte(preCard), &card); err != nil {
		t.Fatalf("pre-RW-12 card snapshot: %v", err)
	}
	if card.Understood != nil {
		t.Errorf("pre-RW-12 card decoded with an understanding block: %+v", card.Understood)
	}
	if card.Questions[0].Phrased != "" || card.Questions[0].Text != "What exactly should this do?" {
		t.Errorf("pre-RW-12 question decoded as %+v", card.Questions[0])
	}

	const preState = `{"phase":"interview","task_id":"t-old","run_id":"t-old.intake","owner":"u1",
		"request":{"user_id":"u1","title":"x"},"family":"software","tier":"standard",
		"guess":{},"taxonomy_id":"software","taxonomy_version":"v1","needs_draft":true,
		"spec_version":0,"plan_version":0,"coverage_rounds":0,"research_bounced":false,
		"critique_rounds":0,"critique_done":false,"card_version":1,"clearance":0}`
	var st intake.State
	if err := json.Unmarshal([]byte(preState), &st); err != nil {
		t.Fatalf("pre-RW-12 state event: %v", err)
	}
	if st.TaxonomyVersion != "v1" || st.Family != intake.FamilySoftware {
		t.Errorf("pre-RW-12 state decoded as %+v", st)
	}

	// An open card written in the old shape still answers.
	f := newFix(t)
	live := f.start(stdRequest())
	f.admit(live.RunID)
	live = f.advance(live.TaskID)
	askID, issued := f.openAsk(live.RunID)
	var raw map[string]any
	snap, err := json.Marshal(issued)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(snap, &raw); err != nil {
		t.Fatal(err)
	}
	delete(raw, "understood")
	for _, q := range raw["questions"].([]any) {
		delete(q.(map[string]any), "phrased")
	}
	stripped, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`UPDATE asks SET snapshot = ? WHERE ask_id = ?`, string(stripped), askID)
		return err
	}); err != nil {
		t.Fatalf("rewrite snapshot: %v", err)
	}
	after := f.answer("u1", askID, intake.Answer{Answers: []intake.SlotAnswer{
		{ID: issued.Questions[0].ID, Value: "still answerable"},
	}})
	var found bool
	for _, r := range after.Resolutions {
		if r.SlotID == issued.Questions[0].ID && r.Value == "still answerable" {
			found = true
		}
	}
	if !found {
		t.Errorf("answering a pre-RW-12 card did not apply: %+v", after.Resolutions)
	}
}
