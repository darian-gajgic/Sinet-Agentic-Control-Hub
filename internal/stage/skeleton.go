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

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// Admitter is the scheduler surface the skeleton drives: the sole run
// ingress (Spec S16.6) plus the out-of-dispatch settle (Spec S10.1).
// *scheduler.Scheduler satisfies it; bound late because the scheduler's
// own Config carries the skeleton as its Dispatcher.
type Admitter interface {
	Enqueue(ctx context.Context, runID string, class scheduler.WorkloadClass) error
	SettleRun(ctx context.Context, runID string) error
	// SettleQueuedTx settles a queued run's queue row inside the caller's
	// transaction — the queue half of the S15.6 cancel of a run that never ran
	// (cancel.go, OQ1): the transition and the settle commit together.
	SettleQueuedTx(ctx context.Context, tx *sql.Tx, runID string) error
}

// Skeleton is the walking-skeleton composition (see the package comment).
type Skeleton struct {
	cfg     Config
	driver  *adapters.Driver
	pipe    *intake.Pipeline
	sched   Admitter
	dutyMap worker.DutyMap
	// router is the S08.8 selection engine (nil in the test-only
	// no-Workers posture; router.go documents the degraded default).
	router *worker.Router
	// helpers tracks live helper sessions per coordinator run (S04.4 ⚙
	// orchestration.max_concurrent_helpers; in-process — helper sessions
	// die with the process, and death is salvage, S04.5).
	helpers helperCounter
	// cancels holds the live engine sessions the S15.6 human cancel verb runs
	// the S03.1 ladder on (cancel.go). Human-driven ONLY — no automated path
	// touches it (NO-AUTO-KILL, S14.4 / G1 D1.3).
	cancels *liveSessions
	// leases is the S02.2 lease keeper this skeleton composes once and shares
	// with the driver and the intake pipeline (P3-RW-5 R3/OQ4).
	leases *run.LeaseKeeper
}

