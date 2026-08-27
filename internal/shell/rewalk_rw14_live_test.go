package shell

// rewalk_rw14_live_test.go — P3-RW-14 Cut B live re-walk of the GPU-shop shape,
// against the REAL engine, in a fresh state dir (CONVENTIONS §10 tier-L).
//
// The journey this reproduces is the one that failed: a software task on a
// registered project, a plan whose every step declares a write_set, a
// read-only "notes" specialist sitting in the registry waiting to be matched.
// In the live world that walk routed to the read-only specialist, ran 7 engine
// sessions, reported them all "completed", billed $1.11, and left the worktree
// holding nothing but .git — then crashed at verify and wedged forever.
//
// Run:  SINET_RW14_LIVE_REWALK=1 go test ./internal/shell -run TestLiveRW14GPUShopRewalk -count=1 -v
//
// The opt-in flag is its OWN, and is required: this test bills a real engine
// call, and a paid leg must never start merely because some environment
// happened to be present in the shell that ran the battery.
//
// ONE paid call (the execute leg). Everything else — planner, critic, judge —
// is in-process, per the tier-L posture; the engine spawn is unconfined (the
// sanctioned B1-1 dev posture, S11.5: a confined engine cannot authenticate
// until the credential proxy lands).

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ---- in-process ceremony (no paid planning/critique/judging) ----

// shopPlanner emits the webshop shape: a step that DECLARES it writes, at the
// class that can (the R8/OQ4 pairing the live plan got wrong).
type shopPlanner struct{}

func (shopPlanner) Draft(_ context.Context, in intake.DraftInput) (intake.Pair, error) {
	return intake.Pair{
		Spec: intake.Spec{
			TaskID: in.Request.TaskID, Owner: in.Request.UserID,
			Version: in.SpecVersion, Status: intake.StatusDraft, Tier: in.Tier,
			Provenance:  "RW-14 re-walk in-process planner (tier-L harness)",
			Restatement: "Add a landing page to the GPU parts shop.",
			ACs: []intake.AC{
				{N: 1, Plain: "A landing page file exists in the app directory.",
					Structured: "app/index.html exists"},
			},
		},
		Plan: intake.Plan{
			TaskID: in.Request.TaskID, Owner: in.Request.UserID,
			Version: in.PlanVersion, SpecVersion: in.SpecVersion,
			Status: intake.StatusDraft, Tier: in.Tier,
			Provenance: "RW-14 re-walk in-process planner (tier-L harness)",
			// The [A15] approach is required at the boundary (P3-GF8 R11).
			Steps: []intake.Step{{
				ID: "S-1", Title: "Create the shop landing page", Class: "C2",
				DoneWhen: "app/index.html exists and names the shop",
				Approach: "I add one static HTML page under app/ and link nothing else to it yet.",
				WriteSet: []string{"app/**"},
			}},
			Coverage: map[string][]string{"AC-1": {"S-1"}},
			Est:      intake.Estimate{Known: false, Basis: "fixed re-walk plan"},
		},
	}, nil
}

func (shopPlanner) Revise(_ context.Context, in intake.ReviseInput) (intake.Pair, error) {
	return in.Pair, nil
}

type shopCritic struct{}

func (shopCritic) Critique(context.Context, intake.Pair) (intake.Verdict, error) {
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}
func (shopCritic) Recheck(context.Context, intake.Pair, []string) (intake.Verdict, error) {
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}

type shopJudge struct{}

func (shopJudge) Compliance(_ context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	ev := strings.TrimSpace(in.Artifact)
	if len(ev) > 40 {
		ev = ev[:40]
	}
	return verify.Axis1Result{Verdicts: []verify.ACVerdict{{Key: "AC-1", Pass: true, Evidence: ev}}}, nil
}

func (shopJudge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{ProbeNotes: map[verify.Probe]string{
		verify.ProbeReasonableUser: "re-walk: pass-through", verify.ProbeImplicitExpectations: "re-walk: pass-through",
		verify.ProbeSideEffects: "re-walk: pass-through", verify.ProbeExpertStandard: "re-walk: pass-through",
	}}, nil
}

func (shopJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "in-process-rewalk-judge", SelfFamily: false}
}

