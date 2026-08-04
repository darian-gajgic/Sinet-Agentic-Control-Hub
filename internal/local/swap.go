package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// swap.go — the Spec S12.10 model-swap gate (brief R10–R15). A duty alias is
// the swap unit; this gate is the SOLE write surface for ⚙ local.alias (S18
// row: "changes only via the S12.10 swap gate"; asserted by test). Two rules
// bind every swap:
//
//   - The PROMOTION rule (R10, verbatim S12.10): a challenger replaces an
//     incumbent only if it wins ≥2 duty suites with non-overlapping 95% Wilson
//     intervals AT EQUAL-OR-BETTER tokens/s. This is the ONLY path by which the
//     bakeoff (S12.9 #1) may flip the workhorse default.
//   - The RECALIBRATE-BEFORE-LIVE rule (R11, P-T15-1): a swap silently
//     invalidates per-duty confidence thresholds + calibration maps, so an
//     alias retarget is REFUSED unless (a) fresh S12.5 calibration exists for
//     the (duty, NEW model hash, engine build) key AND (b) the battery's
//     affected duty suite re-ran on the new target — BEFORE the target goes
//     live behind its alias. "No alias retarget goes live uncalibrated."
//
// Every swap additionally fires the 7.3 revalidation trigger (R12, S12.10 ×
// S08.10a) through a func seam satisfied by worker.Store.FlagByModel at the
// composition root — internal/local NEVER imports worker (CONVENTIONS §26); the
// memory.Committer composition-root precedent. At v0 the production worker set
// is empty, so the firing flags nothing yet (proven by test, documented honest).

// Swap-gate errors callers branch on.
var (
	// ErrUncalibratedTarget refuses an alias retarget whose NEW (duty, model
	// hash, engine build) key has no fresh S12.5 calibration (P-T15-1, R11).
	ErrUncalibratedTarget = errors.New("local: alias retarget refused — no fresh S12.5 calibration for the new (duty, model hash, engine build) key (P-T15-1)")
	// ErrBatterySuiteNotRun refuses a retarget whose affected duty suite has
	// not re-run on the new target (P-T15-1, R11).
	ErrBatterySuiteNotRun = errors.New("local: alias retarget refused — the affected T15 battery suite has not re-run on the new target (P-T15-1)")
	// ErrTargetNotServable refuses a retarget to a non-servable / unknown seat.
	ErrTargetNotServable = errors.New("local: alias retarget refused — target seat is unknown or not servable")
	// ErrPromotionRule refuses a bakeoff flip that does not clear the S12.10
	// promotion rule (R10).
	ErrPromotionRule = errors.New("local: swap refused — challenger does not clear the S12.10 promotion rule (≥2 duty suites won, non-overlapping 95% Wilson, equal-or-better tokens/s)")
)

// SuiteResult is one duty suite's T15 battery outcome on a concrete target
// (duty, model hash, engine build): the pass count, the throughput, and the
// 95% Wilson interval on the pass rate. Produced by the battery runner
// (internal/local/battery), consumed by the promotion rule + persisted by the
// swap gate's store.
type SuiteResult struct {
	Duty           string  `json:"duty"`
	ModelHash      string  `json:"model_hash"`
	EngineBuild    string  `json:"engine_build"`
	BatteryVersion string  `json:"battery_version"`
	Total          int     `json:"total"`
	Passed         int     `json:"passed"`
	TokensPerSec   float64 `json:"tokens_per_s"`
	WilsonLo       float64 `json:"wilson_lo"`
	WilsonHi       float64 `json:"wilson_hi"`
}

// PassRate is the observed suite pass rate.
func (s SuiteResult) PassRate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Passed) / float64(s.Total)
}

// WilsonInterval returns the 95% Wilson score interval for passed/total (z =
// 1.96). Stdlib math; no dependency. Total 0 ⇒ [0,1] (no information).
func WilsonInterval(passed, total int) (lo, hi float64) {
	if total == 0 {
		return 0, 1
	}
	const z = 1.959963984540054 // 97.5th percentile of the standard normal
	n := float64(total)
	p := float64(passed) / n
	z2 := z * z
	denom := 1 + z2/n
	center := (p + z2/(2*n)) / denom
	half := (z / denom) * math.Sqrt(p*(1-p)/n+z2/(4*n*n))
	lo = center - half
	hi = center + half
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	return lo, hi
}

// NewSuiteResult builds a SuiteResult with its Wilson interval filled.
func NewSuiteResult(duty, modelHash, engineBuild, batteryVersion string, passed, total int, tokPerSec float64) SuiteResult {
	lo, hi := WilsonInterval(passed, total)
	return SuiteResult{
		Duty: duty, ModelHash: modelHash, EngineBuild: engineBuild, BatteryVersion: batteryVersion,
		Total: total, Passed: passed, TokensPerSec: tokPerSec, WilsonLo: lo, WilsonHi: hi,
	}
}

