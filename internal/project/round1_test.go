package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestScanForcePushZoneUsesRealBranch (F16b): the force-push danger zone pins
// the project's REAL default branch, never a "<default-branch>" placeholder
// that would flow into ledger §2.
func TestScanForcePushZoneUsesRealBranch(t *testing.T) {
	f := newFix(t)
	d, err := f.store.Scan(t.TempDir(), "trunk")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var forcePush *DangerZone
	for i := range d.DangerZones {
		if d.DangerZones[i].Action == "force-push" {
			forcePush = &d.DangerZones[i]
		}
	}
	if forcePush == nil {
		t.Fatal("scan produced no force-push danger zone")
	}
	if forcePush.Path != "trunk" {
		t.Fatalf("force-push zone path = %q, want the real default branch 'trunk' (never a placeholder)", forcePush.Path)
	}
}

// TestBaseContentByteExact (F5): the pre-task base materializes byte-exact —
// leading blank lines, a trailing newline, and an empty file are preserved
// (cat-file blob is verbatim; no TrimSpace, no synthesized newline). Silent
// mis-anchoring from corrupted content is the worst class (P-T12-2).
func TestBaseContentByteExact(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{
		"leading.txt":  "\n\nafter two blank lines\n",
		"noeol.txt":    "no trailing newline",
		"empty.txt":    "",
		"trailing.txt": "one\ntwo\n\n\n",
	})
	// Creating the workspace records the base ref = the fixture content.
	if _, err := f.store.EnsureWorkspace(ctx, "shop", "task-1"); err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	base, err := f.store.BaseContent(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("BaseContent: %v", err)
	}
	for name, want := range map[string]string{
		"leading.txt":  "\n\nafter two blank lines\n",
		"noeol.txt":    "no trailing newline",
		"empty.txt":    "",
		"trailing.txt": "one\ntwo\n\n\n",
	} {
		got, ok := base[name]
		if !ok {
			t.Fatalf("base missing %q", name)
		}
		if got != want {
			t.Fatalf("base %q = %q, want byte-exact %q", name, got, want)
		}
	}
}

// TestRetentionGCRefusesUnprotected (F7): retention GC refuses to delete a run
// branch whose minted revision is not protected by a reachable platform ref —
// the branch survives (never orphaning the minted commit's only copy).
func TestRetentionGCRefusesUnprotected(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	f.writeFiles(ws.Path, map[string]string{"rev1.txt": "revision one\n"})
	if _, err := f.store.Snapshot(ctx, ws.Path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	ref := "refs/sinet/deliverable/dlv-task-1/rev-1"

	// The ref was NOT created → GC refuses, the branch survives (F7).
	if err := f.store.CollectRunWorktree(ctx, "shop", "task-1", 1, []string{ref}); !errors.Is(err, ErrUnprotectedRevision) {
		t.Fatalf("unprotected collect: err = %v, want ErrUnprotectedRevision", err)
	}
	if sha, _ := f.store.refSHA(ctx, e.StorePath, "refs/heads/"+runBranch("task-1", 1)); sha == "" {
		t.Fatal("the run branch was deleted despite an unprotected minted revision")
	}

	// Create + protect the minted revision properly, then GC succeeds.
	sha := f.git(ws.Path, "rev-parse", "HEAD")
	if err := f.store.CreateRevisionRef(ctx, "shop", ref, sha); err != nil {
		t.Fatalf("CreateRevisionRef: %v", err)
	}
	if err := f.store.CollectRunWorktree(ctx, "shop", "task-1", 1, []string{ref}); err != nil {
		t.Fatalf("protected collect: %v", err)
	}
	// The minted commit survives via its ref after collection + gc.
	f.git(e.StorePath, "gc", "--prune=now", "--quiet")
	if f.gitCode(e.StorePath, "cat-file", "-e", sha+"^{commit}") != 0 {
		t.Fatal("ref-protected minted commit was GC'd after branch collection")
	}
}

// TestUtilityCheckoutLockSemantics (F14): a utility checkout is created LOCKED;
// a locked worktree with its dir absent SURVIVES prune; unlocked-then-absent is
// pruned.
func TestUtilityCheckoutLockSemantics(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	base := f.git(e.StorePath, "rev-parse", "refs/heads/main")

	uc, err := f.store.AddUtilityCheckout(ctx, "shop", base)
	if err != nil {
		t.Fatalf("AddUtilityCheckout: %v", err)
	}
	// The `locked` porcelain attribute is asserted on create.
	locked := false
	list, _ := f.store.worktrees(ctx, e.StorePath)
	for _, w := range list {
		if filepath.Clean(w.Path) == filepath.Clean(uc.Path) && w.Locked {
			locked = true
		}
	}
	if !locked {
		t.Fatal("utility checkout is not locked on create")
	}

	// Dir removed + prune: a LOCKED worktree survives (git prune skips locked).
	if err := os.RemoveAll(uc.Path); err != nil {
		t.Fatal(err)
	}
	if err := f.store.PruneWorktrees(ctx, "shop"); err != nil {
		t.Fatalf("PruneWorktrees: %v", err)
	}
	if !worktreeListed(ctx, f, e.StorePath, uc.Path) {
		t.Fatal("a locked worktree with its dir absent was pruned (locked must survive)")
	}

	// Unlock, then prune → the (absent) worktree is pruned.
	if _, err := f.store.git(ctx, e.StorePath, identity{}, "worktree", "unlock", uc.Path); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := f.store.PruneWorktrees(ctx, "shop"); err != nil {
		t.Fatalf("PruneWorktrees: %v", err)
	}
	if worktreeListed(ctx, f, e.StorePath, uc.Path) {
		t.Fatal("an unlocked worktree with its dir absent survived prune")
	}
}

// TestOrphanWorktreeFlaggedNeverDeleted (F15): a prunable admin entry (worktree
// dir removed) is FLAGGED as orphan by the GC pass, never deleted.
func TestOrphanWorktreeFlaggedNeverDeleted(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "shop", "task-1")

	// Remove the worktree dir out from under git → the admin entry is orphaned.
	if err := os.RemoveAll(ws.Path); err != nil {
		t.Fatal(err)
	}
	flags, err := f.store.FlagWorktrees(ctx, "shop")
	if err != nil {
		t.Fatalf("FlagWorktrees: %v", err)
	}
	var orphan bool
	for _, fl := range flags {
		if filepath.Clean(fl.Path) == filepath.Clean(ws.Path) {
			orphan = true
		}
	}
	if !orphan {
		t.Fatalf("orphaned worktree not flagged: %+v", flags)
	}
	// FlagWorktrees only flags — the admin entry is NOT deleted.
	if !worktreeListed(ctx, f, e.StorePath, ws.Path) {
		t.Fatal("FlagWorktrees deleted the orphan admin entry (it must only flag)")
	}
}

func worktreeListed(ctx context.Context, f *fix, store, path string) bool {
	list, err := f.store.worktrees(ctx, store)
	if err != nil {
		return false
	}
	for _, w := range list {
		if filepath.Clean(w.Path) == filepath.Clean(path) {
			return true
		}
	}
	return false
}
