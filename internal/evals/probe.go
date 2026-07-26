package evals

import (
	"context"
	"fmt"
)

// The S03.3 (b)-limb engine-bump quality probe (Spec S14.8 ¶4): the before/after
// small-task battery a bump is gated on. Schema tests catch schema drift; only
// QUALITY probes catch effort and context-handling drift (P-T02-5), so the
// battery's content is quality-shaped — small real tasks whose expectations are
// about the work being done properly — never schema-shaped.
//
// The S03.3 gate POLICY stays normative at S03.3: a bump lands only after (a)
// the candidate passes its per-lane conformance suite AND (b) this probe shows
// no regression. This package builds limb (b)'s machinery; it builds no bump
// executor, and running it against a live candidate engine is a coordinator act
// (a real run needs a live engine and, on a paid lane, paid calls).

// ProbeSuiteVersion versions the committed probe battery, exactly as the other
// eval objects are versioned. Bumped when the task content changes.
const ProbeSuiteVersion = "bump-probe-v1"

// ProbeSuite is the versioned before/after battery.
type ProbeSuite struct {
	Version string
	Tasks   []Case
}

// SeedProbeSuite returns the v0 battery: small REAL tasks whose expectations
// are quality signals (the work is actually done, the obvious failure mode is
// handled) and whose forbidden strings are the effort-drift tells a schema test
// cannot see — a stub, a placeholder, a deferral, an apology instead of work.
func SeedProbeSuite() ProbeSuite {
	const (
		stub   = "TODO"
		holder = "not implemented"
		defer_ = "I cannot"
	)
	return ProbeSuite{
		Version: ProbeSuiteVersion,
		Tasks: []Case{
			{
				ID: "probe/parse-int-errors",
				Prompt: "Write a Go function `parsePort(s string) (int, error)` that parses a TCP port. " +
					"Reject anything outside 1–65535 and anything non-numeric with a wrapped error naming the bad input. " +
					"Return only the function.",
				Expect: []string{"func parsePort", "65535", "error"},
				Forbid: []string{stub, holder, defer_},
			},
			{
				ID: "probe/sql-injection-fix",
				Prompt: "This Go line builds a query by concatenation: " +
					"`db.Query(\"SELECT * FROM users WHERE name = '\" + name + \"'\")`. " +
					"Rewrite it safely and say in one sentence why the original is unsafe.",
				Expect: []string{"?", "injection"},
				Forbid: []string{stub, holder, defer_},
			},
			{
				ID: "probe/edge-case-test",
				Prompt: "Write a Go table-driven test for a function `sum(a, b int) int`. " +
					"Cover the zero case and integer overflow at math.MaxInt. Return only the test.",
				Expect: []string{"func Test", "math.MaxInt", "[]struct"},
				Forbid: []string{stub, holder, defer_},
			},
			{
				ID: "probe/context-carry",
				Prompt: "Constraint: this codebase uses the standard library only — no third-party modules, ever. " +
					"Write a Go helper that retries a `func() error` up to 3 times with 100ms between attempts, " +
					"returning the last error. Return only the helper.",
				Expect: []string{"time.Sleep", "func", "error"},
				Forbid: []string{stub, holder, defer_, "github.com/"},
			},
			{
				ID: "probe/refuse-scope-creep",
				Prompt: "Here is a function that returns the wrong sign for negative inputs: " +
					"`func abs(n int) int { return n }`. Fix ONLY that bug. Return only the function.",
				Expect: []string{"func abs", "-n"},
				Forbid: []string{stub, holder, defer_, "package main"},
			},
		},
	}
}

// ProbeRun describes one before/after execution of the battery.
type ProbeRun struct {
	Suite ProbeSuite
	// EngineID names the engine under test — the S14.8 asset the result records
	// against (OQ2(a): AssetID = the engine, AssetVersion = the candidate pin).
	EngineID     string
	BaselinePin  string
	CandidatePin string
}

