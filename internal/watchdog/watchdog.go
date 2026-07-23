// Package watchdog is the Spec S14.4 self-health watching suite: three tiers
// of anomaly detection that run on platform code plus the local tier, cost no
// allowance, and keep working when every paid window is empty.
//
//   - Tier 0 (tier0.go) — deterministic, model-free counters over run_events
//     (loop, ping-pong, error-loop, silence, spend, suspicious-completion),
//     re-derived STATELESS from a bounded recent-events window per run (Spec
//     S14.1: the event log IS the observability substrate — no side store).
//   - Tier 1 (tier1.go) — a local-model disambiguator on the S12.4
//     `watchdog-disambiguator` duty alias, invoked ONLY on a Tier-0 trigger.
//     It ANNOTATES ({loop|productive|unclear} + confidence + evidence), never
//     gates, never kills; low margin ⇒ the verdict is `unclear` (Spec S12.4).
//   - Tier 2 (tier2.go) — pause-and-flag, ALWAYS on a Tier-0 trigger: the run
//     parks at its checkpoint boundary via the S02.3 FSM and `watchdog.flagged`
//     is appended in the SAME WriteTx (containment is structural — the
//     scheduler admits no next paid call for a parked run, Spec S10.8).
//
// HARD INVARIANT (Spec S14.4 / G1 D1.3): NO auto-kill anywhere. No check
// kills, terminates, tombstones, or gates a run — the watchdog contains and
// parks (pause-and-flag). Tier-1 ANNOTATES, never decides; Tier-2 is always a
// pause. importwall_test.go's AST proof asserts this package invokes no kill/
// terminate/tombstone primitive.
//
// The suite also runs the four registered platform-health checks (sweep.go:
// resume-reconcile, listener-binding audit, organ-absence, event-log size) and
// the dead-man canary V2 (deadman.go). Scheduling is the shell's: a shell-owned
// sweep goroutine drives Sweep + DeadManProbe on structural intervals and a
// Tail loop feeds event-driven Tier-0 (Spec S10.7 / CONVENTIONS §7/§8 — the
// WAL-truncate / recovery-sweep precedent). This package owns the LOGIC; the
// shell owns WHEN (the independent-observer property for the dead-man canary).
//
// Import wall (brief R31): imports run/eventlog/metering/settings/storage/local
// + stdlib. internal/local NEVER imports internal/watchdog (no cycle);
// internal/api reads the flags from run_events without importing this package.
package watchdog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Event types PRODUCED by this suite (brief R20/R29). B5-1 declared these three
// as declare-only under FamilyWatchdog; B5-3 is their producer and flips them to
// minted in contract.go. They are consumed by dotted key elsewhere (the SSE
// topic map, the inbox projection) — this package is the sole emitter.
const (
	// EventFlagged is a Tier-2 pause-and-flag: rule id + trigger evidence
	// {signature, counts, spend-vs-baseline} + Tier-1 annotation ref + severity
	// (S14.4; satisfies the FamilyWatchdog required-field descriptor).
	EventFlagged = "watchdog.flagged"
	// EventAnnotated is the Tier-1 verdict referenced by a flag (S14.4).
	EventAnnotated = "watchdog.annotated"
	// EventSuppressed is a per-rule suppression — a tuning signal, never a
	// resolution (S14.4).
	EventSuppressed = "watchdog.suppressed"
)

// Rule ids (the anomaly class carried on every flag; the dedup key with the
// run, R22). A rule id IS its anomaly class.
const (
	RuleLoop                 = "watchdog.loop"                  // R2
	RulePingPong             = "watchdog.pingpong"              // R3
	RuleErrorLoop            = "watchdog.error_loop"            // R4
	RuleSilence              = "watchdog.silence"               // R5
	RuleSpend                = "watchdog.spend"                 // R6
	RuleSuspiciousCompletion = "watchdog.suspicious_completion" // R7
	RuleResumeReconcile      = "watchdog.resume_reconcile"      // R24
	RuleListener             = "watchdog.listener"              // R25
	RuleOrganAbsence         = "watchdog.organ_absence"         // R26
	RuleEventLogSize         = "watchdog.event_log_size"        // R27
	RuleDeadMan              = "watchdog.dead_man"              // R28-DMC / OQ5
	RuleFlagNowRate          = "watchdog.flag_now_rate"         // R23 meta-alert
)