// devCheckRunner executes a pack check in the workspace it was handed, reading
// the verdict from the process wait status (S07.3 rule 3). Unconfined — the
// same sanctioned dev posture as the engine spawn; the C2 network-off runner
// is a host-component concern (P-T06-2), not what this walk is proving.
type devCheckRunner struct{ saw []string }

func (r *devCheckRunner) RunCheck(ctx context.Context, req verify.CheckRequest) (verify.CheckResult, error) {
	r.saw = append(r.saw, req.Workspace)
	if err := os.MkdirAll(req.EvidenceDir, 0o700); err != nil {
		return verify.CheckResult{}, err
	}
	cmd := exec.CommandContext(ctx, req.Check.Argv[0], req.Check.Argv[1:]...)
	cmd.Dir = req.Workspace
	out, _ := cmd.CombinedOutput()
	evidence := filepath.Join(req.EvidenceDir, req.Check.ID+".log")
	_ = os.WriteFile(evidence, out, 0o600)
	code := 0
	if ee, ok := cmd.ProcessState, true; ok && ee != nil {
		code = ee.ExitCode()
	}
	return verify.CheckResult{ExitCode: code, EvidenceRef: evidence}, nil
}

// reapEngineChildren kills every process this test still owns as a direct
// child, on EVERY exit path — pass, fail, panic or t.Fatal.
//
// The incident this exists to prevent: a walk that died mid-dispatch left
// billed engine sessions running with nobody waiting on them. A paid child
// outliving the test that spawned it is an orphan that bills in silence, so
// the reap is unconditional and never depends on the walk reaching its end.
// The process GROUP goes first: the engine spawns its own children, and
// killing only the parent would re-orphan them one level down.
func reapEngineChildren(t *testing.T) {
	t.Helper()
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	killed := 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		// comm (field 2) may hold spaces and parens: parse after the LAST ')'.
		close := strings.LastIndexByte(string(stat), ')')
		if close < 0 {
			continue
		}
		fields := strings.Fields(string(stat)[close+1:])
		if len(fields) < 2 {
			continue
		}
		ppid, err := strconv.Atoi(fields[1]) // state, then ppid
		if err != nil || ppid != self {
			continue
		}
		if pgid, err := syscall.Getpgid(pid); err == nil && pgid != syscall.Getpgrp() {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
		killed++
	}
	if killed > 0 {
		t.Logf("reaped %d engine child process(es) still alive at exit", killed)
	}
}

