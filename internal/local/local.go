// Package local is the Spec S12 platform-plane surface of the permanent
// free tier: the duty-alias `/v1` client, alias resolution over the ⚙
// map + the committed model manifest, llama-swap config generation, the
// eager-unload verb, the GameMode hook machinery, and the conformance
// suite that asserts the pinned serving stack behaviorally.
//
// Consumer class (Spec S12.1): this package is the PLATFORM PLANE only
// (loopback, direct to llama-swap, no tokens/bridge — the GPU broker's
// sandboxed plane is Spec S12.6, B4-6). Its callers are S12.1 class (b):
// the control plane invokes S12.4 duty aliases directly with no engine
// session involved (intake triage, tie-break, utility drafting). The
// class-(a) local ENGINE lane (runs executing on local models via the
// opencode adapter, S03.2) has NO v0 consumer — the local lane exists as
// metering/routing vocabulary and nothing dispatches onto it here
// (P3/STATE binding reading).
//
// CORRECTED 2026-08-24 (P3-LN-2B R23): the reason changed even though the
// fact did not. This clause used to rest on "no second adapter exists";
// the opencode adapter landed at P3-LN-1 and is registered at LN-2A, so
// what is actually absent is a commissioned local PROVIDER ENTRY — a
// separate lane commissioning, not a missing substrate.
//
// Import wall (brief R24, CONVENTIONS §26): this package imports
// storage/eventlog/gates/settings-interface + stdlib ONLY. It NEVER
// imports adapters/stage/intake/worker/verify/memory/broker/scheduler/
// review/project — the duty-seam adapters that satisfy intake.Classifier
// and worker.TieBreaker live in internal/stage (which already imports
// those), exactly as internal/stage/router.go bridges worker→intake
// (B3-3). The metering marker this package writes into the D7 usage
// block is a JSON wire contract read by internal/metering; neither
// package imports the other.
//
// Every local call — class (b) here — writes ONE D7 ledger usage row on
// the consuming run (Spec S12.1), priced $0 as the permanent free tier;
// duty.go owns that write, usage.go the marker.
package local

import (
	"errors"
	"strings"
)

// LaneLocal is the S03.2 local lane name stamped into the D7 usage marker
// so the metering fold prices the row on the permanent free tier (Spec
// S12.1). It is a spec-fixed lane string (S03.2 names anthropic/zai/local);
// internal/metering reads the same literal from the marker — the wire
// contract, not a shared symbol (the import wall stays clean).
const LaneLocal = "local"

// Serving-stack pins, LIVE-verified 2026-07-22 and test-coupled to the
// components.lock entries (CONVENTIONS §10 Pin↔lock precedent; the
// conformance suite reports an installed-vs-pin delta LOUDLY, never
// retargets). llama-swap ships static release binaries; official llama.cpp
// ships NO prebuilt Linux CUDA binary (verified — Ubuntu assets are
// CPU/Vulkan/ROCm/OpenVINO/SYCL only), so the CUDA serving build is the
// R3(b) user-level source build (recorded in the lock notes + packet).
const (
	// LlamaSwapPin is the llama-swap organ-unit release pin (MIT). The
	// S16.3 row observed "v240 current"; live-current at build time is v241.
	LlamaSwapPin = "v241"
	// LlamaCppPin is the llama.cpp `llama-server` engine b-tag pin (MIT).
	LlamaCppPin = "b10085"
)

// Settings is this package's narrow view of the ⚙ registry (CONVENTIONS §6:
// dotted-key reads, never constants). *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
	Bool(key string) (bool, error)
	String(key string) (string, error)
	StringMap(key string) (map[string]string, error)
}

// ⚙ keys consumed by this package (all declared since B0-2, Spec S18 — NO new
// key; the 118-key tally stays green, R21). B4-6 additionally consumes the
// three GPU-broker/VRAM keys that B4-5 only referenced.
const (
	keyAlias        = "local.alias"               // TypeMap duty→seat (S12.4; B4-5 R15)
	keyTTLFast      = "local.ttl.fast_s"          // fast/CPU-class seat TTL (B4-5 R10)
	keyTTLWorkhorse = "local.ttl.workhorse_s"     // workhorse-class seat TTL (B4-5 R10)
	keyGameModeHook = "local.gamemode_hook"       // gates the whole GameMode behavior (B4-5 R13)
	keyUnloadGrace  = "local.unload.term_grace_s" // llama-swap unloadTimeout (B4-5) + the Sinet-side SIGTERM→grace→SIGKILL grace (R11)

	keyGuardBandMB         = "local.vram.guard_band_mb"      // admission math headroom (R9)
	keyBatteryGPUAdmission = "local.battery.gpu_admission"   // battery admission {never,urgent-only,always} (R14)
	keySandboxLogprobs     = "local.broker.sandbox_logprobs" // sandboxed-plane logprobs gate, per-template (R5)
)

