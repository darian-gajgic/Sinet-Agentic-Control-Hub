package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoHeadIsDefaultBranchTip(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	head, err := f.store.RepoHead(ctx, "shop")
	if err != nil {
		t.Fatalf("RepoHead: %v", err)
	}
	want := f.git(e.StorePath, "rev-parse", "refs/heads/main")
	if head != want {
		t.Fatalf("RepoHead = %q, want the default-branch tip %q", head, want)
	}
	// A pending (non-active) project supplies no repo HEAD (never faked, R33).
	if _, _, err := f.store.Onboard(ctx, OnboardInput{ProjectID: "draft", Owner: "alice", Name: "draft", Source: f.fixtureRepo(map[string]string{"a": "b"})}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := f.store.RepoHead(ctx, "draft"); err == nil {
		t.Fatal("RepoHead on a pending project should error rather than supply a head")
	}
}

func TestBranchPerPipelineWorktree(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})

	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if ws.Branch != "sinet/run/task-1" {
		t.Fatalf("branch = %q, want sinet/run/task-1", ws.Branch)
	}
	// The worktree is checked out on the run branch.
	if b := f.git(ws.Path, "rev-parse", "--abbrev-ref", "HEAD"); b != "sinet/run/task-1" {
		t.Fatalf("worktree HEAD branch = %q, want the run branch", b)
	}
	// The branch base is project HEAD at creation (Spec S13.1 rev-1 old side).
	mainHead := f.git(e.StorePath, "rev-parse", "refs/heads/main")
	if ws.Base != mainHead {
		t.Fatalf("base = %q, want default-branch HEAD %q", ws.Base, mainHead)
	}
	// Idempotent: a second call returns the SAME worktree and branch — commits
	// accumulate across stages/rounds on one branch (Spec S13.5).
	ws2, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace #2: %v", err)
	}
	if ws2.Path != ws.Path || ws2.Branch != ws.Branch {
		t.Fatalf("second EnsureWorkspace diverged: %+v vs %+v", ws2, ws)
	}
}

func TestSnapshotTreeLevelJunkExcluded(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}

	// Simulate a bash side effect: a NEW untracked file (tool edits AND bash
	// output — Spec S13.5 "capture bash side effects, not just tool edits") and
	// a junk file that platform ignore rules must exclude.
	f.writeFiles(ws.Path, map[string]string{
		"generated/out.txt": "produced by a bash step\n",
		"debug.log":         "junk that must never enter a snapshot\n",
	})
	if err := os.MkdirAll(filepath.Join(ws.Path, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	f.writeFiles(ws.Path, map[string]string{"node_modules/pkg.js": "junk dep\n"})

	sha, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	tree := f.git(ws.Path, "ls-tree", "-r", "--name-only", "HEAD")
	if !strings.Contains(tree, "generated/out.txt") {
		t.Fatalf("snapshot did not capture the untracked bash-side-effect file:\n%s", tree)
	}
	if strings.Contains(tree, "debug.log") || strings.Contains(tree, "node_modules/") {
		t.Fatalf("snapshot captured junk (platform ignore rules not applied):\n%s", tree)
	}
	// The snapshot is authored as the platform with per-invocation identity.
	if an := f.git(ws.Path, "log", "-1", "--format=%an"); an != "Sinet platform" {
		t.Fatalf("snapshot author = %q, want Sinet platform", an)
	}
	if ae := f.git(ws.Path, "log", "-1", "--format=%ae"); ae != "platform@sinet.invalid" {
		t.Fatalf("snapshot email = %q, want platform@sinet.invalid", ae)
	}
	if sha == ws.Base {
		t.Fatal("snapshot did not advance the run branch past the base")
	}
}

func TestSnapshotNoChangeNoEmptyCommit(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "shop", "task-1")

	f.writeFiles(ws.Path, map[string]string{"a.txt": "one\n"})
	first, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	countBefore := f.git(ws.Path, "rev-list", "--count", "HEAD")

	// Nothing changed → the same sha, and NO new (empty) commit (Spec S13.5;
	// still-valid D7 material).
	second, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot #2: %v", err)
	}
	if second != first {
		t.Fatalf("no-change snapshot returned a new sha: %s -> %s", first, second)
	}
	if countAfter := f.git(ws.Path, "rev-list", "--count", "HEAD"); countAfter != countBefore {
		t.Fatalf("no-change snapshot created a commit: %s -> %s", countBefore, countAfter)
	}

	// A further change makes a new commit on the SAME branch (accumulation).
	f.writeFiles(ws.Path, map[string]string{"b.txt": "two\n"})
	third, _ := f.store.Snapshot(ctx, ws.Path)
	if third == second {
		t.Fatal("a real change produced no new snapshot commit")
	}
	if anc := f.gitCode(ws.Path, "merge-base", "--is-ancestor", second, third); anc != 0 {
		t.Fatal("snapshots did not accumulate on the same branch (second is not an ancestor of third)")
	}
}

