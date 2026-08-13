package memory_test

// rw12seeds_test.go — the P3-RW-12 §7 T11 leg: the Deep-Plan taxonomies enter
// the 8.3 knowledge gate (Spec S09.10 row 1, S09.4 station 4, S09.8).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/retention"
)

// rw12Families are the four families whose question sets P3-RW-12 seeds.
var rw12Families = []intake.Family{
	intake.FamilyResearch, intake.FamilyContent, intake.FamilyData, intake.FamilyChore,
}

// TestTaxonomyGovernanceCreatesAndSupersedes (§7 T11; R5): the four new
// question sets become governed house objects, the software set moves to v2
// by SUPERSESSION rather than an in-place edit, every governed file is the
// strict JSON the operator can edit, and a boot never resurrects what someone
// removed.
func TestTaxonomyGovernanceCreatesAndSupersedes(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()

	// House scope needs its D10 holder; without one the boot skips and retries.
	if _, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")

	// The world as it stands after B2: the software set governed at its
	// B2-ratified v1 content.
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	b2Software, err := f.store.Get(ctx, "seed-intake-taxonomy-software")
	if err != nil {
		t.Fatalf("Get software seed: %v", err)
	}
	if !strings.Contains(b2Software.OriginRef, "B2-report.md") {
		t.Fatalf("B2 entry provenance = %q", b2Software.OriginRef)
	}

	res, err := f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureRW12TaxonomyGovernance: %v", err)
	}
	if res.Created != 4 {
		t.Errorf("created = %d, want the 4 new family taxonomies", res.Created)
	}
	if res.Superseded != 1 {
		t.Errorf("superseded = %d, want the software set moved to v2", res.Superseded)
	}
	if res.Repaired != 0 {
		t.Errorf("repaired = %d on a clean world, want 0", res.Repaired)
	}

	for _, fam := range rw12Families {
		id := "seed-intake-taxonomy-" + string(fam)
		e, err := f.store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if e.Scope != memory.ScopeHouse || e.Kind != memory.KindTaxonomy ||
			e.Status != memory.StatusActive || e.Origin != memory.OriginImported {
			t.Errorf("%s = %+v", id, e)
		}
		if e.TopicKey != "intake/taxonomy/"+string(fam) {
			t.Errorf("%s topic key = %q", id, e.TopicKey)
		}
		if e.Selectors.TaskType != "machinery:intake" {
			t.Errorf("%s selector = %+v, want the machinery marker (never a stage brief)", id, e.Selectors)
		}
		if !strings.Contains(e.OriginRef, "P3-RW-12") {
			t.Errorf("%s provenance = %q, want the RW-12 ratification record", id, e.OriginRef)
		}
		if e.FilePath != filepath.Join("house", "intake-taxonomy-"+string(fam)+".json") {
			t.Errorf("%s file path = %q", id, e.FilePath)
		}
	}

	// Supersession, not an edit: the B2 row is RETIRED and the new active
	// version chains to it (S09.8).
	old, err := f.store.Get(ctx, "seed-intake-taxonomy-software")
	if err != nil {
		t.Fatalf("Get retired software seed: %v", err)
	}
	if old.Status != memory.StatusRetired {
		t.Errorf("B2 software entry status = %q, want retired", old.Status)
	}
	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatalf("HouseObject software: %v", err)
	}
	if cur.Supersedes != old.ID {
		t.Errorf("new software version supersedes %q, want %q", cur.Supersedes, old.ID)
	}
	if cur.Version != old.Version+1 {
		t.Errorf("new software version = %d, want %d", cur.Version, old.Version+1)
	}
	if !strings.Contains(cur.OriginRef, "P3-RW-12") {
		t.Errorf("superseding version provenance = %q", cur.OriginRef)
	}

	// Every governed active file IS the operator-editable override input, and
	// it says exactly what the runtime seed says (the §17 proven-by-test rule).
	seeds := intake.SeedTaxonomies()
	for _, fam := range append([]intake.Family{intake.FamilySoftware, intake.FamilyGeneric}, rw12Families...) {
		path := filepath.Join(f.root, "house", "intake-taxonomy-"+string(fam)+".json")
		got, err := intake.LoadTaxonomy(path)
		if err != nil {
			t.Fatalf("governed %s taxonomy fails LoadTaxonomy: %v", fam, err)
		}
		if !reflect.DeepEqual(got, seeds[fam]) {
			t.Errorf("governed %s file diverges from the in-code seed", fam)
		}
	}

	// Idempotent across boots: nothing created, nothing superseded twice.
	res, err = f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil || res.Created != 0 || res.Superseded != 0 || res.Repaired != 0 {
		t.Fatalf("second ensure: %+v err=%v", res, err)
	}

	// Governance is real: a removed object stays dead.
	if _, err := f.gate.Remove(ctx, "darian", "seed-intake-taxonomy-research", "not wanted"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	res, err = f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil || res.Created != 0 {
		t.Fatalf("ensure after removal: %+v err=%v", res, err)
	}
	e, err := f.store.Get(ctx, "seed-intake-taxonomy-research")
	if err != nil || e.Status != memory.StatusRemoved {
		t.Fatalf("removed entry = %+v (%v)", e, err)
	}
}