func TestLiveRW14GPUShopRewalk(t *testing.T) {
	// A PAID leg never starts from the mere presence of environment: the
	// opt-in is explicit and its own flag, so no battery run, no CI sweep and
	// no inherited shell can bill the operator by accident. (The pre-
	// registered-paid-leg discipline, applied to tests — the incident that
	// earned this line cost 204 stray worlds and five orphaned sessions.)
	if os.Getenv("SINET_RW14_LIVE_REWALK") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): the tier-L re-walk bills a real engine call and runs ONLY under the explicit opt-in SINET_RW14_LIVE_REWALK=1")
	}
	if _, err := exec.LookPath(claudecli.DefaultBinary); err != nil {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): no claude engine installed")
	}
	// Cancelling the context kills every CommandContext-spawned engine; the
	// reap then catches anything that outlived its context. Both, because
	// either alone has a hole.
	ctx, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()
	t.Cleanup(func() { cancelAll(); reapEngineChildren(t) })
	const owner = "u-operator"

	db, log, reg := seamDB(t)
	stateDir := t.TempDir()
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)
	led := ledger.NewStore(db, log)
	reviewStore := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(stateDir, "review")}
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(stateDir, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	workStore, err := worker.NewStore(worker.Config{DB: db, Log: log, Settings: reg,
		Root: filepath.Join(stateDir, "workers")})
	if err != nil {
		t.Fatalf("worker.NewStore: %v", err)
	}
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, role, created_ts) VALUES (?, ?, ?)`,
			owner, "operator", time.Now().UTC().Format(time.RFC3339Nano))
		return e
	}); err != nil {
		t.Fatalf("seed operator: %v", err)
	}

	// ---- the project: a real repo, registered, with captured commands so a
	// check pack exists and V1 actually runs (Cut A's wire). ----
	src := t.TempDir()
	rw14Git(t, src, "init", "-q", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("# GPU parts shop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rw14Git(t, src, "add", "-A")
	rw14Git(t, src, "commit", "-qm", "init")
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{
		ProjectID: "shop", Owner: owner, Name: "The GPU parts shop", Source: src, Family: "software",
	}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", owner, nil); err != nil {
		t.Fatalf("Approve project: %v", err)
	}
	// The check IS the R6 proof: it passes only if the work is present AND the
	// history is gone — run by the platform, in whatever dir V1 was handed.
	if _, err := proj.Capture(ctx, project.CaptureInput{
		ProjectID: "shop", By: owner, Family: "software",
		Commands: project.Commands{Test: "test -f app/index.html && test ! -e .git"},
	}); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// ---- the wt-notes-shaped specialist: read-only equipment, description
	// tuned to match. In the live world THIS took the job. ----
	seedNotesWorker(t, workStore, owner)

	// ---- composition ----
	pseams := &projectSeams{proj: proj, runs: runs, db: db,
		prices: metering.NewEffectiveDatedTable("empty-v0"),
		review: reviewStore, scratch: filepath.Join(stateDir, "verify-workspaces")}
	runner := &devCheckRunner{}
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters: map[string]adapters.Adapter{
			// HookCmd MUST be set explicitly in a test binary. Its fallback is
			// os.Executable()+" engine-hook" — correct in production, where the
			// executable IS sinet, but inside `shell.test` it makes the engine's
			// gate hook re-execute THE TEST BINARY, whose main ignores the
			// unknown argument and runs the suite again: a new world, a new
			// billed engine session, recursively, once per hook invocation.
			// That is the real mechanism behind this file's stray-world
			// incident, and it is why every other stage harness pins this
			// string. An env opt-in cannot prevent it — the child inherits it.
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Settings: reg, HookCmd: "/opt/sinet/bin/sinet engine-hook",
			},
		},
		ArtifactRoot: filepath.Join(stateDir, "artifacts"),
		RunRoot:      filepath.Join(stateDir, "runs"),
		CopyAsideDir: filepath.Join(stateDir, "copy-aside"),
		Planner:      shopPlanner{}, Critic: shopCritic{}, Judge: shopJudge{},
		Review:  reviewStore,
		Workers: workStore,

		Registry:          registrySeam{proj: proj},
		Fingerprint:       pseams.Fingerprint,
		Snapshot:          pseams.Snapshot,
		CreateRevisionRef: pseams.CreateRevisionRef,
		BaseContent:       pseams,
		WorkspaceCwd:      pseams.WorkspaceCwd,
		CheckPackFor:      pseams.CheckPackFor,
		CheckRunner:       runner,
		// The two Cut-B seams under test.
		VerificationWorkspace: pseams.VerificationWorkspace,
		RepoFacts:             pseams.RepoFacts,
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	pseams.pipe = sk.Pipeline()
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     10 * time.Minute,
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	sur := sk.Surface()

	tick := func(label string) int {
		t.Helper()
		n, err := sched.Tick(ctx)
		if err != nil {
			t.Fatalf("Tick(%s): %v", label, err)
		}
		sched.WaitInFlight()
		t.Logf("tick %-8s dispatched %d", label, n)
		return n
	}

	// ---- the walk ----
	raw, err := sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"Shop landing page","text":"Add a landing page to the GPU parts shop.","project":"shop"}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	var v struct {
		TaskID    string `json:"task_id"`
		OpenAskID string `json:"open_ask_id"`
		OpenCard  struct {
			Kind string `json:"kind"`
		} `json:"open_card"`
		Kanban string `json:"kanban"`
	}
	dec := func() {
		t.Helper()
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	dec()
	taskID := v.TaskID
	t.Logf("task %s", taskID)
	tick("intake")

	// Answer every open card until the plan is approved.
	for i := 0; i < 6; i++ {
		if raw, err = sur.Task(ctx, taskID); err != nil {
			t.Fatalf("Task: %v", err)
		}
		dec()
		if v.OpenAskID == "" {
			break
		}
		var body json.RawMessage
		step := false
		switch v.OpenCard.Kind {
		case "decision.family":
			body = json.RawMessage(`{"choice":"software"}`)
		case "interview":
			body = json.RawMessage(`{"force_proceed":true}`)
		case "approval":
			body, step = json.RawMessage(`{"action":"approve"}`), true
		default:
			t.Fatalf("unexpected card %q", v.OpenCard.Kind)
		}
		if raw, err = sur.Answer(ctx, owner, v.OpenAskID, body, step); err != nil {
			t.Fatalf("Answer(%s): %v", v.OpenCard.Kind, err)
		}
		if step {
			break
		}
	}

	// ---- R8/R9: what the router decided, and why ----
	// The routing block rides the APPROVAL card — visible and overridable
	// BEFORE anything runs (S08.8 7.7 accountability), which is where a human
	// would have caught the live defect for free.
	var routing string
	if err := db.QueryRowContext(ctx,
		`SELECT a.snapshot FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.snapshot LIKE '%"routing"%'
		  ORDER BY a.rowid DESC LIMIT 1`, taskID).Scan(&routing); err != nil {
		t.Fatalf("approval card with a routing block: %v", err)
	}
	t.Logf("APPROVAL CARD: %s", routing)
	var card struct {
		Approval struct {
			Routing struct {
				Generalist  bool   `json:"generalist"`
				WorkerName  string `json:"worker_name"`
				PlainReason string `json:"plain_reason"`
			} `json:"routing"`
		} `json:"approval"`
	}
	if err := json.Unmarshal([]byte(routing), &card); err != nil {
		t.Fatalf("decode approval card: %v", err)
	}
	rd := card.Approval.Routing
	if !rd.Generalist {
		t.Fatalf("a writing plan routed to specialist %q — R8 did not hold live", rd.WorkerName)
	}
	// The BY-NAME guarantee (§19 no-fit discipline): the card must say which
	// specialist was refused and that it could not write. "No specialist fits"
	// is what the operator saw before this packet, and it is indistinguishable
	// from "nobody matched" — the one fact that would have explained the live
	// $1.11 is the culprit's own name.
	if !strings.Contains(rd.PlainReason, "release-notes-writer") {
		t.Errorf("the reason never NAMES the refused specialist — an operator cannot act on an anonymous refusal: %q", rd.PlainReason)
	}
	if !strings.Contains(strings.ToLower(rd.PlainReason), "write") {
		t.Errorf("the reason never says the refusal was about writing: %q", rd.PlainReason)
	}

	// ---- the one paid call ----
	tick("execute")
	execRun := taskID + ".execute"
	r, err := runs.Get(ctx, execRun)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}
	t.Logf("execute run state=%s", r.State)

	var spend float64
	var donePayload string
	if err := db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = 'engine.done' ORDER BY event_seq DESC LIMIT 1`,
		execRun).Scan(&donePayload); err == nil {
		var usage struct {
			TotalCostUSD float64 `json:"total_cost_usd"`
		}
		_ = json.Unmarshal([]byte(donePayload), &usage)
		spend = usage.TotalCostUSD
	}
	t.Logf("SPEND (engine-reported): $%.4f", spend)

	// Did the billed work actually LAND? The live world's answer was no.
	ws, ok, err := proj.ExistingWorkspace(ctx, "shop", taskID)
	if err != nil || !ok {
		t.Fatalf("no worktree for the task: ok=%v err=%v", ok, err)
	}
	var landed []string
	_ = filepath.WalkDir(ws, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(ws, p)
		if !strings.HasPrefix(rel, ".git") {
			landed = append(landed, rel)
		}
		return nil
	})
	t.Logf("WORKTREE after execute: %v", landed)

	// ASSERTED, not logged: this is the acceptance question. The seed README
	// does not count — the engine must have put something NEW on disk, which
	// is exactly what 7 sessions and $1.11 failed to do in the live world.
	var written []string
	for _, f := range landed {
		if f != "README.md" {
			written = append(written, f)
		}
	}
	if len(written) == 0 {
		t.Fatalf("execute completed and billed but wrote NOTHING into the worktree (contents: %v) — the RW-14 defect, live", landed)
	}

	snapshot, base, err := proj.SnapshotAndBase(ctx, "shop", taskID)
	if err != nil {
		t.Fatalf("SnapshotAndBase: %v", err)
	}
	t.Logf("snapshot=%s base=%s wrote_nothing=%v", short(snapshot), short(base), snapshot == base)
	if snapshot == "" || base == "" {
		t.Fatalf("a repo-backed task produced no snapshot/base pair (snapshot=%q base=%q) — R7 would have no facts", snapshot, base)
	}
	if snapshot == base {
		t.Fatalf("the tree never left the base commit (%s) although files landed — snapshot and worktree disagree", short(base))
	}

	// ---- verify ----
	tick("verify")
	vr, err := runs.Get(ctx, taskID+".verify")
	if err != nil {
		t.Fatalf("verify run: %v", err)
	}
	t.Logf("verify run state=%s", vr.State)
	t.Logf("V1 check workspaces: %v", runner.saw)
	// The verify leg must REACH a terminal, not wedge — the Cut A invariant
	// this journey exists to keep: never crashed, never tombstoned.
	switch vr.State {
	case run.StateCrashed, run.StateTombstoned:
		t.Fatalf("verify run is %q — the wedge is back", vr.State)
	case run.StateCompleted, run.StateParked:
		// completed = a verdict landed; parked = a card the operator can answer.
	default:
		t.Fatalf("verify run is %q — neither a verdict nor an answerable door", vr.State)
	}
	// R6, asserted: the checks ran, and they ran in the materialized revision.
	if len(runner.saw) == 0 {
		t.Fatal("no V1 check ever ran — the pack never reached the runner, so R6 proved nothing")
	}
	for _, dir := range runner.saw {
		if strings.Contains(dir, filepath.Join("runs", execRun, "cwd")) {
			t.Errorf("V1 ran in the execute scratch cwd %q — R6 did not hold live", dir)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
			t.Errorf("V1 workspace %q still carries VCS history (err %v) — S07.3 rule 1 did not hold live", dir, err)
		}
	}

	if raw, err = sur.Task(ctx, taskID); err != nil {
		t.Fatalf("Task: %v", err)
	}
	dec()
	t.Logf("KANBAN: %s", v.Kanban)

	rows, _ := db.QueryContext(ctx,
		`SELECT e.type, count(*) FROM run_events e JOIN runs r ON r.run_id = e.run_id
		  WHERE r.task_id = ? GROUP BY e.type ORDER BY e.type`, taskID)
	defer rows.Close()
	for rows.Next() {
		var ty string
		var n int
		_ = rows.Scan(&ty, &n)
		t.Logf("  event %-28s %d", ty, n)
	}
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	if sha == "" {
		return "(none)"
	}
	return sha
}