// Promotion is the S12.10 promotion-rule decision (R10).
type Promotion struct {
	Promote   bool     `json:"promote"`
	Wins      int      `json:"wins"`
	WonDuties []string `json:"won_duties"`
	Reason    string   `json:"reason"`
}

// DecidePromotion applies the S12.10 promotion rule (R10): the challenger
// replaces the incumbent iff it wins ≥2 duty suites with NON-OVERLAPPING 95%
// Wilson intervals (challenger's lower bound above the incumbent's upper bound)
// AT EQUAL-OR-BETTER tokens/s. Suites are matched by duty; a duty present on
// only one side is not a contest. Deterministic.
func DecidePromotion(incumbent, challenger []SuiteResult) Promotion {
	byDuty := map[string]SuiteResult{}
	for _, s := range incumbent {
		byDuty[s.Duty] = s
	}
	var won []string
	for _, ch := range challenger {
		inc, ok := byDuty[ch.Duty]
		if !ok {
			continue
		}
		// Non-overlapping, challenger higher: its Wilson lower bound clears the
		// incumbent's Wilson upper bound. Equal-or-better throughput.
		if ch.WilsonLo > inc.WilsonHi && ch.TokensPerSec >= inc.TokensPerSec {
			won = append(won, ch.Duty)
		}
	}
	sort.Strings(won)
	p := Promotion{Wins: len(won), WonDuties: won}
	p.Promote = len(won) >= 2
	if p.Promote {
		p.Reason = fmt.Sprintf("challenger wins %d duty suites (%v) with non-overlapping 95%% Wilson at equal-or-better tokens/s (S12.10)", len(won), won)
	} else {
		p.Reason = fmt.Sprintf("challenger wins %d duty suite(s) (%v); the S12.10 rule needs ≥2 (non-overlapping Wilson, equal-or-better tokens/s)", len(won), won)
	}
	return p
}

// CalibrationStore is the swap gate's view of the calibration + battery record
// store (calstore.go): the P-T15-1 existence checks. *CalStore satisfies it.
type CalibrationStore interface {
	HasCalibration(ctx context.Context, key CalibrationKey) (bool, error)
	HasBatterySuite(ctx context.Context, duty, modelHash, engineBuild string) (bool, error)
}

// AliasWriter is the swap gate's narrow write seam onto ⚙ local.alias — the ONE
// place the alias map is written (R11). Satisfied at the composition root by an
// adapter over *settings.Registry.Set (one tx: override row + settings_events +
// settings.changed, S01.10/S18.1); internal/local never imports settings, so
// the wall stays clean (the memory.Committer / AliasWriter precedent).
type AliasWriter interface {
	SetAliasMap(ctx context.Context, m map[string]string, actorID, reason string) error
}

// FlagByModelFunc is the 7.3 revalidation-trigger seam (R12): flag every worker
// template version whose validation record or model pin references the swapped
// model, so it is revalidated before further unsupervised use (S08.10a). It
// returns the flagged version ids. Satisfied at the root by
// worker.Store.FlagByModel; nil ⇒ no worker store wired (dev), the swap records
// that it flagged nothing.
type FlagByModelFunc func(ctx context.Context, model, reason string) ([]string, error)

// Provisional swap event name (pending the S14 event contract, B5).
const EventLocalAliasRetargeted = "local.alias_retargeted"

// SwapGate is the S12.10 gate — the sole write surface for ⚙ local.alias
// (R11). It reads the current alias map through Settings, enforces P-T15-1
// against the CalibrationStore, writes the new map through the AliasWriter, and
// fires the 7.3 trigger through FlagByModel.
type SwapGate struct {
	settings    Settings
	store       CalibrationStore
	writer      AliasWriter
	flag        FlagByModelFunc
	log         *eventlog.Log
	engineBuild string
}

// SwapDeps wires a live swap gate (the shell builds it when the stack is
// configured). EngineBuild defaults to LlamaCppPin.
type SwapDeps struct {
	Settings    Settings
	Store       CalibrationStore
	Writer      AliasWriter
	Flag        FlagByModelFunc
	Events      *eventlog.Log
	EngineBuild string
}

// NewSwapGate wires the swap gate (R11).
func NewSwapGate(d SwapDeps) *SwapGate {
	eb := d.EngineBuild
	if eb == "" {
		eb = LlamaCppPin
	}
	return &SwapGate{settings: d.Settings, store: d.Store, writer: d.Writer, flag: d.Flag, log: d.Events, engineBuild: eb}
}

// RetargetResult reports a completed alias retarget.
type RetargetResult struct {
	Duty            string   `json:"duty"`
	OldSeatKey      string   `json:"old_seat_key"`
	NewSeatKey      string   `json:"new_seat_key"`
	NewModel        string   `json:"new_model"`
	FlaggedVersions []string `json:"flagged_versions"`
}

