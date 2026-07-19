package gates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Checkpoint-per-paid-call (Spec S02.4, D7): a checkpoint row MUST be
// written after every paid model call, in the same transaction as its
// run-event append, so re-spend after any disaster is bounded structurally
// to "work since the last paid call". The platform checkpoint row is
// authoritative — engine transcripts are NEVER durable checkpoints
// (P-T01-1); the transcript copy-aside and its engine_sessions indexing are
// the adapter's duty at each checkpoint (B1, Spec S03).

// EventCheckpoint is the run-event type appended with every checkpoint row
// (Spec S02.3: "a paid call appends a checkpoint event and stays running").
// Name provisional pending the S14 event contract (B5).
const EventCheckpoint = "run.checkpoint"

// checkpointEventSchemaVersion versions the run.checkpoint payload.
const checkpointEventSchemaVersion = 1

// Checkpoint write errors.
var (
	// ErrNotCheckpointable rejects a checkpoint on a run that is not in a
	// paid-call state: a paid call happens while running, or while draining
	// to its checkpoint boundary (Spec S02.3, S02.4).
	ErrNotCheckpointable = errors.New("gates: run is not in a checkpointable state (running/draining)")
	// ErrRunNotFound is returned when the run row does not exist.
	ErrRunNotFound = errors.New("gates: run not found")
)

// NewCheckpoint carries the five ratified blocks of Spec S02.4, identical
// across substrates (measured schema parity).
type NewCheckpoint struct {
	RunID string

	// (a) usage block — input/output tokens, cache_read/cache_creation by
	// TTL bucket, cost fields. Stored as validated JSON.
	Usage json.RawMessage

	// (b) session cursor — substrate, engine session id AS REPORTED by the
	// engine's system/init (never the requested id), message index, cwd
	// key, transcript path.
	SessionSubstrate string
	SessionID        string
	MessageIndex     int64
	CwdKey           string
	TranscriptPath   string

	// (c) Ledger revision — content or content-hash (Spec S05 owns the
	// internal schema).
	LedgerRevision string

	// (d) artifact snapshot ref — native per-step snapshot id (opencode
	// lane) or platform-owned snapshot commit (Claude lane).
	ArtifactSnapshotRef string

	// (e) version fields — for the freshness pass (Spec S02.6).
	ModelID               string
	InvocationFingerprint string
	ToolSchemaVersion     string
	PromptSchemaVersion   string
}

// Checkpoint is one persisted checkpoints row.
type Checkpoint struct {
	ID       int64
	RunID    string
	UserID   string
	EventSeq int64

	Usage json.RawMessage

	SessionSubstrate string
	SessionID        string
	MessageIndex     int64
	CwdKey           string
	TranscriptPath   string

	LedgerRevision      string
	ArtifactSnapshotRef string

	ModelID               string
	InvocationFingerprint string
	ToolSchemaVersion     string
	PromptSchemaVersion   string

	CreatedTS time.Time
}

// Checkpoints is the checkpoint write/read API over the checkpoints table.
type Checkpoints struct {
	db  *storage.DB
	log *eventlog.Log
}

// NewCheckpoints returns a Checkpoints store appending through log.
func NewCheckpoints(db *storage.DB, log *eventlog.Log) *Checkpoints {
	return &Checkpoints{db: db, log: log}
}

// Write persists one checkpoint: the run.checkpoint event append and the
// checkpoints row commit in one write transaction (Spec S02.4).
func (c *Checkpoints) Write(ctx context.Context, n NewCheckpoint) (Checkpoint, error) {
	var cp Checkpoint
	err := c.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		cp, err = c.WriteTx(ctx, tx, n)
		return err
	})
	return cp, err
}

