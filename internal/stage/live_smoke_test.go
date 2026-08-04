package stage_test

// Tier-L live smoke (CONVENTIONS §10; the B1-1 precedent): prove the
// walking-skeleton wiring holds against the REAL engine with exactly ONE
// minimal paid call — the execute leg. Everything else that would cost
// money (planner, critic, judge) is an in-process fake: the FULL live
// pipeline is deliberately reserved for the B2 gate demo with the
// operator watching (P3/gates/B2-demo-script.md).
//
// Run:  SINET_LIVE_SMOKE=1 go test ./internal/stage -run TestLiveSmokeExecuteLeg -v

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// smokePlanner fabricates the approved-plan ceremony in-process (no paid
// planning call — that leg is fake-engine-covered in the E2E and live at
// the gate demo).
type smokePlanner struct{}

func (smokePlanner) Draft(_ context.Context, in intake.DraftInput) (intake.Pair, error) {
	pair := intake.Pair{
		Spec: intake.Spec{
			TaskID: in.Request.TaskID, Owner: in.Request.UserID,
			Version: in.SpecVersion, Status: intake.StatusDraft, Tier: in.Tier,
			Provenance:  "live-smoke in-process planner (tier-L harness)",
			Restatement: "Write one sentence appreciating SQLite.",
			ACs: []intake.AC{
				{N: 1, Plain: "The sentence mentions SQLite by name."},
			},
		},
		Plan: intake.Plan{
			TaskID: in.Request.TaskID, Owner: in.Request.UserID,
			Version: in.PlanVersion, SpecVersion: in.SpecVersion,
			Status: intake.StatusDraft, Tier: in.Tier,
			Provenance: "live-smoke in-process planner (tier-L harness)",
			Steps: []intake.Step{{
				ID: "S-1", Title: "Write the sentence", Class: "C1",
				DoneWhen: "the final message is one sentence that mentions SQLite",
			}},
			Coverage: map[string][]string{"AC-1": {"S-1"}},
			Est:      intake.Estimate{Known: false, Basis: "fixed tier-L smoke plan"},
		},
	}
	return pair, nil
}

func (p smokePlanner) Revise(ctx context.Context, in intake.ReviseInput) (intake.Pair, error) {
	return in.Pair, nil
}

type smokeCritic struct{}

func (smokeCritic) Critique(context.Context, intake.Pair) (intake.Verdict, error) {
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}
func (smokeCritic) Recheck(context.Context, intake.Pair, []string) (intake.Verdict, error) {
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}

// smokeJudge SHIPs with extractive evidence quoted FROM the real artifact
// (the validator demands a verbatim substring) — wiring proof, not judge
// quality; the real judge runs at the gate demo.
type smokeJudge struct{}

func (smokeJudge) Compliance(_ context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	evidence := strings.TrimSpace(in.Artifact)
	if len(evidence) > 40 {
		evidence = evidence[:40]
	}
	return verify.Axis1Result{Verdicts: []verify.ACVerdict{
		{Key: "AC-1", Pass: strings.Contains(in.Artifact, "SQLite"), Evidence: evidence},
	}}, nil
}

func (smokeJudge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{ProbeNotes: map[verify.Probe]string{
		verify.ProbeReasonableUser:       "smoke: pass-through",
		verify.ProbeImplicitExpectations: "smoke: pass-through",
		verify.ProbeSideEffects:          "smoke: pass-through",
		verify.ProbeExpertStandard:       "smoke: pass-through",
	}}, nil
}

func (smokeJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "in-process-smoke-judge", SelfFamily: false}
}

func TestLiveSmokeExecuteLeg(t *testing.T) {
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): tier-L runs only under SINET_LIVE_SMOKE=1 (one minimal paid call)")
	}
	if _, err := exec.LookPath(claudecli.DefaultBinary); err != nil {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): no claude engine installed")
	}
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
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters: map[string]adapters.Adapter{
			// The REAL pinned engine, unconfined dev spawn (the sanctioned
			// B1-1 posture: confined+authenticated needs the S11.5 proxy,
			// a B2-gate host component).
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{Settings: reg},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		Planner:      smokePlanner{},
		Critic:       smokeCritic{},
		Judge:        smokeJudge{},
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     5 * time.Minute,
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	sur := sk.Surface()

	tick := func(want int) {
		t.Helper()
		n, err := sched.Tick(ctx)
		if err != nil {
			t.Fatalf("Tick: %v", err)
		}
		sched.WaitInFlight()
		if n != want {
			t.Fatalf("tick dispatched %d, want %d", n, want)
		}
	}

	raw, err := sur.Submit(ctx, "u-smoke", json.RawMessage(
		`{"title":"smoke","text":"Write one sentence appreciating SQLite."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	var v struct {
		TaskID    string `json:"task_id"`
		OpenAskID string `json:"open_ask_id"`
	}
	mustDecode := func() {
		t.Helper()
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	mustDecode()
	taskID := v.TaskID
	tick(1) // intake → interview card
	if raw, err = sur.Task(ctx, taskID); err != nil {
		t.Fatalf("Task: %v", err)
	}
	mustDecode()
	if raw, err = sur.Answer(ctx, "u-smoke", v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false); err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	mustDecode()
	if raw, err = sur.Answer(ctx, "u-smoke", v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	tick(1) // execute — THE one paid call, on the real engine
	tick(1) // verify — in-process judge, free

	execRun := taskID + ".execute"
	r, err := runs.Get(ctx, execRun)
	if err != nil || r.State != run.StateCompleted {
		t.Fatalf("execute run: %+v err=%v", r, err)
	}
	cp, ok, err := cps.Last(ctx, execRun)
	if err != nil || !ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	if cp.LedgerRevision == "" {
		t.Error("checkpoint without the live ledger-revision block (S02.4c)")
	}
	// The engine-reported spend figure is a cross-check (D5) riding the
	// engine.done payload; the platform prices UNPRICED at v0 (empty
	// table). Reported here for the packet record.
	var donePayload string
	if err := db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = 'engine.done' ORDER BY event_seq DESC LIMIT 1`,
		execRun).Scan(&donePayload); err != nil {
		t.Fatalf("engine.done: %v", err)
	}
	var usage struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	}
	_ = json.Unmarshal([]byte(donePayload), &usage)
	deliverable, err := os.ReadFile(filepath.Join(root, "artifacts", taskID, "deliverable-rev1.md"))
	if err != nil {
		t.Fatalf("deliverable: %v", err)
	}
	var receipt string
	if err := db.QueryRowContext(ctx, `SELECT usage_json FROM receipts WHERE run_id = ?`, execRun).Scan(&receipt); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	fmt.Printf("LIVE SMOKE: engine cost cross-check USD=%.6f; checkpoint model=%s session=%s\n",
		usage.TotalCostUSD, cp.ModelID, cp.SessionID)
	fmt.Printf("LIVE SMOKE: deliverable=%q\n", strings.TrimSpace(string(deliverable)))
	if !strings.Contains(receipt, `"execution"`) {
		t.Errorf("execute receipt without execution line item: %s", receipt)
	}
}
