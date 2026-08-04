package memory_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
)

func TestGateProvenanceAndLifecycleStamping(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("op", "operator")

	lesson := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson,
		Title: "retry flaky fetches", Content: "fetches to api.example flake; retry once", TopicKey: "net/retries",
	})
	if lesson.Origin != memory.OriginHumanDirect || lesson.ApprovedBy != "alice" || lesson.Status != memory.StatusActive {
		t.Fatalf("lesson provenance = %+v", lesson)
	}
	if lesson.Layer != memory.LayerL2 || lesson.Version != 1 {
		t.Fatalf("lesson layer/version = %+v", lesson)
	}
	// Lifecycle stamping (Spec S09.8): lessons carry ⚙
	// memory.reverify_lessons_days (default 90); preferences none; house
	// objects ⚙ memory.reverify_house_days (default 180).
	if lesson.ReverifyIntervalDays != 90 || lesson.VerifiedBy != "alice" || lesson.VerifiedTS == "" {
		t.Fatalf("lesson lifecycle = %+v", lesson)
	}
	pref := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindPreference, Title: "tabs", Content: "use tabs",
	})
	if pref.ReverifyIntervalDays != 0 {
		t.Fatalf("preference interval = %d, want 0 (flagged-only)", pref.ReverifyIntervalDays)
	}

	// D10: house promotion requires the operator.
	if _, err := f.gate.CreateManual(ctx, "alice", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindPlaybook, Title: "h", Content: "x",
	}); !errors.Is(err, memory.ErrOperatorRequired) {
		t.Fatalf("member house write: %v, want ErrOperatorRequired", err)
	}
	house := mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindPlaybook, Title: "house playbook", Content: "house rules",
	})
	if house.ReverifyIntervalDays != 180 {
		t.Fatalf("house interval = %d, want 180", house.ReverifyIntervalDays)
	}

	// Worker-overlay writes activate at v1 (Spec S09.4).
	if _, err := f.gate.CreateManual(ctx, "alice", memory.Draft{
		Scope: memory.ScopeWorkerOverlay, ScopeRef: "tmpl", Kind: memory.KindLesson, Title: "o", Content: "x",
	}); !errors.Is(err, memory.ErrScopeDormant) {
		t.Fatalf("overlay write: %v, want ErrScopeDormant", err)
	}

	// The write event is on the audit log.
	if got := f.events(memory.EventWrite); len(got) != 3 {
		t.Fatalf("knowledge.write events = %d, want 3", len(got))
	}

	// ⚙ Numbers reads the full lifecycle set by dotted key (S18 defaults).
	n, err := f.store.Numbers()
	if err != nil {
		t.Fatalf("Numbers: %v", err)
	}
	if n.L1TTLDays != 90 || n.ReverifyLessonsDays != 90 || n.ReverifyHouseDays != 180 ||
		n.ProposalsPerTaskMax != 2 || n.DigestIntervalDays != 7 || n.DistillThresholdLessons != 3 ||
		n.VectorGateTaskMissRate != 0.05 || n.VectorGateCorpusEntries != 5000 {
		t.Fatalf("Numbers = %+v, want the ratified S18 defaults", n)
	}
}

func TestSupersessionIsANewVersionRow(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("bob", "member")

	v1 := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "L", Content: "old body", TopicKey: "top/l",
	})
	if _, err := f.gate.NewVersion(ctx, "bob", v1.ID, memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "L", Content: "bob edit",
	}); !errors.Is(err, memory.ErrNotOwner) {
		t.Fatalf("foreign supersession: %v, want ErrNotOwner", err)
	}
	v2, err := f.gate.NewVersion(ctx, "alice", v1.ID, memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "L", Content: "new body", TopicKey: "top/l",
	})
	if err != nil {
		t.Fatalf("NewVersion: %v", err)
	}
	if v2.Version != 2 || v2.Supersedes != v1.ID {
		t.Fatalf("v2 = %+v", v2)
	}
	old, err := f.store.Get(ctx, v1.ID)
	if err != nil {
		t.Fatalf("Get v1: %v", err)
	}
	if old.Status != memory.StatusRetired || old.Content != "old body" {
		t.Fatalf("superseded v1 = %+v (want retired, content intact)", old)
	}
	// Supersession does NOT self-conflict: no open edge between versions.
	if got := f.events(memory.EventConflict); len(got) != 0 {
		t.Fatalf("conflict events on supersession: %v", got)
	}
}

