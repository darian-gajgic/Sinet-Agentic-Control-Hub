package recovery

// laddercard.go — P3-RW-14 R1/R2: a ladder decision that ENDS a lineage
// leaves a durable, answerable door.
//
// Spec S02.5 step 7 names "tombstone-review cards" and Spec S02.3 names the
// finalize terminal "finalized (finalize-with-card)". Both were recorded only
// as a STRING inside the transition's event payload, so a lineage the ladder
// gave up on left its task wedged in whatever stage column it died in, with
// nothing to answer — the finding-that-died-in-a-log this platform declares
// structurally impossible (Spec S07.7; P-T06-4 escalation-path death). Live
// proof (builder world, 2026-08-16): task t-4b982b38a62baee5 sat at kanban
// "verifying" with its verify lineage tombstoned (event_seq 1075) and ZERO
// open asks.
//
// The card is a plain S02.2 `asks` row written in the SAME transaction as the
// terminal transition (CONVENTIONS §8: a crash and its bound decision commit
// together — so does the door), owner-attributed to the run's owner (15.6),
// and the owning task leaves its stage column for "attention" in that same
// transaction (Spec S15.5 blocked-on-a-human; S15.6 the inbox is where "the
// platform needs you" lives).
//
// Two deliberate restraints:
//
//   - This package imports NOTHING new for it. The card is JSON shaped here,
//     and its field names mirror the S07.7 card (kind/task_id/run_id/
//     issued_ts/sla/summary/detail/choices/ask_id) so ONE inbox renderer reads
//     both kinds. The run-FSM choreography of ANSWERING it belongs to the
//     stage layer, exactly as the S07.7 cards' does.
//   - It carries no S07.7 route-table CATEGORY. A ladder terminal is not a
//     verification finding, and minting a category for it would invent
//     ratified vocabulary this packet was not given (P3-RW-14 §9 item 10). It
//     carries the ratified SINK type (decision card) and the S02 vocabulary of
//     the terminal that raised it.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S07.7 inbox-wide SLA numbers, consumed at card issue (P3-RW-14 OQ5:
// "these are inbox-wide numbers" — the ladder's cards are approval class and
// invent no key of their own; CONVENTIONS §2: read by dotted key).
const (
	keyCardRemindHours = "verification.card_remind_hours"
	keyCardPushHours   = "verification.card_push_hours"
)

// LadderAskPrefix keys the answer surface's prefix dispatch (CONVENTIONS §16;
// the `ask-verify-` / `intake:` precedent).
const LadderAskPrefix = "ask-ladder-"

// LadderAskID is the card's durable ask id. It is DERIVED from the run id
// rather than random: a lineage terminates exactly once, so the derived id
// makes a second card for the same terminal a primary-key violation instead of
// a silent duplicate in the operator's inbox.
func LadderAskID(runID string) string { return LadderAskPrefix + runID }

// Ladder terminal kinds, in the spec's own words.
const (
	// CardTombstoneReview is Spec S02.5 step 7's tombstone-review card.
	CardTombstoneReview = "tombstone-review"
	// CardFinalizeWithCard is Spec S02.3's finalize-with-card.
	CardFinalizeWithCard = "finalize-with-card"
)

// The card's verbs (P3-RW-14 OQ2, ratified): retry grants the lineage a fresh
// bounded budget; cancel ends the task. Every other verb is refused loudly by
// the answer surface — a card never silently absorbs an answer it cannot mean.
const (
	VerbRetry  = "retry"
	VerbCancel = "cancel"
)

// LadderSLA is the card's reminder schedule (approval class, Spec S07.7).
type LadderSLA struct {
	Class            string `json:"class"`
	RemindAfterHours int64  `json:"remind_after_hours,omitempty"`
	PushAfterHours   int64  `json:"push_after_hours,omitempty"`
}

