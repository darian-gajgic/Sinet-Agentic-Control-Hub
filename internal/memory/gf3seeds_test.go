package memory_test

// gf3seeds_test.go — the P3-GF3-BE1 governance leg (brief §8 T8; Spec S09.10
// row 1, S09.4 station 4, S09.8; CONVENTIONS §57).
//
// The headline these assert: the v3 question sets enter the record as their
// OWN version under their own record, which says the operator has not ratified
// them yet; RW-12's record still says exactly what its gate approved; the two
// Ensures chain instead of fighting, so a boot is a no-op after the first; and
// governance is still governance — a removed object stays dead and a governed
// file is still the operator-editable override input.

import (
	"context"
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

// gf3Revised are the two families this packet revised.
var gf3Revised = []intake.Family{intake.FamilySoftware, intake.FamilyGeneric}

// bootTaxonomyGovernance runs the boot sequence in its wired order: B2 first
// (it creates the entries), then RW-12, then GF3.
func bootTaxonomyGovernance(t *testing.T, f *fix) memory.TaxonomyGovernanceResult {
	t.Helper()
	ctx := context.Background()
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	if _, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); err != nil {
		t.Fatalf("EnsureRW12TaxonomyGovernance: %v", err)
	}
	res, err := f.gate.EnsureGF3TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureGF3TaxonomyGovernance: %v", err)
	}
	return res
}

// TestGF3TaxonomyGovernanceSupersedesUnderAPendingRecord (T8): a fresh world
// ends with the v3 content governed, chained onto the version RW-12 ratified,
// under a record that states ratification is PENDING at the resumed gate.
func TestGF3TaxonomyGovernanceSupersedesUnderAPendingRecord(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)

	// House scope needs its D10 holder; without one the boot defers and retries.
	if _, err := f.gate.EnsureGF3TaxonomyGovernance(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")

	res := bootTaxonomyGovernance(t, f)
	if res.Superseded != len(gf3Revised) {
		t.Fatalf("superseded = %d, want the %d revised sets moved to v3", res.Superseded, len(gf3Revised))
	}
	if res.Created != 0 || res.Repaired != 0 || res.Unverifiable != 0 {
		t.Errorf("clean world result = %+v, want supersessions only", res)
	}

	// The content THIS record covers, which since P3-GF7 is the frozen v3
	// snapshot for software rather than the live seed: a ratification record
	// that follows the code is not a record (CONVENTIONS §57).
	seeds := map[intake.Family]*intake.Taxonomy{
		intake.FamilySoftware: memory.GF3SoftwareTaxonomyForTest(),
		intake.FamilyGeneric:  intake.SeedTaxonomies()[intake.FamilyGeneric],
	}
	for _, fam := range gf3Revised {
		cur, err := f.store.HouseObject(ctx, "intake/taxonomy/"+string(fam))
		if err != nil {
			t.Fatalf("HouseObject %s: %v", fam, err)
		}
		if !strings.Contains(cur.OriginRef, "P3-GF3-BE1") {
			t.Errorf("%s active version provenance = %q, want this packet's record", fam, cur.OriginRef)
		}
		if !strings.Contains(cur.OriginRef, "PENDING") {
			t.Errorf("%s record does not say ratification is pending: %q", fam, cur.OriginRef)
		}
		// Supersession, never an in-place edit (S09.8): the version it replaced
		// is retired, chained, and still attributed to the gate that approved it.
		if cur.Supersedes == "" {
			t.Fatalf("%s v3 chains to nothing", fam)
		}
		old, err := f.store.Get(ctx, cur.Supersedes)
		if err != nil {
			t.Fatalf("Get superseded %s: %v", fam, err)
		}
		if old.Status != memory.StatusRetired {
			t.Errorf("%s superseded version status = %q, want retired", fam, old.Status)
		}
		if cur.Version != old.Version+1 {
			t.Errorf("%s version = %d, want %d", fam, cur.Version, old.Version+1)
		}
		// The governed file IS the operator-editable override input, and it
		// says what the runtime serves.
		got, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-"+string(fam)+".json"))
		if err != nil {
			t.Fatalf("governed %s taxonomy fails LoadTaxonomy: %v", fam, err)
		}
		if !reflect.DeepEqual(got, seeds[fam]) {
			t.Errorf("governed %s file diverges from the v3 content this record covers", fam)
		}
		if got.Version != "v3" {
			t.Errorf("governed %s file is %q, want v3", fam, got.Version)
		}
	}

	// RW-12's record still says what its gate ratified: the software version it
	// wrote is the v2 content, not this packet's.
	softCur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	rw12Version, err := f.store.Get(ctx, softCur.Supersedes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rw12Version.OriginRef, "P3-RW-12") {
		t.Fatalf("the version GF3 superseded is not RW-12's: %q", rw12Version.OriginRef)
	}
	// What that version was committed WITH is the frozen v2 content, not
	// whatever the code says today — the record read from the row's own write
	// event, which is the half a torn file cannot move. (The retired version's
	// bytes are not on disk: a new version reuses the canonical file name, so
	// superseded content lives in git history, S09.8.)
	var committed string
	if err := f.db.QueryRowContext(ctx, `
		SELECT json_extract(payload, '$.content_hash') FROM run_events
		 WHERE type = ? AND json_extract(payload, '$.entry_id') = ?
		 ORDER BY event_seq DESC LIMIT 1`, memory.EventWrite, rw12Version.ID).Scan(&committed); err != nil {
		t.Fatalf("read RW-12 write event: %v", err)
	}
	if committed != memory.RW12SoftwareContentHashForTest() {
		t.Errorf("RW-12's governed software version was committed with %s, want the frozen v2 content %s",
			committed, memory.RW12SoftwareContentHashForTest())
	}
}

