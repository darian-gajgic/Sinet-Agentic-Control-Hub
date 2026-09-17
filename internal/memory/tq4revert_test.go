package memory_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// Coordinator-inline pin after the drain cap (P3-TQ-4a, 2026-09-18): the
// `decisionRecorded` guard is load-bearing on its own. An operator who reverts
// the governed playbook through the gate to the BYTE-EXACT frozen seed-1 (the
// one content shape equality cannot tell from "never applied here yet") must
// stand across later boots; without the guard the content check alone would
// re-supersede to seed-2 every boot (S09.8/S09.10: the version history is the
// decision, not content equality).
func TestTQ4LaterOperatorRevertToSeed1Stands(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.user("darian", "operator")

	// B3's seeder alone leaves the governed file at the frozen seed-1 bytes.
	if _, err := f.gate.EnsureComposerPlaybook(ctx); err != nil {
		t.Fatalf("EnsureComposerPlaybook: %v", err)
	}
	seed1, err := os.ReadFile(tq4PlaybookPath(f))
	if err != nil {
		t.Fatal(err)
	}
	// This packet's supersession to seed-2.
	if res, err := f.gate.EnsureTQ4KnowledgeGovernance(ctx); err != nil || !res.Superseded {
		t.Fatalf("precondition: seed-2 supersession = %+v err=%v", res, err)
	}
	cur, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Content == string(seed1) {
		t.Fatal("precondition: the active content is still seed-1")
	}

	// The operator reverts to the byte-exact seed-1 through the gate.
	reverted, err := f.gate.NewVersion(ctx, "darian", cur.ID, memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindPlaybook, Title: cur.Title,
		Content: string(seed1), TopicKey: cur.TopicKey, Selectors: cur.Selectors,
		FileBacked: true, FileName: filepath.Base(cur.FilePath),
	})
	if err != nil {
		t.Fatalf("operator revert to seed-1 through the gate: %v", err)
	}

	for boot := 0; boot < 2; boot++ {
		if _, err := f.gate.EnsureComposerPlaybook(ctx); err != nil {
			t.Fatalf("boot %d, B3: %v", boot+1, err)
		}
		res, err := f.gate.EnsureTQ4KnowledgeGovernance(ctx)
		if err != nil {
			t.Fatalf("boot %d, TQ4: %v", boot+1, err)
		}
		if res.Superseded {
			t.Fatalf("boot %d re-applied seed-2 over the operator's later decision (%+v) — "+
				"content equality is not a decision; the version history is", boot+1, res)
		}
	}
	after, err := f.store.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != reverted.ID {
		t.Fatalf("the active version is %q, want the operator's revert %q", after.ID, reverted.ID)
	}
	if after.Content != string(seed1) {
		t.Error("the active content is not the seed-1 the operator reverted to")
	}
}
