package history_test

// pinattribution_test.go — P3-RW-3 acceptance on the Layer-0 side: what
// migration 0022 does to the money altitude, what the pin probe costs, and what
// the migration ledger says afterwards.
//
// OQ-(b) is dispositioned B1 — money FOLLOWS attribution: a pinned task's
// intake-ceremony spend lands under its project from birth. The invariant that
// survives every option is 0016's own: the sum over cost_per_project equals the
// sum over cost_per_run. No spend goes missing, none double-counts.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// pinMigration is the file this packet adds. It is named here rather than
// globbed so a differently-named migration is a failure, not a silent pass.
const pinMigration = "0022_pin_attribution.sql"

// pinState seeds one intake.state event carrying a resolved registry slice.
func pinState(f *fixture, runID, owner, project string) {
	f.t.Helper()
	f.event(runID, owner, "intake.state", map[string]any{
		"phase":    "triage",
		"registry": map[string]any{"project": project, "ref": "registry-slice/" + project},
	})
}

// moneyWorld is the seeded world every money assertion below ranges over. It is
// deliberately mixed: attribution by pin only, by claim only, by both, and by
// neither; priced, partially unpriced and receipt-less runs; two owners. A sum
// invariant asserted over one shape proves nothing about the others.
func moneyWorld(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.user(opUser, "operator")
	f.user(member1, "member")
	f.user(member2, "member")

	// Pinned at triage, never approved: no artifact_claims row exists for it.
	f.task("t-pin", member1, "Pinned, unapproved", "doing")
	f.run("r-pin-priced", member1, "t-pin", "completed", "anthropic")
	f.run("r-pin-unpriced", member1, "t-pin", "completed", "anthropic")
	pinState(f, "r-pin-priced", member1, "proj-a")
	f.receipt("r-pin-priced", member1, 1.50, 4, 0)
	f.receipt("r-pin-unpriced", member1, 0, 2, 2)

	// Claimed at plan approval, with no pipeline state of its own.
	f.task("t-claim", member1, "Approved, claimed", "doing")
	f.run("r-claim", member1, "t-claim", "completed", "anthropic")
	f.receipt("r-claim", member1, 2.25, 3, 0)
	f.exec(`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	        VALUES (?,?,?,?,?,?,?)`, "t-claim", "proj-b", member1, `["**"]`, "W", "active", f.ts(0))

	// Both edges, agreeing — the post-approval life of a pinned task.
	f.task("t-both", member1, "Pinned then approved", "doing")
	f.run("r-both", member1, "t-both", "completed", "anthropic")
	pinState(f, "r-both", member1, "proj-a")
	f.receipt("r-both", member1, 3.00, 5, 0)
	f.exec(`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	        VALUES (?,?,?,?,?,?,?)`, "t-both", "proj-a", member1, `["**"]`, "W", "active", f.ts(0))

	// Neither edge, other owner: the honest bucket, plus a terminal run that
	// materialized no receipt at all.
	f.task("t-none", member2, "Unattributed", "doing")
	f.run("r-none", member2, "t-none", "completed", "zai")
	f.run("r-noreceipt", member2, "t-none", "completed", "zai")
	f.receipt("r-none", member2, 0.75, 1, 0)
	return f
}

type projectBucket struct {
	runs  int64
	usd   float64
	calls int64
}

