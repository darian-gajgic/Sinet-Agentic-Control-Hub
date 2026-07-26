package evals

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// The revalidation runbook (Spec S14.8 ¶3): flag dependent workers/judges/
// rubrics (7.3) → run the affected golden sets and planted-defect suites →
// compare TPR/TNR and task metrics against the PINNED baseline, per slice →
// green: release with a dated revalidation stamp; red: the asset stays flagged
// and its red raises the owner's card.
//
// The three trigger classes are exactly the spec's, and each drives the surface
// that already exists: an engine bump flags through S08.10(b)'s FlagByEnginePin
// (S03.3 step 5 — Sinet's deliberate-bump procedure is the announcement clock,
// P-T14-1); a model/duty-alias swap CONSUMES the flag set the S12.10 swap gate
// already produced before the alias went live (P-T15-1 — the runbook invents no
// second gate); a watchlist drift finding is accept-shape only, since its
// emitter is B5-6's and this package fakes no emitter.

// TriggerKind is one of the three S14.8 ¶3 revalidation triggers.
type TriggerKind string

const (
	// TriggerDrift is a watchlist drift finding (emitter: B5-6).
	TriggerDrift TriggerKind = "drift_finding"
	// TriggerEngineBump is an engine/CLI pin bump (S03.3 step 5, S08.10(b)).
	TriggerEngineBump TriggerKind = "engine_bump"
	// TriggerSeatSwap is a model / duty-alias swap (S12.10).
	TriggerSeatSwap TriggerKind = "model_swap"
)

// Runbook errors.
var (
	// ErrUnknownTrigger: the trigger is outside the spec's three classes.
	ErrUnknownTrigger = errors.New("evals: unknown revalidation trigger")
	// ErrNoFlagHook: the flag surface for this trigger class is not wired.
	ErrNoFlagHook = errors.New("evals: flag surface not wired at the composition root")
	// ErrUnmeasuredBaseline: the asset carries no measured baseline, so there
	// is nothing to compare against (an unmeasured judge says so honestly).
	ErrUnmeasuredBaseline = errors.New("evals: asset has no measured pinned baseline")
)

// Trigger is one revalidation trigger.
type Trigger struct {
	Kind TriggerKind
	// Subject is what the trigger named: the engine pin (bump), the model id
	// (swap / model drift), or the drifted subject on a watchlist finding.
	Subject string
	// Reason is the one-line cause recorded on every flag.
	Reason string
	// Flagged is the flag set a SWAP trigger carries — the S12.10 swap gate
	// already flagged these before the alias went live, and the runbook
	// consumes that set rather than re-flagging.
	Flagged []string
}

// Hooks are the func seams the runbook drives at the composition root
// (internal/evals never imports internal/worker or internal/local).
type Hooks struct {
	// FlagByEnginePin is worker.Store.FlagByEnginePin (S08.10(b)).
	FlagByEnginePin func(ctx context.Context, pin, reason string) ([]string, error)
	// FlagByModel is worker.Store.FlagByModel (S08.10(a)).
	FlagByModel func(ctx context.Context, model, reason string) ([]string, error)
	// Stamp is the green-stamp verb (worker.Store.Revalidate): a flagged
	// template version returns to active ONLY through a dated revalidation
	// record, and a red stamp leaves it flagged.
	Stamp func(ctx context.Context, st Stamp) error
}

// Runbook drives the S14.8 ¶3 pipeline over the S14.8 store.
type Runbook struct {
	Store   *Store
	Catalog *Catalog
	Hooks   Hooks
}

