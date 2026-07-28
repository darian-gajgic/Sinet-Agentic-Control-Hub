package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// cancel.go is the ratified S02.3 cancel mapping (feature 4.5) behind the
// S15.6 cancel verbs. S02.3 has NO cancelled state, so a cancel is expressed
// in the states the FSM already has (CONVENTIONS §14 reading 9, restated §16):
//
//	running / draining → completed, with the cancel reason on the transition
//	parked            → finalized (finalize-with-card; generation unchanged,
//	                    no resume edge — the B2-5 choreography, generalized)
//	queued            → finalized (the OQ1 edge; the queue row settles in the
//	                    SAME transaction, so a cancelled run can never be
//	                    claimed after its cancellation committed)
//	claimed / new     → 409-retry: the transient CAS window is never raced
//	already terminal  → the honest already-ended answer, nothing fired
//
// and the kanban column reads `cancelled`.
//
// ── NO AUTO-KILL (S14.4 / G1 D1.3; CONVENTIONS §31) ────────────────────────
//
// A cancel is a HUMAN act. Every entry point in this file takes an `actor`
// that is a person resolved from an authenticated session, and the ONLY
// callers are the two S15.6 HTTP verbs through Surface (surface.go). No
// watchdog, no recovery pass, no scheduler loop, no benchmark driver and no
// engine-side path reaches any of it — the watchdog's import wall and the
// benchmark package's no-kill wall stay byte-meaningful, and
// TestCancelIsReachableFromTheHTTPVerbsOnly asserts the caller set directly.

// liveSessions maps a run to the live engine session's control handle for the
// one purpose the S03.1 cancel ladder exists to serve: a person pressing
// cancel on work that is running RIGHT NOW.
//
// The registry is in-process and dies with the process, which is correct — a
// session is a child of this process, so a handle that outlived it would name
// something that no longer exists, and a run interrupted by process death is
// the recovery ladder's story (S02.5), not this one.
type liveSessions struct {
	mu   sync.Mutex
	sess map[string]adapters.Session
	// requested maps a run to the GENERATION its human cancel was requested at.
	// It outlives the session handle deliberately: the dispatch leg unwinding
	// BECAUSE of the ladder must not then file a crash corpse for the recovery
	// ladder to fork (see Skeleton.crash).
	//
	// The suppression is SINGLE-USE and GENERATION-SCOPED, and both properties
	// are load-bearing (drain D1). A sticky boolean would swallow a LATER,
	// genuine crash of the same run: a cancel whose terminal transition failed,
	// or a run that resumed afterwards, would leave the flag set forever and
	// the next real crash would file no corpse — an in-process zombie the
	// recovery ladder never forks, which is the exact failure D1.3's pause-and-
	// flag posture exists to prevent. So the mark carries the generation it was
	// taken at (a resume bumps the generation and renders any stale mark inert,
	// §8), it is CONSUMED the first time it suppresses a crash, and CancelRun
	// clears it on every exit path where the terminal transition did not commit.
	requested map[string]int64
}

func newLiveSessions() *liveSessions {
	return &liveSessions{sess: map[string]adapters.Session{}, requested: map[string]int64{}}
}

// register records the live session of a run at spawn (the S03 OnSession
// control seam).
func (l *liveSessions) register(runID string, sess adapters.Session) {
	if sess == nil {
		return
	}
	l.mu.Lock()
	l.sess[runID] = sess
	l.mu.Unlock()
}

// deregister drops the handle when the session ends.
func (l *liveSessions) deregister(runID string) {
	l.mu.Lock()
	delete(l.sess, runID)
	l.mu.Unlock()
}

// markRequested records a human cancel for the run AT ITS CURRENT GENERATION
// and returns its live session handle, if one is registered.
func (l *liveSessions) markRequested(runID string, generation int64) adapters.Session {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requested[runID] = generation
	return l.sess[runID]
}

