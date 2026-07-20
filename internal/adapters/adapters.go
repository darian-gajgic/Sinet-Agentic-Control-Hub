// Package adapters is the adapter seam of Spec S01.3 (and the adapters
// module seam of S01.1): the D3 adapter contract of Spec S03.1, the ONLY
// component that touches engine specifics. Everything above it — scheduler,
// watchdogs, the run FSM (S02), orchestration (S04), metering (S10) —
// speaks this contract and nothing else. Engines run UNMODIFIED behind it
// (S16.1 adopt-don't-fork; S16.6): a new engine or provider lane is one new
// adapter with orchestration, metering, and state untouched.
//
// The seven S03.1 verbs map onto this package as:
//
//	start        → Adapter.Start
//	stream       → Session.Events (closed platform event set below)
//	checkpoint   → per-paid-call Usage events + Session.Cursor; the payload
//	               and store are Spec S02.4's (gates.Checkpoints) — the
//	               Driver in this package persists them
//	pause        → Session.Pause (interrupt-at-safe-boundary + park; no
//	               true suspend exists on any v0 engine)
//	resume       → Adapter.Resume (FULL invocation re-supply from the park
//	               record; freshness-gating per S02.6 is the caller's duty)
//	cancel       → Session.Cancel (boundary → engine-abort → group-TERM →
//	               KILL ladder); the disposition returns via Session.Wait
//	report_usage → Usage rows on events and Outcome.Totals; priced by
//	               Sinet's own table, never the engine (D5; Spec S10)
//
// Verbs are shaped as a strict superset of ACP's stable verb set so a
// future transport convergence is a swap, not a re-architecture (S03.1).
package adapters

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// Substrate names (Spec S03.2): v0 ships exactly two substrates carrying
// three lanes; additions are config-only (S03.6).
const (
	// SubstrateClaudeCLI is the wrapped `claude` CLI (Anthropic lane),
	// pinned per components.lock; bump procedure Spec S03.3.
	SubstrateClaudeCLI = "claude-cli"
	// SubstrateOpencode is the pinned `opencode serve` (Z.AI + local
	// lanes). Its adapter lands with its consuming packet; the name is
	// fixed here so runs.substrate values converge.
	SubstrateOpencode = "opencode"
)

// Lane names (Spec S03.2).
const (
	LaneAnthropic = "anthropic"
	LaneZAI       = "zai"
	LaneLocal     = "local"
)

// EventKind enumerates the closed platform event set of Spec S03.1:
// event ∈ {message, usage, rate_limit, gate_ask, tool_result, done}.
//
// Forward-tolerant parsing is a MUST on every adapter (S03.1, P-T01-2):
// unknown ENGINE event types are logged and skipped, never fatal, and never
// minted as new platform kinds outside this set.
type EventKind string

const (
	KindMessage    EventKind = "message"
	KindUsage      EventKind = "usage"
	KindRateLimit  EventKind = "rate_limit"
	KindGateAsk    EventKind = "gate_ask"
	KindToolResult EventKind = "tool_result"
	KindDone       EventKind = "done"
)

// EventTypePrefix namespaces adapter-observed events in run_events
// (type = EventTypePrefix + Kind). Names are provisional pending the S14
// event contract (B5), matching the B0-3/B0-4 precedent.
const EventTypePrefix = "engine."

// engineEventSchemaVersion versions the engine.* event payloads.
const engineEventSchemaVersion = 1

// ExcerptCap bounds message/tool-result excerpts embedded in event
// payloads (refs-not-blobs, P-T07-5: full content lives in the engine
// transcript and its copy-aside, indexed via engine_sessions). Not a ⚙ —
// S18 ratifies no such key (the sseBatchSize precedent, CONVENTIONS §7);
// the ⚙ state.event_payload_cap backstop still applies at append.
const ExcerptCap = 4096

// Event is one engine event normalized to the platform contract. Payload is
// the bounded platform payload the Driver appends to run_events; the raw
// engine frame is never forwarded wholesale (P-T07-5).
type Event struct {
	Kind    EventKind
	Payload json.RawMessage

	// Usage is set when Kind == KindUsage — one paid model call, the D7
	// checkpoint trigger (Spec S02.4).
	Usage *Usage
	// Ask is set when Kind == KindGateAsk — the durable ask-record input,
	// a platform record from the moment the ask is observed (Spec S03.4).
	Ask *Ask
}

