package history_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// layer2_test.go — the guardrail stack END TO END, against a real migrated
// database, on the real read-only handle, with the real duty path.
//
// TIER F throughout: a local.FakeServer + a real local.Duty stands in for the
// Arctic seat, which is exactly the right instrument — the point of these tests
// is that WHATEVER the seat emits dies at the guardrail, so the seat's actual
// competence is irrelevant to them. The live-seat leg is a bring-up act
// (SANCTIONED SKIP, CONVENTIONS §10); nothing here dials one. $0.

type sqlStack struct {
	*fixture
	fake *local.FakeServer
	ro   *storage.ReadOnly
}

func newOpenSQLStack(t *testing.T) *sqlStack {
	t.Helper()
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)

	f := newFixture(t)
	duty := local.NewDuty(local.DutyDeps{
		Registry:    local.NewRegistry(f.reg),
		Client:      local.NewClient(fake.URL),
		Checkpoints: gates.NewCheckpoints(f.db, f.log),
		Events:      f.log,
	})
	seat, err := duty.ResolveSeat(local.AliasSQLOpen)
	if err != nil {
		t.Fatalf("resolve the %s seat: %v", local.AliasSQLOpen, err)
	}
	if !seat.Servable {
		t.Skipf("SANCTIONED SKIP: the %s seat is not servable in this manifest (%s)", local.AliasSQLOpen, seat.Note)
	}
	ro, err := f.db.OpenReadOnly(f.ctx)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })

	runs := run.NewStore(f.db, f.log)
	advisory := func(ctx context.Context, label string) (string, func()) {
		id := "platform.advisory." + label + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
		if _, err := runs.Create(ctx, run.NewRun{
			ID: id, UserID: run.ActorPlatform, Lane: local.LaneLocal, Substrate: "local",
		}); err != nil {
			return "", nil
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				return "", nil
			}
		}
		return id, func() {
			_, _ = runs.Transition(context.WithoutCancel(ctx), id, run.StateCompleted,
				run.TransitionOptions{Actor: run.ActorPlatform})
		}
	}
	st, err := history.New(history.Config{
		DB: f.db, ReadOnly: ro, Log: f.log, Duty: duty, Advisory: advisory,
		Now: func() time.Time { return f.now },
	})
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	f.st = st
	return &sqlStack{fixture: f, fake: fake, ro: ro}
}

// emits sets what the "seat" returns for the next generation.
func (s *sqlStack) emits(content string) {
	s.fake.SetResponse(local.FakeResponse{Content: content, InputTokens: 40, OutputTokens: 60})
}

