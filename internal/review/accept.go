package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// The S13.6 accept state verb — storage + eventlog only (CONVENTIONS §22).
// The OUTWARD accept mechanics (applies-cleanly depth-1 queue, platform
// squash, deterministic trailers, broker CAS push through the effect journal)
// compose ABOVE this package, in internal/accept over internal/project +
// internal/gates + internal/broker (Spec S13.6; CONVENTIONS §27). This verb is
// the durable state outcome those mechanics commit on success: the deliverable
// moves to accepted, and a newly accepted definition supersedes its prior
// accepted version (Spec S13.1 in-review/accepted/superseded).

// AcceptResult is the durable outcome of an accept.
type AcceptResult struct {
	Deliverable Deliverable
	// Superseded lists any deliverables moved to superseded by this accept
	// (a definition's prior accepted version, Spec S13.1). Empty for the
	// common task-deliverable case.
	Superseded []string
}

// Accept moves a deliverable to the accepted state (Spec S13.6). It is
// idempotent on an already-accepted deliverable so the class-A accept effect
// can replay safely across the S02.7 crash window (a blind retry re-runs
// Accept, which is a no-op the second time). A superseded deliverable cannot
// be accepted, and a deliverable with no minted revision has nothing to accept
// (ErrBadInput). Supersession: any OTHER currently-accepted deliverable
// sharing this one's non-empty subject_ref moves to superseded (the
// definition-supersession case, Spec S13.1); task deliverables (empty
// subject_ref) supersede nothing across tasks. The event is platform-scope
// (an approval-card human act, not a run event), owner-attributed (15.6) with
// the accepting principal in the payload.
func (s *Store) Accept(ctx context.Context, deliverableID, acceptingUser string) (AcceptResult, error) {
	if deliverableID == "" || acceptingUser == "" {
		return AcceptResult{}, fmt.Errorf("%w: accept needs a deliverable and an accepting user", ErrBadInput)
	}
	d, err := s.Deliverable(ctx, deliverableID)
	if err != nil {
		return AcceptResult{}, err
	}
	switch d.State {
	case StateAccepted:
		return AcceptResult{Deliverable: d}, nil // idempotent (class-A replay)
	case StateSuperseded:
		return AcceptResult{}, fmt.Errorf("%w: deliverable %q is superseded and cannot be accepted", ErrBadInput, deliverableID)
	}
	if d.CurrentRevision < 1 {
		return AcceptResult{}, fmt.Errorf("%w: deliverable %q has no minted revision to accept", ErrBadInput, deliverableID)
	}

	var res AcceptResult
	now := s.nowRFC3339()
	err = s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		// Supersede any prior accepted version sharing a non-empty subject_ref
		// (Spec S13.1: a new accepted definition supersedes the old).
		var superseded []string
		if d.SubjectRef != "" {
			rows, err := tx.QueryContext(ctx,
				`SELECT deliverable_id FROM deliverables
				 WHERE subject_ref = ? AND state = ? AND deliverable_id <> ?`,
				d.SubjectRef, StateAccepted, deliverableID)
			if err != nil {
				return fmt.Errorf("review: scan prior accepted: %w", err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				superseded = append(superseded, id)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			for _, id := range superseded {
				if _, err := tx.ExecContext(ctx,
					`UPDATE deliverables SET state = ?, updated_ts = ? WHERE deliverable_id = ?`,
					StateSuperseded, now, id); err != nil {
					return fmt.Errorf("review: supersede %q: %w", id, err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE deliverables SET state = ?, updated_ts = ? WHERE deliverable_id = ?`,
			StateAccepted, now, deliverableID); err != nil {
			return fmt.Errorf("review: accept %q: %w", deliverableID, err)
		}
		payload, err := json.Marshal(map[string]any{
			"deliverable_id": deliverableID,
			"revision":       d.CurrentRevision,
			"project_id":     d.ProjectID,
			"accepted_by":    acceptingUser,
			"superseded":     superseded,
		})
		if err != nil {
			return err
		}
		if _, err := s.Log.AppendTx(ctx, tx, eventlog.Append{
			UserID: d.Owner, Type: EventAccepted, SchemaVersion: eventSchemaVersion,
			Payload: payload, Time: s.now(),
		}); err != nil {
			return err
		}
		res.Superseded = superseded
		return nil
	})
	if err != nil {
		return AcceptResult{}, err
	}
	res.Deliverable, err = s.Deliverable(ctx, deliverableID)
	if err != nil {
		return AcceptResult{}, err
	}
	return res, nil
}

// AcceptedRevision returns the accepted deliverable's current (accepted)
// revision — the pin the accept push targets. It errors if the deliverable is
// not accepted, so a push can never be composed against an unaccepted
// deliverable (Spec S13.6: main only via accepts).
func (s *Store) AcceptedRevision(ctx context.Context, deliverableID string) (Revision, error) {
	d, err := s.Deliverable(ctx, deliverableID)
	if err != nil {
		return Revision{}, err
	}
	if d.State != StateAccepted {
		return Revision{}, fmt.Errorf("%w: deliverable %q is %s, not accepted", ErrBadInput, deliverableID, d.State)
	}
	return s.RevisionAt(ctx, deliverableID, d.CurrentRevision)
}
