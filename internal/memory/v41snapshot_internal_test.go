package memory

import (
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// v41snapshot_internal_test.go — the build-time half of the snapshot doctrine,
// applied to P3-V41's own provenance (CONVENTIONS §57; the RW-12, GF3 and GF7
// tripwires this continues).
//
// EnsureV41TaxonomyGovernance writes the software question set under a FIXED
// record. That is honest exactly while the content is the content that record
// covers, so the digest is pinned and this is the tripwire. Between the four
// packets' tripwires all six seeded sets stay watched: RW-12's covers the four
// families it still governs, GF7's second test covers the generic set GF3
// governs, and this file covers the software set this packet governs — plus the
// frozen v4 bytes GF7's record still attests to.
//
// If this fails, the fix is NOT to update the digest. Mint a governance function
// with your own provenance record, exactly as this packet did to GF7's.
func TestV41DigestsMatchTheShippedSeeds(t *testing.T) {
	if err := verifyV41Snapshot(); err != nil {
		t.Fatalf("the shipped software question set no longer matches the content the P3-V41 record covers:\n%v", err)
	}
}

// TestV41SeedDriftIsRefused: divergence between the shipped seed and this
// packet's digest is ErrSeedDiverged — a loud skip, never a boot failure, and
// the forcing function that sends the next editor to write its own Ensure.
func TestV41SeedDriftIsRefused(t *testing.T) {
	saved := v41ContentDigest[intake.FamilySoftware]
	v41ContentDigest[intake.FamilySoftware] = "0000000000000000000000000000000000000000000000000000000000000000"
	defer func() { v41ContentDigest[intake.FamilySoftware] = saved }()
	if err := verifyV41Snapshot(); !errors.Is(err, ErrSeedDiverged) {
		t.Fatalf("drifted seed: %v, want ErrSeedDiverged", err)
	}
}

// TestV41SnapshotStillHoldsTheV4Bytes: this packet froze the v4 software set
// (gf7taxonomy_v4.go) so GF7's record keeps covering the bytes it was written
// against. Nothing else watches that freeze against GF7's pinned digest once the
// live seed has moved on. This does, constant against constant.
func TestV41SnapshotStillHoldsTheV4Bytes(t *testing.T) {
	frozen, err := gf7TaxonomyContent(intake.FamilySoftware)
	if err != nil {
		t.Fatalf("frozen v4 software content: %v", err)
	}
	if got, want := contentHash(frozen), gf7ContentDigest[intake.FamilySoftware]; got != want {
		t.Errorf("the frozen v4 snapshot hashes to %s, want the digest the P3-GF7 record pins %s — "+
			"the snapshot must reproduce the content that record covers, byte for byte", got, want)
	}
}
