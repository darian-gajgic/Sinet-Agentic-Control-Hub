package history_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
)

// moneyViewMigrations are the migrations that create or re-create a Layer-0
// money view. The no-money-by-generation scan covers every one of them: a view
// RE-created in a later file is exactly as capable of computing money as the
// file that first created it. 0022 (P3-RW-3) re-creates cost_per_project over
// the completed task→project edge, and 0023 (P3-RW-7) re-creates it again over
// the same edge widened with the onboarding arm.
var moneyViewMigrations = []string{
	"0016_queryable_history.sql",
	"0022_pin_attribution.sql",
	"0023_onboarding_attribution.sql",
}

// migrationText reads migration 0016 — the views' authority. Tests assert
// against the SHIPPING text, never a copy of it (the §36 drain r2 R1 lesson: a
// test asserting a property of an expression that is not the one shipping is
// how a regression survives).
func migrationText(t *testing.T) string {
	t.Helper()
	return migrationTextOf(t, moneyViewMigrations[0])
}

func migrationTextOf(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "storage", "migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(b)
}

// migrationSQLOnly strips comment lines, so a scan for a forbidden CONSTRUCT
// judges the SQL rather than the prose explaining why the construct is absent.
// A citation in a comment is the correct place for a citation.
func migrationSQLOnly(t *testing.T) string {
	t.Helper()
	return migrationSQLOnlyOf(t, moneyViewMigrations[0])
}

