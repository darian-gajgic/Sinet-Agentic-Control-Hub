package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// seedIndexFixture plants enough rows that the planner prefers an index over a
// scan (on a handful of rows SQLite reasonably chooses a scan whatever indexes
// exist), across the types and owners the derives read.
func seedIndexFixture(t *testing.T, log *eventlog.Log, runs *run.Store) {
	t.Helper()
	ctx := context.Background()
	r := runningRun(t, runs, "r1", "alice")
	now := time.Now().UTC()
	for i := 0; i < 60; i++ {
		for _, e := range []struct{ owner, typ, payload string }{
			{"alice", "watchdog.flagged", `{"rule":"watchdog.loop","anomaly_class":"watchdog.loop"}`},
			{"platform", "watchdog.suppressed", `{"rule":"watchdog.loop","actor":"op"}`},
			{"platform", "benchmark.alarm", `{"action":"raise","domain":"software-development"}`},
			{"platform", "drift.finding", `{"change_class":"price","fingerprint":"fp","severity":"flag-now","summary":"s","source":"x","lanes":["anthropic"]}`},
		} {
			if _, err := log.Append(ctx, eventlog.Append{
				UserID: e.owner, Type: e.typ, SchemaVersion: 1,
				Payload: json.RawMessage(e.payload), Time: now,
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := log.Append(ctx, eventlog.Append{
			RunID: r.ID, Generation: r.Generation, UserID: "alice",
			Type: "run.state_changed", SchemaVersion: 1,
			Payload: json.RawMessage(`{"from":"running","to":"running"}`), Time: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// projection_indexes_test.go — the B5-3 F16 discharge, pinned to the VERBATIM
// production query texts (P3-B5-8A drain D7).
//
// The first cut of this test asserted PARAPHRASED query shapes, which cannot
// catch the thing it exists to catch: a production query edited into a full
// table scan. Every case below runs the exact exported constant the projection
// runs, in both its operator and its member (owner-scoped) form, and names the
// index that must serve it. Two of them are served by migration 0001's
// run_events_run_idx, not by anything 0015 added — recorded honestly rather
// than credited to this packet.

// planFor returns the EXPLAIN QUERY PLAN detail lines for a query.
func planFor(t *testing.T, db *storage.DB, q string, args ...any) string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatalf("EXPLAIN %q: %v", q, err)
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
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestDeriveQueriesAreServedByAnIndex is the F16 acceptance: every previously
// full-scanning derive seeks, and it seeks through the index this packet says
// it does.
func TestDeriveQueriesAreServedByAnIndex(t *testing.T) {
	db, log, runs := wdTestDB(t)
	seedIndexFixture(t, log, runs)

	cases := []struct {
		name  string
		query string
		args  []any
		index string
	}{
		{
			"benchmarkAlarms / QueryByType (operator)",
			QueryByType + OrderByEventSeq,
			[]any{"benchmark.alarm"},
			"run_events_type_seq_idx",
		},
		{
			"benchmarkAlarms / QueryByType (member, owner-scoped)",
			QueryByType + OwnerScopeClause + OrderByEventSeq,
			[]any{"benchmark.alarm", "alice"},
			"run_events_type_",
		},
		{
			"watchdogFlags / QueryWatchdogFlags (operator)",
			QueryWatchdogFlags + OrderByEventSeq,
			nil,
			"run_events_type_seq_idx",
		},
		{
			"watchdogFlags / QueryWatchdogFlags (member, owner-scoped)",
			QueryWatchdogFlags + OwnerScopeClause + OrderByEventSeq,
			[]any{"alice"},
			"run_events_type_",
		},
		{
			"maxSeqByKey / QueryMaxSeqByKey (operator)",
			QueryMaxSeqByKey,
			[]any{"watchdog.suppressed"},
			"run_events_type_",
		},
		{
			"maxSeqByKey / QueryMaxSeqByKey (member, owner-or-platform)",
			QueryMaxSeqByKey + OwnerOrPlatformScopeClause,
			[]any{"watchdog.suppressed", "alice"},
			"run_events_type_",
		},
		{
			"driftCards / QueryDriftCards (operator) — the generated column + its index",
			QueryDriftCards + OrderByEventSeq,
			[]any{"2020-01-01T00:00:00Z"},
			"run_events_",
		},
		{
			"driftCards / QueryDriftCards (member, owner-scoped)",
			QueryDriftCards + OwnerScopeClause + OrderByEventSeq,
			[]any{"2020-01-01T00:00:00Z", "alice"},
			"run_events_",
		},
		{
			// Served by 0001's run_events_run_idx, NOT by a 0015 addition: that
			// partial index already leads with run_id and the type set is a small
			// residual filter within one run's rows.
			"per-run derive / QueryLatestTypeForRun (2 types)",
			QueryLatestTypeForRun(2),
			[]any{"r1", "run.wedged", "run.state_changed"},
			"run_events_run",
		},
		{
			"per-run derive / QueryLatestTypeForRun (1 type)",
			QueryLatestTypeForRun(1),
			[]any{"r1", "run.state_changed"},
			"run_events_run",
		},
	}

	for _, c := range cases {
		detail := planFor(t, db, c.query, c.args...)
		if !strings.Contains(detail, "USING INDEX") && !strings.Contains(detail, "USING COVERING INDEX") {
			t.Errorf("%s still full-scans run_events (the F16 carry):\n%s", c.name, detail)
		}
		if !strings.Contains(detail, c.index) {
			t.Errorf("%s is served by a different index than documented (want one matching %q):\n%s",
				c.name, c.index, detail)
		}
	}

	// Non-tautology probe: a query no index serves DOES report a scan, so the
	// assertions above are capable of failing.
	if detail := planFor(t, db, `SELECT event_seq FROM run_events WHERE schema_version = 1`); strings.Contains(detail, "USING INDEX") {
		t.Fatalf("the query-plan assertion is tautological — an unindexed column reported an index:\n%s", detail)
	}
}

// TestDottedOrderPrefixScanIsIndexed is drain D5: dotted_order is advertised as
// the ordered-tree path, so its prefix scan must SEEK. An unindexed generated
// column is a write cost with no read benefit.
func TestDottedOrderPrefixScanIsIndexed(t *testing.T) {
	db, log, runs := wdTestDB(t)
	seedIndexFixture(t, log, runs)
	detail := planFor(t, db,
		`SELECT event_seq FROM run_events WHERE dotted_order >= ? AND dotted_order < ? ORDER BY dotted_order`,
		"r1.", "r1/")
	if !strings.Contains(detail, "run_events_dotted_order_idx") {
		t.Errorf("the ordered-tree prefix scan is not served by run_events_dotted_order_idx:\n%s", detail)
	}
	if strings.Contains(detail, "SCAN run_events\n") {
		t.Errorf("the ordered-tree prefix scan is a full table scan:\n%s", detail)
	}
}

// TestChangeClassColumnIsReachableFromProduction is drain D6: an index nothing
// reads is a write cost paid on every INSERT for nothing. The production drift
// query must name the COLUMN, not re-evaluate the json_extract expression.
func TestChangeClassColumnIsReachableFromProduction(t *testing.T) {
	if strings.Contains(QueryDriftCards, "json_extract") {
		t.Error("QueryDriftCards still re-evaluates json_extract; the generated column exists so it does not have to")
	}
	if !strings.Contains(QueryDriftCards, "change_class") {
		t.Fatal("QueryDriftCards does not read the change_class column at all")
	}
	// The COALESCE is load-bearing and must survive the rewrite: a row whose
	// payload names no change class must not be treated as class 'none'.
	if !strings.Contains(QueryDriftCards, "COALESCE(change_class, '')") {
		t.Error("the COALESCE guard was dropped from QueryDriftCards")
	}
	// And the production projection actually runs it.
	db, log, runs := wdTestDB(t)
	seedIndexFixture(t, log, runs)
	p := &projector{db: db}
	cards, err := p.driftCards(context.Background(), ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("driftCards over the generated column: %v", err)
	}
	if len(cards) == 0 {
		t.Error("driftCards returned nothing over the seeded findings — the column read must be equivalent to the expression it replaced")
	}
}
