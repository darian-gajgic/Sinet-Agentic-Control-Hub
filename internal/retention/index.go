package retention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// index.go — the S14.10 ¶4 index layer this packet owns:
//
//	"FTS5 over run summaries, verdict texts, and drift summaries; rollup tables
//	 maintained by scheduler jobs."
//
// Both are DERIVED STATE, never a system of record (S14.1: tools are runners or
// projectors, never stores). Everything here is rebuildable from run_events by
// replay, which Rebuild does; nothing reads them for a decision.
//
// The pass is a projector over the log past a DURABLE cursor. Durable rather
// than head-bootstrapped because a missed row here is a permanent hole in the
// search surface — a summary written during a restart would simply never be
// findable — unlike a detection tail, where a missed row is a stale signal that
// the next evaluation re-derives.
//
// Idempotence is by ROWID: every indexed row's FTS rowid IS its source
// event_seq, so re-indexing a row replaces it rather than duplicating it. That
// is also what makes Rebuild safe to run at any time.

// IndexInterval is the shell driver's tick for the indexer — a STRUCTURAL
// CONSTANT, not a ⚙ row (the §35 sampling-loop precedent). Its reason: the
// cursor is durable, so the tick decides only how promptly a new summary,
// verdict or drift finding becomes searchable — never whether it is indexed.
// Interim under the standing settings-tab directive.
const IndexInterval = time.Minute

// indexBatch bounds one indexing transaction. Structural: it changes only how
// many transactions a catch-up takes, never the result.
const indexBatch = 512

// The three S14.10 ¶4 FTS5 sources, as stored in history_fts.kind.
const (
	KindRunSummary = "run_summary"
	KindVerdict    = "verdict"
	KindDrift      = "drift"
)

// indexedTypes maps a run_events type to the FTS kind it feeds. Legacy aliases
// are included so a pre-rename row is still indexed under its record's kind
// (§29 R14).
var indexedTypes = map[string]string{
	EventSummaryWritten: KindRunSummary,
	"verdict.recorded":  KindVerdict,
	"verify.round":      KindVerdict,
	"drift.finding":     KindDrift,
}

// IndexResult is one indexing pass.
type IndexResult struct {
	Indexed    int64
	Rolled     int64
	FromSeq    int64
	ThroughSeq int64
}

// Index advances the derived index (FTS5 + rollup) over every run_events row
// past the durable cursor. Idempotent and restart-safe: re-running it indexes
// nothing twice and skips nothing.
func (s *Store) Index(ctx context.Context) (IndexResult, error) {
	var res IndexResult
	for {
		batch, err := s.indexOnce(ctx)
		if err != nil {
			return res, err
		}
		if batch.ThroughSeq == 0 {
			return res, nil
		}
		if res.FromSeq == 0 {
			res.FromSeq = batch.FromSeq
		}
		res.Indexed += batch.Indexed
		res.Rolled += batch.Rolled
		res.ThroughSeq = batch.ThroughSeq
	}
}

func (s *Store) indexOnce(ctx context.Context) (IndexResult, error) {
	var res IndexResult
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var cursor int64
		if err := tx.QueryRowContext(ctx,
			`SELECT last_seq FROM retention_index_cursor WHERE row_id = 'indexer'`).Scan(&cursor); err != nil {
			return fmt.Errorf("retention: read index cursor: %w", err)
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT event_seq, COALESCE(run_id, ''), user_id, type, payload, ts
			   FROM run_events WHERE event_seq > ? ORDER BY event_seq LIMIT ?`, cursor, indexBatch)
		if err != nil {
			return fmt.Errorf("retention: read log for indexing: %w", err)
		}
		type row struct {
			seq                        int64
			runID, owner, typ, pay, ts string
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.seq, &r.runID, &r.owner, &r.typ, &r.pay, &r.ts); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(batch) == 0 {
			return nil
		}
		res.FromSeq = batch[0].seq

		for _, r := range batch {
			res.ThroughSeq = r.seq
			// Rollup: one row per (day, owner, type).
			day := r.ts
			if len(day) >= 10 {
				day = day[:10]
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO run_event_rollup (day, user_id, type, n) VALUES (?, ?, ?, 1)
				 ON CONFLICT (day, user_id, type) DO UPDATE SET n = n + 1`,
				day, r.owner, r.typ); err != nil {
				return fmt.Errorf("retention: roll up seq %d: %w", r.seq, err)
			}
			res.Rolled++

			kind, ok := indexedTypes[r.typ]
			if !ok {
				continue
			}
			body, err := s.searchBody(ctx, tx, kind, r.runID, r.pay)
			if err != nil {
				return err
			}
			if strings.TrimSpace(body) == "" {
				continue
			}
			ref := r.runID
			if ref == "" {
				ref = fmt.Sprintf("event:%d", r.seq)
			}
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM history_fts WHERE rowid = ?`, r.seq); err != nil {
				return fmt.Errorf("retention: clear fts row %d: %w", r.seq, err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO history_fts (rowid, body, kind, ref, user_id) VALUES (?, ?, ?, ?, ?)`,
				r.seq, body, kind, ref, r.owner); err != nil {
				return fmt.Errorf("retention: index seq %d: %w", r.seq, err)
			}
			res.Indexed++
		}

		now := s.now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx,
			`UPDATE retention_index_cursor SET last_seq = ?, updated_ts = ? WHERE row_id = 'indexer'`,
			res.ThroughSeq, now); err != nil {
			return fmt.Errorf("retention: advance index cursor: %w", err)
		}
		return nil
	})
	return res, err
}

