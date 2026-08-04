package verify_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.6 acceptance: the exact retry package, fresh session per round,
// findings carried verbatim with anchors, finding keys as the convergence
// signal, re-review against the ORIGINAL criteria with note suppression
// after round 1, and the stop rules terminating in an ESCALATE card with
// the full round history.

// blockerOn returns a compliance script failing one AC with a blocker at a
// fixed anchor.
func blockerOn(criterion, anchor, text string) func(verify.JudgeInput) (verify.Axis1Result, error) {
	return func(in verify.JudgeInput) (verify.Axis1Result, error) {
		res := passAll(in)
		for i := range res.Verdicts {
			if res.Verdicts[i].Key == criterion {
				res.Verdicts[i].Pass = false
				res.Verdicts[i].Evidence = ""
			}
		}
		res.Findings = []verify.Finding{{
			Severity: verify.SeverityBlocker, Category: verify.CatACBlocker,
			Criterion: criterion, Anchor: anchor, Text: text,
		}}
		return res, nil
	}
}

func TestRetryPackageExactAndFreshRounds(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")

	rounds := 0
	j := &fakeJudge{}
	j.compliance = func(in verify.JudgeInput) (verify.Axis1Result, error) {
		if in.Round == 1 {
			return blockerOn("AC-1", "main.go:3", "the demo panics on empty input")(in)
		}
		return passAll(in), nil
	}

	var pkgs []verify.RetryPackage
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		rounds++
		pkgs = append(pkgs, pkg)
		d := pkg.Deliverable
		d.Content = d.Content + "// fixed panic per F1\n"
		return d, nil
	}

	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip || rounds != 1 {
		t.Fatalf("verdict %s after %d revisions", out.Verdict, rounds)
	}

	// The retry package is EXACTLY: SPEC + frozen ACs + numbered findings
	// with anchors + the deliverable (Spec S07.6).
	pkg := pkgs[0]
	if pkg.Spec.Restatement == "" || pkg.Spec.Version != 1 {
		t.Fatalf("retry package without the original SPEC: %+v", pkg.Spec)
	}
	if len(pkg.ACs) != 2 {
		t.Fatalf("retry package without the frozen ACs: %+v", pkg.ACs)
	}
	if len(pkg.Findings) != 1 {
		t.Fatalf("retry package findings: %+v", pkg.Findings)
	}
	fd := pkg.Findings[0]
	if fd.N != 1 || fd.Text != "the demo panics on empty input" || fd.Anchor != "main.go:3" || fd.Criterion != "AC-1" {
		t.Fatalf("finding not carried verbatim+numbered+anchored: %+v", fd)
	}
	if pkg.Deliverable.Content == "" || pkg.Round != 2 {
		t.Fatalf("retry package deliverable/round: rev=%d round=%d", pkg.Deliverable.Revision, pkg.Round)
	}

	// The round-2 judge saw the prior findings verbatim (re-review with
	// prior findings in scope) and the SAME frozen criteria + rubric.
	in2 := j.inputs[1]
	if in2.Round != 2 || len(in2.PriorFindings) != 1 || in2.PriorFindings[0].Text != fd.Text {
		t.Fatalf("round-2 judge input: %+v", in2.PriorFindings)
	}
	if in2.RubricID != j.inputs[0].RubricID || in2.RubricVersion != j.inputs[0].RubricVersion {
		t.Fatal("rubric version drifted between rounds (original-criteria rule)")
	}
	if len(in2.ACs) != len(j.inputs[0].ACs) {
		t.Fatal("frozen AC set drifted between rounds")
	}
}

func TestNotesNeverTriggerRounds(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			res := passAll(in)
			res.Findings = []verify.Finding{{
				Severity: verify.SeverityNote, Category: verify.CatSanityBlocker,
				Criterion: "AC-1", Anchor: "main.go:1", Text: "consider a doc comment",
			}}
			return res, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	revised := false
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		revised = true
		return pkg.Deliverable, nil
	}
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShipWithNotes || revised {
		t.Fatalf("notes spun a loop: verdict=%s revised=%v", out.Verdict, revised)
	}
	if j.complianceCalls != 1 {
		t.Fatalf("compliance calls %d", j.complianceCalls)
	}
}

func TestNoteSuppressionAfterRoundOne(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	j.compliance = func(in verify.JudgeInput) (verify.Axis1Result, error) {
		res, _ := blockerOn("AC-1", "main.go:3", "panics")(in)
		if in.Round > 1 {
			// Round 2: same blocker resolved? No — new NOTE with a NEW key
			// plus the resolved blocker gone; the fresh note must be
			// suppressed to a count (goalpost-drift suppression).
			res = passAll(in)
			res.Findings = []verify.Finding{{
				Severity: verify.SeverityNote, Category: verify.CatSanityBlocker,
				Criterion: "AC-2", Anchor: "main.go:99", Text: "brand-new nitpick",
			}}
		}
		return res, nil
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.Content += "// fix\n"
		return d, nil
	}
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	last := out.Rounds[len(out.Rounds)-1]
	if last.SuppressedNotes != 1 {
		t.Fatalf("new round-2 note not suppressed to a count: %+v", last)
	}
	if len(last.Findings) != 0 {
		t.Fatalf("suppressed note still itemized: %+v", last.Findings)
	}
	// Suppressed notes still surface the with-notes verdict.
	if out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s", out.Verdict)
	}
}

