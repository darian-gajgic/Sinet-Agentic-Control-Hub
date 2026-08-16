package stage_test

import (
	"context"
	"encoding/json"
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

// P3-RW-14 R6/R5-edge acceptance — what the V1 checks are actually pointed at,
// and what happens when a launch-domain project has nothing to check with.
//
// The live defect R6 closes: for a project-backed task the real work lives in
// the git worktree, while verifyInput handed V1 `RunRoot/<executeRunID>/cwd` —
// the execute leg's plain scratch dir, which for such a task is EMPTY. Cut A
// wired check packs, which made that latent hazard load-bearing: a project
// WITH captured commands would run its whole suite against an empty tree and
// fail work that was in fact done. Spec S07.3 rule 1 names the correct
// substrate — the revision's content with answer-bearing VCS history STRIPPED,
// so a pass measures the work and not the repo's own history.

// recordingRunner captures the workspace each check was pointed at.
type recordingRunner struct{ workspaces []string }

func (r *recordingRunner) RunCheck(_ context.Context, req verify.CheckRequest) (verify.CheckResult, error) {
	r.workspaces = append(r.workspaces, req.Workspace)
	return verify.CheckResult{ExitCode: 0, EvidenceRef: "evidence/" + req.Check.ID + ".log"}, nil
}

// passJudge is a wired judge that passes every criterion, so the drain runs
// past V1 to a terminal instead of stopping on a judge that is not there.
type passJudge struct{}

func (passJudge) Compliance(_ context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	var out verify.Axis1Result
	for _, ac := range in.ACs {
		out.Verdicts = append(out.Verdicts, verify.ACVerdict{
			Key: fmt.Sprintf("AC-%d", ac.N), Pass: true, Evidence: in.Artifact,
		})
	}
	return out, nil
}

func (passJudge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{ProbeNotes: map[verify.Probe]string{
		verify.ProbeReasonableUser:       "done and good",
		verify.ProbeImplicitExpectations: "nothing absent",
		verify.ProbeSideEffects:          "no unrequested changes",
		verify.ProbeExpertStandard:       "competent",
	}}, nil
}

func (passJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "fake-judge-1", SelfFamily: true}
}

// seamHarness is the walking skeleton with the verification seams this packet
// touches substituted: the pack resolver, the check runner, and the R6
// verification-workspace materializer.
func seamHarness(
	t *testing.T,
	packFor func(ctx context.Context, domain, taskID string) (*verify.CheckPack, error),
	runner verify.CheckRunner,
	ws func(ctx context.Context, taskID string, revision int) (string, func(), error),
) *harness {
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
		Judge:        passJudge{},
		CheckPackFor: packFor,
		CheckRunner:  runner,

		VerificationWorkspace: ws,
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

// softwarePack is the shape the composition root builds from a project's
// captured commands (internal/shell packChecks) — one static rung that the
// recording runner answers 0 for.
func softwarePack() *verify.CheckPack {
	return &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-24 * time.Hour),
		Checks: []verify.Check{
			{ID: "build", Stage: verify.StageStatic, Argv: []string{"/bin/sh", "-lc", "go build ./..."},
				FindingCategory: verify.CatACBlocker},
		},
	}
}

