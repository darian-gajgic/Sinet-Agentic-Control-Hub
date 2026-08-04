package benchmark_test

// harness_test.go — the shared hermetic fixture for the S14.7 battery
// (CONVENTIONS §3): a t.TempDir() DB, stdlib testing only, ZERO network and
// ZERO paid calls. Every arm in these suites is a tier-F fake: the direct arm
// never reaches an engine, so this packet's battery costs $0 by construction,
// which is also what BENCH-REG §4.5 expects — real accrual is a bring-up act
// under the registration's own schedule, not a test.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

const repoRoot = "../../"

// registrationPath is the signed registration itself. registered_test.go pins
// every frozen value in internal/benchmark against THIS file, so drift between
// the registration text and the running platform fails the build — which is
// what BENCH-REG §17's "silent drift … is a defect of the platform" requires.
const registrationPath = repoRoot + "Spec/benchmark-preregistration-v1.md"

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type harness struct {
	db    *storage.DB
	log   *eventlog.Log
	reg   *settings.Registry
	runs  *run.Store
	store *benchmark.Store
	clk   *clock
}

// clock is the movable test clock: dueness, epochs and latencies derive from
// stored state, so a suite advances the clock rather than sleeping.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

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
	clk := &clock{t: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	st := benchmark.NewStore(db, log)
	st.Now = clk.now
	return &harness{
		db: db, log: log, reg: reg, runs: run.NewStore(db, log),
		store: st, clk: clk,
	}
}

// practice wires a Practice over the harness with tier-F seams.
func (h *harness) practice(t *testing.T) *benchmark.Practice {
	t.Helper()
	return &benchmark.Practice{
		Store: h.store, Log: h.log, Runs: h.runs,
		Queue:    h.scheduler(t),
		Ledger:   h.ledger(),
		Briefs:   fixedBrief,
		Platform: func(context.Context, string) (string, error) { return "platform answer", nil },
		Direct:   func(context.Context, string) (string, error) { return "direct answer", nil },
		// Position is fixed to platform-on-A in most suites so an assertion
		// about arms is not a coin flip; the randomization itself is asserted
		// separately in pair_test.go.
		Rand:   func(int) int { return 0 },
		Logger: discardLogger(),
		Now:    h.clk.now,
	}
}

