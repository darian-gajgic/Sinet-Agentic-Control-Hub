package review_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

// THE S13.4 drain: every open comment collected (ported), numbered
// [F1..Fn] stable, batch-consumed with ONE shared consumed_at + attempt
// ref, orphans and notes delivered.
func TestDrainNumbersConsumesAndDelivers(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, rev1Body)
	ctx := context.Background()

	// A human note made against revision 1 (it will be PORTED to rev 2 at
	// drain time), plus a round's findings on revision 2.
	if _, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", Author: "u1", Body: "prefer shorter names",
		Severity: review.SeverityNote, Anchor: anchor(2, "bravo"),
	}); err != nil {
		t.Fatalf("human note: %v", err)
	}
	f.mint(2, "alpha\nbravo\ncharlie\ndelta\necho\nfoxtrot\n")
	if _, err := f.store.AddFindings(ctx, "dlv-t1", 2, []review.FindingInput{
		{Author: "u1", RunID: "r1", Severity: review.SeverityBlocker, Category: "AC-BLOCKER",
			Criterion: "AC-1", Body: "foxtrot violates AC-1", RawAnchor: "deliverable.md:6",
			Suggested: "drop foxtrot"},
		{Author: "u1", RunID: "r1", Severity: review.SeverityNote,
			Body: "consider a summary line", RawAnchor: "unknown:style"},
	}); err != nil {
		t.Fatalf("findings: %v", err)
	}

	batch, err := f.store.Drain(ctx, review.DrainRequest{
		DeliverableID: "dlv-t1", AttemptRef: "r1#revise-r2", RunID: "r1",
	})
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(batch) != 3 {
		t.Fatalf("every open comment drains (notes travel, P-T12-2): %d", len(batch))
	}
	// Numbering: [F1..Fn] contiguous, blockers first, then insertion
	// order.
	if batch[0].Comment.Severity != review.SeverityBlocker || batch[0].Number != 1 {
		t.Fatalf("blockers first: %+v", batch[0])
	}
	for i, b := range batch {
		if b.Number != i+1 {
			t.Fatalf("contiguous numbering: %+v", batch)
		}
	}
	// The batch stamp: ONE consumed_at + attempt ref across every row.
	stamp := batch[0].Comment.ConsumedTS
	for _, b := range batch {
		if b.Comment.ConsumedTS != stamp || b.Comment.ConsumedBy != "r1#revise-r2" ||
			b.Comment.Status != review.StatusConsumed || b.Comment.FindingNumber != b.Number {
			t.Fatalf("batch stamp: %+v", b.Comment)
		}
	}
	// The suggested change travels with the numbered point (Spec S13.1).
	if batch[0].Comment.Suggested != "drop foxtrot" {
		t.Fatalf("suggested change delivered: %+v", batch[0].Comment)
	}
	// The ported human note delivered with its rev-2 placement.
	var foundNote bool
	for _, b := range batch {
		if b.Comment.Body == "prefer shorter names" {
			foundNote = true
			if b.Placement.RevisionN != 2 || b.Placement.Anchor.LineText != "bravo" {
				t.Fatalf("ported note placement: %+v", b.Placement)
			}
		}
	}
	if !foundNote {
		t.Fatalf("the rev-1 note must drain on rev 2")
	}

	// The drained event: what that rework received (auditable), run-scoped.
	evs := f.events(review.EventDrained)
	if len(evs) != 1 || evs[0].RunID != "r1" {
		t.Fatalf("drained events: %+v", evs)
	}
	var payload struct {
		AttemptRef string `json:"attempt_ref"`
		ConsumedAt string `json:"consumed_at"`
		Points     []struct {
			N        int    `json:"n"`
			Severity string `json:"severity"`
		} `json:"points"`
	}
	if err := json.Unmarshal(evs[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.AttemptRef != "r1#revise-r2" || payload.ConsumedAt != stamp || len(payload.Points) != 3 {
		t.Fatalf("drained payload: %+v", payload)
	}

	// A second drain finds nothing open: consumed is final; the next round
	// mints FRESH findings (Spec S13.3).
	again, err := f.store.Drain(ctx, review.DrainRequest{
		DeliverableID: "dlv-t1", AttemptRef: "r1#revise-r3", RunID: "r1",
	})
	if err != nil || len(again) != 0 {
		t.Fatalf("second drain: %v %v", again, err)
	}
}

// Orphaned findings still drain — delivery is never conditional on
// anchoring success (P-T12-2).
func TestDrainDeliversOrphans(t *testing.T) {
	f := newFix(t)
	f.task("t4", "u1")
	f.run("r4", "t4")
	ctx := context.Background()
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-t4", Owner: "u1", TaskID: "t4", Type: "markdown",
	}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t4", N: 1, RunID: "r4", AttemptRef: "r4#round-1",
		Files: map[string]string{"a.md": "alpha\n"},
	}); err != nil {
		t.Fatalf("mint 1: %v", err)
	}
	if _, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t4", Author: "u1", Body: "fix alpha", Severity: review.SeverityBlocker,
		Anchor: &review.AnchorRecord{FilePath: "a.md", Side: review.SideNew, LineNo: 1, LineText: "alpha"},
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	// Revision 2 renames the file: the comment orphans.
	if _, err := f.store.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t4", N: 2, RunID: "r4", AttemptRef: "r4#round-2",
		Files: map[string]string{"b.md": "beta\n"},
	}); err != nil {
		t.Fatalf("mint 2: %v", err)
	}
	batch, err := f.store.Drain(ctx, review.DrainRequest{
		DeliverableID: "dlv-t4", AttemptRef: "r4#revise-r2", RunID: "r4",
	})
	if err != nil || len(batch) != 1 {
		t.Fatalf("orphan drains: %v err %v", batch, err)
	}
	if batch[0].Placement.Status != review.AnchorOrphan {
		t.Fatalf("placement: %+v", batch[0].Placement)
	}
	if batch[0].Comment.Anchor.LineText != "alpha" || batch[0].Comment.RevisionN != 1 {
		t.Fatalf("orphan keeps quote + original-revision link: %+v", batch[0].Comment)
	}
}
