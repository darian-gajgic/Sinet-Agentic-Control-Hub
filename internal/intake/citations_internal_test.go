package intake

import (
	"os"
	"strings"
	"testing"
)

// TestPlanCitationsByteIdentity: the S09.6 citation member is additive and
// optional — a plan WITHOUT citations renders byte-identically to before (no
// section), and a plan WITH citations survives the S06.6 write/load round-trip
// (sha256 + byte-identical re-render still passes). (R30/R20 rubric item 20.)
func TestPlanCitationsByteIdentity(t *testing.T) {
	store := artifactStore{root: t.TempDir()}
	base := testPair("t1", "u1").Plan

	// Citation-free render carries no citation section.
	noCite := renderPlanMD(&base)
	if strings.Contains(string(noCite), "Cited knowledge") {
		t.Fatal("a citation-free plan rendered the Cited knowledge section")
	}

	// With citations: survives the round-trip (load verifies sha256 + a
	// byte-identical re-render, so a non-deterministic render would fail here).
	cited := base
	cited.CitedEntries = []string{"know-1", "know-2"}
	ref, err := store.write(store.planPath("t1", 1), renderPlanMD(&cited), &cited, 1)
	if err != nil {
		t.Fatalf("write cited plan: %v", err)
	}
	loaded, err := store.loadPlan(&ref)
	if err != nil {
		t.Fatalf("load cited plan (byte-identity check): %v", err)
	}
	if len(loaded.CitedEntries) != 2 || loaded.CitedEntries[0] != "know-1" {
		t.Fatalf("citations did not survive: %+v", loaded.CitedEntries)
	}
	md, _ := os.ReadFile(ref.Path)
	if !strings.Contains(string(md), "## Cited knowledge") || !strings.Contains(string(md), "- know-1") {
		t.Fatalf("cited plan markdown missing the section:\n%s", md)
	}
}