// scheduler builds a REAL scheduler. The direct arm must ride the ordinary
// admission surface (G2 Def.4), so the suites enqueue through the production
// type rather than a stand-in; the dispatcher is a no-op because nothing in
// this packet executes an arm.
func (h *harness) scheduler(t *testing.T) *scheduler.Scheduler {
	t.Helper()
	s, err := scheduler.New(scheduler.Config{
		DB: h.db, Runs: h.runs, Settings: h.reg,
		Dispatcher: dispatcherFunc(func(context.Context, run.Run) error { return nil }),
		Logger:     discardLogger(), Now: h.clk.now,
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	return s
}

type dispatcherFunc func(context.Context, run.Run) error

func (f dispatcherFunc) Dispatch(ctx context.Context, r run.Run) error { return f(ctx, r) }

// testStatementSource is what the fixture brief reports as the artifact the
// frozen statement was drawn from — the shape the real seam produces.
const testStatementSource = "s06-artifact-of-record:SPEC-v2.md@sha256:deadbeefcafe"

func fixedBrief(_ context.Context, taskID string) (benchmark.TaskBrief, error) {
	return benchmark.TaskBrief{
		TaskID: taskID, Statement: "the confirmed task statement, frozen",
		StatementSource: testStatementSource,
		Attachments:     []string{testStatementSource},
	}, nil
}

// recordingQueue captures what was enqueued, for the suites that assert the
// direct arm's admission shape.
type recordingQueue struct {
	inner  benchmark.Queue
	runIDs []string
	class  []scheduler.WorkloadClass
}

func (q *recordingQueue) Enqueue(ctx context.Context, runID string, c scheduler.WorkloadClass) error {
	q.runIDs = append(q.runIDs, runID)
	q.class = append(q.class, c)
	if q.inner != nil {
		return q.inner.Enqueue(ctx, runID, c)
	}
	return nil
}

// ── fixture builders ─────────────────────────────────────────────────────────

func (h *harness) user(t *testing.T, id string, optedIn bool) {
	t.Helper()
	h.userAs(t, id, "member", optedIn)
}

// operator inserts the one S01.9 role holder — the principal §12 requires for
// an alarm disposition.
func (h *harness) operator(t *testing.T, id string) {
	t.Helper()
	h.userAs(t, id, "operator", false)
}

func (h *harness) userAs(t *testing.T, id, role string, optedIn bool) {
	t.Helper()
	ctx := context.Background()
	if err := h.exec(ctx, `INSERT INTO users (user_id, role, created_ts) VALUES (?, ?, ?)`,
		id, role, h.clk.now().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if optedIn {
		if err := h.practice(t).SetOptIn(ctx, id, id, true); err != nil {
			t.Fatalf("SetOptIn: %v", err)
		}
	}
}

// task creates a task, its platform run, a verdict record naming the code
// domain, an intake.state carrying the tier, and an accepted deliverable —
// the full shape the §4.2 eligibility conjunction reads.
type taskOpts struct {
	taskID     string
	owner      string
	codeDomain string
	tier       string
	family     string
	accept     bool
}

func (h *harness) task(t *testing.T, o taskOpts) string {
	t.Helper()
	ctx := context.Background()
	ts := h.clk.now().Format(time.RFC3339Nano)
	if err := h.exec(ctx, `INSERT INTO tasks (task_id, user_id, created_ts) VALUES (?, ?, ?)`,
		o.taskID, o.owner, ts); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	runID := o.taskID + ".r1"
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: o.owner, TaskID: o.taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("create platform run: %v", err)
	}
	if o.codeDomain != "" {
		h.appendRun(t, runID, "verdict.recorded", map[string]any{
			"round": 1, "verdict": "pass", "ac_ids": []string{"AC1"},
			"golden_set": map[string]any{"measured": false}, "domain": o.codeDomain,
		})
	}
	if o.tier != "" {
		h.appendRun(t, runID, "intake.state", map[string]any{
			"phase": "done", "task_id": o.taskID, "run_id": runID,
			"tier": o.tier, "family": o.family,
		})
	}
	dID := o.taskID + ".d1"
	state := "in-review"
	if o.accept {
		state = "accepted"
	}
	if err := h.exec(ctx, `INSERT INTO deliverables
		(deliverable_id, user_id, task_id, dtype, current_revision, state, created_ts, updated_ts)
		VALUES (?, ?, ?, 'code', 1, ?, ?, ?)`, dID, o.owner, o.taskID, state, ts, ts); err != nil {
		t.Fatalf("insert deliverable: %v", err)
	}
	if err := h.exec(ctx, `INSERT INTO deliverable_revisions
		(deliverable_id, n, user_id, run_id, pin_kind, content_sha256, platform_ref, created_ts)
		VALUES (?, 1, ?, ?, 'content', 'abc', 'obj/ref', ?)`, dID, o.owner, runID, ts); err != nil {
		t.Fatalf("insert revision: %v", err)
	}
	return dID
}

// accept appends the deliverable.accepted event the sampling hook keys on. It
// is written directly rather than through internal/review, which this packet
// does not touch and does not import.
func (h *harness) accept(t *testing.T, owner, deliverableID string) int64 {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"deliverable_id": deliverableID, "revision": 1, "accepted_by": owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	seq, err := h.log.Append(context.Background(), eventlog.Append{
		UserID: owner, Type: "deliverable.accepted", SchemaVersion: 1,
		Payload: payload, Time: h.clk.now(),
	})
	if err != nil {
		t.Fatalf("append accept: %v", err)
	}
	return seq
}

func (h *harness) appendRun(t *testing.T, runID, typ string, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if _, err := h.log.Append(context.Background(), eventlog.Append{
		RunID: runID, Generation: r.Generation, UserID: r.UserID, Type: typ,
		SchemaVersion: 1, Payload: raw, Time: h.clk.now(),
	}); err != nil {
		t.Fatalf("append %s: %v", typ, err)
	}
}

// checkpoint writes one metered checkpoint row so the ledger reports a measured
// consumption and an observed model identity for a run — the §14 readings. The
// tokens scale with `units` so a suite can give pairs different consumption and
// assert a median rather than a constant.
func (h *harness) checkpoint(t *testing.T, runID, model string, units int64) {
	t.Helper()
	ctx := context.Background()
	r, err := h.runs.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	usage := fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d}`, units*1000, units*500)
	seq, err := h.log.Append(ctx, eventlog.Append{
		RunID: runID, Generation: r.Generation, UserID: r.UserID, Type: "usage.recorded",
		SchemaVersion: 1, Payload: json.RawMessage(`{"message_id":"m","message_index":1}`),
		Time: h.clk.now(),
	})
	if err != nil {
		t.Fatalf("append usage: %v", err)
	}
	if err := h.exec(ctx, `INSERT INTO checkpoints
		(run_id, user_id, event_seq, model_id, session_id, message_index, usage_json, created_ts)
		VALUES (?, ?, ?, ?, 's1', 1, ?, ?)`,
		runID, r.UserID, seq, model, usage,
		h.clk.now().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert checkpoint: %v", err)
	}
}

// ledger prices at $1 per 1000 input tokens on the direct arm's lane, so a
// checkpoint's consumption is a legible number in the median assertions.
func (h *harness) ledger() *metering.Ledger {
	table := metering.NewEffectiveDatedTable("test-v0")
	for _, model := range []string{"frontier-a", "frontier-b", "platform-model"} {
		table.Add(metering.PriceRow{
			Model: model, Lane: "anthropic",
			Prices:        metering.UnitPrices{InputUSD: 0.001, OutputUSD: 0.002},
			EffectiveFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}
	return metering.NewLedger(h.db, table, metering.NoMeteredExceptions(), h.reg)
}

func (h *harness) exec(ctx context.Context, q string, args ...any) error {
	return h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
}

// setRate overrides ⚙ benchmark.sampling_rate, exercising the LIVE read: the
// sampler re-reads the key at every evaluation, so a mid-run change applies to
// the next draw without a restart.
func (h *harness) setRate(t *testing.T, preGate, maintenance string) {
	t.Helper()
	value, err := json.Marshal(map[string]string{"pre-gate": preGate, "maintenance": maintenance})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.reg.Set(context.Background(), settings.SetRequest{
		Key:    benchmark.KeySamplingRate,
		Value:  value,
		Actor:  settings.Actor{Kind: settings.ActorOperator, ID: "op"},
		Reason: "test",
	}); err != nil {
		t.Fatalf("set %s: %v", benchmark.KeySamplingRate, err)
	}
}

// readPackageSource concatenates the package's non-test sources, for the
// source-scan assertions (the §12/§23 conformance-scan precedent).
func readPackageSource(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "internal/benchmark")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	files := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(body)
		files++
	}
	if files == 0 {
		t.Fatal("the source scan read no files — it would pass vacuously")
	}
	return b.String()
}

// registrationText reads the signed registration.
func registrationText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "Spec/benchmark-preregistration-v1.md"))
	if err != nil {
		t.Fatalf("read the registration: %v", err)
	}
	return string(b)
}

// sqlTxAlias names *sql.Tx for the suites that call a Tx-scoped verb directly.
type sqlTxAlias = sql.Tx

// readPackageCode returns the package's IDENTIFIERS with comments and string
// literals excluded, for negative assertions about what the code does. Comments
// are excluded because a comment-inclusive scan trips on the doc comments that
// explain why a mechanism is absent, which is exactly backwards; string
// literals are excluded because display prose legitimately names things the
// code must not reference. String-VALUE assertions have their own scanner in
// importwall_test.go, where the value itself is the hazard.
func readPackageCode(t *testing.T) string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join(repoRoot, "internal/benchmark"), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	files := 0
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			files++
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.Ident:
					b.WriteString(x.Name)
					b.WriteByte('\n')
				}
				return true
			})
		}
	}
	if files == 0 {
		t.Fatal("the code scan read no files — it would pass vacuously")
	}
	return b.String()
}

// pairSpec describes one pair to drive end to end, for the suites that need an
// accrual rather than a single pair.
type pairSpec struct {
	task string
	// choice/guess are the §3.3 form. decline vetoes instead.
	choice  benchmark.Choice
	guess   benchmark.Side
	decline bool
	// directModel is the direct arm's OBSERVED identity — the only thing that
	// splits an epoch (BENCH-REG §9).
	directModel string
	// platformModel is recorded per pair and NEVER splits an epoch.
	platformModel string
	// units scales the arms' measured consumption, for the §13 median.
	units int64
	// platformOnB puts the platform arm on side B, so a suite can prove the
	// win/loss scoring follows the POSITION and not the letter voted.
	platformOnB bool
}

// recordPair drives one pair from the sampling hook to its §14 record.
func (h *harness) recordPair(t *testing.T, s pairSpec) benchmark.Pair {
	t.Helper()
	ctx := context.Background()
	pair := h.sampledPair(t, s.task)
	p := h.practice(t)
	p.Rand = func(int) int {
		if s.platformOnB {
			return 1
		}
		return 0
	}
	if s.decline {
		got, err := p.Decline(ctx, pair.PairID)
		if err != nil {
			t.Fatalf("Decline(%s): %v", s.task, err)
		}
		return got
	}
	if _, err := p.DispatchDirectArm(ctx, pair.PairID); err != nil {
		t.Fatalf("DispatchDirectArm(%s): %v", s.task, err)
	}
	if _, err := p.RenderBlind(ctx, pair.PairID); err != nil {
		t.Fatalf("RenderBlind(%s): %v", s.task, err)
	}
	if s.units > 0 {
		if s.platformModel == "" {
			s.platformModel = "platform-model"
		}
		h.checkpoint(t, pair.PlatformRunID, s.platformModel, s.units)
		h.checkpoint(t, benchmark.DirectRunID(pair.PairID), s.directModel, s.units)
	}
	a, err := benchmark.NewAnswer(s.choice, s.guess)
	if err != nil {
		t.Fatalf("NewAnswer: %v", err)
	}
	got, err := p.RecordVerdict(ctx, pair.PairID, a)
	if err != nil {
		t.Fatalf("RecordVerdict(%s): %v", s.task, err)
	}
	return got
}

// recordWins drives n pairs the platform wins, all guessed correctly unless
// blindHalf is set, on one direct-arm identity.
func (h *harness) recordWins(t *testing.T, prefix string, n int, wins int, directModel string) {
	t.Helper()
	for i := range n {
		choice := benchmark.ChoiceA // platform is on side A (default draw)
		if i >= wins {
			choice = benchmark.ChoiceB
		}
		h.recordPair(t, pairSpec{
			task: fmt.Sprintf("%s%d", prefix, i), choice: choice,
			guess: benchmark.SideB, directModel: directModel, units: 1,
		})
	}
}
