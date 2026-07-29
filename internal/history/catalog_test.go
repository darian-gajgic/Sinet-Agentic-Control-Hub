package history_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

func runID(i int) string { return fmt.Sprintf("r-bulk-%02d", i) }

// TestCatalogIsInBandAndCoversEveryCategory — acceptance 27, first half. OQ3:
// "~30–50" is a BAND, not a registered number; the catalog must sit inside it
// with every named category represented. A thin catalog fails the intent (it is
// the reliability floor); an inflated one is padding.
func TestCatalogIsInBandAndCoversEveryCategory(t *testing.T) {
	c := history.Catalog()
	if len(c) < 30 || len(c) > 50 {
		t.Errorf("catalog holds %d queries, want the S14.10 ¶2 band of 30–50", len(c))
	}
	byCat := map[history.Category]int{}
	for _, q := range c {
		byCat[q.Category]++
	}
	for _, cat := range history.Categories {
		if byCat[cat] == 0 {
			t.Errorf("category %q has no catalog query — S14.10 ¶2 names it", cat)
		}
	}
	for cat := range byCat {
		known := false
		for _, k := range history.Categories {
			if k == cat {
				known = true
			}
		}
		if !known {
			t.Errorf("catalog query carries the unregistered category %q", cat)
		}
	}
	// Names are unique and namespaced by category, so an answer's query name
	// reads as a place in the catalog rather than a bare word.
	seen := map[string]bool{}
	for _, q := range c {
		if seen[q.Name] {
			t.Errorf("duplicate catalog name %q", q.Name)
		}
		seen[q.Name] = true
		if !strings.Contains(q.Name, ".") {
			t.Errorf("catalog name %q is not namespaced", q.Name)
		}
		if strings.TrimSpace(q.Description) == "" {
			t.Errorf("catalog entry %q has no description — a disambiguation card renders it", q.Name)
		}
	}
}

// TestEveryCatalogQueryIsStructurallySafe — acceptance 27, second half. The
// four structural rules, asserted over the WHOLE table rather than trusted per
// row: parameterized only, owner-scoped, bounded, single-statement.
func TestEveryCatalogQueryIsStructurallySafe(t *testing.T) {
	for _, q := range history.Catalog() {
		sql := q.SQL
		if sql == "" {
			t.Errorf("%s: no SQL", q.Name)
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(sql), "SELECT") {
			t.Errorf("%s: does not start with SELECT — the catalog reads, it never writes", q.Name)
		}
		if strings.Contains(sql, ";") {
			t.Errorf("%s: contains a statement separator — one query, one statement", q.Name)
		}
		// Rule 1: `?` placeholders only. A %-verb would mean the SQL was built
		// by formatting, which is the interpolation S14.10 ¶2 rules out.
		for _, verb := range []string{"%s", "%q", "%v", "%d"} {
			if strings.Contains(sql, verb) {
				t.Errorf("%s: contains the format verb %q — slot values are BOUND, never formatted", q.Name, verb)
			}
		}
		// Rule 2: the owner-scope predicate, in the exact text production uses.
		if !strings.Contains(sql, history.OwnerScopeOpen) || !strings.Contains(sql, history.OwnerScopeClose) {
			t.Errorf("%s: carries no owner-scope predicate — S01.9 applies to every projection", q.Name)
		}
		// Rule 3: bounded.
		if !strings.HasSuffix(strings.TrimSpace(sql), "LIMIT ?") {
			t.Errorf("%s: does not end LIMIT ? — no answer is unbounded", q.Name)
		}
		// Rule 4: the placeholder count matches the bind order the executor
		// uses (slots, then the scope value twice, then the limit).
		want := len(q.Slots) + 3
		if got := strings.Count(sql, "?"); got != want {
			t.Errorf("%s: %d placeholders, want %d (%d slots + 2 scope + 1 limit) — bind order and SQL have drifted apart",
				q.Name, got, want, len(q.Slots))
		}
		// Slot names must not collide with the grammar's own members.
		for _, sl := range q.Slots {
			if sl.Name == "reason" || sl.Name == "abstain" {
				t.Errorf("%s: slot %q collides with the schema's own reason/abstain members", q.Name, sl.Name)
			}
			if sl.Type == "" {
				t.Errorf("%s: slot %q is untyped — S14.10 ¶2 requires typed slots", q.Name, sl.Name)
			}
			if strings.TrimSpace(sl.Desc) == "" {
				t.Errorf("%s: slot %q has no description — the slot filler is prompted with it", q.Name, sl.Name)
			}
		}
	}

	// Non-tautology probe: the rules must be capable of failing.
	bad := `SELECT * FROM runs WHERE state = '%s'; DROP TABLE runs`
	if !strings.Contains(bad, "%s") || !strings.Contains(bad, ";") ||
		strings.Contains(bad, history.OwnerScopeOpen) || strings.HasSuffix(bad, "LIMIT ?") {
		t.Fatal("the catalog structural rules cannot detect their own probe — they would pass vacuously")
	}
}

