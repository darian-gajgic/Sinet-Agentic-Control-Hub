package stage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// Admitter is the scheduler surface the skeleton drives: the sole run
// ingress (Spec S16.6) plus the out-of-dispatch settle (Spec S10.1).
// *scheduler.Scheduler satisfies it; bound late because the scheduler's
// own Config carries the skeleton as its Dispatcher.
type Admitter interface {
	Enqueue(ctx context.Context, runID string, class scheduler.WorkloadClass) error
	SettleRun(ctx context.Context, runID string) error
}

// Skeleton is the walking-skeleton composition (see the package comment).
type Skeleton struct {
	cfg    Config
	driver *adapters.Driver
	pipe   *intake.Pipeline
	sched  Admitter
}

var _ scheduler.Dispatcher = (*Skeleton)(nil)

// New assembles the skeleton. The intake pipeline is built here with its
// engine seams wired (Planner/Critic per S06.10; overridable via Config);
// Utility stays NIL — S06.10 pins it to the LOCAL tier (Spec S12, B4) and
// wiring it to a paid engine would violate the ceremony cut line, so the
// pipeline's deterministic fallback covers card phrasing until then.
// Classifier/Registry/SpotCheck are likewise B4/S13 seams: their absence
// degrades exactly as the spec directs (fail-closed high tier, S06.2).
func New(cfg Config) (*Skeleton, error) {
	switch {
	case cfg.DB == nil, cfg.Log == nil, cfg.Runs == nil, cfg.Checkpoints == nil,
		cfg.Ledger == nil, cfg.Settings == nil:
		return nil, errors.New("stage: DB, Log, Runs, Checkpoints, Ledger and Settings are required")
	case len(cfg.Adapters) == 0:
		return nil, errors.New("stage: at least one adapter must be registered (Spec S03.2)")
	case cfg.ArtifactRoot == "" || cfg.RunRoot == "":
		return nil, errors.New("stage: ArtifactRoot and RunRoot are required")
	}
	if cfg.Substrate == "" {
		cfg.Substrate = adapters.SubstrateClaudeCLI
	}
	if cfg.Lane == "" {
		cfg.Lane = adapters.LaneAnthropic
	}
	if _, ok := cfg.Adapters[cfg.Substrate]; !ok {
		return nil, fmt.Errorf("stage: configured substrate %q has no registered adapter", cfg.Substrate)
	}
	s := &Skeleton{cfg: cfg}
	s.driver = &adapters.Driver{
		Runs:         cfg.Runs,
		Checkpoints:  cfg.Checkpoints,
		Log:          cfg.Log,
		DB:           cfg.DB,
		CopyAsideDir: cfg.CopyAsideDir,
		// The S02.4(c) ledger-revision block goes LIVE here (B2-1 seam →
		// B2-4 wiring): every paid-call checkpoint embeds the current
		// ledger revision ref.
		LedgerRevision: cfg.Ledger.CheckpointRef,
	}
	s.pipe = &intake.Pipeline{
		DB:           cfg.DB,
		Log:          cfg.Log,
		Runs:         cfg.Runs,
		Ledger:       cfg.Ledger,
		Settings:     cfg.Settings,
		ArtifactRoot: cfg.ArtifactRoot,
		Planner:      cfg.Planner,
		Critic:       cfg.Critic,
		Now:          cfg.Now,
	}
	if s.pipe.Planner == nil {
		s.pipe.Planner = &EnginePlanner{s: s}
	}
	if s.pipe.Critic == nil {
		s.pipe.Critic = &EngineCritic{s: s}
	}
	return s, nil
}

// Bind attaches the scheduler (late-bound: the scheduler's Config carries
// this skeleton as its Dispatcher). Must be called before any Submit.
func (s *Skeleton) Bind(a Admitter) { s.sched = a }

// Pipeline exposes the intake pipeline (the ledger assembly's Plan source
// and tests compose against it).
func (s *Skeleton) Pipeline() *intake.Pipeline { return s.pipe }

func (s *Skeleton) logger() *slog.Logger {
	if s.cfg.Logger != nil {
		return s.cfg.Logger
	}
	return slog.Default()
}

func (s *Skeleton) now() time.Time {
	if s.cfg.Now != nil {
		return s.cfg.Now()
	}
	return time.Now()
}

// ---- run roles ----

type role string

const (
	roleIntake  role = "intake"
	roleExecute role = "execute"
	roleVerify  role = "verify"
)