// auditRows reads every Layer-2 audit record, newest last.
func (s *sqlStack) auditRows(t *testing.T) []history.OpenSQLAudit {
	t.Helper()
	rows, err := s.db.QueryContext(s.ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq`, history.EventQueryAudited)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []history.OpenSQLAudit
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var rec history.OpenSQLAudit
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			t.Fatalf("an audit payload did not unmarshal: %v", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestOpenSQLEndToEnd — a realistic reasoning-model answer runs the whole
// stack: extracted, guarded, scoped, bounded, executed on the read-only
// handle, flagged lower-confidence, audited.
func TestOpenSQLEndToEnd(t *testing.T) {
	s := newOpenSQLStack(t)
	seedTwoOwners(t, s.fixture)
	s.emits(arcticCoT)

	a, err := s.st.AskOpenSQL(s.ctx, "which runs cost the most?", opScope(), 20)
	if err != nil {
		t.Fatalf("AskOpenSQL: %v", err)
	}
	if a.Layer != history.LayerOpenSQL {
		t.Errorf("layer = %d, want %d", a.Layer, history.LayerOpenSQL)
	}
	if a.Confidence != history.ConfidenceLower {
		t.Errorf("confidence = %q, want %q — every Layer-2 answer is flagged lower-confidence",
			a.Confidence, history.ConfidenceLower)
	}
	if a.Query != history.OpenSQLQueryName {
		t.Errorf("query name = %q, want %q — every answer names its query", a.Query, history.OpenSQLQueryName)
	}
	if len(a.Rows) == 0 {
		t.Error("the generated statement returned no rows over a seeded database")
	}
	if a.Audit == nil {
		t.Fatal("the answer carries no audit record")
	}
	if a.Audit.Outcome != history.OutcomeExecuted {
		t.Errorf("outcome = %q, want %q", a.Audit.Outcome, history.OutcomeExecuted)
	}
	if a.Audit.LimitInjected <= 0 {
		t.Error("no row bound was injected")
	}
	if a.Audit.TimeoutMS != history.OpenSQLTimeout.Milliseconds() {
		t.Errorf("timeout recorded as %d ms, want %d", a.Audit.TimeoutMS, history.OpenSQLTimeout.Milliseconds())
	}
	if !strings.Contains(a.Audit.SQLExecuted, "LIMIT ?") {
		t.Errorf("the executed statement carries no injected bound: %s", a.Audit.SQLExecuted)
	}

	rows := s.auditRows(t)
	if len(rows) != 1 {
		t.Fatalf("%d audit rows, want exactly 1", len(rows))
	}
	if rows[0].Question != "which runs cost the most?" {
		t.Errorf("the audit row does not carry the question: %q", rows[0].Question)
	}
	if rows[0].SQLGenerated == "" {
		t.Error("the audit row does not carry the generated statement")
	}
}

// TestEveryLayer2AttemptIsAudited — R33, including the refusals. A guardrail
// whose refusals are not recorded cannot be reviewed.
func TestEveryLayer2AttemptIsAudited(t *testing.T) {
	s := newOpenSQLStack(t)
	seedTwoOwners(t, s.fixture)

	outputs := []string{
		arcticCoT,                        // executes
		"SELECT payload FROM run_events", // refused: allowlist
		"SELECT * FROM cost_per_run; DROP TABLE runs",  // refused: multi-statement
		"I cannot answer that question.",               // no SQL
		"SELECT * FROM cost_per_run WHERE user_id = ?", // refused: bind parameter
	}
	for _, out := range outputs {
		s.emits(out)
		_, _ = s.st.AskOpenSQL(s.ctx, "q: "+out[:min(20, len(out))], opScope(), 10)
	}

	rows := s.auditRows(t)
	if len(rows) != len(outputs) {
		t.Fatalf("%d audit rows for %d attempts — every attempt is audited", len(rows), len(outputs))
	}
	wantOutcomes := []string{
		history.OutcomeExecuted, history.OutcomeRefused, history.OutcomeRefused,
		history.OutcomeNoSQL, history.OutcomeRefused,
	}
	for i, rec := range rows {
		if rec.Outcome != wantOutcomes[i] {
			t.Errorf("attempt %d recorded outcome %q, want %q", i, rec.Outcome, wantOutcomes[i])
		}
		if rec.Outcome != history.OutcomeExecuted && rec.Refusal == "" {
			t.Errorf("attempt %d was not executed but records no reason", i)
		}
		if rec.Alias != local.AliasSQLOpen {
			t.Errorf("attempt %d records alias %q, want %q", i, rec.Alias, local.AliasSQLOpen)
		}
	}
}

// TestOpenSQLOwnerScopeIsEnforcedCrossOwner — the read-only handle can see the
// whole database, so S01.9 scoping has to be a property of the STATEMENT. A
// member's generated SQL never returns another owner's rows, even when the
// model asks for all of them.
func TestOpenSQLOwnerScopeIsEnforcedCrossOwner(t *testing.T) {
	s := newOpenSQLStack(t)
	seedTwoOwners(t, s.fixture)

	// The model deliberately writes an UNSCOPED statement: no predicate, no
	// filter, "give me everything". The guardrail is what makes it scoped.
	const greedy = "```sql\nSELECT run_id, user_id FROM cost_per_run\n```"

	owners := func(a history.Answer) map[string]bool {
		got := map[string]bool{}
		for i, c := range a.Columns {
			if c != "user_id" {
				continue
			}
			for _, r := range a.Rows {
				got[str(r[i])] = true
			}
		}
		return got
	}

	s.emits(greedy)
	op, err := s.st.AskOpenSQL(s.ctx, "every run", opScope(), 100)
	if err != nil {
		t.Fatalf("operator: %v", err)
	}
	seen := owners(op)
	if !seen[member1] || !seen[member2] {
		t.Fatalf("the operator saw %v, want both owners — the fixture would not be discriminating", seen)
	}

	s.emits(greedy)
	alice, err := s.st.AskOpenSQL(s.ctx, "every run", memberScope(member1), 100)
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	seen = owners(alice)
	if seen[member2] {
		t.Error("alice's open-SQL answer returned bob's rows (S01.9)")
	}
	if !seen[member1] {
		t.Error("alice's open-SQL answer returned none of her own rows")
	}

	s.emits(greedy)
	bob, err := s.st.AskOpenSQL(s.ctx, "every run", memberScope(member2), 100)
	if err != nil {
		t.Fatalf("bob: %v", err)
	}
	if seen := owners(bob); seen[member1] {
		t.Error("bob's open-SQL answer returned alice's rows (S01.9)")
	}
	// And the scoping is recorded, so an unscoped answer would be visible in
	// the audit trail rather than only in the rows.
	if bob.Audit == nil || bob.Audit.OwnerScoped != 1 {
		t.Errorf("the audit record does not show the owner predicate being applied: %+v", bob.Audit)
	}
}

// TestOpenSQLBoundIsInjectedAndObserved — the row bound applies to a statement
// that asked for everything, and truncation is observed rather than inferred.
func TestOpenSQLBoundIsInjectedAndObserved(t *testing.T) {
	s := newOpenSQLStack(t)
	s.user(member1, "member")
	s.task("t1", member1, "Task", "doing")
	for i := 0; i < 12; i++ {
		id := "r" + strconv.Itoa(i)
		s.run(id, member1, "t1", "completed", "anthropic")
		s.receipt(id, member1, float64(i), 1, 0)
	}
	s.emits("```sql\nSELECT run_id FROM cost_per_run\n```")

	a, err := s.st.AskOpenSQL(s.ctx, "all runs", opScope(), 5)
	if err != nil {
		t.Fatalf("AskOpenSQL: %v", err)
	}
	if len(a.Rows) != 5 {
		t.Errorf("bound 5 returned %d rows", len(a.Rows))
	}
	if !a.Truncated {
		t.Error("a truncated Layer-2 answer did not say so")
	}
	// A caller asking for more than the Layer-2 ceiling is clamped to it.
	s.emits("```sql\nSELECT run_id FROM cost_per_run\n```")
	big, err := s.st.AskOpenSQL(s.ctx, "all runs", opScope(), history.OpenSQLMaxRows*100)
	if err != nil {
		t.Fatal(err)
	}
	if big.Audit.LimitInjected != history.OpenSQLMaxRows {
		t.Errorf("bound = %d, want the ceiling %d", big.Audit.LimitInjected, history.OpenSQLMaxRows)
	}
}

// TestOpenSQLTimeoutBoundsARunawayStatement — MEASURED: a runaway recursive
// query never returns, and the row bound does NOT stop it (the aggregate never
// yields a row to count). Only the wall clock does.
func TestOpenSQLTimeoutBoundsARunawayStatement(t *testing.T) {
	s := newOpenSQLStack(t)
	seedTwoOwners(t, s.fixture)
	// Reads an allowlisted view, so it passes every other limb, and runs away.
	s.emits("```sql\nWITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c)\n" +
		"SELECT count(*) FROM c, cost_per_run\n```")

	start := time.Now()
	_, err := s.st.AskOpenSQL(s.ctx, "count everything forever", opScope(), 10)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("a runaway statement returned successfully")
	}
	if elapsed > 3*history.OpenSQLTimeout {
		t.Errorf("the runaway ran for %s — the deadline did not bound it", elapsed)
	}
	rows := s.auditRows(t)
	if len(rows) != 1 {
		t.Fatalf("%d audit rows, want 1 — a timed-out query is still audited", len(rows))
	}
	if rows[0].Outcome != history.OutcomeFailed {
		t.Errorf("outcome = %q, want %q", rows[0].Outcome, history.OutcomeFailed)
	}
}

