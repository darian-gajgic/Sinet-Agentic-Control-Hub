package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// Exactly ONE code path hands review feedback to a retry (Spec S13.4;
// FC-v1 §2): Store.Drain. At rework dispatch it (1) collects every open
// comment/finding on the deliverable — human comments and verification
// findings, one schema — ported to the current revision; (2) numbers them
// [F1..Fn], stable within the attempt; (3) marks the batch consumed with
// ONE shared consumed_at + the consuming attempt ref; (4) returns the
// numbered points for delivery into the next attempt's brief (the S07.6
// retry package rides the S05 brief seam). Orphaned and file-degraded
// findings are STILL delivered — delivery is never conditional on
// anchoring success (P-T12-2). Severity distinguishes blockers from notes:
// only genuine blockers trigger rounds (the S07 verdict's law); notes
// travel with the batch (5.4).
//
// A conformance test pins that consumption is written nowhere else.

// DrainRequest names one drain act.
type DrainRequest struct {
	DeliverableID string
	// AttemptRef is the consuming attempt (the rework round's session
	// identity) — the batch stamp's "what did that rework receive" key.
	AttemptRef string
	// RunID scopes the review.drained event (the verify run driving the
	// rework).
	RunID string
}

// Drained is one numbered point of a drained batch.
type Drained struct {
	Comment   Comment   `json:"comment"`
	Number    int       `json:"number"` // [F1..Fn]
	Placement Placement `json:"placement"`
}

// Drain executes the S13.4 drain point. An empty open set returns an empty
// batch (nothing consumed, no event).
func (s *Store) Drain(ctx context.Context, req DrainRequest) ([]Drained, error) {
	if req.DeliverableID == "" || req.AttemptRef == "" {
		return nil, fmt.Errorf("%w: drain needs a deliverable and an attempt ref", ErrBadInput)
	}
	d, err := s.Deliverable(ctx, req.DeliverableID)
	if err != nil {
		return nil, err
	}
	open, err := s.OpenComments(ctx, req.DeliverableID)
	if err != nil {
		return nil, err
	}
	if len(open) == 0 {
		return nil, nil
	}
	// Port every open comment to the current revision — placements are
	// recorded per comment per revision (Spec S13.3); orphan/file results
	// keep draining below.
	placements, err := s.placeAll(ctx, req.DeliverableID, d.CurrentRevision, open)
	if err != nil {
		return nil, err
	}
	byID := map[int64]Placement{}
	for _, p := range placements {
		byID[p.CommentID] = p
	}

	// Deterministic numbering (Spec S13.4 "stable within the attempt"):
	// blockers before notes, insertion order within each — comment_id is
	// the explicit, dump-stable insertion sequence.
	batch := make([]Drained, 0, len(open))
	for _, c := range open {
		batch = append(batch, Drained{Comment: c, Placement: byID[c.ID]})
	}
	sort.SliceStable(batch, func(i, j int) bool {
		bi := batch[i].Comment.Severity == SeverityBlocker
		bj := batch[j].Comment.Severity == SeverityBlocker
		if bi != bj {
			return bi
		}
		return batch[i].Comment.ID < batch[j].Comment.ID
	})
	for i := range batch {
		batch[i].Number = i + 1
	}

	// One transaction: the shared batch stamp + the drained event.
	consumedAt := s.nowRFC3339()
	err = s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, b := range batch {
			res, err := tx.ExecContext(ctx,
				`UPDATE review_comments
				    SET status = 'consumed', finding_number = ?, consumed_ts = ?, consumed_by = ?
				  WHERE comment_id = ? AND status = 'open'`,
				b.Number, consumedAt, req.AttemptRef, b.Comment.ID)
			if err != nil {
				return fmt.Errorf("review: consume comment %d: %w", b.Comment.ID, err)
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n != 1 {
				return fmt.Errorf("review: comment %d was not open at drain time", b.Comment.ID)
			}
		}
		type evPoint struct {
			ID       int64  `json:"id"`
			Number   int    `json:"n"`
			Severity string `json:"severity"`
			Status   string `json:"anchor_status"`
		}
		pts := make([]evPoint, 0, len(batch))
		for _, b := range batch {
			pts = append(pts, evPoint{ID: b.Comment.ID, Number: b.Number,
				Severity: b.Comment.Severity, Status: string(b.Placement.Status)})
		}
		payload, err := json.Marshal(map[string]any{
			"deliverable_id": req.DeliverableID,
			"revision_n":     d.CurrentRevision,
			"attempt_ref":    req.AttemptRef,
			"consumed_at":    consumedAt,
			"points":         pts,
		})
		if err != nil {
			return err
		}
		a := eventlog.Append{
			UserID: d.Owner, Type: EventDrained,
			SchemaVersion: eventSchemaVersion, Payload: payload, Time: s.now(),
		}
		if req.RunID != "" {
			owner, gen, err := s.runScope(ctx, tx, req.RunID)
			if err != nil {
				return err
			}
			a.RunID, a.Generation, a.UserID = req.RunID, gen, owner
		}
		_, err = s.Log.AppendTx(ctx, tx, a)
		return err
	})
	if err != nil {
		return nil, err
	}
	for i := range batch {
		batch[i].Comment.Status = StatusConsumed
		batch[i].Comment.FindingNumber = batch[i].Number
		batch[i].Comment.ConsumedTS = consumedAt
		batch[i].Comment.ConsumedBy = req.AttemptRef
	}
	return batch, nil
}
