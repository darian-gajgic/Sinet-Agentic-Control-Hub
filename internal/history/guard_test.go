package history_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// guard_test.go — the extraction and parsing limbs, unit-tested against
// REALISTIC reasoning-model output. These are the fixtures the adverse
// measurement describes: a long chain of thought with the statement buried
// after it (P3/measurements/2026-07-22-sql-open-arctic.md, 0/30 on the raw
// alias for exactly this reason).

// arcticCoT is the shape the deployed seat actually emits: reasoning first, at
// length, then the answer in a fenced block. The raw-alias output contract
// scored 0/30 against output like this; extraction is what makes it usable.
const arcticCoT = `Okay, let me work through this. The user is asking about how much each
run cost. Looking at the schema, I can see there is a view called cost_per_run
which has run_id, user_id, priced_usd and pricing_status columns.

I need to be careful here — the question says "most expensive", so I should
order by the priced amount descending. But wait, some rows may be UNPRICED,
which means priced_usd could be null. Let me think about whether that matters.
Actually for an ordering query it does not, nulls sort last in SQLite by
default for DESC... no, nulls sort first for DESC. Hmm. Let me just order by
priced_usd DESC and let the caller see the pricing_status column.

Actually, let me reconsider the columns once more. The view has what I need.
I'll select everything and order it.

` + "```sql" + `
SELECT run_id, user_id, priced_usd, pricing_status
FROM cost_per_run
ORDER BY priced_usd DESC
` + "```" + `

That should answer the question.`

func TestExtractsTheStatementFromReasoningOutput(t *testing.T) {
	got, err := history.ExtractSQL(arcticCoT)
	if err != nil {
		t.Fatalf("ExtractSQL on realistic chain-of-thought: %v", err)
	}
	if !strings.HasPrefix(strings.ToUpper(got), "SELECT RUN_ID") {
		t.Fatalf("extracted %q, want the fenced statement", got)
	}
	if strings.Contains(got, "Okay") || strings.Contains(got, "That should answer") {
		t.Errorf("extraction carried reasoning prose into the statement: %q", got)
	}
	// It must survive the guard, or extraction has not actually helped.
	res, err := history.Guard(got, history.Scope{Operator: true}, 50)
	if err != nil {
		t.Fatalf("the extracted statement was refused: %v", err)
	}
	if len(res.Views) != 1 || res.Views[0] != history.ViewCostPerRun {
		t.Errorf("views = %v, want [cost_per_run]", res.Views)
	}
}

func TestExtractionVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // a substring the extraction must start with
	}{
		{
			name: "unfenced reasoning then the statement last",
			raw: "I need to look at routing quality. The routing_quality view joins decisions to outcomes.\n" +
				"Let me select the cause and the verdict counts.\n" +
				"SELECT cause, verdicts FROM routing_quality",
			want: "SELECT cause",
		},
		{
			name: "a think block wrapping the reasoning",
			raw: "<think>The user wants per-person cost. I should use cost_per_person. " +
				"Wait, is it per_person or per_user? The schema says cost_per_person.</think>\n" +
				"SELECT user_id, priced_usd FROM cost_per_person",
			want: "SELECT user_id",
		},
		{
			name: "an untagged fence",
			raw:  "Here is the query:\n```\nSELECT * FROM cost_per_task\n```\nDone.",
			want: "SELECT * FROM cost_per_task",
		},
		{
			name: "trailing semicolon inside the fence",
			raw:  "```sql\nSELECT * FROM cost_per_run;\n```",
			want: "SELECT * FROM cost_per_run",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := history.ExtractSQL(c.raw)
			if err != nil {
				t.Fatalf("ExtractSQL: %v", err)
			}
			if !strings.HasPrefix(got, c.want) {
				t.Errorf("extracted %q, want it to start with %q", got, c.want)
			}
			if _, err := history.Guard(got, history.Scope{Operator: true}, 20); err != nil {
				t.Errorf("the extracted statement was refused: %v", err)
			}
		})
	}
}

