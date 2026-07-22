package review_test

import (
	"context"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

const rev1Body = "alpha\nbravo\ncharlie\ndelta\necho\n"

func anchor(line int, text string) *review.AnchorRecord {
	return &review.AnchorRecord{FilePath: "deliverable.md", Side: review.SideNew, LineNo: line, LineText: text}
}

// Server-side validation at birth: supplied positions are never trusted as
// placement authority (FC-v1 §2).
func TestAddCommentServerSideValidation(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, rev1Body)
	ctx := context.Background()

	// Honest anchor → exact.
	c1, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "rename this", Anchor: anchor(2, "bravo"),
	})
	if err != nil || c1.BornStatus != review.AnchorExact {
		t.Fatalf("exact birth: %+v err %v", c1, err)
	}

	// Lied position with the quote nearby (⚙ drift 2 default) → drifted,
	// and the VALIDATED position is what the placement records.
	c2, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "tighten", Anchor: anchor(4, "charlie"),
	})
	if err != nil || c2.BornStatus != review.AnchorDrifted {
		t.Fatalf("drifted birth: %+v err %v", c2, err)
	}
	_, placements, err := f.store.PlacedComments(ctx, "dlv-t1", 1)
	if err != nil {
		t.Fatalf("PlacedComments: %v", err)
	}
	byID := map[int64]review.Placement{}
	for _, p := range placements {
		byID[p.CommentID] = p
	}
	if p := byID[c2.ID]; p.Anchor.LineNo != 3 || p.Status != review.AnchorDrifted {
		t.Fatalf("drifted placement corrects the position: %+v", p)
	}

	// Quote nowhere → file-level, quote kept on the row (never dropped).
	c3, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "where is this", Anchor: anchor(2, "no such line"),
	})
	if err != nil || c3.BornStatus != review.AnchorFile {
		t.Fatalf("file degrade birth: %+v err %v", c3, err)
	}
	if c3.Anchor.LineText != "no such line" {
		t.Fatalf("file degrade keeps the claimed quote: %+v", c3.Anchor)
	}

	// Drift 0 tightens the ladder: the same lied position now degrades.
	f.withDrift(0)
	c4, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "strict", Anchor: anchor(4, "charlie"),
	})
	if err != nil || c4.BornStatus != review.AnchorFile {
		t.Fatalf("drift 0 birth: %+v err %v", c4, err)
	}
}

func TestFileAndDeliverableLevelComments(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, rev1Body)
	ctx := context.Background()

	cf, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "whole file needs a header",
		FileLevel: "deliverable.md",
	})
	if err != nil || cf.BornStatus != review.AnchorFile || cf.Anchor.FilePath != "deliverable.md" {
		t.Fatalf("file-level comment: %+v err %v", cf, err)
	}

	cd, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "cross-cutting: tone is off",
	})
	if err != nil || cd.BornStatus != review.AnchorFile || cd.Anchor.FilePath != "" {
		t.Fatalf("deliverable-level comment: %+v err %v", cd, err)
	}
}

func TestAddFindingsAnchorParsing(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, rev1Body)
	ctx := context.Background()

	ids, err := f.store.AddFindings(ctx, "dlv-t1", 1, []review.FindingInput{
		{Author: "u1", RunID: "r1", Severity: review.SeverityBlocker,
			Category: "AC-BLOCKER", Criterion: "AC-1",
			Body: "bravo violates AC-1", RawAnchor: "deliverable.md:2"},
		{Author: "u1", RunID: "r1", Severity: review.SeverityNote,
			Body: "style nit", RawAnchor: "unknown:AC-2"},
	})
	if err != nil || len(ids) != 2 {
		t.Fatalf("AddFindings: ids %v err %v", ids, err)
	}
	c1, err := f.store.CommentByID(ctx, ids[0])
	if err != nil || c1.BornStatus != review.AnchorExact || c1.Anchor.LineText != "bravo" {
		t.Fatalf("positional finding: %+v err %v", c1, err)
	}
	c2, err := f.store.CommentByID(ctx, ids[1])
	if err != nil || c2.BornStatus != review.AnchorFile || c2.OriginAnchor != "unknown:AC-2" {
		t.Fatalf("opaque finding keeps the origin anchor: %+v err %v", c2, err)
	}
	// One review.comment event covers the recording act, run-scoped.
	evs := f.events(review.EventComment)
	if len(evs) != 1 || evs[0].RunID != "r1" {
		t.Fatalf("comment events: %+v", evs)
	}
}