func TestCapHitCardWithRoundHistory(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	// A different blocker key each round and substantially different
	// content each round: no convergence — the HARD CAP is what stops it.
	j := &fakeJudge{}
	j.compliance = func(in verify.JudgeInput) (verify.Axis1Result, error) {
		return blockerOn("AC-1", fmt.Sprintf("main.go:%d", in.Round), fmt.Sprintf("distinct problem %d", in.Round))(in)
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	rounds := 0
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		rounds++
		d := pkg.Deliverable
		d.Content = strings.Repeat(fmt.Sprintf("wholly rewritten attempt %d\nline-%d\n", rounds, rounds), 5+rounds)
		return d, nil
	}
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	// ⚙ verification.rework_rounds = 3: three judged rounds, then the card.
	if len(out.Rounds) != 3 {
		t.Fatalf("judged rounds %d, want 3 (⚙ default)", len(out.Rounds))
	}
	if j.complianceCalls != 3 {
		t.Fatalf("judge calls %d — the cost bound is rounds × 2", j.complianceCalls)
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("want CAP-HIT card, got %+v", out.Card)
	}
	if len(out.Card.Rounds) != 3 || out.Card.BestEffort == "" {
		t.Fatalf("card must carry the FULL round history + best-effort state: rounds=%d best=%q",
			len(out.Card.Rounds), out.Card.BestEffort)
	}
	if !strings.Contains(out.Card.Summary, "verification.rework_rounds") {
		t.Fatalf("card summary must name the stop rule: %q", out.Card.Summary)
	}
}

func TestConvergenceByFindingKeys(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	// Cap raised to 5 so convergence (patience 2) fires FIRST: the same
	// finding key recurs unresolved.
	j := &fakeJudge{compliance: blockerOn("AC-1", "main.go:3", "still panics")}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Settings = testSettings{base: f.reg, ints: map[string]int64{"verification.rework_rounds": 5}}
	rounds := 0
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		rounds++
		d := pkg.Deliverable
		d.Content = strings.Repeat(fmt.Sprintf("attempt %d rewritten from scratch\nblock-%d\n", rounds, rounds), 5+rounds)
		return d, nil
	}
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	// Rounds 2 and 3 repeat round 1's key → recurrence hits patience 2 at
	// round 3 — well before the cap of 5.
	if len(out.Rounds) != 3 {
		t.Fatalf("rounds %d, want 3 (convergence before cap)", len(out.Rounds))
	}
	if out.Card == nil || !strings.Contains(out.Card.Summary, "finding keys") {
		t.Fatalf("convergence card: %+v", out.Card)
	}
}

func TestConvergenceByArtifactSimilarity(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	// Different finding keys each round (no key recurrence) but the
	// executor only makes cosmetic edits: similarity convergence fires.
	j := &fakeJudge{}
	j.compliance = func(in verify.JudgeInput) (verify.Axis1Result, error) {
		return blockerOn("AC-1", fmt.Sprintf("f%d", in.Round), fmt.Sprintf("problem %d", in.Round))(in)
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Settings = testSettings{base: f.reg, ints: map[string]int64{"verification.rework_rounds": 5}}
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "stable line %d of the artifact\n", i)
	}
	base := b.String()
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.Content = base + fmt.Sprintf("// cosmetic tweak %d\n", pkg.Round)
		return d, nil
	}
	in := input(verify.Deliverable{
		TaskID: "t1", RunID: "r1", Domain: verify.DomainSoftware, Type: "code",
		Revision: 1, Content: base,
	})
	out, err := v.Verify(context.Background(), in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Card == nil || !strings.Contains(out.Card.Summary, "similarity") {
		t.Fatalf("similarity convergence card: %+v", out.Card)
	}
	if len(out.Rounds) >= 5 {
		t.Fatalf("similarity convergence never fired; rounds=%d", len(out.Rounds))
	}
}

func TestRequesterCommentsEnterTheChannel(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{} // judge is clean; the requester's contested point drives the round
	v := f.verifier(j, &scriptRunner{}, passPack())
	var pkg verify.RetryPackage
	v.Revise = func(_ context.Context, p verify.RetryPackage) (verify.Deliverable, error) {
		pkg = p
		d := p.Deliverable
		d.Content += "// addressed requester point\n"
		return d, nil
	}
	in := input(deliverable("t1", "r1"))
	in.Comments = []verify.RequesterComment{{
		Text: "the greeting must be in German", Anchor: "main.go:2", Criterion: "AC-2", Blocking: true,
	}}
	out, err := v.Verify(context.Background(), in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s", out.Verdict)
	}
	if len(pkg.Findings) != 1 || !pkg.Findings[0].Requester || pkg.Findings[0].Anchor != "main.go:2" {
		t.Fatalf("requester comment did not enter the numbered-anchored channel: %+v", pkg.Findings)
	}
}

func TestReviseSeamMissingEscalates(t *testing.T) {
	// REVISE with no executor seam: never silent — the drain terminates in
	// a card carrying best-effort state.
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{compliance: blockerOn("AC-1", "main.go:3", "panics")}
	v := f.verifier(j, &scriptRunner{}, passPack())
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("want CAP-HIT card when rework is unavailable, got %+v", out.Card)
	}
	if !strings.Contains(out.Card.Summary, "rework unavailable") {
		t.Fatalf("card summary: %q", out.Card.Summary)
	}
}
