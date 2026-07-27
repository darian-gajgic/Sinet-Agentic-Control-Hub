package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// projection_benchmark_test.go — the B5-7 benchmark inbox derivations (S14.7):
// the S15.6 blind-pair verdict form and the BENCH-REG §12 alarm card, both
// DERIVED (from migration 0014's benchmark_pairs table and from the
// benchmark.alarm rows) with internal/api importing nothing from
// internal/benchmark. Internal test (package api) driving the projector
// directly, the drift/watchdog/conformance precedent.

// mustExec writes a fixture row straight into the platform DB: internal/api
// never imports internal/benchmark, so these suites seed the 0014 tables with
// SQL exactly as the production projection reads them.
func mustExec(t *testing.T, p *projector, q string, args ...any) {
	t.Helper()
	if err := p.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), q, args...)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func appendAlarm(t *testing.T, log *eventlog.Log, at time.Time, action, domain string, lossG float64) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"action": action, "domain": domain, "epoch_id": domain + "#e1",
		"severity": "flag-now", "summary": "the platform is losing in " + domain,
		"loss_g": lossG, "threshold": 0.95, "expansion_freeze": action == "raise",
		"retention": "keep-forever",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(context.Background(), eventlog.Append{
		UserID: "platform", Type: "benchmark.alarm", SchemaVersion: 1, Payload: payload, Time: at,
	}); err != nil {
		t.Fatalf("append benchmark.alarm: %v", err)
	}
}

// TestBenchmarkAlarmCardStandsUntilItsDisposition: the card derives from the
// log — a raise stands, a clear (the operator's logged disposition) supersedes
// it — and it is platform-scope, so only the operator sees it.
func TestBenchmarkAlarmCardStandsUntilItsDisposition(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	p := &projector{db: db}
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	appendAlarm(t, log, base, "raise", "software-development", 0.969)
	snap, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.BenchmarkAlarms) != 1 {
		t.Fatalf("%d alarm cards, want 1", len(snap.BenchmarkAlarms))
	}
	card := snap.BenchmarkAlarms[0]
	if card.Severity != "flag-now" {
		t.Errorf("alarm severity = %q, want the flag-now class §12 names", card.Severity)
	}
	if !card.ExpansionFreeze {
		t.Error("the card must carry the standing §12 expansion freeze as data")
	}
	if card.LossG != 0.969 || card.Threshold != 0.95 {
		t.Errorf("the card must carry the trip condition as measured and as registered: %+v", card)
	}

	// A member never sees a platform-scope alarm (S01.9).
	member, err := p.inbox(ctx, ownerScope{UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(member.BenchmarkAlarms) != 0 {
		t.Errorf("a member saw %d platform-scope alarm card(s)", len(member.BenchmarkAlarms))
	}

	// The disposition clears it.
	appendAlarm(t, log, base.Add(time.Hour), "clear", "software-development", 0.969)
	snap, err = p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.BenchmarkAlarms) != 0 {
		t.Errorf("%d alarm cards after the disposition, want 0", len(snap.BenchmarkAlarms))
	}
}

