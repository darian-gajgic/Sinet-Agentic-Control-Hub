// Package run implements the run-lifecycle FSM of Spec S02.3 over the runs
// table: the ratified stored states, the ratified transition set, and the
// storage discipline that every transition is a run_events append in the
// same transaction as the runs.state update.
//
// Stalled and wedged are NEVER stored — they are derived classifications
// computed at reconcile time from unit-liveness input and event-cursor
// advance against ⚙ recovery.dead_after (Spec S02.3; the reconcile lives in
// internal/recovery). A stored stall would go stale across a nap and
// mislabel a live worker.
//
// Generation is the per-run monotonic fencing counter, bumped on every
// takeover or resume and stamped into every event append (Spec S02
// vocabulary, S02.5 step 4): the resume edge parked→running bumps it here;
// the takeover bump at fork-from-checkpoint is composed by
// internal/recovery. The event-log writer rejects appends whose generation
// does not match the run's current one, which is what renders double-resume
// inert (P-T02-3).
package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// State is a stored FSM state (Spec S02.3). The runs.state CHECK constraint
// carries exactly this set.
type State string

// The ratified stored states (Spec S02.3).
const (
	StateNew        State = "new"          // created, not yet admitted
	StateQueued     State = "queued"       // admitted to the claim queue
	StateClaimed    State = "claimed"      // CAS-claimed, lease held
	StateRunning    State = "running"      // engine subprocess live, cursor advancing
	StateParked     State = "parked"       // suspended on a limit event or open gate/ask
	StateDraining   State = "draining"     // maintenance: finishing the current paid call to a checkpoint boundary
	StateCompleted  State = "completed"    // terminal
	StateCrashed    State = "crashed"      // terminal, supersedable by recovery (harvest / fork / bound)
	StateFinalized  State = "finalized"    // terminal: finalize-with-card on stale resume
	StateTombstoned State = "tombstoned"   // terminal: repeat-offender wedged/crashed run
	StateDiedAtGate State = "died-at-gate" // terminal: budget/ceiling preempted a park before resume
)

// transitions is the ratified edge set. Every edge cites its Spec S02
// anchor; edges marked "implied" are clearly-implied readings recorded in
// the P3-B0-4 packet report.
var transitions = map[State][]State{
	StateNew:    {StateQueued},  // S02.3: new→queued (admitted)
	StateQueued: {StateClaimed}, // S02.3: queued→claimed (CAS-claimed, lease held)
	StateClaimed: {
		StateRunning, // S02.3: claimed→running (engine subprocess live)
		StateCrashed, // implied by S02.5 step 2: a claimed run whose lease died with no unit is DEAD → "mark crashed"
	},
	StateRunning: {
		StateParked,     // S02.3: running→parked (limit event or open gate/ask)
		StateDraining,   // S02.3: running→draining (maintenance)
		StateCompleted,  // S02.3: running→completed (incl. harvest of finished-during-outage)
		StateCrashed,    // S02.3: running→crashed
		StateTombstoned, // S02.3: "a repeat-offender wedged run → tombstoned"
	},
	StateDraining: {
		StateParked,    // S02.3: draining→parked (drain reached its checkpoint boundary)
		StateCompleted, // implied by S02.5 step 2: FINISHED-DURING-OUTAGE harvest of a draining run's exit-0 corpse
		StateCrashed,   // implied by S02.5 step 2: DEAD mid-drain
	},
	StateParked: {
		StateRunning,    // S02.3: resume (provider signal, schedule, or approval) — bumps generation
		StateDiedAtGate, // S02.3: a budget/ceiling preempts the park before it can resume
		StateFinalized,  // S02.3: finalize-with-card on stale resume (S02.6 escalation terminal)
	},
	StateCrashed: {
		StateCompleted,  // S02.3: "{crashed | finished-during-outage}→completed via harvest"
		StateTombstoned, // S02.5 step 3: "repeat offenders are tombstoned"
		StateFinalized,  // S02.5 step 3: interrupted > ⚙ recovery.stale_finalize → finalized-with-card
	},
	StateCompleted:  {},
	StateFinalized:  {},
	StateTombstoned: {},
	StateDiedAtGate: {},
}

// generationBumps lists the edges that are a resume in the Spec S02 sense
// ("generation … bumped on every takeover or resume"). The takeover bump at
// fork is internal/recovery's, on the crashed row.
var generationBumps = map[[2]State]bool{
	{StateParked, StateRunning}: true,
}