// TestB2GovernedTaxonomiesAreTheRatifiedSnapshot (§11 governance-divergence
// trap; drain r1 F3): BOTH B2 taxonomy entries record what the B2 gate
// ACTUALLY ratified. A fresh world must not write today's content under the B2
// provenance — the ratification record is a snapshot, never a live pointer at
// whatever the code says now.
//
// The generic half is the one that matters going forward: P3-RW-12 left the
// generic set unchanged, so the frozen fixture and the live seed agree TODAY.
// That is exactly why it is pinned here — the packet that eventually edits
// generic must trip this test and freeze its own snapshot, instead of silently
// re-dating the B2 gate.
func TestB2GovernedTaxonomiesAreTheRatifiedSnapshot(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("darian", "operator")
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	for _, tc := range []struct {
		fam      intake.Family
		version  string
		slots    int
		ratified string
	}{
		{intake.FamilySoftware, "v1", 10, "the ClarifyCodeBench 10"},
		{intake.FamilyGeneric, "v1", 8, "the 8-slot generic fallback"},
	} {
		got, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-"+string(tc.fam)+".json"))
		if err != nil {
			t.Fatalf("LoadTaxonomy %s: %v", tc.fam, err)
		}
		if got.Version != tc.version {
			t.Errorf("B2-governed %s taxonomy is %q; the B2 gate ratified %s", tc.fam, got.Version, tc.version)
		}
		if len(got.Slots) != tc.slots {
			t.Errorf("B2-governed %s taxonomy carries %d slots; the B2 gate ratified %s", tc.fam, len(got.Slots), tc.ratified)
		}
	}
}