// TestExtractionNeverTruncatesACompoundStatement — the bug a naive "take the
// last SELECT" extractor has: it would silently drop the first half of a
// compound statement, or a whole with-clause, and answer a DIFFERENT question
// than the model asked. Silent truncation is worse than refusal.
func TestExtractionNeverTruncatesACompoundStatement(t *testing.T) {
	cases := []struct{ name, raw, wantPrefix string }{
		{
			"a union keeps both halves",
			"SELECT run_id FROM cost_per_run UNION SELECT run_id FROM cost_done_directly",
			"SELECT run_id FROM cost_per_run UNION",
		},
		{
			"a with-clause keeps its definition",
			"WITH top AS (SELECT * FROM cost_per_run) SELECT * FROM top",
			"WITH top AS",
		},
		{
			"a subquery is not mistaken for the statement",
			"SELECT * FROM cost_per_run WHERE user_id IN (SELECT user_id FROM cost_per_person)",
			"SELECT * FROM cost_per_run WHERE",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := history.ExtractSQL(c.raw)
			if err != nil {
				t.Fatalf("ExtractSQL: %v", err)
			}
			if !strings.HasPrefix(got, c.wantPrefix) {
				t.Fatalf("extracted %q — a truncated statement asks a different question", got)
			}
			if got != c.raw {
				t.Errorf("extraction changed the statement:\n got %q\nwant %q", got, c.raw)
			}
		})
	}
}

// TestTheHonestFailurePath — the model produced no statement. Nothing is
// salvaged from the prose, and the caller is told so.
func TestTheHonestFailurePath(t *testing.T) {
	for _, raw := range []string{
		"I'm sorry, I don't have enough information about the schema to answer that.",
		"",
		"<think>I could not work out which view to use. There is no obvious one.</think>",
		"The answer is 42.",
	} {
		if got, err := history.ExtractSQL(raw); !errors.Is(err, history.ErrNoSQL) {
			t.Errorf("ExtractSQL(%q) = %q, %v — want ErrNoSQL", raw, got, err)
		}
	}
}

// TestReasoningThenHostileSQL — the case the whole packet is for: the model's
// reasoning is fine, and the statement it lands on is not. Extraction finds it
// and the guard kills it. Extraction must NOT be a way past the guard.
func TestReasoningThenHostileSQL(t *testing.T) {
	raw := `The user asked which runs failed. Let me think about the schema.

I could read cost_per_run, but honestly the raw event log would tell me more —
run_events has the whole payload. Let me query that directly instead.

` + "```sql" + `
SELECT payload FROM run_events WHERE type = 'verdict.recorded'
` + "```"

	got, err := history.ExtractSQL(raw)
	if err != nil {
		t.Fatalf("extraction should still find the statement: %v", err)
	}
	if !strings.Contains(got, "run_events") {
		t.Fatalf("extracted %q, want the hostile statement so the guard can judge it", got)
	}
	_, err = history.Guard(got, history.Scope{Operator: true}, 10)
	if !errors.Is(err, history.ErrRefused) {
		t.Fatalf("the guard accepted a read of the raw event log: %v", err)
	}
	if !strings.Contains(err.Error(), "allowlisted") {
		t.Errorf("refusal reason %q does not name the allowlist", err)
	}
}

