package verify_test

// tq4drain_test.go — P3-TQ-4a drain r1: the two rules the packet's own battery
// left unpinned.
//
// F5: a detected npm rung whose manifest is not in the checked-out work is a
// PLATFORM condition, so it is decided before execution and never becomes a
// FAIL that blames the work (Spec S07.3 rule 1).
//
// F4's evidence half in a GRADUATED pack: once one hand-captured command
// graduates a project, its pack can be mixed — the owner's rungs and the
// platform's, side by side. The detected ones are still evidence there, and a
// consumer that mints findings must read Check.Origin rather than the posture.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// TestTQ4StaleDetectedRungAgainstAManifestlessTreeIsUnrunnable [drain r1 F5]:
// the tree under review IS there and its package.json is not — a detected set
// that has gone stale against the work. Running the command anyway produces a
// failure that has nothing to do with the work, which is the one outcome rule 1
// forbids here.
func TestTQ4StaleDetectedRungAgainstAManifestlessTreeIsUnrunnable(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"README.md": "the manifest this rung needs is gone\n"})
	runner := &scriptRunner{}
	pack := detectedPack(detectedCheck("detected:build", verify.StageStatic, "/bin/sh", "-lc", "npm run build"))
	v := f.verifier(&fakeJudge{}, runner, pack)
	in := input(deliverable("t1", "r1"))
	in.Workspace = tree

	out, err := v.Verify(ctx, in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	build := outcomeFor(t, out.Rounds[0], "detected:build")
	if build.State != verify.CheckUnverifiable {
		t.Fatalf("detected:build state %q, want UNVERIFIABLE-HERE — the manifest that declares this script is not in the checked-out work", build.State)
	}
	if build.AttributedTo != verify.DetectedUnrunnable {
		t.Fatalf("detected:build attributed %q, want %q", build.AttributedTo, verify.DetectedUnrunnable)
	}
	if !strings.Contains(build.Detail, "package.json") {
		t.Fatalf("detected:build detail %q does not name what is missing", build.Detail)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("the rung was handed to the runner anyway (calls %v) — the precondition decides before execution, never from an exit code", runner.calls)
	}
}

// TestTQ4DetectedRungInAGraduatedPackMintsNoFinding [drain r1 F4]: a project
// that captured one command of its own graduates, and its pack then carries the
// owner's rung beside the platform's detected ones. A detected rung is still
// evidence there: it mints no finding, because a blocker raised from something
// nobody captured would let the platform's own guess decide a person's round.
// The same failure on an OWNER rung still mints one — the guard reads
// Check.Origin, it does not remove the rule.
func TestTQ4DetectedRungInAGraduatedPackMintsNoFinding(t *testing.T) {
	ctx := context.Background()
	mixedPack := func() *verify.CheckPack {
		return &verify.CheckPack{
			Domain: verify.DomainSoftware, Version: 2, VerifiedOn: time.Now().Add(-24 * time.Hour),
			Checks: []verify.Check{
				{ID: "lint", Stage: verify.StageStatic, Argv: []string{"true"}, FindingCategory: verify.CatACBlocker},
				{ID: "detected:test", Stage: verify.StageUnit, Argv: []string{"true"},
					FindingCategory: verify.CatACBlocker, Origin: verify.ProvenanceDetected},
			},
		}
	}
	findingFor := func(t *testing.T, r verify.RoundRecord, anchor string) bool {
		t.Helper()
		if r.V1 == nil {
			t.Fatal("no V1 record")
		}
		for _, fi := range r.V1.Findings {
			if fi.Anchor == anchor {
				return true
			}
		}
		return false
	}

	t.Run("a detected rung's runner failure mints nothing", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		runner := &scriptRunner{errs: map[string]error{"detected:test": errors.New("the sandbox refused to compose")}}
		v := f.verifier(&fakeJudge{}, runner, mixedPack())
		out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		r := out.Rounds[0]
		if got := outcomeFor(t, r, "detected:test"); got.State != verify.CheckRunnerFailed {
			t.Fatalf("detected:test state %q, want RUNNER-FAILURE recorded — nothing is silently skipped", got.State)
		}
		if findingFor(t, r, "check:detected:test") {
			t.Fatal("a detected rung minted a finding — it is evidence the platform noticed, not a bar the project set")
		}
	})

	t.Run("an owner rung's runner failure still mints one", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t2", "r2")
		runner := &scriptRunner{errs: map[string]error{"lint": errors.New("the sandbox refused to compose")}}
		v := f.verifier(&fakeJudge{}, runner, mixedPack())
		out, err := v.Verify(ctx, input(deliverable("t2", "r2")))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if !findingFor(t, out.Rounds[0], "check:lint") {
			t.Fatal("an owner check's runner failure minted no finding — the guard reads Check.Origin, it does not remove the rule")
		}
	})
}