func runRole(runID string) (role, bool) {
	switch {
	case strings.HasSuffix(runID, RunSuffixIntake):
		return roleIntake, true
	case strings.HasSuffix(runID, RunSuffixExecute):
		return roleExecute, true
	case strings.HasSuffix(runID, RunSuffixVerify):
		return roleVerify, true
	}
	return "", false
}

// ---- scheduler.Dispatcher ----

// Dispatch routes a CAS-claimed run by its role (the walking-skeleton
// run-naming convention; a first-class role record is S04/S08 machinery,
// B3). Unroutable runs error loudly — nothing is silently absorbed.
func (s *Skeleton) Dispatch(ctx context.Context, r run.Run) error {
	rl, ok := runRole(r.ID)
	if !ok {
		return fmt.Errorf("stage: run %q has no skeleton role (want *%s|*%s|*%s)", r.ID,
			RunSuffixIntake, RunSuffixExecute, RunSuffixVerify)
	}
	switch rl {
	case roleIntake:
		return s.dispatchIntake(ctx, r)
	case roleExecute:
		return s.dispatchExecute(ctx, r)
	default:
		return s.dispatchVerify(ctx, r)
	}
}

// crash marks a dispatched run crashed with its cause — the Driver's
// spawn-failure precedent: a dispatch leg that cannot proceed leaves a
// classifiable corpse for the recovery ladder, never a silent zombie.
func (s *Skeleton) crash(ctx context.Context, runID, cause string) {
	if _, err := s.cfg.Runs.Transition(ctx, runID, run.StateCrashed, run.TransitionOptions{
		Reason: "stage dispatch failed", Actor: run.ActorPlatform,
		Detail: reasonDetail(cause),
	}); err != nil {
		s.logger().Error("stage: crash transition", "run", runID, "err", err)
	}
}

func reasonDetail(cause string) json.RawMessage {
	b, err := json.Marshal(struct {
		Cause string `json:"cause"`
	}{cause})
	if err != nil {
		return nil
	}
	return b
}

// ---- intake leg ----

func (s *Skeleton) dispatchIntake(ctx context.Context, r run.Run) error {
	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateRunning, run.TransitionOptions{
		Reason: "intake stage work (S06.1)", Actor: run.ActorPlatform,
	}); err != nil {
		return err
	}
	st, err := s.pipe.Advance(ctx, r.TaskID)
	if err != nil {
		s.crash(ctx, r.ID, "intake advance: "+err.Error())
		return fmt.Errorf("stage: intake advance: %w", err)
	}
	return s.afterIntake(ctx, st)
}

// afterIntake continues from an intake state: an open card waits (the run
// is parked — gates wait, Spec S06.1); an approved plan completes the
// intake run and launches execution.
func (s *Skeleton) afterIntake(ctx context.Context, st *intake.State) error {
	switch {
	case st.OpenAskID != "":
		s.logger().Info("stage: intake gate open", "task", st.TaskID, "card", string(st.OpenAskKind), "ask", st.OpenAskID)
		return nil
	case st.Phase == intake.PhaseApproved:
		return s.finishIntake(ctx, st)
	case st.Phase == intake.PhaseCancelled:
		s.setKanban(ctx, st.TaskID, "cancelled")
		return nil
	default:
		return nil
	}
}

// finishIntake completes the intake run (its work — an approved plan —
// exists), settles its queue row + ceremony receipt, and launches the
// execution run. Idempotent across the dispatch and answer paths.
func (s *Skeleton) finishIntake(ctx context.Context, st *intake.State) error {
	r, err := s.cfg.Runs.Get(ctx, st.RunID)
	if err != nil {
		return err
	}
	if r.State == run.StateRunning {
		if _, err := s.cfg.Runs.Transition(ctx, st.RunID, run.StateCompleted, run.TransitionOptions{
			Reason: "intake complete: plan approved (S06.9)", Actor: run.ActorPlatform,
		}); err != nil {
			return err
		}
	}
	if s.sched != nil {
		if err := s.sched.SettleRun(ctx, st.RunID); err != nil {
			s.logger().Warn("stage: settle intake run", "run", st.RunID, "err", err)
		}
	}
	s.setKanban(ctx, st.TaskID, "executing")
	return s.launchRole(ctx, st.TaskID, st.Owner, roleExecute)
}