// TestTheWholeBatteryChangesNothing — the S04.5/S03.5 shape at the database
// level: run every hostile output through the REAL verb against a real
// database, then assert the database is exactly as it was. The audit rows are
// the only permitted difference, and they are counted.
func TestTheWholeBatteryChangesNothing(t *testing.T) {
	s := newOpenSQLStack(t)
	seedTwoOwners(t, s.fixture)

	before := s.snapshot(t)
	attempts := injectionAttempts()
	for _, at := range attempts {
		s.emits(at.raw)
		a, err := s.st.AskOpenSQL(s.ctx, at.name, opScope(), 10)
		if err == nil && (at.wants != "" || at.noSQL) {
			t.Errorf("%s: the hostile output was ACCEPTED and executed as %q", at.name, a.Audit.SQLExecuted)
		}
	}
	after := s.snapshot(t)
	// Three tables are EXPECTED to grow, each by a known amount, and asserting
	// the amount is stronger than excusing the table:
	//   run_events  — the audit row per attempt, plus the advisory run's own
	//                 lifecycle events (S12.1 R18: every duty call meters).
	//   runs        — one advisory run per generation.
	//   checkpoints — one D7 usage checkpoint per generation.
	// Everything else must be untouched.
	expectedGrowth := map[string]int{"runs": len(attempts), "checkpoints": len(attempts)}
	for table, n := range before {
		if table == "run_events" {
			continue
		}
		if want, grows := expectedGrowth[table]; grows {
			if after[table]-n != want {
				t.Errorf("%s grew by %d, want exactly %d (one per metered generation)", table, after[table]-n, want)
			}
			continue
		}
		if after[table] != n {
			t.Errorf("%s went from %d rows to %d — the battery changed the database", table, n, after[table])
		}
	}
	if len(after) != len(before) {
		t.Errorf("the schema gained or lost objects: %d → %d tables", len(before), len(after))
	}
	// Every attempt is on the record.
	if got := len(s.auditRows(t)); got != len(attempts) {
		t.Errorf("%d audit rows for %d attempts", got, len(attempts))
	}
}

