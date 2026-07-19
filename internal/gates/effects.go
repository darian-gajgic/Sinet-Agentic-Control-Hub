package gates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The two-phase effect journal (Spec S02.7): exactly-once outward effects.
// Journal/state writes are exactly-once (same transaction as the FSM
// transition — callers compose via the Tx variants); provider calls are
// at-least-once (classes A–C) or at-most-once (class D); dedup lives at the
// effect boundary. The `executing` row is written in its own transaction
// BEFORE the provider call, so a crash anywhere in the window leaves an
// in-doubt row the recovery ladder resolves per class (Spec S02.5 step 6).

// Class is the ratified idempotency class (Spec S02.7 table).
type Class string

const (
	// ClassA is natively idempotent (e.g. git push --force-with-lease):
	// blind retry is safe across the crash window.
	ClassA Class = "A"
	// ClassB is key-dedupable (Stripe-class APIs, Resend-class email):
	// retry with the stored idempotency key, respecting the provider's
	// window (the per-provider registry is data, not code — B1+).
	ClassB Class = "B"
	// ClassC is query-before-retry (GitHub comments/issues): the executor
	// embeds the deterministic effect-id marker in the provider-visible
	// payload and searches before any retry.
	ClassC Class = "C"
	// ClassD has no idempotency (plain SMTP): at-most-once execution; the
	// crash window flips to unknown + a decision card (P-T07-3). Admissible
	// only as an explicit per-channel exception (G2 D2.4).
	ClassD Class = "D"
)

func validClass(c Class) bool {
	switch c {
	case ClassA, ClassB, ClassC, ClassD:
		return true
	}
	return false
}

// EffectState is a stored journal state (Spec S02.7 lifecycle).
type EffectState string

const (
	EffectProposed  EffectState = "proposed"  // created by the gate; payload normalized + hashed
	EffectApproved  EffectState = "approved"  // approver + timestamp recorded, payload_hash pinned
	EffectExecuting EffectState = "executing" // written BEFORE the provider call; in-doubt if found at recovery
	EffectSucceeded EffectState = "succeeded"
	EffectFailed    EffectState = "failed"
	// EffectUnknown is a first-class terminal: the provider outcome is
	// unknowable (class D crash window); a human decides from the card
	// (5.6, P-T07-3).
	EffectUnknown EffectState = "unknown"
)

// Journal errors callers branch on.
var (
	ErrEffectNotFound = errors.New("gates: effect not found")
	// ErrBadState rejects a lifecycle call against a row not in the
	// required state (e.g. approving an executing effect).
	ErrBadState = errors.New("gates: effect is not in the required state")
	// ErrApprovalExpired: the approval outlived ⚙ effects.approval_expiry;
	// Terraform saved-plan semantics send the effect back to re-approval
	// (Spec S02.7).
	ErrApprovalExpired = errors.New("gates: approval expired — effect returned to proposed for re-approval")
	// ErrPayloadDrift: the stored payload no longer matches the pinned
	// payload_hash; drift sends the effect back to re-approval (Spec S02.7).
	ErrPayloadDrift = errors.New("gates: payload drift against pinned hash — effect returned to proposed for re-approval")
)

// keyApprovalExpiry is the ⚙ approval-expiry window (G2 Def.2, Spec S02.7).
const keyApprovalExpiry = "effects.approval_expiry"

// Settings is the journal's view of the settings registry.
type Settings interface {
	Duration(key string) (time.Duration, error)
}

// Effect is one effects row.
type Effect struct {
	ID     string
	RunID  string // "" when not attributed to a run
	UserID string
	Class  Class

	Payload     json.RawMessage // stored in canonical form (see canonicalJSON)
	PayloadHash string

	State      EffectState
	ApprovedBy string
	ApprovedTS time.Time // zero when unapproved

	IdempotencyKey    string // = the effect UUID, set at first execution, stable across retries
	ProviderWindowRef string
	Attempts          int64
	Result            json.RawMessage // outcome record, or the unknown-card content

	CreatedTS time.Time
	UpdatedTS time.Time
}

// Journal is the two-phase effect journal over the effects table.
type Journal struct {
	db       *storage.DB
	settings Settings
	now      func() time.Time
}

// JournalConfig configures a Journal. Now defaults to time.Now; tests
// inject a fake clock for the approval-expiry window.
type JournalConfig struct {
	DB       *storage.DB
	Settings Settings
	Now      func() time.Time
}

