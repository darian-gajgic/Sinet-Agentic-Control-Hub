package verify_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.4 acceptance: the entailment machinery is present and SHIPS IDLE —
// activation is gated on the web-research domain plus the TBD-BRINGUP
// calibration bar; coverage splits mandatory/sampled on the ⚙ rate; pairs
// are judged against fetched source content by type construction.

type echoChecker struct {
	calls []verify.EntailmentPair
}

func (c *echoChecker) Entail(_ context.Context, p verify.EntailmentPair) (verify.EntailmentVerdict, error) {
	c.calls = append(c.calls, p)
	if strings.Contains(p.FetchedContent, p.Claim.Text) {
		return verify.EntailSupported, nil
	}
	return verify.EntailUnsupported, nil
}

func TestEntailmentShipsIdle(t *testing.T) {
	s := regSettings(t)
	cases := []struct {
		name   string
		gate   *verify.EntailmentGate
		domain string
		active bool
	}{
		{"nil gate", nil, verify.DomainWebResearch, false},
		{"software domain even calibrated", &verify.EntailmentGate{Settings: s, Checker: &echoChecker{}, Calibrated: true}, verify.DomainSoftware, false},
		{"web-research uncalibrated", &verify.EntailmentGate{Settings: s, Checker: &echoChecker{}}, verify.DomainWebResearch, false},
		{"web-research calibrated no seat", &verify.EntailmentGate{Settings: s, Calibrated: true}, verify.DomainWebResearch, false},
		{"web-research calibrated with seat", &verify.EntailmentGate{Settings: s, Checker: &echoChecker{}, Calibrated: true}, verify.DomainWebResearch, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.gate.Active(tc.domain); got != tc.active {
				t.Fatalf("Active=%v want %v", got, tc.active)
			}
		})
	}
	// Checking through an inactive gate errors — idle is loud, never a
	// silent fake pass.
	idle := &verify.EntailmentGate{Settings: s}
	if _, err := idle.Check(context.Background(), verify.DomainSoftware, nil); err == nil {
		t.Fatal("idle gate accepted a check")
	}
}

func TestEntailmentCoverageSplit(t *testing.T) {
	s := regSettings(t) // ⚙ verification.entailment_sample_rate: TBD-BRINGUP default 0
	claims := []verify.Claim{
		{ID: "load-1", Text: "a", SourceURL: "u", LoadBearing: true},
		{ID: "rest-1", Text: "b", SourceURL: "u"},
		{ID: "rest-2", Text: "c", SourceURL: "u"},
	}
	gate := &verify.EntailmentGate{Settings: s}

	// Rate 0: mandatory coverage only.
	checked, sampledOut, err := gate.Coverage(claims)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if len(checked) != 1 || checked[0].ID != "load-1" || sampledOut != 2 {
		t.Fatalf("rate-0 coverage: %v sampledOut=%d", checked, sampledOut)
	}

	// Rate 1: everything.
	gate.Settings = testSettings{base: newFix(t).reg, floats: map[string]float64{"verification.entailment_sample_rate": 1}}
	checked, sampledOut, err = gate.Coverage(claims)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if len(checked) != 3 || sampledOut != 0 {
		t.Fatalf("rate-1 coverage: %v", checked)
	}

	// Deterministic: the same set on a re-run (no RNG state).
	again, _, err := gate.Coverage(claims)
	if err != nil || len(again) != len(checked) {
		t.Fatalf("sampling not deterministic: %v %v", again, err)
	}
}

func TestEntailmentJudgesFetchedContent(t *testing.T) {
	s := regSettings(t)
	chk := &echoChecker{}
	gate := &verify.EntailmentGate{Settings: s, Checker: chk, Calibrated: true}
	pairs := []verify.EntailmentPair{
		{Claim: verify.Claim{ID: "c1", Text: "licensed Apache-2.0", SourceURL: "https://x", LoadBearing: true},
			FetchedContent: "This project is licensed Apache-2.0."},
		{Claim: verify.Claim{ID: "c2", Text: "licensed MIT", SourceURL: "https://x", LoadBearing: true},
			FetchedContent: "This project is licensed Apache-2.0."},
	}
	out, err := gate.Check(context.Background(), verify.DomainWebResearch, pairs)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if out[0].Verdict != verify.EntailSupported || out[1].Verdict != verify.EntailUnsupported {
		t.Fatalf("verdicts: %+v", out)
	}
	// The checker received the FETCHED content — the only text there is to
	// judge against (structural, Spec S07.4).
	for _, call := range chk.calls {
		if call.FetchedContent == "" {
			t.Fatal("checker called without fetched content")
		}
	}
}

func TestPipelineRecordsEntailmentIdle(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	if _, err := v.Verify(context.Background(), input(deliverable("t1", "r1"))); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	rounds := f.events(verify.EventRound)
	if len(rounds) != 1 || !strings.Contains(string(rounds[0].Payload), `"entailment":"idle`) {
		t.Fatalf("idle posture not recorded on the verdict row")
	}
}