func migrationSQLOnlyOf(t *testing.T, name string) string {
	t.Helper()
	var b strings.Builder
	for _, line := range strings.Split(migrationTextOf(t, name), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestEveryCostQuestionIsANamedView — acceptance 21. S14.10 ¶1 names nine cost
// questions; each must resolve to a named SQL view that actually exists in the
// database and is selectable.
func TestEveryCostQuestionIsANamedView(t *testing.T) {
	want := []history.CostQuestion{
		history.QuestionPerRun, history.QuestionPerTask, history.QuestionPerProject,
		history.QuestionPerPerson, history.QuestionPerPeriod,
		history.QuestionBudgetRemainder, history.QuestionBurnRate,
		history.QuestionLimitEvents, history.QuestionDoneDirectly,
	}
	got := map[history.CostQuestion]bool{}
	for _, q := range history.CostQuestions() {
		got[q] = true
	}
	for _, q := range want {
		if !got[q] {
			t.Errorf("S2.5 cost question %q has no named Layer-0 view", q)
		}
	}

	f := newFixture(t)
	for _, v := range history.Views() {
		// Every registered view name must be a real view in the schema.
		var kind string
		err := f.db.QueryRowContext(f.ctx,
			`SELECT type FROM sqlite_schema WHERE name = ?`, v.Name).Scan(&kind)
		if err != nil {
			t.Errorf("view %q registered but not in the schema: %v", v.Name, err)
			continue
		}
		if kind != "view" {
			t.Errorf("%q is a %s, not a view", v.Name, kind)
		}
		if v.OwnerColumn == "" {
			continue
		}
		if _, err := f.st.SelectView(f.ctx, v.Name, opScope(), 10); err != nil {
			t.Errorf("SelectView(%q): %v", v.Name, err)
		}
	}
}

// TestNoMoneyByGeneration — acceptance 22, the checkable NEGATIVE. S14.10 ¶1:
// "the assistant SELECTS views — it never computes money by generation."
//
// The guard has two limbs and a non-tautology probe:
//   - the query layer has no token→currency arithmetic and no rate;
//   - it does not import internal/metering, so it cannot reach the price table.
func TestNoMoneyByGeneration(t *testing.T) {
	// Limb 1: every money view's migration text. A money figure may be READ
	// from a receipt or DIVIDED by a count of days (a rate over time is not a
	// rate per token); it may never be multiplied by anything, and no
	// price/rate identifier may appear.
	tokenFields := []string{
		"prompt_tokens", "billed_output_tokens", "cache_read_tokens",
		"cache_creation_tokens", "server_tool_calls", "output_tokens", "input_tokens",
	}
	// Any `*` in an arithmetic position over a usd column would be the defect.
	// Restrict the scan to lines mentioning usd so a `SELECT *` never trips it.
	mult := regexp.MustCompile(`[a-z_0-9)]\s*\*\s*[a-z_0-9(]`)
	for _, name := range moneyViewMigrations {
		sql := migrationSQLOnlyOf(t, name)
		for _, tf := range tokenFields {
			if strings.Contains(sql, tf) {
				t.Errorf("migration %s reads the token field %q — the query layer selects PRICED figures; pricing lives in internal/metering (S14.10 ¶1)", name, tf)
			}
		}
		for _, bad := range []string{"per_token", "per_million", "price_per", "rate_usd", "* price", "price *"} {
			if strings.Contains(strings.ToLower(sql), bad) {
				t.Errorf("migration %s contains %q — no token→currency arithmetic may exist in the query layer", name, bad)
			}
		}
		for _, line := range strings.Split(sql, "\n") {
			l := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(l, "--") || !strings.Contains(l, "usd") {
				continue
			}
			if mult.MatchString(l) {
				t.Errorf("migration %s multiplies in a money expression — money is READ, never computed:\n  %s", name, line)
			}
		}
	}

	// Limb 2: the package source.
	for _, name := range goFiles(t, ".") {
		src := readFile(t, name)
		for _, tf := range tokenFields {
			if strings.Contains(src, tf) {
				t.Errorf("%s references the token field %q — the query layer never turns tokens into money", name, tf)
			}
		}
		if strings.Contains(src, `"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"`) {
			t.Errorf("%s imports internal/metering — the query layer selects a priced view; it never reaches the price table", name)
		}
	}

	// Non-tautology probe: the scan must be capable of failing.
	probe := "SELECT prompt_tokens * 3.0 AS priced_usd FROM x"
	if !strings.Contains(probe, tokenFields[0]) || !mult.MatchString(strings.ToLower(probe)) {
		t.Fatal("the money-by-generation scan cannot detect its own probe — it would pass vacuously")
	}
}

// TestUnpricedRendersUnpricedNeverZero — acceptance 23. The v0 price table
// ships EMPTY, so a receipt whose calls all went unpriced must render UNPRICED
// with a NULL amount. A silent 0.0 is indistinguishable from genuinely free
// work (S10.1: "no row silently prices to $0").
func TestUnpricedRendersUnpricedNeverZero(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t1", member1, "A task", "doing")
	f.run("r-unpriced", member1, "t1", "completed", "anthropic")
	f.run("r-partial", member1, "t1", "completed", "anthropic")
	f.run("r-noreceipt", member1, "t1", "completed", "anthropic")
	// Every call unpriced — the v0 posture.
	f.receipt("r-unpriced", member1, 0, 4, 4)
	// Some priced, some not.
	f.receipt("r-partial", member1, 1.25, 4, 1)

	a, err := f.st.SelectView(f.ctx, history.ViewCostPerRun, opScope(), 50)
	if err != nil {
		t.Fatal(err)
	}
	byRun := map[string]int{}
	for i := range a.Rows {
		byRun[str(cell(t, a, i, "run_id"))] = i
	}
	for _, tc := range []struct {
		run, status string
		nilAmount   bool
	}{
		{"r-unpriced", "UNPRICED", true},
		{"r-partial", "PARTIAL", false},
		{"r-noreceipt", "NO-RECEIPT", true},
	} {
		i, ok := byRun[tc.run]
		if !ok {
			t.Fatalf("run %q missing from cost_per_run", tc.run)
		}
		if got := str(cell(t, a, i, "pricing_status")); got != tc.status {
			t.Errorf("run %q pricing_status = %q, want %q", tc.run, got, tc.status)
		}
		amount := cell(t, a, i, "priced_usd")
		if tc.nilAmount && amount != nil {
			t.Errorf("run %q priced_usd = %v, want NULL — an unpriced row must never render a number (S10.1)", tc.run, amount)
		}
		if !tc.nilAmount && amount == nil {
			t.Errorf("run %q priced_usd is NULL but some calls priced", tc.run)
		}
	}
}

// TestBudgetRemainderIsAnHonestAbsence — acceptance 24. No operator budget is
// persisted anywhere at v0, so the remainder has no minuend. The view must
// report the absence WITH ITS REASON, not a fabricated figure and not a zero.
func TestBudgetRemainderIsAnHonestAbsence(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t1", member1, "A task", "doing")
	f.run("r1", member1, "t1", "completed", "anthropic")
	f.receipt("r1", member1, 0, 2, 2)

	a, err := f.st.SelectView(f.ctx, history.ViewBudgetRemainder, opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) == 0 {
		t.Fatal("cost_budget_remainder returned no row — the absence must be reported for the person, not by returning nothing")
	}
	if got := cell(t, a, 0, "remainder_usd"); got != nil {
		t.Errorf("remainder_usd = %v, want NULL — no budget is persisted, so no remainder exists", got)
	}
	if got := str(cell(t, a, 0, "remainder_status")); got != "UNAVAILABLE" {
		t.Errorf("remainder_status = %q, want UNAVAILABLE", got)
	}
	reason := str(cell(t, a, 0, "absence_reason"))
	if !strings.Contains(reason, "no operator budget") {
		t.Errorf("absence_reason does not say why the figure is absent: %q", reason)
	}
	if got := str(cell(t, a, 0, "budget_declared")); got != "0" {
		t.Errorf("budget_declared = %q, want 0", got)
	}
}

// TestDoneDirectlyCitesTheRegisteredFormula — acceptance 25. The done-directly
// formula is REGISTERED in Spec/benchmark-preregistration-v1.md §13 and its
// numbers are read-only (BENCH-REG §17). The view must CITE the receipt's own
// formula_ref rather than restate anything.
func TestDoneDirectlyCitesTheRegisteredFormula(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t1", member1, "A task", "doing")
	f.run("r1", member1, "t1", "completed", "anthropic")
	f.receipt("r1", member1, 0, 2, 2)

	a, err := f.st.SelectView(f.ctx, history.ViewDoneDirectly, opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) != 1 {
		t.Fatalf("cost_done_directly returned %d rows, want 1", len(a.Rows))
	}
	if got := str(cell(t, a, 0, "formula_ref")); got != "Spec/benchmark-preregistration-v1.md §13" {
		t.Errorf("formula_ref = %q — the view must carry the receipt's citation", got)
	}
	if got := str(cell(t, a, 0, "pricing_status")); got != "UNPRICED" {
		t.Errorf("pricing_status = %q, want UNPRICED at the empty v0 price table", got)
	}
	if got := cell(t, a, 0, "heuristic_usd"); got != nil {
		t.Errorf("heuristic_usd = %v on an unpriced receipt, want NULL", got)
	}

	// The migration must not RESTATE the registration: it selects the
	// receipt's own formula_ref and never embeds the reference text itself.
	sql := migrationSQLOnly(t)
	if !strings.Contains(sql, "$.direct_use.formula_ref") {
		t.Error("the done-directly view does not select the receipt's formula_ref — it must cite, not restate")
	}
	if strings.Contains(sql, "benchmark-preregistration") {
		t.Error("migration 0016 embeds the registration reference — a registered number/label is CITED from the receipt, never restated (BENCH-REG §17)")
	}
}

// TestLayer0IsOwnerScoped — S01.9 on the deterministic layer: the operator sees
// every row including another owner's; a member sees only their own.
func TestLayer0IsOwnerScoped(t *testing.T) {
	f := newFixture(t)
	for _, u := range []string{member1, member2} {
		f.user(u, "member")
		f.task("t-"+u, u, "task "+u, "doing")
		f.run("r-"+u, u, "t-"+u, "completed", "anthropic")
		f.receipt("r-"+u, u, 0, 1, 1)
	}
	op, err := f.st.SelectView(f.ctx, history.ViewCostPerRun, opScope(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Rows) != 2 {
		t.Fatalf("operator sees %d runs, want 2", len(op.Rows))
	}
	mem, err := f.st.SelectView(f.ctx, history.ViewCostPerRun, memberScope(member1), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(mem.Rows) != 1 {
		t.Fatalf("member sees %d runs, want 1", len(mem.Rows))
	}
	if got := str(cell(t, mem, 0, "user_id")); got != member1 {
		t.Errorf("member scope leaked owner %q", got)
	}

	// A view with no owner column is operator-only rather than silently
	// unscoped: refusing is honest, leaking is not.
	if _, err := f.st.SelectView(f.ctx, history.ViewTaskProjectEdge, memberScope(member1), 10); err == nil {
		t.Error("an owner-column-less view was served to a member — it must be refused (S01.9)")
	}
}

// TestSelectViewRefusesUnknownNames — a view name never originates in user text.
func TestSelectViewRefusesUnknownNames(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"runs", "run_events", "receipts", "cost_per_run; DROP TABLE runs", ""} {
		if _, err := f.st.SelectView(f.ctx, name, opScope(), 10); err == nil {
			t.Errorf("SelectView(%q) succeeded — only registry names are selectable", name)
		}
	}
}

// TestCostAltitudesAggregateConsistently — the per-task/person/project
// altitudes must account for the same money the per-run altitude does. In
// particular the project altitude must not DROP work with no project: it lands
// in the explicit '(no project)' bucket.
func TestCostAltitudesAggregateConsistently(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t1", member1, "Task one", "doing")
	f.task("t2", member1, "Task two", "doing")
	f.run("r1", member1, "t1", "completed", "anthropic")
	f.run("r2", member1, "t2", "completed", "anthropic")
	f.receipt("r1", member1, 2.0, 4, 0)
	f.receipt("r2", member1, 3.0, 4, 0)
	// r1's task is claimed for a project; r2's is not.
	f.exec(`INSERT INTO repo_registry (project_id, user_id, name, store_path, default_branch, state, created_ts, updated_ts)
	        VALUES ('p1', ?, 'Proj One', '/tmp/p1', 'main', 'active', ?, ?)`, member1, f.ts(0), f.ts(0))
	f.exec(`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	        VALUES ('t1', 'p1', ?, 'src/**', 'W', 'held', ?)`, member1, f.ts(0))

	person, err := f.st.SelectView(f.ctx, history.ViewCostPerPerson, opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := str(cell(t, person, 0, "priced_usd")); got != "5" {
		t.Errorf("cost_per_person priced_usd = %q, want 5", got)
	}

	proj, err := f.st.SelectView(f.ctx, history.ViewCostPerProject, opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	buckets := map[string]string{}
	for i := range proj.Rows {
		id := str(cell(t, proj, i, "project_id"))
		buckets[id] = str(cell(t, proj, i, "priced_usd"))
		switch buckets[id] {
		case "2":
			total += 2
		case "3":
			total += 3
		}
	}
	if _, ok := buckets["p1"]; !ok {
		t.Errorf("cost_per_project has no p1 bucket: %v", buckets)
	}
	if _, ok := buckets["(no project)"]; !ok {
		t.Errorf("work with no project was DROPPED rather than bucketed: %v", buckets)
	}
	if total != 5 {
		t.Errorf("cost_per_project totals %v, want 5 — the project altitude must account for the same money", total)
	}
	if got := str(cell(t, proj, 0, "project_name")); got != "Proj One" && got != "" {
		t.Errorf("unexpected project_name %q", got)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			out = append(out, filepath.Join(dir, n))
		}
	}
	if len(out) == 0 {
		t.Fatalf("no source files found in %s — the scan would pass vacuously", dir)
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