// TestGF3GovernanceBootsAreIdempotent: the two Ensures CHAIN. A second boot
// finds nothing to do — neither writes over the other, which a content-only
// trigger would have done forever, two versions per boot.
func TestGF3GovernanceBootsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernance(t, f)

	versions := func() map[string]int64 {
		t.Helper()
		out := map[string]int64{}
		for _, fam := range gf3Revised {
			cur, err := f.store.HouseObject(ctx, "intake/taxonomy/"+string(fam))
			if err != nil {
				t.Fatalf("HouseObject %s: %v", fam, err)
			}
			out[string(fam)] = cur.Version
		}
		return out
	}
	before := versions()

	for boot := 0; boot < 2; boot++ {
		rw12, err := f.gate.EnsureRW12TaxonomyGovernance(ctx)
		if err != nil || rw12.Created != 0 || rw12.Superseded != 0 || rw12.Repaired != 0 {
			t.Fatalf("boot %d, RW-12 ensure: %+v err=%v — a settled record is not rewritten", boot+2, rw12, err)
		}
		gf3, err := f.gate.EnsureGF3TaxonomyGovernance(ctx)
		if err != nil || gf3.Created != 0 || gf3.Superseded != 0 || gf3.Repaired != 0 {
			t.Fatalf("boot %d, GF3 ensure: %+v err=%v", boot+2, gf3, err)
		}
	}
	if after := versions(); !reflect.DeepEqual(before, after) {
		t.Errorf("versions moved across three boots: %v then %v", before, after)
	}
}

// TestGF3GateRefusalStands (drain r1 F1, the evaluator's probe): ratification
// is PENDING, so the gate may REFUSE v3 — and the refusal is executed as a
// supersession back to the byte-exact content RW-12 ratified. The next boot
// must leave that standing.
//
// A content-comparing Ensure could not: the refusal restores exactly the
// predecessor content it was written against, so it reads as "not revised yet"
// and the boot re-applies the revision the operator just rejected. Any governed
// edit that byte-equals a snapshot has the same shape, which is why the guard
// is the entry's version history rather than its bytes.
func TestGF3GateRefusalStands(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernance(t, f)

	// The gate refuses: the operator supersedes back to the frozen v2 content.
	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	refused, err := json.MarshalIndent(memory.RW12SoftwareTaxonomyForTest(), "", "  ")
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

	// Two more boots: neither Ensure may touch it.
	for boot := 0; boot < 2; boot++ {
		if res, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); err != nil || res.Superseded != 0 {
			t.Fatalf("boot %d after the refusal, RW-12: %+v err=%v", boot+1, res, err)
		}
		res, err := f.gate.EnsureGF3TaxonomyGovernance(ctx)
		if err != nil {
			t.Fatalf("boot %d after the refusal, GF3: %v", boot+1, err)
		}
		if res.Superseded != 0 {
			t.Fatalf("boot %d re-applied the revision the operator refused (superseded=%d) — the gate's decision must stand",
				boot+1, res.Superseded)
		}
	}
	after, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != refusal.ID {
		t.Fatalf("the active version is %q, want the operator's refusal %q", after.ID, refusal.ID)
	}
	if !strings.Contains(after.Content, `"version": "v2"`) {
		t.Error("the active content is not the v2 the refusal restored")
	}
	got, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-software.json"))
	if err != nil {
		t.Fatalf("the governed file after a refusal fails LoadTaxonomy: %v", err)
	}
	if !reflect.DeepEqual(got, memory.RW12SoftwareTaxonomyForTest()) {
		t.Error("the governed file does not hold the refused-back-to content")
	}
}

