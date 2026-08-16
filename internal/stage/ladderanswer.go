package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// ladderanswer.go — answering the recovery ladder's terminal cards (P3-RW-14
// R3). internal/recovery MINTS them (S02.5 step 7's tombstone-review card,
// S02.3's finalize-with-card); this file owns the run-FSM choreography around
// them, exactly as answer.go does for the S07.7 verification cards:
//
//   - the ask closes and the run acts in ONE transaction (CONVENTIONS §8);
//   - `retry` supersedes the ended lineage with a generation-bumped successor
//     whose recovery attempts are RESET — "an answer is an explicit human grant
//     of a fresh bounded budget" (the §16 / S06.7(a) precedent) — admitted
//     through the one ingress (Spec S16.6), never fire-and-forget;
//   - `cancel` ends the task under the ratified S02.3 mapping (CONVENTIONS §14
//     reading 9); the ended run is NEVER rewritten — a terminal is a fact.
//
// The sentinel vocabulary is verify's on purpose: one answer surface, one set
// of refusals, one transport mapping (surface.go mapVerifyErr).

// IsLadderAskID reports whether an ask id is a recovery-ladder terminal card —
// the answer surface's prefix dispatch key (CONVENTIONS §16).
func IsLadderAskID(askID string) bool {
	return strings.HasPrefix(askID, recovery.LadderAskPrefix)
}

// AnswerLadderAsk applies one ladder-terminal card answer and returns the
// card's task id for the caller's task view.
func (s *Skeleton) AnswerLadderAsk(ctx context.Context, actor, askID string, raw json.RawMessage) (string, error) {
	card, owner, err := recovery.LoadOpenLadderCard(ctx, s.cfg.DB, askID)
	if err != nil {
		return "", err
	}
	if actor != owner {
		return card.TaskID, fmt.Errorf("%w: %q", verify.ErrNotRequester, actor)
	}
	ans, err := verify.DecodeAnswer(raw)
	if err != nil {
		return card.TaskID, err
	}
	switch string(ans.Choice) {
	case recovery.VerbRetry:
		return card.TaskID, s.ladderRetry(ctx, actor, askID, card, ans, raw)
	case recovery.VerbCancel:
		return card.TaskID, s.ladderCancel(ctx, actor, askID, card, ans, raw)
	case "":
		return card.TaskID, fmt.Errorf("%w: missing \"choice\"", verify.ErrBadAnswer)
	default:
		return card.TaskID, fmt.Errorf("%w: verb %q (a ladder-terminal card answers retry or cancel)",
			verify.ErrUnsupportedAnswer, ans.Choice)
	}
}

