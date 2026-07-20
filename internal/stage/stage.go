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
//     + PlacePinned + the SessionStart re-injection channel), driven through
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
// Boundary discipline (Spec S19.5): worker/model selection is Spec S08's
// (B3) — the model here is a documented dev default behind the Config
// seams; orchestration/helpers are Spec S04's (B3) — the stage-split
// EXECUTION of an overflow proposal waits there, the proposal events land
// now; deliverable/revision mechanics are Spec S13's (B4) — the deliverable
// at B2-4 is the execute session's result text, captured as a durable
// artifact; the local tier is Spec S12's (B4) — the Utility duty stays
// unwired rather than faked onto a paid engine (S06.10 pins it local).
package stage

import (
	"context"
	"log/slog"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
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
// turns"). DELIBERATELY UNCONSUMED at B2-4 — a named seam, never faked
// (packet rule): the Claude lane's stage sessions are single `-p`
// invocations and the platform has NO mid-session turn-injection channel
// (the B1-4 spike mapped the channels: prompt assembly at start,
// SessionStart re-injection on startup|resume|compact, PreToolUse for gated
// tools — none is a per-N-turns recitation writer). The channel arrives
// with the S04 orchestration layer (B3, helper/coordinator sessions) or an
// engine-side hook that fires per turn; consuming the key before the
// channel exists would fake the mechanism.
const keyRecitationInterval = "context.recitation_interval_turns"

// anthropicContextWindowTokens is the Anthropic-lane model context window
// the stage-fit budget measures against (Spec S05.3 "the lane's
// pinned-model context window"). A model FACT, not an operator ⚙ (S18
// declares no window key); the per-model window record is Spec S08's worker
// registry (B3) — until it lands, the current Anthropic production models
// all carry a 200k window and this single documented constant is the
// honest dev posture (flagged to the B2 gate; the standing settings-tab
// directive applies). Recalibration per model generation is the S14 eval
// machinery's (Spec S05.3).
const anthropicContextWindowTokens = 200_000

// DefaultModel is the dev-mode execution/ceremony model until Spec S08
// worker/model selection lands (B3). The cheap frontier model of the B1-1
// live-smoke precedent; overridable per Config.Model. A documented seam
// stub, not a routing decision — S08 owns selection.
const DefaultModel = "claude-haiku-4-5"

// Run-role suffixes (the walking-skeleton run-naming convention; intake
// fixed `<task>.intake` at B2-2 — CONVENTIONS §14; the skeleton fixes
// `.execute`/`.verify` at B2-4). internal/metering derives receipt purpose
// tags from the same constants (S06.10/S07.11 ceremony/verification
// itemization); a first-class run-role record is S04/S08 machinery (B3).
const (
	RunSuffixIntake  = metering.RunSuffixIntake
	RunSuffixExecute = metering.RunSuffixExecute
	RunSuffixVerify  = metering.RunSuffixVerify
)

// EventContextOverflow is the S05.3 overflow event type: emitted run-scoped
// when a stage session's measured context footprint crosses the ⚙
// thresholds, carrying the proposal (stage split, or re-plan on a second
// overflow / an overweight brief). Name provisional pending the S14 event
// contract (B5), extending the CONVENTIONS §7/§8/§13/§14 naming note.
const EventContextOverflow = "context.overflow"

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

	// Substrate/Lane/Model select the engine for skeleton-created runs
	// until Spec S08 routing lands (B3). Defaults: claude-cli / anthropic /
	// DefaultModel.
	Substrate string
	Lane      string
	Model     string

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

	Logger *slog.Logger
	Now    func() time.Time
}