// snapshot counts rows in every table, so a write anywhere shows up.
func (s *sqlStack) snapshot(t *testing.T) map[string]int {
	t.Helper()
	rows, err := s.db.QueryContext(s.ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	rows.Close()
	out := map[string]int{}
	for _, n := range names {
		var c int
		if err := s.db.QueryRowContext(s.ctx, `SELECT count(*) FROM "`+n+`"`).Scan(&c); err != nil {
			t.Fatalf("count %s: %v", n, err)
		}
		out[n] = c
	}
	return out
}

// TestLayer2IsUnavailableHonestly — no read-only handle means no Layer 2, and
// it says so. Layers 0 and 1 are unaffected, which is the whole point of the
// floor being below it.
func TestLayer2IsUnavailableHonestly(t *testing.T) {
	f := newFixture(t) // no ReadOnly, no Duty
	seedTwoOwners(t, f)
	a, err := f.st.AskOpenSQL(f.ctx, "anything", opScope(), 10)
	if !errors.Is(err, history.ErrOpenSQLUnavailable) {
		t.Fatalf("AskOpenSQL without a handle: %v, want ErrOpenSQLUnavailable", err)
	}
	if a.Confidence != history.ConfidenceLower {
		t.Errorf("even the unavailable answer must carry the lower-confidence flag, got %q", a.Confidence)
	}
	if a.Audit == nil || a.Audit.Outcome != history.OutcomeUnavailable {
		t.Errorf("the unavailable path was not audited: %+v", a.Audit)
	}
	// The floor still answers.
	if _, err := f.st.SelectView(f.ctx, history.ViewCostPerRun, opScope(), 10); err != nil {
		t.Errorf("Layer 0 stopped working because Layer 2 is absent: %v", err)
	}
}