// LadderCard is the durable snapshot of a ladder terminal's card — the full
// reconstruction the ask row carries (Spec S02.2: an ask row is a full
// reconstruction snapshot).
type LadderCard struct {
	// Kind is the ratified sink type (Spec S07.7: decision card).
	Kind string `json:"kind"`
	// Card names the S02 terminal that raised it (tombstone-review /
	// finalize-with-card).
	Card string `json:"card"`

	TaskID string `json:"task_id,omitempty"`
	// RunID is the terminated run — the lineage's CURRENT identity.
	RunID string `json:"run_id"`
	// LineageBirthRunID is where the lineage started, before any fork
	// (Spec S02.5 step 2 fork lineage).
	LineageBirthRunID string `json:"lineage_birth_run_id"`
	Generation        int64  `json:"generation"`
	RecoveryAttempts  int64  `json:"recovery_attempts"`

	// Ground is the ladder's decision ground — WHY this lineage ended here.
	Ground string `json:"ground"`
	// LastCrashCause is the cause recorded on the lineage's last crash, the
	// fact an operator needs first (the live world's was "verification drain:
	// verify: launch-domain deliverable without a domain check pack").
	LastCrashCause string `json:"last_crash_cause,omitempty"`

	IssuedTS string    `json:"issued_ts"`
	SLA      LadderSLA `json:"sla"`

	Summary string   `json:"summary"`
	Detail  []string `json:"detail,omitempty"`
	Choices []string `json:"choices"`

	AskID string `json:"ask_id"`
}

// LoadOpenLadderCard reads one OPEN ladder card and its owner — the requester
// the answer must come from (15.6/D10). A missing row answers sql.ErrNoRows so
// the answer surface maps it to its ordinary not-found status.
func LoadOpenLadderCard(ctx context.Context, db *storage.DB, askID string) (LadderCard, string, error) {
	var snapshot, owner string
	err := db.QueryRowContext(ctx,
		`SELECT snapshot, user_id FROM asks WHERE ask_id = ? AND status = 'open'`, askID).
		Scan(&snapshot, &owner)
	if err != nil {
		return LadderCard{}, "", fmt.Errorf("recovery: no open ladder card %q: %w", askID, err)
	}
	var c LadderCard
	if err := json.Unmarshal([]byte(snapshot), &c); err != nil {
		return LadderCard{}, "", fmt.Errorf("recovery: decode ladder card %q: %w", askID, err)
	}
	return c, owner, nil
}