// searchBody renders the indexable text of one row. A run summary's text is the
// SUMMARY ROW (objective + narrative + final state), not the event payload,
// which carries only refs; verdict and drift texts come from the payload.
func (s *Store) searchBody(ctx context.Context, tx *sql.Tx, kind, runID, payload string) (string, error) {
	switch kind {
	case KindRunSummary:
		var objective, narrative, finalState string
		err := tx.QueryRowContext(ctx,
			`SELECT objective, narrative, final_state FROM run_summaries WHERE run_id = ?`, runID).
			Scan(&objective, &narrative, &finalState)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil // the event exists without its row only mid-write; the next pass catches it
		}
		if err != nil {
			return "", fmt.Errorf("retention: read summary body %q: %w", runID, err)
		}
		return strings.TrimSpace(objective + "\n" + narrative + "\n" + finalState), nil
	case KindVerdict:
		return textFields(payload,
			"verdict", "rubric_id", "summary", "message", "findings", "failed", "unknown", "axis2_skipped"), nil
	case KindDrift:
		return textFields(payload,
			"summary", "change_class", "source", "severity", "lanes", "fingerprint"), nil
	}
	return "", nil
}

// textFields flattens the named payload fields into searchable text. Values are
// rendered as-is (strings, numbers, and the leaf strings of nested arrays and
// objects); an unparseable payload yields no text rather than an error — the
// index is a projection and a malformed row must never stop the pass.
func textFields(payload string, keys ...string) string {
	var m map[string]any
	if json.Unmarshal([]byte(payload), &m) != nil {
		return ""
	}
	var b strings.Builder
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		flatten(&b, v)
	}
	return strings.TrimSpace(b.String())
}

func flatten(b *strings.Builder, v any) {
	switch x := v.(type) {
	case string:
		b.WriteString(x)
		b.WriteByte(' ')
	case float64:
		fmt.Fprintf(b, "%v ", x)
	case bool:
		fmt.Fprintf(b, "%v ", x)
	case []any:
		for _, e := range x {
			flatten(b, e)
		}
	case map[string]any:
		for _, k := range sortedAnyKeys(x) {
			flatten(b, x[k])
		}
	}
}

func sortedAnyKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Deterministic order so the indexed text of one payload is stable.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// SearchHit is one FTS5 match.
type SearchHit struct {
	EventSeq int64
	Kind     string
	Ref      string
	UserID   string
}

// Search runs an FTS5 MATCH over the indexed history, owner-scoped per S01.9
// (an empty userID is the operator scope: every row, including platform-scope).
// It returns REFS only — the caller renders from the record, so no excerpt of a
// raw payload leaves this surface. Redact-before-match and excerpt bounding are
// the B5-8B search layer's obligation (CONVENTIONS §30); this is the index it
// searches over.
func (s *Store) Search(ctx context.Context, query, userID string, limit int) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT rowid, kind, ref, user_id FROM history_fts WHERE history_fts MATCH ?`
	args := []any{query}
	if userID != "" {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY rowid DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("retention: search %q: %w", query, err)
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.EventSeq, &h.Kind, &h.Ref, &h.UserID); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// RollupRow is one (day, owner, type) count.
type RollupRow struct {
	Day    string
	UserID string
	Type   string
	N      int64
}

// Rollup reads the derived event-count rollup, owner-scoped ("" = operator).
func (s *Store) Rollup(ctx context.Context, userID string) ([]RollupRow, error) {
	q := `SELECT day, user_id, type, n FROM run_event_rollup`
	var args []any
	if userID != "" {
		q += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY day, user_id, type`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("retention: read rollup: %w", err)
	}
	defer rows.Close()
	var out []RollupRow
	for rows.Next() {
		var r RollupRow
		if err := rows.Scan(&r.Day, &r.UserID, &r.Type, &r.N); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReconcileIndex repairs the one state a restore leaves behind: the S13.10 dump
// never carries FTS5 content (the shadow tables are deliberately not dumped —
// internal/storage/backup.go), while the cursor table IS an ordinary table and
// restores at its old position. A rebuilt database would therefore sit with an
// empty search index and a cursor saying everything is indexed.
//
// Called at boot. It rebuilds only when the index is EMPTY and the cursor is
// past zero — the exact signature of that restore, and a state normal operation
// cannot reach (the indexer writes the cursor and the FTS row in one
// transaction). Returns whether it rebuilt.
func (s *Store) ReconcileIndex(ctx context.Context) (bool, error) {
	var indexed, cursor int64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM history_fts`).Scan(&indexed); err != nil {
		return false, fmt.Errorf("retention: count indexed rows: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT last_seq FROM retention_index_cursor WHERE row_id = 'indexer'`).Scan(&cursor); err != nil {
		return false, fmt.Errorf("retention: read index cursor: %w", err)
	}
	if indexed > 0 || cursor == 0 {
		return false, nil
	}
	if _, err := s.Rebuild(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// Rebuild discards the derived index and replays it from the log — the property
// that makes it derived state rather than a system of record (S14.1). Nothing
// is lost by running it; nothing outside these two tables is touched.
func (s *Store) Rebuild(ctx context.Context) (IndexResult, error) {
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`DELETE FROM history_fts`,
			`DELETE FROM run_event_rollup`,
			`UPDATE retention_index_cursor SET last_seq = 0, updated_ts = ? WHERE row_id = 'indexer'`,
		} {
			var err error
			if strings.HasPrefix(stmt, "UPDATE") {
				_, err = tx.ExecContext(ctx, stmt, s.now().UTC().Format(time.RFC3339Nano))
			} else {
				_, err = tx.ExecContext(ctx, stmt)
			}
			if err != nil {
				return fmt.Errorf("retention: reset derived index: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return IndexResult{}, err
	}
	return s.Index(ctx)
}
