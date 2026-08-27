package memory_test

// gf7seeds_test.go — the P3-GF7 governance leg (brief §9 T6; Spec S09.10 row 1,
// S09.4 station 4, S09.8; CONVENTIONS §57). Committed RED with the grounding
// reds and green at the P3-GF7 implementation commit.
//
// The headline these assert: the v4 SOFTWARE question set enters the record as
// its own version under its own PENDING record; the version chain a fresh world
// writes reads v1 → v2 → v3 → v4; GF3's Ensure still writes the FROZEN v3 bytes
// its record covers rather than following the code; the generic set is left
// exactly where GF3 put it; the boots chain instead of fighting; and a refusal
// at the planning-rework exit gate stands.

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
)

// bootTaxonomyGovernanceGF7 runs the boot sequence in its wired order: B2 (it
// creates the entries), then RW-12, then GF3, then this packet's.
func bootTaxonomyGovernanceGF7(t *testing.T, f *fix) memory.TaxonomyGovernanceResult {
	t.Helper()
	ctx := context.Background()
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	if _, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); err != nil {
		t.Fatalf("EnsureRW12TaxonomyGovernance: %v", err)
	}
	if _, err := f.gate.EnsureGF3TaxonomyGovernance(ctx); err != nil {
		t.Fatalf("EnsureGF3TaxonomyGovernance: %v", err)
	}
	res, err := f.gate.EnsureGF7TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureGF7TaxonomyGovernance: %v", err)
	}
	return res
}

// TestGF7GovernanceWritesTheV4ChainUnderAPendingRecord (T6): a fresh world ends
// with the v4 software content governed, chained onto the version GF3 wrote,
// under a record that states ratification is PENDING at the planning-rework
// exit gate — and the generic set untouched at v3.
func TestGF7GovernanceWritesTheV4ChainUnderAPendingRecord(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)

	// House scope needs its D10 holder; without one the boot defers and retries.
	if _, err := f.gate.EnsureGF7TaxonomyGovernance(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")

	res := bootTaxonomyGovernanceGF7(t, f)
	if res.Superseded != 1 {
		t.Fatalf("superseded = %d, want the software set alone moved to v4", res.Superseded)
	}
	if res.Created != 0 || res.Repaired != 0 || res.Unverifiable != 0 {
		t.Errorf("clean world result = %+v, want one supersession and nothing else", res)
	}

	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatalf("HouseObject software: %v", err)
	}
	if !strings.Contains(cur.OriginRef, "P3-GF7") {
		t.Errorf("active software provenance = %q, want this packet's record", cur.OriginRef)
	}
	if !strings.Contains(cur.OriginRef, "PENDING") {
		t.Errorf("the v4 record does not say ratification is pending: %q", cur.OriginRef)
	}
	if !strings.Contains(strings.ToLower(cur.OriginRef), "opus") {
		t.Errorf("the v4 record names no drafting model (S06.5): %q", cur.OriginRef)
	}

	// The chain a fresh world writes: v1 → v2 → v3 → v4, each under the record
	// of the gate that owns it (S09.8 — a new version, never an in-place edit).
	wantPackets := []string{"P3-GF7", "P3-GF3-BE1", "P3-RW-12"}
	entry := cur
	for i, packet := range wantPackets {
		if !strings.Contains(entry.OriginRef, packet) {
			t.Fatalf("chain position %d is %q, want %s's version", i, entry.OriginRef, packet)
		}
		if i == len(wantPackets)-1 {
			break
		}
		if entry.Supersedes == "" {
			t.Fatalf("%s's version chains to nothing", packet)
		}
		prev, err := f.store.Get(ctx, entry.Supersedes)
		if err != nil {
			t.Fatalf("Get superseded %s: %v", entry.Supersedes, err)
		}
		if prev.Status != memory.StatusRetired {
			t.Errorf("the version %s superseded is %q, want retired", packet, prev.Status)
		}
		entry = prev
	}

	// The governed file IS the operator-editable override input, and it says
	// what the runtime serves.
	got, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-software.json"))
	if err != nil {
		t.Fatalf("governed software taxonomy fails LoadTaxonomy: %v", err)
	}
	if !reflect.DeepEqual(got, intake.SeedTaxonomies()[intake.FamilySoftware]) {
		t.Error("the governed software file diverges from the in-code v4 seed")
	}
	if got.Version != "v4" {
		t.Errorf("governed software file is %q, want v4", got.Version)
	}

	// Generic is nobody's business here: no operator finding touches it.
	generic, err := f.store.HouseObject(ctx, "intake/taxonomy/generic")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generic.OriginRef, "P3-GF3-BE1") {
		t.Errorf("the generic set moved off GF3's record: %q", generic.OriginRef)
	}
}