// Usage is the S02.4(a) usage block for one paid call, extracted verbatim
// from engine reporting. Sinet meters independently (D4) and prices from
// its own table (D5, Spec S10); EngineCostUSD is a cross-check figure only.
type Usage struct {
	ModelID string

	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	// CacheCreationByTTL splits cache_creation by TTL bucket (S02.4a),
	// e.g. "ephemeral_5m" / "ephemeral_1h".
	CacheCreationByTTL map[string]int64

	// EngineCostUSD is the engine-reported figure (emitted under
	// subscription auth on the Anthropic lane — the D5 reporting figure).
	EngineCostUSD float64

	// MessageID and MessageIndex locate the paid call in the session
	// (S02.4b message index). Engines may split one model call across
	// several stream frames; adapters group by message id so one paid call
	// is exactly one Usage event.
	MessageID    string
	MessageIndex int64

	// Total marks a run-total row (terminal result envelope) rather than a
	// per-call row. Totals ride Outcome and events; only per-call rows
	// (Total == false) trigger checkpoints — D7 is per paid call.
	Total bool

	// Raw is the verbatim engine usage object (bounded), kept as
	// cross-check material for the S10 ledger.
	Raw json.RawMessage
}

// Cursor is the S02.4(b) session cursor: substrate, the engine session id
// AS REPORTED by the engine (never the requested id), message index, cwd
// key, transcript path. Engine transcripts are NEVER durable state
// (P-T01-1): the cursor is a resume optimization and harvest index only.
type Cursor struct {
	Substrate      string
	SessionID      string
	MessageIndex   int64
	CwdKey         string
	TranscriptPath string
}

// Ask is one observed gate/question (Spec S03.4). The platform ask-record
// is authoritative from the moment of observation on every lane —
// engine-side park state is volatile (P-T01-1, P-T02-1).
type Ask struct {
	// ID is the engine-side ask identity (Claude lane: the tool_use_id of
	// the deferred call).
	ID        string
	ToolName  string
	ToolInput json.RawMessage

	// EngineExpiry is when the engine's own retention would evaporate the
	// parked state (P-T02-1); zero when the engine advertises none. The
	// platform record outlives it regardless.
	EngineExpiry time.Time

	// Park is the invocation-reconstruction snapshot needed to resume.
	Park *ParkRecord
}

// Park reasons.
const (
	ParkReasonGateAsk = "gate_ask" // exit-park on an observed gate (S03.4)
	ParkReasonPause   = "pause"    // platform-requested boundary interrupt (S03.1 pause)
)

// ParkRecord is the S03.1 park_record: the full-invocation snapshot that
// makes resume a complete re-supply. The engine restores NOTHING on resume
// — not hooks, not permission mode (Spec S03.4, measured): a resume built
// from anything less than this record silently executes parked calls
// unreviewed.
type ParkRecord struct {
	RunID     string
	Substrate string
	Cursor    Cursor
	Reason    string

	// DeferredToolUse carries the engine's own pending-call object
	// verbatim ({id,name,input} on the Claude lane exit JSON) so the ask
	// record is built from the exit alone, no transcript parsing (S03.4).
	DeferredToolUse json.RawMessage `json:",omitempty"`

	// Start is the entire original invocation (S03.4 obligation: settings
	// content, permission mode, model, tool allowlist all re-supplied).
	Start StartRequest

	// Fingerprint is the invocation-config fingerprint of S02.4(e)
	// (settings hash + permission mode + model), the freshness-pass input.
	Fingerprint string

	ParkedAt time.Time
}

