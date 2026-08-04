package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// Template → overlay → instance on the S09 machinery (Spec S08.4 × S09.4):
// the compile reads the overlay slice through memory.Store.OverlaySlice.
// At v0 the scope is write-dormant — the gate refuses worker-overlay
// writes — so the slice is structurally empty; the machinery is proven by
// simulating the v1 writer with a raw row insert (tests are not the
// platform; the schema ships whole at v0, Spec S09.4).

type memOverlays struct{ s *memory.Store }

func (m memOverlays) OverlaySlice(ctx context.Context, owner, templateID string) ([]worker.OverlayItem, error) {
	entries, err := m.s.OverlaySlice(ctx, owner, templateID)
	if err != nil {
		return nil, err
	}
	items := make([]worker.OverlayItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, worker.OverlayItem{EntryID: e.ID, Title: e.Title, Content: e.Content, Version: e.Version})
	}
	return items, nil
}

func TestOverlayThroughS09Machinery(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	ctx := context.Background()

	memStore, err := memory.NewStore(f.db, f.log, f.reg, t.TempDir())
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	store := func() *worker.Store {
		st, err := worker.NewStore(worker.Config{
			DB: f.db, Log: f.log, Settings: f.reg, Root: f.root,
			Overlays: memOverlays{s: memStore},
		})
		if err != nil {
			t.Fatalf("NewStore(overlays): %v", err)
		}
		return st
	}()

	tpl, v, err := store.CreateDraft(ctx, "alice", agenticSrc, readGrants(), humanProv())
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := store.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: "alice", SampleTask: "s", Engine: &fakeDry{}, Model: "m", EnginePin: "p",
	}); err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if _, err := store.Approve(ctx, "alice", v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// v0 dormancy is structural: the S09 gate refuses worker-overlay
	// writes, so the slice is empty and the compile carries no overlay.
	if _, err := memory.NewGate(memStore).CreateManual(ctx, "alice", memory.Draft{
		Scope: memory.ScopeWorkerOverlay, ScopeRef: tpl.ID, Kind: memory.KindLesson,
		Title: "t", Content: "c", TopicKey: "k",
	}); !errors.Is(err, memory.ErrScopeDormant) {
		t.Fatalf("overlay write at v0: %v, want ErrScopeDormant (S09.4)", err)
	}
	base, err := store.CompileForRun(ctx, tpl.ID, "alice", worker.InstanceRefs{RunID: "r1"})
	if err != nil {
		t.Fatalf("CompileForRun: %v", err)
	}
	if strings.Contains(string(base.Worker.AgentsJSON), "overlay lesson") {
		t.Fatalf("empty-at-v0 overlay produced content")
	}

	// Simulate the v1 writer: a raw active worker-overlay L2 row (the
	// schema ships whole; dormancy is the absence of a writer).
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO knowledge_entries (entry_id, user_id, scope, scope_ref, layer, kind, title,
			   content, topic_key, selectors, status, version, origin, created_ts, updated_ts)
			 VALUES ('k-ov1', 'alice', 'worker_overlay', ?, 'L2', 'lesson', 'spelling',
			   'Use British spelling.', 'style/spelling', '{}', 'active', 1, 'human_direct', ?, ?)`,
			tpl.ID, now, now)
		return err
	})
	if err != nil {
		t.Fatalf("insert overlay row: %v", err)
	}

	c, err := store.CompileForRun(ctx, tpl.ID, "alice", worker.InstanceRefs{RunID: "r2"})
	if err != nil {
		t.Fatalf("CompileForRun(overlay): %v", err)
	}
	var agents map[string]map[string]string
	if err := json.Unmarshal(c.Worker.AgentsJSON, &agents); err != nil {
		t.Fatalf("agents JSON: %v", err)
	}
	prompt := agents["code-reviewer"]["prompt"]
	if !strings.Contains(prompt, "[overlay lesson k-ov1 v1] spelling") ||
		!strings.Contains(prompt, "Use British spelling.") {
		t.Fatalf("overlay slice not compiled through the S09 machinery:\n%s", prompt)
	}
	// Another user's instance is disjoint by construction (10.1): bob has
	// no overlay on this template.
	f.user("bob", "member")
	cb, err := store.CompileForRun(ctx, tpl.ID, "bob", worker.InstanceRefs{RunID: "r3"})
	if err != nil {
		t.Fatalf("CompileForRun(bob): %v", err)
	}
	if strings.Contains(string(cb.Worker.AgentsJSON), "overlay lesson") {
		t.Fatalf("overlay leaked across users (10.1)")
	}
}