// NewJournal returns the effect journal.
func NewJournal(cfg JournalConfig) (*Journal, error) {
	if cfg.DB == nil {
		return nil, errors.New("gates: nil DB")
	}
	if cfg.Settings == nil {
		return nil, errors.New("gates: nil settings registry")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Journal{db: cfg.DB, settings: cfg.Settings, now: cfg.Now}, nil
}

// Proposal is a structured action request entering the gate (the gh-aw
// safe-outputs shape, Spec S16.5): the engine proposes; it never executes.
type Proposal struct {
	RunID             string // optional attribution to a run
	UserID            string
	Class             Class
	Payload           json.RawMessage
	ProviderWindowRef string // per-provider idempotency-window ref (registry data, B1+)
}

// Propose journals a proposed effect: payload normalized + hashed, state
// proposed (Spec S02.7).
func (j *Journal) Propose(ctx context.Context, p Proposal) (Effect, error) {
	var e Effect
	err := j.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		e, err = j.ProposeTx(ctx, tx, p)
		return err
	})
	return e, err
}

// ProposeTx is Propose inside the caller's transaction, so a proposal can
// commit atomically with the FSM transition that parks its run on the gate
// (Spec S02.7 guarantee vocabulary).
func (j *Journal) ProposeTx(ctx context.Context, tx *sql.Tx, p Proposal) (Effect, error) {
	if p.UserID == "" {
		return Effect{}, errors.New("gates: effect without user_id (owner attribution, 15.6)")
	}
	if !validClass(p.Class) {
		return Effect{}, fmt.Errorf("gates: invalid effect class %q", p.Class)
	}
	canonical, err := canonicalJSON(p.Payload)
	if err != nil {
		return Effect{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Effect{}, err
	}
	now := j.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO effects (effect_id, run_id, user_id, class, payload, payload_hash,
		     state, provider_window_ref, attempts, created_ts, updated_ts)
		 VALUES (?, ?, ?, ?, ?, ?, 'proposed', ?, 0, ?, ?)`,
		id, nullString(p.RunID), p.UserID, string(p.Class), string(canonical),
		hashPayload(canonical), nullString(p.ProviderWindowRef), now, now)
	if err != nil {
		return Effect{}, fmt.Errorf("gates: insert effect: %w", err)
	}
	return j.getTx(ctx, tx, id)
}

// Approve records the approval: approver + timestamp, payload_hash pinned
// (proposed→approved, Spec S02.7).
func (j *Journal) Approve(ctx context.Context, effectID, approver string) (Effect, error) {
	if approver == "" {
		return Effect{}, errors.New("gates: approval without approver")
	}
	var e Effect
	err := j.db.WriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := j.getTx(ctx, tx, effectID)
		if err != nil {
			return err
		}
		if cur.State != EffectProposed {
			return fmt.Errorf("%w: approve wants proposed, effect %q is %s", ErrBadState, effectID, cur.State)
		}
		// Pin: the stored payload must still match its recorded hash.
		canonical, err := canonicalJSON(cur.Payload)
		if err != nil {
			return err
		}
		if hashPayload(canonical) != cur.PayloadHash {
			return fmt.Errorf("%w: effect %q at approval", ErrPayloadDrift, effectID)
		}
		now := j.now().UTC().Format(time.RFC3339Nano)
		_, err = tx.ExecContext(ctx,
			`UPDATE effects SET state = 'approved', approved_by = ?, approved_ts = ?, updated_ts = ?
			 WHERE effect_id = ?`, approver, now, now, effectID)
		if err != nil {
			return fmt.Errorf("gates: approve effect %q: %w", effectID, err)
		}
		e, err = j.getTx(ctx, tx, effectID)
		return err
	})
	return e, err
}

// BeginExecute flips approved→executing in its OWN transaction, WRITTEN
// BEFORE the provider call (Spec S02.7): inside that transaction the
// payload_hash is re-verified, the approval window ⚙ effects.approval_expiry
// is enforced, the idempotency key (= the effect UUID) is set — stable
// across retries — and attempts increments. Expiry or drift sends the
// effect back to proposed for re-approval (Terraform saved-plan semantics)
// and returns ErrApprovalExpired / ErrPayloadDrift.
func (j *Journal) BeginExecute(ctx context.Context, effectID string) (Effect, error) {
	expiry, err := j.settings.Duration(keyApprovalExpiry)
	if err != nil {
		return Effect{}, fmt.Errorf("gates: read ⚙ %s: %w", keyApprovalExpiry, err)
	}
	var (
		e Effect
		// The back-to-re-approval reset is a durable state change and must
		// COMMIT even though the call fails, so its sentinel travels
		// outside the transaction error path (a returned error would roll
		// the reset back).
		reapproval error
	)
	err = j.db.WriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := j.getTx(ctx, tx, effectID)
		if err != nil {
			return err
		}
		if cur.State != EffectApproved {
			return fmt.Errorf("%w: execute wants approved, effect %q is %s", ErrBadState, effectID, cur.State)
		}
		now := j.now().UTC()
		nowTS := now.Format(time.RFC3339Nano)
		if now.Sub(cur.ApprovedTS) > expiry {
			if _, err := tx.ExecContext(ctx,
				`UPDATE effects SET state = 'proposed', approved_by = NULL, approved_ts = NULL, updated_ts = ?
				 WHERE effect_id = ?`, nowTS, effectID); err != nil {
				return fmt.Errorf("gates: expire approval %q: %w", effectID, err)
			}
			reapproval = fmt.Errorf("%w: effect %q approved %s ago (⚙ %s = %s)",
				ErrApprovalExpired, effectID, now.Sub(cur.ApprovedTS).Round(time.Second), keyApprovalExpiry, expiry)
			return nil
		}
		canonical, err := canonicalJSON(cur.Payload)
		if err != nil {
			return err
		}
		if hashPayload(canonical) != cur.PayloadHash {
			if _, err := tx.ExecContext(ctx,
				`UPDATE effects SET state = 'proposed', approved_by = NULL, approved_ts = NULL, updated_ts = ?
				 WHERE effect_id = ?`, nowTS, effectID); err != nil {
				return fmt.Errorf("gates: reset drifted effect %q: %w", effectID, err)
			}
			reapproval = fmt.Errorf("%w: effect %q at execution", ErrPayloadDrift, effectID)
			return nil
		}
		key := cur.IdempotencyKey
		if key == "" {
			key = cur.ID // idempotency_key = the effect UUID (Spec S02.7)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE effects SET state = 'executing', idempotency_key = ?, attempts = attempts + 1, updated_ts = ?
			 WHERE effect_id = ?`, key, nowTS, effectID); err != nil {
			return fmt.Errorf("gates: begin execute %q: %w", effectID, err)
		}
		e, err = j.getTx(ctx, tx, effectID)
		return err
	})
	if err != nil {
		return Effect{}, err
	}
	if reapproval != nil {
		return Effect{}, reapproval
	}
	return e, nil
}

