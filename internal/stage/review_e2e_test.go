package stage_test

// review_e2e_test.go — the S13.1–S13.4 acceptance over the REAL spine +
// REAL review store (fake engine, zero paid calls): revisions minted at
// the verification handoff (one per round, pins matching the persisted
// artifacts), judge findings landing as durable review comments, THE
// S13.4 drain composing every rework's retry package with batch-stamped
// consumption, and the S07.7 answer path draining parked findings +
// guidance through the same one path.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

func TestReviewSchemaRidesVerifyE2E(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=fail-unless-guidance")
	ctx := context.Background()
	const owner = "u-operator"
	taskID, verifyRunID, askID, card := driveToOpenCapHit(t, h, owner)
	dlvID := stage.TaskDeliverableID(taskID)

	// ---- S13.1: the deliverable row + immutable revision lineage, minted
	// one per round at the verification handoff. ----
	dlv, err := h.review.Deliverable(ctx, dlvID)
	if err != nil {
		t.Fatalf("deliverable row: %v", err)
	}
	if dlv.Owner != owner || dlv.TaskID != taskID || dlv.State != review.StateInReview {
		t.Fatalf("deliverable row: %+v", dlv)
	}
	lastRound := card.Rounds[len(card.Rounds)-1]
	if dlv.CurrentRevision != lastRound.Revision {
		t.Fatalf("current revision %d, want the parked round's %d", dlv.CurrentRevision, lastRound.Revision)
	}
	revs, err := h.review.Revisions(ctx, dlvID)
	if err != nil || len(revs) != lastRound.Revision {
		t.Fatalf("lineage 1..N: %d revs err %v", len(revs), err)
	}
	// Every judged round's pin matches the minted revision's pin, and the
	// verdict ref filled (fill-once) with the round's verdict.recorded seq.
	for _, rd := range card.Rounds {
		rev, err := h.review.RevisionAt(ctx, dlvID, rd.Revision)
		if err != nil {
			t.Fatalf("revision %d: %v", rd.Revision, err)
		}
		if rev.ContentSHA256 != rd.ContentSHA {
			t.Fatalf("rev %d pin %s != round record %s", rd.Revision, rev.ContentSHA256, rd.ContentSHA)
		}
		if rev.VerdictRef != rd.EventSeq {
			t.Fatalf("rev %d verdict ref %d != verdict.recorded seq %d", rd.Revision, rev.VerdictRef, rd.EventSeq)
		}
		if rev.RunID != verifyRunID {
			t.Fatalf("rev %d minting run %q", rd.Revision, rev.RunID)
		}
	}

	// ---- S13.4 pre-park: each dispatched rework drained ONE batch,
	// consumed with the shared stamp naming its attempt. ----
	drainedEvents := 0
	for _, e := range h.reviewEvents(t, review.EventDrained) {
		if strings.Contains(string(e), taskID) {
			drainedEvents++
		}
	}
	// One drain per dispatched rework — a rework ran between each judged
	// round pair; the CAP-HIT terminal itself never drains.
	if want := len(card.Rounds) - 1; drainedEvents != want {
		t.Fatalf("pre-park drains: %d, want %d", drainedEvents, want)
	}
	openBefore, err := h.review.OpenComments(ctx, dlvID)
	if err != nil || len(openBefore) == 0 {
		t.Fatalf("the parked round's findings stay OPEN on the card: %d err %v", len(openBefore), err)
	}

	// ---- S07.7 answer: guidance lands as durable requester comments and
	// the resumed rework drains them WITH the parked findings — one path,
	// one batch stamp. ----
	answer := json.RawMessage(`{"choice":"revise_with_guidance","guidance":[{"text":"apply the agreed marker line","criterion":"AC-1"}]}`)
	if _, err := h.sur.Answer(ctx, owner, askID, answer, false); err != nil {
		t.Fatalf("Answer(revise_with_guidance): %v", err)
	}

	all, err := h.allReviewComments(ctx, dlvID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	var (
		guidanceRow *review.Comment
		batchTS     = map[string]string{}
	)
	for i := range all {
		c := all[i]
		if c.Kind == review.KindHuman && strings.Contains(c.Body, "agreed marker line") {
			guidanceRow = &all[i]
		}
		if c.Status == review.StatusConsumed {
			if prev, ok := batchTS[c.ConsumedBy]; ok && prev != c.ConsumedTS {
				t.Fatalf("batch %s carries two consumed_at stamps: %s vs %s", c.ConsumedBy, prev, c.ConsumedTS)
			}
			batchTS[c.ConsumedBy] = c.ConsumedTS
		} else if c.Status != review.StatusOpen {
			t.Fatalf("comment %d in impossible state %q", c.ID, c.Status)
		}
	}
	if guidanceRow == nil {
		t.Fatalf("guidance did not land as a durable requester comment")
	}
	if guidanceRow.Status != review.StatusConsumed || guidanceRow.Severity != review.SeverityBlocker {
		t.Fatalf("guidance row: %+v", guidanceRow)
	}
	resumeAttempt := guidanceRow.ConsumedBy
	if !strings.HasPrefix(resumeAttempt, verifyRunID+"#revise-r") {
		t.Fatalf("guidance consumed by %q, want the resumed rework attempt", resumeAttempt)
	}
	// The parked machine findings drained in the SAME batch as the
	// guidance (Spec S13.4 collects every open comment).
	sameBatch := 0
	for _, c := range all {
		if c.ConsumedBy == resumeAttempt {
			sameBatch++
			if c.ConsumedTS != guidanceRow.ConsumedTS {
				t.Fatalf("batch stamp not shared: %+v", c)
			}
		}
	}
	if sameBatch < 2 {
		t.Fatalf("resumed batch drained %d comments, want the parked findings + guidance", sameBatch)
	}
	// Numbering within the batch is contiguous [F1..Fn].
	seen := map[int]bool{}
	for _, c := range all {
		if c.ConsumedBy == resumeAttempt {
			seen[c.FindingNumber] = true
		}
	}
	for i := 1; i <= sameBatch; i++ {
		if !seen[i] {
			t.Fatalf("batch numbering has a hole at F%d: %v", i, seen)
		}
	}

	// The SHIP left no open comments behind on the deliverable? Notes may
	// remain open (they ride to the requester); blockers must all be
	// consumed.
	openAfter, err := h.review.OpenComments(ctx, dlvID)
	if err != nil {
		t.Fatalf("open after: %v", err)
	}
	for _, c := range openAfter {
		if c.Severity == review.SeverityBlocker {
			t.Fatalf("a blocker survived the drain unconsumed: %+v", c)
		}
	}

	// The final revision minted for the guidance rework carries the marker
	// content (pin round-trip through the object dir).
	files, err := h.review.RevisionFiles(ctx, dlvID, dlv.CurrentRevision+1)
	if err != nil {
		t.Fatalf("guidance revision files: %v", err)
	}
	if !strings.Contains(files[stage.DeliverableFileName], "GUIDANCE-APPLIED") {
		t.Fatalf("resumed revision content: %q", files[stage.DeliverableFileName])
	}
}

// reviewEvents returns raw payloads of one review event type.
func (h *harness) reviewEvents(t *testing.T, eventType string) []json.RawMessage {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq`, eventType)
	if err != nil {
		t.Fatalf("read %s events: %v", eventType, err)
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, json.RawMessage(p))
	}
	return out
}

// allReviewComments lists every comment row of a deliverable by walking
// the contiguous rowid sequence (rows are never deleted).
func (h *harness) allReviewComments(ctx context.Context, dlvID string) ([]review.Comment, error) {
	var all []review.Comment
	for id := int64(1); ; id++ {
		c, err := h.review.CommentByID(ctx, id)
		if err != nil {
			break
		}
		if c.DeliverableID == dlvID {
			all = append(all, c)
		}
	}
	return all, nil
}