// TestGF3EachEnsureMintsExactlyOneVersion (drain r1 F1(b)): across repeated
// boots each record appears exactly once in the chain — the version history is
// a list of decisions, not a boot counter.
func TestGF3EachEnsureMintsExactlyOneVersion(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernance(t, f)
	for boot := 0; boot < 3; boot++ {
		if _, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); err != nil {
			t.Fatalf("boot %d RW-12: %v", boot+2, err)
		}
		if _, err := f.gate.EnsureGF3TaxonomyGovernance(ctx); err != nil {
			t.Fatalf("boot %d GF3: %v", boot+2, err)
		}
	}
	for _, fam := range gf3Revised {
		for _, packet := range []string{"P3-RW-12", "P3-GF3-BE1"} {
			var n int
			if err := f.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM knowledge_entries WHERE topic_key = ? AND origin_ref LIKE ?`,
				"intake/taxonomy/"+string(fam), "%"+packet+"%").Scan(&n); err != nil {
				t.Fatal(err)
			}
			// Generic was unchanged by RW-12, so that record legitimately minted
			// nothing for it; nobody may mint more than one.
			if n > 1 {
				t.Errorf("%s carries %d versions under %s — an Ensure applies its decision once", fam, n, packet)
			}
			if packet == "P3-GF3-BE1" && n != 1 {
				t.Errorf("%s carries %d versions under %s, want exactly 1", fam, n, packet)
			}
		}
	}
}

// TestGF3GovernanceLeavesWhatIsNotItsOwn: governance is real. A removed object
// stays removed, and a governed version neither record attests to — an
// operator's own edit through the gate — is left standing rather than reverted
// at the next boot.
func TestGF3GovernanceLeavesWhatIsNotItsOwn(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernance(t, f)

	// The operator edits the generic set through the gate: a new version, their
	// own content, their own provenance.
	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/generic")
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(cur.Content, `"version": "v3"`, `"version": "operator-1"`, 1)
	if edited == cur.Content {
		t.Fatal("fixture drift: the governed generic content does not carry a v3 version line")
	}
	draft := memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindTaxonomy, Title: cur.Title,
		Content: edited, TopicKey: cur.TopicKey, Selectors: cur.Selectors,
		FileBacked: true, FileName: filepath.Base(cur.FilePath),
	}
	if _, err := f.gate.NewVersion(ctx, "darian", cur.ID, draft); err != nil {
		t.Fatalf("NewVersion: %v", err)
	}

	if _, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); err != nil {
		t.Fatalf("RW-12 ensure after the operator edit: %v", err)
	}
	if res, err := f.gate.EnsureGF3TaxonomyGovernance(ctx); err != nil || res.Superseded != 0 {
		t.Fatalf("GF3 ensure after the operator edit: %+v err=%v", res, err)
	}
	after, err := f.store.HouseObject(ctx, "intake/taxonomy/generic")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after.Content, `"version": "operator-1"`) {
		t.Error("a boot reverted the operator's own governed edit — an Ensure moves only the content it was written against")
	}

	// And a removed object stays dead.
	if _, err := f.gate.Remove(ctx, "darian", "seed-intake-taxonomy-software", "not wanted"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if res, err := f.gate.EnsureGF3TaxonomyGovernance(ctx); err != nil || res.Created != 0 || res.Superseded != 0 {
		t.Fatalf("ensure after removal: %+v err=%v", res, err)
	}
}

// TestGF3GovernanceRepairsATornFileItOwns (the drain r1 F4 posture, carried):
// when the ROW already holds this packet's content and only the disk disagrees,
// the file is repaired from the row and no version is minted.
func TestGF3GovernanceRepairsATornFileItOwns(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	bootTaxonomyGovernance(t, f)

	path := filepath.Join(f.root, "house", "intake-taxonomy-software.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := f.gate.EnsureGF3TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureGF3TaxonomyGovernance: %v", err)
	}
	if res.Repaired != 1 || res.Superseded != 0 {
		t.Fatalf("torn-file result = %+v, want one repair and no new version", res)
	}
	got, err := intake.LoadTaxonomy(path)
	if err != nil {
		t.Fatalf("repaired file fails LoadTaxonomy: %v", err)
	}
	if !reflect.DeepEqual(got, memory.GF3SoftwareTaxonomyForTest()) {
		t.Error("the repaired file is not the content the row attests to")
	}
}