// Succeed records a successful provider outcome (executing→succeeded).
func (j *Journal) Succeed(ctx context.Context, effectID string, result json.RawMessage) (Effect, error) {
	return j.finish(ctx, effectID, EffectSucceeded, result)
}

// Fail records a definitive provider failure (executing→failed).
func (j *Journal) Fail(ctx context.Context, effectID string, result json.RawMessage) (Effect, error) {
	return j.finish(ctx, effectID, EffectFailed, result)
}

// MarkUnknown records an unknowable outcome (executing→unknown): first-class
// terminal plus the decision-card content telling the human what to check
// (5.6, P-T07-3).
func (j *Journal) MarkUnknown(ctx context.Context, effectID, note string) (Effect, error) {
	card, err := json.Marshal(struct {
		Card string `json:"card"`
		Note string `json:"note"`
	}{Card: "effect-outcome-unknown", Note: note})
	if err != nil {
		return Effect{}, fmt.Errorf("gates: marshal unknown card: %w", err)
	}
	return j.finish(ctx, effectID, EffectUnknown, card)
}

func (j *Journal) finish(ctx context.Context, effectID string, to EffectState, result json.RawMessage) (Effect, error) {
	if len(result) > 0 && !json.Valid(result) {
		return Effect{}, errors.New("gates: effect result is not valid JSON")
	}
	var e Effect
	err := j.db.WriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := j.getTx(ctx, tx, effectID)
		if err != nil {
			return err
		}
		if cur.State != EffectExecuting {
			return fmt.Errorf("%w: %s wants executing, effect %q is %s", ErrBadState, to, effectID, cur.State)
		}
		now := j.now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx,
			`UPDATE effects SET state = ?, result = ?, updated_ts = ? WHERE effect_id = ?`,
			string(to), nullString(string(result)), now, effectID); err != nil {
			return fmt.Errorf("gates: finish effect %q: %w", effectID, err)
		}
		e, err = j.getTx(ctx, tx, effectID)
		return err
	})
	return e, err
}

// InDoubtResolution records what the recovery pass did with one in-doubt
// executing row.
type InDoubtResolution struct {
	EffectID string
	Class    Class
	To       EffectState
}

