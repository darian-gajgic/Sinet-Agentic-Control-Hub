package retention_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// fixture is the hermetic harness: a migrated temp DB, the event log, the run
// store with the terminal hook installed, and the retention store under test.
type fixture struct {
	t        *testing.T
	ctx      context.Context
	db       *storage.DB
	log      *eventlog.Log
	reg      *settings.Registry
	runs     *run.Store
	store    *retention.Store
	now      time.Time
	narrated int
}

type fixtureOpt func(*retention.Config, *fixture)

// withNarrator installs a fake local-tier narrator (tier F: no stack is dialed,
// $0 by construction).
func withNarrator(fn retention.Narrator) fixtureOpt {
	return func(c *retention.Config, f *fixture) {
		c.Narrator = func(ctx context.Context, runID string, agg retention.Aggregate) (retention.Narrative, error) {
			f.narrated++
			return fn(ctx, runID, agg)
		}
	}
}

func withRemoveFile(fn func(string) error) fixtureOpt {
	return func(c *retention.Config, _ *fixture) { c.RemoveFile = fn }
}

func newFixture(t *testing.T, opts ...fixtureOpt) *fixture {
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
		t.Fatalf("settings Attach: %v", err)
	}
	f := &fixture{t: t, ctx: ctx, db: db, log: log, reg: reg, now: time.Now().UTC()}
	cfg := retention.Config{DB: db, Log: log, Settings: reg, Now: func() time.Time { return f.now }}
	for _, o := range opts {
		o(&cfg, f)
	}
	store, err := retention.New(cfg)
	if err != nil {
		t.Fatalf("retention.New: %v", err)
	}
	f.store = store
	if _, err := store.EnsureKeepForeverSeeded(ctx); err != nil {
		t.Fatalf("EnsureKeepForeverSeeded: %v", err)
	}
	f.runs = run.NewStore(db, log)
	f.runs.SetTerminalHook(func(ctx context.Context, tx *sql.Tx, r run.Run) error {
		_, _, err := store.WriteAtRunEndTx(ctx, tx, r.ID)
		return err
	})
	return f
}

func (f *fixture) user(id, role string) {
	f.t.Helper()
	if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx,
			`INSERT INTO users (user_id, role, created_ts) VALUES (?, ?, ?)`,
			id, role, f.now.Format(time.RFC3339Nano))
		return err
	}); err != nil {
		f.t.Fatalf("seed user %q: %v", id, err)
	}
}

func (f *fixture) task(id, owner, title string) {
	f.t.Helper()
	if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
			id, owner, title, f.now.Format(time.RFC3339Nano))
		return err
	}); err != nil {
		f.t.Fatalf("seed task %q: %v", id, err)
	}
}

