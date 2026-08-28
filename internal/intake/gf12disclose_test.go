package intake_test

// gf12disclose_test.go — P3-GF12, the builder-owed half of the packet's green
// (brief §9): nothing the marker guard or the refused-emission landing does is
// SILENT, the refused-emission card's other arm actually cancels, and the
// answered-marker record really reaches the seat that has to honor it.
//
// The reds pin the OUTCOMES (a card instead of a crash, a settled fact never
// re-asked, a bounded loop). These pin the record those outcomes leave behind,
// which is the half a person auditing the run later actually reads: §60's spirit
// and S06.7(a)'s "never disappears silently" both say the same thing — a
// platform that quietly resolves something on your behalf owes you the sentence
// saying so.
//
// Deterministic throughout: the fake planner seat emits over-cap and
// marker-restating content on demand, and no engine is reached.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// decisionSaying returns the first ledger decision whose text contains want, or
// nil — the ledger is the audit truth, so an assertion about disclosure is an
// assertion about a row in it.
func decisionSaying(t *testing.T, f *fix, taskID, want string) *ledger.Decision {
	t.Helper()
	doc := f.ledgerDoc(taskID)
	for i := range doc.Decisions {
		if strings.Contains(doc.Decisions[i].Text, want) {
			return &doc.Decisions[i]
		}
	}
	return nil
}

// TestGF12SettleFromRecordIsNeverSilent (brief R6): settling a re-emitted marker
// from the answered-marker record is a resolution the PLATFORM made on the
// requester's behalf, so it says so — naming the point it took off the open list
// and the answer it stood down on.
func TestGF12SettleFromRecordIsNeverSilent(t *testing.T) {
	const marker = "Which timezone should the opening hours be read in?"
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Spec.Clarifications = []string{marker}
		return p, nil
	}
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		p := baseRevise(in)
		p.Spec.Clarifications = []string{marker} // the witnessed re-ask
		return p, nil
	}
	st := f.answer("u1", askID, intake.Answer{ForceProceed: true})
	clarAsk, card := f.openAsk(st.RunID)
	st = f.answer("u1", clarAsk, intake.Answer{Answers: []intake.SlotAnswer{
		{ID: card.Questions[0].ID, Value: "Central European Time"},
	}})

	d := decisionSaying(t, f, st.TaskID, "settled from the record")
	if d == nil {
		t.Fatal("the platform settled a re-emitted marker and recorded nothing — a resolution made on the requester's behalf is never silent")
	}
	if d.Author != ledger.AuthorPlatform {
		t.Errorf("the settle is authored %q — it is the PLATFORM's act, not the requester's", d.Author)
	}
	for _, want := range []string{marker, "Central European Time"} {
		if !strings.Contains(d.Text, want) {
			t.Errorf("the disclosure does not carry %q — it reads: %s", want, d.Text)
		}
	}
	if !strings.Contains(d.Reason, "S06.6") {
		t.Errorf("the disclosure cites no spec arm: %s", d.Reason)
	}
}

// TestGF12MarkerConversionIsNeverSilent (brief R7): the leftover a bounded loop
// stops asking about is CONVERTED, not dropped — and the conversion is on the
// ledger with the round count that caused it, so a person reading back can tell
// "it stopped asking" from "it forgot".
func TestGF12MarkerConversionIsNeverSilent(t *testing.T) {
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
	for st.OpenAskKind == intake.CardClarification {
		clarAsk, card := f.openAsk(st.RunID)
		var answers []intake.SlotAnswer
		for _, q := range card.Questions {
			answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.Text})
		}
		st = f.answer("u1", clarAsk, intake.Answer{Answers: answers})
	}
	d := decisionSaying(t, f, st.TaskID, "converted to a listed assumption")
	if d == nil {
		t.Fatal("the marker loop hit its bound and converted a leftover without saying so")
	}
	if d.Author != ledger.AuthorPlatform {
		t.Errorf("the conversion is authored %q, want the platform", d.Author)
	}
	if !strings.Contains(d.Text, "fresh doubt") {
		t.Errorf("the conversion does not name the point it converted: %s", d.Text)
	}
	// And the assumption itself is on the SPEC, origin-labelled, where the
	// approval card's centerpiece renders it and Re-plan contests it.
	pair, err := f.p.CurrentPair(context.Background(), st.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range pair.Spec.Assumptions {
		if a.Origin == intake.AssumptionOriginMarker && strings.Contains(a.Text, "fresh doubt") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no marker-origin assumption carries the converted point: %+v", pair.Spec.Assumptions)
	}
}

