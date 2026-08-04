// Package eventlog is the append-only platform event log over the
// run_events table — the D7 event record, the observability substrate, and
// the platform's only audit truth (Spec S02.2, S01.11; journald is the ops
// log only). It is one of the fixed module seams of Spec S01.1.
//
// Ordering comes only from event_seq — never from timestamps; no single
// clock is trusted on a sleeping laptop (Spec S02.5, P-T07-4). Run-scoped
// appends carry the run's generation and the writer rejects appends whose
// generation does not match the run's current generation — the fencing that
// renders double-resume inert (Spec S02.5 step 4, P-T02-3). Payloads are
// validated before persist and capped at ⚙ state.event_payload_cap; bulk
// output lives as files referenced by path+hash, never inlined (Spec S02.1,
// P-T07-5).
package eventlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Settings is the eventlog-facing view of the settings registry (Spec
// S01.10): effective ⚙ values by dotted key.
type Settings interface {
	Int(key string) (int64, error)
}

// keyPayloadCap is the dotted ⚙ key for the payload cap (owned by Spec S02).
const keyPayloadCap = "state.event_payload_cap"

// Append rejection errors. Callers branch on these during recovery (stale
// fencing) and at the payload gate.
var (
	ErrUnknownRun                = errors.New("eventlog: unknown run")
	ErrStaleGeneration           = errors.New("eventlog: stale-generation append rejected (Spec S02.5)")
	ErrFutureGeneration          = errors.New("eventlog: generation ahead of the run's current generation")
	ErrPayloadTooLarge           = errors.New("eventlog: payload exceeds ⚙ state.event_payload_cap (P-T07-5)")
	ErrInvalidEvent              = errors.New("eventlog: invalid event")
	ErrInvalidPayload            = errors.New("eventlog: payload is not valid JSON")
	ErrGenerationOnPlatformEvent = errors.New("eventlog: platform-scope events carry no generation")
)

// Log appends to and reads from the platform event log.
type Log struct {
	db       *storage.DB
	settings Settings
}

// New returns a Log over db reading its ⚙ values from settings.
func New(db *storage.DB, settings Settings) *Log {
	return &Log{db: db, settings: settings}
}

// Append is one event to append. RunID "" is a platform-scope event
// (settings, auth, lifecycle); a run-scoped append must carry the run's
// current generation.
type Append struct {
	RunID         string
	Generation    int64
	UserID        string
	Type          string
	SchemaVersion int
	Payload       json.RawMessage
	// Time is recorded as the wall-clock ts and defaults to now. It is
	// never an ordering authority (Spec S02.5, P-T07-4).
	Time time.Time
}

// Event is one persisted event log row.
type Event struct {
	Seq           int64
	RunID         string // "" for platform-scope events
	Generation    int64  // 0 for platform-scope events
	UserID        string
	Type          string
	SchemaVersion int
	Payload       json.RawMessage
	Time          time.Time
}

// Append validates and appends one event in its own write transaction,
// returning its event_seq.
func (l *Log) Append(ctx context.Context, a Append) (int64, error) {
	var seq int64
	err := l.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		seq, err = l.AppendTx(ctx, tx, a)
		return err
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}

// AppendTx validates and appends one event inside the caller's write
// transaction, so state transitions and their event append commit
// atomically (Spec S02.3: every transition is a run_events append in the
// same transaction as the runs.state update).
func (l *Log) AppendTx(ctx context.Context, tx *sql.Tx, a Append) (int64, error) {
	if err := l.validate(a); err != nil {
		return 0, err
	}
	if a.RunID != "" {
		if err := checkGeneration(ctx, tx, a.RunID, a.Generation); err != nil {
			return 0, err
		}
	}
	ts := a.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	var runID, generation any
	if a.RunID != "" {
		runID, generation = a.RunID, a.Generation
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		runID, generation, a.UserID, a.Type, a.SchemaVersion, string(a.Payload),
		ts.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("eventlog: append: %w", err)
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("eventlog: append seq: %w", err)
	}
	return seq, nil
}

// validate is the validate-before-persist gate (Spec S02.1): never persist —
// and therefore never forward into a later turn — an unvalidated payload.
func (l *Log) validate(a Append) error {
	switch {
	case a.Type == "":
		return fmt.Errorf("%w: empty type", ErrInvalidEvent)
	case a.UserID == "":
		return fmt.Errorf("%w: empty user_id (owner attribution, 15.6)", ErrInvalidEvent)
	case a.SchemaVersion < 1:
		return fmt.Errorf("%w: schema_version %d < 1", ErrInvalidEvent, a.SchemaVersion)
	case a.RunID == "" && a.Generation != 0:
		return ErrGenerationOnPlatformEvent
	case a.RunID != "" && a.Generation < 0:
		return fmt.Errorf("%w: negative generation", ErrInvalidEvent)
	}
	if len(a.Payload) == 0 || !json.Valid(a.Payload) {
		return ErrInvalidPayload
	}
	cap, err := l.settings.Int(keyPayloadCap)
	if err != nil {
		return fmt.Errorf("eventlog: read ⚙ %s: %w", keyPayloadCap, err)
	}
	if int64(len(a.Payload)) > cap {
		return fmt.Errorf("%w: %d bytes > %d", ErrPayloadTooLarge, len(a.Payload), cap)
	}
	return nil
}

// checkGeneration enforces fencing against the run's authoritative
// generation counter on the runs row (Spec S02.2, S02.5 step 4).
func checkGeneration(ctx context.Context, tx *sql.Tx, runID string, generation int64) error {
	var current int64
	err := tx.QueryRowContext(ctx, `SELECT generation FROM runs WHERE run_id = ?`, runID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %q", ErrUnknownRun, runID)
	}
	if err != nil {
		return fmt.Errorf("eventlog: read run generation: %w", err)
	}
	switch {
	case generation < current:
		return fmt.Errorf("%w: append generation %d, run at %d", ErrStaleGeneration, generation, current)
	case generation > current:
		return fmt.Errorf("%w: append generation %d, run at %d", ErrFutureGeneration, generation, current)
	}
	return nil
}

// After returns up to limit events with event_seq > after, in event_seq
// order — the short-batch cursor read of Spec S02.1 (no long-lived read
// transactions).
func (l *Log) After(ctx context.Context, after int64, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("eventlog: non-positive read limit %d", limit)
	}
	rows, err := l.db.QueryContext(ctx,
		`SELECT event_seq, run_id, generation, user_id, type, schema_version, payload, ts
		 FROM run_events WHERE event_seq > ? ORDER BY event_seq LIMIT ?`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("eventlog: read after %d: %w", after, err)
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var (
			e          Event
			runID      sql.NullString
			generation sql.NullInt64
			payload    string
			ts         string
		)
		if err := rows.Scan(&e.Seq, &runID, &generation, &e.UserID, &e.Type, &e.SchemaVersion, &payload, &ts); err != nil {
			return nil, fmt.Errorf("eventlog: scan: %w", err)
		}
		e.RunID = runID.String
		e.Generation = generation.Int64
		e.Payload = json.RawMessage(payload)
		if e.Time, err = time.Parse(time.RFC3339Nano, ts); err != nil {
			return nil, fmt.Errorf("eventlog: parse ts %q: %w", ts, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Head returns the highest event_seq (0 when the log is empty) — the cursor
// bootstrap for tailing readers.
func (l *Log) Head(ctx context.Context) (int64, error) {
	var head sql.NullInt64
	err := l.db.QueryRowContext(ctx, `SELECT MAX(event_seq) FROM run_events`).Scan(&head)
	if err != nil {
		return 0, fmt.Errorf("eventlog: head: %w", err)
	}
	return head.Int64, nil
}
