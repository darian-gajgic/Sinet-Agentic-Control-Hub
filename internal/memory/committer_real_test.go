package memory_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
)

// TestFileCommitFillsThroughRealGitCommitter (F12): the S09.2 commit-on-approval
// composed end-to-end — the REAL memory.Gate driving the REAL project.GitCommitter
// on a fixture knowledge dir. A file-backed approval produces a real git commit
// authored by the approver (D9), fills knowledge_entries.file_commit NULL→hash,
// and an identical re-approval is a no-op that returns the covering commit (R27).
func TestFileCommitFillsThroughRealGitCommitter(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")

	proj, err := project.New(project.Config{DB: f.db, Log: f.log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	f.gate.Committer = proj.Committer()

	e := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson,
		Title: "filed", Content: "a lesson body\n", FileBacked: true, FileName: "filed.md",
	})
	if e.FileCommit == "" {
		t.Fatal("file_commit not filled by the real committer")
	}
	// The row's file_commit is the fill (NULL→hash), content offloaded to the file.
	row, err := f.store.Get(ctx, e.ID)
	if err != nil || row.FileCommit != e.FileCommit || row.Content != "" {
		t.Fatalf("row = %+v (%v)", row, err)
	}

	// A REAL commit with author = the approver (D9), in the scope dir's repo.
	scopeDir := filepath.Join(f.root, "users", "alice")
	if an := gitField(t, scopeDir, "%an"); an != "alice" {
		t.Fatalf("commit author = %q, want the approver alice (D9)", an)
	}
	if ae := gitField(t, scopeDir, "%ae"); ae != "alice@users.noreply.sinet.invalid" {
		t.Fatalf("commit email = %q, want the ID-based no-reply form", ae)
	}

	// Idempotent identical re-approval: re-committing the same file returns the
	// covering commit, never erroring on an empty diff (R27).
	again, err := proj.Committer().Commit(ctx, scopeDir, []string{filepath.Join(scopeDir, "filed.md")}, "alice", "re-approve identical")
	if err != nil {
		t.Fatalf("idempotent re-commit: %v", err)
	}
	if again != e.FileCommit {
		t.Fatalf("identical re-approval made a new commit: %s -> %s", e.FileCommit, again)
	}
}

func gitField(t *testing.T, dir, format string) string {
	t.Helper()
	c := exec.Command("git", "log", "-1", "--format="+format)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}
