package verify_test

// tq7drain_test.go — P3-TQ-7 drain round 1. Three properties the acceptance
// battery left unpinned:
//
//   - the kill guard reads the PER-CHECK origin, and nothing else. Every
//     fixture in the acceptance battery sets posture, pack-level provenance
//     and per-check origin together, so a guard reading any one of the three
//     passed it. A GRADUATED, MIXED pack separates them: posture "" and no
//     pack-level provenance, with one owner rung beside one detected rung
//     (Spec S07.8 [A16] — A16's precedence is per slot, so an owner who
//     captured only a lint command still gets the build rung the platform
//     detected, in the same pack).
//   - the minted finding carries the category the CHECK DECLARED (Spec S07.3),
//     which every fixture happened to declare as AC-BLOCKER.
//   - the output tail cuts on whole lines in BOTH directions: it advances off
//     a mid-line cut, and it does not throw away a line that already fits.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq7MixedPack is a GRADUATED project's pack with mixed origins: the owner
// captured lint, the platform detected build in the produced tree. Posture is
// empty (the project graduated) and no pack-level provenance is set (a mixed
// pack has no single answer), so Check.Origin is the ONLY fact distinguishing
// the two rungs — which is exactly what makes it a guard test.
func tq7MixedPack() *verify.CheckPack {
	return &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 4, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "lint", Stage: verify.StageStatic, Argv: []string{"/bin/sh", "-lc", "npm run lint"}, FindingCategory: verify.CatACBlocker},
			{ID: "build", Stage: verify.StageStatic, Argv: []string{"/bin/sh", "-lc", "npm run build"}, Origin: verify.ProvenanceDetected, FindingCategory: verify.CatACBlocker},
		},
	}
}

// TestTQ7MixedPackKillsOnTheOwnersRungOnly [R9 guard]: in one graduated pack
// carrying both kinds of rung, a failing DETECTED rung is evidence and a
// failing OWNER rung is the kill. The pack's posture and pack-level provenance
// are empty in both halves, so neither can be what decides it.
func TestTQ7MixedPackKillsOnTheOwnersRungOnly(t *testing.T) {
	ctx := context.Background()

	// The guard's two decoys, asserted on the fixture itself: a mint guarded
	// on either of these would fire on the detected rung below.
	pack := tq7MixedPack()
	if pack.Posture != "" || pack.Provenance != "" {
		t.Fatalf("the mixed fixture must carry neither posture nor pack-level provenance: %+v", pack)
	}

	t.Run("the detected rung fails", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		v := f.verifier(&fakeJudge{}, &tq7Runner{exits: map[string]int{"build": 1}}, tq7MixedPack())

		out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		rd := out.Rounds[0]
		if got := checkBlockers(rd.Findings); len(got) != 0 {
			t.Fatalf("a detected rung of a graduated pack minted the kill: %+v", got)
		}
		if _, ok := findingAt(rd.Findings, "check:build"); ok {
			t.Fatalf("the detected build minted a finding: %+v", rd.Findings)
		}
		if rd.Verdict == verify.VerdictRevise {
			t.Fatalf("a detected rung's failure drove the round to REVISE: %+v", rd.Findings)
		}
		// The failure is still RECORDED — evidence is not silence.
		recorded := false
		for _, c := range rd.V1.Checks {
			if c.CheckID == "build" && c.State == verify.CheckFailed && c.ExitCode == 1 {
				recorded = true
			}
		}
		if !recorded {
			t.Fatalf("the detected build's failure was not recorded as evidence: %+v", rd.V1.Checks)
		}
		// What this round then does with its clean verdict is NOT this test's
		// subject: a graduated pack whose only failure is detected ships
		// today, and whether that is right is a TQ-4a follow-up. Asserted
		// here: the kill did not fire, and nothing above depends on it.
	})

	t.Run("the owner rung fails", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		v := f.verifier(&fakeJudge{}, &tq7Runner{exits: map[string]int{"lint": 1}}, tq7MixedPack())

		out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		rd := out.Rounds[0]
		fd, ok := findingAt(rd.Findings, "check:lint")
		if !ok {
			t.Fatalf("the owner's failed lint minted nothing in a mixed pack: %+v", rd.Findings)
		}
		if fd.Severity != verify.SeverityBlocker || fd.Demoted || fd.Criterion != "check:lint" {
			t.Fatalf("owner finding %+v, want an undemoted blocker citing check:lint", fd)
		}
		if _, ok := findingAt(rd.Findings, "check:build"); ok {
			t.Fatalf("the passing detected rung minted a finding: %+v", rd.Findings)
		}
		if rd.Verdict != verify.VerdictRevise {
			t.Fatalf("round verdict %s, want REVISE — the owner's own check refused the work", rd.Verdict)
		}
		if len(out.VerifiedItems) != 0 {
			t.Fatalf("a killed round verified %v", out.VerifiedItems)
		}
	})
}