// leaseHolderStage names the stage layer in the lease block while it drives a
// resumed drain.
const leaseHolderStage = "stage-drain"

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
	// A commissioned lane must name a substrate that actually exists here.
	// Falling back to the default instead would run the lane's work on the
	// wrong engine, which is precisely the failure this map was added to end.
	for lane, sub := range cfg.LaneSubstrates {
		if _, ok := cfg.Adapters[sub]; !ok {
			return nil, fmt.Errorf("stage: lane %q names substrate %q, which has no registered adapter (S03.2)", lane, sub)
		}
	}
	s := &Skeleton{cfg: cfg, cancels: newLiveSessions()}
	s.dutyMap = cfg.DutyMap
	if s.dutyMap == nil {
		s.dutyMap = worker.DefaultDutyMap()
	}
	if cfg.Workers != nil {
		// The S08.8 selection engine over the worker store. Coverage at v0:
		// the configured lane is the owner's flat-rate lane; the local tier
		// is absent until B4; the metered-exception list is EMPTY (G1 P7) —
		// MeteredAllowed stays nil so the metered branch structurally
		// refuses. Pressure orders multiple flat lanes (D5: consumption
		// pressure, never dollars) — moot with one lane, wired regardless.
		// Alternates carry the second flat-rate lane's seats (P3-LN-2B R21).
		// They select only under coverage, and coverage grows only when a
		// lane is actually COMMISSIONED — so with nothing commissioned this
		// is the pre-LN-2 single-lane path exactly.
		s.router = &worker.Router{
			Store:      cfg.Workers,
			DutyMap:    s.dutyMap,
			Alternates: cfg.AlternateSeats,
			// LocalAvailable flips true when the S12 local stack is configured
			// (B4-5): the utility seat joins the effective DutyMap (built at
			// the composition root) and a class-(a) dispatch onto it degrades
			// to the paid seat with the refined reason (R22).
			Coverage: worker.Coverage{
				FlatRateLanes:  append([]string{cfg.Lane}, cfg.CommissionedLanes...),
				LocalAvailable: cfg.LocalAvailable,
			},
			TieBreak: cfg.TieBreak,
			Pressure: cfg.RoutePressure,
		}
	}
	// The lease keeper of Spec S02.2 (P3-RW-5): ONE helper, shared by every
	// layer this skeleton composes, so the fence and the ⚙ recovery.heartbeat
	// cadence exist in exactly one place (internal/run — no new import edges).
	leases, err := run.NewLeaseKeeper(run.LeaseKeeperConfig{
		DB: cfg.DB, Runs: cfg.Runs, Settings: cfg.Settings, Logger: cfg.Logger,
	})
	if err != nil {
		return nil, err
	}
	s.leases = leases
	s.driver = &adapters.Driver{
		Runs:         cfg.Runs,
		Checkpoints:  cfg.Checkpoints,
		Log:          cfg.Log,
		DB:           cfg.DB,
		Leases:       leases,
		CopyAsideDir: cfg.CopyAsideDir,
		// The S02.4(c) ledger-revision block goes LIVE here (B2-1 seam →
		// B2-4 wiring): every paid-call checkpoint embeds the current
		// ledger revision ref.
		LedgerRevision: cfg.Ledger.CheckpointRef,
		// The S02.4(d) artifact-snapshot block goes LIVE (B4-2): a paid-call
		// checkpoint on a project-backed run carries the platform snapshot
		// commit sha; a workspace-less run leaves it empty (nil-path, R17/R18).
		Snapshot: cfg.Snapshot,
	}
	s.pipe = &intake.Pipeline{
		DB:           cfg.DB,
		Log:          cfg.Log,
		Runs:         cfg.Runs,
		Ledger:       cfg.Ledger,
		Settings:     cfg.Settings,
		Leases:       leases,
		ArtifactRoot: cfg.ArtifactRoot,
		Planner:      cfg.Planner,
		Critic:       cfg.Critic,
		// The S12.1 class-(b) local duty seams (B4-5): wired only when the
		// local stack is configured; nil degrades exactly per the S06 rows
		// (Classifier fails closed to high, Utility/SpotCheck skipped) — R20.
		Classifier: cfg.Classifier,
		Utility:    cfg.Utility,
		SpotCheck:  cfg.SpotCheck,
		Phraser:    cfg.Phraser,
		// The S13.7 registry feeds intake resolution (S06.2) and the S02.6/
		// S09.6 approval-staleness fingerprint (B4-2 seams).
		Registry:           cfg.Registry,
		Fingerprint:        cfg.Fingerprint,
		CitedEntryVersions: cfg.CitedEntryVersions,
		Now:                cfg.Now,
		// The pipeline's degrade log (PH-1 F3) rides the same logger as every
		// other layer this skeleton composes.
		Logger: cfg.Logger,
	}
	// The review store's Compare old-side 0 resolves the repo-backed pre-task
	// base through the topology seam (R25); nil keeps the empty-base behavior.
	if cfg.Review != nil && cfg.BaseContent != nil {
		cfg.Review.BaseContent = cfg.BaseContent
	}
	// The two S12.4 duty seams whose signatures carry no run id are bound to the
	// current-identity reader HERE, where the pipeline is born (P3-RW-6 R7):
	// their $0 D7 rows must ride the run the pipeline is driving, and on a
	// recovery fork that is the fork. Unbound (a seam built elsewhere and never
	// passed through this constructor) keeps the birth composition exactly.
	if u, ok := s.pipe.Utility.(*localUtility); ok {
		u.currentRun = s.currentIntakeRun
	}
	if sc, ok := s.pipe.SpotCheck.(*localSpotCheck); ok {
		sc.currentRun = s.currentIntakeRun
	}
	if s.pipe.Planner == nil {
		s.pipe.Planner = &EnginePlanner{s: s}
	}
	if s.pipe.Critic == nil {
		s.pipe.Critic = &EngineCritic{s: s}
	}
	if s.router != nil {
		s.pipe.Router = &skeletonRouter{s: s}
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

// currentIntakeRun resolves a task's CURRENT intake run id — the identity the
// pipeline state carries right now, which after a recovery-fork rebind is the
// FORK (Spec S02.5 step 2; P3-RW-6 R7). It is the reader for the seams whose
// signature admits no run id (the S06.8 Critic; the S12.4 local duties), and it
// realizes the general rule this packet records: a run id is composed exactly
// ONCE, at birth — every later consumer reads the run's current identity from
// state and never re-derives it from the task id (CONVENTIONS §16, extended).
//
// No state (a task that never started, a test posture) falls back to the birth
// composition, which is exactly what the identity was before any fork.
func (s *Skeleton) currentIntakeRun(ctx context.Context, taskID string) string {
	if st, err := s.pipe.LoadState(ctx, taskID); err == nil && st.RunID != "" {
		return st.RunID
	}
	return taskID + RunSuffixIntake
}

// executeRunID resolves the execute run that ACTUALLY produced the task's
// deliverable — the current identity of the task's execute lineage, walked
// forward through `runs.parent_run_id` (Spec S02.5 step 3) from the birth id.
//
// Work dirs are per RUN ID (runDirs), so composing `<task>.execute` handed the
// verification checks the SUPERSEDED PARENT's stale, usually empty cwd whenever
// a recovery fork produced the deliverable — the same defect as the shell's
// workspace gate, one layer up (P3-RW-6 R12, drain F1). A suffix strip cannot
// fix this one: it removes a fork suffix, it cannot reconstruct one from a task
// id. The lineage row is the honest record and the only durable one available
// here — the ledger's artifact entry names only the producing STAGE, and the
// S13 revision row (which does carry a run id) is minted later, by the verify
// leg itself, so it cannot answer a question asked before it exists.
//
// The non-fork path is byte-equivalent: with no successor row the walk returns
// the birth id it started from, which is also what a task with no execute run
// at all yields.
func (s *Skeleton) executeRunID(ctx context.Context, taskID string) string {
	id := taskID + RunSuffixExecute
	const maxHops = 64 // a lineage is bounded by ⚙ recovery.max_attempts many times over
	for hop := 0; hop < maxHops; hop++ {
		var next string
		err := s.cfg.DB.QueryRowContext(ctx,
			`SELECT run_id FROM runs WHERE parent_run_id = ? ORDER BY generation DESC LIMIT 1`, id).Scan(&next)
		if err != nil || next == "" {
			return id
		}
		id = next
	}
	s.logger().Warn("stage: execute lineage exceeds the hop bound; using the newest run found",
		"task", taskID, "run", id)
	return id
}

// ---- run roles ----

type role string

const (
	roleIntake  role = "intake"
	roleExecute role = "execute"
	roleVerify  role = "verify"
	roleCompose role = "compose"
)

func runRole(runID string) (role, bool) {
	// Recovery forks append .g<generation> segments to the parent run id
	// (Spec S02.5 fork lineage, B0-4), so the role must survive any number
	// of fork suffixes — verify.g1 dispatches exactly like verify. Without
	// this, every recovery fork of a pipeline run was unroutable and
	// crashed at dispatch, burning the ladder to tombstone (found live at
	// the B2 gate demo, 2026-07-20).
	//
	// The stripping itself is the ONE shared matcher (P3-RW-6 OQ5): three
	// private implementations existed and the third — the shell's raw
	// HasSuffix on the workspace gate — was wrong, so a fork of an execute run
	// silently lost its worktree. metering owns the suffix constants this
	// package already aliases, so it owns the strip too; a fourth copy is a
	// defect.
	id := metering.StripForkSuffix(runID)
	switch {
	case strings.HasSuffix(id, RunSuffixIntake):
		return roleIntake, true
	case strings.HasSuffix(id, RunSuffixExecute):
		return roleExecute, true
	case strings.HasSuffix(id, RunSuffixVerify):
		return roleVerify, true
	case strings.HasSuffix(id, RunSuffixCompose):
		return roleCompose, true
	case strings.HasSuffix(id, RunSuffixOnboard):
		return roleOnboard, true
	case strings.HasSuffix(id, RunSuffixDirect):
		// The BENCH-REG §2 direct arm (direct.go, B6-2C). Its run id is
		// `<pair>.direct`, minted by internal/benchmark; the suffix is pinned to
		// that construction at the composition root.
		return roleDirect, true
	}
	return "", false
}

// Routable reports whether a run id carries a dispatchable skeleton role — the
// question `Skeleton.Dispatch` asks, exported so the composition root can ask it
// too (P3-RW-6 R15).
//
// The shell composes the recovery ladder's NoFork predicate from it: a crashed
// run with NO role must FINALIZE rather than take S02.5 step 3's default fork,
// because its successor could never dispatch — the fork would crash, the ladder
// would fork again, and the lineage would burn to a tombstone having done
// nothing. That is not hypothetical: `platform.deadman.*` canary runs are
// roleless by construction (S14.4) and an orphaned one ages past
// ⚙ recovery.dead_after like any other run. Fork policy stays where fork policy
// lives — this is a predicate the shell composes, so internal/recovery still
// learns nothing about run classes and the §31 watchdog wall is untouched.
//
// It is fork-suffix aware for free (runRole strips through the shared matcher),
// which is the property the shell's predicate needs: routability cannot flip
// between generations of one lineage.
func Routable(runID string) bool {
	_, ok := runRole(runID)
	return ok
}

// ---- scheduler.Dispatcher ----

// Dispatch routes a CAS-claimed run by its role (the walking-skeleton
// run-naming convention; a first-class role record is S04/S08 machinery,
// B3). Unroutable runs error loudly — nothing is silently absorbed.
func (s *Skeleton) Dispatch(ctx context.Context, r run.Run) error {
	rl, ok := runRole(r.ID)
	if !ok {
		// A dispatch that cannot proceed leaves a CLASSIFIABLE CORPSE, never a
		// silent zombie (the crash helper's own contract): returning the error
		// alone left the run `claimed`, holding its cap-1 (user, lane) slot until
		// a recovery sweep noticed it up to ⚙ recovery.dead_after later — which is
		// how a handful of unroutable seeds starved every real run in the demo
		// world. claimed→crashed is the ratified edge for exactly this (run.go,
		// "implied by S02.5 step 2").
		//
		// NO-AUTO-KILL (S14.4 / G1 D1.3) is untouched: this run is ours — we just
		// claimed it — and it is PROVABLY undispatchable, so nothing live is being
		// killed. The corpse is what lets the ladder decide (and with the shell's
		// composed NoFork predicate, a roleless corpse finalizes rather than
		// forking into another run nobody can route).
		cause := fmt.Sprintf("run %q has no skeleton role (want *%s|*%s|*%s|*%s|*%s|*%s)", r.ID,
			RunSuffixIntake, RunSuffixExecute, RunSuffixVerify, RunSuffixCompose, RunSuffixOnboard,
			RunSuffixDirect)
		s.crash(ctx, r.ID, cause)
		return fmt.Errorf("stage: %s", cause)
	}
	switch rl {
	case roleIntake:
		return s.dispatchIntake(ctx, r)
	case roleExecute:
		return s.dispatchExecute(ctx, r)
	case roleCompose:
		return s.dispatchCompose(ctx, r)
	case roleOnboard:
		return s.dispatchOnboard(ctx, r)
	case roleDirect:
		return s.dispatchDirect(ctx, r)
	default:
		return s.dispatchVerify(ctx, r)
	}
}

// crash marks a dispatched run crashed with its cause — the Driver's
// spawn-failure precedent: a dispatch leg that cannot proceed leaves a
// classifiable corpse for the recovery ladder, never a silent zombie.
func (s *Skeleton) crash(ctx context.Context, runID, cause string) {
	// THE RECORD OF THE ENDING OUTLIVES THE REQUEST (P3-RW-10 R2). The corpse is
	// what makes the strand healable, and the commonest reason a leg dies is that
	// its caller did — a requester's page navigating away mid-answer kills the
	// request context, the drive with it, and (before this) the posture too: the
	// state read failed, the transition failed, and the run sat `running` with
	// nobody driving it. Detaching HERE covers every caller in one move — the
	// dispatch legs, the intake beats via mapDriveErr, the verify answer sites.
	// Values (identity, log attribution) are kept; only cancellation is dropped.
	ctx = context.WithoutCancel(ctx)
	// The suppression is consulted at the run's CURRENT generation and consumed
	// on use (drain D1): it silences exactly the one unwind its own cancel
	// caused. A mark left by a cancel that never committed, or one predating a
	// resume (which bumps the generation, §8), can never match here — so a
	// genuine crash after either always leaves its corpse for the ladder.
	if cur, err := s.cfg.Runs.Get(ctx, runID); err == nil &&
		s.cancels.consumeCancel(runID, cur.Generation) {
		s.logger().Info("stage: dispatch leg unwound by a human cancel; no crash corpse",
			"run", runID, "generation", cur.Generation, "cause", cause)
		return
	}
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
	// The dispatched run's OWN identity crosses into the pipeline (Spec S02.5
	// step 2): on a recovery fork the pipeline rebinds its state onto this run
	// and supersedes the parent. Driving by task id alone left the pipeline
	// checking the crashed parent's state — ErrNotRunning on every generation,
	// re-fork, tombstone (the live-world t-80eff lineage). The skeleton hands
	// over r.ID and nothing else; the rebind and its lineage guard are the
	// pipeline's own (it owns the state that carries the identity).
	st, err := s.pipe.AdvanceDispatched(ctx, r.TaskID, r.ID)
	if err != nil {
		s.crash(ctx, r.ID, "intake advance: "+err.Error())
		return fmt.Errorf("stage: intake advance: %w", err)
	}
	return s.afterIntake(ctx, st)
}

// afterIntake continues from an intake state: an open card waits (the run
// is parked — gates wait, Spec S06.1); an approved plan completes the
// intake run and launches execution. A recorded compose request launches
// its ceremony run FIRST — it runs while the card stays open (Spec S08.6:
// composition never rides approval), and the ensure is idempotent.
func (s *Skeleton) afterIntake(ctx context.Context, st *intake.State) error {
	if st.Compose != nil {
		if err := s.ensureComposeRun(ctx, st); err != nil {
			return err
		}
	}
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

	// S08.8: the dispatch consumes the recorded selection (approval-card
	// decision incl. any re-route/pin) and emits the settled routing.decided
	// event on THIS run — worker + version per run, the version→outcome
	// join key (7.7; S2.6). Recording failure fails the dispatch: silent
	// routing is banned (14.3).
	er := s.executeRouting(ctx, r)
	if err := s.emitRoutingDecided(ctx, r, er); err != nil {
		s.crash(ctx, r.ID, "routing record: "+err.Error())
		return err
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
	// that step's contract (Spec S05.4). An overflow stage-split continues
	// the planned stage in successor sub-stage sessions (split.go); the
	// overflow count is scoped PER PLANNED STAGE per S05.3 ("a second
	// overflow within one planned stage escalates to a re-plan proposal").
	deliverable := ""
	for i, step := range pair.Plan.Steps {
		// The S10.4 pause-my-automation boundary (pause.go): between stage
		// sessions, never inside one. A parked leg returns cleanly with
		// everything it has already done recorded — nothing is killed, nothing
		// is rolled back, and the queue is untouched (P-T08-4).
		if s.pauseParkPoint(ctx, r, step.ID) {
			return nil
		}
		res, err := s.runPlannedStage(ctx, r, er, step)
		if errors.Is(err, errPausedPark) {
			// The owner paused mid-stage and a SPLIT successor boundary caught
			// it. The run is parked; the leg returns cleanly (pause.go).
			return nil
		}
		if err != nil {
			s.crash(ctx, r.ID, fmt.Sprintf("step %s session: %v", step.ID, err))
			return fmt.Errorf("stage: execute step %s: %w", step.ID, err)
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

	// Execute-leg stage close takes a platform snapshot commit (Spec S13.5,
	// R19): it captures any bash side effects since the last checkpoint as
	// revision raw material for the verification handoff mint. A workspace-less
	// run returns "" — the content-pin lane, no snapshot. Best-effort: a
	// snapshot failure is logged, never a crash of a completed execution.
	if s.cfg.Snapshot != nil {
		if _, serr := s.cfg.Snapshot(ctx, r.ID); serr != nil {
			s.logger().Warn("stage: execute-leg stage-close snapshot failed",
				"run", r.ID, "err", serr)
		}
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
	// The S10.4 pause boundary for the VERIFY leg (pause.go): a paused owner's
	// run parks at its next stage-session boundary whichever leg it is in, and
	// the drain's first judge session has not started yet.
	if s.pauseParkPoint(ctx, r, "verify") {
		return nil
	}
	return s.runVerifyDrain(ctx, r, 1)
}

// runVerifyDrain runs the S07 drain for a RUNNING verify run and lands its
// terminal. It is the ONE path both the dispatch leg and the S07.7
// infrastructure card's `retry` answer take (P3-RW-14 R4: retry re-enters the
// drain), so the two can never drift apart.
//
// Two failure dispositions, and the difference is the packet's point:
//
//   - a DETERMINISTIC verification-infrastructure refusal (no check pack for a
//     launch domain, an unwired seam, an unvalidated judge seat) is a screen
//     OUTAGE: it escalates to a durable decision card and PARKS (Spec S07.2 "a
//     screen outage escalates rather than approves"; S07.7 sink-types-only).
//     Crashing it fed the S02.5 ladder a refusal no fork could ever fix — the
//     live 2026-08-16 wedge: three identical crashes, a tombstone, and a task
//     sitting at "verifying" forever with nothing to answer;
//   - a MECHANICAL failure (the deliverable could not be read, the judge
//     transport died, a DB error) still CRASHES for the ladder: a fork of that
//     may well succeed, and a corpse is what makes the heal machine-only
//     (CONVENTIONS §16/§21).
func (s *Skeleton) runVerifyDrain(ctx context.Context, r run.Run, revision int) error {
	in, err := s.verifyInput(ctx, r.ID, r.TaskID, revision)
	if err != nil {
		s.crash(ctx, r.ID, err.Error())
		return err
	}
	out, err := s.drainOrCard(ctx, r, in)
	if err != nil {
		return err // drainOrCard has already left the ladder its corpse
	}
	if err := s.verifyTerminal(ctx, r.ID, r.TaskID, out); err != nil {
		// The drain finished but its terminal record did not land: the run holds
		// a paid outcome nobody is driving to a state. Leave the ladder a corpse
		// rather than a strand (P3-RW-10 R5b; CONVENTIONS §16).
		s.crash(ctx, r.ID, "verification terminal record: "+err.Error())
		return fmt.Errorf("stage: verify terminal: %w", err)
	}
	return nil
}

// drainOrCard produces the outcome runVerifyDrain lands: the drain's own, or —
// for a deterministic verification-infrastructure refusal — an ESCALATE outcome
// carrying the decision card (P3-RW-14 R4). Anything mechanical crashes the run
// for the ladder here and returns the error; the outcome then never exists, so
// there is exactly ONE place a verification outcome reaches the run FSM.
func (s *Skeleton) drainOrCard(ctx context.Context, r run.Run, in verify.VerifyInput) (verify.Outcome, error) {
	v, err := s.newVerifier(ctx, in.Deliverable.Domain, r.TaskID)
	if err == nil {
		var out verify.Outcome
		out, err = v.Verify(ctx, in)
		if err == nil {
			return out, nil
		}
	}
	if _, outage := verify.AsPreambleRefusal(err); !outage {
		s.crash(ctx, r.ID, "verification drain: "+err.Error())
		return verify.Outcome{}, fmt.Errorf("stage: verify: %w", err)
	}
	// The ending outlives the request that provoked it (CONVENTIONS §56).
	ctx = context.WithoutCancel(ctx)
	esc := &verify.Escalator{DB: s.cfg.DB, Log: s.cfg.Log, Settings: s.cfg.Settings, Now: s.cfg.Now}
	card, rerr := esc.Raise(ctx, verify.InfrastructureEscalation(in.Deliverable, r.UserID, err))
	if rerr != nil {
		// The door itself could not be written: that IS mechanical, so the run
		// becomes a corpse the ladder can fork rather than a silent wedge.
		s.crash(ctx, r.ID, "verification infrastructure card: "+rerr.Error())
		return verify.Outcome{}, fmt.Errorf("stage: verify infrastructure card: %w", rerr)
	}
	s.logger().Warn("stage: verification infrastructure outage — escalating to a decision card instead of approving (S07.2/S07.7)",
		"run", r.ID, "task", r.TaskID, "ask", card.AskID, "cause", err)
	return verify.Outcome{Verdict: verify.VerdictEscalate, Card: &card}, nil
}

// newVerifier assembles the S07 Verifier over the skeleton's seams for one
// domain (shared by the dispatch leg and the S07.7 resume leg).
//
// This is the one judge-construction site, so it is where P-T06-5 is enforced
// (Spec S14.8 ¶3): a judge-model change BLOCKS unsupervised judging until the
// golden-set re-measurement passes. The gate is verify's pure check over the
// rubric bundle's own pin and recorded rates against the seat that will
// actually judge; the S14.8 revalidation runbook owns the re-measurement and
// the rubric version bump that clears it. Refusing here means no verdict can be
// minted under an unvalidated judge.
func (s *Skeleton) newVerifier(ctx context.Context, domain, taskID string) (*verify.Verifier, error) {
	// The rubric resolution mirrors the drain's own (a nil Rubric resolves to
	// the software seed); it is set on the Verifier so the bundle the gate
	// checks IS the bundle that judges.
	rubric := verify.SeedSoftwareRubric()
	judge := s.cfg.Judge
	if judge == nil {
		// The production path: the shell injects no Judge, so every real
		// verification passes through the gate. An injected Judge is the
		// composition root's own dev/test seam (the nil-Confiner precedent).
		judge = &EngineJudge{s: s}
		if err := verify.UnsupervisedJudgingGate(rubric, judge.Meta().Model); err != nil {
			// An unvalidated judge seat is a screen that cannot run — the same
			// outage class as a missing check pack, and it terminates the same
			// way: a decision card the operator can answer, never a crash the
			// ladder re-forks into the identical refusal (P3-RW-14 R4).
			return nil, verify.NewPreambleRefusal(err)
		}
	}
	pack, err := s.checkPack(ctx, domain, taskID)
	if err != nil {
		return nil, err
	}
	revise := s.cfg.Revise
	if revise == nil {
		revise = s.engineRevise
	}
	var sink verify.ReviewSink
	if s.cfg.Review != nil {
		sink = reviewSink{s: s}
	}
	return &verify.Verifier{
		DB:       s.cfg.DB,
		Log:      s.cfg.Log,
		Ledger:   s.cfg.Ledger,
		Settings: s.cfg.Settings,
		Judge:    judge,
		Rubric:   rubric,
		Pack:     pack,
		Runner:   s.cfg.CheckRunner,
		Review:   sink,
		// The S13 verification-workspace seam (Spec S07.3 rule 1): V1 checks
		// run against the revision's stripped content, not the execute leg's
		// scratch cwd, whenever the task is project-backed (P3-RW-14 R6).
		Workspace: verifyWorkspaceSeam{s: s},
		// Research counters stay nil at B2-4: stage sessions exist now,
		// but the research TOOL substrate (WebSearch/egress) does not —
		// nodes record UNVERIFIABLE-HERE loudly, never a fake pass
		// (Spec S07.3; flagged to the B2 gate with the egress batch).
		Revise: revise,
		Now:    s.cfg.Now,
	}, nil
}

// checkPack resolves the per-(domain, project) V1 check pack through the S13.7
// registry seam (P3-RW-14 R5). The distinction the drain rests on:
//
//   - (nil, nil) — no pack applies here (no seam wired, or a non-launch domain
//     whose ratified degraded mode is V1 empty, Spec S07.8);
//   - an error wrapping ErrNoCheckPack / ErrBadPack — the pack the launch
//     domain REQUIRES is absent or unusable, and the error names exactly what
//     is missing. That is the preamble refusal class: it becomes the operator's
//     decision card, never a crash and never a silently degraded launch domain;
//   - any other error — the registry itself failed. Mechanical: it crashes for
//     the ladder, because the next sweep may well read the registry fine.
func (s *Skeleton) checkPack(ctx context.Context, domain, taskID string) (*verify.CheckPack, error) {
	if s.cfg.CheckPackFor == nil {
		return nil, nil
	}
	pack, err := s.cfg.CheckPackFor(ctx, domain, taskID)
	if err != nil {
		if errors.Is(err, verify.ErrNoCheckPack) || errors.Is(err, verify.ErrBadPack) {
			return nil, verify.NewPreambleRefusal(err)
		}
		return nil, fmt.Errorf("stage: resolve the %s check pack: %w", domain, err)
	}
	return pack, nil
}

// verifyWorkspaceSeam adapts the composition-root materializer to verify's
// S13 WorkspaceProvider. It exists so internal/verify keeps taking an
// interface and internal/stage keeps taking a func field over
// internal/project (CONVENTIONS §23) — neither package learns about the other.
type verifyWorkspaceSeam struct{ s *Skeleton }

// VerificationWorkspace returns the materialized revision, or "" when this
// task has none to materialize — the honest absence the drain falls back on
// (the execute leg's own cwd, via the RW-6 forward walk).
func (w verifyWorkspaceSeam) VerificationWorkspace(ctx context.Context, d verify.Deliverable) (string, func(), error) {
	if w.s.cfg.VerificationWorkspace == nil {
		return "", nil, nil
	}
	return w.s.cfg.VerificationWorkspace(ctx, d.TaskID, d.Revision)
}

// verifyInput builds the drain input for one persisted deliverable
// revision of a task's verify run.
func (s *Skeleton) verifyInput(ctx context.Context, runID, taskID string, revision int) (verify.VerifyInput, error) {
	pair, st, err := s.pipe.ApprovedPair(ctx, taskID)
	if err != nil {
		return verify.VerifyInput{}, fmt.Errorf("stage: verify without approved plan: %w", err)
	}
	content, err := s.readDeliverable(taskID, revision)
	if err != nil {
		return verify.VerifyInput{}, err
	}
	execCwd := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(s.executeRunID(ctx, taskID)), "cwd")
	evidence := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(runID), "evidence")
	if err := os.MkdirAll(evidence, 0o700); err != nil {
		return verify.VerifyInput{}, err
	}
	// The S07.2 wrote-nothing inputs (P3-RW-14 R7), all durable PLATFORM
	// facts: what the plan CLAIMED it would write, and where the tree
	// actually stands versus the pre-task base. Nothing the engine said
	// about its own work is admissible here — that report is exactly what
	// this gate exists to disbelieve.
	globs, unbounded := pair.Plan.WriteGlobs()
	var snapshotSHA, baseSHA string
	if s.cfg.RepoFacts != nil {
		if snapshotSHA, baseSHA, err = s.cfg.RepoFacts(ctx, taskID); err != nil {
			// A git read failure is mechanical: it must not become a
			// wrote-nothing verdict, and it must not silently pass either.
			return verify.VerifyInput{}, fmt.Errorf("stage: read repo facts for verification: %w", err)
		}
	}
	return verify.VerifyInput{
		Deliverable: verify.Deliverable{
			TaskID: taskID,
			// The drain's events, judge assemblies and checkpoints ride
			// THIS run: it is running through the drain (Spec S02.4
			// checkpoint writability) and its consumption is the
			// verification tax (Spec S07.11).
			RunID:        runID,
			Domain:       domainFor(st.Family),
			Type:         "markdown",
			Revision:     revision,
			Content:      content,
			SnapshotSHA:  snapshotSHA,
			BaseSHA:      baseSHA,
			WriteClaimed: len(globs) > 0 || unbounded,
		},
		Spec:          pair.Spec,
		Steps:         pair.Plan.Steps,
		ResearchNodes: pair.Plan.ResearchNodes,
		Tier:          st.Tier,
		Workspace:     execCwd,
		EvidenceDir:   evidence,
	}, nil
}

// verifyTerminal lands a drain outcome on the run FSM (shared by the
// dispatch leg and the S07.7 resume leg, on a RUNNING verify run):
//
//   - SHIP / SHIP-with-notes → the run completes and settles. The walking
//     skeleton ends at the verified deliverable + its receipts; V3 (human
//     accept) and delivery mechanics are Spec S13's (B4).
//   - every card terminal → the run PARKS on the durable ask (Spec S02.3:
//     parked = suspended on an open gate/ask; Spec S07.7: escalations are
//     structured, RESUMABLE asks — answering resumes the pipeline in
//     place). Parked-on-gate is "blocked, not failed" (Spec S02.6); the
//     queue row stays claimed until a later answer completes or finalizes
//     the run (the intake gate pattern).
//
// The outcome is an ENDING RECORD and outlives the request that asked for it
// (P3-RW-10 R5): the drain has already been paid for, so a requester who walked
// away mid-answer must not cost the platform the verdict (D7 no paid work is
// lost; S07.7 no finding dies silently).
func (s *Skeleton) verifyTerminal(ctx context.Context, runID, taskID string, out verify.Outcome) error {
	ctx = context.WithoutCancel(ctx)
	switch out.Verdict {
	case verify.VerdictShip, verify.VerdictShipWithNotes:
		s.setKanban(ctx, taskID, "done")
		s.logger().Info("stage: deliverable verified", "task", taskID,
			"verdict", string(out.Verdict), "verified_items", out.VerifiedItems, "rounds", len(out.Rounds))
		if _, err := s.cfg.Runs.Transition(ctx, runID, run.StateCompleted, run.TransitionOptions{
			Reason: "verification drain finished: " + string(out.Verdict), Actor: run.ActorPlatform,
		}); err != nil {
			return err
		}
		s.settle(ctx, runID)
		return nil
	default:
		s.setKanban(ctx, taskID, "attention")
		s.logger().Warn("stage: verification escalated", "task", taskID, "verdict", string(out.Verdict))
		_, err := s.cfg.Runs.Transition(ctx, runID, run.StateParked, run.TransitionOptions{
			Reason: "verification card open: parked on the resumable ask (S07.7; S02.3 gate park)",
			Actor:  run.ActorPlatform,
		})
		return err
	}
}

// settle settles a terminal run's queue row + receipt outside a dispatch
// (the finishIntake pattern; SettleRun is idempotent, so the dispatch
// path's automatic settle composes safely).
func (s *Skeleton) settle(ctx context.Context, runID string) {
	if s.sched == nil {
		return
	}
	if err := s.sched.SettleRun(ctx, runID); err != nil {
		s.logger().Warn("stage: settle run", "run", runID, "err", err)
	}
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