// TestRW14VerifyWorkspaceIsStrippedRevision: for a project-backed task the V1
// checks run against the materialized revision — its files present, its VCS
// history absent — and NEVER against the execute leg's scratch cwd.
//
// This binds the plumbing end to end: that the seam is consulted, that what it
// returns is what the CheckRunner is pointed at, that the scratch cwd is not,
// and that the checks can see no `.git`. The real materialization it stands in
// for (a locked utility checkout at the revision's SnapshotSHA, copied without
// `.git` — CONVENTIONS §25) is bound over a real project store in
// internal/shell.
func TestRW14VerifyWorkspaceIsStrippedRevision(t *testing.T) {
	runner := &recordingRunner{}

	// The materialized revision: the work that execute actually did, with the
	// answer-bearing history stripped off (Spec S07.3 rule 1, P-T06-2).
	var (
		asked    []string
		released int
	)
	materialize := func(_ context.Context, taskID string, revision int) (string, func(), error) {
		asked = append(asked, fmt.Sprintf("%s@%d", taskID, revision))
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "app"), 0o700); err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, "app", "main.go"),
			[]byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
			return "", nil, err
		}
		return dir, func() { released++ }, nil
	}

	h := seamHarness(t, func(context.Context, string, string) (*verify.CheckPack, error) {
		return softwarePack(), nil
	}, runner, materialize)
	taskID := walkToVerify(t, h, "software")

	if len(runner.workspaces) == 0 {
		t.Fatal("no check ever ran — the V1 leg never reached the runner")
	}
	scratch := filepath.Join(filepath.Dir(h.artifactRoot), "runs", taskID+".execute", "cwd")
	for i, ws := range runner.workspaces {
		if ws == scratch {
			t.Fatalf("check %d ran in the execute leg's scratch cwd %q — the empty dir, not the revision", i, ws)
		}
		if _, err := os.Stat(filepath.Join(ws, "app", "main.go")); err != nil {
			t.Errorf("check %d ran in %q, which does not hold the revision's files: %v", i, ws, err)
		}
		if _, err := os.Stat(filepath.Join(ws, ".git")); !os.IsNotExist(err) {
			t.Errorf("check %d ran against a workspace that still carries VCS history (.git present) — "+
				"a pass there can measure lookup, not work (Spec S07.3 rule 1)", i)
		}
	}
	if len(asked) == 0 {
		t.Fatal("the verification-workspace seam was never consulted")
	}
	if !strings.HasPrefix(asked[0], taskID+"@") {
		t.Errorf("the seam was asked for %q, want the task under review", asked[0])
	}
	if released == 0 {
		t.Error("the materialized workspace was never released — a locked checkout would leak")
	}
}

// TestRW14NoCommandsProjectCardsNotCrashes: the live GPU-shop shape — a
// software (launch-domain) task on a project whose registry capture holds NO
// commands. The platform has nothing to check the work with, so it says so on
// a card naming exactly what to capture; it never crashes into the recovery
// ladder, and it never ships anything unverified.
//
// The refusal WORDING is derived from a real command-less capture in
// internal/shell (TestRW14NoCapturedCommandsNamesWhatToCapture); this binds
// what the pipeline does with it.
func TestRW14NoCommandsProjectCardsNotCrashes(t *testing.T) {
	ctx := context.Background()
	const missing = `no build, test or lint commands are captured for project "shop", so there is nothing to check this work with — capture them for the project (its Commands), then retry`

	runner := &recordingRunner{}
	h := seamHarness(t, func(context.Context, string, string) (*verify.CheckPack, error) {
		return nil, fmt.Errorf("%w: %s", verify.ErrNoCheckPack, missing)
	}, runner, nil)
	taskID := walkToVerify(t, h, "software")

	// Nothing crashed and nothing was tombstoned: a missing check inventory is
	// a screen OUTAGE, which escalates — it is not a fault to fork (S07.2).
	rows, err := h.db.QueryContext(ctx,
		`SELECT run_id, state FROM runs WHERE task_id = ?`, taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	states := map[string]string{}
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			t.Fatal(err)
		}
		states[id] = st
	}
	for id, st := range states {
		if st == string(run.StateCrashed) || st == string(run.StateTombstoned) {
			t.Errorf("run %s is %q — a command-less project is a decision for a human, not a corpse for the ladder", id, st)
		}
	}
	if got := states[taskID+".verify"]; got != string(run.StateParked) {
		t.Fatalf("verify run is %q, want parked on the card (S02.3 parked-on-ask)", got)
	}

	// The door names exactly what to capture.
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
	if !card.Infrastructure {
		t.Fatalf("card is not the verification-infrastructure card: %+v", card)
	}
	if !strings.Contains(card.Summary, "commands are captured") {
		t.Fatalf("card summary %q does not tell the operator what to capture", card.Summary)
	}

	// Nothing ran and nothing shipped (Spec S07.1: the card releases nothing).
	if len(runner.workspaces) != 0 {
		t.Errorf("%d check(s) ran although no pack resolved", len(runner.workspaces))
	}
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
