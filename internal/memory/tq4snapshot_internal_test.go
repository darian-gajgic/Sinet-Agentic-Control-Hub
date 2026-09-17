package memory

// tq4snapshot_internal_test.go — the build-time tripwire for the P3-TQ-4a
// composer-playbook record (CONVENTIONS §57). tq4PlaybookDigest is the runtime
// half: it pins the content this packet's provenance attests to, and a boot
// refuses to write anything under that record once the in-code playbook has
// moved past it. This is the half that says so at build time, before a boot
// ever has to.

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// TestTQ4DigestMatchesTheShippedPlaybook fails the moment the in-code playbook
// and this packet's ratified digest disagree. The fix is never to update the
// digest — it records what the P3-TQ-4 gate is asked to ratify — but to freeze
// this content as a snapshot and mint a new Ensure with its own provenance.
func TestTQ4DigestMatchesTheShippedPlaybook(t *testing.T) {
	if got := contentHash(worker.ComposerPlaybookSeed()); got != tq4PlaybookDigest {
		t.Fatalf("composer playbook digest = %s, ratified %s — do not update the digest; freeze this content and mint a new Ensure (CONVENTIONS §57)", got, tq4PlaybookDigest)
	}
	if err := verifyTQ4Snapshot(); err != nil {
		t.Fatalf("snapshot verification: %v", err)
	}
}

// TestTQ4PredecessorIsTheB3RatifiedPlaybook binds the other end of the
// supersession: the frozen seed-1 snapshot is what the B3 seeder now writes,
// and it is NOT the current seed — a predecessor that tracked the live code
// would make the supersession unreachable.
func TestTQ4PredecessorIsTheB3RatifiedPlaybook(t *testing.T) {
	seed1 := tq4PlaybookSeed1()
	if seed1 == worker.ComposerPlaybookSeed() {
		t.Fatal("the frozen predecessor equals the live seed — the snapshot is a pointer again, and nothing would ever supersede")
	}
	if want := "Seed seed-1 (P3-B3-5)."; !contains(seed1, want) {
		t.Fatalf("the frozen snapshot does not name the version it froze (%q)", want)
	}
	if contains(seed1, "Web deliverables") {
		t.Fatal("the frozen seed-1 snapshot carries this packet's own section — it is meant to hold what B3 ratified")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
