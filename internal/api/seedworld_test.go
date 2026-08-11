package api_test

// seedworld_test.go — the dev-only seeded demo world (P3-UI-7 R17/R18/R19).
//
// WHY IT EXISTS. The B6 click-through has always run against a brand-new,
// EMPTY database, so the surfaces that need rows to exist at all — the task
// detail, the review surface with its diff, its anchored comments and its
// try-it frames — have never been walkable. The operator has therefore never
// seen most of what B6 built. This seeds the SAME world the golden fixtures are
// produced from into the click-through's own throwaway state dir, so every
// surface is walkable.
//
// WHY IT IS A `_test.go` FILE, AND WHY THAT IS THE WALL (OQ4 as ratified).
// Files ending in `_test.go` are never compiled into `go build ./cmd/sinet`, so
// the shipped binary cannot contain this code AT ALL — not dormant behind a
// posture check, not behind a build flag, not at all. That is strictly stronger
// than the row's own "never in the shipped binary's default path", and it keeps
// the B0-3 rule intact: posture is an environment fact, never a build flag, and
// the compile boundary is not a posture mechanism. It also puts the seed beside
// the producers it reuses, so there is ONE world rather than a second copy of
// it (this package's own §40-C lesson).
//
// It writes outside `t.TempDir()`, which §2 otherwise forbids, and the env gate
// is what makes that a deliberate act rather than something a test run does to
// you — exactly the `SINET_WRITE_API_FIXTURES` precedent one file over. Without
// the variable this skips; with it, it seeds the directory the variable names.
//
//	SINET_SEED_DEMO_WORLD=/path/to/throwaway go test ./internal/api -run TestSeedDemoWorld
//
// `P3/gates/B6-clickthrough.sh` is its only caller, after that script's own
// production-refusal preflight.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// seedEnv names the throwaway state directory to seed. Its absence is what
// makes every ordinary run — every run CI can make — a no-op.
const seedEnv = "SINET_SEED_DEMO_WORLD"

// TestSeedDemoWorld drives the REAL producers into a real state directory.
func TestSeedDemoWorld(t *testing.T) {
	dir := os.Getenv(seedEnv)
	if dir == "" {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10, tier-R): %s is unset — the demo seed is a deliberate act, never something a test run does", seedEnv)
	}
	// Refuse production outright. The script's own preflight already refuses it,
	// and this is the second lock: a seed that could be pointed at the live
	// state directory by a typo is one typo away from writing fixture users into
	// the household's own database.
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve %s: %v", dir, err)
	}
	for _, forbidden := range []string{"/var/lib/sinet", "/etc/sinet"} {
		if abs == forbidden || strings.HasPrefix(abs, forbidden+"/") {
			t.Fatalf("%s points at production (%s). Refusing.", seedEnv, abs)
		}
	}
	// The three the click-through's preflight unsets, so the refusal and the
	// preflight name the SAME set — an asymmetry here would mean a posture the
	// script clears but the seed would still accept (drain r1 D5).
	for _, v := range []string{"STATE_DIRECTORY", "CONFIGURATION_DIRECTORY", "NOTIFY_SOCKET"} {
		if os.Getenv(v) != "" {
			t.Fatalf("%s is set, which is production posture. Refusing to seed.", v)
		}
	}

	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(abs, storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("open %s: %v", abs, err)
	}
	defer db.Close()
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	b := &backend{db: db, log: log, store: auth.New(db, log), reg: reg}

	demoWorldOn(t, b, abs)

	// What the walk needs to know, printed rather than documented elsewhere: a
	// runbook that drifts from the seed is worse than no runbook.
	t.Logf("seeded demo world at %s", abs)
	t.Logf("people (all with PIN %s): op (operator) · alice (member) · bob (member)", fixturePIN)
	t.Logf("the dev fallback browses without signing in; use the header's Sign in affordance to exercise the real login")
	t.Logf("meters and budgets render the REAL stores' honest state — the fixture world's metering and price DOUBLES")
	t.Logf("cannot reach a throwaway binary, so where the committed bodies show a served figure this world shows the")
	t.Logf("real store's answer, absences and all. That difference is the honest one and is not a defect to report.")
}

