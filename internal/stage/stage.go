// Package stage is the S05.3 stage runtime and the B2 walking-skeleton
// composition: the machinery that turns the landed organs — intake (Spec
// S06), the Task Context Ledger (Spec S05), verification (Spec S07), the
// adapter substrate (Spec S03), the scheduler (Spec S10) and the sandbox
// seams (Spec S11) — into ONE spine: intake → plan → execute → verify →
// checkpoint → receipt (Spec S19.5 B2: "closes the thin end-to-end path").
//
// Three duties live here:
//
//   - The stage RUNNER (runner.go): one fresh engine session per pipeline
//     stage (Spec S05.3 fresh-context-per-stage — a continued transcript is
//     never the mechanism for crossing a stage boundary), built from the
//     ledger's assembled stage brief (Spec S05.4: BriefText prompt assembly
//
//   - PlacePinned + the SessionStart re-injection channel), driven through
//     the adapter Driver so every paid call checkpoints (D7, Spec S02.4)
//     with the live ledger-revision block, and watched by the stage-fit
//     budget machinery (⚙ context.stage_fit_target / ⚙
//     context.stage_overflow_threshold, Spec S05.3).
//
//   - The engine-session implementations of the pipeline model seams
//     (engines.go): intake.Planner/Critic (the S06.10 planning-model
//     duties) and verify.Judge plus the S07.6 fresh-session rework
//     executor — each a prompt contract + strict parse over one stage
//     session. Trust stays in the platform validation (the spine, the
//     judge validators), never in the seam.
//
//   - The walking-skeleton dispatcher (skeleton.go): the composition-root
//     realization of scheduler.Dispatcher that routes dispatched runs by
//     ROLE — `<task>.intake` drives the intake pipeline (cards park the
//     run; answers resume it in place), `<task>.execute` runs the approved
//     plan's steps as engine stage sessions, `<task>.verify` runs the S07
//     drain — and chains approval → execution → verification. Fire-and-
//     forget stays banned: every run reaches an engine only via Enqueue →
//     claim → dispatch (S16.6), and receipts materialize per run-end
//     (Spec S10.1).
//
// Boundary discipline (Spec S19.5): worker/model selection is Spec S08.8's
// (live since B3-3; Config.Model remains the dev/test override);
// stage-split EXECUTION of an overflow proposal is live since B3-5
// (split.go — consolidate-to-ledger → end session → successor sub-stage
// brief, Spec S05.3), as is the S08.6 composition leg (compose.go);
// deliverable/revision mechanics are Spec S13's (B4) — the deliverable
// at B2-4 is the execute session's result text, captured as a durable
// artifact; the local tier is Spec S12's (B4) — the Utility duty stays
// unwired rather than faked onto a paid engine (S06.10 pins it local).
package stage

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// The stage-runtime ⚙ keys (Spec S05.3; declared in the S18 registry since
// B0-2, consumed here — CONVENTIONS §2: read by dotted key, never
// hardcoded).
const (
	// keyStageFitTarget: stages are PLANNED TO FIT — target ≤ this fraction
	// of the lane's pinned-model context window at stage start, measured by
	// the adapter's usage accounting (Spec S05.3, G1 Def.11). A stage brief
	// that cannot fit the target is a plan-shape defect and raises a
	// re-plan proposal.
	keyStageFitTarget = "context.stage_fit_target"
	// keyStageOverflow: at this fraction the control plane emits a
	// context.overflow event proposing a stage split at the next checkpoint
	// boundary; a second overflow within one planned stage escalates to a
	// re-plan proposal (Spec S05.3).
	keyStageOverflow = "context.stage_overflow_threshold"
)

// keyRecitationInterval is the ⚙ in-stage recitation cadence of Spec S05.3
// ("the coordinator re-reads the ledger's state/next_actions every N
// turns"; 0 = off). CONSUMED since B3-4 (Research/18 §7-C1; the B2-4
// "no per-turn channel" seam closed): the platform reciter (recite.go)
// counts turns from persisted run events and authors due recitations onto
// the S03.4 ctl-dir airlock; the lowering-compiled PostToolUse hook is the
// delivery valve — probe-proven on the installed engine
// (P3/measurements/2026-07-21-posttooluse-additionalcontext-probe.md).
const keyRecitationInterval = "context.recitation_interval_turns"