// launchRole creates and enqueues a role run for the task (Spec S16.6:
// Enqueue is the sole ingress). An existing live run for the role is
// re-enqueued only from new/queued; anything else is reported.
func (s *Skeleton) launchRole(ctx context.Context, taskID, owner string, rl role) error {
	if s.sched == nil {
		return errors.New("stage: no scheduler bound (Bind was not called)")
	}
	runID := taskID + "." + string(rl)
	_, err := s.cfg.Runs.Create(ctx, run.NewRun{
		ID:        runID,
		UserID:    owner,
		TaskID:    taskID,
		Substrate: s.cfg.Substrate,
		Lane:      s.cfg.Lane,
	})
	if err != nil && !errors.Is(err, run.ErrExists) {
		return fmt.Errorf("stage: create %s run: %w", rl, err)
	}
	// The requester is waiting on this task end-to-end: interactive class
	// (Spec S10.7 ladder; class refinement is S04/S08 policy, B3).
	if err := s.sched.Enqueue(ctx, runID, scheduler.ClassInteractive); err != nil {
		return fmt.Errorf("stage: enqueue %s run: %w", rl, err)
	}
	s.logger().Info("stage: launched role run", "run", runID, "role", string(rl))
	return nil
}

// ---- execute leg ----

// execTools is the execution-session toolset at B2-4: workspace-local
// editing only. No Bash (no shell until effect gating composes with the
// executor, Spec S02.7/S03.4 — outward effects stay structurally
// unreachable), no network tools (the egress substrate is a deferred host
// change, Spec S11.4). Worker-template toolsets are Spec S08's (B3).
var execTools = []string{"Read", "Write", "Edit"}

// execPermissionMode is the engine-side consent matching exactly that
// toolset in headless sessions: "acceptEdits" auto-accepts workspace-local
// file writes/edits, everything else still denies. Without it a
// non-interactive session's Write permission prompt auto-denies and the
// executor can only plead for access (found live at the B2 gate demo,
// 2026-07-20). Consent is cooperation, not the boundary — enforcement is
// the confinement class (Spec S11.1).
const execPermissionMode = "acceptEdits"