// demoWorldOn builds THE OPERATOR'S DEMO WORLD on b: the golden-fixture world,
// plus the deliberate differences that world must not carry in front of a
// person, plus the seed guards that hold both differences in place.
//
// The split from `fixtureWorldOn` is what keeps ONE world with two honest
// endings. The committed goldens (`fixtureWorld`, a `t.TempDir()` database read
// by `go test` and nothing else) never reach this function, so nothing here
// rewrites a committed body; the demo world is the one that gets opened by a
// REAL control plane, with the real recovery ladder and the real scheduler
// running against it, and that is the difference every line below is about.
//
// It is also what lets the guards run in CI: `TestSeedDemoWorld` seeds a real
// directory and is skipped everywhere else, so a check that lived only there
// would be a check the ordinary `go test ./...` never makes.
func demoWorldOn(t *testing.T, b *backend, root string) {
	t.Helper()
	// THE SAME WORLD THE GOLDEN FIXTURES ARE PRODUCED FROM, through the same
	// producers in the same order — users through the real auth store with real
	// PINs, tasks and runs across every state the board buckets, asks and
	// approval cards from the real intake card types, the review surface with
	// its revisions and anchored comments, decisions through the real verbs,
	// workforce rows, memory entries, chat sessions and turns, the push devices,
	// the pause flip and the history index.
	fixtureWorldOn(t, b, root)

	// SEED HYGIENE 1: THE BOARD COLUMN (checkpoint-2 finding C2-6).
	//
	// `fixtureWorldOn` plants `t-archive` at the kanban status `moonshot`, which
	// is not one of the board's declared columns. That is CORRECT for the
	// goldens: it is the fixture that proves the board's forward-tolerant
	// unknown-status column (B6-5 OQ7) gives a producer string the view has
	// never seen its own column rather than vanishing the card. It is WRONG in
	// front of the operator, who is walking a product and reads "moonshot" as a
	// column this platform has, not as a test's proof obligation.
	//
	// So the DEMO WORLD ALONE moves the card on, through the same statement the
	// real producer issues (`Skeleton.setKanban`, internal/stage/skeleton.go),
	// to `intake` — the value the intake pipeline itself writes, and the honest
	// one for this row: a run of `t-archive` still waiting on an unanswered
	// coverage card, and an `intake.state`/drafting event. `done` would have been
	// the louder lie — a finished task with a question still open in the inbox.
	exec(t, b, `UPDATE tasks SET kanban_status = ? WHERE task_id = ?`, "intake", "t-archive")

	// SEED HYGIENE 2: NO RUN THE RECOVERY LADDER WILL WAKE UP (P3-RW-4).
	//
	// The goldens are read by a test binary. The demo world is opened by a REAL
	// control plane (`P3/gates/B6-clickthrough.sh`), which starts the S02.5
	// ladder (one pass at boot, then every ⚙ `recovery.sweep_interval`) and the
	// S10.7 scheduler claim loop (every 500ms). Both of them act on the states a
	// live run wears — and a SEEDED row wears them with no process behind it:
	//
	//   - `running` / `claimed` / `draining` are exactly what the ladder scans
	//     (internal/recovery/recovery.go). A seeded row has no unit, so the pass
	//     reads it `unit_state:"gone"` with a dead lease, crashes it and forks a
	//     successor — every sweep, forever. The forks are named `<id>.g<n>`, and
	//     a fork of a run whose id carries no skeleton role cannot dispatch
	//     (internal/stage/skeleton.go), so the chain cycles claimed→crashed and
	//     permanently occupies the cap-1 (user, lane) scheduler slot. That is
	//     what starved every REAL run in the demo world, fresh-project
	//     onboarding included.
	//   - `queued` plus a `queue` row is the same wound one step earlier: the
	//     claim loop CAS-claims the row, `Dispatch` errors on the roleless id,
	//     and the run is left `claimed` with nothing to settle it — raw material
	//     for the paragraph above.
	//
	// So every seeded run lands in a state the ladder does not scan and the
	// scheduler will not claim, and the surfaces stay populated by CHOOSING the
	// honest inert equivalent rather than by deleting the world:
	// `backedBy` is the ask that makes each row's park HONEST, and it is checked
	// rather than asserted in a comment: `parked` means "suspended on a limit
	// event or open gate/ask" (internal/run), and it is an UNANSWERED ask that
	// the server derives `waiting_on_human` from. A row parked here whose ask
	// had been answered would be a card the operator cannot see beside a run
	// that claims to be waiting for them.
	for _, r := range []struct{ id, state, backedBy string }{
		// The four below keep their card in the inbox and their row on mission
		// control — under "Blocked on a human" rather than "Running", which is
		// the honest reading of a world with no engine attached to anything.
		{"r-ship", "parked", "ask-ship"},
		{"r-claim", "parked", "ask-claim"},
		{"r-triage", "parked", "ask-coverage"},
		{"r-archive", "parked", "ask-sweep"},
		// The one with no ask. `parked` with no limit marker is a DESIGNED state
		// the surface renders as "parked, no horizon given" (r-stall is the
		// fixture that proves it), so the operator's own task keeps a run on
		// screen without a fabricated horizon and without a fabricated finish.
		{"r-ops", "parked", ""},
	} {
		exec(t, b, `UPDATE runs SET state = ? WHERE run_id = ?`, r.state, r.id)
		if r.backedBy == "" {
			continue
		}
		var open int
		if err := b.db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM asks WHERE ask_id = ? AND run_id = ? AND answered_ts IS NULL`,
			r.backedBy, r.id).Scan(&open); err != nil {
			t.Fatalf("read %s's backing ask: %v", r.id, err)
		}
		if open != 1 {
			t.Errorf("the demo world parks %s on %s, but that ask is not open on it (%d rows) — "+
				"a run parked on nothing is a card the operator cannot answer", r.id, r.backedBy, open)
		}
	}
	// ...and the claim queue empties with them. A parked run is not in the claim
	// queue: leaving the rows behind would have the loop's orphan-materialization
	// path (internal/scheduler/claim.go) treating them as work to pick up the
	// moment anything flipped a state back. The cost is stated rather than
	// worked around: with nothing dispatchable in the seed there is no honest
	// `queued` row left, so mission control's Queued bucket and the board's
	// recorded drag order are EMPTY in the demo world — a queued run that no
	// dispatch could serve would be a worse lie than an empty column.
	exec(t, b, `DELETE FROM queue WHERE run_id IN ('r-triage', 'r-archive', 'r-ops')`)

	assertNoUndeclaredKanbanStatus(t, b)
	assertNoRunTheLadderWillWake(t, b)
}

// TestDemoWorldIsLadderInert is the CI-visible half of the seed guard: it
// builds the operator's demo world in a `t.TempDir()` — the same statements
// `TestSeedDemoWorld` runs against a real directory — so both hygiene
// assertions above are made on every ordinary `go test ./...`, with no
// environment variable and nothing written outside the sandbox (§2).
//
// Without it the guards would only run inside the deliberate seeding act, and a
// row that regrew a live state would reach the operator's click-through before
// anything said so.
func TestDemoWorldIsLadderInert(t *testing.T) {
	b := newBackend(t)
	demoWorldOn(t, b, t.TempDir())

	// ...and the walk still has something to walk. Inertness bought by emptying
	// the world would satisfy every assertion in the guard and fail the only
	// thing the seed exists for, so the run list is READ BACK through the real
	// handler: every seeded run is still served, and at least one of them is
	// still waiting on a person — which is the bucket the reshaped rows moved
	// into and the reason the demo did not lose its attention cards.
	var list struct {
		Runs []struct {
			RunID          string `json:"run_id"`
			State          string `json:"state"`
			WaitingOnHuman bool   `json:"waiting_on_human"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "op", "/api/runs")), &list); err != nil {
		t.Fatalf("decode /api/runs: %v", err)
	}
	served := map[string]bool{}
	var blocked int
	for _, r := range list.Runs {
		served[r.RunID] = true
		if r.WaitingOnHuman {
			blocked++
		}
	}
	for _, id := range []string{
		"r-ship", "r-claim", "r-triage", "r-archive", "r-ops",
		"r-audit", "r-brief", "r-notes", "r-stall", "t-chatborn.intake",
	} {
		if !served[id] {
			t.Errorf("/api/runs no longer serves %s — the demo world lost a row the walk needs", id)
		}
	}
	if blocked == 0 {
		t.Error(`/api/runs serves no run waiting on a person: "Blocked on a human" is where the ` +
			`reshaped rows land, so an empty one means the demo lost its attention cards`)
	}
}