// The B2-4 model/window constants are RETIRED (B3-3): the model comes from
// Spec S08.8 selection (worker execution profiles resolved against the
// duty map — worker.DefaultDutyMap at v0), and the per-model context-window
// record rides the duty-map seat rows (worker.Seat.WindowTokens — the
// "worker registry" home the B2-4 constant's comment named). Config.Model
// remains as the dev/test override only.

// Run-role suffixes (the walking-skeleton run-naming convention; intake
// fixed `<task>.intake` at B2-2 — CONVENTIONS §14; the skeleton fixes
// `.execute`/`.verify` at B2-4). internal/metering derives receipt purpose
// tags from the same constants (S06.10/S07.11 ceremony/verification
// itemization); a first-class run-role record is S04/S08 machinery (B3).
const (
	RunSuffixIntake  = metering.RunSuffixIntake
	RunSuffixExecute = metering.RunSuffixExecute
	RunSuffixVerify  = metering.RunSuffixVerify
	// RunSuffixCompose names the S08.6 composition ceremony run (B3-5);
	// metering itemizes it as ceremony to the requester.
	RunSuffixCompose = metering.RunSuffixCompose
)

// EventContextOverflow is the S05.3 overflow event type: emitted run-scoped
// when a stage session's measured context footprint crosses the ⚙
// thresholds, carrying the proposal (stage split, or re-plan on a second
// overflow / an overweight brief). Name provisional pending the S14 event
// contract (B5), extending the CONVENTIONS §7/§8/§13/§14 naming note.
const EventContextOverflow = "compaction.anomaly"

// contextOverflowSchemaVersion versions the context.overflow payload.
const contextOverflowSchemaVersion = 1

// Settings is this package's view of the settings registry (CONVENTIONS
// §2). *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
	Duration(key string) (time.Duration, error)
	String(key string) (string, error)
	FloatFor(key, userID string) (float64, error)
	Bool(key string) (bool, error)
	Strings(key string) ([]string, error)
}