// CanTransition reports whether from→to is a ratified edge (Spec S02.3).
func CanTransition(from, to State) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// IsTerminal reports whether s admits no further transitions. StateCrashed
// is deliberately NOT terminal in this sense: Spec S02.3 lists it as a
// terminal of the live run, but recovery supersedes it (harvest, fork,
// tombstone, finalize).
func IsTerminal(s State) bool { return len(transitions[s]) == 0 }

// Event types minted by the run machinery. Names are provisional pending
// the S14 event contract (B5), matching the B0-3 precedent for the platform
// lifecycle events.
const (
	// EventCreated is a run row coming into existence (initial state, no
	// prior state). Appended in the same transaction as the INSERT so the
	// run's existence is in the D7 event record from its first instant.
	EventCreated = "run.created"
	// EventState is a stored-state transition: payload {from, to, reason,
	// actor, detail}. One append per transition, same transaction as the
	// runs.state update (Spec S02.3).
	EventState = "run.state"
	// EventForked is the birth event of a fork-from-checkpoint successor
	// (Spec S02.5 step 2), appended by internal/recovery.
	EventForked = "run.forked"
	// EventWedged flags a derived WEDGED classification (pause-and-flag,
	// never auto-kill — Spec S02.5 step 2), appended by internal/recovery.
	EventWedged = "run.wedged"
	// EventHarvest is one mined engine-store record appended during
	// finished-during-outage harvest, deduplicated by (session_id,
	// message_id) (Spec S02.5 step 2), appended by internal/recovery.
	EventHarvest = "run.harvest"
)

// ActorPlatform attributes acts of the platform itself (the recovery
// ladder, the shell) inside event payloads. Run-scoped event rows are
// owner-attributed to the run's owner per 15.6; the acting principal rides
// the payload. Value-identical to shell.PlatformUserID.
const ActorPlatform = "platform"

// stateEventSchemaVersion versions the run.* event payloads.
const stateEventSchemaVersion = 1

// Errors callers branch on.
var (
	ErrNotFound          = errors.New("run: not found")
	ErrInvalidTransition = errors.New("run: transition not in the ratified S02.3 edge set")
	ErrExists            = errors.New("run: run_id already exists")
)

// Run is one runs row (Spec S02.2 runs family + the 0002 lineage columns).
type Run struct {
	ID     string
	UserID string
	TaskID string
	State  State

	Substrate string
	Lane      string

	// Lease block (Spec S02.2): wall-clock deadline evaluated suspend-aware
	// at reconcile (Spec S02.5 step 4); never an ordering authority.
	LeaseHolder   string
	LeaseDeadline time.Time // zero = no lease
	HeartbeatSeq  int64     // heartbeat cursor: last event_seq the holder asserted

	Generation   int64
	UnitName     string
	WorkspaceRef string

	// Ceilings (3.7): zero = unset at B0.
	CeilingTimeS   int64
	CeilingSteps   int64
	CeilingCostUSD float64

	// Recovery lineage (migration 0002; Spec S02.5 step 3).
	ParentRunID      string
	DispatchID       string
	RecoveryAttempts int64

	CreatedTS time.Time
	UpdatedTS time.Time
}

// Store reads and writes runs rows under the S02.3 discipline. Safe for
// concurrent use (the underlying pool serializes writes, Spec S02.1).
type Store struct {
	db  *storage.DB
	log *eventlog.Log
}

// NewStore returns a Store over db appending through log.
func NewStore(db *storage.DB, log *eventlog.Log) *Store {
	return &Store{db: db, log: log}
}

// NewRun describes a run row to create. State defaults to StateNew and
// Generation to 0; recovery sets the lineage fields when creating a
// fork-from-checkpoint successor (Spec S02.5 step 2).
type NewRun struct {
	ID        string
	UserID    string
	TaskID    string
	Substrate string
	Lane      string

	WorkspaceRef   string
	CeilingTimeS   int64
	CeilingSteps   int64
	CeilingCostUSD float64

	State      State
	Generation int64

	ParentRunID      string
	DispatchID       string
	RecoveryAttempts int64
}

// Create inserts the run row and appends its birth event in one write
// transaction.
func (s *Store) Create(ctx context.Context, n NewRun) (Run, error) {
	var r Run
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		r, err = s.CreateTx(ctx, tx, n, EventCreated, nil)
		return err
	})
	return r, err
}

