package history_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// plantedSecret is a real-shaped Anthropic key — the load-bearing redaction
// class (the live claude-cli lane). It is planted in a drift summary, which
// B5-8A's projector indexes into history_fts.
const plantedSecret = "sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHIIIIJJJJ"

// TestSearchRedactsBeforeMatching — acceptance 31, the codor-C2 obligation.
//
// A query for a planted secret's PLAINTEXT must return no confirmation. The
// property is bought at index time (the corpus is redacted) and reinforced here
// (the query is redacted before it is matched), and the test proves the whole
// path end to end: plant → index → search → nothing.
//
// It is deliberately NOT a test that the reader filters results. A search that
// matched raw text and then filtered the output would still be an oracle —
// timing, row counts and error behaviour all leak. Only an absent match is an
// absent answer.
func TestSearchRedactsBeforeMatching(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.event("", member1, "drift.finding", map[string]any{
		"source": "provider-docs", "change_class": "contract", "severity": "high",
		"lanes":   []string{"anthropic"},
		"summary": "the sample request now shows " + plantedSecret + " as the auth header value",
	})
	f.indexHistory()

	// The benign half of the same record IS findable — without this the
	// negative below would pass on an empty index.
	hits, err := f.st.Search(f.ctx, "sample request auth header", opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits.Rows) == 0 {
		t.Fatal("the planted record is not findable at all — the negative below would be vacuous")
	}

	// The secret's plaintext confirms nothing.
	oracle, err := f.st.Search(f.ctx, plantedSecret, opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(oracle.Rows) != 0 {
		t.Errorf("searching the planted secret's plaintext returned %d hits — the search surface is an ORACLE for a redacted secret (codor C2)", len(oracle.Rows))
	}
	// And the distinctive tail of the key, on its own, is equally inert.
	if tail, err := f.st.Search(f.ctx, "IIIIJJJJ", opScope(), 10); err != nil {
		t.Fatal(err)
	} else if len(tail.Rows) != 0 {
		t.Errorf("a fragment of the planted secret matched %d rows", len(tail.Rows))
	}

	// A question that is NOTHING BUT a secret contributes no term at all, and
	// the answer says so rather than silently returning an empty page.
	bare, err := f.st.Search(f.ctx, "  "+plantedSecret+"  ", opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(bare.Rows) != 0 {
		t.Errorf("a secret-only question matched %d rows", len(bare.Rows))
	}
	joined := strings.Join(bare.Notes, " | ")
	if !strings.Contains(joined, "redacted") || !strings.Contains(joined, "no searchable term") {
		t.Errorf("a secret-only question did not explain why it matched nothing: %q", joined)
	}

	// No excerpt anywhere carries the plaintext, and the marker is what stands
	// in its place.
	sawMarker := false
	for _, row := range hits.Rows {
		for _, c := range row {
			s, ok := c.(string)
			if !ok {
				continue
			}
			if strings.Contains(s, plantedSecret) {
				t.Fatalf("an excerpt carried the planted secret verbatim: %q", s)
			}
			if strings.Contains(s, "[REDACTED:anthropic_key]") {
				sawMarker = true
			}
		}
	}
	if !sawMarker {
		t.Error("no excerpt showed the redaction marker — the excerpt should show that something WAS redacted, not silently omit it")
	}

	// STORE-RAW / SERVE-REDACTED (§30 R19): the event row itself is untouched.
	var raw string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT payload FROM run_events WHERE type = 'drift.finding'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, plantedSecret) {
		t.Error("the stored payload was mutated — redaction is a SERVE-side property; the raw body is the audit truth")
	}
}

// TestSearchExcerptsAreBounded — the second half of R28's excerpt condition.
func TestSearchExcerptsAreBounded(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	long := strings.Repeat("alpha beta gamma delta epsilon zeta eta theta ", 200)
	f.event("", member1, "drift.finding", map[string]any{
		"source": "verbose-source", "change_class": "contract", "severity": "low",
		"summary": long,
	})
	f.indexHistory()

	a, err := f.st.Search(f.ctx, "gamma", opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) == 0 {
		t.Fatal("no hit on a seeded term")
	}
	got := str(cell(t, a, 0, "excerpt"))
	if utf8.RuneCountInString(got) > history.SearchExcerptRunes+len("…(excerpt bounded)") {
		t.Errorf("excerpt is %d runes, past the %d bound", utf8.RuneCountInString(got), history.SearchExcerptRunes)
	}
	if strings.TrimSpace(got) == "" {
		t.Error("the excerpt is empty — a bounded excerpt still has to help a person recognize the hit")
	}
}

// TestSearchIsOwnerScoped — S01.9 on the search surface.
func TestSearchIsOwnerScoped(t *testing.T) {
	f := newFixture(t)
	for _, u := range []string{member1, member2} {
		f.user(u, "member")
		f.event("", u, "drift.finding", map[string]any{
			"source": "src-" + u, "change_class": "contract", "severity": "low",
			"summary": "distinctivetoken belonging to " + u,
		})
	}
	f.indexHistory()

	op, err := f.st.Search(f.ctx, "distinctivetoken", opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Rows) != 2 {
		t.Fatalf("operator search returned %d hits, want 2", len(op.Rows))
	}
	mem, err := f.st.Search(f.ctx, "distinctivetoken", memberScope(member1), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(mem.Rows) != 1 {
		t.Fatalf("member search returned %d hits, want 1", len(mem.Rows))
	}
	if got := str(cell(t, mem, 0, "user_id")); got != member1 {
		t.Errorf("member search leaked owner %q", got)
	}
}

// TestSearchQuerySyntaxIsNeutralized — an FTS5 MATCH expression is a language.
// A question is TEXT, so the query builder must be a whitelist rather than an
// escape: syntax in the question is matched, never executed, and never errors.
func TestSearchQuerySyntaxIsNeutralized(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.event("", member1, "drift.finding", map[string]any{
		"source": "s", "change_class": "contract", "severity": "low", "summary": "ordinary content here",
	})
	f.indexHistory()

	for _, q := range []string{
		`kind:run_summary`,
		`"unterminated`,
		`ordinary OR (content AND NOT here)`,
		`ordinar*`,
		`NEAR(ordinary content, 2)`,
		`^ordinary`,
		`{body}: ordinary`,
		``,
		`   `,
	} {
		a, err := f.st.Search(f.ctx, q, opScope(), 10)
		if err != nil {
			t.Errorf("search %q returned an error instead of treating the syntax as text: %v", q, err)
			continue
		}
		if a.Query != history.SearchQueryName {
			t.Errorf("search %q produced an answer named %q", q, a.Query)
		}
	}
}
