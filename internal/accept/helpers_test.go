package accept_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_ALLOW_PROTOCOL=file", "HOME=/nonexistent",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@x")
}

func (f *fix) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *fix) runCmd(name string, args ...string) {
	f.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("%s: %v\n%s", name, err, out)
	}
}

func (f *fix) remoteMain() string {
	return f.git("--git-dir="+f.remote, "rev-parse", "refs/heads/main")
}

// seedRemote makes a bare remote with a base commit on main and returns its
// path and the base sha.
func (f *fix) seedRemote(baseBody string) (string, string) {
	f.t.Helper()
	src := filepath.Join(f.t.TempDir(), "seed")
	f.git("init", "-q", "-b", "main", src)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte(baseBody), 0o600)
	f.git("-C", src, "add", "-A")
	f.git("-C", src, "commit", "-q", "-m", "base")
	bare := filepath.Join(f.t.TempDir(), "remote.git")
	f.git("init", "--bare", "-q", "-b", "main", bare)
	f.git("-C", src, "push", "file://"+bare, "HEAD:refs/heads/main")
	base := f.git("-C", src, "rev-parse", "HEAD")
	return bare, base
}

// mkTaskRun inserts a task and creates a run (for the review revision's
// run-scoped mint event).
func (f *fix) mkTaskRun(taskID, runID, userID string) {
	f.t.Helper()
	f.exec(`INSERT OR IGNORE INTO users (user_id, role, created_ts) VALUES (?, 'operator', ?)`, userID, nowStr())
	f.exec(`INSERT INTO tasks (task_id, user_id, created_ts) VALUES (?, ?, ?)`, taskID, userID, nowStr())
	runs := run.NewStore(f.db, f.log)
	if _, err := runs.Create(f.ctx, run.NewRun{ID: runID, UserID: userID, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic"}); err != nil {
		f.t.Fatalf("create run: %v", err)
	}
}

func (f *fix) exec(query string, args ...any) {
	f.t.Helper()
	err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx, query, args...)
		return err
	})
	if err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

func (f *fix) storeBrokerKey(profile, key string) {
	f.t.Helper()
	if err := f.client.Store(profile, broker.KindGitSSHKey, key); err != nil {
		f.t.Fatalf("store broker key: %v", err)
	}
}

// startBroker spins up a real broker (AllowStore for the signing-key enrollment
// in the sign test) and returns a client.
func startBroker(t *testing.T) *broker.Client {
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
	srv.AllowStore = true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(done) }()
	c, err := broker.Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return c
}

func nowStr() string { return time.Now().UTC().Format(time.RFC3339Nano) }