// Retarget points a duty alias at a NEW seat, enforcing the S12.10 gate (R11).
// It refuses unless the new (duty, model hash, engine build) key has fresh
// calibration AND its affected battery suite re-ran on the new target — BEFORE
// the alias goes live. The ⚙ write flows through the AliasWriter (the sole
// alias-write surface); every swap fires the 7.3 trigger.
func (g *SwapGate) Retarget(ctx context.Context, duty, newSeatKey, actorID, reason string) (RetargetResult, error) {
	if g == nil || g.store == nil || g.writer == nil {
		return RetargetResult{}, ErrStackAbsent
	}
	dutyKey := normalizeAlias(duty)
	seat, ok := SeatByKey(newSeatKey)
	if !ok || !seat.Servable {
		return RetargetResult{}, fmt.Errorf("%w: %q", ErrTargetNotServable, newSeatKey)
	}
	newHash := seat.ModelHash()
	if newHash == "" {
		return RetargetResult{}, fmt.Errorf("%w: %q has no recorded model hash (weights not pulled/hashed)", ErrTargetNotServable, newSeatKey)
	}
	key := CalibrationKey{Duty: dutyKey, ModelHash: newHash, EngineBuild: g.engineBuild}

	// P-T15-1 hard block (R11): calibration AND the battery suite for the NEW
	// target must both exist before the alias goes live.
	hasCal, err := g.store.HasCalibration(ctx, key)
	if err != nil {
		return RetargetResult{}, fmt.Errorf("local: swap gate calibration check: %w", err)
	}
	if !hasCal {
		return RetargetResult{}, fmt.Errorf("%w: %s", ErrUncalibratedTarget, key)
	}
	hasSuite, err := g.store.HasBatterySuite(ctx, dutyKey, newHash, g.engineBuild)
	if err != nil {
		return RetargetResult{}, fmt.Errorf("local: swap gate battery check: %w", err)
	}
	if !hasSuite {
		return RetargetResult{}, fmt.Errorf("%w: %s", ErrBatterySuiteNotRun, key)
	}

	// Read the current alias map, record the old target, write the new one
	// through the sole write surface (R11).
	cur, err := g.settings.StringMap(keyAlias)
	if err != nil {
		return RetargetResult{}, fmt.Errorf("local: read ⚙ %s: %w", keyAlias, err)
	}
	oldSeatKey := cur[dutyKey]
	next := make(map[string]string, len(cur))
	for k, v := range cur {
		next[k] = v
	}
	next[dutyKey] = newSeatKey
	if err := g.writer.SetAliasMap(ctx, next, actorID, reason); err != nil {
		return RetargetResult{}, fmt.Errorf("local: swap gate ⚙ %s write: %w", keyAlias, err)
	}

	// Fire the 7.3 revalidation trigger (R12, F4): flag every worker version
	// whose validation record or model pin references the SWAPPED ALIAS or the
	// OLD target's model (S12.10 × S08.10a) — NOT the new target, which nothing
	// was validated against yet. The match templates are tuned against what
	// workers ran on BEFORE the swap: the duty alias and the old model. Fired
	// for both, deduped. nil seam ⇒ no worker store (dev) — flags nothing,
	// recorded honestly.
	var flagged []string
	if g.flag != nil {
		reasonMsg := fmt.Sprintf("S12.10 swap: duty %q retargeted %q→%q (7.3 revalidation — was validated against the old target)", dutyKey, oldSeatKey, newSeatKey)
		targets := []string{dutyKey} // the swapped alias
		if oldSeat, ok := SeatByKey(oldSeatKey); ok && oldSeat.Model != "" {
			targets = append(targets, oldSeat.Model) // the OLD target's model
		}
		seen := map[string]bool{}
		for _, tgt := range targets {
			hits, ferr := g.flag(ctx, tgt, reasonMsg)
			if ferr != nil {
				return RetargetResult{}, fmt.Errorf("local: swap gate 7.3 flag firing (%q): %w", tgt, ferr)
			}
			for _, v := range hits {
				if !seen[v] {
					seen[v] = true
					flagged = append(flagged, v)
				}
			}
		}
	}

	res := RetargetResult{Duty: dutyKey, OldSeatKey: oldSeatKey, NewSeatKey: newSeatKey, NewModel: seat.Model, FlaggedVersions: flagged}
	g.record(ctx, res)
	return res, nil
}

func (g *SwapGate) record(ctx context.Context, res RetargetResult) {
	if g.log == nil {
		return
	}
	payload, err := json.Marshal(res)
	if err != nil {
		return
	}
	_, _ = g.log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: EventLocalAliasRetargeted, SchemaVersion: dutyEventSchemaVersion, Payload: payload,
	})
}
