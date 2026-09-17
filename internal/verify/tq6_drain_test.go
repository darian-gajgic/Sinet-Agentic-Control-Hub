package verify_test

// tq6_drain_test.go — P3-TQ-6 drain round 1: the four guards the acceptance
// battery left unpinned (evaluation F1–F4), plus the path-neutral wording of
// the disagreement sentence (W). Each test was mutation-verified against the
// guard it covers: removing the guard turns it red.
//
// F1 dedupe by anchor, F2 the Unknown guard, F3 "every check" in the FAIL
// detail (R3), F4 the caller's verdict slice is never mutated (R4).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq6FailContract is one refuted step contract in the shape both V1 branches
// produce: a FAIL carrying the plain-words reason behind it.
func tq6FailContract(stepID, detail string) verify.StepContract {
	return verify.StepContract{
		StepID:   stepID,
		DoneWhen: "the page loads",
		State:    verify.ContractFail,
		Detail:   detail,
	}
}

// TestTQ6DuplicateCoverageEntryRaisesOneDisagreement [F1]: intake checks only
// that each step a coverage entry cites exists, so AC-1 → [S-1, S-1] is an
// accepted plan shape. Two findings on one anchor would be two rows with the
// identical S07.6 finding key — one ask, carded twice. The detector emits
// exactly one. This also pins the W wording: the sentence must be true on the
// bootstrap path too, where no check ran and the tree decided the contract.
func TestTQ6DuplicateCoverageEntryRaisesOneDisagreement(t *testing.T) {
	verdicts := []verify.ACVerdict{{Key: "AC-1", Pass: true, BoundTo: "plain"}}
	contracts := []verify.StepContract{tq6FailContract("S-1", `The automated check "lint" did not pass.`)}

	out, findings := verify.ContractDisagreements(verdicts, contracts, map[string][]string{"AC-1": {"S-1", "S-1"}})

	if len(findings) != 1 {
		t.Fatalf("findings %d, want exactly 1 — a step named twice in one coverage entry is one disagreement: %+v", len(findings), findings)
	}
	fd := findings[0]
	if fd.Anchor != "step:S-1/AC-1" {
		t.Fatalf("anchor %q, want step:S-1/AC-1", fd.Anchor)
	}
	if got, want := fd.Key(), verify.FindingKey("CHECK-INTEGRITY|step:S-1/AC-1|CHECK-INTEGRITY"); got != want {
		t.Fatalf("finding key %q, want %q (round-stable, carded once per drain)", got, want)
	}
	if !out[0].Disagreement {
		t.Fatalf("verdict %+v, want Disagreement marked", out[0])
	}
	// W: the opening must hold on both V1 branches. On the bootstrap path the
	// files decided the contract and no check ran, so "the automated check
	// found" would be a false sentence on the requester's card.
	if !strings.HasPrefix(fd.Text, "Verification found step S-1's promise unmet, but the review marked criterion 1 as met.") {
		t.Fatalf("disagreement text %q does not open in the path-neutral words", fd.Text)
	}
	if strings.Contains(fd.Text, "The automated check found") {
		t.Fatalf("disagreement text %q asserts a check ran; on the bootstrap path none did", fd.Text)
	}
	assertPlainWords(t, fd.Text)
}

// TestTQ6UnknownVerdictRaisesNoDisagreement [F2]: ValidateAxis1 passes a
// judge-returned verdict that is both Unknown and Pass through untouched, so
// the detector's own Unknown guard is what stops an undecided criterion from
// contradicting anything. An Unknown verdict asserts nothing, so it cannot
// disagree with the checks — raising a card there would spend a person's
// attention on a criterion the judge declined to decide.
func TestTQ6UnknownVerdictRaisesNoDisagreement(t *testing.T) {
	verdicts := []verify.ACVerdict{{Key: "AC-1", Pass: true, Unknown: true, BoundTo: "plain"}}
	contracts := []verify.StepContract{tq6FailContract("S-1", `The automated check "lint" did not pass.`)}

	out, findings := verify.ContractDisagreements(verdicts, contracts, map[string][]string{"AC-1": {"S-1"}})

	if len(findings) != 0 {
		t.Fatalf("an undecided criterion raised a disagreement: %+v", findings)
	}
	if out[0].Disagreement {
		t.Fatalf("verdict %+v marked as disagreeing while Unknown", out[0])
	}
}

