package stage

// sit1_mint_internal_test.go — P3-SIT-1 R1, committed RED by grounding
// (Amendment-A carve-out, CONVENTIONS §3).
//
// The finding: t-3120e8e3d14591d3 produced a 23-file React/Vite app in its
// project worktree, and the verification handoff minted it as a `markdown`
// deliverable with ONE content object (the S-8 step report) — because
// verifyInput names the report's V0 shape type ("markdown", skeleton.go) and
// the review sink copies that word onto the deliverable row BEFORE it learns
// the run is repo-backed (review_sink.go: ensure runs first, the snapshot
// seam second). Spec S13.1: for repo-backed work the deliverable IS the tree
// at the snapshot pin; the report is a companion object.
//
// These tests drive the real reviewSink over a real review store with the
// snapshot seam faked (the composition root's role), and assert the minted
// dtype. The V0 shape type on verify.Deliverable is NOT what moves: the
// report is still markdown for the shape check.

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

const sit1Snapshot = "0123456789abcdef0123456789abcdef01234567"

type sit1MintFix struct {
	ctx context.Context
	sk  *Skeleton
	rev *review.Store
}

// newSIT1MintFix composes a Skeleton with exactly what MintCandidate reads: the
// run store (the minting run's owner), the review store, and the snapshot seam
// — "" for a workspace-less run (the content-pin lane), a sha for a
// repo-backed one.
func newSIT1MintFix(t *testing.T, snapshot string) *sit1MintFix {
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
	runs := run.NewStore(db, log)
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
			"t-sit1", "alice", "webshop", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := runs.Create(ctx, run.NewRun{
		ID: "t-sit1" + RunSuffixVerify, UserID: "alice", TaskID: "t-sit1", Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("create verify run: %v", err)
	}
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(t.TempDir(), "review")}
	cfg := Config{DB: db, Log: log, Settings: reg, Runs: runs, Review: rev}
	if snapshot != "" {
		cfg.Snapshot = func(context.Context, string) (string, error) { return snapshot, nil }
	}
	return &sit1MintFix{ctx: ctx, sk: &Skeleton{cfg: cfg}, rev: rev}
}

func (f *sit1MintFix) mint(t *testing.T, domain string) {
	t.Helper()
	d := verify.Deliverable{
		TaskID: "t-sit1", RunID: "t-sit1" + RunSuffixVerify, Domain: domain,
		Type: "markdown", Revision: 1, Content: "# S-8 step report\n\nBuilt the shop.\n",
	}
	if err := (reviewSink{s: f.sk}).MintCandidate(f.ctx, d, 1); err != nil {
		t.Fatalf("MintCandidate: %v", err)
	}
}

// TestSIT1MintRepoBackedSoftwareWorkAsCode — R1. A software task whose run is
// repo-backed (the snapshot seam answers a sha) mints THE task deliverable
// with dtype "code": the deliverable is the tree at the pin (Spec S13.1), and
// "markdown" would be a false statement about a webshop. The report stays on
// the revision as its one companion content object, and the snapshot pin
// still fills at insert.
//
// RED on the grounding tree: the minted dtype is "markdown".
func TestSIT1MintRepoBackedSoftwareWorkAsCode(t *testing.T) {
	f := newSIT1MintFix(t, sit1Snapshot)
	f.mint(t, verify.DomainSoftware)

	d, err := f.rev.Deliverable(f.ctx, TaskDeliverableID("t-sit1"))
	if err != nil {
		t.Fatalf("Deliverable: %v", err)
	}
	if d.Type != "code" {
		t.Fatalf("repo-backed software work minted with dtype %q, want \"code\" — the deliverable is the tree at the snapshot pin (Spec S13.1), not the step report", d.Type)
	}
	r, err := f.rev.RevisionAt(f.ctx, d.ID, 1)
	if err != nil {
		t.Fatalf("RevisionAt: %v", err)
	}
	if r.SnapshotSHA != sit1Snapshot {
		t.Fatalf("revision 1 snapshot_sha = %q, want the seam's %q", r.SnapshotSHA, sit1Snapshot)
	}
	if len(r.Objects) != 1 || r.Objects[0].Name != DeliverableFileName || r.PinKind != "content" {
		t.Fatalf("the step report must stay the revision's one companion content object %q, got pin_kind %q objects %+v",
			DeliverableFileName, r.PinKind, r.Objects)
	}
}

// TestSIT1MintContentLaneKeepsTheReportType — GREEN GUARD (the non-tautological
// control): a workspace-less software run has nothing but the report, and its
// deliverable stays what the report is.
func TestSIT1MintContentLaneKeepsTheReportType(t *testing.T) {
	f := newSIT1MintFix(t, "")
	f.mint(t, verify.DomainSoftware)
	d, err := f.rev.Deliverable(f.ctx, TaskDeliverableID("t-sit1"))
	if err != nil {
		t.Fatalf("Deliverable: %v", err)
	}
	if d.Type != "markdown" {
		t.Fatalf("content-lane software work minted with dtype %q, want \"markdown\" (no snapshot pin, no tree)", d.Type)
	}
	if r, _ := f.rev.RevisionAt(f.ctx, d.ID, 1); r.SnapshotSHA != "" {
		t.Fatalf("content-lane revision carries snapshot_sha %q, want NULL", r.SnapshotSHA)
	}
}

// TestSIT1MintNonSoftwareDomainKeepsItsType — GREEN GUARD: a repo-backed run in
// another domain (a research report written into a project) pins a snapshot
// too, but its deliverable type is what its content says; only software work's
// deliverable is a code tree (Spec S07.8 launch roster; S13.2 type table).
func TestSIT1MintNonSoftwareDomainKeepsItsType(t *testing.T) {
	f := newSIT1MintFix(t, sit1Snapshot)
	f.mint(t, verify.DomainWebResearch)
	d, err := f.rev.Deliverable(f.ctx, TaskDeliverableID("t-sit1"))
	if err != nil {
		t.Fatalf("Deliverable: %v", err)
	}
	if d.Type != "markdown" {
		t.Fatalf("repo-backed %s work minted with dtype %q, want \"markdown\"", verify.DomainWebResearch, d.Type)
	}
}
