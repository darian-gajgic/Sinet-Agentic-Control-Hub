package stage_test

// verifyoutage_rw14_test.go — P3-RW-14 R4 at the STAGE boundary, both
// directions (evaluator checklist item 3):
//
//   - the launch-domain no-pack refusal parks on a card (that arm is the
//     brief's committed acceptance test, verifywedge_rw14_e2e_test.go);
//   - a MECHANICAL mid-drain failure — here a judge whose transport dies —
//     still leaves the corpse the recovery ladder exists to fork.
//
// One conversion too many would silence real breakage into an inbox item
// nobody can act on; one too few is the wedge this packet closes.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// deadJudge is a wired judge whose transport dies — a mechanical failure INSIDE
// the drain, not a missing screen.
type deadJudge struct{ err error }

func (j deadJudge) Compliance(context.Context, verify.JudgeInput) (verify.Axis1Result, error) {
	return verify.Axis1Result{}, j.err
}
func (j deadJudge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{}, j.err
}
func (j deadJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "dead-judge-1", SelfFamily: true}
}

// outageHarness is newHarness with one seam substituted: the Judge (a dead
// transport) or the check-pack resolver.
func outageHarness(t *testing.T, judge verify.Judge, packFor func(ctx context.Context, domain, taskID string) (*verify.CheckPack, error)) *harness {
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
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(root, "review")}
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary: self, HookCmd: "/opt/sinet/bin/sinet engine-hook", Settings: reg,
				Env:         append(os.Environ(), "SINET_STAGE_FAKE=1"),
				CancelGrace: 500 * time.Millisecond,
			},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		Review:       rev,
		Judge:        judge,
		CheckPackFor: packFor,
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
		sk: sk, sched: sched, sur: sk.Surface(), review: rev,
		artifactRoot: filepath.Join(root, "artifacts")}
}

// walkToVerify walks the give-work journey to the verify tick and returns the
// task id.
func walkToVerify(t *testing.T, h *harness, family string) string {
	t.Helper()
	ctx := context.Background()
	const owner = "u-operator"
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the intake run", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v := decodeView(t, raw)
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"choice":"`+family+`"}`), false)
	if err != nil {
		t.Fatalf("answer the family question: %v", err)
	}
	v = decodeView(t, raw)
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(t, raw)
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the execute run", n)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the verify run", n)
	}
	return taskID
}

// TestRW14MechanicalDrainFailureStillCrashesForTheLadder: the drain STARTED and
// its judge died — that is what the recovery ladder is for, and a fork of it may
// well succeed. It must NOT become an infrastructure card.
func TestRW14MechanicalDrainFailureStillCrashesForTheLadder(t *testing.T) {
	ctx := context.Background()
	h := outageHarness(t, deadJudge{err: errors.New("judge transport: connection reset")}, nil)
	taskID := walkToVerify(t, h, "generic")

	r, err := h.runs.Get(ctx, taskID+".verify")
	if err != nil {
		t.Fatal(err)
	}
	if r.State != run.StateCrashed {
		t.Fatalf("verify run is %q, want crashed — a mechanical failure is the ladder's business (CONVENTIONS §21)", r.State)
	}
	var open int
	if err := h.db.QueryRowContext(ctx,
		`SELECT count(*) FROM asks a JOIN runs r ON r.run_id = a.run_id
		 WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open asks = %d — a dead judge is not a decision for a human to make", open)
	}
}

// TestRW14PackResolverRefusalBecomesTheCard: whatever the pack seam refuses
// with — here a project whose capture has no commands — reaches the operator
// VERBATIM on the card; the run parks, the board says attention, and nothing
// crashes.
func TestRW14PackResolverRefusalBecomesTheCard(t *testing.T) {
	ctx := context.Background()
	const missing = `no build, test or lint commands are captured for project "shop" — capture them, then retry`
	h := outageHarness(t, nil, func(context.Context, string, string) (*verify.CheckPack, error) {
		return nil, fmt.Errorf("%w: %s", verify.ErrNoCheckPack, missing)
	})
	taskID := walkToVerify(t, h, "software")

	r, err := h.runs.Get(ctx, taskID+".verify")
	if err != nil {
		t.Fatal(err)
	}
	if r.State != run.StateParked {
		t.Fatalf("verify run is %q, want parked on the card (S02.3 parked-on-ask)", r.State)
	}
	var snapshot string
	if err := h.db.QueryRowContext(ctx,
		`SELECT a.snapshot FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&snapshot); err != nil {
		t.Fatalf("read the card: %v", err)
	}
	var card verify.Card
	if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
		t.Fatal(err)
	}
	switch {
	case !card.Infrastructure:
		t.Fatalf("card is not the infrastructure card: %+v", card)
	case !strings.Contains(card.Summary, missing):
		t.Fatalf("card summary %q does not carry what the seam named as missing", card.Summary)
	}
	// Nothing SHIPped: no ledger item was flipped to verified, and the board
	// says a human is needed (Spec S07.1 — the card releases nothing).
	doc, found, err := h.led.Current(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("ledger doc: %v found=%v", err, found)
	}
	for _, it := range doc.State.Items {
		if it.Status == ledger.StatusVerified {
			t.Fatalf("work item %s is verified although V1 never ran", it.ID)
		}
	}
	view, err := h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	if got := decodeView(t, view).Kanban; got != "attention" {
		t.Fatalf("kanban = %q, want attention", got)
	}
}