// clearRequested drops a mark whose cancel did not commit. A suppression that
// outlives its own failed attempt is exactly the zombie of D1.
func (l *liveSessions) clearRequested(runID string) {
	l.mu.Lock()
	delete(l.requested, runID)
	l.mu.Unlock()
}

// consumeCancel reports whether a human cancel is pending for the run AT THIS
// generation, and consumes it. Single-use: the mark suppresses exactly the one
// unwind its own cancel caused. Generation-scoped: a resume bumps the run's
// generation (§8), so a mark taken before it can never match afterwards.
func (l *liveSessions) consumeCancel(runID string, generation int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	gen, ok := l.requested[runID]
	if !ok || gen != generation {
		return false
	}
	delete(l.requested, runID)
	return true
}

// CancelOutcome is one run's cancel disposition — the ratified mapping made
// legible: which edge was taken, whether anything fired, and whether the live
// session's ladder ran.
type CancelOutcome struct {
	RunID string `json:"run_id"`
	From  string `json:"from"`
	To    string `json:"to"`
	// Applied is false when nothing fired: an already-terminal run answers the
	// idempotent repeat rather than a second cancellation.
	Applied bool `json:"applied"`
	// LadderInvoked reports the S03.1 safe-boundary → abort → TERM → KILL
	// ladder running on a live session.
	LadderInvoked bool   `json:"ladder_invoked"`
	Detail        string `json:"detail"`
}

// TaskCancelOutcome is a task-level cancel: every non-terminal run of the task
// under the same mapping, each disposition listed individually.
type TaskCancelOutcome struct {
	TaskID  string          `json:"task_id"`
	Kanban  string          `json:"kanban_status"`
	Runs    []CancelOutcome `json:"runs"`
	Applied bool            `json:"applied"`
}

// ErrCancelInFlight is the transient CAS window: a claimed (or not-yet-queued)
// run is between the scheduler's claim and the dispatcher's first transition,
// and a cancel there would race the dispatch. The verb answers 409-retry.
var ErrCancelInFlight = fmt.Errorf("stage: the run is being dispatched right now — retry the cancel in a moment")

// ErrCancelRaced is the OTHER transient window: the run's state moved between
// the read and the transition (a stage leg finished, the ladder landed first,
// the recovery pass acted). The cancel is refused rather than retried in place
// — the caller re-reads and decides again — and nothing double-fires either
// way, because the CAS-guarded transition is what failed (drain D8).
var ErrCancelRaced = fmt.Errorf("stage: the run changed state while the cancel was being applied — re-read it and decide again")

// ErrCancelQueueUnsettleable refuses a queued-run cancel in a process with no
// scheduler bound: the OQ1 edge and its queue-row settle commit together or
// not at all (drain D11).
var ErrCancelQueueUnsettleable = fmt.Errorf("stage: no scheduler is bound in this process, so a queued run's cancel cannot settle its queue row")

// raced maps a lost CAS / invalid-edge transition onto ErrCancelRaced, leaving
// every other failure as itself.
func raced(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, run.ErrInvalidTransition) || strings.Contains(err.Error(), "concurrent state change") {
		return fmt.Errorf("%w: %v", ErrCancelRaced, err)
	}
	return err
}

// CancelRun applies the ratified cancel mapping to ONE run. actor is the
// authenticated person who pressed cancel; the transport has already resolved
// owner scope (S01.9).
func (s *Skeleton) CancelRun(ctx context.Context, actor, runID string) (CancelOutcome, error) {
	r, err := s.cfg.Runs.Get(ctx, runID)
	if err != nil {
		return CancelOutcome{}, err
	}
	out, err := s.cancelOne(ctx, actor, r)
	if err != nil {
		return CancelOutcome{}, err
	}
	if out.Applied && r.TaskID != "" {
		s.setKanban(ctx, r.TaskID, kanbanCancelled)
	}
	return out, nil
}

