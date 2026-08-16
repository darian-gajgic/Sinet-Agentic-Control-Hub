package verify_test

import (
	"context"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// P3-RW-14 R7 acceptance — the wrote-nothing gate (Spec S07.2 diff-empty;
// S13.1 "revision 1 is presented against the pre-task state"; S02.8 claims are
// the durable knowledge that the task writes).
//
// The live defect this binds: a repo-backed execute leg burned $1.11 across 7
// engine sessions, every stage session reported "completed", and the worktree
// held nothing but `.git` — every checkpoint's snapshot ref was the store-init
// base commit. V0's ratified diff-empty gate could not see it: it compares
// revision N to N-1 by BYTES, and revision 1 of a repo-backed task has no
// previous revision. The snapshot commit and the attempt's base ref are both
// durable PLATFORM facts (no engine-supplied signal), and an active W claim /
// non-empty step write_set is the durable knowledge that the task writes — so
// snapshot == base under a write claim is a malformed verdict: a hard kill at
// $0, one regeneration, never a judge call.

func TestRW14WroteNothingIsV0Malformed(t *testing.T) {
	// The gate is a pure function of platform facts on the deliverable.
	t.Run("gate", func(t *testing.T) {
		const base = "e35f8d1aa0dc0ffee0000000000000000000beef"
		const moved = "9c1d2e3f40000000000000000000000000000abc"
		cases := []struct {
			name       string
			d          verify.Deliverable
			wantKilled bool
		}{
			{
				name: "repo-backed rev 1, W claim, snapshot == base",
				d: verify.Deliverable{
					Revision: 1, SnapshotSHA: base, BaseSHA: base, WriteClaimed: true,
				},
				wantKilled: true,
			},
			{
				name: "same setup, snapshot != base — real work landed",
				d: verify.Deliverable{
					Revision: 1, SnapshotSHA: moved, BaseSHA: base, WriteClaimed: true,
				},
			},
			{
				name: "no write claim: a read-only plan legitimately moves no tree",
				d: verify.Deliverable{
					Revision: 1, SnapshotSHA: base, BaseSHA: base,
				},
			},
			{
				name: "not repo-backed: no snapshot pin, the gate has no facts",
				d: verify.Deliverable{
					Revision: 1, WriteClaimed: true,
				},
			},
			{
				name: "repo-backed but the base ref is unknown: never guess",
				d: verify.Deliverable{
					Revision: 1, SnapshotSHA: base, WriteClaimed: true,
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := tc.d
				d.Domain, d.Type = verify.DomainSoftware, "code"
				d.Content = "package main\n\nfunc main() {}\n"
				res := verify.RunV0(verify.DefaultPreGates(nil), d)
				var reason string
				for _, r := range res.Reasons {
					if strings.Contains(r, "wrote nothing") {
						reason = r
					}
				}
				if tc.wantKilled {
					if !res.Malformed || reason == "" {
						t.Fatalf("want a wrote-nothing malformed verdict, got malformed=%v reasons=%v",
							res.Malformed, res.Reasons)
					}
					if !strings.Contains(reason, "execution wrote nothing to the claimed write set") {
						t.Errorf("reason %q does not carry the ratified wording", reason)
					}
					return
				}
				if reason != "" {
					t.Fatalf("gate tripped on a candidate it must pass: %q", reason)
				}
			})
		}
	})

	// Drain-level: the kill costs zero judge calls (Spec S07.2 — a V0
	// malformed verdict is a hard kill, never a paid call).
	t.Run("costs no judge call", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		j := &fakeJudge{}
		v := f.verifier(j, &scriptRunner{}, passPack())

		d := deliverable("t1", "r1")
		const base = "e35f8d1aa0dc0ffee0000000000000000000beef"
		d.SnapshotSHA, d.BaseSHA, d.WriteClaimed = base, base, true

		out, err := v.Verify(context.Background(), input(d))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if out.Verdict != verify.VerdictEscalate {
			t.Fatalf("verdict %s, want ESCALATE (the one-regeneration bound with no regeneration seam)", out.Verdict)
		}
		if len(j.inputs) != 0 {
			t.Fatalf("the wrote-nothing kill paid for %d judge call(s); it must cost $0", len(j.inputs))
		}
	})

	// The same drain with a moved tree reaches the paid layers: the gate is a
	// kill for the empty case only, never a blanket refusal of repo-backed work.
	t.Run("a moved tree still reaches the judges", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t2", "r2")
		j := &fakeJudge{}
		v := f.verifier(j, &scriptRunner{}, passPack())

		d := deliverable("t2", "r2")
		d.SnapshotSHA = "9c1d2e3f40000000000000000000000000000abc"
		d.BaseSHA = "e35f8d1aa0dc0ffee0000000000000000000beef"
		d.WriteClaimed = true

		out, err := v.Verify(context.Background(), input(d))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if out.Verdict != verify.VerdictShip {
			t.Fatalf("verdict %s, want SHIP", out.Verdict)
		}
		if len(j.inputs) == 0 {
			t.Fatal("a candidate that moved the tree never reached the judges")
		}
	})
}
