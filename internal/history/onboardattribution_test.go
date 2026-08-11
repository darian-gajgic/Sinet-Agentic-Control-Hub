package history_test

// onboardattribution_test.go — P3-RW-7 acceptance on the Layer-0 side: what
// migration 0023 does to the task→project edge for an ONBOARDING task, the
// money that follows the edge, and the migration ceremony around it.
//
// THE GAP. `stage.OnboardTaskID(p)` = `onboard-<p>`, and the onboarding task
// has neither of 0022's two edges: it writes no `artifact_claims` (it never
// reaches plan approval) and appends no `intake.state` event (it is not an
// intake-pipeline task). So the one task whose id NAMES its project read
// '(no project)' everywhere the single-place linkage is read — the board, the
// task-detail lineage, Layer 0/2 and `cost_per_project`.
//
// THE FIX (coordinator disposition OQ1 = option B arm (i), STATE 2026-08-11):
// 0023 re-creates `task_project` with a THIRD COALESCE arm resolved by JOINING
// THE REGISTRY — `repo_registry.project_id` is a durable, owner-attributed row
// that EXISTS BEFORE the task does (project.Register runs inside OnboardStart,
// before StartOnboarding's insert tx) and that production never deletes. An
// `onboard-*` id with no registry row therefore resolves to an honest NULL and
// keeps the '(no project)' bucket, rather than to a name with nothing behind it.
//
// Committed RED per P3/briefs/P3-RW-7.md §8: T4 (money follows attribution) and
// T7 (ceremony) fail until 0023 lands. T5's honest-NULL limb is born GREEN and
// is a regression pin on the bucket, not a claim about new behavior.

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
)

// onboardMigration is the file this packet adds. Named rather than globbed, for
// pinMigration's reason: a differently-named migration is a failure, not a
// silent pass.
const onboardMigration = "0023_onboarding_attribution.sql"

// pinMigrationSHA freezes 0022. TestMigrationLedgerAdvances' own table pins
// 0001–0021 and is NOT edited by this packet (0022's text pin at its :382 is a
// statement about 0022 and stays true); 0022 became immutable the moment it was
// committed, so the packet that adds the NEXT file is where its hash belongs.
const pinMigrationSHA = "c5d2c00696abd0fa4a3268021b0720f6786e050c31227c2445ab299ef8795214"

// registryEntry seeds one repo_registry row — the durable relational anchor the
// onboarding arm reads. Owner-attributed, `pending` before approval and `active`
// after, never deleted (0008's repo_registry_no_delete trigger).
func (f *fixture) registryEntry(projectID, owner, name, state string) {
	f.t.Helper()
	f.exec(`INSERT INTO repo_registry
	          (project_id, user_id, name, store_path, default_branch, state, capture_version, created_ts, updated_ts)
	        VALUES (?, ?, ?, ?, 'main', ?, 1, ?, ?)`,
		projectID, owner, name, "/store/"+projectID, state, f.ts(0), f.ts(0))
}

// onboardWorld is the seeded world every assertion below ranges over. It is
// deliberately mixed so that a sum invariant proved over it is not a statement
// about one shape: an onboarding task mid-ceremony (registry `pending`), one
// past approval (`active`), a task WEARING the onboarding id shape with no
// registry row behind it, a claims-attributed task, a pin-attributed task, and
// a task with no edge at all — across two owners.
func onboardWorld(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.user(opUser, "operator")
	f.user(member1, "member")
	f.user(member2, "member")

	// Mid-ceremony: the entry is registered and pending, the owner has not
	// approved yet. This is precisely the window the board showed as blank.
	f.registryEntry("proj-onb", member1, "onb repo", "pending")
	f.task("onboard-proj-onb", member1, "Onboard onb repo", "intake")
	f.run("onboard-proj-onb.onboard", member1, "onboard-proj-onb", "parked", "interactive")
	f.receipt("onboard-proj-onb.onboard", member1, 0.50, 2, 0)

	// Past approval: the entry is active, the onboarding task is done.
	f.registryEntry("proj-live", member1, "live repo", "active")
	f.task("onboard-proj-live", member1, "Onboard live repo", "done")
	f.run("onboard-proj-live.onboard", member1, "onboard-proj-live", "completed", "interactive")
	f.receipt("onboard-proj-live.onboard", member1, 1.25, 3, 0)

	// The id shape with NOTHING behind it: arm (i) is a JOIN, so this resolves
	// to no project at all rather than to the name `substr` would have carved
	// out of the id.
	f.task("onboard-ghost", member2, "Not an onboarding of anything", "intake")
	f.run("r-ghost", member2, "onboard-ghost", "completed", "zai")
	f.receipt("r-ghost", member2, 0.10, 1, 0)

	// The two edges 0022 already reads must keep winning, in their order.
	f.task("t-claim", member1, "Approved, claimed", "doing")
	f.run("r-claim", member1, "t-claim", "completed", "anthropic")
	f.receipt("r-claim", member1, 2.25, 3, 0)
	f.exec(`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	        VALUES (?,?,?,?,?,?,?)`, "t-claim", "proj-b", member1, `["**"]`, "W", "active", f.ts(0))

	f.task("t-pin", member1, "Pinned, unapproved", "doing")
	f.run("r-pin", member1, "t-pin", "completed", "anthropic")
	pinState(f, "r-pin", member1, "proj-a")
	f.receipt("r-pin", member1, 3.00, 5, 0)

	// No edge of any kind: the honest bucket must still hold it.
	f.task("t-none", member2, "Unattributed", "doing")
	f.run("r-none", member2, "t-none", "completed", "zai")
	f.receipt("r-none", member2, 0.75, 1, 0)
	return f
}

