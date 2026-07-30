package conformance_test

// conformance_test.go — the S14.5 registry hermetic battery (rubric 23): a
// t.TempDir() DB, stdlib testing only, zero network. Migration + seed-row
// conformance, fixtures resolve by source scan, the one-tx recording verb,
// dueness math with restart-safety, derive-row derivation, and the import wall.

import (
	"context"
	"database/sql"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

const repoRoot = "../../"

type harness struct {
	db  *storage.DB
	log *eventlog.Log
	reg *settings.Registry
	st  *conformance.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := db.ReapplySettings(ctx); err != nil {
		t.Fatalf("ReapplySettings: %v", err)
	}
	return &harness{db: db, log: log, reg: reg, st: conformance.NewStore(db, log, reg)}
}

func (h *harness) seed(t *testing.T) {
	t.Helper()
	n, err := h.st.EnsureSeeded(context.Background())
	if err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	if n == 0 {
		t.Fatal("EnsureSeeded returned 0 rows")
	}
}

func (h *harness) at(now time.Time) { h.st.Now = func() time.Time { return now } }

func find(t *testing.T, states []conformance.RowState, id string) conformance.RowState {
	t.Helper()
	for _, s := range states {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("row %q not found in %d states", id, len(states))
	return conformance.RowState{}
}

func (h *harness) list(t *testing.T) []conformance.RowState {
	t.Helper()
	states, err := h.st.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return states
}

func (h *harness) evalScoreCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM run_events WHERE type = ?`, conformance.EventScoreRecorded).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// ── migration + table (rubric 1) ──

func TestMigrationContiguousUserVersion(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v, err := h.db.UserVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// EXACT pin, moved forward deliberately as each packet lands its migration:
	// 0011 is this packet's, 0012 is B5-5's, 0013 is B5-6A's (the S14.6 watch-row
	// config store), 0017 is B6-2B's (the S10.4 budgets/pause + the S15.5 hint),
	// 0018 is B6-2C's (the BENCH-REG §2 direct-arm capture column) and 0019 is
	// B6-3A's (the S10.3 price table's durable home).
	// A floor would let an unnoticed migration slip in.
	if v != 21 {
		t.Fatalf("user_version = %d, want 21 (migrations through 0021 applied contiguously)", v)
	}
	var n int
	if err := h.db.QueryRowContext(ctx, `SELECT count(*) FROM conformance_registry`).Scan(&n); err != nil {
		t.Fatalf("conformance_registry not queryable (migration did not create it): %v", err)
	}
}

// ── seed-row conformance (rubric 3) ──

func TestSeedRowsCoverEveryS14_5Group(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	states := h.list(t)

	// The six spec row-groups (adapter per-lane split into the lanes whose
	// suites EXIST, R7): declared-by + schedule verbatim from the S14.5 table.
	spec := []struct{ id, section, schedule string }{
		{"kill9-suspend-cycle", "S02.9", "before any storage-touching component bump; quarterly sweep"},
		{"adapter-anthropic", "S03.1/S03.5", "on any engine/CLI version change + weekly"},
		{"adapter-local", "S03.2/S12.10", "on any engine/CLI version change + weekly"},
		{"d6-violation-attempts", "S04.5", "quarterly + on engine bump"},
		{"compaction-canary", "S05.7", "on engine bump + quarterly"},
		{"forced-escalation-e2e", "S06/S04/S07", "quarterly, with the full escalation drill (G1 Def.3 superseded set)"},
		{"verified-restore-drill", "S02.9/S13", "per S13's schedule"},
	}
	for _, want := range spec {
		got := find(t, states, want.id)
		if got.OwningSection != want.section {
			t.Errorf("row %q owning_section = %q, want %q (S14.5 table verbatim)", want.id, got.OwningSection, want.section)
		}
		if got.Schedule != want.schedule {
			t.Errorf("row %q schedule = %q, want %q (S14.5 table verbatim)", want.id, got.Schedule, want.schedule)
		}
	}

	// The two discretionary rows: the no-engine-SSE-replay assertion (CF2) and
	// the dead-man canary drill (OQ5(c)).
	find(t, states, "no-engine-sse-replay")
	find(t, states, "dead-man-canary")
	// The S14.8 regression-sweep row (B5-5): the S14.8 results' recording home.
	find(t, states, conformance.RowRegressionSweep)
	// The S15.1 scheduled SPA dependency pass (B6-4), co-scheduled with S16.7.
	find(t, states, "frontend-dependency-pass")
	if len(states) != 12 {
		t.Fatalf("seed produced %d rows, want 12 (7 spec-group rows + no-sse-replay + dead-man + the S14.8 regression sweep + the B5-8B Layer-2 injection battery + the B6-4 frontend dependency pass)", len(states))
	}

	// The zai lane is OMITTED (R7): no row claims it, and the reason is recorded.
	for _, s := range states {
		if strings.Contains(s.ID, "zai") {
			t.Errorf("a zai adapter row exists (%q) — the zai lane has NO suite and must not be registered (R7)", s.ID)
		}
	}
	zaiReasonRecorded := false
	for _, s := range states {
		if strings.Contains(s.Notes, "zai") {
			zaiReasonRecorded = true
		}
	}
	if !zaiReasonRecorded {
		t.Error("the zai-omission reason is not recorded in any row's notes (R7 honest-dormancy)")
	}
}

// ── fixtures resolve (rubric 4) ──

func TestSeedFixturesResolveToRealTests(t *testing.T) {
	for _, row := range conformance.SeedRows() {
		if len(row.Fixtures) == 0 {
			t.Errorf("row %q has no fixtures", row.ID)
		}
		for _, fx := range row.Fixtures {
			switch {
			case fx.Run != "":
				if !testFuncExists(t, filepath.Join(repoRoot, fx.Pkg), fx.Run) {
					t.Errorf("row %q fixture names test %q in %q — not found in-tree (a row must never claim a suite that does not exist, rubric 4)", row.ID, fx.Run, fx.Pkg)
				}
			case fx.File != "":
				if _, err := os.Stat(filepath.Join(repoRoot, fx.File)); err != nil {
					t.Errorf("row %q fixture names file %q — not found: %v", row.ID, fx.File, err)
				}
			default:
				t.Errorf("row %q has a fixture with neither a test (Pkg/Run) nor a file", row.ID)
			}
		}
	}
}

// testFuncExists reports whether a Go test function named fn is declared in any
// _test.go file under dir (a source scan — no execution).
func testFuncExists(t *testing.T, dir, fn string) bool {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	needle := "func " + fn + "("
	for _, p := range matches {
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if strings.Contains(string(body), needle) {
			return true
		}
	}
	return false
}

// ── idempotency + immutability triggers (R5) ──

func TestEnsureSeededIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	n1, err := h.st.EnsureSeeded(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := h.st.EnsureSeeded(ctx)
	if err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if n1 != n2 {
		t.Fatalf("seed count changed on re-seed: %d then %d", n1, n2)
	}
	var rows int
	if err := h.db.QueryRowContext(ctx, `SELECT count(*) FROM conformance_registry`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != n1 {
		t.Fatalf("re-seed changed row count: %d rows, %d seeded", rows, n1)
	}
}

func TestStructureImmutableTrigger(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	// A last_run/last_result update is allowed (the mutable result state).
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE conformance_registry SET last_run = ?, last_result = 'green' WHERE row_id = 'kill9-suspend-cycle'`,
			time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("updating last_run/last_result must be allowed: %v", err)
	}

	// A structural update aborts (R5): identity/schedule/etc. are immutable.
	for _, col := range []string{"schedule", "owning_section", "fixtures", "cadence", "affect_class"} {
		err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`UPDATE conformance_registry SET `+col+` = 'x' WHERE row_id = 'kill9-suspend-cycle'`)
			return err
		})
		if err == nil {
			t.Errorf("UPDATE of structural column %q was allowed — the structure-immutable trigger must abort it (R5)", col)
		}
	}
}

