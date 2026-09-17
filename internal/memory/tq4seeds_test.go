package memory_test

// tq4seeds_test.go — the P3-TQ-4a composer-playbook governance leg (Spec S09.10
// row 1, S09.8, S09.4 station 4; CONVENTIONS §57), added at drain r1 F1/F2.
//
// The headline these assert: a world that holds the B3-governed seed-1 playbook
// receives seed-2 at its next boot as its own SUPERSESSION under a record that
// says ratification is owed; a fresh world's chain reads seed-1 → seed-2; B3's
// Ensure keeps writing the frozen seed-1 bytes ITS record covers rather than
// whatever the code says today; and a decision made after this packet's stands.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// tq4Seed1Content is the sha256 of the composer playbook as the B3 record
// covers it — the bytes ComposerPlaybookSeed() rendered at main 3a5e706, before
// this packet revised it.
const tq4Seed1Content = "f8714e78c02bdf033948b977cba032f8034261252c21c71bc9b8b8b52dcb8dd2"

func tq4PlaybookPath(f *fix) string {
	return filepath.Join(f.root, "house", "composer-playbook.md")
}

// bootPlaybookGovernance runs the boot sequence in its wired order: B3's
// seeding (it creates the entry), then this packet's supersession.
func bootPlaybookGovernance(t *testing.T, f *fix) memory.PlaybookGovernanceResult {
	t.Helper()
	if _, err := f.gate.EnsureComposerPlaybook(context.Background()); err != nil {
		t.Fatalf("EnsureComposerPlaybook: %v", err)
	}
	res, err := f.gate.EnsureTQ4KnowledgeGovernance(context.Background())
	if err != nil {
		t.Fatalf("EnsureTQ4KnowledgeGovernance: %v", err)
	}
	return res
}

// TestTQ4SupersessionYieldsSeed2InAWorldThatHeldSeed1 (F2): the operator's live
// worlds hold seed-1 under B3's record. The next boot supersedes it with seed-2
// under this packet's, chained by supersedes_id, the seed-1 version retired
// (S09.8) — and the governed file says seed-2.
func TestTQ4SupersessionYieldsSeed2InAWorldThatHeldSeed1(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	if _, err := f.gate.EnsureComposerPlaybook(ctx); err != nil {
		t.Fatalf("EnsureComposerPlaybook: %v", err)
	}

	// The world holds seed-1, and holds it because B3's Ensure writes the
	// FROZEN bytes its record covers — not whatever the code says today.
	raw, err := os.ReadFile(tq4PlaybookPath(f))
	if err != nil {
		t.Fatalf("read governed playbook: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != tq4Seed1Content {
		t.Fatalf("B3's Ensure wrote content hashing %s, want the B3-ratified %s — a live pointer would have written seed-2 under a record that never read it (§57)", got, tq4Seed1Content)
	}
	if strings.Contains(string(raw), "Web deliverables") {
		t.Fatal("B3's record was written with this packet's own section in it")
	}

	res, err := f.gate.EnsureTQ4KnowledgeGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureTQ4KnowledgeGovernance: %v", err)
	}
	if !res.Superseded || res.Repaired || res.Unverifiable {
		t.Fatalf("clean world result = %+v, want one supersession and nothing else", res)
	}

	cur, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if err != nil {
		t.Fatalf("HouseObject playbook: %v", err)
	}
	for _, want := range []string{"P3-TQ-4a", "A16", "seed-2", "PENDING"} {
		if !strings.Contains(cur.OriginRef, want) {
			t.Errorf("the seed-2 record is missing %q: %q", want, cur.OriginRef)
		}
	}
	if cur.Supersedes == "" {
		t.Fatal("seed-2 chains to nothing")
	}
	prev, err := f.store.Get(ctx, cur.Supersedes)
	if err != nil {
		t.Fatalf("Get superseded %s: %v", cur.Supersedes, err)
	}
	if !strings.Contains(prev.OriginRef, "B3") {
		t.Errorf("seed-2 supersedes %q, want B3's seed-1 version", prev.OriginRef)
	}
	if prev.Status != memory.StatusRetired {
		t.Errorf("the seed-1 version is %q, want retired (S09.8: superseded versions retired, never deleted)", prev.Status)
	}
	if cur.Content != worker.ComposerPlaybookSeed() {
		t.Error("the governed playbook diverges from the in-code seed-2")
	}
	if !strings.Contains(cur.Content, "Web deliverables") {
		t.Error("the governed seed-2 carries no web-deliverable section")
	}
}