// CancelTask cancels every non-terminal run of a task under the same mapping
// (S15.2 tasks row: "cancel (4.5)"). A run that refuses (the transient claimed
// window) fails the whole call so the caller retries the task as a unit rather
// than being told a partial cancellation succeeded.
func (s *Skeleton) CancelTask(ctx context.Context, actor, taskID string) (TaskCancelOutcome, error) {
	rows, err := s.cfg.DB.QueryContext(ctx,
		`SELECT run_id FROM runs WHERE task_id = ? ORDER BY created_ts, run_id`, taskID)
	if err != nil {
		return TaskCancelOutcome{}, fmt.Errorf("stage: read task runs %q: %w", taskID, err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return TaskCancelOutcome{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return TaskCancelOutcome{}, err
	}

	out := TaskCancelOutcome{TaskID: taskID, Runs: []CancelOutcome{}}
	for _, id := range ids {
		r, err := s.cfg.Runs.Get(ctx, id)
		if err != nil {
			return TaskCancelOutcome{}, err
		}
		one, err := s.cancelOne(ctx, actor, r)
		if err != nil {
			return TaskCancelOutcome{}, err
		}
		out.Runs = append(out.Runs, one)
		out.Applied = out.Applied || one.Applied
	}
	// The board column moves only when something was actually cancelled (drain
	// D4). A task whose runs had all already ended is not "cancelled" — it
	// finished — and a no-op cancel must not rewrite the record of that.
	if out.Applied {
		out.Kanban = kanbanCancelled
		s.setKanban(ctx, taskID, kanbanCancelled)
	}
	return out, nil
}

// kanbanCancelled is the board column a cancelled task lands in (CONVENTIONS
// §14 reading 9 / §16: "kanban cancelled").
const kanbanCancelled = "cancelled"

// cancelOne is the mapping itself.
func (s *Skeleton) cancelOne(ctx context.Context, actor string, r run.Run) (CancelOutcome, error) {
	out := CancelOutcome{RunID: r.ID, From: string(r.State), To: string(r.State)}
	reason := "cancelled by " + actor + " (4.5)"

	switch r.State {
	case run.StateCompleted, run.StateFinalized, run.StateTombstoned, run.StateDiedAtGate:
		out.Detail = "the run had already ended; nothing was cancelled"
		return out, nil
	case run.StateCrashed:
		// Terminal-but-supersedable (S02.3): the recovery ladder owns its
		// disposition (harvest / fork / tombstone / finalize, S02.5 step 3).
		// Reaching in here would race that decision, so the cancel reports the
		// fact instead of inventing an edge.
		out.Detail = "the run already crashed; the recovery ladder owns its disposition (S02.5)"
		return out, nil
	case run.StateClaimed, run.StateNew:
		return out, ErrCancelInFlight

	case run.StateRunning, run.StateDraining:
		// The live session first: the S03.1 ladder is the only thing that stops
		// an engine that is mid-call, and it runs BEFORE the terminal
		// transition so the disposition the adapter reports is the cancel's.
		out.LadderInvoked = s.runLadder(ctx, r)
		if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateCompleted, run.TransitionOptions{
			Reason: reason, Actor: actor, Detail: cancelDetail(actor, out.LadderInvoked),
		}); err != nil {
			// The terminal transition did NOT commit, so the crash suppression
			// this cancel took must not outlive it (drain D1): a later genuine
			// crash of this run has to leave its corpse.
			s.cancels.clearRequested(r.ID)
			return CancelOutcome{}, raced(err)
		}
		out.To, out.Applied = string(run.StateCompleted), true
		out.Detail = "running work cancelled: the run completes carrying the cancel reason (§14 reading 9)"
		s.settle(ctx, r.ID)
		return out, nil

	case run.StateParked:
		// finalize-with-card: no resume edge, generation unchanged. Any open
		// ask closes in the SAME transaction as the transition (the B2-5
		// choreography, §16) — a parked run's card must never outlive the run
		// it was parked on.
		err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
			if err := closeOpenAsksTx(ctx, tx, r.ID, actor, s.now()); err != nil {
				return err
			}
			_, err := s.cfg.Runs.TransitionTx(ctx, tx, r.ID, run.StateFinalized, run.TransitionOptions{
				Reason: reason, Actor: actor, Detail: cancelDetail(actor, false),
			})
			return err
		})
		if err != nil {
			return CancelOutcome{}, raced(err)
		}
		out.To, out.Applied = string(run.StateFinalized), true
		out.Detail = "parked work cancelled: finalize-with-card, no resume edge (§14 reading 9)"
		s.settle(ctx, r.ID)
		return out, nil

	case run.StateQueued:
		// The OQ1 edge. The queue row settles in the SAME transaction, so the
		// claim loop can never pick up a run whose cancellation has committed —
		// which means a process with no scheduler bound cannot serve this
		// cancel at all. Refusing FAIL-CLOSED is the only honest answer: a
		// transition committed without its queue settle is exactly the
		// half-commit that leaves a finalized run claimable (drain D11).
		if s.sched == nil {
			return out, ErrCancelQueueUnsettleable
		}
		err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
			if _, err := s.cfg.Runs.TransitionTx(ctx, tx, r.ID, run.StateFinalized, run.TransitionOptions{
				Reason: reason, Actor: actor, Detail: cancelDetail(actor, false),
			}); err != nil {
				return err
			}
			return s.sched.SettleQueuedTx(ctx, tx, r.ID)
		})
		if err != nil {
			return CancelOutcome{}, raced(err)
		}
		out.To, out.Applied = string(run.StateFinalized), true
		out.Detail = "queued work cancelled before it ran: finalize-with-card, queue row settled in the same transaction (OQ1)"
		return out, nil
	}
	return out, fmt.Errorf("stage: run %q is in unmapped state %s", r.ID, r.State)
}