// startRun creates a run and drives it to running.
func (f *fixture) startRun(id, owner, taskID string) run.Run {
	f.t.Helper()
	r, err := f.runs.Create(f.ctx, run.NewRun{ID: id, UserID: owner, TaskID: taskID})
	if err != nil {
		f.t.Fatalf("create run %q: %v", id, err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if r, err = f.runs.Transition(f.ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			f.t.Fatalf("transition %q -> %s: %v", id, st, err)
		}
	}
	return r
}

// appendAt writes one event with an explicit timestamp (the horizon's unit).
func (f *fixture) appendAt(runID, owner, typ, payload string, ts time.Time) int64 {
	f.t.Helper()
	a := eventlog.Append{UserID: owner, Type: typ, SchemaVersion: 1, Payload: json.RawMessage(payload), Time: ts}
	if runID != "" {
		a.RunID = runID
	}
	seq, err := f.log.Append(f.ctx, a)
	if err != nil {
		f.t.Fatalf("append %s: %v", typ, err)
	}
	return seq
}

func (f *fixture) payloadAt(seq int64) string {
	f.t.Helper()
	var p string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT payload FROM run_events WHERE event_seq = ?`, seq).Scan(&p); err != nil {
		f.t.Fatalf("read payload %d: %v", seq, err)
	}
	return p
}

// setHorizon writes a per-user ⚙ retention.compaction_horizon override.
func (f *fixture) setHorizon(userID string, months int64) {
	f.t.Helper()
	raw, _ := json.Marshal(months)
	if err := f.reg.Set(f.ctx, settings.SetRequest{
		Key:     "retention.compaction_horizon",
		ForUser: userID,
		Value:   raw,
		Actor:   settings.Actor{Kind: settings.ActorOperator, ID: "op"},
		Reason:  "test",
	}); err != nil {
		f.t.Fatalf("set horizon for %q: %v", userID, err)
	}
}

// ── R6: the keep-forever set is derived from the S14.2 family registry ───────

// TestKeepForeverCoversTheSevenS149Classes (rubric 7) asserts the seven classes
// S14.9 ¶2 names are each covered by a family whose decision is KEEP.
func TestKeepForeverCoversTheSevenS149Classes(t *testing.T) {
	keep := retention.KeepForeverFamilies()
	for _, c := range []struct {
		class  string
		family eventlog.Family
	}{
		{"run summaries", eventlog.FamilyRunSummary},
		{"verdicts", eventlog.FamilyVerificationVerdict},
		{"decisions", eventlog.FamilyHumanDecision},
		{"receipts", eventlog.FamilyUsageLimits},
		{"routing records", eventlog.FamilyRouting},
		{"drift events", eventlog.FamilyDriftCanary},
		{"benchmark records", eventlog.FamilyBenchmarkEval},
	} {
		if !keep[c.family] {
			t.Errorf("S14.9 keep-forever class %q maps to family %q, which is not keep-forever", c.class, c.family)
		}
	}
	if got := len(keep); got != 15 {
		t.Errorf("the keep-forever decision covers %d families, want all 15 S14.2 families", got)
	}
}

// TestEveryRegisteredTypeLandsOnADeliberateSide (rubric 8) is the non-tautology
// half: a newly minted type must land on a DECIDED side of the line. A family
// with no decision — the shape an S00.9 family amendment would create — makes
// every one of its types undecided and fails here.
func TestEveryRegisteredTypeLandsOnADeliberateSide(t *testing.T) {
	keep := retention.KeepForeverFamilies()
	for _, ts := range eventlog.Registry().Types() {
		if _, decided := keep[ts.Family]; !decided {
			t.Errorf("registered type %q is in family %q, which carries NO keep-forever decision — a new family must be decided, never defaulted", ts.Type, ts.Family)
		}
	}
	// The probe: a family the decision table does not know is undecided, so the
	// assertion above can genuinely fail.
	if _, decided := keep[eventlog.Family("a_family_that_does_not_exist")]; decided {
		t.Fatal("the coverage check is tautological — an unknown family reads as decided")
	}
}

// TestBenchmarkPreMarkedRowsAgreeWithTheFamilyMapping (rubric 9): B5-7 marked
// its records Retention: "keep-forever" and deferred ENFORCEMENT here. The
// family mapping is the authority; this is the cross-check that the two agree.
func TestBenchmarkPreMarkedRowsAgreeWithTheFamilyMapping(t *testing.T) {
	for _, typ := range []string{"benchmark.pair_recorded", "benchmark.alarm", "eval.score_recorded"} {
		if !retention.IsKeepForever(typ) {
			t.Errorf("%q is pre-marked keep-forever in internal/benchmark but the family mapping strips it", typ)
		}
	}
}

// TestKeepForeverSeedIsIdempotentAndCoversAliases: the seed is the ONE source
// of truth the export view and the compaction predicate both read, and a
// pre-rename row must not be stripped for wearing its old name (§29 R14).
func TestKeepForeverSeedIsIdempotentAndCoversAliases(t *testing.T) {
	f := newFixture(t)
	again, err := f.store.EnsureKeepForeverSeeded(f.ctx)
	if err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if again != 0 {
		t.Errorf("re-seeding inserted %d rows, want 0 (EnsureSeeded is idempotent)", again)
	}
	seeded, err := f.store.KeepForeverSeeded(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range seeded {
		got[e.Type] = true
	}
	for _, typ := range retention.KeepForeverTypes() {
		if !got[typ.Type] {
			t.Errorf("keep-forever type %q derived from the registry is missing from the seeded allowlist", typ.Type)
		}
	}
	// verify.round is the legacy alias of verdict.recorded (§29 R14).
	if !got["verify.round"] {
		t.Error("the legacy alias verify.round is not allowlisted — a pre-rename verdict row would be stripped")
	}
	if got["tool.completed"] {
		t.Error("tool.completed is trace, not keep-forever — the allowlist must not carry it")
	}
}

// TestKeepForeverIsOneWay: the allowlist's no-delete trigger. A demotion in
// code preserves MORE data, never less.
func TestKeepForeverIsOneWay(t *testing.T) {
	f := newFixture(t)
	err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx, `DELETE FROM retention_keep_forever WHERE type = 'routing.decided'`)
		return err
	})
	if err == nil {
		t.Fatal("a keep-forever row was deleted; the class must be one-way")
	}
}

// ── OQ1: the narrowed append-only trigger (rubric: test the refusals) ────────

// TestNarrowedTriggerPermitsOnlyTheCompactionStrip proves the ONE transition
// 0015 opened and every refusal it kept: an arbitrary payload rewrite, an
// identity-column change, a delete, a strip of a row inside the horizon floor,
// and a second strip of an already-compacted row.
func TestNarrowedTriggerPermitsOnlyTheCompactionStrip(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	old := f.appendAt("", "alice", "tool.completed", `{"tool":"grep","secret":"OLD-BODY"}`,
		f.now.AddDate(0, -13, 0))
	fresh := f.appendAt("", "alice", "tool.completed", `{"tool":"grep","secret":"FRESH-BODY"}`, f.now)

	exec := func(q string, args ...any) error {
		return f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(f.ctx, q, args...)
			return err
		})
	}

	for _, c := range []struct {
		name string
		q    string
		args []any
	}{
		{"arbitrary payload rewrite", `UPDATE run_events SET payload = '{"evil":true}' WHERE event_seq = ?`, []any{old}},
		{"payload set to another marker-like body", `UPDATE run_events SET payload = '{"compacted":true}' WHERE event_seq = ?`, []any{old}},
		{"identity: type", `UPDATE run_events SET type = 'tampered' WHERE event_seq = ?`, []any{old}},
		{"identity: user_id", `UPDATE run_events SET user_id = 'mallory' WHERE event_seq = ?`, []any{old}},
		{"identity: ts", `UPDATE run_events SET ts = '1999-01-01T00:00:00Z' WHERE event_seq = ?`, []any{old}},
		{"identity: event_seq", `UPDATE run_events SET event_seq = 9999 WHERE event_seq = ?`, []any{old}},
		{"identity: schema_version", `UPDATE run_events SET schema_version = 2 WHERE event_seq = ?`, []any{old}},
		{"delete", `DELETE FROM run_events WHERE event_seq = ?`, []any{old}},
		{"strip inside the horizon floor", `UPDATE run_events SET payload = ? WHERE event_seq = ?`, []any{retention.CompactedPayload, fresh}},
	} {
		if err := exec(c.q, c.args...); err == nil {
			t.Errorf("%s: the trigger permitted it — run_events must stay append-only but for the one S14.9 strip", c.name)
		}
	}

	// The ONE permitted transition.
	if err := exec(`UPDATE run_events SET payload = ? WHERE event_seq = ?`, retention.CompactedPayload, old); err != nil {
		t.Fatalf("the sanctioned S14.9 strip was refused: %v", err)
	}
	if got := f.payloadAt(old); got != retention.CompactedPayload {
		t.Errorf("payload after strip = %q, want the compacted marker", got)
	}
	// And it is one-way: a second strip is refused.
	if err := exec(`UPDATE run_events SET payload = ? WHERE event_seq = ?`, retention.CompactedPayload, old); err == nil {
		t.Error("an already-compacted row was stripped again; the transition must be one-way and once")
	}
	// The row is still there — event_seq stays gap-free.
	var n int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM run_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("run_events count = %d, want 2 (compaction elides bodies, never rows)", n)
	}
}

// TestCompactedMarkerMatchesMigration pins the Go constant to the SQL literal
// the 0015 trigger holds. Without this the two could drift into a state where
// the pass writes a body the DB accepts as "compacted" but nothing else
// recognizes.
func TestCompactedMarkerMatchesMigration(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "storage", "migrations", "0015_retention_history.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(body)
	if want := "'" + retention.CompactedPayload + "'"; !contains(sqlText, want) {
		t.Errorf("migration 0015 does not hold retention.CompactedPayload verbatim:\n%s", retention.CompactedPayload)
	}
	if want := "'" + retention.ExportStrippedPayload + "'"; !contains(sqlText, want) {
		t.Errorf("migration 0015 does not hold retention.ExportStrippedPayload verbatim:\n%s", retention.ExportStrippedPayload)
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// ── R18: the B5-7 D5(b) declined-row freeze ─────────────────────────────────

// TestDeclinedBenchmarkPairIsFrozen (R18): 0014 froze a `recorded` pair; 0015
// extends the freeze to a `declined` one. A `sampled` row still moves.
func TestDeclinedBenchmarkPairIsFrozen(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	insert := func(id, state string) {
		t.Helper()
		if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(f.ctx,
				`INSERT INTO benchmark_pairs (pair_id, user_id, domain, task_id, deliverable_id,
				     phase, rate_pct, state, sampled_ts, updated_ts)
				 VALUES (?, 'alice', 'software-development', 't', 'd', 'pre-gate', 100, ?, ?, ?)`,
				id, state, f.now.Format(time.RFC3339Nano), f.now.Format(time.RFC3339Nano))
			return err
		}); err != nil {
			t.Fatalf("seed pair %q: %v", id, err)
		}
	}
	insert("p-declined", "declined")
	insert("p-sampled", "sampled")

	err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx, `UPDATE benchmark_pairs SET declined = 1 WHERE pair_id = 'p-declined'`)
		return err
	})
	if err == nil {
		t.Error("a declined benchmark pair's row was mutated; 0015 must freeze it alongside a recorded one (§35 D5(b))")
	}
	if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx, `UPDATE benchmark_pairs SET state = 'dispatched' WHERE pair_id = 'p-sampled'`)
		return err
	}); err != nil {
		t.Errorf("a sampled pair's working state must still move: %v", err)
	}
}

// ── R16/R17: the §29 flips and the topic reading ────────────────────────────

// TestBothNewTypesAreMintedUnderOneFamily (rubric 18) checks this packet's end
// of the §29 flip; internal/eventlog's contract_test pins the inventory.
func TestBothNewTypesAreMintedUnderOneFamily(t *testing.T) {
	for _, c := range []struct {
		typ    string
		family eventlog.Family
	}{
		{retention.EventSummaryWritten, eventlog.FamilyRunSummary},
		{retention.EventCompacted, eventlog.FamilyPlatform},
	} {
		ts, ok := eventlog.Registry().TypeSpec(c.typ)
		if !ok {
			t.Errorf("%q is not registered", c.typ)
			continue
		}
		if ts.Status != eventlog.StatusMinted {
			t.Errorf("%q status = %q, want minted (this packet is its producer)", c.typ, ts.Status)
		}
		if ts.Family != c.family {
			t.Errorf("%q family = %q, want %q", c.typ, ts.Family, c.family)
		}
		if ts.SchemaVersion != 1 {
			t.Errorf("%q schema_version = %d, want 1", c.typ, ts.SchemaVersion)
		}
		if ts.Provenance == "" || !contains(ts.Provenance, "internal/retention") {
			t.Errorf("%q provenance = %q, want a real internal/retention producer site", c.typ, ts.Provenance)
		}
	}
}

// ── the seams ───────────────────────────────────────────────────────────────
