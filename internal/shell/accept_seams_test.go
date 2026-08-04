package shell

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// TestBuildAcceptSurfaceComposition (F1): the accept orchestration is driven
// through the SHELL-CONSTRUCTED object — buildAcceptSurface with the production
// stores, a real broker (server+client), and file:// git fixtures — proving it
// is compiled in and reachable. It exercises the class-A effect, the broker CAS
// push, the accepted state move, and the sibling-accept freshness fire. No
// test-local mirror of the wiring: the test calls the same buildAcceptSurface
// the shell does (the B4-2 F9 real-seams precedent).
func TestBuildAcceptSurfaceComposition(t *testing.T) {
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	log := eventlog.New(db, reg)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatal(err)
	}
	reviewStore := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(t.TempDir(), "review")}
	journal, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg})
	if err != nil {
		t.Fatal(err)
	}
	runs := run.NewStore(db, log)
	socket := startCompBroker(t)

	// A project cloned from a seeded bare remote + an in-review deliverable
	// whose candidate is a run-branch snapshot.
	remote := seedCompRemote(t)
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{
		ProjectID: "proj", Owner: "u1", Name: "P", Source: remote, RemoteURL: "file://" + remote,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := proj.Approve(ctx, "proj", "u1", nil); err != nil {
		t.Fatal(err)
	}
	ws, err := proj.EnsureWorkspace(ctx, "proj", "pipe1")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(ws.Path, "a.txt"), []byte("candidate\n"), 0o600)
	candidate, err := proj.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := proj.CreateRevisionRef(ctx, "proj", review.RevisionRef("dlv", 1), candidate); err != nil {
		t.Fatal(err)
	}
	// Seed a task, an ACTIVE run in the project, the deliverable + revision.
	compSeed(t, ctx, db, log, runs)
	if _, err := reviewStore.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv", Owner: "u1", TaskID: "t1", ProjectID: "proj", Type: "markdown",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewStore.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv", N: 1, RunID: "r1", AttemptRef: "r1#round-1",
		Files: map[string]string{"a.txt": "candidate\n"}, SnapshotSHA: candidate,
	}); err != nil {
		t.Fatal(err)
	}

	// THE SHELL WIRING: buildAcceptSurface with the production stores.
	surf, err := buildAcceptSurface(acceptDeps{
		Proj: proj, Journal: journal, Review: reviewStore, Runs: runs,
		DB: db, Log: log, Settings: reg, BrokerSocket: socket,
		Pipeline: nil, // FollowUp.Start not exercised here (its own intake test covers it)
		ProjectForTask: func(_ context.Context, taskID string) (string, error) {
			if taskID == "t1" {
				return "proj", nil
			}
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("buildAcceptSurface: %v", err)
	}
	if surf.Accepter == nil || surf.FollowUp == nil {
		t.Fatal("accept surface incomplete")
	}

	out, err := surf.Accepter.Accept(ctx, accept.Input{
		DeliverableID: "dlv", AcceptingUser: "u1", AcceptingUserName: "User One", ProjectID: "proj",
		Subject: "feat: ship it", Engine: "claude-cli", Model: "opus-4-8", VendorNoreply: "noreply@anthropic.invalid",
	})
	if err != nil {
		t.Fatalf("Accept through the shell-constructed object: %v", err)
	}
	if !out.Accepted || out.Card != nil {
		t.Fatalf("accept did not complete: %+v", out)
	}
	// The class-A effect succeeded; the deliverable is accepted.
	if eff, _ := journal.Get(ctx, out.EffectID); eff.Class != gates.ClassA || eff.State != gates.EffectSucceeded {
		t.Errorf("effect not a succeeded class-A: %+v", eff)
	}
	if d, _ := reviewStore.Deliverable(ctx, "dlv"); d.State != review.StateAccepted {
		t.Errorf("deliverable state %q, want accepted", d.State)
	}
	// The remote's protected ref advanced to the accept commit.
	if tip := compGit(t, "--git-dir="+remote, "rev-parse", "refs/heads/main"); tip != out.Commit {
		t.Errorf("remote main %s != accept commit %s", tip, out.Commit)
	}
	// The sibling-accept freshness trigger fired to the active project run.
	if len(out.RoutedRuns) != 1 || out.RoutedRuns[0] != "r1" {
		t.Errorf("sibling-accept did not route the active run: %v", out.RoutedRuns)
	}
}

func compSeed(t *testing.T, ctx context.Context, db *storage.DB, log *eventlog.Log, runs *run.Store) {
	t.Helper()
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO users (user_id, role, created_ts) VALUES ('u1','operator','t')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO tasks (task_id, user_id, created_ts) VALUES ('t1','u1','t')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r, err := runs.Create(ctx, run.NewRun{ID: "r1", UserID: "u1", TaskID: "t1", Substrate: "claude-cli", Lane: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	// Move the run to an ACTIVE (non-terminal) state so the sibling-accept scan
	// finds it: new → queued → claimed → running.
	_ = r
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, "r1", to, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
}

func startCompBroker(t *testing.T) string {
	t.Helper()
	store, err := broker.OpenStore(t.TempDir(), "op")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "broker.sock")
	ln, err := broker.Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := broker.NewServer(store, uint32(os.Getuid()), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return socket
}

func seedCompRemote(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "seed")
	compGit(t, "init", "-q", "-b", "main", src)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("base\n"), 0o600)
	compGit(t, "-C", src, "add", "-A")
	compGit(t, "-C", src, "commit", "-q", "-m", "base")
	bare := filepath.Join(t.TempDir(), "remote.git")
	compGit(t, "init", "--bare", "-q", "-b", "main", bare)
	compGit(t, "-C", src, "push", "file://"+bare, "HEAD:refs/heads/main")
	return bare
}

func compGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_ALLOW_PROTOCOL=file", "HOME=/nonexistent",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@x")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