// Flag runs step 1: it flags the dependent assets through the existing S08.10
// surfaces and returns the flagged template ids.
func (rb *Runbook) Flag(ctx context.Context, t Trigger) ([]string, error) {
	reason := t.Reason
	if reason == "" {
		reason = fmt.Sprintf("S14.8 revalidation trigger %s: %s", t.Kind, t.Subject)
	}
	switch t.Kind {
	case TriggerEngineBump:
		if rb.Hooks.FlagByEnginePin == nil {
			return nil, fmt.Errorf("%w: FlagByEnginePin", ErrNoFlagHook)
		}
		return rb.Hooks.FlagByEnginePin(ctx, t.Subject, reason)
	case TriggerSeatSwap:
		// The swap gate already fired the 7.3 trigger (S12.10 / P-T15-1). The
		// runbook consumes that set; re-flagging here would be a second gate.
		return append([]string(nil), t.Flagged...), nil
	case TriggerDrift:
		if rb.Hooks.FlagByModel == nil {
			return nil, fmt.Errorf("%w: FlagByModel", ErrNoFlagHook)
		}
		return rb.Hooks.FlagByModel(ctx, t.Subject, reason)
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownTrigger, t.Kind)
}

// Baseline is an asset version's PINNED baseline — its last measured rates.
type Baseline struct {
	TPR, TNR float64
	Measured bool
	// MeasuredOn dates the measurement the baseline pins.
	MeasuredOn string
	// TaskMetrics are the pinned task-metric baselines (higher is better).
	TaskMetrics map[string]float64
}

// BaselineFromRubric reads a rubric version's pinned baseline off the bundle's
// own recorded golden-set rates (Spec S07.11) — the same numbers the verdict
// record carries, so the runbook and the verdicts can never disagree.
func BaselineFromRubric(r *verify.RubricBundle) (Baseline, error) {
	g := r.GoldenSet
	if !g.Measured || g.TPR == nil || g.TNR == nil {
		return Baseline{}, fmt.Errorf("%w: %s v%d", ErrUnmeasuredBaseline, r.ID, r.Version)
	}
	return Baseline{TPR: *g.TPR, TNR: *g.TNR, Measured: true, MeasuredOn: g.MeasuredOn}, nil
}

// Measured is one asset version's re-measured outcome.
type Measured struct {
	// PlantedCatch is the planted-defect suite's catch rate — the slice guard.
	PlantedCatch float64
	// CleanPass is the clean-control pass rate (the judge's true-negative
	// rate), carried report-only.
	CleanPass float64
	// TaskMetrics are the re-measured task metrics (higher is better).
	TaskMetrics map[string]float64
	// Runner / RunnerVersion identify the instrument that produced this.
	Runner        string
	RunnerVersion string
	RanAt         time.Time
}

// Stamp is the dated revalidation record (Spec S14.8 ¶3; S08.10(a)).
type Stamp struct {
	TemplateID     string
	VersionID      string
	Actor          string
	TriggerKind    string
	TriggerSubject string
	// Suites names the eval objects that ran.
	Suites string
	// Baseline names the pinned baseline the comparison ran against.
	Baseline string
	Green    bool
}

// Revalidation is one asset version's pass through the runbook.
type Revalidation struct {
	Trigger  Trigger
	Object   EvalObject
	Baseline Baseline
	Measured Measured
	// TemplateID / VersionID / Actor address the worker-lifecycle stamp. Empty
	// VersionID ⇒ the asset has no worker-lifecycle row (a rubric): its verdict
	// rides the recorded result and the floor check, with no stamp.
	TemplateID string
	VersionID  string
	Actor      string
}

// Outcome is one revalidation pass's verdict.
type Outcome struct {
	Green      bool
	Comparison Comparison
	FloorCheck FloorCheck
	// Stamped reports whether a dated revalidation stamp was written.
	Stamped bool
	Reason  string
}