// TestGuardInjectsTheOwnerPredicateAndTheBound — the two rewrites, on a
// statement that survives.
func TestGuardInjectsTheOwnerPredicateAndTheBound(t *testing.T) {
	res, err := history.Guard("SELECT * FROM cost_per_run", history.Scope{UserID: "alice"}, 10)
	if err != nil {
		t.Fatalf("Guard: %v", err)
	}
	if res.Scoped != 1 {
		t.Errorf("Scoped = %d, want 1 view reference under the owner predicate", res.Scoped)
	}
	if !strings.Contains(res.Statement, "? = '' OR user_id = ?") {
		t.Errorf("the statement carries no owner predicate: %s", res.Statement)
	}
	if !strings.HasSuffix(res.Statement, "LIMIT ?") {
		t.Errorf("the statement carries no injected bound: %s", res.Statement)
	}
	if len(res.Args) != 3 {
		t.Fatalf("args = %v, want two scope values and the bound", res.Args)
	}
	if res.Args[0] != "alice" || res.Args[1] != "alice" {
		t.Errorf("a member's scope bound %v, want their identity", res.Args[:2])
	}
	if res.Args[2] != res.Limit+1 {
		t.Errorf("the bound argument is %v, want limit+1 (%d) so truncation is observed", res.Args[2], res.Limit+1)
	}
	// The operator binds the empty string, which the predicate short-circuits.
	op, err := history.Guard("SELECT * FROM cost_per_run", history.Scope{Operator: true}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if op.Args[0] != "" {
		t.Errorf("the operator bound %q, want the empty string", op.Args[0])
	}
}

// TestGuardPreservesAliases — the rewrite must not break a statement that
// names its tables, or the guard becomes a source of false refusals.
func TestGuardPreservesAliases(t *testing.T) {
	for _, stmt := range []string{
		"SELECT c.run_id FROM cost_per_run c",
		"SELECT c.run_id FROM cost_per_run AS c",
		"SELECT cost_per_run.run_id FROM cost_per_run",
		"SELECT a.run_id FROM cost_per_run a JOIN cost_done_directly b ON a.run_id = b.run_id",
		"WITH t AS (SELECT * FROM cost_per_run) SELECT * FROM t",
		"SELECT count(*) FROM cost_per_run WHERE pricing_status = 'UNPRICED'",
		"SELECT replace(pricing_status, 'UN', '') FROM cost_per_run",
	} {
		if _, err := history.Guard(stmt, history.Scope{UserID: "alice"}, 10); err != nil {
			t.Errorf("Guard(%q) refused a legitimate read: %v", stmt, err)
		}
	}
}

// TestTheBoundIsTightenedNotTrusted — a model-supplied limit larger than the
// ceiling does not widen the answer.
func TestTheBoundIsTightenedNotTrusted(t *testing.T) {
	res, err := history.Guard("SELECT * FROM cost_per_run LIMIT 1000000", history.Scope{Operator: true}, 1000000)
	if err != nil {
		t.Fatalf("Guard: %v", err)
	}
	if res.Limit > history.OpenSQLMaxRows {
		t.Errorf("injected bound %d exceeds the Layer-2 ceiling %d", res.Limit, history.OpenSQLMaxRows)
	}
}

// TestGuardDoesNotPanicOnLeadingWhitespace — drain r2 R1. The D4 tail
// truncation used strings.TrimSpace, which strips the LEADING side too and
// shifted the text against token offsets computed from the original — so
// Guard PANICKED with a slice-bounds error instead of returning.
//
// Measured before the fix: `Guard("   SELECT * FROM cost_per_run", …)` →
// panic: slice bounds out of range [29:26]; the leading-newline/tab and
// leading-space-with-trailing-comment forms panicked the same way.
//
// A panic in a guardrail is a CRASH, not a refusal, and Guard is exported —
// B6/S15 is its next caller. The precondition is enforced, not documented.
func TestGuardDoesNotPanicOnLeadingWhitespace(t *testing.T) {
	for _, stmt := range []string{
		"   SELECT * FROM cost_per_run",
		"\n\t SELECT * FROM cost_per_run",
		"\n\nSELECT * FROM cost_per_run",
		"   SELECT * FROM cost_per_run ;",
		"\n\t SELECT * FROM cost_per_run ;",
		"  SELECT * FROM cost_per_run -- trailing",
		"\n  SELECT * FROM cost_per_run -- trailing\n",
		"  SELECT c.run_id FROM cost_per_run AS c ;",
		"\t WITH t AS (SELECT * FROM cost_per_run) SELECT * FROM t ;",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Guard(%q) PANICKED: %v — a guardrail must refuse, never crash", stmt, r)
				}
			}()
			res, err := history.Guard(stmt, history.Scope{UserID: "alice"}, 10)
			if err != nil {
				t.Errorf("Guard(%q) refused a legitimate read: %v", stmt, err)
				return
			}
			if !strings.HasSuffix(res.Statement, "LIMIT ?") {
				t.Errorf("Guard(%q) built %q — no injected bound", stmt, res.Statement)
			}
			if strings.Contains(res.Statement, ";") {
				t.Errorf("Guard(%q) built %q — the separator survived", stmt, res.Statement)
			}
			if !strings.Contains(res.Statement, `AS "`) {
				t.Errorf("Guard(%q) built %q — the alias is not defensively quoted", stmt, res.Statement)
			}
		}()
	}
}

// TestLeadingWhitespaceStatementsExecute — the other half of R1: not panicking
// is not enough, the statement it builds must still run.
func TestLeadingWhitespaceStatementsExecute(t *testing.T) {
	f := newFixture(t)
	seedTwoOwners(t, f)
	ro, err := f.db.OpenReadOnly(f.ctx)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })

	for _, stmt := range []string{
		"   SELECT * FROM cost_per_run",
		"\n\t SELECT * FROM cost_per_run ;",
		"  SELECT * FROM cost_per_run -- trailing",
	} {
		res, err := history.Guard(stmt, history.Scope{UserID: member1}, 10)
		if err != nil {
			t.Errorf("Guard(%q): %v", stmt, err)
			continue
		}
		rows, err := ro.QueryContext(f.ctx, res.Statement, res.Args...)
		if err != nil {
			t.Errorf("built statement did not execute for %q: %v\n  %s", stmt, err, res.Statement)
			continue
		}
		rows.Close()
	}
}

