package watchlist

import (
	"context"
	"fmt"
)

// canary_behavioral.go — the behavioral canary (Spec S14.6 ¶3 bullet 3).
//
// A scheduled fixed-prompt mini-eval per lane on the ⚙ canary.behavioral_interval
// cadence, alerting on a score drop. It is THE ONLY DRIFT DETECTION AVAILABLE TO
// THE SUBSCRIPTION LANES: Z.AI exposes no logprobs endpoint-wide and the wrapped
// Anthropic CLI exposes none either, so neither paid lane can carry a logprob
// canary (S03.7; SPIKE P2-S1 Probe 8) — if a subscription lane's served model
// quietly changes behaviour, this canary is what notices.
//
// It REUSES the S14.8 pinned-runner seam and builds no second runner. The seam
// arrives as a func composed at the shell root rather than an internal/evals
// import, because this package's import wall bars that edge (the evals→verify→
// intake→adapters graph stays out of an observability leaf, CONVENTIONS
// §28/§33/§34) — BehavioralOutcome is a mirror of the runner's outcome shape,
// the same no-import-either-way pattern local.SuiteResult already uses. The
// runner's CLI facts (flag set, exit codes, results schema version) are B5-5's
// and are not re-derived here.
//
// CARRIED DEPENDENCY: the promptfoo binary does not exist on the host until the
// B5-gate operator act, and running a five-task battery on a paid lane is a real
// request. So the seam is nil at v0 on both counts — the canary is skipped
// honestly rather than faked — and the suite is stub-verified only.

// BehavioralOutcome is one mini-eval run's result: the pass-rate half of the
// S14.8 runner outcome, mirrored so neither package imports the other.
type BehavioralOutcome struct {
	// Runner / RunnerVersion identify what actually ran (never the pin assumed
	// from the manifest).
	Runner        string
	RunnerVersion string
	// Suite names the battery that ran.
	Suite string
	Cases int
	// PassRate is the fraction of cases that passed; the tracked scalar.
	PassRate float64
}

// BehavioralRun executes the fixed-prompt mini-eval for one lane. Nil ⇒ the
// canary is absent: either the operator has not armed the real-request legs or
// no runner binary is installed.
type BehavioralRun func(ctx context.Context, lane string) (BehavioralOutcome, error)

// BehavioralCanary is the scheduled per-lane mini-eval.
type BehavioralCanary struct {
	// Run is the S14.8 runner seam; nil ⇒ DISARMED/absent.
	Run BehavioralRun
	// Lanes are the lanes to probe. The subscription lanes are the ones that
	// NEED it; the local lane also carries the logprob canary, which is the
	// cheaper and sharper signal there.
	Lanes []string
}

// NewBehavioralCanary builds the behavioral canary over the paid lanes.
func NewBehavioralCanary(run BehavioralRun) *BehavioralCanary {
	return &BehavioralCanary{Run: run, Lanes: PaidLanes()}
}

func (b *BehavioralCanary) runner(c *Canaries, lane string) func(context.Context) (CanaryResult, error) {
	return func(ctx context.Context) (CanaryResult, error) { return b.RunLane(ctx, c, lane) }
}

// RunLane runs the mini-eval on one lane and compares its pass rate against the
// previous run's, derived from the log.
func (b *BehavioralCanary) RunLane(ctx context.Context, c *Canaries, lane string) (CanaryResult, error) {
	if b.Run == nil {
		return CanaryResult{}, ErrCanaryDisarmed
	}
	out, err := b.Run(ctx, lane)
	if err != nil {
		return CanaryResult{
			Lane:        lane,
			Summary:     fmt.Sprintf("behavioral canary could not run on lane %s", lane),
			Detail:      err.Error(),
			ChangeClass: ClassUnclear,
			Err:         err.Error(),
		}, nil
	}

	rate := out.PassRate
	res := CanaryResult{
		Lane:        lane,
		Measurement: &rate,
		Detail: fmt.Sprintf("suite %s, %d cases, pass rate %.2f, runner %s %s",
			out.Suite, out.Cases, rate, out.Runner, out.RunnerVersion),
	}

	prev, ok, err := c.LastMeasurement(ctx, CanaryBehavioral, lane)
	if err != nil {
		return res, err
	}
	if !ok {
		// First observation establishes the baseline and raises nothing: there
		// was no prior score to have dropped from.
		res.Pass, res.Baseline = true, true
		res.Summary = fmt.Sprintf("behavioral canary baseline established on lane %s at pass rate %.2f", lane, rate)
		return res, nil
	}

	res.Delta = rate - prev
	if prev-rate > behavioralDropTolerance {
		// A served model whose behaviour moved is a models-class change. It
		// carries NO model-id subject on a subscription lane — the lane serves
		// whatever the provider points it at, and guessing an id would flag the
		// wrong assets — so the finding records that no revalidation fired
		// (OQ4(a)), which is the residual gap made visible.
		res.ChangeClass = ClassModels
		res.Summary = fmt.Sprintf("behavioral canary score DROP on lane %s: pass rate %.2f → %.2f (%.2f, tolerance %.2f)",
			lane, prev, rate, res.Delta, behavioralDropTolerance)
		return res, nil
	}
	res.Pass = true
	res.Summary = fmt.Sprintf("behavioral canary on lane %s passed: pass rate %.2f (%+.2f)", lane, rate, res.Delta)
	return res, nil
}
