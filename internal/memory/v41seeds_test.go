package memory_test

// v41seeds_test.go — the P3-V41 governance leg (brief P3/briefs/P3-V41.md §9;
// Spec S09.10 row 1, S09.8, S09.4 station 4; CONVENTIONS §57). Committed RED by
// the grounding agent (Amendment A) and green at the implementation commit.
//
// The headline these assert: a world that holds the GF7-governed v4 software
// set receives v4.1 at its next boot as its own SUPERSESSION under a record that
// says the operator RATIFIED it; a fresh world's chain reads v1 → v2 → v3 → v4 →
// v4.1; GF7's Ensure keeps writing the frozen v4 bytes its record covers; only
// the weight moved; and a decision made after this packet's stands.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
)

// gf7SoftwareDigest is the sha256 of the governed v4 software content the
// P3-GF7 record attests to (internal/memory/gf7seeds.go gf7ContentDigest).
// After P3-V41 it is the digest of the FROZEN snapshot, never of the live seed.
const gf7SoftwareDigest = "a4dcd1c049caaf6dc99707b6cd5da253dbf33e4c96ab64db1d22f7ce2bb53b22"

func v41SoftwarePath(f *fix) string {
	return filepath.Join(f.root, "house", "intake-taxonomy-software.json")
}

// bootTaxonomyGovernanceV41 runs the boot sequence in its wired order: B2 (it
// creates the entries), RW-12, GF3, GF7, then this packet's.
func bootTaxonomyGovernanceV41(t *testing.T, f *fix) memory.TaxonomyGovernanceResult {
	t.Helper()
	bootTaxonomyGovernanceGF7(t, f)
	res, err := f.gate.EnsureV41TaxonomyGovernance(context.Background())
	if err != nil {
		t.Fatalf("EnsureV41TaxonomyGovernance: %v", err)
	}
	return res
}

// TestV41SupersessionYieldsV41InAWorldThatHeldV4 (T1): the operator's live
// worlds hold v4 under GF7's PENDING record. The next boot supersedes it with
// v4.1 under this packet's RATIFIED record, chained by supersedes_id, the v4
// version retired (S09.8) — and the governed file, the operator-editable
// override input, says v4.1 with quality_bar at 12.
func TestV41SupersessionYieldsV41InAWorldThatHeldV4(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernanceGF7(t, f)

	// The world holds v4 (GF7's frozen bytes after this packet; the live seed
	// before it — both v4).
	held, err := intake.LoadTaxonomy(v41SoftwarePath(f))
	if err != nil {
		t.Fatalf("governed software taxonomy fails LoadTaxonomy: %v", err)
	}
	if held.Version != "v4" || held.Slot("quality_bar") == nil || held.Slot("quality_bar").Weight != 8 {
		t.Fatalf("precondition: the world holds %q with quality_bar %+v, want v4 at weight 8", held.Version, held.Slot("quality_bar"))
	}

	res, err := f.gate.EnsureV41TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureV41TaxonomyGovernance: %v", err)
	}
	if res.Superseded != 1 {
		t.Fatalf("superseded = %d, want the software set alone moved to v4.1", res.Superseded)
	}
	if res.Created != 0 || res.Repaired != 0 || res.Unverifiable != 0 {
		t.Errorf("clean world result = %+v, want one supersession and nothing else", res)
	}

	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatalf("HouseObject software: %v", err)
	}
	for _, want := range []string{"P3-V41", "RATIFIED", "2026-09-17", "rework-sitting-gate.md", "A2", "quality_bar"} {
		if !strings.Contains(cur.OriginRef, want) {
			t.Errorf("the v4.1 record is missing %q: %q", want, cur.OriginRef)
		}
	}
	if strings.Contains(cur.OriginRef, "PENDING") {
		t.Errorf("the v4.1 record says PENDING — it is the ratified one: %q", cur.OriginRef)
	}
	if cur.Supersedes == "" {
		t.Fatal("v4.1 chains to nothing")
	}
	prev, err := f.store.Get(ctx, cur.Supersedes)
	if err != nil {
		t.Fatalf("Get superseded %s: %v", cur.Supersedes, err)
	}
	if !strings.Contains(prev.OriginRef, "P3-GF7") {
		t.Errorf("v4.1 supersedes %q, want GF7's v4 version", prev.OriginRef)
	}
	if prev.Status != memory.StatusRetired {
		t.Errorf("the v4 version is %q, want retired (S09.8: superseded versions retired, never deleted)", prev.Status)
	}

	got, err := intake.LoadTaxonomy(v41SoftwarePath(f))
	if err != nil {
		t.Fatalf("governed software taxonomy fails LoadTaxonomy: %v", err)
	}
	if got.Version != "v4.1" {
		t.Errorf("governed software file is %q, want v4.1", got.Version)
	}
	if qb := got.Slot("quality_bar"); qb == nil || qb.Weight != 12 {
		t.Errorf("governed quality_bar = %+v, want weight 12", qb)
	}
	if !reflect.DeepEqual(got, intake.SeedTaxonomies()[intake.FamilySoftware]) {
		t.Error("the governed software file diverges from the in-code v4.1 seed")
	}

	generic, err := f.store.HouseObject(ctx, "intake/taxonomy/generic")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generic.OriginRef, "P3-GF3-BE1") {
		t.Errorf("the generic set moved off GF3's record: %q", generic.OriginRef)
	}
}

