package evals

import (
	"context"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
)

// The regression sweep (Spec S14.8 ¶3 "on-trigger + ⚙ eval.sweep_interval full
// sweep") and the recording path every S14.8 result takes.
//
// Recording shape (OQ2(a), coordinator-dispositioned): ONE conformance-registry
// row is the S14.8 recording home. Per-asset results record against it with the
// asset carried in the payload's asset id/version, and ONE aggregate
// sweep-level result records LAST — red if any per-asset result was red — so
// the row's resting state IS the sweep verdict and the existing B5-4 red→card
// derivation raises the card. The (b)-limb engine-bump probe records against
// the SAME row with the ENGINE as the asset, which keeps the per-lane adapter
// rows purely (a)-limb.

// monthDays matches the S14.5 registry's coarse structural month (CONVENTIONS
// §32): dueness is a passive read surface, not a precise scheduler, and both
// surfaces must agree on the same ⚙ months value.
const monthDays = 30

// Runner identities recorded on every result (Spec S14.2 family 14 requires a
// runner + runner version; the identity is honest and version-pinned).
const (
	// RunnerPromptfoo is the adopted pinned eval runner for asset-facing golden
	// and planted-defect objects (Spec S14.8 ¶1).
	RunnerPromptfoo = "promptfoo"
	// RunnerBattery is the in-repo Go-native T15 acceptance battery, itself
	// pinned by its repo version and keyed by the engine build (Spec S12.9;
	// OQ6(a): it stays the local-duty executor and RECORDS THROUGH this
	// surface — S12.9's "pinned eval runner" is the S14.8 runner FAMILY, which
	// includes it. Re-plumbing it as promptfoo configs would duplicate a
	// gate-validated instrument).
	RunnerBattery = "internal/local/battery"
	// RunnerRider1 is the committed golden-set harness that re-measures the
	// judge (P-T06-5 instrument, B4-7 rider 1).
	RunnerRider1 = "internal/adapters/claudecli/rider1_golden_test.go"
)

// SweepState is the passive dueness read for the regression sweep. It derives
// from STORED state (the registry row's last run) plus the ⚙ cadence read live,
// so it survives restart and suspend — never an in-memory ticker.
type SweepState struct {
	Due        bool
	HasRun     bool
	LastRun    time.Time
	LastResult string
	// Interval is the resolved ⚙ eval.sweep_interval.
	Interval time.Duration
}

// SweepState reads the sweep row's stored state and resolves dueness against ⚙
// eval.sweep_interval (by dotted key, read at evaluation time).
func (s *Store) SweepState(ctx context.Context) (SweepState, error) {
	rows, err := s.conf.List(ctx)
	if err != nil {
		return SweepState{}, err
	}
	months, err := s.settings.Int(keyEvalSweepInterval)
	if err != nil {
		return SweepState{}, fmt.Errorf("evals: read ⚙ %s: %w", keyEvalSweepInterval, err)
	}
	st := SweepState{Interval: time.Duration(months) * monthDays * 24 * time.Hour}
	for _, r := range rows {
		if r.ID != conformance.RowRegressionSweep {
			continue
		}
		st.HasRun, st.LastRun, st.LastResult = r.HasRun, r.LastRun, r.LastResult
		st.Due = !r.HasRun || s.now().Sub(r.LastRun) >= st.Interval
		return st, nil
	}
	return SweepState{}, fmt.Errorf("%w: %q", conformance.ErrUnknownRow, conformance.RowRegressionSweep)
}

// AssetResult is one per-asset eval outcome recorded against the sweep row.
type AssetResult struct {
	AssetKind    string
	AssetID      string
	AssetVersion string
	// Suite is the versioned eval object that produced the result (an eval
	// set's id@version pair, or a probe/sweep suite version).
	Suite string
	// Runner / RunnerVersion identify what executed it.
	Runner        string
	RunnerVersion string
	Metrics       map[string]any
	Passed        bool
	RanAt         time.Time
}

