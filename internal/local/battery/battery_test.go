package battery

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
)

// cannedCompleter returns a fixed completion per model/prompt for hermetic
// tests — no network, no real model.
type cannedCompleter struct {
	fn func(spec local.ChatSpec) (local.Completion, error)
}

func (c cannedCompleter) Chat(_ context.Context, spec local.ChatSpec) (local.Completion, error) {
	return c.fn(spec)
}

// classResp builds a classification completion with a label and a margin at the
// label token (so LabelMargin can read it).
func classResp(labels map[string]string, margin float64) local.Completion {
	// Reconstruct a JSON object with the labels; give the first label field's
	// value token a controlled top1/top2 gap.
	obj := map[string]any{"reason": "ok", "abstain": false}
	for k, v := range labels {
		obj[k] = v
	}
	raw, _ := json.Marshal(obj)
	// Build a logprob stream where the value of "verdict"/"family"/... carries
	// the margin. We emit the whole JSON as tokens: a coarse split is fine — the
	// value token is its own piece.
	var lps []local.TokenLogprob
	// emit `{"reason":"ok",` then each label as `"key":"val"` with the val token
	// carrying the gap, then `,"abstain":false}`.
	push := func(text string, gap float64) {
		lps = append(lps, local.TokenLogprobFixture(text, gap))
	}
	push(`{"reason":"ok"`, 3.0)
	for k, v := range labels {
		push(`,"`+k+`":"`, 3.0)
		push(v, margin)
		push(`"`, 3.0)
	}
	push(`,"abstain":false}`, 3.0)
	return local.Completion{Content: string(raw), Logprobs: lps, OutputTokens: 12}
}

func TestRunSuiteClassificationScoresAndMargins(t *testing.T) {
	suite := &Suite{
		Duty: "entailment", Kind: KindClassification,
		System: "judge", Fields: []local.LabelField{{Name: "verdict", Enum: []string{"supported", "unsupported"}}},
		LabelFields: []string{"verdict"},
		Cases: []Case{
			{ID: "e-01", Prompt: "claim vs source", ExpectLabels: map[string]string{"verdict": "supported"}},
			{ID: "e-02", Prompt: "claim vs source", ExpectLabels: map[string]string{"verdict": "supported"}},
		},
	}
	// The model answers "supported" for both (both pass), with margin 1.5.
	comp := cannedCompleter{fn: func(spec local.ChatSpec) (local.Completion, error) {
		return classResp(map[string]string{"verdict": "supported"}, 1.5), nil
	}}
	r := NewRunner(comp, nil, nil)
	out, err := r.RunSuite(context.Background(), suite, Target{Model: "m", ModelHash: "hh", EngineBuild: "b10085"})
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if out.Result.Passed != 2 || out.Result.Total != 2 {
		t.Errorf("passed %d/%d, want 2/2", out.Result.Passed, out.Result.Total)
	}
	if len(out.Labeled) != 2 {
		t.Fatalf("expected 2 labeled items (margins), got %d", len(out.Labeled))
	}
	for _, li := range out.Labeled {
		if li.Wrong {
			t.Error("a passing case should be labeled not-wrong")
		}
		if li.Margin < 1.4 || li.Margin > 1.6 {
			t.Errorf("margin %v, want ~1.5", li.Margin)
		}
	}
	if out.Result.WilsonLo <= 0 {
		t.Error("Wilson interval should be filled")
	}
}

func TestRunSuiteClassificationFailsOnWrongLabel(t *testing.T) {
	suite := &Suite{
		Duty: "entailment", Kind: KindClassification, System: "judge",
		Fields:      []local.LabelField{{Name: "verdict", Enum: []string{"supported", "unsupported"}}},
		LabelFields: []string{"verdict"},
		Cases:       []Case{{ID: "e-01", Prompt: "x", ExpectLabels: map[string]string{"verdict": "supported"}}},
	}
	comp := cannedCompleter{fn: func(spec local.ChatSpec) (local.Completion, error) {
		return classResp(map[string]string{"verdict": "unsupported"}, 0.3), nil // wrong
	}}
	out, _ := NewRunner(comp, nil, nil).RunSuite(context.Background(), suite, Target{Model: "m", ModelHash: "h", EngineBuild: "b"})
	if out.Result.Passed != 0 {
		t.Errorf("a wrong label must fail: passed %d", out.Result.Passed)
	}
	if len(out.Labeled) != 1 || !out.Labeled[0].Wrong {
		t.Errorf("the labeled item should be marked wrong: %+v", out.Labeled)
	}
}