func TestAdoptionCopiesNeverReference(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("bob", "member")

	src := mustCreate(t, f.gate, "bob", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "bob lesson", Content: "shared wisdom", TopicKey: "top/adopt",
	})
	adopted, err := f.gate.Adopt(ctx, "alice", src.ID)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if adopted.Owner != "alice" || adopted.Scope != memory.ScopeUser || adopted.Origin != memory.OriginAdoptedFrom {
		t.Fatalf("adopted = %+v", adopted)
	}
	var ref struct{ Entry, Owner string }
	if err := json.Unmarshal([]byte(adopted.OriginRef), &ref); err != nil || ref.Entry != src.ID || ref.Owner != "bob" {
		t.Fatalf("adoption origin_ref = %q (%v)", adopted.OriginRef, err)
	}
	var n int
	if err := f.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_adoptions WHERE user_id='alice' AND source_entry_id=? AND adopted_entry_id=?`,
		src.ID, adopted.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("adoption row: n=%d err=%v", n, err)
	}

	// A copy, never a live reference: removing the source leaves the copy.
	if _, err := f.gate.Remove(ctx, "bob", src.ID, "outdated"); err != nil {
		t.Fatalf("Remove source: %v", err)
	}
	got, err := f.store.Get(ctx, adopted.ID)
	if err != nil || got.Status != memory.StatusActive || got.Content != "shared wisdom" {
		t.Fatalf("adopted copy after source removal = %+v (%v)", got, err)
	}
}

func TestRemovalLeavesEveryFutureAssemblyImmediately(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.task("t1", "alice")
	f.run("r1", "t1", "alice")
	f.objective("r1")

	e := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "inject me", Content: "lesson body", TopicKey: "top/rm",
	})
	src := &memory.Source{S: f.store}
	brief, err := f.ledger.Assemble(ctx, ledger.AssembleInput{
		RunID: "r1", Stage: "step-1", Sources: ledger.Sources{Knowledge: src},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if !manifestHas(brief.Manifest, "knowledge/"+e.ID, "") {
		t.Fatalf("entry not injected: %+v", brief.Manifest)
	}

	influence, err := f.gate.Remove(ctx, "alice", e.ID, "wrong")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(influence) != 1 || influence[0].RunID != "r1" {
		t.Fatalf("influence = %+v, want the injected run r1", influence)
	}

	brief, err = f.ledger.Assemble(ctx, ledger.AssembleInput{
		RunID: "r1", Stage: "step-2", Sources: ledger.Sources{Knowledge: src},
	})
	if err != nil {
		t.Fatalf("Assemble after removal: %v", err)
	}
	if manifestHas(brief.Manifest, "knowledge/"+e.ID, "") {
		t.Fatalf("removed entry still assembling: %+v", brief.Manifest)
	}
	row, err := f.store.Get(ctx, e.ID)
	if err != nil || row.Status != memory.StatusRemoved || row.Content == "" {
		t.Fatalf("audit row after removal = %+v (%v)", row, err)
	}
}

func TestTrueDeletionLeavesTombstone(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("bob", "member")

	e := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson,
		Title: "sensitive", Content: "very private xylophone fact", TopicKey: "top/del",
		FileBacked: true,
	})
	filePath := filepath.Join(f.root, e.FilePath)
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file-backed entry has no file: %v", err)
	}

	// True deletion is the OWNER's right (S4.1) — not even the operator's.
	if err := f.gate.TrueDelete(ctx, "bob", e.ID); !errors.Is(err, memory.ErrNotOwner) {
		t.Fatalf("foreign true-delete: %v, want ErrNotOwner", err)
	}
	if err := f.gate.TrueDelete(ctx, "alice", e.ID); err != nil {
		t.Fatalf("TrueDelete: %v", err)
	}
	stub, err := f.store.Get(ctx, e.ID)
	if err != nil {
		t.Fatalf("Get tombstone: %v", err)
	}
	if !stub.Tombstone || stub.Status != memory.StatusRemoved ||
		stub.Content != "" || stub.Title != "" || stub.FilePath != "" ||
		stub.TombstoneNote != "removed at owner request" ||
		stub.CreatedTS == "" || stub.UpdatedTS == "" {
		t.Fatalf("tombstone = %+v (want the minimal audit stub, G2 Def.9)", stub)
	}
	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("working-tree file survived the purge: %v", err)
	}
	// The FTS index no longer serves the purged content.
	f.task("t1", "alice")
	f.run("r1", "t1", "alice")
	hits, err := f.store.Search(ctx, "r1", "xylophone")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("purged content still searchable: %+v", hits)
	}
	// Idempotent.
	if err := f.gate.TrueDelete(ctx, "alice", e.ID); err != nil {
		t.Fatalf("repeat TrueDelete: %v", err)
	}
	if got := f.events(memory.EventDelete); len(got) != 1 {
		t.Fatalf("knowledge.delete events = %d, want 1", len(got))
	}
}

func TestWriteTimeConflictDetection(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.task("t1", "alice")
	f.run("r1", "t1", "alice")
	f.objective("r1")

	e1 := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "use rebase", Content: "always rebase", TopicKey: "git/merge-style",
	})
	e2 := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "use merge", Content: "always merge", TopicKey: "git/merge-style",
	})

	var conflictID int64
	var affected, question string
	err := f.db.QueryRowContext(ctx, `
		SELECT conflict_id, user_id, question FROM knowledge_conflicts
		 WHERE status='open' AND entry_id=? AND other_entry_id=?`, e2.ID, e1.ID).
		Scan(&conflictID, &affected, &question)
	if err != nil {
		t.Fatalf("conflict edge missing: %v", err)
	}
	if affected != "alice" || !strings.Contains(question, "git/merge-style") {
		t.Fatalf("conflict question = %q to %q", question, affected)
	}
	if got := f.events(memory.EventConflict); len(got) != 1 {
		t.Fatalf("conflict events = %d, want 1", len(got))
	}

	// A known-conflicting pair entering one frame is flagged in the trace
	// manifest and the question re-raised (Spec S09.7).
	brief, err := f.ledger.Assemble(ctx, ledger.AssembleInput{
		RunID: "r1", Stage: "s", Sources: ledger.Sources{Knowledge: &memory.Source{S: f.store}},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	var flagged int
	for _, m := range brief.Manifest {
		if m.ItemID == "knowledge/"+e1.ID || m.ItemID == "knowledge/"+e2.ID {
			if len(m.ConflictsWith) != 1 {
				t.Fatalf("manifest entry %s conflicts_with = %v", m.ItemID, m.ConflictsWith)
			}
			flagged++
		}
	}
	if flagged != 2 {
		t.Fatalf("flagged pair entries = %d, want 2", flagged)
	}
	if got := f.events(memory.EventConflict); len(got) != 2 { // write-time + re-raise
		t.Fatalf("conflict events after assembly = %d, want 2 (re-raise)", len(got))
	}

	if err := f.gate.ResolveConflict(ctx, "alice", conflictID); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	brief, err = f.ledger.Assemble(ctx, ledger.AssembleInput{
		RunID: "r1", Stage: "s2", Sources: ledger.Sources{Knowledge: &memory.Source{S: f.store}},
	})
	if err != nil {
		t.Fatalf("Assemble after resolve: %v", err)
	}
	for _, m := range brief.Manifest {
		if len(m.ConflictsWith) != 0 {
			t.Fatalf("resolved conflict still flagged: %+v", m)
		}
	}
}

// fakeScreen is a test contradiction screen (the S09.7 advisory seam).
type fakeScreen struct{ called bool }

func (s *fakeScreen) Screen(ctx context.Context, a, b memory.Entry) (bool, string, error) {
	s.called = true
	return true, "the two entries prescribe opposite merge styles", nil
}

func TestContradictionScreenSeamRefinesQuestion(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	screen := &fakeScreen{}
	f.gate.Screen = screen

	mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "a", Content: "x", TopicKey: "k",
	})
	mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "b", Content: "y", TopicKey: "k",
	})
	if !screen.called {
		t.Fatal("advisory screen never consulted")
	}
	var question string
	if err := f.db.QueryRowContext(ctx,
		`SELECT question FROM knowledge_conflicts WHERE status='open'`).Scan(&question); err != nil {
		t.Fatalf("edge: %v", err)
	}
	if !strings.Contains(question, "opposite merge styles") {
		t.Fatalf("question lacks the advisory rationale: %q", question)
	}
}

func TestInjectionBudgetsDropAndCurationCard(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.task("t1", "alice")
	f.run("r1", "t1", "alice")
	f.objective("r1")

	// ⚙ memory.injection_budget_tokens.user default 1500 tokens = 6000
	// bytes at the 4-bytes/token estimate: big alone exceeds it.
	big := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson,
		Title: "big", Content: strings.Repeat("wordy knowledge body ", 400), TopicKey: "top/big",
	})
	small := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson,
		Title: "small", Content: "tiny lesson", TopicKey: "top/small",
	})
	// Deterministic drop order (Spec S09.8): equal specificity ⇒ oldest
	// last_injected_at first; make big the stale one.
	err := f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE knowledge_entries SET last_injected_ts='2020-01-01T00:00:00Z' WHERE entry_id=?`, big.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE knowledge_entries SET last_injected_ts='2026-01-01T00:00:00Z' WHERE entry_id=?`, small.ID)
		return err
	})
	if err != nil {
		t.Fatalf("stage last_injected: %v", err)
	}

	brief, err := f.ledger.Assemble(ctx, ledger.AssembleInput{
		RunID: "r1", Stage: "s", Sources: ledger.Sources{Knowledge: &memory.Source{S: f.store}},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	// small injected; big manifested as over_budget_dropped, never a block.
	if !manifestHas(brief.Manifest, "knowledge/"+small.ID, "") {
		t.Fatalf("small entry missing: %+v", brief.Manifest)
	}
	if !manifestHas(brief.Manifest, "knowledge/"+big.ID, ledger.DispositionOverBudgetDropped) {
		t.Fatalf("dropped entry not manifested (silent truncation!): %+v", brief.Manifest)
	}
	for _, b := range brief.Blocks {
		if b.ItemID == "knowledge/"+big.ID {
			t.Fatal("over-budget entry injected as a block")
		}
		if b.ItemID == "knowledge/"+small.ID && b.Precedence != ledger.PrecedenceUser {
			t.Fatalf("user-scope precedence label = %q", b.Precedence)
		}
	}

	// The curation card to the scope owner, level-triggered.
	asks := f.openAsks("ask-knowledge-curation-user-alice-%")
	if len(asks) != 1 {
		t.Fatalf("curation cards = %v, want exactly 1", asks)
	}
	if got := f.events(memory.EventCuration); len(got) != 1 {
		t.Fatalf("curation events = %d, want 1", len(got))
	}
	// A second assembly under the same pressure re-raises nothing new.
	if _, err := f.ledger.Assemble(ctx, ledger.AssembleInput{
		RunID: "r1", Stage: "s2", Sources: ledger.Sources{Knowledge: &memory.Source{S: f.store}},
	}); err != nil {
		t.Fatalf("second Assemble: %v", err)
	}
	if asks := f.openAsks("ask-knowledge-curation-user-alice-%"); len(asks) != 1 {
		t.Fatalf("curation card not level-triggered: %v", asks)
	}

	// last_injected_at moved for the injected entry (staleness signal).
	row, err := f.store.Get(ctx, small.ID)
	if err != nil || row.LastInjectedTS == "2026-01-01T00:00:00Z" {
		t.Fatalf("last_injected not recorded: %+v (%v)", row, err)
	}
}

func TestSelectorMatchingIsConservative(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("op", "operator")
	f.task("t1", "alice")
	f.run("r1", "t1", "alice")

	// Unmatchable facts (domain/task-type/triggers) keep the entry out;
	// the project selector matches only the platform-derived project.
	domainSel := mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindConvention, Title: "domain-scoped",
		Content: "software only", Selectors: memory.Selectors{Domain: "software"},
	})
	plain := mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindConvention, Title: "plain", Content: "always",
	})
	projSel := mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindConvention, Title: "proj-scoped",
		Content: "proj-x only", Selectors: memory.Selectors{Project: "proj-x"},
	})

	src := &memory.Source{S: f.store}
	items, err := src.Items(ctx, ledger.SliceQuery{TaskID: "t1", Owner: "alice", Stage: "s"})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	ids := itemIDs(items)
	if ids["knowledge/"+domainSel.ID] {
		t.Fatal("domain-selector entry injected without a confirmable domain fact")
	}
	if ids["knowledge/"+projSel.ID] {
		t.Fatal("project-selector entry injected without the project")
	}
	if !ids["knowledge/"+plain.ID] {
		t.Fatal("selector-free house entry missing")
	}

	f.claim("t1", "proj-x", "alice")
	items, err = src.Items(ctx, ledger.SliceQuery{TaskID: "t1", Owner: "alice", Stage: "s"})
	if err != nil {
		t.Fatalf("Items with claim: %v", err)
	}
	if !itemIDs(items)["knowledge/"+projSel.ID] {
		t.Fatal("project-selector entry missing despite matching claim")
	}
}

func TestCleanModeExcludesUserScope(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("op", "operator")
	f.task("t1", "alice")

	userE := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindPreference, Title: "personal", Content: "mine",
	})
	houseE := mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindConvention, Title: "house", Content: "shared",
	})
	items, err := (&memory.Source{S: f.store}).Items(ctx,
		ledger.SliceQuery{TaskID: "t1", Owner: "alice", Stage: "verify", Clean: true})
	if err != nil {
		t.Fatalf("Items clean: %v", err)
	}
	ids := itemIDs(items)
	if ids["knowledge/"+userE.ID] {
		t.Fatal("user-scope entry reached a clean slice (S05.4 exception)")
	}
	if !ids["knowledge/"+houseE.ID] {
		t.Fatal("house entry missing from clean slice")
	}
}

func TestSearchMissLogsRetrievalMissEvent(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.task("t1", "alice")
	f.run("r1", "t1", "alice")

	hits, err := f.store.Search(ctx, "r1", "absent-term-quixotic")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %+v", hits)
	}
	got := f.events(memory.EventSearchMiss)
	if len(got) != 1 || !strings.Contains(got[0], "absent-term-quixotic") {
		t.Fatalf("search-miss events = %v", got)
	}
}

// fakeCommitter tests the S13 git seam boundary.
type fakeCommitter struct {
	author string
	paths  []string
}

func (c *fakeCommitter) Commit(ctx context.Context, dir string, paths []string, author, message string) (string, error) {
	c.author = author
	c.paths = paths
	return "deadbeefcafe", nil
}

func TestFileBackedCommitSeam(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	c := &fakeCommitter{}
	f.gate.Committer = c

	e := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson,
		Title: "filed", Content: "body", FileBacked: true, FileName: "filed.md",
	})
	if e.FilePath != filepath.Join("users", "alice", "filed.md") {
		t.Fatalf("file path = %q", e.FilePath)
	}
	if e.FileCommit != "deadbeefcafe" || c.author != "alice" {
		t.Fatalf("commit hash = %q author = %q (approver-as-author, D9)", e.FileCommit, c.author)
	}
	row, err := f.store.Get(context.Background(), e.ID)
	if err != nil || row.FileCommit != "deadbeefcafe" || row.Content != "" {
		t.Fatalf("row = %+v (%v)", row, err)
	}
}

func manifestHas(m []ledger.ManifestEntry, itemID, disposition string) bool {
	for _, e := range m {
		if e.ItemID == itemID && e.Disposition == disposition {
			return true
		}
	}
	return false
}

func itemIDs(items []ledger.Item) map[string]bool {
	out := map[string]bool{}
	for _, it := range items {
		if it.Disposition == "" {
			out[it.ItemID] = true
		}
	}
	return out
}