// Revalidate runs the pipeline for one asset version: compare per slice against
// the pinned baseline and the registered floor, record the result, then stamp.
// Green releases the version through the dated stamp; red leaves it flagged and
// records the red, which raises its card through the B5-4 derivation.
func (rb *Runbook) Revalidate(ctx context.Context, rv Revalidation) (Outcome, error) {
	if !rv.Baseline.Measured {
		return Outcome{}, fmt.Errorf("%w: %s v%d", ErrUnmeasuredBaseline, rv.Object.AssetID, rv.Object.AssetVersion)
	}
	chk, err := rb.Store.CheckFloor(ctx, rv.Object.AssetID, rv.Object.AssetVersion, rv.Measured.PlantedCatch)
	if err != nil {
		return Outcome{}, err
	}
	cmp := Compare(rb.slices(rv, chk))
	out := Outcome{
		Green:      cmp.Green && chk.Green,
		Comparison: cmp,
		FloorCheck: chk,
	}
	out.Reason = cmp.Reason
	if !chk.Green {
		out.Reason = chk.Reason + "; " + cmp.Reason
	}

	metrics := cmp.Metrics()
	metrics["floor_registered"] = chk.Registered
	metrics["floor"] = chk.Floor
	metrics["floor_green"] = chk.Green
	metrics["trigger"] = string(rv.Trigger.Kind)
	if err := rb.Store.RecordAsset(ctx, AssetResult{
		AssetKind:     rv.Object.AssetKind,
		AssetID:       rv.Object.AssetID,
		AssetVersion:  "v" + strconv.Itoa(rv.Object.AssetVersion),
		Suite:         rv.Object.PlantedSuite.String(),
		Runner:        rv.Measured.Runner,
		RunnerVersion: rv.Measured.RunnerVersion,
		Metrics:       metrics,
		Passed:        out.Green,
		RanAt:         rv.Measured.RanAt,
	}); err != nil {
		return Outcome{}, err
	}

	if rv.VersionID == "" {
		return out, nil
	}
	if rb.Hooks.Stamp == nil {
		return Outcome{}, ErrNoStampHook
	}
	if err := rb.Hooks.Stamp(ctx, Stamp{
		TemplateID: rv.TemplateID, VersionID: rv.VersionID, Actor: rv.Actor,
		TriggerKind: string(rv.Trigger.Kind), TriggerSubject: rv.Trigger.Subject,
		Suites: rv.Object.GoldenSet.String() + " + " + rv.Object.PlantedSuite.String(),
		Baseline: fmt.Sprintf("TPR %.3f / TNR %.3f measured %s", rv.Baseline.TPR, rv.Baseline.TNR,
			rv.Baseline.MeasuredOn),
		Green: out.Green,
	}); err != nil {
		return Outcome{}, err
	}
	out.Stamped = true
	return out, nil
}

// slices builds the comparison slices for one revalidation: the planted-defect
// guard (with its registered floor), the report-only clean-control rate, and
// every pinned task metric.
func (rb *Runbook) slices(rv Revalidation, chk FloorCheck) []Slice {
	out := []Slice{{
		Name:     "planted_defect_catch",
		Baseline: rv.Baseline.TPR,
		Measured: rv.Measured.PlantedCatch,
		Floor:    chk.Floor,
		HasFloor: chk.Registered,
		Guard:    true,
	}, {
		Name:       "clean_control_pass",
		Baseline:   rv.Baseline.TNR,
		Measured:   rv.Measured.CleanPass,
		ReportOnly: true,
	}}
	for name, base := range rv.Baseline.TaskMetrics {
		out = append(out, Slice{Name: name, Baseline: base, Measured: rv.Measured.TaskMetrics[name]})
	}
	return out
}

// JudgeRemeasurement builds the recorded result for a P-T06-5 golden-set
// re-measurement of a rubric version's judge. The instrument is the committed
// rider-1 harness (B4-7), re-run per judge change; a passing re-measurement is
// followed by the rubric version bump that clears the S07 judge gate.
func JudgeRemeasurement(obj EvalObject, judgeSeat string, m Measured, green bool) AssetResult {
	return AssetResult{
		AssetKind:     AssetRubric,
		AssetID:       obj.AssetID,
		AssetVersion:  "v" + strconv.Itoa(obj.AssetVersion),
		Suite:         obj.GoldenSet.String(),
		Runner:        RunnerRider1,
		RunnerVersion: verify.SeedGoldenSet().ID + "@v" + strconv.Itoa(verify.SeedGoldenSet().Version),
		Metrics: map[string]any{
			"judge_seat":           judgeSeat,
			"planted_defect_catch": m.PlantedCatch,
			"clean_control_pass":   m.CleanPass,
			"probe":                "P-T06-5 judge-change golden-set re-measurement",
		},
		Passed: green,
		RanAt:  m.RanAt,
	}
}