func TestNoDeleteTrigger(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	err := h.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), `DELETE FROM conformance_registry WHERE row_id = 'kill9-suspend-cycle'`)
		return err
	})
	if err == nil {
		t.Error("DELETE of a conformance obligation row was allowed — the no-delete trigger must abort it")
	}
}

// ── the recording verb (R11/R12; rubric 6/7) ──

func greenResult(rowID string) conformance.Result {
	return conformance.Result{
		RowID: rowID, SuiteVersion: "v1", AssetID: "engine:claude-cli", AssetVersion: "2.1.215",
		Runner: "go test ./internal/adapters/claudecli/", RunnerVersion: "go1.26.5",
		Metrics: map[string]any{"cases_total": 42, "cases_passed": 42}, Passed: true,
	}
}

func TestRecordResultOneTxGreenAndRed(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	// GREEN: one eval.score_recorded event AND the row update land together.
	if err := h.st.RecordResult(ctx, greenResult("adapter-anthropic")); err != nil {
		t.Fatalf("RecordResult green: %v", err)
	}
	if got := h.evalScoreCount(t); got != 1 {
		t.Fatalf("eval.score_recorded events = %d, want 1", got)
	}
	got := find(t, h.list(t), "adapter-anthropic")
	if got.LastResult != conformance.ResultGreen || got.LastRun.IsZero() {
		t.Fatalf("green record: last_result=%q last_run zero=%v; want green + a set last_run", got.LastResult, got.LastRun.IsZero())
	}

	// RED: the row flips and a second event lands.
	red := greenResult("adapter-anthropic")
	red.Passed = false
	if err := h.st.RecordResult(ctx, red); err != nil {
		t.Fatalf("RecordResult red: %v", err)
	}
	if got := h.evalScoreCount(t); got != 2 {
		t.Fatalf("eval.score_recorded events = %d, want 2", got)
	}
	if got := find(t, h.list(t), "adapter-anthropic"); got.LastResult != conformance.ResultRed {
		t.Fatalf("red record: last_result=%q, want red", got.LastResult)
	}
}