// ladderRetry grants the ended lineage a fresh bounded budget: a
// generation-bumped successor with attempts reset, created and enqueued so the
// claim loop drives it exactly like any other run.
func (s *Skeleton) ladderRetry(ctx context.Context, actor, askID string, card recovery.LadderCard, ans verify.Answer, raw json.RawMessage) error {
	if s.sched == nil {
		return fmt.Errorf("stage: no scheduler bound, so a retried run could not be admitted (Spec S16.6)")
	}
	// A card outlives the lineage it was raised on, so it can be answered after
	// the TASK itself has ended — a second ladder card on the same task, still
	// open when the first one was cancelled at (P3-RW-14A drain D2b). Retrying
	// there would resurrect a cancelled task: a fresh run enqueued, billed, and
	// the board dragged back out of `cancelled`. The card refuses instead.
	if column, err := s.taskColumn(ctx, card.TaskID); err != nil {
		return err
	} else if column == kanbanCancelled || column == kanbanDone {
		return fmt.Errorf("%w: task %s is already %s — a fresh attempt would restart work its owner has already ended (cancel this card, or start a new request)",
			verify.ErrNotResumable, card.TaskID, column)
	}
	r, err := s.cfg.Runs.Get(ctx, card.RunID)
	if err != nil {
		return err
	}
	s.recordLadderDecision(ctx, card, actor, "requester retried the "+card.Card+" card"+noteSuffix(ans),
		"retry (S02.5 step 3 terminal, P3-RW-14 R3): a fresh generation-bumped attempt of the same lineage with recovery attempts reset — an answer is an explicit human grant of a fresh bounded budget")

	newGen := r.Generation + 1
	successorID := fmt.Sprintf("%s.g%d", r.ID, newGen)
	// A dispatch id distinct from the ladder's own `<run>#g<n>` scheme: this
	// attempt was granted by a human, not by an interruption, and the unique
	// index is what makes a double-answer impossible to double-start.
	dispatchID := fmt.Sprintf("%s#answered-retry#g%d", r.ID, newGen)
	detail, err := json.Marshal(struct {
		ParentRunID      string `json:"parent_run_id"`
		ParentGeneration int64  `json:"parent_generation"`
		DispatchID       string `json:"dispatch_id"`
		AnsweredAsk      string `json:"answered_ask"`
		Actor            string `json:"actor"`
	}{r.ID, r.Generation, dispatchID, askID, actor})
	if err != nil {
		return fmt.Errorf("stage: marshal ladder-retry detail: %w", err)
	}
	err = s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := closeLadderAskTx(ctx, tx, askID, raw, s.now()); err != nil {
			return err
		}
		// Fence the ended generation before the successor exists (Spec S02.5
		// step 4), the same discipline the ladder's own fork keeps.
		if _, err := s.cfg.Runs.BumpGenerationTx(ctx, tx, r.ID); err != nil {
			return err
		}
		_, err := s.cfg.Runs.CreateTx(ctx, tx, run.NewRun{
			ID:             successorID,
			UserID:         r.UserID,
			TaskID:         r.TaskID,
			Substrate:      r.Substrate,
			Lane:           r.Lane,
			WorkspaceRef:   r.WorkspaceRef,
			CeilingTimeS:   r.CeilingTimeS,
			CeilingSteps:   r.CeilingSteps,
			CeilingCostUSD: r.CeilingCostUSD,
			State:          run.StateQueued,
			Generation:     newGen,
			ParentRunID:    r.ID,
			DispatchID:     dispatchID,
			// THE fresh budget (P3-RW-14 R3): the human's answer is what buys
			// the lineage its ⚙ recovery.max_attempts back.
			RecoveryAttempts: 0,
		}, run.EventForked, detail)
		return err
	})
	if err != nil {
		return err
	}
	if err := s.sched.Enqueue(ctx, successorID, scheduler.ClassInteractive); err != nil {
		return fmt.Errorf("stage: admit the retried run %q: %w", successorID, err)
	}
	// The board follows the work: the task is being driven again, so it leaves
	// the attention column for the column of the role that is running.
	if card.TaskID != "" {
		if column, ok := kanbanForRun(successorID); ok {
			s.setKanban(ctx, card.TaskID, column)
		}
	}
	s.logger().Info("stage: ladder-terminal card answered with retry — fresh attempt admitted (S02.5 step 3; P3-RW-14 R3)",
		"run", successorID, "parent", r.ID, "task", card.TaskID, "ask", askID)
	return nil
}

