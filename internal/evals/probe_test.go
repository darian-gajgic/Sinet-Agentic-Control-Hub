package evals_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/evals"
)

// fakeRunner scores the probe battery from a script: one outcome set per
// invocation, in order (before, then after). It stands in for a live candidate
// engine, which a REAL before/after run needs and this hermetic battery does
// not have.
type fakeRunner struct {
	name, version string
	scripted      [][]evals.CaseOutcome
	calls         []evals.RunConfig
}

func (r *fakeRunner) Identity(context.Context) (string, string, error) {
	return r.name, r.version, nil
}

func (r *fakeRunner) Run(_ context.Context, cfg evals.RunConfig) (evals.RunOutcome, error) {
	out := evals.RunOutcome{Suite: cfg.Suite, Provider: cfg.Provider}
	out.Cases = r.scripted[len(r.calls)]
	for _, c := range out.Cases {
		if c.Pass {
			out.Successes++
		} else {
			out.Failures++
		}
	}
	r.calls = append(r.calls, cfg)
	return out, nil
}

func allPass(suite evals.ProbeSuite) []evals.CaseOutcome {
	out := make([]evals.CaseOutcome, 0, len(suite.Tasks))
	for _, tk := range suite.Tasks {
		out = append(out, evals.CaseOutcome{ID: tk.ID, Pass: true, Score: 1})
	}
	return out
}

// ── the S03.3 (b)-limb quality probe (rubric 15; Spec S14.8 ¶4) ──

func TestBumpProbeRedsOnRegressedTask(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	suite := evals.SeedProbeSuite()
	if len(suite.Tasks) < 5 || suite.Version != evals.ProbeSuiteVersion {
		t.Fatalf("the probe battery must be a versioned small-task set: %d tasks, version %q",
			len(suite.Tasks), suite.Version)
	}
	// The content is QUALITY-shaped, not schema-shaped: every task forbids the
	// effort-drift tells only a quality probe can see (P-T02-5).
	for _, tk := range suite.Tasks {
		if len(tk.Expect) == 0 || len(tk.Forbid) == 0 {
			t.Errorf("probe task %q has no graded expectations", tk.ID)
		}
	}

	after := allPass(suite)
	after[2] = evals.CaseOutcome{ID: suite.Tasks[2].ID, Pass: false, Score: 0, Reason: "stubbed the test out"}
	r := &fakeRunner{name: "promptfoo", version: evals.PromptfooPin,
		scripted: [][]evals.CaseOutcome{allPass(suite), after}}

	res, err := f.store.RunProbe(ctx, r, evals.ProbeRun{
		Suite: suite, EngineID: "claude-cli", BaselinePin: "2.1.218", CandidatePin: "2.1.219",
	})
	if err != nil {
		t.Fatalf("RunProbe: %v", err)
	}
	if res.Green {
		t.Fatal("one regressed task must red limb (b) even though four of five improved or held")
	}
	if !strings.Contains(res.Comparison.Reason, suite.Tasks[2].ID) {
		t.Errorf("the reason must name the regressed task: %q", res.Comparison.Reason)
	}
	// Before AND after ran, against the two pins.
	if len(r.calls) != 2 || r.calls[0].Provider != "2.1.218" || r.calls[1].Provider != "2.1.219" {
		t.Fatalf("before/after did not run against both pins: %+v", r.calls)
	}

	// It records against the S14.8 sweep row with the ENGINE as the asset and
	// the CANDIDATE PIN as the asset version, so the per-lane adapter rows stay
	// purely (a)-limb (OQ2(a)).
	var payload string
	if err := f.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		conformance.EventScoreRecorded).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatal(err)
	}
	if got["suite_id"] != conformance.RowRegressionSweep {
		t.Errorf("the probe must record against the S14.8 row, not a per-lane row: %v", got["suite_id"])
	}
	if got["asset_id"] != "claude-cli" || got["asset_version"] != "2.1.219" {
		t.Errorf("asset = %v v%v, want the engine at the candidate pin", got["asset_id"], got["asset_version"])
	}
	if got["result"] != conformance.ResultRed {
		t.Errorf("result = %v, want red", got["result"])
	}

	// A clean candidate is green: the verdict tracks the measurement, not the
	// machinery.
	clean := &fakeRunner{name: "promptfoo", version: evals.PromptfooPin,
		scripted: [][]evals.CaseOutcome{allPass(suite), allPass(suite)}}
	res, err = f.store.RunProbe(ctx, clean, evals.ProbeRun{
		Suite: suite, EngineID: "claude-cli", BaselinePin: "2.1.218", CandidatePin: "2.1.219",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Green {
		t.Fatalf("no regression must be green: %s", res.Comparison.Reason)
	}
}

func TestProbeWithoutARunnerIsRefused(t *testing.T) {
	f := newFix(t)
	// No runner configured is an honest refusal, never a fabricated green.
	if _, err := f.store.RunProbe(context.Background(), nil, evals.ProbeRun{
		Suite: evals.SeedProbeSuite(), EngineID: "claude-cli",
	}); err != evals.ErrNoRunner {
		t.Fatalf("no runner: %v, want ErrNoRunner", err)
	}
}