// Severity is a flag's alert-routing class (R21) — distinct from the S15
// approval-inbox risk tiers.
const (
	SeverityFlagNow     = "flag-now"     // stalls/loops/spend on active runs; foreign listener; dead-man
	SeverityDailyDigest = "daily-digest" // everything else
)

// ⚙ keys consumed by dotted key at evaluation time (live-apply, R8/R33). All
// declared since B0-2 (Spec S18) — this package re-declares NONE (the 118/33
// tally stays green).
const (
	keyLoopRepeat      = "watchdog.loop_repeat"
	keyPingPongCycles  = "watchdog.pingpong_cycles"
	keyErrorLoop       = "watchdog.error_loop"
	keySilenceBudget   = "watchdog.silence_budget" // TypeMap <run_type>→seconds
	keySpendMedianMult = "watchdog.spend_median_mult"
	keySpendArmDays    = "watchdog.spend_arm_days"
	keyFlagNowTarget   = "watchdog.flag_now_target"
	keySuppressRetune  = "watchdog.suppress_retune_count"
	keyDeadAfter       = "recovery.dead_after" // the silence-budget seed (Spec S02)
)

// Structural constants (NOT ⚙ — the sseBatchSize precedent, §7; a knob for any
// of these is a future S18 amendment under the settings-tab directive, R33).
const (
	// recentWindow bounds the per-run recent-events lookback for the stateless
	// Tier-0 counters (R1). Comfortably above the largest ⚙ counter clamp
	// (pingpong 12 cycles = 24 calls) so a re-derivation never misses a trip.
	recentWindow = 128
	// allGenerations, passed as the generation to readRecentEvents, disables the
	// generation fence — the whole run across generations (still the recentWindow
	// bound). Used ONLY by the suspicious-completion path (drain-R1): the fence is
	// behavioral-only; a completed run's whole-run activity must be counted.
	allGenerations = -1
	// eventSchemaVersion versions the watchdog.* event payloads.
	eventSchemaVersion = 1
	// eventLogSizeThresholdBytes is the WAL/table-growth flag threshold (R27) —
	// a conservative structural constant; a future knob is an S18 amendment.
	eventLogSizeThresholdBytes = 512 << 20 // 512 MiB
)

// SweepInterval and DeadManInterval are the shell-owned driver cadences,
// exported so the shell reads them (the shell owns WHEN; this package owns the
// LOGIC). Structural constants (no new ⚙; the dead-man daily cadence and the
// sweep interval are interim under the settings-tab directive, OQ1/R28/R33).
const (
	SweepInterval   = 30 * time.Second
	DeadManInterval = 24 * time.Hour
)

// Settings is the watchdog-facing view of the ⚙ registry (dotted-key reads,
// never constants; §6). *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
	Duration(key string) (time.Duration, error)
	StringMap(key string) (map[string]string, error)
}

// SpendReader is the narrow metering seam for the S14.4 spend rule (R6/§5):
// the daily per-person priced total + trailing median (money-math stays in
// metering, S14.10 Layer-0). *metering.Ledger satisfies it. Nil ⇒ the spend
// rule is dormant. SpendOwners lists the day's spenders from the usage rows (a
// person whose runs all completed today is still checked — drain D11), so the
// sweep never derives its owner set from currently-active runs.
type SpendReader interface {
	SpendWindow(ctx context.Context, userID string, asOf time.Time, windowDays int) (metering.SpendWindow, error)
	SpendOwners(ctx context.Context, asOf time.Time) ([]string, error)
}

// AdvisoryMeter opens a short-lived platform run for a run-less advisory local
// call and returns its id plus a settle func (the §28 AdvisoryMeter seam). The
// Tier-1 $0 D7 row lands on it when the watched run is not checkpointable
// (terminal/parked — OQ4). Nil ⇒ a non-checkpointable watched run gets no
// Tier-1 annotation (skipped honestly; the flag still fires — Tier-2 is always).
type AdvisoryMeter func(ctx context.Context, purpose string) (runID string, settle func(), err error)

