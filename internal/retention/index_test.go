package retention_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// TestFTS5CoversTheThreeS1410Sources (rubric 15): "FTS5 over run summaries,
// verdict texts, and drift summaries".
func TestFTS5CoversTheThreeS1410Sources(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.task("t1", "alice", "Rebuild the widget compiler")
	f.startRun("r1", "alice", "t1")
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	f.appendAt("", "alice", "verdict.recorded",
		`{"round":1,"verdict":"fail","rubric_id":"rubric-software","findings":[{"note":"unhandled nullpointer"}]}`, f.now)
	f.appendAt("", "alice", "drift.finding",
		`{"source":"anthropic-pricing","change_class":"price","summary":"opus tier repriced upward"}`, f.now)

	res, err := f.store.Index(f.ctx)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if res.Indexed != 3 {
		t.Errorf("indexed = %d, want 3 (one run summary, one verdict, one drift finding)", res.Indexed)
	}

	for _, c := range []struct{ query, kind string }{
		{"widget", retention.KindRunSummary},
		{"nullpointer", retention.KindVerdict},
		{"repriced", retention.KindDrift},
	} {
		hits, err := f.store.Search(f.ctx, c.query, "", 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", c.query, err)
		}
		if len(hits) != 1 || hits[0].Kind != c.kind {
			t.Errorf("Search(%q) = %+v, want one %s hit", c.query, hits, c.kind)
		}
	}

	// Owner-scoping (S01.9): a member's scope sees only their own rows.
	f.user("bob", "member")
	if hits, err := f.store.Search(f.ctx, "widget", "bob", 10); err != nil || len(hits) != 0 {
		t.Errorf("bob's scoped search = %+v (%v), want no hits on alice's summary", hits, err)
	}
	if hits, err := f.store.Search(f.ctx, "widget", "alice", 10); err != nil || len(hits) != 1 {
		t.Errorf("alice's scoped search = %+v (%v), want her own summary", hits, err)
	}
}

// TestIndexIsIdempotentAndCursorDurable: re-running indexes nothing twice, and
// the cursor survives a new Store over the same database (restart-safe).
func TestIndexIsIdempotentAndCursorDurable(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.appendAt("", "alice", "drift.finding", `{"summary":"one finding"}`, f.now)

	first, err := f.store.Index(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Indexed != 1 {
		t.Fatalf("first pass indexed %d, want 1", first.Indexed)
	}
	second, err := f.store.Index(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Indexed != 0 || second.Rolled != 0 {
		t.Errorf("second pass = %+v, want a no-op past the durable cursor", second)
	}
	var rows int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM history_fts`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("history_fts rows = %d, want 1 (rowid = event_seq makes re-indexing a replace)", rows)
	}
	var cursor int64
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT last_seq FROM retention_index_cursor WHERE row_id = 'indexer'`).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if cursor == 0 {
		t.Error("the indexer cursor must be durable, not head-bootstrapped")
	}
}

// TestRollupsAreDerivedAndRebuildable (rubric 17): rollups are derived state,
// rebuildable from the log by replay — never a system of record.
func TestRollupsAreDerivedAndRebuildable(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	for i := 0; i < 3; i++ {
		f.appendAt("", "alice", "tool.completed", `{"tool":"grep"}`, f.now)
	}
	f.appendAt("", "alice", "drift.finding", `{"summary":"x"}`, f.now)

	if _, err := f.store.Index(f.ctx); err != nil {
		t.Fatal(err)
	}
	rollup, err := f.store.Rollup(f.ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, r := range rollup {
		counts[r.Type] += r.N
	}
	if counts["tool.completed"] != 3 || counts["drift.finding"] != 1 {
		t.Errorf("rollup = %+v, want tool.completed 3 / drift.finding 1", counts)
	}

	// Derived: wiping both tables and replaying reproduces them exactly.
	before := counts
	if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx, `DELETE FROM run_event_rollup`)
		return err
	}); err != nil {
		t.Fatalf("a rollup must be deletable — it is derived state, not a record: %v", err)
	}
	if _, err := f.store.Rebuild(f.ctx); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	rollup, err = f.store.Rollup(f.ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	after := map[string]int64{}
	for _, r := range rollup {
		after[r.Type] += r.N
	}
	for typ, n := range before {
		if after[typ] != n {
			t.Errorf("rebuild changed %s: %d -> %d; rollups must be reproducible from the log", typ, n, after[typ])
		}
	}
}

