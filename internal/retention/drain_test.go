package retention_test

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/redact"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// drain_test.go — P3-B5-8A drain round 1. Each test is the evaluator's own
// probe, kept as a standing assertion so the finding cannot return.

// ── D1: the keep-forever boundary is enforced at the DB layer ───────────────

// TestKeepForeverBodiesCannotBeElidedByAnyWriter is the F1 probe. Before the
// drain the narrowed trigger consulted only the marker, the age and the
// identity columns — so one raw UPDATE elided three keep-forever bodies past
// the pass entirely. The allowlist is now a limb of the trigger's own WHEN.
func TestKeepForeverBodiesCannotBeElidedByAnyWriter(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")

	// One row per keep-forever class the evaluator elided, all old enough and
	// bulky enough that ONLY the keep-forever limb can refuse them.
	planted := map[string]int64{}
	for _, typ := range []string{
		"verdict.recorded", "routing.decided", "benchmark.pair_recorded",
		"decision.recorded", "usage.recorded", "drift.finding", "run.summary_written",
	} {
		planted[typ] = f.appendAt("", "alice", typ,
			bulky(`{"keep":"`+typ+`-BODY"}`), f.now.AddDate(0, -24, 0))
	}

	exec := func(q string, args ...any) error {
		return f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(f.ctx, q, args...)
			return err
		})
	}

	// (a) single-row elision, one class at a time.
	for typ, seq := range planted {
		err := exec(`UPDATE run_events SET payload = ? WHERE event_seq = ?`, retention.CompactedPayload, seq)
		if err == nil {
			t.Errorf("%s: a keep-forever body was elided by a single-row UPDATE", typ)
			continue
		}
		if !strings.Contains(err.Error(), "keep-forever") {
			t.Errorf("%s: refusal must name the keep-forever boundary; got %v", typ, err)
		}
	}

	// (b) BULK elision — one statement, no WHERE, the shape that did the damage.
	if err := exec(`UPDATE run_events SET payload = ?`, retention.CompactedPayload); err == nil {
		t.Error("a bulk UPDATE elided keep-forever bodies; the boundary must hold against a writer with no WHERE clause")
	}

	// (c) every body survived, byte-for-byte.
	for typ, seq := range planted {
		if got := f.payloadAt(seq); !strings.Contains(got, typ+"-BODY") {
			t.Errorf("%s: body was lost (payload now %q)", typ, truncate(got))
		}
	}

	// (d) the legacy alias of a keep-forever type is covered too (§29 R14): a
	// pre-rename row is the same record and must not be elided for its old name.
	aliasSeq := f.appendAt("", "alice", "verify.round",
		bulky(`{"keep":"ALIAS-BODY"}`), f.now.AddDate(0, -24, 0))
	if err := exec(`UPDATE run_events SET payload = ? WHERE event_seq = ?`,
		retention.CompactedPayload, aliasSeq); err == nil {
		t.Error("a pre-rename verdict row (verify.round) was elided; the alias is allowlisted")
	}

	// (e) the pass agrees with the trigger — it strips nothing here.
	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if res.EventsStripped != 0 {
		t.Errorf("the pass stripped %d keep-forever rows; the predicate and the trigger must agree", res.EventsStripped)
	}
}

// triggerFloorExpr extracts the horizon-floor expression from migration 0015's
// run_events_payload_compaction_only trigger, so the ordering assertion is
// pinned to the SHIPPED text (the §35 registered_test source-scan precedent).
func triggerFloorExpr(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "storage", "migrations", "0015_retention_history.sql"))
	if err != nil {
		t.Fatal(err)
	}
	const marker = "OR OLD.ts > "
	i := strings.Index(string(body), marker)
	if i < 0 {
		t.Fatal("migration 0015 no longer carries the horizon-floor comparison")
	}
	rest := string(body)[i+len(marker):]
	j := strings.IndexByte(rest, '\n')
	if j < 0 {
		t.Fatal("the horizon-floor expression is unterminated")
	}
	expr := strings.TrimSpace(rest[:j])
	if !strings.Contains(expr, "strftime") || !strings.Contains(expr, "minute") {
		t.Fatalf("extracted floor expression looks wrong: %q", expr)
	}
	return expr
}

