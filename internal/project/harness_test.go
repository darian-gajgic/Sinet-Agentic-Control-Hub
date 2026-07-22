package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13.5/S13.7 acceptance harness: a real platform.db (migration 0008
// applied), the real event log, the project store rooted under t.TempDir(),
// and hermetic git fixtures. Every git test runs against LOCAL fixture repos
// — no network, no host git config (CONVENTIONS §3; the packet constraint).

type fix struct {
	t     *testing.T
	db    *storage.DB
	log   *eventlog.Log
	store *Store
	root  string
}

func newFix(t *testing.T) *fix {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	root := filepath.Join(t.TempDir(), "projects")
	store, err := New(Config{DB: db, Log: log, Root: root})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	return &fix{t: t, db: db, log: log, store: store, root: root}
}

// gitEnv is the hermetic env for fixture git — identical posture to the
// package's own baseEnv so fixtures never read/write host config.
func gitEnv() []string {
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

// git runs a git command in dir for test setup/inspection, failing the test on
// error.
func (f *fix) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitCode runs a git command returning its exit code (for probing pruned
// objects: `cat-file -e` exits non-zero when absent).
func (f *fix) gitCode(dir string, args ...string) int {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		f.t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return 0
}

// fixtureRepo builds a local git repository with the given files committed on
// its default branch and returns its path (a clone source / project store).
func (f *fix) fixtureRepo(files map[string]string) string {
	f.t.Helper()
	dir := filepath.Join(f.t.TempDir(), "fixture-repo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		f.t.Fatal(err)
	}
	f.git(dir, "init", "-q", "--initial-branch=main", dir)
	f.writeFiles(dir, files)
	f.git(dir, "add", "-A")
	f.git(dir, "commit", "-q", "-m", "fixture seed")
	return dir
}

// writeFiles writes a map of relative-path → content under dir.
func (f *fix) writeFiles(dir string, files map[string]string) {
	f.t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			f.t.Fatal(err)
		}
	}
}

// activeProject onboards + approves a project from a fixture repo, returning
// the active entry.
func (f *fix) activeProject(projectID, owner, name string, files map[string]string) Entry {
	f.t.Helper()
	ctx := context.Background()
	src := f.fixtureRepo(files)
	_, _, err := f.store.Onboard(ctx, OnboardInput{ProjectID: projectID, Owner: owner, Name: name, Source: src})
	if err != nil {
		f.t.Fatalf("Onboard: %v", err)
	}
	e, err := f.store.Approve(ctx, projectID, owner, nil)
	if err != nil {
		f.t.Fatalf("Approve: %v", err)
	}
	return e
}