func (s *Skeleton) dispatchExecute(ctx context.Context, r run.Run) error {
	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateRunning, run.TransitionOptions{
		Reason: "execute stage sessions (S05.3)", Actor: run.ActorPlatform,
	}); err != nil {
		return err
	}
	pair, _, err := s.pipe.ApprovedPair(ctx, r.TaskID)
	if err != nil {
		// D10: nothing executes without an approved plan.
		s.crash(ctx, r.ID, "no approved plan: "+err.Error())
		return fmt.Errorf("stage: execute without approved plan: %w", err)
	}

	// Ledger work items for the plan steps (Spec S05.1 §4): the platform
	// records the stage sessions' claims — the session-verb TOOL channel
	// (engine-called verbs) is S04 orchestration machinery (B3).
	gen := r.Generation
	verbs := s.cfg.Ledger.SessionVerbs(r.ID, "execute", gen)
	items := make([]ledger.WorkItem, 0, len(pair.Plan.Steps))
	acRefs := coverageByStep(&pair.Plan)
	for _, step := range pair.Plan.Steps {
		items = append(items, ledger.WorkItem{
			ID: step.ID, Summary: step.Title, ACRefs: acRefs[step.ID], Status: ledger.StatusPending,
		})
	}
	first := pair.Plan.Steps[0].ID
	if _, err := verbs.State(ctx, ledger.StateUpdate{Upserts: items, Current: &first}); err != nil {
		s.crash(ctx, r.ID, "seed work items: "+err.Error())
		return err
	}

	// One fresh engine session per plan step (Spec S05.3 execute-N): the
	// stage name IS the step id, so the assembly's Plan source injects
	// that step's contract (Spec S05.4).
	deliverable := ""
	overflows := 0
	for i, step := range pair.Plan.Steps {
		instructions := stageMarker(markerExecute) + fmt.Sprintf(
			"You are executing plan step %s of task %s: %s\n"+
				"Done when: %s\n"+
				"Work in your working directory. When the step is complete, output the step's "+
				"complete deliverable content as your final message — full content, no commentary wrapper.\n",
			step.ID, r.TaskID, step.Title, step.DoneWhen)
		res, err := s.Session(ctx, SessionInput{
			RunID:          r.ID,
			Stage:          step.ID,
			Assemble:       true,
			Sources:        ledger.Sources{Plan: &intake.PlanSource{P: s.pipe}},
			Instructions:   instructions,
			Class:          step.Class,
			Tools:          execTools,
			PermissionMode: execPermissionMode,
			PriorOverflows: overflows,
		})
		if err != nil {
			s.crash(ctx, r.ID, fmt.Sprintf("step %s session: %v", step.ID, err))
			return fmt.Errorf("stage: execute step %s: %w", step.ID, err)
		}
		if res.Budget.Overflowed {
			overflows++
		}
		deliverable = res.Text
		next := ""
		if i+1 < len(pair.Plan.Steps) {
			next = pair.Plan.Steps[i+1].ID
		}
		done := ledger.WorkItem{ID: step.ID, Status: ledger.StatusDoneUnverified}
		if _, err := verbs.State(ctx, ledger.StateUpdate{Upserts: []ledger.WorkItem{done}, Current: &next}); err != nil {
			s.crash(ctx, r.ID, "record step completion: "+err.Error())
			return err
		}
	}

	// Deliverable of record: the final step session's result text, durably
	// filed and ledger-registered. Revision/diff/comment mechanics are
	// Spec S13's (B4) — this is the walking-skeleton deliverable capture.
	path, sha, err := s.writeDeliverable(r.TaskID, 1, deliverable)
	if err != nil {
		s.crash(ctx, r.ID, "write deliverable: "+err.Error())
		return err
	}
	if _, err := verbs.Artifact(ctx, path, "deliverable", "execution deliverable rev1 (B2-4 capture; S13 owns revisions)", sha); err != nil {
		s.crash(ctx, r.ID, "register deliverable: "+err.Error())
		return err
	}
	// Stage close (Spec S05.1): every assigned item accounted for, verify
	// handed forward.
	empty := ""
	next := []string{"verify deliverable against the frozen ACs (S07)"}
	doc, err := verbs.State(ctx, ledger.StateUpdate{Current: &empty, NextActions: &next})
	if err != nil {
		s.crash(ctx, r.ID, "stage close: "+err.Error())
		return err
	}
	assigned := make([]string, 0, len(pair.Plan.Steps))
	for _, step := range pair.Plan.Steps {
		assigned = append(assigned, step.ID)
	}
	if err := ledger.CheckStageClose(doc, assigned); err != nil {
		s.crash(ctx, r.ID, "stage-close gate: "+err.Error())
		return err
	}

	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateCompleted, run.TransitionOptions{
		Reason: "execution complete: deliverable produced", Actor: run.ActorPlatform,
	}); err != nil {
		return err
	}
	s.setKanban(ctx, r.TaskID, "verifying")
	return s.launchRole(ctx, r.TaskID, r.UserID, roleVerify)
}

// coverageByStep inverts the plan's AC coverage map: step id → AC numbers.
func coverageByStep(p *intake.Plan) map[string][]int {
	out := map[string][]int{}
	for key, steps := range p.Coverage {
		var n int
		if _, err := fmt.Sscanf(key, "AC-%d", &n); err != nil {
			continue
		}
		for _, sid := range steps {
			out[sid] = append(out[sid], n)
		}
	}
	return out
}

