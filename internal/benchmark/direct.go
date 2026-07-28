package benchmark

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// direct.go is the storage half of the BENCH-REG §2 direct arm's single-shot
// OUTPUT (P3-B6-2C, migration 0018) plus the pairs-by-state listing the
// dispatch→render driver reads to know what is due.
//
// What lives here is deliberately small. The DRIVER is the shell's (it owns
// WHEN, the §31/§34/§35 precedent) and the LEG is the dispatcher's (it owns the
// engine), so this package gains no dispatcher, no engine edge and no clock: it
// gains a read of "which pairs are in this state" and a write/read pair for the
// captured text. Nothing here kills, parks or transitions anything, and nothing
// here decides that a pair should advance — the state checks on
// DispatchDirectArm and RenderBlind remain the only authority over the §3 order.

// stateBatchCap bounds one pairs-by-state read. Structural, not ⚙ (S18 ratifies
// no such key — the §7 sseBatchSize / §35 tail-batch precedent, interim under
// the standing settings-tab directive), with its reason: the caller is a driver
// pass that works a queue down, and a pass is a batch. The bound decides how
// many pairs one pass picks up, never which pairs are due — dueness is the
// STORED state and a pair skipped by the bound is picked up by the next pass in
// exactly the same order (sampled_ts, pair_id).
const stateBatchCap = 256

// PairsInState returns the pairs sitting in one §3 working state, oldest draw
// first, bounded. It is a READ: it advances nothing and decides nothing.
//
// The driver needs it because a pair's dueness is its STORED state — there is no
// timer and no queue of pending work anywhere in this practice. limit ≤ 0 means
// the structural batch cap.
func (s *Store) PairsInState(ctx context.Context, state string, limit int) ([]Pair, error) {
	if limit <= 0 || limit > stateBatchCap {
		limit = stateBatchCap
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+pairColumns+` FROM benchmark_pairs WHERE state = ?
		  ORDER BY sampled_ts, pair_id LIMIT ?`, state, limit)
	if err != nil {
		return nil, fmt.Errorf("benchmark: list pairs in state %q: %w", state, err)
	}
	defer rows.Close()
	out := []Pair{}
	for rows.Next() {
		p, err := scanPair(rows)
		if err != nil {
			return nil, fmt.Errorf("benchmark: scan pair: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ErrNoDirectCapture: the direct arm of this run has produced no captured
// output. It is an HONEST ABSENCE and never a placeholder — a fabricated body
// would render a "blind pair" of the platform against itself and make every
// statistic downstream fiction (§35; BENCH-REG §2).
var ErrNoDirectCapture = errors.New("benchmark: the direct arm has produced no captured output (BENCH-REG §2)")

// CaptureDirectText records what the §2 single-shot direct arm produced, keyed
// by its RUN, which is how the dispatcher knows it: the pair is found through
// the direct_run_id the dispatch wrote. It reports whether a pair took the
// capture.
//
// text is stored VERBATIM. The platform never truncates it — §3.2 reports the
// length confound and never corrects it — and an EMPTY text is stored as an
// empty string rather than skipped, because an arm that produced nothing is a
// real single-shot outcome and not a missing measurement (the null-versus-empty
// split migration 0018 exists to keep).
//
// The write is scoped to a pair that has not yet written its §14 record: a
// recorded or declined pair is frozen (0014/0015 triggers), and a capture
// arriving after a result would be an attempt to change what a committed record
// was computed from (BENCH-REG §17). Such a late capture is reported as "no pair
// took it" rather than as an error — the run genuinely finished, and its own
// disposition is already on the record.
func (s *Store) CaptureDirectText(ctx context.Context, runID, text string) (bool, error) {
	if runID == "" {
		return false, fmt.Errorf("%w: a direct-arm capture needs its run id", ErrBadInput)
	}
	var n int64
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE benchmark_pairs SET direct_text = ?, updated_ts = ?
			  WHERE direct_run_id = ? AND state NOT IN (?, ?)`,
			text, s.ts(), runID, StateRecorded, StateDeclined)
		if err != nil {
			return fmt.Errorf("benchmark: capture direct-arm output of %q: %w", runID, err)
		}
		n, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CapturedDirectText reads back what the §2 direct arm produced for a run. It is
// the real read behind the shell's DirectText seam.
//
// An unknown run, or a pair whose capture column is still NULL, returns
// ErrNoDirectCapture: the direct arm has not answered, so there is nothing to
// render blind and the pair correctly stays where it is. A captured EMPTY answer
// returns ("", nil) — the arm answered with nothing, which is an outcome.
func (s *Store) CapturedDirectText(ctx context.Context, runID string) (string, error) {
	if runID == "" {
		return "", fmt.Errorf("%w: no run named", ErrNoDirectCapture)
	}
	var text sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT direct_text FROM benchmark_pairs WHERE direct_run_id = ?`, runID).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: no pair names run %q as its direct arm", ErrNoDirectCapture, runID)
	}
	if err != nil {
		return "", fmt.Errorf("benchmark: read direct-arm capture of %q: %w", runID, err)
	}
	if !text.Valid {
		return "", fmt.Errorf("%w: run %q has not written one — single-shot execution and its capture are the dispatcher's, and no pair completes without it", ErrNoDirectCapture, runID)
	}
	return text.String, nil
}