// TestCostPerProjectAccountsForAllMoney — R7 under OQ-(b) B1. The invariant is
// asserted as a PROPERTY over the whole seeded world (every attribution shape,
// every pricing status, both owners), not as one example: the project altitude
// must account for exactly the money the run altitude does.
func TestCostPerProjectAccountsForAllMoney(t *testing.T) {
	f := moneyWorld(t)

	// The independent aggregation: attribution read one task at a time out of
	// task_project, money read one run at a time out of cost_per_run, and the
	// buckets built in Go. Nothing here re-uses cost_per_project's GROUP BY, so
	// a fan-out in the view shows up as a disagreement rather than as a shared
	// mistake.
	attribution := map[string]string{}
	rows, err := f.db.QueryContext(f.ctx, `SELECT task_id, project_id FROM task_project`)
	if err != nil {
		t.Fatalf("read task_project: %v", err)
	}
	seenTask := map[string]bool{}
	for rows.Next() {
		var task, project string
		if err := rows.Scan(&task, &project); err != nil {
			t.Fatal(err)
		}
		if seenTask[task] {
			t.Errorf("task %q has more than one task_project row — a fan-out double-counts money (R7)", task)
		}
		seenTask[task] = true
		attribution[task] = project
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	want := map[string]*projectBucket{}
	var wantRuns int64
	var wantUSD float64
	rows, err = f.db.QueryContext(f.ctx, `SELECT run_id, user_id, task_id, priced_usd, total_calls FROM cost_per_run`)
	if err != nil {
		t.Fatalf("read cost_per_run: %v", err)
	}
	for rows.Next() {
		var runID, owner, task string
		var usd *float64
		var calls int64
		if err := rows.Scan(&runID, &owner, &task, &usd, &calls); err != nil {
			t.Fatal(err)
		}
		project, ok := attribution[task]
		if !ok {
			project = "(no project)"
		}
		key := project + "\x00" + owner
		if want[key] == nil {
			want[key] = &projectBucket{}
		}
		want[key].runs++
		want[key].calls += calls
		wantRuns++
		if usd != nil {
			want[key].usd += *usd
			wantUSD += *usd
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if wantRuns == 0 {
		t.Fatal("the seeded world produced no cost_per_run rows — every assertion below would be vacuous")
	}

	got := map[string]*projectBucket{}
	var gotRuns int64
	var gotUSD float64
	rows, err = f.db.QueryContext(f.ctx, `SELECT project_id, user_id, runs, priced_usd, total_calls FROM cost_per_project`)
	if err != nil {
		t.Fatalf("read cost_per_project: %v", err)
	}
	for rows.Next() {
		var project, owner string
		var runs, calls int64
		var usd *float64
		if err := rows.Scan(&project, &owner, &runs, &usd, &calls); err != nil {
			t.Fatal(err)
		}
		b := &projectBucket{runs: runs, calls: calls}
		if usd != nil {
			b.usd = *usd
		}
		got[project+"\x00"+owner] = b
		gotRuns += runs
		gotUSD += b.usd
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// Sum preservation, both directions (R7).
	if gotRuns != wantRuns {
		t.Errorf("cost_per_project accounts for %d runs, cost_per_run has %d — spend went missing or double-counted (R7)", gotRuns, wantRuns)
	}
	if math.Abs(gotUSD-wantUSD) > 1e-9 {
		t.Errorf("cost_per_project sums to %.6f, cost_per_run to %.6f (R7)", gotUSD, wantUSD)
	}
	if len(got) != len(want) {
		t.Errorf("bucket count %d, want %d: got %v want %v", len(got), len(want), keysOf(got), keysOf(want))
	}
	for key, w := range want {
		g, ok := got[key]
		if !ok {
			t.Errorf("bucket %q is missing from cost_per_project", strings.ReplaceAll(key, "\x00", "/"))
			continue
		}
		if g.runs != w.runs || g.calls != w.calls || math.Abs(g.usd-w.usd) > 1e-9 {
			t.Errorf("bucket %q = %d runs/%d calls/%.4f usd, want %d/%d/%.4f",
				strings.ReplaceAll(key, "\x00", "/"), g.runs, g.calls, g.usd, w.runs, w.calls, w.usd)
		}
	}

	// B1, named: the pinned-unapproved task's spend is under its project, not
	// in the bucket. Both of its runs are there, including the unpriced one.
	pinned := got["proj-a\x00"+member1]
	if pinned == nil {
		t.Fatalf("no proj-a bucket for %s — a pinned task's money must follow its attribution (OQ-(b) B1)", member1)
	}
	if pinned.runs != 3 {
		t.Errorf("proj-a carries %d runs, want 3 (two pinned-unapproved plus the approved one)", pinned.runs)
	}
	if math.Abs(pinned.usd-4.50) > 1e-9 {
		t.Errorf("proj-a priced_usd = %.4f, want 4.5000", pinned.usd)
	}
	if b := got["(no project)\x00"+member1]; b != nil {
		t.Errorf("%s has a '(no project)' bucket of %d runs — every one of their tasks is attributed", member1, b.runs)
	}
	// The honest bucket still accounts for genuinely unattributed work.
	if b := got["(no project)\x00"+member2]; b == nil || b.runs != 2 {
		t.Errorf("the '(no project)' bucket for %s = %v, want 2 runs (one priced, one receipt-less)", member2, b)
	}

	// R7 — the linkage note must stay TRUE for what ships: it now names BOTH
	// edges, because the view now reads both.
	var note string
	if err := f.db.QueryRowContext(f.ctx, `SELECT DISTINCT linkage FROM cost_per_project LIMIT 1`).Scan(&note); err != nil {
		t.Fatalf("read linkage note: %v", err)
	}
	for _, must := range []string{"artifact_claims", "intake.state"} {
		if !strings.Contains(note, must) {
			t.Errorf("the linkage note %q does not name %q — it must describe the edge the view actually reads (R7)", note, must)
		}
	}
}

func keysOf(m map[string]*projectBucket) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, strings.ReplaceAll(k, "\x00", "/"))
	}
	return out
}

// planLines returns the EXPLAIN QUERY PLAN detail lines for a query, in order.
func planLines(t *testing.T, f *fixture, q string, args ...any) []string {
	t.Helper()
	rows, err := f.db.QueryContext(f.ctx, "EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatalf("EXPLAIN %q: %v", q, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.TrimSpace(detail))
	}
	return out
}

// TestPinProbeSeeks — R14. The latest-intake.state-per-task probe is the one
// new read this packet adds to a surface the operator hits on every board load,
// so it is EXPLAIN-pinned by FULL index name (the §38 D3 precedent: substring
// pinning is how two wrong attributions survived).
//
// Both production forms are pinned rather than averaged: taskLineage's point
// read and handleTaskList's LEFT JOIN. A scan of run_events fails.
func TestPinProbeSeeks(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	// Enough rows that a scan is a real alternative for the planner — on a
	// handful of rows SQLite reasonably chooses one whatever indexes exist.
	for i := 0; i < 80; i++ {
		task := fmt.Sprintf("t%03d", i)
		runID := fmt.Sprintf("r%03d", i)
		f.task(task, member1, "task", "doing")
		f.run(runID, member1, task, "completed", "anthropic")
		pinState(f, runID, member1, fmt.Sprintf("proj-%d", i%5))
	}

	forms := []struct {
		name string
		sql  string
		args []any
	}{
		{
			"taskLineage point read",
			`SELECT project_id, project_choices FROM task_project WHERE task_id = ?`,
			[]any{"t042"},
		},
		{
			"handleTaskList left join",
			`SELECT t.task_id, COALESCE(tp.project_id, ?) FROM tasks t
			   LEFT JOIN task_project tp ON tp.task_id = t.task_id
			  ORDER BY t.created_ts DESC, t.task_id DESC LIMIT ?`,
			[]any{"(no project)", 200},
		},
	}
	// The two legs of the pin probe, by FULL index name.
	wantLines := []string{
		"SEARCH r USING INDEX runs_task_idx (task_id=?)",
		"SEARCH e USING INDEX run_events_run_type_idx (run_id=? AND type=?)",
	}
	for _, form := range forms {
		lines := planLines(t, f, form.sql, form.args...)
		joined := strings.Join(lines, "\n")
		for _, want := range wantLines {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: the plan does not carry %q (R14):\n%s", form.name, want, joined)
			}
		}
		for _, line := range lines {
			if line == "SCAN e" || strings.HasPrefix(line, "SCAN e ") {
				t.Errorf("%s: the pin probe SCANS run_events (R14):\n%s", form.name, joined)
			}
			if line == "SCAN r" || strings.HasPrefix(line, "SCAN r ") {
				t.Errorf("%s: the task→runs hop SCANS runs — that is what runs_task_idx exists for (R14):\n%s", form.name, joined)
			}
		}
	}
}