// CreateTx inserts the run row and appends its birth event (eventType, with
// an optional payload detail object) inside the caller's transaction, so a
// recovery fork commits atomically with the crash it supersedes.
func (s *Store) CreateTx(ctx context.Context, tx *sql.Tx, n NewRun, eventType string, detail json.RawMessage) (Run, error) {
	switch {
	case n.ID == "":
		return Run{}, errors.New("run: empty run_id")
	case n.UserID == "":
		return Run{}, errors.New("run: empty user_id (owner attribution, 15.6)")
	}
	if n.State == "" {
		n.State = StateNew
	}
	if _, ok := transitions[n.State]; !ok {
		return Run{}, fmt.Errorf("run: %q is not a stored S02.3 state", n.State)
	}
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO runs (run_id, user_id, task_id, state, substrate, lane,
		                   generation, workspace_ref,
		                   ceiling_time_s, ceiling_steps, ceiling_cost_usd,
		                   parent_run_id, dispatch_id, recovery_attempts,
		                   created_ts, updated_ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.UserID, nullString(n.TaskID), string(n.State), n.Substrate, n.Lane,
		n.Generation, nullString(n.WorkspaceRef),
		nullInt(n.CeilingTimeS), nullInt(n.CeilingSteps), nullFloat(n.CeilingCostUSD),
		nullString(n.ParentRunID), nullString(n.DispatchID), n.RecoveryAttempts,
		ts, ts)
	if err != nil {
		if isConstraint(err) {
			return Run{}, fmt.Errorf("%w: %q", ErrExists, n.ID)
		}
		return Run{}, fmt.Errorf("run: create %q: %w", n.ID, err)
	}
	payload, err := json.Marshal(struct {
		State  string          `json:"state"`
		Detail json.RawMessage `json:"detail,omitempty"`
	}{State: string(n.State), Detail: detail})
	if err != nil {
		return Run{}, fmt.Errorf("run: marshal %s payload: %w", eventType, err)
	}
	if _, err := s.log.AppendTx(ctx, tx, eventlog.Append{
		RunID:         n.ID,
		Generation:    n.Generation,
		UserID:        n.UserID,
		Type:          eventType,
		SchemaVersion: stateEventSchemaVersion,
		Payload:       payload,
	}); err != nil {
		return Run{}, err
	}
	return s.getTx(ctx, tx, n.ID)
}

// TransitionOptions annotate one transition's event payload.
type TransitionOptions struct {
	Reason string
	Actor  string          // acting principal; the row's user_id stays the run owner (15.6)
	Detail json.RawMessage // optional JSON object
}

// Transition moves the run to state to in one write transaction: the
// runs.state update and the run_events append commit atomically (Spec
// S02.3). The resume edge parked→running bumps the generation and appends
// at the new one (Spec S02.5 step 4).
func (s *Store) Transition(ctx context.Context, runID string, to State, opts TransitionOptions) (Run, error) {
	var r Run
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		r, err = s.TransitionTx(ctx, tx, runID, to, opts)
		return err
	})
	return r, err
}