func TestRunSuiteOutputContract(t *testing.T) {
	suite := &Suite{
		Duty: "utility", Kind: KindOutputContract, System: "draft",
		Cases: []Case{
			{ID: "u-01", Prompt: "x", Contract: &Contract{MustContainFields: []string{"what", "wrong", "recommend"}, MaxChars: 2000}},
			{ID: "u-02", Prompt: "x", Contract: &Contract{MaxChars: 10}}, // will fail the length cap
		},
	}
	comp := cannedCompleter{fn: func(spec local.ChatSpec) (local.Completion, error) {
		return local.Completion{Content: `{"what":"a","wrong":"b","recommend":"c"}`, OutputTokens: 8}, nil
	}}
	out, _ := NewRunner(comp, nil, nil).RunSuite(context.Background(), suite, Target{Model: "m", ModelHash: "h", EngineBuild: "b"})
	if out.Result.Passed != 1 {
		t.Errorf("one case passes the contract, one fails the length cap; passed %d", out.Result.Passed)
	}
	if len(out.Labeled) != 0 {
		t.Error("output-contract suites produce no margins (generation shape)")
	}
}

// stubSettings satisfies local.Settings for the AC gate test.
type stubSettings struct{ acOnly bool }

func (s stubSettings) Int(string) (int64, error)                   { return 0, nil }
func (s stubSettings) Bool(k string) (bool, error)                 { return s.acOnly, nil }
func (s stubSettings) String(string) (string, error)               { return "", nil }
func (s stubSettings) StringMap(string) (map[string]string, error) { return nil, nil }

func TestACGateRefusesOnBattery(t *testing.T) {
	suite := SeedSuites()[0]
	comp := cannedCompleter{fn: func(spec local.ChatSpec) (local.Completion, error) {
		return classResp(map[string]string{"family": "software", "stakes": "low", "size": "small"}, 1.0), nil
	}}
	// ac_only true + on battery ⇒ refuse.
	r := NewRunner(comp, stubSettings{acOnly: true}, &local.FakePowerReader{OnBat: true})
	if _, err := r.RunSuite(context.Background(), suite, Target{Model: "m", ModelHash: "h", EngineBuild: "b"}); err != ErrOnBattery {
		t.Errorf("expected ErrOnBattery, got %v", err)
	}
	// ac_only true + on AC ⇒ allowed.
	r2 := NewRunner(comp, stubSettings{acOnly: true}, &local.FakePowerReader{OnBat: false})
	if _, err := r2.RunSuite(context.Background(), suite, Target{Model: "m", ModelHash: "h", EngineBuild: "b"}); err != nil {
		t.Errorf("on AC the battery gate must allow the run, got %v", err)
	}
}

func TestSeedSuitesValidate(t *testing.T) {
	suites := SeedSuites()
	if len(suites) < 8 {
		t.Fatalf("expected the covered duty suites, got %d", len(suites))
	}
	for _, s := range suites {
		if err := s.Validate(); err != nil {
			t.Errorf("seed suite %q invalid: %v", s.Duty, err)
		}
	}
	// embedder is deliberately absent (post-gate).
	if _, ok := SuiteByDuty("embedder"); ok {
		t.Error("embedder must have no suite (post-gate)")
	}
	// intake-triage (the bakeoff duty) meets the governance target.
	if s, ok := SuiteByDuty("intake-triage"); !ok || len(s.Cases) < TargetPerDuty {
		t.Errorf("intake-triage should meet the %d-case target (the bakeoff duty)", TargetPerDuty)
	}
}