// readAttribution collapses task_project into a Go map and fails on a fan-out —
// one row per task is what keeps money from double-counting (0016's invariant).
func readAttribution(t *testing.T, f *fixture) (map[string]string, map[string]int64) {
	t.Helper()
	project := map[string]string{}
	choices := map[string]int64{}
	rows, err := f.db.QueryContext(f.ctx, `SELECT task_id, project_id, project_choices FROM task_project`)
	if err != nil {
		t.Fatalf("read task_project: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var task, proj string
		var n int64
		if err := rows.Scan(&task, &proj, &n); err != nil {
			t.Fatal(err)
		}
		if _, dup := project[task]; dup {
			t.Errorf("task %q has more than one task_project row — a fan-out double-counts money", task)
		}
		project[task], choices[task] = proj, n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return project, choices
}

// TestOnboardingTaskResolvesItsProject — brief T3/T5 at the view altitude, and
// the R3 single place: the edge answers `<p>` for `onboard-<p>` when a registry
// row exists, and answers NOTHING when one does not.
func TestOnboardingTaskResolvesItsProject(t *testing.T) {
	f := onboardWorld(t)
	project, choices := readAttribution(t, f)

	for _, c := range []struct{ task, want string }{
		{"onboard-proj-onb", "proj-onb"},
		{"onboard-proj-live", "proj-live"},
	} {
		if got := project[c.task]; got != c.want {
			t.Errorf("task_project[%s] = %q, want %q — the onboarding task's id names its project and the registry row predates the task (R3)",
				c.task, got, c.want)
		}
		if n := choices[c.task]; n != 1 {
			t.Errorf("task_project[%s].project_choices = %d, want 1 — a registry id resolves exactly one project", c.task, n)
		}
	}

	// T5: the id shape alone attributes NOTHING. Arm (i) is a join, not
	// id-surgery: a name with no relational anchor is not attribution.
	if got, ok := project["onboard-ghost"]; ok {
		t.Errorf("task_project[onboard-ghost] = %q — an onboard-shaped id with no repo_registry row must resolve to an honest NULL (R7, arm (i))", got)
	}
	// R7: everything that is not an onboarding task is untouched.
	if got := project["t-claim"]; got != "proj-b" {
		t.Errorf("task_project[t-claim] = %q, want proj-b — the claims arm still wins first", got)
	}
	if got := project["t-pin"]; got != "proj-a" {
		t.Errorf("task_project[t-pin] = %q, want proj-a — the pin arm still resolves before the onboard arm", got)
	}
	if got, ok := project["t-none"]; ok {
		t.Errorf("task_project[t-none] = %q — a task with no edge belongs in the '(no project)' bucket", got)
	}
}

// TestOnboardingSpendFollowsItsProject — brief T4 (R4). The onboarding
// ceremony's own spend lands under the project it onboards, with the registry
// name and state joined, and 0016's sum-preservation invariant survives the
// third arm: the project altitude accounts for exactly the money the run
// altitude does.
func TestOnboardingSpendFollowsItsProject(t *testing.T) {
	f := onboardWorld(t)
	attribution, _ := readAttribution(t, f)

	// The independent aggregation, built in Go out of task_project and
	// cost_per_run, so a fan-out in cost_per_project shows up as a
	// disagreement rather than as a shared mistake.
	want := map[string]*projectBucket{}
	var wantRuns int64
	var wantUSD float64
	rows, err := f.db.QueryContext(f.ctx, `SELECT user_id, task_id, priced_usd, total_calls FROM cost_per_run`)
	if err != nil {
		t.Fatalf("read cost_per_run: %v", err)
	}
	for rows.Next() {
		var owner, task string
		var usd *float64
		var calls int64
		if err := rows.Scan(&owner, &task, &usd, &calls); err != nil {
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
	names := map[string]string{}
	states := map[string]string{}
	var gotRuns int64
	var gotUSD float64
	rows, err = f.db.QueryContext(f.ctx,
		`SELECT project_id, project_name, project_state, user_id, runs, priced_usd, total_calls FROM cost_per_project`)
	if err != nil {
		t.Fatalf("read cost_per_project: %v", err)
	}
	for rows.Next() {
		var project, name, state, owner string
		var runs, calls int64
		var usd *float64
		if err := rows.Scan(&project, &name, &state, &owner, &runs, &usd, &calls); err != nil {
			t.Fatal(err)
		}
		b := &projectBucket{runs: runs, calls: calls}
		if usd != nil {
			b.usd = *usd
		}
		got[project+"\x00"+owner] = b
		names[project], states[project] = name, state
		gotRuns += runs
		gotUSD += b.usd
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// Sum preservation, both directions (R4).
	if gotRuns != wantRuns {
		t.Errorf("cost_per_project accounts for %d runs, cost_per_run has %d — spend went missing or double-counted (R4)", gotRuns, wantRuns)
	}
	if math.Abs(gotUSD-wantUSD) > 1e-9 {
		t.Errorf("cost_per_project sums to %.6f, cost_per_run to %.6f (R4)", gotUSD, wantUSD)
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

	// Named: the onboarding ceremony's spend is under the project it onboards,
	// with the registry facts joined (0022's cost_per_project shape).
	onb := got["proj-onb\x00"+member1]
	if onb == nil {
		t.Fatalf("no proj-onb bucket for %s — the onboarding ceremony's spend must follow its attribution (R4)", member1)
	}
	if onb.runs != 1 || math.Abs(onb.usd-0.50) > 1e-9 {
		t.Errorf("proj-onb bucket = %d runs/%.4f usd, want 1/0.5000", onb.runs, onb.usd)
	}
	if names["proj-onb"] != "onb repo" || states["proj-onb"] != "pending" {
		t.Errorf("proj-onb joins name=%q state=%q, want %q/%q — the registry row is the anchor",
			names["proj-onb"], states["proj-onb"], "onb repo", "pending")
	}
	if names["proj-live"] != "live repo" || states["proj-live"] != "active" {
		t.Errorf("proj-live joins name=%q state=%q, want %q/%q", names["proj-live"], states["proj-live"], "live repo", "active")
	}

	// R7: the honest bucket still holds exactly the genuinely unattributed
	// work — bob's plain task, his receipt-bearing ghost-id task, and nothing
	// of alice's.
	if b := got["(no project)\x00"+member1]; b != nil {
		t.Errorf("%s has a '(no project)' bucket of %d runs — every one of their tasks is attributed", member1, b.runs)
	}
	if b := got["(no project)\x00"+member2]; b == nil || b.runs != 2 {
		t.Errorf("the '(no project)' bucket for %s = %v, want 2 runs (the unattributed task and the onboard-shaped id with no registry row)", member2, b)
	}

	// R6: the linkage note must stay TRUE for what ships — it now names all
	// three edges, because the view now reads all three.
	var note string
	if err := f.db.QueryRowContext(f.ctx, `SELECT DISTINCT linkage FROM cost_per_project LIMIT 1`).Scan(&note); err != nil {
		t.Fatalf("read linkage note: %v", err)
	}
	for _, must := range []string{"artifact_claims", "intake.state", "repo_registry"} {
		if !strings.Contains(note, must) {
			t.Errorf("the linkage note %q does not name %q — it must describe the edge the view actually reads (R6)", note, must)
		}
	}
}

// TestOnboardArmDoesNotDisturbThePinProbePlan — the §38 EXPLAIN-evidence
// discipline applied to the added arm (0022's measured-plan precedent, R14
// there). TestPinProbeSeeks pins the pin probe's two legs on a world with NO
// registry rows; the added arm is only interesting when repo_registry has rows
// to correlate against, so the same pin is re-asserted HERE under that world.
//
// The arm itself is a correlated subquery over repo_registry — the registry is
// tiny (one row per project the operator ever onboarded) and the coordinator's
// disposition accepts its cost; what must NOT happen is the added arm pushing
// the runs/run_events legs off their indexes.
func TestOnboardArmDoesNotDisturbThePinProbePlan(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	for i := 0; i < 80; i++ {
		task := fmt.Sprintf("t%03d", i)
		runID := fmt.Sprintf("r%03d", i)
		f.task(task, member1, "task", "doing")
		f.run(runID, member1, task, "completed", "anthropic")
		pinState(f, runID, member1, "proj-x")
	}
	// Registry rows to correlate against, including one whose onboarding task
	// is on the board.
	for i := 0; i < 20; i++ {
		f.registryEntry(fmt.Sprintf("proj-%03d", i), member1, "repo", "active")
	}
	f.task("onboard-proj-003", member1, "Onboard", "intake")

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
	wantLines := []string{
		"SEARCH r USING INDEX runs_task_idx (task_id=?)",
		"SEARCH e USING INDEX run_events_run_type_idx (run_id=? AND type=?)",
	}
	for _, form := range forms {
		lines := planLines(t, f, form.sql, form.args...)
		joined := strings.Join(lines, "\n")
		for _, want := range wantLines {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: the added onboard arm cost the pin probe its plan — %q is gone:\n%s", form.name, want, joined)
			}
		}
		for _, line := range lines {
			if line == "SCAN e" || strings.HasPrefix(line, "SCAN e ") {
				t.Errorf("%s: the pin probe SCANS run_events:\n%s", form.name, joined)
			}
			if line == "SCAN r" || strings.HasPrefix(line, "SCAN r ") {
				t.Errorf("%s: the task→runs hop SCANS runs:\n%s", form.name, joined)
			}
		}
		// Non-vacuity: the plan must actually reach repo_registry, or this
		// test is pinning a view that never grew the arm.
		if !strings.Contains(joined, "repo_registry") {
			t.Errorf("%s: the plan never touches repo_registry — the onboard arm is not in the shipping view:\n%s", form.name, joined)
		}
	}
}

// TestOnboardMigrationCeremony — brief T7 (R5). The fix is a VIEW change, so it
// is a NEW numbered migration with its own version bump; 0022 is byte-identical
// to what was committed; and the no-money-by-generation scan covers the new
// file, because a view RE-created in a later file is exactly as capable of
// computing money as the file that first created it.
func TestOnboardMigrationCeremony(t *testing.T) {
	dir := filepath.Join("..", "storage", "migrations")

	b, err := os.ReadFile(filepath.Join(dir, pinMigration))
	if err != nil {
		t.Fatalf("read %s: %v", pinMigration, err)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != pinMigrationSHA {
		t.Errorf("%s changed: sha256 %s, want %s — a committed migration is IMMUTABLE (CONVENTIONS §6)", pinMigration, got, pinMigrationSHA)
	}

	raw, err := os.ReadFile(filepath.Join(dir, onboardMigration))
	if err != nil {
		t.Fatalf("read %s: %v — the view change is migration 0023 (R5)", onboardMigration, err)
	}
	text := string(raw)
	if !strings.Contains(text, "PRAGMA user_version = 23") {
		t.Errorf("%s does not bump user_version to 23", onboardMigration)
	}
	for _, want := range []string{
		"DROP VIEW cost_per_project", "DROP VIEW task_project",
		"CREATE VIEW task_project", "CREATE VIEW cost_per_project",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not %q — the re-create is DROP + CREATE in one transaction (R5)", onboardMigration, want)
		}
	}

	sqlOnly := stripSQLComments(text)
	// The money prohibition covers the new file too (S14.10 ¶1).
	for _, tf := range []string{"prompt_tokens", "output_tokens", "input_tokens", "cache_read_tokens"} {
		if strings.Contains(sqlOnly, tf) {
			t.Errorf("%s reads the token field %q — money is READ, never computed (S14.10 ¶1)", onboardMigration, tf)
		}
	}
	mult := regexp.MustCompile(`[a-z_0-9)]\s*\*\s*[a-z_0-9(]`)
	for _, line := range strings.Split(sqlOnly, "\n") {
		l := strings.ToLower(strings.TrimSpace(line))
		if !strings.Contains(l, "usd") {
			continue
		}
		if mult.MatchString(l) {
			t.Errorf("%s multiplies in a money expression:\n  %s", onboardMigration, line)
		}
	}

	// The relational anchor the arm reads is named in the shipping SQL — the
	// 'intake.state' literal pin one arm over. Arm (i) is a JOIN; a file that
	// carved the project out of the id instead would not name the table.
	if !strings.Contains(sqlOnly, "repo_registry") {
		t.Errorf("%s never names repo_registry — the onboarding arm is a registry JOIN, not id surgery (OQ1 arm (i))", onboardMigration)
	}

	// The money-scan list must have grown, or the standing
	// TestNoMoneyByGeneration proves nothing about the new file.
	var covered bool
	for _, m := range moneyViewMigrations {
		if m == onboardMigration {
			covered = true
		}
	}
	if !covered {
		t.Errorf("moneyViewMigrations does not list %s — it re-creates cost_per_project, so the no-money scan must cover it (R5)", onboardMigration)
	}

	// The applied database: the version moved contiguously and both re-created
	// views parse and are selectable.
	f := newFixture(t)
	var version int
	if err := f.db.QueryRowContext(f.ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 23 {
		t.Errorf("user_version = %d, want 23 (migrations through 0023 applied contiguously)", version)
	}
	for _, view := range []string{"task_project", "cost_per_project"} {
		var n int
		if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM `+view).Scan(&n); err != nil {
			t.Errorf("re-created view %s is not selectable: %v", view, err)
		}
	}
}