func TestFreshAttemptNewBranchOldStays(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws1, _ := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	f.writeFiles(ws1.Path, map[string]string{"work.txt": "attempt 1\n"})
	if _, err := f.store.Snapshot(ctx, ws1.Path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	ws2, err := f.store.FreshAttempt(ctx, "shop", "task-1", 2)
	if err != nil {
		t.Fatalf("FreshAttempt: %v", err)
	}
	if ws2.Branch == ws1.Branch {
		t.Fatalf("fresh attempt reused the branch %q", ws2.Branch)
	}
	// The old attempt's branch STAYS (Spec S13.5: never rewriting the old one).
	if sha, _ := f.store.refSHA(ctx, e.StorePath, "refs/heads/"+ws1.Branch); sha == "" {
		t.Fatal("fresh attempt deleted the old run branch")
	}
}

func TestAdvanceRunBranchRefusesNonFastForward(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	f.writeFiles(ws.Path, map[string]string{"a.txt": "one\n"})
	tip, _ := f.store.Snapshot(ctx, ws.Path)

	// A divergent commit that is NOT a descendant of the current tip.
	divergent := f.git(e.StorePath, "commit-tree", ws.Base+"^{tree}", "-p", ws.Base, "-m", "divergent")
	if err := f.store.AdvanceRunBranch(ctx, "shop", "task-1", 1, divergent); !errors.Is(err, ErrNotFastForward) {
		t.Fatalf("non-ff advance: err = %v, want ErrNotFastForward", err)
	}
	// The run branch is unmoved by the refused move.
	if cur, _ := f.store.refSHA(ctx, e.StorePath, "refs/heads/"+ws.Branch); cur != tip {
		t.Fatalf("refused advance moved the branch: %s -> %s", tip, cur)
	}

	// A genuine fast-forward is allowed.
	f.writeFiles(ws.Path, map[string]string{"b.txt": "two\n"})
	newTip, _ := f.store.Snapshot(ctx, ws.Path)
	// reset the branch back and prove AdvanceRunBranch can fast-forward it.
	f.git(e.StorePath, "update-ref", "refs/heads/"+ws.Branch, tip)
	if err := f.store.AdvanceRunBranch(ctx, "shop", "task-1", 1, newTip); err != nil {
		t.Fatalf("fast-forward advance: %v", err)
	}
}

func TestMintedRevisionRefSurvivesBranchGC(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "shop", "task-1")

	f.writeFiles(ws.Path, map[string]string{"rev1.txt": "revision one\n"})
	protected, _ := f.store.Snapshot(ctx, ws.Path)
	// Create the platform retention ref for the minted revision (R20).
	ref := "refs/sinet/deliverable/dlv-task-1/rev-1"
	if err := f.store.CreateRevisionRef(ctx, "shop", ref, protected); err != nil {
		t.Fatalf("CreateRevisionRef: %v", err)
	}
	// A second, UN-referenced snapshot commit (the safety net, not a revision).
	f.writeFiles(ws.Path, map[string]string{"scratch.txt": "not a revision\n"})
	unprotected, _ := f.store.Snapshot(ctx, ws.Path)

	// Abandon the attempt: retention GC removes the worktree + branch.
	if err := f.store.CollectRunWorktree(ctx, "shop", "task-1", 1); err != nil {
		t.Fatalf("CollectRunWorktree: %v", err)
	}
	// Force a real gc prune; the minted-revision commit MUST survive via its
	// platform ref, the un-referenced snapshot must NOT (Spec S13.1).
	f.git(e.StorePath, "gc", "--prune=now", "--quiet")
	if f.gitCode(e.StorePath, "cat-file", "-e", protected+"^{commit}") != 0 {
		t.Fatal("minted-revision commit was GC'd despite its platform ref (Spec S13.1)")
	}
	if f.gitCode(e.StorePath, "cat-file", "-e", unprotected+"^{commit}") == 0 {
		t.Fatal("an un-referenced snapshot commit survived GC — the ref is meant to be load-bearing")
	}
}