// TransitionTx is Transition inside the caller's transaction, for composing
// multi-step recovery decisions atomically (Spec S02.5 step 2).
func (s *Store) TransitionTx(ctx context.Context, tx *sql.Tx, runID string, to State, opts TransitionOptions) (Run, error) {
	cur, err := s.getTx(ctx, tx, runID)
	if err != nil {
		return Run{}, err
	}
	if !CanTransition(cur.State, to) {
		return Run{}, fmt.Errorf("%w: %s→%s (run %q)", ErrInvalidTransition, cur.State, to, runID)
	}
	gen := cur.Generation
	if generationBumps[[2]State{cur.State, to}] {
		gen++ // resume takeover fence (Spec S02.5 step 4)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx,
		`UPDATE runs SET state = ?, generation = ?, updated_ts = ?
		 WHERE run_id = ? AND state = ? AND generation = ?`,
		string(to), gen, now, runID, string(cur.State), cur.Generation)
	if err != nil {
		return Run{}, fmt.Errorf("run: transition %q %s→%s: %w", runID, cur.State, to, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Run{}, fmt.Errorf("run: transition %q: %w", runID, err)
	} else if n != 1 {
		// Unreachable under the single-writer posture (Spec S02.1); kept as
		// a guard against a concurrent-writer defect.
		return Run{}, fmt.Errorf("run: transition %q %s→%s: concurrent state change", runID, cur.State, to)
	}
	payload, err := json.Marshal(struct {
		From   string          `json:"from"`
		To     string          `json:"to"`
		Reason string          `json:"reason,omitempty"`
		Actor  string          `json:"actor,omitempty"`
		Detail json.RawMessage `json:"detail,omitempty"`
	}{From: string(cur.State), To: string(to), Reason: opts.Reason, Actor: opts.Actor, Detail: opts.Detail})
	if err != nil {
		return Run{}, fmt.Errorf("run: marshal transition payload: %w", err)
	}
	if _, err := s.log.AppendTx(ctx, tx, eventlog.Append{
		RunID:         runID,
		Generation:    gen,
		UserID:        cur.UserID,
		Type:          EventState,
		SchemaVersion: stateEventSchemaVersion,
		Payload:       payload,
	}); err != nil {
		return Run{}, err
	}
	return s.getTx(ctx, tx, runID)
}

// SetLeaseTx sets the run's lease block (holder, wall-clock deadline,
// heartbeat cursor) inside the caller's transaction — the S02.2 lease the
// scheduler establishes when it CAS-claims a run (Spec S10.7: "claimed,
// lease held"). The lease is a set of wall-clock columns, not a state change,
// so it carries no run_events append; the queued→claimed transition (which
// does) composes with it in the same claim transaction. Deadlines are
// evaluated suspend-aware at reconcile (Spec S02.5 step 4); a zero deadline
// clears the lease.
func (s *Store) SetLeaseTx(ctx context.Context, tx *sql.Tx, runID, holder string, deadline time.Time, heartbeatSeq int64) error {
	var (
		deadlineArg any
		holderArg   any
		hbArg       any
	)
	if holder != "" {
		holderArg = holder
	}
	if !deadline.IsZero() {
		deadlineArg = deadline.UTC().Format(time.RFC3339Nano)
	}
	if heartbeatSeq > 0 {
		hbArg = heartbeatSeq
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx,
		`UPDATE runs SET lease_holder = ?, lease_deadline_ts = ?, heartbeat_event_seq = ?, updated_ts = ?
		 WHERE run_id = ?`,
		holderArg, deadlineArg, hbArg, now, runID)
	if err != nil {
		return fmt.Errorf("run: set lease %q: %w", runID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("run: set lease %q: %w", runID, err)
	} else if n != 1 {
		return fmt.Errorf("%w: %q", ErrNotFound, runID)
	}
	return nil
}

// BumpGenerationTx bumps the run's fencing counter without a state change —
// the takeover fence at fork-from-checkpoint: after the bump, a zombie
// holder of the previous generation is stale and its appends are rejected
// (Spec S02.5 step 4, P-T02-3). The caller (internal/recovery) composes it
// with the crash transition and the successor creation in one transaction.
func (s *Store) BumpGenerationTx(ctx context.Context, tx *sql.Tx, runID string) (Run, error) {
	cur, err := s.getTx(ctx, tx, runID)
	if err != nil {
		return Run{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		`UPDATE runs SET generation = ?, updated_ts = ? WHERE run_id = ? AND generation = ?`,
		cur.Generation+1, now, runID, cur.Generation); err != nil {
		return Run{}, fmt.Errorf("run: bump generation %q: %w", runID, err)
	}
	return s.getTx(ctx, tx, runID)
}

// Get returns one run.
func (s *Store) Get(ctx context.Context, runID string) (Run, error) {
	row := s.db.QueryRowContext(ctx, selectRuns+` WHERE run_id = ?`, runID)
	return scanRun(row)
}

// GetTx returns one run inside the caller's transaction.
func (s *Store) GetTx(ctx context.Context, tx *sql.Tx, runID string) (Run, error) {
	return s.getTx(ctx, tx, runID)
}

func (s *Store) getTx(ctx context.Context, tx *sql.Tx, runID string) (Run, error) {
	return scanRun(tx.QueryRowContext(ctx, selectRuns+` WHERE run_id = ?`, runID))
}

// InStates returns every run whose state is in states, ordered by run_id
// for determinism.
func (s *Store) InStates(ctx context.Context, states ...State) ([]Run, error) {
	if len(states) == 0 {
		return nil, nil
	}
	q := selectRuns + ` WHERE state IN (?` + strings.Repeat(",?", len(states)-1) + `) ORDER BY run_id`
	args := make([]any, len(states))
	for i, st := range states {
		args[i] = string(st)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("run: list states: %w", err)
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SuccessorByDispatchTx returns the run created under dispatchID, if any —
// the durable one-dispatch-per-interruption lookup (Spec S02.5 step 3).
func (s *Store) SuccessorByDispatchTx(ctx context.Context, tx *sql.Tx, dispatchID string) (Run, bool, error) {
	r, err := scanRun(tx.QueryRowContext(ctx, selectRuns+` WHERE dispatch_id = ?`, dispatchID))
	if errors.Is(err, ErrNotFound) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	return r, true, nil
}

// SuccessorOfTx returns the fork successor of parentRunID, if any — the
// supersession lineage lookup (Spec S02.3 "superseded by
// fork-from-last-checkpoint as a new run").
func (s *Store) SuccessorOfTx(ctx context.Context, tx *sql.Tx, parentRunID string) (Run, bool, error) {
	r, err := scanRun(tx.QueryRowContext(ctx, selectRuns+` WHERE parent_run_id = ? LIMIT 1`, parentRunID))
	if errors.Is(err, ErrNotFound) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	return r, true, nil
}

const selectRuns = `SELECT run_id, user_id, task_id, state, substrate, lane,
       lease_holder, lease_deadline_ts, heartbeat_event_seq,
       generation, unit_name, workspace_ref,
       ceiling_time_s, ceiling_steps, ceiling_cost_usd,
       parent_run_id, dispatch_id, recovery_attempts,
       created_ts, updated_ts
  FROM runs`

type rowScanner interface{ Scan(dest ...any) error }

func scanRun(row rowScanner) (Run, error) {
	var (
		r                                  Run
		taskID, leaseHolder, leaseDeadline sql.NullString
		heartbeatSeq                       sql.NullInt64
		unitName, workspaceRef             sql.NullString
		ceilTime, ceilSteps                sql.NullInt64
		ceilCost                           sql.NullFloat64
		parentRunID, dispatchID            sql.NullString
		state, createdTS, updatedTS        string
	)
	err := row.Scan(&r.ID, &r.UserID, &taskID, &state, &r.Substrate, &r.Lane,
		&leaseHolder, &leaseDeadline, &heartbeatSeq,
		&r.Generation, &unitName, &workspaceRef,
		&ceilTime, &ceilSteps, &ceilCost,
		&parentRunID, &dispatchID, &r.RecoveryAttempts,
		&createdTS, &updatedTS)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("run: scan: %w", err)
	}
	r.TaskID = taskID.String
	r.State = State(state)
	r.LeaseHolder = leaseHolder.String
	r.HeartbeatSeq = heartbeatSeq.Int64
	r.UnitName = unitName.String
	r.WorkspaceRef = workspaceRef.String
	r.CeilingTimeS = ceilTime.Int64
	r.CeilingSteps = ceilSteps.Int64
	r.CeilingCostUSD = ceilCost.Float64
	r.ParentRunID = parentRunID.String
	r.DispatchID = dispatchID.String
	if leaseDeadline.Valid && leaseDeadline.String != "" {
		if r.LeaseDeadline, err = time.Parse(time.RFC3339Nano, leaseDeadline.String); err != nil {
			return Run{}, fmt.Errorf("run: parse lease deadline %q: %w", leaseDeadline.String, err)
		}
	}
	if r.CreatedTS, err = time.Parse(time.RFC3339Nano, createdTS); err != nil {
		return Run{}, fmt.Errorf("run: parse created_ts %q: %w", createdTS, err)
	}
	if r.UpdatedTS, err = time.Parse(time.RFC3339Nano, updatedTS); err != nil {
		return Run{}, fmt.Errorf("run: parse updated_ts %q: %w", updatedTS, err)
	}
	return r, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

// isConstraint reports whether err is a SQLite constraint violation. The
// driver is behind the storage seam, so this matches on the stable SQLite
// error text rather than importing driver types here (Spec S01.3: only
// internal/storage imports the driver).
func isConstraint(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "constraint failed") ||
		strings.Contains(err.Error(), "UNIQUE"))
}