// seedNotesWorker registers the live world's actual culprit: a release-notes
// specialist granted {Read, Grep, Glob} at C1 — no write instrument, read-only
// workspace — whose description echoes the task's own words.
func seedNotesWorker(t *testing.T, store *worker.Store, owner string) {
	t.Helper()
	ctx := context.Background()
	src := `---
name: release-notes-writer
description: Writes release notes and landing page copy for shop projects
kind: agentic
domain: software
selectors:
  family: build-create
  task_classes: [landing page]
  triggers: [landing page]
profile:
  duty: execution
equipment:
  tools: [Read, Grep, Glob]
---
Summarize the merged work as release notes.
`
	_, v, err := store.CreateDraft(ctx, owner, src, worker.RequestedGrants{
		Tools: []string{"Read", "Grep", "Glob"}, Class: "C1", Egress: worker.EgressNone,
	}, worker.Provenance{AuthorKind: "human", Origin: worker.OriginHumanWritten})
	if err != nil {
		t.Fatalf("CreateDraft(notes): %v", err)
	}
	res, err := store.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: owner, SampleTask: "write sample notes", Engine: rewalkDry{},
		Model: "claude-haiku-4-5", EnginePin: claudecli.Pin,
	})
	if err != nil {
		t.Fatalf("RunBattery(notes): %v", err)
	}
	if !res.Green {
		t.Fatalf("notes battery not green: %+v", res)
	}
	if _, err := store.Approve(ctx, owner, v.ID,
		worker.ApproveOpts{AckFlagged: res.Audit.FlaggedItems()}); err != nil {
		t.Fatalf("Approve(notes): %v", err)
	}
}

type rewalkDry struct{}

func (rewalkDry) DryRun(context.Context, worker.DryRunRequest) (worker.DryRunResult, error) {
	return worker.DryRunResult{Completed: true, TranscriptRef: "fake://transcript",
		Output: "notes: ok", CostUSD: 0}, nil
}
