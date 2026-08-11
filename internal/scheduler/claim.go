package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// laneKey identifies a per-(user, lane) concurrency slot bucket (Spec S02.2
// lanes family; S10.7 per-(user, lane) slots).
type laneKey struct {
	user string
	lane string
}

// SetLaneCap declares the per-(user, lane) concurrency cap — DATA the operator
// edits, seeded from observed concurrency (Spec S10.7; the lanes table is data,
// not a ⚙). Zero serializes the lane; a raised cap admits more concurrent runs.
func (s *Scheduler) SetLaneCap(ctx context.Context, userID, lane string, cap int64) error {
	if cap < 0 {
		return fmt.Errorf("scheduler: negative lane cap for %q/%s", userID, lane)
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO lanes (user_id, lane, concurrency_cap) VALUES (?, ?, ?)
			 ON CONFLICT (user_id, lane) DO UPDATE SET concurrency_cap = excluded.concurrency_cap`,
			userID, lane, cap)
		if err != nil {
			return fmt.Errorf("scheduler: set lane cap %q/%s: %w", userID, lane, err)
		}
		return nil
	})
}

// laneCap returns the declared per-(user, lane) cap, or defaultLaneConcurrency
// when no lanes row exists (Spec S10.7: observed concurrency starts at 1).
func (s *Scheduler) laneCap(ctx context.Context, userID, lane string) int64 {
	var cap int64
	err := s.db.QueryRowContext(ctx,
		`SELECT concurrency_cap FROM lanes WHERE user_id = ? AND lane = ?`, userID, lane).Scan(&cap)
	if err != nil {
		return defaultLaneConcurrency
	}
	return cap
}

// activeSlots counts the runs currently occupying a slot per (user, lane) —
// claimed, running, and draining runs hold a live slot; queued/parked/terminal
// do not (Spec S10.7 observed concurrency). Parked runs release their slot and
// re-contend on resume.
func (s *Scheduler) activeSlots(ctx context.Context) (map[laneKey]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, lane, COUNT(*) FROM runs
		  WHERE state IN (?, ?, ?) GROUP BY user_id, lane`,
		string(run.StateClaimed), string(run.StateRunning), string(run.StateDraining))
	if err != nil {
		return nil, fmt.Errorf("scheduler: count active slots: %w", err)
	}
	defer rows.Close()
	out := map[laneKey]int64{}
	for rows.Next() {
		var k laneKey
		var n int64
		if err := rows.Scan(&k.user, &k.lane, &n); err != nil {
			return nil, fmt.Errorf("scheduler: scan active slots: %w", err)
		}
		out[k] = n
	}
	return out, rows.Err()
}