// WriteTx is Write inside the caller's transaction, so a caller can commit
// the checkpoint atomically with an FSM transition (Spec S02.7 guarantee
// vocabulary: journal/state writes are exactly-once).
func (c *Checkpoints) WriteTx(ctx context.Context, tx *sql.Tx, n NewCheckpoint) (Checkpoint, error) {
	if n.RunID == "" {
		return Checkpoint{}, errors.New("gates: checkpoint without run_id")
	}
	if len(n.Usage) == 0 {
		n.Usage = json.RawMessage("{}")
	}
	if !json.Valid(n.Usage) {
		return Checkpoint{}, errors.New("gates: checkpoint usage block is not valid JSON")
	}
	var (
		userID, state string
		generation    int64
	)
	err := tx.QueryRowContext(ctx,
		`SELECT user_id, state, generation FROM runs WHERE run_id = ?`, n.RunID).
		Scan(&userID, &state, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return Checkpoint{}, fmt.Errorf("%w: %q", ErrRunNotFound, n.RunID)
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("gates: read run %q: %w", n.RunID, err)
	}
	if state != "running" && state != "draining" {
		return Checkpoint{}, fmt.Errorf("%w: %q is %s", ErrNotCheckpointable, n.RunID, state)
	}

	// Event first (the row references its event_seq). Payload stays small —
	// refs, not blobs (P-T07-5): the row holds the full blocks.
	payload, err := json.Marshal(struct {
		ModelID             string `json:"model_id,omitempty"`
		SessionID           string `json:"session_id,omitempty"`
		MessageIndex        int64  `json:"message_index,omitempty"`
		ArtifactSnapshotRef string `json:"artifact_snapshot_ref,omitempty"`
	}{n.ModelID, n.SessionID, n.MessageIndex, n.ArtifactSnapshotRef})
	if err != nil {
		return Checkpoint{}, fmt.Errorf("gates: marshal checkpoint payload: %w", err)
	}
	seq, err := c.log.AppendTx(ctx, tx, eventlog.Append{
		RunID:         n.RunID,
		Generation:    generation,
		UserID:        userID,
		Type:          EventCheckpoint,
		SchemaVersion: checkpointEventSchemaVersion,
		Payload:       payload,
	})
	if err != nil {
		return Checkpoint{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO checkpoints (run_id, user_id, event_seq, usage_json,
		     session_substrate, session_id, message_index, cwd_key, transcript_path,
		     ledger_revision, artifact_snapshot_ref,
		     model_id, invocation_fingerprint, tool_schema_version, prompt_schema_version,
		     created_ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.RunID, userID, seq, string(n.Usage),
		n.SessionSubstrate, n.SessionID, n.MessageIndex, n.CwdKey, n.TranscriptPath,
		n.LedgerRevision, n.ArtifactSnapshotRef,
		n.ModelID, n.InvocationFingerprint, n.ToolSchemaVersion, n.PromptSchemaVersion,
		now)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("gates: insert checkpoint: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Checkpoint{}, fmt.Errorf("gates: checkpoint id: %w", err)
	}
	return c.getTx(ctx, tx, id)
}

// Last returns the run's most recent checkpoint by event order (event_seq
// is the sole ordering authority, Spec S02.5) — the fork-from-checkpoint
// and freshness input.
func (c *Checkpoints) Last(ctx context.Context, runID string) (Checkpoint, bool, error) {
	cp, err := scanCheckpoint(c.db.QueryRowContext(ctx,
		selectCheckpoints+` WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return Checkpoint{}, false, nil
	}
	if err != nil {
		return Checkpoint{}, false, err
	}
	return cp, true, nil
}

// LastTx is Last inside the caller's transaction.
func (c *Checkpoints) LastTx(ctx context.Context, tx *sql.Tx, runID string) (Checkpoint, bool, error) {
	cp, err := scanCheckpoint(tx.QueryRowContext(ctx,
		selectCheckpoints+` WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return Checkpoint{}, false, nil
	}
	if err != nil {
		return Checkpoint{}, false, err
	}
	return cp, true, nil
}

func (c *Checkpoints) getTx(ctx context.Context, tx *sql.Tx, id int64) (Checkpoint, error) {
	cp, err := scanCheckpoint(tx.QueryRowContext(ctx, selectCheckpoints+` WHERE checkpoint_id = ?`, id))
	if err != nil {
		return Checkpoint{}, fmt.Errorf("gates: read checkpoint %d: %w", id, err)
	}
	return cp, nil
}

const selectCheckpoints = `SELECT checkpoint_id, run_id, user_id, event_seq, usage_json,
       session_substrate, session_id, message_index, cwd_key, transcript_path,
       ledger_revision, artifact_snapshot_ref,
       model_id, invocation_fingerprint, tool_schema_version, prompt_schema_version,
       created_ts
  FROM checkpoints`

type rowScanner interface{ Scan(dest ...any) error }

func scanCheckpoint(row rowScanner) (Checkpoint, error) {
	var (
		cp           Checkpoint
		usage        string
		messageIndex sql.NullInt64
		createdTS    string
	)
	err := row.Scan(&cp.ID, &cp.RunID, &cp.UserID, &cp.EventSeq, &usage,
		&cp.SessionSubstrate, &cp.SessionID, &messageIndex, &cp.CwdKey, &cp.TranscriptPath,
		&cp.LedgerRevision, &cp.ArtifactSnapshotRef,
		&cp.ModelID, &cp.InvocationFingerprint, &cp.ToolSchemaVersion, &cp.PromptSchemaVersion,
		&createdTS)
	if err != nil {
		return Checkpoint{}, err
	}
	cp.Usage = json.RawMessage(usage)
	cp.MessageIndex = messageIndex.Int64
	if cp.CreatedTS, err = time.Parse(time.RFC3339Nano, createdTS); err != nil {
		return Checkpoint{}, fmt.Errorf("gates: parse checkpoint created_ts %q: %w", createdTS, err)
	}
	return cp, nil
}