// assertNoRunTheLadderWillWake is the P3-RW-4 guard, asserted at seed time
// rather than trusted.
//
// Two rules, both about the demo world meeting a LIVE control plane:
//
//	(1) no run row in a state the recovery ladder scans; and
//	(2) no live `queue` row for a run no dispatch could serve.
//
// The scanned set is READ OUT of the ladder's own source rather than copied
// here. A second maintained copy of that vocabulary is this package's §40-C
// hazard in its purest form: a ladder that starts scanning one more state would
// leave a drifted copy passing while the demo world burned again.
func assertNoRunTheLadderWillWake(t *testing.T, b *backend) {
	t.Helper()
	scanned := ladderScannedStates(t)

	rows, err := b.db.QueryContext(context.Background(),
		`SELECT run_id, state FROM runs ORDER BY run_id`)
	if err != nil {
		t.Fatalf("read the seeded runs: %v", err)
	}
	defer rows.Close()
	var seen, routable, roleless int
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			t.Fatalf("scan a seeded run: %v", err)
		}
		seen++
		if skeletonRoutable(id) {
			routable++
		} else {
			roleless++
		}
		if scanned[run.State(state)] {
			t.Errorf("the demo world seeds run %s in %q, which the S02.5 recovery ladder scans — "+
				"a live control plane observes it unit_state:\"gone\", crashes it and forks it every sweep (P3-RW-4)",
				id, state)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the seeded runs: %v", err)
	}
	// Non-vacuity, three ways: a world with no runs would prove nothing; and the
	// routability oracle below must be shown to DISCRIMINATE on this world's own
	// ids rather than answering one way for everything.
	if seen == 0 {
		t.Fatalf("the seeded world has no runs, so the ladder-inertness check proves nothing")
	}
	if routable == 0 || roleless == 0 {
		t.Fatalf("the seeded world has %d skeleton-routable and %d roleless run ids; "+
			"the queue check below needs both kinds present to prove it can tell them apart", routable, roleless)
	}

	// (2) The queue half. A row the scheduler can still act on — anything but a
	// settled one — must name a run whose id carries a skeleton role AND that is
	// actually `queued`, because that is the pair the claim loop acts on. A
	// roleless id here is a dispatch that errors with the run left `claimed`,
	// which is where rule (1)'s failure mode comes from.
	qrows, err := b.db.QueryContext(context.Background(),
		`SELECT q.queue_id, q.run_id, q.status, COALESCE(r.state, '<no run>')
		   FROM queue q LEFT JOIN runs r ON r.run_id = q.run_id
		  ORDER BY q.queue_id`)
	if err != nil {
		t.Fatalf("read the seeded queue: %v", err)
	}
	defer qrows.Close()
	for qrows.Next() {
		var qid int64
		var runID, status, state string
		if err := qrows.Scan(&qid, &runID, &status, &state); err != nil {
			t.Fatalf("scan a seeded queue row: %v", err)
		}
		if status == "done" {
			continue // settled history, not something the loop will pick up
		}
		if !skeletonRoutable(runID) || state != string(run.StateQueued) {
			t.Errorf("the demo world seeds queue row %d for run %s (queue %q, run %q): the claim loop "+
				"will CAS-claim it and the dispatch cannot serve it — seed a queue row only for a run that "+
				"may legitimately dispatch (P3-RW-4)", qid, runID, status, state)
		}
	}
	if err := qrows.Err(); err != nil {
		t.Fatalf("read the seeded queue: %v", err)
	}
	// Only claim it if it is true: a summary line printed beside its own
	// failures is the kind of thing a person skims past.
	if !t.Failed() {
		t.Logf("seed hygiene: %d runs, none in a state the ladder scans (%v); no queue row a dispatch could not serve",
			seen, sortedStates(scanned))
	}
}