func TestRecordResultAtomicNeitherOnUnknownRow(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	before := h.evalScoreCount(t)
	err := h.st.RecordResult(context.Background(), greenResult("no-such-row"))
	if err == nil {
		t.Fatal("RecordResult on an unknown row must fail")
	}
	if after := h.evalScoreCount(t); after != before {
		t.Fatalf("a failed record leaked an event: %d → %d (the append + row update are ONE tx, R12)", before, after)
	}
}

func TestRecordResultRefusesDeriveRows(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	for _, id := range []string{"verified-restore-drill", "dead-man-canary"} {
		err := h.st.RecordResult(ctx, greenResult(id))
		if err == nil {
			t.Errorf("RecordResult on DERIVE row %q must be refused (its result is owned by its subsystem, OQ2/OQ5)", id)
		}
		if got := find(t, h.list(t), id); got.HasRun && got.LastResult == conformance.ResultGreen {
			// derive rows resolve from events (none here) — a refused record must
			// not have written the table.
			t.Errorf("DERIVE row %q shows a recorded green result — the recording surface wrote a derive row", id)
		}
	}
	if h.evalScoreCount(t) != 0 {
		t.Error("a refused derive-row record still appended an event")
	}
}

func TestRecordResultRejectsIncomplete(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	bad := greenResult("adapter-anthropic")
	bad.RunnerVersion = ""
	if err := h.st.RecordResult(context.Background(), bad); err == nil {
		t.Fatal("RecordResult with a missing contract-minimum field must fail (Spec S14.2)")
	}
}

