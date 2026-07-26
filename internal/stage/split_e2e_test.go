package stage_test

// split_e2e_test.go — the S05.3 stage-split acceptance: the full overflow
// loop through the real spine with the fake engine (zero paid calls) —
// overflow event (proposal) → auto-acceptance → boundary interrupt ends
// the session → consolidate-to-ledger (platform decision + handed-forward
// state + stage-close gate) → REAL successor sub-stage brief assembled
// from the UPDATED ledger → execution continues and the task completes,
// with the stage-fit budget re-checked per sub-stage.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// newSplitHarness is newHarness with a 500-token duty-seat window (fit 250
// / overflow 350 under the ⚙ defaults) and the fake engine's overflow
// knob: the first execute session's second call measures footprint 440 and
// crosses the threshold (the recite_e2e local-harness precedent for a
// custom stage.Config).
func newSplitHarness(t *testing.T) *harness {
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

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	root := t.TempDir()
	seat := worker.Seat{Model: "claude-haiku-4-5", Lane: "anthropic", WindowTokens: 500}
	// The ceremony seat carries the ratified judge model: this test forces a
	// context overflow (the tiny window), not a judge change, and P-T06-5 blocks
	// unsupervised judging under a seat the rubric was never measured on
	// (Spec S14.8 ¶3).
	ceremony := worker.Seat{Model: "claude-opus-4-8", Lane: "anthropic", WindowTokens: 500}
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		DutyMap: worker.DutyMap{
			worker.DutyExecution: seat, worker.DutyPlanning: ceremony, worker.DutyJudge: ceremony,
		},
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary:      self,
				HookCmd:     "/opt/sinet/bin/sinet engine-hook",
				Settings:    reg,
				Env:         append(os.Environ(), "SINET_STAGE_FAKE=1", "SINET_STAGE_FAKE_OVERFLOW=1"),
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
	return &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), artifactRoot: filepath.Join(root, "artifacts")}
}

func TestStageSplitOverflowE2E(t *testing.T) {
	h := newSplitHarness(t)
	ctx := context.Background()
	const owner = "u-split"

	// Submit → interview → force-proceed → approve (high tier, step-up).
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("intake tick dispatched %d", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v := decodeView(t, raw)
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(t, raw)
	if v.OpenCard.Kind != "approval" {
		t.Fatalf("expected the approval card, got %s", raw)
	}
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}

	// Execute: the first S-1 session overflows mid-flight → the stage split
	// executes → the successor sub-stage session completes the step. Then
	// verify ships.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("verify tick dispatched %d", n)
	}

	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v = decodeView(t, raw)
	if v.Kanban != "done" {
		t.Fatalf("kanban %q, want done (%s)", v.Kanban, raw)
	}

	execRun := taskID + ".execute"

	// 1. The proposal record: exactly ONE compaction.anomaly event, proposing
	// the stage split for the planned stage S-1 (the successor sub-stage
	// stayed under budget — re-checked per sub-stage — so no second event).
	overflows := eventsOfType(t, h.db, execRun, "compaction.anomaly")
	if len(overflows) != 1 {
		t.Fatalf("compaction.anomaly events = %d, want exactly 1: %v", len(overflows), overflows)
	}
	ovf := overflows[0]
	if ovf["proposal"] != "stage-split" || ovf["stage"] != "S-1" {
		t.Fatalf("overflow payload = %v", ovf)
	}
	if !strings.Contains(ovf["executes_at"].(string), "auto-accepted") {
		t.Fatalf("executes_at = %v", ovf["executes_at"])
	}

	// 2. The acceptance is fully logged: the platform decision on the task
	// ledger records the executed split.
	doc, found, err := h.led.Current(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("ledger Current: found=%v err=%v", found, err)
	}
	var splitDecision bool
	for _, d := range doc.Decisions {
		if d.Author == ledger.AuthorPlatform && strings.Contains(d.Text, "stage split executed") &&
			strings.Contains(d.Text, "S-1#2") {
			splitDecision = true
		}
	}
	if !splitDecision {
		t.Fatalf("no platform split decision in %+v", doc.Decisions)
	}

	// 3. REAL sub-stage briefs via the ledger: one assembly manifest per
	// sub-stage session, the successor's built from the UPDATED (higher)
	// ledger revision.
	manifests := eventsOfType(t, h.db, execRun, "knowledge.injected")
	versionsByStage := map[string]float64{}
	for _, m := range manifests {
		if kind, _ := m["kind"].(string); kind != "" && kind != "assembly" {
			continue // reinjection/recitation entries are not assemblies
		}
		stageName, _ := m["stage"].(string)
		if lv, ok := m["ledger_version"].(float64); ok {
			versionsByStage[stageName] = lv
		}
	}
	v1, ok1 := versionsByStage["S-1"]
	v2, ok2 := versionsByStage["S-1#2"]
	if !ok1 || !ok2 {
		t.Fatalf("missing sub-stage assemblies: %v", versionsByStage)
	}
	if v2 <= v1 {
		t.Fatalf("successor brief ledger_version %v not after predecessor %v (S05.3: successor brief from the UPDATED ledger)", v2, v1)
	}

	// 4. The consolidated hand-forward IS what the successor assembled
	// from: at the successor's revision the step is in flight and named in
	// next_actions (the stage-close gate's handed-forward channel).
	atSplit, err := h.led.AtVersion(ctx, taskID, int64(v2))
	if err != nil {
		t.Fatalf("AtVersion(%v): %v", v2, err)
	}
	var item *ledger.WorkItem
	for i := range atSplit.State.Items {
		if atSplit.State.Items[i].ID == "S-1" {
			item = &atSplit.State.Items[i]
		}
	}
	if item == nil || item.Status != ledger.StatusInProgress {
		t.Fatalf("split-boundary item = %+v", item)
	}
	forwarded := false
	for _, n := range atSplit.State.NextActions {
		if n == "S-1" {
			forwarded = true
		}
	}
	if !forwarded {
		t.Fatalf("step not handed forward in next_actions: %+v", atSplit.State.NextActions)
	}

	// 5. Two engine sessions carried the one planned stage (distinct
	// engine session ids across the run's checkpoints) and their paid
	// calls all checkpointed against the execute run (2 split-leg calls +
	// 1 successor call, D7 per paid call).
	var sessions int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT session_id) FROM checkpoints WHERE run_id = ?`, execRun).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 2 {
		t.Fatalf("distinct engine sessions = %d, want 2 (split leg + successor)", sessions)
	}
	var cps int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM checkpoints WHERE run_id = ?`, execRun).Scan(&cps); err != nil {
		t.Fatal(err)
	}
	if cps != 3 {
		t.Fatalf("checkpoints = %d, want 3 (D7 per paid call across both sub-sessions)", cps)
	}

	// 6. The step closed verified through the normal drain — the split
	// never weakened the record.
	final, _, err := h.led.Current(ctx, taskID)
	if err != nil {
		t.Fatalf("ledger final: %v", err)
	}
	for _, it := range final.State.Items {
		if it.ID == "S-1" && it.Status != ledger.StatusVerified {
			t.Fatalf("final S-1 status = %s, want verified", it.Status)
		}
	}
}