// runStateByIdent resolves the identifiers the ladder's source names to their
// values. It is compile-checked against the run package, so a renamed constant
// breaks the build here rather than silently shrinking the scanned set.
var runStateByIdent = map[string]run.State{
	"StateNew": run.StateNew, "StateQueued": run.StateQueued, "StateClaimed": run.StateClaimed,
	"StateRunning": run.StateRunning, "StateParked": run.StateParked, "StateDraining": run.StateDraining,
	"StateCompleted": run.StateCompleted, "StateCrashed": run.StateCrashed, "StateFinalized": run.StateFinalized,
	"StateTombstoned": run.StateTombstoned, "StateDiedAtGate": run.StateDiedAtGate,
}

// ladderScannedStates reads the states the S02.5 sweep OBSERVES out of
// internal/recovery's own source — every `InStates(...)` argument in it. Today
// that is claimed/running/draining (step 1) and crashed (the supersedable
// pass); tomorrow it is whatever the ladder actually scans, without this file
// being touched.
func ladderScannedStates(t *testing.T) map[run.State]bool {
	t.Helper()
	const src = "../recovery/recovery.go"
	source, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read the recovery ladder's own source (%s): %v", src, err)
	}
	out := map[run.State]bool{}
	for _, call := range regexp.MustCompile(`InStates\(ctx,([^)]*)\)`).
		FindAllStringSubmatch(string(source), -1) {
		for _, m := range regexp.MustCompile(`run\.(State\w+)`).FindAllStringSubmatch(call[1], -1) {
			s, ok := runStateByIdent[m[1]]
			if !ok {
				t.Fatalf("%s scans run.%s, which this file cannot resolve — add it to runStateByIdent "+
					"(a state the ladder scans and the seed guard does not know is a hole in the guard)", src, m[1])
			}
			out[s] = true
		}
	}
	// Non-vacuity: a parse that found nothing would wave every seeded row
	// through, which is exactly the failure this guard exists to prevent.
	if len(out) < 4 {
		t.Fatalf("%s parsed to %d scanned states (%v), so the ladder-inertness check proves nothing",
			src, len(out), sortedStates(out))
	}
	return out
}

