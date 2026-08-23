package conformance_test

// zai_ln2b_test.go — P3-LN-2B §9 spec 35 (S14.5; CONVENTIONS §32).
//
// registry.go said in its own Notes that the zai lane row completes at LN-2.
// This is that row, and this is the test that makes the claim checkable.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
)

func TestZAILaneRowRegistered(t *testing.T) {
	var row conformance.Row
	found := false
	for _, r := range conformance.SeedRows() {
		if r.ID == "adapter-zai" {
			row, found = r, true
		}
	}
	if !found {
		t.Fatal("no adapter-zai row — the zai LANE is commissioned at LN-2 and registry.go's own Notes say its row lands here")
	}

	if row.AffectClass != conformance.AffectLane {
		t.Errorf("affect class = %q, want %q (a lane suite is lane-affecting ⇒ flag-now)", row.AffectClass, conformance.AffectLane)
	}
	if row.Cadence != conformance.CadenceWeekly {
		t.Errorf("cadence = %q, want %q (the other two lane rows' cadence)", row.Cadence, conformance.CadenceWeekly)
	}
	if len(row.Fixtures) == 0 {
		t.Fatal("the zai row names no fixtures")
	}

	// Handles are prose-prefixed by tier, as the sibling adapter rows are.
	tiers := map[string]int{"tier F:": 0, "tier R:": 0, "tier L:": 0}
	for _, fx := range row.Fixtures {
		for prefix := range tiers {
			if strings.Contains(fx.Handle, prefix) {
				tiers[prefix]++
			}
		}
	}
	for prefix, n := range tiers {
		if n == 0 {
			t.Errorf("no fixture handle carries %q — the row must say which tier each leg is", prefix)
		}
	}

	// The Notes distinguish what the row proves (the LANE, atop the LN-1
	// substrate) from what it does not (no live provider call before the
	// ceremony), and they must no longer claim the lane is omitted.
	notes := row.Notes
	if strings.Contains(notes, "OMITTED") {
		t.Errorf("the zai row still says the lane is OMITTED: %q", notes)
	}
	for _, needle := range []string{"lane", "substrate", "LN-CEREMONY"} {
		if !strings.Contains(notes, needle) {
			t.Errorf("the zai row's notes never mention %q — the honest scope is what makes the row readable: %q", needle, notes)
		}
	}

	// The sibling rows' claim is discharged: nothing still promises the row
	// "completes at LN-2", because it has.
	for _, r := range conformance.SeedRows() {
		if r.ID == "adapter-zai" {
			continue
		}
		if strings.Contains(r.Notes, "complete at LN-2") || strings.Contains(r.Notes, "land at LN-2") {
			t.Errorf("row %q still defers the zai rows to LN-2, which is this packet: %q", r.ID, r.Notes)
		}
	}
}