func truncate(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// ── D3: only BULKY payloads are stripped ────────────────────────────────────

// TestSmallAuditPayloadsSurvivePastTheHorizon is the F3 probe: the evaluator's
// four small payloads (140 B total) became 412 B of markers — content destroyed
// AND the database grown. S14.9 says "BULKY event payloads"; the qualifier is
// the fix.
func TestSmallAuditPayloadsSurvivePastTheHorizon(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")

	small := map[string]int64{
		"auth.login":       f.appendAt("", "alice", "auth.login", `{"kind":"pin","user":"alice"}`, f.now.AddDate(0, -24, 0)),
		"settings.changed": f.appendAt("", "alice", "settings.changed", `{"key":"obs.sse_keepalive","old":20,"new":25}`, f.now.AddDate(0, -24, 0)),
		"run.state_changed": f.appendAt("", "alice", "run.state_changed",
			`{"from":"running","to":"completed"}`, f.now.AddDate(0, -24, 0)),
		"platform.started": f.appendAt("", "alice", "platform.started", `{"version":"v0"}`, f.now.AddDate(0, -24, 0)),
	}
	big := f.appendAt("", "alice", "engine.message", bulky(`{"text":"a long trace excerpt"}`), f.now.AddDate(0, -24, 0))

	before := payloadBytes(t, f, false)
	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	after := payloadBytes(t, f, false)
	afterAll := payloadBytes(t, f, true)

	if res.EventsStripped != 1 {
		t.Errorf("events stripped = %d, want 1 (only the bulky row)", res.EventsStripped)
	}
	for typ, seq := range small {
		got := f.payloadAt(seq)
		if got == retention.CompactedPayload {
			t.Errorf("%s (%d B) was elided — it is the audit trail, not the bulk", typ, len(got))
		}
	}
	if f.payloadAt(big) != retention.CompactedPayload {
		t.Error("the bulky payload was NOT stripped")
	}

	// BytesReclaimed is what the STRIP actually reclaimed, to the byte.
	if res.BytesReclaimed <= 0 {
		t.Errorf("BytesReclaimed = %d, want a real positive reclaim", res.BytesReclaimed)
	}
	if got := before - after; got != res.BytesReclaimed {
		t.Errorf("BytesReclaimed = %d but the compacted rows shed %d bytes — the figure must be honest by construction",
			res.BytesReclaimed, got)
	}
	// And the pass never GROWS the log on net, audit row included. (Before the
	// bulk floor this was false: 140 B of small payloads became 412 B of markers.)
	if afterAll >= before {
		t.Errorf("total payload bytes went %d -> %d including the pass's own audit row; a pass must never grow the database", before, afterAll)
	}
}

// TestBulkFloorExceedsTheMarker is the arithmetic that makes the reclaim honest
// BY CONSTRUCTION: no threshold below the marker's own length can ever reclaim.
func TestBulkFloorExceedsTheMarker(t *testing.T) {
	if retention.BulkPayloadFloorBytes <= len(retention.CompactedPayload) {
		t.Fatalf("BulkPayloadFloorBytes = %d must exceed the %d-byte marker, or a strip GROWS the database",
			retention.BulkPayloadFloorBytes, len(retention.CompactedPayload))
	}
}

// payloadBytes sums run_events payload bytes. inclAudit=false excludes the
// pass's OWN retention.compacted rows, which are what BytesReclaimed measures
// against — the audit record is a cost of auditing, never "growth from a strip".
func payloadBytes(t *testing.T, f *fixture, inclAudit bool) int64 {
	t.Helper()
	q := `SELECT sum(length(payload)) FROM run_events`
	var args []any
	if !inclAudit {
		q += ` WHERE type <> ?`
		args = append(args, retention.EventCompacted)
	}
	var n sql.NullInt64
	if err := f.db.QueryRowContext(f.ctx, q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n.Int64
}

// ── D4: the search index is not a plaintext-secret oracle ───────────────────

// TestSearchIsNotAPlaintextSecretOracle is the F4 probe: a planted
// `api_key=sk-ORACLE-…` was findable verbatim through Search. The corpus is now
// redacted at index time, so the plaintext matches nothing.
func TestSearchIsNotAPlaintextSecretOracle(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")

	const secret = "sk-ORACLE0123456789abcdefghij"
	f.appendAt("", "alice", "drift.finding",
		`{"source":"probe","summary":"leaked API_KEY=`+secret+` in the fetch log"}`, f.now)
	f.appendAt("", "alice", "verdict.recorded",
		`{"round":1,"verdict":"fail","findings":[{"note":"the agent printed `+secret+` to stdout"}]}`, f.now)

	if _, err := f.store.Index(f.ctx); err != nil {
		t.Fatal(err)
	}

	// The oracle question, asked every way FTS5 allows.
	for _, q := range []string{secret, `"` + secret + `"`, "sk-ORACLE0123456789abcdefghij"} {
		hits, err := f.store.Search(f.ctx, q, "", 10)
		if err != nil {
			// A tokenizer rejection is also "no confirmation" — but a clean zero
			// is what we want, so only a HIT is a failure.
			continue
		}
		if len(hits) != 0 {
			t.Errorf("Search(%q) returned %d hits — the index confirms a plaintext secret", q, len(hits))
		}
	}

	// The raw secret is nowhere in the FTS corpus at all.
	rows, err := f.db.QueryContext(f.ctx, `SELECT body FROM history_fts`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	indexed := 0
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		indexed++
		if strings.Contains(body, secret) {
			t.Errorf("the raw secret is stored in history_fts: %q", truncate(body))
		}
		if !strings.Contains(body, "[REDACTED:") {
			t.Errorf("indexed body carries no redaction marker: %q", truncate(body))
		}
	}
	if indexed != 2 {
		t.Fatalf("indexed %d rows, want 2", indexed)
	}

	// The REDACTED form is findable — redaction scrubs the secret, it does not
	// blind the search.
	if hits, err := f.store.Search(f.ctx, "REDACTED", "", 10); err != nil || len(hits) != 2 {
		t.Errorf("Search(REDACTED) = %d hits (%v), want both redacted rows findable", len(hits), err)
	}
	// Honest content is untouched.
	for _, q := range []string{"stdout", "fetch"} {
		if hits, err := f.store.Search(f.ctx, q, "", 10); err != nil || len(hits) == 0 {
			t.Errorf("Search(%q) = %d hits (%v); honest content must stay findable", q, len(hits), err)
		}
	}

	// STORE-RAW / SERVE-REDACTED (§30 R19): run_events.payload is NOT mutated.
	var raw int
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT count(*) FROM run_events WHERE payload LIKE ?`, "%"+secret+"%").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != 2 {
		t.Errorf("%d raw rows still carry the secret, want 2 — redaction is a property of the derived index, never of the log", raw)
	}
}

// TestRedactionMarkerIsTheRealPrimitive pins the corpus scrub to internal/redact
// rather than to a local re-implementation.
func TestRedactionMarkerIsTheRealPrimitive(t *testing.T) {
	const secret = "sk-ant-api03-ORACLE0123456789abcdef"
	if got := redact.Redact("token " + secret); strings.Contains(got, secret) {
		t.Fatalf("internal/redact no longer catches the Anthropic key class: %q", got)
	}
}

// ── D8: the summary carries counts, never trace-derived free text ───────────

// TestSummaryCarriesNoToolNamesOrFreeTextReason is the F8 probe: the run summary
// is keep-forever, so it LEAVES the host in every snapshot while the trace it
// was computed from does not. Tool names and the unbounded transition `reason`
// were riding across that boundary.
func TestSummaryCarriesNoToolNamesOrFreeTextReason(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.task("t1", "alice", "Ship it")
	r := f.startRun("r1", "alice", "t1")

	// The evaluator's canaries, in the two fields that carried them.
	if _, err := f.log.Append(f.ctx, mkAppend(r, "tool.completed",
		`{"tool":"TOOLNAME-CANARY","args_digest":"a1"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{
		Reason: "REASON-CANARY: an unbounded free-text explanation",
	}); err != nil {
		t.Fatal(err)
	}

	sum, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	// The counts survive — the information S14.9 ¶1 actually asks for.
	if sum.Aggregate.ToolCalls.Total != 1 || sum.Aggregate.ToolCalls.Distinct != 1 {
		t.Errorf("tool calls = %+v, want the COUNTS preserved", sum.Aggregate.ToolCalls)
	}
	// The stages are enumerated FSM states, both ends.
	last := sum.Aggregate.Stages[len(sum.Aggregate.Stages)-1]
	if last.Name != "completed" || last.From != "running" {
		t.Errorf("last stage = %+v, want the enumerated running -> completed", last)
	}

	// Neither canary is anywhere in the stored aggregate…
	var stored string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT aggregate_json FROM run_summaries WHERE run_id = 'r1'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"TOOLNAME-CANARY", "REASON-CANARY"} {
		if strings.Contains(stored, canary) {
			t.Errorf("%s is stored in the run summary — it would cross the 11.3 export boundary the trace does not", canary)
		}
	}

	// …nor in the snapshot dump.
	vac := filepath.Join(t.TempDir(), "vac.db")
	if err := f.db.VacuumInto(f.ctx, vac); err != nil {
		t.Fatal(err)
	}
	dump, _, err := storage.DumpFrom(f.ctx, vac)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"TOOLNAME-CANARY", "REASON-CANARY"} {
		if strings.Contains(dump, canary) {
			t.Errorf("%s reached the snapshot dump through the run summary", canary)
		}
	}
}

