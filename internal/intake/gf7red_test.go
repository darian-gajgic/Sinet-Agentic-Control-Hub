package intake_test

// gf7red_test.go — P3-GF7 acceptance tests, committed RED with the grounding
// brief (P3/briefs/P3-GF7.md §9; Amendment-A carve-out, CONVENTIONS §38 area).
// The window opens at the grounding commit and closes at the P3-GF7
// implementation commit. Every test here compiles against the CURRENT type
// surface and fails only on v4 behavior that does not exist yet.
//
// Binding operator record: P3/design/b6-gate-operator-findings-r5-2026-08-23.md
// (§A per-slot verdicts, §C hard rules, §D W2); harvest
// P3/design/w1-nexus-live-harvest-2026-08-27.md (H10, H18, H23).

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf7Killed are the three slots the operator killed as-asked (r5 §D W2):
// behavior and terminology invert into stated understanding; indices_ranges
// resolves internally. They must never again appear as a question on any card.
var gf7Killed = map[string]bool{
	"behavior":       true,
	"terminology":    true,
	"indices_ranges": true,
}

func gf7HasOption(opts []intake.Option, value string) bool {
	for _, o := range opts {
		if o.Value == value {
			return true
		}
	}
	return false
}

// TestGF7KilledSlotsAreNeverAsked (brief R2; r5 §C rules 2+3): a standard-tier
// software interview, driven card by card, never presents a killed slot as a
// question. RED at v3: behavior and terminology ship on card 2, indices_ranges
// on card 3.
func TestGF7KilledSlotsAreNeverAsked(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)

	for i := 0; i < 10; i++ {
		askID, card := f.openAsk(st.RunID)
		if card.Kind != intake.CardInterview {
			break
		}
		var answers []intake.SlotAnswer
		for _, q := range card.Questions {
			if gf7Killed[q.ID] {
				t.Errorf("slot %q was asked on an interview card — killed as-asked by the operator (r5 §D W2)", q.ID)
			}
			answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
		}
		st = f.answer("u1", askID, intake.Answer{Answers: answers})
		if st.OpenAskID == "" || st.OpenAskKind != intake.CardInterview {
			break
		}
	}
}

// TestGF7SoftwareV4ShapeAndSystemDecidesDefault (brief R1 + R7): the software
// seed is the v4 revision, and ordering_atomicity offers the planner-chooses
// arm — the operator ruled system-decides "the only honest default" for a
// non-IT requester (r5 §A round 1). RED at v3 on both counts.
func TestGF7SoftwareV4ShapeAndSystemDecidesDefault(t *testing.T) {
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	if soft.Version != "v4" {
		t.Errorf("software seed version = %q, want %q (the W2 revision, r5 §D)", soft.Version, "v4")
	}
	oa := soft.Slot("ordering_atomicity")
	if oa == nil {
		t.Fatal("ordering_atomicity slot missing — it is on the operator's KEEP list (r5 §D W2)")
	}
	if !gf7HasOption(oa.Options, "planner_chooses") {
		t.Errorf("ordering_atomicity offers no planner-chooses option; system-decides must be the recommended default (r5 §A round 1)")
	}
}

// TestGF7SystemResolvedSlotsLandAsSpecAssumptions (brief R3; W1 H10): the
// killed slots keep their Clearance accounting by resolving as confirmable
// assumptions — after an interview answered to the floor, the SPEC carries a
// slot-origin assumption for each. RED at v3: the slots are asked and
// answered, so no such assumptions exist.
func TestGF7SystemResolvedSlotsLandAsSpecAssumptions(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil

	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)

	pair, err := f.p.CurrentPair(context.Background(), st.TaskID)
	if err != nil {
		t.Fatalf("CurrentPair: %v", err)
	}
	for id := range gf7Killed {
		origin := "slot:" + id
		found := false
		for _, a := range pair.Spec.Assumptions {
			if a.Origin == origin {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SPEC lists no %q assumption — a never-asked slot must surface as a confirmable assumption (H10; S06.5)", origin)
		}
	}
}

// TestGF7InterviewCardServesTheFamilyGuess (brief R9; W1 H18): the served
// interview-card JSON carries the family guess and what resolved it, so the
// surface can show the guess and offer correction BEFORE questions are
// answered. Asserted on the raw ask snapshot because the members do not exist
// on the Card type yet. RED at v3: neither key is present.
func TestGF7InterviewCardServesTheFamilyGuess(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil

	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)

	var snapshot string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT snapshot FROM asks WHERE run_id = ? AND status = 'open'`, st.RunID).
		Scan(&snapshot); err != nil {
		t.Fatalf("open ask snapshot: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(snapshot), &m); err != nil {
		t.Fatalf("snapshot decode: %v", err)
	}
	if got, _ := m["family"].(string); got != string(intake.FamilySoftware) {
		t.Errorf("card family = %q, want %q — the guess must be visible on the card (H18)", got, intake.FamilySoftware)
	}
	if got, _ := m["family_source"].(string); got != intake.FamilySourceClassifier {
		t.Errorf("card family_source = %q, want %q — the card must say the family is a guess (H18)", got, intake.FamilySourceClassifier)
	}
}

// TestGF7UnitsWhyLineRewritten (brief R7; r5 §A round 3): the condemned units
// why-line is gone — why-lines explain in plain project terms or say nothing.
// RED at v3: the seed carries this exact sentence.
func TestGF7UnitsWhyLineRewritten(t *testing.T) {
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	units := soft.Slot("units")
	if units == nil {
		return // survives only under an OQ ruling; nothing to check here
	}
	const condemned = "Mixed-up units stay invisible: everything looks right until a number is ten times off."
	if units.Why == condemned {
		t.Errorf("units why-line is still the condemned sentence — \"It's asking and describing nonsense here\" (r5 §A round 3)")
	}
}