func (s *Skeleton) writeDeliverable(taskID string, revision int, content string) (path, sha string, err error) {
	dir := filepath.Join(s.cfg.ArtifactRoot, sanitizePathComponent(taskID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("stage: deliverable dir: %w", err)
	}
	path = filepath.Join(dir, "deliverable-rev"+strconv.Itoa(revision)+".md")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return "", "", fmt.Errorf("stage: write deliverable: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", "", fmt.Errorf("stage: write deliverable: %w", err)
	}
	sum := sha256.Sum256([]byte(content))
	return path, hex.EncodeToString(sum[:]), nil
}

func (s *Skeleton) readDeliverable(taskID string, revision int) (string, error) {
	path := filepath.Join(s.cfg.ArtifactRoot, sanitizePathComponent(taskID), "deliverable-rev"+strconv.Itoa(revision)+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("stage: read deliverable: %w", err)
	}
	return string(raw), nil
}

// ---- verify leg ----

// domainFor maps the intake family to the verification domain (Spec S07.8
// launch roster: software at v0; web-research at v0.1). Non-launch domains
// verify in the ratified degraded mode (V1 empty) until their packs arrive.
func domainFor(f intake.Family) string {
	switch f {
	case intake.FamilySoftware:
		return verify.DomainSoftware
	case intake.FamilyResearch:
		return verify.DomainWebResearch
	default:
		return string(f)
	}
}

func (s *Skeleton) dispatchVerify(ctx context.Context, r run.Run) error {
	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateRunning, run.TransitionOptions{
		Reason: "verification drain (S07.1)", Actor: run.ActorPlatform,
	}); err != nil {
		return err
	}
	pair, st, err := s.pipe.ApprovedPair(ctx, r.TaskID)
	if err != nil {
		s.crash(ctx, r.ID, "no approved plan: "+err.Error())
		return fmt.Errorf("stage: verify without approved plan: %w", err)
	}
	content, err := s.readDeliverable(r.TaskID, 1)
	if err != nil {
		s.crash(ctx, r.ID, err.Error())
		return err
	}
	domain := domainFor(st.Family)

	judge := s.cfg.Judge
	if judge == nil {
		judge = &EngineJudge{s: s}
	}
	revise := s.cfg.Revise
	if revise == nil {
		revise = s.engineRevise
	}
	v := &verify.Verifier{
		DB:       s.cfg.DB,
		Log:      s.cfg.Log,
		Ledger:   s.cfg.Ledger,
		Settings: s.cfg.Settings,
		Judge:    judge,
		Pack:     s.cfg.CheckPacks[domain],
		Runner:   s.cfg.CheckRunner,
		// Research counters stay nil at B2-4: stage sessions exist now,
		// but the research TOOL substrate (WebSearch/egress) does not —
		// nodes record UNVERIFIABLE-HERE loudly, never a fake pass
		// (Spec S07.3; flagged to the B2 gate with the egress batch).
		Revise: revise,
		Now:    s.cfg.Now,
	}
	execCwd := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(r.TaskID+RunSuffixExecute), "cwd")
	evidence := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(r.ID), "evidence")
	if err := os.MkdirAll(evidence, 0o700); err != nil {
		return err
	}
	out, err := v.Verify(ctx, verify.VerifyInput{
		Deliverable: verify.Deliverable{
			TaskID: r.TaskID,
			// The drain's events, judge assemblies and checkpoints ride
			// THIS run: it is running through the drain (Spec S02.4
			// checkpoint writability) and its consumption is the
			// verification tax (Spec S07.11).
			RunID:    r.ID,
			Domain:   domain,
			Type:     "markdown",
			Revision: 1,
			Content:  content,
		},
		Spec:          pair.Spec,
		Steps:         pair.Plan.Steps,
		ResearchNodes: pair.Plan.ResearchNodes,
		Tier:          st.Tier,
		Workspace:     execCwd,
		EvidenceDir:   evidence,
	})
	if err != nil {
		s.crash(ctx, r.ID, "verification drain: "+err.Error())
		return fmt.Errorf("stage: verify: %w", err)
	}

	switch out.Verdict {
	case verify.VerdictShip, verify.VerdictShipWithNotes:
		// The walking skeleton ends at the verified deliverable + its
		// receipts; V3 (human accept) and delivery mechanics are Spec
		// S13's (B4).
		s.setKanban(ctx, r.TaskID, "done")
		s.logger().Info("stage: deliverable verified", "task", r.TaskID,
			"verdict", string(out.Verdict), "verified_items", out.VerifiedItems, "rounds", len(out.Rounds))
	default:
		// The drain ended on a card (escalation/REOPEN-SPEC/cap-hit): the
		// durable ask waits for the human — the run's drain work is done.
		s.setKanban(ctx, r.TaskID, "attention")
		s.logger().Warn("stage: verification escalated", "task", r.TaskID, "verdict", string(out.Verdict))
	}
	_, err = s.cfg.Runs.Transition(ctx, r.ID, run.StateCompleted, run.TransitionOptions{
		Reason: "verification drain finished: " + string(out.Verdict), Actor: run.ActorPlatform,
	})
	return err
}

// ---- shared ----

// setKanban updates the task's kanban status — provisional strings until
// the S15/9.1 board semantics land (B6); recorded so the demo can watch
// the task move.
func (s *Skeleton) setKanban(ctx context.Context, taskID, status string) {
	err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET kanban_status = ? WHERE task_id = ?`, status, taskID)
		return err
	})
	if err != nil {
		s.logger().Warn("stage: kanban update", "task", taskID, "err", err)
	}
}