// StartRequest is the S03.1 start verb input: (run_id, compiled_worker,
// model, owner_cred_ref) plus the platform-owned spawn context.
type StartRequest struct {
	RunID  string
	UserID string

	// Worker is the compiled worker (S03.5 engine lowering): compiled
	// config is the ONLY config the engine sees; one knob per config
	// channel, enforced per lane.
	Worker CompiledWorker

	// Model is chosen per invocation by the platform.
	Model string

	// OwnerCredRef is the per-run owner credential REFERENCE (D2): the
	// credential rides the engine process, never the sandbox, and never
	// passes through this package as material — on the Anthropic lane it
	// is the per-user CLAUDE_CONFIG_DIR path (S03.2; credential lookup
	// lives in the config root). Empty = engine-default resolution (dev
	// posture); broker-mediated injection is Spec S11 (B1-3).
	OwnerCredRef string

	// Class is the run's compiled confinement class (Spec S11.6: "C0".."C2"
	// at v0), carried declaratively from the control-plane record. Empty +
	// a Confiner defaults to C1 (the strongest, simplest class). Consumed by
	// the Confiner; opaque to the adapter (the class is enforcement state a
	// worker can never alter, 14.2).
	Class string

	// Confiner, when set, wraps the engine spawn in the composed per-run
	// sandbox (Spec S11): the adapter builds its *exec.Cmd through it instead
	// of spawning the engine directly. Nil = unconfined dev spawn (the B1-1
	// posture; CONVENTIONS §10). Never serialized (json:"-"): confinement is
	// recompiled every run from control-plane data, never read from a park
	// record (S11.6/S11.7).
	Confiner Confiner `json:"-"`

	// CredInject, when set, resolves and injects owner/engine credentials
	// into the engine environment at spawn — the S11.5 broker injection,
	// S01.6 "engines receive credentials at start". Resolved FRESH per spawn
	// from the broker: the secret is NEVER stored on this struct, in a park
	// record, in the event log, or in the DB (json:"-"). For a confined
	// (in-sandbox) engine the injected value on the model channel is a
	// sentinel; the real token rides the S11.5 injection proxy.
	CredInject func(base []string) ([]string, error) `json:"-"`

	// Cwd is the isolated run working directory (S03.5: cwd is a config
	// channel; the engine must never walk up into ambient project config).
	Cwd string

	// WorkDir is the platform-owned scratch directory for lowered config
	// (settings file, gate-hook control dir) — never inside Cwd, never a
	// shared project dir (S03.5).
	WorkDir string

	// Platform ceilings (3.7). The engine-side belts are derived at
	// lowering as ⚙ adapter.engine_ceiling_backstop_mult × these, so the
	// platform gate always fires first (S03.4 ceiling ordering).
	CeilingCostUSD float64
	CeilingSteps   int64
}

// CompiledWorker is the compiled worker body+config an engine receives
// (S03.5). The compiler itself is Spec S08's (B3); at B1 callers supply the
// compiled fields directly.
type CompiledWorker struct {
	// Prompt is the initial user-turn body.
	Prompt string
	// SystemPromptAppend rides the lane's append-system-prompt knob.
	SystemPromptAppend string
	// AgentsJSON + AgentName inject the agent definition inline — no disk
	// write into any shared dir (S03.5 agent/prompt-body row).
	AgentsJSON json.RawMessage
	AgentName  string
	// ToolAllowlist is the default-deny toolset. It MUST exclude the
	// engine's native-spawn family (S03.5 sole-controller posture);
	// adapters validate and refuse.
	ToolAllowlist []string
	// GatedTools are the tools whose calls park on the platform gate
	// (S03.4); the adapter wires the lane's gate mechanism for exactly
	// these.
	GatedTools []string
	// PermissionMode is part of the invocation snapshot (S03.4: measured
	// NOT restored by the engine on resume; always re-supplied).
	PermissionMode string
	// SessionStartContextPath, when set, wires the lane's deterministic
	// re-injection channel (Spec S05.4/S05.7 step 3; resolved by the B1-4
	// spike, 2026-07-20): a SessionStart hook matched on source
	// startup|resume|compact re-emits this platform-placed pinned-context
	// file (the ledger's pinned sections, verbatim) as additionalContext.
	// The file is placed by the S05 assembly and manifested like every
	// injection; empty = no re-injection hook.
	SessionStartContextPath string
	// Schema versions for the S02.4(e) checkpoint block.
	ToolSchemaVersion   string
	PromptSchemaVersion string
}

// Answer carries the human/platform answer that unparks a run
// (Adapter.Resume input).
type Answer struct {
	// AskID names the ask being answered (must match the park record's
	// deferred call on the gate path).
	AskID string
	// UpdatedInput, when set, replaces the gated call's input wholesale
	// (S03.4: the hook returns allow + updatedInput). Nil approves the
	// original input unchanged.
	UpdatedInput json.RawMessage
	// Continuation is the user-turn body for resuming a pause-park (a
	// gate-defer resume needs no prompt at all — S03.4).
	Continuation string
}

// OutcomeKind classifies how a session ended.
type OutcomeKind string

const (
	// OutcomeCompleted: terminal result, run work finished.
	OutcomeCompleted OutcomeKind = "completed"
	// OutcomeParked: interrupt-at-safe-boundary + park (gate ask or
	// pause); Outcome.Park is set (S03.4).
	OutcomeParked OutcomeKind = "parked"
	// OutcomeDiedAtGate: an engine budget/ceiling trip pre-empted the park
	// (S03.4 ceiling ordering; S02.3 died-at-gate).
	OutcomeDiedAtGate OutcomeKind = "died-at-gate"
	// OutcomeCanceled: the cancel ladder ended the session (S03.1).
	OutcomeCanceled OutcomeKind = "canceled"
	// OutcomeCrashed: abnormal end with nothing to park — fork-from-
	// checkpoint is the standard recovery (P-T01-4; Spec S02.5).
	OutcomeCrashed OutcomeKind = "crashed"
)

