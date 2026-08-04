package scheduler

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// hint.go — the S15.5 board-drag priority hint (P3-B6-2B, R10).
//
// S15.5 sanctions exactly one drag interaction at v0: "reordering one's own
// queued tasks as a scheduler priority hint". Everything about the shape below
// follows from the three words in that sentence that do the work:
//
//   - OWN — the hint lives on the queue row of the acting person's own run, and
//     priorityLess only ever compares it between two candidates of the SAME
//     owner. It cannot reach into anybody else's slots. The transport enforces
//     the ownership; the ordering rule makes the enforcement structural.
//   - QUEUED — a hint applies to work that has not been claimed. A run that
//     moved on is answered as the stale board it is (below), never as an error
//     that implies an authority the drag does not have.
//   - HINT — it is a tie-break WITHIN one (user, workload class) bucket. It
//     never outranks the S10.7 class ladder, and it changes no FSM state of any
//     kind: the run's stage columns stay FSM-owned (S02.3).

// SetPriorityHint writes the S15.5 drag hint onto a run's QUEUED queue row.
// Lower ranks are claimed earlier; 0 is "no hint".
//
// The bool reports whether a queued row was actually updated. False is not a
// failure — it is the honest stale-board answer: the run has been claimed, has
// finished, or was cancelled since the board the person dragged was drawn, and
// the ordering they asked for no longer has anything to order. The caller says
// so rather than pretending the hint took.
//
// The write is scoped to `status = 'queued'` in the statement itself, so it can
// never mutate a row the claim loop is holding a lease on: a hint that raced a
// claim simply matches no row.
func (s *Scheduler) SetPriorityHint(ctx context.Context, runID string, rank int64) (bool, error) {
	applied := false
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE queue SET hint_rank = ?
			  WHERE run_id = ? AND status = ?
			    AND EXISTS (SELECT 1 FROM runs r WHERE r.run_id = queue.run_id AND r.state = ?)`,
			rank, runID, queueQueued, string(run.StateQueued))
		if err != nil {
			return fmt.Errorf("scheduler: set priority hint for %q: %w", runID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		applied = n > 0
		return nil
	})
	if err != nil {
		return false, err
	}
	if applied {
		// The claim loop re-sorts on its next pass; the nudge is what makes the
		// reorder visible now rather than at the poll backstop.
		s.doNudge()
	}
	return applied, nil
}