// TestGovernanceDecidesFromTheRowNotTheFile (drain r1 F4): the crash window in
// writeEntry — file placed, transaction not yet committed — must not become a
// permanent silent false record.
//
// The torn state is simulated exactly: the governed file is overwritten with
// the NEW bytes while the OLD row stays active, which is precisely what a
// crash between those two steps leaves behind. A boot that compared the FILE
// would find it already equal to the in-code seed, skip the supersession, and
// keep skipping it forever, leaving the active row attesting content it does
// not have.
func TestGovernanceDecidesFromTheRowNotTheFile(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("darian", "operator")
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	before, err := f.store.Get(ctx, "seed-intake-taxonomy-software")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Tear the write: v2 bytes on disk, v1 row still active.
	v2, err := json.MarshalIndent(intake.SeedTaxonomies()[intake.FamilySoftware], "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.root, "house", "intake-taxonomy-software.json")
	if err := os.WriteFile(path, append(v2, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureRW12TaxonomyGovernance: %v", err)
	}
	if res.Repaired != 1 {
		t.Errorf("repaired = %d, want 1 — the torn write must be noticed, not absorbed", res.Repaired)
	}
	if res.Superseded != 1 {
		t.Fatalf("superseded = %d, want 1 — deciding from the file would have skipped this forever", res.Superseded)
	}
	old, err := f.store.Get(ctx, before.ID)
	if err != nil {
		t.Fatalf("Get old version: %v", err)
	}
	if old.Status != memory.StatusRetired {
		t.Errorf("torn-state old version status = %q, want retired", old.Status)
	}
	cur, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
	if err != nil {
		t.Fatalf("HouseObject: %v", err)
	}
	if cur.Supersedes != before.ID || !strings.Contains(cur.OriginRef, "P3-RW-12") {
		t.Errorf("repaired version = supersedes %q origin %q", cur.Supersedes, cur.OriginRef)
	}
	if cur.Content != string(v2)+"\n" {
		t.Error("the active version's content does not match the file it governs")
	}
	// And it settles: a following boot finds nothing to do.
	if res, err := f.gate.EnsureRW12TaxonomyGovernance(ctx); err != nil ||
		res.Created != 0 || res.Superseded != 0 || res.Repaired != 0 {
		t.Fatalf("boot after repair: %+v err=%v", res, err)
	}
}

// TestGovernanceSkipsWhenProvenanceIsUnverifiable (drain r2, R1): when the
// committed content hash cannot be read, the entry is LEFT ALONE — loudly,
// never by silently trusting the file instead, and never by failing the boot.
//
// Both reachable shapes are covered. What is NOT simulated here, because the
// database structurally refuses it, is the S14.9 compaction UPDATE itself:
// `run_events.ts` is an identity column (0015 run_events_identity_immutable)
// so a row cannot be backdated past the trigger's one-month floor, and DELETE
// is refused outright. Measured alongside: a knowledge.write payload is ~240
// bytes against retention's 1024-byte BulkPayloadFloorBytes, so the compaction
// pass does not select these rows today at all. The posture is implemented
// anyway — that floor is an interim structural constant, and the no-record
// branch below is reachable regardless of compaction.
func TestGovernanceSkipsWhenProvenanceIsUnverifiable(t *testing.T) {
	// A compacted body carries no entry_id, which is WHY elision lands on the
	// no-matching-row branch rather than on a partial read.
	if strings.Contains(retention.CompactedPayload, "entry_id") {
		t.Fatal("the compacted marker now carries an entry_id — re-derive which branch elision reaches")
	}

	for _, tc := range []struct {
		name   string
		tamper func(t *testing.T, f *fix, entryID string)
	}{
		{
			// The provenance row is readable but its body no longer carries
			// the hash — what an elided/rewritten record leaves behind. This
			// is the shape that used to scan SQL NULL into a string and take
			// the whole boot down.
			"provenance body carries no hash",
			func(t *testing.T, f *fix, entryID string) {
				payload := fmt.Sprintf(`{"entry_id":%q,"scope":"house","kind":"taxonomy"}`, entryID)
				if _, err := f.log.Append(context.Background(), eventlog.Append{
					UserID: "darian", Type: memory.EventWrite, SchemaVersion: 1,
					Payload: []byte(payload),
				}); err != nil {
					t.Fatalf("append bodiless provenance event: %v", err)
				}
			},
		},
		{
			// No provenance event for the ACTIVE version at all — a restored
			// or imported world, or one whose log no longer holds it. This is
			// the branch that used to fall back to trusting the file, which is
			// exactly the posture drain r1 removed.
			"no provenance event for the active version",
			func(t *testing.T, f *fix, entryID string) {
				insertUngovernedVersion(t, f, entryID)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFix(t)
			ctx := context.Background()
			f.user("darian", "operator")
			if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
				t.Fatalf("EnsureB2SeedGovernance: %v", err)
			}
			before, err := f.store.HouseObject(ctx, "intake/taxonomy/software")
			if err != nil {
				t.Fatalf("HouseObject: %v", err)
			}
			tc.tamper(t, f, before.ID)

			res, err := f.gate.EnsureRW12TaxonomyGovernance(ctx)
			if err != nil {
				t.Fatalf("an unreadable provenance hash must never fail the boot: %v", err)
			}
			if res.Unverifiable == 0 {
				t.Errorf("unverifiable = 0 — an unreadable record must be reported, not absorbed")
			}
			if res.Superseded != 0 {
				t.Errorf("superseded = %d — with no readable record there is nothing to compare against, "+
					"and the FILE is not a substitute (the posture drain r1 removed)", res.Superseded)
			}
			if res.Repaired != 0 {
				t.Errorf("repaired = %d — nothing may be rewritten on an unverifiable boot", res.Repaired)
			}
			if res.Created != 4 {
				t.Errorf("created = %d, want the 4 new families — they need no prior record", res.Created)
			}
			// The governed file is left exactly as the B2 gate wrote it.
			got, err := intake.LoadTaxonomy(filepath.Join(f.root, "house", "intake-taxonomy-software.json"))
			if err != nil {
				t.Fatalf("LoadTaxonomy: %v", err)
			}
			if got.Version != "v1" {
				t.Errorf("governed file = %q; nothing may be rewritten on an unverifiable boot", got.Version)
			}
		})
	}
}

// insertUngovernedVersion adds a HIGHER-version active row for the same house
// topic — so HouseObject serves it — carrying no knowledge.write event of its
// own. It writes a row the gate would have written and nothing else, which is
// what a restored database or a pruned log leaves behind.
func insertUngovernedVersion(t *testing.T, f *fix, supersedes string) {
	t.Helper()
	execSQL(t, f, `
		INSERT INTO knowledge_entries (
			entry_id, user_id, scope, scope_ref, layer, kind, title,
			content, file_path, topic_key, selectors, status, version,
			supersedes_id, origin, origin_ref, proposer_model,
			approved_by, approved_ts, verified_by, verified_ts,
			reverify_interval_days, created_ts, updated_ts)
		SELECT 'restored-software-version', user_id, scope, scope_ref, layer, kind, title,
			content, file_path, topic_key, selectors, 'active', 99,
			?, origin, origin_ref, proposer_model,
			approved_by, approved_ts, verified_by, verified_ts,
			reverify_interval_days, created_ts, updated_ts
		  FROM knowledge_entries WHERE entry_id = ?`, supersedes, supersedes)
	execSQL(t, f, `UPDATE knowledge_entries SET status = 'retired' WHERE entry_id = ?`, supersedes)
}

// execSQL runs one statement through the write path.
func execSQL(t *testing.T, f *fix, query string, args ...any) {
	t.Helper()
	if err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), query, args...)
		return err
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
}