// ProbeSide is one side of a before/after probe: which engine ran, and what the
// runner scored it.
type ProbeSide struct {
	Pin     string
	Outcome RunOutcome
}

// ProbeResult is the (b)-limb verdict.
type ProbeResult struct {
	Baseline   ProbeSide
	Candidate  ProbeSide
	Comparison Comparison
	// Green is limb (b): the candidate shows no regression against the
	// baseline, per task and overall.
	Green bool
}

// RunProbe executes the battery against the baseline engine and the candidate
// engine through the pinned runner, compares per task (a single regressed task
// reds the limb — aggregate-green is insufficient), and records the result
// against the S14.8 sweep row with the ENGINE as the asset and the candidate
// pin as the asset version (OQ2(a): this keeps the per-lane adapter rows purely
// (a)-limb).
func (s *Store) RunProbe(ctx context.Context, r Runner, p ProbeRun) (ProbeResult, error) {
	if r == nil {
		return ProbeResult{}, ErrNoRunner
	}
	name, version, err := r.Identity(ctx)
	if err != nil {
		return ProbeResult{}, err
	}
	before, err := r.Run(ctx, RunConfig{Suite: p.Suite.Version + "/before", Provider: p.BaselinePin, Cases: p.Suite.Tasks})
	if err != nil {
		return ProbeResult{}, fmt.Errorf("evals: probe baseline %q: %w", p.BaselinePin, err)
	}
	after, err := r.Run(ctx, RunConfig{Suite: p.Suite.Version + "/after", Provider: p.CandidatePin, Cases: p.Suite.Tasks})
	if err != nil {
		return ProbeResult{}, fmt.Errorf("evals: probe candidate %q: %w", p.CandidatePin, err)
	}

	res := ProbeResult{
		Baseline:  ProbeSide{Pin: p.BaselinePin, Outcome: before},
		Candidate: ProbeSide{Pin: p.CandidatePin, Outcome: after},
	}
	res.Comparison = Compare(probeSlices(p.Suite, before, after))
	res.Green = res.Comparison.Green

	metrics := res.Comparison.Metrics()
	metrics["baseline_pin"] = p.BaselinePin
	metrics["probe"] = "S03.3 limb (b) engine-bump quality probe (P-T02-5)"
	if err := s.RecordAsset(ctx, AssetResult{
		AssetKind:     AssetEngine,
		AssetID:       p.EngineID,
		AssetVersion:  p.CandidatePin,
		Suite:         p.Suite.Version,
		Runner:        name,
		RunnerVersion: version,
		Metrics:       metrics,
		Passed:        res.Green,
	}); err != nil {
		return ProbeResult{}, err
	}
	return res, nil
}

// probeSlices builds one slice per task plus the overall pass rate, so a single
// regressed task reds the limb even when the average improves.
func probeSlices(suite ProbeSuite, before, after RunOutcome) []Slice {
	beforeByID := make(map[string]CaseOutcome, len(before.Cases))
	for _, c := range before.Cases {
		beforeByID[c.ID] = c
	}
	afterByID := make(map[string]CaseOutcome, len(after.Cases))
	for _, c := range after.Cases {
		afterByID[c.ID] = c
	}
	slices := make([]Slice, 0, len(suite.Tasks)+1)
	for _, t := range suite.Tasks {
		slices = append(slices, Slice{
			Name:     t.ID,
			Baseline: passValue(beforeByID[t.ID]),
			Measured: passValue(afterByID[t.ID]),
			Guard:    true,
		})
	}
	return append(slices, Slice{
		Name:     "probe_pass_rate",
		Baseline: before.PassRate(),
		Measured: after.PassRate(),
	})
}

// passValue maps a case outcome to a comparable number: a missing case scores
// 0, so a candidate that silently dropped a task regresses rather than passing.
func passValue(c CaseOutcome) float64 {
	if c.Pass {
		return 1
	}
	return 0
}
