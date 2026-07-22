package review_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

// The S13.5 wiring seams landing at B4-2: the snapshot-sha fill-once slot and
// the base-content seam feeding Compare's old-side 0.

func TestSnapshotSHAFillOnce(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.seed()

	// A content-pin lane (composer definitions, non-repo tasks) mints with
	// snapshot_sha NULL — a valid honest state (Spec S13.5, R19).
	rev := f.mint(1, "hello")
	if rev.SnapshotSHA != "" {
		t.Fatalf("content-pin revision minted with snapshot_sha = %q, want NULL", rev.SnapshotSHA)
	}
	got, _ := f.store.RevisionAt(ctx, "dlv-t1", 1)
	if got.SnapshotSHA != "" {
		t.Fatalf("stored snapshot_sha = %q, want NULL", got.SnapshotSHA)
	}

	// Fill NULL → sha.
	if err := f.store.SetSnapshotSHA(ctx, "dlv-t1", 1, "aaaa1111"); err != nil {
		t.Fatalf("SetSnapshotSHA: %v", err)
	}
	got, _ = f.store.RevisionAt(ctx, "dlv-t1", 1)
	if got.SnapshotSHA != "aaaa1111" {
		t.Fatalf("snapshot_sha after fill = %q, want aaaa1111", got.SnapshotSHA)
	}
	// Idempotent re-fill with the SAME sha.
	if err := f.store.SetSnapshotSHA(ctx, "dlv-t1", 1, "aaaa1111"); err != nil {
		t.Fatalf("idempotent re-fill: %v", err)
	}
	// A second, DIFFERENT fill aborts (fill-once, the migration trigger).
	if err := f.store.SetSnapshotSHA(ctx, "dlv-t1", 1, "bbbb2222"); err == nil {
		t.Fatal("second different snapshot_sha fill succeeded, want fill-once abort")
	}
}

func TestMintSnapshotAtInsert(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.seed()
	rev, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t1", N: 1, RunID: "r1",
		Files:       map[string]string{"deliverable.md": "body"},
		SnapshotSHA: "sha-at-insert",
	})
	if err != nil {
		t.Fatalf("MintRevision: %v", err)
	}
	if rev.SnapshotSHA != "sha-at-insert" {
		t.Fatalf("minted snapshot_sha = %q, want sha-at-insert", rev.SnapshotSHA)
	}
	got, _ := f.store.RevisionAt(ctx, "dlv-t1", 1)
	if got.SnapshotSHA != "sha-at-insert" {
		t.Fatalf("stored snapshot_sha = %q, want sha-at-insert", got.SnapshotSHA)
	}
}

// fakeBase is a test BaseContentSource (the topology package's role at the
// composition root).
type fakeBase struct {
	files map[string]string
	ok    bool
}

func (b fakeBase) BaseContent(ctx context.Context, deliverableID string) (map[string]string, bool, error) {
	return b.files, b.ok, nil
}

func TestCompareBaseZeroRepoBacked(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.task("t1", "u1")
	f.run("r1", "t1")
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "code"}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t1", N: 1, RunID: "r1",
		Files: map[string]string{"main.go": "package main\nfunc main() {}\n"},
	}); err != nil {
		t.Fatalf("MintRevision: %v", err)
	}

	// Repo-backed: Compare(0,1) diffs the branch base → rev 1 (Spec S13.1).
	f.store.BaseContent = fakeBase{files: map[string]string{"main.go": "package main\n"}, ok: true}
	cmp, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Compare(0,1): %v", err)
	}
	if !strings.Contains(cmp.Unified, "+func main") {
		t.Fatalf("Compare(0,1) did not diff base→rev1 (added line missing):\n%s", cmp.Unified)
	}
	if strings.Contains(cmp.Unified, "+package main") {
		t.Fatalf("base line shown as added — the base was not consulted:\n%s", cmp.Unified)
	}

	// Non-repo (ok=false): the empty base — rev1 is all-added (today's behavior).
	f.store.BaseContent = fakeBase{ok: false}
	cmp2, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Compare non-repo: %v", err)
	}
	if !strings.Contains(cmp2.Unified, "+package main") {
		t.Fatalf("non-repo base should show rev1 all-added:\n%s", cmp2.Unified)
	}
}