// TestTQ4FreshWorldChainAndIdempotence (F2): a fresh world defers until an
// operator exists (D10), then writes the whole chain seed-1 → seed-2, each
// under the record of the gate that owns it; later boots find nothing to do.
func TestTQ4FreshWorldChainAndIdempotence(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	if _, err := f.gate.EnsureTQ4KnowledgeGovernance(ctx); !errors.Is(err, memory.ErrNoOperator) {
		t.Fatalf("without operator: %v, want ErrNoOperator", err)
	}
	f.user("darian", "operator")
	if res := bootPlaybookGovernance(t, f); !res.Superseded {
		t.Fatalf("fresh world: %+v, want the seed-2 supersession", res)
	}

	cur, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if err != nil {
		t.Fatal(err)
	}
	prev, err := f.store.Get(ctx, cur.Supersedes)
	if err != nil {
		t.Fatalf("Get superseded: %v", err)
	}
	if !strings.Contains(cur.OriginRef, "P3-TQ-4a") || !strings.Contains(prev.OriginRef, "B3") {
		t.Fatalf("chain reads %q → %q, want B3's seed-1 then this packet's seed-2", prev.OriginRef, cur.OriginRef)
	}

	for boot := 0; boot < 2; boot++ {
		if _, err := f.gate.EnsureComposerPlaybook(ctx); err != nil {
			t.Fatalf("boot %d, B3: %v", boot+2, err)
		}
		res, err := f.gate.EnsureTQ4KnowledgeGovernance(ctx)
		if err != nil || res.Superseded || res.Repaired {
			t.Fatalf("boot %d re-applied: %+v err=%v", boot+2, res, err)
		}
	}
	after, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != cur.ID {
		t.Errorf("the playbook version moved across three boots: %s then %s", cur.ID, after.ID)
	}
}

// TestTQ4LaterDecisionsStandAndHandEditsRepair (F2; S09.8, CONVENTIONS §57):
// (a) a version the operator writes through the knowledge gate after seed-2 is
// a LATER decision — a boot never reverts it; (b) an edit made to the governed
// FILE with no gate row is not a decision — the boot repairs the file from the
// row it is committed with, mints no version, and reports the repair.
func TestTQ4LaterDecisionsStandAndHandEditsRepair(t *testing.T) {
	ctx := context.Background()

	t.Run("a governed edit after seed-2 stands", func(t *testing.T) {
		f := newFix(t)
		f.user("darian", "operator")
		bootPlaybookGovernance(t, f)
		cur, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
		if err != nil {
			t.Fatal(err)
		}
		mine, err := f.gate.NewVersion(ctx, "darian", cur.ID, memory.Draft{
			Scope: memory.ScopeHouse, Kind: memory.KindPlaybook, Title: cur.Title,
			Content:  cur.Content + "\n## The operator's own section\n\nKeep this.\n",
			TopicKey: cur.TopicKey, Selectors: cur.Selectors,
			FileBacked: true, FileName: filepath.Base(cur.FilePath),
		})
		if err != nil {
			t.Fatalf("operator NewVersion: %v", err)
		}
		for boot := 0; boot < 2; boot++ {
			if _, err := f.gate.EnsureComposerPlaybook(ctx); err != nil {
				t.Fatalf("boot %d, B3: %v", boot+1, err)
			}
			res, err := f.gate.EnsureTQ4KnowledgeGovernance(ctx)
			if err != nil || res.Superseded || res.Repaired {
				t.Fatalf("boot %d re-applied over the operator's version: %+v err=%v", boot+1, res, err)
			}
		}
		after, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
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
		bootPlaybookGovernance(t, f)
		before, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
		if err != nil {
			t.Fatal(err)
		}
		torn := strings.Replace(before.Content, "Web deliverables", "Whatever somebody typed", 1)
		if torn == before.Content {
			t.Fatal("fixture: the seed-2 content carries no web-deliverable heading to tear")
		}
		if err := os.WriteFile(tq4PlaybookPath(f), []byte(torn), 0o600); err != nil {
			t.Fatal(err)
		}
		res, err := f.gate.EnsureTQ4KnowledgeGovernance(ctx)
		if err != nil {
			t.Fatalf("EnsureTQ4KnowledgeGovernance: %v", err)
		}
		if !res.Repaired || res.Superseded {
			t.Fatalf("result after a raw file edit = %+v, want exactly one repair and no new version", res)
		}
		raw, err := os.ReadFile(tq4PlaybookPath(f))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != before.Content {
			t.Error("the governed file was not restored to the content its row is committed with")
		}
		after, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
		if err != nil {
			t.Fatal(err)
		}
		if after.ID != before.ID || after.Version != before.Version {
			t.Errorf("a repair minted a version: %s v%d → %s v%d", before.ID, before.Version, after.ID, after.Version)
		}
	})
}

// TestTQ4LeavesTheTaxonomyRecordsAlone (F2; CONVENTIONS §57's own instruction):
// this packet minted its own Ensure rather than extending RW-12's. If it had
// touched RW-12's digests or its frozen snapshot, RW-12's own verification
// would refuse to write and the taxonomy chain would stop at B2.
func TestTQ4LeavesTheTaxonomyRecordsAlone(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")
	if _, err := f.gate.EnsureB2SeedGovernance(ctx); err != nil {
		t.Fatalf("EnsureB2SeedGovernance: %v", err)
	}
	res, err := f.gate.EnsureRW12TaxonomyGovernance(ctx)
	if err != nil {
		t.Fatalf("EnsureRW12TaxonomyGovernance: %v — this packet must not have moved RW-12's digests or its snapshot (§57)", err)
	}
	if res.Created == 0 && res.Superseded == 0 {
		t.Fatal("RW-12's governance did nothing on a fresh world — its record no longer matches its own content")
	}
}
