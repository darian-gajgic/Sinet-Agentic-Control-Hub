package memory

import (
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf7snapshot_internal_test.go — the build-time half of the snapshot doctrine,
// applied to P3-GF7's own provenance (CONVENTIONS §57; the RW-12 and GF3
// tripwires this continues).
//
// EnsureGF7TaxonomyGovernance writes the software question set under a FIXED
// record. That is honest exactly while the content is the content that record
// covers, so the digest is pinned and this is the tripwire. Between the three
// packets' tripwires all six seeded sets stay watched: RW-12's covers the four
// families it still governs, this file's second test covers the generic set GF3
// governs, and the first covers the software set this packet governs.
//
// If this fails, the fix is NOT to update the digest. Mint a governance function
// with your own provenance record, exactly as this packet did to GF3's.
func TestGF7DigestsMatchTheShippedSeeds(t *testing.T) {
	if err := verifyGF7Snapshot(); err != nil {
		t.Fatalf("the shipped software question set no longer matches the content the P3-GF7 record covers:\n%v", err)
	}
}

// TestGF7SnapshotStillHoldsTheV3GenericSeed: after the GF3 content source was
// repointed at the frozen v3 snapshot, nothing else watches the generic set's
// live seed against the record that still governs it. This does.
func TestGF7SnapshotStillHoldsTheV3GenericSeed(t *testing.T) {
	frozen, err := gf3TaxonomyContent(intake.FamilyGeneric)
	if err != nil {
		t.Fatalf("frozen generic content: %v", err)
	}
	live, err := taxonomyContentOf(intake.FamilyGeneric, intake.SeedTaxonomies()[intake.FamilyGeneric])
	if err != nil {
		t.Fatalf("live generic content: %v", err)
	}
	if live != frozen {
		t.Error("the shipped generic question set no longer matches the content the P3-GF3-BE1 record covers and still governs — " +
			"an edited question set needs its OWN governance function and provenance record (CONVENTIONS §57)")
	}
}

// TestGF7SeedDriftIsRefused: divergence between the shipped seed and this
// packet's digest is ErrSeedDiverged — a loud skip, never a boot failure, and
// the forcing function that sends the next editor to write its own Ensure.
func TestGF7SeedDriftIsRefused(t *testing.T) {
	saved := gf7ContentDigest[intake.FamilySoftware]
	gf7ContentDigest[intake.FamilySoftware] = "0000000000000000000000000000000000000000000000000000000000000000"
	defer func() { gf7ContentDigest[intake.FamilySoftware] = saved }()
	if err := verifyGF7Snapshot(); !errors.Is(err, ErrSeedDiverged) {
		t.Fatalf("drifted seed: %v, want ErrSeedDiverged", err)
	}
}