// TestV41FreshWorldChainAndIdempotence (T2): a fresh world defers until an
// operator exists (D10), then writes the whole chain v4.1 → v4 → v3 → v2, each
// under the record of the gate that owns it; later boots find nothing to do.
func TestV41FreshWorldChainAndIdempotence(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	if _, err := f.gate.EnsureV41TaxonomyGovernance(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")
	if res := bootTaxonomyGovernanceV41(t, f); res.Superseded != 1 {
		t.Fatalf("fresh world: %+v, want one v4.1 supersession", res)
	}

	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	entry := cur
	for i, packet := range []string{"P3-V41", "P3-GF7", "P3-GF3-BE1", "P3-RW-12"} {
		if !strings.Contains(entry.OriginRef, packet) {
			t.Fatalf("chain position %d is %q, want %s's version", i, entry.OriginRef, packet)
		}
		if packet == "P3-RW-12" {
			break
		}
		prev, err := f.store.Get(ctx, entry.Supersedes)
		if err != nil {
			t.Fatalf("Get superseded %q at %s: %v", entry.Supersedes, packet, err)
		}
		if prev.Status != memory.StatusRetired {
			t.Errorf("the version %s superseded is %q, want retired", packet, prev.Status)
		}
		entry = prev
	}

	for boot := 0; boot < 2; boot++ {
		for name, ensure := range map[string]func(context.Context) (memory.TaxonomyGovernanceResult, error){
			"RW-12": f.gate.EnsureRW12TaxonomyGovernance, "GF3": f.gate.EnsureGF3TaxonomyGovernance,
			"GF7": f.gate.EnsureGF7TaxonomyGovernance, "V41": f.gate.EnsureV41TaxonomyGovernance,
		} {
			if res, err := ensure(ctx); err != nil || res.Superseded != 0 || res.Repaired != 0 || res.Created != 0 {
				t.Fatalf("boot %d, %s: %+v err=%v", boot+2, name, res, err)
			}
		}
	}
	after, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != cur.ID {
		t.Errorf("the software version moved across three boots: %s then %s", cur.ID, after.ID)
	}
}

// TestV41GF7StillWritesItsOwnFrozenV4Bytes (T3, the snapshot half; CONVENTIONS
// §57): GF7's Ensure keeps writing the content ITS record covers — the bytes
// that hash to gf7ContentDigest — not whatever the code says today. GREEN at v4
// (the live seed IS that content) and it must stay green once the live seed is
// v4.1, which is exactly what the frozen snapshot is for.
func TestV41GF7StillWritesItsOwnFrozenV4Bytes(t *testing.T) {
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernanceGF7(t, f)
	raw, err := os.ReadFile(v41SoftwarePath(f))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != gf7SoftwareDigest {
		t.Errorf("GF7's Ensure wrote content hashing %s, want the digest its record pins %s — it must write the frozen v4 snapshot", got, gf7SoftwareDigest)
	}
	got, err := intake.LoadTaxonomy(v41SoftwarePath(f))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "v4" || got.Slot("quality_bar").Weight != 8 {
		t.Errorf("GF7 wrote %q with quality_bar %d, want the v4 / weight-8 content its record covers", got.Version, got.Slot("quality_bar").Weight)
	}
}

// TestV41OnlyTheWeightMoved (T4): the governed v4 content (what GF7's chain
// writes) and the live v4.1 seed differ in exactly three places — the version
// string, the quality_bar weight, and the Source, where the v4 record's closing
// "PENDING" sentence flips and the v4.1 record is appended; everything before
// that sentence is byte-identical. RED at v4: the live seed is v4.
func TestV41OnlyTheWeightMoved(t *testing.T) {
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernanceGF7(t, f)
	v4, err := intake.LoadTaxonomy(v41SoftwarePath(f))
	if err != nil {
		t.Fatal(err)
	}
	live := intake.SeedTaxonomies()[intake.FamilySoftware]
	if live.Version != "v4.1" {
		t.Errorf("live seed version = %q, want v4.1", live.Version)
	}
	const pending = "Operator ratification PENDING at the planning-rework exit gate."
	if !strings.HasSuffix(v4.Source, pending) {
		t.Fatalf("the governed v4 Source does not end with the PENDING sentence this packet flips: %q", v4.Source)
	}
	if prefix := strings.TrimSuffix(v4.Source, pending); !strings.HasPrefix(live.Source, prefix) {
		t.Errorf("the v4.1 Source does not carry the v4 record verbatim up to the sentence that flips")
	}
	norm := *live
	norm.Version, norm.Source = v4.Version, v4.Source
	norm.Slots = append([]intake.Slot(nil), live.Slots...)
	if qb := norm.Slot("quality_bar"); qb != nil {
		qb.Weight = 8
	}
	if !reflect.DeepEqual(&norm, v4) {
		t.Errorf("something other than the version, the Source and the quality_bar weight moved between v4 and v4.1")
	}
}

// TestV41LaterDecisionsStandAndHandEditsRepair (T5; S09.8, CONVENTIONS §57):
// (a) a version the operator writes through the knowledge gate after v4.1 is a
// LATER decision — a boot never reverts it; (b) an edit made to the governed
// FILE with no gate row is not a decision — the boot repairs the file from the
// row it is committed with, mints no version, and counts the repair.
func TestV41LaterDecisionsStandAndHandEditsRepair(t *testing.T) {
	ctx := context.Background()

	t.Run("a governed edit after v4.1 stands", func(t *testing.T) {
		f := newFix(t)
		f.user("darian", "operator")
		bootTaxonomyGovernanceV41(t, f)
		cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
		if err != nil {
			t.Fatal(err)
		}
		edited := *intake.SeedTaxonomies()[intake.FamilySoftware]
		edited.Version = "v4.1-operator"
		edited.Slot("quality_bar").Weight = 14 // the operator's own weight (S06.5 operator-editable)
		raw, err := json.MarshalIndent(&edited, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		mine, err := f.gate.NewVersion(ctx, "darian", cur.ID, memory.Draft{
			Scope: memory.ScopeHouse, Kind: memory.KindTaxonomy, Title: cur.Title,
			Content: string(raw) + "\n", TopicKey: cur.TopicKey, Selectors: cur.Selectors,
			FileBacked: true, FileName: filepath.Base(cur.FilePath),
		})
		if err != nil {
			t.Fatalf("operator NewVersion: %v", err)
		}
		for boot := 0; boot < 2; boot++ {
			if res, err := f.gate.EnsureGF7TaxonomyGovernance(ctx); err != nil || res.Superseded != 0 || res.Repaired != 0 {
				t.Fatalf("boot %d, GF7: %+v err=%v", boot+1, res, err)
			}
			if res, err := f.gate.EnsureV41TaxonomyGovernance(ctx); err != nil || res.Superseded != 0 || res.Repaired != 0 {
				t.Fatalf("boot %d re-applied over the operator's version: %+v err=%v", boot+1, res, err)
			}
		}
		after, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
		if err != nil {
			t.Fatal(err)
		}
		if after.ID != mine.ID {
			t.Fatalf("the active version is %q, want the operator's %q", after.ID, mine.ID)
		}
	})

	t.Run("a raw file edit is repaired from the row", func(t *testing.T) {
		f := newFix(t)
		f.user("darian", "operator")
		bootTaxonomyGovernanceV41(t, f)
		before, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
		if err != nil {
			t.Fatal(err)
		}
		torn := strings.Replace(before.Content, `"weight": 12`, `"weight": 9`, 1)
		if torn == before.Content {
			t.Fatal("fixture: the v4.1 content carries no weight-12 to tear")
		}
		if err := os.WriteFile(v41SoftwarePath(f), []byte(torn), 0o600); err != nil {
			t.Fatal(err)
		}
		res, err := f.gate.EnsureV41TaxonomyGovernance(ctx)
		if err != nil {
			t.Fatalf("EnsureV41TaxonomyGovernance: %v", err)
		}
		if res.Repaired != 1 || res.Superseded != 0 || res.Created != 0 {
			t.Fatalf("result after a raw file edit = %+v, want exactly one repair and no new version", res)
		}
		raw, err := os.ReadFile(v41SoftwarePath(f))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != before.Content {
			t.Error("the governed file was not restored to the content its row is committed with")
		}
		after, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
		if err != nil {
			t.Fatal(err)
		}
		if after.ID != before.ID || after.Version != before.Version {
			t.Errorf("a repair minted a version: %s v%d → %s v%d", before.ID, before.Version, after.ID, after.Version)
		}
	})
}
