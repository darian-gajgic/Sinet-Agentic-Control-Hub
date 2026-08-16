package stage_test

// route_e2e_test.go — the B3-3 dispatch acceptance: the stage dispatcher
// consumes REAL S08.8 selection end-to-end through the fake engine (zero
// paid calls): the approval card carries the routing block, the execute
// dispatch emits the settled routing.decided event (worker + version per
// run — the version→outcome join key), worker-backed runs compile per
// invocation (worker.compiled config-hash record; tighter-class rule), and
// the generalist no-fit path writes its gap record.

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// softwareClassifier steers triage to the software family at standard tier
// (the worker-backed path needs a domain-rowed family; ungated by the
// high-tier PIN step-up).
type softwareClassifier struct{}

func (softwareClassifier) Classify(_ context.Context, _ intake.TriageInput) (intake.TriageProposal, error) {
	return intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Est: intake.Estimate{SizeClass: "S", Known: false, Basis: "fake"},
	}, nil
}

type routedHarness struct {
	*harness
	workers *worker.Store
}

// newRoutedHarness builds the e2e harness WITH the S08 worker store wired
// (the B3-3 composition posture) and the software-family classifier.
func newRoutedHarness(t *testing.T) *routedHarness {
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
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)
	led := ledger.NewStore(db, log)
	workers, err := worker.NewStore(worker.Config{
		DB: db, Log: log, Settings: reg, Root: filepath.Join(t.TempDir(), "workers"),
	})
	if err != nil {
		t.Fatalf("worker.NewStore: %v", err)
	}

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Workers: workers,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary:      self,
				HookCmd:     "/opt/sinet/bin/sinet engine-hook",
				Settings:    reg,
				Env:         append(os.Environ(), "SINET_STAGE_FAKE=1"),
				CancelGrace: 500 * time.Millisecond,
			},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	sk.Pipeline().Classifier = softwareClassifier{}
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	h := &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), artifactRoot: filepath.Join(root, "artifacts")}
	return &routedHarness{harness: h, workers: workers}
}

// insertUser adds a person row (worker store verbs are human-gated).
func insertUser(t *testing.T, db *storage.DB, id, role string) {
	t.Helper()
	if err := db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO users (user_id, role, created_ts) VALUES (?, ?, ?)`,
			id, role, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

// eventsOfType returns payloads of run-events of one type for a run.
func eventsOfType(t *testing.T, db *storage.DB, runID, typ string) []map[string]any {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq`, runID, typ)
	if err != nil {
		t.Fatalf("query %s: %v", typ, err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// walkToApproval submits and answers through to the open approval card.
func (h *routedHarness) walkToApproval(ctx context.Context, owner string) (taskID, askID string, cardJSON string) {
	h.t.Helper()
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		h.t.Fatalf("Submit: %v", err)
	}
	v := decodeView(h.t, raw)
	taskID = v.TaskID
	if n := h.tick(ctx); n != 1 {
		h.t.Fatalf("tick dispatched %d", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		h.t.Fatalf("Task: %v", err)
	}
	raw = clearFamilyGate(h.t, ctx, h.sur, owner, raw) // P3-RW-11: the family question comes first
	v = decodeView(h.t, raw)
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		h.t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(h.t, raw)
	if v.OpenCard.Kind != "approval" {
		h.t.Fatalf("expected approval card, got %s", raw)
	}
	// The raw ask snapshot carries the full card incl. the routing block.
	var snapshot string
	if err := h.db.QueryRowContext(ctx,
		`SELECT snapshot FROM asks WHERE ask_id = ?`, v.OpenAskID).Scan(&snapshot); err != nil {
		h.t.Fatalf("ask snapshot: %v", err)
	}
	return taskID, v.OpenAskID, snapshot
}

func TestGeneralistExecuteEmitsRoutingDecidedAndGapRecord(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	const owner = "u-route"

	taskID, askID, cardJSON := h.walkToApproval(ctx, owner)

	// Selection VISIBLE pre-execution: the approval card carries the
	// routing block with the plain-language reason (S08.8).
	if !strings.Contains(cardJSON, `"routing"`) || !strings.Contains(cardJSON, `"plain_reason"`) {
		t.Fatalf("approval card missing the routing block: %s", cardJSON)
	}
	if !strings.Contains(cardJSON, "generalist-with-injected-knowledge") {
		t.Fatalf("no-fit generalist default not on the card: %s", cardJSON)
	}

	// The no-fit wrote its gap record through worker.RecordGap.
	var gaps int
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gap_records`).Scan(&gaps); err != nil {
		t.Fatal(err)
	}
	if gaps != 1 {
		t.Fatalf("gap records = %d, want 1", gaps)
	}

	// Approve (standard tier — no step-up) and dispatch the execute run.
	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"action":"approve"}`), false); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	if n := h.tick(ctx); n < 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}

	execRun := taskID + ".execute"
	decided := eventsOfType(t, h.db, execRun, "routing.decided")
	if len(decided) != 1 {
		t.Fatalf("routing.decided events = %d, want exactly 1", len(decided))
	}
	d := decided[0]
	if d["generalist"] != true || d["model"] != "claude-sonnet-5" || d["lane"] != "anthropic" {
		t.Fatalf("routing.decided payload = %v", d)
	}
	if s, _ := d["plain_reason"].(string); !strings.Contains(s, "generalist") {
		t.Fatalf("plain_reason = %q", s)
	}
	if d["window_tokens"].(float64) != 200_000 {
		t.Fatalf("window record = %v (the retired constant must ride the seat row)", d["window_tokens"])
	}
}