func TestRecordResultGoldenPayloadFields(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	if err := h.st.RecordResult(ctx, greenResult("adapter-anthropic")); err != nil {
		t.Fatal(err)
	}
	events, err := h.log.After(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var payload json.RawMessage
	for _, e := range events {
		if e.Type == conformance.EventScoreRecorded {
			payload = e.Payload
		}
	}
	if payload == nil {
		t.Fatal("no eval.score_recorded event found")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	for _, k := range []string{"suite_id", "suite_version", "asset_id", "asset_version", "metrics", "runner", "runner_version", "result"} {
		if _, ok := m[k]; !ok {
			t.Errorf("eval.score_recorded payload missing contract-minimum key %q (S14.2 Benchmark & eval; R11)", k)
		}
	}
}

// TestRecordResultAtomicBothOrNeither is the GENUINE both-or-neither proof
// (drain D2): a test-local trigger ABORTs the row UPDATE, which RecordResult
// does AFTER appending the event, both in ONE WriteTx — so the append must roll
// back with it. A two-transaction implementation (append committed before the
// update) would leave the event behind and fail here; the abort-before-append
// failure tests (unknown/derive/incomplete row) cannot catch that.
func TestRecordResultAtomicBothOrNeither(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`CREATE TRIGGER test_abort_result_update
			   BEFORE UPDATE OF last_run, last_result ON conformance_registry
			 BEGIN SELECT RAISE(ABORT, 'test: abort the row update AFTER the append point'); END;`)
		return err
	}); err != nil {
		t.Fatalf("install test trigger: %v", err)
	}
	t.Cleanup(func() {
		_ = h.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DROP TRIGGER IF EXISTS test_abort_result_update`)
			return err
		})
	})

	before := h.evalScoreCount(t)
	if err := h.st.RecordResult(ctx, greenResult("adapter-anthropic")); err == nil {
		t.Fatal("RecordResult must error when the row update aborts")
	}
	if after := h.evalScoreCount(t); after != before {
		t.Fatalf("the eval.score_recorded append was NOT rolled back with the aborted update: %d → %d — both-or-neither FAILED (append + update are not ONE tx)", before, after)
	}
	var lr sql.NullString
	if err := h.db.QueryRowContext(ctx, `SELECT last_result FROM conformance_registry WHERE row_id = 'adapter-anthropic'`).Scan(&lr); err != nil {
		t.Fatal(err)
	}
	if lr.Valid {
		t.Errorf("the row update landed despite the aborted tx: last_result = %q", lr.String)
	}
}

// ── the bump-gating set (rubric 10; R19) ──

func TestBumpGatingSetSurfacesBLimbReference(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	rows, err := h.st.BumpGatingRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The engine_bump set is exactly the five engine-bump-triggered rows.
	want := map[string]bool{
		"adapter-anthropic": true, "adapter-local": true, "no-engine-sse-replay": true,
		"d6-violation-attempts": true, "compaction-canary": true,
		// The S14.8 regression sweep carries engine_bump because it is where
		// limb (b) — the before/after quality probe — records (B5-5 OQ2(a)).
		conformance.RowRegressionSweep: true,
	}
	if len(rows) != len(want) {
		t.Fatalf("BumpGatingRows returned %d rows, want %d (the engine_bump set)", len(rows), len(want))
	}
	for _, st := range rows {
		if !want[st.ID] {
			t.Errorf("BumpGatingRows returned %q — not an engine_bump row", st.ID)
		}
		if !strings.Contains(st.TriggerSet, "engine_bump") {
			t.Errorf("row %q trigger_set = %q, missing engine_bump", st.ID, st.TriggerSet)
		}
		// Each row carries the two-limb bump-gate with its (b)-limb S14.8/B5-5
		// reference as row DATA (R19): a bump lands only after (a) the suite
		// passes AND (b) the S14.8 quality probe shows no regression.
		for _, needle := range []string{"(a)", "(b)", "S14.8", "B5-5"} {
			if !strings.Contains(st.Notes, needle) {
				t.Errorf("engine_bump row %q notes miss %q — the two-limb (b)-limb reference must ride the row: %q", st.ID, needle, st.Notes)
			}
		}
	}
	// No non-engine_bump row leaks in (the drill/restore/dead-man/kill-9 rows
	// carry other triggers).
	for _, st := range rows {
		switch st.ID {
		case "forced-escalation-e2e", "verified-restore-drill", "dead-man-canary", "kill9-suspend-cycle":
			t.Errorf("non-engine_bump row %q leaked into the bump-gating set", st.ID)
		}
	}
}

// ── dueness (rubric 9) ──

func TestDuenessStructuralAndSettingsBacked(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	h.at(now)

	// Never-run: every row is due (its overdue-ness is trivially visible, OQ3).
	due, err := h.st.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 12 {
		t.Fatalf("never-run: %d rows due, want all 12", len(due))
	}

	// Record a quarterly row at now → not due; +100d → due (quarterly = 3 months).
	r := greenResult("kill9-suspend-cycle")
	r.RanAt = now
	if err := h.st.RecordResult(ctx, r); err != nil {
		t.Fatal(err)
	}
	if find(t, h.list(t), "kill9-suspend-cycle").Due {
		t.Error("a quarterly row recorded just now must not be due")
	}
	h.at(now.Add(100 * 24 * time.Hour))
	if !find(t, h.list(t), "kill9-suspend-cycle").Due {
		t.Error("a quarterly row 100 days stale must be due")
	}

	// A weekly row: 8 days stale → due, fresh → not.
	h.at(now)
	rw := greenResult("adapter-anthropic")
	rw.RanAt = now.Add(-8 * 24 * time.Hour)
	if err := h.st.RecordResult(ctx, rw); err != nil {
		t.Fatal(err)
	}
	if !find(t, h.list(t), "adapter-anthropic").Due {
		t.Error("a weekly row 8 days stale must be due")
	}

	// The settings-backed forced-escalation row reads ⚙ verification.drill_interval_days
	// (default 90): 100 days stale → due, 10 days → not.
	rf := greenResult("forced-escalation-e2e")
	rf.RanAt = now.Add(-100 * 24 * time.Hour)
	if err := h.st.RecordResult(ctx, rf); err != nil {
		t.Fatal(err)
	}
	if !find(t, h.list(t), "forced-escalation-e2e").Due {
		t.Error("forced-escalation 100 days stale must be due at the default 90-day drill interval")
	}
	// The forced-escalation row exposes the visible-only escalation-canary cadence
	// (⚙ verification.canary_interval_hours default 24h; OQ4(a)).
	if got := find(t, h.list(t), "forced-escalation-e2e").CanaryEvery; got != 24*time.Hour {
		t.Errorf("forced-escalation CanaryEvery = %v, want 24h (⚙ verification.canary_interval_hours default)", got)
	}

	// The B6-4 SPA dependency-pass row reads ⚙ frontend.dependency_pass_interval
	// (default 90 days) LIVE: 95 days stale is due, and widening the ⚙ value
	// un-dues the same row with no re-seed and no restart.
	rp := greenResult("frontend-dependency-pass")
	rp.RanAt = now.Add(-95 * 24 * time.Hour)
	if err := h.st.RecordResult(ctx, rp); err != nil {
		t.Fatal(err)
	}
	if !find(t, h.list(t), "frontend-dependency-pass").Due {
		t.Error("the SPA dependency pass 95 days stale must be due at the default 90-day interval")
	}
	if err := h.reg.Set(ctx, settings.SetRequest{
		Key: "frontend.dependency_pass_interval", Value: json.RawMessage(`180`),
		Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}, Reason: "widen the SPA pass",
	}); err != nil {
		t.Fatalf("Set frontend.dependency_pass_interval: %v", err)
	}
	if find(t, h.list(t), "frontend-dependency-pass").Due {
		t.Error("at ⚙ 180 days, a 95-day-old SPA pass is not due — the cadence is not being read live")
	}
}

// TestDuenessSurvivesRestart proves dueness is derived from stored state, never
// an in-memory ticker (R17): a FRESH Store over the same DB (a process restart)
// computes identical dueness from the persisted last_run + the clock.
func TestDuenessSurvivesRestart(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	r := greenResult("kill9-suspend-cycle")
	r.RanAt = now
	h.at(now)
	if err := h.st.RecordResult(ctx, r); err != nil {
		t.Fatal(err)
	}

	// A brand-new Store (no carried in-memory state) at now: not due.
	fresh := conformance.NewStore(h.db, h.log, h.reg)
	fresh.Now = func() time.Time { return now }
	s1, err := fresh.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if find(t, s1, "kill9-suspend-cycle").Due {
		t.Error("fresh Store at record time: row must not be due (dueness derives from stored last_run)")
	}
	// Same fresh Store advanced 100 days: due — proving the clock, not a ticker.
	fresh.Now = func() time.Time { return now.Add(100 * 24 * time.Hour) }
	s2, err := fresh.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !find(t, s2, "kill9-suspend-cycle").Due {
		t.Error("fresh Store +100d: row must be due (a ticker would have reset on restart — the exact failure dueness-from-log guards)")
	}
}

// ── derive rows (OQ2/OQ5(c)) ──

func (h *harness) appendPlatform(t *testing.T, typ, payload string, ts time.Time) {
	t.Helper()
	if _, err := h.log.Append(context.Background(), eventlog.Append{
		UserID: "platform", Type: typ, SchemaVersion: 1, Payload: json.RawMessage(payload), Time: ts,
	}); err != nil {
		t.Fatalf("append %s: %v", typ, err)
	}
}

func TestDeriveRestoreDrillFromEvents(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	h.at(now)

	// Never run: HasRun false, due.
	if got := find(t, h.list(t), "verified-restore-drill"); got.HasRun || !got.Due {
		t.Fatalf("restore-drill never run: HasRun=%v Due=%v; want false,true", got.HasRun, got.Due)
	}

	// A passing drill: green, HasRun, last_run from the event; not due (default 3 months).
	t0 := now.Add(-24 * time.Hour)
	h.appendPlatform(t, "platform.restore_drill", `{"snapshot_id":"s1","ok":true}`, t0)
	got := find(t, h.list(t), "verified-restore-drill")
	if got.LastResult != conformance.ResultGreen || !got.HasRun {
		t.Fatalf("restore-drill after a pass: last_result=%q HasRun=%v; want green,true", got.LastResult, got.HasRun)
	}
	if got.Due {
		t.Error("a restore-drill run yesterday must not be due at the default 3-month interval")
	}

	// A later failing drill wins (latest event): red.
	h.appendPlatform(t, "platform.restore_drill", `{"snapshot_id":"s1","ok":false}`, now)
	if got := find(t, h.list(t), "verified-restore-drill"); got.LastResult != conformance.ResultRed {
		t.Fatalf("restore-drill after a fail: last_result=%q, want red (latest event wins)", got.LastResult)
	}

	// A DERIVE row never carries last_result in the table, so it never surfaces
	// as a red conformance card (no double-fire — the backup's own High flag is
	// the alert, OQ2). Prove the column stays NULL.
	var lastResult sql.NullString
	if err := h.db.QueryRowContext(ctx, `SELECT last_result FROM conformance_registry WHERE row_id='verified-restore-drill'`).Scan(&lastResult); err != nil {
		t.Fatal(err)
	}
	if lastResult.Valid {
		t.Errorf("derive row wrote last_result=%q to the table — it must stay NULL (no conformance-card double-fire)", lastResult.String)
	}
}

func TestDeriveDeadManFromEvents(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	h.at(now)

	// Never probed: HasRun false, due (daily).
	if got := find(t, h.list(t), "dead-man-canary"); got.HasRun || !got.Due {
		t.Fatalf("dead-man never probed: HasRun=%v Due=%v; want false,true", got.HasRun, got.Due)
	}

	// A canary birth 1h ago: green (no alarm), not due (daily = 24h).
	birth := now.Add(-time.Hour)
	h.insertDeadmanBirth(t, "platform.deadman.1", birth)
	got := find(t, h.list(t), "dead-man-canary")
	if got.LastResult != conformance.ResultGreen || !got.HasRun {
		t.Fatalf("dead-man after a pass: last_result=%q HasRun=%v; want green,true", got.LastResult, got.HasRun)
	}
	if got.Due {
		t.Error("a dead-man probe 1h ago must not be due at the 24h cadence")
	}
	h.at(now.Add(25 * time.Hour))
	if !find(t, h.list(t), "dead-man-canary").Due {
		t.Error("a dead-man probe 25h stale must be due")
	}

	// A run-less watchdog.flagged rule=watchdog.dead_man after the birth → red.
	h.at(now)
	h.appendPlatform(t, "watchdog.flagged", `{"rule":"watchdog.dead_man","anomaly_class":"watchdog.dead_man","severity":"flag-now"}`, now)
	if got := find(t, h.list(t), "dead-man-canary"); got.LastResult != conformance.ResultRed {
		t.Fatalf("dead-man after an alarm: last_result=%q, want red", got.LastResult)
	}
}

// insertDeadmanBirth inserts a platform.deadman.* run + its run.created event
// directly (the run_events FK to runs requires the run to exist). Raw SQL, no
// import of internal/run — mirrors what the watchdog canary produces.
func (h *harness) insertDeadmanBirth(t *testing.T, runID string, ts time.Time) {
	t.Helper()
	tsStr := ts.UTC().Format(time.RFC3339Nano)
	if err := h.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO runs (run_id, user_id, state, created_ts, updated_ts) VALUES (?, 'platform', 'completed', ?, ?)`,
			runID, tsStr, tsStr); err != nil {
			return err
		}
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
			 VALUES (?, 0, 'platform', 'run.created', 1, '{}', ?)`, runID, tsStr)
		return err
	}); err != nil {
		t.Fatalf("insert deadman birth: %v", err)
	}
}

// ── import wall (rubric 13) ──

func TestImportWall(t *testing.T) {
	allowed := map[string]bool{}
	const modPrefix = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/"
	for _, p := range []string{"storage", "eventlog", "settings"} {
		allowed[modPrefix+p] = true
	}
	fset := token.NewFileSet()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		scanned++
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if !strings.Contains(p, ".") {
				continue // stdlib
			}
			if !allowed[p] {
				t.Errorf("%s imports %q — outside internal/conformance's pinned set {storage, eventlog, settings, stdlib}; it must never import a producer package or internal/api (R21)", filepath.Base(path), p)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no conformance source files scanned")
	}
}