// TestSummaryFieldsAreRedactedAndBounded: anything that DOES remain is scrubbed
// and bounded, so no unbounded field crosses the boundary.
func TestSummaryFieldsAreRedactedAndBounded(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	long := strings.Repeat("L", 4096)
	f.task("t1", "alice", "sk-ant-api03-TITLESECRET0123456789 "+long)
	r := f.startRun("r1", "alice", "t1")
	if _, err := f.log.Append(f.ctx, mkAppend(r, "verdict.recorded",
		`{"round":1,"verdict":"sk-ant-api03-VERDICTSECRET0123456789","rubric_id":"`+long+`"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	sum, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sum.Aggregate.Objective, "TITLESECRET") {
		t.Error("the objective carries an unredacted secret")
	}
	if len([]rune(sum.Aggregate.Objective)) > 201 {
		t.Errorf("the objective is %d runes — unbounded fields must not cross the boundary", len([]rune(sum.Aggregate.Objective)))
	}
	v := sum.Aggregate.Verdicts[0]
	if strings.Contains(v.Verdict, "VERDICTSECRET") {
		t.Error("a verdict label carries an unredacted secret")
	}
	if len([]rune(v.RubricID)) > 97 {
		t.Errorf("a rubric id is %d runes — labels are bounded", len([]rune(v.RubricID)))
	}
}

// TestUnknownFSMStateIsDroppedNotCarried: the enumerated-state rule is a real
// filter, not a rename. A hostile or malformed `to` value carries nothing.
func TestUnknownFSMStateIsDroppedNotCarried(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	r := f.startRun("r1", "alice", "")
	// A forged run.state_changed with a non-FSM `to` value.
	if _, err := f.log.Append(f.ctx, mkAppend(r, "run.state_changed",
		`{"from":"running","to":"INJECTED-STATE-CANARY"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	sum, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range sum.Aggregate.Stages {
		if strings.Contains(st.Name, "INJECTED") || strings.Contains(st.From, "INJECTED") {
			t.Errorf("an unrecognized state value was carried into the summary: %+v", st)
		}
	}
}

// ── D9: the boundary and the trigger floor share a clock domain ─────────────

// TestHorizonOneIsNotRefusedByTheTriggerFloor is the F9 probe at its tightest
// point, recalibrated at drain r2 (R2).
//
// The round-1 version planted its row two minutes past the boundary — OUTSIDE
// the one-minute slack band — so it passed under the WRONG sign, under no slack
// and under the right sign alike, and could not catch the R1 regression it
// existed to guard. The row now sits ONE SECOND past the boundary, inside the
// band, so the assertion is sensitive to the sign: with the floor a minute
// EARLIER than the boundary this row is selected by the pass and refused by the
// trigger, and D2's unit atomicity rolls the whole batch back.
func TestHorizonOneIsNotRefusedByTheTriggerFloor(t *testing.T) {
	f := newFixture(t)
	f.user("edge", "member")
	f.setHorizon("edge", 1)
	// One second past the boundary: old enough for the pass to select, and well
	// inside the minute of slack the trigger floor carries.
	seq := f.appendAt("", "edge", "engine.message",
		bulky(`{"text":"edge"}`), f.now.AddDate(0, -1, 0).Add(-time.Second))

	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatalf("the pass failed at the clamp minimum: %v", err)
	}
	if len(res.FailedOwners) != 0 {
		t.Fatalf("owner legs failed at horizon = 1: %v (%+v)", res.FailedOwners, res.Owners)
	}
	if res.EventsStripped != 1 {
		t.Errorf("events stripped = %d, want 1 — the trigger floor must never refuse a legitimate strip", res.EventsStripped)
	}
	if f.payloadAt(seq) != retention.CompactedPayload {
		t.Error("the row one second past a 1-month horizon was not stripped")
	}
}

// TestTriggerFloorIsNeverEarlierThanAnyBoundary states the R1 guarantee as the
// ORDERING it actually is, measured directly rather than inferred from a strip.
//
// The trigger permits a strip iff OLD.ts <= floor; the pass selects rows with
// ts <= boundary. "A legitimate strip is never refused" is therefore exactly
// `boundary <= floor` for every boundary the pass can compute, and the tightest
// case is the clamp minimum (1 month), where the two coincide. A minus sign on
// the slack inverts this by exactly the minute it adds.
func TestTriggerFloorIsNeverEarlierThanAnyBoundary(t *testing.T) {
	f := newFixture(t)

	// The floor expression is EXTRACTED FROM THE MIGRATION, not copied here: a
	// test holding its own copy asserts the ordering of an expression that is
	// not the one shipping, which is precisely how the sign regression survived.
	floorExpr := triggerFloorExpr(t)
	var floor string
	if err := f.db.QueryRowContext(f.ctx, `SELECT `+floorExpr).Scan(&floor); err != nil {
		t.Fatalf("evaluate the shipped floor expression %s: %v", floorExpr, err)
	}
	// The boundary at every horizon the clamp admits, starting at its minimum.
	for _, months := range []int64{1, 2, 3, 6, 12, 24} {
		var boundary string
		if err := f.db.QueryRowContext(f.ctx,
			`SELECT strftime('%Y-%m-%dT%H:%M:%SZ', ?, ?)`,
			f.now.Format(time.RFC3339Nano), fmt.Sprintf("-%d months", months)).Scan(&boundary); err != nil {
			t.Fatal(err)
		}
		// Compare on the shared second-precision prefix (the floor carries no Z).
		if boundary[:19] > floor {
			t.Errorf("horizon %d months: boundary %q is LATER than the trigger floor %q — rows in that band are selected by the pass and refused by the trigger",
				months, boundary[:19], floor)
		}
	}

	// The sign is the whole point: the inverted floor must violate the ordering
	// at the clamp minimum, so this assertion is capable of failing.
	var wrongFloor, tightest string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT `+strings.Replace(floorExpr, "'+1 minute'", "'-1 minute'", 1)).Scan(&wrongFloor); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT strftime('%Y-%m-%dT%H:%M:%SZ', ?, '-1 months')`,
		f.now.Format(time.RFC3339Nano)).Scan(&tightest); err != nil {
		t.Fatal(err)
	}
	if tightest[:19] <= wrongFloor {
		t.Fatal("the ordering check is tautological — the inverted slack still satisfies it")
	}
}

// TestCompactionUnitIsAtomicWithItsAuditRow is drain r2 R3: the S14.9 ¶2
// coupling must be IMPOSSIBLE to break silently, not merely correct today.
//
// The evaluator rewrote stripUnit to commit its strips first and append the
// audit row in a follow-up transaction, and the whole suite still passed — the
// happy path cannot see the difference. This fails the audit append INSIDE the
// unit transaction and asserts the elisions went with it: zero markers, zero
// audit rows, every body intact. Decouple the append and this test fails loudly.
func TestCompactionUnitIsAtomicWithItsAuditRow(t *testing.T) {
	boom := errors.New("injected audit-append failure")
	f := newFixture(t, withAuditFault(func() error { return boom }))
	f.user("alice", "member")

	const n = 5
	seqs := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		seqs = append(seqs, f.appendAt("", "alice", "engine.message",
			bulky(`{"text":"BODY-`+itoa(int64(i))+`"}`), f.now.AddDate(0, -24, 0)))
	}

	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatalf("an owner-level fault must not fail the PASS: %v", err)
	}
	if len(res.FailedOwners) != 1 || res.FailedOwners[0] != "alice" {
		t.Errorf("FailedOwners = %v, want [alice]", res.FailedOwners)
	}
	if res.EventsStripped != 0 {
		t.Errorf("the pass reports %d strips although its audit append failed", res.EventsStripped)
	}

	// EVERY body survived — the elisions rolled back with the append.
	for i, seq := range seqs {
		got := f.payloadAt(seq)
		if got == retention.CompactedPayload {
			t.Errorf("row %d was elided although its audit row was never written — the unit is not atomic", i)
		}
		if !strings.Contains(got, "BODY-"+itoa(int64(i))) {
			t.Errorf("row %d body was lost: %q", i, truncate(got))
		}
	}
	// …and no audit row was committed either.
	var audits int
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, retention.EventCompacted).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Errorf("%d retention.compacted rows committed although the append faulted", audits)
	}

	// The guard is not vacuous: with no fault the very same fixture strips and
	// audits normally, so the test above fails for the right reason.
	clean := newFixture(t)
	clean.user("alice", "member")
	cleanSeq := clean.appendAt("", "alice", "engine.message",
		bulky(`{"text":"BODY-clean"}`), clean.now.AddDate(0, -24, 0))
	cleanRes, err := clean.store.Compact(clean.ctx, clean.now)
	if err != nil {
		t.Fatal(err)
	}
	if cleanRes.EventsStripped != 1 || len(cleanRes.AuditEventSeqs) != 1 {
		t.Fatalf("without the fault the pass must strip and audit: %+v", cleanRes)
	}
	if clean.payloadAt(cleanSeq) != retention.CompactedPayload {
		t.Error("the unfaulted pass did not strip")
	}
}