// TestIndexReconcilesAfterARestore: the S13.10 dump never carries FTS content,
// while the cursor table restores at its old position. Boot repairs it.
func TestIndexReconcilesAfterARestore(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.appendAt("", "alice", "drift.finding", `{"summary":"restorable finding"}`, f.now)
	if _, err := f.store.Index(f.ctx); err != nil {
		t.Fatal(err)
	}
	// The exact shape a restore leaves: the index empty, the cursor at head.
	if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx, `DELETE FROM history_fts`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if hits, _ := f.store.Search(f.ctx, "restorable", "", 10); len(hits) != 0 {
		t.Fatal("the index was not emptied by the test setup")
	}
	rebuilt, err := f.store.ReconcileIndex(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rebuilt {
		t.Fatal("ReconcileIndex did not rebuild an empty index behind a non-zero cursor")
	}
	hits, err := f.store.Search(f.ctx, "restorable", "", 10)
	if err != nil || len(hits) != 1 {
		t.Errorf("search after reconcile = %+v (%v), want the finding back", hits, err)
	}
	// It is not a rebuild-every-boot: a healthy index is left alone.
	if again, err := f.store.ReconcileIndex(f.ctx); err != nil || again {
		t.Errorf("ReconcileIndex rebuilt a healthy index (%v, %v)", again, err)
	}
}

// TestFTSShadowTablesAreHandledByTheBackupLayer (rubric 15, second half): a new
// FTS5 virtual table must inherit internal/storage's shadow-table handling —
// its shadows are never dumped and never trip the "unexpected prefix" refusal.
func TestFTSShadowTablesAreHandledByTheBackupLayer(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.appendAt("", "alice", "drift.finding", `{"summary":"indexed body"}`, f.now)
	if _, err := f.store.Index(f.ctx); err != nil {
		t.Fatal(err)
	}
	vac := filepath.Join(t.TempDir(), "vac.db")
	if err := f.db.VacuumInto(f.ctx, vac); err != nil {
		t.Fatal(err)
	}
	dump, uv, err := storage.DumpFrom(f.ctx, vac)
	if err != nil {
		t.Fatalf("DumpFrom must handle the new FTS5 table's shadows: %v", err)
	}
	for _, shadow := range []string{"history_fts_data", "history_fts_idx", "history_fts_docsize", "history_fts_config"} {
		if strings.Contains(dump, `INSERT INTO "`+shadow+`"`) {
			t.Errorf("shadow table %q was dumped; the virtual table owns it", shadow)
		}
	}
	if !strings.Contains(dump, `CREATE VIRTUAL TABLE history_fts`) {
		t.Error("the FTS5 virtual table itself must be in the dump")
	}
	rebuilt := filepath.Join(t.TempDir(), "rebuilt.db")
	if err := storage.RebuildFromDump(f.ctx, rebuilt, dump, uv); err != nil {
		t.Fatalf("RebuildFromDump with the new FTS5 table: %v", err)
	}
}

// TestDottedOrderIsAnOrderedTreePath (rubric 16): a prefix scan on the run's
// path reads its subtree in event order, and the absent-linkage case degrades
// to depth 0 rather than fabricating a parent.
func TestDottedOrderIsAnOrderedTreePath(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	r := f.startRun("r1", "alice", "")
	// Two rows with no linkage and one carrying the S04.4 spawn depth.
	f.appendAt("", "alice", "tool.completed", `{"tool":"grep"}`, f.now) // platform-scope
	if _, err := f.log.Append(f.ctx, mkAppend(r, "helper.spawned", `{"depth":1,"brief_hash":"h"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.log.Append(f.ctx, mkAppend(r, "tool.completed", `{"tool":"write"}`)); err != nil {
		t.Fatal(err)
	}

	rows, err := f.db.QueryContext(f.ctx,
		`SELECT dotted_order FROM run_events WHERE dotted_order LIKE 'r1.%' ORDER BY dotted_order`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		got = append(got, d)
	}
	if len(got) < 2 {
		t.Fatalf("the run's subtree prefix scan found %d rows, want the run's own events", len(got))
	}
	for _, d := range got {
		if !strings.HasPrefix(d, "r1.") {
			t.Errorf("dotted_order %q escaped the run's prefix", d)
		}
	}
	// Lexicographic order IS event order (event_seq is zero-padded).
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Errorf("dotted_order is not monotonic: %q then %q", got[i-1], got[i])
		}
	}
	// The depth-1 spawn row sits at its own level; a linkage-less row at 00.
	var depths []string
	for _, d := range got {
		parts := strings.SplitN(d, ".", 3)
		depths = append(depths, parts[1])
	}
	if !contains(strings.Join(depths, ","), "01") {
		t.Errorf("no row carried the recorded spawn depth: %v", depths)
	}
	if !contains(strings.Join(depths, ","), "00") {
		t.Errorf("a row with no linkage must degrade to depth 00, never a fabricated parent: %v", depths)
	}
	// A platform-scope row (no run) gets the honest 'platform' root.
	var platformPath string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT dotted_order FROM run_events WHERE run_id IS NULL ORDER BY event_seq DESC LIMIT 1`).
		Scan(&platformPath); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(platformPath, "platform.") {
		t.Errorf("platform-scope dotted_order = %q, want the honest platform root", platformPath)
	}
}

// TestCompactionPredicateIsServedByAnIndex (drain D7): this package's OWN
// production query, VERBATIM — a paraphrase could not catch it being edited
// into a full scan. The api-side derives are pinned in
// internal/api/projection_indexes_test.go against their own exported constants,
// which is where those queries live.
func TestCompactionPredicateIsServedByAnIndex(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	for i := 0; i < 80; i++ {
		f.appendAt("", "alice", "engine.message", `{"text":"x"}`, f.now.AddDate(0, -24, 0))
		f.appendAt("", "alice", "verdict.recorded", `{"verdict":"pass"}`, f.now.AddDate(0, -24, 0))
	}

	rows, err := f.db.QueryContext(f.ctx,
		"EXPLAIN QUERY PLAN "+retention.CompactableRowsQuery,
		"alice", "2020-01-01T00:00:00Z", retention.CompactedPayload,
		int64(retention.BulkPayloadFloorBytes), 500)
	if err != nil {
		t.Fatalf("EXPLAIN the compaction predicate: %v", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		b.WriteString(detail)
		b.WriteString("\n")
	}
	detail := b.String()
	// The owner+age scan rides run_events_user_ts_idx; the allowlist join rides
	// the retention_keep_forever primary key.
	if !strings.Contains(detail, "run_events_user_ts_idx") {
		t.Errorf("the compaction predicate does not seek on (user_id, ts):\n%s", detail)
	}
	if strings.Contains(detail, "SCAN run_events") {
		t.Errorf("the compaction predicate full-scans run_events:\n%s", detail)
	}
	if !strings.Contains(detail, "retention_keep_forever") {
		t.Errorf("the keep-forever join is not visible in the plan:\n%s", detail)
	}
}

func mkAppend(r run.Run, typ, payload string) eventlog.Append {
	return eventlog.Append{
		RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
		Type: typ, SchemaVersion: 1, Payload: []byte(payload),
	}
}
