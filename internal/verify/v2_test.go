package verify_test

import (
	"context"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.5 acceptance: the judge input slice is exactly the ratified set via
// clean-context assembly (never the transcript, never the overlay), axis 1
// is per-AC binary with extractive evidence and an Unknown escape, V1
// outcomes are consumed as evidence (disagreement → CHECK-INTEGRITY, never
// override), axis 2 is separately prompted, and the verdict enum composes
// per the ratified rules.

func TestJudgeInputSliceCleanContext(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	ctx := context.Background()

	// Pollute the ledger with executor-frame material that must NOT reach
	// the judge: learned notes and decisions.
	verbs := f.ledger.SessionVerbs("r1", "execute", 0)
	if _, err := verbs.Note(ctx, "executor-only insight"); err != nil {
		t.Fatalf("Note: %v", err)
	}
	if _, err := verbs.Decide(ctx, "picked approach A", "cheaper", 0); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	d := deliverable("t1", "r1")
	d.Diff = "+ println(\"hello\")"
	v1 := &verify.V1Result{PackVersion: 1, Checks: []verify.CheckOutcome{{CheckID: "ac2", ACKey: "AC-2", State: verify.CheckPassed}}}
	prior := []verify.Finding{{N: 1, Severity: verify.SeverityBlocker, Category: verify.CatACBlocker, Criterion: "AC-1", Anchor: "main.go:1", Text: "prior", Round: 1}}

	in, err := verify.BuildJudgeInput(ctx, f.ledger, d, verify.SeedSoftwareRubric(), v1, prior, 2)
	if err != nil {
		t.Fatalf("BuildJudgeInput: %v", err)
	}

	// The clean brief: objective_ac only from the ledger, plus the S07.5
	// Extra items — nothing from the executor's frame.
	ids := map[string]bool{}
	for _, e := range in.Brief.Manifest {
		ids[e.ItemID] = true
		if e.PrecedenceLabel == string(ledger.PrecedenceUser) {
			t.Fatalf("user-overlay item %q reached a verification brief", e.ItemID)
		}
	}
	for _, want := range []string{"ledger/objective_ac", "verify/artifact", "verify/diff", "verify/rubric", "verify/v1-outcomes", "verify/prior-findings"} {
		if !ids[want] {
			t.Fatalf("judge slice misses %q; manifest %v", want, ids)
		}
	}
	for _, banned := range []string{"ledger/learned_this_task", "ledger/decisions", "ledger/state"} {
		if ids[banned] {
			t.Fatalf("executor-frame item %q leaked into the judge slice", banned)
		}
	}
	if !in.Brief.Clean {
		t.Fatal("judge brief not assembled in clean mode (Spec S05.4 exception)")
	}
	if strings.Contains(in.BriefText, "executor-only insight") {
		t.Fatal("learned_this_task content leaked into the judge prompt")
	}
	if len(in.ACs) != 2 || in.ACs[1].Structured == "" {
		t.Fatalf("frozen ACs not carried: %+v", in.ACs)
	}
	// One manifest event per assembly (Spec S05.4).
	if in.Brief.ManifestEventSeq == 0 {
		t.Fatal("assembly manifest event missing")
	}
}

func TestValidateAxis1(t *testing.T) {
	acs := []ledger.AcceptanceCriterion{
		{N: 1, Plain: "runs"},
		{N: 2, Plain: "greets", Structured: "stdout contains hello"},
		{N: 3, Plain: "documented"},
	}
	artifact := "the artifact body with real evidence inside"

	res := verify.Axis1Result{Verdicts: []verify.ACVerdict{
		{Key: "AC-1", Pass: true, Evidence: "real evidence"}, // extractive → stands
		{Key: "AC-2", Pass: false, Evidence: "says goodbye"}, // V1 says PASS → disagreement
		// AC-3 missing → forced Unknown.
	}}
	v1 := map[string]verify.CheckOutcomeState{"AC-2": verify.CheckPassed}
	verdicts, integrity := verify.ValidateAxis1(res, acs, v1, artifact)

	byKey := map[string]verify.ACVerdict{}
	for _, v := range verdicts {
		byKey[v.Key] = v
	}
	if !byKey["AC-1"].Pass || byKey["AC-1"].BoundTo != "plain" {
		t.Fatalf("AC-1: %+v", byKey["AC-1"])
	}
	// The mechanical fact stands; the disagreement is CHECK-INTEGRITY,
	// never an override of V1 by the judge.
	if !byKey["AC-2"].Pass || !byKey["AC-2"].FromV1 || !byKey["AC-2"].Disagreement {
		t.Fatalf("AC-2 must carry the V1 fact + disagreement: %+v", byKey["AC-2"])
	}
	if byKey["AC-2"].BoundTo != "structured" {
		t.Fatalf("AC-2 must bind to the structured sub-line: %+v", byKey["AC-2"])
	}
	if len(integrity) != 1 || integrity[0].Category != verify.CatCheckIntegrity {
		t.Fatalf("disagreement finding: %+v", integrity)
	}
	if !byKey["AC-3"].Unknown || byKey["AC-3"].Forced == "" {
		t.Fatalf("missing verdict must force Unknown, recorded: %+v", byKey["AC-3"])
	}
}

func TestValidateAxis1NonExtractiveEvidence(t *testing.T) {
	acs := []ledger.AcceptanceCriterion{{N: 1, Plain: "runs"}}
	res := verify.Axis1Result{Verdicts: []verify.ACVerdict{
		{Key: "AC-1", Pass: true, Evidence: "fabricated quote not in the artifact"},
	}}
	verdicts, _ := verify.ValidateAxis1(res, acs, nil, "the actual artifact")
	if !verdicts[0].Unknown || verdicts[0].Forced != "unknown: non-extractive evidence" {
		t.Fatalf("non-extractive PASS must force Unknown (Spec S07.10): %+v", verdicts[0])
	}
}

func TestComputeVerdictMatrix(t *testing.T) {
	blocker := verify.Finding{Severity: verify.SeverityBlocker, Criterion: "AC-1"}
	note := verify.Finding{Severity: verify.SeverityNote}
	cases := []struct {
		name     string
		ax2      *verify.Axis2Result
		findings []verify.Finding
		ax1Esc   string
		want     verify.Verdict
	}{
		{"clean ships", &verify.Axis2Result{}, nil, "", verify.VerdictShip},
		{"notes ship with notes", &verify.Axis2Result{}, []verify.Finding{note}, "", verify.VerdictShipWithNotes},
		{"blockers revise", &verify.Axis2Result{}, []verify.Finding{blocker, note}, "", verify.VerdictRevise},
		{"reopen-spec dominates", &verify.Axis2Result{ReopenSpec: "spec is contradictory"}, []verify.Finding{blocker}, "", verify.VerdictReopenSpec},
		{"axis1 escalate", &verify.Axis2Result{}, []verify.Finding{blocker}, "systemic", verify.VerdictEscalate},
		{"axis2 escalate", &verify.Axis2Result{Escalate: "beyond rework"}, nil, "", verify.VerdictEscalate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := verify.ComputeVerdict(tc.ax2, tc.findings, tc.ax1Esc); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestAxis2SeparatelyPromptedAndTrivialBandVerified(t *testing.T) {
	// Launch domain, TRIVIAL tier: the zero-interaction band skips
	// ceremony, never verification — V0–V2 run like everything else and
	// axis 2 runs on every launch-domain deliverable (Spec S07.8).
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	in := input(deliverable("t1", "r1"))
	in.Tier = intake.TierTrivial
	out, err := v.Verify(context.Background(), in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s", out.Verdict)
	}
	if j.complianceCalls != 1 || j.sanityCalls != 1 {
		t.Fatalf("dual-axis calls: compliance=%d sanity=%d (want 1/1 — separately prompted, ≤ rounds × 2)", j.complianceCalls, j.sanityCalls)
	}
}

func TestAxis2StakesGating(t *testing.T) {
	// Outside launch domains axis 2 is stakes-gated at
	// ⚙ verification.sanity_stakes_floor (default standard); steered
	// output never skips it (4.7 backstop).
	f := newFix(t)
	f.seedTask("t1", "r1")

	run := func(domain string, tier intake.Tier, steered bool) (*fakeJudge, verify.Outcome) {
		j := &fakeJudge{}
		v := f.verifier(j, nil, nil) // no pack: non-launch degraded V1 (S07.8)
		in := input(deliverable("t1", "r1"))
		in.Deliverable.Domain = domain
		in.Deliverable.Steered = steered
		in.Tier = tier
		out, err := v.Verify(context.Background(), in)
		if err != nil {
			t.Fatalf("Verify(%s,%s): %v", domain, tier, err)
		}
		return j, out
	}

	j, out := run("prose", intake.TierLow, false)
	if j.sanityCalls != 0 {
		t.Fatal("low-tier non-launch deliverable ran axis 2 below the floor")
	}
	if out.Rounds[0].Axis2Skipped == "" {
		t.Fatal("axis-2 skip not recorded (never silent)")
	}
	if j.complianceCalls != 1 {
		t.Fatal("axis 1 must run everywhere (V2 is universal)")
	}

	j, _ = run("prose", intake.TierHigh, false)
	if j.sanityCalls != 1 {
		t.Fatal("high-tier deliverable skipped axis 2")
	}

	j, _ = run("prose", intake.TierLow, true)
	if j.sanityCalls != 1 {
		t.Fatal("steered output skipped axis 2 (4.7 backstop violated)")
	}
}

func TestBlockerWithoutCitationDemoted(t *testing.T) {
	// A blocker citing no frozen criterion can only be a note — demoted and
	// recorded, keeping goalposts structurally fixed (Spec S07.5).
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			res := passAll(in)
			res.Findings = []verify.Finding{{
				Severity: verify.SeverityBlocker, Category: verify.CatACBlocker,
				Criterion: "", Anchor: "main.go:9", Text: "I just don't like it",
			}}
			return res, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	// Demoted to note → no rework round; ships with notes.
	if out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s, want SHIP-with-notes", out.Verdict)
	}
	fs := out.Rounds[0].Findings
	if len(fs) != 1 || fs[0].Severity != verify.SeverityNote || !fs[0].Demoted {
		t.Fatalf("uncited blocker not demoted+recorded: %+v", fs)
	}
}

func TestReopenSpecCard(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		sanity: func(verify.JudgeInput) (verify.Axis2Result, error) {
			r := cleanSanity()
			r.ReopenSpec = "the spec stores plaintext passwords; compliance would be harmful"
			return r, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictReopenSpec || out.Card == nil {
		t.Fatalf("want REOPEN-SPEC card, got %s %v", out.Verdict, out.Card)
	}
	// The ratified three choices; the judge never edits the spec (D10).
	want := []string{"proceed_as_specified", "adjust_specification", "rethink"}
	if len(out.Card.Choices) != 3 {
		t.Fatalf("choices %v", out.Card.Choices)
	}
	for i, c := range want {
		if out.Card.Choices[i] != c {
			t.Fatalf("choices %v, want %v", out.Card.Choices, want)
		}
	}
	// No ledger item was verified on this path.
	if len(out.VerifiedItems) != 0 {
		t.Fatal("REOPEN-SPEC must not verify items")
	}
}