// skeletonRoutable mirrors internal/stage's unexported `runRole`: strip any
// number of `.g<n>` recovery-fork segments, then look for a role suffix. The
// SUFFIXES are the stage package's own exported constants, so the vocabulary
// cannot drift even though the predicate has to be restated.
func skeletonRoutable(runID string) bool {
	id := runID
	for {
		i := strings.LastIndexByte(id, '.')
		if i < 0 {
			return false
		}
		seg := id[i+1:]
		if len(seg) >= 2 && seg[0] == 'g' && strings.IndexFunc(seg[1:], func(r rune) bool {
			return r < '0' || r > '9'
		}) < 0 {
			id = id[:i]
			continue
		}
		break
	}
	for _, suffix := range []string{
		stage.RunSuffixIntake, stage.RunSuffixExecute, stage.RunSuffixVerify,
		stage.RunSuffixCompose, stage.RunSuffixOnboard, stage.RunSuffixDirect,
	} {
		if strings.HasSuffix(id, suffix) {
			return true
		}
	}
	return false
}

func sortedStates(set map[run.State]bool) []string {
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, string(s))
	}
	sort.Strings(out)
	return out
}

// assertNoUndeclaredKanbanStatus is the seed-hygiene guarantee, asserted at
// seed time rather than trusted: NO task in the operator's demo world may sit
// in a column the board did not declare. A future fixture row planted for a
// view's forward-tolerance proof would otherwise silently reappear in the
// operator's walk, which is exactly how "moonshot" got there.
func assertNoUndeclaredKanbanStatus(t *testing.T, b *backend) {
	t.Helper()
	declared := declaredKanbanStatuses(t)
	rows, err := b.db.QueryContext(context.Background(),
		`SELECT task_id, kanban_status FROM tasks ORDER BY task_id`)
	if err != nil {
		t.Fatalf("read the seeded board: %v", err)
	}
	defer rows.Close()
	var seen int
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatalf("scan a seeded task: %v", err)
		}
		seen++
		if !declared[status] {
			t.Errorf("the demo world shows %s in the raw column %q, which the board never declared — "+
				"a test artifact in front of the operator (checkpoint-2 C2-6)", id, status)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the seeded board: %v", err)
	}
	// Non-vacuity: a world with no tasks would pass the loop above having
	// proved nothing at all.
	if seen == 0 {
		t.Fatalf("the seeded world has no tasks, so the seed-hygiene check proves nothing")
	}
	t.Logf("seed hygiene: %d tasks, every one in a declared board column", seen)
}

