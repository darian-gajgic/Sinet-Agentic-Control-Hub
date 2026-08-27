package accept_test

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// gf10refusal_test.go — P3-GF10 property P1 (R6), written RED before the
// implementation. Brief: P3/briefs/P3-GF10.md §4 R6/R7.
//
// S13.6 step 3 is an invariant of the ACT, not of the surface that offers it.
// The API's acceptable() guard is a door; this package is where the commit is
// actually composed, so a push-arm accept whose trailer inputs are empty has to
// be refused HERE — before Propose, so no effect row records an act that must
// never happen, and before the squash, so no permanent commit carries a
// Co-Authored-By line naming nobody.

// TestGF10PushArmRefusesTrailersAroundNobody drives a real repo-backed accept
// with each trailer input blanked in turn and re-reads the world to prove
// nothing outward happened.
//
// RED until GF10 lands: today Accept trusts its caller, renders
// "Co-Authored-By:  <>" from the empty strings and pushes it.
func TestGF10PushArmRefusesTrailersAroundNobody(t *testing.T) {
	for _, tc := range []struct {
		name  string
		blank func(*accept.Input)
	}{
		{"no engine", func(in *accept.Input) { in.Engine = "" }},
		{"no model", func(in *accept.Input) { in.Model = "" }},
		{"no vendor address", func(in *accept.Input) { in.VendorNoreply = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFix(t)
			in, _ := f.prepare("candidate\n")
			remoteBefore := f.remoteMain()
			headBefore, err := f.proj.RepoHead(f.ctx, "proj")
			if err != nil {
				t.Fatal(err)
			}
			tc.blank(&in)

			if _, err := f.acc.Accept(f.ctx, in); err == nil {
				t.Fatal("a push-arm accept with an empty trailer input must be refused: the commit it would make " +
					"carries a co-author line naming nobody (S13.6 step 3)")
			}
			// Refused BEFORE Propose: an effect row is the journal's record that an
			// outward act was authorized, and this one never was.
			if n := f.countRows(`SELECT COUNT(*) FROM effects`); n != 0 {
				t.Errorf("the refusal proposed %d effects — it must propose none", n)
			}
			if f.remoteMain() != remoteBefore {
				t.Error("the remote's protected ref moved for a refused accept")
			}
			if now, err := f.proj.RepoHead(f.ctx, "proj"); err != nil || now != headBefore {
				t.Errorf("the project's default branch moved (%q → %q, err %v)", headBefore, now, err)
			}
			d, err := f.rev.Deliverable(f.ctx, "dlv")
			if err != nil {
				t.Fatal(err)
			}
			if d.State != review.StateInReview {
				t.Errorf("state %q after a refused accept, want in-review", d.State)
			}
		})
	}

	// The non-tautological control: with all three inputs present the SAME
	// accept completes and pushes, so the refusal above is about the empty
	// strings and nothing else.
	t.Run("control: complete inputs still push", func(t *testing.T) {
		f := newFix(t)
		in, _ := f.prepare("candidate\n")
		out, err := f.acc.Accept(f.ctx, in)
		if err != nil {
			t.Fatalf("the push arm must still work with truthful trailer inputs: %v", err)
		}
		if !out.Accepted || out.Commit == "" || out.EffectID == "" {
			t.Fatalf("the push arm is byte-for-byte what it was: %+v", out)
		}
	})
}
