package intake_test

// gf3be1_added_test.go — the P3-GF3-BE1 legs the executor added beyond the
// grounding battery (brief §8 T9, T10), plus the properties the brief marks as
// spec-stated invariants rather than examples.
//
// The landed suite's own tests for these postures stay untouched: this file
// sits beside TestPhrasingAbsenceIsVerbatimZeroClicks and
// TestPreRW12SnapshotsDecodeAndAnswer and asks the same questions of the
// fields this packet added.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// ---- T9: honest absence, extended to the suggestion decoration ----

// TestGF3SuggestionAbsenceIsVerbatimZeroClicks: a seat that errors costs the
// requester nothing NEW either. The card carries no phrasing, no suggestion and
// no suggested option; it asks the same questions on the same number of cards;
// and the operator gets one WARN naming the run and the cause.
//
// This is the limb that can actually fail: a suggestion is the one decoration a
// surface is tempted to synthesize, because "no recommendation" looks like a
// hole where "stock wording" does not (§26 — a deterministic default is never
// dressed up as model output).
func TestGF3SuggestionAbsenceIsVerbatimZeroClicks(t *testing.T) {
	card := func(seam intake.Phraser, logTo *bytes.Buffer) (intake.Card, int) {
		f := newFix(t)
		if logTo != nil {
			f.p.Logger = slog.New(slog.NewTextHandler(logTo, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
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

	base, baseAsks := card(nil, nil)
	var logged bytes.Buffer
	broken, brokenAsks := card(&fakePhraser{err: errors.New("local: stack absent")}, &logged)

	if brokenAsks != baseAsks {
		t.Errorf("ask rows = %d with a broken seat, want %d — a degraded seat never adds a click", brokenAsks, baseAsks)
	}
	if len(broken.Questions) != len(base.Questions) {
		t.Fatalf("broken-seat card carries %d questions, want the same %d", len(broken.Questions), len(base.Questions))
	}
	for i, q := range broken.Questions {
		if q.ID != base.Questions[i].ID || q.Text != base.Questions[i].Text {
			t.Errorf("question %d changed with a broken seat: %+v", i, q)
		}
		if q.Phrased != "" || q.Suggested != "" || q.SuggestedOption != "" {
			t.Errorf("question %q carries decoration though the seat failed: phrased=%q suggested=%q option=%q",
				q.ID, q.Phrased, q.Suggested, q.SuggestedOption)
		}
		// The taxonomy's own content is still all there: a card with no
		// recommendation is still a card a person can answer.
		if q.Why == "" || len(q.Options) < 2 {
			t.Errorf("question %q lost its own why/options when the seat degraded: %+v", q.ID, q)
		}
	}
	// The absent seat's card carries no decoration either, and never fabricates.
	for _, q := range base.Questions {
		if q.Suggested != "" || q.SuggestedOption != "" {
			t.Errorf("question %q carries a suggestion with no seat wired: %+v", q.ID, q)
		}
	}
	if !strings.Contains(logged.String(), "level=WARN") {
		t.Errorf("the seat failed and the operator log says nothing (PH-1 F3):\n%s", logged.String())
	}
}

// TestGF3SuggestionsNeverEscapeTheAskedSet is the property behind T2's example
// (S06.5's duty split: the seat phrases and suggests, the taxonomy decides what
// is asked). Whatever the seat returns — suggestions for every slot in the
// vocabulary, options belonging to other questions, empty strings — the card
// carries exactly the questions the pipeline selected, and each decoration is
// its own question's or absent.
func TestGF3SuggestionsNeverEscapeTheAskedSet(t *testing.T) {
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	f := newFix(t)
	f.p.Phraser = &fakePhraser{fn: func(in intake.PhraseInput) (intake.PhraseResult, error) {
		res := intake.PhraseResult{
			Suggestions:      map[string]string{},
			SuggestedOptions: map[string]string{},
		}
		// A suggestion, and an option value, for EVERY slot of the family —
		// including the ones this card never asked about. Each option value is
		// taken from a DIFFERENT slot, so a fold that checked "is this a known
		// option value anywhere" instead of "on this question" would leak.
		for i, s := range soft.Slots {
			res.Suggestions[s.ID] = "SUGGEST-" + s.ID
			other := soft.Slots[(i+1)%len(soft.Slots)]
			if len(other.Options) > 0 {
				res.SuggestedOptions[s.ID] = other.Options[0].Value
			}
		}
		res.Suggestions["a_slot_nobody_asked"] = "SMUGGLED"
		return res, nil
	}}

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	_, card := f.openAsk(st.RunID)

	asked := map[string]bool{}
	for _, q := range card.Questions {
		asked[q.ID] = true
	}
	if len(asked) != len(card.Questions) {
		t.Fatal("the card repeats a question id")
	}
	for _, q := range card.Questions {
		slot := soft.Slot(q.ID)
		if slot == nil {
			t.Fatalf("question %q is not a slot of the active set", q.ID)
		}
		if q.Suggested != "SUGGEST-"+q.ID {
			t.Errorf("question %q carries %q, want its OWN suggestion", q.ID, q.Suggested)
		}
		if q.SuggestedOption == "" {
			continue
		}
		found := false
		for _, o := range q.Options {
			if o.Value == q.SuggestedOption {
				found = true
			}
		}
		if !found {
			t.Errorf("question %q points at option %q, which is not one of its own options", q.ID, q.SuggestedOption)
		}
	}
	// Nothing about a slot that was not asked reached the card at all.
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SMUGGLED") {
		t.Error("a suggestion for a question the pipeline never selected reached the card")
	}
	for _, s := range soft.Slots {
		if !asked[s.ID] && strings.Contains(string(raw), "SUGGEST-"+s.ID) {
			t.Errorf("the unasked slot %q reached the card through its suggestion", s.ID)
		}
	}
}

// ---- T10: pre-GF3 snapshots ----

// TestGF3PreSnapshotsDecodeAndAnswer: everything this packet adds is additive
// JSON with omitempty, so an ask snapshot and a state event written before it
// decode unchanged and still answer. (The RW-12 sibling stays untouched; this
// asks the same question of the GF3 fields.)
func TestGF3PreSnapshotsDecodeAndAnswer(t *testing.T) {
	const preCard = `{"kind":"interview","task_id":"t-old","run_id":"t-old.intake","version":1,
		"issued_ts":"2026-08-20T10:00:00Z","clearance":40,"tier":"standard",
		"questions":[{"id":"behavior","text":"What exactly should this do?","weight":10,
			"phrased":"What should it do?","options":[{"label":"a","value":"a"},{"label":"b","value":"b"}]}],
		"understood":{"items":[{"slot_id":"units","name":"Units","how":"registry","value":"mm"}]}}`
	var card intake.Card
	if err := json.Unmarshal([]byte(preCard), &card); err != nil {
		t.Fatalf("pre-GF3 card snapshot: %v", err)
	}
	if card.ClearanceFloor != 0 {
		t.Errorf("pre-GF3 card decoded with a clearance floor: %v", card.ClearanceFloor)
	}
	q := card.Questions[0]
	if q.Why != "" || q.Suggested != "" || q.SuggestedOption != "" || q.Resolution != nil {
		t.Errorf("pre-GF3 question decoded with GF3 decoration: %+v", q)
	}
	if q.Phrased != "What should it do?" || q.Text != "What exactly should this do?" {
		t.Errorf("pre-GF3 question lost its landed fields: %+v", q)
	}

	const preState = `{"phase":"interview","task_id":"t-old","run_id":"t-old.intake","owner":"u1",
		"request":{"user_id":"u1","title":"x"},"family":"software","tier":"standard",
		"guess":{},"taxonomy_id":"software","taxonomy_version":"v2","needs_draft":true,
		"resolutions":[{"slot_id":"units","how":"registry","value":"mm"}],
		"spec_version":0,"plan_version":0,"coverage_rounds":0,"research_bounced":false,
		"critique_rounds":0,"critique_done":false,"card_version":1,"clearance":6}`
	var st intake.State
	if err := json.Unmarshal([]byte(preState), &st); err != nil {
		t.Fatalf("pre-GF3 state event: %v", err)
	}
	if st.TaxonomyVersion != "v2" || len(st.Resolutions) != 1 {
		t.Errorf("pre-GF3 state decoded as %+v", st)
	}

	// A card issued in the pre-GF3 shape still answers, including the arms this
	// packet added — the skip reads the snapshot, and a snapshot with no
	// suggestion simply has none to carry.
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
	delete(raw, "clearance_floor")
	for _, q := range raw["questions"].([]any) {
		delete(q.(map[string]any), "why")
		delete(q.(map[string]any), "suggested")
		delete(q.(map[string]any), "suggested_option")
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
		{ID: issued.Questions[1].ID, Skip: true},
	}})
	byID := map[string]intake.SlotResolution{}
	for _, r := range after.Resolutions {
		byID[r.SlotID] = r
	}
	if got := byID[issued.Questions[0].ID]; got.Value != "still answerable" {
		t.Errorf("answering a pre-GF3 card did not apply: %+v", got)
	}
	skipped := byID[issued.Questions[1].ID]
	if skipped.How != intake.ResolvedAssumption {
		t.Fatalf("skipping on a pre-GF3 card did not resolve it: %+v", skipped)
	}
	if strings.TrimSpace(skipped.Assumption) == "" {
		t.Error("the skip assumption is empty — with no served suggestion it must still say what happened, in words")
	}
}
