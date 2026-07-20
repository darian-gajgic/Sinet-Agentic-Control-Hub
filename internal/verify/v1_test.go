package verify_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.3 acceptance: the executable ladder runs cheap-first with
// first-upstream-failure attribution, stage contracts carry the four
// ratified states, quarantined checks are never retried, the verified-on
// stamp goes stale on the ⚙ audit interval, and the pass/fail verdict is
// computed platform-side through the sandbox seam.

func ladderPack() *verify.CheckPack {
	prov := "authored separately (S07.3 rule 4)"
	return &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "lint", Stage: verify.StageStatic, Argv: []string{"lint"}, StepID: "S-1", FindingCategory: verify.CatACBlocker},
			{ID: "unit", Stage: verify.StageUnit, Argv: []string{"unit"}, StepID: "S-1", ACKey: "AC-2", Provenance: prov, FindingCategory: verify.CatACBlocker},
			{ID: "smoke", Stage: verify.StageSmoke, Argv: []string{"smoke"}, StepID: "S-2", FindingCategory: verify.CatACBlocker},
			{ID: "e2e", Stage: verify.StageE2E, Argv: []string{"e2e"}, StepID: "S-2", FindingCategory: verify.CatACBlocker},
		},
	}
}

func ladderSteps() []intake.Step {
	return []intake.Step{
		{ID: "S-1", Title: "impl", DoneWhen: "static and unit pass", Class: "C1"},
		{ID: "S-2", Title: "wire", DoneWhen: "smoke and e2e pass", Class: "C1"},
		{ID: "S-3", Title: "docs", DoneWhen: "reviewer judges the prose", Class: "C1"},
	}
}

func v1req() verify.CheckRequest {
	return verify.CheckRequest{RunID: "r1", Workspace: "ws", EvidenceDir: "ev"}
}

func regSettings(t *testing.T) testSettings {
	t.Helper()
	return testSettings{base: newFix(t).reg}
}

func TestPackValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *verify.CheckPack)
	}{
		{"no verified-on stamp", func(p *verify.CheckPack) { p.VerifiedOn = time.Time{} }},
		{"acceptance check without provenance", func(p *verify.CheckPack) { p.Checks[1].Provenance = "" }},
		{"undeclared finding category", func(p *verify.CheckPack) { p.Checks[0].FindingCategory = "" }},
		{"unknown ladder stage", func(p *verify.CheckPack) { p.Checks[0].Stage = "fuzz" }},
		{"duplicate id", func(p *verify.CheckPack) { p.Checks[1].ID = "lint" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := ladderPack()
			tc.mutate(p)
			if err := p.Validate(); !errors.Is(err, verify.ErrBadPack) {
				t.Fatalf("want ErrBadPack, got %v", err)
			}
		})
	}
	if err := ladderPack().Validate(); err != nil {
		t.Fatalf("valid pack rejected: %v", err)
	}
}

