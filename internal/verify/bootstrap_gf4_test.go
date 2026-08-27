package verify_test

// bootstrap_gf4_test.go — P3-GF4 acceptance battery for the bootstrap
// verification posture (Spec S07.8, amendment A14 2026-08-27): the drain of a
// launch-domain deliverable whose registered project has no captured
// build/test/lint command RUNS — honestly, advisorily, and without ever
// parking on the absence.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// bootstrapPack is the seam's answer for a registered project holding no
// executable rung.
func bootstrapPack() *verify.CheckPack {
	return verify.BootstrapPack(verify.DomainSoftware, 2)
}

// TestGF4BootstrapDrainShipsWithNotes [R1, R3, R4, R5, R12]: a clean artifact
// under the bootstrap posture reaches a drain outcome — never an
// infrastructure card — with a V1 record that is present and honest, a
// note-class posture disclosure, and no ledger item flipped to verified on an
// advisory verdict.
func TestGF4BootstrapDrainShipsWithNotes(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())
	in := input(deliverable("t1", "r1"))
	// The trivial band changes V3's blocking-ness, never the drain (S06.4);
	// under bootstrap V3 blocks at every tier, so this is the harder case.
	in.Tier = intake.TierTrivial

	out, err := v.Verify(ctx, in)
	if err != nil {
		t.Fatalf("Verify: %v — the bootstrap posture is a drain, not a refusal (Spec S07.8)", err)
	}
	if out.Card != nil {
		t.Fatalf("bootstrap drain raised a card: %+v", out.Card)
	}
	if out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s, want SHIP-with-notes — an advisory verdict is never a clean SHIP (Spec S07.8)", out.Verdict)
	}
	if len(out.Rounds) != 1 {
		t.Fatalf("rounds = %d, want 1", len(out.Rounds))
	}
	r := out.Rounds[0]

	// V0 present and unchanged in behavior [R2].
	if r.V0 == nil || r.V0.Malformed {
		t.Fatalf("V0 record = %+v, want the ordinary passing pre-gate result", r.V0)
	}

	// V1 present and honest [R3].
	if r.V1 == nil {
		t.Fatal("no V1 record — the bootstrap posture RUNS V1's stage-contract checks, it does not skip the layer")
	}
	if len(r.V1.Checks) == 0 {
		t.Fatal("no ladder rung recorded — a silent skip is exactly what UNVERIFIABLE-HERE exists to prevent")
	}
	for _, c := range r.V1.Checks {
		if c.State != verify.CheckUnverifiable {
			t.Fatalf("rung %q state %q, want UNVERIFIABLE-HERE (never PASS, never absent)", c.CheckID, c.State)
		}
		if strings.TrimSpace(c.Detail) == "" {
			t.Fatalf("rung %q records no detail naming the missing capture", c.CheckID)
		}
		if c.EvidenceRef != "" || c.EvidenceSHA != "" {
			t.Fatalf("rung %q carries fabricated evidence %q/%q — nothing ran", c.CheckID, c.EvidenceRef, c.EvidenceSHA)
		}
	}
	if len(r.V1.Steps) != len(steps()) {
		t.Fatalf("step contracts = %d, want one per PLAN step (%d)", len(r.V1.Steps), len(steps()))
	}
	for _, sc := range r.V1.Steps {
		if sc.State != verify.ContractUnverifiable {
			t.Fatalf("step %q contract %q, want UNVERIFIABLE-HERE — the deciding substrate is absent, not inapplicable", sc.StepID, sc.State)
		}
		if sc.AttributedTo != verify.BootstrapAttribution {
			t.Fatalf("step %q attributed to %q, want the stable %q marker", sc.StepID, sc.AttributedTo, verify.BootstrapAttribution)
		}
	}
	if r.V1.StaleAudit {
		t.Fatal("a bootstrap round flagged a stale audit — the P-T06-1 interval applies only when a real suite runs")
	}
	if n := len(f.events(verify.EventV1)); n != 1 {
		t.Fatalf("verify.v1 event rows = %d, want 1 (Spec S07.11 records the layer that ran)", n)
	}

	// The posture is durable on the round record [R5], and review is mandatory
	// as DATA rather than folklore [R6b].
	if r.Posture != verify.PostureBootstrap {
		t.Fatalf("round posture %q, want %q", r.Posture, verify.PostureBootstrap)
	}
	if !strings.Contains(r.PostureNote, "restores the full ladder") {
		t.Fatalf("round posture note %q does not say what capturing the project's commands restores", r.PostureNote)
	}
	if !r.ReviewMandatory {
		t.Fatal("the round record does not carry the mandatory-review fact (Spec S07.8: V3 blocks at every stakes tier)")
	}

	// Both axes ran [R5].
	if len(r.Axis1) == 0 || r.Axis2 == nil {
		t.Fatalf("axis1=%d axis2=%v — a launch domain gets axis 2 on every deliverable (Spec S07.8)", len(r.Axis1), r.Axis2)
	}

	// The disclosure is note-class, and pack absence blocks nothing [R4].
	posture := 0
	for _, fd := range r.Findings {
		if fd.Severity == verify.SeverityBlocker {
			t.Fatalf("bootstrap round minted a blocker finding %+v — pack absence must never force REVISE to the cap", fd)
		}
		if strings.Contains(fd.Text, "restores the full ladder") {
			posture++
		}
	}
	if posture == 0 {
		t.Fatalf("no note-class posture disclosure among the round findings: %+v", r.Findings)
	}

	// The advisory verdict flips nothing to verified [R12 / OQ2].
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("advisory SHIP verified %v — SetVerified stays the only path to verified and no authoritative verdict exists (Spec S05.1/S07.11)", out.VerifiedItems)
	}
	doc, found, err := f.ledger.Current(ctx, "t1")
	if err != nil || !found {
		t.Fatalf("ledger doc: %v found=%v", err, found)
	}
	for _, item := range doc.State.Items {
		if item.Status != ledger.StatusDoneUnverified {
			t.Fatalf("work item %s is %q, want done_unverified under an advisory verdict", item.ID, item.Status)
		}
	}
}

