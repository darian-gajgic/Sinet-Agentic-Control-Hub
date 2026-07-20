package memory_test

import (
	"context"
	"database/sql"
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
)

// STANDING conformance battery (Spec S09.1, S09.3; R11 §4.8): the
// isolation and write-gate walls are proven by attempted violation on
// every run of the suite, never assumed.

// TestFTS5AvailableAtPinnedDriver proves FTS5 is compiled into the pinned
// modernc.org/sqlite driver by exercising it — the migration creates the
// virtual table and MATCH must answer (Spec S09.3: knowledge_search is
// FTS5-backed; availability is asserted by test, not assumed).
func TestFTS5AvailableAtPinnedDriver(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson,
		Title: "fts probe", Content: "the quick zephyr fox", TopicKey: "probe/fts",
	})
	var n int
	if err := f.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_fts WHERE knowledge_fts MATCH 'zephyr'`).Scan(&n); err != nil {
		t.Fatalf("FTS5 MATCH failed — FTS5 unavailable at the pinned driver: %v", err)
	}
	if n != 1 {
		t.Fatalf("MATCH hits = %d, want 1", n)
	}
}

// TestConformanceCrossUserReadMustFail is THE standing cross-user test of
// Spec S09.3/R11 §4.8: it ATTEMPTS cross-user reads through the one
// search module and they must fail. Scope is derived server-side from
// runs.owner_user; the query text is sanitized, so no crafted input can
// widen the predicate.
func TestConformanceCrossUserReadMustFail(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("bob", "member")
	f.user("op", "operator")

	mustCreate(t, f.gate, "bob", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindPreference,
		Title: "bob private preference", Content: "bob prefers zanzibar deployments", TopicKey: "pref/deploy",
	})
	mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindPreference,
		Title: "alice preference", Content: "alice prefers zanzibar rollbacks", TopicKey: "pref/deploy2",
	})
	mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindConvention,
		Title: "house convention", Content: "house zanzibar convention", TopicKey: "house/conv",
	})
	mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeProject, ScopeRef: "proj-x", Kind: memory.KindConvention,
		Title: "proj-x truth", Content: "project zanzibar truth", TopicKey: "proj/conv",
	})

	f.task("t-alice", "alice")
	f.run("r-alice", "t-alice", "alice")

	assertNoBob := func(query string) {
		t.Helper()
		hits, err := f.store.Search(ctx, "r-alice", query)
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}
		for _, h := range hits {
			if h.Scope == memory.ScopeUser && strings.Contains(h.Title, "bob") {
				t.Fatalf("cross-user leak: query %q returned %+v", query, h)
			}
			if h.Scope == memory.ScopeProject {
				t.Fatalf("project leak without claim: query %q returned %+v", query, h)
			}
		}
	}

	// The straight attempt and a battery of widening attempts: FTS5
	// operators, quote escapes, column filters, SQL fragments.
	for _, q := range []string{
		"zanzibar",
		"zanzibar OR bob",
		`"zanzibar" OR title:bob`,
		`zanzibar" OR "1"="1`,
		"content: zanzibar NOT nothing",
		"{title content}: zanzibar",
		"zanzibar; SELECT * FROM knowledge_entries",
	} {
		assertNoBob(q)
	}

	// The same term through alice's own run DOES return her entry and the
	// house entry — the wall is scope, not the term.
	hits, err := f.store.Search(ctx, "r-alice", "zanzibar")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var sawAlice, sawHouse bool
	for _, h := range hits {
		if h.Scope == memory.ScopeUser && strings.Contains(h.Title, "alice") {
			sawAlice = true
		}
		if h.Scope == memory.ScopeHouse {
			sawHouse = true
		}
	}
	if !sawAlice || !sawHouse {
		t.Fatalf("own-scope search lost entries: alice=%v house=%v hits=%+v", sawAlice, sawHouse, hits)
	}

	// Project scope opens ONLY through the platform-derived project fact.
	f.claim("t-alice", "proj-x", "alice")
	hits, err = f.store.Search(ctx, "r-alice", "zanzibar")
	if err != nil {
		t.Fatalf("Search with claim: %v", err)
	}
	var sawProj bool
	for _, h := range hits {
		if h.Scope == memory.ScopeProject && h.ScopeRef == "proj-x" {
			sawProj = true
		}
	}
	if !sawProj {
		t.Fatalf("claimed project scope missing from search: %+v", hits)
	}
}

// TestWorkersStructurallyCannotWriteL2 pins the S09.1 write wall from
// every side reachable at v0: the gate refuses non-person actors (workers,
// reserved ids), no L1 writer exists (the observation kind is refused for
// everyone), proposals are readable by no search path, and the schema
// itself rejects in-place mutation and deletion even for raw SQL holders.
func TestWorkersStructurallyCannotWriteL2(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")

	for _, actor := range []string{"platform", "dev", "run-r1", ""} {
		_, err := f.gate.CreateManual(ctx, actor, memory.Draft{
			Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "x", Content: "y",
		})
		if !errors.Is(err, memory.ErrNotHuman) {
			t.Fatalf("actor %q: err = %v, want ErrNotHuman", actor, err)
		}
	}

	// No L1 writer at v0 (Spec S09.4): the observation kind has no write
	// path, human or otherwise.
	_, err := f.gate.CreateManual(ctx, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindObservation, Title: "obs", Content: "o",
	})
	if !errors.Is(err, memory.ErrLayerForbidden) {
		t.Fatalf("observation write: err = %v, want ErrLayerForbidden", err)
	}

	// Proposals are not memory (Spec S09.2): a pending proposal reaches no
	// search result even when its text matches.
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lesson_proposals (proposal_id, user_id, scope, kind, title, content, status, created_ts)
			VALUES ('p1', 'alice', 'user', 'lesson', 'pending zugzwang', 'proposal zugzwang body', 'open', ?)`,
			time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		t.Fatalf("insert proposal: %v", err)
	}
	f.task("t1", "alice")
	f.run("r1", "t1", "alice")
	hits, err := f.store.Search(ctx, "r1", "zugzwang")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("proposal leaked into search: %+v", hits)
	}

	// Schema wall: in-place content mutation and deletion are aborted by
	// the migration triggers — supersession/tombstoning are the only
	// paths, even for code holding raw SQL (Spec S09.2/S09.5).
	e := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "immutable", Content: "v1 body",
	})
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE knowledge_entries SET content = 'poisoned' WHERE entry_id = ?`, e.ID)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("in-place content edit: err = %v, want immutability abort", err)
	}
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM knowledge_entries WHERE entry_id = ?`, e.ID)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "never deleted") {
		t.Fatalf("row delete: err = %v, want delete abort", err)
	}
}

// TestEngineFacingPackagesDoNotImportMemory pins the capability-withholding
// import graph (Spec S09.1; the §15 quality-vs-effects import precedent):
// the engine-facing packages — adapters, the stage dispatcher, the ledger
// assembly — never import internal/memory, so no engine-reachable code
// path can even name the write gate. The composition root (shell) hands
// stage only the ledger.Source view.
func TestEngineFacingPackagesDoNotImportMemory(t *testing.T) {
	for _, dir := range []string{
		"../adapters", "../adapters/claudecli", "../stage", "../ledger",
	} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		if len(files) == 0 {
			t.Fatalf("no sources under %s — package moved? update the conformance scan", dir)
		}
		fset := token.NewFileSet()
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			for _, imp := range parsed.Imports {
				if strings.Contains(imp.Path.Value, "internal/memory") {
					t.Errorf("%s imports internal/memory — engine-facing code must hold no knowledge write capability (Spec S09.1)", name)
				}
			}
		}
	}
}
