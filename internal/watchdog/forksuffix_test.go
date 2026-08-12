package watchdog

// forksuffix_test.go — P3-RW-6 drain r1 F2: the fork-suffix strip this package
// reads run ids through is PINNED here, where it is used.
//
// The strip was a private mirror of metering's and now delegates to it (the one
// shared matcher, P3-RW-6 OQ5). The swap was semantics-identical, but nothing
// in-package would have noticed if it were not — and this package's two readers
// of the strip decide real things: which runs are expected to produce activity
// (Tier-0's silence rule) and which ⚙ watchdog.silence_budget.<run_type> entry
// a run is judged against. A fork is the SAME work under a new identity (Spec
// S02.5 step 3), so both answers must survive any chain of `.g<n>` suffixes;
// getting it wrong silently re-buckets a recovered run onto the default budget.
//
// No new import: this file stays inside the §31 forward allowlist by using none
// beyond what the package already carries.

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestForkSuffixSurvivesSilenceClassification(t *testing.T) {
	chains := []string{"", ".g1", ".g2", ".g12", ".g4.g5.g6"}

	// The silence rule's activity expectation is a property of the ROLE, so a
	// recovered execute/verify run is still expected to produce activity, and a
	// recovered ceremony run is still not.
	for _, chain := range chains {
		for _, id := range []string{"t-x.execute", "t-x.verify"} {
			r := run.Run{ID: id + chain, UserID: "alice", Lane: "anthropic"}
			if !classExpectsActivity(r) {
				t.Errorf("classExpectsActivity(%q) = false, want true — a fork is the same work", r.ID)
			}
		}
		for _, id := range []string{"t-x.intake", "t-x.compose", "r-zombie", "t-x.gadget"} {
			r := run.Run{ID: id + chain, UserID: "alice", Lane: "anthropic"}
			if classExpectsActivity(r) {
				t.Errorf("classExpectsActivity(%q) = true, want false", r.ID)
			}
		}
		// A platform-owned run is never activity-watched, fork suffix or not.
		if classExpectsActivity(run.Run{ID: "t-x.execute" + chain, UserID: run.ActorPlatform}) {
			t.Errorf("a platform-owned run (%q) must not expect activity", "t-x.execute"+chain)
		}
	}

	// The ⚙ watchdog.silence_budget.<run_type> key a run is judged against is
	// likewise the role's, never "default" just because the run was recovered.
	for _, chain := range chains {
		for id, want := range map[string]string{
			"t-x.intake":  "intake",
			"t-x.execute": "execute",
			"t-x.verify":  "verify",
			"t-x.compose": "compose",
		} {
			if got := runType(run.Run{ID: id + chain, Lane: "anthropic"}); got != want {
				t.Errorf("runType(%q) = %q, want %q", id+chain, got, want)
			}
		}
		// No role: the lane, then the structural default — and a fork suffix
		// never turns a roleless id into a role.
		if got := runType(run.Run{ID: "r-zombie" + chain, Lane: "anthropic"}); got != "anthropic" {
			t.Errorf("runType(roleless with a lane) = %q, want the lane", got)
		}
		if got := runType(run.Run{ID: "platform.deadman.17548700" + chain}); got != "default" {
			t.Errorf("runType(roleless, no lane) = %q, want default", got)
		}
	}

	// The near-misses a naive strip gets wrong: these are NOT fork suffixes, so
	// the base id stands and the role is read from it unchanged.
	for _, tc := range []struct{ id, want string }{
		{"t-x.execute.g", "default"},   // ".g" is not ".g<n>"
		{"t-x.execute.gx", "default"},  // not digits
		{"t-x.execute.g1x", "default"}, // not digits
		{"t-x.g2.execute", "execute"},  // a fork-shaped segment mid-id is not a suffix
		{"t-g1.execute", "execute"},    // a task literally named t-g1
	} {
		if got := runType(run.Run{ID: tc.id}); got != tc.want {
			t.Errorf("runType(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}
