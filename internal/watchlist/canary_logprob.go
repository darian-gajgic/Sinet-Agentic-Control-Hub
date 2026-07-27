package watchlist

import (
	"context"
	"fmt"
	"math"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
)

// canary_logprob.go — the logprob canary (Spec S14.6 ¶3 bullet 4).
//
// LOCAL LANE ONLY. The reason is recorded in the spec and is CITED here, not
// inferred — Spec S03.7, verbatim:
//
//	"No logprobs on the Z.AI coding endpoint (`logprobs:true` → 200 but no
//	`logprobs` key, endpoint-wide) → the Z.AI lane is behavioral-eval-only for
//	drift detection; only the local lane gets the logprob canary"
//	[SPIKE P2-S1 Probe 8]
//
// and the wrapped Anthropic CLI exposes none either, so BOTH subscription lanes
// are behavioral-eval-only (S14.6 ¶3). A canary that ran on a lane with no
// logprobs would produce nothing and report it as a measurement, so this one
// REFUSES a non-local lane outright — a checkable negative, asserted by test.
//
// It invents no inference path: the probe is the watchlist's OWN triage duty on
// its OWN schema against a FIXED prompt, and the tracked scalar is
// local.LabelMargin — the S12.5 top1−top2 gap at the decisive label token, the
// same scalar S12.9 calibration fits. One-token drift tracking at ~zero cost:
// the call is local-tier, so it costs no allowance, and it rides the same $0 D7
// advisory-meter seam the second pass does rather than being issued unmetered.

// logprobCanaryLabelField is the decisive label whose margin is tracked — the
// enum field of the watchlist triage schema, which is exactly the token that
// commits the classification.
const logprobCanaryLabelField = "change_class"

// logprobCanaryProbe is the FIXED prompt. It is deliberately unambiguous and
// deliberately frozen: the canary measures the DECODE PATH's confidence on an
// unchanging input, so any movement is the model or the build moving, never the
// question moving.
const logprobCanaryProbe = `A watchlist source changed. Classify the change.

source: https://example.invalid/pricing
watch kind: page
watched lane or candidate: local

observed change:
- Input tokens: $3.00 per million
+ Input tokens: $4.50 per million`

// LogprobCanary tracks label-margin drift on the local lane.
type LogprobCanary struct {
	// Duty is the local duty caller; nil (or an unconfigured stack) makes the
	// canary honestly absent.
	Duty *local.Duty
	// Meter opens the $0 platform run the D7 row lands on; nil skips the run
	// rather than issuing an unmetered call.
	Meter AdvisoryMeter
}

// NewLogprobCanary builds the logprob canary.
func NewLogprobCanary(duty *local.Duty, meter AdvisoryMeter) *LogprobCanary {
	return &LogprobCanary{Duty: duty, Meter: meter}
}

func (l *LogprobCanary) runner(c *Canaries, lane string) func(context.Context) (CanaryResult, error) {
	return func(ctx context.Context) (CanaryResult, error) { return l.Run(ctx, c, lane) }
}

// Run measures the label margin on the local lane and compares it against the
// previous run's. It REFUSES any other lane (S03.7).
func (l *LogprobCanary) Run(ctx context.Context, c *Canaries, lane string) (CanaryResult, error) {
	if lane != LaneLocal {
		return CanaryResult{}, fmt.Errorf("%w: %q exposes no logprobs (S03.7)", ErrLaneNotLocal, lane)
	}
	// NOT an arming matter: this canary is local-tier and costs no allowance,
	// so CanaryArmEnv never gates it. An absent stack or meter is a stack
	// absence and is accounted for as one (drain D5).
	if l.Duty == nil {
		return CanaryResult{}, fmt.Errorf("%w: no local tier is configured", ErrCanaryStackAbsent)
	}
	if l.Meter == nil {
		return CanaryResult{}, fmt.Errorf("%w: no advisory metering seam is wired, so the $0 D7 row could not be written and the probe was not issued unmetered", ErrCanaryStackAbsent)
	}
	runID, settle, err := l.Meter(ctx, "watchlist-canary")
	if err != nil {
		return CanaryResult{
			Lane:        lane,
			Summary:     "logprob canary skipped: no advisory metering run could be opened",
			Detail:      err.Error(),
			ChangeClass: ClassUnclear,
			Err:         err.Error(),
		}, nil
	}
	if settle != nil {
		defer settle()
	}

	schema := WatchlistSchema()
	out, err := l.Duty.Call(ctx, runID, local.DutyRequest{
		Alias: local.AliasWatchlist,
		System: "You classify a detected change on an AI provider's page, feed or API. " +
			"Reason briefly FIRST, then answer. Never guess a label.",
		User:           logprobCanaryProbe + "\n\nAnswer as JSON matching this schema exactly:\n" + string(schema),
		Schema:         schema,
		Name:           "watchlist-triage",
		MaxTokens:      classifyMaxTokens,
		Classification: true,
	})
	if err != nil {
		return CanaryResult{
			Lane:        lane,
			Summary:     "logprob canary could not reach the local tier",
			Detail:      err.Error(),
			ChangeClass: ClassUnclear,
			Err:         err.Error(),
		}, nil
	}

	margin, ok := local.LabelMargin(out.Logprobs, []string{logprobCanaryLabelField})
	if !ok {
		// No margin is available — the seat returned no top-2 alternatives at
		// the label token. That is recorded honestly, never as a margin of 0
		// (which would read as a coin-flip that never happened).
		return CanaryResult{
			Lane:        lane,
			Summary:     "logprob canary could not locate a label margin in the completion",
			Detail:      "the local seat returned no top-2 alternatives at the " + logprobCanaryLabelField + " token",
			ChangeClass: ClassUnclear,
			Err:         "no label margin available",
		}, nil
	}

	res := CanaryResult{
		Lane:        lane,
		Measurement: &margin,
		Detail: fmt.Sprintf("label margin %.3f at %s, seat model %s",
			margin, logprobCanaryLabelField, out.Model),
	}
	prev, had, err := c.LastMeasurement(ctx, CanaryLogprob, lane)
	if err != nil {
		return res, err
	}
	if !had {
		res.Pass, res.Baseline = true, true
		res.Summary = fmt.Sprintf("logprob canary baseline established on the local lane at margin %.3f", margin)
		return res, nil
	}

	res.Delta = margin - prev
	if math.Abs(res.Delta) > logprobMarginTolerance {
		// The local lane DOES have a model id — the seat's — so this canary can
		// carry a real model-id subject into the S14.8 revalidation runbook
		// (OQ4(a)), which a subscription lane's behavioral canary cannot.
		res.ChangeClass = ClassModels
		res.Summary = fmt.Sprintf("logprob canary margin DRIFT on the local lane: %.3f → %.3f (%+.3f, tolerance %.2f) on seat model %s",
			prev, margin, res.Delta, logprobMarginTolerance, out.Model)
		if out.Model != "" {
			res.Subjects = []string{out.Model}
		}
		return res, nil
	}
	res.Pass = true
	res.Summary = fmt.Sprintf("logprob canary on the local lane passed: margin %.3f (%+.3f)", margin, res.Delta)
	return res, nil
}
