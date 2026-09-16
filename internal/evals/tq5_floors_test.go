package evals_test

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq5_floors_test.go — P3-TQ-5 acceptance (Spec S14.8 ¶2/¶5 floors per
// (asset, VERSION); S07.9 P-T06-5). The seeded floor is derived from the
// measurement of the version it names: rubric-software v3 was measured on
// claude-opus-5, so its floor's basis names that seat and that measurement
// file — never the 2026-07-22 opus-4-8 run under a v3 label.

func TestTQ5SeedFloorTracksTheOpus5Measurement(t *testing.T) {
	floors := evals.SeedFloors()
	if len(floors) != 1 {
		t.Fatalf("seed floors = %d, want 1 (the software rubric)", len(floors))
	}
	f := floors[0]
	r := verify.SeedSoftwareRubric()
	if f.AssetKind != evals.AssetRubric || f.AssetID != r.ID || f.AssetVersion != r.Version || f.AssetVersion != 3 {
		t.Fatalf("floor keyed %s/%s v%d, want %s v3 (a floor tracks the version it was measured against)", f.AssetKind, f.AssetID, f.AssetVersion, r.ID)
	}
	for _, needle := range []string{"claude-opus-5", "2026-09-17-rider1-golden-set-opus5.md", "Wilson"} {
		if !strings.Contains(f.Basis, needle) {
			t.Errorf("floor basis does not name %q: %q", needle, f.Basis)
		}
	}
	if strings.Contains(f.Basis, "opus-4-8") {
		t.Errorf("floor basis still credits the retired judge: %q", f.Basis)
	}
	if f.CatchFloor <= 0 || f.CatchFloor > 1 {
		t.Errorf("catch floor %v is not a measured Wilson lower bound in (0,1]", f.CatchFloor)
	}
	if f.TNRReportOnly == nil {
		t.Error("the TNR companion is missing — it is carried report-only, never dropped")
	}
	if f.Ratified {
		t.Error("a seeded floor ships ratified=false; ratification is the gate's act, never the seed's")
	}
}
