package history_test

// P3-RW-18 D4 acceptance tests — committed RED with the brief (Amendment-A;
// red window closed by the packet's implementation commit).
//
// Walk-proven defect: the own-words history ask "what got cancelled recently
// and why?" was confidently routed to cost.budget_remainder (margin 8.575,
// served as a note). Two causes, two fixes:
//
//   1. The 50-query catalog holds NOTHING about cancels, so the classifier had
//      no right answer and guessed instead of abstaining. A cancelled canned
//      query joins the status category (S14.10 ¶2's catalog is the reliability
//      floor; S15.5 mission control filters by status).
//   2. Nothing structural checks that the picked query is even about the
//      question. S12.5 forbids fabricating a confidence threshold for an
//      uncalibrated seat — and the walk margin (8.575, HIGH and wrong) proves
//      a margin floor would not help anyway. The floor is LEXICAL: the picked
//      entry must share at least one content token with the question (the
//      deterministic topChoices primitive), else the honest no-match card —
//      "no catalog query matches" + real choices — never a guess (S12.4).

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
)

// TestTheCatalogAnswersWhatGotCancelled — the canned entry exists, sits in the
// status category, needs no slot fill (one fewer model call), and carries the
// S01.9 owner-scope predicate like every other entry.
func TestTheCatalogAnswersWhatGotCancelled(t *testing.T) {
	q, ok := history.QueryByName("status.tasks_cancelled")
	if !ok {
		t.Fatal("the catalog holds no status.tasks_cancelled — the fifty queries contain no 'cancelled' (walk-proven RW-18 D4)")
	}
	if q.Category != history.CatStatus {
		t.Errorf("status.tasks_cancelled sits in category %q, want status", q.Category)
	}
	if len(q.Slots) != 0 {
		t.Errorf("status.tasks_cancelled declares %d slots, want 0 — 'recently' is the ORDER BY, not a slot", len(q.Slots))
	}
	if !strings.Contains(q.SQL, history.OwnerScopeOpen) {
		t.Error("status.tasks_cancelled carries no owner-scope predicate (S01.9)")
	}
	hay := strings.ToLower(q.Name + " " + string(q.Category) + " " + q.Description)
	for _, w := range []string{"cancelled", "who", "why"} {
		if !strings.Contains(hay, w) {
			t.Errorf("the entry's name/description does not carry %q — the intent prompt and the lexical floor read this vocabulary", w)
		}
	}
}

// TestALexicallyUnrelatedPickGetsTheHonestNoMatchCard — the walk mismatch,
// replayed byte-for-byte: the duty confidently picks cost.budget_remainder for
// a cancel question. The structural floor must refuse to serve it and answer
// with the honest no-match card instead: what it could not do, plus the real
// catalog choices ("here's what I have").
func TestALexicallyUnrelatedPickGetsTheHonestNoMatchCard(t *testing.T) {
	s := newStack(t, false)
	seedTwoOwners(t, s.fixture)
	s.answer(`{"reason":"a spend question","query":"cost.budget_remainder","abstain":false}`,
		margins("cost.budget_remainder", 8.575))

	a, err := s.st.Ask(s.ctx, "what got cancelled recently and why?", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.Card == nil {
		t.Fatalf("a lexically unrelated pick was served as the answer (%q) — the walk-proven confident mismatch (margin 8.575); want the honest no-match card", a.Query)
	}
	if len(a.Rows) != 0 {
		t.Errorf("the no-match card carried %d rows — nothing was run, nothing may be shown", len(a.Rows))
	}
	if len(a.Card.Choices) == 0 {
		t.Error("the no-match card offers no choices — 'here's what I have' needs the catalog shortlist")
	}
	if !strings.Contains(strings.ToLower(a.Card.Reason), "match") {
		t.Errorf("the card does not say no catalog query matches: %q", a.Card.Reason)
	}
}

// TestARelatedPickStillResolves — the floor is a backstop against unrelated
// picks, never a second classifier: the existing resolvable shape (question
// vocabulary present in the entry) keeps resolving exactly as before.
func TestARelatedPickStillResolves(t *testing.T) {
	s := newStack(t, false)
	seedTwoOwners(t, s.fixture)
	s.answer(`{"reason":"the asker wants live runs","query":"status.runs_active","abstain":false}`,
		margins("status.runs_active", 4.0))

	a, err := s.st.Ask(s.ctx, "what is running right now?", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.Card != nil {
		t.Fatalf("the lexical floor swallowed a related pick: %+v", a.Card)
	}
	if a.Query != "status.runs_active" {
		t.Errorf("answer names %q, want status.runs_active", a.Query)
	}
}