func TestLadderFirstUpstreamFailure(t *testing.T) {
	s := regSettings(t)
	r := &scriptRunner{exits: map[string]int{"unit": 1}}
	res, err := verify.RunV1(context.Background(), ladderPack(), r, v1req(), ladderSteps(), time.Now(), s)
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	states := map[string]verify.CheckOutcomeState{}
	attributed := map[string]string{}
	for _, c := range res.Checks {
		states[c.CheckID] = c.State
		attributed[c.CheckID] = c.AttributedTo
	}
	if states["lint"] != verify.CheckPassed || states["unit"] != verify.CheckFailed {
		t.Fatalf("ladder states wrong: %v", states)
	}
	// The failing stage stops later stages: smoke/e2e are
	// UNVERIFIABLE-HERE, attributed to the first upstream failure.
	for _, id := range []string{"smoke", "e2e"} {
		if states[id] != verify.CheckUnverifiable {
			t.Fatalf("%s: want UNVERIFIABLE-HERE, got %s", id, states[id])
		}
		if attributed[id] != "unit" {
			t.Fatalf("%s attributed to %q, want unit", id, attributed[id])
		}
	}
	if len(r.calls) != 2 { // lint + unit only — cheap-first stops paid-nothing later rungs
		t.Fatalf("later-stage checks ran after the failure: %v", r.calls)
	}
	// Stage contracts (Spec S07.3): S-1 FAIL, S-2 UNVERIFIABLE-HERE
	// (attributed), S-3 N-A (no mechanical check decides it).
	byStep := map[string]verify.StepContract{}
	for _, sc := range res.Steps {
		byStep[sc.StepID] = sc
	}
	if byStep["S-1"].State != verify.ContractFail || byStep["S-1"].AttributedTo != "unit" {
		t.Fatalf("S-1 contract: %+v", byStep["S-1"])
	}
	if byStep["S-2"].State != verify.ContractUnverifiable || byStep["S-2"].AttributedTo != "unit" {
		t.Fatalf("S-2 contract: %+v", byStep["S-2"])
	}
	if byStep["S-3"].State != verify.ContractNA {
		t.Fatalf("S-3 contract: %+v", byStep["S-3"])
	}
	// Contract completeness: category + escalation route declared.
	if byStep["S-1"].Category == "" || byStep["S-1"].Route == "" {
		t.Fatalf("S-1 contract without category/route (Spec S07.3): %+v", byStep["S-1"])
	}
}

func TestQuarantineSkippedNeverRetried(t *testing.T) {
	s := regSettings(t)
	p := ladderPack()
	if err := p.QuarantineCheck(verify.Quarantine{
		CheckID: "unit", Owner: "op", Reason: "flaky on CI", FixBy: time.Now().Add(48 * time.Hour),
	}); err != nil {
		t.Fatalf("QuarantineCheck: %v", err)
	}
	r := &scriptRunner{}
	res, err := verify.RunV1(context.Background(), p, r, v1req(), ladderSteps(), time.Now(), s)
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	for _, id := range r.calls {
		if id == "unit" {
			t.Fatal("quarantined check was executed (never retried-until-green, S07.3 rule 6)")
		}
	}
	found := false
	for _, c := range res.Checks {
		if c.CheckID == "unit" && c.State == verify.CheckQuarantined {
			found = true
		}
	}
	if !found {
		t.Fatalf("quarantine skip not recorded: %+v", res.Checks)
	}
	if len(res.Findings) == 0 || res.Findings[0].Category != verify.CatCheckIntegrity {
		t.Fatalf("quarantine skip must surface as a CHECK-INTEGRITY finding, got %+v", res.Findings)
	}
	// Quarantine requires owner + fix-by (rule 6).
	if err := p.QuarantineCheck(verify.Quarantine{CheckID: "lint", Owner: "", FixBy: time.Now()}); !errors.Is(err, verify.ErrBadPack) {
		t.Fatalf("ownerless quarantine accepted: %v", err)
	}
}

func TestAuditStaleFlagsVerdict(t *testing.T) {
	s := regSettings(t) // ⚙ verification.check_audit_interval_days = 90
	p := ladderPack()
	p.VerifiedOn = time.Now().Add(-91 * 24 * time.Hour)
	res, err := verify.RunV1(context.Background(), p, &scriptRunner{}, v1req(), nil, time.Now(), s)
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	if !res.StaleAudit {
		t.Fatal("suite past ⚙ audit interval not flagged stale (P-T06-1)")
	}
	p.VerifiedOn = time.Now().Add(-24 * time.Hour)
	res, err = verify.RunV1(context.Background(), p, &scriptRunner{}, v1req(), nil, time.Now(), s)
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	if res.StaleAudit {
		t.Fatal("fresh suite flagged stale")
	}
}

