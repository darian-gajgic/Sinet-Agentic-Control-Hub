package memory_test

// v41guard_test.go — executor-authored cover for the one branch of
// EnsureV41TaxonomyGovernance that no shipped test reached (P3-V41 drain r1 F1):
// the decisionRecorded guard, "a later operator decision stands" (Spec S09.8;
// CONVENTIONS §57).
//
// It is the V41 analogue of TestGF7GateRefusalStands, and it exists because
// content equality alone cannot carry the rule. An operator who moves the set
// back to the byte-exact v4 content leaves a committed hash identical to this
// packet's PREDECESSOR hash, which is precisely the shape the Ensure treats as
// "not applied here yet". Only the version history distinguishes the two, so the
// guard asks the history and this test is what holds it there.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
)

// TestV41LaterOperatorRevertToV4Stands: after v4.1 is active, the operator uses
// the knowledge gate to go back to the frozen v4 content. That is a LATER
// decision about the same topic, so every subsequent boot must leave it alone —
// no supersession, no new v4.1 row, the operator's version still current.
func TestV41LaterOperatorRevertToV4Stands(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernanceV41(t, f)

	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cur.OriginRef, "P3-V41") {
		t.Fatalf("precondition: the active version is %q, want this packet's v4.1", cur.OriginRef)
	}

	// The revert is byte-exact v4 — the content THIS packet's Ensure was written
	// against, which is what makes the guard load-bearing rather than decorative.
	back, err := json.MarshalIndent(memory.GF7SoftwareTaxonomyForTest(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := f.gate.NewVersion(ctx, "darian", cur.ID, memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindTaxonomy, Title: cur.Title,
		Content: string(back) + "\n", TopicKey: cur.TopicKey, Selectors: cur.Selectors,
		FileBacked: true, FileName: filepath.Base(cur.FilePath),
	})
	if err != nil {
		t.Fatalf("operator revert to v4 through the gate: %v", err)
	}

	for boot := 0; boot < 2; boot++ {
		if res, err := f.gate.EnsureGF7TaxonomyGovernance(ctx); err != nil || res.Superseded != 0 {
			t.Fatalf("boot %d after the revert, GF7: %+v err=%v", boot+1, res, err)
		}
		res, err := f.gate.EnsureV41TaxonomyGovernance(ctx)
		if err != nil {
			t.Fatalf("boot %d after the revert, V41: %v", boot+1, err)
		}
		if res.Superseded != 0 {
			t.Fatalf("boot %d re-applied v4.1 over the operator's later decision (superseded=%d) — "+
				"content equality is not a decision; the version history is", boot+1, res.Superseded)
		}
	}

	after, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != reverted.ID {
		t.Fatalf("the active version is %q, want the operator's revert %q", after.ID, reverted.ID)
	}
	if !strings.Contains(after.Content, `"version": "v4"`) {
		t.Error("the active content is not the v4 the operator reverted to")
	}
}