func TestWorkerBackedExecuteCompilesPerInvocation(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	const owner = "u-route"
	insertUser(t, h.db, owner, "member")

	// An ACTIVE software worker whose selectors match the request text.
	// Selector family carries the ceiling-table task-class vocabulary
	// (implement-fix: Write/Edit under the C2 ceiling — B3-2's station-2
	// keying); the S08.8 match lands via the trigger phrase + FTS.
	src := `---
name: note-writer
description: Writes short appreciation notes about software
kind: agentic
domain: software
selectors:
  family: implement-fix
  triggers: [appreciation note]
profile:
  duty: execution
equipment:
  tools: [Read, Write, Edit]
---
Write the requested note faithfully.
`
	tpl, v, err := h.workers.CreateDraft(ctx, owner, src, worker.RequestedGrants{
		Tools: []string{"Read", "Write", "Edit"}, Class: "C1", Egress: worker.EgressNone,
	}, worker.Provenance{AuthorKind: "human", Origin: worker.OriginHumanWritten})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := h.workers.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: owner, SampleTask: "write a sample note", Engine: passDry{},
		Model: "claude-haiku-4-5", EnginePin: "claude-cli@2.1.216",
	}); err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if _, err := h.workers.Approve(ctx, owner, v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	taskID, askID, cardJSON := h.walkToApproval(ctx, owner)
	if !strings.Contains(cardJSON, "note-writer") {
		t.Fatalf("selected worker not visible on the card: %s", cardJSON)
	}
	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"action":"approve"}`), false); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	if n := h.tick(ctx); n < 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}

	execRun := taskID + ".execute"
	decided := eventsOfType(t, h.db, execRun, "routing.decided")
	if len(decided) != 1 {
		t.Fatalf("routing.decided = %d", len(decided))
	}
	d := decided[0]
	// Worker + version per run — the version→outcome join key (S08.4).
	if d["worker"] != tpl.ID || d["version"] != v.ID {
		t.Fatalf("join key missing: %v", d)
	}

	// Per-invocation compile recorded on the run (S08.3): one
	// worker.compiled event per stage session with the config hash.
	compiled := eventsOfType(t, h.db, execRun, "worker.compiled")
	if len(compiled) != 1 {
		t.Fatalf("worker.compiled events = %d, want one per stage session", len(compiled))
	}
	c := compiled[0]
	if c["template"] != tpl.ID || c["version"] != v.ID {
		t.Fatalf("compile record = %v", c)
	}
	if hash, _ := c["config_hash"].(string); len(hash) != 64 {
		t.Fatalf("config_hash = %q", c["config_hash"])
	}
	if c["stage"] != "S-1" {
		t.Fatalf("compile stage = %v", c["stage"])
	}
}

// passDry is a green DryEngine for the battery.
type passDry struct{}

func (passDry) DryRun(_ context.Context, _ worker.DryRunRequest) (worker.DryRunResult, error) {
	return worker.DryRunResult{Completed: true, TranscriptRef: "fake://dry", Output: "ok", CostUSD: 0.01}, nil
}