// TestGF12RefusedEmissionRecordsBothHalves (brief R3): the refusal is a PLATFORM
// finding and the extra round is a HUMAN grant, and the ledger says which is
// which — the S06.7(a) authorship split, applied to the door that replaced the
// witnessed crash.
func TestGF12RefusedEmissionRecordsBothHalves(t *testing.T) {
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		return intake.Pair{}, overCapRefusal(1294)
	}
	raw, _ := json.Marshal(intake.Answer{ForceProceed: true})
	st, err := f.p.Answer(context.Background(), "u1", askID, raw)
	if err != nil {
		t.Fatalf("the over-cap emission crashed the drive: %v", err)
	}
	refusal := decisionSaying(t, f, st.TaskID, "refused by the artifact contract")
	if refusal == nil {
		t.Fatal("a refused emission left no platform finding on the ledger")
	}
	if refusal.Author != ledger.AuthorPlatform {
		t.Errorf("the refusal is authored %q, want the platform", refusal.Author)
	}
	if !strings.Contains(refusal.Text, "cap 1200") {
		t.Errorf("the refusal record does not carry what was refused: %s", refusal.Text)
	}

	f.planner.draft = nil
	emissionAsk, card := f.openAsk(st.RunID)
	// The card says what it cost so far and offers the two real choices.
	if !strings.Contains(card.Decision.Summary, "cap 1200") {
		t.Errorf("the card does not tell the requester what was refused: %s", card.Decision.Summary)
	}
	if len(card.Decision.Choices) != 2 {
		t.Fatalf("the card offers %d choices, want exactly two (one more round, or stop)", len(card.Decision.Choices))
	}
	st = f.answer("u1", emissionAsk, intake.Answer{Choice: intake.ChoiceReplan})
	grant := decisionSaying(t, f, st.TaskID, "granted one more planning round")
	if grant == nil {
		t.Fatal("the granted round left no record — a paid round nobody can attribute")
	}
	if grant.Author != ledger.AuthorHuman {
		t.Errorf("the grant is authored %q — an explicit human grant is recorded as one", grant.Author)
	}
}

// TestGF12RefusedEmissionCanBeStoppedInsteadOfRetried (brief R3): the card's
// other arm is a real door, not decoration — Rethink cancels the intake exactly
// as the SPEC-DOUBT card's does, and nothing was accepted on the way out.
func TestGF12RefusedEmissionCanBeStoppedInsteadOfRetried(t *testing.T) {
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		return intake.Pair{}, overCapRefusal(1569)
	}
	raw, _ := json.Marshal(intake.Answer{ForceProceed: true})
	st, err := f.p.Answer(context.Background(), "u1", askID, raw)
	if err != nil {
		t.Fatalf("the over-cap emission crashed the drive: %v", err)
	}
	emissionAsk, _ := f.openAsk(st.RunID)
	st = f.answer("u1", emissionAsk, intake.Answer{Choice: intake.ChoiceRethink, Note: "I'll ask for something smaller"})
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase = %q, want cancelled — Rethink is the SPEC-DOUBT card's own mapping", st.Phase)
	}
	if got := f.runState(st.RunID); got != run.StateFinalized {
		t.Fatalf("run state = %q, want finalized (cancel from an open card, §14 reading 9)", got)
	}
	if st.SpecRef != nil {
		t.Fatal("a cancelled refusal left an accepted artifact behind")
	}
}

// TestGF12SettledMarkersReachThePlanningSeat (brief R5): the durable record is
// only half the fix — the seat that must not re-ask has to SEE it. Both seam
// inputs carry it, and the record survives the revision that used to consume it.
func TestGF12SettledMarkersReachThePlanningSeat(t *testing.T) {
	const marker = "Should the archive keep the old files or replace them?"
	f, _, askID := driveToInterviewCard(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Spec.Clarifications = []string{marker}
		return p, nil
	}
	var seen *intake.ReviseInput
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		seen = &in
		return baseRevise(in), nil
	}
	st := f.answer("u1", askID, intake.Answer{ForceProceed: true})
	clarAsk, card := f.openAsk(st.RunID)
	st = f.answer("u1", clarAsk, intake.Answer{Answers: []intake.SlotAnswer{
		{ID: card.Questions[0].ID, Value: "keep the old files"},
	}})
	if seen == nil {
		t.Fatal("the clarification answer never reached a revision")
	}
	if len(seen.SettledMarkers) != 1 || seen.SettledMarkers[0].Marker != marker {
		t.Fatalf("the revise input carries %+v — the answered marker must ride every later planning session", seen.SettledMarkers)
	}
	if seen.SettledMarkers[0].Answer != "keep the old files" {
		t.Errorf("the record lost the requester's own words: %q", seen.SettledMarkers[0].Answer)
	}
	// The record OUTLIVES the revision that consumed the one-shot ReviseReq —
	// that evaporation is what made the witnessed confirm-loop undetectable.
	after, err := f.p.LoadState(context.Background(), st.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.SettledMarkers) != 1 {
		t.Fatalf("the durable record holds %d markers after the revision consumed the request, want 1", len(after.SettledMarkers))
	}
}