// TestGF4BootstrapNeverParksOnPackAbsence [R4, R7]: the drain still terminates
// in a card when the WORK is bad — but the cause is the blocker, never the
// absent pack, and the card names the posture in plain words.
func TestGF4BootstrapNeverParksOnPackAbsence(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{compliance: blockerOn("AC-1", "main.go:3", "the demo panics on empty input")}
	v := f.verifier(j, nil, bootstrapPack())
	v.Settings = testSettings{base: f.reg, ints: map[string]int64{"verification.rework_rounds": 2}}
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.Content += "// another attempt\n"
		return d, nil
	}

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Card == nil {
		t.Fatal("a persistent blocker under bootstrap must still reach the CAP-HIT card")
	}
	if out.Card.Category != verify.CatCapHit {
		t.Fatalf("card category %q, want CAP-HIT (the blocker's own terminal)", out.Card.Category)
	}
	if out.Card.Infrastructure {
		t.Fatalf("the drain parked on the infrastructure card: %+v — pack absence is no longer a refusal", out.Card)
	}
	if !strings.Contains(out.Card.Summary, "rework stopped without SHIP") {
		t.Fatalf("card summary %q does not name the blocker's own cause", out.Card.Summary)
	}
	detail := strings.Join(out.Card.Detail, "\n")
	if !strings.Contains(detail, "restores the full ladder") {
		t.Fatalf("card detail %q does not name the bootstrap posture in plain words (Spec S07.8)", detail)
	}
	for _, r := range out.Rounds {
		for _, fd := range r.Findings {
			if fd.Severity == verify.SeverityBlocker && strings.Contains(fd.Text, "restores the full ladder") {
				t.Fatalf("round %d raised the posture as a blocker: %+v", r.Round, fd)
			}
		}
	}
}

// TestGF4PostureRecomputedPerRevision [R8]: the posture is resolved fresh for
// every judged round from the registry's CURRENT capture, so commands captured
// mid-drain restore the executable ladder on the next revision with no
// bootstrap residue.
func TestGF4PostureRecomputedPerRevision(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
		if in.Round == 1 {
			return blockerOn("AC-1", "main.go:3", "the demo panics on empty input")(in)
		}
		return passAll(in), nil
	}}
	runner := &scriptRunner{}
	v := f.verifier(j, runner, nil)
	resolutions := 0
	v.ResolvePack = func(context.Context) (*verify.CheckPack, error) {
		resolutions++
		if resolutions == 1 {
			return bootstrapPack(), nil
		}
		return passPack(), nil
	}
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.Content += "// fixed per F1\n"
		return d, nil
	}

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) != 2 {
		t.Fatalf("rounds = %d, want 2 (round 1 bootstrap, round 2 on the captured pack)", len(out.Rounds))
	}
	if out.Rounds[0].Posture != verify.PostureBootstrap {
		t.Fatalf("round 1 posture %q, want bootstrap", out.Rounds[0].Posture)
	}
	if out.Rounds[1].Posture != "" {
		t.Fatalf("round 2 posture %q — once the capture holds a command the advisory marking DROPS, with no residue", out.Rounds[1].Posture)
	}
	if out.Rounds[1].PostureNote != "" || out.Rounds[1].ReviewMandatory {
		t.Fatalf("round 2 still carries bootstrap residue: note=%q mandatory=%v", out.Rounds[1].PostureNote, out.Rounds[1].ReviewMandatory)
	}
	if out.Rounds[1].V1 == nil || len(out.Rounds[1].V1.Checks) == 0 {
		t.Fatal("round 2 ran no executable rung — the captured pack must run for real")
	}
	for _, c := range out.Rounds[1].V1.Checks {
		if c.State != verify.CheckPassed && c.State != verify.CheckFailed {
			t.Fatalf("round-2 check %q state %q, want a mechanical PASS/FAIL from the runner", c.CheckID, c.State)
		}
	}
	for _, fd := range out.Rounds[1].Findings {
		if strings.Contains(fd.Text, "restores the full ladder") {
			t.Fatalf("round 2 still carries the bootstrap note: %+v", fd)
		}
	}
	if len(runner.calls) == 0 {
		t.Fatal("the runner was never called — round 2 did not execute the ladder")
	}
}