// runLadder marks the human cancel AT THE RUN'S CURRENT GENERATION and runs
// the S03.1 ladder on the live session when one is registered in THIS process.
// A run with no registered session is not an error: it may be between stage
// sessions, or its session may have already ended — the terminal transition is
// the record either way, and the mark is still taken so the leg's own unwind
// files no corpse.
func (s *Skeleton) runLadder(ctx context.Context, r run.Run) bool {
	runID := r.ID
	sess := s.cancels.markRequested(runID, r.Generation)
	if sess == nil {
		return false
	}
	if err := sess.Cancel(ctx); err != nil {
		// The ladder is best-effort: its failure is loud, and the run still
		// ends. A cancel that could not reach the engine must never leave the
		// run looking live.
		s.logger().Error("stage: cancel ladder", "run", runID, "err", err)
		return false
	}
	s.logger().Info("stage: cancel ladder invoked on the live session (S03.1)", "run", runID)
	return true
}

func cancelDetail(actor string, ladder bool) json.RawMessage {
	b, err := json.Marshal(struct {
		Cause         string `json:"cause"`
		Actor         string `json:"actor"`
		LadderInvoked bool   `json:"ladder_invoked"`
	}{"human cancel (4.5)", actor, ladder})
	if err != nil {
		return nil
	}
	return b
}

// closeOpenAsksTx closes every open ask of a run as cancelled, inside the
// caller's transaction. The answer body records who cancelled, so the closed
// card says why it will never be answered.
func closeOpenAsksTx(ctx context.Context, tx *sql.Tx, runID, actor string, now time.Time) error {
	answer, err := json.Marshal(struct {
		Action string `json:"action"`
		Actor  string `json:"actor"`
		Reason string `json:"reason"`
	}{"cancel", actor, "the run was cancelled (4.5); the card was never answered"})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE asks SET status = 'cancelled', answered_ts = ?, answer = ?
		  WHERE run_id = ? AND status = 'open'`,
		now.UTC().Format(time.RFC3339Nano), string(answer), runID); err != nil {
		return fmt.Errorf("stage: close open asks of %q: %w", runID, err)
	}
	return nil
}