// ── Registered-check seams (R24–R27), satisfied by the shell, nil-safe ──

// ForeignListener is one non-loopback listener the recurring audit found beyond
// the operator allowlist (R25).
type ForeignListener struct {
	Addr    string `json:"addr"`
	Process string `json:"process"`
}

// ListenerAudit re-runs the S01.6/S01.8 listener-binding audit (R25), reusing
// the shell's listener-lint logic. Returns the foreign (non-allowlisted,
// non-loopback) listeners; empty ⇒ clean. Tested against fixtures — it never
// opens a socket.
type ListenerAudit func(ctx context.Context) ([]ForeignListener, error)

// OrganStatus is one adopted organ's liveness (R26).
type OrganStatus struct {
	Organ string `json:"organ"`
	Up    bool   `json:"up"`
	Note  string `json:"note,omitempty"`
}

// OrganLiveness probes the adopted organs (watchlist executor, local-model
// units, port-pool — the shell owns the unit set, R26). A down organ surfaces
// as a degraded-mode digest flag.
type OrganLiveness func(ctx context.Context) ([]OrganStatus, error)

// WakeStatus reports the S01.7 wake-completion signal (R24): whether the last
// wake's steps (recovery flush, network-identity reconcile, listener re-audit)
// completed. Pending is false when no wake is being reconciled.
type WakeStatus struct {
	Pending   bool     `json:"pending"`   // a wake is being reconciled
	Completed bool     `json:"completed"` // all steps done
	Missing   []string `json:"missing,omitempty"`
}

// WakeReconcile reports the shell's wake-completion signal (R24). Nil/absent ⇒
// the check is honestly-absent (the S01.7 logind wiring is a later packet).
type WakeReconcile func(ctx context.Context) (WakeStatus, error)

// WALStat is the storage-growth reading behind the event-log-size watch (R27).
type WALStat struct {
	WALBytes    int64 `json:"wal_bytes"`
	EventRows   int64 `json:"event_rows"`
	EventsBytes int64 `json:"events_bytes"`
}

// WALSizeReader reads the S02 WAL/table-growth meters (R27). Nil ⇒ the check
// reads the run_events row count directly (the fallback below).
type WALSizeReader func(ctx context.Context) (WALStat, error)

// ── flag payloads (satisfy the FamilyWatchdog required-field descriptor) ──

// SpendEvidence is the spend-vs-baseline trigger evidence (R6/R17).
type SpendEvidence struct {
	TodayUSD      float64 `json:"today_usd"`
	MedianUSD     float64 `json:"median_usd"`
	Multiplier    float64 `json:"multiplier"`
	DaysOfHistory int     `json:"days_of_history"`
	Armed         bool    `json:"armed"`
	Dormant       bool    `json:"dormant"` // v0: empty price table → no dollars (R6)
}

// FlagPayload is the watchdog.flagged event body (R17): rule id + severity +
// anomaly class (the dedup key) + trigger evidence + the Tier-1 annotation ref.
type FlagPayload struct {
	Rule            string         `json:"rule"`
	AnomalyClass    string         `json:"anomaly_class"`
	Severity        string         `json:"severity"`
	Signature       string         `json:"signature,omitempty"`
	Counts          map[string]int `json:"counts,omitempty"`
	SpendVsBaseline *SpendEvidence `json:"spend_vs_baseline,omitempty"`
	AnnotationRef   *int64         `json:"annotation_ref,omitempty"` // event_seq of the watchdog.annotated
	Detail          string         `json:"detail,omitempty"`
	Parked          bool           `json:"parked"` // Tier-2 parked the run (false for terminal/platform)
	Actor           string         `json:"actor"`  // always the platform (S14.4 acts are platform)
}