// frozenMigrations pins every migration this packet must not touch to its
// content hash. CONVENTIONS §6: a migration file, once committed, is IMMUTABLE
// — schema change is a new numbered file, never an edit. A hash is what makes
// that rule checkable rather than merely stated (R8).
var frozenMigrations = []struct{ name, sha string }{
	{"0001_core_schema.sql", "3a8d8e6a9a3e88d2985e8a7d8a0db4a4dc972a12d4de36d9c3ecd19366d5f1b0"},
	{"0002_run_recovery_lineage.sql", "a04b2342d9bb3839722cc19f57f48069e0da2a28022f23c844d27dd2fa9c742d"},
	{"0003_auth_stack.sql", "764faa5025f0c24b7e79482ec460d62797e1c234e6169900e2fa33fc40dbb0bb"},
	{"0004_knowledge.sql", "e102f9cb1db1855993dcf7369c335c8b9bd7c4194a461bbc3601b28237f3a0a5"},
	{"0005_workers.sql", "f258c21ea8df4fbb9b93d9fe59a77aa177d330309270f5480c00c515b7efbcc9"},
	{"0006_worker_search.sql", "eb5c167e41d50cfc53938ade30c5aff8ddf6ff51cb8f956e948820643cf65010"},
	{"0007_deliverables.sql", "0adaf510bdff25a303dc92e270535a567dcac3ff9e114aa8a07b022a7f859992"},
	{"0008_repo_registry.sql", "dfc773b6319794082293bce1f80ad28fee4e6da22605c7cca65958b5ad2fe375"},
	{"0009_snapshot_ledger_followups.sql", "f0e597e642018b84d9f38eb72293ecdc3eca9aa35ed0af27ef5a345769b49da4"},
	{"0010_calibration_battery.sql", "0f8033ca6d5f8294cfcb4104b702683ecadcaf4d703936e8e1ce6a8be17b7726"},
	{"0011_conformance_registry.sql", "1d76dfa7475cb3f399cfa55cebffe4ea572e196a24b98f6b04754128cf0eb72d"},
	{"0012_eval_floors.sql", "5296d010683091114e101cd88335e2bbedbba7b7509bbc42a492bafe36bf81fc"},
	{"0013_watchlist.sql", "86c5dbd5553e3bca0a23ab676aab096c1e3f8def259114c56380406f46f9c081"},
	{"0014_benchmark.sql", "6461167b084aef001089ddf169ec1f4ffc855b16d23a825470cf3c67eb0291ca"},
	{"0015_retention_history.sql", "dae182c621a0ab5aa575cf4b70b07ec36cc13e1890398f5c6d0e1e00137e2243"},
	{"0016_queryable_history.sql", "d59ed383fe161f3309d3e3f7310131cfd1f532e18d6f2e567a2bf8ccf2daf568"},
	{"0017_budgets_pause_hint.sql", "c48af78f95abd237e23044b8d238ee47b61ffae2875d25e2da1de939b7bd1c4a"},
	{"0018_benchmark_direct_capture.sql", "578b6f486b7e17cdc983deee2bff2c41b3de89817298aa142156c6fa49336c0a"},
	{"0019_price_rows.sql", "37a8416c032d1e6b99212f86ff551aa68f5d6e1ddfb45f29c60f28c75e72b8b2"},
	{"0020_chat.sql", "31125792db9a3225de1e3961449aed2b604ffe2dc95751d69129847ea294f4a8"},
	{"0021_push_subscriptions.sql", "571c0860b875061f8d2f53ddc57ecaf52bc44b7697384b1fca7c1e33dce845c7"},
}