// TestEveryFilterAxisIsReachable — acceptance 30, first half. The S14.10
// preamble names eight filter/search axes; each must be reachable through at
// least one catalog query's typed slot.
func TestEveryFilterAxisIsReachable(t *testing.T) {
	reached := map[history.SlotType][]string{}
	for _, q := range history.Catalog() {
		for _, sl := range q.Slots {
			reached[sl.Type] = append(reached[sl.Type], q.Name)
		}
	}
	for _, axis := range history.Axes {
		if len(reached[axis]) == 0 {
			t.Errorf("filter axis %q is reachable through no catalog query (S14.10 preamble)", axis)
		}
	}
	if len(history.Axes) < 8 {
		t.Fatalf("the axis list holds %d entries — S14.10's preamble names eight", len(history.Axes))
	}
}

// TestEveryAxisRunsAndIsOwnerScoped — acceptance 30, second half. Every axis is
// exercised END TO END against a two-owner database: the operator sees both
// owners' rows, the member sees only their own, and the axis actually filters.
func TestEveryAxisRunsAndIsOwnerScoped(t *testing.T) {
	f := newFixture(t)
	seedTwoOwners(t, f)

	// One query per axis, with a value that matches alice's row only.
	cases := []struct {
		axis  history.SlotType
		query string
		slots map[string]string
	}{
		{history.SlotStatus, "status.runs_by_state", map[string]string{"state": "completed"}},
		{history.SlotTaskID, "status.runs_for_task", map[string]string{"task_id": "t-" + member1}},
		{history.SlotLane, "status.runs_by_lane", map[string]string{"lane": "lane-" + member1}},
		{history.SlotRunID, "verdicts.for_run", map[string]string{"run_id": "r-" + member1}},
		{history.SlotDomain, "verdicts.by_domain", map[string]string{"domain": "software"}},
		{history.SlotWorker, "routing.by_worker", map[string]string{"worker": "wt-" + member1}},
		{history.SlotProject, "cost.for_project", map[string]string{"project": "(no project)"}},
		{history.SlotPerson, "cost.for_person", map[string]string{"person": member1}},
		{history.SlotDateFrom, "status.runs_in_date_range", map[string]string{
			"from": f.ts(-time.Hour), "to": f.ts(time.Hour)}},
	}
	for _, c := range cases {
		t.Run(string(c.axis), func(t *testing.T) {
			op, err := f.st.RunQuery(f.ctx, c.query, c.slots, opScope(), 50)
			if err != nil {
				t.Fatalf("operator %s: %v", c.query, err)
			}
			mem, err := f.st.RunQuery(f.ctx, c.query, c.slots, memberScope(member1), 50)
			if err != nil {
				t.Fatalf("member %s: %v", c.query, err)
			}
			other, err := f.st.RunQuery(f.ctx, c.query, c.slots, memberScope(member2), 50)
			if err != nil {
				t.Fatalf("other member %s: %v", c.query, err)
			}
			if len(op.Rows) == 0 {
				t.Fatalf("%s returned nothing for the operator — the axis is not exercised", c.query)
			}
			if len(mem.Rows) == 0 {
				t.Fatalf("%s returned nothing for the owning member", c.query)
			}
			if len(other.Rows) != 0 {
				t.Errorf("%s leaked %d rows to a member who owns none (S01.9)", c.query, len(other.Rows))
			}
			if op.Query != c.query {
				t.Errorf("answer names %q, want %q", op.Query, c.query)
			}
		})
	}
}

// TestEveryCatalogQueryExecutes — the whole table runs against a real schema.
// A query with a typo in a column name is a broken floor, and the floor is the
// point.
func TestEveryCatalogQueryExecutes(t *testing.T) {
	f := newFixture(t)
	seedTwoOwners(t, f)
	for _, q := range history.Catalog() {
		slots := map[string]string{}
		for _, sl := range q.Slots {
			slots[sl.Name] = sampleSlotValue(f, sl)
		}
		if _, err := f.st.RunQuery(f.ctx, q.Name, slots, opScope(), 10); err != nil {
			t.Errorf("%s: %v", q.Name, err)
		}
		// And under a member scope, which is a different code path through the
		// same predicate.
		if _, err := f.st.RunQuery(f.ctx, q.Name, slots, memberScope(member1), 10); err != nil {
			t.Errorf("%s (member scope): %v", q.Name, err)
		}
	}
}