// RecordAsset records one per-asset result against the sweep row (OQ2(a)),
// through conformance.RecordResult so the event append and the row mirror land
// in ONE WriteTx.
func (s *Store) RecordAsset(ctx context.Context, r AssetResult) error {
	metrics := make(map[string]any, len(r.Metrics)+1)
	for k, v := range r.Metrics {
		metrics[k] = v
	}
	if r.AssetKind != "" {
		metrics["asset_kind"] = r.AssetKind
	}
	return s.conf.RecordResult(ctx, conformance.Result{
		RowID:         conformance.RowRegressionSweep,
		SuiteVersion:  r.Suite,
		AssetID:       r.AssetID,
		AssetVersion:  r.AssetVersion,
		Runner:        r.Runner,
		RunnerVersion: r.RunnerVersion,
		Metrics:       metrics,
		Passed:        r.Passed,
		RanAt:         r.RanAt,
	})
}

// SweepResult is the aggregate sweep-level outcome, recorded LAST so the row's
// resting state is the sweep verdict.
type SweepResult struct {
	// Assets is how many per-asset results the sweep recorded.
	Assets int
	// RedAssets names the assets that came back red (empty ⇒ the sweep is
	// green).
	RedAssets []string
	// Runner / RunnerVersion identify the sweep driver.
	Runner        string
	RunnerVersion string
	RanAt         time.Time
}

// SweepSuiteVersion versions the sweep procedure itself (the set of obligations
// a full sweep covers), bumped when the sweep's composition changes.
const SweepSuiteVersion = "s14.8-sweep-v1"

// RecordSweep records the aggregate sweep verdict against the sweep row: red
// when ANY per-asset result was red (Spec S14.8 ¶3; the red→card derivation is
// the existing B5-4 conformance card, owner-scoped, never a bare asks row).
func (s *Store) RecordSweep(ctx context.Context, r SweepResult) error {
	metrics := map[string]any{
		"assets_evaluated": r.Assets,
		"assets_red":       len(r.RedAssets),
	}
	if len(r.RedAssets) > 0 {
		metrics["red_assets"] = r.RedAssets
	}
	months, err := s.settings.Int(keyEvalSweepInterval)
	if err != nil {
		return fmt.Errorf("evals: read ⚙ %s: %w", keyEvalSweepInterval, err)
	}
	metrics["sweep_interval_months"] = months
	return s.conf.RecordResult(ctx, conformance.Result{
		RowID:         conformance.RowRegressionSweep,
		SuiteVersion:  SweepSuiteVersion,
		AssetID:       "sinet-platform",
		AssetVersion:  SweepSuiteVersion,
		Runner:        r.Runner,
		RunnerVersion: r.RunnerVersion,
		Metrics:       metrics,
		Passed:        len(r.RedAssets) == 0,
		RanAt:         r.RanAt,
	})
}

// LocalSuiteResult mirrors local.SuiteResult across the import wall
// (internal/evals never imports internal/local). It is the OQ6(a)
// record-through input: the T15 battery stays the local-duty executor and its
// SuiteResults land here as eval.score_recorded, which is the connective
// obligation S12.9 carries.
type LocalSuiteResult struct {
	Duty           string
	ModelHash      string
	EngineBuild    string
	BatteryVersion string
	Total          int
	Passed         int
	TokensPerSec   float64
	WilsonLo       float64
	WilsonHi       float64
	// Green is the suite's verdict against the duty's acceptance bar. S12.9
	// owns that bar — this package records the verdict, it never invents one.
	Green bool
}

// RecordLocalSuite records one T15 battery suite result against the sweep row.
// The asset is the (duty, model hash) seat the suite measured and the asset
// version is the engine build — exactly the (duty, model hash, engine build)
// key the S12.10 swap gate keeps valid.
func (s *Store) RecordLocalSuite(ctx context.Context, r LocalSuiteResult) error {
	if r.Total <= 0 {
		return fmt.Errorf("evals: local suite result for duty %q has no cases", r.Duty)
	}
	rate := float64(r.Passed) / float64(r.Total)
	return s.conf.RecordResult(ctx, conformance.Result{
		RowID:         conformance.RowRegressionSweep,
		SuiteVersion:  r.BatteryVersion,
		AssetID:       r.Duty + "@" + r.ModelHash,
		AssetVersion:  r.EngineBuild,
		Runner:        RunnerBattery,
		RunnerVersion: r.BatteryVersion,
		Metrics: map[string]any{
			"asset_kind":   "local_seat",
			"pass_rate":    rate,
			"passed":       r.Passed,
			"total":        r.Total,
			"tokens_per_s": r.TokensPerSec,
			"wilson_lo":    r.WilsonLo,
			"wilson_hi":    r.WilsonHi,
		},
		Passed: r.Green,
		RanAt:  s.now(),
	})
}
