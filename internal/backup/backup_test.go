package backup_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/backup"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13.10 acceptance battery: a real platform.db + event log, a real broker
// (server + client) for the CAS push, file:// bare-repo snapshot fixtures in
// t.TempDir(), and a FRESH temp age keypair for every run — the escrow identity
// file is NEVER read by tests, and no key material is hardcoded (R20/R23).

type harness struct {
	t      *testing.T
	ctx    context.Context
	db     *storage.DB
	log    *eventlog.Log
	reg    *settings.Registry
	crypto backup.Crypto
	id     *age.X25519Identity
	remote string
	local  string
	snap   *backup.Snapshotter
	clock  *time.Time
	keep   *int64
	pusher backup.Pusher
	now    func() time.Time
}

// keepSettings overlays ⚙ backup.keep for a fixture (the review driftSettings
// pattern); the rest delegates to the real registry.
type keepSettings struct {
	base *settings.Registry
	keep *int64
}

func (s keepSettings) Int(key string) (int64, error) {
	if key == "backup.keep" && *s.keep > 0 {
		return *s.keep, nil
	}
	return s.base.Int(key)
}

func newHarness(t *testing.T) *harness { return newHarnessWith(t, nil) }

// newHarnessWith builds the fixture; modify tweaks the Config (file stores,
// chunk threshold) before the Snapshotter is created.
func newHarnessWith(t *testing.T, modify func(*backup.Config)) *harness {
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
	seed(t, ctx, db, log)

	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	remote := initBareRemote(t)
	local := filepath.Join(t.TempDir(), "snap-local")

	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	clock := &now
	keep := int64(30)
	pusher := startBroker(t)
	crypto := backup.Crypto{Recipients: []age.Recipient{id.Recipient()}, Identity: id}

	cfg := backup.Config{
		DB: db, Log: log, Settings: keepSettings{base: reg, keep: &keep},
		Crypto: crypto, Pusher: pusher,
		Repo: backup.RepoConfig{LocalDir: local, Remote: "file://" + remote, Branch: "main"},
		Now:  func() time.Time { return *clock },
	}
	if modify != nil {
		modify(&cfg)
	}
	snap, err := backup.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &harness{t: t, ctx: ctx, db: db, log: log, reg: reg,
		crypto: crypto, id: id, remote: remote, local: local, snap: snap,
		clock: clock, keep: &keep, pusher: pusher, now: cfg.Now}
}

