package memory

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf3snapshot_internal_test.go — drain r1 F2: the build-time tripwire the R2
// re-point would otherwise have removed.
//
// Before the re-point, verifyRW12Snapshot hashed the LIVE seeds, so editing any
// question set without minting a governance packet failed loudly. After it, RW-12
// compares its frozen snapshot to its own digests — constant against constant —
// which is exactly right for the record and leaves the four families this packet
// did NOT revise with nothing watching them: an ungoverned edit of research,
// content, data or chore would trip nothing at all, and their governed files
// would drift from the runtime seed silently. That is the §57 drift class.
//
// So the forcing function moves here, in the honest shape: those four families
// are still governed by the RW-12 record, so their live seed must still BE the
// content that record covers. The revised two are exempt because their divergence
// is the point of this packet, and they have their own digests
// (TestGF3DigestsCoverTheRevisedSeeds below).
//
// If this fails, the fix is NOT to update the snapshot. Mint a governance
// function with your own provenance, exactly as this packet did to RW-12's.
func TestUnrevisedSeedsStillMatchTheRW12Snapshot(t *testing.T) {
	unrevised := []intake.Family{
		intake.FamilyResearch, intake.FamilyContent, intake.FamilyData, intake.FamilyChore,
	}
	for _, fam := range unrevised {
		frozen, err := taxonomyContent(fam) // the RW-12-ratified snapshot
		if err != nil {
			t.Fatalf("frozen %s content: %v", fam, err)
		}
		live, err := taxonomyContentOf(fam, intake.SeedTaxonomies()[fam])
		if err != nil {
			t.Fatalf("live %s content: %v", fam, err)
		}
		if live != frozen {
			t.Errorf("the shipped %s question set no longer matches the content the P3-RW-12 gate ratified and still governs — "+
				"an edited question set needs its OWN governance function and provenance record (CONVENTIONS §57)", fam)
		}
	}
}

// TestGF3DigestsCoverTheRevisedSeeds is the same tripwire for the two families
// this packet DID revise: their live content is what the GF3 record attests to,
// and it stops being true the moment someone edits them without minting the next
// packet's Ensure. Between the two tests, all six seeded sets are watched.
func TestGF3DigestsCoverTheRevisedSeeds(t *testing.T) {
	if err := verifyGF3Snapshot(); err != nil {
		t.Fatalf("the shipped question sets no longer match the content the P3-GF3-BE1 record covers:\n%v", err)
	}
	covered := map[intake.Family]bool{}
	for _, fam := range gf3SupersedeFamilies {
		covered[fam] = true
	}
	for _, fam := range []intake.Family{intake.FamilySoftware, intake.FamilyGeneric} {
		if !covered[fam] {
			t.Errorf("the revised %s set is not covered by this packet's digest map", fam)
		}
	}
}
