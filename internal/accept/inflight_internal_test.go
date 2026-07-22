package accept

import "testing"

// The depth-1 merge-queue in-flight registry (Spec S13.6 step 1, F2): at most
// one in-flight accept per project carries the HEAD-plus-first commit the next
// candidate validates against; it is marked only after a SUCCESSFUL push and
// cleared on terminal, effID-scoped so a stale clear cannot drop a newer
// in-flight.
func TestInflightRegistry(t *testing.T) {
	a := &Accepter{inflightByProject: map[string]inflight{}}

	// A rejected/failed accept never marks (mark is after push success), so the
	// next candidate falls through to real HEAD — projectOnto returns "" here
	// (no in-flight) and would query RepoHead in production. Both LEGS:
	if _, ok := a.inflightByProject["p"]; ok {
		t.Fatal("a fresh registry has an in-flight entry")
	}

	// A successful first accept records HEAD-plus-first.
	a.markInflight("p", "eff1", "COMMIT_A")
	if inf, ok := a.inflightByProject["p"]; !ok || inf.commit != "COMMIT_A" {
		t.Fatalf("in-flight not recorded: %+v", a.inflightByProject["p"])
	}

	// A stale clear (wrong effID) must NOT drop a newer in-flight.
	a.markInflight("p", "eff2", "COMMIT_B") // depth-1: newest wins
	a.clearInflight("p", "eff1")
	if inf, ok := a.inflightByProject["p"]; !ok || inf.commit != "COMMIT_B" {
		t.Errorf("a stale clear dropped the newer in-flight: %+v ok=%v", a.inflightByProject["p"], ok)
	}

	// The matching clear removes it → the next candidate revalidates against
	// real HEAD (leg two).
	a.clearInflight("p", "eff2")
	if _, ok := a.inflightByProject["p"]; ok {
		t.Error("the matching clear did not remove the in-flight entry")
	}
}