// TestGF4WiringDefectStillRefuses [R9]: bootstrap is COMPUTED, never a
// default. A launch domain whose pack machinery is simply not wired has no
// registry answer to compute from, so it keeps the loud refusal — the company
// TestLaunchDomainRequiresPack keeps.
func TestGF4WiringDefectStillRefuses(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, nil) // no Pack, no ResolvePack
	_, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err == nil {
		t.Fatal("an unwired pack seam silently degraded a launch domain")
	}
	if !errors.Is(err, verify.ErrNoCheckPack) {
		t.Fatalf("err = %v, want the ErrNoCheckPack class", err)
	}
	if _, outage := verify.AsPreambleRefusal(err); !outage {
		t.Fatalf("err = %v, want the preamble-refusal class that becomes the operator's card", err)
	}
}

// TestGF4VerdictRowCarriesThePostureBothDirections [R5, R10]: the keep-forever
// verdict row is written for a bootstrap round with its posture members, and a
// full-posture round's row omits them.
func TestGF4VerdictRowCarriesThePostureBothDirections(t *testing.T) {
	ctx := context.Background()

	bootstrapFix := newFix(t)
	bootstrapFix.seedTask("t1", "r1")
	bv := bootstrapFix.verifier(&fakeJudge{}, nil, bootstrapPack())
	if _, err := bv.Verify(ctx, input(deliverable("t1", "r1"))); err != nil {
		t.Fatalf("bootstrap Verify: %v", err)
	}
	payload := lastRoundPayload(t, bootstrapFix)
	if payload.Posture != string(verify.PostureBootstrap) {
		t.Fatalf("verdict.recorded posture = %q, want %q", payload.Posture, verify.PostureBootstrap)
	}
	if !strings.Contains(payload.PostureNote, "restores the full ladder") {
		t.Fatalf("verdict.recorded posture note %q does not carry the plain-words statement", payload.PostureNote)
	}
	if !payload.ReviewMandatory {
		t.Fatal("verdict.recorded does not carry the mandatory-review fact")
	}
	if payload.Retention != "keep-forever" {
		t.Fatalf("retention %q, want keep-forever (G2 Def.11)", payload.Retention)
	}

	fullFix := newFix(t)
	fullFix.seedTask("t1", "r1")
	fv := fullFix.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	if _, err := fv.Verify(ctx, input(deliverable("t1", "r1"))); err != nil {
		t.Fatalf("full-posture Verify: %v", err)
	}
	full := lastRoundPayload(t, fullFix)
	if full.Posture != "" || full.PostureNote != "" || full.ReviewMandatory {
		t.Fatalf("a full-posture round carries bootstrap members: %+v — the members are omitempty in both directions", full)
	}
}

// roundRow is the subset of the verdict.recorded payload this packet adds.
type roundRow struct {
	Posture         string `json:"posture"`
	PostureNote     string `json:"posture_note"`
	ReviewMandatory bool   `json:"review_mandatory"`
	Retention       string `json:"retention"`
}

func lastRoundPayload(t *testing.T, f *fix) roundRow {
	t.Helper()
	rows := f.events(verify.EventRound)
	if len(rows) == 0 {
		t.Fatal("no verdict.recorded row (Spec S07.11: every verdict is recorded)")
	}
	var out roundRow
	if err := json.Unmarshal(rows[len(rows)-1].Payload, &out); err != nil {
		t.Fatalf("decode verdict.recorded: %v", err)
	}
	return out
}