// candidates reads the claimable queue rows joined to their runs — only rows
// whose run is still in the queued state (a run that moved on is stale and
// skipped). The lane comes from the run (Spec S02.2 runs.lane).
func (s *Scheduler) candidates(ctx context.Context) ([]candidate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT q.queue_id, q.run_id, q.user_id, r.lane, r.substrate, q.priority_lane, q.enqueued_ts, q.hint_rank
		   FROM queue q JOIN runs r ON r.run_id = q.run_id
		  WHERE q.status = ? AND r.state = ?`,
		queueQueued, string(run.StateQueued))
	if err != nil {
		return nil, fmt.Errorf("scheduler: read candidates: %w", err)
	}
	defer rows.Close()
	var out []candidate
	for rows.Next() {
		var (
			c        candidate
			class    string
			enqueued string
		)
		if err := rows.Scan(&c.queueID, &c.runID, &c.userID, &c.lane, &c.substrate, &class, &enqueued, &c.hintRank); err != nil {
			return nil, fmt.Errorf("scheduler: scan candidate: %w", err)
		}
		c.class = WorkloadClass(class)
		if !validClass(c.class) {
			c.class = DefaultWorkloadClass
		}
		if t, err := time.Parse(time.RFC3339Nano, enqueued); err == nil {
			c.enqueuedTS = t
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// reconcileQueueRows materializes a queue row for any run sitting in the queued
// state without one — recovery fork successors enter queued directly and the
// queue-row materialization is the scheduler's (Spec S02.5 step 3; B0-4 note).
func (s *Scheduler) reconcileQueueRows(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.run_id, r.user_id FROM runs r
		  WHERE r.state = ?
		    AND NOT EXISTS (SELECT 1 FROM queue q WHERE q.run_id = r.run_id AND q.status IN (?, ?))`,
		string(run.StateQueued), queueQueued, queueClaimed)
	if err != nil {
		return fmt.Errorf("scheduler: find unqueued runs: %w", err)
	}
	defer rows.Close()
	type orphan struct{ runID, userID string }
	var orphans []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.runID, &o.userID); err != nil {
			return fmt.Errorf("scheduler: scan unqueued run: %w", err)
		}
		orphans = append(orphans, o)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, o := range orphans {
		err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			return s.materializeQueueRowTx(ctx, tx, o.runID, o.userID, DefaultWorkloadClass)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// settleEndedRows settles the queue rows of runs that have ENDED — the
// level-triggered half of S10.7's queue discipline ("'done' is a settled
// admission"), run once per claim pass beside reconcileQueueRows.
//
// The gap it closes: the post-dispatch settle fires only on the dispatch path
// and only for `IsTerminal` states, and `IsTerminal(crashed)` is deliberately
// false (crashed is supersedable). So when the recovery ladder LATER tombstones,
// finalizes or harvests a crashed run — or forks it, leaving the superseded
// parent behind — nobody settled the row: recovery never touches the queue (that
// wall is deliberate, CONVENTIONS §11) and `SettleRun` refuses non-terminals.
// The rows accumulated forever, and every surface that joins the queue family
// read a lie about what was still admitted (16 leaked rows in the live world).
//
// Level-triggered rather than event-driven, deliberately: it is the kubelet
// shape the ladder already follows, it heals rows that PREDATE this code, and it
// keeps the queue family single-owner — no recovery→queue seam is invented.
//
// NO-AUTO-KILL (S14.4 / G1 D1.3): this settles QUEUE ROWS of runs that already
// ended; it never transitions a run, never touches a live claim, and never
// touches a `parked` run — an open-gate park keeps its claimed row by documented
// design (skeleton.go's gate comment) and its resume continues under it.
func (s *Scheduler) settleEndedRows(ctx context.Context) error {
	type ended struct {
		queueID int64
		runID   string
		state   run.State
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT q.queue_id, q.run_id, r.state
		  FROM queue q JOIN runs r ON r.run_id = q.run_id
		 WHERE q.status IN (?, ?)
		   AND r.state IN (?, ?, ?, ?, ?)
		 ORDER BY q.queue_id`,
		queueQueued, queueClaimed,
		string(run.StateCompleted), string(run.StateCrashed), string(run.StateFinalized),
		string(run.StateTombstoned), string(run.StateDiedAtGate))
	if err != nil {
		return fmt.Errorf("scheduler: find ended queue rows: %w", err)
	}
	defer rows.Close()
	var out []ended
	for rows.Next() {
		var e ended
		var state string
		if err := rows.Scan(&e.queueID, &e.runID, &state); err != nil {
			return fmt.Errorf("scheduler: scan ended queue row: %w", err)
		}
		e.state = run.State(state)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// One bounded write transaction per row (Spec S02.1 read hygiene: no
	// long-lived transaction against the WAL).
	for _, e := range out {
		err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx,
				`UPDATE queue SET status = ? WHERE queue_id = ? AND status IN (?, ?)`,
				queueDone, e.queueID, queueQueued, queueClaimed)
			if err != nil {
				return fmt.Errorf("settle queue row %d: %w", e.queueID, err)
			}
			if n, err := res.RowsAffected(); err != nil || n == 0 {
				return err // already settled by a concurrent pass; nothing else to do
			}
			// The receipt is the run-end record (S10.1: receipts per run-end),
			// materialized through the same idempotent path the dispatch settle
			// uses. A CRASHED run settles QUEUE-ONLY: its admission is over, but
			// its lineage is not — the spend so far sits on its checkpoints and
			// each successor ends with its own receipt, so minting one here would
			// assert an ending the lineage has not reached (D4).
			if s.receipts != nil && run.IsTerminal(e.state) {
				if _, err := s.receipts.MaterializeTx(ctx, tx, e.runID); err != nil {
					return fmt.Errorf("materialize receipt for %q: %w", e.runID, err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// sortCandidates orders candidates by the S10.7 priority ladder + aging
// (workload.go priorityLess), highest priority first.
//
// Two steps, in this order and never merged (drain D1): the S15.5 drag hint is
// applied first as a key permutation WITHIN each (user, class) group, and only
// then is the whole slice sorted by the resulting keys. Sorting by a fixed value
// is what makes the claim order transitive and therefore independent of the row
// order the candidates arrived in.
func sortCandidates(cands []candidate, now time.Time) {
	permuteHints(cands, now)
	sort.SliceStable(cands, func(i, j int) bool { return priorityLess(cands[i], cands[j], now) })
}

// ── nudge + in-flight tracking ─────────────────────────────────────────────

func (s *Scheduler) doNudge() {
	select {
	case s.nudge <- struct{}{}:
	default: // a nudge is already pending; coalesce
	}
}

func (s *Scheduler) nudged() <-chan struct{} { return s.nudge }

func (s *Scheduler) track(runID string) {
	s.inflightMu.Lock()
	s.inflight[runID] = struct{}{}
	s.inflightMu.Unlock()
}

func (s *Scheduler) untrack(runID string) {
	s.inflightMu.Lock()
	delete(s.inflight, runID)
	s.inflightMu.Unlock()
}

func (s *Scheduler) trackedRuns() []string {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	out := make([]string, 0, len(s.inflight))
	for id := range s.inflight {
		out = append(out, id)
	}
	return out
}
