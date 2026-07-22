package review_test

import (
	"context"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

// The 0007 walls proven by raw-SQL attempted violations (the 0004/0005
// battery precedent): immutability and append-only are TRIGGERS, binding
// even for raw-SQL holders.
func TestSchemaWalls(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, "alpha\nbravo\n")
	f.mint(2, "alpha\nbravo\ncharlie\n")
	ctx := context.Background()
	ids, err := f.store.AddFindings(ctx, "dlv-t1", 2, []review.FindingInput{
		{Author: "u1", RunID: "r1", Severity: review.SeverityBlocker, Criterion: "AC-1",
			Body: "wall probe", RawAnchor: "deliverable.md:1"},
	})
	if err != nil || len(ids) != 1 {
		t.Fatalf("seed finding: %v %v", ids, err)
	}
	id := ids[0]

	// deliverables: never deleted; identity immutable; revision pointer
	// never rewinds (lineage never compressed).
	f.mustAbort(`DELETE FROM deliverables WHERE deliverable_id = 'dlv-t1'`)
	f.mustAbort(`UPDATE deliverables SET task_id = 'other' WHERE deliverable_id = 'dlv-t1'`)
	f.mustAbort(`UPDATE deliverables SET dtype = 'code' WHERE deliverable_id = 'dlv-t1'`)
	f.mustAbort(`UPDATE deliverables SET current_revision = 1 WHERE deliverable_id = 'dlv-t1'`)
	// The state MAY move (accept mechanics are S13.6's; the column moves).
	f.exec(`UPDATE deliverables SET state = 'accepted' WHERE deliverable_id = 'dlv-t1'`)

	// deliverable_revisions: immutable once minted; never deleted; the two
	// fill-once slots fill once.
	f.mustAbort(`DELETE FROM deliverable_revisions WHERE deliverable_id = 'dlv-t1' AND n = 1`)
	f.mustAbort(`UPDATE deliverable_revisions SET content_sha256 = 'beef' WHERE deliverable_id = 'dlv-t1' AND n = 1`)
	f.mustAbort(`UPDATE deliverable_revisions SET attempt_ref = 'rewritten' WHERE deliverable_id = 'dlv-t1' AND n = 1`)
	f.exec(`UPDATE deliverable_revisions SET snapshot_sha = 'abc123' WHERE deliverable_id = 'dlv-t1' AND n = 1`)
	f.mustAbort(`UPDATE deliverable_revisions SET snapshot_sha = 'def456' WHERE deliverable_id = 'dlv-t1' AND n = 1`)
	f.exec(`UPDATE deliverable_revisions SET verdict_ref = 7 WHERE deliverable_id = 'dlv-t1' AND n = 1`)
	f.mustAbort(`UPDATE deliverable_revisions SET verdict_ref = 8 WHERE deliverable_id = 'dlv-t1' AND n = 1`)

	// review_comments: never dropped; content immutable; consumption
	// stamps move only as the complete one-way batch stamp.
	f.mustAbort(`DELETE FROM review_comments WHERE comment_id = ?`, id)
	f.mustAbort(`UPDATE review_comments SET body = 'rewritten' WHERE comment_id = ?`, id)
	f.mustAbort(`UPDATE review_comments SET line_text = 'moved' WHERE comment_id = ?`, id)
	f.mustAbort(`UPDATE review_comments SET severity = 'note' WHERE comment_id = ?`, id)
	// A partial consumption stamp violates the lifecycle CHECKs.
	f.mustAbort(`UPDATE review_comments SET status = 'consumed' WHERE comment_id = ?`, id)
	f.mustAbort(`UPDATE review_comments SET finding_number = 1 WHERE comment_id = ?`, id)
	// The complete stamp lands…
	f.exec(`UPDATE review_comments SET status = 'consumed', finding_number = 1,
	        consumed_ts = '2026-07-22T00:00:00Z', consumed_by = 'r1#revise-r2' WHERE comment_id = ?`, id)
	// …and is final: no re-open, no re-number (a re-review that deems it
	// unfixed produces a FRESH finding, Spec S13.3).
	f.mustAbort(`UPDATE review_comments SET status = 'open', finding_number = NULL,
	        consumed_ts = NULL, consumed_by = '' WHERE comment_id = ?`, id)
	f.mustAbort(`UPDATE review_comments SET finding_number = 9, consumed_ts = '2026-07-23T00:00:00Z',
	        consumed_by = 'other' WHERE comment_id = ?`, id)

	// A comment cannot be born with an impossible anchor shape: a
	// positional status without an anchor, or a position without its
	// quote.
	f.mustAbort(`INSERT INTO review_comments (user_id, deliverable_id, revision_n, kind, severity, body,
	        anchor_status, status, created_ts)
	        VALUES ('u1', 'dlv-t1', 1, 'human', 'note', 'x', 'exact', 'open', '2026-07-22T00:00:00Z')`)
	f.mustAbort(`INSERT INTO review_comments (user_id, deliverable_id, revision_n, kind, severity, body,
	        file_path, side, line_no, anchor_status, status, created_ts)
	        VALUES ('u1', 'dlv-t1', 1, 'human', 'note', 'x', 'f.md', 'new', 2, 'exact', 'open', '2026-07-22T00:00:00Z')`)

	// review_comment_anchors: recorded placements never change and never
	// vanish.
	f.mustAbort(`UPDATE review_comment_anchors SET line_no = 99 WHERE comment_id = ?`, id)
	f.mustAbort(`DELETE FROM review_comment_anchors WHERE comment_id = ?`, id)
	// An orphan placement carries no live location (CHECK).
	f.mustAbort(`INSERT INTO review_comment_anchors (comment_id, revision_n, status, file_path, computed_ts)
	        VALUES (?, 2, 'orphan', 'f.md', '2026-07-22T00:00:00Z')`, id)
}
