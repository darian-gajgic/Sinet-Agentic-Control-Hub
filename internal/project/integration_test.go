package project_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The composition-root flow the shell wires (Spec S13.5/S13.1, R19/R20/R25):
// project + review composed exactly as the projectSeams adapter does, proving
// the git side (snapshot commit + platform ref + base content) and the
// review side (snapshot_sha fill + Compare old-side 0) work end-to-end.

type baseSeam struct {
	proj    *project.Store
	project string
	task    string
}

func (b baseSeam) BaseContent(ctx context.Context, deliverableID string) (map[string]string, bool, error) {
	files, err := b.proj.BaseContent(ctx, b.project, b.task)
	if err != nil || len(files) == 0 {
		return nil, false, err
	}
	return files, true, nil
}

func gitFixtureRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "src")
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent",
		"GIT_AUTHOR_NAME=fx", "GIT_AUTHOR_EMAIL=fx@t.invalid",
		"GIT_COMMITTER_NAME=fx", "GIT_COMMITTER_EMAIL=fx@t.invalid")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	os.MkdirAll(dir, 0o700)
	run("init", "-q", "--initial-branch=main", dir)
	for rel, c := range files {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(c), 0o600)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "seed")
	return dir
}

func TestSnapshotToRevisionAndBaseComposition(t *testing.T) {
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
	runs := run.NewStore(db, log)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(t.TempDir(), "review")}

	// Onboard + activate a code project (the base carries a main.go).
	src := gitFixtureRepo(t, map[string]string{"main.go": "package main\n"})
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop", Source: src}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// A task + run for review's run-scoped mint event.
	mustExec(t, db, ctx, `INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES ('t1','alice','shop change', ?)`, now())
	if _, err := runs.Create(ctx, run.NewRun{ID: "t1.execute", UserID: "alice", TaskID: "t1", Substrate: "claude-cli", Lane: "anthropic"}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	// The execute leg: worktree on the run branch, write, snapshot.
	ws, err := proj.EnsureWorkspace(ctx, "shop", "t1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshotSHA, err := proj.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// The verification handoff mint records the snapshot sha (R19) and the
	// composition creates the platform ref (R20).
	dlvID := "dlv-t1"
	if _, err := rev.EnsureDeliverable(ctx, review.EnsureInput{ID: dlvID, Owner: "alice", TaskID: "t1", Type: "code"}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := rev.MintRevision(ctx, review.MintInput{
		DeliverableID: dlvID, N: 1, RunID: "t1.execute",
		Files:       map[string]string{"main.go": "package main\nfunc main() {}\n"},
		SnapshotSHA: snapshotSHA,
	}); err != nil {
		t.Fatalf("MintRevision: %v", err)
	}
	ref := review.RevisionRef(dlvID, 1)
	if err := proj.CreateRevisionRef(ctx, "shop", ref, snapshotSHA); err != nil {
		t.Fatalf("CreateRevisionRef: %v", err)
	}

	// The minted revision carries the snapshot sha, and the platform ref
	// resolves to the snapshot commit.
	got, _ := rev.RevisionAt(ctx, dlvID, 1)
	if got.SnapshotSHA != snapshotSHA {
		t.Fatalf("revision snapshot_sha = %q, want %q", got.SnapshotSHA, snapshotSHA)
	}
	e, _ := proj.Get(ctx, "shop")
	if refSHA := gitOut(t, e.StorePath, "rev-parse", ref); refSHA != snapshotSHA {
		t.Fatalf("platform ref → %q, want the snapshot commit %q", refSHA, snapshotSHA)
	}

	// Compare(0,1) via the base-content seam: real branch base → rev 1.
	rev.BaseContent = baseSeam{proj: proj, project: "shop", task: "t1"}
	cmp, err := rev.Compare(ctx, dlvID, 0, 1)
	if err != nil {
		t.Fatalf("Compare(0,1): %v", err)
	}
	if !strings.Contains(cmp.Unified, "+func main") {
		t.Fatalf("Compare(0,1) did not diff the real branch base → rev 1:\n%s", cmp.Unified)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func mustExec(t *testing.T, db *storage.DB, ctx context.Context, q string, args ...any) {
	t.Helper()
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
