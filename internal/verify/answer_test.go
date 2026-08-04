package verify_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// The S07.7 answer contract + resume-in-place battery (B2-5): the three
// verbs' validation, the best-effort pin, and the drain re-entry semantics
// — guidance into a FRESH bounded round through the exact retry package,
// caps/convergence still binding, a recurrent stop re-parking with the full
// round history. Scripted judge/revise; the real DB/event log/ledger.

func TestDecodeAndValidateAnswer(t *testing.T) {
	capHit := verify.Card{
		Category: verify.CatCapHit,
		Choices:  []string{"accept_best_effort", "revise_with_guidance", "cancel"},
	}
	reopen := verify.Card{
		Category: verify.CatReopenSpec,
		Choices:  []string{"proceed_as_specified", "adjust_specification", "rethink"},
	}
	integrity := verify.Card{
		Category: verify.CatCheckIntegrity,
		Choices:  []string{"fix_suite", "waive_check", "cancel"},
	}
	cases := []struct {
		name    string
		card    verify.Card
		raw     string
		wantErr error
	}{
		{"accept on cap-hit ok", capHit, `{"choice":"accept_best_effort"}`, nil},
		{"cancel on cap-hit ok", capHit, `{"choice":"cancel","note":"n"}`, nil},
		{"revise with guidance ok", capHit, `{"choice":"revise_with_guidance","guidance":[{"text":"fix the ending","criterion":"AC-1"}]}`, nil},
		{"missing choice", capHit, `{}`, verify.ErrBadAnswer},
		{"unknown verb", capHit, `{"choice":"approve"}`, verify.ErrUnsupportedAnswer},
		{"revise without guidance", capHit, `{"choice":"revise_with_guidance"}`, verify.ErrBadAnswer},
		{"revise with empty guidance text", capHit, `{"choice":"revise_with_guidance","guidance":[{"text":"  "}]}`, verify.ErrBadAnswer},
		{"reopen-spec card verbs not implemented", reopen, `{"choice":"accept_best_effort"}`, verify.ErrUnsupportedAnswer},
		{"check-integrity cancel not implemented", integrity, `{"choice":"cancel"}`, verify.ErrUnsupportedAnswer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ans, err := verify.DecodeAnswer(json.RawMessage(tc.raw))
			if err == nil {
				err = verify.ValidateAnswer(tc.card, ans)
			}
			if !errors.Is(err, tc.wantErr) && (tc.wantErr != nil || err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestIsVerifyAskID(t *testing.T) {
	for id, want := range map[string]bool{
		"ask-verify-1fac3a3fb7b9c842": true,
		"canary-00aa":                 true,
		"intake:t-1:3":                false,
		"":                            false,
	} {
		if got := verify.IsVerifyAskID(id); got != want {
			t.Errorf("IsVerifyAskID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestBestEffortPin(t *testing.T) {
	if _, _, err := (verify.Card{}).BestEffortPin(); !errors.Is(err, verify.ErrNotResumable) {
		t.Fatalf("no rounds: err = %v, want ErrNotResumable", err)
	}
	legacy := verify.Card{Rounds: []verify.RoundRecord{{Round: 1, ContentSHA: "aa"}}}
	if _, _, err := legacy.BestEffortPin(); !errors.Is(err, verify.ErrNotResumable) {
		t.Fatalf("legacy card without revision pin: err = %v, want ErrNotResumable", err)
	}
	good := verify.Card{Rounds: []verify.RoundRecord{{Round: 1, Revision: 1, ContentSHA: "aa"}, {Round: 2, Revision: 3, ContentSHA: "bb"}}}
	rev, sha, err := good.BestEffortPin()
	if err != nil || rev != 3 || sha != "bb" {
		t.Fatalf("pin = (%d, %s, %v), want (3, bb, nil)", rev, sha, err)
	}
}

// ---- resume harness ----

const resumeBase = "line-one\nline-two\n"

// failingJudge fails AC-1 with a stable blocker finding key unless the
// artifact carries the guidance marker (and unless alwaysFail).
func failingJudge(alwaysFail bool) *fakeJudge {
	return &fakeJudge{compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
		pass := !alwaysFail && strings.Contains(in.Artifact, "GUIDANCE-APPLIED")
		var res verify.Axis1Result
		for _, ac := range in.ACs {
			v := verify.ACVerdict{Key: acKey(ac.N), Pass: true, Evidence: "line-one"}
			if ac.N == 1 && !pass {
				v = verify.ACVerdict{Key: v.Key, Pass: false}
			}
			res.Verdicts = append(res.Verdicts, v)
		}
		if !pass {
			res.Findings = []verify.Finding{{
				Severity: verify.SeverityBlocker, Category: verify.CatACBlocker,
				Criterion: "AC-1", Anchor: "sec:1", Text: "AC-1 is not met by this revision",
			}}
		}
		return res, nil
	}}
}

// scriptedRevise reworks by rebuilding from the base: a fresh non-cumulative
// revision, applying the guidance marker when (and only when) the retry
// package carries requester findings. touch keeps revisions byte-distinct.
func scriptedRevise(pkgs *[]verify.RetryPackage) func(context.Context, verify.RetryPackage) (verify.Deliverable, error) {
	return func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		*pkgs = append(*pkgs, pkg)
		content := resumeBase
		for _, f := range pkg.Findings {
			if f.Requester {
				content += "GUIDANCE-APPLIED\n"
				break
			}
		}
		content += "touch-r" + acKey(pkg.Round) + "\n"
		d := pkg.Deliverable
		d.Content = content
		d.Revision = pkg.Deliverable.Revision + 1
		return d, nil
	}
}

// driveToCapHit runs the fresh drain to its CAP-HIT card under cap 2.
func driveToCapHit(t *testing.T, f *fix, v *verify.Verifier) (verify.Outcome, verify.Card, verify.VerifyInput) {
	t.Helper()
	in := verify.VerifyInput{
		Deliverable: verify.Deliverable{
			TaskID: "t1", RunID: "t1.verify", Domain: "generic", Type: "markdown",
			Revision: 1, Content: resumeBase,
		},
		Spec: spec(), Steps: steps(), Tier: intake.TierStandard,
	}
	out, err := v.Verify(context.Background(), in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictEscalate || out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("fresh drain: verdict %s card %+v, want CAP-HIT", out.Verdict, out.Card)
	}
	return out, *out.Card, in
}

func resumeSettings(f *fix, rounds, patience int64) testSettings {
	return testSettings{base: f.reg, ints: map[string]int64{
		"verification.rework_rounds":               rounds,
		"verification.convergence_patience_rounds": patience,
	}}
}

func guidancePoints() []verify.RequesterComment {
	return verify.Answer{Guidance: []verify.GuidancePoint{
		{Text: "apply the agreed marker line", Criterion: "AC-1", Anchor: "sec:1"},
	}}.Comments()
}

func TestResumeGuidanceFeedsFreshRoundToShip(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "t1.verify")
	var pkgs []verify.RetryPackage
	judge := failingJudge(false)
	v := f.verifier(judge, nil, nil)
	v.Settings = resumeSettings(f, 2, 99)
	v.Revise = scriptedRevise(&pkgs)

	out, card, in := driveToCapHit(t, f, v)
	if len(card.Rounds) != 2 || card.Rounds[1].Revision != 2 {
		t.Fatalf("card rounds = %+v, want 2 rounds pinned at rev2", card.Rounds)
	}
	if len(pkgs) != 1 || pkgs[0].Round != 2 {
		t.Fatalf("fresh drain revise packages = %+v", pkgs)
	}

	// Resume with the pinned best-effort revision + guidance; the judge
	// passes once the marker lands.
	in.Deliverable = verify.Deliverable{
		TaskID: "t1", RunID: "t1.verify", Domain: "generic", Type: "markdown",
		Revision: 2, Content: resumeBase + "touch-rAC-2\n",
	}
	if in.Deliverable.SHA256() != card.Rounds[1].ContentSHA {
		t.Fatalf("test fixture drift: pinned content mismatch")
	}
	res, err := v.ResumeWithGuidance(context.Background(), in, card, guidancePoints())
	if err != nil {
		t.Fatalf("ResumeWithGuidance: %v", err)
	}
	if res.Verdict != verify.VerdictShip {
		t.Fatalf("resumed verdict = %s, want SHIP", res.Verdict)
	}
	// One coherent history: 2 carried + 1 resumed round, numbered 1..3.
	if len(res.Rounds) != 3 || res.Rounds[2].Round != 3 || res.Rounds[2].Revision != 3 {
		t.Fatalf("resumed rounds = %+v, want 3 rounds ending round 3 rev 3", res.Rounds)
	}
	if len(res.VerifiedItems) == 0 {
		t.Fatalf("SHIP after guidance verified no ledger items")
	}

	// The retry package was EXACTLY the ratified members with the guidance
	// as numbered requester blockers (Spec S07.6).
	if len(pkgs) != 2 {
		t.Fatalf("revise calls = %d, want 2 (fresh drain + guidance rework)", len(pkgs))
	}
	g := pkgs[1]
	if g.Round != 3 || g.Deliverable.SHA256() != card.Rounds[1].ContentSHA || len(g.ACs) != 2 {
		t.Fatalf("guidance package = round %d rev %d ACs %d", g.Round, g.Deliverable.Revision, len(g.ACs))
	}
	if len(g.Findings) != 1 || !g.Findings[0].Requester || g.Findings[0].N != 1 ||
		g.Findings[0].Severity != verify.SeverityBlocker || g.Findings[0].Criterion != "AC-1" {
		t.Fatalf("guidance findings = %+v, want one numbered requester blocker citing AC-1", g.Findings)
	}
	// The resumed judge pass carried the history AND the guidance in its
	// prior-findings scope (Spec S07.6 "prior findings in scope").
	last := judge.inputs[len(judge.inputs)-1]
	if last.Round != 3 {
		t.Fatalf("resumed judge round = %d, want 3", last.Round)
	}
	foundGuidance, foundHistory := false, false
	for _, pf := range last.PriorFindings {
		if pf.Requester && pf.Text == "apply the agreed marker line" {
			foundGuidance = true
		}
		if pf.Text == "AC-1 is not met by this revision" {
			foundHistory = true
		}
	}
	if !foundGuidance || !foundHistory {
		t.Fatalf("resumed prior findings missing guidance=%v history=%v: %+v", foundGuidance, foundHistory, last.PriorFindings)
	}
	// No second card: the original ask row is the only one.
	if got := len(f.openAsks()); got != 1 {
		t.Fatalf("open asks after SHIP resume = %d, want 1 (the answered card is closed by the stage layer)", got)
	}
	_ = out
}

func TestResumeRecurrentCapHitReparksWithFullHistory(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "t1.verify")
	var pkgs []verify.RetryPackage
	judge := failingJudge(true) // guidance cannot satisfy this judge
	v := f.verifier(judge, nil, nil)
	v.Settings = resumeSettings(f, 2, 99)
	v.Revise = scriptedRevise(&pkgs)

	_, card, in := driveToCapHit(t, f, v)

	in.Deliverable = verify.Deliverable{
		TaskID: "t1", RunID: "t1.verify", Domain: "generic", Type: "markdown",
		Revision: 2, Content: resumeBase + "touch-rAC-2\n",
	}
	res, err := v.ResumeWithGuidance(context.Background(), in, card, guidancePoints())
	if err != nil {
		t.Fatalf("ResumeWithGuidance: %v", err)
	}
	if res.Verdict != verify.VerdictEscalate || res.Card == nil || res.Card.Category != verify.CatCapHit {
		t.Fatalf("resumed verdict = %s card %+v, want a recurrent CAP-HIT", res.Verdict, res.Card)
	}
	// The fresh cap budget bound the resumed drain to 2 judged rounds; the
	// re-park card carries the FULL history, numbered continuously.
	if len(res.Card.Rounds) != 4 {
		t.Fatalf("re-park rounds = %d, want 4 (2 carried + 2 resumed)", len(res.Card.Rounds))
	}
	for i, r := range res.Card.Rounds {
		if r.Round != i+1 {
			t.Fatalf("round numbering broken at %d: %+v", i, res.Card.Rounds)
		}
	}
	if res.Card.Rounds[3].Revision != 4 {
		t.Fatalf("re-park best-effort pin = rev%d, want rev4", res.Card.Rounds[3].Revision)
	}
	// Two open asks now: the answered-at-stage-level original + the new one.
	if got := len(f.openAsks()); got != 2 {
		t.Fatalf("open asks = %d, want 2", got)
	}
	// Revise calls: fresh drain 1 + guidance 1 + resumed in-drain 1.
	if len(pkgs) != 3 {
		t.Fatalf("revise calls = %d, want 3", len(pkgs))
	}
}

func TestResumeConvergenceSeededAcrossThePark(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "t1.verify")
	var pkgs []verify.RetryPackage
	judge := failingJudge(true)
	v := f.verifier(judge, nil, nil)
	// patience 1 with a huge cap: only convergence can stop the drain.
	v.Settings = resumeSettings(f, 99, 1)
	v.Revise = scriptedRevise(&pkgs)

	in := verify.VerifyInput{
		Deliverable: verify.Deliverable{
			TaskID: "t1", RunID: "t1.verify", Domain: "generic", Type: "markdown",
			Revision: 1, Content: resumeBase,
		},
		Spec: spec(), Steps: steps(), Tier: intake.TierStandard,
	}
	out, err := v.Verify(context.Background(), in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Card == nil || len(out.Rounds) != 2 {
		t.Fatalf("fresh drain under patience 1: rounds = %d card %v, want 2 + CAP-HIT", len(out.Rounds), out.Card)
	}

	// Resume: the seeded convergence state (pre-park blocker keys) makes the
	// FIRST resumed round's recurrence trip the stop — cross-park inertia
	// still binds (Spec S07.6), without waiting for a second resumed round.
	in.Deliverable = verify.Deliverable{
		TaskID: "t1", RunID: "t1.verify", Domain: "generic", Type: "markdown",
		Revision: 2, Content: resumeBase + "touch-rAC-2\n",
	}
	res, err := v.ResumeWithGuidance(context.Background(), in, *out.Card, guidancePoints())
	if err != nil {
		t.Fatalf("ResumeWithGuidance: %v", err)
	}
	if res.Verdict != verify.VerdictEscalate || res.Card == nil {
		t.Fatalf("resumed verdict = %s, want ESCALATE on convergence", res.Verdict)
	}
	if len(res.Card.Rounds) != 3 {
		t.Fatalf("resumed rounds = %d, want exactly 3 (one resumed round tripped the seeded stop)", len(res.Card.Rounds))
	}
	if !strings.Contains(res.Card.Summary, "recur") {
		t.Fatalf("stop reason %q does not name the recurrence", res.Card.Summary)
	}
}

func TestResumeRejectsPinMismatchAndBadInputs(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "t1.verify")
	var pkgs []verify.RetryPackage
	v := f.verifier(failingJudge(true), nil, nil)
	v.Settings = resumeSettings(f, 2, 99)
	v.Revise = scriptedRevise(&pkgs)
	_, card, in := driveToCapHit(t, f, v)

	// Content that does not match the best-effort pin refuses to rework.
	in.Deliverable = verify.Deliverable{
		TaskID: "t1", RunID: "t1.verify", Domain: "generic", Type: "markdown",
		Revision: 2, Content: "tampered\n",
	}
	if _, err := v.ResumeWithGuidance(context.Background(), in, card, guidancePoints()); err == nil ||
		!strings.Contains(err.Error(), "pin") {
		t.Fatalf("pin mismatch err = %v, want a loud pin refusal", err)
	}
	// Unanswerable category.
	bad := card
	bad.Category = verify.CatReopenSpec
	in.Deliverable.Content = resumeBase + "touch-rAC-2\n"
	if _, err := v.ResumeWithGuidance(context.Background(), in, bad, guidancePoints()); !errors.Is(err, verify.ErrUnsupportedAnswer) {
		t.Fatalf("category err = %v, want ErrUnsupportedAnswer", err)
	}
	// No guidance.
	if _, err := v.ResumeWithGuidance(context.Background(), in, card, nil); !errors.Is(err, verify.ErrBadAnswer) {
		t.Fatalf("no-guidance err = %v, want ErrBadAnswer", err)
	}
	// No Revise seam.
	v.Revise = nil
	if _, err := v.ResumeWithGuidance(context.Background(), in, card, guidancePoints()); !errors.Is(err, verify.ErrSeamMissing) {
		t.Fatalf("no-seam err = %v, want ErrSeamMissing", err)
	}
}
