package verify_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// The verify ↔ S13 seam contract: minting at the handoff (one per round),
// findings landing minus suite defects, THE drain composing the package,
// and the resume path's guidance→drain order. The REAL review store rides
// the stage e2e; here a scripted sink pins the calling contract.

type sinkCall struct {
	op      string
	rev     int
	attempt string
	n       int // findings count
}

type fakeSink struct {
	calls []sinkCall
	// openFindings scripts what DrainOpen returns.
	openFindings []verify.Finding
}

func (s *fakeSink) MintCandidate(_ context.Context, d verify.Deliverable, round int) error {
	s.calls = append(s.calls, sinkCall{op: "mint", rev: d.Revision, attempt: verify.MintRef(d.RunID, round)})
	return nil
}

func (s *fakeSink) RecordFindings(_ context.Context, d verify.Deliverable, findings []verify.Finding) error {
	s.calls = append(s.calls, sinkCall{op: "findings", rev: d.Revision, n: len(findings)})
	for _, f := range findings {
		if f.Category == verify.CatCheckIntegrity {
			return fmt.Errorf("check-integrity finding reached the review stream: %+v", f)
		}
	}
	s.openFindings = append(s.openFindings, findings...)
	return nil
}

func (s *fakeSink) RecordVerdict(_ context.Context, d verify.Deliverable, seq int64) error {
	s.calls = append(s.calls, sinkCall{op: "verdict", rev: d.Revision, n: int(seq)})
	return nil
}

func (s *fakeSink) RecordGuidance(_ context.Context, d verify.Deliverable, author string, comments []verify.RequesterComment) error {
	s.calls = append(s.calls, sinkCall{op: "guidance", rev: d.Revision, n: len(comments)})
	for i, c := range comments {
		s.openFindings = append(s.openFindings, verify.Finding{
			N: len(s.openFindings) + i + 1, Severity: verify.SeverityBlocker,
			Category: verify.CatSanityBlocker, Criterion: c.Criterion,
			Anchor: c.Anchor, Text: c.Text, Requester: true,
		})
	}
	return nil
}

func (s *fakeSink) DrainOpen(_ context.Context, d verify.Deliverable, attemptRef string) ([]verify.Finding, error) {
	batch := make([]verify.Finding, len(s.openFindings))
	copy(batch, s.openFindings)
	for i := range batch {
		batch[i].N = i + 1
	}
	s.openFindings = nil
	s.calls = append(s.calls, sinkCall{op: "drain", rev: d.Revision, attempt: attemptRef, n: len(batch)})
	return batch, nil
}

func (s *fakeSink) ops(op string) []sinkCall {
	var out []sinkCall
	for _, c := range s.calls {
		if c.op == op {
			out = append(out, c)
		}
	}
	return out
}

// One REVISE round then SHIP: mint per round, findings landed per round,
// the drain composing the retry package with notes traveling.
func TestReviewSeamMintRecordDrain(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	round := 0
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			round++
			res := passAll(in)
			if round == 1 {
				res.Verdicts[0].Pass = false
				res.Verdicts[0].Evidence = ""
				res.Findings = []verify.Finding{
					{Severity: verify.SeverityBlocker, Category: verify.CatACBlocker,
						Criterion: "AC-1", Anchor: "deliverable.md:1", Text: "demo does not run"},
					{Severity: verify.SeverityNote, Category: verify.CatACBlocker,
						Criterion: "AC-1", Anchor: "", Text: "style could be tighter"},
				}
			}
			return res, nil
		},
	}
	sink := &fakeSink{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Review = sink
	var gotPkg *verify.RetryPackage
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		gotPkg = &pkg
		d := pkg.Deliverable
		d.PrevContent = d.Content
		d.Content = d.Content + "// fixed\n"
		d.Revision++
		return d, nil
	}

	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip && out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s", out.Verdict)
	}

	// One mint per round, candidate revisions 1 then 2 (Spec S13.1).
	mints := sink.ops("mint")
	if len(mints) != 2 || mints[0].rev != 1 || mints[1].rev != 2 ||
		mints[0].attempt != "r1#round-1" || mints[1].attempt != "r1#round-2" {
		t.Fatalf("mint per round: %+v", mints)
	}
	// The verdict ref pinned per judged round.
	if verdicts := sink.ops("verdict"); len(verdicts) != 2 {
		t.Fatalf("verdict refs: %+v", verdicts)
	}
	// Round 1's findings landed (blocker + note + the Unknown escape);
	// round 2 landed zero (clean).
	recs := sink.ops("findings")
	if len(recs) != 2 || recs[0].n == 0 {
		t.Fatalf("findings landings: %+v", recs)
	}

	// THE drain composed the package: attempt ref names the fed round; the
	// note TRAVELED (severity distinguishes, only blockers triggered the
	// round — the REVISE verdict preceded the drain).
	drains := sink.ops("drain")
	if len(drains) != 1 || drains[0].attempt != "r1#revise-r2" {
		t.Fatalf("drain calls: %+v", drains)
	}
	if gotPkg == nil || gotPkg.Round != 2 {
		t.Fatalf("package round: %+v", gotPkg)
	}
	var blockers, notes int
	for _, fd := range gotPkg.Findings {
		switch fd.Severity {
		case verify.SeverityBlocker:
			blockers++
		case verify.SeverityNote:
			notes++
		}
		if fd.Round != 2 {
			t.Fatalf("drained finding round stamp: %+v", fd)
		}
	}
	if blockers == 0 || notes == 0 {
		t.Fatalf("package severities (notes travel): %d blockers %d notes: %+v", blockers, notes, gotPkg.Findings)
	}
	// Numbering [F1..Fn] contiguous from the drain.
	for i, fd := range gotPkg.Findings {
		if fd.N != i+1 {
			t.Fatalf("package numbering: %+v", gotPkg.Findings)
		}
	}
}