func TestSandboxCheckRunnerPlatformSideVerdict(t *testing.T) {
	// The runner composes through the Confiner seam (network-off class) and
	// the PLATFORM derives pass/fail from the wait status; the evidence
	// artifact is retained (Spec S07.3 rules 1/3; refs-not-blobs).
	conf := &fakeConfiner{}
	r := &verify.SandboxCheckRunner{Confiner: conf}
	ev := t.TempDir()
	res, err := r.RunCheck(context.Background(), verify.CheckRequest{
		RunID:       "r1",
		Check:       verify.Check{ID: "unit", Stage: verify.StageUnit, Argv: []string{"run-checks", "3"}, FindingCategory: verify.CatACBlocker},
		Workspace:   t.TempDir(),
		EvidenceDir: ev,
	})
	if err != nil {
		t.Fatalf("RunCheck: %v", err)
	}
	if conf.req.Class != "C2" {
		t.Fatalf("default check class %q, want C2 (network-off until the egress substrate lands, CONVENTIONS §12)", conf.req.Class)
	}
	if len(conf.spec.Argv) != 2 || conf.spec.Argv[0] != "run-checks" {
		t.Fatalf("check argv not passed through the seam: %v", conf.spec.Argv)
	}
	if res.ExitCode != 3 {
		t.Fatalf("platform-side exit derivation: got %d want 3", res.ExitCode)
	}
	raw, err := os.ReadFile(filepath.Join(ev, "unit.log"))
	if err != nil || !strings.Contains(string(raw), "check-output") {
		t.Fatalf("evidence artifact not retained: %v %q", err, raw)
	}
	if res.EvidenceSHA == "" {
		t.Fatal("evidence without content hash")
	}
}

func TestRunnerFailureIsNotAVerdict(t *testing.T) {
	s := regSettings(t)
	r := &scriptRunner{errs: map[string]error{"lint": errors.New("sandbox compose failed")}}
	res, err := verify.RunV1(context.Background(), ladderPack(), r, v1req(), nil, time.Now(), s)
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	var lint verify.CheckOutcome
	for _, c := range res.Checks {
		if c.CheckID == "lint" {
			lint = c
		}
	}
	if lint.State != verify.CheckRunnerFailed {
		t.Fatalf("runner failure recorded as %s", lint.State)
	}
	// A screen outage escalates rather than approves: blocker finding.
	found := false
	for _, f := range res.Findings {
		if f.Category == verify.CatCheckIntegrity && f.Severity == verify.SeverityBlocker {
			found = true
		}
	}
	if !found {
		t.Fatalf("runner failure did not raise a blocker finding: %+v", res.Findings)
	}
}

func TestCheckResearch(t *testing.T) {
	s := regSettings(t) // ⚙ verification.research_rerun_limit = 1
	ctx := context.Background()
	nodes := []intake.ResearchNode{{RuleID: "P47-1", StepID: "S-1"}}

	// Zero invocations, budget available → one fresh-session re-run.
	out, err := verify.CheckResearch(ctx, verify.CanaryResearchUsage(), "t1", nodes, map[string]int{}, s)
	if err != nil {
		t.Fatalf("CheckResearch: %v", err)
	}
	if !out[0].RerunRequested || out[0].NeedsCard {
		t.Fatalf("want rerun request, got %+v", out[0])
	}

	// Budget consumed, still zero → RESEARCH-NOT-RUN card.
	out, err = verify.CheckResearch(ctx, verify.CanaryResearchUsage(), "t1", nodes, map[string]int{"P47-1": 1}, s)
	if err != nil {
		t.Fatalf("CheckResearch: %v", err)
	}
	if !out[0].NeedsCard {
		t.Fatalf("want card after the re-run budget, got %+v", out[0])
	}

	// Counters unknown → UNVERIFIABLE-HERE, loudly, never a fake pass and
	// never a false card.
	out, err = verify.CheckResearch(ctx, nil, "t1", nodes, map[string]int{}, s)
	if err != nil {
		t.Fatalf("CheckResearch: %v", err)
	}
	if out[0].State != verify.ContractUnverifiable || out[0].NeedsCard {
		t.Fatalf("unknown counters: %+v", out[0])
	}
}
