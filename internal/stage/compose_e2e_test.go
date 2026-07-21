package stage_test

// compose_e2e_test.go — the S08.6 acceptance: the no-fit card's compose
// verb reaches REAL generation → battery → approval-as-diff, dev-mode
// through the fake engine (zero paid calls): two distinct no-fit tasks
// earn the gap → the second task's card offers compose → the answer
// launches the `.compose` ceremony run (billed to the requester as
// ceremony) → one-shot generation at the planning seat → CreateDraft with
// composer provenance → the UNCHANGED four-station battery (production
// EngineDryRun witness) → approval-as-diff → after the human approves the
// worker, a re-plan recomputes routing, selects it, and execution compiles
// it per invocation.

import (
	"context"
	"database/sql"
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
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// newComposeHarness is the routed harness plus the composition seams: the
// worker store, the REAL S09 memory machinery serving the governed
// composer playbook through the ComposerPlaybook seam (the shell's
// wiring shape), and the engine pin keying validation records.
func newComposeHarness(t *testing.T, operator string) *routedHarness {
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

	// The operator account + the governed playbook (house scope needs its
	// D10 holder; the composer reads the current approved version).
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO users (user_id, role, created_ts) VALUES (?, 'operator', ?)`,
			operator, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("insert operator: %v", err)
	}
	memStore, err := memory.NewStore(db, log, reg, filepath.Join(t.TempDir(), "knowledge"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	if created, err := memory.NewGate(memStore).EnsureComposerPlaybook(ctx); err != nil || !created {
		t.Fatalf("EnsureComposerPlaybook: created=%v err=%v", created, err)
	}

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Workers: workers,
		ComposerPlaybook: func(ctx context.Context) (worker.Playbook, error) {
			e, err := memStore.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
			if err != nil {
				return worker.Playbook{}, err
			}
			return worker.Playbook{EntryID: e.ID, Version: e.Version, Content: e.Content}, nil
		},
		EnginePin: claudecli.Pin,
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

func TestNoFitComposeVerbE2E(t *testing.T) {
	h := newComposeHarness(t, "u-comp")
	ctx := context.Background()
	const owner = "u-comp"

	// Two DISTINCT no-fit tasks earn the gap (⚙ workers.gap_proposal_count
	// default 2 — recurrence means distinct tasks).
	_, _, firstCard := h.walkToApproval(ctx, owner)
	if strings.Contains(firstCard, `"compose_earned":true`) {
		t.Fatalf("first occurrence must not be earned: %s", firstCard)
	}
	taskB, askB, cardB := h.walkToApproval(ctx, owner)
	if !strings.Contains(cardB, `"compose_earned":true`) || !strings.Contains(cardB, `"compose"`) {
		t.Fatalf("second occurrence: compose not offered on the card: %s", cardB)
	}

	// The compose verb: the card stays OPEN, the ceremony run launches.
	raw, err := h.sur.Answer(ctx, owner, askB, json.RawMessage(`{"action":"compose"}`), false)
	if err != nil {
		t.Fatalf("Answer(compose): %v", err)
	}
	v := decodeView(t, raw)
	if v.OpenAskID != askB {
		t.Fatalf("approval card must stay open across compose (S08.6): %s", raw)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("compose tick dispatched %d", n)
	}

	composeRun := taskB + ".compose"
	r, err := h.runs.Get(ctx, composeRun)
	if err != nil {
		t.Fatalf("compose run: %v", err)
	}
	if r.State != run.StateCompleted {
		t.Fatalf("compose run state = %s, want completed", r.State)
	}
	// Billed and itemized to the requester as CEREMONY (S08.6; metering
	// suffix derivation): the receipt materialized at settle.
	var usageJSON string
	if err := h.db.QueryRowContext(ctx,
		`SELECT usage_json FROM receipts WHERE run_id = ?`, composeRun).Scan(&usageJSON); err != nil {
		t.Fatalf("compose receipt: %v", err)
	}
	if !strings.Contains(usageJSON, `"ceremony"`) {
		t.Fatalf("compose receipt not itemized as ceremony: %s", usageJSON)
	}
	// Generation + witness dry run both checkpointed against the ceremony
	// run (D7 per paid call).
	var cpN int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM checkpoints WHERE run_id = ?`, composeRun).Scan(&cpN); err != nil {
		t.Fatal(err)
	}
	if cpN != 2 {
		t.Fatalf("compose checkpoints = %d, want 2 (one-shot generation + station-3 witness)", cpN)
	}

	// The composed draft: composer provenance, validated green, golden seed
	// unverified, approval-as-diff card ready, gap composed.
	var tplID, verID string
	if err := h.db.QueryRowContext(ctx, `
		SELECT t.template_id, v.version_id FROM worker_templates t
		  JOIN worker_template_versions v ON v.template_id = t.template_id
		 WHERE t.name = 'note-composer'`).Scan(&tplID, &verID); err != nil {
		t.Fatalf("composed template row: %v", err)
	}
	ver, err := h.workers.VersionByID(ctx, verID)
	if err != nil {
		t.Fatalf("VersionByID: %v", err)
	}
	if ver.AuthorKind != "composer" || ver.Origin != worker.OriginComposed ||
		ver.Composer != "claude-haiku-4-5" || !strings.Contains(ver.PlaybookVer, "seed-composer-playbook@v1") {
		t.Fatalf("composer provenance = %+v", ver)
	}
	if !strings.HasPrefix(ver.EvidenceRef, "gap:") || ver.OriginRef != taskB {
		t.Fatalf("evidence = %q origin_ref = %q", ver.EvidenceRef, ver.OriginRef)
	}
	rec, err := h.workers.LatestValidation(ctx, verID)
	if err != nil {
		t.Fatalf("LatestValidation: %v", err)
	}
	if !rec.Green || rec.EnginePin != claudecli.Pin {
		t.Fatalf("validation = green=%v pin=%q", rec.Green, rec.EnginePin)
	}
	if !strings.Contains(rec.DryRunRef, `"verified":false`) || !strings.Contains(rec.DryRunRef, "witnessed") {
		t.Fatalf("dry-run record = %s", rec.DryRunRef)
	}
	card, err := h.workers.BuildApprovalCard(ctx, verID)
	if err != nil {
		t.Fatalf("BuildApprovalCard: %v", err)
	}
	if card.Diff == "" || card.Provenance.AuthorKind != "composer" {
		t.Fatalf("approval-as-diff card = %+v", card)
	}
	gapSig := ver.EvidenceRef[len("gap:"):]
	gap, err := h.workers.Gap(ctx, gapSig)
	if err != nil {
		t.Fatalf("Gap: %v", err)
	}
	if gap.Disposition != worker.GapComposed {
		t.Fatalf("gap disposition = %s", gap.Disposition)
	}

	// A second compose on the same task refuses (one composition per task).
	if _, err := h.sur.Answer(ctx, owner, askB, json.RawMessage(`{"action":"compose"}`), false); err == nil {
		t.Fatal("second compose on the same task must refuse")
	}

	// Station 4: the owner approves the worker (the battery gated it; no
	// above-ceiling flags to acknowledge — the draft stayed inside the
	// implement-fix ceiling row).
	if _, err := h.workers.Approve(ctx, owner, verID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve worker: %v", err)
	}

	// Re-plan recomputes routing (the B3-3 mechanics): the fresh selection
	// now finds the ACTIVE composed specialist, visible on the new card.
	raw, err = h.sur.Answer(ctx, owner, askB, json.RawMessage(
		`{"action":"replan","contest":{"target":"S-1","note":"the composed specialist is now active; recompute routing"}}`), false)
	if err != nil {
		t.Fatalf("Answer(replan): %v", err)
	}
	v = decodeView(t, raw)
	if v.OpenAskID == "" || v.OpenCard.Kind != "approval" {
		t.Fatalf("no recomputed approval card: %s", raw)
	}
	var snapshot string
	if err := h.db.QueryRowContext(ctx,
		`SELECT snapshot FROM asks WHERE ask_id = ?`, v.OpenAskID).Scan(&snapshot); err != nil {
		t.Fatalf("recomputed card snapshot: %v", err)
	}
	if !strings.Contains(snapshot, "note-composer") || !strings.Contains(snapshot, `"cause":"selector-match"`) {
		t.Fatalf("recomputed card does not select the composed worker: %s", snapshot)
	}

	// Approve → the execute dispatch compiles the composed worker per
	// invocation and the settled routing.decided carries the join key.
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), false); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	if n := h.tick(ctx); n < 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}
	execRun := taskB + ".execute"
	decided := eventsOfType(t, h.db, execRun, "routing.decided")
	if len(decided) != 1 || decided[0]["worker"] != tplID || decided[0]["version"] != verID {
		t.Fatalf("routing.decided = %v", decided)
	}
	compiled := eventsOfType(t, h.db, execRun, "worker.compiled")
	if len(compiled) != 1 || compiled[0]["template"] != tplID {
		t.Fatalf("worker.compiled = %v", compiled)
	}
}