// Battery-admission enum values (⚙ local.battery.gpu_admission; Spec S12.8).
const (
	BatteryNever      = "never"
	BatteryUrgentOnly = "urgent-only"
	BatteryAlways     = "always"
)

// Duty-alias names (Spec S12.4 registry; alias names [coordinator-draft]).
// The map key set of ⚙ local.alias; unknown alias = loud error (R15).
const (
	AliasUtility          = "utility"
	AliasIntakeTriage     = "intake-triage"
	AliasWatchdog         = "watchdog-disambiguator"
	AliasWatchlist        = "watchlist-triage"
	AliasIntentFilling    = "intent-filling"
	AliasSQLOpen          = "sql-open"
	AliasEntailment       = "entailment"
	AliasContradiction    = "contradiction-screen"
	AliasDistillSummarize = "distill-summarize"
	AliasEmbedder         = "embedder"
)

// Errors callers branch on.
var (
	// ErrUnknownAlias reports a duty alias absent from ⚙ local.alias (R15).
	ErrUnknownAlias = errors.New("local: unknown duty alias")
	// ErrEmbedderPostGate refuses the embedder alias: the G2 Def.8 vector
	// gate (embeddings only at ≥5% lexical-miss rate or >5,000 entries) has
	// not opened, so the embedder is neither pulled nor served (R15/§8
	// reading 9). The alias row stays in the registry as S12.4 data.
	ErrEmbedderPostGate = errors.New("local: embedder alias refused — the G2 Def.8 vector gate has not opened (S12.3 post-gate)")
	// ErrNoSeat reports an alias that resolves to a seat absent from the
	// manifest (a config error, surfaced loudly).
	ErrNoSeat = errors.New("local: alias resolves to no manifest seat")
	// ErrNotServable reports a seat with no serving path at the pinned
	// build (recorded honestly in the manifest, never faked into config).
	ErrNotServable = errors.New("local: seat has no serving path at the pinned build")
	// ErrStackAbsent reports the local serving stack unconfigured or
	// unreachable — callers degrade per each duty's S12.4/S06 row (R17).
	ErrStackAbsent = errors.New("local: serving stack is not configured")
	// ErrAdmissionsStopped reports the eager-unload operator-wins switch is
	// engaged — new local-lane calls are refused until resume (R12).
	ErrAdmissionsStopped = errors.New("local: local-lane admissions stopped (eager-unload engaged)")
	// ErrAbstained reports a duty schema's explicit abstain/unclear member
	// was chosen — never a fabricated label (S12.4 cross-cutting rule; the
	// caller degrades per its S12.4 row).
	ErrAbstained = errors.New("local: duty abstained (schema unclear member)")
	// ErrGPUAdmissionRefused reports the VRAM/battery admission check refused a
	// load (R9/R14): live memory.free below the guard band (a game/compositor
	// holds VRAM) or the battery policy refuses. The caller degrades per its
	// S12.4/S06 row — never a fake load (R17). The reason string carries the
	// live numbers.
	ErrGPUAdmissionRefused = errors.New("local: GPU admission refused")
	// ErrTokenInvalid reports a sandboxed-plane call with an absent/expired
	// per-run bearer token (R3) — the GPU broker never serves it.
	ErrTokenInvalid = errors.New("local: sandboxed-plane bearer token invalid or expired")
	// ErrModelNotAllowed reports a sandboxed call naming a duty alias outside the
	// run's grant allowlist (R3/R4) — the broker refuses it.
	ErrModelNotAllowed = errors.New("local: duty alias not in the run's allowlist")
	// ErrLogprobsRefused reports a sandboxed call requesting logprobs while ⚙
	// local.broker.sandbox_logprobs is off for the run (R5 side-channel
	// reduction) — the platform plane keeps logprobs, the sandboxed plane does not.
	ErrLogprobsRefused = errors.New("local: logprobs refused for the sandboxed caller (⚙ local.broker.sandbox_logprobs off)")
	// ErrTruncated reports a duty reply the engine STOPPED at the length cap
	// (finish_reason "length", or output tokens at the cap) rather than one the
	// model finished. Such a reply is not a result: a constrained-JSON duty
	// returns a half-written object that fails to decode, and a drafting duty
	// returns a sentence that stops mid-word. Before PH-1 this arrived at the
	// caller as an opaque decode failure — indistinguishable from a model that
	// answered badly — and every caller degraded silently. It is now a NAMED
	// cause callers match with errors.Is, so a cap regression is loud on its
	// first occurrence instead of after a cold walk (PH-1 F2; S12.4).
	//
	// The D7 usage row is written BEFORE this error is returned: the call really
	// did run and really did spend those tokens, and R18 meters what happened,
	// not what succeeded.
	ErrTruncated = errors.New("local: duty reply hit its length cap (truncated, not finished)")
)

// normalizeAlias trims and lowercases an alias for map lookup tolerance.
func normalizeAlias(a string) string { return strings.TrimSpace(strings.ToLower(a)) }
