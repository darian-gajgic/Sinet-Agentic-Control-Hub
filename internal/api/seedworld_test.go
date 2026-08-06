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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
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

	// THE SAME WORLD THE GOLDEN FIXTURES ARE PRODUCED FROM, through the same
	// producers in the same order — users through the real auth store with real
	// PINs, tasks and runs across every state the board buckets, asks and
	// approval cards from the real intake card types, the review surface with
	// its revisions and anchored comments, decisions through the real verbs,
	// workforce rows, memory entries, chat sessions and turns, the push devices,
	// the pause flip and the history index.
	fixtureWorldOn(t, b, abs)

	// SEED HYGIENE — the one deliberate difference between the golden-fixture
	// world and the operator's demo world (checkpoint-2 finding C2-6).
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
	// one for this row: a queued run (`r-archive`), an `intake.state`/drafting
	// event, and a coverage card still unanswered. `done` would have been the
	// louder lie — a finished task with a question still open in the inbox.
	//
	// The committed goldens are produced by `fixtureWorld` and never reach this
	// line, so they are untouched by it.
	exec(t, b, `UPDATE tasks SET kanban_status = ? WHERE task_id = ?`, "intake", "t-archive")
	assertNoUndeclaredKanbanStatus(t, b)

	// What the walk needs to know, printed rather than documented elsewhere: a
	// runbook that drifts from the seed is worse than no runbook.
	t.Logf("seeded demo world at %s", abs)
	t.Logf("people (all with PIN %s): op (operator) · alice (member) · bob (member)", fixturePIN)
	t.Logf("the dev fallback browses without signing in; use the header's Sign in affordance to exercise the real login")
	t.Logf("meters and budgets render the REAL stores' honest state — the fixture world's metering and price DOUBLES")
	t.Logf("cannot reach a throwaway binary, so where the committed bodies show a served figure this world shows the")
	t.Logf("real store's answer, absences and all. That difference is the honest one and is not a defect to report.")
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
	// Non-vacuity again: a parse that found nothing would declare every status
	// undeclared, and a parse that found ONE would wave most of them through.
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
	names := []string{"TestSeedDemoWorld", "fixtureWorldOn", seedEnv}

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