// declaredKanbanStatuses reads the column vocabulary from the view that OWNS it
// (`web/src/kanban.ts`) rather than keeping a Go copy of the same six strings.
// A second maintained copy of a vocabulary is this package's own §40-C hazard,
// and a drifted copy would let the check above pass while the operator still
// saw a raw column.
func declaredKanbanStatuses(t *testing.T) map[string]bool {
	t.Helper()
	const src = "../../web/src/kanban.ts"
	source, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read the board's column vocabulary (%s): %v", src, err)
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`\{\s*status:\s*'([^']+)'\s*,\s*label:`).
		FindAllStringSubmatch(string(source), -1) {
		out[m[1]] = true
	}
	// The one ABSORBED status is vocabulary too: the board recognizes a stored
	// `cancelled` and renders it IN Backlog wearing its cancelled sign (map §3
	// v3, checkpoint-2 decision D-B — there is no Cancelled column). A demo
	// world holding a cancelled task is therefore a designed state in front of
	// the operator, not a test artifact, and this check must not call it one.
	// Read from the same file that owns it, same as the columns.
	for _, m := range regexp.MustCompile(`cancelledStatus\s*=\s*'([^']+)'`).
		FindAllStringSubmatch(string(source), -1) {
		out[m[1]] = true
	}
	// Non-vacuity again: a parse that found nothing would declare every status
	// undeclared, and a parse that found ONE would wave most of them through.
	// Six = the five declared columns plus the absorbed `cancelled`.
	if len(out) < 6 {
		t.Fatalf("%s parsed to %d declared statuses (%v), so the seed-hygiene check proves nothing",
			src, len(out), out)
	}
	return out
}

// TestSeedIsUnreachableFromTheShippedBinary is the compile-boundary negative,
// asserted rather than argued.
//
// Every identifier the seed introduces must live ONLY in files ending
// `_test.go`. A production file naming one would mean the seed had leaked into
// something `go build ./cmd/sinet` compiles, which is the one thing this shape
// exists to make impossible.
func TestSeedIsUnreachableFromTheShippedBinary(t *testing.T) {
	roots := []string{"..", "../../cmd"}
	names := []string{"TestSeedDemoWorld", "fixtureWorldOn", "demoWorldOn", seedEnv}

	var production []string
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil //nolint:nilerr // an unreadable path is not a production leak
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return nil //nolint:nilerr
			}
			for _, n := range names {
				if strings.Contains(string(src), n) {
					production = append(production, path+" names "+n)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(production) > 0 {
		t.Errorf("the demo seed reached a file the shipped binary compiles:\n  %s", strings.Join(production, "\n  "))
	}

	// Non-vacuity: the walk really did read production Go. Without this the
	// check above passes over a walk that found nothing at all.
	var scanned int
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				scanned++
			}
			return nil
		})
	}
	if scanned < 50 {
		t.Errorf("the production scan reached only %d files, so it proves nothing", scanned)
	}

	// ...and the seed identifiers really are present in the test tree, so the
	// negative above is about placement rather than about a name nobody uses.
	src, err := os.ReadFile("seedworld_test.go")
	if err != nil {
		t.Fatalf("read this file: %v", err)
	}
	for _, n := range names {
		if !strings.Contains(string(src), n) {
			t.Errorf("%s is not in the seed's own file, so the placement check is vacuous", n)
		}
	}
}

// TestSeedEnvIsNeverAutomated extends the `TestFixtureWriterIsNeverAutomated`
// discipline to the seed: CI never seeds anything, because seeding writes a
// database outside the test sandbox.
func TestSeedEnvIsNeverAutomated(t *testing.T) {
	if os.Getenv(seedEnv) != "" {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10, tier-R): %s is set, which is the deliberate seeding act", seedEnv)
	}
	for _, path := range []string{"../../.github/workflows/ci.yml", "../../Makefile"} {
		src, err := os.ReadFile(path)
		if err != nil {
			continue // an absent file cannot set the variable
		}
		if strings.Contains(string(src), seedEnv) {
			t.Errorf("%s names %s — seeding writes a real database and is an operator act, never something CI does",
				path, seedEnv)
		}
	}
}