// ladderCancel ends the TASK at the card, through the one cancel machinery
// (cancel.go, the ratified S02.3 mapping / CONVENTIONS §14 reading 9).
//
// Riding the cancel machinery (CancelTaskAtCard, cancel.go) rather than writing
// `cancelled` on the task row is the
// whole point (P3-RW-14A drain D2a): a task can hold more than the lineage this
// card came from, and a column that says "cancelled" over a still-running
// sibling is a lie that also keeps billing. CancelTask ends every non-terminal
// run under the ratified mapping and closes the asks parked on them, so no
// sibling can later land a verdict that flips a cancelled task to `done`.
//
// The card's own run is left exactly as it is: it had already ended, and
// rewriting a terminal would falsify the record of how it ended (CancelTask
// reports it as "already ended; nothing was cancelled").
func (s *Skeleton) ladderCancel(ctx context.Context, actor, askID string, card recovery.LadderCard, ans verify.Answer, raw json.RawMessage) error {
	s.recordLadderDecision(ctx, card, actor, "requester cancelled at the "+card.Card+" card"+noteSuffix(ans),
		"cancel is always available (4.5); the whole task ends under the ratified S02.3 mapping — every live sibling run with it (CONVENTIONS §14 reading 9)")
	if card.TaskID != "" {
		if _, err := s.CancelTaskAtCard(ctx, actor, card.TaskID); err != nil {
			// The card stays OPEN on a refused cancel (a claimed run mid-dispatch
			// answers 409-retry): the door must not be consumed by a decision the
			// platform did not carry out.
			return err
		}
	}
	err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		return closeLadderAskTx(ctx, tx, askID, raw, s.now())
	})
	if err != nil {
		return err
	}
	if card.TaskID != "" {
		// CancelTask moves the column only when it actually cancelled something
		// (its own drain D4 rule). Here the human's answer IS the decision, and
		// the commonest shape has nothing live left to cancel — so the column is
		// set from the answer.
		s.setKanban(ctx, card.TaskID, kanbanCancelled)
	}
	s.logger().Info("stage: ladder-terminal card answered with cancel — the task ends (4.5)",
		"run", card.RunID, "task", card.TaskID, "ask", askID)
	return nil
}

// taskColumn reads a task's stored kanban column ("" when the card sits on a
// run with no task — the dead-man canary shape, which has no board and no
// terminal state to respect).
func (s *Skeleton) taskColumn(ctx context.Context, taskID string) (string, error) {
	if taskID == "" {
		return "", nil
	}
	var column string
	err := s.cfg.DB.QueryRowContext(ctx,
		`SELECT kanban_status FROM tasks WHERE task_id = ?`, taskID).Scan(&column)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("stage: read task %q column: %w", taskID, err)
	}
	return column, nil
}

// recordLadderDecision files the human decision in the task's ledger. A ladder
// card can sit on a run with NO task (the dead-man canary, a benchmark direct
// arm): there is no ledger to write to then, and the durable answer on the ask
// row plus the run events remain the record — so the absence is logged, never
// fatal to the answer.
func (s *Skeleton) recordLadderDecision(ctx context.Context, card recovery.LadderCard, actor, text, reason string) {
	if card.TaskID == "" {
		return
	}
	if _, err := s.cfg.Ledger.RecordDecision(ctx, card.RunID, ledger.AuthorHuman, actor, "recovery", text, reason, 0); err != nil {
		s.logger().Warn("stage: ladder card decision not recorded in the ledger",
			"run", card.RunID, "task", card.TaskID, "err", err)
	}
}

// closeLadderAskTx records the answer on the open ask row inside the caller's
// transaction — composed with the run's act, so a card can never be consumed
// without the platform doing what it said (the intake closeAskTx shape).
func closeLadderAskTx(ctx context.Context, tx *sql.Tx, askID string, answer json.RawMessage, now time.Time) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE asks SET status = 'answered', answered_ts = ?, answer = ? WHERE ask_id = ? AND status = 'open'`,
		now.UTC().Format(time.RFC3339Nano), string(answer), askID)
	if err != nil {
		return fmt.Errorf("stage: close ladder card %q: %w", askID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("stage: close ladder card %q: %w", askID, err)
	} else if n != 1 {
		return fmt.Errorf("%w: %q", verify.ErrUnknownAsk, askID)
	}
	return nil
}

// kanbanForRun maps a retried run's role onto the board column that honestly
// describes it. A role the board has no column for leaves the column alone.
func kanbanForRun(runID string) (string, bool) {
	rl, ok := runRole(runID)
	if !ok {
		return "", false
	}
	switch rl {
	case roleIntake:
		return "intake", true // the birth column the pipeline itself writes
	case roleExecute:
		return "executing", true
	case roleVerify:
		return "verifying", true
	default:
		return "", false
	}
}