func TestRevisionOneOldSideAnchorMapsViaBase(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.withDrift(0) // drift 0 → without the base a moved line degrades, not searches
	f.task("t1", "u1")
	f.run("r1", "t1")
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "code"}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	// rev1 == base content: a comment born on rev1's old side anchors on it.
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t1", N: 1, RunID: "r1",
		Files: map[string]string{"f.go": "L1\nL2\nL3\nL4\nL5\nTARGET\n"},
	}); err != nil {
		t.Fatalf("mint rev1: %v", err)
	}
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t1", N: 2, RunID: "r1",
		Files: map[string]string{"f.go": "HEADER\nL1\nL2\nL3\nL4\nL5\nTARGET\n"},
	}); err != nil {
		t.Fatalf("mint rev2: %v", err)
	}

	// A comment born on rev1, side=old, at line 2 ("L2" in the base).
	if _, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", RevisionN: 1, Author: "u1", Body: "on the base",
		Anchor: &review.AnchorRecord{FilePath: "f.go", Side: review.SideOld, LineNo: 6, LineText: "TARGET"},
	}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	// WITH the base wired, porting the rev-1 old-side comment onto rev2 maps
	// (ladder step 1) — the base is a NON-NIL source; L2 (base line 2) maps to
	// rev2 line 3.
	f.store.BaseContent = fakeBase{files: map[string]string{"f.go": "L1\nL2\nL3\nL4\nL5\nTARGET\n"}, ok: true}
	_, placements, err := f.store.PlacedComments(ctx, "dlv-t1", 2)
	if err != nil {
		t.Fatalf("PlacedComments with base: %v", err)
	}
	if len(placements) != 1 {
		t.Fatalf("placements = %d, want 1", len(placements))
	}
	if placements[0].Status != review.AnchorMapped {
		t.Fatalf("with base, status = %q, want mapped (ladder step 1)", placements[0].Status)
	}
	if placements[0].Anchor.LineNo != 7 {
		t.Fatalf("mapped line = %d, want 7 (TARGET shifted by the inserted HEADER)", placements[0].Anchor.LineNo)
	}
}

func TestRevisionOneOldSideDegradesWithoutBase(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.withDrift(0)
	f.task("t1", "u1")
	f.run("r1", "t1")
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "code"}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t1", N: 1, RunID: "r1",
		Files: map[string]string{"f.go": "L1\nL2\nL3\nL4\nL5\nTARGET\n"},
	}); err != nil {
		t.Fatalf("mint rev1: %v", err)
	}
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t1", N: 2, RunID: "r1",
		Files: map[string]string{"f.go": "HEADER\nL1\nL2\nL3\nL4\nL5\nTARGET\n"},
	}); err != nil {
		t.Fatalf("mint rev2: %v", err)
	}
	if _, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", RevisionN: 1, Author: "u1", Body: "on the base",
		Anchor: &review.AnchorRecord{FilePath: "f.go", Side: review.SideOld, LineNo: 6, LineText: "TARGET"},
	}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	// No base seam: the old-side-of-rev-1 source is nil → it degrades honestly
	// (no diff map; drift 0 so the moved line is not found by search) instead
	// of a mis-map. It is NOT the mapped status the base would have produced.
	_, placements, err := f.store.PlacedComments(ctx, "dlv-t1", 2)
	if err != nil {
		t.Fatalf("PlacedComments without base: %v", err)
	}
	if placements[0].Status == review.AnchorMapped {
		t.Fatal("without the base, an old-side-of-rev-1 anchor must not map (nil source)")
	}
}