func seed(t *testing.T, ctx context.Context, db *storage.DB, log *eventlog.Log) {
	t.Helper()
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, display_name, role, created_ts) VALUES ('op','Operator','operator','2026-07-22T00:00:00Z')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES ('t1','op','Seed task','2026-07-22T00:00:00Z')`)
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := log.Append(ctx, eventlog.Append{
			UserID: "op", Type: "seed.event", SchemaVersion: 1,
			Payload: []byte(`{"n":` + string(rune('0'+i)) + `}`),
		}); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
}

// initBareRemote makes an empty bare snapshot repo (default branch main) and
// returns its path (the file:// URL is "file://"+path).
func initBareRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "snap-remote.git")
	runGit(t, "", "init", "--bare", "-b", "main", dir)
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
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

// startBroker spins up a real broker Server + Client over a UDS; the client
// satisfies backup.Pusher, so the snapshot push exercises the real CAS path.
func startBroker(t *testing.T) backup.Pusher {
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

func (h *harness) advance(d time.Duration) { *h.clock = h.clock.Add(d) }

// TestSnapshotAndDrillRoundTrip is the end-to-end acceptance: a snapshot
// produces one encrypted blob CAS-pushed to the fixture remote with a
// snapshot_ledger row, and the restore drill fetches it, verifies against the
// ledger, decrypts with the escrow identity, rebuilds the DB, and passes the
// S02.9 invariants — the live DB untouched.
func TestSnapshotAndDrillRoundTrip(t *testing.T) {
	h := newHarness(t)
	row, err := h.snap.Snapshot(h.ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if row.SHA256 == "" || row.SizeBytes == 0 || row.RepoCommit == "" {
		t.Fatalf("ledger row incomplete: %+v", row)
	}
	// The blob landed in the REMOTE (the broker pushed it), not merely locally.
	tip := runGit(t, "", "--git-dir="+h.remote, "rev-parse", "refs/heads/main")
	if tip != row.RepoCommit {
		t.Errorf("remote tip %s != ledger repo_commit %s", tip, row.RepoCommit)
	}

	liveBefore := liveEventCount(t, h)
	h.advance(time.Hour)
	res, err := h.snap.RestoreDrill(h.ctx)
	if err != nil {
		t.Fatalf("RestoreDrill: %v", err)
	}
	if !res.OK || !res.LedgerVerified || !res.Check.AllOK() {
		t.Fatalf("drill did not pass: %+v", res)
	}
	if res.Check.RunEventCount == 0 {
		t.Errorf("rebuilt run_events empty — seed events not restored")
	}
	// The live DB is not REBUILT by the drill (the rebuild lands in scratch);
	// the only live change is the drill's own audit event (R22: "an event-log
	// record either way"). So the live event log gained exactly one row and the
	// seed data is intact — it was never overwritten by the restore.
	if got := liveEventCount(t, h); got != liveBefore+1 {
		t.Errorf("live event log went %d -> %d, want +1 (only the drill event)", liveBefore, got)
	}
	if !seedTaskExists(t, h) {
		t.Error("seed data missing from the live DB — the drill overwrote it")
	}
	// The drill result is an event-log record.
	if !eventExists(t, h, backup.EventDrill) {
		t.Error("no restore-drill event recorded")
	}
}

// TestDrillFailsClosedOnSwappedBlob: a swapped remote blob fails the ledger
// check, aborts before decrypt, records a failure, and raises a High flag
// (P-T12-4).
func TestDrillFailsClosedOnSwappedBlob(t *testing.T) {
	h := newHarness(t)
	if _, err := h.snap.Snapshot(h.ctx); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// Swap the blob in the remote: check out, overwrite the chunk file, push.
	work := filepath.Join(t.TempDir(), "attacker")
	runGit(t, "", "clone", "file://"+h.remote, work)
	entries, _ := os.ReadDir(work)
	var chunk string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "snapshot-") {
			chunk = e.Name()
		}
	}
	if chunk == "" {
		t.Fatal("no snapshot blob found in the remote")
	}
	if err := os.WriteFile(filepath.Join(work, chunk), []byte("TAMPERED-CIPHERTEXT"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, work, "add", "-A")
	runGit(t, work, "-c", "commit.gpgsign=false", "commit", "-m", "tamper")
	runGit(t, work, "push", "file://"+h.remote, "HEAD:refs/heads/main")

	h.advance(time.Hour)
	res, err := h.snap.RestoreDrill(h.ctx)
	if err == nil {
		t.Fatal("drill did not fail closed on a swapped blob")
	}
	if res.OK || res.LedgerVerified {
		t.Errorf("drill reported success on a swapped blob: %+v", res)
	}
	if !eventExists(t, h, backup.EventDrillFlag) {
		t.Error("no High flag raised for the fail-closed drill")
	}
}

// TestKeepN: with ⚙ backup.keep overridden to 2, a third snapshot prunes the
// oldest blob from the remote tree while the ledger keeps all rows.
func TestKeepN(t *testing.T) {
	h := newHarness(t)
	h.setKeep(2)
	var bases []string
	for i := 0; i < 3; i++ {
		h.advance(time.Duration(i) * time.Hour)
		row, err := h.snap.Snapshot(h.ctx)
		if err != nil {
			t.Fatalf("Snapshot %d: %v", i, err)
		}
		bases = append(bases, row.BlobName)
	}
	// The remote tree keeps only the newest 2 snapshots' blobs.
	tree := runGit(t, "", "--git-dir="+h.remote, "ls-tree", "--name-only", "refs/heads/main")
	if strings.Contains(tree, bases[0]) {
		t.Errorf("oldest snapshot %s not pruned; tree:\n%s", bases[0], tree)
	}
	for _, b := range bases[1:] {
		if !strings.Contains(tree, b) {
			t.Errorf("retained snapshot %s missing; tree:\n%s", b, tree)
		}
	}
	// The ledger keeps every row (append-only).
	var n int
	if err := h.db.QueryRowContext(h.ctx, "SELECT count(*) FROM snapshot_ledger").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("snapshot_ledger has %d rows, want 3 (append-only, retention prunes blobs not rows)", n)
	}
}

func (h *harness) setKeep(n int64) { *h.keep = n }

func liveEventCount(t *testing.T, h *harness) int {
	t.Helper()
	var n int
	if err := h.db.QueryRowContext(h.ctx, "SELECT count(*) FROM run_events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func seedTaskExists(t *testing.T, h *harness) bool {
	t.Helper()
	var n int
	if err := h.db.QueryRowContext(h.ctx, "SELECT count(*) FROM tasks WHERE task_id = 't1'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func eventExists(t *testing.T, h *harness, typ string) bool {
	t.Helper()
	var n int
	if err := h.db.QueryRowContext(h.ctx, "SELECT count(*) FROM run_events WHERE type = ?", typ).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}