// TestBenchmarkVerdictCardIsScopedToTheRequester: the requester is the VOTER,
// so a member sees their own pending form — one of the few inbox cards that is
// not operator-owned — and the operator sees all.
func TestBenchmarkVerdictCardIsScopedToTheRequester(t *testing.T) {
	ctx := context.Background()
	db, _, _ := wdTestDB(t)
	p := &projector{db: db}

	mustExec(t, p, `INSERT INTO benchmark_pairs
		(pair_id, user_id, domain, task_id, deliverable_id, phase, rate_pct, sampled_ts,
		 state, render_a, render_b, updated_ts)
		VALUES ('bp-1','alice','software-development','t1','t1.d1','pre-gate',100,
		        '2026-08-01T12:00:00Z','rendered','platform text','direct','2026-08-01T12:00:00Z')`)
	mustExec(t, p, `INSERT INTO benchmark_pairs
		(pair_id, user_id, domain, task_id, deliverable_id, phase, rate_pct, sampled_ts,
		 state, render_a, render_b, updated_ts)
		VALUES ('bp-2','bob','software-development','t2','t2.d1','pre-gate',100,
		        '2026-08-01T12:00:00Z','rendered','x','y','2026-08-01T12:00:00Z')`)
	// A pair not yet rendered is not a card: there is nothing to vote on.
	mustExec(t, p, `INSERT INTO benchmark_pairs
		(pair_id, user_id, domain, task_id, deliverable_id, phase, rate_pct, sampled_ts,
		 state, updated_ts)
		VALUES ('bp-3','alice','software-development','t3','t3.d1','pre-gate',100,
		        '2026-08-01T12:00:00Z','sampled','2026-08-01T12:00:00Z')`)

	alice, err := p.inbox(ctx, ownerScope{UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(alice.BenchmarkVerdicts) != 1 || alice.BenchmarkVerdicts[0].PairID != "bp-1" {
		t.Fatalf("alice sees %+v, want only her own pending pair", alice.BenchmarkVerdicts)
	}
	if !alice.BenchmarkVerdicts[0].GuessRequired {
		t.Error("the card must carry the mandatory-guess flag — BENCH-REG §3.3 is frozen")
	}
	if alice.BenchmarkVerdicts[0].LengthA != len("platform text") {
		t.Errorf("the card must carry the §3.2 length confound by side: %+v", alice.BenchmarkVerdicts[0])
	}

	operator, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(operator.BenchmarkVerdicts) != 2 {
		t.Errorf("the operator sees %d pending pairs, want both", len(operator.BenchmarkVerdicts))
	}
}

// TestVerdictCardRevealsNoArmIdentity is the projection half of BENCH-REG §3.4:
// the card type has no arm/position/model field, and the query that fills it
// selects no such column. Asserted structurally so a later field addition trips
// here rather than silently unblinding a live pair.
func TestVerdictCardRevealsNoArmIdentity(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("projection.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "projection.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The struct carries no revealing field.
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "InboxBenchmarkVerdict" {
			return true
		}
		found = true
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, field := range st.Fields.List {
			for _, name := range field.Names {
				lower := strings.ToLower(name.Name)
				for _, leak := range []string{"position", "model", "arm", "platformrun", "directrun"} {
					if strings.Contains(lower, leak) {
						t.Errorf("InboxBenchmarkVerdict.%s could reveal arm identity before the record lands (BENCH-REG §3.4)", name.Name)
					}
				}
			}
		}
		return false
	})
	if !found {
		t.Fatal("InboxBenchmarkVerdict not found — the assertion would be vacuous")
	}

	// The QUERY selects no revealing column either — scanned as the SQL literal
	// itself, so prose about the rule cannot satisfy or trip the rule.
	body := string(src)
	i := strings.Index(body, "func (p *projector) benchmarkVerdicts")
	if i < 0 {
		t.Fatal("benchmarkVerdicts not found")
	}
	start := strings.Index(body[i:], "`")
	if start < 0 {
		t.Fatal("the pending-verdict query literal was not found")
	}
	rest := body[i+start+1:]
	end := strings.Index(rest, "`")
	if end < 0 {
		t.Fatal("the pending-verdict query literal is unterminated")
	}
	query := rest[:end]
	if !strings.Contains(query, "benchmark_pairs") {
		t.Fatalf("the scanned literal is not the pending-verdict query: %q", query)
	}
	for _, column := range []string{"position", "platform_model", "direct_model", "direct_run_id", "platform_run_id", "epoch_id"} {
		if strings.Contains(query, column) {
			t.Errorf("the pending-verdict query names %q — no pre-record read surface may select it (BENCH-REG §3.4)", column)
		}
	}
}

// TestBenchmarkFamilyRoutesToTheInboxTopic is the OQ5(a) disposition, proven by
// test: FamilyBenchmarkEval now routes to `inbox`, so the verdict form and the
// alarm reach a topic-subscribed client live. The four-topic set is untouched.
func TestBenchmarkFamilyRoutesToTheInboxTopic(t *testing.T) {
	for _, typ := range []string{"benchmark.pair_recorded", "benchmark.alarm", "eval.score_recorded"} {
		fam, known := eventlog.Classify(typ)
		if !known || fam != eventlog.FamilyBenchmarkEval {
			t.Fatalf("%s classifies as %q,%v; want the Benchmark & eval family", typ, fam, known)
		}
		tags := topicsForEvent(eventlog.Event{Type: typ})
		if !hasTopic(tags, topicInbox) {
			t.Errorf("%s carries topics %v — a benchmark decision surface must reach the inbox topic (S3.9)", typ, tags)
		}
	}
	// decision.recorded (the §12 disposition and the §4.2.1 consent flip) rides
	// the Human-decision family, which was already an inbox member.
	if tags := topicsForEvent(eventlog.Event{Type: "decision.recorded"}); !hasTopic(tags, topicInbox) {
		t.Errorf("decision.recorded carries topics %v, want inbox", tags)
	}
	// The closed subscribe set did NOT grow.
	if len(knownTopics) != 4 {
		t.Errorf("%d topics, want the ratified four {run, board, fleet, inbox}", len(knownTopics))
	}
	for _, want := range []string{topicRun, topicBoard, topicFleet, topicInbox} {
		if !knownTopics[want] {
			t.Errorf("topic %q is missing", want)
		}
	}
}

func hasTopic(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