// TestGF7GF3StillWritesItsOwnFrozenBytes (T6, the snapshot half): GF3's Ensure
// must keep writing the content ITS record covers, not whatever the code says
// today — the live-pointer drift the snapshot doctrine closed (CONVENTIONS §57).
func TestGF7GF3StillWritesItsOwnFrozenBytes(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.gate.EnsureGF3TaxonomyGovernance(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-software.json"))
	if err != nil {
		t.Fatalf("governed software taxonomy fails LoadTaxonomy: %v", err)
	}
	if !reflect.DeepEqual(got, memory.GF3SoftwareTaxonomyForTest()) {
		t.Error("GF3's Ensure no longer writes the v3 content its record attests to")
	}
	if got.Version != "v3" {
		t.Errorf("GF3 wrote %q, want the v3 its gate record covers", got.Version)
	}
}

// TestGF7GovernanceBootsAreIdempotent: the three Ensures CHAIN. A second and
// third boot find nothing to do — an Ensure applies its packet's decision once.
func TestGF7GovernanceBootsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernanceGF7(t, f)

	before, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	for boot := 0; boot < 2; boot++ {
		rw12, err := f.gate.EnsureRW12TaxonomyGovernance(ctx)
		if err != nil || rw12.Superseded != 0 || rw12.Repaired != 0 {
			t.Fatalf("boot %d, RW-12: %+v err=%v", boot+2, rw12, err)
		}
		gf3, err := f.gate.EnsureGF3TaxonomyGovernance(ctx)
		if err != nil || gf3.Superseded != 0 || gf3.Repaired != 0 {
			t.Fatalf("boot %d, GF3: %+v err=%v", boot+2, gf3, err)
		}
		gf7, err := f.gate.EnsureGF7TaxonomyGovernance(ctx)
		if err != nil || gf7.Superseded != 0 || gf7.Repaired != 0 {
			t.Fatalf("boot %d, GF7: %+v err=%v", boot+2, gf7, err)
		}
	}
	after, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || after.ID != before.ID {
		t.Errorf("the software version moved across three boots: %s v%d then %s v%d",
			before.ID, before.Version, after.ID, after.Version)
	}
}

// TestGF7GateRefusalStands: ratification is PENDING, so the operator may REFUSE
// v4 at the planning-rework exit gate — and the refusal is executed as its own
// supersession back to the byte-exact v3 content. The next boot must leave that
// standing: the guard is the entry's version history, never its bytes.
func TestGF7GateRefusalStands(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernanceGF7(t, f)

	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	refused, err := json.MarshalIndent(memory.GF3SoftwareTaxonomyForTest(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	draft := memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindTaxonomy, Title: cur.Title,
		Content: string(refused) + "\n", TopicKey: cur.TopicKey, Selectors: cur.Selectors,
		FileBacked: true, FileName: filepath.Base(cur.FilePath),
	}
	refusal, err := f.gate.NewVersion(ctx, "darian", cur.ID, draft)
	if err != nil {
		t.Fatalf("execute the gate refusal: %v", err)
	}

	for boot := 0; boot < 2; boot++ {
		if res, err := f.gate.EnsureGF3TaxonomyGovernance(ctx); err != nil || res.Superseded != 0 {
			t.Fatalf("boot %d after the refusal, GF3: %+v err=%v", boot+1, res, err)
		}
		res, err := f.gate.EnsureGF7TaxonomyGovernance(ctx)
		if err != nil {
			t.Fatalf("boot %d after the refusal, GF7: %v", boot+1, err)
		}
		if res.Superseded != 0 {
			t.Fatalf("boot %d re-applied the revision the operator refused (superseded=%d)", boot+1, res.Superseded)
		}
	}
	after, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != refusal.ID {
		t.Fatalf("the active version is %q, want the operator's refusal %q", after.ID, refusal.ID)
	}
	if !strings.Contains(after.Content, `"version": "v3"`) {
		t.Error("the active content is not the v3 the refusal restored")
	}
}

// TestGF7GovernanceLeavesWhatIsNotItsOwn: a removed object stays dead, and a
// governed version this record does not attest to is left standing rather than
// reverted at the next boot.
func TestGF7GovernanceLeavesWhatIsNotItsOwn(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernanceGF7(t, f)

	if _, err := f.gate.Remove(ctx, "darian", "seed-intake-taxonomy-software", "not wanted"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	res, err := f.gate.EnsureGF7TaxonomyGovernance(ctx)
	if err != nil || res.Created != 0 || res.Superseded != 0 {
		t.Fatalf("ensure after removal: %+v err=%v", res, err)
	}
}