// TestQuotedAliasIsRefusedEvenWhenItsContentIsPlain — drain r2 R4, limb
// `checkAlias` on its own. Limb 3a (plainIdent over every quoted identifier)
// already catches an alias carrying SQL, so it MASKS this one: the case that
// separates them is a quoted alias whose content IS a plain name. 3a passes it;
// checkAlias must not. Neutering checkAlias makes exactly this test fail.
func TestQuotedAliasIsRefusedEvenWhenItsContentIsPlain(t *testing.T) {
	for _, stmt := range []string{
		`SELECT * FROM cost_per_run AS "alias"`,
		`SELECT * FROM cost_per_run "alias"`,
		"SELECT * FROM cost_per_run AS [alias]",
		"SELECT * FROM cost_per_run AS `alias`",
	} {
		err := func() error {
			_, err := history.Guard(stmt, history.Scope{UserID: "alice"}, 10)
			return err
		}()
		if !errors.Is(err, history.ErrRefused) {
			t.Errorf("Guard(%q) = %v — a quoted alias must be refused on its own limb", stmt, err)
			continue
		}
		if !strings.Contains(err.Error(), "quoted table alias") {
			t.Errorf("Guard(%q) refused with %q — want the alias limb's own reason", stmt, err)
		}
	}
	// The control: the same alias UNQUOTED is accepted, so the limb refuses the
	// quoting and not the name.
	if _, err := history.Guard("SELECT * FROM cost_per_run AS alias", history.Scope{UserID: "alice"}, 10); err != nil {
		t.Errorf("an unquoted alias was refused: %v", err)
	}
}

// TestEmittedAliasIsDefensivelyQuoted — drain r2 R4, limb `quoteIdent` on its
// own. checkAlias already refuses anything that could carry SQL, so this limb
// is redundant TODAY and exists against a future relaxation of the checks above
// — which is precisely the scenario no other test would catch. Neutering
// quoteIdent makes this test fail.
func TestEmittedAliasIsDefensivelyQuoted(t *testing.T) {
	cases := map[string]string{
		"SELECT * FROM cost_per_run":                    `AS "cost_per_run"`,
		"SELECT c.run_id FROM cost_per_run c":           `AS "c"`,
		"SELECT c.run_id FROM cost_per_run AS c":        `AS "c"`,
		"SELECT * FROM cost_per_run JOIN cost_per_task": `AS "cost_per_task"`,
	}
	for stmt, want := range cases {
		res, err := history.Guard(stmt, history.Scope{UserID: "alice"}, 10)
		if err != nil {
			t.Errorf("Guard(%q): %v", stmt, err)
			continue
		}
		if !strings.Contains(res.Statement, want) {
			t.Errorf("Guard(%q) emitted\n  %s\nwant the alias quoted as %s — the emission limb is the belt behind checkAlias",
				stmt, res.Statement, want)
		}
	}
	// The escape is doubled, so a quote inside a name can never close its own
	// quoting. Asserted on the helper's observable contract through a name that
	// reaches emission as the view's own.
	if got := history.QuoteIdentForTest(`a"b`); got != `"a""b"` {
		t.Errorf("quoteIdent(%q) = %q, want %q (doubled-quote escape)", `a"b`, got, `"a""b"`)
	}
}

// TestOffsetBeltRefusesStaleSpans — the fail-closed belt behind the rewrite, on
// its own. It is unreachable while the rewrite's arithmetic is right, which is
// exactly why nothing else can assert it: the drain-r2 R1 panic proved that a
// shifted span reaches a slice, so the belt must REFUSE such a span rather than
// let it through. Neutering checkOffsets makes this test fail.
func TestOffsetBeltRefusesStaleSpans(t *testing.T) {
	const src = "select * from cost_per_run"
	bad := map[string][][2]int{
		"end past the source":   {{0, len(src) + 3}},            // the R1 shape
		"end before start":      {{10, 4}},                      // inverted span
		"out of order":          {{12, 20}, {0, 6}},             // second precedes the first
		"overlapping":           {{0, 12}, {6, 20}},             // spans share bytes
		"negative start":        {{-1, 6}},                      // underflow
		"start past the source": {{len(src) + 1, len(src) + 2}}, // wholly outside
	}
	for name, spans := range bad {
		if err := history.CheckOffsetsForTest(src, spans); err == nil {
			t.Errorf("%s: the offset belt admitted %v — a stale span must refuse, never reach the slice", name, spans)
		}
	}
	// A well-formed, ordered, in-range set still passes: the belt refuses stale
	// spans, it does not refuse the rewrite itself.
	if err := history.CheckOffsetsForTest(src, [][2]int{{0, 6}, {14, len(src)}}); err != nil {
		t.Errorf("the offset belt refused a valid span set: %v", err)
	}
}
