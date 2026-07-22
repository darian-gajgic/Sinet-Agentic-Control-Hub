package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitterCommitsAsApprover(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	c := f.store.Committer()

	dir := filepath.Join(t.TempDir(), "knowledge", "house")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "entry.md")
	f.writeFiles(dir, map[string]string{"entry.md": "# a house convention\n"})

	// init-if-absent + commit-on-approval with author = the approver (D9).
	hash, err := c.Commit(ctx, dir, []string{file}, "alice", "knowledge: convention v1")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if hash == "" {
		t.Fatal("Commit returned an empty hash")
	}
	if an := f.git(dir, "log", "-1", "--format=%an"); an != "alice" {
		t.Fatalf("commit author = %q, want the approver alice (D9)", an)
	}
	if ae := f.git(dir, "log", "-1", "--format=%ae"); ae != "alice@users.noreply.sinet.invalid" {
		t.Fatalf("commit email = %q, want the ID-based no-reply form", ae)
	}
	// The file is committed.
	if tree := f.git(dir, "ls-tree", "-r", "--name-only", "HEAD"); tree != "entry.md" {
		t.Fatalf("committed tree = %q, want entry.md", tree)
	}
}

func TestCommitterNoOpToleranceOnIdenticalContent(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	c := f.store.Committer()
	dir := filepath.Join(t.TempDir(), "knowledge")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "e.md")
	f.writeFiles(dir, map[string]string{"e.md": "body\n"})

	first, err := c.Commit(ctx, dir, []string{file}, "alice", "v1")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// Re-committing identical content must NOT error — it returns the covering
	// commit (HEAD), the idempotent re-approval path (R27).
	second, err := c.Commit(ctx, dir, []string{file}, "alice", "v1 again")
	if err != nil {
		t.Fatalf("idempotent re-commit: %v", err)
	}
	if second != first {
		t.Fatalf("no-op re-commit made a new commit: %s -> %s", first, second)
	}
}

func TestCommitterUsesEnclosingRepo(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	c := f.store.Committer()
	root := filepath.Join(t.TempDir(), "knowledge")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	// The shell pre-inits the knowledge root as ONE repo (EnsureRepo).
	if err := f.store.EnsureRepo(ctx, root); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}
	houseDir := filepath.Join(root, "house")
	userDir := filepath.Join(root, "users", "alice")
	if err := os.MkdirAll(houseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		t.Fatal(err)
	}
	f.writeFiles(houseDir, map[string]string{"h.md": "house\n"})
	f.writeFiles(userDir, map[string]string{"u.md": "user\n"})

	if _, err := c.Commit(ctx, houseDir, []string{filepath.Join(houseDir, "h.md")}, "op", "house v1"); err != nil {
		t.Fatalf("Commit house: %v", err)
	}
	if _, err := c.Commit(ctx, userDir, []string{filepath.Join(userDir, "u.md")}, "alice", "user v1"); err != nil {
		t.Fatalf("Commit user: %v", err)
	}
	// Both scope dirs committed into the SAME enclosing repo (not nested).
	top := f.git(userDir, "rev-parse", "--show-toplevel")
	if filepath.Clean(top) != filepath.Clean(root) {
		t.Fatalf("user scope committed into a nested repo %q, want the knowledge root %q", top, root)
	}
	tree := f.git(root, "ls-tree", "-r", "--name-only", "HEAD")
	if !strings.Contains(tree, "house/h.md") || !strings.Contains(tree, "users/alice/u.md") {
		t.Fatalf("enclosing repo tree missing scope files:\n%s", tree)
	}
}