// TestTQ7MintedFindingCarriesTheChecksDeclaredCategory [R1]: the category is
// the one the CHECK declared, never a literal chosen at the mint site (Spec
// S07.3 — a stage contract is incomplete unless it declares its finding
// categories and their escalation routes). Production packs declare
// AC-BLOCKER, which is why every other fixture does; a pack declaring
// SANITY-BLOCKER must route under SANITY-BLOCKER.
func TestTQ7MintedFindingCarriesTheChecksDeclaredCategory(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	pack := &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 2, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "smoke", Stage: verify.StageSmoke, Argv: []string{"smoke"}, FindingCategory: verify.CatSanityBlocker},
		},
	}
	v := f.verifier(&fakeJudge{}, &tq7Runner{exits: map[string]int{"smoke": 1}}, pack)

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	rd := out.Rounds[0]
	fd, ok := findingAt(rd.Findings, "check:smoke")
	if !ok {
		t.Fatalf("no finding anchored check:smoke: %+v", rd.Findings)
	}
	if fd.Category != verify.CatSanityBlocker {
		t.Fatalf("minted category %q, want the check's declared SANITY-BLOCKER", fd.Category)
	}
	if fd.Severity != verify.SeverityBlocker || fd.Demoted {
		t.Fatalf("finding is %q (demoted=%v), want an undemoted blocker — admission is by origin, not by category", fd.Severity, fd.Demoted)
	}
	if got, want := fd.Key(), verify.FindingKey("check:smoke|check:smoke|SANITY-BLOCKER"); got != want {
		t.Fatalf("finding key %q, want %q", got, want)
	}
	if rd.Verdict != verify.VerdictRevise {
		t.Fatalf("round verdict %s, want REVISE", rd.Verdict)
	}
}

// tq7Lines builds n lines of exactly width bytes each (the newline included),
// each naming its own number so a test can say WHICH lines survived the cut.
func tq7Lines(n, width int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		head := fmt.Sprintf("line %03d ", i)
		b.WriteString(head + strings.Repeat("x", width-len(head)-1) + "\n")
	}
	return b.String()
}

// tq7TailOf returns the output tail a finding's text carries.
func tq7TailOf(t *testing.T, text string) string {
	t.Helper()
	const marker = "This is the end of what it printed:\n\n"
	i := strings.Index(text, marker)
	if i < 0 {
		t.Fatalf("finding text carries no output tail: %q", text)
	}
	return text[i+len(marker):]
}

// TestTQ7OutputTailCutsOnWholeLines [R4]: the tail is whole lines in both
// directions. A cut landing MID-line advances to the next boundary, so a point
// never opens on half a line; a cut landing exactly ON a boundary advances no
// further, because discarding a whole line that already fits loses evidence
// for nothing.
func TestTQ7OutputTailCutsOnWholeLines(t *testing.T) {
	ctx := context.Background()
	s := regSettings(t)

	cases := []struct {
		name      string
		lines     int
		width     int
		wantFirst int // the first line number that must survive the cut
	}{
		// 100 × 64 B = 6400; the last 2048 B begin exactly at line 69, so all
		// 32 remaining lines fit and every one of them must be carried.
		{"cut lands on a line boundary", 100, 64, 69},
		// 100 × 60 B = 6000; the last 2048 B begin inside line 66, so the cut
		// advances to line 67 rather than carrying a fragment.
		{"cut lands mid-line", 100, 60, 67},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := tq7Lines(tc.lines, tc.width)
			res, err := verify.RunV1(ctx, tq7OwnerPack(),
				&tq7Runner{exits: map[string]int{"build": 1}, tails: map[string]string{"build": out}},
				v1req(), nil, nil, time.Now(), s)
			if err != nil {
				t.Fatalf("RunV1: %v", err)
			}
			fd, ok := findingAt(res.Findings, "check:build")
			if !ok {
				t.Fatalf("no finding anchored check:build: %+v", res.Findings)
			}
			tail := tq7TailOf(t, fd.Text)
			if len(tail) > 2048 {
				t.Fatalf("tail is %d bytes, over the 2 KB bound", len(tail))
			}
			got := strings.Split(tail, "\n")
			wantN := tc.lines - tc.wantFirst + 1
			if len(got) != wantN {
				t.Fatalf("tail carries %d lines, want %d (lines %d..%d)", len(got), wantN, tc.wantFirst, tc.lines)
			}
			for i, line := range got {
				want := fmt.Sprintf("line %03d ", tc.wantFirst+i)
				if !strings.HasPrefix(line, want) {
					t.Fatalf("tail line %d is %q, want it to begin %q — the cut carried a fragment or dropped a whole line", i, line, want)
				}
				if len(line) != tc.width-1 {
					t.Fatalf("tail line %d is %d bytes, want a whole %d-byte line: %q", i, len(line), tc.width-1, line)
				}
			}
		})
	}
}
