package watchlist

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Store is the S14.6 watch-row config store over `watch_rows` (migration
// 0013). Row structure is immutable by trigger; only the fetch-state columns
// move.
type Store struct {
	db *storage.DB
	// Now is the test clock seam; nil ⇒ time.Now.
	Now func() time.Time
}

// NewStore returns a watch-row store over db.
func NewStore(db *storage.DB) *Store { return &Store{db: db} }

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// EnsureSeeded idempotently seeds rows (INSERT OR IGNORE, so re-running is a
// no-op and a committed row is never rewritten — the §17/§32 seed precedent;
// the structure-immutable trigger blocks a structural update anyway). Seeding
// is UNCONDITIONAL: platform obligations need no D10 holder, unlike the S09.10
// house seeds. It returns the number of rows in the seed set.
func (s *Store) EnsureSeeded(ctx context.Context, rows []Row) (int, error) {
	for _, r := range rows {
		if err := r.Validate(); err != nil {
			return 0, err
		}
	}
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO watch_rows
				   (row_id, kind, url_or_feed, parser_hint, lane_or_candidate, tier,
				    cadence, enabled, source_group, notes, migrate_bias, user_id)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				r.ID, r.Kind, r.URL, r.ParserHint, r.Lane, r.Tier, r.Cadence,
				boolInt(r.Enabled), r.Group, r.Notes, boolInt(r.MigrateBias), platformUser,
			); err != nil {
				return fmt.Errorf("watchlist: seed row %q: %w", r.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

const rowColumns = `row_id, kind, url_or_feed, parser_hint, lane_or_candidate, tier,
	cadence, enabled, source_group, notes, migrate_bias,
	last_fetch_at, last_etag, last_modified, last_content_hash, seen_entries,
	structural_baseline, fail_streak, decay_stage, external_ref`

// List returns every watch row with its fetch state, ordered by id.
func (s *Store) List(ctx context.Context) ([]Row, error) {
	return s.query(ctx, `SELECT `+rowColumns+` FROM watch_rows ORDER BY row_id`)
}

// Row returns one watch row by id.
func (s *Store) Row(ctx context.Context, id string) (Row, error) {
	rows, err := s.query(ctx, `SELECT `+rowColumns+` FROM watch_rows WHERE row_id = ?`, id)
	if err != nil {
		return Row{}, err
	}
	if len(rows) == 0 {
		return Row{}, fmt.Errorf("%w: %q", ErrUnknownRow, id)
	}
	return rows[0], nil
}

// Due returns the enabled, POLLABLE rows whose cadence has elapsed. Dueness
// derives from stored state (`last_fetch_at` + cadence), never an in-memory
// ticker, so it survives restart and suspend.
func (s *Store) Due(ctx context.Context) ([]Row, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]Row, 0, len(all))
	for _, r := range all {
		if r.due(now) {
			out = append(out, r)
		}
	}
	return out, nil
}

// PendingReview returns the enabled STUDY rows whose review cadence has
// elapsed — the read surface the S16.7 quarterly dependency pass consumes
// (R21). It raises nothing and moves nothing: outputs are PROPOSALS, never
// silent bumps (S16.7; G2 Def.16).
func (s *Store) PendingReview(ctx context.Context) ([]Row, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]Row, 0)
	for _, r := range all {
		if r.reviewDue(now) {
			out = append(out, r)
		}
	}
	return out, nil
}

// PageRows returns the enabled page-kind rows — the reconciliation input for
// the changedetection.io driver.
func (s *Store) PageRows(ctx context.Context) ([]Row, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0)
	for _, r := range all {
		if r.Kind == KindPage && r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

// RecordSuccess stamps a successful fetch cycle: the fetch time, the returned
// validators + content hash, the extended seen-entry set and structural
// baseline, and a CLEARED fail streak. Only fetch-state columns move (the
// structure-immutable trigger enforces it).
func (s *Store) RecordSuccess(ctx context.Context, id string, st FetchState) error {
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE watch_rows SET last_fetch_at = ?, last_etag = ?, last_modified = ?,
			        last_content_hash = ?, seen_entries = ?, structural_baseline = ?,
			        fail_streak = 0
			   WHERE row_id = ?`,
			st.LastFetchAt.UTC().Format(time.RFC3339Nano), st.ETag, st.LastModified,
			st.ContentHash, strings.Join(st.SeenEntries, "\n"), st.Baseline, id)
		if err != nil {
			return fmt.Errorf("watchlist: record fetch for %q: %w", id, err)
		}
		return affectedOne(res, id)
	})
}

// RecordFailure stamps a failed fetch cycle: the attempt time and an
// INCREMENTED fail streak. It returns the new streak so the caller can compare
// it against ⚙ watchlist.fetch_fail_streak — a dead watch MUST announce itself
// (P-T11-3).
func (s *Store) RecordFailure(ctx context.Context, id string, at time.Time) (int, error) {
	var streak int
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE watch_rows SET last_fetch_at = ?, fail_streak = fail_streak + 1 WHERE row_id = ?`,
			at.UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return fmt.Errorf("watchlist: record fetch failure for %q: %w", id, err)
		}
		if err := affectedOne(res, id); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT fail_streak FROM watch_rows WHERE row_id = ?`, id).Scan(&streak)
	})
	return streak, err
}

// SetDecayStage advances a row's fetcher-escalation stage and resets its fail
// streak so the escalated fetcher gets a fresh budget before the next rung
// (P-T11-3).
func (s *Store) SetDecayStage(ctx context.Context, id string, stage int) error {
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE watch_rows SET decay_stage = ?, fail_streak = 0 WHERE row_id = ?`, stage, id)
		if err != nil {
			return fmt.Errorf("watchlist: set decay stage for %q: %w", id, err)
		}
		return affectedOne(res, id)
	})
}

// SetExternalRef records the changedetection.io watch uuid a page row
// reconciles onto, so reconciliation is idempotent across restarts and a stale
// remote watch is deleted rather than orphaned.
func (s *Store) SetExternalRef(ctx context.Context, id, ref string) error {
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE watch_rows SET external_ref = ? WHERE row_id = ?`, ref, id)
		if err != nil {
			return fmt.Errorf("watchlist: set external ref for %q: %w", id, err)
		}
		return affectedOne(res, id)
	})
}

func (s *Store) query(ctx context.Context, q string, args ...any) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("watchlist: query watch rows: %w", err)
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var (
			r             Row
			enabled, bias int
			lastFetch     sql.NullString
			seen          string
		)
		if err := rows.Scan(&r.ID, &r.Kind, &r.URL, &r.ParserHint, &r.Lane, &r.Tier,
			&r.Cadence, &enabled, &r.Group, &r.Notes, &bias,
			&lastFetch, &r.ETag, &r.LastModified, &r.ContentHash, &seen,
			&r.Baseline, &r.FailStreak, &r.DecayStage, &r.ExternalRef); err != nil {
			return nil, fmt.Errorf("watchlist: scan watch row: %w", err)
		}
		r.Enabled = enabled == 1
		r.MigrateBias = bias == 1
		if lastFetch.Valid && lastFetch.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, lastFetch.String); err == nil {
				r.LastFetchAt, r.HasFetched = t, true
			}
		}
		if seen != "" {
			r.SeenEntries = strings.Split(seen, "\n")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func affectedOne(res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", ErrUnknownRow, id)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
