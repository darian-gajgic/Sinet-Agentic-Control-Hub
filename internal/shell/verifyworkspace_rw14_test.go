package shell

// verifyworkspace_rw14_test.go — P3-RW-14 R6: the REAL verification-workspace
// substrate, over a real git repository.
//
// Spec S07.3 rule 1 is a MUST: the verification workspace carries no
// answer-bearing VCS history (P-T06-2 — 63% of one frontier model's benchmark
// successes were retrieved rather than derived, so a suite run with history
// present measures lookup ability, not work). The materialization is the §25
// pair: a locked utility checkout at the revision's snapshot pin, then a
// `.git`-less copy. This binds the copy against a checkout git actually made —
// which is the only way to catch that a worktree's `.git` is a FILE pointer,
// not a directory.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

func rw14GitEnv() []string {
	env := []string{
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file",
		"HOME=/nonexistent",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@test.invalid",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@test.invalid",
	}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	return env
}

func rw14Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, rw14GitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestRW14StrippedRevisionCarriesTheWorkAndNoHistory: the materialization a
// project-backed verification runs its checks in holds the revision's files
// and no VCS history, and the locked checkout it came from is released clean.
func TestRW14StrippedRevisionCarriesTheWorkAndNoHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	db, log, _ := seamDB(t)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: t.TempDir()})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}

	// A real source repository with real history — the history this must strip.
	src := t.TempDir()
	rw14Git(t, src, "init", "-q", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("# shop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rw14Git(t, src, "add", "-A")
	rw14Git(t, src, "commit", "-qm", "THE ANSWER IS IN THE HISTORY")

	if _, _, err := proj.Onboard(ctx, project.OnboardInput{
		ProjectID: "shop", Owner: "alice", Name: "shop", Source: src,
	}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// The execute leg's shape: a worktree, real files written into it, and the
	// stage-close snapshot commit that pins them.
	ws, err := proj.EnsureWorkspace(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ws.Path, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"app/main.py": "print('shop')\n",
		"run.sh":      "#!/bin/sh\necho ok\n",
	} {
		if err := os.WriteFile(filepath.Join(ws.Path, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// An executable check script must stay executable, or a captured
	// `./run.sh` check would fail for a reason that has nothing to do with
	// the work.
	if err := os.Chmod(filepath.Join(ws.Path, "run.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	sha, err := proj.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if sha == "" {
		t.Fatal("no snapshot commit for a worktree that was written to")
	}

	// The R6 substrate, exactly as VerificationWorkspace composes it.
	checkout, err := proj.AddUtilityCheckout(ctx, "shop", sha)
	if err != nil {
		t.Fatalf("AddUtilityCheckout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout.Path, ".git")); err != nil {
		t.Fatalf("the checkout has no .git to strip — the fixture proves nothing: %v", err)
	}
	scratch := filepath.Join(t.TempDir(), "verify-workspaces")
	dir, err := strippedCopy(scratch, checkout.Path)
	if err != nil {
		t.Fatalf("strippedCopy: %v", err)
	}

	// The work is there.
	for _, name := range []string{"app/main.py", "run.sh", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("the checks would not see %s: %v", name, err)
		}
	}
	// The history is not — in either of its forms.
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("VCS history survived into the verification workspace (err %v) — a pass there can measure lookup, not work (S07.3 rule 1)", err)
	}
	// And nothing in the tree can lead back to it.
	if err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			t.Errorf("a .git entry survived at %s", p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The executable bit rides along.
	info, err := os.Stat(filepath.Join(dir, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Errorf("run.sh lost its executable bit (mode %v) — a captured ./run.sh check would fail for the wrong reason", info.Mode())
	}
	// The workspace is platform-owned, never system temp (§25 / F16).
	if !strings.HasPrefix(dir, scratch) {
		t.Errorf("materialized outside the platform scratch root: %q", dir)
	}

	// The checks never dirty the checkout, so releasing it succeeds — a
	// dirtied checkout is a loud refusal, and a leak would accumulate locked
	// worktrees for every verification the platform ever runs.
	if err := proj.ReleaseUtilityCheckout(ctx, "shop", checkout.Path); err != nil {
		t.Fatalf("ReleaseUtilityCheckout: %v", err)
	}
	if _, err := os.Stat(checkout.Path); !os.IsNotExist(err) {
		t.Errorf("the locked checkout outlived its release (err %v)", err)
	}
}

// A symlink must not ride into the verification workspace: the checks read
// plain source, and a link pointing out of the tree is exactly the escape the
// strip exists to prevent.
func TestRW14StrippedCopyDropsSymlinks(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "real.txt"), []byte("in\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dir, err := strippedCopy(filepath.Join(t.TempDir(), "scratch"), src)
	if err != nil {
		t.Fatalf("strippedCopy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "real.txt")); err != nil {
		t.Errorf("real file missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "escape")); !os.IsNotExist(err) {
		t.Errorf("a symlink out of the tree survived into the verification workspace (err %v)", err)
	}
}