// terminalCardTx writes the ladder terminal's open ask row and moves the
// owning task out of its stage column, inside the caller's transaction — the
// same transaction as the terminal transition, so a lineage can never end
// without its door (P3-RW-14 R1/R2).
func (l *Ladder) terminalCardTx(ctx context.Context, tx *sql.Tx, r run.Run, kind, ground, summary string, detail []string) (string, error) {
	sla, err := l.cardSLA()
	if err != nil {
		return "", err
	}
	birth, err := lineageBirthTx(ctx, tx, r)
	if err != nil {
		return "", err
	}
	cause, err := lastCrashCauseTx(ctx, tx, r.ID)
	if err != nil {
		return "", err
	}
	askID := LadderAskID(r.ID)
	issued := l.cfg.Now().UTC().Format(time.RFC3339Nano)
	card := LadderCard{
		Kind:              "decision_card",
		Card:              kind,
		TaskID:            r.TaskID,
		RunID:             r.ID,
		LineageBirthRunID: birth,
		Generation:        r.Generation,
		RecoveryAttempts:  r.RecoveryAttempts,
		Ground:            ground,
		LastCrashCause:    cause,
		IssuedTS:          issued,
		SLA:               sla,
		Summary:           summary,
		Detail:            append(append([]string(nil), detail...), cardVerbHelp...),
		Choices:           []string{VerbRetry, VerbCancel},
		AskID:             askID,
	}
	if cause != "" {
		card.Detail = append(card.Detail, "What the last attempt reported: "+cause)
	}
	snapshot, err := json.Marshal(card)
	if err != nil {
		return "", fmt.Errorf("recovery: marshal ladder card: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
		 VALUES (?, ?, ?, ?, 'open', ?)`,
		askID, r.ID, r.UserID, string(snapshot), issued); err != nil {
		return "", fmt.Errorf("recovery: insert ladder card %q: %w", askID, err)
	}
	if err := attentionTx(ctx, tx, r); err != nil {
		return "", err
	}
	return askID, nil
}

// cardVerbHelp states the two verbs in the requester's words (CONVENTIONS §57
// drafting rule 1: requester-facing strings are written for someone who is not
// a programmer).
var cardVerbHelp = []string{
	"Answer `retry` to start a fresh attempt with a full budget again, or `cancel` to end the task.",
}

// cardSLA computes the approval-class reminder schedule from the ratified
// inbox-wide ⚙ set (Spec S07.7; P3-RW-14 OQ5 — no new key).
func (l *Ladder) cardSLA() (LadderSLA, error) {
	remind, err := l.cfg.Settings.Int(keyCardRemindHours)
	if err != nil {
		return LadderSLA{}, fmt.Errorf("recovery: read ⚙ %s: %w", keyCardRemindHours, err)
	}
	push, err := l.cfg.Settings.Int(keyCardPushHours)
	if err != nil {
		return LadderSLA{}, fmt.Errorf("recovery: read ⚙ %s: %w", keyCardPushHours, err)
	}
	return LadderSLA{Class: "approval", RemindAfterHours: remind, PushAfterHours: push}, nil
}

// attentionTx moves the owning task out of its stage column when the ended
// lineage was the last thing driving it (P3-RW-14 R2; 9.1/S1.3 task–kanban
// orthogonality). Tasks already `done` or `cancelled` are never touched: a
// finished task is not "blocked on a human", and a ladder terminal on some
// leftover run must not rewrite the record of how it ended.
func attentionTx(ctx context.Context, tx *sql.Tx, r run.Run) error {
	if r.TaskID == "" {
		return nil // a platform run with no task has no board column
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE tasks SET kanban_status = 'attention'
		 WHERE task_id = ?
		   AND kanban_status NOT IN ('done', 'cancelled')
		   AND NOT EXISTS (
		       SELECT 1 FROM runs
		        WHERE task_id = ? AND run_id <> ?
		          AND state IN ('new', 'queued', 'claimed', 'running', 'draining', 'parked'))`,
		r.TaskID, r.TaskID, r.ID)
	if err != nil {
		return fmt.Errorf("recovery: kanban attention for task %q: %w", r.TaskID, err)
	}
	return nil
}

// lineageBirthTx walks `runs.parent_run_id` back to the lineage's birth id —
// the identity the work was admitted under, before any fork (Spec S02.5 step
// 2). The walk is bounded: a cycle is impossible by construction (a successor
// is always created after its parent), and the bound keeps a hand-edited row
// from turning a reconcile pass into a spin.
func lineageBirthTx(ctx context.Context, tx *sql.Tx, r run.Run) (string, error) {
	id, parent := r.ID, r.ParentRunID
	for i := 0; parent != "" && i < 64; i++ {
		id = parent
		var next sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT parent_run_id FROM runs WHERE run_id = ?`, id).Scan(&next)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return id, nil
			}
			return "", fmt.Errorf("recovery: walk lineage of %q: %w", r.ID, err)
		}
		parent = next.String
	}
	return id, nil
}

// lastCrashCauseTx reads the cause recorded on the lineage's newest crash
// transition. The cause is what the crashing layer put on the event (the stage
// legs write `{"cause": …}`); a crash without one — the ladder's own DEAD
// classification — yields the transition reason instead, and never an error:
// the door does not depend on the prose.
func lastCrashCauseTx(ctx context.Context, tx *sql.Tx, runID string) (string, error) {
	var cause, reason sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT json_extract(payload, '$.detail.cause'), json_extract(payload, '$.reason')
		  FROM run_events
		 WHERE run_id = ? AND type = ? AND json_extract(payload, '$.to') = 'crashed'
		 ORDER BY event_seq DESC LIMIT 1`, runID, run.EventState).Scan(&cause, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("recovery: read last crash cause of %q: %w", runID, err)
	}
	if cause.Valid && cause.String != "" {
		return cause.String, nil
	}
	return reason.String, nil
}