// AnnotationPayload is the watchdog.annotated event body (R12/R17): the Tier-1
// verdict + confidence + one-line evidence, referenced by a flag.
type AnnotationPayload struct {
	Rule       string  `json:"rule"`
	Verdict    string  `json:"verdict"` // loop | productive | unclear
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
	Reason     string  `json:"reason,omitempty"`
	Model      string  `json:"model,omitempty"`
	Abstained  bool    `json:"abstained,omitempty"`
	Actor      string  `json:"actor"`
}

// SuppressPayload is the watchdog.suppressed event body (R19): the rule + the
// running suppress count; the retune-proposal card is emitted separately.
type SuppressPayload struct {
	Rule    string `json:"rule"`
	Actor   string `json:"actor"`
	Count   int    `json:"count"`
	Reason  string `json:"reason,omitempty"`
	Propose bool   `json:"propose_retune"` // this suppress hit the ⚙ retune threshold (R19)
}

// Deps wires a Watchdog at the composition root. Only DB/Log/Runs/Settings are
// required; every model/metering/check seam is nil-safe so an unconfigured
// stack degrades honestly (Tier-1 skips, spend dormant, checks absent) while
// Tier-0/Tier-2 keep working (they need no model — S14.4).
type Deps struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Runs     *run.Store
	Settings Settings

	// Duty is the S12.4 local disambiguator seat (Tier-1). Nil ⇒ no annotation
	// (the flag still fires; Tier-2 is always).
	Duty *local.Duty
	// Meter lands the Tier-1 $0 D7 row for a non-checkpointable watched run
	// (OQ4). Nil ⇒ such runs get no annotation, surfaced honestly.
	Meter AdvisoryMeter
	// Spend backs the S14.4 spend rule (R6). Nil ⇒ dormant.
	Spend SpendReader

	// Registered-check seams (R24–R27), nil-safe.
	Listener ListenerAudit
	Organs   OrganLiveness
	Wake     WakeReconcile
	WAL      WALSizeReader

	Logger *slog.Logger
}

// Watchdog is the S14.4 suite. Safe for the single-writer in-process posture
// (the storage pool serializes writes, Spec S02.1).
type Watchdog struct {
	db       *storage.DB
	log      *eventlog.Log
	runs     *run.Store
	settings Settings
	duty     *local.Duty
	meter    AdvisoryMeter
	spend    SpendReader
	listener ListenerAudit
	organs   OrganLiveness
	wake     WakeReconcile
	wal      WALSizeReader
	logger   *slog.Logger
}

// New wires a Watchdog. A nil Deps.Logger defaults to slog.Default().
func New(d Deps) *Watchdog {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Watchdog{
		db: d.DB, log: d.Log, runs: d.Runs, settings: d.Settings,
		duty: d.Duty, meter: d.Meter, spend: d.Spend,
		listener: d.Listener, organs: d.Organs, wake: d.Wake, wal: d.WAL,
		logger: logger,
	}
}

// severityFor routes a rule to its alert class (R21): flag-now for active-run
// stalls/loops/spend, the foreign-listener violation, and the dead-man alarm;
// daily-digest for everything else.
func severityFor(rule string) string {
	switch rule {
	case RuleLoop, RulePingPong, RuleErrorLoop, RuleSilence, RuleSpend, RuleListener, RuleDeadMan:
		return SeverityFlagNow
	default:
		return SeverityDailyDigest
	}
}

// ── shared read helpers (derive-from-log; no side store, Spec S14.1) ──

// recentEvent is one run_events row the Tier-0 counters read.
type recentEvent struct {
	Seq     int64
	Type    string
	Payload json.RawMessage
	TS      time.Time
}

