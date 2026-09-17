package shell

// project_tree_seam_test.go — P3-SIT-1 R14: the composition of review's
// TreeSource over internal/project.
//
// The two packages never meet by import (CONVENTIONS §23), so the only thing
// that can make the tree lane work in a real process is this adapter, and the
// only thing that can make it work CORRECTLY is that both sides mean the same
// words. Both are bound here: a deliverable id resolves through its task's
// durable intake match to the registered project, the recorded base comes back
// as the base, and project's change kinds are asserted to BE review's constants
// rather than to look like them.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// TestTreeSeamResolvesADeliverablesOwnProject: the seam answers about the
// deliverable's OWN project — resolved through the task's durable intake match,
// never a name re-match — and a deliverable whose task has no registered
// project gets the honest absence rather than somebody else's tree.
func TestTreeSeamResolvesADeliverablesOwnProject(t *testing.T) {
	db, log, reg := seamDB(t)
	ctx := context.Background()
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop"}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	runs := run.NewStore(db, log)
	pipe := &intake.Pipeline{
		DB: db, Log: log, Runs: runs, Ledger: ledger.NewStore(db, log), Settings: reg,
		ArtifactRoot: filepath.Join(t.TempDir(), "artifacts"),
		Registry:     registrySeam{proj: proj},
	}
	if _, err := pipe.Start(ctx, intake.Request{
		TaskID: "t-shop", UserID: "alice", Title: "change the shop", Text: "adjust the checkout copy", Project: "shop",
	}); err != nil {
		t.Fatalf("intake Start: %v", err)
	}
	ps := &projectSeams{proj: proj, runs: runs, db: db, pipe: pipe}

	// Before a workspace exists there is no recorded base — an absence, not a
	// guess and not an error (Spec S13.5).
	if sha, ok, err := ps.TreeBase(ctx, "dlv-t-shop"); ok || sha != "" || err != nil {
		t.Fatalf("TreeBase before EnsureWorkspace = %q ok=%v err=%v, want the honest absence", sha, ok, err)
	}
	ws, err := proj.EnsureWorkspace(ctx, "shop", "t-shop")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	sha, ok, err := ps.TreeBase(ctx, "dlv-t-shop")
	if err != nil || !ok {
		t.Fatalf("TreeBase: %q ok=%v err=%v", sha, ok, err)
	}
	if sha != ws.Base {
		t.Fatalf("TreeBase = %q, want the recorded base %q", sha, ws.Base)
	}

	// A task with no registered project resolves to no tree, and says so by
	// answering an absence rather than by reaching into another project.
	if sha, ok, err := ps.TreeBase(ctx, "dlv-t-unknown"); ok || sha != "" || err != nil {
		t.Fatalf("TreeBase for an unregistered task = %q ok=%v err=%v", sha, ok, err)
	}
	if _, err := ps.TreeChanges(ctx, "dlv-t-unknown", ws.Base, ws.Base); err == nil {
		t.Error("listing the files of a deliverable with no project answered without error")
	}
}

// TestTreeSeamPassesTheProjectStoresOwnFacts: the inventory, the kinds and the
// bytes cross the seam unchanged — and the kind WORDS are review's own
// constants, which is the half a shape test cannot catch. Two packages that
// each spell "renamed" their own way would produce a surface that silently
// renders nothing for that row.
func TestTreeSeamPassesTheProjectStoresOwnFacts(t *testing.T) {
	db, log, reg := seamDB(t)
	ctx := context.Background()
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	src := t.TempDir()
	rw14Git(t, src, "init", "-q", "-b", "main", ".")
	for name, body := range map[string]string{
		"README.md":   "# shop\n",
		"src/app.go":  "package app\n",
		"docs/old.md": "a page that moves\nline two\nline three\nline four\n",
	} {
		p := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rw14Git(t, src, "add", "-A")
	rw14Git(t, src, "commit", "-qm", "seed")
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop", Source: src}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	runs := run.NewStore(db, log)
	pipe := &intake.Pipeline{
		DB: db, Log: log, Runs: runs, Ledger: ledger.NewStore(db, log), Settings: reg,
		ArtifactRoot: filepath.Join(t.TempDir(), "artifacts"),
		Registry:     registrySeam{proj: proj},
	}
	if _, err := pipe.Start(ctx, intake.Request{
		TaskID: "t-shop", UserID: "alice", Title: "shop work", Text: "build the cart", Project: "shop",
	}); err != nil {
		t.Fatalf("intake Start: %v", err)
	}
	ps := &projectSeams{proj: proj, runs: runs, db: db, pipe: pipe}
	ws, err := proj.EnsureWorkspace(ctx, "shop", "t-shop")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "src/cart.go"), []byte("package app\n\nfunc Cart() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(ws.Path, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "src/app.go"), []byte("package app\n\nfunc Run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(ws.Path, "docs/old.md"), filepath.Join(ws.Path, "docs/new.md")); err != nil {
		t.Fatal(err)
	}
	pin, err := proj.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	rows, err := ps.TreeChanges(ctx, "dlv-t-shop", ws.Base, pin)
	if err != nil {
		t.Fatalf("TreeChanges: %v", err)
	}
	kinds := map[string]string{}
	for _, r := range rows {
		kinds[r.Path] = r.Kind
	}
	// ONE vocabulary, two readers. Every kind the project store can emit is
	// driven through the seam here and asserted to BE review's own constant —
	// a spelling that diverged on one side would render as nothing at all, and
	// comparing two constants would not catch a word git never produces.
	for path, want := range map[string]string{
		"src/cart.go": review.KindAdded,
		"README.md":   review.KindDeleted,
		"src/app.go":  review.KindModified,
		"docs/new.md": review.KindRenamed,
	} {
		if kinds[path] != want {
			t.Errorf("%s crossed the seam as %q, want review's %q (all rows: %v)", path, kinds[path], want, kinds)
		}
	}
	for _, r := range rows {
		if r.Kind == review.KindRenamed && r.OldPath != "docs/old.md" {
			t.Errorf("the rename lost its old path: %+v", r)
		}
	}

	data, size, ok, err := ps.TreeBlob(ctx, "dlv-t-shop", pin, "src/cart.go", 0)
	if err != nil || !ok {
		t.Fatalf("TreeBlob: ok=%v err=%v", ok, err)
	}
	if string(data) != "package app\n\nfunc Cart() {}\n" || size != int64(len(data)) {
		t.Fatalf("TreeBlob = %q (size %d), want the bytes the test wrote", data, size)
	}
}

// TestShellWiresTheReviewStoresTreeSeam: *projectSeams IS a review.TreeSource.
// The composition root assigns it to review.Store.Tree, and a type that stopped
// satisfying the interface would fail the build there — this states the
// requirement where the reason for it is written down.
func TestShellWiresTheReviewStoresTreeSeam(t *testing.T) {
	var seam review.TreeSource = &projectSeams{}
	if seam == nil {
		t.Fatal("the composition-root adapter does not satisfy review.TreeSource")
	}
	store := &review.Store{}
	store.Tree = seam
	if store.Tree == nil {
		t.Fatal("review.Store.Tree does not hold the shell's adapter")
	}
}