// TestTQ6FailDetailNamesEveryFailedCheck [F3, R3]: two checks at the same rung
// bind one step and both fail. R3 says the detail names every check that
// failed — a requester told only "lint" would fix it and see the step fail
// again on the check nobody mentioned.
func TestTQ6FailDetailNamesEveryFailedCheck(t *testing.T) {
	ctx := context.Background()
	pack := &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "lint", Stage: verify.StageStatic, Argv: []string{"lint"}, StepID: "S-1", FindingCategory: verify.CatACBlocker},
			{ID: "unit", Stage: verify.StageStatic, Argv: []string{"unit"}, StepID: "S-1", FindingCategory: verify.CatACBlocker},
		},
	}
	steps := []intake.Step{{ID: "S-1", Title: "impl", DoneWhen: "static passes", Class: "C1"}}

	res, err := verify.RunV1(ctx, pack, &scriptRunner{exits: map[string]int{"lint": 1, "unit": 1}},
		v1req(), steps, map[string][]string{"AC-1": {"S-1"}}, time.Now(), regSettings(t))
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	if len(res.Steps) != 1 || res.Steps[0].State != verify.ContractFail {
		t.Fatalf("contracts %+v, want one FAIL", res.Steps)
	}
	if got, want := res.Steps[0].Detail, `The automated checks "lint" and "unit" did not pass.`; got != want {
		t.Fatalf("FAIL detail %q, want %q — every failed check is named", got, want)
	}
	assertPlainWords(t, res.Steps[0].Detail)

	fd, ok := findingAt(res.Findings, "step:S-1")
	if !ok {
		t.Fatalf("no finding anchored step:S-1: %+v", res.Findings)
	}
	for _, want := range []string{"lint", "unit"} {
		if !strings.Contains(fd.Text, want) {
			t.Fatalf("finding text %q does not name the failed check %q", fd.Text, want)
		}
	}
}

// TestTQ6DetectorNeverMutatesTheCallersVerdicts [F4, R4]: the detector marks
// the verdicts it RETURNS. The drain assigns those to the round record, so a
// detector writing through to the caller's backing array would be invisible
// here and load-bearing anywhere the pre-detector verdicts are still read.
func TestTQ6DetectorNeverMutatesTheCallersVerdicts(t *testing.T) {
	verdicts := []verify.ACVerdict{
		{Key: "AC-1", Pass: true, BoundTo: "plain"},
		{Key: "AC-2", Pass: true, BoundTo: "plain"},
	}
	contracts := []verify.StepContract{
		tq6FailContract("S-1", `The automated check "lint" did not pass.`),
		{StepID: "S-2", DoneWhen: "the data loads", State: verify.ContractPass},
	}
	coverage := map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-2"}}

	out, findings := verify.ContractDisagreements(verdicts, contracts, coverage)

	if len(findings) != 1 {
		t.Fatalf("findings %+v, want exactly the AC-1 disagreement", findings)
	}
	for i, v := range verdicts {
		if v.Disagreement {
			t.Fatalf("the caller's own verdict %d (%+v) was marked — the detector copies before it writes", i, v)
		}
	}
	if !out[0].Disagreement {
		t.Fatalf("returned AC-1 verdict %+v, want Disagreement marked", out[0])
	}
	if out[1].Disagreement {
		t.Fatalf("returned AC-2 verdict %+v, want it untouched (its step passed)", out[1])
	}
}