// readRecentEvents returns the run's newest `limit` events in OLDEST-first order
// (the counters read forward). generation >= 0 FENCES to that generation; passing
// allGenerations (< 0) reads the whole run unfenced. Re-derived fresh each
// evaluation — no durable cursor, no state table (R1; Spec S14.1). Short-batch,
// no long-lived read tx (S02.1).
//
// The generation FENCE (drain D1) is load-bearing for the BEHAVIORAL counters
// (loop/ping-pong/error-loop): the resume edge parked→running bumps the run's
// generation (Spec S02.5 step 4, verified in internal/run/run.go generationBumps),
// and pre-park events carry the OLD generation. Without the fence, one tail tick
// after an operator resume re-reads the stale pre-park trace and re-trips —
// re-parking a run the operator just said "resume — I was wrong" to, with zero
// new events (and burning a Tier-1 duty call). Fencing to the CURRENT generation
// drops the pre-park trace; the resume transition event is at the new generation,
// so a legitimate fresh anomaly still flags. The suspicious-completion path,
// though, reads UNFENCED (drain-R1): a run that did real work before a resume and
// then completed quietly is not a silent failure, so its whole-run activity — not
// just the post-resume generation — must be counted.
func (w *Watchdog) readRecentEvents(ctx context.Context, runID string, generation int64, limit int) ([]recentEvent, error) {
	fence, args := "", []any{runID}
	if generation >= 0 {
		fence = " AND generation = ?"
		args = append(args, generation)
	}
	args = append(args, limit)
	rows, err := w.db.QueryContext(ctx,
		`SELECT event_seq, type, payload, ts FROM run_events
		 WHERE run_id = ?`+fence+` ORDER BY event_seq DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []recentEvent
	for rows.Next() {
		var (
			e       recentEvent
			payload string
			tsRaw   string
		)
		if err := rows.Scan(&e.Seq, &e.Type, &payload, &tsRaw); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		e.TS = parseTS(tsRaw)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// reverse to oldest-first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// toolCompleted filters the recent events to canonical tool-completion rows
// (tool.completed and its legacy alias), oldest-first.
func toolCompleted(events []recentEvent) []recentEvent {
	var out []recentEvent
	for _, e := range events {
		if e.Type == "tool.completed" || e.Type == "engine.tool_result" {
			out = append(out, e)
		}
	}
	return out
}

// toolSignature is the stable loop signature of a tool-completion event: a hash
// over the tool name + the args digest (never raw args — the §30 discipline,
// R2). Returns "" when no tool name is present, so a payload that carries no
// signature never trips a loop (the safe direction; honest for a claude-cli
// tool.completed that lacks the name/digest until the adapter conforms — the
// B5-2 F13 carry).
func toolSignature(pay json.RawMessage) string {
	name := firstString(pay, "tool", "name", "tool_name")
	if name == "" {
		return ""
	}
	digest := firstString(pay, "args_digest", "digest")
	if digest == "" {
		if raw := firstString(pay, "args", "tool_input", "input"); raw != "" {
			sum := sha256.Sum256([]byte(raw))
			digest = "sha256:" + hex.EncodeToString(sum[:])[:16]
		}
	}
	sum := sha256.Sum256([]byte(name + "\x00" + digest))
	return hex.EncodeToString(sum[:])[:16]
}

// isToolError reports whether a tool-completion payload represents an error
// (R4). Reads the canonical error indicators; tolerant of the several shapes a
// producer may emit.
func isToolError(pay json.RawMessage) bool {
	if firstBool(pay, "is_error", "error", "failed") {
		return true
	}
	if s := firstString(pay, "status", "outcome", "result"); s == "error" || s == "failed" || s == "failure" {
		return true
	}
	if code, ok := firstNumber(pay, "exit_code", "exit_status", "returncode"); ok && code != 0 {
		return true
	}
	return false
}

// firstString returns the first present non-empty string among keys.
func firstString(payload json.RawMessage, keys ...string) string {
	m := decodeObject(payload)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// firstBool returns the first present bool among keys.
func firstBool(payload json.RawMessage, keys ...string) bool {
	m := decodeObject(payload)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if b, ok := v.(bool); ok {
				return b
			}
		}
	}
	return false
}

// firstNumber returns the first present number among keys and whether one was
// found.
func firstNumber(payload json.RawMessage, keys ...string) (float64, bool) {
	m := decodeObject(payload)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if f, ok := v.(float64); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func decodeObject(payload json.RawMessage) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil
	}
	return m
}

// parseTS parses a stored RFC3339(Nano) timestamp; zero on failure. Stored ts
// is display/time-absence only, never an ordering authority (Spec S02.5).
func parseTS(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