func TestUtilityCheckoutLifecycle(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	base := f.git(e.StorePath, "rev-parse", "refs/heads/main")

	uc, err := f.store.AddUtilityCheckout(ctx, "shop", base)
	if err != nil {
		t.Fatalf("AddUtilityCheckout: %v", err)
	}
	// Created LOCKED (Spec S13.5): a prune must not remove it.
	if err := f.store.PruneWorktrees(ctx, "shop"); err != nil {
		t.Fatalf("PruneWorktrees: %v", err)
	}
	if _, err := os.Stat(uc.Path); err != nil {
		t.Fatalf("locked utility checkout was pruned: %v", err)
	}

	// A DIRTY utility checkout is never auto-deleted (Spec S13.5).
	f.writeFiles(uc.Path, map[string]string{"dirty.txt": "uncommitted\n"})
	if err := f.store.ReleaseUtilityCheckout(ctx, "shop", uc.Path); !errors.Is(err, ErrDirty) {
		t.Fatalf("release dirty: err = %v, want ErrDirty", err)
	}
	if _, err := os.Stat(uc.Path); err != nil {
		t.Fatal("a dirty utility checkout was deleted despite the never-delete-dirty rule")
	}

	// Clean it → release removes it, prune-swept.
	if err := os.Remove(filepath.Join(uc.Path, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseUtilityCheckout(ctx, "shop", uc.Path); err != nil {
		t.Fatalf("release clean: %v", err)
	}
	if _, err := os.Stat(uc.Path); !os.IsNotExist(err) {
		t.Fatalf("clean utility checkout not removed: %v", err)
	}
}

func TestGCFlagsDirtyOrphanNeverDeletes(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "shop", "task-1")

	// Uncommitted work in the run worktree → flagged, never deleted.
	f.writeFiles(ws.Path, map[string]string{"wip.txt": "in progress\n"})
	flags, err := f.store.FlagWorktrees(ctx, "shop")
	if err != nil {
		t.Fatalf("FlagWorktrees: %v", err)
	}
	var flagged bool
	for _, fl := range flags {
		if fl.Path == ws.Path || filepath.Clean(fl.Path) == filepath.Clean(ws.Path) {
			flagged = true
		}
	}
	if !flagged {
		t.Fatalf("dirty run worktree not flagged: %+v", flags)
	}
	if _, err := os.Stat(ws.Path); err != nil {
		t.Fatal("FlagWorktrees deleted a worktree (it must only flag)")
	}
	// A dirty run worktree also refuses retention GC (ErrDirty).
	if err := f.store.CollectRunWorktree(ctx, "shop", "task-1", 1); !errors.Is(err, ErrDirty) {
		t.Fatalf("collect dirty run worktree: err = %v, want ErrDirty", err)
	}
}