// TestRunQueryRefusesBadSlots — a filter that silently did not apply returns a
// WIDER answer than the caller asked for, so a missing or unknown slot is an
// error rather than an empty string.
func TestRunQueryRefusesBadSlots(t *testing.T) {
	f := newFixture(t)
	if _, err := f.st.RunQuery(f.ctx, "status.runs_by_state", map[string]string{}, opScope(), 10); err == nil {
		t.Error("a missing required slot was accepted")
	}
	if _, err := f.st.RunQuery(f.ctx, "status.runs_by_state",
		map[string]string{"state": "running", "sneaky": "x"}, opScope(), 10); err == nil {
		t.Error("an unknown slot was accepted")
	}
	if _, err := f.st.RunQuery(f.ctx, "no.such.query", nil, opScope(), 10); err == nil {
		t.Error("an unknown query name was accepted")
	}
}

// TestSlotValuesAreBoundNotInterpolated — the SQL-injection negative for
// Layer 1: a slot value carrying SQL is DATA. It filters nothing and destroys
// nothing.
func TestSlotValuesAreBoundNotInterpolated(t *testing.T) {
	f := newFixture(t)
	seedTwoOwners(t, f)
	hostile := []string{
		`completed' OR '1'='1`,
		`completed'; DROP TABLE runs; --`,
		`' UNION SELECT payload FROM run_events --`,
	}
	for _, h := range hostile {
		a, err := f.st.RunQuery(f.ctx, "status.runs_by_state", map[string]string{"state": h}, opScope(), 50)
		if err != nil {
			t.Fatalf("bound hostile value errored instead of matching nothing: %v", err)
		}
		if len(a.Rows) != 0 {
			t.Errorf("hostile slot value %q matched %d rows — it must be DATA", h, len(a.Rows))
		}
	}
	// The tables are still there.
	var n int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM runs`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("runs table damaged by a bound value: n=%d err=%v", n, err)
	}
}

// TestEveryAnswerCarriesItsQueryName — acceptance 29 / R26, across every verb
// that produces an answer.
func TestEveryAnswerCarriesItsQueryName(t *testing.T) {
	f := newFixture(t)
	seedTwoOwners(t, f)
	f.indexHistory()

	answers := map[string]history.Answer{}
	v, err := f.st.SelectView(f.ctx, history.ViewCostPerRun, opScope(), 5)
	if err != nil {
		t.Fatal(err)
	}
	answers["SelectView"] = v
	q, err := f.st.RunQuery(f.ctx, "status.runs_active", nil, opScope(), 5)
	if err != nil {
		t.Fatal(err)
	}
	answers["RunQuery"] = q
	s, err := f.st.Search(f.ctx, "objective", opScope(), 5)
	if err != nil {
		t.Fatal(err)
	}
	answers["Search"] = s
	// The unresolved path still names something: the catalog itself.
	a, err := f.st.Ask(f.ctx, "what happened yesterday?", opScope(), 5)
	if err != nil {
		t.Fatal(err)
	}
	answers["Ask(card)"] = a

	for verb, ans := range answers {
		if strings.TrimSpace(ans.Query) == "" {
			t.Errorf("%s produced an answer with no query name (R26)", verb)
		}
		if ans.Confidence == "" {
			t.Errorf("%s produced an answer with no confidence label", verb)
		}
		switch ans.Layer {
		case history.LayerDeterministic:
			if ans.Confidence != history.ConfidenceDeterministic {
				t.Errorf("%s: layer 0 answer labelled %q", verb, ans.Confidence)
			}
		case history.LayerCanned:
			if ans.Confidence != history.ConfidenceCanned {
				t.Errorf("%s: layer 1 answer labelled %q", verb, ans.Confidence)
			}
		default:
			if ans.Confidence != history.ConfidenceLower {
				t.Errorf("%s: layer 2 answer labelled %q", verb, ans.Confidence)
			}
		}
	}
}

// TestAnswersAreBounded — the structural row bound applies, and truncation is
// OBSERVED rather than inferred.
func TestAnswersAreBounded(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t1", member1, "Task", "doing")
	for i := 0; i < 12; i++ {
		f.run(runID(i), member1, "t1", "completed", "anthropic")
	}
	a, err := f.st.RunQuery(f.ctx, "status.runs_by_state", map[string]string{"state": "completed"}, opScope(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) != 5 {
		t.Errorf("limit 5 returned %d rows", len(a.Rows))
	}
	if !a.Truncated {
		t.Error("a truncated answer did not say so")
	}
	full, err := f.st.RunQuery(f.ctx, "status.runs_by_state", map[string]string{"state": "completed"}, opScope(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if full.Truncated {
		t.Error("a complete answer claimed truncation")
	}
	// A caller-supplied limit above the ceiling is clamped, never honored.
	huge, err := f.st.RunQuery(f.ctx, "status.runs_by_state",
		map[string]string{"state": "completed"}, opScope(), history.MaxRowLimit*10)
	if err != nil {
		t.Fatal(err)
	}
	if len(huge.Rows) != 12 {
		t.Errorf("clamped limit returned %d rows, want 12", len(huge.Rows))
	}
}

// TestEvalScoresServesTheRecordedSuiteResults is the D4(b) unlock, driven.
//
// Before B6-6 no Layer-0 view and no Layer-1 query read `eval.score_recorded`,
// so "what did the last suite run score, and against which floor?" — a
// deterministic question about rows the platform already stores — was reachable
// only through Layer-2 open SQL or a redacted search. This asserts the query
// answers it with the columns the PRODUCER actually mints, and that the two
// paths differ honestly: the runbook path registers a floor, the sweep path
// does not, and a null floor is that fact rather than a zero.
func TestEvalScoresServesTheRecordedSuiteResults(t *testing.T) {
	f := newFixture(t)
	// The two shapes internal/conformance.RecordResult really writes: one from
	// the S14.8 runbook (with the registered floor inside `metrics`), one from
	// the sweep (no floor). Both are platform-attributed, as the producer
	// appends them.
	f.event("", "platform", "eval.score_recorded", map[string]any{
		"suite_id": "routing-regression", "suite_version": "v3", "asset_id": "selector-v7",
		"asset_version": "7", "runner": "internal/evals", "runner_version": "v1", "result": "green",
		"metrics": map[string]any{"floor": 0.82, "floor_green": true, "floor_registered": true},
	})
	f.event("", "platform", "eval.score_recorded", map[string]any{
		"suite_id": "prompt-sweep", "suite_version": "v1", "asset_id": "worker-notes",
		"asset_version": "2", "runner": "internal/evals", "runner_version": "v1", "result": "red",
		"metrics": map[string]any{"assets_evaluated": 4, "assets_red": 1},
	})

	a, err := f.st.RunQuery(f.ctx, "verdicts.eval_scores", nil, opScope(), 20)
	if err != nil {
		t.Fatalf("run the eval-scores query: %v", err)
	}
	if len(a.Rows) != 2 {
		t.Fatalf("the query returned %d rows, want the 2 recorded results", len(a.Rows))
	}
	for _, col := range []string{"suite_id", "asset_id", "result", "floor", "runner"} {
		if !hasColumn(a, col) {
			t.Errorf("the answer has no %q column — a reader cannot tell what was scored", col)
		}
	}
	// Newest first, by the append-only sequence rather than by a timestamp.
	if got := cell(t, a, 0, "suite_id"); got != "prompt-sweep" {
		t.Errorf("row 0 suite is %v, want the newest result first", got)
	}
	// The runbook path carries its registered floor…
	if got := cell(t, a, 1, "floor"); got != 0.82 {
		t.Errorf("the registered floor is %v, want 0.82 read from the producer's own metrics block", got)
	}
	// …and the sweep path honestly carries none.
	if got := cell(t, a, 0, "floor"); got != nil {
		t.Errorf("a sweep result reports floor %v; the path registers none and a number here would be invented", got)
	}

	// Owner scope: the producer attributes these rows to `platform`, so a member
	// sees none. Asserted rather than assumed, because "returns nothing" and
	// "was never scoped" look identical from outside.
	m, err := f.st.RunQuery(f.ctx, "verdicts.eval_scores", nil, memberScope(member1), 20)
	if err != nil {
		t.Fatalf("run the eval-scores query as a member: %v", err)
	}
	if len(m.Rows) != 0 {
		t.Errorf("a member read %d platform-scope eval rows", len(m.Rows))
	}
}

// hasColumn reports whether an answer carries a named column.
func hasColumn(a history.Answer, col string) bool {
	for _, c := range a.Columns {
		if c == col {
			return true
		}
	}
	return false
}
