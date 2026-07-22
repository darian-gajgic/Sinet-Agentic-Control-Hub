package stage_test

import (
	"context"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// fixedMeter returns a pre-created running run for advisory metering.
func fixedMeter(runs *run.Store, id string) stage.AdvisoryMeter {
	return func(ctx context.Context, _ string) (string, func(), error) {
		return id, func() {}, nil
	}
}

func TestEntailmentCheckerVerdictMapping(t *testing.T) {
	ctx := context.Background()
	duty, fake, _, runs := localSeamEnv(t)
	runningRun(t, runs, "adv.run")
	checker := stage.NewEntailmentChecker(duty, fixedMeter(runs, "adv.run"))

	// entailment alias → the Granite Guardian seat (servable in the manifest).
	granite := "Granite Guardian 3.3-8B"
	pair := verify.EntailmentPair{
		Claim:          verify.Claim{ID: "c1", Text: "The library is Apache-2.0 licensed."},
		FetchedContent: "This project is licensed under the Apache License, Version 2.0.",
	}
	fake.SetModelResponse(granite, local.FakeResponse{Content: `{"reason":"entailed","verdict":"supported","abstain":false}`, InputTokens: 30, OutputTokens: 5})
	if v, err := checker.Entail(ctx, pair); err != nil || v != verify.EntailSupported {
		t.Errorf("supported mapping: got %v %v", v, err)
	}
	fake.SetModelResponse(granite, local.FakeResponse{Content: `{"reason":"contradicted","verdict":"unsupported","abstain":false}`})
	if v, _ := checker.Entail(ctx, pair); v != verify.EntailUnsupported {
		t.Errorf("unsupported mapping: got %v", v)
	}
	fake.SetModelResponse(granite, local.FakeResponse{Content: `{"reason":"cant tell","verdict":"unsupported","abstain":true}`})
	if v, _ := checker.Entail(ctx, pair); v != verify.EntailUnverifiable {
		t.Errorf("abstain ⇒ unverifiable: got %v", v)
	}
}

func TestEntailmentCheckerEmptySourceUnverifiable(t *testing.T) {
	ctx := context.Background()
	duty, _, _, runs := localSeamEnv(t)
	runningRun(t, runs, "adv.run")
	checker := stage.NewEntailmentChecker(duty, fixedMeter(runs, "adv.run"))
	// Empty fetched content cannot decide — unverifiable, no model call.
	v, err := checker.Entail(ctx, verify.EntailmentPair{Claim: verify.Claim{ID: "c1", Text: "x"}, FetchedContent: ""})
	if err != nil || v != verify.EntailUnverifiable {
		t.Errorf("empty source ⇒ unverifiable; got %v %v", v, err)
	}
}

func TestEntailmentGateStaysIdle(t *testing.T) {
	// R17: the checker is LIVE-wired but the GATE stays idle — the software
	// launch domain never activates entailment, and calibrated=false blocks it.
	duty, _, _, runs := localSeamEnv(t)
	runningRun(t, runs, "adv.run")
	checker := stage.NewEntailmentChecker(duty, fixedMeter(runs, "adv.run"))
	gate := &verify.EntailmentGate{Checker: checker, Calibrated: false}
	if gate.Active(verify.DomainSoftware) {
		t.Error("entailment must be idle for the software domain (S07.4)")
	}
	if gate.Active(verify.DomainWebResearch) {
		t.Error("entailment must stay idle while uncalibrated even for web-research (G3 Def.4)")
	}
}

func TestEntailmentCheckerNilSafe(t *testing.T) {
	if c := stage.NewEntailmentChecker(nil, nil); c != nil {
		t.Error("a nil duty must yield a nil checker")
	}
}