// The resume path with the sink wired: the guidance lands as durable
// requester comments FIRST, then THE drain composes the package — the
// parked round's still-open findings and the guidance drain together
// through the one path (Spec S13.4 collects EVERY open comment; the B2-5
// guidance-only package was the pre-S13 in-memory shape).
func TestReviewSeamResumeGuidanceThenDrain(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "t1.verify")
	var pkgs []verify.RetryPackage
	judge := failingJudge(false)
	sink := &fakeSink{}
	v := f.verifier(judge, nil, nil)
	v.Settings = resumeSettings(f, 2, 99)
	v.Revise = scriptedRevise(&pkgs)
	v.Review = sink

	_, card, in := driveToCapHit(t, f, v)
	// The park left the final round's findings OPEN (no drain ran at the
	// card terminal) — the fake sink still holds them.
	if len(sink.openFindings) == 0 {
		t.Fatalf("parked round findings must stay open at the card")
	}

	in.Deliverable = verify.Deliverable{
		TaskID: "t1", RunID: "t1.verify", Domain: "generic", Type: "markdown",
		Revision: 2, Content: resumeBase + "touch-rAC-2\n",
	}
	res, err := v.ResumeWithGuidance(context.Background(), in, card, guidancePoints())
	if err != nil {
		t.Fatalf("ResumeWithGuidance: %v", err)
	}
	if res.Verdict != verify.VerdictShip {
		t.Fatalf("resumed verdict %s", res.Verdict)
	}

	// Order: guidance recorded, THEN one drain for the resumed attempt.
	var seq []string
	for _, c := range sink.calls {
		if c.op == "guidance" || c.op == "drain" {
			seq = append(seq, fmt.Sprintf("%s@%s", c.op, c.attempt))
		}
	}
	want := []string{"drain@t1.verify#revise-r2", "guidance@", "drain@t1.verify#revise-r3"}
	if len(seq) != 3 || seq[0] != want[0] || seq[1] != want[1] || seq[2] != want[2] {
		t.Fatalf("seam sequence %v, want %v", seq, want)
	}

	// The resumed package = the drained batch: parked machine findings AND
	// the guidance, renumbered contiguously.
	g := pkgs[len(pkgs)-1]
	var machine, requester int
	for i, fd := range g.Findings {
		if fd.N != i+1 {
			t.Fatalf("resumed package numbering: %+v", g.Findings)
		}
		if fd.Requester {
			requester++
		} else {
			machine++
		}
	}
	if machine == 0 || requester == 0 {
		t.Fatalf("resumed package carries parked findings + guidance: %+v", g.Findings)
	}
}

// A V0-regenerated candidate re-mints as the round's new revision — the
// malformed mint stays in the never-compressed lineage.
func TestReviewSeamMintsRegeneratedCandidate(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	sink := &fakeSink{}
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	v.Review = sink
	v.Regenerate = func(_ context.Context, d verify.Deliverable, _ []string) (verify.Deliverable, error) {
		d.Content = "package main\n\nfunc main() { println(\"hello\") }\n"
		d.Revision++
		return d, nil
	}
	d := deliverable("t1", "r1")
	d.Content = "" // V0 hard-kill: empty artifact
	out, err := v.Verify(context.Background(), input(d))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s", out.Verdict)
	}
	mints := sink.ops("mint")
	if len(mints) != 2 || mints[0].rev != 1 || mints[1].rev != 2 {
		t.Fatalf("malformed + regenerated candidates both minted: %+v", mints)
	}
	// Both mints belong to round 1 (the regeneration is not a judged
	// round).
	if mints[0].attempt != "r1#round-1" || mints[1].attempt != "r1#round-1" {
		t.Fatalf("regeneration mint refs: %+v", mints)
	}
}