// Outcome is the terminal disposition of one session (Session.Wait).
type Outcome struct {
	Kind OutcomeKind

	// Park is set for OutcomeParked (and, when the ask was observed before
	// the preempt, for OutcomeDiedAtGate).
	Park *ParkRecord
	// Ask is set when the park is an observed gate ask.
	Ask *Ask

	// Totals is the run-total usage row from the terminal envelope
	// (Total == true), when the engine emitted one.
	Totals *Usage
	// Result is the bounded terminal envelope (cross-check material).
	Result json.RawMessage

	// GateFallback reports the S03.4 detection contract: more than one
	// gated-tool fire in a single turn without a defer exit ⇒ the engine
	// silently fell back from the exit-park primitive (P-T02-4). Handling
	// policy is ⚙ adapter.parallel_gate_fallback.
	GateFallback bool

	// Detail is a short human-readable cause (crash reason, cancel stage).
	Detail string
}

// SpawnSpec is the engine's own lowered spawn — argv, environment, and the
// platform-owned paths that must be visible inside the sandbox — that a
// Confiner wraps in the per-run sandbox (Spec S11). Defined here so the
// adapter seam owns the type and internal/sandbox implements the seam
// (sandbox → adapters is the only import direction; no cycle).
type SpawnSpec struct {
	// Argv is the engine command (Argv[0] = binary); Env is the lowered
	// engine environment (empty-env base; a sentinel on the credential
	// channel for a confined engine — D2/S11.5).
	Argv []string
	Env  []string
	// Workspace is the per-run workspace host path (bound at the same path
	// inside; ro for C1, rw for C2; the chdir target — Spec S11.3).
	Workspace string
	// ROConfig are platform-owned config paths bound read-only (the lowered
	// settings dir; the engine can never rewrite its own config — S11.7
	// P-T09-1 deny-writes-to-config).
	ROConfig []string
	// RWExchange are platform-owned exchange dirs bound read-write (the S03.4
	// gate control dir: the in-sandbox hook appends fires.log).
	RWExchange []string
	// EnginePrefix is an extra host tree bound read-only so a real engine
	// installed outside /usr is reachable inside the sandbox. Empty for
	// system-path binaries.
	EnginePrefix string
}

// Confiner wraps an engine spawn in the composed per-run sandbox (Spec S11).
// It returns the *exec.Cmd to start (bwrap prepended) and a cleanup to run
// after Start. The implementation is internal/sandbox.Composer.
type Confiner interface {
	Confine(req StartRequest, spec SpawnSpec) (*exec.Cmd, func(), error)
}

// Adapter is the per-substrate implementation of the D3 contract. One
// Adapter value serves many runs; each Start/Resume yields one Session
// (one engine invocation).
type Adapter interface {
	// Substrate returns the fixed substrate name (runs.substrate value).
	Substrate() string

	// Start spawns the engine for a run with compiled config (S03.1
	// start): per-run owner credential in the engine process, never the
	// sandbox (D2); all spawning stays control-plane-owned (S03.5).
	Start(ctx context.Context, req StartRequest) (Session, error)

	// Resume re-supplies the ENTIRE invocation from the park record
	// (S03.4 obligation), plus the answer when unparking a gate ask.
	// Freshness-gating (S02.6) happens before this call, in the caller.
	Resume(ctx context.Context, rec ParkRecord, ans *Answer) (Session, error)
}

// Session is one live engine invocation.
type Session interface {
	// Events streams normalized platform events (S03.1 stream). The
	// channel closes when the engine invocation ends; Wait then reports
	// the disposition.
	Events() <-chan Event

	// Cursor reports the current S02.4(b) session cursor. The session id
	// is the engine-REPORTED id from the moment the engine announces it.
	Cursor() Cursor

	// Fingerprint is the invocation-config fingerprint (S02.4e) of this
	// invocation.
	Fingerprint() string

	// Pause requests interrupt-at-safe-boundary + park (S03.1 pause):
	// never mid-tool-call (P-T01-4). The park record arrives via Wait.
	Pause(ctx context.Context) error

	// Cancel runs the cancel ladder (S03.1): safe-boundary → engine abort
	// → process-group TERM → KILL. The disposition arrives via Wait.
	Cancel(ctx context.Context) error

	// Wait blocks until the invocation ends and returns its Outcome.
	// It is safe to call after the Events channel closes.
	Wait(ctx context.Context) (Outcome, error)
}