// TestMigrationLedgerAdvances — R8. The fix is a VIEW change, so it is a new
// numbered migration with its own version bump, and every earlier file is
// byte-identical to what was committed.
func TestMigrationLedgerAdvances(t *testing.T) {
	dir := filepath.Join("..", "storage", "migrations")
	for _, m := range frozenMigrations {
		b, err := os.ReadFile(filepath.Join(dir, m.name))
		if err != nil {
			t.Errorf("read %s: %v", m.name, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != m.sha {
			t.Errorf("%s changed: sha256 %s, want %s — a committed migration is IMMUTABLE (R8, CONVENTIONS §6)", m.name, got, m.sha)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, pinMigration))
	if err != nil {
		t.Fatalf("read %s: %v — the view change is migration 0022 (R8)", pinMigration, err)
	}
	text := string(raw)
	if !strings.Contains(text, "PRAGMA user_version = 22") {
		t.Errorf("%s does not bump user_version to 22", pinMigration)
	}
	for _, want := range []string{
		"DROP VIEW cost_per_project", "DROP VIEW task_project",
		"CREATE VIEW task_project", "CREATE VIEW cost_per_project",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not %q — the re-create is DROP + CREATE in one transaction (R8)", pinMigration, want)
		}
	}

	// The money prohibition covers the new file too. The standing scan is
	// TestNoMoneyByGeneration; this limb is the acceptance statement of it.
	sqlOnly := stripSQLComments(text)
	for _, tf := range []string{"prompt_tokens", "output_tokens", "input_tokens", "cache_read_tokens"} {
		if strings.Contains(sqlOnly, tf) {
			t.Errorf("%s reads the token field %q — money is READ, never computed (S14.10 ¶1)", pinMigration, tf)
		}
	}
	mult := regexp.MustCompile(`[a-z_0-9)]\s*\*\s*[a-z_0-9(]`)
	for _, line := range strings.Split(sqlOnly, "\n") {
		l := strings.ToLower(strings.TrimSpace(line))
		if !strings.Contains(l, "usd") {
			continue
		}
		if mult.MatchString(l) {
			t.Errorf("%s multiplies in a money expression:\n  %s", pinMigration, line)
		}
	}

	// The type literal the view names is pinned to the registry, the way 0016's
	// limit-event literals are: a retype must fail loudly, not silently empty
	// the pin edge.
	if !strings.Contains(sqlOnly, "'intake.state'") {
		t.Errorf("%s does not name the 'intake.state' type literal the pin edge reads", pinMigration)
	}
	if fam, known := eventlog.Classify("intake.state"); !known || fam != eventlog.FamilyRunLifecycle {
		t.Errorf("intake.state does not classify to the run-lifecycle family (known=%v fam=%q)", known, fam)
	}

	// The applied database: the version moved and both re-created views parse
	// and are selectable.
	f := newFixture(t)
	var version int
	if err := f.db.QueryRowContext(f.ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 24 {
		t.Errorf("user_version = %d, want 24 (migrations through 0024 applied contiguously)", version)
	}
	for _, view := range []string{"task_project", "cost_per_project"} {
		var n int
		if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM `+view).Scan(&n); err != nil {
			t.Errorf("re-created view %s is not selectable: %v", view, err)
		}
	}
}

// stripSQLComments drops whole-line SQL comments so a scan for a forbidden
// CONSTRUCT judges the SQL and not the prose explaining why it is absent.
func stripSQLComments(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
