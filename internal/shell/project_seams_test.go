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

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

func seamDB(t *testing.T) (*storage.DB, *eventlog.Log, *settings.Registry) {
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
	return db, eventlog.New(db, reg), reg
}

// TestCitedEntryResolverVanishesOnSupersession (F11): the shell cited-entry
// resolver returns an ACTIVE entry's version, and OMITS an entry that was
// superseded or removed — its key vanishes, so the intake fingerprint's
// mapDrift fires vanished-key drift. A row's Version is immutable (supersession
// mints a NEW entry id and retires the old), so drift manifests as the cited
// id no longer resolving active — not a fake version-bump lane.
func TestCitedEntryResolverVanishesOnSupersession(t *testing.T) {
	db, log, reg := seamDB(t)
	ctx := context.Background()
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, role, created_ts) VALUES ('alice','member',?)`,
			time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("user: %v", err)
	}
	memStore, err := memory.NewStore(db, log, reg, filepath.Join(t.TempDir(), "knowledge"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	gate := memory.NewGate(memStore)
	resolve := memoryCitedEntryVersions(memStore)

	e, err := gate.CreateManual(ctx, "alice", memory.Draft{
		Scope: memory.ScopeProject, ScopeRef: "proj-x", Kind: memory.KindConvention,
		Title: "truth", Content: "v1", TopicKey: "t/x",
	})
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	// Active → resolves to its version.
	got, err := resolve(ctx, []string{e.ID})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got[e.ID] != "1" {
		t.Fatalf("active entry resolved to %q, want 1", got[e.ID])
	}

	// Supersession retires the cited id → it vanishes from the resolution.
	if _, err := gate.NewVersion(ctx, "alice", e.ID, memory.Draft{
		Scope: memory.ScopeProject, ScopeRef: "proj-x", Kind: memory.KindConvention,
		Title: "truth", Content: "v2", TopicKey: "t/x",
	}); err != nil {
		t.Fatalf("NewVersion: %v", err)
	}
	got, err = resolve(ctx, []string{e.ID})
	if err != nil {
		t.Fatalf("resolve after supersede: %v", err)
	}
	if _, present := got[e.ID]; present {
		t.Fatalf("superseded cited id still resolves (%v) — must vanish for drift", got)
	}

	// A genuinely absent id also vanishes (removed/never existed).
	got, _ = resolve(ctx, []string{"know-nonexistent"})
	if _, present := got["know-nonexistent"]; present {
		t.Fatal("an absent id resolved — must vanish")
	}
}

// TestFingerprintSuppliesRealRepoHead (F13/rubric 22): the projectSeams
// fingerprint probe supplies the matched project's REAL repo HEAD, and it
// tracks the HEAD as it moves; an empty project supplies no head (never faked).
func TestFingerprintSuppliesRealRepoHead(t *testing.T) {
	db, log, _ := seamDB(t)
	ctx := context.Background()
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	ps := &projectSeams{proj: proj, runs: run.NewStore(db, log), db: db}

	// A repo-less project onboarded (git init) + activated.
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop"}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	fp, err := ps.Fingerprint(ctx, "shop")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	head0, _ := proj.RepoHead(ctx, "shop")
	if fp.RepoHead == "" || fp.RepoHead != head0 {
		t.Fatalf("fingerprint repo HEAD = %q, want the real HEAD %q", fp.RepoHead, head0)
	}
	// Unobservable members stay empty (never faked).
	if fp.SpecPlanVersion != "" || fp.PriceTableVersion != "" || len(fp.SourceHashes) != 0 {
		t.Fatalf("fingerprint fabricated members: %+v", fp)
	}

	// No project → no repo HEAD.
	if empty, _ := ps.Fingerprint(ctx, ""); empty.RepoHead != "" {
		t.Fatalf("empty-project fingerprint supplied a head %q (never faked)", empty.RepoHead)
	}

	// The HEAD MOVES → the probe reports the new HEAD (staleness would fire).
	e, _ := proj.Get(ctx, "shop")
	ws, err := proj.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	// Advance the default branch by committing on it directly (a sibling accept).
	writeAndCommit(t, e.StorePath, "moved.txt", "moved\n")
	_ = ws
	fp2, _ := ps.Fingerprint(ctx, "shop")
	head1, _ := proj.RepoHead(ctx, "shop")
	if fp2.RepoHead != head1 || fp2.RepoHead == head0 {
		t.Fatalf("fingerprint did not track the moved HEAD: %q (was %q, now %q)", fp2.RepoHead, head0, head1)
	}
}

func writeAndCommit(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	gitDo(t, dir, "add", "-A")
	gitDo(t, dir, "commit", "-q", "-m", "advance")
}

func gitDo(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent",
		"GIT_AUTHOR_NAME=fx", "GIT_AUTHOR_EMAIL=fx@t.invalid", "GIT_COMMITTER_NAME=fx", "GIT_COMMITTER_EMAIL=fx@t.invalid")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