// TestBoundaryIsDerivedInTheDatabaseClockDomain: the boundary comes back in the
// same format the trigger floor compares against, from the same engine.
func TestBoundaryIsDerivedInTheDatabaseClockDomain(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.appendAt("", "alice", "engine.message", bulky(`{"text":"x"}`), f.now.AddDate(0, -24, 0))
	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	leg := ownerLeg(t, res, "alice")
	// strftime('%Y-%m-%dT%H:%M:%SZ', …) — second precision, trailing Z, 20 chars.
	if len(leg.BoundaryTS) != 20 || !strings.HasSuffix(leg.BoundaryTS, "Z") {
		t.Errorf("boundary %q is not the DB strftime form the trigger floor shares", leg.BoundaryTS)
	}
	var dbSide string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT strftime('%Y-%m-%dT%H:%M:%SZ', ?, '-6 months')`,
		f.now.Format(time.RFC3339Nano)).Scan(&dbSide); err != nil {
		t.Fatal(err)
	}
	if leg.BoundaryTS != dbSide {
		t.Errorf("boundary %q != the database's own answer %q", leg.BoundaryTS, dbSide)
	}
}

// ── D10: the pass has durable evidence it ran ───────────────────────────────

// TestPassLivenessIsAReadNotAnInference: a quiet household and a driver
// goroutine that died at boot must be distinguishable without a per-pass event.
func TestPassLivenessIsAReadNotAnInference(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")

	// Before any pass: the stamp says plainly that it has never run.
	st, err := f.store.PassState(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ran || st.RunsTotal != 0 {
		t.Errorf("fresh pass state = %+v, want never-run", st)
	}

	// A pass that compacts NOTHING still stamps — that is the whole point.
	if _, err := f.store.Compact(f.ctx, f.now); err != nil {
		t.Fatal(err)
	}
	st, err = f.store.PassState(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Ran || st.RunsTotal != 1 {
		t.Errorf("after a no-op pass: %+v, want Ran with RunsTotal 1", st)
	}
	if st.LastRunTS.IsZero() {
		t.Error("the stamp carries no last-run time")
	}
	if st.LastError != "" {
		t.Errorf("a clean no-op pass recorded an error: %q", st.LastError)
	}
	// And no event was written for it.
	var events int
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, retention.EventCompacted).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Errorf("a no-op pass wrote %d retention.compacted rows, want 0 (liveness is the stamp, not an event)", events)
	}

	// A pass that DOES compact updates the totals.
	f.appendAt("", "alice", "engine.message", bulky(`{"text":"old"}`), f.now.AddDate(0, -24, 0))
	if _, err := f.store.Compact(f.ctx, f.now); err != nil {
		t.Fatal(err)
	}
	st, err = f.store.PassState(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.RunsTotal != 2 || st.LastEvents != 1 || st.EventsTotal != 1 || st.LastBytes <= 0 {
		t.Errorf("pass state = %+v, want 2 runs / 1 event last / 1 total / positive bytes", st)
	}
}
