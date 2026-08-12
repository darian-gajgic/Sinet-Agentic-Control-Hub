package memory

import "testing"

// TestRW12DigestsMatchTheShippedSeeds (drain r1 F3): the build-time half of
// the snapshot doctrine, applied to THIS packet's own provenance.
//
// EnsureRW12TaxonomyGovernance writes question-set content under a FIXED
// ratification record. That is only honest while the content is the content
// that record covers, so the digests are pinned and this test is the tripwire.
// Without it, a later packet editing a question set would have its content
// quietly superseded into governance at the next boot, attributed to a gate
// that never read it — with nothing anywhere to notice.
//
// If this fails, the fix is NOT to update the digests. Mint a governance
// function with your own provenance record, exactly as P3-RW-12 did to B2's
// (see b2taxonomy_v1.go for why the record must be a snapshot).
func TestRW12DigestsMatchTheShippedSeeds(t *testing.T) {
	if err := verifyRW12Snapshot(); err != nil {
		t.Fatalf("the shipped question sets no longer match the content the P3-RW-12 gate ratified:\n%v", err)
	}
}