// Config assembles the stage runtime + walking skeleton. DB, Log, Runs,
// Checkpoints, Ledger, Settings and Adapters are mandatory; the seams
// default as documented.
type Config struct {
	DB          *storage.DB
	Log         *eventlog.Log
	Runs        *run.Store
	Checkpoints *gates.Checkpoints
	Ledger      *ledger.Store
	Settings    Settings

	// Adapters maps substrate name → adapter (Spec S03.2). The claude-cli
	// adapter registers here at B2-4 (the registration runDispatcher
	// deferred since B1-2, CONVENTIONS §11).
	Adapters map[string]adapters.Adapter

	// Substrate/Lane name the ceremony substrate (Spec S03.2). Model, when
	// set, is a dev/test OVERRIDE of every duty seat and selection result —
	// production leaves it empty and Spec S08.8 selection + the duty map
	// decide (B3-3).
	Substrate string
	Lane      string
	Model     string

	// Workers is the S08 worker store; wiring it activates S08.8 selection
	// (the Router built by New over DutyMap/Coverage below). NIL IS A
	// TEST-ONLY POSTURE — the composition root always wires it at B3-3 —
	// under which cards carry no routing block and dispatch runs the
	// generalist duty-map default, loudly logged.
	Workers *worker.Store
	// DutyMap is the requester duty-map view (S06.10 recommended default +
	// per-user override at its surface, B6/v1). Nil = worker.DefaultDutyMap.
	DutyMap worker.DutyMap
	// RoutePressure is the D5 flat-lane ordering input (consumption
	// pressure, never dollars); nil short-circuits with a single covered
	// lane. metering.PressureGauge adapts to it at the composition root.
	RoutePressure worker.PressureReader
	// TieBreak is the S12 local-duty tie-break seam (B4-5; nil = the
	// deterministic degraded order with the absence recorded).
	TieBreak worker.TieBreaker
	// LocalAvailable reports the S12 local serving stack is configured (B4-5):
	// the effective DutyMap gains the utility seat and Coverage.LocalAvailable
	// flips true, so a class-(a) engine dispatch onto duty utility degrades to
	// the paid seat with the refined honest reason (BINDING reading; R22).
	LocalAvailable bool

	// The S12.1 class-(b) intake duty seams (B4-5; brief R20): local
	// implementations wired ONLY when the local stack is configured — the
	// pipeline degrades exactly per the S06 rows otherwise (Classifier fails
	// closed to high, Utility falls back to deterministic text, SpotCheck is
	// skipped). Nil is the sanctioned dev/test posture.
	Classifier intake.Classifier
	Utility    intake.Utility
	SpotCheck  intake.SpotCheck
	// Phraser is the S06.5 phrase-and-summarize seat, the same utility alias
	// as Utility (P3-RW-12). Nil leaves interview cards carrying the
	// taxonomy's own words — no clicks added, nothing faked.
	Phraser intake.Phraser

	// Confiner wraps engine spawns in the composed per-run sandbox (Spec
	// S11). NIL IS THE SANCTIONED DEV POSTURE (the B1-1 unconfined dev
	// spawn, CONVENTIONS §10): a confined engine cannot authenticate until
	// the credential-injection proxy lands (S11.5 sentinel rule — the real
	// token never enters the sandbox), and that proxy is part of the
	// B2-gate host batch (srt activation + egress substrate). Production
	// posture wires the sandbox.Composer here.
	Confiner adapters.Confiner
	// CredInject builds the per-spawn broker credential injector (Spec
	// S11.5, S01.6 "engines receive credentials at start"). Nil = dev
	// posture (the engine resolves its own config-root credential via
	// OwnerCredRef / ambient default). broker.EnvInjector satisfies the
	// inner shape.
	CredInject func(userID string) func(base []string) ([]string, error)

	// ArtifactRoot is the durable artifact directory (intake pair files,
	// deliverable revisions). RunRoot holds per-run cwd/work dirs;
	// CopyAsideDir the transcript copy-aside tree (Spec S02.4).
	ArtifactRoot string
	RunRoot      string
	CopyAsideDir string

	// CheckPacks maps deliverable domain → V1 check pack (Spec S07.3). The
	// software pack is per-project registry machinery (Spec S13, B4) — the
	// map ships EMPTY at B2-4, so software-domain deliverables fail verify
	// LOUDLY (ErrNoCheckPack) rather than run a silent degraded launch
	// domain; non-launch domains verify with V1 empty, the ratified
	// degraded mode (Spec S07.8).
	CheckPacks map[string]*verify.CheckPack
	// CheckRunner executes V1 checks (required when a pack is present).
	CheckRunner verify.CheckRunner

	// Seam overrides (Spec S08 selection arrives at B3; tests and the
	// bounded live smoke substitute in-process fakes). Nil = the engine-
	// session implementations of engines.go.
	Planner intake.Planner
	Critic  intake.Critic
	Judge   verify.Judge
	Revise  func(ctx context.Context, pkg verify.RetryPackage) (verify.Deliverable, error)

	// Knowledge is the S05.4 knowledge slice source (Spec S09.3, B3-1):
	// house/project/user knowledge selected by pure registry lookup within
	// per-scope ⚙ budgets. Nil = the seam is absent and briefs assemble
	// without knowledge slices. It feeds planning and execute-stage
	// briefs; the judge's clean slice stays the S07-fixed input set
	// (CONVENTIONS §15) and the critic stays artifact-only (§14).
	Knowledge ledger.Source

	// ComposerPlaybook reads the CURRENT APPROVED composer playbook — the
	// governed S09.10 house object the S08.6 composer consumes by policy.
	// A seam (this package never imports the memory store, CONVENTIONS
	// §17): the composition root adapts memory.Store.HouseObject. Nil, or
	// an absent object, makes composition refuse LOUDLY — the playbook is
	// a required policy input, never silently skipped.
	ComposerPlaybook func(ctx context.Context) (worker.Playbook, error)

	// EnginePin keys the validation records the composition leg writes
	// (Spec S08.1: a record per version × model × engine pin) — the
	// composition root supplies the consuming lane's pinned engine version
	// (the adapter's exported Pin; this package never imports substrate
	// packages, CONVENTIONS §10).
	EnginePin string

	// Review is the S13.1–S13.4 review store (B4-1). Wiring it activates
	// revision minting at the verification handoff, findings-as-durable-
	// comments, and THE S13.4 drain composing retry packages; the composer
	// leg additionally presents composed definitions as deliverables at
	// birth (Spec S13.1). NIL IS A TEST-ONLY POSTURE (the pre-S13
	// in-memory channel); the composition root always wires it.
	Review *review.Store

	// The S13.5/S13.7 git-topology + registry seams (B4-2), wired by the
	// composition root over internal/project through narrow func fields (the
	// Driver.Snapshot/LedgerRevision precedent; this package never imports
	// internal/project — the walls stay clean, brief R35 / CONVENTIONS §23). All nil in
	// the pre-S13.5 posture: intake resolves no registry, deliverables pin
	// content only (snapshot_sha NULL), and the pre-task base stays empty.
	//
	// Registry is the S06.2 intake resolution seam (production impl over
	// repo_registry); Fingerprint + CitedEntryVersions feed the S02.6/S09.6
	// approval-staleness fingerprint; Snapshot is the S02.4d checkpoint
	// artifact ref (wired to Driver.Snapshot) AND the round-boundary snapshot
	// recorded on a minted revision; CreateRevisionRef creates the S13.1
	// platform ref at snapshot-fill time (R20); BaseContent resolves a
	// repo-backed deliverable's pre-task base for Compare's old-side 0 (R25);
	// WorkspaceCwd swaps the plain run dir for the project worktree on an
	// execute leg of a registered project (R34).
	Registry intake.Registry
	// Paused is the S10.4 pause-my-automation read seam (pause.go, B6-2B): the
	// execute leg consults it between stage sessions and parks the run when the
	// owner has paused their automation. Composed at the shell root over
	// *metering.Pause; nil ⇒ nobody is paused (the pre-0017 behavior).
	Paused func(ctx context.Context, userID string) (bool, error)

	// The BENCH-REG §2 direct-arm seams (direct.go, B6-2C), composed at the shell
	// root over internal/benchmark. This package never imports internal/benchmark
	// and that package never imports this one — the §34/§35 seam posture, held by
	// the benchmark import wall — so the two things the `.direct` leg needs cross
	// the boundary as func fields, exactly like Snapshot/LedgerRevision/Paused.
	//
	// BenchmarkStatement resolves the frozen §2 task statement from the S06
	// artifact of record (hash-verified at the seam); BenchmarkCapture persists
	// the single shot's final text against the run, reporting whether a pair took
	// it. BOTH nil is the sanctioned no-benchmark posture (tests and any process
	// composed without the S14.7 practice) and a `.direct` run dispatched there
	// fails LOUDLY — the arm is never run on an invented prompt.
	BenchmarkStatement func(ctx context.Context, taskID string) (string, error)
	BenchmarkCapture   func(ctx context.Context, runID, text string) (bool, error)

	Fingerprint        func(ctx context.Context, project string) (run.Fingerprint, error)
	CitedEntryVersions func(ctx context.Context, keys []string) (map[string]string, error)
	Snapshot           func(ctx context.Context, runID string) (string, error)
	CreateRevisionRef  func(ctx context.Context, runID, ref, snapshotSHA string) error
	BaseContent        review.BaseContentSource
	WorkspaceCwd       func(ctx context.Context, runID string) (path string, ok bool, err error)

	// The S13.7 onboarding-as-task seams (R5/F1), wired over internal/project.
	// OnboardStart runs register → clone → scan → draft, returning the drafted
	// card payload (idempotent under re-dispatch); OnboardApprove activates the
	// pending entry on owner approval (D10), applying an optional edited draft.
	// Nil = the onboarding run role is unavailable (StartOnboarding errors).
	OnboardStart   func(ctx context.Context, projectID, owner, name, source string) (json.RawMessage, error)
	OnboardApprove func(ctx context.Context, projectID, owner string, editedDraft json.RawMessage) error

	Logger *slog.Logger
	Now    func() time.Time
}