// ReconcileInDoubt resolves every in-doubt `executing` row per its class
// (Spec S02.5 step 6, S02.7 crash-window dispositions):
//
//   - classes A–C return to approved for replay — A retries blind, B
//     retries with the stored idempotency key inside the provider window,
//     C searches for its embedded effect-id marker before any retry (the
//     executor branches on class and attempts>0; B1+);
//   - class D flips to unknown with a decision card (at-most-once,
//     P-T07-3).
//
// The idempotency key and attempt count survive the flip, which is what
// makes the at-least-once replay invisible at the provider.
func (j *Journal) ReconcileInDoubt(ctx context.Context) ([]InDoubtResolution, error) {
	inDoubt, err := j.InState(ctx, EffectExecuting)
	if err != nil {
		return nil, err
	}
	var out []InDoubtResolution
	for _, e := range inDoubt {
		switch e.Class {
		case ClassD:
			if _, err := j.MarkUnknown(ctx, e.ID,
				"crash window on an idempotency-less channel: the provider may or may not have acted; "+
					"check the provider's own record before deciding (Spec S02.7 class D)"); err != nil {
				return out, err
			}
			out = append(out, InDoubtResolution{EffectID: e.ID, Class: e.Class, To: EffectUnknown})
		default: // A, B, C — replay path
			err := j.db.WriteTx(ctx, func(tx *sql.Tx) error {
				now := j.now().UTC().Format(time.RFC3339Nano)
				_, err := tx.ExecContext(ctx,
					`UPDATE effects SET state = 'approved', updated_ts = ?
					 WHERE effect_id = ? AND state = 'executing'`, now, e.ID)
				return err
			})
			if err != nil {
				return out, fmt.Errorf("gates: reconcile effect %q: %w", e.ID, err)
			}
			out = append(out, InDoubtResolution{EffectID: e.ID, Class: e.Class, To: EffectApproved})
		}
	}
	return out, nil
}

// Get returns one effect.
func (j *Journal) Get(ctx context.Context, effectID string) (Effect, error) {
	e, err := scanEffect(j.db.QueryRowContext(ctx, selectEffects+` WHERE effect_id = ?`, effectID))
	if errors.Is(err, sql.ErrNoRows) {
		return Effect{}, fmt.Errorf("%w: %q", ErrEffectNotFound, effectID)
	}
	return e, err
}

// InState returns every effect in state, ordered by effect_id.
func (j *Journal) InState(ctx context.Context, state EffectState) ([]Effect, error) {
	rows, err := j.db.QueryContext(ctx, selectEffects+` WHERE state = ? ORDER BY effect_id`, string(state))
	if err != nil {
		return nil, fmt.Errorf("gates: list effects: %w", err)
	}
	defer rows.Close()
	var out []Effect
	for rows.Next() {
		e, err := scanEffect(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (j *Journal) getTx(ctx context.Context, tx *sql.Tx, effectID string) (Effect, error) {
	e, err := scanEffect(tx.QueryRowContext(ctx, selectEffects+` WHERE effect_id = ?`, effectID))
	if errors.Is(err, sql.ErrNoRows) {
		return Effect{}, fmt.Errorf("%w: %q", ErrEffectNotFound, effectID)
	}
	return e, err
}

const selectEffects = `SELECT effect_id, run_id, user_id, class, payload, payload_hash,
       state, approved_by, approved_ts, idempotency_key, provider_window_ref,
       attempts, result, created_ts, updated_ts
  FROM effects`

func scanEffect(row rowScanner) (Effect, error) {
	var (
		e                              Effect
		runID, approvedBy, approvedTS  sql.NullString
		idempotencyKey, providerWindow sql.NullString
		result                         sql.NullString
		class, state, payload          string
		createdTS, updatedTS           string
	)
	err := row.Scan(&e.ID, &runID, &e.UserID, &class, &payload, &e.PayloadHash,
		&state, &approvedBy, &approvedTS, &idempotencyKey, &providerWindow,
		&e.Attempts, &result, &createdTS, &updatedTS)
	if err != nil {
		return Effect{}, err
	}
	e.RunID = runID.String
	e.Class = Class(class)
	e.State = EffectState(state)
	e.Payload = json.RawMessage(payload)
	e.ApprovedBy = approvedBy.String
	e.IdempotencyKey = idempotencyKey.String
	e.ProviderWindowRef = providerWindow.String
	if result.Valid {
		e.Result = json.RawMessage(result.String)
	}
	if approvedTS.Valid && approvedTS.String != "" {
		if e.ApprovedTS, err = time.Parse(time.RFC3339Nano, approvedTS.String); err != nil {
			return Effect{}, fmt.Errorf("gates: parse approved_ts %q: %w", approvedTS.String, err)
		}
	}
	if e.CreatedTS, err = time.Parse(time.RFC3339Nano, createdTS); err != nil {
		return Effect{}, fmt.Errorf("gates: parse effect created_ts %q: %w", createdTS, err)
	}
	if e.UpdatedTS, err = time.Parse(time.RFC3339Nano, updatedTS); err != nil {
		return Effect{}, fmt.Errorf("gates: parse effect updated_ts %q: %w", updatedTS, err)
	}
	return e, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