// The port ladder across revisions, with placements recorded per comment
// per revision (Spec S13.3).
func TestPortLadderAcrossRevisions(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, rev1Body)
	ctx := context.Background()

	exact, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "on charlie", Anchor: anchor(3, "charlie"),
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	moved, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "on echo", Anchor: anchor(5, "echo"),
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	gone, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "on delta", Anchor: anchor(4, "delta"),
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}

	// Revision 2: two lines inserted at the top (everything shifts), delta
	// deleted, charlie kept, echo kept.
	f.mint(2, "intro\nintro two\nalpha\nbravo\ncharlie\nfoxtrot\necho\n")

	_, placements, err := f.store.PlacedComments(ctx, "dlv-t1", 2)
	if err != nil {
		t.Fatalf("PlacedComments rev2: %v", err)
	}
	byID := map[int64]review.Placement{}
	for _, p := range placements {
		byID[p.CommentID] = p
	}
	tracked := func(s review.AnchorStatus) bool {
		return s == review.AnchorExact || s == review.AnchorMapped || s == review.AnchorDrifted
	}
	// The exact rung labels depend on git's hunk shaping (nearby edits
	// merge hunks); rung-precise cases are the anchor unit tests. Here the
	// LADDER OUTCOME is asserted: moved lines stay tracked at the position
	// holding their quote, deleted lines degrade to file-level.
	if p := byID[exact.ID]; !tracked(p.Status) || p.Anchor.LineNo != 5 || p.Anchor.LineText != "charlie" {
		t.Fatalf("shifted line tracked to its quote: %+v", p)
	}
	if p := byID[moved.ID]; !tracked(p.Status) || p.Anchor.LineNo != 7 || p.Anchor.LineText != "echo" {
		t.Fatalf("moved line tracked: %+v", p)
	}
	if p := byID[gone.ID]; p.Status != review.AnchorFile {
		t.Fatalf("deleted line degrades to file-level: %+v", p)
	}

	// A revision that drops the anchored FILE forces the ladder's last
	// rung: a separate deliverable whose rev2 renames the file — every
	// a.md anchor becomes an explicit ORPHAN, never dropped.
	f.task("t3", "u1")
	f.run("r3", "t3")
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-t3", Owner: "u1", TaskID: "t3", Type: "markdown",
	}); err != nil {
		t.Fatalf("ensure t3: %v", err)
	}
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t3", N: 1, RunID: "r3", AttemptRef: "r3#round-1",
		Files: map[string]string{"a.md": "alpha\n"},
	}); err != nil {
		t.Fatalf("mint t3/1: %v", err)
	}
	ca, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t3", Author: "u1", Body: "on a.md",
		Anchor: &review.AnchorRecord{FilePath: "a.md", Side: review.SideNew, LineNo: 1, LineText: "alpha"},
	})
	if err != nil {
		t.Fatalf("comment t3: %v", err)
	}
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t3", N: 2, RunID: "r3", AttemptRef: "r3#round-2",
		Files: map[string]string{"b.md": "beta\n"},
	}); err != nil {
		t.Fatalf("mint t3/2: %v", err)
	}
	_, p3, err := f.store.PlacedComments(ctx, "dlv-t3", 2)
	if err != nil {
		t.Fatalf("PlacedComments t3 rev2: %v", err)
	}
	if len(p3) != 1 || p3[0].Status != review.AnchorOrphan {
		t.Fatalf("file gone → ORPHAN explicit state: %+v", p3)
	}
	cRow, err := f.store.CommentByID(ctx, ca.ID)
	if err != nil || cRow.Anchor.LineText != "alpha" || cRow.RevisionN != 1 {
		t.Fatalf("orphan keeps quote + original-revision link: %+v err %v", cRow, err)
	}

	// Placements are recorded per (comment, revision) — the anchors table
	// has a row for every rendered pair, and a re-render agrees.
	var count int
	if err := f.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM review_comment_anchors a
		  JOIN review_comments c ON c.comment_id = a.comment_id
		 WHERE c.deliverable_id = 'dlv-t1'`).Scan(&count); err != nil {
		t.Fatalf("count anchors: %v", err)
	}
	// 3 comments × (birth rev1 + port rev2).
	if count != 6 {
		t.Fatalf("anchor state rows per comment per revision: %d", count)
	}
	if _, _, err := f.store.PlacedComments(ctx, "dlv-t1", 2); err != nil {
		t.Fatalf("re-render re-validation: %v", err)
	}
}

// The no-comment-without-render-location invariant is total at the data
// layer: every comment yields a placement whose status maps to a render
// location (inline, file strip, or the always-visible synthetic strip).
func TestNoCommentWithoutRenderLocation(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, rev1Body)
	ctx := context.Background()

	inputs := []review.CommentInput{
		{DeliverableID: "dlv-t1", Author: "u1", Body: "anchored", Anchor: anchor(1, "alpha")},
		{DeliverableID: "dlv-t1", Author: "u1", Body: "lied", Anchor: anchor(5, "alpha")},
		{DeliverableID: "dlv-t1", Author: "u1", Body: "nowhere", Anchor: anchor(2, "zzz")},
		{DeliverableID: "dlv-t1", Author: "u1", Body: "file", FileLevel: "deliverable.md"},
		{DeliverableID: "dlv-t1", Author: "u1", Body: "cross-cutting"},
	}
	for _, in := range inputs {
		if _, err := f.store.AddComment(ctx, in); err != nil {
			t.Fatalf("AddComment %q: %v", in.Body, err)
		}
	}
	f.mint(2, "totally different\n")
	comments, placements, err := f.store.PlacedComments(ctx, "dlv-t1", 2)
	if err != nil {
		t.Fatalf("PlacedComments: %v", err)
	}
	if len(comments) != len(placements) {
		t.Fatalf("every comment places: %d comments, %d placements", len(comments), len(placements))
	}
	valid := map[review.AnchorStatus]bool{
		review.AnchorExact: true, review.AnchorMapped: true, review.AnchorDrifted: true,
		review.AnchorFile: true, review.AnchorOrphan: true,
	}
	for _, p := range placements {
		if !valid[p.Status] {
			t.Fatalf("placement without a render location: %+v", p)
		}
	}
}
